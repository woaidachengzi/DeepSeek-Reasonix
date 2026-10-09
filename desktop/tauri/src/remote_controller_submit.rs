use super::*;

const UNKNOWN: &str =
    "remote send outcome is unknown; refresh the selected session and do not automatically retry";

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct SessionSubmitRequest {
    pub controller_id: String,
    pub session_path: String,
    pub runtime_epoch: String,
    pub revision: u64,
    pub text: String,
}

impl RemoteControllerClient {
    pub fn session_submit(
        &self,
        request: SessionSubmitRequest,
    ) -> Result<BridgeRemoteControllerSessionSubmitResponse, String> {
        if request.session_path.is_empty()
            || !clean(&request.session_path, 32768)
            || request.runtime_epoch.is_empty()
            || !clean(&request.runtime_epoch, 4096)
            || request.revision == 0
            || request.revision > MAX_JS
            || request.text.trim().is_empty()
            || request.text.len() > 512 << 10
            || request.text.contains('\0')
        {
            return Err(INVALID.into());
        }
        let route = format!("{}/session-submit", path(&request.controller_id)?);
        let body = json!(BridgeRemoteControllerSessionSubmitRequest {
            session_path: request.session_path.clone(),
            runtime_epoch: request.runtime_epoch.clone(),
            revision: request.revision,
            text: request.text,
        });
        if serde_json::to_vec(&body).map_or(true, |v| v.len() > 1 << 20) {
            return Err(INVALID.into());
        }
        // One dispatch only. Generic HTTP errors hide private diagnostics and
        // cannot prove whether the remote user message was already admitted.
        let response: BridgeRemoteControllerSessionSubmitResponse = self
            .request("POST", &route, Some(body), 27)
            .map_err(|_| UNKNOWN.to_string())?;
        if !view(&response.controller)
            || response.controller.id != request.controller_id
            || response.receipt.protocol_version != 1
            || response.receipt.session_path != request.session_path
            || response.receipt.runtime_epoch != request.runtime_epoch
            || response.receipt.revision != request.revision
            || !response.receipt.accepted
            || serde_json::to_vec(&response).map_or(true, |v| v.len() > 48 << 10)
        {
            return Err(UNKNOWN.into());
        }
        // Accepted is admission only. Actual events own turn/save completion.
        Ok(response)
    }
}

#[cfg(test)]
#[path = "remote_controller_submit_tests.rs"]
mod tests;
