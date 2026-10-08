//! Host-only typed remote catalogue IPC. No arbitrary URL/token/session path,
//! no local RuntimeManager operation, and no supervisor lock during HTTP I/O.
use crate::bridge::request_json_with_timeout;
use crate::protocol_generated::*;
use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use serde::{de::DeserializeOwned, Deserialize};
use serde_json::{json, Value};
use std::{collections::HashSet, net::SocketAddr, time::Duration};

const INVALID: &str = "invalid remote controller request; select a saved host and workspace";
const FAILED: &str =
    "remote controller is unavailable; reconnect the saved SSH host and reopen the workspace";
const MAX_JS: u64 = 9_007_199_254_740_991;

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
        // Never pass a decoder or network/free-text error across the renderer
        // boundary: future Serve fields may contain credentials or paths.
        let response = request_json_with_timeout(
            self.address,
            &self.token,
            method,
            path,
            body,
            None,
            Duration::from_secs(timeout),
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
        let response: BridgeRemoteControllerSessionsResponse = self.request(
            "GET",
            &format!("{}/sessions", path(&request.controller_id)?),
            None,
            20,
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
}

#[cfg(test)]
#[path = "remote_controller_tests.rs"]
mod tests;
