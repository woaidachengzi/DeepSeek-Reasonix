use tauri::{
    menu::{MenuBuilder, MenuItem, MenuItemBuilder},
    tray::{TrayIconBuilder, TrayIconEvent},
    Manager,
};

pub struct TrayMenuState {
    open: MenuItem<tauri::Wry>,
    quit: MenuItem<tauri::Wry>,
}

impl TrayMenuState {
    pub fn set_locale(&self, locale: &str) -> Result<(), String> {
        let (open, quit) = match locale {
            "zh" | "zh-TW" => ("打开", "退出"),
            "en" => ("Open", "Quit"),
            _ => return Err("unsupported tray language".into()),
        };
        let previous_open = self.open.text().map_err(|_| "read tray language")?;
        self.open.set_text(open).map_err(|_| "update tray language")?;
        if self.quit.set_text(quit).is_err() {
            let _ = self.open.set_text(previous_open);
            return Err("update tray language; restart Preview and retry".into());
        }
        Ok(())
    }

    #[cfg(target_os = "macos")]
    pub(crate) fn labels(&self) -> Result<(String, String), String> {
        Ok((self.open.text().map_err(|_| "read tray open title")?,
            self.quit.text().map_err(|_| "read tray quit title")?))
    }
}

pub fn show_main_window(app: &tauri::AppHandle) {
    #[cfg(target_os = "macos")]
    crate::native_window_smoke::observe_restore(app, false);
    #[cfg(target_os = "macos")]
    {
        let handle = app.clone();
        let _ = app.run_on_main_thread(move || crate::initial_presentation::request(&handle));
    }
    #[cfg(not(target_os = "macos"))]
    present_main_window(app, true);
}

pub(crate) fn present_main_window(app: &tauri::AppHandle, unhide: bool) {
    #[cfg(not(target_os = "macos"))]
    let _ = unhide;
    let handle = app.clone();
    // Singleton callbacks arrive on a worker thread. Restore the application
    // and its window together on the native UI thread; showing an NSWindow
    // alone does not undo the macOS application's Hide action.
    let _ = app.run_on_main_thread(move || {
        #[cfg(target_os = "macos")]
        if !crate::initial_presentation::can_present(&handle) {
            return;
        }
        #[cfg(target_os = "macos")]
        if let Some(main_thread) = objc2::MainThreadMarker::new() {
            if objc2_app_kit::NSApplication::sharedApplication(main_thread).isHidden() {
                // An automatic initial reveal must not undo a user Hide.
                if !unhide { return; }
                let _ = handle.show();
            }
        }
        if let Some(window) = handle.get_webview_window("main") {
            if let Some(state) = handle.try_state::<crate::window_state::PreviewWindowState>() {
                state.restore(&window);
            }
            let _ = window.unminimize();
            let _ = window.show();
            let _ = window.set_focus();
        }
        #[cfg(target_os = "macos")]
        crate::native_window_smoke::observe_restore(&handle, true);
    });
}

pub fn create_tray(app: &tauri::App) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let open = MenuItemBuilder::new("Open").id("tray_show").build(app)?;
    let quit = MenuItemBuilder::new("Quit").id("tray_quit").build(app)?;
    let menu = MenuBuilder::new(app)
        .item(&open)
        .separator()
        .item(&quit)
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

    app.manage(TrayMenuState { open, quit });

    Ok(())
}
