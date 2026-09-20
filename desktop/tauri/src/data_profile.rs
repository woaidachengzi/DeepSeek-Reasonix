use std::{
    env, fs,
    path::{Path, PathBuf},
};

use tauri::Manager;

const CORE_PROFILE_DIR: &str = "reasonix-core";

/// Selects the Go core's private storage root for the Tauri preview.
///
/// The established Wails client uses Reasonix's default user profile. A Tauri
/// preview must not silently read, upgrade, or write that profile: apart from
/// corrupting a migration experiment, doing so would let two independently
/// managed desktop hosts write the same configuration and session state.
///
/// An explicitly supplied `REASONIX_HOME` remains authoritative. That is a
/// deliberate developer/operator choice, including for a future, explicit
/// import flow; normal preview launches receive the app-private profile.
pub fn configure_preview_profile(app: &tauri::App) -> Result<PathBuf, String> {
    if let Some(home) = explicit_reasonix_home() {
        return Ok(home);
    }

    let app_data = app
        .path()
        .app_data_dir()
        .map_err(|error| format!("resolve Tauri preview data directory: {error}"))?;
    let home = preview_home(&app_data);
    fs::create_dir_all(&home).map_err(|error| {
        format!(
            "create Tauri preview data directory {}: {error}",
            home.display()
        )
    })?;

    // The Tauri host is single-threaded during setup, before the bridge
    // sidecar is spawned. The sidecar inherits this environment and the Go
    // core routes config, state, sessions, and cache below this root.
    env::set_var("REASONIX_HOME", &home);
    Ok(home)
}

fn explicit_reasonix_home() -> Option<PathBuf> {
    env::var_os("REASONIX_HOME")
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
}

fn preview_home(app_data_dir: &Path) -> PathBuf {
    app_data_dir.join(CORE_PROFILE_DIR)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn preview_profile_is_nested_under_tauri_app_data() {
        let app_data = Path::new("/tmp/reasonix-preview");
        assert_eq!(
            preview_home(app_data),
            PathBuf::from("/tmp/reasonix-preview/reasonix-core")
        );
    }

    // Data safety contract: the Tauri host must never write to the Wails
    // default profile. The preview_home() path always nests under a
    // "reasonix-core" subdirectory, keeping the two hosts' state roots
    // completely independent.

    #[test]
    fn preview_home_is_always_under_core_subdirectory() {
        let home = preview_home(Path::new("/any/data/dir"));
        assert!(
            home.ends_with("reasonix-core"),
            "preview_home must nest under reasonix-core: {}",
            home.display()
        );
        assert!(
            home.starts_with("/any/data/dir"),
            "preview_home must be under the app data dir: {}",
            home.display()
        );
    }

    #[test]
    fn preview_home_does_not_leak_to_parent() {
        let home = preview_home(Path::new("/tmp/state"));
        // The parent must be the app data dir, not the state root itself.
        assert_eq!(
            home.parent(),
            Some(Path::new("/tmp/state")),
            "preview_home must not write directly to the app data dir"
        );
    }

    #[test]
    fn explicit_reasonix_home_overrides_preview_profile() {
        // Simulate an explicitly set REASONIX_HOME — it must be returned
        // as-is, bypassing the preview nesting.
        let custom = PathBuf::from("/custom/reasonix");
        env::set_var("REASONIX_HOME", &custom);
        let result = explicit_reasonix_home();
        assert_eq!(result, Some(custom));
        env::remove_var("REASONIX_HOME");
    }

    #[test]
    fn empty_reasonix_home_is_ignored() {
        env::set_var("REASONIX_HOME", "");
        assert_eq!(explicit_reasonix_home(), None);
        env::remove_var("REASONIX_HOME");
    }

    #[test]
    fn absent_reasonix_home_falls_back_to_preview() {
        env::remove_var("REASONIX_HOME");
        assert_eq!(explicit_reasonix_home(), None);
    }

    // Cross-host isolation: the core profile directory name is hardcoded
    // to "reasonix-core", so the Tauri host and the Wails host can never
    // resolve the same state root by default.
    #[test]
    fn core_profile_dir_constant_prevents_collision() {
        assert_eq!(CORE_PROFILE_DIR, "reasonix-core");
        // Wails uses the bare app data dir (no subdirectory).
        // Tauri nests under "reasonix-core".
        // These must never be equal.
        let wails_root = PathBuf::from("/app/data");
        let tauri_root = preview_home(&wails_root);
        assert_ne!(
            wails_root, tauri_root,
            "Tauri and Wails must not share the same state root"
        );
    }
}
