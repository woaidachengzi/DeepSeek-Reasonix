use tauri::Manager;

#[test]
fn configured_asset_scope_serves_only_app_theme_assets() {
    let mut config: tauri::Config =
        serde_json::from_str(include_str!("../tauri.conf.json")).unwrap();
    // Resolve the real Tauri variable without touching the user's Preview data.
    config.identifier = "io.reasonix.theme-scope-fixture".into();
    let asset_config = config.app.security.asset_protocol.clone();
    assert!(asset_config.enable);
    let mut context = tauri::test::mock_context(tauri::test::noop_assets());
    *context.config_mut() = config;
    let app = tauri::test::mock_builder().build(context).unwrap();
    let app_data = app.path().app_data_dir().unwrap();
    let scope = tauri::scope::fs::Scope::new(&app, &asset_config.scope).unwrap();

    for image in ["background.webp", "background-task.webp"] {
        assert!(
            scope.is_allowed(app_data.join("theme-assets/user-ui-import").join(image)),
            "imported theme image must be allowed: {image}"
        );
    }
    for path in [
        app_data.join("host-preferences.json"),
        app_data.join("reasonix-core/credentials.json"),
        app_data.join("theme-assets/../host-preferences.json"),
        app_data.join("theme-assets/.hidden/background.webp"),
        app_data
            .parent()
            .unwrap()
            .join("foreign/theme-assets/background.webp"),
        std::env::temp_dir().join("reasonix-theme-scope-foreign.webp"),
    ] {
        assert!(
            !scope.is_allowed(&path),
            "foreign path must be denied: {}",
            path.display()
        );
    }
}
