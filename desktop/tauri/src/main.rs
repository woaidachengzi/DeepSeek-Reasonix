#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod bridge;
mod credential_namespace;
mod data_profile;
mod host_preferences;
#[cfg(target_os = "macos")]
mod initial_presentation;
mod keychain;
#[cfg(target_os = "macos")]
mod legacy_ui_fixture;
#[cfg(target_os = "macos")]
mod legacy_ui_preferences;
mod local_paths;
mod menu;
#[cfg(target_os = "macos")]
mod native_appearance;
#[cfg(target_os = "macos")]
mod native_appearance_smoke;
#[cfg(target_os = "macos")]
mod native_clipboard_smoke;
#[cfg(target_os = "macos")]
mod native_dialog_smoke;
#[cfg(target_os = "macos")]
mod native_edit_smoke;
#[cfg(target_os = "macos")]
mod native_link_smoke;
#[cfg(target_os = "macos")]
mod native_menu_smoke;
#[cfg(target_os = "macos")]
mod native_profile_smoke;
#[cfg(target_os = "macos")]
mod native_reload_smoke;
#[cfg(target_os = "macos")]
mod native_remote_image_smoke;
#[cfg(target_os = "macos")]
mod native_task_smoke;
#[cfg(target_os = "macos")]
mod native_ui_image_smoke;
#[cfg(target_os = "macos")]
mod native_ui_message_copy_smoke;
#[cfg(target_os = "macos")]
mod native_ui_storage_smoke;
#[cfg(target_os = "macos")]
mod native_ui_terminal_smoke;
#[cfg(target_os = "macos")]
mod native_window_smoke;
mod notifications;
mod opener_catalog;
mod pasted_images;
mod protocol_generated;
mod remote_controller;
mod runtime_info;
mod session_shadow;
mod terminal;
#[cfg(test)]
mod theme_asset_scope_tests;
mod tray;
#[cfg(target_os = "macos")]
mod ui_origin;
mod window_state;
mod workbench_catalog;
mod workbench_projects;

use tauri::Emitter;

#[cfg(test)]
pub(crate) mod test_env {
    use std::sync::{Mutex, MutexGuard};

    static PROCESS_ENV_LOCK: Mutex<()> = Mutex::new(());

    pub(crate) struct Guard {
        _lock: MutexGuard<'static, ()>,
        reasonix_home: Option<std::ffi::OsString>,
        reasonix_state_home: Option<std::ffi::OsString>,
        reasonix_cache_home: Option<std::ffi::OsString>,
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
            reasonix_cache_home: std::env::var_os("REASONIX_CACHE_HOME"),
            preview_sqlite_events: std::env::var_os("REASONIX_PREVIEW_SQLITE_EVENTS"),
        }
    }

    impl Drop for Guard {
        fn drop(&mut self) {
            restore("REASONIX_HOME", self.reasonix_home.take());
            restore("REASONIX_STATE_HOME", self.reasonix_state_home.take());
            restore("REASONIX_CACHE_HOME", self.reasonix_cache_home.take());
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

use base64::{engine::general_purpose::STANDARD as BASE64_STANDARD, Engine as _};
use serde::{Deserialize, Serialize};
use std::{
    collections::{HashMap, HashSet},
    fs,
    fs::File,
    io::{Read, Write},
    path::{Path, PathBuf},
    sync::{
        atomic::{AtomicU64, Ordering},
        Mutex,
    },
};
use tauri_plugin_dialog::DialogExt;
use tauri_plugin_opener::OpenerExt;
use zip::{write::SimpleFileOptions, CompressionMethod, ZipArchive, ZipWriter};

use bridge::{
    AnswerMCPInteractionRequest, AnswerQuestionRequest, ApproveRequest, AttachFileRequest,
    BridgeAttachment, BridgeCodeRewindPlanResponse, BridgeCombinedRewindPlanResponse,
    BridgeCombinedRewindResultResponse, BridgeConversationRewindPlanResponse,
    BridgeConversationRewindResultResponse, BridgeDeleteSessionResponse, BridgeHistory,
    BridgeLegacyConversationForkResultResponse, BridgeProjectFolder, BridgeProviderSummaryResponse,
    BridgeRemoteBrowseRequest, BridgeRemoteBrowseResponse, BridgeRemoteDisconnectRequest,
    BridgeRemoteDisconnectResponse, BridgeRemoteFilePreviewRequest,
    BridgeRemoteFilePreviewResponse, BridgeRemoteFileSaveRequest, BridgeRemoteFileSaveResponse,
    BridgeSession, BridgeSessionBalanceResponse, BridgeSessionHeadSwitchResponse,
    BridgeSessionHeadsResponse, BridgeSetAgentPreferenceRequest, BridgeSetDefaultModelRequest,
    BridgeSetModelRoleRequest, BridgeSetSessionModelRequest, BridgeSnapshot, BridgeStatus,
    BridgeSupervisor, BridgeWorkspaceChangeDetailResponse, BridgeWorkspaceChangesResponse,
    BridgeWorkspaceCheckpointsResponse, BridgeWorkspaceFileResponse,
    BridgeWorkspaceFileRevertPlanResponse, BridgeWorkspaceFileRevertResultResponse,
    BridgeWorkspaceImageResponse, BridgeWorkspaceListResponse, CodeRewindCommitRequest,
    CodeRewindPreviewRequest, CombinedRewindCommitRequest, CombinedRewindPreviewRequest,
    ConversationRewindCommitRequest, ConversationRewindPreviewRequest,
    ConversationRewindUndoRequest, DeleteProviderConfigRequest, DesktopPreferences,
    DiscoverProviderModelsRequest, DiscoveredProviderModels, HooksSettingsChange,
    HooksSettingsView, LegacySessionCatalogEntry, MCPClearAuthRequest, MCPClearAuthResponse,
    MCPMarketplaceEntry, MCPMarketplaceResponse, MCPOAuthRequest, MCPOAuthResponse,
    MCPRuntimeActionRequest, MCPRuntimeActionResponse, MCPServerActivationRequest,
    MCPServerDeleteRequest, MCPServerInput, MCPServerMutationResponse, MCPServerView,
    MemorySettingsChange, MemorySettingsView, MemorySuggestionAcceptance,
    MemorySuggestionAcceptanceRequest, MemorySuggestionsView, NetworkSettingsChange,
    NetworkSettingsView, OpenSessionRequest, PendingSessionDeleteCursor, PendingSessionDeletePage,
    PendingSessionTitleRecovery, PermissionSettingsChange, PermissionSettingsView,
    PluginDoctorView, PluginInstallPlan, PluginInstallRequest, PluginOperationResult,
    PluginRemoveRequest, PluginSettingsChange, PluginSettingsView, ProviderConfigList,
    RemoteConnectRequest, RemoteConnectResponse, RemoteSSHConfigScanView, RemoteSettingsChange,
    RemoteSettingsView, RenameSessionRequest, SandboxSettingsChange, SandboxSettingsView,
    SaveProviderConfigRequest, ScanImportCandidate, ScanImportSelection, SecretsSettingsChange,
    SecretsSettingsView, SessionCatalogMetadata, SessionDirectoryCursor, SessionDirectoryEntry,
    SessionDirectoryPage, SessionFirstMessageTitle, SessionHeadSwitchRequest, SessionPreview,
    SessionRequest, SkillArchiveRequest, SkillArchiveResult, SkillInstallPlan, SkillInstallRequest,
    SkillInstallResult, SkillsSettingsChange, SkillsSettingsView, SubagentProfileInput,
    SubagentSettingsChange, SubagentSettingsView, SubagentTryStatusView, SubmitRequest,
    WorkspaceChangeDetailRequest, WorkspaceFileRequest, WorkspaceFileRevertCommitRequest,
    WorkspaceFileRevertUndoRequest, WorkspaceImageRequest, WorkspaceRequest,
};
use data_profile::{
    PreviewProfile, PreviewProfileStatus, ProfileImportResult, ProjectFoldersImportResult,
};
use host_preferences::{CloseBehavior, HostPreferences, UserTheme};
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
struct WorkbenchTitleBackfillResult {
    sessions: Vec<WorkbenchSession>,
    resolved_titles: Vec<SessionFirstMessageTitle>,
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
async fn save_provider_api_key(
    window: tauri::WebviewWindow,
    provider_name: String,
    api_key: String,
) -> Result<(), String> {
    if window.label() != "main" {
        return Err("credential operations require the main window".into());
    }
    tauri::async_runtime::spawn_blocking(move || {
        window
            .state::<BridgeSupervisor>()
            .persist_provider_api_key(&provider_name, Some(&api_key))
            .map(|_| ())
    })
    .await
    .map_err(|_| "credential operation failed; retry saving")?
}

#[tauri::command]
async fn clear_provider_api_key(
    window: tauri::WebviewWindow,
    provider_name: String,
) -> Result<bool, String> {
    if window.label() != "main" {
        return Err("credential operations require the main window".into());
    }
    tauri::async_runtime::spawn_blocking(move || {
        window
            .state::<BridgeSupervisor>()
            .persist_provider_api_key(&provider_name, None)
            .map(|_| true)
    })
    .await
    .map_err(|_| "credential operation failed; retry clearing")?
}

#[tauri::command]
fn restart_bridge(supervisor: State<'_, BridgeSupervisor>) -> Result<BridgeStatus, String> {
    supervisor.restart()
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
fn bridge_session_approval_mode(
    supervisor: State<'_, BridgeSupervisor>,
    session_id: String,
    mode: Option<String>,
) -> Result<String, String> {
    supervisor.session_approval_mode(&session_id, mode)
}

#[tauri::command]
fn bridge_set_session_model(
    supervisor: State<'_, BridgeSupervisor>,
    session_id: String,
    request: BridgeSetSessionModelRequest,
) -> Result<BridgeSession, String> {
    supervisor.set_session_model(&session_id, request)
}

#[tauri::command]
fn bridge_rename_session(
    supervisor: State<'_, BridgeSupervisor>,
    request: RenameSessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.rename_session(request)
}

#[tauri::command]
fn bridge_session_archives(
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<Vec<bridge::ArchivedSession>, String> {
    supervisor.session_archives(None)
}

#[tauri::command]
fn bridge_change_session_archive(
    supervisor: State<'_, BridgeSupervisor>,
    session_id: String,
    archived: bool,
) -> Result<Vec<bridge::ArchivedSession>, String> {
    supervisor.session_archives(Some((session_id, archived)))
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
fn mcp_runtime_action(
    supervisor: State<'_, BridgeSupervisor>,
    request: MCPRuntimeActionRequest,
) -> Result<MCPRuntimeActionResponse, String> {
    supervisor.mcp_runtime_action(request)
}

#[tauri::command]
fn clear_mcp_authentication(
    supervisor: State<'_, BridgeSupervisor>,
    request: MCPClearAuthRequest,
) -> Result<MCPClearAuthResponse, String> {
    supervisor.clear_mcp_authentication(request)
}

#[tauri::command]
fn start_mcp_oauth(
    supervisor: State<'_, BridgeSupervisor>,
    request: MCPOAuthRequest,
) -> Result<MCPOAuthResponse, String> {
    supervisor.start_mcp_oauth(request)
}

#[tauri::command]
fn mcp_oauth_status(
    supervisor: State<'_, BridgeSupervisor>,
    request: MCPOAuthRequest,
) -> Result<MCPOAuthResponse, String> {
    supervisor.mcp_oauth_status(request)
}

#[tauri::command]
fn cancel_mcp_oauth(
    supervisor: State<'_, BridgeSupervisor>,
    request: MCPOAuthRequest,
) -> Result<MCPOAuthResponse, String> {
    supervisor.cancel_mcp_oauth(request)
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
fn set_mcp_server_enabled(
    supervisor: State<'_, BridgeSupervisor>,
    request: MCPServerActivationRequest,
    workspace_root: Option<String>,
) -> Result<MCPServerMutationResponse, String> {
    supervisor.set_mcp_server_enabled(request, workspace_root.as_deref())
}

#[tauri::command]
fn search_mcp_marketplace(
    supervisor: State<'_, BridgeSupervisor>,
    query: String,
) -> Result<MCPMarketplaceResponse, String> {
    supervisor.search_mcp_marketplace(&query)
}

#[tauri::command]
fn resolve_mcp_marketplace(
    supervisor: State<'_, BridgeSupervisor>,
    name: String,
) -> Result<MCPMarketplaceEntry, String> {
    supervisor.resolve_mcp_marketplace(&name)
}

#[tauri::command]
fn bridge_session_snapshot(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeSnapshot, String> {
    supervisor.snapshot(request)
}

#[tauri::command]
fn bridge_session_balance(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeSessionBalanceResponse, String> {
    supervisor.session_balance(request)
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
async fn stage_pasted_image(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    data_url: Option<String>,
) -> Result<Option<pasted_images::StagedImage>, String> {
    if window.label() != "main" {
        return Err("image paste is only available in the main window".into());
    }
    // Clipboard reads can deadlock Linux on the UI thread. Both decoding and
    // file IO are confined to a worker and only invoked by an explicit Paste.
    tauri::async_runtime::spawn_blocking(move || {
        let (extension, bytes) = if let Some(value) = data_url {
            pasted_images::decode_data_url(&value)?
        } else {
            use tauri_plugin_clipboard_manager::ClipboardExt;
            let image = match app.clipboard().read_image() {
                Ok(image) => image,
                Err(error)
                    if error
                        .to_string()
                        .contains("not available in the requested format") =>
                {
                    return Ok(None)
                }
                Err(_) => {
                    return Err(
                        "unable to read clipboard image; copy the image again or use Add file"
                            .into(),
                    )
                }
            };
            (
                "png".into(),
                pasted_images::encode_rgba(image.width(), image.height(), image.rgba())?,
            )
        };
        app.state::<pasted_images::PastedImages>()
            .stage(&extension, &bytes)
            .map(Some)
    })
    .await
    .map_err(|_| "image paste worker stopped")?
}

#[tauri::command]
fn discard_pasted_image(
    window: tauri::WebviewWindow,
    images: State<'_, pasted_images::PastedImages>,
    token: String,
) -> Result<(), String> {
    if window.label() != "main" {
        return Err("image paste is only available in the main window".into());
    }
    images.discard(&token)
}

#[tauri::command]
async fn bridge_remote_controller_attach(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: remote_controller::AttachRequest,
) -> Result<protocol_generated::BridgeRemoteControllerResponse, String> {
    remote_controller::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .remote_controller_client()?
            .attach(request)
    })
    .await
    .map_err(|_| "remote controller worker stopped; reopen the remote workspace".to_string())?
}

#[tauri::command]
async fn bridge_remote_controller_sessions(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: remote_controller::HandleRequest,
) -> Result<protocol_generated::BridgeRemoteControllerSessionsResponse, String> {
    remote_controller::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .remote_controller_client()?
            .sessions(request)
    })
    .await
    .map_err(|_| "remote controller worker stopped; reopen the remote workspace".to_string())?
}

#[tauri::command]
async fn bridge_remote_controller_close(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: remote_controller::HandleRequest,
) -> Result<protocol_generated::BridgeRemoteControllerCloseResponse, String> {
    remote_controller::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .remote_controller_client()?
            .close(request)
    })
    .await
    .map_err(|_| "remote controller worker stopped; reopen the remote workspace".to_string())?
}

#[tauri::command]
async fn bridge_remote_controller_session_view(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: remote_controller::SessionViewRequest,
) -> Result<protocol_generated::BridgeRemoteControllerSessionViewResponse, String> {
    remote_controller::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .remote_controller_client()?
            .session_view(request)
    })
    .await
    .map_err(|_| "remote controller worker stopped; reopen the remote workspace".to_string())?
}

#[tauri::command]
async fn bridge_remote_controller_session_image(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: remote_controller::SessionImageRequest,
) -> Result<protocol_generated::BridgeRemoteControllerSessionImageResponse, String> {
    remote_controller::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .remote_controller_client()?
            .session_image(request)
    })
    .await
    .map_err(|_| "remote image worker stopped; reopen the remote workspace".to_string())?
}

#[tauri::command]
fn bridge_remote_controller_subscribe(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: remote_controller::SubscribeRequest,
) -> Result<remote_controller::SubscriptionIdentity, String> {
    remote_controller::ensure_main_window(window.label())?;
    let (client, operation) = app
        .state::<BridgeSupervisor>()
        .reserve_remote_subscription(window.label(), request)?;
    let identity = operation.identity();
    std::thread::Builder::new()
        .name("remote-session-events".into())
        .spawn(move || remote_controller::run_subscription(app, client, operation))
        .map_err(|_| {
            "remote subscription worker could not start; reopen the session".to_string()
        })?;
    // Receipt is not stream readiness or send/resume authority. The listener
    // must bind surface+generation before invoke; wait for the ready notice.
    Ok(identity)
}

#[tauri::command]
fn bridge_remote_controller_unsubscribe(
    window: tauri::WebviewWindow,
    supervisor: State<'_, BridgeSupervisor>,
    request: remote_controller::UnsubscribeRequest,
) -> Result<(), String> {
    supervisor.close_remote_subscription(window.label(), &request.subscription_id)
}

#[tauri::command]
async fn bridge_terminal_workspace(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: terminal::WorkspaceRequest,
) -> Result<protocol_generated::BridgeTerminalWorkspaceResponse, String> {
    terminal::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .terminal_client()?
            .workspace(request)
    })
    .await
    .map_err(|_| "terminal worker stopped; refresh the active session".to_string())?
}

#[tauri::command]
async fn bridge_terminal_create(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: terminal::CreateRequest,
) -> Result<protocol_generated::BridgeTerminalSessionResponse, String> {
    terminal::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .terminal_client()?
            .create(request)
    })
    .await
    .map_err(|_| "terminal worker stopped; refresh the active session".to_string())?
}

#[tauri::command]
async fn bridge_terminal_output(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: terminal::TargetRequest,
) -> Result<protocol_generated::BridgeTerminalOutputResponse, String> {
    terminal::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .terminal_client()?
            .output(request)
    })
    .await
    .map_err(|_| "terminal worker stopped; refresh the active session".to_string())?
}

#[tauri::command]
async fn bridge_terminal_input(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: terminal::InputRequest,
) -> Result<protocol_generated::BridgeTerminalActionResponse, String> {
    terminal::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .terminal_client()?
            .input(request)
    })
    .await
    .map_err(|_| "terminal worker stopped; refresh the active session".to_string())?
}

#[tauri::command]
async fn bridge_terminal_resize(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: terminal::ResizeRequest,
) -> Result<protocol_generated::BridgeTerminalActionResponse, String> {
    terminal::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .terminal_client()?
            .resize(request)
    })
    .await
    .map_err(|_| "terminal worker stopped; refresh the active session".to_string())?
}

#[tauri::command]
async fn bridge_terminal_rename(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: terminal::RenameRequest,
) -> Result<protocol_generated::BridgeTerminalActionResponse, String> {
    terminal::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .terminal_client()?
            .rename(request)
    })
    .await
    .map_err(|_| "terminal worker stopped; refresh the active session".to_string())?
}

#[tauri::command]
async fn bridge_terminal_close(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: terminal::CloseRequest,
) -> Result<protocol_generated::BridgeTerminalActionResponse, String> {
    terminal::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>()
            .terminal_client()?
            .close(request)
    })
    .await
    .map_err(|_| "terminal worker stopped; refresh the active session".to_string())?
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
async fn bridge_workspace_image(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    request: WorkspaceImageRequest,
) -> Result<BridgeWorkspaceImageResponse, String> {
    if window.label() != "main" {
        return Err("workspace images are only available in the main window".into());
    }
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<BridgeSupervisor>().workspace_image(request)
    })
    .await
    .map_err(|_| "workspace image worker failed".to_string())?
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

#[tauri::command]
fn bridge_workspace_file_revert_preview(
    supervisor: State<'_, BridgeSupervisor>,
    request: WorkspaceChangeDetailRequest,
) -> Result<BridgeWorkspaceFileRevertPlanResponse, String> {
    supervisor.workspace_file_revert_preview(request)
}

#[tauri::command]
fn bridge_workspace_file_revert_commit(
    supervisor: State<'_, BridgeSupervisor>,
    request: WorkspaceFileRevertCommitRequest,
) -> Result<BridgeWorkspaceFileRevertResultResponse, String> {
    supervisor.workspace_file_revert_commit(request)
}

#[tauri::command]
fn bridge_workspace_file_revert_undo(
    supervisor: State<'_, BridgeSupervisor>,
    request: WorkspaceFileRevertUndoRequest,
) -> Result<BridgeWorkspaceFileRevertResultResponse, String> {
    supervisor.workspace_file_revert_undo(request)
}

#[tauri::command]
fn bridge_workspace_checkpoints(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeWorkspaceCheckpointsResponse, String> {
    supervisor.workspace_checkpoints(request)
}

#[tauri::command]
fn bridge_code_rewind_preview(
    supervisor: State<'_, BridgeSupervisor>,
    request: CodeRewindPreviewRequest,
) -> Result<BridgeCodeRewindPlanResponse, String> {
    supervisor.code_rewind_preview(request)
}

#[tauri::command]
fn bridge_code_rewind_commit(
    supervisor: State<'_, BridgeSupervisor>,
    request: CodeRewindCommitRequest,
) -> Result<BridgeWorkspaceFileRevertResultResponse, String> {
    supervisor.code_rewind_commit(request)
}

#[tauri::command]
fn bridge_conversation_rewind_preview(
    supervisor: State<'_, BridgeSupervisor>,
    request: ConversationRewindPreviewRequest,
) -> Result<BridgeConversationRewindPlanResponse, String> {
    supervisor.conversation_rewind_preview(request)
}

#[tauri::command]
fn bridge_conversation_rewind_commit(
    supervisor: State<'_, BridgeSupervisor>,
    request: ConversationRewindCommitRequest,
) -> Result<BridgeConversationRewindResultResponse, String> {
    supervisor.conversation_rewind_commit(request)
}

#[tauri::command]
fn bridge_conversation_rewind_undo(
    supervisor: State<'_, BridgeSupervisor>,
    request: ConversationRewindUndoRequest,
) -> Result<BridgeConversationRewindResultResponse, String> {
    supervisor.conversation_rewind_undo(request)
}

#[tauri::command]
fn bridge_session_heads(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeSessionHeadsResponse, String> {
    supervisor.session_heads(request)
}

#[tauri::command]
fn bridge_session_head_switch(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionHeadSwitchRequest,
) -> Result<BridgeSessionHeadSwitchResponse, String> {
    supervisor.session_head_switch(request)
}

#[tauri::command]
fn bridge_combined_rewind_preview(
    supervisor: State<'_, BridgeSupervisor>,
    request: CombinedRewindPreviewRequest,
) -> Result<BridgeCombinedRewindPlanResponse, String> {
    supervisor.combined_rewind_preview(request)
}

#[tauri::command]
fn bridge_combined_rewind_commit(
    supervisor: State<'_, BridgeSupervisor>,
    request: CombinedRewindCommitRequest,
) -> Result<BridgeCombinedRewindResultResponse, String> {
    supervisor.combined_rewind_commit(request)
}

#[tauri::command]
fn bridge_legacy_fork_preview(
    supervisor: State<'_, BridgeSupervisor>,
    request: ConversationRewindPreviewRequest,
) -> Result<BridgeConversationRewindPlanResponse, String> {
    supervisor.legacy_fork_preview(request)
}

#[tauri::command]
fn bridge_legacy_fork_commit(
    supervisor: State<'_, BridgeSupervisor>,
    request: ConversationRewindCommitRequest,
) -> Result<BridgeLegacyConversationForkResultResponse, String> {
    supervisor.legacy_fork_commit(request)
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
async fn legacy_ui_preferences(app: tauri::AppHandle) -> Result<serde_json::Value, String> {
    #[cfg(target_os = "macos")]
    {
        tauri::async_runtime::spawn_blocking(move || {
            legacy_ui_preferences::read(&app).and_then(|snapshot| {
                serde_json::to_value(snapshot)
                    .map_err(|_| "Could not prepare UI preference preview; retry".into())
            })
        })
        .await
        .map_err(|_| "UI preference preview stopped; retry".to_string())?
    }
    #[cfg(not(target_os = "macos"))]
    {
        let _ = app;
        Err("Old UI preference preview is available only on macOS".into())
    }
}

#[tauri::command]
fn preview_runtime_info(supervisor: State<'_, BridgeSupervisor>) -> PreviewRuntimeInfo {
    PreviewRuntimeInfo::from_status(supervisor.status())
}

#[tauri::command]
fn provider_summary(
    supervisor: State<'_, BridgeSupervisor>,
    scope: Option<String>,
    workspace_root: Option<String>,
) -> Result<BridgeProviderSummaryResponse, String> {
    match scope.as_deref().unwrap_or("global") {
        "global" => supervisor.provider_summary(),
        scope => supervisor.provider_summary_for_scope(scope, workspace_root.as_deref()),
    }
}

#[tauri::command]
fn provider_configs(supervisor: State<'_, BridgeSupervisor>) -> Result<ProviderConfigList, String> {
    supervisor.provider_configs()
}

#[tauri::command]
fn save_provider_config(
    supervisor: State<'_, BridgeSupervisor>,
    input: SaveProviderConfigRequest,
) -> Result<ProviderConfigList, String> {
    supervisor.save_provider_config(input)
}

#[tauri::command]
fn delete_provider_config(
    supervisor: State<'_, BridgeSupervisor>,
    input: DeleteProviderConfigRequest,
) -> Result<ProviderConfigList, String> {
    supervisor.delete_provider_config(input)
}

#[tauri::command]
fn preview_provider_reasoning(
    supervisor: State<'_, BridgeSupervisor>,
    input: SaveProviderConfigRequest,
) -> Result<Vec<bridge::ProviderModelReasoning>, String> {
    supervisor.preview_provider_reasoning(input)
}

#[tauri::command]
fn discover_provider_models(
    supervisor: State<'_, BridgeSupervisor>,
    input: DiscoverProviderModelsRequest,
) -> Result<DiscoveredProviderModels, String> {
    supervisor.discover_provider_models(input)
}

#[tauri::command]
fn usage_stats(
    supervisor: State<'_, BridgeSupervisor>,
    request: serde_json::Value,
) -> Result<serde_json::Value, String> {
    supervisor.usage_stats(request)
}

#[tauri::command]
fn storage_settings(supervisor: State<'_, BridgeSupervisor>) -> Result<serde_json::Value, String> {
    supervisor.storage_settings()
}

#[tauri::command]
fn capability_diagnostics(
    supervisor: State<'_, BridgeSupervisor>,
    workspace_root: String,
    include_session_runtime: bool,
) -> Result<serde_json::Value, String> {
    supervisor.capability_diagnostics(&workspace_root, include_session_runtime)
}

#[tauri::command]
fn runtime_doctor(supervisor: State<'_, BridgeSupervisor>) -> Result<serde_json::Value, String> {
    supervisor.runtime_doctor()
}

#[tauri::command]
fn remote_settings(supervisor: State<'_, BridgeSupervisor>) -> Result<RemoteSettingsView, String> {
    supervisor.remote_settings()
}

#[tauri::command]
fn scan_remote_ssh_config(
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<RemoteSSHConfigScanView, String> {
    supervisor.scan_remote_ssh_config()
}

#[tauri::command]
fn change_remote_settings(
    supervisor: State<'_, BridgeSupervisor>,
    change: RemoteSettingsChange,
) -> Result<RemoteSettingsView, String> {
    supervisor.change_remote_settings(change)
}

#[tauri::command]
async fn connect_remote_host(
    supervisor: State<'_, BridgeSupervisor>,
    request: RemoteConnectRequest,
) -> Result<RemoteConnectResponse, String> {
    let client = supervisor.management_client()?;
    tauri::async_runtime::spawn_blocking(move || {
        client.remote_request::<RemoteConnectResponse>(
            "/v1/settings/remote/connect",
            serde_json::json!(request),
        )
    })
    .await
    .map_err(|_| "remote operation worker failed".to_string())?
}

#[tauri::command]
async fn disconnect_remote_host(
    supervisor: State<'_, BridgeSupervisor>,
    request: BridgeRemoteDisconnectRequest,
) -> Result<BridgeRemoteDisconnectResponse, String> {
    let client = supervisor.management_client()?;
    tauri::async_runtime::spawn_blocking(move || {
        client.remote_request::<BridgeRemoteDisconnectResponse>(
            "/v1/settings/remote/disconnect",
            serde_json::json!(request),
        )
    })
    .await
    .map_err(|_| "remote operation worker failed".to_string())?
}

#[tauri::command]
async fn remote_forwards(
    supervisor: State<'_, BridgeSupervisor>,
    input: serde_json::Value,
) -> Result<serde_json::Value, String> {
    let client = supervisor.management_client()?;
    tauri::async_runtime::spawn_blocking(move || {
        client.remote_request("/v1/settings/remote/forwards", input)
    })
    .await
    .map_err(|error| error.to_string())?
}

#[tauri::command]
async fn remote_serve(
    supervisor: State<'_, BridgeSupervisor>,
    input: serde_json::Value,
) -> Result<serde_json::Value, String> {
    let (path, input) = remote_serve_request(input)?;
    let client = supervisor.management_client()?;
    tauri::async_runtime::spawn_blocking(move || client.remote_request(path, input))
        .await
        .map_err(|_| "remote operation worker failed".to_string())?
}

fn remote_serve_request(
    mut input: serde_json::Value,
) -> Result<(&'static str, serde_json::Value), String> {
    let action = input
        .get("action")
        .and_then(serde_json::Value::as_str)
        .ok_or_else(|| "remote Serve action is required".to_string())?;
    let path = match action {
        "status" => "/v1/settings/remote/serve/status",
        "start" => "/v1/settings/remote/serve/start",
        "stop" => "/v1/settings/remote/serve/stop",
        "logs" => "/v1/settings/remote/serve/logs",
        _ => return Err("remote Serve action is invalid".to_string()),
    };
    input.as_object_mut().unwrap().remove("action");
    Ok((path, input))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct RemoteControllerLaunch {
    protocol_version: u64,
    url: String,
}

#[tauri::command]
async fn open_remote_controller(
    app: tauri::AppHandle,
    supervisor: State<'_, BridgeSupervisor>,
    input: serde_json::Value,
) -> Result<(), String> {
    let client = supervisor.management_client()?;
    let launch = tauri::async_runtime::spawn_blocking(move || {
        client
            .remote_request::<RemoteControllerLaunch>("/v1/settings/remote/controller/open", input)
    })
    .await
    .map_err(|_| "remote operation worker failed".to_string())??;
    if launch.protocol_version != 1 {
        return Err("desktop bridge protocol version is unsupported".to_string());
    }
    app.opener()
        .open_url(validated_remote_controller_url(&launch.url)?, None::<&str>)
        .map_err(|_| "could not open remote controller in the system browser".to_string())
}

#[tauri::command]
async fn browse_remote_host(
    supervisor: State<'_, BridgeSupervisor>,
    request: BridgeRemoteBrowseRequest,
) -> Result<BridgeRemoteBrowseResponse, String> {
    let client = supervisor.management_client()?;
    tauri::async_runtime::spawn_blocking(move || {
        client.remote_request::<BridgeRemoteBrowseResponse>(
            "/v1/settings/remote/browse",
            serde_json::json!(request),
        )
    })
    .await
    .map_err(|_| "remote operation worker failed".to_string())?
}

#[tauri::command]
async fn preview_remote_file(
    supervisor: State<'_, BridgeSupervisor>,
    request: BridgeRemoteFilePreviewRequest,
) -> Result<BridgeRemoteFilePreviewResponse, String> {
    let client = supervisor.management_client()?;
    tauri::async_runtime::spawn_blocking(move || {
        client.remote_request::<BridgeRemoteFilePreviewResponse>(
            "/v1/settings/remote/preview",
            serde_json::json!(request),
        )
    })
    .await
    .map_err(|_| "remote operation worker failed".to_string())?
}

#[tauri::command]
async fn save_remote_file(
    supervisor: State<'_, BridgeSupervisor>,
    request: BridgeRemoteFileSaveRequest,
) -> Result<BridgeRemoteFileSaveResponse, String> {
    let client = supervisor.management_client()?;
    tauri::async_runtime::spawn_blocking(move || {
        client.remote_request::<BridgeRemoteFileSaveResponse>(
            "/v1/settings/remote/save",
            serde_json::json!(request),
        )
    })
    .await
    .map_err(|_| "remote operation worker failed".to_string())?
}

#[tauri::command]
async fn change_remote_path(
    supervisor: State<'_, BridgeSupervisor>,
    request: serde_json::Value,
) -> Result<serde_json::Value, String> {
    let client = supervisor.management_client()?;
    tauri::async_runtime::spawn_blocking(move || {
        client.remote_request("/v1/settings/remote/paths", request)
    })
    .await
    .map_err(|_| "remote operation worker failed".to_string())?
}

#[tauri::command]
fn permission_settings(
    supervisor: State<'_, BridgeSupervisor>,
    workspace_root: Option<String>,
) -> Result<PermissionSettingsView, String> {
    supervisor.permission_settings(workspace_root.unwrap_or_default())
}

#[tauri::command]
fn change_permission_settings(
    supervisor: State<'_, BridgeSupervisor>,
    change: PermissionSettingsChange,
) -> Result<PermissionSettingsView, String> {
    supervisor.change_permission_settings(change)
}

#[tauri::command]
fn secrets_settings(
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<SecretsSettingsView, String> {
    supervisor.secrets_settings()
}

#[tauri::command]
fn change_secrets_settings(
    supervisor: State<'_, BridgeSupervisor>,
    change: SecretsSettingsChange,
) -> Result<SecretsSettingsView, String> {
    supervisor.change_secrets_settings(change)
}

#[tauri::command]
fn sandbox_settings(
    supervisor: State<'_, BridgeSupervisor>,
    workspace_root: Option<String>,
    session_id: Option<String>,
) -> Result<SandboxSettingsView, String> {
    supervisor.sandbox_settings(workspace_root.as_deref(), session_id.as_deref())
}

#[tauri::command]
fn change_sandbox_settings(
    supervisor: State<'_, BridgeSupervisor>,
    change: SandboxSettingsChange,
) -> Result<SandboxSettingsView, String> {
    supervisor.change_sandbox_settings(change)
}

#[tauri::command]
fn network_settings(
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<NetworkSettingsView, String> {
    supervisor.network_settings()
}

#[tauri::command]
fn bot_runtime_status(
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<serde_json::Value, String> {
    supervisor.bot_runtime_status()
}

#[tauri::command]
fn bot_pairing(supervisor: State<'_, BridgeSupervisor>) -> Result<serde_json::Value, String> {
    supervisor.bot_pairing()
}
#[tauri::command]
fn change_bot_pairing(
    supervisor: State<'_, BridgeSupervisor>,
    action: String,
    code: String,
) -> Result<serde_json::Value, String> {
    supervisor.change_bot_pairing(action, code)
}

#[tauri::command]
fn restart_bot_runtime(
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<serde_json::Value, String> {
    supervisor.restart_bot_runtime()
}

#[tauri::command]
fn bot_settings(supervisor: State<'_, BridgeSupervisor>) -> Result<serde_json::Value, String> {
    supervisor.bot_settings()
}

#[tauri::command]
fn change_bot_settings(
    supervisor: State<'_, BridgeSupervisor>,
    change: serde_json::Value,
) -> Result<serde_json::Value, String> {
    supervisor.change_bot_settings(change)
}

#[tauri::command]
fn change_network_settings(
    supervisor: State<'_, BridgeSupervisor>,
    change: NetworkSettingsChange,
) -> Result<NetworkSettingsView, String> {
    supervisor.change_network_settings(change)
}

#[tauri::command]
fn skills_settings(
    supervisor: State<'_, BridgeSupervisor>,
    workspace_root: String,
) -> Result<SkillsSettingsView, String> {
    supervisor.skills_settings(&workspace_root)
}

#[tauri::command]
fn change_skills_settings(
    supervisor: State<'_, BridgeSupervisor>,
    change: SkillsSettingsChange,
) -> Result<SkillsSettingsView, String> {
    supervisor.change_skills_settings(change)
}

#[tauri::command]
fn plan_skill_install(
    supervisor: State<'_, BridgeSupervisor>,
    request: SkillInstallRequest,
) -> Result<SkillInstallPlan, String> {
    supervisor.plan_skill_install(request)
}

#[tauri::command]
fn install_skill(
    supervisor: State<'_, BridgeSupervisor>,
    request: SkillInstallRequest,
) -> Result<SkillInstallResult, String> {
    supervisor.install_skill(request)
}

#[tauri::command]
fn archive_skill(
    supervisor: State<'_, BridgeSupervisor>,
    request: SkillArchiveRequest,
) -> Result<SkillArchiveResult, String> {
    supervisor.archive_skill(request)
}

#[tauri::command]
fn restore_skill(
    supervisor: State<'_, BridgeSupervisor>,
    request: SkillArchiveRequest,
) -> Result<SkillArchiveResult, String> {
    supervisor.restore_skill(request)
}

#[tauri::command]
fn plugin_settings(supervisor: State<'_, BridgeSupervisor>) -> Result<PluginSettingsView, String> {
    supervisor
        .plugin_settings()
        .map(plugin_settings_without_theme_paths)
}

#[tauri::command]
fn change_plugin_settings(
    supervisor: State<'_, BridgeSupervisor>,
    change: PluginSettingsChange,
) -> Result<PluginSettingsView, String> {
    supervisor
        .change_plugin_settings(change)
        .map(plugin_settings_without_theme_paths)
}

#[tauri::command]
fn plugin_doctor(
    supervisor: State<'_, BridgeSupervisor>,
    name: String,
) -> Result<PluginDoctorView, String> {
    supervisor.plugin_doctor(&name)
}

fn plugin_settings_without_theme_paths(mut view: PluginSettingsView) -> PluginSettingsView {
    for plugin in &mut view.plugins {
        plugin.themes.clear();
    }
    view
}

#[tauri::command]
fn plan_plugin_install(
    supervisor: State<'_, BridgeSupervisor>,
    source: String,
    mode: String,
    replace: bool,
    expected_name: String,
    expected_revision: String,
) -> Result<PluginInstallPlan, String> {
    supervisor.plan_plugin_install(&source, &mode, replace, &expected_name, &expected_revision)
}

#[tauri::command]
fn install_plugin(
    supervisor: State<'_, BridgeSupervisor>,
    request: PluginInstallRequest,
) -> Result<PluginOperationResult, String> {
    supervisor.install_plugin(request)
}

#[tauri::command]
fn remove_plugin(
    supervisor: State<'_, BridgeSupervisor>,
    request: PluginRemoveRequest,
) -> Result<PluginOperationResult, String> {
    supervisor.remove_plugin(request)
}

#[tauri::command]
fn subagent_settings(
    supervisor: State<'_, BridgeSupervisor>,
    workspace_root: String,
) -> Result<SubagentSettingsView, String> {
    supervisor.subagent_settings(&workspace_root)
}

#[tauri::command]
fn change_subagent_settings(
    supervisor: State<'_, BridgeSupervisor>,
    change: SubagentSettingsChange,
) -> Result<SubagentSettingsView, String> {
    supervisor.change_subagent_settings(change)
}

#[tauri::command]
async fn try_subagent_profile(
    supervisor: State<'_, BridgeSupervisor>,
    workspace_root: String,
    input: SubagentProfileInput,
    task: String,
) -> Result<String, String> {
    let client = supervisor.subagent_try_client()?;
    tauri::async_runtime::spawn_blocking(move || {
        client.try_subagent_profile(&workspace_root, input, &task)
    })
    .await
    .map_err(|error| format!("subagent try task failed: {error}"))?
}

#[tauri::command]
async fn subagent_profile_try_status(
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<SubagentTryStatusView, String> {
    let client = supervisor.subagent_try_client()?;
    tauri::async_runtime::spawn_blocking(move || client.subagent_profile_try_status())
        .await
        .map_err(|error| format!("subagent try status query failed: {error}"))?
}

#[tauri::command]
async fn cancel_subagent_profile_try(
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<(), String> {
    let client = supervisor.subagent_try_client()?;
    tauri::async_runtime::spawn_blocking(move || client.cancel_subagent_profile_try())
        .await
        .map_err(|error| format!("subagent try cancellation failed: {error}"))?
}

#[tauri::command]
fn hooks_settings(
    supervisor: State<'_, BridgeSupervisor>,
    scope: String,
    workspace_root: String,
) -> Result<HooksSettingsView, String> {
    supervisor.hooks_settings(&scope, &workspace_root)
}

#[tauri::command]
fn change_hooks_settings(
    supervisor: State<'_, BridgeSupervisor>,
    change: HooksSettingsChange,
) -> Result<HooksSettingsView, String> {
    supervisor.change_hooks_settings(change)
}

#[tauri::command]
fn memory_settings(
    supervisor: State<'_, BridgeSupervisor>,
    workspace_root: String,
) -> Result<MemorySettingsView, String> {
    supervisor.memory_settings(&workspace_root)
}

#[tauri::command]
fn change_memory_settings(
    supervisor: State<'_, BridgeSupervisor>,
    change: MemorySettingsChange,
) -> Result<MemorySettingsView, String> {
    supervisor.change_memory_settings(change)
}

#[tauri::command]
fn memory_suggestions(
    supervisor: State<'_, BridgeSupervisor>,
    workspace_root: String,
) -> Result<MemorySuggestionsView, String> {
    supervisor.memory_suggestions(&workspace_root)
}

#[tauri::command]
fn accept_memory_suggestion(
    supervisor: State<'_, BridgeSupervisor>,
    request: MemorySuggestionAcceptanceRequest,
) -> Result<MemorySuggestionAcceptance, String> {
    supervisor.accept_memory_suggestion(request)
}

#[tauri::command]
fn desktop_preferences(
    app: tauri::AppHandle,
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<DesktopPreferences, String> {
    #[cfg(target_os = "macos")]
    {
        app.state::<native_appearance::NativeAppearance>()
            .sync(&app, &supervisor)
    }
    #[cfg(not(target_os = "macos"))]
    {
        let _ = app;
        supervisor.desktop_preferences()
    }
}

#[tauri::command]
fn set_desktop_approval(
    supervisor: State<'_, BridgeSupervisor>,
    mode: String,
) -> Result<DesktopPreferences, String> {
    supervisor.set_desktop_approval(mode)
}

#[tauri::command]
fn set_desktop_terminal_theme(
    supervisor: State<'_, BridgeSupervisor>,
    theme: String,
) -> Result<DesktopPreferences, String> {
    supervisor.set_desktop_terminal_theme(theme)
}

#[tauri::command]
fn set_desktop_appearance(
    app: tauri::AppHandle,
    supervisor: State<'_, BridgeSupervisor>,
    theme: String,
    style: String,
) -> Result<DesktopPreferences, String> {
    #[cfg(target_os = "macos")]
    {
        app.state::<native_appearance::NativeAppearance>()
            .save(&app, &supervisor, theme, style)
    }
    #[cfg(not(target_os = "macos"))]
    {
        let _ = app;
        supervisor.set_desktop_appearance(theme, style)
    }
}

#[tauri::command]
fn set_desktop_language(
    supervisor: State<'_, BridgeSupervisor>,
    language: String,
) -> Result<DesktopPreferences, String> {
    supervisor.set_desktop_language(language)
}

#[tauri::command]
fn set_tray_locale(window: tauri::WebviewWindow, locale: String) -> Result<(), String> {
    window
        .try_state::<tray::TrayMenuState>()
        .ok_or("system tray is unavailable; use the main window")?
        .set_locale(&locale)
}

#[tauri::command]
fn set_desktop_currency(
    supervisor: State<'_, BridgeSupervisor>,
    currency: String,
) -> Result<DesktopPreferences, String> {
    supervisor.set_desktop_currency(currency)
}

#[tauri::command]
fn set_default_model(
    supervisor: State<'_, BridgeSupervisor>,
    request: BridgeSetDefaultModelRequest,
) -> Result<BridgeProviderSummaryResponse, String> {
    supervisor.set_default_model(request)
}

#[tauri::command]
fn set_model_role(
    supervisor: State<'_, BridgeSupervisor>,
    request: BridgeSetModelRoleRequest,
) -> Result<BridgeProviderSummaryResponse, String> {
    supervisor.set_model_role(request)
}

#[tauri::command]
fn set_agent_preferences(
    supervisor: State<'_, BridgeSupervisor>,
    request: BridgeSetAgentPreferenceRequest,
) -> Result<BridgeProviderSummaryResponse, String> {
    supervisor.set_agent_preferences(request)
}

#[tauri::command]
fn test_provider_model(
    request: protocol_generated::BridgeProviderModelProbeRequest,
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<protocol_generated::BridgeProviderModelProbeResponse, String> {
    supervisor.test_provider_model(request)
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
        "旧会话兼容目录无法读取；当前仅显示未核验的持久目录，不能据此操作会话。修复目录后重新检查会话目录。"
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
) -> Result<WorkbenchTitleBackfillResult, String> {
    // The identity directory can contain migrated sessions that are not yet
    // present in the host's bounded recent-session catalog. Backfill every
    // supplied identity title first; the bridge only fills fallback titles,
    // and catalog.fill_titles below remains a no-op for identity-only rows.
    let _sessions = catalog.list()?;
    let resolved = supervisor.backfill_session_titles(identity_title_backfill_request(titles))?;
    let sessions = catalog.fill_titles(
        resolved
            .iter()
            .map(|title| WorkbenchTitle {
                session_id: title.session_id.clone(),
                title: title.title.clone(),
            })
            .collect(),
    )?;
    Ok(WorkbenchTitleBackfillResult {
        sessions,
        resolved_titles: resolved,
    })
}

fn identity_title_backfill_request(titles: Vec<WorkbenchTitle>) -> Vec<SessionFirstMessageTitle> {
    titles
        .into_iter()
        .map(|title| SessionFirstMessageTitle {
            session_id: title.session_id,
            title: title.title,
        })
        .collect()
}

#[cfg(test)]
mod identity_title_backfill_tests {
    use super::{identity_title_backfill_request, WorkbenchTitle};

    #[test]
    fn includes_identity_only_sessions_missing_from_the_host_catalog() {
        let request = identity_title_backfill_request(vec![WorkbenchTitle {
            session_id: "migrated-session".into(),
            title: "Recovered from first user message".into(),
        }]);
        assert_eq!(request.len(), 1);
        assert_eq!(request[0].session_id, "migrated-session");
        assert_eq!(request[0].title, "Recovered from first user message");
    }
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

fn validated_external_url(value: &str) -> Result<String, String> {
    if value.len() > 16 << 10 || value.chars().any(char::is_control) {
        return Err("external link is invalid".into());
    }
    let url = url::Url::parse(value).map_err(|_| "external link is invalid")?;
    if !matches!(url.scheme(), "http" | "https")
        || url.host_str().is_none()
        || !url.username().is_empty()
        || url.password().is_some()
    {
        return Err("external link must be an HTTP(S) URL without userinfo".into());
    }
    Ok(url.into())
}

// A controller launch URL carries the Serve token in a fragment. It comes
// only from the authenticated sidecar, never from renderer input, and must be
// the exact loopback bootstrap shape emitted by that sidecar before the
// host hands it to the system browser.
fn validated_remote_controller_url(value: &str) -> Result<String, String> {
    if value.len() > 1024 || value.chars().any(char::is_control) {
        return Err("remote controller URL is invalid".into());
    }
    let url = url::Url::parse(value).map_err(|_| "remote controller URL is invalid")?;
    let loopback = url
        .host_str()
        .and_then(|host| host.parse::<std::net::IpAddr>().ok())
        .is_some_and(|ip| ip.is_loopback());
    if url.scheme() != "http"
        || !loopback
        || url.port().is_none()
        || !url.username().is_empty()
        || url.password().is_some()
        || url.path() != "/"
        || url.query().is_some()
    {
        return Err("remote controller must use a loopback HTTP URL".into());
    }
    let mut pairs = url::form_urlencoded::parse(url.fragment().unwrap_or_default().as_bytes());
    let Some((key, token)) = pairs.next() else {
        return Err("remote controller token is missing".into());
    };
    if key != "token"
        || pairs.next().is_some()
        || token.len() != 64
        || !token.bytes().all(|byte| byte.is_ascii_hexdigit())
    {
        return Err("remote controller token is invalid".into());
    }
    Ok(url.into())
}

// Markdown includes email links as well as web links. Keep this distinct from
// OAuth/browser actions, which accept only HTTP(S), and never allow arbitrary
// application schemes or mail-client attachment parameters.
fn validated_external_link(value: &str) -> Result<String, String> {
    let url = url::Url::parse(value).map_err(|_| "external link is invalid")?;
    if url.scheme() != "mailto" {
        return validated_external_url(value);
    }
    if value.len() > 16 << 10
        || value.chars().any(char::is_control)
        || url.host_str().is_some()
        || url.fragment().is_some()
        || ["%0a", "%0d", "%00"]
            .iter()
            .any(|encoded| url.path().to_ascii_lowercase().contains(encoded))
        || url.query_pairs().any(|(name, value)| {
            let name = name.to_ascii_lowercase();
            !matches!(name.as_str(), "subject" | "body" | "cc" | "bcc")
                || value
                    .chars()
                    .any(|c| c == '\0' || (name != "body" && c.is_control()))
        })
    {
        return Err("email link must use standard recipients, subject and body fields".into());
    }
    Ok(url.into())
}

#[tauri::command]
fn open_external_link(app: tauri::AppHandle, url: String) -> Result<(), String> {
    app.opener()
        .open_url(validated_external_link(&url)?, None::<&str>)
        .map_err(|error| error.to_string())
}

#[tauri::command]
fn open_external_url(app: tauri::AppHandle, url: String) -> Result<(), String> {
    let url = validated_external_url(&url)?;
    app.opener()
        .open_url(url, None::<&str>)
        .map_err(|error| error.to_string())
}

#[tauri::command]
fn get_close_behavior(
    app: tauri::AppHandle,
    preferences: State<'_, HostPreferences>,
) -> CloseBehavior {
    // Report the behavior that CloseRequested actually uses, without rewriting
    // an explicit saved preference when a Linux tray is temporarily unavailable.
    preferences.close_behavior().effective_for_tray(
        std::env::consts::OS,
        app.try_state::<tray::TrayMenuState>().is_some(),
    )
}

#[tauri::command]
fn set_close_behavior(
    app: tauri::AppHandle,
    preferences: State<'_, HostPreferences>,
    behavior: CloseBehavior,
) -> Result<CloseBehavior, String> {
    if behavior.effective_for_tray(
        std::env::consts::OS,
        app.try_state::<tray::TrayMenuState>().is_some(),
    ) != behavior
    {
        return Err("system tray is unavailable; choose quit when closing".into());
    }
    preferences.set_close_behavior(behavior)?;
    Ok(behavior)
}

#[tauri::command]
fn get_zoom_factor(preferences: State<'_, HostPreferences>) -> f64 {
    preferences.zoom_factor()
}

#[tauri::command]
fn set_zoom_factor(
    window: tauri::WebviewWindow,
    preferences: State<'_, HostPreferences>,
    factor: f64,
) -> Result<f64, String> {
    let previous = preferences.zoom_factor();
    preferences.set_zoom_factor(factor)?;
    let applied = preferences.zoom_factor();
    if let Err(error) = window.set_zoom(applied) {
        let _ = preferences.set_zoom_factor(previous);
        return Err(format!("apply WebView zoom: {error}"));
    }
    Ok(applied)
}

#[tauri::command]
fn get_active_theme_id(preferences: State<'_, HostPreferences>) -> String {
    preferences.active_theme_id()
}

#[tauri::command]
fn set_active_theme_id(
    app: tauri::AppHandle,
    supervisor: State<'_, BridgeSupervisor>,
    preferences: State<'_, HostPreferences>,
    id: String,
) -> Result<String, String> {
    if id.starts_with("plugin:")
        && !load_plugin_theme_views(&app, &supervisor)?
            .iter()
            .any(|theme| theme.theme.id == id)
    {
        return Err("plugin theme is unavailable; enable its plugin and refresh".into());
    }
    preferences.set_active_theme_id(id.clone())?;
    Ok(preferences.active_theme_id())
}

#[tauri::command]
fn list_plugin_themes(
    app: tauri::AppHandle,
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<Vec<UserThemeView>, String> {
    load_plugin_theme_views(&app, &supervisor)
}

#[tauri::command]
fn list_user_themes(preferences: State<'_, HostPreferences>) -> Vec<UserThemeView> {
    preferences
        .user_themes()
        .into_iter()
        .map(|theme| user_theme_view(&preferences, theme))
        .collect()
}

#[tauri::command]
fn save_user_theme(
    preferences: State<'_, HostPreferences>,
    mut theme: UserTheme,
) -> Result<UserThemeView, String> {
    if let Some(data_url) = theme.background_asset_data_url.take() {
        let (extension, bytes) = decode_theme_image_data_url(&data_url)?;
        let filename = format!("background.{extension}");
        validate_theme_image(&filename, &bytes)?;
        let mut background = theme.background.take().unwrap_or_default();
        background.image = filename;
        theme.background = Some(background);
        theme.background_asset_bytes = Some(bytes);
    }
    if let Some(data_url) = theme.task_background_asset_data_url.take() {
        let (extension, bytes) = decode_theme_image_data_url(&data_url)?;
        let filename = format!("background-task.{extension}");
        validate_theme_image(&filename, &bytes)?;
        let mut background = theme.task_background.take().unwrap_or_default();
        background.image = filename;
        theme.task_background = Some(background);
        theme.task_background_asset_bytes = Some(bytes);
    }
    let saved = preferences.save_user_theme(theme)?;
    Ok(user_theme_view(&preferences, saved))
}

fn decode_theme_image_data_url(data_url: &str) -> Result<(String, Vec<u8>), String> {
    const MAX_ENCODED_IMAGE: usize = ((16 << 20) * 4 / 3) + 8;
    if data_url.len() > MAX_ENCODED_IMAGE {
        return Err("theme image exceeds the 16 MiB limit".into());
    }
    let (extension, encoded) = if let Some(value) = data_url.strip_prefix("data:image/png;base64,")
    {
        ("png", value)
    } else if let Some(value) = data_url.strip_prefix("data:image/jpeg;base64,") {
        ("jpg", value)
    } else if let Some(value) = data_url.strip_prefix("data:image/jpg;base64,") {
        ("jpg", value)
    } else if let Some(value) = data_url.strip_prefix("data:image/webp;base64,") {
        ("webp", value)
    } else {
        return Err("theme image must be PNG, JPEG, or WebP".into());
    };
    let bytes = BASE64_STANDARD
        .decode(encoded)
        .map_err(|error| format!("decode theme image: {error}"))?;
    if bytes.len() > 16 << 20 {
        return Err("theme image exceeds the 16 MiB limit".into());
    }
    Ok((extension.into(), bytes))
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct UserThemeView {
    #[serde(flatten)]
    theme: UserTheme,
    background_path: Option<String>,
    task_background_path: Option<String>,
    kind: &'static str,
    #[serde(skip_serializing_if = "Option::is_none")]
    plugin_name: Option<String>,
}

fn user_theme_view(preferences: &HostPreferences, theme: UserTheme) -> UserThemeView {
    let image_path = |image: Option<&str>| {
        image
            .and_then(|image| preferences.theme_asset_path(&theme, image))
            .map(|path| path.to_string_lossy().into_owned())
    };
    let background_path = image_path(theme.background.as_ref().map(|bg| bg.image.as_str()));
    let task_background_path =
        image_path(theme.task_background.as_ref().map(|bg| bg.image.as_str()));
    UserThemeView {
        theme,
        background_path,
        task_background_path,
        kind: "user",
        plugin_name: None,
    }
}

fn load_plugin_theme_views(
    app: &tauri::AppHandle,
    supervisor: &BridgeSupervisor,
) -> Result<Vec<UserThemeView>, String> {
    let plugins = supervisor.plugin_settings()?;
    let app_data = app
        .path()
        .app_data_dir()
        .map_err(|error| format!("resolve Preview data directory: {error}"))?;
    let asset_root = app_data.join("theme-assets").join("plugin-themes");
    let mut views = Vec::new();
    for plugin in plugins
        .plugins
        .into_iter()
        .filter(|plugin| plugin.enabled && plugin.status == "ready")
    {
        let root = match Path::new(&plugin.root).canonicalize() {
            Ok(root) => root,
            Err(_) => continue,
        };
        for contribution in plugin.themes {
            let source = Path::new(&contribution.path);
            let metadata = match fs::symlink_metadata(source) {
                Ok(metadata) if metadata.is_file() && !metadata.file_type().is_symlink() => {
                    metadata
                }
                _ => continue,
            };
            if metadata.len() > (36 << 20) {
                continue;
            }
            let source = match canonical_plugin_theme_file(&root, source) {
                Some(source) => source,
                _ => continue,
            };
            let mut theme = match read_theme_package(&source) {
                Ok(theme) => theme,
                Err(_) => continue,
            };
            let source_theme_id = theme.id.clone();
            if !valid_plugin_theme_pack_id(&source_theme_id) {
                continue;
            }
            theme.id = "user-plugin-validation".into();
            theme = match host_preferences::validate_user_theme(theme) {
                Ok(theme) => theme,
                Err(_) => continue,
            };
            theme.id = format!("plugin:{}:{}", plugin.name, source_theme_id);
            let background_path = stage_plugin_theme_asset(
                &asset_root,
                &plugin.name,
                &source_theme_id,
                "background",
                theme
                    .background
                    .as_ref()
                    .map(|background| background.image.as_str()),
                theme.background_asset_bytes.as_deref(),
            )?;
            let task_background_path = stage_plugin_theme_asset(
                &asset_root,
                &plugin.name,
                &source_theme_id,
                "background-task",
                theme
                    .task_background
                    .as_ref()
                    .map(|background| background.image.as_str()),
                theme.task_background_asset_bytes.as_deref(),
            )?;
            views.push(UserThemeView {
                theme,
                background_path,
                task_background_path,
                kind: "plugin",
                plugin_name: Some(plugin.name.clone()),
            });
        }
    }
    views.sort_by(|left, right| left.theme.id.cmp(&right.theme.id));
    views.dedup_by(|left, right| left.theme.id == right.theme.id);
    Ok(views)
}

fn canonical_plugin_theme_file(root: &Path, source: &Path) -> Option<PathBuf> {
    let metadata = fs::symlink_metadata(source).ok()?;
    if !metadata.is_file() || metadata.file_type().is_symlink() {
        return None;
    }
    let source = source.canonicalize().ok()?;
    source.starts_with(root).then_some(source)
}

fn valid_plugin_theme_pack_id(id: &str) -> bool {
    !id.is_empty()
        && id.len() <= 64
        && id.as_bytes()[0].is_ascii_lowercase()
        && id
            .bytes()
            .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
        && !id.ends_with('-')
        && !id.contains("--")
}

fn ensure_plugin_theme_asset_directory(path: &Path) -> Result<(), String> {
    match fs::create_dir(path) {
        Ok(()) => {}
        Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => {}
        Err(error) => return Err(format!("create plugin theme asset directory: {error}")),
    }
    let metadata = fs::symlink_metadata(path)
        .map_err(|error| format!("inspect plugin theme asset directory: {error}"))?;
    if metadata.file_type().is_symlink() || !metadata.is_dir() {
        return Err("plugin theme asset directory is not a regular directory".into());
    }
    Ok(())
}

fn stage_plugin_theme_asset(
    asset_root: &Path,
    plugin_name: &str,
    theme_id: &str,
    prefix: &str,
    image_name: Option<&str>,
    image_bytes: Option<&[u8]>,
) -> Result<Option<String>, String> {
    static ASSET_WRITE_LOCK: Mutex<()> = Mutex::new(());
    let (Some(image_name), Some(image_bytes)) =
        (image_name.filter(|name| !name.is_empty()), image_bytes)
    else {
        return Ok(None);
    };
    if !valid_theme_image_name(image_name) {
        return Err("plugin theme image name is invalid".into());
    }
    validate_theme_image(image_name, image_bytes)?;
    let extension = image_name
        .rsplit_once('.')
        .map(|(_, ext)| ext.to_ascii_lowercase())
        .ok_or_else(|| "plugin theme image has no extension".to_string())?;
    if !matches!(extension.as_str(), "png" | "jpg" | "jpeg" | "webp") {
        return Err("plugin theme image extension is unsupported".into());
    }
    let component = |value: &str| {
        value
            .as_bytes()
            .iter()
            .map(|byte| format!("{byte:02x}"))
            .collect::<String>()
    };
    let directory = asset_root.join(format!(
        "{}-{}",
        component(plugin_name),
        component(theme_id)
    ));
    let _guard = ASSET_WRITE_LOCK
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    if let Some(parent) = asset_root.parent() {
        ensure_plugin_theme_asset_directory(parent)?;
    }
    ensure_plugin_theme_asset_directory(asset_root)?;
    ensure_plugin_theme_asset_directory(&directory)?;
    let path = directory.join(format!("{prefix}.{extension}"));
    match fs::symlink_metadata(&path) {
        Ok(metadata) if metadata.file_type().is_symlink() || !metadata.is_file() => {
            return Err("plugin theme asset is not a regular file".into());
        }
        Ok(_) => {}
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => return Err(format!("inspect plugin theme asset: {error}")),
    }
    if fs::read(&path).ok().as_deref() != Some(image_bytes) {
        static NEXT_TEMP: AtomicU64 = AtomicU64::new(0);
        let nonce = NEXT_TEMP.fetch_add(1, Ordering::Relaxed);
        let temporary = directory.join(format!(".{prefix}-{}-{nonce}.tmp", std::process::id()));
        let write_result = (|| {
            let mut file = fs::OpenOptions::new()
                .write(true)
                .create_new(true)
                .open(&temporary)
                .map_err(|error| format!("create plugin theme asset temporary file: {error}"))?;
            file.write_all(image_bytes)
                .map_err(|error| format!("write plugin theme asset: {error}"))?;
            drop(file);
            if path.exists() {
                fs::remove_file(&path)
                    .map_err(|error| format!("replace plugin theme asset: {error}"))?;
            }
            fs::rename(&temporary, &path)
                .map_err(|error| format!("publish plugin theme asset: {error}"))
        })();
        if write_result.is_err() {
            let _ = fs::remove_file(&temporary);
        }
        write_result?;
    }
    Ok(Some(path.to_string_lossy().into_owned()))
}

#[tauri::command]
fn delete_user_theme(preferences: State<'_, HostPreferences>, id: String) -> Result<(), String> {
    preferences.delete_user_theme(&id)
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ThemePackageManifest {
    schema_version: u8,
    id: String,
    name: String,
    #[serde(default)]
    author: Option<String>,
    #[serde(default)]
    description: Option<String>,
    #[serde(default)]
    license: Option<String>,
    base_style: String,
    #[serde(default)]
    tokens: host_preferences::ThemeTokens,
    #[serde(default)]
    recipes: ThemePackageRecipes,
    #[serde(default)]
    background: Option<host_preferences::ThemeBackground>,
    #[serde(default)]
    task_background: Option<host_preferences::ThemeSceneBackground>,
}

#[derive(Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ThemePackageRecipes {
    #[serde(default = "default_theme_density")]
    density: String,
    #[serde(default = "default_theme_corners")]
    corners: String,
}

fn default_theme_density() -> String {
    "comfortable".into()
}

fn default_theme_corners() -> String {
    "soft".into()
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct ExportThemePackage<'a> {
    schema_version: u8,
    id: &'a str,
    name: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    author: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    description: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    license: Option<&'a str>,
    base_style: &'a str,
    tokens: &'a host_preferences::ThemeTokens,
    recipes: ExportThemeRecipes<'a>,
    #[serde(skip_serializing_if = "Option::is_none")]
    background: Option<&'a host_preferences::ThemeBackground>,
    #[serde(skip_serializing_if = "Option::is_none")]
    task_background: Option<&'a host_preferences::ThemeSceneBackground>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct ExportThemeRecipes<'a> {
    density: &'a str,
    corners: &'a str,
}

fn selected_theme_path(
    file: Option<tauri_plugin_dialog::FilePath>,
) -> Result<Option<PathBuf>, String> {
    file.map(|selected| selected.into_path().map_err(|error| error.to_string()))
        .transpose()
}

fn receive_file_dialog_selection(
    receiver: std::sync::mpsc::Receiver<Option<tauri_plugin_dialog::FilePath>>,
) -> Result<Option<tauri_plugin_dialog::FilePath>, String> {
    receiver
        .recv()
        .map_err(|_| "file dialog closed unexpectedly; retry selecting a file".into())
}

fn read_theme_package(path: &Path) -> Result<host_preferences::UserTheme, String> {
    const MAX_PACKAGE_BYTES: u64 = 36 << 20;
    const MAX_MANIFEST_BYTES: u64 = 1 << 20;
    const MAX_IMAGE_BYTES: u64 = 16 << 20;
    let metadata =
        std::fs::metadata(path).map_err(|error| format!("read selected theme: {error}"))?;
    if metadata.len() > MAX_PACKAGE_BYTES {
        return Err("theme package exceeds the 36 MiB import limit".into());
    }
    let file = File::open(path).map_err(|error| format!("open selected theme: {error}"))?;
    let mut archive =
        ZipArchive::new(file).map_err(|error| format!("invalid theme package: {error}"))?;
    if archive.is_empty() || archive.len() > 3 {
        return Err("theme package must contain theme.json and at most two images".into());
    }
    let mut names = HashSet::new();
    for index in 0..archive.len() {
        let entry = archive
            .by_index(index)
            .map_err(|error| format!("read theme entry: {error}"))?;
        let name = entry.name();
        if name.is_empty() || name.contains('/') || name.contains('\\') || entry.is_dir() {
            return Err("theme package may only contain root-level files".into());
        }
        if !names.insert(name.to_ascii_lowercase()) {
            return Err("theme package contains duplicate entry names".into());
        }
        if entry
            .unix_mode()
            .is_some_and(|mode| mode & 0o170000 == 0o120000)
        {
            return Err("theme package must not contain symlinks".into());
        }
        if name.eq_ignore_ascii_case("theme.json") {
            if entry.size() > MAX_MANIFEST_BYTES {
                return Err("theme manifest exceeds the 1 MiB import limit".into());
            }
        } else if !valid_theme_image_name(name) || entry.size() > MAX_IMAGE_BYTES {
            return Err("theme package contains an unsupported or oversized image".into());
        }
    }
    let mut bytes = Vec::new();
    archive
        .by_name("theme.json")
        .map_err(|_| "theme package is missing theme.json".to_string())?
        .take(MAX_MANIFEST_BYTES + 1)
        .read_to_end(&mut bytes)
        .map_err(|error| format!("read theme manifest: {error}"))?;
    if bytes.len() as u64 > MAX_MANIFEST_BYTES {
        return Err("theme manifest exceeds the 1 MiB import limit".into());
    }
    let mut manifest: ThemePackageManifest = serde_json::from_slice(&bytes)
        .map_err(|error| format!("invalid theme manifest: {error}"))?;
    if !matches!(manifest.schema_version, 1 | 2) {
        return Err("unsupported theme package schema version".into());
    }
    let home_name = manifest
        .background
        .as_ref()
        .map(|bg| bg.image.clone())
        .filter(|name| !name.is_empty());
    let task_name = manifest
        .task_background
        .as_ref()
        .map(|bg| bg.image.clone())
        .filter(|name| !name.is_empty());
    let mut declared = HashSet::new();
    for image in [home_name.as_deref(), task_name.as_deref()]
        .into_iter()
        .flatten()
    {
        if !valid_theme_image_name(image) || !declared.insert(image.to_ascii_lowercase()) {
            return Err("theme background image name is invalid or duplicated".into());
        }
    }
    let actual: HashSet<String> = names
        .into_iter()
        .filter(|name| name != "theme.json")
        .collect();
    if actual != declared {
        return Err("theme package images must exactly match the manifest".into());
    }
    let read_asset = |name: &str, archive: &mut ZipArchive<File>| -> Result<Vec<u8>, String> {
        let mut asset = Vec::new();
        archive
            .by_name(name)
            .map_err(|_| format!("theme package is missing image {name}"))?
            .take(MAX_IMAGE_BYTES + 1)
            .read_to_end(&mut asset)
            .map_err(|error| format!("read theme image: {error}"))?;
        if asset.len() as u64 > MAX_IMAGE_BYTES {
            return Err("theme image exceeds the 16 MiB import limit".into());
        }
        validate_theme_image(name, &asset)?;
        Ok(asset)
    };
    let background_asset_bytes = home_name
        .as_deref()
        .map(|name| read_asset(name, &mut archive))
        .transpose()?;
    let task_background_asset_bytes = task_name
        .as_deref()
        .map(|name| read_asset(name, &mut archive))
        .transpose()?;
    if let (Some(background), Some(name)) = (manifest.background.as_mut(), home_name.as_deref()) {
        background.image = canonical_theme_asset_name("background", name);
    }
    if let (Some(background), Some(name)) =
        (manifest.task_background.as_mut(), task_name.as_deref())
    {
        background.image = canonical_theme_asset_name("background-task", name);
    }
    Ok(host_preferences::UserTheme {
        id: manifest.id,
        name: manifest.name,
        author: manifest.author,
        description: manifest.description,
        license: manifest.license,
        base_style: manifest.base_style,
        tokens: manifest.tokens,
        density: manifest.recipes.density,
        corners: manifest.recipes.corners,
        background: manifest.background,
        task_background: manifest.task_background,
        background_asset_bytes,
        task_background_asset_bytes,
        background_asset_data_url: None,
        task_background_asset_data_url: None,
        clear_background: false,
        clear_task_background: false,
    })
}

fn valid_theme_image_name(name: &str) -> bool {
    name.len() <= 128
        && !name.is_empty()
        && name
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || b"._-".contains(&byte))
        && !name.contains("..")
        && matches!(
            name.rsplit_once('.')
                .map(|(_, ext)| ext.to_ascii_lowercase())
                .as_deref(),
            Some("png" | "jpg" | "jpeg" | "webp")
        )
}

fn canonical_theme_asset_name(prefix: &str, source: &str) -> String {
    let extension = source
        .rsplit_once('.')
        .map(|(_, ext)| ext.to_ascii_lowercase())
        .unwrap_or_else(|| "webp".into());
    format!("{prefix}.{extension}")
}

fn validate_theme_image(name: &str, bytes: &[u8]) -> Result<(), String> {
    if bytes.is_empty() || bytes.len() > 16 << 20 {
        return Err("theme image is empty or exceeds 16 MiB".into());
    }
    let extension = name
        .rsplit_once('.')
        .map(|(_, ext)| ext.to_ascii_lowercase())
        .unwrap_or_default();
    let dimensions = if bytes.starts_with(&[0x89, b'P', b'N', b'G', 0x0d, 0x0a, 0x1a, 0x0a])
        && extension == "png"
        && bytes.len() >= 24
        && &bytes[12..16] == b"IHDR"
    {
        (
            u32::from_be_bytes(bytes[16..20].try_into().unwrap()),
            u32::from_be_bytes(bytes[20..24].try_into().unwrap()),
        )
    } else if bytes.starts_with(&[0xff, 0xd8, 0xff]) && matches!(extension.as_str(), "jpg" | "jpeg")
    {
        jpeg_dimensions(bytes).ok_or_else(|| "invalid JPEG theme image".to_string())?
    } else if bytes.len() >= 30
        && &bytes[0..4] == b"RIFF"
        && &bytes[8..12] == b"WEBP"
        && extension == "webp"
    {
        webp_dimensions(bytes).ok_or_else(|| "invalid WebP theme image".to_string())?
    } else {
        return Err("theme image must be PNG, JPEG, or WebP with a matching extension".into());
    };
    if dimensions.0 == 0 || dimensions.1 == 0 || dimensions.0 > 8192 || dimensions.1 > 8192 {
        return Err("theme image dimensions must be between 1 and 8192 pixels".into());
    }
    Ok(())
}

fn jpeg_dimensions(bytes: &[u8]) -> Option<(u32, u32)> {
    let mut cursor = 2;
    while cursor + 4 <= bytes.len() {
        if bytes[cursor] != 0xff {
            cursor += 1;
            continue;
        }
        while cursor < bytes.len() && bytes[cursor] == 0xff {
            cursor += 1;
        }
        let marker = *bytes.get(cursor)?;
        cursor += 1;
        if matches!(marker, 0xd8 | 0xd9 | 0x01 | 0xd0..=0xd7) {
            continue;
        }
        let length = u16::from_be_bytes([*bytes.get(cursor)?, *bytes.get(cursor + 1)?]) as usize;
        if length < 2 || cursor + length > bytes.len() {
            return None;
        }
        if matches!(marker, 0xc0..=0xc3 | 0xc5..=0xc7 | 0xc9..=0xcb | 0xcd..=0xcf) {
            return Some((
                u16::from_be_bytes([*bytes.get(cursor + 5)?, *bytes.get(cursor + 6)?]) as u32,
                u16::from_be_bytes([*bytes.get(cursor + 3)?, *bytes.get(cursor + 4)?]) as u32,
            ));
        }
        cursor += length;
    }
    None
}

fn webp_dimensions(bytes: &[u8]) -> Option<(u32, u32)> {
    match &bytes[12..16] {
        b"VP8X" if bytes.len() >= 30 => Some((
            1 + u32::from_le_bytes([bytes[24], bytes[25], bytes[26], 0]),
            1 + u32::from_le_bytes([bytes[27], bytes[28], bytes[29], 0]),
        )),
        b"VP8L" if bytes.len() >= 25 && bytes[20] == 0x2f => {
            let b0 = bytes[21] as u32;
            let b1 = bytes[22] as u32;
            let b2 = bytes[23] as u32;
            let b3 = bytes[24] as u32;
            Some((
                1 + b0 + ((b1 & 0x3f) << 8),
                1 + (b1 >> 6) + (b2 << 2) + ((b3 & 0x0f) << 10),
            ))
        }
        b"VP8 " if bytes.len() >= 30 && bytes[23..26] == [0x9d, 0x01, 0x2a] => Some((
            (u16::from_le_bytes([bytes[26], bytes[27]]) & 0x3fff) as u32,
            (u16::from_le_bytes([bytes[28], bytes[29]]) & 0x3fff) as u32,
        )),
        _ => None,
    }
}

fn write_theme_package(
    theme: &host_preferences::UserTheme,
    asset_root: &Path,
    selected_path: PathBuf,
) -> Result<(), String> {
    if !theme.id.starts_with("user-")
        || theme.id.len() > 64
        || theme.id.ends_with('-')
        || theme.id.contains("--")
        || !theme
            .id
            .bytes()
            .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
    {
        return Err("invalid stored custom theme ID".into());
    }
    let mut path = selected_path;
    if path.extension().and_then(|extension| extension.to_str()) != Some("reasonix-theme") {
        path.set_extension("reasonix-theme");
    }
    let parent = path
        .parent()
        .ok_or_else(|| "invalid export destination".to_string())?;
    let mut temporary = tempfile::NamedTempFile::new_in(parent)
        .map_err(|error| format!("prepare theme export: {error}"))?;
    let manifest = ExportThemePackage {
        schema_version: 2,
        id: &theme.id,
        name: &theme.name,
        author: theme.author.as_deref(),
        description: theme.description.as_deref(),
        license: theme.license.as_deref(),
        base_style: &theme.base_style,
        tokens: &theme.tokens,
        recipes: ExportThemeRecipes {
            density: &theme.density,
            corners: &theme.corners,
        },
        background: theme.background.as_ref(),
        task_background: theme.task_background.as_ref(),
    };
    let bytes = serde_json::to_vec_pretty(&manifest)
        .map_err(|error| format!("serialize theme: {error}"))?;
    {
        let mut archive = ZipWriter::new(temporary.as_file_mut());
        archive
            .start_file(
                "theme.json",
                SimpleFileOptions::default().compression_method(CompressionMethod::Deflated),
            )
            .map_err(|error| format!("create theme package: {error}"))?;
        archive
            .write_all(&bytes)
            .map_err(|error| format!("write theme package: {error}"))?;
        for image in [
            theme.background.as_ref().map(|bg| bg.image.as_str()),
            theme.task_background.as_ref().map(|bg| bg.image.as_str()),
        ]
        .into_iter()
        .flatten()
        .filter(|image| !image.is_empty())
        {
            if !valid_theme_image_name(image) {
                return Err("invalid stored theme image name".into());
            }
            let image_candidate = asset_root.join(&theme.id).join(image);
            let root = asset_root
                .canonicalize()
                .map_err(|error| format!("resolve theme asset directory: {error}"))?;
            let image_path = image_candidate
                .canonicalize()
                .map_err(|error| format!("resolve stored theme image: {error}"))?;
            if !image_path.starts_with(&root)
                || !image_candidate
                    .symlink_metadata()
                    .is_ok_and(|metadata| metadata.file_type().is_file())
            {
                return Err("stored theme image is outside the theme asset directory".into());
            }
            let image_bytes = std::fs::read(&image_path)
                .map_err(|error| format!("read stored theme image: {error}"))?;
            validate_theme_image(image, &image_bytes)?;
            archive
                .start_file(
                    image,
                    SimpleFileOptions::default().compression_method(CompressionMethod::Deflated),
                )
                .map_err(|error| format!("add theme image: {error}"))?;
            archive
                .write_all(&image_bytes)
                .map_err(|error| format!("write theme image: {error}"))?;
        }
        archive
            .finish()
            .map_err(|error| format!("finish theme package: {error}"))?;
    }
    temporary
        .as_file()
        .sync_all()
        .map_err(|error| format!("sync theme package: {error}"))?;
    temporary
        .persist(&path)
        .map_err(|error| format!("save theme package: {}", error.error))?;
    Ok(())
}

#[tauri::command]
async fn import_user_theme(
    app: tauri::AppHandle,
    preferences: State<'_, HostPreferences>,
) -> Result<Option<UserThemeView>, String> {
    let (sender, receiver) = std::sync::mpsc::channel();
    app.dialog()
        .file()
        .add_filter("Reasonix Theme", &["reasonix-theme"])
        .pick_file(move |selection| {
            let _ = sender.send(selection);
        });
    let selected =
        tauri::async_runtime::spawn_blocking(move || receive_file_dialog_selection(receiver))
            .await
            .map_err(|_| "file dialog task failed; retry selecting a file")??;
    let Some(path) = selected_theme_path(selected)? else {
        return Ok(None);
    };
    let theme = read_theme_package(&path)?;
    preferences
        .import_user_theme(theme)
        .map(|theme| Some(user_theme_view(&preferences, theme)))
}

#[tauri::command]
async fn export_user_theme(
    app: tauri::AppHandle,
    preferences: State<'_, HostPreferences>,
    id: String,
) -> Result<bool, String> {
    let theme = preferences
        .user_themes()
        .into_iter()
        .find(|theme| theme.id == id)
        .ok_or_else(|| "custom theme not found".to_string())?;
    let (sender, receiver) = std::sync::mpsc::channel();
    app.dialog()
        .file()
        .add_filter("Reasonix Theme", &["reasonix-theme"])
        .set_file_name(format!("{}.reasonix-theme", theme.id))
        .save_file(move |selection| {
            let _ = sender.send(selection);
        });
    let selected =
        tauri::async_runtime::spawn_blocking(move || receive_file_dialog_selection(receiver))
            .await
            .map_err(|_| "file dialog task failed; retry selecting a file")??;
    let Some(path) = selected_theme_path(selected)? else {
        return Ok(false);
    };
    let asset_root = app
        .path()
        .app_data_dir()
        .map_err(|error| format!("resolve theme asset directory: {error}"))?
        .join("theme-assets");
    write_theme_package(&theme, &asset_root, path)?;
    Ok(true)
}

fn frontend_diagnostics_export_filename(payload: &serde_json::Value) -> Result<String, String> {
    const MAX_EXPORT_BYTES: usize = 8 << 20;
    let schema_version = payload
        .get("schemaVersion")
        .and_then(serde_json::Value::as_u64)
        .ok_or_else(|| "frontend diagnostics schema version is missing".to_string())?;
    if schema_version != 1 && schema_version != 2 {
        return Err("frontend diagnostics schema version is unsupported".to_string());
    }
    let report_id = payload
        .pointer("/manifest/reportId")
        .and_then(serde_json::Value::as_str)
        .ok_or_else(|| "frontend diagnostics report ID is missing".to_string())?;
    if !payload
        .get("events")
        .is_some_and(serde_json::Value::is_array)
    {
        return Err("frontend diagnostics events are missing".to_string());
    }
    let encoded = serde_json::to_vec_pretty(payload).map_err(|error| error.to_string())?;
    if encoded.len() > MAX_EXPORT_BYTES {
        return Err("frontend diagnostics report exceeds the 8 MiB export limit".to_string());
    }
    let suffix: String = report_id
        .chars()
        .filter(|character| character.is_ascii_alphanumeric() || *character == '-')
        .take(8)
        .collect();
    let suffix = if suffix.is_empty() { "trace" } else { &suffix };
    Ok(format!("reasonix-frontend-diagnostics-{suffix}.json"))
}

fn write_frontend_diagnostics_export(
    path: &Path,
    payload: &serde_json::Value,
) -> Result<(), String> {
    frontend_diagnostics_export_filename(payload)?;
    let encoded = serde_json::to_vec_pretty(payload).map_err(|error| error.to_string())?;
    let parent = path
        .parent()
        .ok_or("invalid export destination; choose a file in a writable folder")?;
    // Match the other native Save As paths: complete and sync a private sibling
    // before replacing the chosen name. Never truncate an existing report or
    // follow its symlink to modify another file.
    let mut temporary = tempfile::NamedTempFile::new_in(parent)
        .map_err(|_| "cannot prepare diagnostics export; choose a writable folder")?;
    temporary
        .write_all(&encoded)
        .map_err(|_| "cannot write diagnostics export; check free space and retry")?;
    temporary
        .as_file()
        .sync_all()
        .map_err(|_| "cannot finish diagnostics export; check free space and retry")?;
    temporary
        .persist(path)
        .map_err(|_| "cannot save diagnostics export; choose a writable file destination")?;
    Ok(())
}

#[tauri::command]
async fn export_frontend_diagnostics(
    app: tauri::AppHandle,
    payload: serde_json::Value,
) -> Result<bool, String> {
    let filename = frontend_diagnostics_export_filename(&payload)?;
    let (sender, receiver) = std::sync::mpsc::channel();
    app.dialog()
        .file()
        .add_filter("JSON diagnostics", &["json"])
        .set_file_name(filename)
        .save_file(move |selection| {
            let _ = sender.send(selection);
        });
    let selected =
        tauri::async_runtime::spawn_blocking(move || receive_file_dialog_selection(receiver))
            .await
            .map_err(|_| "file dialog task failed; retry selecting a file")??;
    let Some(path) = selected_theme_path(selected)? else {
        return Ok(false);
    };
    write_frontend_diagnostics_export(&path, &payload)?;
    Ok(true)
}

#[cfg(test)]
mod frontend_diagnostics_export_tests {
    use super::*;

    fn payload() -> serde_json::Value {
        serde_json::json!({
            "schemaVersion": 2,
            "manifest": { "reportId": "0123456789abcdef", "createdAt": "2026-09-30T00:00:00Z" },
            "summary": { "eventCount": 1 },
            "events": [{ "t": 0, "type": "start" }]
        })
    }

    #[test]
    fn export_name_uses_only_a_bounded_safe_report_id() {
        let mut report = payload();
        report["manifest"]["reportId"] = serde_json::Value::String("../private path/abcdef".into());
        assert_eq!(
            frontend_diagnostics_export_filename(&report).unwrap(),
            "reasonix-frontend-diagnostics-privatep.json"
        );
    }

    #[test]
    fn export_rejects_unknown_schema_and_oversized_payload() {
        let mut report = payload();
        report["schemaVersion"] = serde_json::Value::from(99);
        assert!(frontend_diagnostics_export_filename(&report).is_err());

        let mut report = payload();
        report["events"] =
            serde_json::Value::Array(vec![serde_json::Value::String("x".repeat((8 << 20) + 1))]);
        assert!(frontend_diagnostics_export_filename(&report)
            .unwrap_err()
            .contains("8 MiB"));
    }

    #[test]
    fn export_writes_json_to_the_user_selected_path() {
        let directory = tempfile::tempdir().expect("temporary export directory");
        let path = directory.path().join("trace.json");
        let report = payload();
        write_frontend_diagnostics_export(&path, &report).expect("write selected report path");
        let saved: serde_json::Value =
            serde_json::from_slice(&fs::read(path).expect("read report"))
                .expect("parse exported report");
        assert_eq!(saved, report);
    }

    #[test]
    fn invalid_report_and_failed_publish_preserve_existing_destinations() {
        let directory = tempfile::tempdir().unwrap();
        let destination = directory.path().join("existing.json");
        fs::write(&destination, b"previous report").unwrap();
        let mut invalid = payload();
        invalid["schemaVersion"] = serde_json::Value::from(99);
        assert!(write_frontend_diagnostics_export(&destination, &invalid).is_err());
        assert_eq!(fs::read(&destination).unwrap(), b"previous report");

        let folder = directory.path().join("selected-folder");
        fs::create_dir(&folder).unwrap();
        fs::write(folder.join("keep.json"), b"must remain").unwrap();
        let error = write_frontend_diagnostics_export(&folder, &payload()).unwrap_err();
        assert_eq!(
            error,
            "cannot save diagnostics export; choose a writable file destination"
        );
        assert_eq!(fs::read(folder.join("keep.json")).unwrap(), b"must remain");
        assert_eq!(fs::read_dir(directory.path()).unwrap().count(), 2);

        let missing = directory.path().join("missing/report.json");
        assert!(write_frontend_diagnostics_export(&missing, &payload()).is_err());
        assert_eq!(fs::read_dir(directory.path()).unwrap().count(), 2);
        write_frontend_diagnostics_export(&destination, &payload()).unwrap();
        assert_eq!(
            serde_json::from_slice::<serde_json::Value>(&fs::read(destination).unwrap()).unwrap(),
            payload()
        );
    }

    #[cfg(unix)]
    #[test]
    fn exported_diagnostics_are_private_even_when_replacing_a_public_report() {
        use std::os::unix::fs::PermissionsExt;
        let directory = tempfile::tempdir().unwrap();
        let destination = directory.path().join("public.json");
        fs::write(&destination, b"previous public report").unwrap();
        fs::set_permissions(&destination, fs::Permissions::from_mode(0o644)).unwrap();
        write_frontend_diagnostics_export(&destination, &payload()).unwrap();
        assert_eq!(
            fs::metadata(destination).unwrap().permissions().mode() & 0o777,
            0o600
        );
    }

    #[cfg(unix)]
    #[test]
    fn export_replaces_selected_alias_without_modifying_other_file_names() {
        use std::os::unix::fs::symlink;
        let directory = tempfile::tempdir().unwrap();
        let original = directory.path().join("existing.json");
        fs::write(&original, b"original private report").unwrap();
        let selected = directory.path().join("selected.json");
        symlink(&original, &selected).unwrap();
        write_frontend_diagnostics_export(&selected, &payload()).unwrap();
        assert_eq!(fs::read(&original).unwrap(), b"original private report");
        assert!(fs::symlink_metadata(&selected).unwrap().is_file());

        let hardlink = directory.path().join("linked.json");
        fs::hard_link(&original, &hardlink).unwrap();
        write_frontend_diagnostics_export(&hardlink, &payload()).unwrap();
        assert_eq!(fs::read(&original).unwrap(), b"original private report");
    }
}

#[cfg(test)]
mod file_dialog_selection_tests {
    use super::*;

    #[test]
    fn selected_path_and_explicit_cancel_are_distinct_from_callback_failure() {
        let directory = tempfile::tempdir().unwrap();
        let path = directory.path().join("selected ' 中文.json");
        let (sender, receiver) = std::sync::mpsc::channel();
        sender
            .send(Some(tauri_plugin_dialog::FilePath::Path(path.clone())))
            .unwrap();
        drop(sender);
        assert_eq!(
            selected_theme_path(receive_file_dialog_selection(receiver).unwrap()).unwrap(),
            Some(path)
        );

        let (sender, receiver) = std::sync::mpsc::channel();
        sender.send(None).unwrap();
        drop(sender);
        assert!(receive_file_dialog_selection(receiver).unwrap().is_none());

        let (sender, receiver) = std::sync::mpsc::channel();
        drop(sender);
        assert_eq!(
            receive_file_dialog_selection(receiver).unwrap_err(),
            "file dialog closed unexpectedly; retry selecting a file"
        );
    }
}

#[cfg(test)]
mod theme_package_tests {
    use super::*;
    use std::collections::BTreeMap;

    #[test]
    fn exported_reasonix_theme_round_trips_tokens_and_recipes() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join("warm-night.reasonix-theme");
        let theme = host_preferences::UserTheme {
            id: "user-warm-night".into(),
            name: "Warm Night".into(),
            base_style: "carbon".into(),
            tokens: host_preferences::ThemeTokens {
                light: BTreeMap::from([("accent".into(), "#6a4614".into())]),
                dark: BTreeMap::from([("bg".into(), "#15120d".into())]),
            },
            density: "compact".into(),
            corners: "round".into(),
            ..UserTheme::default()
        };
        write_theme_package(&theme, directory.path(), path.clone()).expect("export theme package");
        assert_eq!(
            read_theme_package(&path).expect("import theme package"),
            theme
        );
    }

    #[test]
    fn import_round_trips_theme_background_image_assets() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join("with-image.reasonix-theme");
        let file = File::create(&path).expect("create archive");
        let mut archive = ZipWriter::new(file);
        let options = SimpleFileOptions::default().compression_method(CompressionMethod::Deflated);
        let mut image = vec![0; 24];
        image[..8].copy_from_slice(&[0x89, b'P', b'N', b'G', 0x0d, 0x0a, 0x1a, 0x0a]);
        image[12..16].copy_from_slice(b"IHDR");
        image[16..20].copy_from_slice(&1_u32.to_be_bytes());
        image[20..24].copy_from_slice(&1_u32.to_be_bytes());
        archive
            .start_file("theme.json", options)
            .expect("manifest entry");
        archive
            .write_all(
                br#"{"schemaVersion":2,"id":"warm","name":"Warm","baseStyle":"carbon","tokens":{},"recipes":{"density":"comfortable","corners":"soft"},"background":{"image":"wallpaper.png"}}"#,
            )
            .expect("manifest content");
        archive
            .start_file("wallpaper.png", options)
            .expect("image entry");
        archive.write_all(&image).expect("image bytes");
        archive.finish().expect("finish archive");

        let mut imported = read_theme_package(&path).expect("image archive imports");
        assert_eq!(
            imported.background.as_ref().unwrap().image,
            "background.png"
        );
        assert_eq!(imported.background_asset_bytes, Some(image));
        imported.id = "user-warm".into();
        let asset_root = directory.path().join("theme-assets");
        let theme_asset_dir = asset_root.join("user-warm");
        std::fs::create_dir_all(&theme_asset_dir).expect("create export asset directory");
        std::fs::write(
            theme_asset_dir.join("background.png"),
            imported.background_asset_bytes.as_ref().unwrap(),
        )
        .expect("write export image asset");
        let exported = directory.path().join("with-image-export.reasonix-theme");
        write_theme_package(&imported, &asset_root, exported.clone()).expect("export image theme");
        let round_trip = read_theme_package(&exported).expect("re-import image export");
        assert_eq!(round_trip.background, imported.background);
        assert_eq!(
            round_trip.background_asset_bytes,
            imported.background_asset_bytes
        );
    }

    #[test]
    fn theme_editor_image_data_urls_are_bounded_and_format_checked() {
        let mut image = vec![0; 24];
        image[..8].copy_from_slice(&[0x89, b'P', b'N', b'G', 0x0d, 0x0a, 0x1a, 0x0a]);
        image[12..16].copy_from_slice(b"IHDR");
        image[16..20].copy_from_slice(&1_u32.to_be_bytes());
        image[20..24].copy_from_slice(&1_u32.to_be_bytes());
        let data_url = format!("data:image/png;base64,{}", BASE64_STANDARD.encode(&image));
        let (extension, decoded) = decode_theme_image_data_url(&data_url).expect("decode PNG URL");
        assert_eq!(extension, "png");
        validate_theme_image("background.png", &decoded)
            .expect("validate PNG header and dimensions");
        assert!(decode_theme_image_data_url("data:image/svg+xml;base64,PHN2Zy8+").is_err());
        assert!(decode_theme_image_data_url("data:image/png;base64,not-base64!").is_err());
    }

    #[test]
    fn plugin_theme_ids_and_cached_assets_are_scoped() {
        assert!(valid_plugin_theme_pack_id("neon-night"));
        assert!(!valid_plugin_theme_pack_id("../escape"));
        assert!(!valid_plugin_theme_pack_id("Neon"));
        let directory = tempfile::tempdir().expect("temporary directory");
        let mut bytes = vec![0; 24];
        bytes[..8].copy_from_slice(&[0x89, b'P', b'N', b'G', 0x0d, 0x0a, 0x1a, 0x0a]);
        bytes[12..16].copy_from_slice(b"IHDR");
        bytes[16..20].copy_from_slice(&1_u32.to_be_bytes());
        bytes[20..24].copy_from_slice(&1_u32.to_be_bytes());
        let path = stage_plugin_theme_asset(
            directory.path(),
            "theme-plugin",
            "neon-night",
            "background",
            Some("background.png"),
            Some(bytes.as_slice()),
        )
        .expect("stage plugin theme asset")
        .expect("theme asset path");
        let path = PathBuf::from(path);
        assert!(path.starts_with(directory.path()));
        assert_eq!(fs::read(path).expect("read staged asset"), bytes);
        assert!(stage_plugin_theme_asset(
            directory.path(),
            "theme-plugin",
            "neon-night",
            "background",
            Some("../../escape.png"),
            Some(bytes.as_slice()),
        )
        .is_err());
    }

    #[test]
    #[cfg(unix)]
    fn plugin_theme_assets_reject_existing_symlink_directories_and_files() {
        use std::os::unix::fs::symlink;

        let directory = tempfile::tempdir().expect("temporary directory");
        let outside = tempfile::tempdir().expect("outside directory");
        let asset_root = directory.path().join("theme-assets").join("plugin-themes");
        fs::create_dir_all(asset_root.parent().expect("theme assets parent"))
            .expect("create theme assets parent");
        symlink(outside.path(), &asset_root).expect("link plugin theme assets outside Preview");
        let image = vec![
            0x89, b'P', b'N', b'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0, b'I', b'H', b'D', b'R', 0,
            0, 0, 1, 0, 0, 0, 1,
        ];
        assert!(stage_plugin_theme_asset(
            &asset_root,
            "plugin",
            "theme",
            "background",
            Some("background.png"),
            Some(&image),
        )
        .is_err());
        assert_eq!(
            fs::read_dir(outside.path())
                .expect("inspect outside")
                .count(),
            0
        );

        fs::remove_file(&asset_root).expect("remove directory link");
        let staged = stage_plugin_theme_asset(
            &asset_root,
            "plugin",
            "theme",
            "background",
            Some("background.png"),
            Some(&image),
        )
        .expect("stage regular asset")
        .expect("asset path");
        let outside_file = outside.path().join("background.png");
        fs::write(&outside_file, &image).expect("write outside image");
        fs::remove_file(&staged).expect("remove staged image");
        symlink(&outside_file, &staged).expect("link image outside Preview");
        assert!(stage_plugin_theme_asset(
            &asset_root,
            "plugin",
            "theme",
            "background",
            Some("background.png"),
            Some(&image),
        )
        .is_err());
    }

    #[test]
    #[cfg(unix)]
    fn plugin_theme_files_must_be_regular_files_inside_the_plugin_root() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let plugin_root = directory.path().join("plugin");
        let themes = plugin_root.join("themes");
        fs::create_dir_all(&themes).expect("create plugin themes directory");
        let regular = themes.join("theme.reasonix-theme");
        fs::write(&regular, b"theme").expect("write theme file");
        let escaped = directory.path().join("outside.reasonix-theme");
        fs::write(&escaped, b"outside").expect("write outside file");
        let link = themes.join("linked.reasonix-theme");
        std::os::unix::fs::symlink(&escaped, &link).expect("create theme symlink");
        let canonical_root = plugin_root.canonicalize().expect("canonical plugin root");
        assert!(canonical_plugin_theme_file(&canonical_root, &regular).is_some());
        assert!(canonical_plugin_theme_file(&canonical_root, &link).is_none());
        assert!(canonical_plugin_theme_file(&canonical_root, &escaped).is_none());
    }
}

fn apply_menu_zoom(app: &tauri::AppHandle, factor: f64) {
    let preferences = app.state::<HostPreferences>();
    let previous = preferences.zoom_factor();
    if let Err(error) = preferences.set_zoom_factor(factor) {
        eprintln!("Reasonix could not save WebView zoom: {error}");
        return;
    }
    let applied = preferences.zoom_factor();
    if let Some(window) = app.get_webview_window("main") {
        if let Err(error) = window.set_zoom(applied) {
            let _ = preferences.set_zoom_factor(previous);
            eprintln!("Reasonix could not apply WebView zoom: {error}");
        }
    }
}

fn setup_preview(app: &mut tauri::App) -> Result<(), Box<dyn std::error::Error>> {
    #[cfg(target_os = "macos")]
    app.manage(native_window_smoke::WindowSmokeState::default());
    #[cfg(target_os = "macos")]
    app.manage(native_clipboard_smoke::ClipboardSmokeState::default());
    #[cfg(target_os = "macos")]
    app.manage(native_link_smoke::LinkSmokeState::default());
    let profile = data_profile::configure_preview_profile(app).map_err(std::io::Error::other)?;
    #[cfg(target_os = "macos")]
    {
        let origin = ui_origin::PreviewUiOrigin::for_profile(profile.home())
            .map_err(std::io::Error::other)?;
        app.manage(origin);
    }
    #[cfg(target_os = "macos")]
    app.manage(initial_presentation::InitialPresentation::default());
    let mut main_config = app
        .config()
        .app
        .windows
        .iter()
        .find(|config| config.label == "main")
        .ok_or_else(|| std::io::Error::other("main window configuration missing"))?
        .clone();
    #[cfg(all(target_os = "macos", not(debug_assertions)))]
    {
        main_config.url = ui_origin::webview_url(app.handle()).map_err(std::io::Error::other)?;
    }
    // Keep the established dev-server URL on development builds.
    let _ = &mut main_config;
    let main_builder = tauri::WebviewWindowBuilder::from_config(app, &main_config)?;
    #[cfg(all(target_os = "macos", not(debug_assertions)))]
    let main_builder = {
        let origin = app.state::<ui_origin::PreviewUiOrigin>().inner().clone();
        main_builder.on_navigation(move |url| origin.matches(url))
    };
    #[cfg(target_os = "macos")]
    let main_builder = main_builder.visible(false).focused(false);
    main_builder.build()?;
    let window_state = PreviewWindowState::for_app(app)?;
    // Register before a queued native restore can complete or reenter setup.
    app.manage(window_state);
    let host_preferences = HostPreferences::for_app(app).map_err(std::io::Error::other)?;
    let workbench_catalog = WorkbenchCatalog::for_app(app).map_err(std::io::Error::other)?;
    let project_catalog = WorkbenchProjectCatalog::for_app(app).map_err(std::io::Error::other)?;
    if let Some(window) = app.get_webview_window("main") {
        let window_state = app.state::<PreviewWindowState>();
        window_state.restore(&window);
        window_state.capture(&window);
        #[cfg(target_os = "macos")]
        if std::env::var("REASONIX_TAURI_NATIVE_WINDOW_SMOKE").as_deref() == Ok("restore-normal") {
            window_state
                .verify_pending_capture()
                .map_err(std::io::Error::other)?;
            native_window_smoke::record(app.handle(), "restore-capture-fenced")
                .map_err(std::io::Error::other)?;
        }
        window
            .set_zoom(host_preferences.zoom_factor())
            .map_err(std::io::Error::other)?;
    }
    let supervisor = BridgeSupervisor::from_environment(app.handle().clone());

    #[cfg(target_os = "macos")]
    {
        let menu = menu::build_app_menu(app).map_err(|e| std::io::Error::other(e.to_string()))?;
        app.set_menu(menu)?;
    }

    if let Err(error) = tray::create_tray(app) {
        #[cfg(target_os = "linux")]
        eprintln!("Reasonix tray is unavailable; window close will quit: {error}");
        #[cfg(not(target_os = "linux"))]
        return Err(std::io::Error::other(error.to_string()).into());
    }

    // Initialize keychain store
    let keychain = keychain::KeychainStore::for_profile(&profile, app.handle());

    supervisor.start().map_err(std::io::Error::other)?;
    #[cfg(target_os = "macos")]
    {
        let appearance = native_appearance::NativeAppearance::default();
        if appearance.sync(app.handle(), &supervisor).is_err() {
            eprintln!("Reasonix could not restore the native appearance; retry appearance settings or restart Preview.");
        }
        app.manage(appearance);
    }
    // Normal startup reads profile credentials through the core; never trigger
    // a system keychain authorization prompt merely by opening Preview.
    let notifications =
        notifications::NotificationState::for_profile(&profile, &app.config().identifier);
    app.manage(std::sync::Arc::clone(&notifications));
    notifications::NotificationState::install(app.handle(), &notifications);
    app.manage(keychain);

    app.manage(supervisor);
    app.manage(profile);
    app.manage(host_preferences);
    app.manage(workbench_catalog);
    app.manage(project_catalog);
    app.manage(SessionShadowSnapshotCache::default());
    app.manage(pasted_images::PastedImages::default());
    Ok(())
}

fn main() {
    let app = tauri::Builder::default()
        .register_uri_scheme_protocol("reasonix-preview", |context, request| {
            #[cfg(target_os = "macos")]
            { ui_origin::asset_response(context.app_handle(), request) }
            #[cfg(not(target_os = "macos"))]
            {
                let _ = (context, request);
                tauri::http::Response::builder().status(403).body(Vec::<u8>::new()).unwrap()
            }
        })
        .on_page_load(|webview, payload| {
            #[cfg(target_os = "macos")]
            native_window_smoke::observe(webview, payload);
            #[cfg(target_os = "macos")]
            initial_presentation::observe(webview, payload);
            #[cfg(target_os = "macos")]
            native_clipboard_smoke::observe(webview, payload);
            #[cfg(target_os = "macos")]
            native_link_smoke::observe(webview, payload);
            #[cfg(not(target_os = "macos"))]
            let _ = (webview, payload);
        })
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_opener::Builder::new()
            .open_js_links_on_click(false)
            .build())
        .plugin(tauri_plugin_clipboard_manager::init())
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            // When a second instance is launched, focus the existing window
            tray::show_main_window(app);
        }))
        .plugin(tauri_plugin_updater::Builder::new().build())
        .setup(|app| {
            // Tauri executes setup inside its event-loop callback and panics
            // on a returned Err. Handle initialization refusals here, including
            // damaged profile identity, rather than aborting the application.
            setup_preview(app).unwrap_or_else(|error| {
                eprintln!("Reasonix could not start: {error}");
                std::process::exit(1);
            });
            Ok(())
        })
        .on_window_event(|window, event| {
            if window.label() != "main" { return; }
            if matches!(event, tauri::WindowEvent::Focused(true)) {
                if let (Some(state), Some(webview)) = (
                    window.app_handle().try_state::<PreviewWindowState>(),
                    window.app_handle().get_webview_window("main"),
                ) {
                    state.restore(&webview);
                    state.capture(&webview);
                }
            }
            if matches!(event, tauri::WindowEvent::Moved(_) | tauri::WindowEvent::Resized(_)) {
                if let (Some(state), Some(webview)) = (
                    window.app_handle().try_state::<PreviewWindowState>(),
                    window.app_handle().get_webview_window("main"),
                ) {
                    state.capture(&webview);
                }
            }
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                api.prevent_close();
                let behavior = window.app_handle().state::<HostPreferences>().close_behavior()
                    .effective_for_tray(std::env::consts::OS, window.app_handle().try_state::<tray::TrayMenuState>().is_some());
                match behavior {
                    CloseBehavior::KeepRunning => {
                        // Match Wails' macOS background close: hide the
                        // application so the shell's Dock/tray restore path
                        // unhides it together with the retained main window.
                        #[cfg(target_os = "macos")]
                        let _ = window.app_handle().hide();
                        #[cfg(not(target_os = "macos"))]
                        let _ = window.hide();
                    }
                    CloseBehavior::Quit => window.app_handle().exit(0),
                }
            }
            if matches!(event, tauri::WindowEvent::Destroyed) {
                if let Some(supervisor) = window.app_handle().try_state::<BridgeSupervisor>() {
                    supervisor.clear_remote_subscriptions();
                }
            }
        })
        .on_menu_event(|app, event| {
            match event.id().as_ref() {
                "quit" => {
                    // Cmd+Q: actually quit the application
                    app.exit(0);
                }
                "settings" => {
                    tray::show_main_window(app);
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.emit("host:open-settings", ());
                    }
                }
                "show_main_window" => {
                    tray::show_main_window(app);
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
                    if let Some(window) = app.get_webview_window("main") {
                        // Bind the sheet to the user's display and keep the native
                        // event loop responsive while the informational dialog is open.
                        dialog
                            .message(&msg)
                            .parent(&window)
                            .title("About Reasonix")
                            .kind(tauri_plugin_dialog::MessageDialogKind::Info)
                            .show(|_| {});
                    }
                }
                "check_updates" => {
                    let version = env!("CARGO_PKG_VERSION");
                    let msg = format!(
                        "Current version: {version}\n\nUpdate checks are not available in Tauri Preview yet.\nFor released versions and manual downloads, visit:\nhttps://github.com/esengine/DeepSeek-Reasonix/releases"
                    );
                    let dialog = tauri_plugin_dialog::DialogExt::dialog(app);
                    if let Some(window) = app.get_webview_window("main") {
                        // Bind the sheet to the user's display and keep the native
                        // event loop responsive while the informational dialog is open.
                        dialog
                            .message(&msg)
                            .parent(&window)
                            .title("Updates")
                            .kind(tauri_plugin_dialog::MessageDialogKind::Info)
                            .show(|_| {});
                    }
                }
                "reload" | "force_reload" => {
                    if let Some(window) = app.get_webview_window("main") {
                        // Native reload remains available when page JavaScript cannot run.
                        if window.reload().is_err() {
                            eprintln!("Reasonix could not reload the main page; quit and reopen Preview.");
                        }
                    }
                }
                "toggle_fullscreen" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.set_fullscreen(!window.is_fullscreen().unwrap_or(false));
                    }
                }
                "zoom_in" => {
                    let current = app.state::<HostPreferences>().zoom_factor();
                    apply_menu_zoom(app, (current + 0.1).min(2.0));
                }
                "zoom_out" => {
                    let current = app.state::<HostPreferences>().zoom_factor();
                    apply_menu_zoom(app, (current - 0.1).max(0.5));
                }
                "zoom_reset" => {
                    apply_menu_zoom(app, 1.0);
                }
                "minimize" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.minimize();
                    }
                }
                "hide" => {
                    #[cfg(target_os = "macos")]
                    let _ = app.hide();
                    #[cfg(not(target_os = "macos"))]
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.hide();
                    }
                }
                _ => {}
            }
        })
        .invoke_handler({
            let handler: fn(tauri::ipc::Invoke<tauri::Wry>) -> bool = tauri::generate_handler![
            bridge_status,
            restart_bridge,
            bridge_open_session,
            bridge_switch_session,
            bridge_set_session_model,
            bridge_terminal_workspace,
            bridge_terminal_create,
            bridge_terminal_output,
            bridge_terminal_input,
            bridge_terminal_resize,
            bridge_terminal_rename,
            bridge_terminal_close,
            bridge_session_approval_mode,
            bridge_rename_session,
            bridge_delete_session,
            bridge_session_archives,
            bridge_change_session_archive,
            bridge_pending_session_deletes_page,
            bridge_pending_session_title_recoveries,
            list_mcp_servers,
            mcp_runtime_action,
            clear_mcp_authentication,
            start_mcp_oauth,
            mcp_oauth_status,
            cancel_mcp_oauth,
            save_mcp_server,
            delete_mcp_server,
            set_mcp_server_enabled,
            search_mcp_marketplace,
            resolve_mcp_marketplace,
            bridge_session_snapshot,
            bridge_session_balance,
            bridge_session_history,
            bridge_session_previews,
            bridge_session_catalog_shadow,
            bridge_import_legacy_session_catalog,
            scan_unclaimed_workbench_sessions,
            import_unclaimed_workbench_sessions,
            bridge_submit,
            bridge_attach_file,
            stage_pasted_image,
            discard_pasted_image,
            bridge_workspace,
            bridge_workspace_file,
            bridge_workspace_image,
            bridge_workspace_changes,
            bridge_workspace_change_detail,
            bridge_workspace_file_revert_preview,
            bridge_workspace_file_revert_commit,
            bridge_workspace_file_revert_undo,
            bridge_workspace_checkpoints,
            bridge_code_rewind_preview,
            bridge_code_rewind_commit,
            bridge_conversation_rewind_preview,
            bridge_conversation_rewind_commit,
            bridge_conversation_rewind_undo,
            bridge_session_heads,
            bridge_session_head_switch,
            bridge_combined_rewind_preview,
            bridge_combined_rewind_commit,
            bridge_legacy_fork_preview,
            bridge_legacy_fork_commit,
            workspace_roots_availability,
            bridge_cancel,
            bridge_approve,
            bridge_answer_question,
            bridge_answer_mcp_interaction,
            bridge_replay_pending_prompts,
            bridge_start_events,
            preview_profile_status,
            legacy_ui_preferences,
            preview_runtime_info,
            provider_summary,
            save_provider_api_key,
            clear_provider_api_key,
            provider_configs,
            save_provider_config,
            delete_provider_config,
            discover_provider_models,
            preview_provider_reasoning,
            usage_stats,
            storage_settings,
            capability_diagnostics,
            runtime_doctor,
            remote_settings,
            scan_remote_ssh_config,
            change_remote_settings,
            connect_remote_host,
            disconnect_remote_host,
            browse_remote_host,
            remote_forwards,
            remote_serve,
            bridge_remote_controller_attach,
            bridge_remote_controller_sessions,
            bridge_remote_controller_close,
            bridge_remote_controller_session_view,
            bridge_remote_controller_session_image,
            bridge_remote_controller_subscribe,
            bridge_remote_controller_unsubscribe,
            open_remote_controller,
            preview_remote_file,
            save_remote_file,
            change_remote_path,
            permission_settings,
            change_permission_settings,
            secrets_settings,
            change_secrets_settings,
            sandbox_settings,
            change_sandbox_settings,
            network_settings,
            bot_runtime_status,
            restart_bot_runtime,
            bot_pairing,
            change_bot_pairing,
            bot_settings,
            change_bot_settings,
            change_network_settings,
            skills_settings,
            change_skills_settings,
            plan_skill_install,
            install_skill,
            archive_skill,
            restore_skill,
            plugin_settings,
            change_plugin_settings,
            plugin_doctor,
            plan_plugin_install,
            install_plugin,
            remove_plugin,
            subagent_settings,
            change_subagent_settings,
            try_subagent_profile,
            subagent_profile_try_status,
            cancel_subagent_profile_try,
            hooks_settings,
            change_hooks_settings,
            memory_settings,
            change_memory_settings,
            memory_suggestions,
            accept_memory_suggestion,
            desktop_preferences,
            set_desktop_approval,
            set_desktop_terminal_theme,
            set_desktop_appearance,
            set_desktop_language,
            set_tray_locale,
            set_desktop_currency,
            set_default_model,
            set_model_role,
            set_agent_preferences,
            test_provider_model,
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
            get_close_behavior,
            set_close_behavior,
            get_zoom_factor,
            set_zoom_factor,
            get_active_theme_id,
            set_active_theme_id,
            list_plugin_themes,
            list_user_themes,
            save_user_theme,
            delete_user_theme,
            import_user_theme,
            export_user_theme,
            export_frontend_diagnostics,
            open_external_url,
            open_external_link,
            local_paths::open_local_path,
            local_paths::reveal_local_path,
            local_paths::save_local_path_as,
            local_paths::local_path_openers,
            local_paths::set_preferred_external_opener,
            local_paths::workspace_external_openers,
            local_paths::open_workspace_external,
            local_paths::open_local_path_with,
            notifications::notification_permission,
            notifications::send_system_notification,
            notifications::pending_notification_clicks,
            notifications::acknowledge_notification_click,
            notifications::resolve_notification_click,
            keychain::keychain_save,
            keychain::keychain_import_legacy,
            keychain::keychain_delete
            ];
            move |invoke: tauri::ipc::Invoke<tauri::Wry>| {
                // Plugin commands keep their capability checks. Every custom
                // native/bridge command belongs to the trusted main window;
                // derive caller identity from Tauri, never renderer arguments.
                if invoke.message.webview_ref().label() != "main" {
                    invoke.resolver.reject("native application commands require the main window");
                    true
                } else {
                    handler(invoke)
                }
            }
        })
        .build(tauri::generate_context!())
        .unwrap_or_else(|error| {
            // App construction errors are ordinary startup failures.
            eprintln!("Reasonix could not start: {error}");
            std::process::exit(1);
        });
    // The locked Wry RequestExit implementation uses ControlFlow::Exit and
    // drops its requested code. Keep the first explicit exit request, run the
    // existing Exit cleanup, then report that status to the operating system.
    let requested_exit_code = std::sync::Arc::new(std::sync::OnceLock::<i32>::new());
    let reported_exit_code = std::sync::Arc::clone(&requested_exit_code);
    let runtime_exit_code = app.run_return(move |app, event| {
        #[cfg(target_os = "macos")]
        if matches!(event, tauri::RunEvent::ExitRequested { .. }) {
            initial_presentation::close(app);
        }
        if let tauri::RunEvent::ExitRequested {
            code: Some(code), ..
        } = &event
        {
            let _ = requested_exit_code.set(*code);
        }
        #[cfg(target_os = "macos")]
        if matches!(event, tauri::RunEvent::Reopen { .. }) {
            native_window_smoke::observe_reopen(app);
            tray::show_main_window(app);
        }
        #[cfg(target_os = "macos")]
        if matches!(event, tauri::RunEvent::Ready) {
            initial_presentation::runtime_ready(app);
            native_window_smoke::start_if_requested(app);
            native_profile_smoke::start_if_requested(app);
        }
        #[cfg(target_os = "macos")]
        if matches!(event, tauri::RunEvent::Ready)
            && std::env::var("REASONIX_TAURI_PACKAGE_SMOKE").as_deref() == Ok("1")
        {
            // The package smoke must exercise the ordinary setup and Exit path
            // without depending on accessibility permissions or UI scripting.
            let handle = app.clone();
            std::thread::spawn(move || {
                let native_status = handle
                    .state::<std::sync::Arc<notifications::NotificationState>>()
                    .permission(false);
                let marker = std::env::var_os("TMPDIR")
                    .map(std::path::PathBuf::from)
                    .map(|dir| dir.join("reasonix-native-notification-smoke.json"));
                let verified = native_status.and_then(|status| {
                    if matches!(
                        status.permission,
                        notifications::Permission::Unavailable | notifications::Permission::Unknown
                    ) {
                        return Err(
                            "packaged native notification authorization is unavailable".into()
                        );
                    }
                    let path = marker.ok_or("notification smoke marker directory unavailable")?;
                    let supervisor = handle.state::<BridgeSupervisor>();
                    let session = supervisor.open_session(OpenSessionRequest {
                        model_ref: None,
                        effort: None,
                        session_id: "tauri-package-workspace-smoke".into(),
                        workspace_root: None,
                    })?;
                    let target = supervisor.workspace_target(&session.id)?;
                    let expected = handle
                        .state::<PreviewProfile>()
                        .home()
                        .join("global-workspace")
                        .canonicalize()
                        .map_err(|_| "Global smoke workspace unavailable")?;
                    if session.workspace_root.is_some()
                        || std::path::Path::new(&target.workspace_root) != expected
                    {
                        return Err("Global smoke workspace escaped the current profile".into());
                    }
                    let target_bytes = serde_json::to_vec(&target)
                        .map_err(|_| "Global smoke status encoding failed")?;
                    std::fs::write(
                        path.with_file_name("reasonix-global-workspace-smoke.json"),
                        target_bytes,
                    )
                    .map_err(|_| "Global smoke status write failed")?;
                    let bytes = serde_json::to_vec(&status)
                        .map_err(|_| "notification smoke status encoding failed")?;
                    std::fs::write(path, bytes)
                        .map_err(|_| "notification smoke status write failed".to_string())
                });
                if verified.is_err() {
                    handle.exit(2);
                    return;
                }
                std::thread::sleep(std::time::Duration::from_secs(8));
                handle.exit(0);
            });
        }
        if matches!(event, tauri::RunEvent::Exit) {
            app.state::<pasted_images::PastedImages>().shutdown();
            #[cfg(target_os = "macos")]
            initial_presentation::close(app);
            #[cfg(target_os = "macos")]
            native_window_smoke::stop_window_observers();
            if let Some(state) = app.try_state::<std::sync::Arc<notifications::NotificationState>>()
            {
                state.shutdown();
            }
            if let Some(window) = app.get_webview_window("main") {
                let _ = app.state::<PreviewWindowState>().save(&window);
            }
            let _ = app.state::<BridgeSupervisor>().stop();
        }
    });
    std::process::exit(
        reported_exit_code
            .get()
            .copied()
            .unwrap_or(runtime_exit_code),
    );
}

#[cfg(test)]
mod external_url_tests {
    use super::{
        remote_serve_request, validated_external_link, validated_external_url,
        validated_remote_controller_url,
    };

    #[test]
    fn serve_route_consumes_renderer_action_before_strict_bridge_decode() {
        for action in ["status", "start", "stop", "logs"] {
            let (path, body) = remote_serve_request(serde_json::json!({
                "action": action, "name": "gpu", "workspace": "/srv/work"
            }))
            .unwrap();
            assert_eq!(path, format!("/v1/settings/remote/serve/{action}"));
            assert_eq!(
                body,
                serde_json::json!({"name": "gpu", "workspace": "/srv/work"})
            );
        }
        assert!(remote_serve_request(serde_json::json!({"action": "exec"})).is_err());
        assert!(remote_serve_request(serde_json::Value::Null).is_err());
    }

    #[test]
    fn accepts_web_links_and_rejects_credentials_and_local_files() {
        assert_eq!(
            validated_external_url("https://example.test/authorize?state=abc").unwrap(),
            "https://example.test/authorize?state=abc"
        );
        assert_eq!(
            validated_external_url("http://127.0.0.1:8023/callback").unwrap(),
            "http://127.0.0.1:8023/callback"
        );
        for value in [
            "javascript:alert(1)",
            "file:///tmp/private",
            "https://user:pass@example.test/private",
            "https://user@example.test/private",
            "//example.test/relative",
        ] {
            assert!(validated_external_url(value).is_err(), "accepted {value}");
        }
    }

    #[test]
    fn markdown_email_links_use_only_the_mail_handler() {
        for link in [
            "mailto:support@example.test",
            "mailto:one@example.test,two@example.test?cc=other@example.test&subject=Help&body=Line%201%0ALine%202",
            "mailto:?subject=Help",
            "https://example.test/中文?state=abc",
        ] {
            assert!(validated_external_link(link).is_ok(), "rejected {link}");
        }
        for link in [
            "mailto://example.test/path",
            "mailto:support@example.test?attach=file:///etc/passwd",
            "mailto:support@example.test?subject=Help%0ABcc:other@example.test",
            "mailto:support%0D%0ABcc:other@example.test",
            "mailto:support@example.test#fragment",
            "mailto:support@example.test?body=%00",
            "file:///tmp/private",
            "vscode://file/tmp/private",
            "javascript:alert(1)",
            "https://user:secret@example.test",
            "https://example.test/\npath",
        ] {
            assert!(validated_external_link(link).is_err(), "accepted {link}");
        }
        assert!(validated_external_url("mailto:support@example.test").is_err());
        assert!(
            validated_external_link(&format!("https://example.test/{}", "x".repeat(16 << 10)))
                .is_err()
        );
    }

    #[test]
    fn remote_controller_requires_exact_loopback_fragment_url() {
        let token = "a".repeat(64);
        assert_eq!(
            validated_remote_controller_url(&format!("http://127.0.0.1:40123/#token={token}"))
                .unwrap(),
            format!("http://127.0.0.1:40123/#token={token}")
        );
        for value in [
            "https://127.0.0.1:40123/?token=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            "http://localhost:40123/?token=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            "http://127.0.0.1:40123/other?token=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            "http://127.0.0.1:40123/?token=short",
            "http://127.0.0.1:40123/?token=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&next=x",
            "http://127.0.0.1:40123/?key=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            "http://127.0.0.1:40123/#token=short",
            "http://127.0.0.1:40123/#token=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&next=x",
        ] {
            assert!(validated_remote_controller_url(value).is_err(), "accepted {value}");
        }
    }
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
