//! Read-only bot observations. No secrets, SDK errors or activation commands.
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashSet;

const INVALID: &str =
    "bot diagnostics response is unsupported; update the Preview host and bridge together";

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum ConfigStatus {
    Disabled,
    BotDisabled,
    MissingCredentials,
    AccessBlocked,
    Configured,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum RuntimeStatus {
    NotObserved,
    Refreshing,
    Unknown,
    Configured,
    Disabled,
    Running,
    Error,
    Closed,
    Degraded,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ConnectionView {
    pub id: String,
    pub config_status: ConfigStatus,
    pub runtime_status: RuntimeStatus,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct DiagnosticsView {
    pub protocol_version: u64,
    pub runtime_observation_only: bool,
    pub connections: Vec<ConnectionView>,
}

pub fn ensure_main_window(label: &str) -> Result<(), String> {
    if label == "main" {
        Ok(())
    } else {
        Err("bot diagnostics is only available in the main window; open Settings there".into())
    }
}

pub fn parse(value: Value) -> Result<DiagnosticsView, String> {
    // Never expose a deserializer's offending private field/value in an IPC error.
    let view: DiagnosticsView = serde_json::from_value(value).map_err(|_| INVALID.to_string())?;
    let mut ids = HashSet::new();
    if view.protocol_version != u64::from(crate::bridge::PROTOCOL_VERSION)
        || !view.runtime_observation_only
        || view.connections.len() > 10_000
        || view.connections.iter().any(|row| {
            row.id.is_empty()
                || row.id.len() > 256
                || row.id.chars().any(char::is_control)
                || !ids.insert(&row.id)
        })
    {
        return Err(INVALID.into());
    }
    Ok(view)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn fixture() -> Value {
        json!({"protocolVersion":1,"runtimeObservationOnly":true,"connections":[
            {"id":"legacy:qq","configStatus":"configured","runtimeStatus":"not_observed"}
        ]})
    }

    #[test]
    fn observation_does_not_promote_configured_to_running() {
        assert_eq!(
            serde_json::to_value(parse(fixture()).unwrap()).unwrap(),
            fixture()
        );
        assert!(
            parse(json!({"protocolVersion":1,"runtimeObservationOnly":true,"connections":[]}))
                .is_ok()
        );
    }

    #[test]
    fn accepts_every_backend_status_without_arbitrary_strings() {
        for config in [
            "disabled",
            "bot_disabled",
            "missing_credentials",
            "access_blocked",
            "configured",
        ] {
            for runtime in [
                "not_observed",
                "refreshing",
                "unknown",
                "configured",
                "disabled",
                "running",
                "error",
                "closed",
                "degraded",
            ] {
                let mut value = fixture();
                value["connections"][0]["configStatus"] = json!(config);
                value["connections"][0]["runtimeStatus"] = json!(runtime);
                assert!(parse(value).is_ok());
            }
        }
    }

    #[test]
    fn rejects_private_fields_and_unknown_status_without_echoing_them() {
        for (field, nested) in [
            ("sdkError", false),
            ("secret", true),
            ("runtimeStatus", true),
        ] {
            let mut value = fixture();
            if nested {
                value["connections"][0][field] = json!("private-secret-canary");
            } else {
                value[field] = json!("private-secret-canary");
            }
            assert_eq!(parse(value).unwrap_err(), INVALID);
        }
    }

    #[test]
    fn rejects_version_missing_marker_and_false_observation_contract() {
        for (field, replacement) in [
            ("protocolVersion", json!(2)),
            ("runtimeObservationOnly", json!(false)),
            ("runtimeObservationOnly", Value::Null),
        ] {
            let mut value = fixture();
            value[field] = replacement;
            assert!(parse(value).is_err());
        }
    }

    #[test]
    fn rejects_ambiguous_or_unbounded_connection_identity() {
        for id in ["".to_string(), "bad\nidentity".to_string(), "x".repeat(257)] {
            let mut value = fixture();
            value["connections"][0]["id"] = json!(id);
            assert!(parse(value).is_err());
        }
        let mut duplicate = fixture();
        duplicate["connections"]
            .as_array_mut()
            .unwrap()
            .push(fixture()["connections"][0].clone());
        assert!(parse(duplicate).is_err());
    }

    #[test]
    fn only_main_window_can_inspect_diagnostics() {
        assert!(ensure_main_window("main").is_ok());
        for label in ["", "settings", "remote", "main-other"] {
            assert!(ensure_main_window(label).is_err());
        }
    }
}
