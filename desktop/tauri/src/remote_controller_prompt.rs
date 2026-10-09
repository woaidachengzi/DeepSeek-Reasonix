use super::*;

const UNKNOWN: &str = "remote decision outcome is unknown; refresh the selected session and do not automatically retry";

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct SessionPromptRequest {
    pub controller_id: String,
    pub session_path: String,
    pub runtime_epoch: String,
    pub turn_id: String,
    pub prompt_id: String,
    pub prompt_runtime_epoch: String,
    pub kind: String,
    pub answer: Value,
}

fn text(value: &str, budget: usize) -> bool {
    value.len() <= budget && !value.contains('\0')
}

// Generated mirrors preserve typed fields; runtime validation owns the strict
// kind/answer union, byte budgets and authority-sensitive grant combinations.
fn answer(kind: &str, value: &Value) -> bool {
    let Some(fields) = value.as_object() else {
        return false;
    };
    let allowed: &[&str] = match kind {
        "ask" => &["questions"],
        "approval" => &["allow", "session", "persist"],
        "plan" | "recovery" => &["action", "feedback"],
        "mcp" => &["action", "content"],
        _ => return false,
    };
    if fields
        .iter()
        .any(|(key, v)| !allowed.contains(&key.as_str()) || v.is_null())
        || serde_json::to_vec(value).map_or(true, |v| v.len() > 128 << 10)
    {
        return false;
    }
    match kind {
        "ask" => {
            let Ok(a) =
                serde_json::from_value::<BridgeRemoteControllerPromptAskAnswer>(value.clone())
            else {
                return false;
            };
            let Some(raw) = fields["questions"].as_array() else {
                return false;
            };
            let mut ids = HashSet::new();
            a.questions.len() <= 32
                && a.questions.iter().zip(raw).all(|(q, raw)| {
                    raw.as_object().is_some_and(|o| {
                        o.len() == 2 && o.contains_key("questionId") && o.contains_key("selected")
                    }) && !q.question_id.is_empty()
                        && clean(&q.question_id, 4096)
                        && ids.insert(q.question_id.clone())
                        && q.selected.len() <= 64
                        && q.selected.iter().all(|v| text(v, 8192))
                })
        }
        "approval" => {
            let Ok(a) =
                serde_json::from_value::<BridgeRemoteControllerPromptApprovalAnswer>(value.clone())
            else {
                return false;
            };
            let session = a.session.unwrap_or(false);
            let persist = a.persist.unwrap_or(false);
            !(session && persist || !a.allow && (session || persist))
        }
        "plan" => {
            let Ok(a) =
                serde_json::from_value::<BridgeRemoteControllerPromptPlanAnswer>(value.clone())
            else {
                return false;
            };
            matches!(
                a.action.as_str(),
                "start_execution" | "revise_plan" | "exit_plan"
            ) && a.feedback.as_deref().is_none_or(|v| text(v, 4096))
        }
        "recovery" => {
            let Ok(a) =
                serde_json::from_value::<BridgeRemoteControllerPromptRecoveryAnswer>(value.clone())
            else {
                return false;
            };
            matches!(a.action.as_str(), "continue" | "continue_task" | "revise")
                && a.feedback.as_deref().is_none_or(|v| text(v, 4096))
        }
        "mcp" => {
            let Ok(a) =
                serde_json::from_value::<BridgeRemoteControllerPromptMcpAnswer>(value.clone())
            else {
                return false;
            };
            matches!(a.action.as_str(), "accept" | "decline" | "cancel")
                && a.content.as_ref().is_none_or(|v| {
                    a.action == "accept"
                        && v.is_object()
                        && serde_json::to_vec(v).is_ok_and(|b| b.len() <= 64 << 10)
                })
        }
        _ => false,
    }
}

impl RemoteControllerClient {
    pub fn session_prompt(
        &self,
        request: SessionPromptRequest,
    ) -> Result<BridgeRemoteControllerSessionPromptResponse, String> {
        if [
            (&request.session_path, 32768),
            (&request.runtime_epoch, 4096),
            (&request.turn_id, 4096),
            (&request.prompt_id, 4096),
        ]
        .iter()
        .any(|(v, limit)| v.is_empty() || !clean(v, *limit))
            || !clean(&request.prompt_runtime_epoch, 4096)
            || !answer(&request.kind, &request.answer)
        {
            return Err(INVALID.into());
        }
        let route = format!("{}/session-prompt", path(&request.controller_id)?);
        let body = json!(BridgeRemoteControllerSessionPromptRequest {
            session_path: request.session_path.clone(),
            runtime_epoch: request.runtime_epoch.clone(),
            turn_id: request.turn_id.clone(),
            prompt_id: request.prompt_id.clone(),
            prompt_runtime_epoch: request.prompt_runtime_epoch.clone(),
            kind: request.kind.clone(),
            answer: request.answer,
        });
        if serde_json::to_vec(&body).map_or(true, |v| v.len() > 256 << 10) {
            return Err(INVALID.into());
        }
        // One dispatch. Generic HTTP failures cannot prove whether the remote
        // durable decision was committed; never fall back or automatically retry.
        let response: BridgeRemoteControllerSessionPromptResponse = self
            .request("POST", &route, Some(body), 27)
            .map_err(|_| UNKNOWN.to_string())?;
        let receipt = &response.receipt;
        if !view(&response.controller)
            || response.controller.id != request.controller_id
            || receipt.protocol_version != 1
            || receipt.session_path != request.session_path
            || receipt.runtime_epoch != request.runtime_epoch
            || receipt.turn_id != request.turn_id
            || receipt.prompt_id != request.prompt_id
            || receipt.prompt_runtime_epoch != request.prompt_runtime_epoch
            || receipt.kind != request.kind
            || !receipt.resolved
            || serde_json::to_vec(&response).map_or(true, |v| v.len() > 128 << 10)
        {
            return Err(UNKNOWN.into());
        }
        // The decision receipt is not a fabricated turn/save completion.
        Ok(response)
    }
}

#[cfg(test)]
#[path = "remote_controller_prompt_tests.rs"]
mod tests;
