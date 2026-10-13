use crate::bridge::{session_path_component, BridgeSession};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashSet;

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct LiveAnswer {
    pub segments: Vec<String>,
    pub streaming: bool,
}
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct LiveTool {
    pub id: String,
    pub name: String,
    pub state: String,
}
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct LiveProgress {
    pub phase: String,
    pub tools: Vec<LiveTool>,
}
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct BridgeLiveState {
    pub active: bool,
    pub answer: LiveAnswer,
    pub progress: LiveProgress,
    pub truncated: bool,
}

pub fn parse_live_state(value: Option<&Value>) -> Result<Option<BridgeLiveState>, String> {
    let Some(value) = value.filter(|value| !value.is_null()) else {
        return Ok(None);
    };
    let state: BridgeLiveState =
        serde_json::from_value(value.clone()).map_err(|_| "invalid live session snapshot")?;
    if state.answer.segments.len() > 1
        || state
            .answer
            .segments
            .iter()
            .map(|text| text.chars().count())
            .sum::<usize>()
            > 65536
        || !matches!(
            state.progress.phase.as_str(),
            "thinking" | "answering" | "tool" | "finishing"
        )
        || state.progress.tools.len() > 30
        || state.progress.tools.iter().any(|tool| {
            tool.id.is_empty()
                || tool.id.chars().count() > 256
                || tool.name.chars().count() > 120
                || !matches!(tool.state.as_str(), "running" | "done" | "failed")
        })
    {
        return Err("invalid live session snapshot".into());
    }
    Ok(Some(state))
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeLiveSessions {
    pub protocol_version: u64,
    pub selected_session_id: String,
    pub sessions: Vec<BridgeSession>,
}

pub fn parse_live_sessions(value: Value) -> Result<BridgeLiveSessions, String> {
    let inventory: BridgeLiveSessions =
        serde_json::from_value(value).map_err(|_| "invalid live session inventory")?;
    let mut ids = HashSet::new();
    if inventory.protocol_version != 1 || inventory.sessions.len() > 16 {
        return Err("invalid live session inventory".into());
    }
    for session in &inventory.sessions {
        if session_path_component(&session.id)? != session.id
            || !ids.insert(&session.id)
            || !matches!(session.state.as_str(), "idle" | "running" | "paused")
        {
            return Err("invalid live session inventory".into());
        }
    }
    if !inventory.selected_session_id.is_empty() && !ids.contains(&inventory.selected_session_id) {
        return Err("invalid live session inventory".into());
    }
    Ok(inventory)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn snapshot_accepts_public_checkpoint_and_refuses_private_fields() {
        let mut public = json!({"active":true,"answer":{"segments":["answer"],"streaming":true},"progress":{"phase":"thinking","tools":[]},"truncated":false});
        assert!(parse_live_state(Some(&public)).unwrap().unwrap().active);
        public["reasoning"] = json!("private");
        assert!(parse_live_state(Some(&public)).is_err());
        public.as_object_mut().unwrap().remove("reasoning");
        public["answer"]["segments"] = json!(["a".repeat(65537)]);
        assert!(parse_live_state(Some(&public)).is_err());
        assert!(parse_live_state(None).unwrap().is_none());
    }

    #[test]
    fn inventory_rejects_duplicate_owners_and_unknown_selection() {
        let session = json!({"id":"a","path":"/sessions/a.jsonl","state":"running"});
        assert!(parse_live_sessions(
            json!({"protocolVersion":1,"selectedSessionId":"a","sessions":[session.clone()]})
        )
        .is_ok());
        assert!(parse_live_sessions(
            json!({"protocolVersion":1,"selectedSessionId":"b","sessions":[session.clone()]})
        )
        .is_err());
        assert!(parse_live_sessions(json!({"protocolVersion":1,"selectedSessionId":"a","sessions":[session.clone(),session]})).is_err());
    }
}
