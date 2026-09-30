//! Opt-in package acceptance on actual macOS windows. No renderer commands.
//! The Python runner supplies a temporary HOME and reuses it across restarts.

use std::{
    path::PathBuf,
    sync::mpsc,
    time::{Duration, Instant},
};

use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Manager, WebviewWindow};

use crate::{
    host_preferences::{CloseBehavior, HostPreferences},
    tray,
    window_state::PreviewWindowState,
};

#[derive(Clone, Debug, Deserialize, Serialize)]
struct Geometry {
    width: u32,
    height: u32,
    x: i32,
    y: i32,
    scale: f64,
}

impl Geometry {
    fn read(window: &WebviewWindow) -> Result<Self, String> {
        let size = window.inner_size().map_err(|_| "read native size")?;
        let position = window
            .outer_position()
            .map_err(|_| "read native position")?;
        Ok(Self {
            width: size.width,
            height: size.height,
            x: position.x,
            y: position.y,
            scale: window.scale_factor().map_err(|_| "read native scale")?,
        })
    }

    fn matches(&self, other: &Self) -> bool {
        self.width.abs_diff(other.width) <= 1
            && self.height.abs_diff(other.height) <= 1
            && self.x.abs_diff(other.x) <= 1
            && self.y.abs_diff(other.y) <= 1
            && (self.scale - other.scale).abs() < 0.01
    }
}

pub(crate) fn on_main<T: Send + 'static>(
    app: &AppHandle,
    action: impl FnOnce(&AppHandle, &WebviewWindow) -> Result<T, String> + Send + 'static,
) -> Result<T, String> {
    let (send, receive) = mpsc::sync_channel(1);
    let handle = app.clone();
    app.run_on_main_thread(move || {
        let result = handle
            .get_webview_window("main")
            .ok_or("main window missing".into())
            .and_then(|window| action(&handle, &window));
        let _ = send.send(result);
    })
    .map_err(|_| "schedule native window operation")?;
    receive
        .recv_timeout(Duration::from_secs(5))
        .map_err(|_| "native main thread timeout")?
}

pub(crate) fn wait_for(
    app: &AppHandle,
    stage: &'static str,
    predicate: impl Fn(&WebviewWindow) -> Result<bool, String> + Send + Sync + 'static,
) -> Result<(), String> {
    let predicate = std::sync::Arc::new(predicate);
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        let check = std::sync::Arc::clone(&predicate);
        if on_main(app, move |_, window| check(window))? {
            return Ok(());
        }
        if Instant::now() >= deadline {
            let actual = on_main(app, |_, window| {
                Ok(serde_json::json!({
                    "geometry": Geometry::read(window)?,
                    "visible": window.is_visible().map_err(|_| "read visibility")?,
                    "minimized": window.is_minimized().map_err(|_| "read minimized state")?,
                    "maximized": window.is_maximized().map_err(|_| "read maximized state")?,
                "focused": window.is_focused().map_err(|_| "read focused state")?,
                "applicationHidden": objc2_app_kit::NSApplication::sharedApplication(objc2::MainThreadMarker::new().ok_or("not main thread")?).isHidden(),
                "applicationActive": objc2_app_kit::NSApplication::sharedApplication(objc2::MainThreadMarker::new().ok_or("not main thread")?).isActive(),
                }))
            });
            return Err(format!("native window timeout: {stage}; actual={actual:?}"));
        }
        std::thread::sleep(Duration::from_millis(50));
    }
}

fn ordinary(window: &WebviewWindow) -> Result<bool, String> {
    Ok(window.is_visible().map_err(|_| "read visibility")?
        && !window.is_minimized().map_err(|_| "read minimized state")?
        && !window.is_maximized().map_err(|_| "read maximized state")?)
}

fn marker_directory() -> Result<PathBuf, String> {
    std::env::var_os("TMPDIR")
        .map(PathBuf::from)
        .filter(|path| path.is_absolute() && path.is_dir())
        .ok_or("native smoke temporary directory missing".into())
}

fn save(app: &AppHandle) -> Result<(), String> {
    on_main(app, |handle, window| {
        handle.state::<PreviewWindowState>().save(window)
    })
}

fn verify_geometry(app: &AppHandle, stage: &'static str, expected: Geometry) -> Result<(), String> {
    let target = expected.clone();
    wait_for(app, stage, move |window| {
        Ok(ordinary(window)? && target.matches(&Geometry::read(window)?))
    })
    .map_err(|error| {
        let actual = on_main(app, |_, window| {
            Ok(serde_json::json!({
                "geometry": Geometry::read(window)?,
                "visible": window.is_visible().map_err(|_| "read visibility")?,
                "minimized": window.is_minimized().map_err(|_| "read minimized state")?,
                "maximized": window.is_maximized().map_err(|_| "read maximized state")?,
            }))
        });
        format!("{error}; expected={expected:?}; actual={actual:?}")
    })
}

fn exercise(app: &AppHandle, directory: &std::path::Path) -> Result<(), String> {
    on_main(app, |handle, _| {
        tray::show_main_window(handle);
        Ok(())
    })?;
    wait_for(app, "initial ordinary window", ordinary)?;
    let (expected_x, expected_y, logical_width, logical_height) = on_main(app, |_, window| {
        window
            .unmaximize()
            .map_err(|_| "unmaximize initial window")?;
        let monitor = window
            .primary_monitor()
            .map_err(|_| "read primary display")?
            .ok_or("no primary display")?;
        let work = monitor.work_area();
        let logical_width = 1000.0_f64.min(f64::from(work.size.width) / monitor.scale_factor());
        let logical_height = 700.0_f64.min(f64::from(work.size.height) / monitor.scale_factor());
        if logical_width < 900.0 || logical_height < 620.0 {
            return Err("display work area is below the application's minimum size".into());
        }
        window
            .set_size(tauri::LogicalSize::new(logical_width, logical_height))
            .map_err(|_| "resize native window")?;
        let width = (logical_width * monitor.scale_factor()).round() as u32;
        let height = (logical_height * monitor.scale_factor()).round() as u32;
        // Use a central reachable frame. Stage Manager may move a window
        // placed in its reserved left strip when the application activates.
        let x = work.position.x + (work.size.width.saturating_sub(width) / 2) as i32;
        let y = work.position.y + (work.size.height.saturating_sub(height) / 2) as i32;
        window
            .set_position(tauri::PhysicalPosition::new(x, y))
            .map_err(|_| "move native window")?;
        Ok((x, y, logical_width, logical_height))
    })?;
    wait_for(app, "resized and positioned window", move |window| {
        let geometry = Geometry::read(window)?;
        Ok(ordinary(window)?
            && (f64::from(geometry.width) / geometry.scale - logical_width).abs() <= 1.0
            && (f64::from(geometry.height) / geometry.scale - logical_height).abs() <= 1.0
            && geometry.x.abs_diff(expected_x) <= 1
            && geometry.y.abs_diff(expected_y) <= 1)
    })?;
    let normal = on_main(app, |_, window| Geometry::read(window))?;
    std::fs::write(
        directory.join("reasonix-native-window-normal.json"),
        serde_json::to_vec(&normal).map_err(|_| "encode normal geometry")?,
    )
    .map_err(|_| "write normal geometry")?;
    save(app)?;

    on_main(app, |_, window| {
        window.hide().map_err(|_| "hide native window".into())
    })?;
    wait_for(app, "hidden window", |window| {
        window
            .is_visible()
            .map(|value| !value)
            .map_err(|_| "read visibility".into())
    })?;
    on_main(app, |handle, _| {
        tray::show_main_window(handle);
        Ok(())
    })?;
    verify_geometry(app, "shown geometry", normal.clone())?;

    on_main(app, |_, window| {
        window
            .minimize()
            .map_err(|_| "minimize native window".into())
    })?;
    wait_for(app, "minimized window", |window| {
        window
            .is_minimized()
            .map_err(|_| "read minimized state".into())
    })?;
    save(app)?;
    on_main(app, |handle, _| {
        tray::show_main_window(handle);
        Ok(())
    })?;
    verify_geometry(app, "unminimized geometry", normal.clone())?;

    on_main(app, |_, window| {
        window
            .maximize()
            .map_err(|_| "maximize native window".into())
    })?;
    wait_for(app, "maximized window", |window| {
        window
            .is_maximized()
            .map_err(|_| "read maximized state".into())
    })?;
    save(app)
}

fn expected_geometry(directory: &std::path::Path) -> Result<Geometry, String> {
    serde_json::from_slice(
        &std::fs::read(directory.join("reasonix-native-window-normal.json"))
            .map_err(|_| "read expected normal geometry")?,
    )
    .map_err(|_| "decode expected geometry".into())
}

fn application_hide(app: &AppHandle, directory: &std::path::Path) -> Result<(), String> {
    let normal = expected_geometry(directory)?;
    verify_geometry(app, "normal window before application hide", normal.clone())?;
    on_main(app, |handle, _| {
        handle.hide().map_err(|_| "hide macOS application".into())
    })?;
    wait_for(app, "hidden application", |_| {
        Ok(objc2_app_kit::NSApplication::sharedApplication(
            objc2::MainThreadMarker::new().ok_or("not main thread")?,
        )
        .isHidden())
    })?;
    on_main(app, |handle, _| {
        tray::show_main_window(handle);
        Ok(())
    })?;
    wait_for(app, "shown application", |_| {
        Ok(!objc2_app_kit::NSApplication::sharedApplication(
            objc2::MainThreadMarker::new().ok_or("not main thread")?,
        )
        .isHidden())
    })?;
    verify_geometry(app, "application unhide geometry", normal.clone())?;

    save(app)
}

fn background_close(
    app: &AppHandle,
    directory: &std::path::Path,
    require_second: bool,
) -> Result<(), String> {
    let normal = expected_geometry(directory)?;
    verify_geometry(app, "normal window before background close", normal.clone())?;
    on_main(app, |handle, _| {
        handle
            .state::<HostPreferences>()
            .set_close_behavior(CloseBehavior::KeepRunning)
    })?;
    on_main(app, |_, window| {
        window
            .close()
            .map_err(|_| "request background close".into())
    })?;
    wait_for(app, "background close", |window| {
        window
            .is_visible()
            .map(|visible| !visible)
            .map_err(|_| "read visibility".into())
    })?;

    if require_second {
        std::fs::write(
            directory.join("reasonix-native-window-awaiting-instance.json"),
            b"{}",
        )
        .map_err(|_| "write second instance marker")?;
        let target = normal.clone();
        wait_for(app, "second instance focus and geometry", move |window| {
            Ok(ordinary(window)?
                && window.is_focused().map_err(|_| "read focus")?
                && target.matches(&Geometry::read(window)?))
        })?;
    } else {
        on_main(app, |handle, _| {
            tray::show_main_window(handle);
            Ok(())
        })?;
        verify_geometry(app, "background close restore", normal)?;
    }
    save(app)
}

fn run(app: &AppHandle, phase: &str) -> Result<(), String> {
    let directory = marker_directory()?;
    match phase {
        "exercise" => exercise(app, &directory)?,
        "menu-settings-hidden" | "menu-settings-minimized" | "menu-settings-app-hidden" => {
            crate::native_menu_smoke::settings(app, phase)?
        }
        "application-hide" => application_hide(app, &directory)?,
        "background-close" => background_close(app, &directory, false)?,
        "second-instance" => background_close(app, &directory, true)?,
        "restore-maximized" | "restore-normal" => {
            let expected = expected_geometry(&directory)?;
            if phase == "restore-maximized" {
                wait_for(app, "restored maximized window", |window| {
                    window
                        .is_maximized()
                        .map_err(|_| "read maximized state".into())
                })?;
                on_main(app, |_, window| {
                    window
                        .unmaximize()
                        .map_err(|_| "unmaximize restored window".into())
                })?;
            }
            verify_geometry(app, "restored normal geometry", expected)?;
            save(app)?;
        }
        _ => return Err("unknown native window smoke phase".into()),
    }
    Ok(())
}

fn close_to_quit(app: &AppHandle, phase: &str) -> Result<(), String> {
    let directory = marker_directory()?;
    let expected = expected_geometry(&directory)?;
    verify_geometry(app, "normal window before quit", expected)?;
    let restore = phase == "restore-close-quit";
    on_main(app, move |handle, _| {
        let preferences = handle.state::<HostPreferences>();
        if restore {
            if preferences.close_behavior() != CloseBehavior::Quit {
                return Err("quit close preference did not survive restart".into());
            }
            Ok(())
        } else {
            preferences.set_close_behavior(CloseBehavior::Quit)
        }
    })?;
    // Publish readiness before the real close event can terminate this thread.
    // The runner also requires an actual exit and complete sidecar cleanup.
    std::fs::write(
        directory.join("reasonix-native-window-result.json"),
        serde_json::json!({"phase": phase, "ok": true}).to_string(),
    )
    .map_err(|_| "write close acceptance marker")?;
    // Allow the runner to inspect the live sidecar before requesting close.
    std::thread::sleep(Duration::from_secs(3));
    on_main(app, |_, window| {
        window.close().map_err(|_| "request quit close".into())
    })
}

pub fn start_if_requested(app: &AppHandle) {
    let Ok(phase) = std::env::var("REASONIX_TAURI_NATIVE_WINDOW_SMOKE") else {
        return;
    };
    let handle = app.clone();
    std::thread::spawn(move || {
        let result = if phase == "close-quit" || phase == "restore-close-quit" {
            match close_to_quit(&handle, &phase) {
                // Do not call app.exit here: only the close-preference handler
                // may terminate a successful close-to-quit acceptance.
                Ok(()) => return,
                Err(error) => Err(error),
            }
        } else {
            run(&handle, &phase)
        };
        let status = match &result {
            Ok(()) => serde_json::json!({"phase": phase, "ok": true}),
            Err(error) => serde_json::json!({"phase": phase, "ok": false, "error": error}),
        };
        let written = marker_directory().and_then(|directory| {
            std::fs::write(
                directory.join("reasonix-native-window-result.json"),
                status.to_string(),
            )
            .map_err(|_| "write native window result".into())
        });
        // Let the external runner inspect live readiness and authentication.
        // This delay does not serve as a window-state acceptance condition.
        std::thread::sleep(Duration::from_secs(3));
        handle.exit(if result.is_ok() && written.is_ok() {
            0
        } else {
            2
        });
    });
}
