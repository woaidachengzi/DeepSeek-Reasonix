//! Actual AppKit menu action -> Tauri menu handler -> main-window event.
//! Runs only through the opt-in package runner, with its private profile.

use std::sync::{
    atomic::{AtomicUsize, Ordering},
    Arc,
};

use objc2::MainThreadMarker;
use objc2_app_kit::{NSApplication, NSEventModifierFlags, NSMenu};
use serde::{Deserialize, Serialize};
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

fn invoke_named(
    menu: &NSMenu,
    title: &str,
    depth: usize,
    action: Option<objc2::runtime::Sel>,
) -> Result<bool, String> {
    if depth > 3 || !(0..=128).contains(&menu.numberOfItems()) {
        return Err("native menu exceeds acceptance bounds".into());
    }
    for index in 0..menu.numberOfItems() {
        let item = menu.itemAtIndex(index).ok_or("native menu item missing")?;
        if item.title().to_string() == title {
            if let Some(action) = action {
                if item.action() != Some(action) || item.target().is_some() {
                    return Err("installed edit menu is not a native responder role".into());
                }
                menu.update();
            }
            // performActionForItemAtIndex does not perform automatic validation.
            // Check the actual installed item's state before sending its action.
            if !item.isEnabled() {
                return Err("native acceptance menu is disabled".into());
            }
            menu.performActionForItemAtIndex(index);
            return Ok(true);
        }
        if let Some(submenu) = item.submenu() {
            if invoke_named(&submenu, title, depth + 1, action)? {
                return Ok(true);
            }
        }
    }
    Ok(false)
}

pub(crate) fn invoke(app: &AppHandle, title: &'static str) -> Result<(), String> {
    on_main(app, move |_, _| {
        let application =
            NSApplication::sharedApplication(MainThreadMarker::new().ok_or("not main thread")?);
        let menu = application
            .mainMenu()
            .ok_or("installed native menu missing")?;
        if !invoke_named(&menu, title, 0, None)? {
            return Err("installed acceptance menu missing".into());
        }
        Ok(())
    })
}

pub(crate) fn edit(app: &AppHandle, title: &'static str) -> Result<(), String> {
    let action = match title {
        "Undo" => objc2::sel!(undo:),
        "Redo" => objc2::sel!(redo:),
        "Cut" => objc2::sel!(cut:),
        "Copy" => objc2::sel!(copy:),
        "Paste" => objc2::sel!(paste:),
        "Select All" => objc2::sel!(selectAll:),
        _ => return Err("unknown native edit role".into()),
    };
    on_main(app, move |_, _| {
        let application =
            NSApplication::sharedApplication(MainThreadMarker::new().ok_or("not main thread")?);
        let menu = application
            .mainMenu()
            .ok_or("installed native menu missing")?;
        if !invoke_named(&menu, title, 0, Some(action))? {
            return Err("installed edit menu missing".into());
        }
        Ok(())
    })
    .map_err(|error| format!("native {title}: {error}"))
}

fn minimize_from_menu(app: &AppHandle) -> Result<(), String> {
    on_main(app, |_, window| {
        let application = NSApplication::sharedApplication(
            MainThreadMarker::new().ok_or("native minimize menu not on main thread")?,
        );
        let pointer = window
            .ns_window()
            .map_err(|_| "native minimize window unavailable")?;
        if !application.isActive()
            || !application.keyWindow().is_some_and(|key| {
                std::ptr::eq(
                    std::ptr::from_ref(&*key).cast::<std::ffi::c_void>(),
                    pointer.cast_const(),
                )
            })
        {
            return Err("native minimize menu requires the actual active main key window".into());
        }
        let menu = application
            .mainMenu()
            .ok_or("native minimize menu missing")?;
        if !invoke_named(&menu, "Minimize", 0, Some(objc2::sel!(performMiniaturize:)))? {
            return Err("installed native Minimize role missing".into());
        }
        Ok(())
    })
}

#[derive(Debug, Deserialize, Serialize, PartialEq, Eq)]
struct NativeShortcut {
    title: String,
    key: String,
    #[serde(default)]
    meta: bool,
    #[serde(default)]
    ctrl: bool,
    #[serde(default)]
    alt: bool,
    #[serde(default)]
    shift: bool,
}

#[derive(Deserialize)]
struct ShortcutContract {
    required: Vec<NativeShortcut>,
    optional: Vec<NativeShortcut>,
}

impl NativeShortcut {
    fn same_combo(&self, other: &Self) -> bool {
        self.key == other.key
            && self.meta == other.meta
            && self.ctrl == other.ctrl
            && self.alt == other.alt
            && self.shift == other.shift
    }
}

fn collect_shortcuts(
    menu: &NSMenu,
    depth: usize,
    shortcuts: &mut Vec<NativeShortcut>,
) -> Result<(), String> {
    if depth > 3 || !(0..=128).contains(&menu.numberOfItems()) {
        return Err("native menu exceeds acceptance bounds".into());
    }
    for index in 0..menu.numberOfItems() {
        let item = menu.itemAtIndex(index).ok_or("native menu item missing")?;
        let raw_key = item.keyEquivalent().to_string();
        let flags = item.keyEquivalentModifierMask();
        // Preview bindings require Cmd or Ctrl. macOS also adds Fn/typing
        // chords that cannot be assigned through that configuration layer.
        if !raw_key.is_empty()
            && flags.intersects(NSEventModifierFlags::Command | NSEventModifierFlags::Control)
        {
            let key = if raw_key == " " {
                "Space".into()
            } else {
                raw_key
            };
            shortcuts.push(NativeShortcut {
                title: item.title().to_string(),
                key,
                meta: flags.contains(NSEventModifierFlags::Command),
                ctrl: flags.contains(NSEventModifierFlags::Control),
                alt: flags.contains(NSEventModifierFlags::Option),
                shift: flags.contains(NSEventModifierFlags::Shift),
            });
        }
        if let Some(submenu) = item.submenu() {
            collect_shortcuts(&submenu, depth + 1, shortcuts)?;
        }
    }
    Ok(())
}

pub fn shortcuts(app: &AppHandle) -> Result<(), String> {
    let expected: ShortcutContract = serde_json::from_str(include_str!(
        "../../frontend/src/tauri/macosMenuShortcuts.json"
    ))
    .map_err(|_| "invalid shared macOS shortcut contract")?;
    let actual = on_main(app, |_, _| {
        let application =
            NSApplication::sharedApplication(MainThreadMarker::new().ok_or("not main thread")?);
        let menu = application
            .mainMenu()
            .ok_or("installed native menu missing")?;
        let mut shortcuts = Vec::new();
        collect_shortcuts(&menu, 0, &mut shortcuts)?;
        Ok(shortcuts)
    })?;
    // AppKit supplements menus depending on macOS version and input settings.
    // Required application roles must exist; every actual primary-modifier
    // chord, including optional OS items, must be reserved by the frontend.
    // OS item titles may be localized, so optional entries match by chord.
    if expected
        .required
        .iter()
        .any(|required| !actual.contains(required))
        || actual.iter().any(|installed| {
            !expected.required.contains(installed)
                && !expected
                    .optional
                    .iter()
                    .any(|optional| optional.same_combo(installed))
        })
    {
        return Err(format!(
            "installed menu shortcuts differ from frontend reservation; actual={actual:?}"
        ));
    }
    Ok(())
}

pub fn settings(app: &AppHandle, phase: &str) -> Result<(), String> {
    crate::native_window_smoke::record(app, "settings-ready")?;
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
    crate::native_window_smoke::record(app, "settings-shown")?;

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
        "menu-settings-minimized" | "menu-settings-native-minimized" => {
            if phase == "menu-settings-native-minimized" {
                minimize_from_menu(app)?;
            } else {
                on_main(app, |_, window| {
                    window
                        .minimize()
                        .map_err(|_| "minimize settings window".into())
                })?;
            }
            crate::native_window_smoke::record(app, "settings-minimize-requested")?;
            wait_for(app, "minimized settings window", |window| {
                window
                    .is_minimized()
                    .map_err(|_| "read minimized state".into())
            })?;
            crate::native_window_smoke::record(app, "settings-minimized")?;
            if phase == "menu-settings-native-minimized" {
                wait_for(app, "native menu minimization events", |window| {
                    let state = crate::native_window_smoke::snapshot(window)?;
                    Ok(state["willMiniaturize"]
                        .as_u64()
                        .is_some_and(|value| value > 0)
                        && state["didMiniaturize"]
                            .as_u64()
                            .is_some_and(|value| value > 0))
                })?;
            }
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
    invoke(app, "Settings…")?;
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
    if phase == "menu-settings-native-minimized" {
        wait_for(app, "native menu deminiaturization event", |window| {
            Ok(
                crate::native_window_smoke::snapshot(window)?["didDeminiaturize"]
                    .as_u64()
                    .is_some_and(|value| value > 0),
            )
        })?;
    }
    // The listener stays live until all asynchronous native restoration checks
    // finish. Do not turn duplicate menu events into an apparent single success.
    // Access it via the callback's shared counter rather than the event payload.
    if delivered.load(Ordering::SeqCst) != 1 {
        return Err("native settings action emitted duplicate events".into());
    }
    Ok(())
}
