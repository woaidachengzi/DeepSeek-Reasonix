//! Actual AppKit menu action -> Tauri menu handler -> main-window event.
//! Runs only through the opt-in package runner, with its private profile.

use std::sync::{
    atomic::{AtomicUsize, Ordering},
    Arc,
};

use objc2::MainThreadMarker;
use objc2_app_kit::{NSApplication, NSMenu};
use tauri::{AppHandle, EventId, Listener, WebviewWindow};

use crate::native_window_smoke::{on_main, wait_for};

struct SettingsListener {
    window: WebviewWindow,
    id: EventId,
}
impl Drop for SettingsListener {
    fn drop(&mut self) {
        self.window.unlisten(self.id);
    }
}

fn invoke_settings(menu: &NSMenu, depth: usize) -> Result<bool, String> {
    if depth > 3 || !(0..=128).contains(&menu.numberOfItems()) {
        return Err("native menu exceeds acceptance bounds".into());
    }
    for index in 0..menu.numberOfItems() {
        let item = menu.itemAtIndex(index).ok_or("native menu item missing")?;
        if item.title().to_string() == "Settings…" {
            // performActionForItemAtIndex does not perform automatic validation.
            // Check the actual installed item's state before sending its action.
            if !item.isEnabled() {
                return Err("native settings menu is disabled".into());
            }
            menu.performActionForItemAtIndex(index);
            return Ok(true);
        }
        if let Some(submenu) = item.submenu() {
            if invoke_settings(&submenu, depth + 1)? {
                return Ok(true);
            }
        }
    }
    Ok(false)
}

pub fn settings(app: &AppHandle, phase: &str) -> Result<(), String> {
    let delivered = Arc::new(AtomicUsize::new(0));
    let counter = Arc::clone(&delivered);
    let (window, id) = on_main(app, move |_, window| {
        let id = window.listen("host:open-settings", move |_| {
            counter.fetch_add(1, Ordering::SeqCst);
        });
        Ok((window.clone(), id))
    })?;
    let _listener = SettingsListener { window, id };

    // Establish the live hide/show transition used before minimizing in the
    // window smoke. Ready alone does not establish native presentation state.
    on_main(app, |_, window| {
        window
            .hide()
            .map_err(|_| "hide initial settings window".into())
    })?;
    wait_for(app, "initial settings window hidden", |window| {
        window
            .is_visible()
            .map(|visible| !visible)
            .map_err(|_| "read visibility".into())
    })?;
    on_main(app, |handle, _| {
        crate::tray::show_main_window(handle);
        Ok(())
    })?;
    wait_for(app, "initial settings window", |window| {
        Ok(window.is_visible().map_err(|_| "read visibility")?
            && !window.is_minimized().map_err(|_| "read minimized state")?
            && !NSApplication::sharedApplication(MainThreadMarker::new().ok_or("not main thread")?)
                .isHidden())
    })?;

    match phase {
        "menu-settings-hidden" => {
            on_main(app, |_, window| {
                window.hide().map_err(|_| "hide settings window".into())
            })?;
            wait_for(app, "hidden settings window", |window| {
                window
                    .is_visible()
                    .map(|visible| !visible)
                    .map_err(|_| "read visibility".into())
            })?;
        }
        "menu-settings-minimized" => {
            on_main(app, |_, window| {
                window
                    .minimize()
                    .map_err(|_| "minimize settings window".into())
            })?;
            wait_for(app, "minimized settings window", |window| {
                window
                    .is_minimized()
                    .map_err(|_| "read minimized state".into())
            })?;
        }
        "menu-settings-app-hidden" => {
            on_main(app, |handle, _| {
                handle
                    .hide()
                    .map_err(|_| "hide settings application".into())
            })?;
            wait_for(app, "hidden settings application", |_| {
                Ok(NSApplication::sharedApplication(
                    MainThreadMarker::new().ok_or("not main thread")?,
                )
                .isHidden())
            })?;
        }
        _ => return Err("unknown settings acceptance phase".into()),
    }
    on_main(app, |_, _| {
        let application =
            NSApplication::sharedApplication(MainThreadMarker::new().ok_or("not main thread")?);
        let menu = application
            .mainMenu()
            .ok_or("installed native menu missing")?;
        if !invoke_settings(&menu, 0)? {
            return Err("installed settings menu missing".into());
        }
        Ok(())
    })?;
    let observed = Arc::clone(&delivered);
    wait_for(app, "native settings event delivery", move |_| {
        Ok(observed.load(Ordering::SeqCst) > 0)
    })?;
    wait_for(app, "settings window restored", |window| {
        Ok(window.is_visible().map_err(|_| "read visibility")?
            && !window.is_minimized().map_err(|_| "read minimized state")?
            && !NSApplication::sharedApplication(MainThreadMarker::new().ok_or("not main thread")?)
                .isHidden())
    })?;
    // The listener stays live until all asynchronous native restoration checks
    // finish. Do not turn duplicate menu events into an apparent single success.
    // Access it via the callback's shared counter rather than the event payload.
    if delivered.load(Ordering::SeqCst) != 1 {
        return Err("native settings action emitted duplicate events".into());
    }
    Ok(())
}
