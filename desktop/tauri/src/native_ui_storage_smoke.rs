//! Actual packaged WKWebView storage and rendered workspace across profiles.
//! Uses only opt-in Rust probes; no added renderer command or permission.
use std::{
    io::Read,
    path::Path,
    time::{Duration, Instant},
};

use serde::Deserialize;
use tauri::{AppHandle, Manager};

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Control {
    nonce: String,
    workspace: String,
}

pub fn run(app: &AppHandle, directory: &Path, phase: &str) -> Result<(), String> {
    let file = std::fs::File::open(directory.join("reasonix-native-ui-storage-control.json"))
        .map_err(|_| "UI storage control missing")?;
    let mut bytes = Vec::new();
    file.take(8193)
        .read_to_end(&mut bytes)
        .map_err(|_| "read UI storage control")?;
    if bytes.len() > 8192 {
        return Err("UI storage control exceeds bounds".into());
    }
    let control: Control =
        serde_json::from_slice(&bytes).map_err(|_| "invalid UI storage control")?;
    if control.nonce.len() != 32
        || !control.nonce.bytes().all(|b| b.is_ascii_hexdigit())
        || control.workspace.len() > 4096
        || !Path::new(&control.workspace).is_absolute()
        || control.workspace.chars().any(char::is_control)
    {
        return Err("invalid UI storage fixture".into());
    }
    crate::native_window_smoke::wait_for(app, "profile UI page loaded", |window| {
        Ok(crate::native_window_smoke::page_finished(
            window.app_handle(),
        ))
    })?;
    crate::native_window_smoke::on_main(app, |handle, window| {
        if !crate::ui_origin::matches(handle, &window.url().map_err(|_| "read UI origin")?) {
            return Err("main WebView is not bound to the expected profile origin".into());
        }
        Ok(())
    })?;
    let nonce = serde_json::to_string(&control.nonce).map_err(|_| "encode UI fixture")?;
    let workspace =
        serde_json::to_string(&control.workspace).map_err(|_| "encode workspace fixture")?;
    let operation = match phase {
        "ui-store-empty" => "return localStorage.getItem(key) === null && localStorage.getItem(workspaceKey) === null;",
        "ui-store-seed" => "if (localStorage.getItem(key) !== null || localStorage.getItem(workspaceKey) !== null) return false; localStorage.setItem(key, nonce); localStorage.setItem(workspaceKey, workspace); return localStorage.getItem(key) === nonce && localStorage.getItem(workspaceKey) === workspace;",
        "ui-store-restore" => "return localStorage.getItem(key) === nonce && localStorage.getItem(workspaceKey) === workspace && document.querySelector('.tauri-workspace-button')?.textContent?.includes(workspace);",
        "ui-store-clear" => "if (localStorage.getItem(key) !== nonce || localStorage.getItem(workspaceKey) !== workspace) return false; localStorage.removeItem(key); localStorage.removeItem(workspaceKey); return localStorage.getItem(key) === null && localStorage.getItem(workspaceKey) === null;",
        _ => return Err("unknown UI storage phase".into()),
    };
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        // Wait for the actual application mount, not merely the document load.
        if crate::native_edit_smoke::evaluate(
            app,
            "document.querySelector('textarea') ? 'edit-ok' : 'edit-pending'".into(),
        )? {
            break;
        }
        if Instant::now() >= deadline {
            return Err("profile UI did not render".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
    let source = format!("(() => {{ try {{ const key = 'reasonix.native-ui-storage.canary.v1'; const workspaceKey = 'reasonix.tauri.default-workspace.v1'; const nonce = {nonce}; const workspace = {workspace}; return (() => {{ {operation} }})() ? 'edit-ok' : 'edit-pending'; }} catch {{ return 'edit-pending'; }} }})()");
    if !crate::native_edit_smoke::evaluate(app, source)? {
        return Err("profile UI storage or rendered workspace did not match".into());
    }
    Ok(())
}
