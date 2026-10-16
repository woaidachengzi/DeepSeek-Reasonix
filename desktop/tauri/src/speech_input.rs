use crate::bridge::{request_json_with_connected_socket, BridgeSupervisor};
use serde::{Deserialize, Serialize};
use std::{
    collections::{HashMap, VecDeque},
    net::{Shutdown, TcpStream},
    sync::{
        atomic::{AtomicBool, Ordering},
        Arc, Mutex,
    },
    time::Duration,
};
use tauri::{Manager, State};

const CANCELLED: &str = "语音识别已取消；可以重新录音。";
#[derive(Default)]
struct Operation {
    cancelled: AtomicBool,
    socket: Mutex<Option<TcpStream>>,
}
impl Operation {
    fn cancel(&self) {
        self.cancelled.store(true, Ordering::SeqCst);
        if let Ok(socket) = self.socket.lock() {
            if let Some(socket) = socket.as_ref() {
                let _ = socket.shutdown(Shutdown::Both);
            }
        }
    }
    fn connect(&self, socket: &TcpStream) -> Result<(), String> {
        let mut slot = self.socket.lock().map_err(|_| CANCELLED)?;
        if self.cancelled.load(Ordering::SeqCst) {
            return Err(CANCELLED.into());
        }
        *slot = Some(socket.try_clone().map_err(|_| CANCELLED)?);
        Ok(())
    }
}
#[derive(Default)]
struct Registry {
    active: HashMap<String, Arc<Operation>>,
    retired: VecDeque<String>,
}
#[derive(Default)]
pub(crate) struct SpeechInput {
    registry: Mutex<Registry>,
}
fn valid_id(id: &str) -> bool {
    (16..=64).contains(&id.len()) && id.bytes().all(|b| b.is_ascii_hexdigit() || b == b'-')
}
impl SpeechInput {
    fn begin(&self, id: &str) -> Result<Arc<Operation>, String> {
        if !valid_id(id) {
            return Err("invalid speech operation".into());
        }
        let mut registry = self.registry.lock().map_err(|_| CANCELLED)?;
        if registry.retired.iter().any(|old| old == id) || registry.active.contains_key(id) {
            return Err(CANCELLED.into());
        }
        if registry.active.len() >= 2 {
            return Err("语音识别繁忙；请先取消当前识别。".into());
        }
        let operation = Arc::new(Operation::default());
        registry.active.insert(id.into(), operation.clone());
        Ok(operation)
    }
    fn retire(registry: &mut Registry, id: &str) {
        if !registry.retired.iter().any(|old| old == id) {
            registry.retired.push_back(id.into());
        }
        while registry.retired.len() > 64 {
            registry.retired.pop_front();
        }
    }
    fn finish(&self, id: &str, operation: &Arc<Operation>) {
        if let Ok(mut registry) = self.registry.lock() {
            if registry
                .active
                .get(id)
                .is_some_and(|current| Arc::ptr_eq(current, operation))
            {
                registry.active.remove(id);
                Self::retire(&mut registry, id);
            }
        }
    }
    pub(crate) fn cancel(&self, id: &str) -> Result<(), String> {
        if !valid_id(id) {
            return Err("invalid speech operation".into());
        }
        let mut registry = self.registry.lock().map_err(|_| CANCELLED)?;
        if let Some(operation) = registry.active.get(id) {
            operation.cancel();
        }
        Self::retire(&mut registry, id);
        Ok(())
    }
    pub(crate) fn cancel_all(&self) {
        if let Ok(registry) = self.registry.lock() {
            for operation in registry.active.values() {
                operation.cancel();
            }
        }
    }
}
#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct SpeechProvider {
    name: String,
    label: String,
    #[serde(default)]
    preferred: bool,
    #[serde(default)]
    description: String,
}
#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct SpeechCatalog {
    protocol_version: u64,
    providers: Vec<SpeechProvider>,
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct SpeechRequest {
    operation_id: String,
    provider: String,
    audio: String,
}
#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct SpeechTranscript {
    protocol_version: u64,
    text: String,
}

// Settings never return credentials. The Go bridge validates revisions and
// stores write-only keys using the same credential store as other settings.
#[tauri::command]
pub(crate) async fn speech_settings(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
) -> Result<serde_json::Value, String> {
    crate::remote_controller::ensure_main_window(window.label())?;
    speech_settings_request(app, None).await
}
#[tauri::command]
pub(crate) async fn save_speech_settings(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    input: serde_json::Value,
) -> Result<serde_json::Value, String> {
    crate::remote_controller::ensure_main_window(window.label())?;
    if !input.is_object() || input.to_string().len() > 16_384 {
        return Err("语音设置过大或无效；请检查输入。".into());
    }
    speech_settings_request(app, Some(input)).await
}
async fn speech_settings_request(
    app: tauri::AppHandle,
    input: Option<serde_json::Value>,
) -> Result<serde_json::Value, String> {
    tauri::async_runtime::spawn_blocking(move || {
        let (address, token) = app.state::<BridgeSupervisor>().event_connection()?;
        crate::bridge::request_json_with_timeout(
            address,
            &token,
            if input.is_some() { "POST" } else { "GET" },
            "/v1/settings/speech",
            input,
            None,
            Duration::from_secs(10),
        )
    })
    .await
    .map_err(|_| "语音设置读取失败；请重试。".to_string())?
}

#[tauri::command]
pub(crate) async fn speech_providers(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
) -> Result<SpeechCatalog, String> {
    crate::remote_controller::ensure_main_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        let supervisor = app.state::<BridgeSupervisor>();
        let (address, token) = supervisor.event_connection()?;
        let result = crate::bridge::request_json_with_timeout(
            address,
            &token,
            "GET",
            "/v1/speech/providers",
            None,
            None,
            Duration::from_secs(5),
        )?;
        let catalog: SpeechCatalog =
            serde_json::from_value(result).map_err(|_| "invalid speech catalogue")?;
        if catalog.protocol_version != 1 || catalog.providers.len() > 500 {
            return Err("invalid speech catalogue".into());
        }
        Ok(catalog)
    })
    .await
    .map_err(|_| "speech catalogue worker failed".to_string())?
}

#[tauri::command]
pub(crate) async fn speech_transcribe(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    input: SpeechRequest,
) -> Result<SpeechTranscript, String> {
    crate::remote_controller::ensure_main_window(window.label())?;
    if input.audio.len() > 3_840_060 || input.provider.len() > 256 {
        return Err("录音过大；请缩短后重试。".into());
    }
    let operation = app.state::<SpeechInput>().begin(&input.operation_id)?;
    tauri::async_runtime::spawn_blocking(move || {
        let result = (|| {
            if operation.cancelled.load(Ordering::SeqCst) {
                return Err(CANCELLED.into());
            }
            let (address, token) = app.state::<BridgeSupervisor>().event_connection()?;
            let value = request_json_with_connected_socket(
                address,
                &token,
                "POST",
                "/v1/speech/transcribe",
                Some(serde_json::json!({"provider":input.provider,"audio":input.audio})),
                None,
                Duration::from_secs(50),
                |socket| operation.connect(socket),
            )?;
            let result: SpeechTranscript =
                serde_json::from_value(value).map_err(|_| "invalid speech transcript")?;
            if result.protocol_version != 1
                || result.text.trim().is_empty()
                || result.text.len() > 32_000
                || result.text.contains('\0')
            {
                return Err("语音识别结果无效；请重新录音。".into());
            }
            Ok(result)
        })();
        let cancelled = operation.cancelled.load(Ordering::SeqCst);
        app.state::<SpeechInput>()
            .finish(&input.operation_id, &operation);
        if cancelled {
            Err(CANCELLED.into())
        } else {
            result
        }
    })
    .await
    .map_err(|_| "speech worker failed".to_string())?
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct SpeechSynthesisRequest {
    operation_id: String,
    provider: String,
    text: String,
    voice: String,
}
#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct SpeechAudio {
    protocol_version: u64,
    audio: String,
}
#[tauri::command]
pub(crate) async fn speech_synthesize(
    window: tauri::WebviewWindow,
    app: tauri::AppHandle,
    input: SpeechSynthesisRequest,
) -> Result<SpeechAudio, String> {
    crate::remote_controller::ensure_main_window(window.label())?;
    if input.text.len() > 6000 || input.provider.len() > 64 || input.voice.len() > 32 {
        return Err("播报内容无效；请缩短后重试。".into());
    }
    let operation = app.state::<SpeechInput>().begin(&input.operation_id)?;
    tauri::async_runtime::spawn_blocking(move || {
  let result=(|| {
   if operation.cancelled.load(Ordering::SeqCst) { return Err(CANCELLED.into()); }
   let (address,token)=app.state::<BridgeSupervisor>().event_connection()?;
   let value=request_json_with_connected_socket(address,&token,"POST","/v1/speech/synthesize",Some(serde_json::json!({"provider":input.provider,"text":input.text,"voice":input.voice})),None,Duration::from_secs(50),|socket|operation.connect(socket))?;
   let result: SpeechAudio=serde_json::from_value(value).map_err(|_|"播报音频无效；请重试。")?;
   if result.protocol_version!=1 || result.audio.is_empty() || result.audio.len()>11_184_812 { return Err("播报音频无效或过大；请重试。".into()); }
   Ok(result)
  })();
  let cancelled=operation.cancelled.load(Ordering::SeqCst);
  app.state::<SpeechInput>().finish(&input.operation_id,&operation);
  if cancelled { Err(CANCELLED.into()) } else { result }
 }).await.map_err(|_|"语音播报已中断；请重试。".to_string())?
}

#[tauri::command]
pub(crate) fn speech_cancel(
    window: tauri::WebviewWindow,
    state: State<'_, SpeechInput>,
    operation_id: String,
) -> Result<(), String> {
    crate::remote_controller::ensure_main_window(window.label())?;
    state.cancel(&operation_id)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn cancellation_before_connect_and_replay_are_rejected() {
        let state = SpeechInput::default();
        let id = "0123456789abcdef";
        state.cancel(id).unwrap();
        assert!(state.begin(id).is_err());
        let next = "0123456789abcdee";
        let op = state.begin(next).unwrap();
        assert!(state.begin(next).is_err());
        state.cancel(next).unwrap();
        assert!(op.cancelled.load(Ordering::SeqCst));
        state.finish(next, &op);
        assert!(state.begin(next).is_err());
    }
    #[test]
    fn cancellation_shuts_down_an_actual_socket() {
        use std::{io::Read, net::TcpListener};
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let client = TcpStream::connect(listener.local_addr().unwrap()).unwrap();
        let (mut peer, _) = listener.accept().unwrap();
        peer.set_read_timeout(Some(Duration::from_secs(2))).unwrap();
        let op = Operation::default();
        op.connect(&client).unwrap();
        op.cancel();
        assert_eq!(peer.read(&mut [0; 1]).unwrap(), 0);
        assert!(op.connect(&client).is_err());
    }
    #[test]
    fn admission_is_bounded_and_finish_releases() {
        let state = SpeechInput::default();
        let first = state.begin("0000000000000001").unwrap();
        state.begin("0000000000000002").unwrap();
        assert!(state.begin("0000000000000003").is_err());
        state.finish("0000000000000001", &first);
        assert!(state.begin("0000000000000003").is_ok());
        state.cancel_all();
    }
}
