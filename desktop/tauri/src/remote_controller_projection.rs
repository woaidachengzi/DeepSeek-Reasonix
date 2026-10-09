//! Display-only snapshot transport. Not registered as IPC until the live
//! subscription's ready/owner/surface generation can fence snapshot delivery.
use super::*;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct SessionProjectionRequest {
    pub controller_id: String,
    pub session_path: String,
    pub continuation: Option<String>,
}

impl RemoteControllerClient {
    #[cfg(test)]
    pub(crate) fn session_projection(
        &self,
        request: SessionProjectionRequest,
    ) -> Result<BridgeRemoteControllerSessionProjectionResponse, String> {
        self.session_projection_cancellable(request, events::EventCancellation::default())
    }

    pub(crate) fn session_projection_cancellable(
        &self,
        request: SessionProjectionRequest,
        cancellation: events::EventCancellation,
    ) -> Result<BridgeRemoteControllerSessionProjectionResponse, String> {
        if cancellation.is_closed() {
            return Err(FAILED.into());
        }
        if request.session_path.is_empty()
            || !clean(&request.session_path, 32768)
            || request.continuation.as_ref().is_some_and(|id| !handle(id))
        {
            return Err(INVALID.into());
        }
        let route = format!("{}/session-projection", path(&request.controller_id)?);
        let initial = request.continuation.is_none();
        let mut response: BridgeRemoteControllerSessionProjectionResponse = self
            .request_connected(
                "POST",
                &route,
                Some(json!(BridgeRemoteControllerProjectionRequest {
                    session_path: request.session_path.clone(),
                    continuation: request.continuation,
                })),
                25,
                |socket| cancellation.attach(socket),
            )?;
        if !view(&response.controller) || response.controller.id != request.controller_id {
            return Err(FAILED.into());
        }
        validate(&mut response, &request.session_path, initial)?;
        if cancellation.is_closed() {
            return Err(FAILED.into());
        }
        Ok(response)
    }
}

fn validate(
    response: &mut BridgeRemoteControllerSessionProjectionResponse,
    expected: &str,
    initial: bool,
) -> Result<(), String> {
    let p = &mut response.projection;
    let r = &mut p.replay;
    let baseline = p.replay_after_seq;
    let turn = p.active_turn_id.as_deref().unwrap_or("");
    let status = p.turn_status.as_deref().unwrap_or("");
    if p.protocol_version != 1
        || p.session_path != expected
        || !p.read_only
        || p.initial != initial
        || p.page_token.is_some()
        || !valid_history(&p.history)
        || !valid_history(&p.user_suffix)
        || p.history.len() + p.user_suffix.len() > 100000
        || p.user_suffix.iter().any(|row| row.role != "user")
        || !clean(turn, 4096)
        || r.runtime_epoch
            .as_ref()
            .is_some_and(|epoch| !clean(epoch, 4096))
        || r.events.len() > 512
        || r.latest_seq > MAX_JS
        || r.floor_seq == 0
        || r.floor_seq > r.latest_seq + 1
        || baseline < r.floor_seq - 1
        || baseline > r.latest_seq
        || r.next_after_seq < baseline
        || r.next_after_seq > r.latest_seq
        || r.has_more != (r.next_after_seq < r.latest_seq)
        || r.has_more != response.next_page.is_some()
        || response.next_page.as_ref().is_some_and(|id| !handle(id))
        || (!initial && (!p.history.is_empty() || !p.user_suffix.is_empty() || r.events.is_empty()))
    {
        return Err(FAILED.into());
    }
    let mut ids = HashSet::new();
    for row in p.history.iter().chain(&p.user_suffix) {
        if !ids.insert(&row.id) {
            return Err(FAILED.into());
        }
    }
    match status {
        "" | "completed" | "interrupted" | "failed" | "protocol_failed" => {
            if !turn.is_empty()
                || !p.user_suffix.is_empty()
                || !r.events.is_empty()
                || r.has_more
                || baseline != r.latest_seq
            {
                return Err(FAILED.into());
            }
        }
        "queued" | "in_progress" | "waiting_user" | "cancelling" if !turn.is_empty() => {}
        _ => return Err(FAILED.into()),
    }
    let mut after = baseline;
    if !initial {
        after = r.events[0]
            .get("seq")
            .and_then(Value::as_u64)
            .and_then(|seq| seq.checked_sub(1))
            .ok_or_else(|| FAILED.to_string())?;
        if after < baseline || after > r.latest_seq {
            return Err(FAILED.into());
        }
    }
    for frame in &mut r.events {
        // The host HTTP body is byte-bounded. These are validation budgets
        // after Value decoding, not a claim of bounded peak allocation.
        if serde_json::to_vec(frame)
            .map_err(|_| FAILED.to_string())?
            .len()
            > 8 << 20
        {
            return Err(FAILED.into());
        }
        let safe = event_payload::project(frame)?;
        if safe.get("sessionPath").and_then(Value::as_str) != Some(expected)
            || safe.get("turnId").and_then(Value::as_str) != Some(turn)
            || safe.get("seq").and_then(Value::as_u64) != Some(after + 1)
            || !safe.get("status").and_then(Value::as_str).is_some_and(|s| {
                matches!(s, "queued" | "in_progress" | "waiting_user" | "cancelling")
            })
        {
            return Err(FAILED.into());
        }
        after += 1;
        *frame = safe;
    }
    if after != r.next_after_seq || (r.has_more && r.events.is_empty()) {
        return Err(FAILED.into());
    }
    Ok(())
}

#[cfg(test)]
#[path = "remote_controller_projection_tests.rs"]
mod tests;
