//! Serialize saved desktop preferences and the macOS application appearance.
use std::sync::Mutex;

use tauri::{AppHandle, Manager, Theme};

use crate::bridge::{BridgeSupervisor, DesktopPreferences};

#[derive(Default)]
pub struct NativeAppearance {
    update: Mutex<()>,
}

fn apply(app: &AppHandle, theme: &str) -> Result<(), String> {
    let theme = match theme {
        "" | "auto" => None,
        "light" => Some(Theme::Light),
        "dark" => Some(Theme::Dark),
        _ => return Err("invalid native appearance".into()),
    };
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
        let _update = self
            .update
            .lock()
            .map_err(|_| "native appearance lock failed")?;
        let previous = supervisor.desktop_preferences()?;
        // Rejected validation or persistence performs no native update.
        let preferences = supervisor.set_desktop_appearance(theme, style)?;
        if apply(app, &preferences.theme).is_err() {
            let restored = supervisor.set_desktop_appearance(
                if previous.theme.is_empty() {
                    "auto".into()
                } else {
                    previous.theme
                },
                previous.theme_style,
            );
            if let Ok(restored) = restored {
                if apply(app, &restored.theme).is_ok() {
                    return Err("The native appearance could not be updated; the previous appearance was restored. Retry the change.".into());
                }
            }
            return Err("The saved appearance and native window could not be synchronized. Restart Preview to reload the saved appearance.".into());
        }
        Ok(preferences)
    }
}
