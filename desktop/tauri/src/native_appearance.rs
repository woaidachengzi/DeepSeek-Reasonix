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
    {
        crate::native_window_smoke::observe_appearance_request(app);
        if already_applied(app, theme)? {
            crate::native_window_smoke::observe_appearance_skip(app);
            return Ok(());
        }
    }
    app.get_webview_window("main")
        .ok_or("native appearance window unavailable")?
        .set_theme(theme)
        .map_err(|_| "native appearance update failed".into())
}

#[cfg(target_os = "macos")]
fn selection_matches(
    requested: Option<Theme>,
    explicit: Option<&str>,
    effective: Theme,
    cached: Theme,
) -> bool {
    match requested {
        Some(Theme::Light) => explicit == Some("NSAppearanceNameAqua") && cached == Theme::Light,
        Some(Theme::Dark) => explicit == Some("NSAppearanceNameDarkAqua") && cached == Theme::Dark,
        None => explicit.is_none() && cached == effective,
        _ => false,
    }
}

#[cfg(target_os = "macos")]
fn already_applied(app: &AppHandle, requested: Option<Theme>) -> Result<bool, String> {
    fn read(app: &AppHandle, requested: Option<Theme>) -> Result<bool, String> {
        use objc2_app_kit::NSApplication;
        use objc2_foundation::{NSArray, NSString};
        let application = NSApplication::sharedApplication(
            objc2::MainThreadMarker::new().ok_or("native appearance query requires main thread")?,
        );
        let explicit = application
            .appearance()
            .map(|appearance| appearance.name().to_string());
        // Match Tao's effective theme selection, including system inheritance.
        // Explicit nil and explicit Aqua are different even on a light system.
        let choices = NSArray::from_retained_slice(&[
            NSString::from_str("NSAppearanceNameAqua"),
            NSString::from_str("NSAppearanceNameDarkAqua"),
        ]);
        let effective = application
            .effectiveAppearance()
            .bestMatchFromAppearancesWithNames(&choices)
            .ok_or("native effective appearance unavailable")?;
        let effective = if effective.to_string() == "NSAppearanceNameDarkAqua" {
            Theme::Dark
        } else {
            Theme::Light
        };
        let cached = app
            .get_webview_window("main")
            .ok_or("native appearance window unavailable")?
            .theme()
            .map_err(|_| "native cached appearance unavailable")?;
        Ok(selection_matches(
            requested,
            explicit.as_deref(),
            effective,
            cached,
        ))
    }
    if objc2::MainThreadMarker::new().is_some() {
        return read(app, requested);
    }
    let (send, receive) = std::sync::mpsc::sync_channel(1);
    let handle = app.clone();
    app.run_on_main_thread(move || {
        // This scheduled work is read-only. A query timeout cannot leave a
        // delayed theme mutation that overtakes save/rollback or a later choice.
        let _ = send.send(read(&handle, requested));
    })
    .map_err(|_| "schedule native appearance query")?;
    receive
        .recv_timeout(std::time::Duration::from_secs(5))
        .map_err(|_| "native appearance query timed out")?
}

#[cfg(all(test, target_os = "macos"))]
mod selection_tests {
    use super::*;
    #[test]
    fn automatic_inheritance_never_matches_an_explicit_appearance_or_stale_cache() {
        assert!(selection_matches(None, None, Theme::Light, Theme::Light));
        assert!(selection_matches(None, None, Theme::Dark, Theme::Dark));
        assert!(!selection_matches(
            None,
            Some("NSAppearanceNameAqua"),
            Theme::Light,
            Theme::Light
        ));
        assert!(!selection_matches(None, None, Theme::Dark, Theme::Light));
    }
    #[test]
    fn explicit_selection_requires_the_actual_override_and_matching_tauri_cache() {
        assert!(selection_matches(
            Some(Theme::Dark),
            Some("NSAppearanceNameDarkAqua"),
            Theme::Dark,
            Theme::Dark
        ));
        assert!(!selection_matches(
            Some(Theme::Light),
            None,
            Theme::Light,
            Theme::Light
        ));
        assert!(!selection_matches(
            Some(Theme::Dark),
            Some("NSAppearanceNameDarkAqua"),
            Theme::Dark,
            Theme::Light
        ));
        assert!(!selection_matches(
            Some(Theme::Dark),
            Some("NSAppearanceNameAqua"),
            Theme::Light,
            Theme::Light
        ));
    }
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
