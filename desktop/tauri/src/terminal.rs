//! Typed terminal IPC. The client captures a sidecar connection briefly, then
//! performs bounded HTTP work off the WebView thread without holding the
//! supervisor's process lock. It never accepts an executable, argv or env.
use crate::bridge::{request_json_with_timeout, session_path_component};
use crate::protocol_generated::*;
use base64::{engine::general_purpose::STANDARD, Engine};
use serde::{de::DeserializeOwned, Deserialize};
use serde_json::{json, Value};
use std::{collections::HashSet, net::SocketAddr, time::Duration};

const VERSION: u64 = 1;
const MAX_INPUT: usize = 64 << 10;
const MAX_OUTPUT: usize = 128 << 10;
const MAX_EVENT: usize = 8 << 10;
const MAX_JS_INTEGER: u64 = 9_007_199_254_740_991;
const INVALID: &str =
    "invalid terminal request; refresh the active session and use an installed shell";
const BAD_RESPONSE: &str = "invalid terminal response; refresh the active session";

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct WorkspaceRequest {
    pub session_id: String,
}
#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct TargetRequest {
    pub session_id: String,
    pub terminal_id: String,
}
#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CreateRequest {
    pub session_id: String,
    pub request_id: String,
    #[serde(default)]
    pub path: String,
    #[serde(default)]
    pub shell_id: String,
}
#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct InputRequest {
    pub session_id: String,
    pub terminal_id: String,
    pub request_id: String,
    pub data: String,
}
#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ResizeRequest {
    pub session_id: String,
    pub terminal_id: String,
    pub request_id: String,
    pub cols: u64,
    pub rows: u64,
}
#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct RenameRequest {
    pub session_id: String,
    pub terminal_id: String,
    pub request_id: String,
    pub title: String,
}
#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CloseRequest {
    pub session_id: String,
    pub terminal_id: String,
    pub request_id: String,
}

pub fn ensure_main_window(label: &str) -> Result<(), String> {
    if label == "main" {
        Ok(())
    } else {
        Err("integrated terminals are only available in the main window".into())
    }
}

fn terminal_id(id: &str) -> Result<&str, String> {
    if id.len() == 32
        && id
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
    {
        Ok(id)
    } else {
        Err(INVALID.into())
    }
}
fn request_id(id: &str) -> Result<(), String> {
    if !id.is_empty()
        && id.len() <= 128
        && id
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
    {
        Ok(())
    } else {
        Err(INVALID.into())
    }
}
fn workspace_path(session: &str) -> Result<String, String> {
    Ok(format!(
        "/v1/sessions/{}/terminal",
        session_path_component(session)?
    ))
}
fn target_path(session: &str, terminal: &str) -> Result<String, String> {
    Ok(format!(
        "{}/{}",
        workspace_path(session)?,
        terminal_id(terminal)?
    ))
}
fn shell_id(id: &str) -> bool {
    matches!(
        id,
        "" | "default"
            | "auto"
            | "sh"
            | "zsh"
            | "bash"
            | "fish"
            | "powershell"
            | "pwsh"
            | "windows-powershell"
            | "cmd"
    )
}
fn valid_exit(code: i64) -> bool {
    (-2_147_483_648..=4_294_967_295).contains(&code)
}
fn validate_session(session: &BridgeTerminalSessionView) -> Result<(), String> {
    if terminal_id(&session.id).is_err()
        || session.created_at > MAX_JS_INTEGER
        || session.title.chars().count() > 80
        || session.title.chars().any(char::is_control)
        || session.cwd.len() > 32768
        || session.shell.len() > 1024
        || session.exit_code.is_some_and(|code| !valid_exit(code))
    {
        return Err(BAD_RESPONSE.into());
    }
    Ok(())
}
pub fn validate_output(output: &BridgeTerminalOutputView, max: usize) -> Result<(), String> {
    if terminal_id(&output.id).is_err()
        || output.end > MAX_JS_INTEGER
        || output.end < output.start
        || output.data.len() > max.div_ceil(3) * 4
        || output.data.contains(['\r', '\n'])
    {
        return Err(BAD_RESPONSE.into());
    }
    let bytes = STANDARD
        .decode(&output.data)
        .map_err(|_| BAD_RESPONSE.to_string())?;
    if bytes.len() > max || output.end - output.start != bytes.len() as u64 {
        return Err(BAD_RESPONSE.into());
    }
    Ok(())
}

/// Return terminal kinds separately, including invalid frames (which must be
/// dropped rather than reaching Agent consumers). Canonical serialization drops
/// unknown fields; terminal payloads contain only typed byte/exit information.
pub fn terminal_event(event: &BridgeEvent) -> Option<Result<BridgeEvent, String>> {
    let mut clean = event.clone();
    let result = match event.event_kind.as_str() {
        "terminal_output" => (|| {
            let output: BridgeTerminalOutputView = serde_json::from_value(event.payload.clone())
                .map_err(|_| BAD_RESPONSE.to_string())?;
            validate_output(&output, MAX_EVENT)?;
            if output.start == output.end {
                return Err(BAD_RESPONSE.into());
            }
            clean.payload = serde_json::to_value(output).map_err(|_| BAD_RESPONSE.to_string())?;
            Ok(clean.clone())
        })(),
        "terminal_exit" => (|| {
            let exit: BridgeTerminalExitView = serde_json::from_value(event.payload.clone())
                .map_err(|_| BAD_RESPONSE.to_string())?;
            if terminal_id(&exit.id).is_err() || !valid_exit(exit.exit_code) {
                return Err(BAD_RESPONSE.into());
            }
            clean.payload = serde_json::to_value(exit).map_err(|_| BAD_RESPONSE.to_string())?;
            Ok(clean.clone())
        })(),
        _ => return None,
    };
    Some(result.and_then(|clean| {
        if clean.protocol_version != VERSION
            || clean.sequence > MAX_JS_INTEGER
            || session_path_component(&clean.session_id).is_err()
        {
            Err(BAD_RESPONSE.into())
        } else {
            Ok(clean)
        }
    }))
}

#[derive(Clone)]
pub struct TerminalClient {
    address: SocketAddr,
    token: String,
}
impl TerminalClient {
    pub(crate) fn new(address: SocketAddr, token: String) -> Self {
        Self { address, token }
    }

    fn request<T: DeserializeOwned>(
        &self,
        method: &str,
        path: &str,
        body: Option<Value>,
        id: Option<&str>,
    ) -> Result<T, String> {
        if let Some(id) = id {
            request_id(id)?;
        }
        let send = || {
            request_json_with_timeout(
                self.address,
                &self.token,
                method,
                path,
                body.clone(),
                id,
                Duration::from_secs(5),
            )
        };
        // Only repeat a mutation with exactly the same body and request ID.
        let response = send().or_else(|first| {
            if id.is_some() {
                send().map_err(|_| first)
            } else {
                Err(first)
            }
        })?;
        if response.get("protocolVersion").and_then(Value::as_u64) != Some(VERSION) {
            return Err(BAD_RESPONSE.into());
        }
        serde_json::from_value(response).map_err(|_| BAD_RESPONSE.into())
    }

    pub fn workspace(
        &self,
        request: WorkspaceRequest,
    ) -> Result<BridgeTerminalWorkspaceResponse, String> {
        let envelope: BridgeTerminalWorkspaceResponse =
            self.request("GET", &workspace_path(&request.session_id)?, None, None)?;
        let ws = &envelope.workspace;
        let mut ids = HashSet::new();
        if ws.sessions.len() > 10
            || ws.shells.len() > 10
            || ws.reason.as_ref().is_some_and(|v| v.len() > 4096)
        {
            return Err(BAD_RESPONSE.into());
        }
        for session in &ws.sessions {
            validate_session(session)?;
            if !ids.insert(&session.id) {
                return Err(BAD_RESPONSE.into());
            }
        }
        let mut shells = HashSet::new();
        for shell in &ws.shells {
            if shell.id.is_empty()
                || !shell_id(&shell.id)
                || shell.label.len() > 1024
                || !shells.insert(&shell.id)
            {
                return Err(BAD_RESPONSE.into());
            }
        }
        Ok(envelope)
    }

    pub fn create(&self, request: CreateRequest) -> Result<BridgeTerminalSessionResponse, String> {
        if request.path.len() > 4096 || request.path.contains('\0') || !shell_id(&request.shell_id)
        {
            return Err(INVALID.into());
        }
        let envelope: BridgeTerminalSessionResponse = self.request(
            "POST",
            &workspace_path(&request.session_id)?,
            Some(json!(BridgeTerminalCreateRequest {
                path: Some(request.path),
                shell_id: Some(request.shell_id)
            })),
            Some(&request.request_id),
        )?;
        validate_session(&envelope.terminal)?;
        Ok(envelope)
    }
    pub fn output(&self, request: TargetRequest) -> Result<BridgeTerminalOutputResponse, String> {
        let envelope: BridgeTerminalOutputResponse = self.request(
            "GET",
            &format!(
                "{}/output",
                target_path(&request.session_id, &request.terminal_id)?
            ),
            None,
            None,
        )?;
        validate_output(&envelope.output, MAX_OUTPUT)?;
        if envelope.output.id != request.terminal_id {
            return Err(BAD_RESPONSE.into());
        }
        Ok(envelope)
    }
    fn action(
        &self,
        method: &str,
        session: &str,
        terminal: &str,
        suffix: &str,
        body: Option<Value>,
        id: &str,
    ) -> Result<BridgeTerminalActionResponse, String> {
        let envelope: BridgeTerminalActionResponse = self.request(
            method,
            &format!("{}{}", target_path(session, terminal)?, suffix),
            body,
            Some(id),
        )?;
        if !envelope.accepted || envelope.terminal_id != terminal {
            return Err(BAD_RESPONSE.into());
        }
        Ok(envelope)
    }
    pub fn input(&self, request: InputRequest) -> Result<BridgeTerminalActionResponse, String> {
        if request.data.len() > MAX_INPUT.div_ceil(3) * 4 || request.data.contains(['\r', '\n']) {
            return Err(INVALID.into());
        }
        let bytes = STANDARD
            .decode(&request.data)
            .map_err(|_| INVALID.to_string())?;
        if bytes.is_empty() || bytes.len() > MAX_INPUT {
            return Err(INVALID.into());
        }
        self.action(
            "POST",
            &request.session_id,
            &request.terminal_id,
            "/input",
            Some(json!(BridgeTerminalInputRequest { data: request.data })),
            &request.request_id,
        )
    }
    pub fn resize(&self, request: ResizeRequest) -> Result<BridgeTerminalActionResponse, String> {
        if !(1..=1000).contains(&request.cols) || !(1..=500).contains(&request.rows) {
            return Err(INVALID.into());
        }
        self.action(
            "POST",
            &request.session_id,
            &request.terminal_id,
            "/resize",
            Some(json!(BridgeTerminalResizeRequest {
                cols: request.cols,
                rows: request.rows
            })),
            &request.request_id,
        )
    }
    pub fn rename(&self, request: RenameRequest) -> Result<BridgeTerminalActionResponse, String> {
        if request.title.trim().is_empty()
            || request.title.chars().count() > 80
            || request.title.chars().any(char::is_control)
        {
            return Err(INVALID.into());
        }
        self.action(
            "PATCH",
            &request.session_id,
            &request.terminal_id,
            "/title",
            Some(json!(BridgeTerminalRenameRequest {
                title: request.title
            })),
            &request.request_id,
        )
    }
    pub fn close(&self, request: CloseRequest) -> Result<BridgeTerminalActionResponse, String> {
        self.action(
            "DELETE",
            &request.session_id,
            &request.terminal_id,
            "",
            None,
            &request.request_id,
        )
    }
}

#[cfg(test)]
#[path = "terminal_tests.rs"]
mod tests;
