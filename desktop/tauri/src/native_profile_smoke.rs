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
    if phase == "parallel" {
        return run_parallel(&supervisor, &profile, directory);
    }
    if phase == "takeover" {
        if !profile.status().takeover_profile || profile.status().managed_profile {
            return Err("takeover installation did not select the original profile".into());
        }
        let receipt: serde_json::Value = serde_json::from_slice(
            &std::fs::read(profile.home().join("tauri-takeover-v1.json"))
                .map_err(|_| "read takeover acceptance receipt")?,
        ).map_err(|_| "decode takeover acceptance receipt")?;
        let rows = receipt["rows"].as_array().ok_or("takeover rows unavailable")?;
        let page = crate::read_workbench_session_page(
            &supervisor, &app.state::<crate::WorkbenchCatalog>(),
            &app.state::<crate::SessionShadowSnapshotCache>(), Some(200), None,
        )?;
        if page.source == "identity_unverified" || page.total != rows.len() as u64 {
            return Err(format!("takeover navigation source {} count {}", page.source, page.total));
        }
        let entries = rows.iter().map(|row| Ok(crate::bridge::SessionCatalogMetadata {
            session_id: row["sessionId"].as_str().ok_or("takeover identity unavailable")?.into(),
            workspace_root: row["workspaceRoot"].as_str().filter(|value| !value.is_empty()).map(str::to_owned),
        })).collect::<Result<Vec<_>, String>>()?;
        if supervisor.sync_session_catalog(entries)? != rows.len() {
            return Err("takeover recent catalog did not preserve every identity".into());
        }
        let mut counts = Vec::new();
        for row in rows {
            let id = row["sessionId"].as_str().ok_or("takeover identity unavailable")?;
            let workspace = row["workspaceRoot"].as_str().filter(|value| !value.is_empty()).map(str::to_owned);
            let opened = supervisor.switch_session(crate::bridge::OpenSessionRequest {
                session_id: id.into(), workspace_root: workspace,
                model_ref: None, effort: None,
            })?;
            if opened.id != id { return Err("takeover opened a different identity".into()); }
            let history = supervisor.history(crate::bridge::SessionRequest { session_id: id.into() })?;
            counts.push(history.total_messages);
        }
        let initial = supervisor.status().sidecar_instance_id;
        let restarted = supervisor.restart()?;
        if !restarted.running || restarted.sidecar_instance_id == initial {
            return Err("takeover restart did not replace the sidecar".into());
        }
        for (row, expected) in rows.iter().zip(&counts) {
            let id = row["sessionId"].as_str().ok_or("takeover identity unavailable")?;
            supervisor.switch_session(crate::bridge::OpenSessionRequest {
                session_id: id.into(),
                workspace_root: row["workspaceRoot"].as_str().filter(|value| !value.is_empty()).map(str::to_owned),
                model_ref: None, effort: None,
            })?;
            let history = supervisor.history(crate::bridge::SessionRequest { session_id: id.into() })?;
            if history.total_messages != *expected { return Err("takeover history changed after restart".into()); }
        }
        return Ok(serde_json::json!({"phase":phase,"ok":true,"opened":rows.len(),"historyCounts":counts,"restart":true,"navigationSource":page.source}));
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

// Uses the production supervisor and core; no UI scripting or real model API.
// The runner owns an explicit private profile and a bounded loopback fixture.
fn run_parallel(
    supervisor: &BridgeSupervisor,
    profile: &PreviewProfile,
    directory: &Path,
) -> Result<serde_json::Value, String> {
    let home = profile
        .home()
        .canonicalize()
        .map_err(|_| "parallel fixture profile unavailable")?;
    let control = directory
        .canonicalize()
        .map_err(|_| "parallel fixture controls unavailable")?;
    if profile.status().takeover_profile
        || !home.starts_with("/private/tmp")
        || !control.starts_with("/private/tmp")
    {
        return Err("parallel acceptance requires a private Preview fixture".into());
    }
    let config = std::fs::read_to_string(home.join("config.toml"))
        .map_err(|_| "parallel fixture config unavailable")?;
    let fixture: serde_json::Value = serde_json::from_slice(
        &std::fs::read(home.join("parallel-fixture.json"))
            .map_err(|_| "parallel fixture declaration unavailable")?,
    )
    .map_err(|_| "invalid parallel fixture declaration")?;
    let port = fixture["port"]
        .as_u64()
        .filter(|port| *port > 0 && *port <= 65535)
        .ok_or("invalid parallel fixture port")?;
    if !parallel_fixture_config(&config, port) {
        return Err("parallel acceptance requires the local fixture provider".into());
    }
    let workspace = home.join("parallel-workspace");
    std::fs::create_dir_all(&workspace).map_err(|_| "prepare parallel workspace")?;
    let open = |id: &str| {
        supervisor.switch_session(crate::bridge::OpenSessionRequest {
            session_id: id.into(),
            workspace_root: Some(workspace.to_string_lossy().into_owned()),
            model_ref: None,
            effort: None,
        })
    };
    let wait = |id: &str, streaming: bool| -> Result<crate::bridge::BridgeSnapshot, String> {
        let deadline = Instant::now() + Duration::from_secs(15);
        while Instant::now() < deadline {
            let snapshot = supervisor.snapshot(crate::bridge::SessionRequest {
                session_id: id.into(),
            })?;
            let ready = if streaming {
                snapshot.session.state == "running"
                    && snapshot.live_state.as_ref().is_some_and(|live| {
                        live.active
                            && live
                                .answer
                                .segments
                                .iter()
                                .any(|text| text.contains("native-before-close"))
                    })
            } else {
                snapshot.session.state == "idle"
            };
            if ready {
                return Ok(snapshot);
            }
            std::thread::sleep(Duration::from_millis(50));
        }
        Err("parallel fixture state timed out".into())
    };
    open("native-parallel-a")?;
    supervisor.submit(crate::bridge::SubmitRequest {
        session_id: "native-parallel-a".into(),
        input: "private fixture task A".into(),
    })?;
    wait("native-parallel-a", true)?;
    open("native-parallel-b")?;
    supervisor.submit(crate::bridge::SubmitRequest {
        session_id: "native-parallel-b".into(),
        input: "private fixture task B".into(),
    })?;
    wait("native-parallel-b", true)?;
    let inventory = supervisor.live_sessions()?;
    if inventory.sessions.len() != 2
        || inventory
            .sessions
            .iter()
            .any(|session| session.state != "running")
    {
        return Err("packaged supervisor did not preserve two running sessions".into());
    }
    // A's snapshot remains readable while the selected owner is B.
    wait("native-parallel-a", true)?;
    supervisor.cancel(crate::bridge::SessionRequest {
        session_id: "native-parallel-a".into(),
    })?;
    wait("native-parallel-a", false)?;
    supervisor.apply_session_settings(crate::bridge::SessionRequest {
        session_id: "native-parallel-a".into(),
    })?;
    wait("native-parallel-b", true)?;
    let inventory = supervisor.live_sessions()?;
    if inventory.selected_session_id != "native-parallel-b" {
        return Err("background cancellation changed selection".into());
    }
    std::fs::write(
        directory.join("reasonix-native-parallel-cancelled-a.json"),
        b"{}",
    )
    .map_err(|_| "publish parallel cancel marker")?;
    Ok(
        serde_json::json!({"phase":"parallel","ok":true,"runningOwners":2,"backgroundSnapshot":true,"independentCancel":true,"scopedSettingsApply":true,"quitWithBackgroundTask":true}),
    )
}

fn parallel_fixture_config(config: &str, port: u64) -> bool {
    let expected = format!("default_model = \"local/alpha\"\n\n[desktop]\nprovider_access = [\"local\"]\n\n[[providers]]\nname = \"local\"\nkind = \"openai\"\nbase_url = \"http://127.0.0.1:{port}/v1\"\nmodels = [\"alpha\"]\ndefault = \"alpha\"\n");
    config.trim() == expected.trim()
}

#[cfg(test)]
mod parallel_tests {
    use super::parallel_fixture_config;
    #[test]
    fn parallel_fixture_refuses_remote_provider_and_credentials() {
        let config = "default_model = \"local/alpha\"\n\n[desktop]\nprovider_access = [\"local\"]\n\n[[providers]]\nname = \"local\"\nkind = \"openai\"\nbase_url = \"http://127.0.0.1:1234/v1\"\nmodels = [\"alpha\"]\ndefault = \"alpha\"\n";
        assert!(parallel_fixture_config(config, 1234));
        assert!(!parallel_fixture_config(
            &config.replace("127.0.0.1", "remote.example"),
            1234
        ));
        assert!(!parallel_fixture_config(
            &format!("{config}api_key_env = \"REAL_KEY\"\n"),
            1234
        ));
        assert!(!parallel_fixture_config(config, 4321));
    }
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
