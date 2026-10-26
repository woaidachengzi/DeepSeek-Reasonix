//! Settings can inspect readiness and open fixed OS panes, never dispatch input.
use super::{BridgeSupervisor, PROTOCOL_VERSION};
use serde::Deserialize;
use serde_json::Value;
use tauri::State;
use tauri_plugin_opener::OpenerExt;

// The native controller serializes desktop work. Waiting for its status must
// not block the UI thread while another operation is observing an app.
#[tauri::command(async)]
pub fn computer_use_status(supervisor: State<'_, BridgeSupervisor>) -> Result<Value, String> {
    let response = supervisor.request_json("GET", "/v1/settings/computer-use", None, None)?;
    if response.get("protocolVersion").and_then(Value::as_u64) != Some(u64::from(PROTOCOL_VERSION))
    {
        return Err("desktop bridge protocol version is unsupported".into());
    }
    Ok(response)
}

#[derive(Clone, Copy, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ComputerUsePermission {
    Accessibility,
    ScreenRecording,
}

impl ComputerUsePermission {
    fn url(self) -> &'static str {
        match self {
            Self::Accessibility => {
                "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
            }
            Self::ScreenRecording => {
                "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"
            }
        }
    }
}

#[tauri::command]
pub fn open_computer_use_permission(
    app: tauri::AppHandle,
    permission: ComputerUsePermission,
) -> Result<(), String> {
    if !cfg!(target_os = "macos") {
        return Err("this system permission pane is only available on macOS".into());
    }
    app.opener()
        .open_url(permission.url(), None::<&str>)
        .map_err(|error| error.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn permission_links_accept_only_fixed_system_panes() {
        assert!(serde_json::from_str::<ComputerUsePermission>(r#""https://example.com""#).is_err());
        assert!(ComputerUsePermission::Accessibility
            .url()
            .ends_with("Privacy_Accessibility"));
        assert!(ComputerUsePermission::ScreenRecording
            .url()
            .ends_with("Privacy_ScreenCapture"));
    }
}
