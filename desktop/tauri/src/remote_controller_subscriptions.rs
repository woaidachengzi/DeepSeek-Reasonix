//! Native owner/generation registry. No opaque provider payload is emitted here.
use super::{events::EventCancellation, *};
use rand::TryRngCore;
use std::{
    collections::HashMap,
    sync::{
        atomic::{AtomicBool, AtomicUsize, Ordering},
        Arc, Mutex, Weak,
    },
};

const CURRENT_MAX: usize = 16;
const LIVE_MAX: usize = 32; // Includes cancelled workers still holding a ticket.
const SURFACE_MAX: usize = 64; // Generation tombstones survive close until owner changes.
const BUSY: &str = "remote subscriptions are busy; close an unused session and retry";

#[path = "remote_controller_snapshot.rs"]
mod snapshot;
pub(crate) use snapshot::{SnapshotOperation, SnapshotRequest, SnapshotResponse};

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct SubscribeRequest {
    pub controller_id: String,
    pub session_path: String,
    pub surface_id: String,
    pub generation: u64,
}

#[derive(Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct SubscriptionIdentity {
    pub protocol_version: u64,
    pub subscription_id: String,
    pub surface_id: String,
    pub generation: u64,
    pub controller_id: String,
    pub session_path: String,
    pub sidecar_instance_id: String,
}

struct Entry {
    identity: SubscriptionIdentity,
    cancellation: EventCancellation,
    live: Arc<AtomicUsize>,
    ready: AtomicBool,
    snapshot: Mutex<Option<Arc<EventCancellation>>>,
}
impl Entry {
    fn retire(&self) {
        self.cancellation.close();
        if let Ok(snapshot) = self.snapshot.lock() {
            if let Some(snapshot) = snapshot.as_ref() {
                snapshot.close();
            }
        }
    }
}
impl Drop for Entry {
    fn drop(&mut self) {
        self.retire();
        self.live.fetch_sub(1, Ordering::AcqRel);
    }
}
#[derive(Default)]
struct State {
    owner: Option<String>,
    current: HashMap<String, Arc<Entry>>,
    generations: HashMap<String, u64>,
}
#[derive(Default)]
pub(crate) struct RemoteSubscriptions {
    state: Arc<Mutex<State>>,
    live: Arc<AtomicUsize>,
}
// A process observer must never revoke a later sidecar owner. Weak ownership
// also prevents a child monitor from keeping the Supervisor registry alive.
pub(crate) struct OwnerRevocation {
    state: Weak<Mutex<State>>,
    owner: String,
}
impl std::fmt::Debug for OwnerRevocation {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("OwnerRevocation")
    }
}
impl OwnerRevocation {
    pub(crate) fn revoke(self) {
        let Some(state) = self.state.upgrade() else {
            return;
        };
        let retired = if let Ok(mut state) = state.lock() {
            if state.owner.as_deref() != Some(&self.owner) {
                return;
            }
            state.owner = None;
            std::mem::take(&mut state.current)
        } else {
            return;
        };
        for entry in retired.values() {
            entry.retire();
        }
    }
}
impl RemoteSubscriptions {
    pub(crate) fn owner_revocation(&self, owner: &str) -> OwnerRevocation {
        OwnerRevocation {
            state: Arc::downgrade(&self.state),
            owner: owner.to_owned(),
        }
    }
    // Only BridgeSupervisor supplies this identity, never a renderer argument.
    pub(crate) fn set_owner(&self, owner: Option<&str>) {
        let retired = if let Ok(mut state) = self.state.lock() {
            if state.owner.as_deref() == owner {
                return;
            }
            state.owner = owner.map(str::to_owned);
            state.generations.clear();
            std::mem::take(&mut state.current)
        } else {
            return;
        };
        for entry in retired.values() {
            entry.retire();
        }
    }
    pub(crate) fn clear(&self) {
        let retired = if let Ok(mut state) = self.state.lock() {
            // Preserve generation tombstones: a delayed command cannot restore
            // the destroyed surface within the same sidecar owner.
            state.owner = None;
            std::mem::take(&mut state.current)
        } else {
            return;
        };
        for entry in retired.values() {
            entry.retire();
        }
    }
    pub(crate) fn reserve(
        &self,
        label: &str,
        owner: &str,
        request: SubscribeRequest,
    ) -> Result<SubscriptionOperation, String> {
        ensure_main_window(label)?;
        if !handle(&request.controller_id)
            || request.session_path.is_empty()
            || !clean(&request.session_path, 32768)
            || request.surface_id.is_empty()
            || !clean(&request.surface_id, 128)
            || request.generation == 0
            || request.generation > MAX_JS
        {
            return Err(INVALID.into());
        }
        let mut state = self.state.lock().map_err(|_| FAILED.to_string())?;
        if state.owner.as_deref() != Some(owner) {
            return Err(FAILED.into());
        }
        if state
            .generations
            .get(&request.surface_id)
            .is_some_and(|previous| request.generation <= *previous)
        {
            return Err(INVALID.into());
        }
        if !state.generations.contains_key(&request.surface_id)
            && state.generations.len() >= SURFACE_MAX
        {
            return Err(
                "remote surface limit reached; reuse an existing surface or restart the Preview"
                    .into(),
            );
        }
        if (!state.current.contains_key(&request.surface_id) && state.current.len() >= CURRENT_MAX)
            || self.live.load(Ordering::Acquire) >= LIVE_MAX
        {
            return Err(BUSY.into());
        }
        let mut random = [0; 16];
        rand::rngs::OsRng
            .try_fill_bytes(&mut random)
            .map_err(|_| FAILED.to_string())?;
        self.live.fetch_add(1, Ordering::AcqRel);
        let entry = Arc::new(Entry {
            identity: SubscriptionIdentity {
                protocol_version: 1,
                subscription_id: URL_SAFE_NO_PAD.encode(random),
                surface_id: request.surface_id.clone(),
                generation: request.generation,
                controller_id: request.controller_id,
                session_path: request.session_path,
                sidecar_instance_id: owner.to_string(),
            },
            cancellation: EventCancellation::default(),
            live: Arc::clone(&self.live),
            ready: AtomicBool::new(false),
            snapshot: Mutex::new(None),
        });
        state
            .generations
            .insert(request.surface_id.clone(), request.generation);
        let old = state.current.insert(request.surface_id, Arc::clone(&entry));
        drop(state);
        if let Some(old) = old {
            old.retire();
        }
        Ok(SubscriptionOperation {
            state: Arc::clone(&self.state),
            entry,
        })
    }
    pub(crate) fn close(&self, label: &str, id: &str) -> Result<(), String> {
        ensure_main_window(label)?;
        if !handle(id) {
            return Err(INVALID.into());
        }
        let retired = {
            let mut state = self.state.lock().map_err(|_| FAILED.to_string())?;
            let surface = state
                .current
                .iter()
                .find(|(_, entry)| entry.identity.subscription_id == id)
                .map(|(surface, _)| surface.clone());
            surface.and_then(|surface| state.current.remove(&surface))
        };
        if let Some(entry) = retired {
            entry.retire();
        }
        Ok(())
    }
}
impl Drop for RemoteSubscriptions {
    fn drop(&mut self) {
        self.set_owner(None);
    }
}

pub(crate) struct SubscriptionOperation {
    state: Arc<Mutex<State>>,
    entry: Arc<Entry>,
}
impl SubscriptionOperation {
    pub(crate) fn identity(&self) -> SubscriptionIdentity {
        self.entry.identity.clone()
    }
    pub(crate) fn cancellation(&self) -> EventCancellation {
        self.entry.cancellation.clone()
    }
    pub(crate) fn with_ready(
        &self,
        enqueue: impl FnOnce(&SubscriptionIdentity) -> Result<(), ()>,
    ) -> Result<Option<Result<(), ()>>, String> {
        self.with_current(|identity| {
            let result = enqueue(identity);
            if result.is_ok() {
                self.entry.ready.store(true, Ordering::Release);
            }
            result
        })
    }
    // Callback must be a nonblocking native enqueue, never network I/O. Its
    // admission is serialized with revoke/replacement. Renderer must still
    // reject already queued events carrying an obsolete subscription ID.
    pub(crate) fn with_current<T>(
        &self,
        publish: impl FnOnce(&SubscriptionIdentity) -> T,
    ) -> Result<Option<T>, String> {
        self.publish(false, publish)
    }
    // Transport failure closes its socket before the fixed terminal notice.
    // User/owner revoke removed the entry, so cannot publish even this notice.
    pub(crate) fn with_terminal<T>(
        &self,
        publish: impl FnOnce(&SubscriptionIdentity) -> T,
    ) -> Result<Option<T>, String> {
        self.publish(true, publish)
    }
    fn publish<T>(
        &self,
        terminal: bool,
        publish: impl FnOnce(&SubscriptionIdentity) -> T,
    ) -> Result<Option<T>, String> {
        let state = self.state.lock().map_err(|_| FAILED.to_string())?;
        let current = state
            .current
            .get(&self.entry.identity.surface_id)
            .is_some_and(|current| Arc::ptr_eq(current, &self.entry));
        if current
            && state.owner.as_deref() == Some(&self.entry.identity.sidecar_instance_id)
            && (terminal || !self.entry.cancellation.is_closed())
        {
            Ok(Some(publish(&self.entry.identity)))
        } else {
            Ok(None)
        }
    }
}
impl Drop for SubscriptionOperation {
    fn drop(&mut self) {
        self.entry.retire();
        if let Ok(mut state) = self.state.lock() {
            if state
                .current
                .get(&self.entry.identity.surface_id)
                .is_some_and(|current| Arc::ptr_eq(current, &self.entry))
            {
                state.current.remove(&self.entry.identity.surface_id);
            }
        }
    }
}

#[cfg(test)]
#[path = "remote_controller_subscriptions_tests.rs"]
mod tests;
