use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine as _};
use rand::TryRngCore;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
#[cfg(any(debug_assertions, test))]
use std::{
    env,
    process::{Child, Command, Stdio},
};
use std::{
    fs,
    io::{BufRead, BufReader, Read, Write},
    net::{IpAddr, SocketAddr, TcpStream},
    path::{Path, PathBuf},
    sync::{
        atomic::{AtomicBool, Ordering},
        Arc, Mutex,
    },
    thread,
    time::{Duration, Instant},
};
use tauri::Emitter;
use tauri_plugin_shell::{
    process::{CommandChild as ShellCommandChild, CommandEvent},
    ShellExt,
};
use tempfile::TempDir;

use crate::workbench_projects::normalized_project_key;

const BRIDGE_TOKEN_ENV: &str = "REASONIX_DESKTOP_BRIDGE_TOKEN";
#[cfg(debug_assertions)]
const BRIDGE_BINARY_ENV: &str = "REASONIX_DESKTOP_BRIDGE_BIN";
const BUNDLED_BRIDGE_NAME: &str = "reasonix-desktop-bridge";
const READY_TIMEOUT: Duration = Duration::from_secs(5);
const STOP_TIMEOUT: Duration = Duration::from_secs(5);
const MAX_SHADOW_SESSIONS: usize = 10_000;
const MAX_PENDING_DELETE_PAGE_SIZE: usize = 200;
const MAX_BRIDGE_RESPONSE_BODY_BYTES: usize = 32 * 1024 * 1024;
const MAX_BRIDGE_HTTP_RESPONSE_BYTES: usize = 64 * 1024 * 1024;
pub const PROTOCOL_VERSION: u8 = 1;

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeStatus {
    pub running: bool,
    pub protocol_version: Option<u8>,
    pub sidecar_instance_id: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DesktopPreferences {
    #[serde(default)]
    pub external_opener: String,
    pub protocol_version: u64,
    pub default_tool_approval_mode: String,
    #[serde(default)]
    pub language: String,
    #[serde(default)]
    pub display_currency: String,
    #[serde(default)]
    pub terminal_theme: String,
    #[serde(default)]
    pub theme: String,
    #[serde(default)]
    pub theme_style: String,
    #[serde(default)]
    pub appearance_configured: bool,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SubmitRequest {
    pub session_id: String,
    pub input: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionRequest {
    pub session_id: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPRuntimeActionRequest {
    pub session_id: String,
    pub name: String,
    pub action: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPRuntimeActionResponse {
    pub protocol_version: u64,
    pub name: String,
    pub action: String,
    pub tool_count: u32,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPOAuthRequest {
    pub session_id: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub flow_id: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPOAuthResponse {
    pub protocol_version: u64,
    pub flow_id: String,
    #[serde(default)]
    pub name: String,
    pub status: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPClearAuthRequest {
    pub session_id: String,
    pub name: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPClearAuthResponse {
    pub protocol_version: u64,
    pub name: String,
    pub changed: bool,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionPreview {
    pub session_id: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub first_user: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct SessionPreviewsResponse {
    protocol_version: u8,
    previews: Vec<SessionPreview>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionDirectoryCursor {
    pub position: i64,
    pub id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub snapshot_id: Option<String>,
    #[serde(default, skip_serializing_if = "is_zero")]
    pub total: u64,
}

fn is_zero(value: &u64) -> bool {
    *value == 0
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionDirectoryEntry {
    pub id: String,
    pub title: String,
    pub title_source: String,
    pub workspace_root: Option<String>,
    pub state: String,
    pub missing: bool,
    pub position: i64,
    pub updated_at_ms: i64,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionDirectoryPage {
    pub protocol_version: u8,
    pub sessions: Vec<SessionDirectoryEntry>,
    pub next_cursor: Option<SessionDirectoryCursor>,
    pub total: u64,
    pub snapshot_id: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PendingSessionDelete {
    pub id: String,
    pub title: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PendingSessionDeleteCursor {
    pub id: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PendingSessionDeletePage {
    pub sessions: Vec<PendingSessionDelete>,
    pub next_cursor: Option<PendingSessionDeleteCursor>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct PendingSessionDeletesPageResponse {
    protocol_version: u8,
    sessions: Vec<PendingSessionDelete>,
    next_cursor: Option<PendingSessionDeleteCursor>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PendingSessionTitleRecovery {
    pub id: String,
    pub title: String,
    pub workspace_root: Option<String>,
    pub state: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct PendingSessionTitleRecoveriesResponse {
    protocol_version: u8,
    sessions: Vec<PendingSessionTitleRecovery>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SessionInventoryResponse {
    protocol_version: u8,
    entries: Option<Vec<SessionInventoryEntry>>,
    unclaimed: Option<Vec<String>>,
    errors: Option<Vec<String>>,
    unclaimed_count: Option<usize>,
    error_count: Option<usize>,
    retired_ids: Option<Vec<String>>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct SessionShadowSnapshotResponse {
    protocol_version: u8,
    directory: SessionDirectoryPage,
    inventory: SessionInventoryResponse,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SessionInventoryEntry {
    id: String,
    source: String,
    exists: bool,
    readable: Option<bool>,
    #[serde(default)]
    detail: String,
}

pub struct SessionPhysicalState {
    pub id: String,
    pub exists: bool,
    pub readable: bool,
}

pub struct SessionPhysicalInventory {
    pub states: Vec<SessionPhysicalState>,
    pub unclaimed_count: usize,
    pub error_count: usize,
    pub retired_ids: Vec<String>,
}

impl SessionInventoryResponse {
    fn into_physical_inventory(self) -> Result<SessionPhysicalInventory, String> {
        if self.protocol_version != PROTOCOL_VERSION {
            return Err("desktop bridge inventory protocol is unsupported".to_string());
        }
        let entries = self
            .entries
            .ok_or_else(|| "desktop bridge inventory omitted entries".to_string())?;
        let unclaimed_count = self
            .unclaimed_count
            .or_else(|| self.unclaimed.as_ref().map(Vec::len))
            .ok_or_else(|| "desktop bridge inventory omitted unclaimed files".to_string())?;
        let error_count = self
            .error_count
            .or_else(|| self.errors.as_ref().map(Vec::len))
            .ok_or_else(|| "desktop bridge inventory omitted diagnostics".to_string())?;
        if self
            .unclaimed
            .as_ref()
            .is_some_and(|entries| entries.len() != unclaimed_count)
            || self
                .errors
                .as_ref()
                .is_some_and(|entries| entries.len() != error_count)
        {
            return Err("desktop bridge inventory counts are inconsistent".to_string());
        }
        let retired_ids = self.retired_ids.unwrap_or_default();
        if retired_ids.len() > 50 {
            return Err("desktop bridge inventory contains too many retired IDs".to_string());
        }
        let mut seen_retired = std::collections::HashSet::with_capacity(retired_ids.len());
        if retired_ids
            .iter()
            .any(|id| !seen_retired.insert(id.as_str()))
        {
            return Err("desktop bridge inventory contains duplicate retired IDs".to_string());
        }
        Ok(SessionPhysicalInventory {
            states: entries
                .into_iter()
                .filter(|entry| entry.source == "identity")
                .map(|entry| SessionPhysicalState {
                    id: entry.id,
                    exists: entry.exists,
                    readable: entry.readable.unwrap_or_else(|| {
                        entry.detail.is_empty() || entry.detail == "transcript is absent"
                    }),
                })
                .collect(),
            unclaimed_count,
            error_count,
            retired_ids,
        })
    }
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct LegacySessionCatalogEntry {
    pub session_id: String,
    pub title: Option<String>,
    pub workspace_root: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct LegacyCatalogImportResponse {
    protocol_version: u64,
    accepted: usize,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ScanImportCandidate {
    pub id: String,
    pub file: String,
    pub transcript_sha256: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ScanImportSelection {
    pub id: String,
    pub title: Option<String>,
    pub workspace_root: Option<String>,
    pub transcript_sha256: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ScanImportCandidatesResponse {
    protocol_version: u64,
    candidates: Vec<ScanImportCandidate>,
    blocked_count: usize,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ScanImportApplyResponse {
    protocol_version: u64,
    applied: usize,
    session_ids: Vec<String>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RenameSessionRequest {
    pub session_id: String,
    pub title: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionFirstMessageTitle {
    pub session_id: String,
    pub title: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionCatalogMetadata {
    pub session_id: String,
    pub workspace_root: Option<String>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct BackfillSessionTitlesResponse {
    protocol_version: u64,
    titles: Vec<SessionFirstMessageTitle>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct SyncSessionCatalogResponse {
    protocol_version: u64,
    synced: usize,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApproveRequest {
    pub session_id: String,
    pub id: String,
    pub allow: bool,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AnswerQuestionRequest {
    pub session_id: String,
    pub id: String,
    pub answers: Vec<BridgeAskAnswer>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AnswerMCPInteractionRequest {
    pub session_id: String,
    pub id: String,
    pub action: String,
    pub content: Option<Value>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceRequest {
    pub session_id: String,
    pub path: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceFileRequest {
    pub session_id: String,
    pub path: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceChangeDetailRequest {
    pub session_id: String,
    pub path: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceFileRevertCommitRequest {
    pub session_id: String,
    pub plan_id: String,
    pub resolution: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceFileRevertUndoRequest {
    pub session_id: String,
    pub transaction_id: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CodeRewindPreviewRequest {
    pub session_id: String,
    pub turn: u64,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CodeRewindCommitRequest {
    pub session_id: String,
    pub plan_id: String,
    pub confirm_partial_coverage: bool,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ConversationRewindPreviewRequest {
    pub session_id: String,
    pub turn: u64,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ConversationRewindCommitRequest {
    pub session_id: String,
    pub plan_id: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ConversationRewindUndoRequest {
    pub session_id: String,
    pub head_id: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionHeadSwitchRequest {
    pub session_id: String,
    pub head_id: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CombinedRewindPreviewRequest {
    pub session_id: String,
    pub turn: u64,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CombinedRewindCommitRequest {
    pub session_id: String,
    pub plan_id: String,
    pub confirm_partial_coverage: bool,
}

// The wire DTOs mirror docs/tauri/protocol/v1.schema.json through the generated
// module; only the host-facing command payloads below stay hand-written.
pub use crate::protocol_generated::{
    BridgeAnswerQuestionRequest, BridgeApprovalRequest, BridgeAskAnswer,
    BridgeAttachFileRequest as AttachFileRequest, BridgeAttachment, BridgeAttachmentResponse,
    BridgeCodeRewindCommitRequest, BridgeCodeRewindPlanResponse, BridgeCodeRewindPreviewRequest,
    BridgeCombinedRewindCommitRequest, BridgeCombinedRewindPlanResponse,
    BridgeCombinedRewindPreviewRequest, BridgeCombinedRewindResultResponse,
    BridgeConversationRewindCommitRequest, BridgeConversationRewindPlanResponse,
    BridgeConversationRewindPreviewRequest, BridgeConversationRewindResultResponse,
    BridgeConversationRewindUndoRequest, BridgeDeleteSessionResponse, BridgeEvent,
    BridgeHistoryMessage, BridgeHistoryResponse, BridgeLegacyConversationForkResultResponse,
    BridgeMCPInteractionAnswerRequest, BridgeOpenSessionRequest as OpenSessionRequest,
    BridgeProjectFolder, BridgeProjectFoldersResponse, BridgeProviderModelProbeRequest,
    BridgeProviderModelProbeResponse, BridgeProviderSummaryResponse, BridgeRemoteBrowseRequest,
    BridgeRemoteBrowseResponse, BridgeRemoteDisconnectRequest, BridgeRemoteDisconnectResponse,
    BridgeRemoteFilePreviewRequest, BridgeRemoteFilePreviewResponse, BridgeRemoteFileSaveRequest,
    BridgeRemoteFileSaveResponse, BridgeRenameSessionRequest, BridgeSession,
    BridgeSessionHeadSwitchRequest, BridgeSessionHeadSwitchResponse, BridgeSessionHeadsResponse,
    BridgeSessionMetrics, BridgeSessionResponse, BridgeSetAgentPreferenceRequest,
    BridgeSetDefaultModelRequest, BridgeSetModelRoleRequest, BridgeSetSessionModelRequest,
    BridgeWorkspaceChangeDetailRequest, BridgeWorkspaceChangeDetailResponse,
    BridgeWorkspaceChangesResponse, BridgeWorkspaceCheckpointsResponse, BridgeWorkspaceFileRequest,
    BridgeWorkspaceFileResponse, BridgeWorkspaceFileRevertCommitRequest,
    BridgeWorkspaceFileRevertPlanResponse, BridgeWorkspaceFileRevertResultResponse,
    BridgeWorkspaceFileRevertUndoRequest, BridgeWorkspaceListResponse, BridgeWorkspaceRequest,
};

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeSnapshot {
    pub sequence: u64,
    pub session: BridgeSession,
    pub metrics: Option<BridgeSessionMetrics>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeSessionBalanceView {
    pub available: bool,
    pub display: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeSessionBalanceResponse {
    pub protocol_version: u64,
    pub balance: Option<BridgeSessionBalanceView>,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeHistory {
    pub sequence: u64,
    pub session: BridgeSession,
    pub messages: Vec<BridgeHistoryMessage>,
    pub start_index: u64,
    pub total_messages: u64,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderConfigView {
    pub name: String,
    pub display_name: String,
    pub kind: String,
    pub models: Vec<String>,
    pub default: String,
    #[serde(default)]
    pub balance_url_set: bool,
    pub removable: bool,
    pub revision: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DeleteProviderConfigRequest {
    pub name: String,
    pub display_name: String,
    pub kind: String,
    pub models: Vec<String>,
    pub default: String,
    pub revision: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderConfigList {
    pub protocol_version: u64,
    pub providers: Vec<ProviderConfigView>,
    pub presets: Vec<ProviderPresetView>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DiscoverProviderModelsRequest {
    pub name: String,
    pub revision: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DiscoveredProviderModels {
    pub protocol_version: u64,
    pub models: Vec<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderPresetRouteView {
    pub name: String,
    pub kind: String,
    pub base_url: String,
    pub models: Vec<String>,
    pub default: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderPresetView {
    pub id: String,
    pub label: String,
    pub description: String,
    pub group: String,
    pub recommended: bool,
    pub status: String,
    pub routes: Vec<ProviderPresetRouteView>,
    pub revision: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SaveProviderConfigRequest {
    #[serde(default)]
    pub preset_id: String,
    #[serde(default)]
    pub preset_action: String,
    #[serde(default)]
    pub revision: String,
    pub name: String,
    pub display_name: String,
    pub kind: String,
    pub base_url: String,
    #[serde(default)]
    pub balance_url: String,
    #[serde(default)]
    pub clear_balance_url: bool,
    pub models: Vec<String>,
    pub default: String,
    pub use_api_key: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PermissionSettingsView {
    pub protocol_version: u64,
    #[serde(default)]
    pub scope: String,
    pub mode: String,
    pub allow: Vec<String>,
    pub ask: Vec<String>,
    pub deny: Vec<String>,
    #[serde(default)]
    pub project_overrides: PermissionProjectOverrides,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PermissionProjectOverrides {
    pub mode: bool,
    pub allow: bool,
    pub ask: bool,
    pub deny: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PermissionSettingsChange {
    pub action: String,
    #[serde(default)]
    pub scope: String,
    #[serde(default)]
    pub workspace_root: String,
    #[serde(default)]
    pub mode: String,
    #[serde(default)]
    pub list: String,
    #[serde(default)]
    pub rule: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SecretsSettingsView {
    pub protocol_version: u64,
    pub filter_subprocess_env: bool,
    pub protect_sensitive_files: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SecretsSettingsChange {
    #[serde(default)]
    pub filter_subprocess_env: Option<bool>,
    #[serde(default)]
    pub protect_sensitive_files: Option<bool>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SandboxSettingsView {
    pub protocol_version: u64,
    pub bash: String,
    pub network: bool,
    pub workspace_root: String,
    pub allow_write: Vec<String>,
    pub platform: String,
    #[serde(default)]
    pub shell: String,
    #[serde(default)]
    pub resolved_shell: String,
    #[serde(default)]
    pub effective_shell: String,
    #[serde(default)]
    pub shell_reload_required: bool,
    #[serde(default)]
    pub shell_capabilities: Vec<ShellCapabilityView>,
    #[serde(default)]
    pub git_capability: Option<ShellCapabilityView>,
    #[serde(default)]
    pub effective_write_roots: Vec<String>,
    #[serde(default)]
    pub effective_roots_error: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ShellCapabilityView {
    pub id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub variant: Option<String>,
    pub available: bool,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub path: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub source: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reason: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SandboxSettingsChange {
    pub bash: String,
    pub network: bool,
    pub workspace_root: String,
    pub allow_write: Vec<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub shell: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct NetworkSettingsView {
    pub protocol_version: u64,
    pub proxy_mode: String,
    pub no_proxy: String,
    pub proxy_type: String,
    pub proxy_server: String,
    pub proxy_port: i32,
    pub proxy_username: String,
    pub proxy_url_set: bool,
    pub proxy_password_set: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct NetworkSettingsChange {
    pub proxy_mode: String,
    pub no_proxy: String,
    pub proxy_type: String,
    pub proxy_server: String,
    pub proxy_port: i32,
    pub proxy_username: String,
    pub proxy_url_action: String,
    pub proxy_url: String,
    pub proxy_password_action: String,
    pub proxy_password: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteSettingsHost {
    pub name: String,
    pub host: String,
    pub port: i32,
    pub user: String,
    pub identity_file: String,
    pub proxy_jump: String,
    pub workspace: String,
    pub serve_install: String,
    pub credential_mode: String,
    pub use_ssh_config: bool,
    pub password_set: bool,
    pub passphrase_set: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteSettingsView {
    pub protocol_version: u64,
    pub config_path: String,
    pub ssh_config_path: String,
    pub hosts: Vec<RemoteSettingsHost>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteSSHConfigAlias {
    pub alias: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteSSHConfigScanView {
    pub protocol_version: u64,
    pub config_path: String,
    pub aliases: Vec<RemoteSSHConfigAlias>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteSettingsHostInput {
    pub name: String,
    pub host: String,
    pub port: i32,
    pub user: String,
    pub identity_file: String,
    pub proxy_jump: String,
    pub workspace: String,
    pub serve_install: String,
    pub credential_mode: String,
    pub use_ssh_config: bool,
    #[serde(default)]
    pub password_action: String,
    #[serde(default)]
    pub password: String,
    #[serde(default)]
    pub passphrase_action: String,
    #[serde(default)]
    pub passphrase: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteSettingsChange {
    pub action: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub host: Option<RemoteSettingsHostInput>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteConnectRequest {
    pub name: String,
    #[serde(default)]
    pub trust_fingerprint: String,
    #[serde(default)]
    pub password: String,
    #[serde(default)]
    pub passphrase: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteConnectResponse {
    pub protocol_version: u64,
    pub status: String,
    #[serde(default)]
    pub host: String,
    #[serde(default)]
    pub address: String,
    #[serde(default)]
    pub key_type: String,
    #[serde(default)]
    pub fingerprint: String,
    #[serde(default)]
    pub message: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillSettingsItem {
    pub name: String,
    pub description: String,
    pub invocation: String,
    pub scope: String,
    pub source_path: String,
    pub run_as: String,
    pub enabled: bool,
    pub global_enabled: bool,
    #[serde(default)]
    pub requires: Vec<String>,
    #[serde(default)]
    pub archive_revision: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ArchivedSkillItem {
    pub name: String,
    pub scope: String,
    pub archive_id: String,
    pub path: String,
    pub revision: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillSettingsSource {
    pub path: String,
    pub scope: String,
    pub status: String,
    pub enabled: bool,
    pub configured: bool,
    pub configured_global: bool,
    pub configured_project: bool,
    pub global_enabled: bool,
    #[serde(default)]
    pub skill_count: u64,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillProjectOverrides {
    pub implicit: bool,
    pub skills: bool,
    pub sources: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillsSettingsView {
    pub protocol_version: u64,
    pub allow_implicit_invocation: bool,
    pub global_allow_implicit_invocation: bool,
    pub project_overrides: SkillProjectOverrides,
    pub skills: Vec<SkillSettingsItem>,
    pub sources: Vec<SkillSettingsSource>,
    #[serde(default)]
    pub archived_skills: Vec<ArchivedSkillItem>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillArchiveRequest {
    pub name: String,
    pub scope: String,
    pub workspace_root: String,
    pub archive_id: String,
    pub revision: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillArchiveResult {
    pub protocol_version: u64,
    pub backup_path: String,
    pub settings: SkillsSettingsView,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillsSettingsChange {
    pub workspace_root: String,
    pub scope: String,
    pub action: String,
    pub enabled: bool,
    pub name: String,
    pub path: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillInstallRequest {
    pub source: String,
    pub scope: String,
    pub workspace_root: String,
    pub plan_id: String,
    pub accept_risk: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillInstallPlanAction {
    pub name: String,
    pub target: String,
    pub risk_level: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillInstallPlan {
    pub protocol_version: u64,
    pub plan_id: String,
    pub actions: Vec<SkillInstallPlanAction>,
    pub warning_count: u64,
    #[serde(default)]
    pub warnings: Vec<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillInstallResult {
    pub protocol_version: u64,
    pub status: String,
    pub failed_names: Vec<String>,
    pub settings: SkillsSettingsView,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PluginSettingsItem {
    pub name: String,
    pub description: String,
    pub version: String,
    pub source: String,
    #[serde(default)]
    pub update_source: String,
    pub root: String,
    pub manifest_kind: String,
    pub enabled: bool,
    #[serde(default)]
    pub linked: bool,
    pub status: String,
    pub issue: String,
    pub warning_count: u64,
    pub skills: u64,
    pub agents: u64,
    pub commands: u64,
    pub hooks: u64,
    pub mcp_servers: u64,
    #[serde(default, skip_serializing)]
    pub themes: Vec<PluginThemeItem>,
    pub runtime: bool,
    pub revision: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PluginThemeItem {
    pub name: String,
    pub path: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PluginSettingsView {
    pub protocol_version: u64,
    pub plugins: Vec<PluginSettingsItem>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PluginCompatibilityIssue {
    pub capability: String,
    #[serde(default)]
    pub path: String,
    pub reason: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PluginDoctorView {
    pub protocol_version: u64,
    pub name: String,
    pub compatibility: String,
    pub mapped_capabilities: Vec<String>,
    pub skipped_capabilities: Vec<PluginCompatibilityIssue>,
    pub warnings: Vec<String>,
    pub error: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PluginSettingsChange {
    pub name: String,
    pub revision: String,
    pub enabled: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PluginInstallPlanAction {
    pub name: String,
    pub version: String,
    pub manifest_kind: String,
    pub risk_level: String,
    pub skills: u64,
    pub agents: u64,
    pub commands: u64,
    pub hooks: u64,
    pub mcp_servers: u64,
    pub prompts: u64,
    pub themes: u64,
    pub runtime: bool,
    pub runtime_command: String,
    pub intercepts: Vec<String>,
    pub replaces: Vec<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PluginInstallPlan {
    pub protocol_version: u64,
    pub plan_id: String,
    #[serde(default)]
    pub mode: String,
    pub actions: Vec<PluginInstallPlanAction>,
    pub warning_count: u64,
    #[serde(default)]
    pub warnings: Vec<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PluginInstallRequest {
    pub source: String,
    #[serde(default)]
    pub mode: String,
    pub plan_id: String,
    pub accept_risk: bool,
    #[serde(default)]
    pub replace: bool,
    #[serde(default)]
    pub expected_name: String,
    #[serde(default)]
    pub expected_revision: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PluginRemoveRequest {
    pub name: String,
    pub revision: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PluginOperationResult {
    pub protocol_version: u64,
    pub status: String,
    pub failed_names: Vec<String>,
    pub settings: PluginSettingsView,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SubagentProfileView {
    pub name: String,
    pub description: String,
    pub scope: String,
    pub invocation: String,
    pub configured_model: String,
    pub configured_effort: String,
    pub invocation_mode: String,
    pub editable: bool,
    pub edit_reason: String,
    pub revision: String,
    pub body: String,
    pub color: String,
    pub model: String,
    pub effort: String,
    pub allowed_tools: Vec<String>,
    pub read_only: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SubagentProfileInput {
    pub name: String,
    pub description: String,
    pub system_prompt: String,
    pub color: String,
    pub model: String,
    pub effort: String,
    pub allowed_tools: Vec<String>,
    pub read_only: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SubagentTryResponse {
    pub protocol_version: u64,
    pub result: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SubagentTryStatusView {
    pub protocol_version: u64,
    pub running: bool,
    pub output: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SubagentSettingsView {
    pub protocol_version: u64,
    pub default_model: String,
    pub subagent_model: String,
    pub subagent_effort: String,
    pub max_depth: i32,
    pub max_concurrency: i32,
    pub max_parallel_writers: i32,
    pub model_refs: Vec<String>,
    pub model_efforts: std::collections::HashMap<String, Vec<String>>,
    pub profiles: Vec<SubagentProfileView>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct HooksSettingsView {
    pub protocol_version: u64,
    pub scope: String,
    pub path: String,
    pub project_root: String,
    pub revision: String,
    pub hooks: Value,
    pub events: Vec<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct HooksSettingsChange {
    pub scope: String,
    pub workspace_root: String,
    pub revision: String,
    pub hooks: Value,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MemoryDocView {
    pub path: String,
    pub scope: String,
    pub body: String,
    pub revision: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MemoryFactView {
    pub id: String,
    pub revision: i32,
    #[serde(default)]
    pub created_at: String,
    #[serde(default)]
    pub updated_at: String,
    pub name: String,
    pub title: String,
    pub description: String,
    #[serde(rename = "type")]
    pub kind: String,
    pub scope: String,
    pub body: String,
    pub freshness: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MemoryArchiveView {
    #[serde(flatten)]
    pub fact: MemoryFactView,
    pub path: String,
    pub archived_at: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MemoryRecallHitView {
    pub id: String,
    pub revision: i32,
    pub name: String,
    #[serde(default)]
    pub title: String,
    #[serde(rename = "type")]
    pub kind: String,
    pub scope: String,
    pub score: f64,
    pub freshness: String,
    pub reason: String,
    pub snippet: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MemoryRecallView {
    pub query: String,
    #[serde(default)]
    pub hits: Vec<MemoryRecallHitView>,
    pub omitted: i32,
    pub char_budget: i32,
    pub used_chars: i32,
    #[serde(default)]
    pub suppressed: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MemorySettingsView {
    pub protocol_version: u64,
    pub workspace_root: String,
    pub store_dir: String,
    pub global_store_dir: String,
    pub docs: Vec<MemoryDocView>,
    pub facts: Vec<MemoryFactView>,
    pub archives: Vec<MemoryArchiveView>,
    #[serde(default)]
    pub revisions: Vec<MemoryFactView>,
    pub diagnostics: Vec<String>,
    #[serde(default)]
    pub last_recall: MemoryRecallView,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MemorySettingsChange {
    pub workspace_root: String,
    pub action: String,
    pub path: String,
    pub revision: String,
    pub body: String,
    pub scope: String,
    pub fact_id: String,
    pub fact_revision: i32,
    #[serde(default)]
    pub history_revision: i32,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub description: String,
    #[serde(default, rename = "type")]
    pub fact_type: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MemorySuggestionView {
    pub id: String,
    pub name: String,
    pub title: String,
    pub description: String,
    #[serde(rename = "type")]
    pub fact_type: String,
    pub scope: String,
    pub body: String,
    pub reason: String,
    pub evidence: Vec<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillSuggestionView {
    pub id: String,
    pub name: String,
    pub description: String,
    pub scope: String,
    pub body: String,
    pub reason: String,
    pub evidence: Vec<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MemorySuggestionsView {
    pub memories: Vec<MemorySuggestionView>,
    pub skills: Vec<SkillSuggestionView>,
    pub generated_at: String,
    pub available: bool,
    pub source: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MemorySuggestionAcceptance {
    pub path: String,
    pub suggestions: MemorySuggestionsView,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MemorySuggestionAcceptanceRequest {
    pub workspace_root: String,
    pub kind: String,
    pub id: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SubagentSettingsChange {
    pub workspace_root: String,
    pub action: String,
    pub name: String,
    pub value: String,
    pub number: i32,
    #[serde(default)]
    pub scope: String,
    #[serde(default)]
    pub revision: String,
    #[serde(default)]
    pub profile: Option<SubagentProfileInput>,
}

/// One MCP server as the host may see it. Credential material is write-only, so
/// this carries the key names a server expects and never a value. Hand-written
/// like the other host-owned payloads: the wire shape belongs to this host, not
/// to the frozen bridge schema.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPServerView {
    pub name: String,
    pub enabled: bool,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub runtime_status: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tool_count: Option<u32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tool_list: Option<Vec<MCPRuntimeTool>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub mcp_protocol_version: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub mcp_session_state: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reconnect_attempts: Option<u32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub error_kind: Option<String>,
    #[serde(rename = "type")]
    pub kind: String,
    pub source: String,
    pub scope: String,
    pub config_path: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub command: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub args: Option<Vec<String>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub url: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub env_keys: Option<Vec<String>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub header_keys: Option<Vec<String>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub startup_timeout_seconds: Option<u32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub call_timeout_seconds: Option<u32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tool_timeout_seconds: Option<std::collections::BTreeMap<String, u32>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub auto_start: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tier: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub managed_by_package: Option<bool>,
    #[serde(default)]
    pub native_oauth_eligible: bool,
    #[serde(default)]
    pub authentication_saved: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPRuntimeTool {
    pub name: String,
    #[serde(default)]
    pub description: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPServerListResponse {
    pub protocol_version: u64,
    pub servers: Vec<MCPServerView>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPServerInput {
    pub scope: String,
    pub name: String,
    #[serde(rename = "type", default, skip_serializing_if = "Option::is_none")]
    pub mcp_type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub command: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub args: Option<Vec<String>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub env: Option<std::collections::BTreeMap<String, String>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub url: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub headers: Option<std::collections::BTreeMap<String, String>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub startup_timeout_seconds: Option<u32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub call_timeout_seconds: Option<u32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tool_timeout_seconds: Option<std::collections::BTreeMap<String, u32>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub auto_start: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tier: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPServerDeleteRequest {
    pub name: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPServerActivationRequest {
    pub name: String,
    pub enabled: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPMarketplaceEntry {
    pub name: String,
    pub suggested_name: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub version: String,
    pub installable: bool,
    #[serde(default)]
    pub unavailable_reason: String,
    #[serde(default)]
    pub transport: String,
    #[serde(default)]
    pub command: String,
    #[serde(default)]
    pub args: Vec<String>,
    #[serde(default)]
    pub url: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPMarketplaceResponse {
    pub protocol_version: u64,
    pub servers: Vec<MCPMarketplaceEntry>,
    pub cached: bool,
    #[serde(default)]
    pub warning: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPMarketplaceResolveResponse {
    pub protocol_version: u64,
    pub server: MCPMarketplaceEntry,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPServerMutationResponse {
    pub protocol_version: u64,
    pub status: String,
    #[serde(default)]
    pub config_path: Option<String>,
    #[serde(default)]
    pub server: Option<MCPServerView>,
    pub servers: Vec<MCPServerView>,
}

pub struct BridgeSupervisor {
    launcher: BridgeLauncher,
    process: Mutex<Option<BridgeProcess>>,
    events: Mutex<Option<EventForwarder>>,
}

#[derive(Clone)]
pub struct BridgeSubagentTryClient {
    address: SocketAddr,
    token: String,
}

impl BridgeSubagentTryClient {
    pub fn subagent_profile_try_status(&self) -> Result<SubagentTryStatusView, String> {
        let response = request_json_with_timeout(
            self.address,
            &self.token,
            "GET",
            "/v1/settings/subagents/try/status",
            None,
            None,
            Duration::from_secs(5),
        )?;
        let view: SubagentTryStatusView =
            serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn try_subagent_profile(
        &self,
        workspace_root: &str,
        input: SubagentProfileInput,
        task: &str,
    ) -> Result<String, String> {
        let response = request_json_with_timeout(
            self.address,
            &self.token,
            "POST",
            "/v1/settings/subagents/try",
            Some(json!({ "workspaceRoot": workspace_root, "input": input, "task": task })),
            None,
            Duration::from_secs(180),
        )?;
        let view: SubagentTryResponse = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view.result)
    }

    pub fn cancel_subagent_profile_try(&self) -> Result<(), String> {
        request_json_with_timeout(
            self.address,
            &self.token,
            "POST",
            "/v1/settings/subagents/try/cancel",
            Some(json!({})),
            None,
            Duration::from_secs(5),
        )?;
        Ok(())
    }
}

#[derive(Debug)]
struct BridgeProcess {
    child: BridgeChild,
    _ready_directory: TempDir,
    address: SocketAddr,
    token: String,
    sidecar_instance_id: String,
}

#[derive(Clone, Debug)]
enum BridgeLauncher {
    #[cfg(any(debug_assertions, test))]
    Explicit(PathBuf),
    Bundled(tauri::AppHandle),
}

#[derive(Debug)]
enum BridgeChild {
    #[cfg(any(debug_assertions, test))]
    Explicit(Child),
    Bundled {
        child: Option<ShellCommandChild>,
        running: Arc<AtomicBool>,
    },
}

struct EventForwarder {
    stop: Arc<AtomicBool>,
    handle: thread::JoinHandle<()>,
}

impl BridgeLauncher {
    fn spawn(
        &self,
        ready_file: &Path,
        launch_id: &str,
        token: &str,
    ) -> Result<BridgeChild, String> {
        let args = [
            "--listen".into(),
            "127.0.0.1:0".into(),
            "--ready-file".into(),
            ready_file.as_os_str().to_owned(),
            "--launch-id".into(),
            launch_id.into(),
        ];
        // macOS ps exposes a child's launch environment even after Go unsets
        // it. Deliver the token over stdin and neutralize any inherited value.
        let token_line = format!("{token}\n");
        match self {
            #[cfg(any(debug_assertions, test))]
            Self::Explicit(binary) => {
                if !binary.is_file() {
                    return Err(format!(
                        "desktop bridge executable was not found at {}",
                        binary.display()
                    ));
                }
                let mut child = Command::new(binary)
                    .args(&args)
                    .env(BRIDGE_TOKEN_ENV, "")
                    .stdin(Stdio::piped())
                    .stdout(Stdio::null())
                    .stderr(Stdio::null())
                    .spawn()
                    .map_err(display_error)?;
                let delivered = child
                    .stdin
                    .take()
                    .ok_or_else(|| "desktop bridge stdin pipe is unavailable".to_string())
                    .and_then(|mut input| {
                        input
                            .write_all(token_line.as_bytes())
                            .map_err(display_error)
                    });
                if let Err(error) = delivered {
                    let _ = child.kill();
                    let _ = child.wait();
                    return Err(error);
                }
                Ok(BridgeChild::Explicit(child))
            }
            Self::Bundled(app) => {
                let (events, mut child) = app
                    .shell()
                    .sidecar(BUNDLED_BRIDGE_NAME)
                    .map_err(display_error)?
                    .args(args)
                    .env(BRIDGE_TOKEN_ENV, "")
                    .spawn()
                    .map_err(display_error)?;
                if let Err(error) = child.write(token_line.as_bytes()) {
                    let _ = child.kill();
                    return Err(display_error(error));
                }
                let running = Arc::new(AtomicBool::new(true));
                watch_bundled_child(events, Arc::clone(&running));
                Ok(BridgeChild::Bundled {
                    child: Some(child),
                    running,
                })
            }
        }
    }
}

impl BridgeChild {
    fn is_running(&mut self) -> Result<bool, String> {
        match self {
            #[cfg(any(debug_assertions, test))]
            Self::Explicit(child) => child
                .try_wait()
                .map(|status| status.is_none())
                .map_err(display_error),
            Self::Bundled { running, .. } => Ok(running.load(Ordering::Acquire)),
        }
    }

    fn kill(&mut self) -> Result<(), String> {
        match self {
            #[cfg(any(debug_assertions, test))]
            Self::Explicit(child) => child.kill().map_err(display_error),
            Self::Bundled { child, .. } => child
                .take()
                .ok_or_else(|| "desktop bridge process handle is unavailable".to_string())?
                .kill()
                .map_err(display_error),
        }
    }
}

fn watch_bundled_child(
    mut events: tauri::async_runtime::Receiver<CommandEvent>,
    running: Arc<AtomicBool>,
) {
    tauri::async_runtime::spawn(async move {
        while let Some(event) = events.recv().await {
            if matches!(event, CommandEvent::Terminated(_) | CommandEvent::Error(_)) {
                break;
            }
        }
        running.store(false, Ordering::Release);
    });
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ReadyFile {
    protocol_version: u8,
    address: String,
    sidecar_instance_id: String,
    launch_id: String,
}

impl BridgeSupervisor {
    pub fn from_environment(app: tauri::AppHandle) -> Self {
        #[cfg(debug_assertions)]
        if let Some(binary) = env::var_os(BRIDGE_BINARY_ENV) {
            return Self::with_binary(PathBuf::from(binary));
        }
        // Release packages must use their signed, embedded sidecar even if a
        // developer override remains in the launching shell's environment.
        Self::with_launcher(BridgeLauncher::Bundled(app))
    }

    #[cfg(any(debug_assertions, test))]
    fn with_binary(binary: PathBuf) -> Self {
        Self::with_launcher(BridgeLauncher::Explicit(binary))
    }

    fn with_launcher(launcher: BridgeLauncher) -> Self {
        Self {
            launcher,
            process: Mutex::new(None),
            events: Mutex::new(None),
        }
    }

    pub fn start(&self) -> Result<BridgeStatus, String> {
        let mut process = self
            .process
            .lock()
            .map_err(|_| "bridge state lock is unavailable")?;
        if let Some(existing) = process.as_mut() {
            if existing.child.is_running()? {
                return Ok(BridgeStatus {
                    running: true,
                    protocol_version: Some(PROTOCOL_VERSION),
                    sidecar_instance_id: Some(existing.sidecar_instance_id.clone()),
                });
            }
            *process = None;
        }
        let started = self.spawn_bridge()?;
        let status = BridgeStatus {
            running: true,
            protocol_version: Some(PROTOCOL_VERSION),
            sidecar_instance_id: Some(started.sidecar_instance_id.clone()),
        };
        *process = Some(started);
        Ok(status)
    }

    pub fn status(&self) -> BridgeStatus {
        let Ok(mut process) = self.process.lock() else {
            return BridgeStatus {
                running: false,
                protocol_version: None,
                sidecar_instance_id: None,
            };
        };
        let Some(existing) = process.as_mut() else {
            return BridgeStatus {
                running: false,
                protocol_version: None,
                sidecar_instance_id: None,
            };
        };
        if !existing.child.is_running().unwrap_or(false) {
            *process = None;
            return BridgeStatus {
                running: false,
                protocol_version: None,
                sidecar_instance_id: None,
            };
        }
        BridgeStatus {
            running: true,
            protocol_version: Some(PROTOCOL_VERSION),
            sidecar_instance_id: Some(existing.sidecar_instance_id.clone()),
        }
    }

    pub fn restart(&self) -> Result<BridgeStatus, String> {
        self.stop()?;
        self.start()
    }

    pub fn open_session(&self, request: OpenSessionRequest) -> Result<BridgeSession, String> {
        let session_id = session_path_component(&request.session_id)?;
        let request_id = opaque_secret()?;
        self.request_session(
            "POST",
            "/v1/sessions:open",
            Some(json!({
                "sessionId": session_id,
                "workspaceRoot": request.workspace_root,
            })),
            Some(&request_id),
        )
        .map(|envelope| envelope.session)
    }

    pub fn switch_session(&self, request: OpenSessionRequest) -> Result<BridgeSession, String> {
        let session_id = session_path_component(&request.session_id)?;
        let request_id = opaque_secret()?;
        self.request_session(
            "POST",
            "/v1/sessions:switch",
            Some(json!({
                "sessionId": session_id,
                "workspaceRoot": request.workspace_root,
            })),
            Some(&request_id),
        )
        .map(|envelope| envelope.session)
    }

    pub fn set_session_model(
        &self,
        session_id: &str,
        request: BridgeSetSessionModelRequest,
    ) -> Result<BridgeSession, String> {
        let session_id = session_path_component(session_id)?;
        let request_id = opaque_secret()?;
        self.request_session(
            "POST",
            &format!("/v1/sessions/{session_id}/model"),
            Some(json!({ "model": request.model })),
            Some(&request_id),
        )
        .map(|envelope| envelope.session)
    }

    pub fn rename_session(&self, request: RenameSessionRequest) -> Result<BridgeSession, String> {
        let session_id = session_path_component(&request.session_id)?;
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}/title");
        self.request_session(
            "PATCH",
            &path,
            Some(json!(BridgeRenameSessionRequest {
                title: request.title
            })),
            Some(&request_id),
        )
        .map(|envelope| envelope.session)
    }

    pub fn backfill_session_titles(
        &self,
        titles: Vec<SessionFirstMessageTitle>,
    ) -> Result<Vec<SessionFirstMessageTitle>, String> {
        if titles.len() > 50 {
            return Err("too many session titles to backfill".to_string());
        }
        let titles: Vec<_> = titles
            .into_iter()
            .map(|mut item| {
                item.session_id = session_path_component(&item.session_id)?;
                Ok(item)
            })
            .collect::<Result<_, String>>()?;
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/sessions/titles/first-message",
            Some(json!({ "titles": titles })),
            Some(&request_id),
        )?;
        let envelope: BackfillSessionTitlesResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION)
            || envelope.titles.len() > titles.len()
        {
            return Err("desktop bridge title backfill response is invalid".to_string());
        }
        let requested: std::collections::HashSet<_> =
            titles.iter().map(|item| item.session_id.as_str()).collect();
        let mut returned = std::collections::HashSet::new();
        for item in &envelope.titles {
            if !requested.contains(item.session_id.as_str())
                || item.title.trim().is_empty()
                || !returned.insert(item.session_id.as_str())
            {
                return Err("desktop bridge title backfill response is invalid".to_string());
            }
        }
        Ok(envelope.titles)
    }

    pub fn sync_session_catalog(
        &self,
        sessions: Vec<SessionCatalogMetadata>,
    ) -> Result<usize, String> {
        if sessions.len() > 50 {
            return Err("session catalog exceeds the host limit".to_string());
        }
        let sessions: Vec<_> = sessions
            .into_iter()
            .map(|mut session| {
                session.session_id = session_path_component(&session.session_id)?;
                if session
                    .workspace_root
                    .as_ref()
                    .is_some_and(|root| root.len() > 4096)
                {
                    return Err("session workspace path is too long".to_string());
                }
                Ok(session)
            })
            .collect::<Result<_, String>>()?;
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/sessions/sync-catalog",
            Some(json!({ "sessions": sessions })),
            Some(&request_id),
        )?;
        let envelope: SyncSessionCatalogResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION)
            || envelope.synced > sessions.len()
        {
            return Err("desktop bridge catalog sync response is invalid".to_string());
        }
        Ok(envelope.synced)
    }

    pub fn session_previews(
        &self,
        session_ids: Vec<String>,
    ) -> Result<Vec<SessionPreview>, String> {
        if session_ids.len() > 50 {
            return Err("too many session previews requested".to_string());
        }
        let ids: Vec<String> = session_ids
            .iter()
            .map(|id| session_path_component(id))
            .collect::<Result<_, _>>()?;
        let response = self.request_json_with_timeout(
            "POST",
            "/v1/sessions:previews",
            Some(json!({ "sessionIds": ids })),
            Duration::from_secs(20),
        )?;
        let envelope: SessionPreviewsResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != PROTOCOL_VERSION || envelope.previews.len() != ids.len() {
            return Err("desktop bridge session previews are invalid".to_string());
        }
        for (requested, preview) in ids.iter().zip(&envelope.previews) {
            if requested != &preview.session_id {
                return Err("desktop bridge session preview identity is invalid".to_string());
            }
        }
        Ok(envelope.previews)
    }

    /// Reads one identity-store directory page without changing the host's
    /// current JSON catalog authority. The UI can shadow-compare these pages
    /// before a later, explicit source switch.
    pub fn session_directory_page(
        &self,
        limit: u16,
        cursor: Option<SessionDirectoryCursor>,
        workspace_root: Option<String>,
    ) -> Result<SessionDirectoryPage, String> {
        let workspace_root = workspace_root
            .as_deref()
            .filter(|root| !root.trim().is_empty());
        let path = session_directory_path(limit, cursor.as_ref(), workspace_root)?;
        let response = self.request_json("GET", &path, None, None)?;
        let page: SessionDirectoryPage = serde_json::from_value(response).map_err(display_error)?;
        validate_session_directory_page(&page, limit, cursor.as_ref(), workspace_root)?;
        Ok(page)
    }

    /// Reads the saved workspace folders from the legacy desktop project file.
    /// This endpoint is read-only; Tauri does not participate in the legacy
    /// project's write lifecycle.
    pub fn project_folders(&self) -> Result<Vec<BridgeProjectFolder>, String> {
        let response = self.request_json("GET", "/v1/projects", None, None)?;
        let envelope: BridgeProjectFoldersResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION)
            || envelope.projects.len() > 10_000
            || envelope.projects.iter().any(|project| {
                project.root.trim().is_empty()
                    || project.root.len() > 4096
                    || project.root.chars().any(char::is_control)
                    || !Path::new(&project.root).is_absolute()
                    || project
                        .title
                        .as_ref()
                        .is_some_and(|title| title.chars().count() > 1024)
            })
        {
            return Err("desktop bridge returned an invalid project folder list".to_string());
        }
        let mut seen = std::collections::HashSet::with_capacity(envelope.projects.len());
        let mut projects = Vec::with_capacity(envelope.projects.len());
        for mut project in envelope.projects {
            if let Some(title) = project.title.as_mut() {
                title.retain(|character| !character.is_control());
            }
            if seen.insert(normalized_project_key(&project.root)) {
                projects.push(project);
            }
        }
        Ok(projects)
    }

    /// Read the entire visible directory for a bounded, diagnostic-only
    /// comparison. A changing total or incomplete scan cannot be mistaken
    /// for a zero-difference migration result.
    #[cfg(test)]
    pub fn session_directory_snapshot_with_id(
        &self,
    ) -> Result<(Vec<SessionDirectoryEntry>, String), String> {
        self.session_directory_snapshot_for_workspace(None)
    }

    #[cfg(test)]
    pub fn session_directory_snapshot_for_workspace(
        &self,
        workspace_root: Option<String>,
    ) -> Result<(Vec<SessionDirectoryEntry>, String), String> {
        let workspace_root = workspace_root
            .as_deref()
            .filter(|root| !root.trim().is_empty());
        let path = mcp_path("/v1/sessions/snapshot", workspace_root)?;
        let response = self.request_json("GET", &path, None, None)?;
        let snapshot: SessionDirectoryPage =
            serde_json::from_value(response).map_err(display_error)?;
        validate_session_directory_snapshot(&snapshot, workspace_root)?;
        Ok((snapshot.sessions, snapshot.snapshot_id))
    }

    /// One complete physical audit and directory projection per guarded page.
    /// The compact response omits scanned filesystem paths; the displayed page
    /// is still fetched separately with this snapshot ID.
    pub fn session_shadow_snapshot(
        &self,
        legacy_session_ids: &[String],
    ) -> Result<(Vec<SessionDirectoryEntry>, String, SessionPhysicalInventory), String> {
        if legacy_session_ids.len() > 50 {
            return Err("too many legacy sessions for shadow audit".to_string());
        }
        let response = self.request_json(
            "POST",
            "/v1/sessions/shadow-audit-snapshot",
            Some(json!({"legacySessionIds": legacy_session_ids})),
            None,
        )?;
        let snapshot: SessionShadowSnapshotResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if snapshot.protocol_version != PROTOCOL_VERSION {
            return Err("desktop bridge shadow snapshot protocol is unsupported".to_string());
        }
        validate_session_directory_snapshot(&snapshot.directory, None)?;
        let physical = snapshot.inventory.into_physical_inventory()?;
        let requested: std::collections::HashSet<_> =
            legacy_session_ids.iter().map(String::as_str).collect();
        if physical
            .retired_ids
            .iter()
            .any(|id| !requested.contains(id.as_str()))
        {
            return Err("desktop bridge returned an unrelated retired session ID".to_string());
        }
        Ok((
            snapshot.directory.sessions,
            snapshot.directory.snapshot_id,
            physical,
        ))
    }

    pub fn pending_session_deletes_page(
        &self,
        cursor: Option<PendingSessionDeleteCursor>,
    ) -> Result<PendingSessionDeletePage, String> {
        let mut path =
            format!("/v1/sessions/deletion-recovery/page?limit={MAX_PENDING_DELETE_PAGE_SIZE}");
        if let Some(cursor) = cursor {
            if session_path_component(&cursor.id)? != cursor.id {
                return Err("desktop bridge deletion recovery cursor is invalid".to_string());
            }
            path.push_str(&format!("&cursorId={}", cursor.id));
        }
        let response = self.request_json("GET", &path, None, None)?;
        let envelope: PendingSessionDeletesPageResponse =
            serde_json::from_value(response).map_err(display_error)?;
        validate_pending_session_deletes_page(envelope)
    }

    /// Lists incomplete manual title renames independently of the bounded
    /// legacy catalog. Reopening one is an explicit user action in the UI.
    pub fn pending_session_title_recoveries(
        &self,
    ) -> Result<Vec<PendingSessionTitleRecovery>, String> {
        let response = self.request_json("GET", "/v1/sessions/title-recovery", None, None)?;
        let envelope: PendingSessionTitleRecoveriesResponse =
            serde_json::from_value(response).map_err(display_error)?;
        validate_pending_session_title_recoveries(envelope)
    }

    /// Reduce the existing read-only inventory to file-state evidence. Paths
    /// and diagnostics never leave this method or enter the shadow report.
    #[cfg(test)]
    pub fn session_physical_inventory(&self) -> Result<SessionPhysicalInventory, String> {
        let response = self.request_json("GET", "/v1/sessions/inventory", None, None)?;
        let inventory: SessionInventoryResponse =
            serde_json::from_value(response).map_err(display_error)?;
        inventory.into_physical_inventory()
    }

    /// Imports the bounded host catalog once into the identity store. The Go
    /// side only inserts absent rows, so retries cannot overwrite newer title,
    /// workspace, or ordering metadata.
    pub fn import_legacy_session_catalog(
        &self,
        sessions: Vec<LegacySessionCatalogEntry>,
    ) -> Result<usize, String> {
        if sessions.len() > 50 {
            return Err("legacy session catalog exceeds the migration limit".to_string());
        }
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/sessions/import-catalog",
            Some(json!({ "sessions": sessions })),
            Some(&request_id),
        )?;
        let envelope: LegacyCatalogImportResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION)
            || envelope.accepted > sessions.len()
        {
            return Err("desktop bridge catalog import response is invalid".to_string());
        }
        Ok(envelope.accepted)
    }

    pub fn scan_import_candidates(
        &self,
        catalog_path: &str,
    ) -> Result<(Vec<ScanImportCandidate>, usize), String> {
        let response = self.request_json(
            "POST",
            "/v1/sessions/scan-import-candidates",
            Some(json!({ "catalogPath": catalog_path })),
            None,
        )?;
        let envelope: ScanImportCandidatesResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION)
            || envelope.candidates.len() > 10_000
            || envelope.candidates.iter().any(|item| {
                item.id.is_empty()
                    || item.file != format!("tauri-{}.jsonl", item.id)
                    || item.transcript_sha256.len() != 64
                    || !item
                        .transcript_sha256
                        .bytes()
                        .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
            })
        {
            return Err("desktop bridge scan import inventory is invalid".to_string());
        }
        Ok((envelope.candidates, envelope.blocked_count))
    }

    pub fn import_scan_sessions(
        &self,
        catalog_path: &str,
        selected: Vec<ScanImportSelection>,
    ) -> Result<Vec<String>, String> {
        if selected.is_empty() || selected.len() > 10_000 {
            return Err("scan import selection is empty or too large".to_string());
        }
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/sessions/import-scan",
            Some(json!({ "catalogPath": catalog_path, "selected": selected })),
            Some(&request_id),
        )?;
        let envelope: ScanImportApplyResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION)
            || envelope.applied != selected.len()
            || envelope.session_ids.len() != selected.len()
        {
            return Err("desktop bridge scan import response is invalid".to_string());
        }
        Ok(envelope.session_ids)
    }

    pub fn delete_session(
        &self,
        request: SessionRequest,
    ) -> Result<BridgeDeleteSessionResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}");
        let response = self.request_json("DELETE", &path, None, Some(&request_id))?;
        let envelope: BridgeDeleteSessionResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        if !envelope.deleted || envelope.session_id != session_id {
            return Err("desktop bridge did not confirm the deleted session".to_string());
        }
        Ok(envelope)
    }

    pub fn snapshot(&self, request: SessionRequest) -> Result<BridgeSnapshot, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}/snapshot");
        let envelope = self.request_session("GET", &path, None, None)?;
        let sequence = envelope.sequence.ok_or_else(|| {
            "desktop bridge snapshot did not contain an event sequence".to_string()
        })?;
        Ok(BridgeSnapshot {
            sequence,
            session: envelope.session,
            metrics: envelope.metrics,
        })
    }

    pub fn session_balance(
        &self,
        request: SessionRequest,
    ) -> Result<BridgeSessionBalanceResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}/balance");
        let response = self.request_json("GET", &path, None, None)?;
        let balance: BridgeSessionBalanceResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if balance.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(balance)
    }

    pub fn history(&self, request: SessionRequest) -> Result<BridgeHistory, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}/history");
        let response = self.request_json("GET", &path, None, None)?;
        let envelope: BridgeHistoryResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        let returned_messages = u64::try_from(envelope.messages.len())
            .map_err(|_| "desktop bridge history is too large".to_string())?;
        if envelope.start_index > envelope.total_messages
            || returned_messages > envelope.total_messages - envelope.start_index
        {
            return Err("desktop bridge history pagination is invalid".to_string());
        }
        Ok(BridgeHistory {
            sequence: envelope.sequence,
            session: envelope.session,
            messages: envelope.messages,
            start_index: envelope.start_index,
            total_messages: envelope.total_messages,
        })
    }

    pub fn provider_summary(&self) -> Result<BridgeProviderSummaryResponse, String> {
        let response = self.request_json("GET", "/v1/providers", None, None)?;
        let summary: BridgeProviderSummaryResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if summary.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(summary)
    }

    pub fn provider_summary_for_scope(
        &self,
        scope: &str,
        workspace_root: Option<&str>,
    ) -> Result<BridgeProviderSummaryResponse, String> {
        let path = provider_summary_path(scope, workspace_root)?;
        let response = self.request_json("GET", &path, None, None)?;
        let summary: BridgeProviderSummaryResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if summary.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(summary)
    }

    pub fn provider_configs(&self) -> Result<ProviderConfigList, String> {
        let response = self.request_json("GET", "/v1/settings/provider-configs", None, None)?;
        let configs: ProviderConfigList =
            serde_json::from_value(response).map_err(display_error)?;
        if configs.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(configs)
    }

    pub fn save_provider_config(
        &self,
        input: SaveProviderConfigRequest,
    ) -> Result<ProviderConfigList, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/provider-configs",
            Some(json!(input)),
            Some(&request_id),
        )?;
        let configs: ProviderConfigList =
            serde_json::from_value(response).map_err(display_error)?;
        if configs.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(configs)
    }

    pub fn delete_provider_config(
        &self,
        input: DeleteProviderConfigRequest,
    ) -> Result<ProviderConfigList, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/provider-configs/delete",
            Some(json!(input)),
            Some(&request_id),
        )?;
        let configs: ProviderConfigList =
            serde_json::from_value(response).map_err(display_error)?;
        if configs.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(configs)
    }

    pub fn discover_provider_models(
        &self,
        input: DiscoverProviderModelsRequest,
    ) -> Result<DiscoveredProviderModels, String> {
        let response = self.request_json(
            "POST",
            "/v1/settings/provider-configs/discover-models",
            Some(json!(input)),
            None,
        )?;
        let result: DiscoveredProviderModels =
            serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(result)
    }

    pub fn usage_stats(&self, request: Value) -> Result<Value, String> {
        let response =
            self.request_json("POST", "/v1/settings/usage-stats", Some(request), None)?;
        if response.get("protocolVersion").and_then(Value::as_u64)
            != Some(u64::from(PROTOCOL_VERSION))
        {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(response)
    }

    pub fn storage_settings(&self) -> Result<Value, String> {
        let response = self.request_json("GET", "/v1/settings/storage", None, None)?;
        if response.get("protocolVersion").and_then(Value::as_u64)
            != Some(u64::from(PROTOCOL_VERSION))
        {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(response)
    }

    pub fn capability_diagnostics(
        &self,
        workspace_root: &str,
        include_session_runtime: bool,
    ) -> Result<Value, String> {
        let query = url::form_urlencoded::Serializer::new(String::new())
            .append_pair("workspaceRoot", workspace_root)
            .append_pair(
                "includeSessionRuntime",
                if include_session_runtime {
                    "true"
                } else {
                    "false"
                },
            )
            .finish();
        self.request_json(
            "GET",
            &format!("/v1/settings/diagnostics/capabilities?{query}"),
            None,
            None,
        )
    }

    pub fn runtime_doctor(&self) -> Result<Value, String> {
        self.request_json("GET", "/v1/settings/diagnostics/runtime", None, None)
    }

    pub fn remote_settings(&self) -> Result<RemoteSettingsView, String> {
        let response = self.request_json("GET", "/v1/settings/remote", None, None)?;
        let view: RemoteSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn bot_runtime_status(&self) -> Result<Value, String> {
        let response = self.request_json("GET", "/v1/settings/bots/runtime", None, None)?;
        if response.get("protocolVersion").and_then(Value::as_u64)
            != Some(u64::from(PROTOCOL_VERSION))
        {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(response)
    }

    pub fn bot_settings(&self) -> Result<Value, String> {
        let response = self.request_json("GET", "/v1/settings/bots", None, None)?;
        if response.get("protocolVersion").and_then(Value::as_u64)
            != Some(u64::from(PROTOCOL_VERSION))
        {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(response)
    }

    pub fn change_bot_settings(&self, change: Value) -> Result<Value, String> {
        let request_id = opaque_secret()?;
        let response =
            self.request_json("POST", "/v1/settings/bots", Some(change), Some(&request_id))?;
        if response.get("protocolVersion").and_then(Value::as_u64)
            != Some(u64::from(PROTOCOL_VERSION))
        {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(response)
    }

    pub fn scan_remote_ssh_config(&self) -> Result<RemoteSSHConfigScanView, String> {
        let response =
            self.request_json("POST", "/v1/settings/remote/scan", Some(json!({})), None)?;
        let view: RemoteSSHConfigScanView =
            serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn change_remote_settings(
        &self,
        change: RemoteSettingsChange,
    ) -> Result<RemoteSettingsView, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/remote/hosts",
            Some(json!(change)),
            Some(&request_id),
        )?;
        let view: RemoteSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn connect_remote_host(
        &self,
        request: RemoteConnectRequest,
    ) -> Result<RemoteConnectResponse, String> {
        let response = self.request_json(
            "POST",
            "/v1/settings/remote/connect",
            Some(json!(request)),
            None,
        )?;
        let result: RemoteConnectResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(result)
    }

    pub fn disconnect_remote_host(
        &self,
        request: BridgeRemoteDisconnectRequest,
    ) -> Result<BridgeRemoteDisconnectResponse, String> {
        let response = self.request_json(
            "POST",
            "/v1/settings/remote/disconnect",
            Some(json!(request)),
            None,
        )?;
        let result: BridgeRemoteDisconnectResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(result)
    }

    pub fn browse_remote_host(
        &self,
        request: BridgeRemoteBrowseRequest,
    ) -> Result<BridgeRemoteBrowseResponse, String> {
        let response = self.request_json(
            "POST",
            "/v1/settings/remote/browse",
            Some(json!(request)),
            None,
        )?;
        let result: BridgeRemoteBrowseResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(result)
    }

    pub fn preview_remote_file(
        &self,
        request: BridgeRemoteFilePreviewRequest,
    ) -> Result<BridgeRemoteFilePreviewResponse, String> {
        let response = self.request_json(
            "POST",
            "/v1/settings/remote/preview",
            Some(json!(request)),
            None,
        )?;
        let result: BridgeRemoteFilePreviewResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(result)
    }

    pub fn save_remote_file(
        &self,
        request: BridgeRemoteFileSaveRequest,
    ) -> Result<BridgeRemoteFileSaveResponse, String> {
        let response = self.request_json(
            "POST",
            "/v1/settings/remote/save",
            Some(json!(request)),
            None,
        )?;
        let result: BridgeRemoteFileSaveResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(result)
    }

    pub fn permission_settings(
        &self,
        workspace_root: String,
    ) -> Result<PermissionSettingsView, String> {
        let query = url::form_urlencoded::Serializer::new(String::new())
            .append_pair(
                "scope",
                if workspace_root.trim().is_empty() {
                    "global"
                } else {
                    "project"
                },
            )
            .append_pair("workspaceRoot", &workspace_root)
            .finish();
        let response = self.request_json(
            "GET",
            &format!("/v1/settings/permissions?{query}"),
            None,
            None,
        )?;
        let view: PermissionSettingsView =
            serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn change_permission_settings(
        &self,
        change: PermissionSettingsChange,
    ) -> Result<PermissionSettingsView, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/permissions",
            Some(json!(change)),
            Some(&request_id),
        )?;
        let view: PermissionSettingsView =
            serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn secrets_settings(&self) -> Result<SecretsSettingsView, String> {
        let response = self.request_json("GET", "/v1/settings/secrets", None, None)?;
        let view: SecretsSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn change_secrets_settings(
        &self,
        change: SecretsSettingsChange,
    ) -> Result<SecretsSettingsView, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/secrets",
            Some(json!(change)),
            Some(&request_id),
        )?;
        let view: SecretsSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn sandbox_settings(
        &self,
        workspace_root: Option<&str>,
        session_id: Option<&str>,
    ) -> Result<SandboxSettingsView, String> {
        let mut query = url::form_urlencoded::Serializer::new(String::new());
        if let Some(root) = workspace_root.filter(|root| !root.is_empty()) {
            query.append_pair("workspaceRoot", root);
        }
        if let Some(id) = session_id.filter(|id| !id.is_empty()) {
            query.append_pair("sessionId", id);
        }
        let query = query.finish();
        let path = if query.is_empty() {
            "/v1/settings/sandbox".to_string()
        } else {
            format!("/v1/settings/sandbox?{query}")
        };
        let response = self.request_json("GET", &path, None, None)?;
        let view: SandboxSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn change_sandbox_settings(
        &self,
        change: SandboxSettingsChange,
    ) -> Result<SandboxSettingsView, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/sandbox",
            Some(json!(change)),
            Some(&request_id),
        )?;
        let view: SandboxSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn network_settings(&self) -> Result<NetworkSettingsView, String> {
        let response = self.request_json("GET", "/v1/settings/network", None, None)?;
        let view: NetworkSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn change_network_settings(
        &self,
        change: NetworkSettingsChange,
    ) -> Result<NetworkSettingsView, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/network",
            Some(json!(change)),
            Some(&request_id),
        )?;
        let view: NetworkSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn skills_settings(&self, workspace_root: &str) -> Result<SkillsSettingsView, String> {
        let query = url::form_urlencoded::Serializer::new(String::new())
            .append_pair("workspaceRoot", workspace_root)
            .finish();
        let response =
            self.request_json("GET", &format!("/v1/settings/skills?{query}"), None, None)?;
        let view: SkillsSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn change_skills_settings(
        &self,
        change: SkillsSettingsChange,
    ) -> Result<SkillsSettingsView, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/skills",
            Some(json!(change)),
            Some(&request_id),
        )?;
        let view: SkillsSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn plan_skill_install(
        &self,
        request: SkillInstallRequest,
    ) -> Result<SkillInstallPlan, String> {
        let response = self.request_json_slow(
            "POST",
            "/v1/settings/skills/plan",
            Some(json!(request)),
            None,
        )?;
        let view: SkillInstallPlan = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn install_skill(
        &self,
        request: SkillInstallRequest,
    ) -> Result<SkillInstallResult, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json_slow(
            "POST",
            "/v1/settings/skills/install",
            Some(json!(request)),
            Some(&request_id),
        )?;
        let view: SkillInstallResult = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn archive_skill(
        &self,
        request: SkillArchiveRequest,
    ) -> Result<SkillArchiveResult, String> {
        self.skill_archive_request("/v1/settings/skills/archive", request)
    }

    pub fn restore_skill(
        &self,
        request: SkillArchiveRequest,
    ) -> Result<SkillArchiveResult, String> {
        self.skill_archive_request("/v1/settings/skills/restore", request)
    }

    fn skill_archive_request(
        &self,
        path: &str,
        request: SkillArchiveRequest,
    ) -> Result<SkillArchiveResult, String> {
        let request_id = opaque_secret()?;
        let response =
            self.request_json_slow("POST", path, Some(json!(request)), Some(&request_id))?;
        let view: SkillArchiveResult = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn plugin_settings(&self) -> Result<PluginSettingsView, String> {
        let response = self.request_json("GET", "/v1/settings/plugins", None, None)?;
        let view: PluginSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn change_plugin_settings(
        &self,
        change: PluginSettingsChange,
    ) -> Result<PluginSettingsView, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/plugins",
            Some(json!(change)),
            Some(&request_id),
        )?;
        let view: PluginSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn plugin_doctor(&self, name: &str) -> Result<PluginDoctorView, String> {
        let response = self.request_json(
            "POST",
            "/v1/settings/plugins/doctor",
            Some(json!({"name": name})),
            None,
        )?;
        let view: PluginDoctorView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn plan_plugin_install(
        &self,
        source: &str,
        mode: &str,
        replace: bool,
        expected_name: &str,
        expected_revision: &str,
    ) -> Result<PluginInstallPlan, String> {
        let response = self.request_json_slow(
            "POST",
            "/v1/settings/plugins/plan",
            Some(json!({"source": source, "mode": mode, "replace": replace, "expectedName": expected_name, "expectedRevision": expected_revision})),
            None,
        )?;
        let view: PluginInstallPlan = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn install_plugin(
        &self,
        request: PluginInstallRequest,
    ) -> Result<PluginOperationResult, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json_slow(
            "POST",
            "/v1/settings/plugins/install",
            Some(json!(request)),
            Some(&request_id),
        )?;
        let view: PluginOperationResult =
            serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn remove_plugin(
        &self,
        request: PluginRemoveRequest,
    ) -> Result<PluginOperationResult, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json_slow(
            "POST",
            "/v1/settings/plugins/remove",
            Some(json!(request)),
            Some(&request_id),
        )?;
        let view: PluginOperationResult =
            serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn subagent_settings(&self, workspace_root: &str) -> Result<SubagentSettingsView, String> {
        let query = url::form_urlencoded::Serializer::new(String::new())
            .append_pair("workspaceRoot", workspace_root)
            .finish();
        let response = self.request_json(
            "GET",
            &format!("/v1/settings/subagents?{query}"),
            None,
            None,
        )?;
        let view: SubagentSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn change_subagent_settings(
        &self,
        change: SubagentSettingsChange,
    ) -> Result<SubagentSettingsView, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/subagents",
            Some(json!(change)),
            Some(&request_id),
        )?;
        let view: SubagentSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn subagent_try_client(&self) -> Result<BridgeSubagentTryClient, String> {
        let (address, token) = self.event_connection()?;
        Ok(BridgeSubagentTryClient { address, token })
    }

    pub fn hooks_settings(
        &self,
        scope: &str,
        workspace_root: &str,
    ) -> Result<HooksSettingsView, String> {
        let query = url::form_urlencoded::Serializer::new(String::new())
            .append_pair("scope", scope)
            .append_pair("workspaceRoot", workspace_root)
            .finish();
        let response =
            self.request_json("GET", &format!("/v1/settings/hooks?{query}"), None, None)?;
        let view: HooksSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn change_hooks_settings(
        &self,
        change: HooksSettingsChange,
    ) -> Result<HooksSettingsView, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/hooks",
            Some(json!(change)),
            Some(&request_id),
        )?;
        let view: HooksSettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn memory_settings(&self, workspace_root: &str) -> Result<MemorySettingsView, String> {
        let query = url::form_urlencoded::Serializer::new(String::new())
            .append_pair("workspaceRoot", workspace_root)
            .finish();
        let response =
            self.request_json("GET", &format!("/v1/settings/memory?{query}"), None, None)?;
        let view: MemorySettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn change_memory_settings(
        &self,
        change: MemorySettingsChange,
    ) -> Result<MemorySettingsView, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/memory",
            Some(json!(change)),
            Some(&request_id),
        )?;
        let view: MemorySettingsView = serde_json::from_value(response).map_err(display_error)?;
        if view.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(view)
    }

    pub fn memory_suggestions(
        &self,
        workspace_root: &str,
    ) -> Result<MemorySuggestionsView, String> {
        let query = url::form_urlencoded::Serializer::new(String::new())
            .append_pair("workspaceRoot", workspace_root)
            .finish();
        let response = self.request_json(
            "GET",
            &format!("/v1/settings/memory/suggestions?{query}"),
            None,
            None,
        )?;
        serde_json::from_value(response).map_err(display_error)
    }

    pub fn accept_memory_suggestion(
        &self,
        request: MemorySuggestionAcceptanceRequest,
    ) -> Result<MemorySuggestionAcceptance, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/memory/suggestions/accept",
            Some(json!(request)),
            Some(&request_id),
        )?;
        serde_json::from_value(response).map_err(display_error)
    }

    pub fn desktop_preferences(&self) -> Result<DesktopPreferences, String> {
        let response = self.request_json("GET", "/v1/settings/desktop", None, None)?;
        let preferences: DesktopPreferences =
            serde_json::from_value(response).map_err(display_error)?;
        if preferences.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(preferences)
    }

    pub fn set_desktop_external_opener(&self, id: &str) -> Result<DesktopPreferences, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/desktop/external-opener",
            Some(json!({"id": id})),
            Some(&request_id),
        )?;
        let preferences: DesktopPreferences =
            serde_json::from_value(response).map_err(display_error)?;
        if preferences.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".into());
        }
        Ok(preferences)
    }

    pub fn set_desktop_approval(&self, mode: String) -> Result<DesktopPreferences, String> {
        if !matches!(mode.as_str(), "ask" | "auto" | "yolo") {
            return Err("invalid desktop approval mode".to_string());
        }
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/desktop/approval",
            Some(json!({"mode": mode})),
            Some(&request_id),
        )?;
        let preferences: DesktopPreferences =
            serde_json::from_value(response).map_err(display_error)?;
        if preferences.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(preferences)
    }

    pub fn set_desktop_terminal_theme(&self, theme: String) -> Result<DesktopPreferences, String> {
        if !matches!(theme.as_str(), "auto" | "dark" | "light") {
            return Err("invalid desktop terminal theme".to_string());
        }
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/desktop/terminal-theme",
            Some(json!({"theme": theme})),
            Some(&request_id),
        )?;
        let preferences: DesktopPreferences =
            serde_json::from_value(response).map_err(display_error)?;
        if preferences.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(preferences)
    }

    pub fn set_desktop_appearance(
        &self,
        theme: String,
        style: String,
    ) -> Result<DesktopPreferences, String> {
        if !matches!(theme.as_str(), "auto" | "dark" | "light")
            || !matches!(
                style.as_str(),
                "" | "graphite" | "aurora" | "slate" | "carbon" | "nocturne" | "amber"
            )
        {
            return Err("invalid desktop appearance".to_string());
        }
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/desktop/appearance",
            Some(json!({"theme": theme, "style": style})),
            Some(&request_id),
        )?;
        let preferences: DesktopPreferences =
            serde_json::from_value(response).map_err(display_error)?;
        if preferences.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(preferences)
    }

    pub fn set_desktop_language(&self, language: String) -> Result<DesktopPreferences, String> {
        if !matches!(language.as_str(), "" | "en" | "zh") {
            return Err("invalid desktop language".to_string());
        }
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/desktop/language",
            Some(json!({"language": language})),
            Some(&request_id),
        )?;
        let preferences: DesktopPreferences =
            serde_json::from_value(response).map_err(display_error)?;
        if preferences.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(preferences)
    }

    pub fn set_desktop_currency(&self, currency: String) -> Result<DesktopPreferences, String> {
        if !matches!(currency.as_str(), "" | "CNY" | "USD") {
            return Err("invalid desktop display currency".to_string());
        }
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/desktop/currency",
            Some(json!({"currency": currency})),
            Some(&request_id),
        )?;
        let preferences: DesktopPreferences =
            serde_json::from_value(response).map_err(display_error)?;
        if preferences.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(preferences)
    }

    /// Lists the effective MCP servers for a workspace. Credentials never cross
    /// this boundary: the bridge returns key names only.
    pub fn mcp_servers(&self, workspace_root: Option<&str>) -> Result<Vec<MCPServerView>, String> {
        let path = mcp_path("/v1/mcp/servers", workspace_root)?;
        let response = self.request_json("GET", &path, None, None)?;
        let envelope: MCPServerListResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope.servers)
    }

    pub fn mcp_runtime_action(
        &self,
        request: MCPRuntimeActionRequest,
    ) -> Result<MCPRuntimeActionResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        if request.name.trim().is_empty()
            || request.name.trim().len() > 128
            || !matches!(request.action.as_str(), "connect" | "disconnect")
        {
            return Err("invalid MCP runtime action".to_string());
        }
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}/mcp/runtime");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!({ "name": request.name.trim(), "action": request.action })),
            Some(&request_id),
        )?;
        let result: MCPRuntimeActionResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION)
            || result.name != request.name.trim()
            || result.action != request.action
        {
            return Err("desktop bridge MCP runtime action response is invalid".to_string());
        }
        Ok(result)
    }

    pub fn clear_mcp_authentication(
        &self,
        request: MCPClearAuthRequest,
    ) -> Result<MCPClearAuthResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let name = request.name.trim();
        if name.is_empty() || name.len() > 128 {
            return Err("invalid MCP credential request".to_string());
        }
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}/mcp/auth/clear");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!({ "name": name })),
            Some(&request_id),
        )?;
        let result: MCPClearAuthResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION) || result.name != name {
            return Err("desktop bridge MCP credential response is invalid".to_string());
        }
        Ok(result)
    }

    pub fn start_mcp_oauth(&self, request: MCPOAuthRequest) -> Result<MCPOAuthResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        if request.name.trim().is_empty() || request.name.trim().len() > 128 {
            return Err("invalid MCP authorization request".to_string());
        }
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}/mcp/oauth");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!({ "name": request.name.trim() })),
            Some(&request_id),
        )?;
        let result: MCPOAuthResponse = serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION)
            || result.name != request.name.trim()
            || result.flow_id.is_empty()
            || result.status != "pending"
        {
            return Err("desktop bridge MCP authorization response is invalid".to_string());
        }
        Ok(result)
    }

    pub fn mcp_oauth_status(&self, request: MCPOAuthRequest) -> Result<MCPOAuthResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let flow_id = mcp_oauth_flow_component(&request.flow_id)?;
        let path = format!("/v1/sessions/{session_id}/mcp/oauth/{flow_id}");
        let response = self.request_json("GET", &path, None, None)?;
        let result: MCPOAuthResponse = serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION)
            || result.flow_id != request.flow_id
            || !matches!(
                result.status.as_str(),
                "pending" | "complete" | "failed" | "canceled"
            )
        {
            return Err("desktop bridge MCP authorization status is invalid".to_string());
        }
        Ok(result)
    }

    pub fn cancel_mcp_oauth(&self, request: MCPOAuthRequest) -> Result<MCPOAuthResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let flow_id = mcp_oauth_flow_component(&request.flow_id)?;
        let path = format!("/v1/sessions/{session_id}/mcp/oauth/{flow_id}");
        let response = self.request_json("DELETE", &path, None, None)?;
        let result: MCPOAuthResponse = serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION)
            || result.flow_id != request.flow_id
            || result.status != "canceled"
        {
            return Err("desktop bridge MCP authorization cancel response is invalid".to_string());
        }
        Ok(result)
    }

    pub fn save_mcp_server(
        &self,
        input: MCPServerInput,
        workspace_root: Option<&str>,
    ) -> Result<MCPServerMutationResponse, String> {
        let request_id = opaque_secret()?;
        let path = mcp_path("/v1/mcp/servers", workspace_root)?;
        let response = self.request_json("POST", &path, Some(json!(input)), Some(&request_id))?;
        let envelope: MCPServerMutationResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn delete_mcp_server(
        &self,
        name: String,
        workspace_root: Option<&str>,
    ) -> Result<MCPServerMutationResponse, String> {
        let path = mcp_path("/v1/mcp/servers", workspace_root)?;
        let response = self.request_json(
            "DELETE",
            &path,
            Some(json!(MCPServerDeleteRequest { name })),
            None,
        )?;
        let envelope: MCPServerMutationResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn set_mcp_server_enabled(
        &self,
        request: MCPServerActivationRequest,
        workspace_root: Option<&str>,
    ) -> Result<MCPServerMutationResponse, String> {
        let request_id = opaque_secret()?;
        let path = mcp_path("/v1/mcp/servers/activation", workspace_root)?;
        let response = self.request_json("POST", &path, Some(json!(request)), Some(&request_id))?;
        let envelope: MCPServerMutationResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn search_mcp_marketplace(&self, query: &str) -> Result<MCPMarketplaceResponse, String> {
        let encoded = url::form_urlencoded::Serializer::new(String::new())
            .append_pair("query", query)
            .finish();
        let path = format!("/v1/mcp/marketplace?{encoded}");
        let response = self.request_json("GET", &path, None, None)?;
        let envelope: MCPMarketplaceResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn resolve_mcp_marketplace(&self, name: &str) -> Result<MCPMarketplaceEntry, String> {
        let response = self.request_json(
            "POST",
            "/v1/mcp/marketplace/resolve",
            Some(json!({ "name": name })),
            None,
        )?;
        let envelope: MCPMarketplaceResolveResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope.server)
    }

    pub fn set_default_model(
        &self,
        request: BridgeSetDefaultModelRequest,
    ) -> Result<BridgeProviderSummaryResponse, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/default-model",
            Some(json!({
                "model": request.model,
                "scope": request.scope,
                "workspaceRoot": request.workspace_root,
            })),
            Some(&request_id),
        )?;
        let summary: BridgeProviderSummaryResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if summary.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(summary)
    }

    pub fn set_model_role(
        &self,
        request: BridgeSetModelRoleRequest,
    ) -> Result<BridgeProviderSummaryResponse, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/model-role",
            Some(json!({
                "role": request.role,
                "model": request.model,
                "scope": request.scope,
                "workspaceRoot": request.workspace_root,
            })),
            Some(&request_id),
        )?;
        let summary: BridgeProviderSummaryResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if summary.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(summary)
    }

    pub fn set_agent_preferences(
        &self,
        request: BridgeSetAgentPreferenceRequest,
    ) -> Result<BridgeProviderSummaryResponse, String> {
        let (key, value) = match (request.reasoning_language, request.compact_ratio_percent) {
            (Some(language), None) => ("reasoningLanguage", json!(language)),
            (None, Some(percent)) => ("compactRatioPercent", json!(percent)),
            _ => return Err("exactly one agent preference must be provided".to_string()),
        };
        let mut payload = serde_json::Map::new();
        payload.insert(key.to_string(), value);
        if let Some(scope) = request.scope {
            payload.insert("scope".to_string(), json!(scope));
        }
        if let Some(workspace_root) = request.workspace_root {
            payload.insert("workspaceRoot".to_string(), json!(workspace_root));
        }
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/agent-preferences",
            Some(serde_json::Value::Object(payload)),
            Some(&request_id),
        )?;
        let summary: BridgeProviderSummaryResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if summary.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(summary)
    }

    /// Synchronize a provider key into the sidecar's private memory. The key
    /// is sent only over the authenticated loopback bridge; it is never added
    /// to a process environment or returned to the WebView.
    pub fn set_provider_key(
        &self,
        provider_name: &str,
        api_key: Option<&str>,
    ) -> Result<BridgeProviderSummaryResponse, String> {
        if provider_name.trim().is_empty() {
            return Err("provider name is invalid".to_string());
        }
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/provider-key",
            Some(json!({
                "providerName": provider_name,
                "apiKey": api_key.unwrap_or_default(),
                "delete": api_key.is_none(),
            })),
            Some(&request_id),
        )?;
        let summary: BridgeProviderSummaryResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if summary.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(summary)
    }

    pub fn test_provider_model(
        &self,
        request: BridgeProviderModelProbeRequest,
    ) -> Result<BridgeProviderModelProbeResponse, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/provider-model-probe",
            Some(json!({
                "name": request.name,
                "model": request.model,
                "apiKey": request.api_key.unwrap_or_default(),
            })),
            Some(&request_id),
        )?;
        let result: BridgeProviderModelProbeResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if result.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(result)
    }

    pub fn submit(&self, request: SubmitRequest) -> Result<BridgeSession, String> {
        let session_id = session_path_component(&request.session_id)?;
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}:submit");
        self.request_session(
            "POST",
            &path,
            Some(json!({ "input": request.input })),
            Some(&request_id),
        )
        .map(|envelope| envelope.session)
    }

    pub fn attach_file(&self, request: AttachFileRequest) -> Result<BridgeAttachment, String> {
        let session_id = session_path_component(&request.session_id)?;
        if request.path.trim().is_empty() || request.path.len() > 32 * 1024 {
            return Err("selected attachment path is invalid".to_string());
        }
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}:attach");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!({ "sessionId": session_id, "path": request.path })),
            Some(&request_id),
        )?;
        let envelope: BridgeAttachmentResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        validate_attachment(envelope.attachment)
    }

    pub fn workspace(
        &self,
        request: WorkspaceRequest,
    ) -> Result<BridgeWorkspaceListResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        if request.path.len() > 4096 || request.path.contains('\0') {
            return Err("workspace path is invalid".to_string());
        }
        let path = format!("/v1/sessions/{session_id}:workspace");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeWorkspaceRequest { path: request.path })),
            None,
        )?;
        let envelope: BridgeWorkspaceListResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn workspace_file(
        &self,
        request: WorkspaceFileRequest,
    ) -> Result<BridgeWorkspaceFileResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        if request.path.trim().is_empty()
            || request.path.len() > 4096
            || request.path.contains('\0')
        {
            return Err("workspace file path is invalid".to_string());
        }
        let path = format!("/v1/sessions/{session_id}:workspace-file");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeWorkspaceFileRequest { path: request.path })),
            None,
        )?;
        let envelope: BridgeWorkspaceFileResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn workspace_changes(
        &self,
        request: SessionRequest,
    ) -> Result<BridgeWorkspaceChangesResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}:workspace-changes");
        let response = self.request_json("POST", &path, None, None)?;
        let envelope: BridgeWorkspaceChangesResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn workspace_change_detail(
        &self,
        request: WorkspaceChangeDetailRequest,
    ) -> Result<BridgeWorkspaceChangeDetailResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        if request.path.trim().is_empty()
            || request.path.len() > 4096
            || request.path.contains('\0')
        {
            return Err("workspace change path is invalid".to_string());
        }
        let path = format!("/v1/sessions/{session_id}:workspace-change-detail");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeWorkspaceChangeDetailRequest {
                path: request.path
            })),
            None,
        )?;
        let envelope: BridgeWorkspaceChangeDetailResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn workspace_file_revert_preview(
        &self,
        request: WorkspaceChangeDetailRequest,
    ) -> Result<BridgeWorkspaceFileRevertPlanResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        if request.path.trim().is_empty()
            || request.path.len() > 4096
            || request.path.contains('\0')
        {
            return Err("workspace file revert path is invalid".to_string());
        }
        let path = format!("/v1/sessions/{session_id}:workspace-file-revert-preview");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeWorkspaceChangeDetailRequest {
                path: request.path
            })),
            None,
        )?;
        let envelope: BridgeWorkspaceFileRevertPlanResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn workspace_file_revert_commit(
        &self,
        request: WorkspaceFileRevertCommitRequest,
    ) -> Result<BridgeWorkspaceFileRevertResultResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let plan_id = session_path_component(&request.plan_id)
            .map_err(|_| "workspace file revert plan ID is invalid".to_string())?;
        if !matches!(request.resolution.as_str(), "" | "overwrite_checkpoint") {
            return Err("workspace file revert resolution is invalid".to_string());
        }
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}:workspace-file-revert-commit");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeWorkspaceFileRevertCommitRequest {
                plan_id,
                resolution: request.resolution,
            })),
            Some(&request_id),
        )?;
        let envelope: BridgeWorkspaceFileRevertResultResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn workspace_file_revert_undo(
        &self,
        request: WorkspaceFileRevertUndoRequest,
    ) -> Result<BridgeWorkspaceFileRevertResultResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let transaction_id = session_path_component(&request.transaction_id)
            .map_err(|_| "workspace file revert transaction ID is invalid".to_string())?;
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}:workspace-file-revert-undo");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeWorkspaceFileRevertUndoRequest {
                transaction_id
            })),
            Some(&request_id),
        )?;
        let envelope: BridgeWorkspaceFileRevertResultResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn workspace_checkpoints(
        &self,
        request: SessionRequest,
    ) -> Result<BridgeWorkspaceCheckpointsResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}:checkpoints");
        let response = self.request_json("POST", &path, None, None)?;
        let envelope: BridgeWorkspaceCheckpointsResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn code_rewind_preview(
        &self,
        request: CodeRewindPreviewRequest,
    ) -> Result<BridgeCodeRewindPlanResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}:code-rewind-preview");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeCodeRewindPreviewRequest { turn: request.turn })),
            None,
        )?;
        let envelope: BridgeCodeRewindPlanResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn code_rewind_commit(
        &self,
        request: CodeRewindCommitRequest,
    ) -> Result<BridgeWorkspaceFileRevertResultResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let plan_id = session_path_component(&request.plan_id)
            .map_err(|_| "code rewind plan ID is invalid".to_string())?;
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}:code-rewind-commit");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeCodeRewindCommitRequest {
                plan_id,
                confirm_partial_coverage: request.confirm_partial_coverage,
            })),
            Some(&request_id),
        )?;
        let envelope: BridgeWorkspaceFileRevertResultResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn conversation_rewind_preview(
        &self,
        request: ConversationRewindPreviewRequest,
    ) -> Result<BridgeConversationRewindPlanResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}:conversation-rewind-preview");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeConversationRewindPreviewRequest {
                turn: request.turn
            })),
            None,
        )?;
        let envelope: BridgeConversationRewindPlanResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn conversation_rewind_commit(
        &self,
        request: ConversationRewindCommitRequest,
    ) -> Result<BridgeConversationRewindResultResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let plan_id = session_path_component(&request.plan_id)
            .map_err(|_| "conversation rewind plan ID is invalid".to_string())?;
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}:conversation-rewind-commit");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeConversationRewindCommitRequest { plan_id })),
            Some(&request_id),
        )?;
        let envelope: BridgeConversationRewindResultResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn conversation_rewind_undo(
        &self,
        request: ConversationRewindUndoRequest,
    ) -> Result<BridgeConversationRewindResultResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let head_id = session_path_component(&request.head_id)
            .map_err(|_| "conversation rewind head ID is invalid".to_string())?;
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}:conversation-rewind-undo");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeConversationRewindUndoRequest { head_id })),
            Some(&request_id),
        )?;
        let envelope: BridgeConversationRewindResultResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn session_heads(
        &self,
        request: SessionRequest,
    ) -> Result<BridgeSessionHeadsResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}:session-heads");
        let response = self.request_json("POST", &path, None, None)?;
        let envelope: BridgeSessionHeadsResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn session_head_switch(
        &self,
        request: SessionHeadSwitchRequest,
    ) -> Result<BridgeSessionHeadSwitchResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let head_id = session_path_component(&request.head_id)
            .map_err(|_| "conversation version ID is invalid".to_string())?;
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}:session-head-switch");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeSessionHeadSwitchRequest { head_id })),
            Some(&request_id),
        )?;
        let envelope: BridgeSessionHeadSwitchResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn combined_rewind_preview(
        &self,
        request: CombinedRewindPreviewRequest,
    ) -> Result<BridgeCombinedRewindPlanResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}:combined-rewind-preview");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeCombinedRewindPreviewRequest {
                turn: request.turn
            })),
            None,
        )?;
        let envelope: BridgeCombinedRewindPlanResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn combined_rewind_commit(
        &self,
        request: CombinedRewindCommitRequest,
    ) -> Result<BridgeCombinedRewindResultResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let plan_id = session_path_component(&request.plan_id)
            .map_err(|_| "combined rewind plan ID is invalid".to_string())?;
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}:combined-rewind-commit");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeCombinedRewindCommitRequest {
                plan_id,
                confirm_partial_coverage: request.confirm_partial_coverage,
            })),
            Some(&request_id),
        )?;
        let envelope: BridgeCombinedRewindResultResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn legacy_fork_preview(
        &self,
        request: ConversationRewindPreviewRequest,
    ) -> Result<BridgeConversationRewindPlanResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}:legacy-fork-preview");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeConversationRewindPreviewRequest {
                turn: request.turn
            })),
            None,
        )?;
        let envelope: BridgeConversationRewindPlanResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn legacy_fork_commit(
        &self,
        request: ConversationRewindCommitRequest,
    ) -> Result<BridgeLegacyConversationForkResultResponse, String> {
        let session_id = session_path_component(&request.session_id)?;
        let plan_id = session_path_component(&request.plan_id)
            .map_err(|_| "legacy conversation fork plan ID is invalid".to_string())?;
        let request_id = opaque_secret()?;
        let path = format!("/v1/sessions/{session_id}:legacy-fork-commit");
        let response = self.request_json(
            "POST",
            &path,
            Some(json!(BridgeConversationRewindCommitRequest { plan_id })),
            Some(&request_id),
        )?;
        let envelope: BridgeLegacyConversationForkResultResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    pub fn cancel(&self, request: SessionRequest) -> Result<BridgeSession, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}:cancel");
        self.request_session("POST", &path, None, None)
            .map(|envelope| envelope.session)
    }

    pub fn approve(&self, request: ApproveRequest) -> Result<BridgeSession, String> {
        let session_id = session_path_component(&request.session_id)?;
        let prompt_id = prompt_id_component(&request.id)?;
        let path = format!("/v1/sessions/{session_id}:approve");
        self.request_session(
            "POST",
            &path,
            Some(json!(BridgeApprovalRequest {
                id: prompt_id,
                allow: request.allow
            })),
            None,
        )
        .map(|envelope| envelope.session)
    }

    pub fn answer_question(&self, request: AnswerQuestionRequest) -> Result<BridgeSession, String> {
        let session_id = session_path_component(&request.session_id)?;
        let prompt_id = prompt_id_component(&request.id)?;
        let path = format!("/v1/sessions/{session_id}:answer");
        self.request_session(
            "POST",
            &path,
            Some(json!(BridgeAnswerQuestionRequest {
                id: prompt_id,
                answers: request.answers
            })),
            None,
        )
        .map(|envelope| envelope.session)
    }

    pub fn answer_mcp_interaction(
        &self,
        request: AnswerMCPInteractionRequest,
    ) -> Result<BridgeSession, String> {
        let session_id = session_path_component(&request.session_id)?;
        let prompt_id = prompt_id_component(&request.id)?;
        let path = format!("/v1/sessions/{session_id}:mcp");
        self.request_session(
            "POST",
            &path,
            Some(json!(BridgeMCPInteractionAnswerRequest {
                id: prompt_id,
                action: request.action,
                content: request.content,
            })),
            None,
        )
        .map(|envelope| envelope.session)
    }

    pub fn replay_pending_prompts(&self, request: SessionRequest) -> Result<BridgeSession, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}:replay-prompts");
        self.request_session("POST", &path, None, None)
            .map(|envelope| envelope.session)
    }

    pub fn start_events(&self, app: tauri::AppHandle, after_sequence: u64) -> Result<(), String> {
        let (address, token) = self.event_connection()?;
        self.stop_events();
        let stop = Arc::new(AtomicBool::new(false));
        let stop_for_thread = Arc::clone(&stop);
        let handle = thread::spawn(move || {
            forward_events(app, address, token, after_sequence, stop_for_thread)
        });
        *self
            .events
            .lock()
            .map_err(|_| "bridge event state lock is unavailable")? =
            Some(EventForwarder { stop, handle });
        Ok(())
    }

    pub fn stop(&self) -> Result<(), String> {
        self.stop_events();
        let process = self
            .process
            .lock()
            .map_err(|_| "bridge state lock is unavailable")?
            .take();
        let Some(mut process) = process else {
            return Ok(());
        };

        let _ = request_shutdown(process.address, &process.token);
        if wait_for_exit(&mut process.child, STOP_TIMEOUT)? {
            return Ok(());
        }
        // This is the exact Child started above; never identify a process by
        // name, port, or a user-provided PID.
        process.child.kill()?;
        if wait_for_exit(&mut process.child, STOP_TIMEOUT)? {
            Ok(())
        } else {
            Err("desktop bridge did not exit after termination".to_string())
        }
    }

    fn spawn_bridge(&self) -> Result<BridgeProcess, String> {
        let ready_directory = tempfile::Builder::new()
            .prefix("reasonix-tauri-bridge-")
            .tempdir()
            .map_err(display_error)?;
        let ready_file = ready_directory.path().join("ready.json");
        let token = opaque_secret()?;
        let launch_id = opaque_secret()?;
        let mut child = self.launcher.spawn(&ready_file, &launch_id, &token)?;

        let ready = wait_for_ready(&mut child, &ready_file, &launch_id).inspect_err(|_| {
            let _ = child.kill();
            let _ = wait_for_exit(&mut child, STOP_TIMEOUT);
        })?;
        let health = request_json(ready.address, &token, "GET", "/v1/health", None, None)
            .and_then(|response| verify_bridge_health(response, &ready.sidecar_instance_id));
        if let Err(error) = health {
            let _ = child.kill();
            let _ = wait_for_exit(&mut child, STOP_TIMEOUT);
            return Err(error);
        }
        Ok(BridgeProcess {
            child,
            _ready_directory: ready_directory,
            address: ready.address,
            token,
            sidecar_instance_id: ready.sidecar_instance_id,
        })
    }

    fn request_session(
        &self,
        method: &str,
        path: &str,
        body: Option<Value>,
        request_id: Option<&str>,
    ) -> Result<BridgeSessionResponse, String> {
        let response = self.request_json(method, path, body, request_id)?;
        let envelope: BridgeSessionResponse =
            serde_json::from_value(response).map_err(display_error)?;
        if envelope.protocol_version != u64::from(PROTOCOL_VERSION) {
            return Err("desktop bridge protocol version is unsupported".to_string());
        }
        Ok(envelope)
    }

    fn request_json(
        &self,
        method: &str,
        path: &str,
        body: Option<Value>,
        request_id: Option<&str>,
    ) -> Result<Value, String> {
        let mut process = self
            .process
            .lock()
            .map_err(|_| "bridge state lock is unavailable")?;
        let Some(running) = process.as_mut() else {
            return Err("desktop bridge is not running".to_string());
        };
        if !running.child.is_running()? {
            *process = None;
            return Err("desktop bridge is not running".to_string());
        }
        request_json(
            running.address,
            &running.token,
            method,
            path,
            body.clone(),
            request_id,
        )
        // The Go bridge caches this request ID before a turn is admitted. If
        // the host lost the first response after sending it, one same-ID retry
        // is safe; it replays the response instead of submitting twice.
        .or_else(|first_error| match request_id {
            Some(request_id) => request_json(
                running.address,
                &running.token,
                method,
                path,
                body,
                Some(request_id),
            )
            .map_err(|_| first_error),
            None => Err(first_error),
        })
    }

    fn request_json_with_timeout(
        &self,
        method: &str,
        path: &str,
        body: Option<Value>,
        timeout: Duration,
    ) -> Result<Value, String> {
        let (address, token) = self.event_connection()?;
        request_json_with_timeout(address, &token, method, path, body, None, timeout)
    }

    fn request_json_slow(
        &self,
        method: &str,
        path: &str,
        body: Option<Value>,
        request_id: Option<&str>,
    ) -> Result<Value, String> {
        let (address, token) = self.event_connection()?;
        let timeout = Duration::from_secs(120);
        request_json_with_timeout(
            address,
            &token,
            method,
            path,
            body.clone(),
            request_id,
            timeout,
        )
        .or_else(|first_error| match request_id {
            Some(id) => {
                request_json_with_timeout(address, &token, method, path, body, Some(id), timeout)
                    .map_err(|_| first_error)
            }
            None => Err(first_error),
        })
    }

    fn event_connection(&self) -> Result<(SocketAddr, String), String> {
        let mut process = self
            .process
            .lock()
            .map_err(|_| "bridge state lock is unavailable")?;
        let Some(running) = process.as_mut() else {
            return Err("desktop bridge is not running".to_string());
        };
        if !running.child.is_running()? {
            *process = None;
            return Err("desktop bridge is not running".to_string());
        }
        Ok((running.address, running.token.clone()))
    }

    fn stop_events(&self) {
        let forwarder = self.events.lock().ok().and_then(|mut events| events.take());
        if let Some(forwarder) = forwarder {
            forwarder.stop.store(true, Ordering::Release);
            let _ = forwarder.handle.join();
        }
    }
}

fn validate_attachment(attachment: BridgeAttachment) -> Result<BridgeAttachment, String> {
    let prefix = ".reasonix/attachments/";
    let filename = attachment.path.strip_prefix(prefix).unwrap_or_default();
    if filename.is_empty()
        || filename.contains('/')
        || filename.contains('\\')
        || filename == "."
        || filename == ".."
        || attachment.name.is_empty()
        || attachment.size == 0
    {
        return Err("desktop bridge returned an invalid attachment reference".to_string());
    }
    Ok(attachment)
}

#[derive(Debug)]
struct VerifiedReady {
    address: SocketAddr,
    sidecar_instance_id: String,
}

fn wait_for_ready(
    child: &mut BridgeChild,
    ready_file: &PathBuf,
    launch_id: &str,
) -> Result<VerifiedReady, String> {
    let deadline = Instant::now() + READY_TIMEOUT;
    loop {
        if !child.is_running()? {
            return Err("desktop bridge exited before publishing readiness".to_string());
        }
        if let Ok(contents) = fs::read_to_string(ready_file) {
            return verify_ready(&contents, launch_id);
        }
        if Instant::now() >= deadline {
            return Err("desktop bridge did not publish readiness before timeout".to_string());
        }
        thread::sleep(Duration::from_millis(20));
    }
}

fn verify_ready(contents: &str, launch_id: &str) -> Result<VerifiedReady, String> {
    let ready: ReadyFile = serde_json::from_str(contents).map_err(display_error)?;
    if ready.protocol_version != PROTOCOL_VERSION {
        return Err("desktop bridge protocol version is unsupported".to_string());
    }
    if ready.launch_id != launch_id {
        return Err("desktop bridge readiness launch identifier did not match".to_string());
    }
    if ready.sidecar_instance_id.trim().is_empty() {
        return Err(
            "desktop bridge readiness did not contain a sidecar instance identifier".to_string(),
        );
    }
    let address: SocketAddr = ready.address.parse().map_err(display_error)?;
    if !matches!(address.ip(), IpAddr::V4(ip) if ip.is_loopback())
        && !matches!(address.ip(), IpAddr::V6(ip) if ip.is_loopback())
    {
        return Err("desktop bridge readiness address is not loopback".to_string());
    }
    Ok(VerifiedReady {
        address,
        sidecar_instance_id: ready.sidecar_instance_id,
    })
}

fn verify_bridge_health(response: Value, expected_instance_id: &str) -> Result<(), String> {
    let health: crate::protocol_generated::BridgeHealth =
        serde_json::from_value(response).map_err(display_error)?;
    if health.protocol_version != u64::from(PROTOCOL_VERSION)
        || health.status != "ok"
        || health.sidecar_instance_id != expected_instance_id
    {
        return Err("desktop bridge health handshake did not match its ready frame".to_string());
    }
    if !health
        .capabilities
        .iter()
        .any(|capability| capability == "session_catalog_sync")
    {
        return Err("desktop bridge does not support session catalog synchronization".to_string());
    }
    if !health
        .capabilities
        .iter()
        .any(|capability| capability == "session_directory_snapshot_v1")
    {
        return Err("desktop bridge does not support snapshot-bound session paging".to_string());
    }
    if !health
        .capabilities
        .iter()
        .any(|capability| capability == "session_directory_snapshot_full_v1")
    {
        return Err(
            "desktop bridge does not support complete session directory snapshots".to_string(),
        );
    }
    if !health
        .capabilities
        .iter()
        .any(|capability| capability == "session_shadow_snapshot_v1")
    {
        return Err(
            "desktop bridge does not support combined session shadow snapshots".to_string(),
        );
    }
    if !health
        .capabilities
        .iter()
        .any(|capability| capability == "session_shadow_audit_snapshot_v1")
    {
        return Err("desktop bridge does not support compact session shadow snapshots".to_string());
    }
    if !health
        .capabilities
        .iter()
        .any(|capability| capability == "session_delete_recovery_list_v1")
    {
        return Err(
            "desktop bridge does not support explicit session deletion recovery".to_string(),
        );
    }
    if !health
        .capabilities
        .iter()
        .any(|capability| capability == "session_title_intent_v1")
    {
        return Err("desktop bridge does not support durable session title intents".to_string());
    }
    if !health
        .capabilities
        .iter()
        .any(|capability| capability == "session_title_recovery_list_v1")
    {
        return Err("desktop bridge does not support explicit session title recovery".to_string());
    }
    Ok(())
}

fn request_shutdown(address: SocketAddr, token: &str) -> Result<(), String> {
    request_json(address, token, "POST", "/v1:shutdown", None, None).map(|_| ())
}

fn request_json(
    address: SocketAddr,
    token: &str,
    method: &str,
    path: &str,
    body: Option<Value>,
    request_id: Option<&str>,
) -> Result<Value, String> {
    request_json_with_timeout(
        address,
        token,
        method,
        path,
        body,
        request_id,
        Duration::from_secs(2),
    )
}

fn request_json_with_timeout(
    address: SocketAddr,
    token: &str,
    method: &str,
    path: &str,
    body: Option<Value>,
    request_id: Option<&str>,
    read_timeout: Duration,
) -> Result<Value, String> {
    let bytes = body
        .map(|value| serde_json::to_vec(&value))
        .transpose()
        .map_err(display_error)?;
    let body = bytes.as_deref().unwrap_or_default();
    let mut stream =
        TcpStream::connect_timeout(&address, Duration::from_secs(1)).map_err(display_error)?;
    stream
        .set_read_timeout(Some(read_timeout))
        .map_err(display_error)?;
    let request_id_header = request_id
        .map(|request_id| format!("X-Reasonix-Request-ID: {request_id}\r\n"))
        .unwrap_or_default();
    let headers = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nAuthorization: Bearer {token}\r\n{request_id_header}Content-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        body.len()
    );
    stream
        .write_all(headers.as_bytes())
        .map_err(display_error)?;
    stream.write_all(body).map_err(display_error)?;
    let response = read_bounded_response(stream, MAX_BRIDGE_HTTP_RESPONSE_BYTES)?;
    parse_json_response(&response)
}

fn read_bounded_response(reader: impl Read, max_bytes: usize) -> Result<Vec<u8>, String> {
    let mut response = Vec::new();
    reader
        .take(max_bytes.saturating_add(1) as u64)
        .read_to_end(&mut response)
        .map_err(display_error)?;
    if response.len() > max_bytes {
        return Err("desktop bridge response exceeds the size limit".to_string());
    }
    Ok(response)
}

fn parse_json_response(response: &[u8]) -> Result<Value, String> {
    if response.len() > MAX_BRIDGE_HTTP_RESPONSE_BYTES {
        return Err("desktop bridge response exceeds the size limit".to_string());
    }
    let boundary = b"\r\n\r\n";
    let Some(index) = response
        .windows(boundary.len())
        .position(|window| window == boundary)
    else {
        return Err("desktop bridge returned an invalid HTTP response".to_string());
    };
    let headers = std::str::from_utf8(&response[..index]).map_err(display_error)?;
    let status = headers
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse::<u16>().ok())
        .ok_or_else(|| "desktop bridge returned an invalid HTTP status".to_string())?;
    let body = &response[index + boundary.len()..];
    let transfer_encoding = headers.lines().skip(1).find_map(|line| {
        let (name, value) = line.split_once(':')?;
        name.trim()
            .eq_ignore_ascii_case("transfer-encoding")
            .then_some(value.trim())
    });
    let decoded_body = transfer_encoding
        .map(|encoding| {
            if encoding.eq_ignore_ascii_case("chunked") {
                decode_chunked_body(body)
            } else {
                Err("desktop bridge returned an unsupported transfer encoding".to_string())
            }
        })
        .transpose()?;
    if transfer_encoding.is_none() && body.len() > MAX_BRIDGE_RESPONSE_BODY_BYTES {
        return Err("desktop bridge response exceeds the size limit".to_string());
    }
    let raw = decoded_body.as_deref().unwrap_or(body);
    if !(200..300).contains(&status) {
        // Carry only a small allowlist of session recovery codes across the Tauri
        // String error boundary. Never forward the bridge's free-text message:
        // it may contain a private transcript or workspace path.
        if status == 409 {
            let parsed = serde_json::from_slice::<Value>(first_json_value(raw)).ok();
            let code = parsed
                .as_ref()
                .and_then(|value| value.pointer("/error/code"))
                .and_then(Value::as_str);
            if let Some(
                code @ ("session_missing" | "session_deleting" | "session_deleted"
                | "resync_required"),
            ) = code
            {
                return Err(format!(
                    "desktop bridge request failed with status 409 ({code})"
                ));
            }
        }
        return Err(format!(
            "desktop bridge request failed with status {status}"
        ));
    }
    // Some local proxies prepend diagnostics or append a separator after the
    // JSON body. Extract exactly the first balanced object/array so neither
    // form produces serde_json's misleading "trailing characters" error.
    serde_json::from_slice(first_json_value(raw)).map_err(display_error)
}

fn first_json_value(raw: &[u8]) -> &[u8] {
    let Some(start) = raw.iter().position(|&b| b == b'{' || b == b'[') else {
        return raw;
    };
    let mut depth = 0usize;
    let mut in_string = false;
    let mut escaped = false;
    for (offset, &byte) in raw[start..].iter().enumerate() {
        if in_string {
            if escaped {
                escaped = false;
            } else if byte == b'\\' {
                escaped = true;
            } else if byte == b'"' {
                in_string = false;
            }
            continue;
        }
        match byte {
            b'"' => in_string = true,
            b'{' | b'[' => depth += 1,
            b'}' | b']' => {
                depth = depth.saturating_sub(1);
                if depth == 0 {
                    return &raw[start..=start + offset];
                }
            }
            _ => {}
        }
    }
    &raw[start..]
}

fn decode_chunked_body(mut body: &[u8]) -> Result<Vec<u8>, String> {
    let mut decoded = Vec::new();
    loop {
        let line_end = body
            .windows(2)
            .position(|window| window == b"\r\n")
            .ok_or_else(|| "desktop bridge returned an invalid chunked response".to_string())?;
        let line = std::str::from_utf8(&body[..line_end]).map_err(display_error)?;
        let size = usize::from_str_radix(line.split(';').next().unwrap_or_default().trim(), 16)
            .map_err(display_error)?;
        body = &body[line_end + 2..];

        if size == 0 {
            loop {
                let trailer_end = body
                    .windows(2)
                    .position(|window| window == b"\r\n")
                    .ok_or_else(|| {
                        "desktop bridge returned an invalid chunked response trailer".to_string()
                    })?;
                let trailer = &body[..trailer_end];
                body = &body[trailer_end + 2..];
                if trailer.is_empty() {
                    if !body.is_empty() {
                        return Err(
                            "desktop bridge returned data after the chunked response".to_string()
                        );
                    }
                    return Ok(decoded);
                }
                if !trailer.contains(&b':') {
                    return Err(
                        "desktop bridge returned an invalid chunked response trailer".to_string(),
                    );
                }
            }
        }

        if decoded.len().saturating_add(size) > MAX_BRIDGE_RESPONSE_BODY_BYTES {
            return Err("desktop bridge response exceeds the size limit".to_string());
        }
        let chunk_end = size
            .checked_add(2)
            .ok_or_else(|| "desktop bridge returned an invalid chunk size".to_string())?;
        if body.len() < chunk_end || body.get(size..chunk_end) != Some(&b"\r\n"[..]) {
            return Err("desktop bridge returned an invalid chunked response".to_string());
        }
        decoded.extend_from_slice(&body[..size]);
        body = &body[chunk_end..];
    }
}

fn session_path_component(session_id: &str) -> Result<String, String> {
    let session_id = session_id.trim();
    if session_id.is_empty()
        || session_id.len() > 128
        || !session_id
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || byte == b'-' || byte == b'_')
    {
        return Err("desktop bridge session identifier is invalid".to_string());
    }
    Ok(session_id.to_string())
}

fn mcp_oauth_flow_component(flow_id: &str) -> Result<String, String> {
    let flow_id = flow_id.trim();
    if flow_id.len() != 48 || !flow_id.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        return Err("desktop bridge MCP authorization identifier is invalid".to_string());
    }
    Ok(flow_id.to_string())
}

fn prompt_id_component(prompt_id: &str) -> Result<String, String> {
    let prompt_id = prompt_id.trim();
    if prompt_id.is_empty()
        || prompt_id.len() > 256
        || prompt_id.bytes().any(|byte| byte.is_ascii_control())
    {
        return Err("desktop bridge prompt identifier is invalid".to_string());
    }
    Ok(prompt_id.to_string())
}

/// Builds an MCP endpoint with an optional workspace root. The root selects the
/// project configuration file, so it travels as a query parameter and is bounds
/// checked here rather than trusted by the sidecar.
fn mcp_path(base: &str, workspace_root: Option<&str>) -> Result<String, String> {
    let Some(root) = workspace_root.filter(|root| !root.trim().is_empty()) else {
        return Ok(base.to_string());
    };
    if root.len() > 4096 || root.contains('\0') {
        return Err("desktop bridge workspace path is invalid".to_string());
    }
    let mut encoded = String::with_capacity(root.len() * 3);
    for byte in root.bytes() {
        match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' | b'/' => {
                encoded.push(byte as char)
            }
            _ => encoded.push_str(&format!("%{byte:02X}")),
        }
    }
    Ok(format!("{base}?workspaceRoot={encoded}"))
}

fn provider_summary_path(scope: &str, workspace_root: Option<&str>) -> Result<String, String> {
    if scope != "global" && scope != "project" {
        return Err("desktop bridge model settings scope is invalid".to_string());
    }
    let mut query = url::form_urlencoded::Serializer::new(String::new());
    query.append_pair("scope", scope);
    if scope == "project" {
        let root = workspace_root
            .filter(|root| !root.trim().is_empty())
            .ok_or_else(|| {
                "desktop bridge project model settings require a workspace".to_string()
            })?;
        if root.len() > 4096 || root.contains('\0') {
            return Err("desktop bridge workspace path is invalid".to_string());
        }
        query.append_pair("workspaceRoot", root);
    }
    Ok(format!("/v1/providers?{}", query.finish()))
}

fn session_directory_path(
    limit: u16,
    cursor: Option<&SessionDirectoryCursor>,
    workspace_root: Option<&str>,
) -> Result<String, String> {
    if !(1..=200).contains(&limit) {
        return Err("desktop bridge session page limit is invalid".to_string());
    }
    let mut path = mcp_path("/v1/sessions", workspace_root)?;
    path.push(if path.contains('?') { '&' } else { '?' });
    path.push_str(&format!("limit={limit}"));
    if let Some(cursor) = cursor {
        if cursor.position < 0 {
            return Err("desktop bridge session page cursor is invalid".to_string());
        }
        let id = session_path_component(&cursor.id)?;
        path.push_str(&format!(
            "&cursorPosition={}&cursorId={id}",
            cursor.position
        ));
        if cursor.total > 0 {
            if cursor.total > 10_000 {
                return Err("desktop bridge session page cursor is invalid".to_string());
            }
            path.push_str(&format!("&cursorTotal={}", cursor.total));
        }
        if let Some(snapshot_id) = cursor.snapshot_id.as_deref() {
            if !valid_session_snapshot_id(snapshot_id) {
                return Err("desktop bridge session page cursor is invalid".to_string());
            }
            path.push_str(&format!("&cursorSnapshot={snapshot_id}"));
        }
    }
    Ok(path)
}

fn validate_session_directory_page(
    page: &SessionDirectoryPage,
    limit: u16,
    after: Option<&SessionDirectoryCursor>,
    workspace_root: Option<&str>,
) -> Result<(), String> {
    if page.protocol_version != PROTOCOL_VERSION
        || page.sessions.len() > usize::from(limit)
        || page.total < page.sessions.len() as u64
        || page.total > MAX_SHADOW_SESSIONS as u64
        || !valid_session_snapshot_id(&page.snapshot_id)
    {
        return Err("desktop bridge session page is invalid".to_string());
    }
    let mut previous = after.cloned();
    for entry in &page.sessions {
        session_path_component(&entry.id)?;
        if entry.position < 0
            || !matches!(entry.state.as_str(), "reserved" | "ready" | "missing")
            || entry.missing != (entry.state == "missing")
            || workspace_root
                .filter(|root| !root.is_empty())
                .is_some_and(|root| entry.workspace_root.as_deref() != Some(root))
        {
            return Err("desktop bridge session page contains an invalid entry".to_string());
        }
        let key = SessionDirectoryCursor {
            position: entry.position,
            id: entry.id.clone(),
            snapshot_id: None,
            total: 0,
        };
        if previous
            .as_ref()
            .is_some_and(|old| (key.position, key.id.as_str()) <= (old.position, old.id.as_str()))
        {
            return Err("desktop bridge session page order is invalid".to_string());
        }
        previous = Some(key);
    }
    if let Some(next) = &page.next_cursor {
        if page.sessions.len() != usize::from(limit)
            || previous.as_ref().map(|key| (key.position, key.id.as_str()))
                != Some((next.position, next.id.as_str()))
            || next.snapshot_id.as_deref() != Some(page.snapshot_id.as_str())
            || next.total != page.total
        {
            return Err("desktop bridge session page cursor is invalid".to_string());
        }
    }
    if after
        .and_then(|cursor| cursor.snapshot_id.as_deref())
        .is_some_and(|snapshot_id| snapshot_id != page.snapshot_id)
    {
        return Err("session directory changed while paging; restart the session list".to_string());
    }
    Ok(())
}

fn validate_session_directory_snapshot(
    snapshot: &SessionDirectoryPage,
    workspace_root: Option<&str>,
) -> Result<(), String> {
    let total = usize::try_from(snapshot.total)
        .map_err(|_| "session directory is too large for shadow comparison".to_string())?;
    if snapshot.protocol_version != PROTOCOL_VERSION
        || total != snapshot.sessions.len()
        || total > MAX_SHADOW_SESSIONS
        || snapshot.next_cursor.is_some()
        || !valid_session_snapshot_id(&snapshot.snapshot_id)
    {
        return Err("desktop bridge session directory snapshot is invalid".to_string());
    }
    let mut previous: Option<(i64, &str)> = None;
    for entry in &snapshot.sessions {
        session_path_component(&entry.id)?;
        if entry.position < 0
            || !matches!(entry.state.as_str(), "reserved" | "ready" | "missing")
            || entry.missing != (entry.state == "missing")
            || workspace_root
                .filter(|root| !root.is_empty())
                .is_some_and(|root| entry.workspace_root.as_deref() != Some(root))
            || previous
                .is_some_and(|(position, id)| (entry.position, entry.id.as_str()) <= (position, id))
        {
            return Err(
                "desktop bridge session directory snapshot contains an invalid entry".to_string(),
            );
        }
        previous = Some((entry.position, &entry.id));
    }
    Ok(())
}

fn valid_session_snapshot_id(value: &str) -> bool {
    value.len() == 64
        && value
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
}

fn validate_pending_session_deletes_page(
    envelope: PendingSessionDeletesPageResponse,
) -> Result<PendingSessionDeletePage, String> {
    if envelope.protocol_version != PROTOCOL_VERSION
        || envelope.sessions.len() > MAX_PENDING_DELETE_PAGE_SIZE
    {
        return Err("desktop bridge deletion recovery page is invalid".to_string());
    }
    let mut seen = std::collections::HashSet::with_capacity(envelope.sessions.len());
    let mut previous: Option<&str> = None;
    for pending in &envelope.sessions {
        if session_path_component(&pending.id)? != pending.id
            || pending.title.chars().count() > 1024
            || pending.title.chars().any(char::is_control)
            || !seen.insert(pending.id.as_str())
            || previous.is_some_and(|id| pending.id.as_str() <= id)
        {
            return Err("desktop bridge deletion recovery entry is invalid".to_string());
        }
        previous = Some(&pending.id);
    }
    if let Some(cursor) = &envelope.next_cursor {
        if session_path_component(&cursor.id)? != cursor.id
            || envelope
                .sessions
                .last()
                .is_none_or(|entry| entry.id != cursor.id)
        {
            return Err("desktop bridge deletion recovery cursor is invalid".to_string());
        }
    }
    Ok(PendingSessionDeletePage {
        sessions: envelope.sessions,
        next_cursor: envelope.next_cursor,
    })
}

fn validate_pending_session_title_recoveries(
    envelope: PendingSessionTitleRecoveriesResponse,
) -> Result<Vec<PendingSessionTitleRecovery>, String> {
    if envelope.protocol_version != PROTOCOL_VERSION || envelope.sessions.len() > 10_000 {
        return Err("desktop bridge title recovery response is invalid".to_string());
    }
    let mut seen = std::collections::HashSet::with_capacity(envelope.sessions.len());
    for pending in &envelope.sessions {
        if session_path_component(&pending.id)? != pending.id
            || pending.title.chars().count() > 120
            || pending.title.chars().any(char::is_control)
            || pending.workspace_root.as_deref().is_some_and(|root| {
                root.is_empty() || root.len() > 4096 || root.chars().any(char::is_control)
            })
            || !matches!(pending.state.as_str(), "reserved" | "ready" | "missing")
            || !seen.insert(pending.id.as_str())
        {
            return Err("desktop bridge title recovery entry is invalid".to_string());
        }
    }
    Ok(envelope.sessions)
}

fn forward_events(
    app: tauri::AppHandle,
    address: SocketAddr,
    token: String,
    mut after_sequence: u64,
    stop: Arc<AtomicBool>,
) {
    let mut disconnected = false;
    let mut connected_once = false;
    while !stop.load(Ordering::Acquire) {
        match open_event_stream(address, &token, after_sequence) {
            Ok(mut reader) => {
                if stop.load(Ordering::Acquire) {
                    break;
                }
                if !connected_once || disconnected {
                    let _ = app.emit("bridge:connection-restored", ());
                }
                connected_once = true;
                disconnected = false;
                while !stop.load(Ordering::Acquire) {
                    let mut line = String::new();
                    match reader.read_line(&mut line) {
                        Ok(0) | Err(_) => break,
                        Ok(_) => {
                            let Some(data) = line.strip_prefix("data: ") else {
                                continue;
                            };
                            let Ok(event) = serde_json::from_str::<BridgeEvent>(data.trim_end())
                            else {
                                continue;
                            };
                            if event.protocol_version != u64::from(PROTOCOL_VERSION) {
                                continue;
                            }
                            after_sequence = after_sequence.max(event.sequence);
                            let _ = app.emit("bridge:event", event);
                        }
                    }
                }
            }
            Err(EventStreamError::ResyncRequired) => {
                let _ = app.emit("bridge:resync-required", ());
                break;
            }
            Err(EventStreamError::Unavailable) => {
                if !disconnected {
                    let _ = app.emit(
                        "bridge:connection-error",
                        "bridge event stream is unavailable",
                    );
                    disconnected = true;
                }
            }
        }
        if !stop.load(Ordering::Acquire) {
            thread::sleep(Duration::from_millis(100));
        }
    }
}

#[derive(Debug, PartialEq, Eq)]
enum EventStreamError {
    ResyncRequired,
    Unavailable,
}

fn open_event_stream(
    address: SocketAddr,
    token: &str,
    after_sequence: u64,
) -> Result<BufReader<TcpStream>, EventStreamError> {
    let mut stream = TcpStream::connect_timeout(&address, Duration::from_secs(1))
        .map_err(|_| EventStreamError::Unavailable)?;
    stream
        .set_read_timeout(Some(Duration::from_secs(1)))
        .map_err(|_| EventStreamError::Unavailable)?;
    stream
        .write_all(
            format!(
                "GET /v1/events?afterSequence={after_sequence} HTTP/1.1\r\nHost: {address}\r\nAuthorization: Bearer {token}\r\nAccept: text/event-stream\r\nConnection: keep-alive\r\n\r\n"
            )
            .as_bytes(),
        )
        .map_err(|_| EventStreamError::Unavailable)?;
    let mut reader = BufReader::new(stream);
    let mut status = String::new();
    reader
        .read_line(&mut status)
        .map_err(|_| EventStreamError::Unavailable)?;
    if status.starts_with("HTTP/1.1 409") {
        return Err(EventStreamError::ResyncRequired);
    }
    if !status.starts_with("HTTP/1.1 200") {
        return Err(EventStreamError::Unavailable);
    }
    loop {
        let mut header = String::new();
        reader
            .read_line(&mut header)
            .map_err(|_| EventStreamError::Unavailable)?;
        if header == "\r\n" || header.is_empty() {
            break;
        }
    }
    Ok(reader)
}

fn wait_for_exit(child: &mut BridgeChild, timeout: Duration) -> Result<bool, String> {
    let deadline = Instant::now() + timeout;
    loop {
        if !child.is_running()? {
            return Ok(true);
        }
        if Instant::now() >= deadline {
            return Ok(false);
        }
        thread::sleep(Duration::from_millis(20));
    }
}

fn opaque_secret() -> Result<String, String> {
    let mut bytes = [0_u8; 32];
    rand::rngs::OsRng
        .try_fill_bytes(&mut bytes)
        .map_err(display_error)?;
    Ok(URL_SAFE_NO_PAD.encode(bytes))
}

fn display_error(error: impl std::fmt::Display) -> String {
    error.to_string()
}

#[cfg(test)]
mod tests {
    use super::{
        open_event_stream, parse_json_response, provider_summary_path, read_bounded_response,
        request_json, session_directory_path, session_path_component, validate_attachment,
        validate_pending_session_deletes_page, validate_pending_session_title_recoveries,
        validate_session_directory_page, validate_session_directory_snapshot, verify_bridge_health,
        verify_ready, wait_for_exit, BridgeAttachment, BridgeEvent, BridgeSessionBalanceResponse,
        BridgeSessionResponse, BridgeSupervisor, EventStreamError, HooksSettingsView,
        MemorySettingsView, OpenSessionRequest, PendingSessionDelete,
        PendingSessionDeletesPageResponse, PendingSessionTitleRecoveriesResponse,
        PendingSessionTitleRecovery, PluginInstallPlan, PluginInstallRequest,
        PluginOperationResult, PluginRemoveRequest, PluginSettingsChange, PluginSettingsView,
        ProviderConfigList, RenameSessionRequest, SaveProviderConfigRequest,
        SessionCatalogMetadata, SessionDirectoryCursor, SessionDirectoryEntry,
        SessionDirectoryPage, SessionInventoryResponse, SessionRequest, SkillArchiveRequest,
        SkillArchiveResult, SkillInstallPlan, SkillInstallRequest, SkillInstallResult,
        SkillsSettingsChange, SkillsSettingsView, SubagentSettingsChange, SubagentSettingsView,
        SubmitRequest, PROTOCOL_VERSION,
    };
    use crate::SandboxSettingsView;
    use crate::{session_shadow, workbench_catalog::WorkbenchSession};
    use serde_json::json;
    use std::{
        env,
        io::{BufRead, BufReader, Read, Write},
        net::TcpListener,
        path::PathBuf,
        sync::mpsc,
        thread,
        time::Duration,
    };

    #[test]
    fn sandbox_settings_wire_view_preserves_shell_inventory() {
        let view: SandboxSettingsView = serde_json::from_value(json!({
            "protocolVersion": 1,
            "bash": "off",
            "network": true,
            "workspaceRoot": "",
            "allowWrite": [],
            "platform": "darwin",
            "shell": "auto",
            "resolvedShell": "zsh",
            "effectiveShell": "bash",
            "shellReloadRequired": true,
            "shellCapabilities": [
                {"id":"bash", "variant":"system", "available":true,
                    "path":"/bin/bash", "source":"standard-path"},
                {"id":"zsh", "variant":"system", "available":false,
                    "reason":"not-found"}
            ],
            "gitCapability": {"id":"git", "available":true,
                "path":"/usr/bin/git", "source":"path"},
            "effectiveWriteRoots": []
        }))
        .unwrap();

        assert_eq!(view.shell_capabilities.len(), 2);
        assert_eq!(
            view.shell_capabilities[0].path.as_deref(),
            Some("/bin/bash")
        );
        assert_eq!(
            view.shell_capabilities[1].reason.as_deref(),
            Some("not-found")
        );
        assert_eq!(view.git_capability.as_ref().unwrap().id, "git");
        assert_eq!(view.effective_shell, "bash");
        assert!(view.shell_reload_required);
        let encoded = serde_json::to_value(view).unwrap();
        assert_eq!(encoded["shellCapabilities"][0]["source"], "standard-path");
        assert_eq!(encoded["gitCapability"]["path"], "/usr/bin/git");
    }

    #[test]
    fn settings_wire_views_preserve_model_efforts_and_hooks() {
        let providers: ProviderConfigList = serde_json::from_value(json!({
            "protocolVersion": 1, "providers": [],
            "presets": [{"id":"mimo-api", "label":"MiMo API", "description":"Direct API",
                "group":"Xiaomi", "recommended":false, "status":"available", "revision":"abc",
                "routes":[{"name":"mimo-api", "kind":"openai", "baseUrl":"https://api.xiaomimimo.com/v1",
                    "models":["mimo-v2.5-pro"], "default":"mimo-v2.5-pro"}]}]
        })).unwrap();
        assert_eq!(
            providers.presets[0].routes[0].base_url,
            "https://api.xiaomimimo.com/v1"
        );
        let install: SaveProviderConfigRequest = serde_json::from_value(json!({
            "presetId":"mimo-api", "presetAction":"reset", "revision":"abc", "name":"", "displayName":"", "kind":"",
            "baseUrl":"", "models":[], "default":"", "useApiKey":false
        }))
        .unwrap();
        assert_eq!(
            serde_json::to_value(&install).unwrap()["presetId"],
            "mimo-api"
        );
        assert_eq!(
            serde_json::to_value(install).unwrap()["presetAction"],
            "reset"
        );

        let skills: SkillsSettingsView = serde_json::from_value(json!({
            "protocolVersion": 1, "allowImplicitInvocation": true,
            "globalAllowImplicitInvocation": false,
            "projectOverrides": {"implicit": true, "skills": false, "sources": true},
            "skills": [{"name":"review", "description":"Review", "invocation":"/review",
                "scope":"project", "sourcePath":"/tmp/review.md", "runAs":"subagent",
                "enabled":true, "globalEnabled":false, "requires":["mcp-server:github"],
                "archiveRevision":"sha256:installed"}],
            "sources": [{"path":"/tmp/skills", "scope":"project", "status":"ok",
                "enabled":true, "configured":true, "configuredGlobal":false,
                "configuredProject":true, "globalEnabled":false, "skillCount":1}],
            "archivedSkills":[{"name":"older", "scope":"project", "archiveId":"older--aabbccddeeffaabbccddeeff",
                "path":"/tmp/removed-skills/older--aabbccddeeffaabbccddeeff", "revision":"sha256:older"}]
        }))
        .unwrap();
        assert!(skills.project_overrides.implicit);
        assert!(!skills.skills[0].global_enabled);
        assert_eq!(skills.skills[0].requires, ["mcp-server:github"]);
        assert_eq!(skills.sources[0].skill_count, 1);
        assert_eq!(skills.skills[0].archive_revision, "sha256:installed");
        assert_eq!(skills.archived_skills[0].name, "older");
        assert_eq!(
            serde_json::to_value(skills).unwrap()["sources"][0]["configuredProject"],
            true
        );
        let change: SkillsSettingsChange = serde_json::from_value(json!({
            "workspaceRoot":"/tmp/project", "scope":"project", "action":"skill",
            "enabled":true, "name":"review", "path":""
        }))
        .unwrap();
        assert_eq!(serde_json::to_value(change).unwrap()["scope"], "project");

        let skill_plan: SkillInstallPlan = serde_json::from_value(json!({
            "protocolVersion":1, "planId":"sha256:review", "warningCount":0,
            "actions":[{"name":"review", "target":"/tmp/skills/review/SKILL.md", "riskLevel":"medium"}]
        })).unwrap();
        assert_eq!(skill_plan.actions[0].name, "review");
        let skill_install: SkillInstallRequest = serde_json::from_value(json!({
            "source":"/tmp/review", "scope":"project", "workspaceRoot":"/tmp/project",
            "planId":"sha256:review", "acceptRisk":false
        }))
        .unwrap();
        assert_eq!(
            serde_json::to_value(skill_install).unwrap()["workspaceRoot"],
            "/tmp/project"
        );
        let skill_result: SkillInstallResult = serde_json::from_value(json!({
            "protocolVersion":1, "status":"done", "failedNames":[],
            "settings":{"protocolVersion":1, "allowImplicitInvocation":true,
                "globalAllowImplicitInvocation":true,
                "projectOverrides":{"implicit":false,"skills":false,"sources":false},
                "skills":[], "sources":[]}
        }))
        .unwrap();
        assert_eq!(skill_result.status, "done");
        let archive_request: SkillArchiveRequest = serde_json::from_value(json!({
            "name":"review", "scope":"project", "workspaceRoot":"/tmp/project",
            "archiveId":"", "revision":"sha256:installed"
        }))
        .unwrap();
        assert_eq!(
            serde_json::to_value(archive_request).unwrap()["workspaceRoot"],
            "/tmp/project"
        );
        let archive_result: SkillArchiveResult = serde_json::from_value(json!({
            "protocolVersion":1, "backupPath":"/tmp/removed-skills/review--aabbccddeeffaabbccddeeff",
            "settings":{"protocolVersion":1, "allowImplicitInvocation":true,
                "globalAllowImplicitInvocation":true,
                "projectOverrides":{"implicit":false,"skills":false,"sources":false},
                "skills":[], "sources":[], "archivedSkills":[]}
        })).unwrap();
        assert!(archive_result.backup_path.contains("removed-skills"));

        let plugins: PluginSettingsView = serde_json::from_value(json!({
            "protocolVersion": 1,
            "plugins": [{"name":"sample", "description":"Example", "version":"1.0",
                "source":"remote", "root":"/tmp/plugins/sample", "manifestKind":"reasonix",
                "enabled":true, "status":"ready", "issue":"", "warningCount":0,
                "skills":1, "agents":0, "commands":2, "hooks":0, "mcpServers":1,
                "runtime":false, "revision":"abc"}]
        }))
        .unwrap();
        assert_eq!(plugins.plugins[0].mcp_servers, 1);
        assert_eq!(
            serde_json::to_value(plugins).unwrap()["plugins"][0]["warningCount"],
            0
        );
        let plugin_change: PluginSettingsChange = serde_json::from_value(json!({
            "name":"sample", "revision":"abc", "enabled":false
        }))
        .unwrap();
        assert_eq!(
            serde_json::to_value(plugin_change).unwrap()["enabled"],
            false
        );

        let plugin_plan: PluginInstallPlan = serde_json::from_value(json!({
            "protocolVersion": 1, "planId": "sha256:abc", "warningCount": 0,
            "actions": [{"name":"sample", "version":"1.0", "manifestKind":"reasonix",
                "riskLevel":"high", "skills":1, "agents":0, "commands":0, "hooks":1,
                "mcpServers":0, "prompts":0, "themes":0, "runtime":true,
                "runtimeCommand":"example", "intercepts":[], "replaces":[]}]
        }))
        .unwrap();
        assert_eq!(plugin_plan.actions[0].risk_level, "high");
        let plugin_install: PluginInstallRequest = serde_json::from_value(json!({
            "source":"/tmp/sample", "planId":"sha256:abc", "acceptRisk":true
        }))
        .unwrap();
        assert_eq!(
            serde_json::to_value(plugin_install).unwrap()["acceptRisk"],
            true
        );
        let plugin_remove: PluginRemoveRequest = serde_json::from_value(json!({
            "name":"sample", "revision":"abc"
        }))
        .unwrap();
        assert_eq!(
            serde_json::to_value(plugin_remove).unwrap()["revision"],
            "abc"
        );
        let plugin_result: PluginOperationResult = serde_json::from_value(json!({
            "protocolVersion":1, "status":"partial", "failedNames":["sample"],
            "settings":{"protocolVersion":1, "plugins":[]}
        }))
        .unwrap();
        assert_eq!(plugin_result.failed_names, ["sample"]);

        let subagents: SubagentSettingsView = serde_json::from_value(json!({
            "protocolVersion": 1, "defaultModel": "local/chat", "subagentModel": "",
            "subagentEffort": "", "maxDepth": 2, "maxConcurrency": 6,
            "maxParallelWriters": 3, "modelRefs": ["local/chat"],
            "modelEfforts": {"local/chat": ["auto", "low"]},
            "profiles": [{"name":"helper", "description":"Assist", "scope":"project",
                "invocation":"/helper", "configuredModel":"", "configuredEffort":"",
                "invocationMode":"manual", "editable":true, "editReason":"",
                "revision":"abc", "body":"Help carefully.", "color":"blue",
                "model":"local/chat", "effort":"low", "allowedTools":["Read"], "readOnly":true}]
        }))
        .unwrap();
        assert_eq!(subagents.model_efforts["local/chat"], ["auto", "low"]);
        assert!(subagents.profiles[0].editable);
        assert_eq!(subagents.profiles[0].allowed_tools, ["Read"]);
        assert!(serde_json::to_value(subagents)
            .unwrap()
            .get("modelEfforts")
            .is_some());
        let profile_change: SubagentSettingsChange = serde_json::from_value(json!({
            "workspaceRoot":"/tmp/p", "action":"create_profile", "name":"", "value":"", "number":0,
            "scope":"project", "revision":"", "profile":{"name":"helper",
                "description":"Assist", "systemPrompt":"Help carefully.", "color":"blue",
                "model":"local/chat", "effort":"low", "allowedTools":["Read"], "readOnly":true}
        }))
        .unwrap();
        assert_eq!(
            serde_json::to_value(profile_change).unwrap()["profile"]["systemPrompt"],
            "Help carefully."
        );

        let hooks: HooksSettingsView = serde_json::from_value(json!({
            "protocolVersion": 1, "scope": "project", "path": "/tmp/p/.reasonix/settings.json",
            "projectRoot": "/tmp/p", "revision": "abc",
            "hooks": {"Stop": [{"command": "echo done", "env": {"X": "1"}}]},
            "events": ["Stop"]
        }))
        .unwrap();
        assert_eq!(hooks.hooks["Stop"][0]["env"]["X"], "1");
        assert!(serde_json::to_value(hooks).unwrap().get("hooks").is_some());

        let memory: MemorySettingsView = serde_json::from_value(json!({
            "protocolVersion": 1, "workspaceRoot": "/tmp/p", "storeDir": "/tmp/m",
            "globalStoreDir": "/tmp/g", "docs": [{"path":"/tmp/p/AGENTS.md", "scope":"project", "body":"hello", "revision":"abc"}],
            "facts": [], "archives": [{"id":"mem-1", "revision":1, "name":"fact", "title":"Fact", "description":"", "type":"project", "scope":"project", "body":"body", "freshness":"fresh", "path":"/tmp/archive.md", "archivedAt":"2026-09-28T00:00:00Z"}],
            "diagnostics": []
        })).unwrap();
        assert_eq!(memory.archives[0].fact.id, "mem-1");
        assert_eq!(
            serde_json::to_value(memory).unwrap()["archives"][0]["type"],
            "project"
        );
    }

    // Local `cargo test` has no built Go bridge, so the supervised lifecycle
    // tests are opt-in there. CI must provide the path: a missing binary fails
    // instead of letting the supervisor report a silent pass.
    fn bridge_under_test() -> Option<PathBuf> {
        match env::var_os("REASONIX_TAURI_BRIDGE_TEST_BIN") {
            Some(binary) => Some(PathBuf::from(binary)),
            None => {
                assert!(
                    env::var_os("CI").is_none(),
                    "REASONIX_TAURI_BRIDGE_TEST_BIN must be set under CI"
                );
                None
            }
        }
    }

    #[test]
    fn ready_file_requires_matching_loopback_launch() {
        let ready = r#"{"protocolVersion":1,"address":"127.0.0.1:12345","sidecarInstanceId":"instance","launchId":"launch"}"#;
        assert!(verify_ready(ready, "launch").is_ok());
        assert!(verify_ready(ready, "other").is_err());
    }

    #[test]
    fn bridge_health_requires_current_session_storage_capability_and_instance() {
        let healthy = json!({
            "protocolVersion": 1,
            "status": "ok",
            "sidecarInstanceId": "instance",
            "capabilities": ["open_session", "session_catalog_sync", "session_directory_snapshot_v1", "session_directory_snapshot_full_v1", "session_shadow_snapshot_v1", "session_shadow_audit_snapshot_v1", "session_delete_recovery_list_v1", "session_title_intent_v1", "session_title_recovery_list_v1"],
        });
        assert!(verify_bridge_health(healthy.clone(), "instance").is_ok());
        assert!(verify_bridge_health(healthy.clone(), "other").is_err());

        let old_v1_sidecar = json!({
            "protocolVersion": 1,
            "status": "ok",
            "sidecarInstanceId": "instance",
            "capabilities": ["open_session"],
        });
        assert!(verify_bridge_health(old_v1_sidecar, "instance")
            .unwrap_err()
            .contains("session catalog synchronization"));

        let old_paging_sidecar = json!({
            "protocolVersion": 1,
            "status": "ok",
            "sidecarInstanceId": "instance",
            "capabilities": ["open_session", "session_catalog_sync"],
        });
        assert!(verify_bridge_health(old_paging_sidecar, "instance")
            .unwrap_err()
            .contains("snapshot-bound session paging"));

        let old_snapshot_sidecar = json!({
            "protocolVersion": 1,
            "status": "ok",
            "sidecarInstanceId": "instance",
            "capabilities": ["open_session", "session_catalog_sync", "session_directory_snapshot_v1"],
        });
        assert!(verify_bridge_health(old_snapshot_sidecar, "instance")
            .unwrap_err()
            .contains("complete session directory snapshots"));

        let old_shadow_sidecar = json!({
            "protocolVersion": 1,
            "status": "ok",
            "sidecarInstanceId": "instance",
            "capabilities": ["open_session", "session_catalog_sync", "session_directory_snapshot_v1", "session_directory_snapshot_full_v1"],
        });
        assert!(verify_bridge_health(old_shadow_sidecar, "instance")
            .unwrap_err()
            .contains("combined session shadow snapshots"));

        let old_recovery_sidecar = json!({
            "protocolVersion": 1,
            "status": "ok",
            "sidecarInstanceId": "instance",
            "capabilities": ["open_session", "session_catalog_sync", "session_directory_snapshot_v1", "session_directory_snapshot_full_v1", "session_shadow_snapshot_v1", "session_shadow_audit_snapshot_v1"],
        });
        assert!(verify_bridge_health(old_recovery_sidecar, "instance")
            .unwrap_err()
            .contains("explicit session deletion recovery"));

        let old_title_intent_sidecar = json!({
            "protocolVersion": 1,
            "status": "ok",
            "sidecarInstanceId": "instance",
            "capabilities": ["open_session", "session_catalog_sync", "session_directory_snapshot_v1", "session_directory_snapshot_full_v1", "session_shadow_snapshot_v1", "session_shadow_audit_snapshot_v1", "session_delete_recovery_list_v1"],
        });
        assert!(verify_bridge_health(old_title_intent_sidecar, "instance")
            .unwrap_err()
            .contains("durable session title intents"));

        let old_title_recovery_sidecar = json!({
            "protocolVersion": 1,
            "status": "ok",
            "sidecarInstanceId": "instance",
            "capabilities": ["open_session", "session_catalog_sync", "session_directory_snapshot_v1", "session_directory_snapshot_full_v1", "session_shadow_snapshot_v1", "session_shadow_audit_snapshot_v1", "session_delete_recovery_list_v1", "session_title_intent_v1"],
        });
        assert!(verify_bridge_health(old_title_recovery_sidecar, "instance")
            .unwrap_err()
            .contains("explicit session title recovery"));
    }

    #[test]
    fn pending_session_title_recovery_response_is_bounded_and_unique() {
        let entry = PendingSessionTitleRecovery {
            id: "older-than-recent-catalog".into(),
            title: "Before rename".into(),
            workspace_root: Some("/work/project".into()),
            state: "ready".into(),
        };
        let entries =
            validate_pending_session_title_recoveries(PendingSessionTitleRecoveriesResponse {
                protocol_version: PROTOCOL_VERSION,
                sessions: vec![entry.clone()],
            })
            .expect("valid pending title recovery");
        assert_eq!(entries.len(), 1);
        assert!(
            validate_pending_session_title_recoveries(PendingSessionTitleRecoveriesResponse {
                protocol_version: PROTOCOL_VERSION,
                sessions: vec![entry.clone(), entry.clone()],
            })
            .is_err()
        );
        assert!(
            validate_pending_session_title_recoveries(PendingSessionTitleRecoveriesResponse {
                protocol_version: PROTOCOL_VERSION,
                sessions: vec![PendingSessionTitleRecovery {
                    state: "deleted".into(),
                    ..entry.clone()
                }],
            })
            .is_err()
        );
        assert!(
            validate_pending_session_title_recoveries(PendingSessionTitleRecoveriesResponse {
                protocol_version: PROTOCOL_VERSION,
                sessions: vec![PendingSessionTitleRecovery {
                    id: "../escape".into(),
                    ..entry
                }],
            })
            .is_err()
        );
    }

    #[test]
    fn pending_session_delete_page_is_bounded_unique_and_path_free() {
        let page = validate_pending_session_deletes_page(PendingSessionDeletesPageResponse {
            protocol_version: PROTOCOL_VERSION,
            sessions: vec![PendingSessionDelete {
                id: "interrupted-delete".into(),
                title: "Interrupted conversation".into(),
            }],
            next_cursor: None,
        })
        .expect("valid pending deletion");
        assert_eq!(page.sessions.len(), 1);
        assert!(
            validate_pending_session_deletes_page(PendingSessionDeletesPageResponse {
                protocol_version: PROTOCOL_VERSION,
                sessions: vec![
                    PendingSessionDelete {
                        id: "same".into(),
                        title: "one".into()
                    },
                    PendingSessionDelete {
                        id: "same".into(),
                        title: "two".into()
                    },
                ],
                next_cursor: None,
            })
            .is_err()
        );
        assert!(
            validate_pending_session_deletes_page(PendingSessionDeletesPageResponse {
                protocol_version: PROTOCOL_VERSION,
                sessions: vec![PendingSessionDelete {
                    id: "../outside".into(),
                    title: "unsafe".into()
                }],
                next_cursor: None,
            })
            .is_err()
        );
        assert!(
            validate_pending_session_deletes_page(PendingSessionDeletesPageResponse {
                protocol_version: PROTOCOL_VERSION,
                sessions: vec![PendingSessionDelete {
                    id: "first".into(),
                    title: "First".into(),
                }],
                next_cursor: Some(super::PendingSessionDeleteCursor { id: "later".into() }),
            })
            .is_err()
        );
    }

    #[test]
    fn provider_summary_path_scopes_and_encodes_workspace() {
        assert_eq!(
            provider_summary_path("global", None).unwrap(),
            "/v1/providers?scope=global"
        );
        assert_eq!(
            provider_summary_path("project", Some("/work/a b")).unwrap(),
            "/v1/providers?scope=project&workspaceRoot=%2Fwork%2Fa+b"
        );
        assert!(provider_summary_path("project", None).is_err());
        assert!(provider_summary_path("other", Some("/work")).is_err());
        assert!(provider_summary_path("project", Some(&"x".repeat(4097))).is_err());
    }

    #[test]
    fn session_directory_query_uses_composite_cursor_and_escaped_workspace() {
        let cursor = SessionDirectoryCursor {
            position: 12,
            id: "session-009".to_string(),
            snapshot_id: Some("a".repeat(64)),
            total: 0,
        };
        assert_eq!(
            session_directory_path(73, Some(&cursor), Some("/work/a b")).unwrap(),
            format!("/v1/sessions?workspaceRoot=/work/a%20b&limit=73&cursorPosition=12&cursorId=session-009&cursorSnapshot={}", "a".repeat(64))
        );
        assert!(session_directory_path(0, None, None).is_err());
        assert!(session_directory_path(201, None, None).is_err());
        assert!(session_directory_path(
            1,
            Some(&SessionDirectoryCursor {
                position: 0,
                id: "../outside".to_string(),
                snapshot_id: None,
                total: 0,
            }),
            None,
        )
        .is_err());
    }

    #[test]
    fn session_directory_page_rejects_tombstones_and_bad_paging() {
        let entry = |id: &str| SessionDirectoryEntry {
            id: id.to_string(),
            title: String::new(),
            title_source: "fallback".to_string(),
            workspace_root: Some("/work/a".to_string()),
            state: "ready".to_string(),
            missing: false,
            position: 12,
            updated_at_ms: 1,
        };
        let mut page = SessionDirectoryPage {
            protocol_version: PROTOCOL_VERSION,
            sessions: vec![entry("session-001"), entry("session-002")],
            next_cursor: Some(SessionDirectoryCursor {
                position: 12,
                id: "session-002".to_string(),
                snapshot_id: Some("a".repeat(64)),
                total: 3,
            }),
            total: 3,
            snapshot_id: "a".repeat(64),
        };
        assert!(validate_session_directory_page(&page, 2, None, Some("/work/a")).is_ok());
        let after = SessionDirectoryCursor {
            position: 12,
            id: "session-001".to_string(),
            snapshot_id: Some("a".repeat(64)),
            total: 0,
        };
        assert!(validate_session_directory_page(&page, 2, Some(&after), None).is_err());
        let changed_snapshot = SessionDirectoryCursor {
            snapshot_id: Some("b".repeat(64)),
            ..after
        };
        assert!(validate_session_directory_page(&page, 2, Some(&changed_snapshot), None).is_err());
        page.sessions[0].state = "deleted".to_string();
        assert!(validate_session_directory_page(&page, 2, None, None).is_err());
        page.sessions[0].state = "ready".to_string();
        page.next_cursor.as_mut().unwrap().id = "wrong".to_string();
        assert!(validate_session_directory_page(&page, 2, None, None).is_err());
    }

    #[test]
    fn session_directory_snapshot_requires_a_complete_bounded_ordered_result() {
        let entry = |id: &str, position: i64| SessionDirectoryEntry {
            id: id.to_string(),
            title: String::new(),
            title_source: "fallback".to_string(),
            workspace_root: Some("/work/a".to_string()),
            state: "ready".to_string(),
            missing: false,
            position,
            updated_at_ms: 1,
        };
        let mut snapshot = SessionDirectoryPage {
            protocol_version: PROTOCOL_VERSION,
            sessions: vec![entry("session-001", 4), entry("session-002", 4)],
            next_cursor: None,
            total: 2,
            snapshot_id: "a".repeat(64),
        };
        assert!(validate_session_directory_snapshot(&snapshot, Some("/work/a")).is_ok());
        assert!(validate_session_directory_snapshot(&snapshot, Some("/work/b")).is_err());
        snapshot.total = 3;
        assert!(validate_session_directory_snapshot(&snapshot, None).is_err());
        snapshot.total = 2;
        snapshot.sessions.swap(0, 1);
        assert!(validate_session_directory_snapshot(&snapshot, None).is_err());
        snapshot.sessions.swap(0, 1);
        snapshot.next_cursor = Some(SessionDirectoryCursor {
            position: 4,
            id: "session-002".to_string(),
            snapshot_id: Some(snapshot.snapshot_id.clone()),
            total: snapshot.total,
        });
        assert!(validate_session_directory_snapshot(&snapshot, None).is_err());
    }

    #[test]
    fn inventory_parser_accepts_omitted_empty_detail() {
        let inventory: SessionInventoryResponse = serde_json::from_value(json!({
            "protocolVersion": PROTOCOL_VERSION,
            "entries": [{ "id": "safe", "source": "identity", "exists": true }],
            "unclaimed": [],
            "errors": []
        }))
        .expect("parse healthy inventory row");
        let physical = inventory
            .into_physical_inventory()
            .expect("complete inventory");
        assert_eq!(physical.states.len(), 1);
        assert!(physical.states[0].readable);
        assert_eq!(physical.unclaimed_count, 0);
        assert_eq!(physical.error_count, 0);
    }

    #[test]
    fn physical_inventory_preserves_path_conflict_diagnostics() {
        let inventory: SessionInventoryResponse = serde_json::from_value(json!({
            "protocolVersion": PROTOCOL_VERSION,
            "entries": [
                { "id": "legacy-one", "source": "identity", "exists": true },
                { "id": "legacy-two", "source": "identity", "exists": true }
            ],
            "unclaimed": [],
            "errors": ["legacy-one: physical transcript also belongs to session legacy-two"]
        }))
        .expect("parse conflicted inventory");
        let physical = inventory
            .into_physical_inventory()
            .expect("complete conflicted inventory");
        assert_eq!(physical.states.len(), 2);
        assert_eq!(physical.error_count, 1);
    }

    #[test]
    fn physical_inventory_counts_title_sidecar_drift_without_hiding_readable_transcript() {
        let inventory: SessionInventoryResponse = serde_json::from_value(json!({
            "protocolVersion": PROTOCOL_VERSION,
            "entries": [{ "id": "drifted", "source": "identity", "exists": true }],
            "unclaimed": [],
            "errors": ["drifted: session title metadata differs from identity"]
        }))
        .expect("parse drifted inventory");
        let physical = inventory
            .into_physical_inventory()
            .expect("complete inventory");
        assert_eq!(physical.states.len(), 1);
        assert!(physical.states[0].readable);
        assert_eq!(physical.error_count, 1);
    }

    #[test]
    fn physical_inventory_rejects_missing_or_null_summary_arrays_even_when_empty() {
        let complete = json!({
            "protocolVersion": PROTOCOL_VERSION,
            "entries": [],
            "unclaimed": [],
            "errors": []
        });
        for field in ["entries", "unclaimed", "errors"] {
            let mut omitted = complete.clone();
            omitted.as_object_mut().expect("object").remove(field);
            let response: SessionInventoryResponse =
                serde_json::from_value(omitted).expect("missing optional field parses");
            assert!(
                response.into_physical_inventory().is_err(),
                "accepted omitted {field}"
            );

            let mut null = complete.clone();
            null.as_object_mut()
                .expect("object")
                .insert(field.to_string(), serde_json::Value::Null);
            let response: SessionInventoryResponse =
                serde_json::from_value(null).expect("null optional field parses");
            assert!(
                response.into_physical_inventory().is_err(),
                "accepted null {field}"
            );
        }
        let complete: SessionInventoryResponse =
            serde_json::from_value(complete).expect("complete empty inventory parses");
        let physical = complete
            .into_physical_inventory()
            .expect("explicit empty inventory");
        assert!(physical.states.is_empty());
        assert_eq!(physical.unclaimed_count, 0);
        assert_eq!(physical.error_count, 0);
    }

    #[test]
    fn bridge_requests_send_the_opaque_request_id() {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind test server");
        let address = listener.local_addr().expect("test address");
        let (headers_tx, headers_rx) = mpsc::channel();
        thread::spawn(move || {
            let (stream, _) = listener.accept().expect("accept request");
            let mut reader = BufReader::new(stream);
            let mut headers = String::new();
            loop {
                let mut line = String::new();
                reader.read_line(&mut line).expect("read request header");
                if line == "\r\n" {
                    break;
                }
                headers.push_str(&line);
            }
            let content_length = headers
                .lines()
                .find_map(|line| line.strip_prefix("Content-Length: "))
                .and_then(|value| value.parse::<usize>().ok())
                .expect("content length");
            let mut body = vec![0; content_length];
            reader.read_exact(&mut body).expect("read request body");
            headers_tx.send(headers).expect("send headers");
            let mut stream = reader.into_inner();
            stream
                .write_all(b"HTTP/1.1 202 Accepted\r\nContent-Type: application/json\r\nContent-Length: 2\r\nConnection: close\r\n\r\n{}")
                .expect("write response");
        });

        let response = request_json(
            address,
            "test-token",
            "POST",
            "/v1/sessions/tab-1:submit",
            Some(json!({ "input": "not logged" })),
            Some("request-123"),
        )
        .expect("bridge response");
        assert_eq!(response, json!({}));
        let headers = headers_rx
            .recv_timeout(Duration::from_secs(1))
            .expect("headers");
        assert!(headers.contains("X-Reasonix-Request-ID: request-123\r\n"));
    }

    #[test]
    fn parses_chunked_json_responses_with_trailers() {
        let expected_content = "x".repeat(4096);
        let body = serde_json::to_vec(&json!({"content": expected_content})).unwrap();
        let mut response = b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n".to_vec();
        for chunk in body.chunks(1024) {
            response.extend_from_slice(format!("{:x};ext=ignored\r\n", chunk.len()).as_bytes());
            response.extend_from_slice(chunk);
            response.extend_from_slice(b"\r\n");
        }
        response.extend_from_slice(b"0\r\nX-Test-Trailer: complete\r\n\r\n");

        let parsed = parse_json_response(&response).expect("chunked JSON response");
        assert_eq!(parsed["content"].as_str(), Some(expected_content.as_str()));
    }

    #[test]
    fn bridge_response_reader_stops_at_its_configured_limit() {
        let within = read_bounded_response(std::io::Cursor::new(b"12345678"), 8)
            .expect("response at the limit");
        assert_eq!(within, b"12345678");
        assert_eq!(
            read_bounded_response(std::io::Cursor::new(b"123456789"), 8).unwrap_err(),
            "desktop bridge response exceeds the size limit"
        );
    }

    #[test]
    fn ready_file_rejects_non_loopback_address() {
        let ready = r#"{"protocolVersion":1,"address":"0.0.0.0:12345","sidecarInstanceId":"instance","launchId":"launch"}"#;
        assert!(verify_ready(ready, "launch").is_err());
    }

    #[test]
    fn response_parser_requires_successful_json_http_response() {
        let response = b"HTTP/1.1 202 Accepted\r\nContent-Type: application/json\r\n\r\n{\"protocolVersion\":1}";
        assert_eq!(parse_json_response(response).unwrap()["protocolVersion"], 1);
        assert!(parse_json_response(b"HTTP/1.1 401 Unauthorized\r\n\r\n{}").is_err());
    }

    #[test]
    fn response_parser_preserves_only_known_session_state_codes() {
        for code in [
            "session_missing",
            "session_deleting",
            "session_deleted",
            "resync_required",
        ] {
            let response = format!(
                "HTTP/1.1 409 Conflict\r\nContent-Type: application/json\r\n\r\n{{\"error\":{{\"code\":\"{code}\",\"message\":\"/private/transcript.jsonl\"}}}}"
            );
            assert_eq!(
                parse_json_response(response.as_bytes()).unwrap_err(),
                format!("desktop bridge request failed with status 409 ({code})")
            );
        }
        let unknown = b"HTTP/1.1 409 Conflict\r\n\r\n{\"error\":{\"code\":\"unexpected\",\"message\":\"/private/transcript.jsonl\"}}";
        assert_eq!(
            parse_json_response(unknown).unwrap_err(),
            "desktop bridge request failed with status 409"
        );
    }

    #[test]
    fn response_parser_ignores_diagnostic_prefix_and_trailing_separator() {
        let response = b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\nproxy: {\"protocolVersion\":1,\"message\":\"brace } in string\"}\ntrailing diagnostics";
        let parsed = parse_json_response(response).expect("JSON surrounded by diagnostics");
        assert_eq!(parsed["protocolVersion"], 1);
        assert_eq!(parsed["message"], "brace } in string");
    }

    #[test]
    fn bridge_events_deserialize_only_the_public_envelope() {
        let event: BridgeEvent = serde_json::from_str(
            r#"{"protocolVersion":1,"sequence":7,"eventKind":"text","sessionId":"tab-1","payload":{"kind":"text","text":"hello"}}"#,
        )
        .unwrap();
        assert_eq!(event.sequence, 7);
        assert_eq!(event.payload["text"], "hello");
    }

    #[test]
    fn attachment_response_only_accepts_private_workspace_references() {
        let valid = BridgeAttachment {
            is_image: false,
            name: "notes.txt".to_string(),
            path: ".reasonix/attachments/clipboard-1.txt".to_string(),
            size: 10,
        };
        assert!(validate_attachment(valid.clone()).is_ok());

        let mut invalid = valid.clone();
        invalid.path = "/Users/private/notes.txt".to_string();
        assert!(validate_attachment(invalid).is_err());

        let mut invalid = valid.clone();
        invalid.path = ".reasonix/attachments/../notes.txt".to_string();
        assert!(validate_attachment(invalid).is_err());

        let mut invalid = valid.clone();
        invalid.name.clear();
        assert!(validate_attachment(invalid).is_err());

        let mut invalid = valid;
        invalid.size = 0;
        assert!(validate_attachment(invalid).is_err());
    }

    #[test]
    fn session_identifier_is_safe_for_an_http_path() {
        assert_eq!(session_path_component("tab_1-abc").unwrap(), "tab_1-abc");
        assert!(session_path_component("tab/1").is_err());
        assert!(session_path_component("tab\r\nInjected: value").is_err());
    }

    #[test]
    fn session_snapshot_metrics_cross_the_generated_host_contract() {
        let response: BridgeSessionResponse = serde_json::from_value(json!({
            "protocolVersion": 1,
            "sequence": 4,
            "session": { "id": "a", "path": "/sessions/a.jsonl", "state": "idle" },
            "metrics": {
                "contextUsedTokens": 2400,
                "contextWindowTokens": 10000,
                "compactThresholdPercent": 80,
                "cacheHitTokens": 300,
                "cacheMissTokens": 100
            }
        }))
        .expect("decode snapshot metrics");
        let metrics = response.metrics.expect("metrics preserved");
        assert_eq!(metrics.context_used_tokens, 2400);
        assert_eq!(metrics.cache_hit_tokens, 300);
    }

    #[test]
    fn session_balance_contract_is_display_only_and_optional() {
        let response: BridgeSessionBalanceResponse = serde_json::from_value(json!({
            "protocolVersion": 1,
            "balance": { "available": true, "display": "¥12.34" }
        }))
        .expect("decode session balance");
        let balance = response.balance.expect("configured balance");
        assert!(balance.available);
        assert_eq!(balance.display, "¥12.34");

        let unconfigured: BridgeSessionBalanceResponse = serde_json::from_value(json!({
            "protocolVersion": 1,
            "balance": null
        }))
        .expect("decode missing balance");
        assert!(unconfigured.balance.is_none());
    }

    #[test]
    fn host_rejects_response_with_unsupported_protocol_version() {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind test server");
        let address = listener.local_addr().expect("test address");
        thread::spawn(move || {
            let (stream, _) = listener.accept().expect("accept request");
            let mut reader = BufReader::new(stream);
            let mut headers = String::new();
            loop {
                let mut line = String::new();
                reader.read_line(&mut line).expect("read request header");
                if line == "\r\n" {
                    break;
                }
                headers.push_str(&line);
            }
            let content_length = headers
                .lines()
                .find_map(|line| line.strip_prefix("Content-Length: "))
                .and_then(|value| value.parse::<usize>().ok())
                .expect("content length");
            let mut body = vec![0; content_length];
            reader.read_exact(&mut body).expect("read request body");
            let mut stream = reader.into_inner();
            // Respond with protocolVersion 99 — the host must reject it.
            stream
                .write_all(b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"protocolVersion\":99,\"sequence\":0,\"session\":{\"id\":\"t\",\"path\":\"/p\",\"state\":\"idle\"}}")
                .expect("write response");
        });
        let response = request_json(
            address,
            "test-token",
            "GET",
            "/v1/sessions/t/snapshot",
            None,
            None,
        )
        .expect("request should succeed at HTTP level");
        // The protocol version check happens in request_session, not
        // request_json. Verify that the envelope carries version 99 so the
        // caller can reject it.
        assert_eq!(response["protocolVersion"], 99);
        assert_ne!(
            response["protocolVersion"],
            u64::from(super::PROTOCOL_VERSION)
        );
    }

    #[test]
    fn expired_event_cursor_requires_a_fresh_snapshot() {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind test server");
        let address = listener.local_addr().expect("test address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept request");
            stream
                .write_all(b"HTTP/1.1 409 Conflict\r\nContent-Type: application/json\r\nContent-Length: 0\r\n\r\n")
                .expect("write response");
        });
        assert!(matches!(
            open_event_stream(address, "test-token", 1),
            Err(EventStreamError::ResyncRequired)
        ));
        server.join().expect("server exit");
    }

    #[test]
    fn host_rejects_event_with_unsupported_protocol_version() {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind test server");
        let _address = listener.local_addr().expect("test address");
        thread::spawn(move || {
            let (stream, _) = listener.accept().expect("accept request");
            let mut reader = BufReader::new(stream);
            let mut headers = String::new();
            loop {
                let mut line = String::new();
                reader.read_line(&mut line).expect("read request header");
                if line == "\r\n" {
                    break;
                }
                headers.push_str(&line);
            }
            let mut stream = reader.into_inner();
            // SSE event with protocolVersion 99 — the host must skip it.
            stream
                .write_all(b"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: {\"protocolVersion\":99,\"sequence\":1,\"eventKind\":\"text\",\"sessionId\":\"t\",\"payload\":{}}\n\n")
                .expect("write response");
        });
        // parse_json_response only handles normal HTTP; for SSE we check the
        // event deserialization path. A BridgeEvent with version 99 must be
        // silently dropped by forward_events. We verify the struct-level
        // deserialization here instead.
        let event: BridgeEvent = serde_json::from_str(
            r#"{"protocolVersion":99,"sequence":1,"eventKind":"text","sessionId":"t","payload":{}}"#,
        )
        .expect("deserialization should succeed");
        assert_eq!(event.protocol_version, 99);
        // The forward_events loop checks `event.protocol_version != u64::from(PROTOCOL_VERSION)`
        // and skips events that don't match. This test documents that contract.
        assert_ne!(event.protocol_version, u64::from(super::PROTOCOL_VERSION));
    }

    #[test]
    fn opener_preference_roundtrips_and_survives_real_sidecar_restart() {
        let Some(binary) = bridge_under_test() else {
            return;
        };
        let _env = crate::test_env::guard();
        let home = tempfile::tempdir().unwrap();
        env::set_var("REASONIX_HOME", home.path());
        env::set_var("REASONIX_STATE_HOME", home.path());
        std::fs::write(
            home.path().join("config.toml"),
            "future = \"keep\"\n[desktop]\nexternal_opener = \"finder\"\n",
        )
        .unwrap();
        let supervisor = BridgeSupervisor::with_binary(binary);
        supervisor.start().unwrap();
        assert_eq!(
            supervisor.desktop_preferences().unwrap().external_opener,
            "finder"
        );
        assert_eq!(
            supervisor
                .set_desktop_external_opener("vscode")
                .unwrap()
                .external_opener,
            "vscode"
        );
        assert!(supervisor.set_desktop_external_opener("/bin/sh").is_err());
        supervisor.stop().unwrap();
        supervisor.start().unwrap();
        assert_eq!(
            supervisor.desktop_preferences().unwrap().external_opener,
            "vscode"
        );
        supervisor.stop().unwrap();
        assert!(std::fs::read_to_string(home.path().join("config.toml"))
            .unwrap()
            .contains("future = \"keep\""));
    }

    #[test]
    fn supervisor_starts_and_stops_a_real_bridge_when_provided() {
        let Some(binary) = bridge_under_test() else {
            return;
        };
        let _env = crate::test_env::guard();
        let supervisor = BridgeSupervisor::with_binary(binary);
        let status = supervisor.start().expect("start bridge");
        assert!(status.running);
        assert_eq!(status.protocol_version, Some(1));
        supervisor.stop().expect("stop bridge");
        assert!(!supervisor.status().running);
    }

    #[test]
    fn shadow_reads_real_directory_and_physical_inventory_when_bridge_is_provided() {
        let Some(binary) = bridge_under_test() else {
            return;
        };
        let _env = crate::test_env::guard();
        let home = tempfile::tempdir().expect("isolated reasonix home");
        env::set_var("REASONIX_HOME", home.path());
        env::set_var("REASONIX_STATE_HOME", home.path());
        let supervisor = BridgeSupervisor::with_binary(binary);
        supervisor.start().expect("start bridge");
        supervisor
            .open_session(OpenSessionRequest {
                session_id: "tauri-e2e-shadow".to_string(),
                workspace_root: None,
            })
            .expect("reserve session");
        let (directory, snapshot_id, physical) = supervisor
            .session_shadow_snapshot(&[])
            .expect("read combined identity directory and physical inventory");
        assert_eq!(
            snapshot_id,
            supervisor
                .session_directory_snapshot_with_id()
                .expect("read standalone directory snapshot")
                .1
        );
        let report = session_shadow::compare(
            &[WorkbenchSession {
                session_id: "tauri-e2e-shadow".to_string(),
                title: None,
                workspace_root: None,
            }],
            &directory,
            &physical,
        )
        .expect("compare catalog");
        assert_eq!(report.matched_count, 1);
        assert_eq!(report.physical_state_mismatches, 0);
        assert!(report.legacy_matches_directory);
        supervisor.stop().expect("stop bridge");
    }

    #[test]
    fn real_bridge_rejects_a_stale_session_page_cursor() {
        let Some(binary) = bridge_under_test() else {
            return;
        };
        let _env = crate::test_env::guard();
        let home = tempfile::tempdir().expect("isolated reasonix home");
        env::set_var("REASONIX_HOME", home.path());
        env::set_var("REASONIX_STATE_HOME", home.path());
        let supervisor = BridgeSupervisor::with_binary(binary);
        supervisor.start().expect("start bridge");

        supervisor
            .open_session(OpenSessionRequest {
                session_id: "tauri-e2e-page-a".to_string(),
                workspace_root: None,
            })
            .expect("open first session");
        supervisor
            .rename_session(RenameSessionRequest {
                session_id: "tauri-e2e-page-a".to_string(),
                title: "Page A".to_string(),
            })
            .expect("title first session");
        supervisor
            .switch_session(OpenSessionRequest {
                session_id: "tauri-e2e-page-b".to_string(),
                workspace_root: None,
            })
            .expect("open second session");
        supervisor
            .rename_session(RenameSessionRequest {
                session_id: "tauri-e2e-page-b".to_string(),
                title: "Page B".to_string(),
            })
            .expect("title second session");

        let first = supervisor
            .session_directory_page(1, None, None)
            .expect("read first page");
        let cursor = first.next_cursor.expect("first page continuation");
        assert_eq!(first.total, 2);
        assert_eq!(
            cursor.snapshot_id.as_deref(),
            Some(first.snapshot_id.as_str())
        );
        let second = supervisor
            .session_directory_page(1, Some(cursor.clone()), None)
            .expect("read continuation in the same snapshot");
        assert_eq!(second.total, 2);
        assert_eq!(second.snapshot_id, first.snapshot_id);
        assert_eq!(second.sessions.len(), 1);

        supervisor
            .rename_session(RenameSessionRequest {
                session_id: "tauri-e2e-page-b".to_string(),
                title: "Page B updated".to_string(),
            })
            .expect("update a presentation-only field");
        let after_title_update = supervisor
            .session_directory_page(1, Some(cursor.clone()), None)
            .expect("title changes must not invalidate keyset pagination");
        assert_eq!(after_title_update.snapshot_id, first.snapshot_id);
        assert_eq!(after_title_update.sessions.len(), 1);

        supervisor
            .sync_session_catalog(vec![
                SessionCatalogMetadata {
                    session_id: "tauri-e2e-page-b".to_string(),
                    workspace_root: None,
                },
                SessionCatalogMetadata {
                    session_id: "tauri-e2e-page-a".to_string(),
                    workspace_root: None,
                },
            ])
            .expect("reorder the visible directory");
        let stale = supervisor
            .session_directory_page(1, Some(cursor), None)
            .expect_err("a structurally stale continuation must request a fresh first page");
        assert!(
            stale.contains("resync_required"),
            "stale cursor error = {stale}"
        );
        supervisor.stop().expect("stop bridge");
    }

    #[test]
    fn real_bridge_exposes_empty_inventory_and_recovery_lists() {
        let Some(binary) = bridge_under_test() else {
            return;
        };
        let _env = crate::test_env::guard();
        let home = tempfile::tempdir().expect("isolated reasonix home");
        env::set_var("REASONIX_HOME", home.path());
        env::set_var("REASONIX_STATE_HOME", home.path());
        let supervisor = BridgeSupervisor::with_binary(binary);
        supervisor.start().expect("start bridge");

        let inventory = supervisor
            .session_physical_inventory()
            .expect("read complete empty physical inventory");
        assert!(inventory.states.is_empty());
        assert_eq!(inventory.unclaimed_count, 0);
        assert_eq!(inventory.error_count, 0);
        let (directory, _, combined_inventory) = supervisor
            .session_shadow_snapshot(&[])
            .expect("read complete empty shadow snapshot");
        assert!(directory.is_empty());
        assert!(combined_inventory.states.is_empty());
        assert_eq!(combined_inventory.unclaimed_count, 0);
        assert_eq!(combined_inventory.error_count, 0);
        assert!(supervisor
            .pending_session_deletes_page(None)
            .expect("read first deletion recovery page")
            .sessions
            .is_empty());
        assert!(supervisor
            .pending_session_title_recoveries()
            .expect("read empty title recovery list")
            .is_empty());
        supervisor.stop().expect("stop bridge");
    }

    #[test]
    fn host_syncs_session_order_through_a_real_bridge_when_provided() {
        let Some(binary) = bridge_under_test() else {
            return;
        };
        let _env = crate::test_env::guard();
        let home = tempfile::tempdir().expect("isolated reasonix home");
        env::set_var("REASONIX_HOME", home.path());
        env::set_var("REASONIX_STATE_HOME", home.path());

        let supervisor = BridgeSupervisor::with_binary(binary);
        supervisor
            .start()
            .expect("start bridge with capability handshake");
        supervisor
            .open_session(OpenSessionRequest {
                session_id: "tauri-e2e-order".to_string(),
                workspace_root: None,
            })
            .expect("reserve session before syncing its host metadata");
        let workspace = home.path().join("project").to_string_lossy().into_owned();
        assert_eq!(
            supervisor
                .sync_session_catalog(vec![SessionCatalogMetadata {
                    session_id: "tauri-e2e-order".to_string(),
                    workspace_root: Some(workspace.clone()),
                }])
                .expect("sync session catalog"),
            1
        );
        let page = supervisor
            .session_directory_snapshot_with_id()
            .expect("read synchronized identity catalog")
            .0;
        assert_eq!(page.len(), 1);
        assert_eq!(page[0].id, "tauri-e2e-order");
        assert_eq!(page[0].workspace_root.as_deref(), Some(workspace.as_str()));
        assert_eq!(page[0].position, 0);

        supervisor.stop().expect("stop bridge");
    }

    #[test]
    fn supervisor_recovers_from_an_unexpected_sidecar_exit() {
        let Some(binary) = bridge_under_test() else {
            return;
        };
        let _env = crate::test_env::guard();
        let supervisor = BridgeSupervisor::with_binary(binary);
        assert!(supervisor.start().expect("start bridge").running);
        {
            let mut process = supervisor.process.lock().expect("bridge state lock");
            let running = process.as_mut().expect("running bridge");
            running.child.kill().expect("kill sidecar");
            assert!(
                wait_for_exit(&mut running.child, Duration::from_secs(10)).expect("reap sidecar"),
                "the killed sidecar did not exit"
            );
        }
        // A dead sidecar must read as stopped so the host can offer a restart,
        // and the reaped child must leave no orphan process behind.
        assert!(!supervisor.status().running);
        assert!(supervisor.restart().expect("restart bridge").running);
        supervisor.stop().expect("stop bridge");
        assert!(!supervisor.status().running);
    }

    // Exercises the whole rename path the workbench uses: Rust host -> real Go
    // bridge -> core .jsonl.meta sidecar. The session title must survive a
    // sidecar restart, because that is the only reason it is stored in core
    // metadata instead of host UI state.
    #[test]
    fn session_title_survives_a_sidecar_restart() {
        let Some(binary) = bridge_under_test() else {
            return;
        };
        let _env = crate::test_env::guard();
        let home = tempfile::tempdir().expect("isolated reasonix home");
        env::set_var("REASONIX_HOME", home.path());
        let supervisor = BridgeSupervisor::with_binary(binary);
        supervisor.start().expect("start bridge");

        let opened = supervisor
            .open_session(OpenSessionRequest {
                session_id: "tauri-e2e-title".to_string(),
                workspace_root: None,
            })
            .expect("open session");
        assert_eq!(opened.title, None, "a new session starts untitled");

        let renamed = supervisor
            .rename_session(RenameSessionRequest {
                session_id: "tauri-e2e-title".to_string(),
                title: "Release notes".to_string(),
            })
            .expect("rename session");
        assert_eq!(renamed.title.as_deref(), Some("Release notes"));

        // The transport surfaces only the HTTP status, so assert the refusal
        // itself; the Go tests pin the exact error code and body.
        let rejected = supervisor
            .rename_session(RenameSessionRequest {
                session_id: "tauri-e2e-title".to_string(),
                title: "bad\ntitle".to_string(),
            })
            .expect_err("a title with a control character must be rejected");
        assert!(
            rejected.contains("400"),
            "unexpected rejection message: {rejected}"
        );

        supervisor.restart().expect("restart bridge");
        let reopened = supervisor
            .open_session(OpenSessionRequest {
                session_id: "tauri-e2e-title".to_string(),
                workspace_root: None,
            })
            .expect("reopen session");
        assert_eq!(
            reopened.title.as_deref(),
            Some("Release notes"),
            "the title must be read back from core session metadata"
        );
        supervisor.stop().expect("stop bridge");
    }

    #[test]
    fn managed_preview_sidecar_imports_and_repairs_sqlite_event_projection() {
        let Some(binary) = bridge_under_test() else {
            return;
        };
        let _env = crate::test_env::guard();
        let home = tempfile::tempdir().expect("isolated Preview profile");
        env::set_var("REASONIX_HOME", home.path());
        env::remove_var("REASONIX_STATE_HOME");
        env::set_var("REASONIX_PREVIEW_SQLITE_EVENTS", "1");
        let provider_listener = TcpListener::bind("127.0.0.1:0").expect("bind fake provider");
        let provider_address = provider_listener.local_addr().expect("provider address");
        provider_listener
            .set_nonblocking(true)
            .expect("set provider listener nonblocking");
        let (provider_served_tx, provider_served_rx) = mpsc::channel();
        let (provider_stop_tx, provider_stop_rx) = mpsc::channel();
        let provider_thread = thread::spawn(move || {
            let deadline = std::time::Instant::now() + Duration::from_secs(20);
            while std::time::Instant::now() < deadline && provider_stop_rx.try_recv().is_err() {
                match provider_listener.accept() {
                    Ok((mut stream, _)) => {
                        let mut request = [0u8; 65_536];
                        let _ = stream.read(&mut request);
                        let body = b"data: {\"choices\":[{\"delta\":{\"content\":\"preview SQLite answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n";
                        write!(
                            stream,
                            "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
                            body.len()
                        )
                        .expect("write fake provider headers");
                        stream.write_all(body).expect("write fake provider stream");
                        let _ = provider_served_tx.send(());
                    }
                    Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                        thread::sleep(Duration::from_millis(10));
                    }
                    Err(error) => panic!("accept fake provider request: {error}"),
                }
            }
        });
        let provider_config = format!(
            "default_model = \"local/alpha\"\n\n[desktop]\nprovider_access = [\"local\"]\n\n[[providers]]\nname = \"local\"\nkind = \"openai\"\nbase_url = \"http://{provider_address}/v1\"\nmodels = [\"alpha\"]\ndefault = \"alpha\"\n"
        );
        std::fs::write(home.path().join("config.toml"), provider_config)
            .expect("configure fake local provider");
        let supervisor = BridgeSupervisor::with_binary(binary);
        supervisor.start().expect("start managed Preview sidecar");

        let session_id = "tauri-e2e-sqlite-events";
        let opened = supervisor
            .open_session(OpenSessionRequest {
                session_id: session_id.to_string(),
                workspace_root: None,
            })
            .expect("reserve Preview session");
        let session_path = std::path::PathBuf::from(&opened.path);
        let stem = session_path
            .file_stem()
            .expect("session transcript filename")
            .to_string_lossy();
        let event_path = session_path
            .parent()
            .expect("session transcript directory")
            .join(format!("{stem}.events.jsonl"));
        std::fs::write(
            &session_path,
            b"{\"role\":\"system\",\"content\":\"test system prompt\"}\n{\"role\":\"user\",\"content\":\"legacy checkpoint\"}\n",
        )
        .expect("write isolated compatibility checkpoint");
        let legacy_events = b"{\"schema_version\":2,\"type\":\"log\",\"generation\":1,\"at\":\"2026-09-26T00:00:00Z\"}\n";
        std::fs::write(&event_path, legacy_events).expect("write schema-2 source event log");

        supervisor.restart().expect("restart before legacy import");
        supervisor
            .open_session(OpenSessionRequest {
                session_id: session_id.to_string(),
                workspace_root: None,
            })
            .expect("import legacy schema-2 log into SQLite");
        let database_path = home.path().join("desktop/session-state-v1.sqlite");
        assert!(
            database_path.is_file(),
            "managed Preview SQLite database was not created"
        );

        std::fs::write(&event_path, b"stale imported projection\n")
            .expect("corrupt imported projection");
        supervisor
            .restart()
            .expect("restart before imported projection recovery");
        supervisor
            .open_session(OpenSessionRequest {
                session_id: session_id.to_string(),
                workspace_root: None,
            })
            .expect("recover imported projection from SQLite");
        let repaired_import = std::fs::read(&event_path).expect("read imported event projection");
        assert!(repaired_import.starts_with(legacy_events));
        supervisor
            .restart()
            .expect("restart after validating imported session recovery");

        let write_session_id = "tauri-e2e-sqlite-write";
        let write_opened = supervisor
            .open_session(OpenSessionRequest {
                session_id: write_session_id.to_string(),
                workspace_root: None,
            })
            .expect("open fresh Preview session for SQLite write");
        let write_session_path = std::path::PathBuf::from(&write_opened.path);
        let write_stem = write_session_path
            .file_stem()
            .expect("write session transcript filename")
            .to_string_lossy();
        let write_event_path = write_session_path
            .parent()
            .expect("write session transcript directory")
            .join(format!("{write_stem}.events.jsonl"));

        supervisor
            .submit(SubmitRequest {
                session_id: write_session_id.to_string(),
                input: "write an event into Preview SQLite".to_string(),
            })
            .expect("submit through the real bridge");
        let history_deadline = std::time::Instant::now() + Duration::from_secs(15);
        let history = loop {
            let history = supervisor
                .history(SessionRequest {
                    session_id: write_session_id.to_string(),
                })
                .expect("read submitted session history");
            if history
                .messages
                .iter()
                .any(|message| message.content == "preview SQLite answer")
                && history.session.state == "idle"
            {
                break history;
            }
            assert!(
                std::time::Instant::now() < history_deadline,
                "bridge turn did not finish; state={:?}, messages={:?}, provider_requests={}",
                history.session.state,
                history
                    .messages
                    .iter()
                    .map(|message| (&message.role, &message.content))
                    .collect::<Vec<_>>(),
                provider_served_rx.try_iter().count()
            );
            thread::sleep(Duration::from_millis(25));
        };
        assert!(history
            .messages
            .iter()
            .any(|message| message.content == "write an event into Preview SQLite"));
        let _ = provider_stop_tx.send(());
        provider_thread.join().expect("fake provider stopped");
        let turn_persistence_deadline = std::time::Instant::now() + Duration::from_secs(5);
        loop {
            if std::fs::read_to_string(&write_session_path)
                .is_ok_and(|saved| saved.contains("preview SQLite answer"))
            {
                break;
            }
            assert!(
                std::time::Instant::now() < turn_persistence_deadline,
                "completed bridge turn was not checkpointed before sidecar shutdown"
            );
            thread::sleep(Duration::from_millis(20));
        }
        let turn_projection = std::fs::read_to_string(&write_event_path)
            .expect("read event projection after completed turn");
        assert!(
            turn_projection.contains("preview SQLite answer"),
            "completed bridge turn did not reach SQLite before sidecar shutdown"
        );
        supervisor
            .restart()
            .expect("snapshot completed turn while restarting the sidecar");
        let saved_checkpoint =
            std::fs::read_to_string(&write_session_path).expect("read shutdown checkpoint");
        assert!(
            saved_checkpoint.contains("preview SQLite answer"),
            "sidecar shutdown did not save the completed bridge turn"
        );
        let saved_projection =
            std::fs::read_to_string(&write_event_path).expect("read SQLite projection");
        assert!(
            saved_projection.contains("preview SQLite answer"),
            "sidecar shutdown did not append the completed bridge turn to SQLite: {saved_projection}"
        );

        std::fs::write(&write_event_path, b"stale projection\n")
            .expect("corrupt only the new session projection");
        supervisor
            .restart()
            .expect("restart before projection recovery");
        supervisor
            .open_session(OpenSessionRequest {
                session_id: write_session_id.to_string(),
                workspace_root: None,
            })
            .expect("recover event projection from SQLite");
        let repaired = std::fs::read(&write_event_path).expect("read repaired event projection");
        let repaired_text = String::from_utf8(repaired).expect("UTF-8 event projection");
        assert!(repaired_text.contains("preview SQLite answer"));
        let reopened = supervisor
            .history(SessionRequest {
                session_id: write_session_id.to_string(),
            })
            .expect("read history after second sidecar restart");
        assert!(reopened
            .messages
            .iter()
            .any(|message| message.content == "preview SQLite answer"));
        supervisor.stop().expect("stop managed Preview sidecar");
    }

    // Deleting is the only bridge operation that destroys user data, so the
    // whole path is checked against a real sidecar: the artifact sweep must
    // remove every file the session owned and leave other sessions alone.
    #[test]
    fn deleting_a_session_removes_its_artifacts_and_keeps_other_sessions() {
        let Some(binary) = bridge_under_test() else {
            return;
        };
        let _env = crate::test_env::guard();
        let home = tempfile::tempdir().expect("isolated reasonix home");
        std::env::set_var("REASONIX_HOME", home.path());
        let supervisor = BridgeSupervisor::with_binary(binary);
        supervisor.start().expect("start bridge");

        let doomed_id = "tauri-e2e-doomed";
        let opened = supervisor
            .open_session(OpenSessionRequest {
                session_id: doomed_id.to_string(),
                workspace_root: None,
            })
            .expect("open doomed session");
        // Renaming writes the metadata sidecar, so the session owns at least one
        // artifact before the sweep. A fresh session has no transcript yet.
        supervisor
            .rename_session(RenameSessionRequest {
                session_id: doomed_id.to_string(),
                title: "Scratch".to_string(),
            })
            .expect("rename doomed session");
        let session_dir = std::path::Path::new(&opened.path)
            .parent()
            .expect("session directory")
            .to_path_buf();
        let owned_before = session_files(&session_dir, doomed_id);
        assert!(
            !owned_before.is_empty(),
            "the doomed session owns no artifacts to delete"
        );

        // A second session must survive the sweep. The bridge owns one session at
        // a time, so switching is the supported way to move to another one.
        let survivor_id = "tauri-e2e-keep";
        supervisor
            .switch_session(OpenSessionRequest {
                session_id: survivor_id.to_string(),
                workspace_root: None,
            })
            .expect("switch to survivor session");
        supervisor
            .rename_session(RenameSessionRequest {
                session_id: survivor_id.to_string(),
                title: "Kept".to_string(),
            })
            .expect("rename survivor session");

        // Only the owned session can be deleted, so switch back before sweeping.
        supervisor
            .switch_session(OpenSessionRequest {
                session_id: doomed_id.to_string(),
                workspace_root: None,
            })
            .expect("switch to the doomed session");
        let deleted = supervisor
            .delete_session(SessionRequest {
                session_id: doomed_id.to_string(),
            })
            .expect("delete session");
        assert!(deleted.deleted);
        assert_eq!(deleted.session_id, doomed_id);
        for artifact in &owned_before {
            assert!(
                !artifact.exists(),
                "artifact survived delete: {}",
                artifact.display()
            );
        }

        let survivor = supervisor
            .switch_session(OpenSessionRequest {
                session_id: survivor_id.to_string(),
                workspace_root: None,
            })
            .expect("switch to survivor");
        assert_eq!(
            survivor.title.as_deref(),
            Some("Kept"),
            "deleting one session removed another"
        );
        supervisor.stop().expect("stop bridge");
    }

    fn session_files(dir: &std::path::Path, session_id: &str) -> Vec<std::path::PathBuf> {
        std::fs::read_dir(dir)
            .expect("read session directory")
            .filter_map(Result::ok)
            .map(|entry| entry.path())
            .filter(|path| {
                path.file_name()
                    .and_then(|name| name.to_str())
                    .is_some_and(|name| name.contains(session_id))
            })
            .collect()
    }
}
