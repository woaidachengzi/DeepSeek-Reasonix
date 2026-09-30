//! Real AppKit appearance, host settings and restart acceptance.
use tauri::{AppHandle, Manager};

use crate::{
    bridge::{BridgeSupervisor, DesktopPreferences},
    native_window_smoke::{on_main, wait_for},
};

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
    if phase == "appearance-rollback-unconfigured" {
        return unconfigured_rollback(app);
    }
    if phase == "appearance-rollback" {
        return rollback_faults(app);
    }
    let (restored, next, native) = match phase {
        "appearance-dark" => (None, "dark", Some("NSAppearanceNameDarkAqua")),
        "restore-appearance-dark" => (
            Some(Some("NSAppearanceNameDarkAqua")),
            "light",
            Some("NSAppearanceNameAqua"),
        ),
        "restore-appearance-light" => (Some(Some("NSAppearanceNameAqua")), "auto", None),
        "restore-appearance-auto" => (Some(None), "auto", None),
        "restore-appearance-rollback" => (Some(Some("NSAppearanceNameDarkAqua")), "auto", None),
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

fn unconfigured_rollback(app: &AppHandle) -> Result<(), String> {
    isolate_host_faults(app)?;
    let supervisor = app.state::<BridgeSupervisor>();
    let before = supervisor.desktop_preferences()?;
    if before.appearance_configured {
        return Err("fresh appearance rollback fixture is already configured".into());
    }
    verify(app, None)?;
    let mut updates = 0;
    let result = app
        .state::<crate::native_appearance::NativeAppearance>()
        .save_with(
            &supervisor,
            "light".into(),
            "aurora".into(),
            |theme, style| supervisor.set_desktop_appearance(theme, style),
            |previous, expected| supervisor.restore_desktop_appearance(previous, expected),
            |theme| {
                updates += 1;
                if updates == 1 {
                    Err("injected native failure".into())
                } else {
                    crate::native_appearance::apply(app, theme)
                }
            },
        );
    if result.err().as_deref() != Some(crate::native_appearance::RESTORED_ERROR) || updates != 2 {
        return Err("fresh appearance failure did not return the rollback result".into());
    }
    let after = supervisor.desktop_preferences()?;
    if serde_json::to_value(before).map_err(|_| "encode appearance baseline")?
        != serde_json::to_value(after).map_err(|_| "encode appearance result")?
    {
        return Err("failed native appearance changed previously unconfigured preferences".into());
    }
    verify(app, None)
}

fn isolate_host_faults(app: &AppHandle) -> Result<(), String> {
    // These cases exercise the native transaction, not rendered settings UI.
    // Unload the page so a pending frontend preference read cannot silently
    // synchronize the appearance and conceal a failed native rollback.
    on_main(app, |_, window| {
        window
            .navigate(
                "about:blank"
                    .parse()
                    .map_err(|_| "encode isolated appearance page")?,
            )
            .map_err(|_| "isolate native appearance faults".into())
    })?;
    wait_for(app, "isolated native appearance document", |window| {
        Ok(window
            .url()
            .map_err(|_| "read native document URL")?
            .as_str()
            == "about:blank")
    })
}

fn preferences_match(
    supervisor: &BridgeSupervisor,
    expected: &DesktopPreferences,
) -> Result<(), String> {
    let actual = supervisor.desktop_preferences()?;
    if serde_json::to_value(actual).map_err(|_| "encode appearance result")?
        != serde_json::to_value(expected).map_err(|_| "encode appearance baseline")?
    {
        return Err("appearance fault changed unexpected stored preferences".into());
    }
    Ok(())
}

#[derive(Clone, Copy, Debug, PartialEq)]
enum Fault {
    BeforeNative,
    AfterNative,
    Store,
    RestoreStore,
    ConflictingStore,
    RestoreNative,
}

fn rollback_faults(app: &AppHandle) -> Result<(), String> {
    isolate_host_faults(app)?;
    let supervisor = app.state::<BridgeSupervisor>();
    for fault in [
        Fault::BeforeNative,
        Fault::AfterNative,
        Fault::Store,
        Fault::RestoreStore,
        Fault::ConflictingStore,
        Fault::RestoreNative,
    ] {
        let before = crate::set_desktop_appearance(
            app.clone(),
            app.state(),
            "dark".into(),
            "graphite".into(),
        )?;
        verify(app, Some("NSAppearanceNameDarkAqua"))?;
        let (mut stores, mut restores, mut updates) = (0, 0, 0);
        let result = app
            .state::<crate::native_appearance::NativeAppearance>()
            .save_with(
                &supervisor,
                "light".into(),
                "aurora".into(),
                |theme, style| {
                    stores += 1;
                    if fault == Fault::Store {
                        return Err("injected store failure".into());
                    }
                    supervisor.set_desktop_appearance(theme, style)
                },
                |previous, expected| {
                    restores += 1;
                    if fault == Fault::RestoreStore {
                        return Err("injected restore failure".into());
                    }
                    if fault == Fault::ConflictingStore {
                        supervisor.set_desktop_appearance("auto".into(), "amber".into())?;
                    }
                    supervisor.restore_desktop_appearance(previous, expected)
                },
                |theme| {
                    updates += 1;
                    if updates == 1 {
                        if fault != Fault::BeforeNative {
                            crate::native_appearance::apply(app, theme)?;
                        }
                        return Err("injected uncertain native failure".into());
                    }
                    if fault == Fault::RestoreNative {
                        return Err("injected native restore failure".into());
                    }
                    crate::native_appearance::apply(app, theme)
                },
            );
        let expected_error = match fault {
            Fault::Store => "injected store failure",
            Fault::BeforeNative | Fault::AfterNative => crate::native_appearance::RESTORED_ERROR,
            _ => crate::native_appearance::RECOVERY_ERROR,
        };
        let expected_calls = match fault {
            Fault::Store => (1, 0, 0),
            Fault::RestoreStore | Fault::ConflictingStore => (1, 1, 1),
            _ => (1, 1, 2),
        };
        if result.err().as_deref() != Some(expected_error)
            || (stores, restores, updates) != expected_calls
        {
            return Err(format!(
                "appearance {fault:?} returned an incorrect error or transaction boundary"
            ));
        }
        let mut expected = before;
        let native = match fault {
            Fault::Store | Fault::BeforeNative | Fault::AfterNative => {
                Some("NSAppearanceNameDarkAqua")
            }
            Fault::RestoreStore => {
                expected.theme = "light".into();
                expected.theme_style = "aurora".into();
                Some("NSAppearanceNameAqua")
            }
            Fault::ConflictingStore => {
                expected.theme = "auto".into();
                expected.theme_style = "amber".into();
                Some("NSAppearanceNameAqua")
            }
            Fault::RestoreNative => Some("NSAppearanceNameAqua"),
        };
        preferences_match(&supervisor, &expected)?;
        verify(app, native).map_err(|error| format!("appearance {fault:?}: {error}"))?;
    }
    // The last failure leaves saved dark/native light intentionally. The next
    // real host restart must repair it before any test preference command.
    Ok(())
}
