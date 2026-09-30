//! Packaged config import/restart acceptance in the runner's private HOME.
//! Calls the same confirmed host entry points as the settings page.

use std::{
    path::Path,
    time::{Duration, Instant},
};

use tauri::{AppHandle, Manager};

use crate::{
    bridge::{BridgeSetDefaultModelRequest, BridgeSupervisor},
    data_profile::PreviewProfile,
};

fn wait_control(directory: &Path, name: &str) -> Result<(), String> {
    let deadline = Instant::now() + Duration::from_secs(20);
    while Instant::now() < deadline {
        if directory.join(name).is_file() {
            return Ok(());
        }
        std::thread::sleep(Duration::from_millis(50));
    }
    Err("profile acceptance control timeout".into())
}

fn verify_provider(supervisor: &BridgeSupervisor, model: &str) -> Result<(), String> {
    let summary = supervisor.provider_summary()?;
    if summary.default_model != model
        || !summary.providers.iter().any(|provider| {
            provider.name == "native-import" && provider.models == ["alpha", "beta"]
        })
    {
        return Err("imported provider/default model not visible through real bridge".into());
    }
    Ok(())
}

fn run(app: &AppHandle, directory: &Path, phase: &str) -> Result<serde_json::Value, String> {
    std::fs::write(
        directory.join("reasonix-native-profile-waiting.json"),
        b"{}",
    )
    .map_err(|_| "publish profile acceptance readiness")?;
    wait_control(directory, "reasonix-native-profile-begin.json")?;
    let profile = app.state::<PreviewProfile>();
    let supervisor = app.state::<BridgeSupervisor>();
    let before = supervisor.status();
    if !before.running {
        return Err("profile acceptance sidecar not running".into());
    }
    if crate::import_stable_profile(app.state(), false).is_ok()
        || crate::import_stable_project_folders(app.state(), false).is_ok()
    {
        return Err("profile import accepted without confirmation".into());
    }
    let mut backup = None;
    match phase {
        "import" => {
            let status = profile.status();
            if !status.managed_profile
                || !status.import_available
                || !status.project_folders_import_available
            {
                return Err("fresh managed profile did not offer stable imports".into());
            }
            let source = status.stable_config.ok_or("stable fixture missing")?;
            let original = std::fs::read(source).map_err(|_| "read private stable fixture")?;
            let imported = crate::import_stable_profile(app.state(), true)?;
            if std::fs::read(&imported.imported_config).map_err(|_| "read imported config")?
                != original
                || std::fs::read(&imported.backup_config).map_err(|_| "read config backup")?
                    != original
            {
                return Err("profile import did not preserve original bytes".into());
            }
            backup = Some(imported.backup_config);
            let folders = crate::import_stable_project_folders(app.state(), true)?;
            if folders.project_count != 1 {
                return Err("project folder import count differs".into());
            }
            verify_provider(&supervisor, "native-import/alpha")?;
            let updated = supervisor.set_default_model(BridgeSetDefaultModelRequest {
                model: "native-import/beta".into(),
                scope: Some("global".into()),
                workspace_root: None,
            })?;
            if updated.default_model != "native-import/beta" {
                return Err("Preview config update failed".into());
            }
            let restarted = supervisor.restart()?;
            if !restarted.running || restarted.sidecar_instance_id == before.sidecar_instance_id {
                return Err("profile acceptance did not replace sidecar on restart".into());
            }
            verify_provider(&supervisor, "native-import/beta")?;
        }
        "restore" => {
            if !profile.status().managed_profile {
                return Err("restore profile not managed".into());
            }
            verify_provider(&supervisor, "native-import/beta")?;
        }
        "explicit" => {
            let status = profile.status();
            if status.managed_profile
                || status.import_available
                || status.project_folders_import_available
            {
                return Err("explicit profile offered automatic imports".into());
            }
            verify_provider(&supervisor, "native-import/alpha")?;
        }
        _ => return Err("unknown native profile acceptance phase".into()),
    }
    if crate::import_stable_profile(app.state(), true).is_ok()
        || crate::import_stable_project_folders(app.state(), true).is_ok()
    {
        return Err("profile import overwrote existing/explicit data".into());
    }
    Ok(
        serde_json::json!({"phase": phase, "ok": true, "backup": backup,
        "initialInstance": before.sidecar_instance_id, "finalInstance": supervisor.status().sidecar_instance_id}),
    )
}

pub fn start_if_requested(app: &AppHandle) {
    let Ok(phase) = std::env::var("REASONIX_TAURI_PROFILE_SMOKE") else {
        return;
    };
    let Some(directory) = std::env::var_os("TMPDIR")
        .map(std::path::PathBuf::from)
        .filter(|path| path.is_absolute() && path.is_dir())
    else {
        app.exit(2);
        return;
    };
    let handle = app.clone();
    std::thread::spawn(move || {
        let result = run(&handle, &directory, &phase);
        let ok = result.is_ok();
        let value = result
            .unwrap_or_else(|error| serde_json::json!({"phase":phase,"ok":false,"error":error}));
        if std::fs::write(
            directory.join("reasonix-native-profile-result.json"),
            value.to_string(),
        )
        .is_err()
        {
            handle.exit(2);
            return;
        }
        if !ok {
            handle.exit(2);
            return;
        }
        if wait_control(&directory, "reasonix-native-profile-exit.json").is_err() {
            handle.exit(2);
            return;
        }
        handle.exit(0);
    });
}
