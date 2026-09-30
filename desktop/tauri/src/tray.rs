use tauri::{
    menu::{MenuBuilder, MenuItemBuilder},
    tray::{TrayIconBuilder, TrayIconEvent},
    Manager,
};

pub fn show_main_window(app: &tauri::AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.unminimize();
        let _ = window.show();
        let _ = window.set_focus();
    }
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
                let app = tray.app_handle();
                if let Some(window) = app.get_webview_window("main") {
                    if window.is_visible().unwrap_or(false)
                        && !window.is_minimized().unwrap_or(false)
                    {
                        let _ = window.hide();
                    } else {
                        show_main_window(app);
                    }
                }
            }
        })
        .build(app)?;

    Ok(())
}
