use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine as _};
use rand::TryRngCore;
use serde::{Deserialize, Serialize};
use std::{
    env, fs,
    io::{Read, Write},
    net::{IpAddr, SocketAddr, TcpStream},
    path::PathBuf,
    process::{Child, Command, Stdio},
    sync::Mutex,
    thread,
    time::{Duration, Instant},
};
use tempfile::TempDir;

const BRIDGE_TOKEN_ENV: &str = "REASONIX_DESKTOP_BRIDGE_TOKEN";
const BRIDGE_BINARY_ENV: &str = "REASONIX_DESKTOP_BRIDGE_BIN";
const READY_TIMEOUT: Duration = Duration::from_secs(5);
const STOP_TIMEOUT: Duration = Duration::from_secs(5);
const PROTOCOL_VERSION: u8 = 1;

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeStatus {
    pub running: bool,
    pub protocol_version: Option<u8>,
    pub sidecar_instance_id: Option<String>,
}

#[derive(Debug)]
pub struct BridgeSupervisor {
    binary: PathBuf,
    process: Mutex<Option<BridgeProcess>>,
}

#[derive(Debug)]
struct BridgeProcess {
    child: Child,
    _ready_directory: TempDir,
    address: SocketAddr,
    token: String,
    sidecar_instance_id: String,
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
    pub fn from_environment() -> Result<Self, String> {
        let binary = env::var_os(BRIDGE_BINARY_ENV).ok_or_else(|| {
            format!("{BRIDGE_BINARY_ENV} must name the reasonix-desktop-bridge executable")
        })?;
        Ok(Self {
            binary: PathBuf::from(binary),
            process: Mutex::new(None),
        })
    }

    pub fn start(&self) -> Result<BridgeStatus, String> {
        let mut process = self
            .process
            .lock()
            .map_err(|_| "bridge state lock is unavailable")?;
        if let Some(existing) = process.as_mut() {
            if existing.child.try_wait().map_err(display_error)?.is_none() {
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
        if existing.child.try_wait().ok().flatten().is_some() {
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

    pub fn stop(&self) -> Result<(), String> {
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
        process.child.kill().map_err(display_error)?;
        process.child.wait().map_err(display_error)?;
        Ok(())
    }

    fn spawn_bridge(&self) -> Result<BridgeProcess, String> {
        if !self.binary.is_file() {
            return Err(format!(
                "desktop bridge executable was not found at {}",
                self.binary.display()
            ));
        }
        let ready_directory = tempfile::Builder::new()
            .prefix("reasonix-tauri-bridge-")
            .tempdir()
            .map_err(display_error)?;
        let ready_file = ready_directory.path().join("ready.json");
        let token = opaque_secret()?;
        let launch_id = opaque_secret()?;
        let mut child = Command::new(&self.binary)
            .arg("--listen")
            .arg("127.0.0.1:0")
            .arg("--ready-file")
            .arg(&ready_file)
            .arg("--launch-id")
            .arg(&launch_id)
            .env(BRIDGE_TOKEN_ENV, &token)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .map_err(display_error)?;

        let ready = wait_for_ready(&mut child, &ready_file, &launch_id).inspect_err(|_| {
            let _ = child.kill();
            let _ = child.wait();
        })?;
        Ok(BridgeProcess {
            child,
            _ready_directory: ready_directory,
            address: ready.address,
            token,
            sidecar_instance_id: ready.sidecar_instance_id,
        })
    }
}

#[derive(Debug)]
struct VerifiedReady {
    address: SocketAddr,
    sidecar_instance_id: String,
}

fn wait_for_ready(
    child: &mut Child,
    ready_file: &PathBuf,
    launch_id: &str,
) -> Result<VerifiedReady, String> {
    let deadline = Instant::now() + READY_TIMEOUT;
    loop {
        if child.try_wait().map_err(display_error)?.is_some() {
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
    let mut stream =
        TcpStream::connect_timeout(&address, Duration::from_secs(1)).map_err(display_error)?;
    stream
        .set_read_timeout(Some(Duration::from_secs(1)))
        .map_err(display_error)?;
    stream
        .write_all(
            format!(
                "POST /v1:shutdown HTTP/1.1\r\nHost: {address}\r\nAuthorization: Bearer {token}\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
            )
            .as_bytes(),
        )
        .map_err(display_error)?;
    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .map_err(display_error)?;
    if response.starts_with("HTTP/1.1 202") {
        Ok(())
    } else {
        Err("desktop bridge did not accept graceful shutdown".to_string())
    }
}

fn wait_for_exit(child: &mut Child, timeout: Duration) -> Result<bool, String> {
    let deadline = Instant::now() + timeout;
    loop {
        if child.try_wait().map_err(display_error)?.is_some() {
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
    use super::verify_ready;

    #[test]
    fn ready_file_requires_matching_loopback_launch() {
        let ready = r#"{"protocolVersion":1,"address":"127.0.0.1:12345","sidecarInstanceId":"instance","launchId":"launch"}"#;
        assert!(verify_ready(ready, "launch").is_ok());
        assert!(verify_ready(ready, "other").is_err());
    }

    #[test]
    fn ready_file_rejects_non_loopback_address() {
        let ready = r#"{"protocolVersion":1,"address":"0.0.0.0:12345","sidecarInstanceId":"instance","launchId":"launch"}"#;
        assert!(verify_ready(ready, "launch").is_err());
    }
}
