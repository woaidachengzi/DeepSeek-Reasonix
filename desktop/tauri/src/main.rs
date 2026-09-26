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
        preview_sqlite_events: Option<std::ffi::OsString>,
    }

    pub(crate) fn guard() -> Guard {
        let lock = PROCESS_ENV_LOCK
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        Guard {
            _lock: lock,
            reasonix_home: std::env::var_os("REASONIX_HOME"),
            reasonix_state_home: std::env::var_os("REASONIX_STATE_HOME"),
            preview_sqlite_events: std::env::var_os("REASONIX_PREVIEW_SQLITE_EVENTS"),
        }
    }

    impl Drop for Guard {
        fn drop(&mut self) {
            restore("REASONIX_HOME", self.reasonix_home.take());
            restore("REASONIX_STATE_HOME", self.reasonix_state_home.take());
            restore(
                "REASONIX_PREVIEW_SQLITE_EVENTS",
                self.preview_sqlite_events.take(),
            );
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
use std::{
    collections::{HashMap, HashSet},
    path::Path,
    sync::Mutex,
};

use bridge::{
    AnswerMCPInteractionRequest, AnswerQuestionRequest, ApproveRequest, AttachFileRequest,
    BridgeAttachment, BridgeDeleteSessionResponse, BridgeHistory, BridgeProjectFolder,
    BridgeProviderSummaryResponse, BridgeSession, BridgeSetDefaultModelRequest, BridgeSnapshot,
    BridgeStatus, BridgeSupervisor, BridgeWorkspaceChangeDetailResponse,
    BridgeWorkspaceChangesResponse, BridgeWorkspaceFileResponse, BridgeWorkspaceListResponse,
    LegacySessionCatalogEntry, MCPServerDeleteRequest, MCPServerInput, MCPServerMutationResponse,
    MCPServerView, OpenSessionRequest, PendingSessionDeleteCursor, PendingSessionDeletePage,
    PendingSessionTitleRecovery, RenameSessionRequest, ScanImportCandidate, ScanImportSelection,
    SessionCatalogMetadata, SessionDirectoryCursor, SessionDirectoryEntry, SessionDirectoryPage,
    SessionFirstMessageTitle, SessionPreview, SessionRequest, SubmitRequest,
    WorkspaceChangeDetailRequest, WorkspaceFileRequest, WorkspaceRequest,
};
use data_profile::{
    PreviewProfile, PreviewProfileStatus, ProfileImportResult, ProjectFoldersImportResult,
};
use runtime_info::PreviewRuntimeInfo;
use session_shadow::SessionShadowReport;
use tauri::{Manager, State};
use window_state::PreviewWindowState;
use workbench_catalog::{WorkbenchCatalog, WorkbenchSession, WorkbenchTitle};
use workbench_projects::{normalized_project_key, WorkbenchProjectCatalog};

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct WorkbenchSessionPage {
    sessions: Vec<WorkbenchSessionPageEntry>,
    next_cursor: Option<SessionDirectoryCursor>,
    total: u64,
    source: &'static str,
    #[serde(skip_serializing_if = "Option::is_none")]
    unverified_legacy_sessions: Option<Vec<WorkbenchSessionPageEntry>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    shadow_directory_count: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    unclaimed_transcript_count: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    title_mismatch_count: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    missing_transcript_count: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    shadow_report: Option<SessionShadowReport>,
    #[serde(skip_serializing_if = "Option::is_none")]
    catalog_warning: Option<String>,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct WorkbenchProjectFoldersResponse {
    folders: Vec<BridgeProjectFolder>,
    #[serde(skip_serializing_if = "Option::is_none")]
    warning: Option<String>,
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

#[derive(Default)]
struct SessionShadowSnapshotCache(Mutex<Option<CachedSessionShadowSnapshot>>);

struct CachedSessionShadowSnapshot {
    legacy_sessions: Vec<WorkbenchSession>,
    snapshot_id: String,
    // A path-free projection retained for stable continuations or clearly
    // labelled cached pages. Dirty snapshots remain explicitly unverified;
    // their pages are matched to this exact projection and stay read-only.
    directory: Vec<SessionDirectoryEntry>,
    identity_safe: bool,
    unverified_legacy_sessions: Vec<WorkbenchSessionPageEntry>,
    unclaimed_transcript_count: u64,
    title_mismatch_count: u64,
    missing_transcript_count: u64,
    shadow_report: SessionShadowReport,
}

fn cached_cursor_matches(
    snapshot: &CachedSessionShadowSnapshot,
    cursor: &SessionDirectoryCursor,
) -> bool {
    cursor.snapshot_id.as_deref() == Some(snapshot.snapshot_id.as_str())
        && cursor.total == snapshot.directory.len() as u64
}

impl SessionShadowSnapshotCache {
    fn matches(
        &self,
        legacy_sessions: &[WorkbenchSession],
        cursor: &SessionDirectoryCursor,
    ) -> bool {
        let Ok(cached) = self.0.lock() else {
            return false;
        };
        cached.as_ref().is_some_and(|snapshot| {
            session_catalog_structure_matches(&snapshot.legacy_sessions, legacy_sessions)
                && cached_cursor_matches(snapshot, cursor)
        })
    }

    fn unclaimed_transcript_count(
        &self,
        legacy_sessions: &[WorkbenchSession],
        cursor: &SessionDirectoryCursor,
    ) -> Option<u64> {
        let cached = self.0.lock().ok()?;
        let snapshot = cached.as_ref()?;
        (session_catalog_structure_matches(&snapshot.legacy_sessions, legacy_sessions)
            && cached_cursor_matches(snapshot, cursor))
        .then_some(snapshot.unclaimed_transcript_count)
    }

    fn title_mismatch_count(
        &self,
        legacy_sessions: &[WorkbenchSession],
        cursor: &SessionDirectoryCursor,
    ) -> Option<u64> {
        let cached = self.0.lock().ok()?;
        let snapshot = cached.as_ref()?;
        (session_catalog_structure_matches(&snapshot.legacy_sessions, legacy_sessions)
            && cached_cursor_matches(snapshot, cursor))
        .then_some(snapshot.title_mismatch_count)
    }

    fn missing_transcript_count(
        &self,
        legacy_sessions: &[WorkbenchSession],
        cursor: &SessionDirectoryCursor,
    ) -> Option<u64> {
        let cached = self.0.lock().ok()?;
        let snapshot = cached.as_ref()?;
        (session_catalog_structure_matches(&snapshot.legacy_sessions, legacy_sessions)
            && cached_cursor_matches(snapshot, cursor))
        .then_some(snapshot.missing_transcript_count)
    }

    fn shadow_report(
        &self,
        legacy_sessions: &[WorkbenchSession],
        cursor: &SessionDirectoryCursor,
    ) -> Option<SessionShadowReport> {
        let cached = self.0.lock().ok()?;
        let snapshot = cached.as_ref()?;
        (session_catalog_structure_matches(&snapshot.legacy_sessions, legacy_sessions)
            && cached_cursor_matches(snapshot, cursor))
        .then(|| snapshot.shadow_report.clone())
    }

    fn identity_safe(
        &self,
        legacy_sessions: &[WorkbenchSession],
        cursor: &SessionDirectoryCursor,
    ) -> Option<bool> {
        let cached = self.0.lock().ok()?;
        let snapshot = cached.as_ref()?;
        (session_catalog_structure_matches(&snapshot.legacy_sessions, legacy_sessions)
            && cached_cursor_matches(snapshot, cursor))
        .then_some(snapshot.identity_safe)
    }

    fn live_page_matches(
        &self,
        legacy_sessions: &[WorkbenchSession],
        page: &SessionDirectoryPage,
        cursor: &SessionDirectoryCursor,
        limit: u16,
    ) -> bool {
        let Ok(cached) = self.0.lock() else {
            return false;
        };
        let Some(snapshot) = cached.as_ref() else {
            return false;
        };
        session_catalog_structure_matches(&snapshot.legacy_sessions, legacy_sessions)
            && cached_cursor_matches(snapshot, cursor)
            && (!snapshot.identity_safe
                || session_page_matches_legacy_workspace(page, legacy_sessions))
            && page.snapshot_id == snapshot.snapshot_id
            && session_page_matches_shadow_snapshot(page, &snapshot.directory, Some(cursor), limit)
    }

    fn remember(&self, snapshot: CachedSessionShadowSnapshot) {
        if let Ok(mut cached) = self.0.lock() {
            *cached = Some(snapshot);
        }
    }

    fn page(
        &self,
        legacy_sessions: &[WorkbenchSession],
        cursor: Option<&SessionDirectoryCursor>,
        limit: u16,
    ) -> Option<WorkbenchSessionPage> {
        let cached = self.0.lock().ok()?;
        let snapshot = cached.as_ref()?;
        if !session_catalog_structure_matches(&snapshot.legacy_sessions, legacy_sessions)
            || !(1..=200).contains(&limit)
            || cursor.is_some_and(|cursor| !cached_cursor_matches(snapshot, cursor))
        {
            return None;
        }
        let start = match cursor {
            None => 0,
            Some(cursor) => {
                snapshot
                    .directory
                    .binary_search_by(|entry| {
                        (entry.position, entry.id.as_str())
                            .cmp(&(cursor.position, cursor.id.as_str()))
                    })
                    .ok()?
                    + 1
            }
        };
        let end = start
            .saturating_add(usize::from(limit))
            .min(snapshot.directory.len());
        let sessions = snapshot.directory[start..end]
            .iter()
            .cloned()
            .map(page_entry_from_identity)
            .collect::<Vec<_>>();
        // Cached pages keep the previously audited projection and are always
        // read-only. For a clean snapshot, compatibility workspace changes
        // invalidate the page; a structurally conflicting snapshot remains
        // explicitly unverified and is matched against its own shadow rows.
        if snapshot.identity_safe
            && !session_page_matches_legacy_workspace_entries(&sessions, legacy_sessions)
        {
            return None;
        }
        if !session_page_matches_shadow_entries(&sessions, &snapshot.directory, cursor, limit) {
            return None;
        }
        let next_cursor = (end < snapshot.directory.len()).then(|| {
            let last = &snapshot.directory[end - 1];
            SessionDirectoryCursor {
                position: last.position,
                id: last.id.clone(),
                snapshot_id: Some(snapshot.snapshot_id.clone()),
                total: snapshot.directory.len() as u64,
            }
        });
        Some(WorkbenchSessionPage {
            sessions,
            next_cursor,
            total: snapshot.directory.len() as u64,
            source: "cached",
            unverified_legacy_sessions: (cursor.is_none()
                && !snapshot.identity_safe
                && !snapshot.unverified_legacy_sessions.is_empty())
            .then(|| snapshot.unverified_legacy_sessions.clone()),
            shadow_directory_count: Some(snapshot.directory.len() as u64),
            unclaimed_transcript_count: (snapshot.unclaimed_transcript_count > 0)
                .then_some(snapshot.unclaimed_transcript_count),
            title_mismatch_count: (snapshot.title_mismatch_count > 0)
                .then_some(snapshot.title_mismatch_count),
            missing_transcript_count: (snapshot.missing_transcript_count > 0)
                .then_some(snapshot.missing_transcript_count),
            shadow_report: Some(snapshot.shadow_report.clone()),
            catalog_warning: None,
        })
    }

    fn clear(&self) {
        if let Ok(mut cached) = self.0.lock() {
            *cached = None;
        }
    }
}

fn session_catalog_structure_matches(
    previous: &[WorkbenchSession],
    current: &[WorkbenchSession],
) -> bool {
    previous.len() == current.len()
        && previous.iter().zip(current).all(|(previous, current)| {
            previous.session_id == current.session_id
                && previous.workspace_root == current.workspace_root
        })
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

fn workbench_page_from_unverified_identity(page: SessionDirectoryPage) -> WorkbenchSessionPage {
    WorkbenchSessionPage {
        sessions: page
            .sessions
            .into_iter()
            .map(page_entry_from_identity)
            .collect(),
        next_cursor: page.next_cursor,
        total: page.total,
        source: "identity_unverified",
        unverified_legacy_sessions: None,
        shadow_directory_count: Some(page.total),
        unclaimed_transcript_count: None,
        title_mismatch_count: None,
        missing_transcript_count: None,
        shadow_report: None,
        catalog_warning: None,
    }
}

fn apply_catalog_read_warning(
    mut page: WorkbenchSessionPage,
    catalog_warning: Option<&str>,
) -> WorkbenchSessionPage {
    if let Some(warning) = catalog_warning {
        if matches!(page.source, "identity" | "partial_identity") {
            // An unreadable compatibility catalog cannot establish a clean
            // shadow relationship. Keep the persistent directory visible,
            // but never make its rows actionable on this evidence alone.
            page.source = "identity_unverified";
            page.unverified_legacy_sessions = None;
        }
        page.catalog_warning = Some(warning.to_string());
    }
    page
}

fn legacy_sessions_missing_from_identity(
    legacy: &[WorkbenchSession],
    directory: &[SessionDirectoryEntry],
) -> Vec<WorkbenchSessionPageEntry> {
    let identity_ids: HashSet<_> = directory.iter().map(|entry| entry.id.as_str()).collect();
    page_entries_from_legacy(
        legacy
            .iter()
            .filter(|session| !identity_ids.contains(session.session_id.as_str()))
            .cloned()
            .collect(),
        Some(directory),
    )
}

fn session_page_matches_shadow_entries(
    page: &[WorkbenchSessionPageEntry],
    directory: &[SessionDirectoryEntry],
    after: Option<&SessionDirectoryCursor>,
    limit: u16,
) -> bool {
    let start = match after {
        None => 0,
        Some(cursor) => match directory.binary_search_by(|entry| {
            (entry.position, entry.id.as_str()).cmp(&(cursor.position, cursor.id.as_str()))
        }) {
            Ok(index) => index + 1,
            Err(_) => return false,
        },
    };
    let end = (start + usize::from(limit)).min(directory.len());
    page.len() == end - start
        && page
            .iter()
            .zip(&directory[start..end])
            .all(|(entry, audited)| {
                entry.session_id == audited.id
                    && entry.workspace_root == audited.workspace_root
                    && entry.state.as_deref() == Some(audited.state.as_str())
                    && entry.missing == Some(audited.missing)
            })
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
fn bridge_pending_session_deletes_page(
    supervisor: State<'_, BridgeSupervisor>,
    cursor: Option<PendingSessionDeleteCursor>,
) -> Result<PendingSessionDeletePage, String> {
    supervisor.pending_session_deletes_page(cursor)
}

#[tauri::command]
fn bridge_pending_session_title_recoveries(
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<Vec<PendingSessionTitleRecovery>, String> {
    supervisor.pending_session_title_recoveries()
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
    if root.trim().is_empty() || root.len() > 4096 || !Path::new(root).is_absolute() {
        return Some(false);
    }
    match std::fs::metadata(root) {
        Ok(metadata) => Some(metadata.is_dir()),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Some(false),
        Err(_)
            if Path::new(root).ancestors().skip(1).any(|parent| {
                std::fs::metadata(parent).is_ok_and(|metadata| !metadata.is_dir())
            }) =>
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
) -> WorkbenchProjectFoldersResponse {
    merged_workbench_project_folders(&supervisor, &catalog)
}

#[tauri::command]
fn remember_workbench_project_folder(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchProjectCatalog>,
    root: String,
) -> Result<WorkbenchProjectFoldersResponse, String> {
    catalog.remember(&root)?;
    Ok(merged_workbench_project_folders(&supervisor, &catalog))
}

#[tauri::command]
fn rename_workbench_project_folder(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchProjectCatalog>,
    root: String,
    title: String,
) -> Result<WorkbenchProjectFoldersResponse, String> {
    catalog.set_title(&root, &title)?;
    Ok(merged_workbench_project_folders(&supervisor, &catalog))
}

fn merged_workbench_project_folders(
    supervisor: &BridgeSupervisor,
    catalog: &WorkbenchProjectCatalog,
) -> WorkbenchProjectFoldersResponse {
    let (mut folders, legacy_available) = match supervisor.project_folders() {
        Ok(folders) => (folders, true),
        Err(_) => (Vec::new(), false),
    };
    let (local, local_available) = match catalog.list() {
        Ok(local) => (local, true),
        Err(_) => (Vec::new(), false),
    };
    let mut folder_indexes = HashMap::with_capacity(folders.len() + local.len());
    for (index, folder) in folders.iter().enumerate() {
        folder_indexes
            .entry(normalized_project_key(&folder.root))
            .or_insert(index);
    }
    for folder in local {
        let key = normalized_project_key(&folder.root);
        if let Some(&index) = folder_indexes.get(&key) {
            if folder.title.is_some() {
                folders[index].title = folder.title;
            }
        } else {
            folder_indexes.insert(key, folders.len());
            folders.push(folder);
        }
    }
    WorkbenchProjectFoldersResponse {
        folders,
        warning: match (legacy_available, local_available) {
            (true, true) => None,
            (false, true) => {
                Some("旧版项目文件夹清单暂不可用；当前仅显示 Tauri 本地保存的文件夹。".to_string())
            }
            (true, false) => {
                Some("Tauri 本地项目文件夹清单暂不可用；当前仅显示旧版项目来源。".to_string())
            }
            (false, false) => Some(
                "旧版与 Tauri 本地项目文件夹清单都暂不可用，已保存的空项目文件夹可能未显示。"
                    .to_string(),
            ),
        },
    }
}

#[tauri::command]
fn workbench_session_page(
    supervisor: State<'_, BridgeSupervisor>,
    catalog: State<'_, WorkbenchCatalog>,
    shadow_cache: State<'_, SessionShadowSnapshotCache>,
    limit: Option<u16>,
    cursor: Option<SessionDirectoryCursor>,
) -> Result<WorkbenchSessionPage, String> {
    // The legacy JSON catalog remains the actionable recovery source until
    // SQLite has been shadow-verified. When shadow audit is unavailable but
    // the identity page itself is readable, it may be paged for visibility
    // only, with session actions disabled.
    let legacy_sessions_result = catalog.list();
    let catalog_warning = legacy_sessions_result.as_ref().err().map(|_| {
        "旧会话兼容目录无法读取；当前仅显示未核验的持久目录，不能据此操作会话。修复目录后重启 Preview 并重新检查。"
    });
    let legacy_catalog_readable = catalog_warning.is_none();
    let legacy_sessions = legacy_sessions_result.unwrap_or_default();
    let limit = limit.unwrap_or(200);
    if cursor.is_none() && !supervisor.status().running {
        if let Some(page) = shadow_cache.page(&legacy_sessions, None, limit) {
            return Ok(apply_catalog_read_warning(page, catalog_warning));
        }
    }
    if let Some(cursor) = cursor
        .as_ref()
        .filter(|cursor| shadow_cache.matches(&legacy_sessions, cursor))
    {
        // A first-page audit is reusable while the bounded legacy catalog's
        // ID/workspace/order structure and SQLite snapshot stay unchanged.
        // Safe pages also match legacy workspace metadata; dirty pages are
        // checked against their audited identity projection and stay read-only.
        // The bridge verifies the cursor snapshot on every page. This avoids
        // repeating the physical inventory for continuations, while changes
        // made outside the supported Tauri writer path are picked up by an
        // explicit first-page refresh; opening a missing transcript fails closed.
        match supervisor.session_directory_page(limit, Some(cursor.clone()), None) {
            Ok(page)
                if shadow_cache.live_page_matches(&legacy_sessions, &page, cursor, limit)
                    && (!legacy_catalog_readable
                        || catalog.list().is_ok_and(|latest| latest == legacy_sessions)) =>
            {
                let identity_safe = shadow_cache
                    .identity_safe(&legacy_sessions, cursor)
                    .unwrap_or(false);
                let unclaimed_transcript_count = shadow_cache
                    .unclaimed_transcript_count(&legacy_sessions, cursor)
                    .filter(|count| *count > 0);
                let title_mismatch_count = shadow_cache
                    .title_mismatch_count(&legacy_sessions, cursor)
                    .filter(|count| *count > 0);
                let missing_transcript_count = shadow_cache
                    .missing_transcript_count(&legacy_sessions, cursor)
                    .filter(|count| *count > 0);
                let shadow_report = shadow_cache.shadow_report(&legacy_sessions, cursor);
                return Ok(apply_catalog_read_warning(
                    WorkbenchSessionPage {
                        sessions: page
                            .sessions
                            .into_iter()
                            .map(page_entry_from_identity)
                            .collect(),
                        next_cursor: page.next_cursor,
                        total: page.total,
                        source: if !identity_safe {
                            "identity_unverified"
                        } else if unclaimed_transcript_count.is_some()
                            || title_mismatch_count.is_some()
                            || missing_transcript_count.is_some()
                        {
                            "partial_identity"
                        } else {
                            "identity"
                        },
                        unverified_legacy_sessions: None,
                        shadow_directory_count: Some(page.total),
                        unclaimed_transcript_count,
                        title_mismatch_count,
                        missing_transcript_count,
                        shadow_report,
                        catalog_warning: None,
                    },
                    catalog_warning,
                ));
            }
            Err(_) if !supervisor.status().running => {
                if let Some(page) = shadow_cache.page(&legacy_sessions, Some(cursor), limit) {
                    return Ok(apply_catalog_read_warning(page, catalog_warning));
                }
                shadow_cache.clear();
            }
            Err(_) => {}
            Ok(_) => shadow_cache.clear(),
        }
    }
    let shadow = compare_session_catalog_with_directory(&supervisor, &legacy_sessions);
    if shadow.is_err() {
        if let Some(page) = shadow_cache.page(&legacy_sessions, cursor.as_ref(), limit) {
            return Ok(page);
        }
    }
    let cache_snapshot = match &shadow {
        Ok((report, directory, snapshot_id)) => Some((
            snapshot_id.clone(),
            directory.clone(),
            identity_directory_is_safe_to_page(report),
            if identity_directory_is_safe_to_page(report) {
                Vec::new()
            } else {
                legacy_sessions_missing_from_identity(&legacy_sessions, directory)
            },
            report.unclaimed_transcripts as u64,
            report.title_mismatches as u64,
            report.missing_transcripts as u64,
            report.clone(),
        )),
        _ => None,
    };
    let cache_legacy_sessions = legacy_sessions.clone();
    let page_cursor = cursor.clone();
    let page = workbench_session_page_from_shadow(legacy_sessions, cursor, limit, shadow, || {
        supervisor.session_directory_page(limit, page_cursor, None)
    });
    match &page {
        Ok(page)
            if matches!(
                page.source,
                "identity" | "partial_identity" | "identity_unverified"
            ) && page.next_cursor.is_some() =>
        {
            if let Some((
                snapshot_id,
                directory,
                identity_safe,
                unverified_legacy_sessions,
                unclaimed_transcript_count,
                title_mismatch_count,
                missing_transcript_count,
                shadow_report,
            )) = cache_snapshot
            {
                shadow_cache.remember(CachedSessionShadowSnapshot {
                    legacy_sessions: cache_legacy_sessions,
                    snapshot_id,
                    directory,
                    identity_safe,
                    unverified_legacy_sessions,
                    unclaimed_transcript_count,
                    title_mismatch_count,
                    missing_transcript_count,
                    shadow_report,
                });
            } else {
                shadow_cache.clear();
            }
        }
        _ => shadow_cache.clear(),
    }
    page.map(|page| apply_catalog_read_warning(page, catalog_warning))
}

fn session_page_matches_legacy_workspace(
    page: &SessionDirectoryPage,
    legacy_sessions: &[WorkbenchSession],
) -> bool {
    let legacy_by_id: std::collections::HashMap<_, _> = legacy_sessions
        .iter()
        .map(|session| (session.session_id.as_str(), session))
        .collect();
    page.sessions.iter().all(|entry| {
        legacy_by_id.get(entry.id.as_str()).is_none_or(|legacy| {
            legacy.workspace_root.as_deref().unwrap_or("")
                == entry.workspace_root.as_deref().unwrap_or("")
        })
    })
}

fn session_page_matches_legacy_workspace_entries(
    page: &[WorkbenchSessionPageEntry],
    legacy_sessions: &[WorkbenchSession],
) -> bool {
    let legacy_by_id: std::collections::HashMap<_, _> = legacy_sessions
        .iter()
        .map(|session| (session.session_id.as_str(), session))
        .collect();
    page.iter().all(|entry| {
        legacy_by_id
            .get(entry.session_id.as_str())
            .is_none_or(|legacy| {
                legacy.workspace_root.as_deref().unwrap_or("")
                    == entry.workspace_root.as_deref().unwrap_or("")
            })
    })
}

fn workbench_session_page_from_shadow(
    legacy_sessions: Vec<WorkbenchSession>,
    cursor: Option<SessionDirectoryCursor>,
    limit: u16,
    shadow: Result<(SessionShadowReport, Vec<SessionDirectoryEntry>, String), String>,
    load_identity_page: impl FnOnce() -> Result<SessionDirectoryPage, String>,
) -> Result<WorkbenchSessionPage, String> {
    if !(1..=200).contains(&limit) {
        return Err("session page limit is invalid".into());
    }
    let shadow = match shadow {
        Ok((report, directory, snapshot_id)) => Some((report, directory, snapshot_id)),
        Err(error) => {
            if let Some(cursor) = cursor.as_ref() {
                let Some(snapshot_id) = cursor.snapshot_id.as_deref() else {
                    return Err(error);
                };
                if cursor.total == 0 {
                    return Err(error);
                }
                let page = load_identity_page()?;
                if page.snapshot_id != snapshot_id || page.total != cursor.total {
                    return Err(
                        "session directory changed while paging; restart the session list".into(),
                    );
                }
                return Ok(workbench_page_from_unverified_identity(page));
            }
            return match load_identity_page() {
                Ok(page) => Ok(workbench_page_from_unverified_identity(page)),
                Err(_) => Ok(WorkbenchSessionPage {
                    total: legacy_sessions.len() as u64,
                    sessions: page_entries_from_legacy(legacy_sessions, None),
                    next_cursor: None,
                    source: "legacy",
                    unverified_legacy_sessions: None,
                    shadow_directory_count: None,
                    unclaimed_transcript_count: None,
                    title_mismatch_count: None,
                    missing_transcript_count: None,
                    shadow_report: None,
                    catalog_warning: None,
                }),
            };
        }
    };
    let Some((report, directory, snapshot_id)) = shadow else {
        unreachable!("shadow errors return a page or an error above");
    };
    if cursor
        .as_ref()
        .is_some_and(|cursor| cursor.snapshot_id.as_deref() != Some(snapshot_id.as_str()))
    {
        return Err("session catalog changed while paging; restart the session list".into());
    }
    let identity_safe = identity_directory_is_safe_to_page(&report);
    let page_result = load_identity_page();
    match page_result {
        Ok(page)
            if page.snapshot_id == snapshot_id
                && session_page_matches_shadow_snapshot(
                    &page,
                    &directory,
                    cursor.as_ref(),
                    limit,
                ) =>
        {
            let unclaimed_transcript_count =
                (report.unclaimed_transcripts > 0).then_some(report.unclaimed_transcripts as u64);
            let title_mismatch_count =
                (report.title_mismatches > 0).then_some(report.title_mismatches as u64);
            let missing_transcript_count =
                (report.missing_transcripts > 0).then_some(report.missing_transcripts as u64);
            let source = if !identity_safe {
                "identity_unverified"
            } else if unclaimed_transcript_count.is_some()
                || title_mismatch_count.is_some()
                || missing_transcript_count.is_some()
            {
                "partial_identity"
            } else {
                "identity"
            };
            let unverified_legacy_sessions = (!identity_safe && cursor.is_none())
                .then(|| legacy_sessions_missing_from_identity(&legacy_sessions, &directory));
            Ok(WorkbenchSessionPage {
                sessions: page
                    .sessions
                    .into_iter()
                    .map(page_entry_from_identity)
                    .collect(),
                next_cursor: page.next_cursor,
                total: page.total,
                source,
                unverified_legacy_sessions,
                shadow_directory_count: Some(page.total),
                unclaimed_transcript_count,
                title_mismatch_count,
                missing_transcript_count,
                shadow_report: Some(report),
                catalog_warning: None,
            })
        }
        Ok(_) | Err(_) if cursor.is_none() => Ok(WorkbenchSessionPage {
            total: legacy_sessions.len() as u64,
            sessions: page_entries_from_legacy(legacy_sessions, Some(&directory)),
            next_cursor: None,
            source: "legacy",
            unverified_legacy_sessions: None,
            shadow_directory_count: Some(directory.len() as u64),
            unclaimed_transcript_count: None,
            title_mismatch_count: None,
            missing_transcript_count: None,
            shadow_report: Some(report),
            catalog_warning: None,
        }),
        Ok(_) => Err("session directory changed while paging; restart the session list".into()),
        Err(error) => Err(error),
    }
}

// Structural snapshot IDs intentionally ignore title-only updates so they do
// not invalidate keyset cursors between requests. Within one guarded request,
// IDs, order, workspace, and lifecycle must match the audited slice; title is
// mutable display metadata and may advance independently.
fn session_page_matches_shadow_snapshot(
    page: &SessionDirectoryPage,
    directory: &[SessionDirectoryEntry],
    after: Option<&SessionDirectoryCursor>,
    limit: u16,
) -> bool {
    if page.total != directory.len() as u64 {
        return false;
    }
    let start = match after {
        None => 0,
        Some(cursor) => match directory.binary_search_by(|entry| {
            (entry.position, entry.id.as_str()).cmp(&(cursor.position, cursor.id.as_str()))
        }) {
            Ok(index) => index + 1,
            Err(_) => return false,
        },
    };
    let end = (start + usize::from(limit)).min(directory.len());
    if page.sessions.len() != end - start || page.next_cursor.is_some() != (end < directory.len()) {
        return false;
    }
    page.sessions
        .iter()
        .zip(&directory[start..end])
        .all(|(entry, audited)| {
            audited.id == entry.id
                && audited.position == entry.position
                && audited.workspace_root == entry.workspace_root
                && audited.state == entry.state
                && audited.missing == entry.missing
        })
}

#[tauri::command]
fn bridge_session_previews(
    supervisor: State<'_, BridgeSupervisor>,
    session_ids: Vec<String>,
) -> Result<Vec<SessionPreview>, String> {
    supervisor.session_previews(session_ids)
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

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct ScanImportCandidateList {
    candidates: Vec<ScanImportCandidate>,
    blocked_count: usize,
}

#[tauri::command]
fn scan_unclaimed_workbench_sessions(
    supervisor: State<'_, BridgeSupervisor>,
    profile: State<'_, PreviewProfile>,
    catalog: State<'_, WorkbenchCatalog>,
) -> Result<ScanImportCandidateList, String> {
    if !profile.status().managed_profile {
        return Err("审核导入只在隔离的 Preview profile 中开放".into());
    }
    let catalog_path = catalog.path().to_string_lossy().into_owned();
    let (candidates, blocked_count) = supervisor.scan_import_candidates(&catalog_path)?;
    Ok(ScanImportCandidateList {
        candidates,
        blocked_count,
    })
}

#[tauri::command]
fn import_unclaimed_workbench_sessions(
    supervisor: State<'_, BridgeSupervisor>,
    profile: State<'_, PreviewProfile>,
    catalog: State<'_, WorkbenchCatalog>,
    selected: Vec<ScanImportSelection>,
) -> Result<Vec<String>, String> {
    if !profile.status().managed_profile {
        return Err("审核导入只在隔离的 Preview profile 中开放".into());
    }
    if selected.is_empty()
        || selected
            .iter()
            .any(|item| item.title.is_none() || item.workspace_root.is_none())
    {
        return Err("每项都需要明确确认标题和项目归属".into());
    }
    let catalog_path = catalog.path().to_string_lossy().into_owned();
    supervisor.import_scan_sessions(&catalog_path, selected)
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
    let legacy_sessions = catalog.list()?;
    compare_session_catalog_with_directory(supervisor, &legacy_sessions)
        .map(|(report, _, _)| report)
}

fn compare_session_catalog_with_directory(
    supervisor: &BridgeSupervisor,
    legacy_sessions: &[WorkbenchSession],
) -> Result<(SessionShadowReport, Vec<SessionDirectoryEntry>, String), String> {
    let legacy_ids: Vec<_> = legacy_sessions
        .iter()
        .map(|session| session.session_id.clone())
        .collect();
    let (directory, snapshot_id, physical) = supervisor.session_shadow_snapshot(&legacy_ids)?;
    let report = session_shadow::compare(legacy_sessions, &directory, &physical)?;
    Ok((report, directory, snapshot_id))
}

#[cfg(test)]
fn identity_session_catalog_is_verified(report: Result<SessionShadowReport, String>) -> bool {
    report.is_ok_and(|report| report.legacy_matches_directory)
}

fn identity_directory_is_safe_to_page(report: &SessionShadowReport) -> bool {
    report.identity_directory_is_safe_to_page()
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
            app.manage(SessionShadowSnapshotCache::default());
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
            bridge_pending_session_deletes_page,
            bridge_pending_session_title_recoveries,
            list_mcp_servers,
            save_mcp_server,
            delete_mcp_server,
            bridge_session_snapshot,
            bridge_session_history,
            bridge_session_previews,
            bridge_session_catalog_shadow,
            bridge_import_legacy_session_catalog,
            scan_unclaimed_workbench_sessions,
            import_unclaimed_workbench_sessions,
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
            directory_count: usize::from(clean),
            matched_count: usize::from(clean),
            directory_only_count: 0,
            missing_from_directory: usize::from(!clean),
            retired_legacy_count: 0,
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

    fn legacy_session() -> WorkbenchSession {
        WorkbenchSession {
            session_id: "legacy-session".into(),
            title: Some("Legacy title".into()),
            workspace_root: None,
        }
    }

    fn identity_page() -> SessionDirectoryPage {
        SessionDirectoryPage {
            protocol_version: 1,
            sessions: vec![SessionDirectoryEntry {
                id: "identity-session".into(),
                title: "Identity title".into(),
                title_source: "manual".into(),
                workspace_root: None,
                state: "ready".into(),
                missing: false,
                position: 1,
                updated_at_ms: 0,
            }],
            next_cursor: None,
            total: 1,
            snapshot_id: snapshot_id(),
        }
    }

    fn identity_continuation() -> (
        SessionDirectoryPage,
        Vec<SessionDirectoryEntry>,
        SessionDirectoryCursor,
    ) {
        let mut page = identity_page();
        page.total = 2;
        let mut earlier = page.sessions[0].clone();
        earlier.id = "earlier-session".into();
        earlier.title = "Earlier title".into();
        earlier.position = 0;
        let mut directory = vec![earlier.clone()];
        directory.extend(page.sessions.clone());
        let cursor = SessionDirectoryCursor {
            position: earlier.position,
            id: earlier.id,
            snapshot_id: Some(snapshot_id()),
            total: 0,
        };
        (page, directory, cursor)
    }

    fn snapshot_id() -> String {
        "a".repeat(64)
    }

    #[test]
    fn first_page_uses_read_only_identity_page_when_shadow_is_unavailable() {
        let mut identity_query_called = false;
        let page = workbench_session_page_from_shadow(
            vec![legacy_session()],
            None,
            200,
            Err("inventory unavailable".into()),
            || {
                identity_query_called = true;
                Ok(identity_page())
            },
        )
        .expect("read-only identity page");
        assert_eq!(page.source, "identity_unverified");
        assert_eq!(page.sessions[0].session_id, "identity-session");
        assert!(identity_query_called);
        assert!(page.shadow_report.is_none());
        assert!(page.next_cursor.is_none());
    }

    #[test]
    fn dirty_shadow_shows_identity_and_separate_legacy_rows_read_only() {
        let interrupted = WorkbenchSession {
            session_id: "interrupted-delete".into(),
            title: Some("Deletion can be retried".into()),
            workspace_root: None,
        };
        let identity = identity_page();
        let directory = identity.sessions.clone();
        let page = workbench_session_page_from_shadow(
            vec![interrupted],
            None,
            200,
            Ok((report(false), directory, snapshot_id())),
            || Ok(identity),
        )
        .expect("dirty shadow remains visible for review");
        assert_eq!(page.source, "identity_unverified");
        assert_eq!(page.sessions.len(), 1);
        assert_eq!(page.sessions[0].session_id, "identity-session");
        let legacy = page
            .unverified_legacy_sessions
            .expect("separate legacy rows");
        assert_eq!(legacy.len(), 1);
        assert_eq!(legacy[0].session_id, "interrupted-delete");
        assert!(page.next_cursor.is_none());
    }

    #[test]
    fn dirty_shadow_continuation_uses_matching_identity_page_read_only() {
        let (identity_page, directory, cursor) = identity_continuation();
        let page = workbench_session_page_from_shadow(
            vec![legacy_session()],
            Some(cursor),
            200,
            Ok((report(false), directory, snapshot_id())),
            || Ok(identity_page),
        )
        .expect("matching dirty identity page");
        assert_eq!(page.source, "identity_unverified");
        assert_eq!(page.sessions[0].session_id, "identity-session");
    }

    #[test]
    fn continuation_page_fails_closed_when_shadow_and_cursor_are_unavailable() {
        let cursor = SessionDirectoryCursor {
            position: 1,
            id: "cursor-session".into(),
            snapshot_id: Some(snapshot_id()),
            total: 0,
        };
        let mut identity_query_called = false;
        let result = workbench_session_page_from_shadow(
            vec![legacy_session()],
            Some(cursor),
            200,
            Err("inventory unavailable".into()),
            || {
                identity_query_called = true;
                Ok(identity_page())
            },
        );
        assert!(result.is_err(), "unverified continuation was accepted");
        assert!(!identity_query_called, "identity query ran without a total");
    }

    #[test]
    fn clean_continuation_uses_the_identity_page() {
        let (identity_page, directory, cursor) = identity_continuation();
        let page = workbench_session_page_from_shadow(
            vec![legacy_session()],
            Some(cursor),
            200,
            Ok((report(true), directory, snapshot_id())),
            || Ok(identity_page),
        )
        .expect("verified identity page");
        assert_eq!(page.source, "identity");
        assert_eq!(page.sessions[0].session_id, "identity-session");
    }

    #[test]
    fn continuation_rejects_a_new_shadow_snapshot_before_loading_a_page() {
        let mut identity_query_called = false;
        let result = workbench_session_page_from_shadow(
            vec![legacy_session()],
            Some(SessionDirectoryCursor {
                position: 1,
                id: "cursor-session".into(),
                snapshot_id: Some(snapshot_id()),
                total: 0,
            }),
            200,
            Ok((report(true), Vec::new(), "b".repeat(64))),
            || {
                identity_query_called = true;
                Ok(identity_page())
            },
        );
        assert!(
            result.is_err(),
            "continuation accepted a different shadow snapshot"
        );
        assert!(
            !identity_query_called,
            "identity page loaded after the continuation snapshot changed"
        );
    }

    #[test]
    fn first_page_race_falls_back_when_page_differs_from_shadow_snapshot() {
        let page = workbench_session_page_from_shadow(
            vec![legacy_session()],
            None,
            200,
            Ok((report(true), Vec::new(), "b".repeat(64))),
            || Ok(identity_page()),
        )
        .expect("legacy fallback after first-page race");
        assert_eq!(page.source, "legacy");
        assert_eq!(page.sessions[0].session_id, "legacy-session");
    }

    #[test]
    fn title_change_between_shadow_and_page_preserves_structural_snapshot_id() {
        let audited = identity_page().sessions;
        let mut changed = identity_page();
        changed.sessions[0].title = "Title changed after audit".into();
        assert_eq!(changed.snapshot_id, snapshot_id());
        let legacy = WorkbenchSession {
            session_id: "identity-session".into(),
            title: Some("Identity title".into()),
            workspace_root: None,
        };

        let first = workbench_session_page_from_shadow(
            vec![legacy.clone()],
            None,
            200,
            Ok((report(true), audited.clone(), snapshot_id())),
            || Ok(changed.clone()),
        )
        .expect("title-only changes preserve structural pagination");
        assert_eq!(first.source, "identity");
        assert_eq!(
            first.sessions[0].title.as_deref(),
            Some("Title changed after audit")
        );

        let (mut continuation_page, continuation_directory, cursor) = identity_continuation();
        continuation_page.sessions[0].title = "Title changed after audit".into();
        let continuation = workbench_session_page_from_shadow(
            vec![legacy],
            Some(cursor),
            200,
            Ok((report(true), continuation_directory, snapshot_id())),
            || Ok(continuation_page),
        )
        .expect("title-only update does not invalidate a structural continuation snapshot");
        assert_eq!(continuation.source, "identity");
        assert_eq!(
            continuation.sessions[0].title.as_deref(),
            Some("Title changed after audit")
        );
    }

    #[test]
    fn page_must_be_the_exact_slice_of_the_audited_directory() {
        let (mut page, mut directory, cursor) = identity_continuation();
        assert!(session_page_matches_shadow_snapshot(
            &page,
            &directory,
            Some(&cursor),
            200
        ));

        page.sessions.clear();
        assert!(!session_page_matches_shadow_snapshot(
            &page,
            &directory,
            Some(&cursor),
            200
        ));

        let (mut page, _, _) = identity_continuation();
        page.sessions[0].title = "Updated before this page request".into();
        directory[1].title = page.sessions[0].title.clone();
        assert!(session_page_matches_shadow_snapshot(
            &page,
            &directory,
            Some(&cursor),
            200
        ));
    }

    #[test]
    fn identity_page_failure_falls_back_only_for_the_first_page() {
        let first_page = workbench_session_page_from_shadow(
            vec![legacy_session()],
            None,
            200,
            Ok((report(true), Vec::new(), snapshot_id())),
            || Err("identity store unavailable".into()),
        )
        .expect("first page legacy fallback");
        assert_eq!(first_page.source, "legacy");
        assert_eq!(first_page.sessions[0].session_id, "legacy-session");

        let continuation = workbench_session_page_from_shadow(
            vec![legacy_session()],
            Some(SessionDirectoryCursor {
                position: 1,
                id: "cursor-session".into(),
                snapshot_id: Some(snapshot_id()),
                total: 0,
            }),
            200,
            Ok((report(true), Vec::new(), snapshot_id())),
            || Err("identity store unavailable".into()),
        );
        assert!(
            matches!(continuation, Err(error) if error == "identity store unavailable"),
            "continuation failures must not splice legacy rows into an identity page"
        );
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
        let home = tempfile::tempdir().expect("isolated workspace parent");
        let file = home.path().join("regular-file");
        std::fs::write(&file, b"not a directory").expect("write regular file");
        assert_eq!(
            workspace_root_is_available(&file.join("child").to_string_lossy()),
            Some(false)
        );
    }
}
