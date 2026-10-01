//! Serialize saved desktop preferences and the macOS application appearance.
use std::sync::Mutex;

use tauri::{AppHandle, Manager, Theme};

use crate::bridge::{BridgeSupervisor, DesktopPreferences};

#[derive(Default)]
pub struct NativeAppearance {
    update: Mutex<()>,
}

pub(crate) const RESTORED_ERROR: &str = "The native appearance could not be updated; the previous appearance was restored. Retry the change.";
pub(crate) const RECOVERY_ERROR: &str = "The saved appearance and native window could not be synchronized. Restart Preview to reload the saved appearance.";

pub(crate) fn apply(app: &AppHandle, theme: &str) -> Result<(), String> {
    let theme = match theme {
        "" | "auto" => None,
        "light" => Some(Theme::Light),
        "dark" => Some(Theme::Dark),
        _ => return Err("invalid native appearance".into()),
    };
    #[cfg(target_os = "macos")]
    crate::native_window_smoke::observe_appearance_request(app);
    app.get_webview_window("main")
        .ok_or("native appearance window unavailable")?
        .set_theme(theme)
        .map_err(|_| "native appearance update failed".into())
}

impl NativeAppearance {
    pub fn sync(
        &self,
        app: &AppHandle,
        supervisor: &BridgeSupervisor,
    ) -> Result<DesktopPreferences, String> {
        let _update = self
            .update
            .lock()
            .map_err(|_| "native appearance lock failed")?;
        let preferences = supervisor.desktop_preferences()?;
        apply(app, &preferences.theme).map_err(|_| {
            "The native window appearance could not be synchronized. Restart Preview to reload the saved appearance."
        })?;
        Ok(preferences)
    }

    pub fn save(
        &self,
        app: &AppHandle,
        supervisor: &BridgeSupervisor,
        theme: String,
        style: String,
    ) -> Result<DesktopPreferences, String> {
        self.save_with(
            supervisor,
            theme,
            style,
            |theme, style| supervisor.set_desktop_appearance(theme, style),
            |previous, expected| supervisor.restore_desktop_appearance(previous, expected),
            |theme| apply(app, theme),
        )
    }

    // Native package fault injection uses these ports around the same locked
    // transaction. They are private Rust calls, never renderer commands.
    pub(crate) fn save_with(
        &self,
        supervisor: &BridgeSupervisor,
        theme: String,
        style: String,
        mut store: impl FnMut(String, String) -> Result<DesktopPreferences, String>,
        mut restore: impl FnMut(
            &DesktopPreferences,
            &DesktopPreferences,
        ) -> Result<DesktopPreferences, String>,
        mut native_update: impl FnMut(&str) -> Result<(), String>,
    ) -> Result<DesktopPreferences, String> {
        let _update = self
            .update
            .lock()
            .map_err(|_| "native appearance lock failed")?;
        let previous = supervisor.desktop_preferences()?;
        // Rejected validation or persistence performs no native update.
        let preferences = store(theme, style)?;
        if native_update(&preferences.theme).is_err() {
            let restored = restore(&previous, &preferences);
            if let Ok(restored) = restored {
                if native_update(&restored.theme).is_ok() {
                    return Err(RESTORED_ERROR.into());
                }
            }
            return Err(RECOVERY_ERROR.into());
        }
        Ok(preferences)
    }
}
