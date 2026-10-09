//! Shape projection follows generated Go eventwire, not a hand-maintained
//! partial event mirror. Purpose-specific raw JSON stays data, never authority.
use super::*;
use std::{collections::BTreeMap, sync::OnceLock};

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Contract {
    version: u64,
    kinds: Vec<String>,
    event: Shape,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Shape {
    kind: String,
    #[serde(default)]
    nullable: bool,
    #[serde(default)]
    fields: BTreeMap<String, Field>,
    #[serde(default)]
    items: Option<Box<Shape>>,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Field {
    #[serde(default)]
    optional: bool,
    shape: Shape,
}

static CONTRACT: OnceLock<Result<Contract, ()>> = OnceLock::new();
const NODES: usize = 200_000;
const DEPTH: usize = 64;

fn count(value: &Value, depth: usize, remaining: &mut usize) -> Result<(), ()> {
    if depth > DEPTH || *remaining == 0 {
        return Err(());
    }
    *remaining -= 1;
    match value {
        Value::Array(values) => {
            if values.len() > 100_000 {
                return Err(());
            }
            for value in values {
                count(value, depth + 1, remaining)?;
            }
        }
        Value::Object(values) => {
            if values.len() > 10_000 {
                return Err(());
            }
            for value in values.values() {
                count(value, depth + 1, remaining)?;
            }
        }
        _ => {}
    }
    Ok(())
}
impl Shape {
    fn project(&self, value: &Value) -> Result<Value, ()> {
        if value.is_null() {
            return if self.nullable {
                Ok(Value::Null)
            } else {
                Err(())
            };
        }
        match self.kind.as_str() {
            "string" if value.is_string() => Ok(value.clone()),
            "boolean" if value.is_boolean() => Ok(value.clone()),
            "integer" if value.as_i64().is_some_and(|v| v.unsigned_abs() <= MAX_JS) => {
                Ok(value.clone())
            }
            "unsigned" if value.as_u64().is_some_and(|v| v <= MAX_JS) => Ok(value.clone()),
            "number" if value.as_f64().is_some_and(f64::is_finite) => Ok(value.clone()),
            "json" => Ok(value.clone()),
            "array" => {
                let values = value.as_array().ok_or(())?;
                let shape = self.items.as_ref().ok_or(())?;
                Ok(Value::Array(
                    values
                        .iter()
                        .map(|v| shape.project(v))
                        .collect::<Result<_, _>>()?,
                ))
            }
            "map" => {
                let values = value.as_object().ok_or(())?;
                let shape = self.items.as_ref().ok_or(())?;
                Ok(Value::Object(
                    values
                        .iter()
                        .map(|(k, v)| Ok((k.clone(), shape.project(v)?)))
                        .collect::<Result<_, ()>>()?,
                ))
            }
            "object" => {
                let values = value.as_object().ok_or(())?;
                let mut result = serde_json::Map::new();
                for (key, field) in &self.fields {
                    match values.get(key) {
                        Some(value) => {
                            let value = field.shape.project(value)?;
                            // Go omitempty pointers/slices are absent, not null.
                            if !field.optional || !value.is_null() {
                                result.insert(key.clone(), value);
                            }
                        }
                        None if !field.optional => return Err(()),
                        None => {}
                    }
                }
                Ok(Value::Object(result))
            }
            _ => Err(()),
        }
    }
}

fn identifier(value: &Value, key: &str, required: bool) -> bool {
    match value.get(key) {
        Some(value) => value
            .as_str()
            .is_some_and(|value| clean(value, 4096) && (!required || !value.is_empty())),
        None => !required,
    }
}

pub(super) fn project(value: &Value) -> Result<Value, String> {
    let contract = CONTRACT.get_or_init(|| {
        serde_json::from_str(include_str!("remote_event_contract.generated.json")).map_err(|_| ())
    });
    let contract = contract.as_ref().map_err(|_| FAILED.to_string())?;
    let mut remaining = NODES;
    if contract.version != 1 || count(value, 0, &mut remaining).is_err() {
        return Err(FAILED.into());
    }
    let mut result = contract
        .event
        .project(value)
        .map_err(|_| FAILED.to_string())?;
    let kind = result
        .get("kind")
        .and_then(Value::as_str)
        .ok_or_else(|| FAILED.to_string())?;
    if !contract.kinds.iter().any(|known| known == kind)
        || !["turnId", "itemId", "promptId", "messageId"]
            .iter()
            .all(|key| identifier(&result, key, false))
    {
        return Err(FAILED.into());
    }
    let admission = matches!(kind, "user_message_admitted" | "host_input_admitted");
    if kind != "steer" && !admission && result.get("messageId").is_some() {
        return Err(FAILED.into());
    }
    if admission && !identifier(&result, "messageId", true) {
        return Err(FAILED.into());
    }
    if result
        .get("status")
        .and_then(Value::as_str)
        .is_some_and(|status| !valid_turn_status(status))
    {
        return Err(FAILED.into());
    }
    if let Some(state) = result.get("runtimeState") {
        let state: BridgeRemoteControllerRuntimeState =
            serde_json::from_value(state.clone()).map_err(|_| FAILED.to_string())?;
        if !valid_runtime_state(&state) {
            return Err(FAILED.into());
        }
    }
    let required_payload = match kind {
        "tool_dispatch" | "tool_result" | "tool_progress" | "tool_result_preview" => {
            // Go permits legacy tool events without id; preserve that wire
            // compatibility, never synthesize an authority-bearing tool id.
            Some(("tool", false))
        }
        "approval_request" => Some(("approval", true)),
        "ask_request" => Some(("ask", true)),
        "mcp_interaction" => Some(("mcpInteraction", true)),
        "stream_attempt" => Some(("streamAttempt", true)),
        "read_status" => Some(("readStatus", false)),
        "workspace_changed" => Some(("workspace", false)),
        "usage" => Some(("usage", false)),
        "compaction_started" | "compaction_done" => Some(("compaction", false)),
        "context_maintenance" => Some(("maintenance", false)),
        "guardian_assessment" => Some(("guardian", false)),
        "extension_surface" | "extension_status" => Some(("extension", false)),
        "completion_summary" => Some(("completion", false)),
        _ => None,
    };
    if let Some((field, id)) = required_payload {
        if !result.get(field).is_some_and(|payload| {
            payload.is_object()
                && identifier(payload, "id", id)
                && identifier(payload, "turnId", false)
        }) {
            return Err(FAILED.into());
        }
    }
    if admission {
        // Canonical append readiness carries no conversation body or prompt
        // authority. Preserve Serve's independent route stamp only.
        result.as_object_mut().ok_or(FAILED)?.retain(|key, _| {
            [
                "kind",
                "messageId",
                "turnId",
                "seq",
                "status",
                "sessionPath",
                "sessionCurrent",
            ]
            .contains(&key.as_str())
        });
    }
    Ok(result)
}

#[cfg(test)]
#[path = "remote_controller_event_payload_tests.rs"]
mod tests;
