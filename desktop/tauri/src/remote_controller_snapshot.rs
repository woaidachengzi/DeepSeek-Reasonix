//! One cancellable display read per ready live subscription. Receipts and
//! caller-provided owner/generation/path values cannot grant snapshot access.
use super::*;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct SnapshotRequest {
    pub subscription_id: String,
    pub continuation: Option<String>,
}

#[derive(serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct SnapshotResponse {
    pub protocol_version: u64,
    pub subscription: SubscriptionIdentity,
    pub snapshot: BridgeRemoteControllerSessionProjectionResponse,
}

pub(crate) struct SnapshotOperation {
    state: Arc<Mutex<State>>,
    entry: Arc<Entry>,
    cancellation: Arc<EventCancellation>,
    continuation: Option<String>,
}

impl RemoteSubscriptions {
    pub(crate) fn reserve_snapshot(
        &self,
        label: &str,
        owner: &str,
        request: SnapshotRequest,
    ) -> Result<SnapshotOperation, String> {
        ensure_main_window(label)?;
        if !handle(&request.subscription_id)
            || request.continuation.as_ref().is_some_and(|id| !handle(id))
        {
            return Err(INVALID.into());
        }
        let state = self.state.lock().map_err(|_| FAILED.to_string())?;
        if state.owner.as_deref() != Some(owner) {
            return Err(FAILED.into());
        }
        let entry = state
            .current
            .values()
            .find(|entry| entry.identity.subscription_id == request.subscription_id)
            .ok_or_else(|| FAILED.to_string())?;
        if !entry.ready.load(Ordering::Acquire) || entry.cancellation.is_closed() {
            return Err(
                "remote subscription is not ready; wait for the ready notice or reopen the session"
                    .into(),
            );
        }
        let cancellation = Arc::new(EventCancellation::default());
        {
            let mut active = entry.snapshot.lock().map_err(|_| FAILED.to_string())?;
            if active.is_some() {
                return Err("remote snapshot is busy; wait for the current read".into());
            }
            *active = Some(Arc::clone(&cancellation));
        }
        Ok(SnapshotOperation {
            state: Arc::clone(&self.state),
            entry: Arc::clone(entry),
            cancellation,
            continuation: request.continuation,
        })
    }
}

impl SnapshotOperation {
    fn with_current<T>(
        &self,
        publish: impl FnOnce(&SubscriptionIdentity) -> T,
    ) -> Result<T, String> {
        let state = self.state.lock().map_err(|_| FAILED.to_string())?;
        if state.owner.as_deref() != Some(self.entry.identity.sidecar_instance_id.as_str())
            || !state
                .current
                .get(&self.entry.identity.surface_id)
                .is_some_and(|entry| Arc::ptr_eq(entry, &self.entry))
            || self.entry.cancellation.is_closed()
            || self.cancellation.is_closed()
            || !self.entry.ready.load(Ordering::Acquire)
        {
            return Err(FAILED.into());
        }
        Ok(publish(&self.entry.identity))
    }
    pub(crate) fn read(&self, client: RemoteControllerClient) -> Result<SnapshotResponse, String> {
        let identity = self.with_current(Clone::clone)?;
        let snapshot = client.session_projection_cancellable(
            super::super::projection::SessionProjectionRequest {
                controller_id: identity.controller_id,
                session_path: identity.session_path,
                continuation: self.continuation.clone(),
            },
            (*self.cancellation).clone(),
        )?;
        self.with_current(|identity| SnapshotResponse {
            protocol_version: 1,
            subscription: identity.clone(),
            snapshot,
        })
    }
}

impl Drop for SnapshotOperation {
    fn drop(&mut self) {
        self.cancellation.close();
        if let Ok(mut current) = self.entry.snapshot.lock() {
            if current
                .as_ref()
                .is_some_and(|current| Arc::ptr_eq(current, &self.cancellation))
            {
                *current = None;
            }
        }
    }
}

#[cfg(test)]
#[path = "remote_controller_snapshot_tests.rs"]
mod tests;
