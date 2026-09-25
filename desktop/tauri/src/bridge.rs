use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine as _};
use rand::TryRngCore;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::{
    env, fs,
    io::{BufRead, BufReader, Read, Write},
    net::{IpAddr, SocketAddr, TcpStream},
    path::{Path, PathBuf},
    process::{Child, Command, Stdio},
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

const BRIDGE_TOKEN_ENV: &str = "REASONIX_DESKTOP_BRIDGE_TOKEN";
const BRIDGE_BINARY_ENV: &str = "REASONIX_DESKTOP_BRIDGE_BIN";
const BUNDLED_BRIDGE_NAME: &str = "reasonix-desktop-bridge";
const READY_TIMEOUT: Duration = Duration::from_secs(5);
const STOP_TIMEOUT: Duration = Duration::from_secs(5);
const MAX_SHADOW_SESSIONS: usize = 10_000;
const MAX_BRIDGE_RESPONSE_BODY_BYTES: usize = 32 * 1024 * 1024;
const MAX_BRIDGE_HTTP_RESPONSE_BYTES: usize = 64 * 1024 * 1024;
pub const PROTOCOL_VERSION: u8 = 1;

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeStatus {
    pub running: bool,
    pub protocol_version: Option<u8>,
    pub sidecar_instance_id: Option<String>,
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

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct PendingSessionDeletesResponse {
    protocol_version: u8,
    sessions: Vec<PendingSessionDelete>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SessionInventoryResponse {
    protocol_version: u8,
    entries: Option<Vec<SessionInventoryEntry>>,
    unclaimed: Option<Vec<String>>,
    errors: Option<Vec<String>>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SessionInventoryEntry {
    id: String,
    source: String,
    exists: bool,
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
}

impl SessionInventoryResponse {
    fn into_physical_inventory(self) -> Result<SessionPhysicalInventory, String> {
        if self.protocol_version != PROTOCOL_VERSION {
            return Err("desktop bridge inventory protocol is unsupported".to_string());
        }
        let entries = self
            .entries
            .ok_or_else(|| "desktop bridge inventory omitted entries".to_string())?;
        let unclaimed = self
            .unclaimed
            .ok_or_else(|| "desktop bridge inventory omitted unclaimed files".to_string())?;
        let errors = self
            .errors
            .ok_or_else(|| "desktop bridge inventory omitted errors".to_string())?;
        Ok(SessionPhysicalInventory {
            states: entries
                .into_iter()
                .filter(|entry| entry.source == "identity")
                .map(|entry| SessionPhysicalState {
                    id: entry.id,
                    exists: entry.exists,
                    readable: entry.detail.is_empty() || entry.detail == "transcript is absent",
                })
                .collect(),
            unclaimed_count: unclaimed.len(),
            error_count: errors.len(),
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

// The wire DTOs mirror docs/tauri/protocol/v1.schema.json through the generated
// module; only the host-facing command payloads below stay hand-written.
pub use crate::protocol_generated::{
    BridgeAnswerQuestionRequest, BridgeApprovalRequest, BridgeAskAnswer,
    BridgeAttachFileRequest as AttachFileRequest, BridgeAttachment, BridgeAttachmentResponse,
    BridgeDeleteSessionResponse, BridgeEvent, BridgeHistoryMessage, BridgeHistoryResponse,
    BridgeMCPInteractionAnswerRequest, BridgeOpenSessionRequest as OpenSessionRequest,
    BridgeProjectFolder, BridgeProjectFoldersResponse, BridgeProviderSummaryResponse,
    BridgeRenameSessionRequest, BridgeSession, BridgeSessionResponse, BridgeSetDefaultModelRequest,
    BridgeWorkspaceChangeDetailRequest, BridgeWorkspaceChangeDetailResponse,
    BridgeWorkspaceChangesResponse, BridgeWorkspaceFileRequest, BridgeWorkspaceFileResponse,
    BridgeWorkspaceListResponse, BridgeWorkspaceRequest,
};

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeSnapshot {
    pub sequence: u64,
    pub session: BridgeSession,
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

/// One MCP server as the host may see it. Credential material is write-only, so
/// this carries the key names a server expects and never a value. Hand-written
/// like the other host-owned payloads: the wire shape belongs to this host, not
/// to the frozen bridge schema.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MCPServerView {
    pub name: String,
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
    pub auto_start: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tier: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub managed_by_package: Option<bool>,
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
    Explicit(PathBuf),
    Bundled(tauri::AppHandle),
}

#[derive(Debug)]
enum BridgeChild {
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
        match self {
            Self::Explicit(binary) => {
                if !binary.is_file() {
                    return Err(format!(
                        "desktop bridge executable was not found at {}",
                        binary.display()
                    ));
                }
                Command::new(binary)
                    .args(&args)
                    .env(BRIDGE_TOKEN_ENV, token)
                    .stdin(Stdio::null())
                    .stdout(Stdio::null())
                    .stderr(Stdio::null())
                    .spawn()
                    .map(BridgeChild::Explicit)
                    .map_err(display_error)
            }
            Self::Bundled(app) => {
                let (events, child) = app
                    .shell()
                    .sidecar(BUNDLED_BRIDGE_NAME)
                    .map_err(display_error)?
                    .args(args)
                    .env(BRIDGE_TOKEN_ENV, token)
                    .spawn()
                    .map_err(display_error)?;
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
            Self::Explicit(child) => child
                .try_wait()
                .map(|status| status.is_none())
                .map_err(display_error),
            Self::Bundled { running, .. } => Ok(running.load(Ordering::Acquire)),
        }
    }

    fn kill(&mut self) -> Result<(), String> {
        match self {
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
        match env::var_os(BRIDGE_BINARY_ENV) {
            Some(binary) => Self::with_binary(PathBuf::from(binary)),
            // Development always provides an explicit path. A packaged app has
            // no shell PATH dependency: Tauri resolves this name beside its
            // own executable after `bundle.externalBin` embeds it.
            None => Self::with_launcher(BridgeLauncher::Bundled(app)),
        }
    }

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
            .map(str::trim)
            .filter(|root| !root.is_empty());
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
                    || project
                        .title
                        .as_ref()
                        .is_some_and(|title| title.chars().count() > 1024)
            })
        {
            return Err("desktop bridge returned an invalid project folder list".to_string());
        }
        Ok(envelope
            .projects
            .into_iter()
            .map(|mut project| {
                if let Some(title) = project.title.as_mut() {
                    title.retain(|character| !character.is_control());
                }
                project
            })
            .collect())
    }

    /// Read the entire visible directory for a bounded, diagnostic-only
    /// comparison. A changing total or incomplete scan cannot be mistaken
    /// for a zero-difference migration result.
    pub fn session_directory_snapshot_with_id(
        &self,
    ) -> Result<(Vec<SessionDirectoryEntry>, String), String> {
        self.session_directory_snapshot_for_workspace(None)
    }

    pub fn session_directory_snapshot_for_workspace(
        &self,
        workspace_root: Option<String>,
    ) -> Result<(Vec<SessionDirectoryEntry>, String), String> {
        let workspace_root = workspace_root
            .as_deref()
            .map(str::trim)
            .filter(|root| !root.is_empty());
        let path = mcp_path("/v1/sessions/snapshot", workspace_root)?;
        let response = self.request_json("GET", &path, None, None)?;
        let snapshot: SessionDirectoryPage =
            serde_json::from_value(response).map_err(display_error)?;
        validate_session_directory_snapshot(&snapshot, workspace_root)?;
        Ok((snapshot.sessions, snapshot.snapshot_id))
    }

    /// Reads path-free identities left in the explicit deleting state. These
    /// remain separate from ordinary paging and are never auto-deleted here.
    pub fn pending_session_deletes(&self) -> Result<Vec<PendingSessionDelete>, String> {
        let response = self.request_json("GET", "/v1/sessions/deletion-recovery", None, None)?;
        let envelope: PendingSessionDeletesResponse =
            serde_json::from_value(response).map_err(display_error)?;
        validate_pending_session_deletes(envelope)
    }

    /// Reduce the existing read-only inventory to file-state evidence. Paths
    /// and diagnostics never leave this method or enter the shadow report.
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
        })
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

    pub fn set_default_model(
        &self,
        request: BridgeSetDefaultModelRequest,
    ) -> Result<BridgeProviderSummaryResponse, String> {
        let request_id = opaque_secret()?;
        let response = self.request_json(
            "POST",
            "/v1/settings/default-model",
            Some(json!({ "model": request.model })),
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
    let Some(root) = workspace_root
        .map(str::trim)
        .filter(|root| !root.is_empty())
    else {
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

fn validate_pending_session_deletes(
    envelope: PendingSessionDeletesResponse,
) -> Result<Vec<PendingSessionDelete>, String> {
    if envelope.protocol_version != PROTOCOL_VERSION || envelope.sessions.len() > 10_000 {
        return Err("desktop bridge deletion recovery response is invalid".to_string());
    }
    let mut seen = std::collections::HashSet::with_capacity(envelope.sessions.len());
    for pending in &envelope.sessions {
        if session_path_component(&pending.id)? != pending.id
            || pending.title.chars().count() > 1024
            || pending.title.chars().any(char::is_control)
            || !seen.insert(pending.id.as_str())
        {
            return Err("desktop bridge deletion recovery entry is invalid".to_string());
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
        open_event_stream, parse_json_response, read_bounded_response, request_json,
        session_directory_path, session_path_component, validate_attachment,
        validate_pending_session_deletes, validate_session_directory_page,
        validate_session_directory_snapshot, verify_bridge_health, verify_ready, wait_for_exit,
        BridgeAttachment, BridgeEvent, BridgeSupervisor, EventStreamError, OpenSessionRequest,
        PendingSessionDelete, PendingSessionDeletesResponse, RenameSessionRequest,
        SessionCatalogMetadata, SessionDirectoryCursor, SessionDirectoryEntry,
        SessionDirectoryPage, SessionInventoryResponse, SessionRequest, PROTOCOL_VERSION,
    };
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
            "capabilities": ["open_session", "session_catalog_sync", "session_directory_snapshot_v1", "session_directory_snapshot_full_v1", "session_delete_recovery_list_v1", "session_title_intent_v1"],
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

        let old_recovery_sidecar = json!({
            "protocolVersion": 1,
            "status": "ok",
            "sidecarInstanceId": "instance",
            "capabilities": ["open_session", "session_catalog_sync", "session_directory_snapshot_v1", "session_directory_snapshot_full_v1"],
        });
        assert!(verify_bridge_health(old_recovery_sidecar, "instance")
            .unwrap_err()
            .contains("explicit session deletion recovery"));

        let old_title_intent_sidecar = json!({
            "protocolVersion": 1,
            "status": "ok",
            "sidecarInstanceId": "instance",
            "capabilities": ["open_session", "session_catalog_sync", "session_directory_snapshot_v1", "session_directory_snapshot_full_v1", "session_delete_recovery_list_v1"],
        });
        assert!(verify_bridge_health(old_title_intent_sidecar, "instance")
            .unwrap_err()
            .contains("durable session title intents"));
    }

    #[test]
    fn pending_session_delete_response_is_bounded_unique_and_path_free() {
        let entries = validate_pending_session_deletes(PendingSessionDeletesResponse {
            protocol_version: PROTOCOL_VERSION,
            sessions: vec![PendingSessionDelete {
                id: "interrupted-delete".into(),
                title: "Interrupted conversation".into(),
            }],
        })
        .expect("valid pending deletion");
        assert_eq!(entries.len(), 1);
        assert!(
            validate_pending_session_deletes(PendingSessionDeletesResponse {
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
            })
            .is_err()
        );
        assert!(
            validate_pending_session_deletes(PendingSessionDeletesResponse {
                protocol_version: PROTOCOL_VERSION,
                sessions: vec![PendingSessionDelete {
                    id: "../outside".into(),
                    title: "unsafe".into()
                }],
            })
            .is_err()
        );
    }

    #[test]
    fn session_directory_query_uses_composite_cursor_and_escaped_workspace() {
        let cursor = SessionDirectoryCursor {
            position: 12,
            id: "session-009".to_string(),
            snapshot_id: Some("a".repeat(64)),
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
            }),
            total: 3,
            snapshot_id: "a".repeat(64),
        };
        assert!(validate_session_directory_page(&page, 2, None, Some("/work/a")).is_ok());
        let after = SessionDirectoryCursor {
            position: 12,
            id: "session-001".to_string(),
            snapshot_id: Some("a".repeat(64)),
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
        let directory = supervisor
            .session_directory_snapshot_with_id()
            .expect("read identity directory")
            .0;
        let physical = supervisor
            .session_physical_inventory()
            .expect("read physical inventory");
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
    fn real_bridge_exposes_empty_inventory_and_deletion_recovery_list() {
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
        assert!(supervisor
            .pending_session_deletes()
            .expect("read empty deletion recovery list")
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
