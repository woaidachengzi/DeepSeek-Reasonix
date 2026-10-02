use tauri::{
    menu::{MenuBuilder, MenuItemBuilder},
    tray::{TrayIconBuilder, TrayIconEvent},
    Manager,
};

pub fn show_main_window(app: &tauri::AppHandle) {
    #[cfg(target_os = "macos")]
    crate::native_window_smoke::observe_restore(app, false);
    let handle = app.clone();
    // Singleton callbacks arrive on a worker thread. Restore the application
    // and its window together on the native UI thread; showing an NSWindow
    // alone does not undo the macOS application's Hide action.
    let _ = app.run_on_main_thread(move || {
        #[cfg(target_os = "macos")]
        if let Some(main_thread) = objc2::MainThreadMarker::new() {
            if objc2_app_kit::NSApplication::sharedApplication(main_thread).isHidden() {
                let _ = handle.show();
            }
        }
        if let Some(window) = handle.get_webview_window("main") {
            let _ = window.unminimize();
            let _ = window.show();
            let _ = window.set_focus();
        }
        #[cfg(target_os = "macos")]
        crate::native_window_smoke::observe_restore(&handle, true);
    });
}

pub fn create_tray(app: &tauri::App) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let menu = MenuBuilder::new(app)
        .item(
            &MenuItemBuilder::new("Show Reasonix")
                .id("tray_show")
                .build(app)?,
        )
        .separator()
        .item(
            &MenuItemBuilder::new("Quit Reasonix")
                .id("tray_quit")
                .build(app)?,
        )
        .build()?;
    let _tray = TrayIconBuilder::new()
        .icon(app.default_window_icon().expect("no default icon").clone())
        .tooltip("Reasonix")
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(|app, event| match event.id().as_ref() {
            "tray_show" => show_main_window(app),
            "tray_quit" => app.exit(0),
            _ => {}
        })
        .on_tray_icon_event(|tray, event| {
            if matches!(
                event,
                TrayIconEvent::Click {
                    button: tauri::tray::MouseButton::Left,
                    button_state: tauri::tray::MouseButtonState::Up,
                    ..
                }
            ) {
                // Match the stable shell's primary tap: open/restore, even
                // when already visible. Use the same application-unhide and
                // window-focus path as the Show menu, Dock and singleton.
                show_main_window(tray.app_handle());
            }
        })
        .build(app)?;

    Ok(())
}
