//! Native notification boundary. Only fixed event captions reach the OS;
//! private session text, errors, workspace paths and credentials never do.
use crate::bridge::{BridgeSupervisor, SessionRequest};
use rand::TryRngCore;
use serde::{Deserialize, Serialize};
use std::{
    collections::VecDeque,
    fs,
    io::{Read, Write},
    path::{Path, PathBuf},
    sync::{Arc, Mutex},
    time::{SystemTime, UNIX_EPOCH},
};
use tauri::Manager;
#[cfg(target_os = "macos")]
mod macos;
#[cfg(all(unix, any(target_os = "linux", test)))]
#[cfg_attr(not(target_os = "linux"), allow(dead_code))]
mod xdg;

const FILE: &str = "tauri-notification-targets.json";
const LIMIT: u64 = 128 * 1024;
const MAX_TARGETS: usize = 256;
const TTL: u64 = 7 * 24 * 3600;
const STORAGE_ERROR: &str =
    "notification targets are unavailable; check permissions or restore a valid profile";
const DELIVERY_ERROR: &str =
    "notification delivery failed; check system notification settings and retry";
#[cfg(any(target_os = "macos", target_os = "linux"))]
const CLICK_EVENT: &str = "host:notification-clicked";

#[derive(Clone, Copy, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Permission {
    NotDetermined,
    Denied,
    Granted,
    Provisional,
    Unknown,
    Unavailable,
}
impl Permission {
    fn can_deliver(self) -> bool {
        matches!(self, Self::Granted | Self::Provisional | Self::Unknown)
    }
}
#[derive(Clone, Copy, Deserialize, Serialize, Debug, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum NotificationKind {
    TurnDone,
    ApprovalRequest,
    AskRequest,
}
#[derive(Clone, Copy, Deserialize, Debug)]
pub enum Language {
    #[serde(rename = "en")]
    En,
    #[serde(rename = "zh")]
    Zh,
    #[serde(rename = "zh-TW")]
    ZhTw,
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct NotificationRequest {
    pub session_id: String,
    pub kind: NotificationKind,
    pub language: Language,
    #[serde(default)]
    pub failed: bool,
}
impl NotificationRequest {
    fn body(&self) -> &'static str {
        match (self.language, self.kind, self.failed) {
            (Language::En, NotificationKind::TurnDone, false) => "Reply complete.",
            (Language::En, NotificationKind::TurnDone, true) => {
                "Reply failed. Open Reasonix for details."
            }
            (Language::En, NotificationKind::ApprovalRequest, _) => "Tool approval needed.",
            (Language::En, NotificationKind::AskRequest, _) => "A question needs your answer.",
            (Language::Zh, NotificationKind::TurnDone, false) => "回复已完成。",
            (Language::Zh, NotificationKind::TurnDone, true) => {
                "回复失败，请打开 Reasonix 查看详情。"
            }
            (Language::Zh, NotificationKind::ApprovalRequest, _) => "等待工具审批。",
            (Language::Zh, NotificationKind::AskRequest, _) => "等待你回答问题。",
            (Language::ZhTw, NotificationKind::TurnDone, false) => "回覆已完成。",
            (Language::ZhTw, NotificationKind::TurnDone, true) => {
                "回覆失敗，請開啟 Reasonix 查看詳情。"
            }
            (Language::ZhTw, NotificationKind::ApprovalRequest, _) => "等待工具審批。",
            (Language::ZhTw, NotificationKind::AskRequest, _) => "等待你回答問題。",
        }
    }
}
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Target {
    token: String,
    session_id: String,
    created: u64,
}
#[derive(Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
struct RegistryFile {
    version: u8,
    profile: String,
    targets: Vec<Target>,
}
struct Registry {
    targets: Vec<Target>,
    pending: VecDeque<String>,
}
#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Click {
    pub token: String,
    pub session_id: String,
}
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ClickTarget {
    pub session_id: String,
    pub workspace_root: Option<String>,
}
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct NotificationStatus {
    pub permission: Permission,
    pub click_supported: bool,
}

trait Backend: Send + Sync {
    fn status(&self, request: bool) -> Result<NotificationStatus, String>;
    fn send(&self, token: &str, body: &str) -> Result<(), String>;
    fn install(
        &self,
        _app: &tauri::AppHandle,
        _state: std::sync::Weak<NotificationState>,
    ) -> Result<(), String> {
        Ok(())
    }
    fn shutdown(&self) {}
}
struct PlatformBackend {
    #[cfg(not(target_os = "linux"))]
    identifier: String,
    #[cfg(target_os = "linux")]
    xdg: xdg::XdgBackend,
}
impl Backend for PlatformBackend {
    fn status(&self, request: bool) -> Result<NotificationStatus, String> {
        #[cfg(target_os = "macos")]
        {
            let permission = macos::permission(&self.identifier, request)?;
            Ok(NotificationStatus {
                permission,
                click_supported: permission != Permission::Unavailable,
            })
        }
        #[cfg(target_os = "linux")]
        {
            let _ = request;
            Ok(self.xdg.status())
        }
        #[cfg(not(any(target_os = "macos", target_os = "linux")))]
        {
            let _ = request;
            Ok(NotificationStatus {
                permission: Permission::Unknown,
                click_supported: false,
            })
        }
    }
    fn send(&self, token: &str, body: &str) -> Result<(), String> {
        #[cfg(target_os = "macos")]
        {
            macos::send(&self.identifier, token, body)
        }
        #[cfg(target_os = "linux")]
        {
            self.xdg.send(token, body)
        }
        #[cfg(not(any(target_os = "macos", target_os = "linux")))]
        {
            // This backend reports the actual OS request result. Native action
            // wiring on these platforms remains an explicit migration gate.
            let _ = token;
            let mut notification = notify_rust::Notification::new();
            notification
                .appname("Reasonix")
                .icon(&self.identifier)
                .summary("Reasonix")
                .body(body);
            #[cfg(target_os = "windows")]
            notification.app_id(&self.identifier);
            notification
                .show()
                .map(|_| ())
                .map_err(|_| DELIVERY_ERROR.to_string())
        }
    }
    fn install(
        &self,
        app: &tauri::AppHandle,
        state: std::sync::Weak<NotificationState>,
    ) -> Result<(), String> {
        #[cfg(target_os = "linux")]
        {
            let app = app.clone();
            self.xdg.set_callback(Arc::new(move |token| {
                if let Some(state) = state.upgrade() {
                    state.activate(&app, token);
                }
            }))
        }
        #[cfg(not(target_os = "linux"))]
        {
            let _ = (app, state);
            Ok(())
        }
    }
    fn shutdown(&self) {
        #[cfg(target_os = "linux")]
        self.xdg.shutdown();
    }
}

pub struct NotificationState {
    path: PathBuf,
    profile: Result<String, String>,
    registry: Mutex<Result<Registry, String>>,
    authorization: Mutex<()>,
    backend: Arc<dyn Backend>,
}
impl NotificationState {
    pub fn for_profile(
        profile: &crate::data_profile::PreviewProfile,
        identifier: &str,
    ) -> Arc<Self> {
        Arc::new(Self::new(
            profile.home(),
            Arc::new(PlatformBackend {
                #[cfg(not(target_os = "linux"))]
                identifier: identifier.into(),
                #[cfg(target_os = "linux")]
                xdg: xdg::XdgBackend::new(identifier),
            }),
        ))
    }
    fn new(home: &Path, backend: Arc<dyn Backend>) -> Self {
        let profile = crate::credential_namespace::service_for_profile(home);
        let path = home.join(FILE);
        let registry = profile
            .as_ref()
            .map_err(|_| STORAGE_ERROR.to_string())
            .and_then(|profile| load_registry(&path, profile));
        Self {
            path,
            profile,
            registry: Mutex::new(registry),
            authorization: Mutex::new(()),
            backend,
        }
    }
    pub fn install(app: &tauri::AppHandle, state: &Arc<Self>) {
        if state.backend.install(app, Arc::downgrade(state)).is_err() {
            eprintln!(
                "Reasonix native notification listener unavailable; restart Preview to retry"
            );
        }
        #[cfg(target_os = "macos")]
        if let Err(_error) = macos::install(app, Arc::clone(state)) {
            // Unbundled development executables cannot own a notification
            // center. Never impersonate Terminal to pretend delivery works.
            eprintln!(
                "Reasonix native notification center unavailable; use the installed Preview app"
            );
        }
        #[cfg(not(target_os = "macos"))]
        let _ = (app, state);
    }
    pub(crate) fn permission(&self, request: bool) -> Result<NotificationStatus, String> {
        let _guard = self.authorization.lock().map_err(|_| DELIVERY_ERROR)?;
        self.backend.status(request)
    }
    pub fn shutdown(&self) {
        self.backend.shutdown();
    }
    #[cfg(any(target_os = "macos", target_os = "linux"))]
    fn activate(&self, app: &tauri::AppHandle, token: &str) {
        use tauri::Emitter;
        if self.receive(token) {
            let focus = app.clone();
            let _ = app.run_on_main_thread(move || crate::tray::show_main_window(&focus));
            let _ = app.emit(CLICK_EVENT, ());
        }
    }
    fn deliver(&self, request: &NotificationRequest) -> Result<(), String> {
        if !valid_session(&request.session_id) {
            return Err("notification session is invalid".into());
        }
        self.pending()?;
        // One authorization dialog at a time. A denied state is never asked
        // repeatedly; not_determined is the only state that may prompt.
        let _guard = self.authorization.lock().map_err(|_| DELIVERY_ERROR)?;
        let mut permission = self.backend.status(false)?.permission;
        if permission == Permission::NotDetermined {
            permission = self.backend.status(true)?.permission;
        }
        if !permission.can_deliver() {
            return Err(DELIVERY_ERROR.into());
        }
        let token = random_token()?;
        {
            let mut state = self.registry.lock().map_err(|_| STORAGE_ERROR)?;
            let registry = state.as_mut().map_err(|_| STORAGE_ERROR)?;
            let mut targets = registry.targets.clone();
            targets.retain(|target| fresh(target.created, now()));
            if targets.len() >= MAX_TARGETS {
                targets.remove(0);
            }
            targets.push(Target {
                token: token.clone(),
                session_id: request.session_id.clone(),
                created: now(),
            });
            self.persist(&targets)?;
            registry.targets = targets;
        }
        // Retain the target on uncertain delivery: the OS may have accepted
        // it before the response was lost. Expiry and bounds contain it.
        self.backend
            .send(&token, request.body())
            .map_err(|_| DELIVERY_ERROR.into())
    }
    fn persist(&self, targets: &[Target]) -> Result<(), String> {
        let profile = self.profile.as_ref().map_err(|_| STORAGE_ERROR)?;
        if let Ok(meta) = fs::symlink_metadata(&self.path) {
            if meta.file_type().is_symlink() || !meta.is_file() {
                return Err(STORAGE_ERROR.into());
            }
        }
        let mut temp = tempfile::NamedTempFile::new_in(self.path.parent().ok_or(STORAGE_ERROR)?)
            .map_err(|_| STORAGE_ERROR)?;
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            temp.as_file()
                .set_permissions(fs::Permissions::from_mode(0o600))
                .map_err(|_| STORAGE_ERROR)?;
        }
        let bytes = serde_json::to_vec(&RegistryFile {
            version: 1,
            profile: profile.clone(),
            targets: targets.to_vec(),
        })
        .map_err(|_| STORAGE_ERROR)?;
        if bytes.len() as u64 > LIMIT {
            return Err(STORAGE_ERROR.into());
        }
        temp.write_all(&bytes).map_err(|_| STORAGE_ERROR)?;
        temp.as_file().sync_all().map_err(|_| STORAGE_ERROR)?;
        temp.persist(&self.path).map_err(|_| STORAGE_ERROR)?;
        Ok(())
    }
    #[cfg(any(target_os = "macos", target_os = "linux", test))]
    fn receive(&self, token: &str) -> bool {
        let Ok(mut state) = self.registry.lock() else {
            return false;
        };
        let Ok(registry) = state.as_mut() else {
            return false;
        };
        if !registry
            .targets
            .iter()
            .any(|target| target.token == token && fresh(target.created, now()))
            || registry.pending.iter().any(|item| item == token)
        {
            return false;
        }
        if registry.pending.len() >= 32 {
            registry.pending.pop_front();
        }
        registry.pending.push_back(token.into());
        true
    }
    fn pending(&self) -> Result<Vec<Click>, String> {
        let state = self.registry.lock().map_err(|_| STORAGE_ERROR)?;
        let registry = state.as_ref().map_err(|_| STORAGE_ERROR)?;
        Ok(registry
            .pending
            .iter()
            .filter_map(|token| {
                registry
                    .targets
                    .iter()
                    .find(|target| &target.token == token && fresh(target.created, now()))
                    .map(|target| Click {
                        token: target.token.clone(),
                        session_id: target.session_id.clone(),
                    })
            })
            .collect())
    }
    fn acknowledge(&self, token: &str) -> Result<(), String> {
        let mut state = self.registry.lock().map_err(|_| STORAGE_ERROR)?;
        let registry = state.as_mut().map_err(|_| STORAGE_ERROR)?;
        // An arbitrary JS token cannot delete another stored target.
        if !registry.pending.iter().any(|item| item == token) {
            return Err("notification click is unavailable".into());
        }
        let targets: Vec<_> = registry
            .targets
            .iter()
            .filter(|target| target.token != token)
            .cloned()
            .collect();
        self.persist(&targets)?;
        registry.targets = targets;
        registry.pending.retain(|item| item != token);
        Ok(())
    }
    fn resolve(
        &self,
        supervisor: &BridgeSupervisor,
        token: &str,
    ) -> Result<Option<ClickTarget>, String> {
        let click = self
            .pending()?
            .into_iter()
            .find(|item| item.token == token)
            .ok_or("notification click is unavailable")?;
        // Resolve from the current authoritative directory. Never recreate a
        // deleted session or trust a stored/client-supplied workspace path.
        let mut cursor = None;
        for _ in 0..50 {
            let page = supervisor.session_directory_page(200, cursor, None)?;
            if let Some(entry) = page
                .sessions
                .into_iter()
                .find(|entry| entry.id == click.session_id)
            {
                if entry.missing {
                    return Ok(None);
                }
                let mut deletes = None;
                for _ in 0..50 {
                    let pending = supervisor.pending_session_deletes_page(deletes)?;
                    if pending
                        .sessions
                        .iter()
                        .any(|entry| entry.id == click.session_id)
                    {
                        return Ok(None);
                    }
                    deletes = pending.next_cursor;
                    if deletes.is_none() {
                        return Ok(Some(ClickTarget {
                            session_id: entry.id,
                            workspace_root: entry.workspace_root,
                        }));
                    }
                }
                return Err("notification session verification is incomplete".into());
            }
            cursor = page.next_cursor;
            if cursor.is_none() {
                return Ok(None);
            }
        }
        Err("notification session verification is incomplete".into())
    }
}
fn valid_session(id: &str) -> bool {
    !id.is_empty()
        && id.len() <= 128
        && id
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || b"_-".contains(&byte))
}
fn fresh(created: u64, now: u64) -> bool {
    created <= now.saturating_add(60) && now.saturating_sub(created) <= TTL
}
fn now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|value| value.as_secs())
        .unwrap_or(0)
}
fn random_token() -> Result<String, String> {
    let mut bytes = [0u8; 16];
    rand::rngs::OsRng
        .try_fill_bytes(&mut bytes)
        .map_err(|_| DELIVERY_ERROR)?;
    Ok(format!(
        "reasonix-preview-{}",
        bytes
            .iter()
            .map(|byte| format!("{byte:02x}"))
            .collect::<String>()
    ))
}
fn valid_token(token: &str) -> bool {
    token
        .strip_prefix("reasonix-preview-")
        .is_some_and(|value| {
            value.len() == 32
                && value
                    .bytes()
                    .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
        })
}
fn load_registry(path: &Path, profile: &str) -> Result<Registry, String> {
    let meta = match fs::symlink_metadata(path) {
        Ok(meta) => meta,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return Ok(Registry {
                targets: vec![],
                pending: VecDeque::new(),
            })
        }
        Err(_) => return Err(STORAGE_ERROR.into()),
    };
    if !meta.is_file() || meta.file_type().is_symlink() || meta.len() > LIMIT {
        return Err(STORAGE_ERROR.into());
    }
    let file = fs::File::open(path).map_err(|_| STORAGE_ERROR)?;
    if !crate::workbench_projects::same_file_as_path(path, &file).map_err(|_| STORAGE_ERROR)?
        || fs::symlink_metadata(path)
            .map_err(|_| STORAGE_ERROR)?
            .file_type()
            .is_symlink()
    {
        return Err(STORAGE_ERROR.into());
    }
    let mut bytes = vec![];
    file.take(LIMIT + 1)
        .read_to_end(&mut bytes)
        .map_err(|_| STORAGE_ERROR)?;
    if bytes.len() as u64 > LIMIT {
        return Err(STORAGE_ERROR.into());
    }
    let stored: RegistryFile = serde_json::from_slice(&bytes).map_err(|_| STORAGE_ERROR)?;
    if stored.version != 1
        || stored.profile != profile
        || stored.targets.len() > MAX_TARGETS
        || stored
            .targets
            .iter()
            .any(|target| !valid_token(&target.token) || !valid_session(&target.session_id))
    {
        return Err(STORAGE_ERROR.into());
    }
    let mut tokens = std::collections::HashSet::new();
    if stored
        .targets
        .iter()
        .any(|target| !tokens.insert(target.token.clone()))
    {
        return Err(STORAGE_ERROR.into());
    }
    Ok(Registry {
        targets: stored
            .targets
            .into_iter()
            .filter(|target| fresh(target.created, now()))
            .collect(),
        pending: VecDeque::new(),
    })
}
fn main_window(label: &str) -> Result<(), String> {
    if label == "main" {
        Ok(())
    } else {
        Err("notification operations require the main window".into())
    }
}
#[tauri::command]
pub async fn notification_permission(
    window: tauri::WebviewWindow,
    request: bool,
) -> Result<NotificationStatus, String> {
    main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        window.state::<Arc<NotificationState>>().permission(request)
    })
    .await
    .map_err(|_| DELIVERY_ERROR)?
}
#[tauri::command]
pub async fn send_system_notification(
    window: tauri::WebviewWindow,
    request: NotificationRequest,
) -> Result<(), String> {
    main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        let supervisor = window.state::<BridgeSupervisor>();
        let snapshot = supervisor
            .snapshot(SessionRequest {
                session_id: request.session_id.clone(),
            })
            .map_err(|_| "notification session is unavailable")?;
        if snapshot.session.id != request.session_id {
            return Err("notification session does not match".into());
        }
        window.state::<Arc<NotificationState>>().deliver(&request)
    })
    .await
    .map_err(|_| DELIVERY_ERROR)?
}
#[tauri::command]
pub fn pending_notification_clicks(window: tauri::WebviewWindow) -> Result<Vec<Click>, String> {
    main_window(window.label())?;
    window.state::<Arc<NotificationState>>().pending()
}
#[tauri::command]
pub async fn acknowledge_notification_click(
    window: tauri::WebviewWindow,
    token: String,
) -> Result<(), String> {
    main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        window.state::<Arc<NotificationState>>().acknowledge(&token)
    })
    .await
    .map_err(|_| STORAGE_ERROR)?
}
#[tauri::command]
pub async fn resolve_notification_click(
    window: tauri::WebviewWindow,
    token: String,
) -> Result<Option<ClickTarget>, String> {
    main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        window
            .state::<Arc<NotificationState>>()
            .resolve(&window.state::<BridgeSupervisor>(), &token)
    })
    .await
    .map_err(|_| DELIVERY_ERROR)?
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
    struct FakeBackend {
        permission: Mutex<Permission>,
        requested: AtomicUsize,
        fail: AtomicBool,
        clicks: AtomicBool,
        sent: Mutex<Vec<(String, String)>>,
    }
    impl FakeBackend {
        fn new(permission: Permission) -> Arc<Self> {
            Arc::new(Self {
                permission: Mutex::new(permission),
                requested: AtomicUsize::new(0),
                fail: AtomicBool::new(false),
                clicks: AtomicBool::new(false),
                sent: Mutex::new(vec![]),
            })
        }
    }
    impl Backend for FakeBackend {
        fn status(&self, request: bool) -> Result<NotificationStatus, String> {
            let mut permission = self.permission.lock().unwrap();
            if request && *permission == Permission::NotDetermined {
                self.requested.fetch_add(1, Ordering::SeqCst);
                *permission = Permission::Granted;
            }
            Ok(NotificationStatus {
                permission: *permission,
                click_supported: self.clicks.load(Ordering::SeqCst),
            })
        }
        fn send(&self, token: &str, body: &str) -> Result<(), String> {
            self.sent.lock().unwrap().push((token.into(), body.into()));
            if self.fail.load(Ordering::SeqCst) {
                Err("private-platform-diagnostic".into())
            } else {
                Ok(())
            }
        }
    }
    fn request(id: &str) -> NotificationRequest {
        NotificationRequest {
            session_id: id.into(),
            kind: NotificationKind::TurnDone,
            language: Language::Zh,
            failed: false,
        }
    }
    #[test]
    fn click_capability_comes_from_backend_instead_of_compile_target() {
        let dir = tempfile::tempdir().unwrap();
        let backend = FakeBackend::new(Permission::Unknown);
        let state = NotificationState::new(dir.path(), backend.clone());
        assert!(!state.permission(false).unwrap().click_supported);
        backend.clicks.store(true, Ordering::SeqCst);
        let status = state.permission(false).unwrap();
        assert!(status.click_supported);
        assert_eq!(status.permission, Permission::Unknown);
    }
    #[test]
    fn permission_is_truthful_denial_does_not_reprompt_and_payload_is_fixed() {
        let dir = tempfile::tempdir().unwrap();
        let backend = FakeBackend::new(Permission::Denied);
        let state = NotificationState::new(dir.path(), backend.clone());
        assert_eq!(
            state.permission(false).unwrap().permission,
            Permission::Denied
        );
        assert!(state.deliver(&request("session-one")).is_err());
        assert_eq!(backend.requested.load(Ordering::SeqCst), 0);
        assert!(backend.sent.lock().unwrap().is_empty());
        *backend.permission.lock().unwrap() = Permission::NotDetermined;
        state.deliver(&request("session-one")).unwrap();
        state.deliver(&request("session-one")).unwrap();
        assert_eq!(backend.requested.load(Ordering::SeqCst), 1);
        let sent = backend.sent.lock().unwrap();
        assert_eq!(sent[0].1, "回复已完成。");
        assert!(valid_token(&sent[0].0));
        assert_ne!(sent[0].0, sent[1].0);
        assert!(!fs::read_to_string(dir.path().join(FILE))
            .unwrap()
            .contains("回复已完成"));
        assert!(serde_json::from_value::<NotificationRequest>(serde_json::json!({"sessionId":"session-one", "kind":"turn_done", "language":"zh", "body":"private-raw-error"})).is_err());
        assert!(main_window("remote").is_err());
    }
    #[test]
    fn cold_start_clicks_restore_only_matching_profile_and_ack_is_durable() {
        let root = tempfile::tempdir().unwrap();
        let home = root.path().join("one");
        let backend = FakeBackend::new(Permission::Granted);
        let state = NotificationState::new(&home, backend.clone());
        state.deliver(&request("session-one")).unwrap();
        let token = backend.sent.lock().unwrap()[0].0.clone();
        let reopened = NotificationState::new(&home, backend.clone());
        assert!(
            reopened.receive(&token),
            "response before JS listener is queued"
        );
        assert!(!reopened.receive(&token), "duplicate response is coalesced");
        assert_eq!(reopened.pending().unwrap()[0].session_id, "session-one");
        assert!(!reopened.receive("untrusted-token"));
        assert!(reopened.acknowledge("untrusted-token").is_err());
        let other_home = root.path().join("two");
        let other = NotificationState::new(&other_home, backend.clone());
        assert!(!other.receive(&token));
        fs::copy(home.join(FILE), other_home.join(FILE)).unwrap();
        let copied_targets = NotificationState::new(&other_home, backend.clone());
        assert!(
            copied_targets.pending().is_err(),
            "foreign-profile targets are never routed"
        );
        assert!(!copied_targets.receive(&token));
        reopened.acknowledge(&token).unwrap();
        assert!(reopened.pending().unwrap().is_empty());
        assert!(
            !NotificationState::new(&home, backend).receive(&token),
            "ack survives process restart"
        );
    }
    #[test]
    fn registry_limits_expiry_and_corruption_fail_closed_without_overwrite() {
        let dir = tempfile::tempdir().unwrap();
        let backend = FakeBackend::new(Permission::Granted);
        let state = NotificationState::new(dir.path(), backend.clone());
        let profile = state.profile.as_ref().unwrap().clone();
        let expired = Target {
            token: random_token().unwrap(),
            session_id: "old".into(),
            created: now() - TTL - 1,
        };
        state.persist(std::slice::from_ref(&expired)).unwrap();
        assert!(!NotificationState::new(dir.path(), backend.clone()).receive(&expired.token));
        let targets: Vec<_> = (0..MAX_TARGETS)
            .map(|index| Target {
                token: random_token().unwrap(),
                session_id: format!("session-{index}"),
                created: now(),
            })
            .collect();
        state.persist(&targets).unwrap();
        let bounded = NotificationState::new(dir.path(), backend.clone());
        bounded.deliver(&request("newest")).unwrap();
        assert!(!bounded.receive(&targets[0].token));
        assert!(bounded.receive(&targets[1].token));
        let bytes = fs::read(dir.path().join(FILE)).unwrap();
        let record: RegistryFile = serde_json::from_slice(&bytes).unwrap();
        assert_eq!(record.targets.len(), MAX_TARGETS);
        assert_eq!(record.profile, profile);
        for bad in [
            b"not json".as_slice(),
            br#"{"version":1,"profile":"other","targets":[]}"#,
        ] {
            fs::write(dir.path().join(FILE), bad).unwrap();
            let invalid = NotificationState::new(dir.path(), backend.clone());
            assert!(invalid.deliver(&request("safe")).is_err());
            assert_eq!(fs::read(dir.path().join(FILE)).unwrap(), bad);
        }
    }
    #[test]
    fn failed_send_is_reported_without_private_details_and_uncertain_target_remains() {
        let dir = tempfile::tempdir().unwrap();
        let backend = FakeBackend::new(Permission::Granted);
        backend.fail.store(true, Ordering::SeqCst);
        let state = NotificationState::new(dir.path(), backend.clone());
        let error = state.deliver(&request("session-one")).unwrap_err();
        assert!(!error.contains("private-platform-diagnostic"));
        let token = backend.sent.lock().unwrap()[0].0.clone();
        assert!(
            state.receive(&token),
            "uncertain OS acceptance still has a resolvable target"
        );
    }
    #[cfg(unix)]
    #[test]
    fn targets_are_private_and_symlink_or_failed_ack_preserves_pending_state() {
        use std::os::unix::fs::{symlink, PermissionsExt};
        let dir = tempfile::tempdir().unwrap();
        let backend = FakeBackend::new(Permission::Granted);
        let state = NotificationState::new(dir.path(), backend.clone());
        state.deliver(&request("session-one")).unwrap();
        assert_eq!(
            fs::metadata(dir.path().join(FILE))
                .unwrap()
                .permissions()
                .mode()
                & 0o777,
            0o600
        );
        let token = backend.sent.lock().unwrap()[0].0.clone();
        assert!(state.receive(&token));
        let original = dir.path().join("preserved.json");
        fs::rename(dir.path().join(FILE), &original).unwrap();
        symlink(&original, dir.path().join(FILE)).unwrap();
        assert!(state.acknowledge(&token).is_err());
        assert_eq!(state.pending().unwrap()[0].token, token);
        assert!(NotificationState::new(dir.path(), backend)
            .pending()
            .is_err());
        assert!(original.exists());
    }
    #[test]
    fn real_bridge_clicks_resolve_current_roots_and_never_create_missing_sessions() {
        let Some(binary) = std::env::var_os("REASONIX_TAURI_BRIDGE_TEST_BIN") else {
            assert!(std::env::var_os("CI").is_none());
            return;
        };
        let _env = crate::test_env::guard();
        let dir = tempfile::tempdir().unwrap();
        std::env::set_var("REASONIX_HOME", dir.path());
        std::env::set_var("REASONIX_STATE_HOME", dir.path());
        std::env::set_var("REASONIX_CACHE_HOME", dir.path());
        let backend = FakeBackend::new(Permission::Granted);
        let state = NotificationState::new(dir.path(), backend.clone());
        let supervisor = BridgeSupervisor::with_binary(binary.into());
        supervisor.start().unwrap();
        let workspace = dir.path().join("workspace");
        fs::create_dir(&workspace).unwrap();
        supervisor
            .open_session(crate::bridge::OpenSessionRequest {
                session_id: "tauri-notification-test".into(),
                workspace_root: Some(workspace.to_string_lossy().into()),
            })
            .unwrap();
        state.deliver(&request("tauri-notification-test")).unwrap();
        let first = backend.sent.lock().unwrap()[0].0.clone();
        state.receive(&first);
        let target = state.resolve(&supervisor, &first).unwrap().unwrap();
        assert_eq!(target.session_id, "tauri-notification-test");
        assert_eq!(
            target.workspace_root.as_deref(),
            Some(workspace.to_str().unwrap())
        );
        state.deliver(&request("never-existed")).unwrap();
        let unknown = backend.sent.lock().unwrap()[1].0.clone();
        state.receive(&unknown);
        assert!(state.resolve(&supervisor, &unknown).unwrap().is_none());
        assert_eq!(
            supervisor
                .snapshot(SessionRequest {
                    session_id: "tauri-notification-test".into()
                })
                .unwrap()
                .session
                .id,
            "tauri-notification-test"
        );
        supervisor.stop().unwrap();
    }
}
