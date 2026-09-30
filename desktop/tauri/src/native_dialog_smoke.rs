//! Opt-in actual AppKit dialog cancellation in the runner's private profile.
//! Opens production host actions, never fabricates their callback selections.
use std::{
    future::Future,
    path::Path,
    sync::mpsc,
    time::{Duration, Instant},
};

use objc2::MainThreadMarker;
use objc2_app_kit::{NSApplication, NSOpenPanel, NSSavePanel};
use tauri::{AppHandle, Manager};
use tauri_plugin_dialog::DialogExt;

use crate::native_window_smoke::on_main;

fn visible_panels(app: &AppHandle) -> Result<usize, String> {
    on_main(app, |_, _| {
        let application =
            NSApplication::sharedApplication(MainThreadMarker::new().ok_or("not main thread")?);
        Ok(application
            .windows()
            .iter()
            .filter(|window| window.isVisible() && window.downcast_ref::<NSSavePanel>().is_some())
            .count())
    })
}

fn cancel_panel(app: &AppHandle, open: bool, filename: Option<&str>) -> Result<(), String> {
    let deadline = Instant::now() + Duration::from_secs(8);
    loop {
        let filename = filename.map(str::to_owned);
        let cancelled = on_main(app, move |_, _| {
            let application =
                NSApplication::sharedApplication(MainThreadMarker::new().ok_or("not main thread")?);
            let windows = application.windows();
            let panels: Vec<_> = windows
                .iter()
                .filter(|window| {
                    window.isVisible() && window.downcast_ref::<NSSavePanel>().is_some()
                })
                .collect();
            if panels.is_empty() {
                return Ok(false);
            }
            if panels.len() != 1 {
                return Err("native dialog acceptance found multiple visible panels".into());
            }
            let window = &panels[0];
            let panel = window
                .downcast_ref::<NSSavePanel>()
                .ok_or("save panel missing")?;
            if window.downcast_ref::<NSOpenPanel>().is_some() != open {
                return Err("native dialog acceptance found the wrong panel kind".into());
            }
            if filename.is_some_and(|name| panel.nameFieldStringValue().to_string() != name) {
                return Err("native save panel did not show the expected private filename".into());
            }
            // SAFETY: The single live panel belongs to this private host and
            // the current operation. AppKit actions run on its main thread.
            unsafe { panel.cancel(None) };
            Ok(true)
        })?;
        if cancelled {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err("native dialog panel did not become visible".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}

fn cancelled<T: Send + 'static>(
    app: &AppHandle,
    open: bool,
    filename: Option<&str>,
    operation: impl Future<Output = Result<T, String>> + Send + 'static,
) -> Result<T, String> {
    if visible_panels(app)? != 0 {
        return Err("native dialog acceptance refuses an existing visible panel".into());
    }
    let (send, receive) = mpsc::sync_channel(1);
    tauri::async_runtime::spawn(async move {
        let _ = send.try_send(operation.await);
    });
    cancel_panel(app, open, filename)?;
    let result = receive
        .recv_timeout(Duration::from_secs(5))
        .map_err(|_| "native dialog cancel did not return through the host action")??;
    let deadline = Instant::now() + Duration::from_secs(5);
    while visible_panels(app)? != 0 {
        if Instant::now() >= deadline {
            return Err("native dialog remained visible after cancellation".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
    Ok(result)
}

pub fn run(app: &AppHandle, directory: &Path) -> Result<(), String> {
    let root = tempfile::tempdir_in(directory).map_err(|_| "prepare private dialog fixture")?;
    let source = root.path().join("native-dialog-source ' 中文.md");
    std::fs::write(&source, b"private dialog source").map_err(|_| "write private source")?;
    let source_path = source
        .to_str()
        .ok_or("private source path encoding")?
        .to_owned();
    let source_metadata = source.metadata().map_err(|_| "read source metadata")?;
    let window = app
        .get_webview_window("main")
        .ok_or("main window missing")?;
    if !cancelled(
        app,
        false,
        source.file_name().and_then(|name| name.to_str()),
        crate::local_paths::save_local_path_as(window, source_path),
    )?
    .is_empty()
    {
        return Err("cancelled document save returned a destination".into());
    }

    let handle = app.clone();
    let report = serde_json::json!({
        "schemaVersion": 2, "manifest": {"reportId": "cancelqa"}, "events": []
    });
    if cancelled(
        app,
        false,
        Some("reasonix-frontend-diagnostics-cancelqa.json"),
        crate::export_frontend_diagnostics(handle, report),
    )? {
        return Err("cancelled diagnostics export reported success".into());
    }

    let handle = app.clone();
    let before = app
        .state::<crate::host_preferences::HostPreferences>()
        .user_themes();
    let imported = cancelled(app, true, None, async move {
        crate::import_user_theme(handle.clone(), handle.state()).await
    })?;
    if imported.is_some()
        || before
            != app
                .state::<crate::host_preferences::HostPreferences>()
                .user_themes()
    {
        return Err("cancelled theme import changed stored themes".into());
    }

    let handle = app.clone();
    let folder = root.path().to_path_buf();
    let picked = cancelled(app, true, None, async move {
        let (send, receive) = mpsc::channel();
        handle
            .dialog()
            .file()
            .set_title("Reasonix private directory cancellation acceptance")
            .set_directory(folder)
            .pick_folder(move |selection| {
                let _ = send.send(selection);
            });
        tauri::async_runtime::spawn_blocking(move || crate::receive_file_dialog_selection(receive))
            .await
            .map_err(|_| "directory picker task failed")?
    })?;
    if picked.is_some() {
        return Err("cancelled directory picker returned a selection".into());
    }
    let after = source
        .metadata()
        .map_err(|_| "read source after cancellations")?;
    if std::fs::read(&source).map_err(|_| "read private source")? != b"private dialog source"
        || source_metadata.modified().ok() != after.modified().ok()
        || source_metadata.permissions() != after.permissions()
        || std::fs::read_dir(root.path())
            .map_err(|_| "inspect fixture")?
            .count()
            != 1
    {
        return Err("native dialog cancellation modified the source or left a file".into());
    }
    Ok(())
}
