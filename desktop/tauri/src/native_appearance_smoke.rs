//! Real AppKit appearance, host settings and restart acceptance.
use tauri::{AppHandle, Manager};

use crate::{bridge::BridgeSupervisor, native_window_smoke::wait_for};

fn verify(app: &AppHandle, expected: Option<&'static str>) -> Result<(), String> {
    wait_for(app, "native application appearance", move |_| {
        let native = objc2_app_kit::NSApplication::sharedApplication(
            objc2::MainThreadMarker::new().ok_or("not main thread")?,
        );
        // None proves automatic/system inheritance, even if the effective
        // system appearance happens to equal the last explicit choice.
        let appearance = native.appearance().map(|value| value.name().to_string());
        Ok(appearance.as_deref() == expected)
    })
}

pub fn run(app: &AppHandle, phase: &str) -> Result<(), String> {
    let (restored, next, native) = match phase {
        "appearance-dark" => (None, "dark", Some("NSAppearanceNameDarkAqua")),
        "restore-appearance-dark" => (
            Some(Some("NSAppearanceNameDarkAqua")),
            "light",
            Some("NSAppearanceNameAqua"),
        ),
        "restore-appearance-light" => (Some(Some("NSAppearanceNameAqua")), "auto", None),
        "restore-appearance-auto" => (Some(None), "auto", None),
        _ => return Err("unknown appearance acceptance phase".into()),
    };
    // Inspect startup before any command that could mask a failed restore.
    if let Some(expected) = restored {
        verify(app, expected)?;
    }
    let preferences =
        crate::set_desktop_appearance(app.clone(), app.state(), next.into(), "graphite".into())?;
    if preferences.theme != next || preferences.theme_style != "graphite" {
        return Err("saved appearance differs from requested preference".into());
    }
    verify(app, native)?;
    let before = app.state::<BridgeSupervisor>().desktop_preferences()?;
    if crate::set_desktop_appearance(app.clone(), app.state(), "sepia".into(), "graphite".into())
        .is_ok()
        || crate::set_desktop_appearance(app.clone(), app.state(), "light".into(), "unknown".into())
            .is_ok()
    {
        return Err("invalid appearance accepted".into());
    }
    let after = app.state::<BridgeSupervisor>().desktop_preferences()?;
    if after.theme != before.theme || after.theme_style != before.theme_style {
        return Err("invalid appearance changed stored preferences".into());
    }
    verify(app, native)
}
