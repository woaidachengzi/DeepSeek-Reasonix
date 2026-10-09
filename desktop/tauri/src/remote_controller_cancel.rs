use super::*;

const UNKNOWN: &str =
    "remote stop outcome is unknown; refresh the selected session and do not automatically retry";

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct SessionCancelRequest {
    pub controller_id: String,
    pub session_path: String,
    pub runtime_epoch: String,
    pub turn_id: String,
}

impl RemoteControllerClient {
    pub fn session_cancel(
        &self,
        request: SessionCancelRequest,
    ) -> Result<BridgeRemoteControllerSessionCancelResponse, String> {
        if request.session_path.is_empty()
            || !clean(&request.session_path, 32768)
            || request.runtime_epoch.is_empty()
            || !clean(&request.runtime_epoch, 4096)
            || request.turn_id.is_empty()
            || !clean(&request.turn_id, 4096)
        {
            return Err(INVALID.into());
        }
        let route = format!("{}/session-cancel", path(&request.controller_id)?);
        let body = json!(BridgeRemoteControllerSessionCancelRequest {
            session_path: request.session_path.clone(),
            runtime_epoch: request.runtime_epoch.clone(),
            turn_id: request.turn_id.clone(),
        });
        if serde_json::to_vec(&body).map_or(true, |v| v.len() > 40 << 10) {
            return Err(INVALID.into());
        }
        // The generic bridge transport intentionally hides remote diagnostics.
        // After dispatch, any unconfirmed/malformed result stays conservative:
        // never turn a transport error into success or advise an automatic retry.
        let response: BridgeRemoteControllerSessionCancelResponse = self
            .request("POST", &route, Some(body), 22)
            .map_err(|_| UNKNOWN.to_string())?;
        if !view(&response.controller)
            || response.controller.id != request.controller_id
            || response.receipt.protocol_version != 1
            || response.receipt.session_path != request.session_path
            || response.receipt.runtime_epoch != request.runtime_epoch
            || response.receipt.turn_id != request.turn_id
            || !response.receipt.cancelled
            || serde_json::to_vec(&response).map_or(true, |v| v.len() > 48 << 10)
        {
            return Err(UNKNOWN.into());
        }
        Ok(response)
    }
}

#[cfg(test)]
#[path = "remote_controller_cancel_tests.rs"]
mod tests;
