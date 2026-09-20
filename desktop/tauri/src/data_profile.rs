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
}
