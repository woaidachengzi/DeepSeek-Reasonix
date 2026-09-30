//! Actual WKWebView IPC -> clipboard plugin -> system pasteboard acceptance.
//! Page-load receipts carry only a nonce/status, never clipboard contents.
use std::{
    path::Path,
    sync::{
        atomic::{AtomicBool, Ordering},
        mpsc, Mutex,
    },
    time::{Duration, Instant},
};

use tauri::{AppHandle, Manager, Webview, WebviewUrl, WebviewWindow, WebviewWindowBuilder};
use tauri_plugin_clipboard_manager::ClipboardExt;

use crate::native_window_smoke::on_main;

struct Context {
    nonce: String,
    send: mpsc::SyncSender<(String, String)>,
}
pub struct ClipboardSmokeState {
    enabled: bool,
    main_loaded: AtomicBool,
    active: Mutex<Option<Context>>,
}
impl Default for ClipboardSmokeState {
    fn default() -> Self {
        Self {
            enabled: std::env::var("REASONIX_TAURI_NATIVE_WINDOW_SMOKE").as_deref()
                == Ok("clipboard-native"),
            main_loaded: AtomicBool::new(false),
            active: Mutex::new(None),
        }
    }
}

pub fn observe(webview: &Webview, payload: &tauri::webview::PageLoadPayload<'_>) {
    let Some(state) = webview.app_handle().try_state::<ClipboardSmokeState>() else {
        return;
    };
    if !state.enabled {
        return;
    }
    if payload.event() == tauri::webview::PageLoadEvent::Finished
        && webview.label() == "main"
        && payload.url().scheme() == "tauri"
    {
        state.main_loaded.store(true, Ordering::SeqCst);
        return;
    }
    if payload.event() != tauri::webview::PageLoadEvent::Started {
        return;
    }
    let Ok(active) = state.active.lock() else {
        return;
    };
    let Some(context) = active.as_ref() else {
        return;
    };
    let url = payload.url();
    if url.as_str().len() > 4096 {
        return;
    }
    if url.scheme() != "tauri" || url.host_str() != Some("localhost") {
        return;
    }
    let pairs: std::collections::HashMap<_, _> = url.query_pairs().collect();
    if pairs
        .get("native_clipboard_nonce")
        .map(|value| value.as_ref())
        != Some(context.nonce.as_str())
    {
        return;
    }
    let Some(result) = pairs.get("native_clipboard_result") else {
        return;
    };
    if !matches!(
        result.as_ref(),
        "main-ok" | "main-failed" | "denied-ok" | "denied-failed"
    ) {
        return;
    }
    if matches!(webview.label(), "main" | "native-clipboard-denied") {
        let _ = context
            .send
            .try_send((webview.label().into(), result.to_string()));
    }
}

struct ActiveScope(AppHandle);
impl Drop for ActiveScope {
    fn drop(&mut self) {
        if let Ok(mut active) = self.0.state::<ClipboardSmokeState>().active.lock() {
            *active = None;
        }
    }
}
struct HiddenWindow(WebviewWindow);
impl Drop for HiddenWindow {
    fn drop(&mut self) {
        let _ = self.0.destroy();
    }
}

fn script(nonce: &str, denied: bool) -> String {
    let mut source = r#"(() => {
        const nonce = __NONCE__, denied = __DENIED__;
        if (new URL(location.href).searchParams.get('native_clipboard_nonce') === nonce && new URL(location.href).searchParams.has('native_clipboard_result')) return;
        const value = 'reasonix-native-clipboard-' + nonce;
        const report = status => {
            const url = new URL(location.href);
            url.searchParams.set('native_clipboard_nonce', nonce);
            url.searchParams.set('native_clipboard_result', status);
            location.replace(url.href);
        };
        const started = Date.now();
        const ready = async () => {
            if (!window.__TAURI_INTERNALS__?.invoke || location.protocol !== 'tauri:') {
                if (Date.now() - started < 12000) { setTimeout(ready, 25); return; }
                report(denied ? 'denied-failed' : 'main-failed'); return;
            }
            const invoke = (command, args = {}) => window.__TAURI_INTERNALS__.invoke(command, args);
            const rejected = async (command, args) => {
                try { await invoke(command, args); return false; }
                catch (error) {
                    const message = String(error);
                    return message.includes('clipboard-manager') && message.includes('not allowed');
                }
            };
            try {
                if (denied) {
                    const read = await rejected('plugin:clipboard-manager|read_text');
                    const write = await rejected('plugin:clipboard-manager|write_text', { text: value + '-denied' });
                    report(read && write ? 'denied-ok' : 'denied-failed');
                } else {
                    await invoke('plugin:clipboard-manager|write_text', { text: value });
                    const text = await invoke('plugin:clipboard-manager|read_text');
                    const image = await rejected('plugin:clipboard-manager|read_image');
                    report(text === value && image ? 'main-ok' : 'main-failed');
                }
            } catch { report(denied ? 'denied-failed' : 'main-failed'); }
        };
        ready();
    })();"#.to_string();
    source = source.replace(
        "__NONCE__",
        &serde_json::to_string(nonce).expect("encode private nonce"),
    );
    source.replace("__DENIED__", if denied { "true" } else { "false" })
}

fn receipt(
    receive: &mpsc::Receiver<(String, String)>,
    label: &str,
    result: &str,
) -> Result<(), String> {
    match receive.recv_timeout(Duration::from_secs(15)) {
        Ok((actual_label, actual_result)) if actual_label == label && actual_result == result => {
            Ok(())
        }
        Ok(_) => Err("native WebView clipboard permission/readback acceptance failed".into()),
        Err(_) => Err("native WebView clipboard receipt timed out".into()),
    }
}

pub fn run(app: &AppHandle, directory: &Path) -> Result<(), String> {
    let control: serde_json::Value = serde_json::from_slice(
        &std::fs::read(directory.join("reasonix-native-clipboard-control.json"))
            .map_err(|_| "private clipboard control missing")?,
    )
    .map_err(|_| "private clipboard control invalid")?;
    let nonce = control
        .get("nonce")
        .and_then(|value| value.as_str())
        .ok_or("clipboard nonce missing")?;
    if nonce.len() != 32 || !nonce.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        return Err("clipboard nonce invalid".into());
    }
    let (send, receive) = mpsc::sync_channel(2);
    *app.state::<ClipboardSmokeState>()
        .active
        .lock()
        .map_err(|_| "clipboard acceptance lock failed")? = Some(Context {
        nonce: nonce.into(),
        send,
    });
    let _scope = ActiveScope(app.clone());
    let deadline = Instant::now() + Duration::from_secs(12);
    while !app
        .state::<ClipboardSmokeState>()
        .main_loaded
        .load(Ordering::SeqCst)
    {
        if Instant::now() >= deadline {
            return Err("native clipboard main document did not finish loading".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
    let main_script = script(nonce, false);
    let result = (|| {
        on_main(app, move |_, window| {
            window
                .eval(&main_script)
                .map_err(|_| "evaluate native clipboard IPC".into())
        })?;
        receipt(&receive, "main", "main-ok")?;
        let denied_script = script(nonce, true);
        let _hidden = HiddenWindow(on_main(app, move |handle, _| {
            let window = WebviewWindowBuilder::new(
                handle,
                "native-clipboard-denied",
                WebviewUrl::App("index.html".into()),
            )
            .visible(false)
            .focused(false)
            .initialization_script(denied_script)
            .build()
            .map_err(|_| "create clipboard capability probe")?;
            Ok(window)
        })?);
        receipt(&receive, "native-clipboard-denied", "denied-ok")?;
        if app
            .clipboard()
            .read_text()
            .map_err(|_| "read native clipboard plugin")?
            != format!("reasonix-native-clipboard-{nonce}")
        {
            return Err("denied WebView changed the actual system clipboard".into());
        }
        Ok(())
    })();
    // Even a failed/interrupted probe can safely restore its unique canary.
    if app.clipboard().read_text().is_ok_and(|value| {
        value == format!("reasonix-native-clipboard-{nonce}")
            || value == format!("reasonix-native-clipboard-{nonce}-denied")
    }) {
        let count = on_main(app, |_, _| {
            Ok(objc2_app_kit::NSPasteboard::generalPasteboard().changeCount())
        })?;
        std::fs::write(
            directory.join("reasonix-native-clipboard-owned.json"),
            serde_json::json!({"nonce":nonce,"count":count}).to_string(),
        )
        .map_err(|_| "publish clipboard generation")?;
    }
    result
}
