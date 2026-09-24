#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod bridge;
mod data_profile;
mod keychain;
mod menu;
mod protocol_generated;
mod runtime_info;
mod session_shadow;
mod tray;
mod window_state;
mod workbench_catalog;
mod workbench_projects;

#[cfg(test)]
pub(crate) mod test_env {
    use std::sync::{Mutex, MutexGuard};

    static PROCESS_ENV_LOCK: Mutex<()> = Mutex::new(());

    pub(crate) struct Guard {
        _lock: MutexGuard<'static, ()>,
        reasonix_home: Option<std::ffi::OsString>,
        reasonix_state_home: Option<std::ffi::OsString>,
    }

    pub(crate) fn guard() -> Guard {
        let lock = PROCESS_ENV_LOCK
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        Guard {
            _lock: lock,
            reasonix_home: std::env::var_os("REASONIX_HOME"),
            reasonix_state_home: std::env::var_os("REASONIX_STATE_HOME"),
        }
    }

    impl Drop for Guard {
        fn drop(&mut self) {
            restore("REASONIX_HOME", self.reasonix_home.take());
            restore("REASONIX_STATE_HOME", self.reasonix_state_home.take());
        }
    }

    fn restore(name: &str, value: Option<std::ffi::OsString>) {
        match value {
            Some(value) => std::env::set_var(name, value),
            None => std::env::remove_var(name),
        }
    }
}

use serde::Serialize;
use std::path::Path;

use bridge::{
    AnswerMCPInteractionRequest, AnswerQuestionRequest, ApproveRequest, AttachFileRequest,
    BridgeAttachment, BridgeDeleteSessionResponse, BridgeHistory, BridgeProjectFolder,
    BridgeProviderSummaryResponse, BridgeSession, BridgeSetDefaultModelRequest, BridgeSnapshot,
    BridgeStatus, BridgeSupervisor, BridgeWorkspaceChangeDetailResponse,
    BridgeWorkspaceChangesResponse, BridgeWorkspaceFileResponse, BridgeWorkspaceListResponse,
    LegacySessionCatalogEntry, MCPServerDeleteRequest, MCPServerInput, MCPServerMutationResponse,
    MCPServerView, OpenSessionRequest, RenameSessionRequest, SessionCatalogMetadata,
    SessionDirectoryCursor, SessionDirectoryEntry, SessionDirectoryPage, SessionFirstMessageTitle,
    SessionPreview, SessionRequest, SubmitRequest, WorkspaceChangeDetailRequest,
    WorkspaceFileRequest, WorkspaceRequest,
};
use data_profile::{
    PreviewProfile, PreviewProfileStatus, ProfileImportResult, ProjectFoldersImportResult,
};
use runtime_info::PreviewRuntimeInfo;
use session_shadow::SessionShadowReport;
use tauri::{Manager, State};
use window_state::PreviewWindowState;
use workbench_catalog::{WorkbenchCatalog, WorkbenchSession, WorkbenchTitle};
use workbench_projects::WorkbenchProjectCatalog;

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct WorkbenchSessionPage {
    sessions: Vec<WorkbenchSessionPageEntry>,
    next_cursor: Option<SessionDirectoryCursor>,
    total: u64,
    source: &'static str,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct WorkbenchSessionPageEntry {
    session_id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    workspace_root: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    state: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    missing: Option<bool>,
}

fn page_entries_from_legacy(
    sessions: Vec<WorkbenchSession>,
    directory: Option<&[SessionDirectoryEntry]>,
) -> Vec<WorkbenchSessionPageEntry> {
    sessions
        .into_iter()
        .map(|session| {
            let identity = directory
                .and_then(|entries| entries.iter().find(|entry| entry.id == session.session_id));
            WorkbenchSessionPageEntry {
                session_id: session.session_id,
                title: session.title,
                workspace_root: session.workspace_root,
                state: identity.map(|entry| entry.state.clone()),
                missing: identity.map(|entry| entry.missing || entry.state == "missing"),
            }
        })
        .collect()
}

fn page_entry_from_identity(entry: SessionDirectoryEntry) -> WorkbenchSessionPageEntry {
    WorkbenchSessionPageEntry {
        session_id: entry.id,
        title: (!entry.title.is_empty()).then_some(entry.title),
        workspace_root: entry.workspace_root,
        state: Some(entry.state),
        missing: Some(entry.missing),
    }
}

#[tauri::command]
fn bridge_status(supervisor: State<'_, BridgeSupervisor>) -> BridgeStatus {
    supervisor.status()
}

#[tauri::command]
fn restart_bridge(
    supervisor: State<'_, BridgeSupervisor>,
    keychain: State<'_, keychain::KeychainStore>,
) -> Result<BridgeStatus, String> {
    keychain.restart_bridge(&supervisor)
}

#[tauri::command]
fn bridge_open_session(
    supervisor: State<'_, BridgeSupervisor>,
    request: OpenSessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.open_session(request)
}

#[tauri::command]
fn bridge_switch_session(
    supervisor: State<'_, BridgeSupervisor>,
    request: OpenSessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.switch_session(request)
}

#[tauri::command]
fn bridge_rename_session(
    supervisor: State<'_, BridgeSupervisor>,
    request: RenameSessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.rename_session(request)
}

#[tauri::command]
fn bridge_delete_session(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeDeleteSessionResponse, String> {
    supervisor.delete_session(request)
}

#[tauri::command]
fn list_mcp_servers(
    supervisor: State<'_, BridgeSupervisor>,
    workspace_root: Option<String>,
) -> Result<Vec<MCPServerView>, String> {
    supervisor.mcp_servers(workspace_root.as_deref())
}

#[tauri::command]
fn save_mcp_server(
    supervisor: State<'_, BridgeSupervisor>,
    request: MCPServerInput,
    workspace_root: Option<String>,
) -> Result<MCPServerMutationResponse, String> {
    supervisor.save_mcp_server(request, workspace_root.as_deref())
}

#[tauri::command]
fn delete_mcp_server(
    supervisor: State<'_, BridgeSupervisor>,
    request: MCPServerDeleteRequest,
    workspace_root: Option<String>,
) -> Result<MCPServerMutationResponse, String> {
    supervisor.delete_mcp_server(request.name, workspace_root.as_deref())
}

#[tauri::command]
fn bridge_session_snapshot(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeSnapshot, String> {
    supervisor.snapshot(request)
}

#[tauri::command]
fn bridge_session_history(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeHistory, String> {
    supervisor.history(request)
}

#[tauri::command]
fn bridge_submit(
    supervisor: State<'_, BridgeSupervisor>,
    request: SubmitRequest,
) -> Result<BridgeSession, String> {
    supervisor.submit(request)
}

#[tauri::command]
fn bridge_attach_file(
    supervisor: State<'_, BridgeSupervisor>,
    request: AttachFileRequest,
) -> Result<BridgeAttachment, String> {
    supervisor.attach_file(request)
}

#[tauri::command]
fn bridge_workspace(
    supervisor: State<'_, BridgeSupervisor>,
    request: WorkspaceRequest,
) -> Result<BridgeWorkspaceListResponse, String> {
    supervisor.workspace(request)
}

#[tauri::command]
fn bridge_workspace_file(
    supervisor: State<'_, BridgeSupervisor>,
    request: WorkspaceFileRequest,
) -> Result<BridgeWorkspaceFileResponse, String> {
    supervisor.workspace_file(request)
}

#[tauri::command]
fn bridge_workspace_changes(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeWorkspaceChangesResponse, String> {
    supervisor.workspace_changes(request)
}

#[tauri::command]
fn bridge_workspace_change_detail(
    supervisor: State<'_, BridgeSupervisor>,
    request: WorkspaceChangeDetailRequest,
) -> Result<BridgeWorkspaceChangeDetailResponse, String> {
    supervisor.workspace_change_detail(request)
}

fn workspace_root_is_available(root: &str) -> Option<bool> {
    let path = root.trim();
    if path.is_empty() || path.len() > 4096 || !Path::new(path).is_absolute() {
        return Some(false);
    }
    match std::fs::metadata(path) {
        Ok(metadata) => Some(metadata.is_dir()),
        Err(error)
            if error.kind() == std::io::ErrorKind::NotFound
                || error.kind() == std::io::ErrorKind::NotADirectory =>
        {
            Some(false)
        }
        Err(_) => None,
    }
}

#[tauri::command]
fn workspace_roots_availability(roots: Vec<String>) -> Result<Vec<Option<bool>>, String> {
    if roots.len() > 200 {
        return Err("too many workspace roots to inspect".into());
    }
    Ok(roots
        .iter()
        .map(|root| workspace_root_is_available(root))
        .collect())
}

#[tauri::command]
fn bridge_cancel(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.cancel(request)
}

#[tauri::command]
fn bridge_approve(
    supervisor: State<'_, BridgeSupervisor>,
    request: ApproveRequest,
) -> Result<BridgeSession, String> {
    supervisor.approve(request)
}

#[tauri::command]
fn bridge_answer_question(
    supervisor: State<'_, BridgeSupervisor>,
    request: AnswerQuestionRequest,
) -> Result<BridgeSession, String> {
    supervisor.answer_question(request)
}

#[tauri::command]
fn bridge_answer_mcp_interaction(
    supervisor: State<'_, BridgeSupervisor>,
    request: AnswerMCPInteractionRequest,
) -> Result<BridgeSession, String> {
    supervisor.answer_mcp_interaction(request)
}

#[tauri::command]
fn bridge_replay_pending_prompts(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.replay_pending_prompts(request)
}

#[tauri::command]
fn bridge_start_events(
    app: tauri::AppHandle,
    supervisor: State<'_, BridgeSupervisor>,
    after_sequence: Option<u64>,
) -> Result<(), String> {
    supervisor.start_events(app, after_sequence.unwrap_or(0))
}

#[tauri::command]
fn preview_profile_status(profile: State<'_, PreviewProfile>) -> PreviewProfileStatus {
    profile.status()
}

#[tauri::command]
fn preview_runtime_info(supervisor: State<'_, BridgeSupervisor>) -> PreviewRuntimeInfo {
    PreviewRuntimeInfo::from_status(supervisor.status())
}

#[tauri::command]
fn provider_summary(
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<BridgeProviderSummaryResponse, String> {
    supervisor.provider_summary()
}

#[tauri::command]
fn set_default_model(
    supervisor: State<'_, BridgeSupervisor>,
    request: BridgeSetDefaultModelRequest,
) -> Result<BridgeProviderSummaryResponse, String> {
    supervisor.set_default_model(request)
}

#[tauri::command]
fn import_stable_profile(
    profile: State<'_, PreviewProfile>,
    confirmed: bool,
) -> Result<ProfileImportResult, String> {
    if !confirmed {
        return Err("stable config import requires explicit confirmation".into());
    }
    profile.import_stable_config()
}

#[tauri::command]
fn import_stable_project_folders(
    profile: State<'_, PreviewProfile>,
    confirmed: bool,
) -> Result<ProjectFoldersImportResult, String> {
    if !confirmed {
        return Err("stable project folder import requires explicit confirmation".into());
    }
    profile.import_stable_project_folders()
}

#[tauri::command]
fn workbench_sessions(
    catalog: State<'_, WorkbenchCatalog>,
) -> Result<Vec<WorkbenchSession>, String> {
    catalog.list()
}

#[tauri::command]
fn workbench_project_folders(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchProjectCatalog>,
) -> Result<Vec<BridgeProjectFolder>, String> {
    merged_workbench_project_folders(&supervisor, &catalog)
}

#[tauri::command]
fn remember_workbench_project_folder(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchProjectCatalog>,
    root: String,
) -> Result<Vec<BridgeProjectFolder>, String> {
    catalog.remember(&root)?;
    merged_workbench_project_folders(&supervisor, &catalog)
}

#[tauri::command]
fn rename_workbench_project_folder(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchProjectCatalog>,
    root: String,
    title: String,
) -> Result<Vec<BridgeProjectFolder>, String> {
    catalog.set_title(&root, &title)?;
    merged_workbench_project_folders(&supervisor, &catalog)
}

fn merged_workbench_project_folders(
    supervisor: &BridgeSupervisor,
    catalog: &WorkbenchProjectCatalog,
) -> Result<Vec<BridgeProjectFolder>, String> {
    let mut folders = supervisor.project_folders().unwrap_or_default();
    let local = match catalog.list() {
        Ok(local) => local,
        Err(_) if !folders.is_empty() => return Ok(folders),
        Err(error) => return Err(error),
    };
    for folder in local {
        if let Some(existing) = folders.iter_mut().find(|existing| {
            workbench_project_key(&existing.root) == workbench_project_key(&folder.root)
        }) {
            if folder.title.is_some() {
                existing.title = folder.title;
            }
        } else {
            folders.push(folder);
        }
    }
    Ok(folders)
}

fn workbench_project_key(root: &str) -> String {
    let root = root.trim().trim_end_matches(['/', '\\']);
    if cfg!(windows) {
        root.to_ascii_lowercase()
    } else {
        root.to_string()
    }
}

#[tauri::command]
fn workbench_session_page(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchCatalog>,
    limit: Option<u16>,
    cursor: Option<SessionDirectoryCursor>,
) -> Result<WorkbenchSessionPage, String> {
    // The legacy JSON catalog remains the recovery source until SQLite has
    // been shadow-verified. A readable but stale identity DB is not enough to
    // switch the visible sidebar.
    let legacy_sessions = if cursor.is_none() {
        Some(catalog.list()?)
    } else {
        None
    };
    let mut shadow_directory = None;
    if let Some(sessions) = legacy_sessions.as_ref() {
        match compare_session_catalog_with_directory(&supervisor, &catalog) {
            Ok((report, directory)) if identity_session_catalog_is_verified(Ok(report.clone())) => {
                shadow_directory = Some(directory);
            }
            Ok((_, directory)) => {
                let total = sessions.len() as u64;
                return Ok(WorkbenchSessionPage {
                    sessions: page_entries_from_legacy(sessions.clone(), Some(&directory)),
                    next_cursor: None,
                    total,
                    source: "legacy",
                });
            }
            Err(_) => {
                let total = sessions.len() as u64;
                return Ok(WorkbenchSessionPage {
                    sessions: page_entries_from_legacy(sessions.clone(), None),
                    next_cursor: None,
                    total,
                    source: "legacy",
                });
            }
        }
    }

    match supervisor.session_directory_page(limit.unwrap_or(200), cursor.clone(), None) {
        Ok(page) => Ok(WorkbenchSessionPage {
            sessions: page
                .sessions
                .into_iter()
                .map(page_entry_from_identity)
                .collect(),
            next_cursor: page.next_cursor,
            total: page.total,
            source: "identity",
        }),
        Err(_error) if cursor.is_none() => {
            let sessions = match legacy_sessions {
                Some(sessions) => sessions,
                None => catalog.list()?,
            };
            let total = sessions.len() as u64;
            Ok(WorkbenchSessionPage {
                sessions: page_entries_from_legacy(sessions, shadow_directory.as_deref()),
                next_cursor: None,
                total,
                source: "legacy",
            })
        }
        Err(error) => Err(error),
    }
}

#[tauri::command]
fn bridge_session_previews(
    supervisor: State<'_, BridgeSupervisor>,
    session_ids: Vec<String>,
) -> Result<Vec<SessionPreview>, String> {
    supervisor.session_previews(session_ids)
}

#[tauri::command]
fn bridge_session_directory_page(
    supervisor: State<'_, BridgeSupervisor>,
    limit: Option<u16>,
    cursor: Option<SessionDirectoryCursor>,
    workspace_root: Option<String>,
) -> Result<SessionDirectoryPage, String> {
    supervisor.session_directory_page(limit.unwrap_or(200), cursor, workspace_root)
}

#[tauri::command]
fn bridge_import_legacy_session_catalog(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchCatalog>,
) -> Result<usize, String> {
    let sessions = catalog.list()?;
    let imported = supervisor.import_legacy_session_catalog(
        sessions
            .iter()
            .map(|session| LegacySessionCatalogEntry {
                session_id: session.session_id.clone(),
                title: session.title.clone(),
                workspace_root: session.workspace_root.clone(),
            })
            .collect(),
    )?;
    sync_workbench_order(&supervisor, &sessions)?;
    Ok(imported)
}

fn sync_workbench_order(
    supervisor: &BridgeSupervisor,
    sessions: &[WorkbenchSession],
) -> Result<usize, String> {
    if sessions.is_empty() {
        return Ok(0);
    }
    let synced = supervisor.sync_session_catalog(
        sessions
            .iter()
            .map(|session| SessionCatalogMetadata {
                session_id: session.session_id.clone(),
                workspace_root: session.workspace_root.clone(),
            })
            .collect(),
    )?;
    require_complete_workbench_sync(sessions.len(), synced)
}

fn require_complete_workbench_sync(expected: usize, synced: usize) -> Result<usize, String> {
    if synced == expected {
        Ok(synced)
    } else {
        Err("session catalog sync was incomplete".into())
    }
}

#[cfg(test)]
mod workbench_order_sync_tests {
    use super::require_complete_workbench_sync;

    #[test]
    fn accepts_complete_sync_and_rejects_partial_sync() {
        assert_eq!(require_complete_workbench_sync(3, 3), Ok(3));
        assert_eq!(
            require_complete_workbench_sync(3, 2),
            Err("session catalog sync was incomplete".into())
        );
    }
}

fn compare_session_catalog(
    supervisor: &BridgeSupervisor,
    catalog: &WorkbenchCatalog,
) -> Result<SessionShadowReport, String> {
    compare_session_catalog_with_directory(supervisor, catalog).map(|(report, _)| report)
}

fn compare_session_catalog_with_directory(
    supervisor: &BridgeSupervisor,
    catalog: &WorkbenchCatalog,
) -> Result<(SessionShadowReport, Vec<SessionDirectoryEntry>), String> {
    let legacy = catalog.list()?;
    let directory = supervisor.session_directory_snapshot()?;
    let physical = supervisor.session_physical_inventory()?;
    let report = session_shadow::compare(&legacy, &directory, &physical)?;
    Ok((report, directory))
}

fn identity_session_catalog_is_verified(report: Result<SessionShadowReport, String>) -> bool {
    report.is_ok_and(|report| report.legacy_matches_directory)
}

#[tauri::command]
fn bridge_session_catalog_shadow(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchCatalog>,
) -> Result<SessionShadowReport, String> {
    // Bridge inventory errors can contain private filesystem paths. Keep the
    // UI-facing diagnostic count-only even when the shadow scan fails.
    compare_session_catalog(&supervisor, &catalog)
        .map_err(|_| "session catalog shadow is unavailable; retry later".to_string())
}

#[tauri::command]
fn backfill_workbench_titles(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchCatalog>,
    titles: Vec<WorkbenchTitle>,
) -> Result<Vec<WorkbenchSession>, String> {
    let sessions = catalog.list()?;
    let missing: Vec<_> = titles
        .into_iter()
        .filter(|title| {
            sessions
                .iter()
                .any(|session| session.session_id == title.session_id && session.title.is_none())
        })
        .collect();
    if missing.is_empty() {
        return Ok(sessions);
    }
    let resolved = supervisor.backfill_session_titles(
        missing
            .iter()
            .map(|title| SessionFirstMessageTitle {
                session_id: title.session_id.clone(),
                title: title.title.clone(),
            })
            .collect(),
    )?;
    catalog.fill_titles(
        resolved
            .into_iter()
            .map(|title| WorkbenchTitle {
                session_id: title.session_id,
                title: title.title,
            })
            .collect(),
    )
}

#[tauri::command]
fn remember_workbench_session(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchCatalog>,
    request: WorkbenchSession,
) -> Result<Vec<WorkbenchSession>, String> {
    let sessions = catalog.remember(request)?;
    sync_workbench_order(&supervisor, &sessions)?;
    Ok(sessions)
}

#[tauri::command]
fn forget_workbench_session(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchCatalog>,
    session_id: String,
) -> Result<Vec<WorkbenchSession>, String> {
    let sessions = catalog.forget(&session_id)?;
    sync_workbench_order(&supervisor, &sessions)?;
    Ok(sessions)
}

#[tauri::command]
fn platform_info() -> &'static str {
    if cfg!(target_os = "macos") {
        "darwin"
    } else if cfg!(target_os = "windows") {
        "windows"
    } else {
        "linux"
    }
}

fn main() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            // When a second instance is launched, focus the existing window
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.show();
                let _ = window.set_focus();
            }
        }))
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_notification::init())
        .setup(|app| {
            let window_state = PreviewWindowState::for_app(app)?;
            let workbench_catalog =
                WorkbenchCatalog::for_app(app).map_err(std::io::Error::other)?;
            let project_catalog =
                WorkbenchProjectCatalog::for_app(app).map_err(std::io::Error::other)?;
            if let Some(window) = app.get_webview_window("main") {
                window_state.restore(&window);
            }
            let profile =
                data_profile::configure_preview_profile(app).map_err(std::io::Error::other)?;
            let supervisor = BridgeSupervisor::from_environment(app.handle().clone());

            let menu = menu::build_app_menu(app)
                .map_err(|e| std::io::Error::other(e.to_string()))?;
            app.set_menu(menu)?;

            tray::create_tray(app)
                .map_err(|e| std::io::Error::other(e.to_string()))?;

            // Initialize keychain store
            let keychain = keychain::KeychainStore::new();
            keychain.initialize(app.handle()).map_err(std::io::Error::other)?;

            supervisor.start().map_err(std::io::Error::other)?;
            if let Err(error) = keychain.restore_provider_api_keys(&supervisor) {
                // A native credential service can be temporarily unavailable.
                // Keep the app usable with its existing file-backed settings;
                // the provider will simply remain unavailable until the next
                // startup or a user saves the key again.
                eprintln!("Reasonix could not restore system keychain credentials: {error}");
            }
            app.manage(keychain);

            app.manage(supervisor);
            app.manage(profile);
            app.manage(window_state);
            app.manage(workbench_catalog);
            app.manage(project_catalog);
            Ok(())
        })
        .on_window_event(|window, event| {
            // macOS: hide to tray on close button instead of quitting
            #[cfg(target_os = "macos")]
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                // Prevent the default close behavior
                api.prevent_close();
                // Hide the window instead
                let _ = window.hide();
            }
        })
        .on_menu_event(|app, event| {
            match event.id().as_ref() {
                "quit" => {
                    // Cmd+Q: actually quit the application
                    app.exit(0);
                }
                "about" => {
                    let version = env!("CARGO_PKG_VERSION");
                    let stable = runtime_info::STABLE_VERSION;
                    let commit = &runtime_info::STABLE_COMMIT[..8];
                    let msg = format!(
                        "Reasonix Tauri Preview\n\nVersion: {version}\nBased on Stable: {stable} ({commit})\nTauri: {}\nBridge Protocol: {}",
                        tauri::VERSION,
                        bridge::PROTOCOL_VERSION
                    );
                    let dialog = tauri_plugin_dialog::DialogExt::dialog(app);
                    let _ = dialog
                        .message(&msg)
                        .title("About Reasonix")
                        .kind(tauri_plugin_dialog::MessageDialogKind::Info)
                        .blocking_show();
                }
                "check_updates" => {
                    let version = env!("CARGO_PKG_VERSION");
                    let msg = format!(
                        "Current version: {version}\n\nAuto-update checks are performed at startup.\nTo manually check, restart the application."
                    );
                    let dialog = tauri_plugin_dialog::DialogExt::dialog(app);
                    let _ = dialog
                        .message(&msg)
                        .title("Check for Updates")
                        .kind(tauri_plugin_dialog::MessageDialogKind::Info)
                        .blocking_show();
                }
                "reload" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.eval("location.reload()");
                    }
                }
                "force_reload" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.eval("location.reload()");
                    }
                }
                "toggle_fullscreen" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.set_fullscreen(!window.is_fullscreen().unwrap_or(false));
                    }
                }
                "zoom_in" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.eval("document.body.style.zoom = (parseFloat(document.body.style.zoom || '1') + 0.1).toString()");
                    }
                }
                "zoom_out" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.eval("document.body.style.zoom = Math.max(0.5, parseFloat(document.body.style.zoom || '1') - 0.1).toString()");
                    }
                }
                "zoom_reset" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.eval("document.body.style.zoom = '1'");
                    }
                }
                "minimize" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.minimize();
                    }
                }
                "hide" => {
                    let _ = app.hide();
                }
                "hide_others" => {
                    // macOS-only: hide other applications
                    #[cfg(target_os = "macos")]
                    {
                        // Tauri doesn't expose hide_others directly; we rely on
                        // the native menu accelerator to handle this via macOS.
                        // The menu item with Cmd+Alt+H triggers the system behavior.
                    }
                }
                _ => {}
            }
        })
        .invoke_handler(tauri::generate_handler![
            bridge_status,
            restart_bridge,
            bridge_open_session,
            bridge_switch_session,
            bridge_rename_session,
            bridge_delete_session,
            list_mcp_servers,
            save_mcp_server,
            delete_mcp_server,
            bridge_session_snapshot,
            bridge_session_history,
            bridge_session_previews,
            bridge_session_directory_page,
            bridge_session_catalog_shadow,
            bridge_import_legacy_session_catalog,
            bridge_submit,
            bridge_attach_file,
            bridge_workspace,
            bridge_workspace_file,
            bridge_workspace_changes,
            bridge_workspace_change_detail,
            workspace_roots_availability,
            bridge_cancel,
            bridge_approve,
            bridge_answer_question,
            bridge_answer_mcp_interaction,
            bridge_replay_pending_prompts,
            bridge_start_events,
            preview_profile_status,
            preview_runtime_info,
            provider_summary,
            set_default_model,
            import_stable_profile,
            import_stable_project_folders,
            workbench_sessions,
            workbench_project_folders,
            remember_workbench_project_folder,
            rename_workbench_project_folder,
            workbench_session_page,
            backfill_workbench_titles,
            remember_workbench_session,
            forget_workbench_session,
            platform_info,
            keychain::keychain_save,
            keychain::keychain_load,
            keychain::keychain_delete
        ])
        .build(tauri::generate_context!())
        .expect("failed to build Reasonix Tauri host");
    app.run(|app, event| {
        if matches!(event, tauri::RunEvent::Exit) {
            if let Some(window) = app.get_webview_window("main") {
                let _ = app.state::<PreviewWindowState>().save(&window);
            }
            let _ = app.state::<BridgeSupervisor>().stop();
        }
    });
}

#[cfg(test)]
mod session_catalog_gate_tests {
    use super::*;

    fn report(clean: bool) -> SessionShadowReport {
        SessionShadowReport {
            legacy_count: 1,
            directory_count: 1,
            matched_count: 1,
            directory_only_count: 0,
            missing_from_directory: 0,
            title_mismatches: 0,
            workspace_mismatches: 0,
            order_mismatches: 0,
            missing_transcripts: 0,
            physical_state_mismatches: 0,
            unclaimed_transcripts: 0,
            inventory_errors: 0,
            legacy_matches_directory: clean,
        }
    }

    #[test]
    fn identity_sidebar_requires_a_clean_successful_shadow_report() {
        assert!(identity_session_catalog_is_verified(Ok(report(true))));
        assert!(!identity_session_catalog_is_verified(Ok(report(false))));
        assert!(!identity_session_catalog_is_verified(Err(
            "inventory unavailable".into()
        )));
    }
}

#[cfg(test)]
mod workbench_page_lifecycle_tests {
    use super::*;

    #[test]
    fn legacy_fallback_preserves_known_missing_identity_state() {
        let legacy = WorkbenchSession {
            session_id: "missing-session".into(),
            title: Some("Old conversation".into()),
            workspace_root: Some("/work/project".into()),
        };
        let identity = SessionDirectoryEntry {
            id: "missing-session".into(),
            title: "Old conversation".into(),
            title_source: "manual".into(),
            workspace_root: Some("/work/project".into()),
            state: "missing".into(),
            missing: true,
            position: 0,
            updated_at_ms: 0,
        };

        let page = page_entries_from_legacy(vec![legacy], Some(&[identity]));
        assert_eq!(page[0].state.as_deref(), Some("missing"));
        assert_eq!(page[0].missing, Some(true));
    }
}

#[cfg(test)]
mod workspace_availability_tests {
    use super::workspace_root_is_available;

    #[test]
    fn workspace_availability_distinguishes_invalid_and_existing_directories() {
        assert_eq!(workspace_root_is_available("relative/project"), Some(false));
        assert_eq!(workspace_root_is_available(""), Some(false));
        assert_eq!(
            workspace_root_is_available(&std::env::temp_dir().to_string_lossy()),
            Some(true)
        );
    }
}
