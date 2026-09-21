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
    /// When this host binary was built. The window title and diagnostics use it
    /// to tell two builds apart, because Tauri embeds the frontend assets: a
    /// rebuilt UI is invisible until the app is fully relaunched.
    pub preview_build: String,
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
            preview_build: host_build_stamp(),
        }
    }
}

/// Formats the running executable's modification time as UTC without pulling in
/// a date library; enough to answer "which build is this window running".
fn host_build_stamp() -> String {
    let Ok(path) = std::env::current_exe() else {
        return "unknown".to_string();
    };
    let Ok(modified) = std::fs::metadata(&path).and_then(|meta| meta.modified()) else {
        return "unknown".to_string();
    };
    let Ok(elapsed) = modified.duration_since(std::time::UNIX_EPOCH) else {
        return "unknown".to_string();
    };
    format_utc(elapsed.as_secs())
}

fn format_utc(seconds: u64) -> String {
    let days = seconds / 86_400;
    let day_seconds = seconds % 86_400;
    let (hour, minute) = (day_seconds / 3_600, (day_seconds % 3_600) / 60);
    let (year, month, day) = civil_from_days(days as i64);
    format!("{year:04}-{month:02}-{day:02}T{hour:02}:{minute:02}Z")
}

/// Howard Hinnant's days-from-civil inverse, valid for the whole proleptic
/// Gregorian calendar and free of the 1900/2100 branch traps.
fn civil_from_days(days: i64) -> (i64, u32, u32) {
    let z = days + 719_468;
    let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
    let doe = (z - era * 146_097) as u64;
    let yoe = (doe - doe / 1_460 + doe / 36_524 - doe / 146_096) / 365;
    let y = yoe as i64 + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = (doy - (153 * mp + 2) / 5 + 1) as u32;
    let m = if mp < 10 { mp + 3 } else { mp - 9 } as u32;
    (if m <= 2 { y + 1 } else { y }, m, d)
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
        assert!(!info.preview_build.is_empty());
    }

    #[test]
    fn formats_a_known_instant_as_utc() {
        // 2026-09-21T21:10:00Z
        assert_eq!(format_utc(1_790_025_000), "2026-09-21T21:10Z");
        assert_eq!(format_utc(0), "1970-01-01T00:00Z");
    }
}
