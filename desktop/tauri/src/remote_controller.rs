//! Host-only typed remote catalogue IPC. No arbitrary URL/token/session path,
//! no local RuntimeManager operation, and no supervisor lock during HTTP I/O.
use crate::protocol_generated::*;
use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use serde::{de::DeserializeOwned, Deserialize};
use serde_json::{json, Value};
use std::{collections::HashSet, net::SocketAddr, time::Duration};

const INVALID: &str = "invalid remote controller request; select a saved host and workspace";
const FAILED: &str =
    "remote controller is unavailable; reconnect the saved SSH host and reopen the workspace";
const MAX_JS: u64 = 9_007_199_254_740_991;

#[path = "remote_controller_image.rs"]
mod image;
pub use image::SessionImageRequest;

#[path = "remote_controller_cancel.rs"]
mod cancel;
pub use cancel::SessionCancelRequest;

#[path = "remote_controller_submit.rs"]
mod submit;
pub use submit::SessionSubmitRequest;

#[path = "remote_controller_prompt.rs"]
mod prompt;
pub use prompt::SessionPromptRequest;

#[path = "remote_controller_events.rs"]
mod events;

#[path = "remote_controller_event_payload.rs"]
mod event_payload;

#[path = "remote_controller_projection.rs"]
mod projection;

#[path = "remote_controller_subscriptions.rs"]
mod subscriptions;
pub(crate) use subscriptions::{
    OwnerRevocation, RemoteSubscriptions, SnapshotOperation, SnapshotRequest, SnapshotResponse,
    SubscribeRequest, SubscriptionIdentity, SubscriptionOperation,
};

#[path = "remote_controller_subscription_worker.rs"]
mod subscription_worker;
pub(crate) use subscription_worker::{run_subscription, UnsubscribeRequest};

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct AttachRequest {
    pub name: String,
    pub workspace: String,
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct HandleRequest {
    pub controller_id: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct SessionViewRequest {
    pub controller_id: String,
    pub session_path: String,
}

pub fn ensure_main_window(label: &str) -> Result<(), String> {
    if label == "main" {
        Ok(())
    } else {
        Err(INVALID.into())
    }
}
fn handle(id: &str) -> bool {
    id.len() == 22
        && URL_SAFE_NO_PAD
            .decode(id)
            .is_ok_and(|bytes| bytes.len() == 16 && URL_SAFE_NO_PAD.encode(bytes) == id)
}
fn clean(value: &str, limit: usize) -> bool {
    value.len() <= limit && !value.chars().any(char::is_control)
}
fn name(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 64
        && value.as_bytes()[0].is_ascii_alphanumeric()
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
}
fn view(value: &BridgeRemoteControllerView) -> bool {
    handle(&value.id)
        && name(&value.name)
        && !value.workspace.is_empty()
        && clean(&value.workspace, 4096)
        && value.read_only
}
fn path(id: &str) -> Result<String, String> {
    if !handle(id) {
        return Err(INVALID.into());
    }
    Ok(format!("/v1/remote/controllers/{id}"))
}

#[derive(Clone)]
pub struct RemoteControllerClient {
    address: SocketAddr,
    token: String,
}
impl RemoteControllerClient {
    pub(crate) fn new(address: SocketAddr, token: String) -> Self {
        Self { address, token }
    }
    fn request<T: DeserializeOwned>(
        &self,
        method: &str,
        path: &str,
        body: Option<Value>,
        timeout: u64,
    ) -> Result<T, String> {
        self.request_connected(method, path, body, timeout, |_| Ok(()))
    }
    fn request_connected<T: DeserializeOwned, G>(
        &self,
        method: &str,
        path: &str,
        body: Option<Value>,
        timeout: u64,
        connected: impl FnOnce(&std::net::TcpStream) -> Result<G, String>,
    ) -> Result<T, String> {
        // Never pass a decoder or network/free-text error across the renderer
        // boundary: future Serve fields may contain credentials or paths.
        let response = crate::bridge::request_json_with_connected_socket(
            self.address,
            &self.token,
            method,
            path,
            body,
            None,
            Duration::from_secs(timeout),
            connected,
        )
        .map_err(|_| FAILED.to_string())?;
        if response.get("protocolVersion").and_then(Value::as_u64) != Some(1) {
            return Err(FAILED.into());
        }
        serde_json::from_value(response).map_err(|_| FAILED.into())
    }
    pub fn attach(&self, request: AttachRequest) -> Result<BridgeRemoteControllerResponse, String> {
        if !name(&request.name)
            || request.workspace.trim().is_empty()
            || !clean(&request.workspace, 4096)
        {
            return Err(INVALID.into());
        }
        let expected = request.name.clone();
        let response: BridgeRemoteControllerResponse = self.request(
            "POST",
            "/v1/remote/controllers",
            Some(json!(BridgeRemoteControllerRequest {
                name: request.name,
                workspace: request.workspace
            })),
            60,
        )?;
        if !view(&response.controller) || response.controller.name != expected {
            return Err(FAILED.into());
        }
        Ok(response)
    }
    pub fn sessions(
        &self,
        request: HandleRequest,
    ) -> Result<BridgeRemoteControllerSessionsResponse, String> {
        self.sessions_connected(request, |_| Ok(()))
    }
    fn sessions_connected<G>(
        &self,
        request: HandleRequest,
        connected: impl FnOnce(&std::net::TcpStream) -> Result<G, String>,
    ) -> Result<BridgeRemoteControllerSessionsResponse, String> {
        let response: BridgeRemoteControllerSessionsResponse = self.request_connected(
            "GET",
            &format!("{}/sessions", path(&request.controller_id)?),
            None,
            20,
            connected,
        )?;
        if !view(&response.controller)
            || response.controller.id != request.controller_id
            || response.sessions.len() > 10000
        {
            return Err(FAILED.into());
        }
        let mut seen = HashSet::new();
        let mut current = 0;
        for row in &response.sessions {
            if row.path.is_empty()
                || !clean(&row.path, 32768)
                || !clean(&row.name, 4096)
                || row.title.len() > 8192
                || row
                    .title
                    .chars()
                    .any(|r| r.is_control() && !matches!(r, '\r' | '\n' | '\t'))
                || row.turns > MAX_JS
                || row.mtime_milli > MAX_JS
                || !seen.insert(&row.path)
            {
                return Err(FAILED.into());
            }
            if row.current {
                current += 1;
            }
        }
        if current > 1 {
            return Err(FAILED.into());
        }
        Ok(response)
    }
    pub fn close(
        &self,
        request: HandleRequest,
    ) -> Result<BridgeRemoteControllerCloseResponse, String> {
        let response: BridgeRemoteControllerCloseResponse =
            self.request("DELETE", &path(&request.controller_id)?, None, 5)?;
        if !response.closed {
            return Err(FAILED.into());
        }
        Ok(response)
    }

    pub fn session_view(
        &self,
        request: SessionViewRequest,
    ) -> Result<BridgeRemoteControllerSessionViewResponse, String> {
        if request.session_path.is_empty() || !clean(&request.session_path, 32768) {
            return Err(INVALID.into());
        }
        let route = format!("{}/session-view", path(&request.controller_id)?);
        let response: BridgeRemoteControllerSessionViewResponse = self.request(
            "POST",
            &route,
            Some(json!(BridgeRemoteControllerSessionViewRequest {
                session_path: request.session_path.clone()
            })),
            25,
        )?;
        if !view(&response.controller)
            || response.controller.id != request.controller_id
            || !valid_session_view(&response.view, &request.session_path)
        {
            return Err(FAILED.into());
        }
        Ok(response)
    }
}

fn valid_session_view(v: &BridgeRemoteControllerSessionView, expected: &str) -> bool {
    if v.protocol_version != 1
        || v.session_path != expected
        || !v.read_only
        || v.history.len() > 100000
        || !clean(&v.model_ref, 4096)
        || !clean(&v.label, 4096)
    {
        return false;
    }
    match v.ownership.as_str() {
        "serve" => {
            if v.runtime_state.is_none() {
                return false;
            }
        }
        "saved" | "external" => {
            if v.runtime_state.is_some() || !v.model_ref.is_empty() || !v.label.is_empty() {
                return false;
            }
        }
        _ => return false,
    }
    if let Some(s) = &v.runtime_state {
        if !valid_runtime_state(s) {
            return false;
        }
    }
    valid_history(&v.history)
}

fn valid_history(history: &[BridgeRemoteControllerHistoryMessage]) -> bool {
    if history.len() > 100000 {
        return false;
    }
    let mut seen = HashSet::new();
    for m in history {
        if m.id.is_empty() || !clean(&m.id, 4096) || !seen.insert(&m.id) {
            return false;
        }
        if !matches!(
            m.role.as_str(),
            "user" | "assistant" | "tool" | "notice" | "protocol_recovery" | "final_readiness"
        ) || m.missing.as_ref().is_some_and(|v| v.len() > 10000)
            || m.tool_call_id.as_ref().is_some_and(|v| !clean(v, 4096))
            || m.tool_name.as_ref().is_some_and(|v| !clean(v, 4096))
        {
            return false;
        }
        if let Some(calls) = &m.tool_calls {
            if calls.len() > 10000
                || calls.iter().any(|c| {
                    c.id.is_empty()
                        || c.name.is_empty()
                        || !clean(&c.id, 4096)
                        || !clean(&c.name, 4096)
                })
            {
                return false;
            }
        }
        if m.protocol_recovery
            .as_ref()
            .is_some_and(|v| v.id.is_empty() || !clean(&v.id, 4096))
        {
            return false;
        }
        if let Some(searches) = &m.server_search {
            if searches.len() > 10000 {
                return false;
            }
            for search in searches {
                if !clean(&search.id, 4096)
                    || search
                        .sources_status
                        .as_ref()
                        .is_some_and(|s| !clean(s, 4096))
                {
                    return false;
                }
                if let Some(hits) = &search.results {
                    if hits.len() > 10000
                        || hits
                            .iter()
                            .any(|h| h.url.as_ref().is_some_and(|s| !clean(s, 32768)))
                    {
                        return false;
                    }
                }
            }
        }
    }
    true
}

fn valid_turn_status(status: &str) -> bool {
    matches!(
        status,
        "" | "queued"
            | "in_progress"
            | "waiting_user"
            | "cancelling"
            | "completed"
            | "interrupted"
            | "failed"
            | "protocol_failed"
    )
}

fn valid_runtime_state(s: &BridgeRemoteControllerRuntimeState) -> bool {
    s.schema_version <= 1
        && s.revision <= MAX_JS
        && s.turn_event_seq <= MAX_JS
        && s.background_jobs <= MAX_JS
        && clean(&s.runtime_epoch, 4096)
        && clean(&s.turn_id, 4096)
        && clean(&s.activity, 8192)
        && matches!(
            s.phase.as_str(),
            "idle" | "executing" | "finishing" | "closed"
        )
        && valid_turn_status(&s.turn_status)
}

#[cfg(test)]
#[path = "remote_controller_tests.rs"]
mod tests;

#[cfg(test)]
#[path = "remote_controller_view_tests.rs"]
mod view_tests;
