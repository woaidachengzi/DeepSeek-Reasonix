use tauri::menu::{MenuBuilder, MenuItemBuilder, SubmenuBuilder};

pub fn build_app_menu(
    app: &tauri::App,
) -> Result<tauri::menu::Menu<tauri::Wry>, Box<dyn std::error::Error + Send + Sync>> {
    let handle = app.handle();

    let about = MenuItemBuilder::new("About Reasonix")
        .id("about")
        .build(handle)?;
    let check_updates = MenuItemBuilder::new("Check for Updates…")
        .id("check_updates")
        .build(handle)?;

    let app_menu = SubmenuBuilder::new(handle, "Reasonix")
        .item(&about)
        .separator()
        .item(&check_updates)
        .separator()
        .item(
            &MenuItemBuilder::new("Hide Reasonix")
                .id("hide")
                .accelerator("CmdOrCtrl+H")
                .build(handle)?,
        )
        .item(
            &MenuItemBuilder::new("Hide Others")
                .id("hide_others")
                .accelerator("CmdOrCtrl+Alt+H")
                .build(handle)?,
        )
        .separator()
        .item(
            &MenuItemBuilder::new("Quit Reasonix")
                .id("quit")
                .accelerator("CmdOrCtrl+Q")
                .build(handle)?,
        )
        .build()?;

    let edit_menu = SubmenuBuilder::new(handle, "Edit")
        .item(
            &MenuItemBuilder::new("Undo")
                .id("undo")
                .accelerator("CmdOrCtrl+Z")
                .build(handle)?,
        )
        .item(
            &MenuItemBuilder::new("Redo")
                .id("redo")
                .accelerator("CmdOrCtrl+Shift+Z")
                .build(handle)?,
        )
        .separator()
        .item(
            &MenuItemBuilder::new("Cut")
                .id("cut")
                .accelerator("CmdOrCtrl+X")
                .build(handle)?,
        )
        .item(
            &MenuItemBuilder::new("Copy")
                .id("copy")
                .accelerator("CmdOrCtrl+C")
                .build(handle)?,
        )
        .item(
            &MenuItemBuilder::new("Paste")
                .id("paste")
                .accelerator("CmdOrCtrl+V")
                .build(handle)?,
        )
        .item(
            &MenuItemBuilder::new("Select All")
                .id("select_all")
                .accelerator("CmdOrCtrl+A")
                .build(handle)?,
        )
        .build()?;

    let view_menu = SubmenuBuilder::new(handle, "View")
        .item(
            &MenuItemBuilder::new("Reload")
                .id("reload")
                .accelerator("CmdOrCtrl+R")
                .build(handle)?,
        )
        .item(
            &MenuItemBuilder::new("Force Reload")
                .id("force_reload")
                .accelerator("CmdOrCtrl+Shift+R")
                .build(handle)?,
        )
        .separator()
        .item(
            &MenuItemBuilder::new("Toggle Full Screen")
                .id("toggle_fullscreen")
                .accelerator("Ctrl+Cmd+F")
                .build(handle)?,
        )
        .separator()
        .item(
            &MenuItemBuilder::new("Zoom In")
                .id("zoom_in")
                .accelerator("CmdOrCtrl+=")
                .build(handle)?,
        )
        .item(
            &MenuItemBuilder::new("Zoom Out")
                .id("zoom_out")
                .accelerator("CmdOrCtrl+-")
                .build(handle)?,
        )
        .item(
            &MenuItemBuilder::new("Reset Zoom")
                .id("zoom_reset")
                .accelerator("CmdOrCtrl+0")
                .build(handle)?,
        )
        .build()?;

    let window_menu = SubmenuBuilder::new(handle, "Window")
        .item(
            &MenuItemBuilder::new("Minimize")
                .id("minimize")
                .accelerator("CmdOrCtrl+M")
                .build(handle)?,
        )
        .item(&MenuItemBuilder::new("Zoom").id("zoom").build(handle)?)
        .separator()
        .item(
            &MenuItemBuilder::new("Bring All to Front")
                .id("bring_all_to_front")
                .build(handle)?,
        )
        .build()?;

    let menu = MenuBuilder::new(handle)
        .item(&app_menu)
        .item(&edit_menu)
        .item(&view_menu)
        .item(&window_menu)
        .build()?;

    Ok(menu)
}
