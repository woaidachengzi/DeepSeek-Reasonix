//! Opt-in installed Reload menu regression in an owned private profile.
use std::{path::Path, time::{Duration, Instant}};
use tauri::{AppHandle, Manager};

fn check(app: &AppHandle, script: String) -> Result<(), String> {
    let deadline = Instant::now() + Duration::from_secs(15);
    loop {
        if crate::native_edit_smoke::evaluate(app, script.clone())? { return Ok(()); }
        if Instant::now() >= deadline { return Err("native Reload did not restore the rendered main page".into()); }
        std::thread::sleep(Duration::from_millis(25));
    }
}

pub(crate) fn run(app: &AppHandle, directory: &Path) -> Result<(), String> {
    use crate::native_window_smoke::{on_main, wait_for};
    wait_for(app, "initial trusted main page", |window| {
        Ok(crate::native_window_smoke::page_finished(window.app_handle()) && window.is_visible().unwrap_or(false))
    })?;
    check(app, "document.querySelector('.tauri-shell') ? 'edit-ok' : 'edit-pending'".into())?;
    let mut receipts = Vec::new();
    for hidden in [false, true] {
        if hidden {
            on_main(app, |_, window| window.hide().map_err(|_| "hide reload fixture".into()))?;
            wait_for(app, "hidden reload fixture", |window| Ok(!window.is_visible().unwrap_or(true)))?;
        }
        let before = on_main(app, |_, window| crate::native_window_smoke::snapshot(window))?;
        let status = app.state::<crate::bridge::BridgeSupervisor>().status();
        if !status.running || status.sidecar_instance_id.is_none() { return Err("reload fixture sidecar unavailable".into()); }
        let value = if hidden { "owned-hidden-reload" } else { "owned-normal-reload" };
        let setup = format!("window.__reasonixNativeReloadMarker = true; localStorage.setItem('reasonix.native-reload.canary', '{value}'); 'edit-ok'");
        if !crate::native_edit_smoke::evaluate(app, setup)? { return Err("prepare reload realm/storage markers".into()); }
        crate::native_window_smoke::record(app, if hidden { "reload-hidden-before" } else { "reload-normal-before" })?;
        crate::native_menu_smoke::invoke(app, "Reload")?;
        check(app, format!("typeof window.__reasonixNativeReloadMarker === 'undefined' && document.querySelector('.tauri-shell') && localStorage.getItem('reasonix.native-reload.canary') === '{value}' ? 'edit-ok' : 'edit-pending'"))?;
        let after = on_main(app, |_, window| {
            let url = window.url().map_err(|_| "read reloaded main URL")?;
            if !crate::ui_origin::matches(window.app_handle(), &url) { return Err("Reload changed trusted main origin".into()); }
            crate::native_window_smoke::snapshot(window)
        })?;
        let current = app.state::<crate::bridge::BridgeSupervisor>().status();
        if !current.running || current.sidecar_instance_id != status.sidecar_instance_id {
            return Err("Reload replaced or stopped the sidecar".into());
        }
        for field in ["nativeWindowNumber", "geometry", "visible"] {
            if before.get(field) != after.get(field) { return Err(format!("Reload changed native {field}")); }
        }
        if hidden && after["visible"] != false { return Err("Reload reopened a hidden window".into()); }
        crate::native_window_smoke::record(app, if hidden { "reload-hidden-after" } else { "reload-normal-after" })?;
        receipts.push(serde_json::json!({"hidden":hidden,"newRealm":true,"renderedMain":true,"storagePreserved":true,"trustedOrigin":true,"nativeWindowPreserved":true,"sidecarPreserved":true}));
    }
    if !crate::native_edit_smoke::evaluate(app, "localStorage.removeItem('reasonix.native-reload.canary'); 'edit-ok'".into())? { return Err("remove private reload marker".into()); }
    std::fs::write(directory.join("reasonix-native-reload-result.json"), serde_json::json!({"ok":true,"cycles":receipts}).to_string())
        .map_err(|_| "record native Reload acceptance".into())
}
