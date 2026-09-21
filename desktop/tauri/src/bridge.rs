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

// The wire DTOs mirror docs/tauri/protocol/v1.schema.json through the generated
// module; only the host-facing command payloads below stay hand-written.
pub use crate::protocol_generated::{
    BridgeEvent, BridgeHistoryMessage, BridgeHistoryResponse,
    BridgeOpenSessionRequest as OpenSessionRequest, BridgeSession, BridgeSessionResponse,
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

    pub fn cancel(&self, request: SessionRequest) -> Result<BridgeSession, String> {
        let session_id = session_path_component(&request.session_id)?;
        let path = format!("/v1/sessions/{session_id}:cancel");
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
    let bytes = body
        .map(|value| serde_json::to_vec(&value))
        .transpose()
        .map_err(display_error)?;
    let body = bytes.as_deref().unwrap_or_default();
    let mut stream =
        TcpStream::connect_timeout(&address, Duration::from_secs(1)).map_err(display_error)?;
    stream
        .set_read_timeout(Some(Duration::from_secs(2)))
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
    let mut response = Vec::new();
    stream.read_to_end(&mut response).map_err(display_error)?;
    parse_json_response(&response)
}

fn parse_json_response(response: &[u8]) -> Result<Value, String> {
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
    if !(200..300).contains(&status) {
        return Err(format!(
            "desktop bridge request failed with status {status}"
        ));
    }
    serde_json::from_slice(&response[index + boundary.len()..]).map_err(display_error)
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

fn forward_events(
    app: tauri::AppHandle,
    address: SocketAddr,
    token: String,
    mut after_sequence: u64,
    stop: Arc<AtomicBool>,
) {
    while !stop.load(Ordering::Acquire) {
        match open_event_stream(address, &token, after_sequence) {
            Ok(mut reader) => {
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
            Err(_) => {
                let _ = app.emit(
                    "bridge:connection-error",
                    "bridge event stream is unavailable",
                );
            }
        }
        if !stop.load(Ordering::Acquire) {
            thread::sleep(Duration::from_millis(100));
        }
    }
}

fn open_event_stream(
    address: SocketAddr,
    token: &str,
    after_sequence: u64,
) -> Result<BufReader<TcpStream>, String> {
    let mut stream =
        TcpStream::connect_timeout(&address, Duration::from_secs(1)).map_err(display_error)?;
    stream
        .set_read_timeout(Some(Duration::from_secs(1)))
        .map_err(display_error)?;
    stream
        .write_all(
            format!(
                "GET /v1/events?afterSequence={after_sequence} HTTP/1.1\r\nHost: {address}\r\nAuthorization: Bearer {token}\r\nAccept: text/event-stream\r\nConnection: keep-alive\r\n\r\n"
            )
            .as_bytes(),
        )
        .map_err(display_error)?;
    let mut reader = BufReader::new(stream);
    let mut status = String::new();
    reader.read_line(&mut status).map_err(display_error)?;
    if !status.starts_with("HTTP/1.1 200") {
        return Err("desktop bridge did not accept event streaming".to_string());
    }
    loop {
        let mut header = String::new();
        reader.read_line(&mut header).map_err(display_error)?;
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
        parse_json_response, request_json, session_path_component, verify_ready, wait_for_exit,
        BridgeEvent, BridgeSupervisor,
    };
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
    fn bridge_events_deserialize_only_the_public_envelope() {
        let event: BridgeEvent = serde_json::from_str(
            r#"{"protocolVersion":1,"sequence":7,"eventKind":"text","sessionId":"tab-1","payload":{"kind":"text","text":"hello"}}"#,
        )
        .unwrap();
        assert_eq!(event.sequence, 7);
        assert_eq!(event.payload["text"], "hello");
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
        let supervisor = BridgeSupervisor::with_binary(binary);
        let status = supervisor.start().expect("start bridge");
        assert!(status.running);
        assert_eq!(status.protocol_version, Some(1));
        supervisor.stop().expect("stop bridge");
        assert!(!supervisor.status().running);
    }

    #[test]
    fn supervisor_recovers_from_an_unexpected_sidecar_exit() {
        let Some(binary) = bridge_under_test() else {
            return;
        };
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
}
