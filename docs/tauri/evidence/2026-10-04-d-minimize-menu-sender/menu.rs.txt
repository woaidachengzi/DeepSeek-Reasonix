#[cfg(target_os = "macos")]
use tauri::menu::{MenuBuilder, MenuItemBuilder, PredefinedMenuItem, SubmenuBuilder};

/// Match the Wails baseline: macOS uses native responder-chain edit roles.
/// Ordinary items with Cmd+C/V accelerators consume the keys without editing.
#[cfg(target_os = "macos")]
pub fn build_app_menu(
    app: &tauri::App,
) -> Result<tauri::menu::Menu<tauri::Wry>, Box<dyn std::error::Error + Send + Sync>> {
    let handle = app.handle();
    let app_menu = SubmenuBuilder::new(handle, "Reasonix")
        .item(
            &MenuItemBuilder::new("About Reasonix")
                .id("about")
                .build(handle)?,
        )
        .separator()
        // The frontend owns configurable shortcuts, including Settings and
        // text size. Do not install fixed native accelerators for these items.
        .item(
            &MenuItemBuilder::new("Settings…")
                .id("settings")
                .build(handle)?,
        )
        .item(
            &MenuItemBuilder::new("Updates…")
                .id("check_updates")
                .build(handle)?,
        )
        .separator()
        .item(&PredefinedMenuItem::hide(handle, Some("Hide Reasonix"))?)
        .item(&PredefinedMenuItem::hide_others(handle, None)?)
        .item(&PredefinedMenuItem::show_all(handle, None)?)
        .separator()
        .item(
            &MenuItemBuilder::new("Quit Reasonix")
                .id("quit")
                .accelerator("CmdOrCtrl+Q")
                .build(handle)?,
        )
        .build()?;

    let edit_menu = SubmenuBuilder::new(handle, "Edit")
        .item(&PredefinedMenuItem::undo(handle, None)?)
        .item(&PredefinedMenuItem::redo(handle, None)?)
        .separator()
        .item(&PredefinedMenuItem::cut(handle, None)?)
        .item(&PredefinedMenuItem::copy(handle, None)?)
        .item(&PredefinedMenuItem::paste(handle, None)?)
        .item(&PredefinedMenuItem::select_all(handle, None)?)
        .build()?;

    let view_menu = SubmenuBuilder::new(handle, "View")
        .item(&MenuItemBuilder::new("Reload").id("reload").build(handle)?)
        .separator()
        .item(&PredefinedMenuItem::fullscreen(handle, None)?)
        .separator()
        .item(
            &MenuItemBuilder::new("Zoom In")
                .id("zoom_in")
                .build(handle)?,
        )
        .item(
            &MenuItemBuilder::new("Zoom Out")
                .id("zoom_out")
                .build(handle)?,
        )
        .item(
            &MenuItemBuilder::new("Reset Zoom")
                .id("zoom_reset")
                .build(handle)?,
        )
        .build()?;

    let window_menu = SubmenuBuilder::new(handle, "Window")
        .item(&PredefinedMenuItem::minimize(handle, None)?)
        .item(&PredefinedMenuItem::maximize(handle, Some("Zoom"))?)
        .separator()
        .item(
            &MenuItemBuilder::new("Show Reasonix")
                .id("show_main_window")
                .build(handle)?,
        )
        .build()?;

    Ok(MenuBuilder::new(handle)
        .item(&app_menu)
        .item(&edit_menu)
        .item(&view_menu)
        .item(&window_menu)
        .build()?)
}
