use serde::Serialize;

use crate::bridge::{BridgeStatus, PROTOCOL_VERSION};

pub const STABLE_VERSION: &str = "1.38.3";
pub const STABLE_COMMIT: &str = "fa018e4109268c912063c8cc619302fccdb57d74";

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PreviewRuntimeInfo {
    pub stable_version: &'static str,
    pub stable_commit: &'static str,
    pub preview_version: &'static str,
    pub tauri_version: &'static str,
    pub bridge_protocol_version: u8,
    pub sidecar_instance_id: Option<String>,
}

impl PreviewRuntimeInfo {
    pub fn from_status(status: BridgeStatus) -> Self {
        Self {
            stable_version: STABLE_VERSION,
            stable_commit: STABLE_COMMIT,
            preview_version: env!("CARGO_PKG_VERSION"),
            tauri_version: tauri::VERSION,
            bridge_protocol_version: status.protocol_version.unwrap_or(PROTOCOL_VERSION),
            sidecar_instance_id: status.sidecar_instance_id,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn identifies_the_frozen_stable_baseline_and_live_sidecar() {
        let info = PreviewRuntimeInfo::from_status(BridgeStatus {
            running: true,
            protocol_version: Some(1),
            sidecar_instance_id: Some("bridge-test-instance".into()),
        });

        assert_eq!(info.stable_version, "1.38.3");
        assert_eq!(info.stable_commit, STABLE_COMMIT);
        assert_eq!(info.bridge_protocol_version, 1);
        assert_eq!(
            info.sidecar_instance_id.as_deref(),
            Some("bridge-test-instance")
        );
        assert!(!info.preview_version.is_empty());
        assert!(!info.tauri_version.is_empty());
    }
}
