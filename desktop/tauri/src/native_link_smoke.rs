//! Opt-in main WKWebView IPC -> production opener -> actual default browser.
//! Only private loopback canaries are opened; no renderer command is added.
use std::{
    path::Path,
    sync::{
        atomic::{AtomicBool, Ordering},
        mpsc, Mutex,
    },
    time::{Duration, Instant},
};
use tauri::{AppHandle, Manager, Webview};

#[derive(Default)]
pub struct LinkSmokeState {
    loaded: AtomicBool,
    active: Mutex<Option<(String, mpsc::SyncSender<bool>)>>,
}

pub fn observe(webview: &Webview, payload: &tauri::webview::PageLoadPayload<'_>) {
    if std::env::var("REASONIX_TAURI_NATIVE_WINDOW_SMOKE").as_deref() != Ok("external-browser")
        || webview.label() != "main"
        || payload.url().scheme() != "tauri"
        || payload.url().host_str() != Some("localhost")
    {
        return;
    }
    let Some(state) = webview.app_handle().try_state::<LinkSmokeState>() else {
        return;
    };
    if payload.event() == tauri::webview::PageLoadEvent::Finished {
        state.loaded.store(true, Ordering::SeqCst);
    }
    if payload.event() == tauri::webview::PageLoadEvent::Started {
        let Ok(active) = state.active.lock() else {
            return;
        };
        let Some((nonce, send)) = active.as_ref() else {
            return;
        };
        if payload.url().as_str().len() > 4096 {
            return;
        }
        let pairs: std::collections::HashMap<_, _> = payload.url().query_pairs().collect();
        if pairs.get("native_link_nonce").map(|value| value.as_ref()) == Some(nonce.as_str()) {
            if let Some(status) = pairs.get("native_link_result") {
                let _ = send.try_send(status == "ok");
            }
        }
    }
}

struct Scope(AppHandle);
impl Drop for Scope {
    fn drop(&mut self) {
        if let Ok(mut active) = self.0.state::<LinkSmokeState>().active.lock() {
            *active = None;
        }
    }
}

pub fn run(app: &AppHandle, directory: &Path) -> Result<(), String> {
    let control: serde_json::Value = serde_json::from_slice(
        &std::fs::read(directory.join("reasonix-native-link-control.json"))
            .map_err(|_| "private link control missing")?,
    )
    .map_err(|_| "private link control invalid")?;
    let nonce = control
        .get("nonce")
        .and_then(|value| value.as_str())
        .filter(|nonce| nonce.len() == 32 && nonce.bytes().all(|byte| byte.is_ascii_hexdigit()))
        .ok_or("invalid link nonce")?
        .to_string();
    let port = control
        .get("port")
        .and_then(|value| value.as_u64())
        .filter(|port| *port > 0 && *port <= u16::MAX as u64)
        .ok_or("invalid loopback port")?;
    let (send, receive) = mpsc::sync_channel(1);
    *app.state::<LinkSmokeState>()
        .active
        .lock()
        .map_err(|_| "link probe lock failed")? = Some((nonce.clone(), send));
    let _scope = Scope(app.clone());
    let deadline = Instant::now() + Duration::from_secs(12);
    while !app.state::<LinkSmokeState>().loaded.load(Ordering::SeqCst) {
        if Instant::now() >= deadline {
            return Err("main link document load timed out".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
    let source = r#"(async () => {
        const nonce = __NONCE__, base = __BASE__;
        const report = result => {
            const receipt = new URL(location.href);
            receipt.searchParams.set('native_link_nonce', nonce);
            receipt.searchParams.set('native_link_result', result);
            location.replace(receipt.href);
        };
        try {
            const invoke = window.__TAURI_INTERNALS__.invoke;
            const rejected = async (command, url, expected) => {
                try { await invoke(command, {url}); throw new Error('accepted invalid link'); }
                catch (error) { if (!String(error).includes(expected)) throw error; }
            };
            for (const command of ['open_external_link', 'open_external_url']) {
                await rejected(command, 'file:///private/tmp/must-not-open', 'HTTP(S)');
                await rejected(command, 'reasonix-unregistered:must-not-open', 'HTTP(S)');
                await rejected(command, 'http://user:secret@127.0.0.1/', 'without userinfo');
                await rejected(command, 'https://example.test/\u0000', 'invalid');
            }
            await rejected('open_external_link', 'mailto:probe@example.test?attach=/private/tmp/must-not-open', 'standard recipients');
            await rejected('open_external_url', 'mailto:probe@example.test', 'HTTP(S)');
            for (const command of ['open_external_link', 'open_external_url']) {
                await invoke(command, {url: base + '/' + command});
            }
            report('ok');
        } catch { report('failed'); }
    })();"#
        .replace("__NONCE__", &serde_json::to_string(&nonce).map_err(|_| "encode link nonce")?)
        .replace("__BASE__", &serde_json::to_string(&format!("http://127.0.0.1:{port}/{nonce}")).map_err(|_| "encode loopback URL")?);
    crate::native_window_smoke::on_main(app, move |_, window| {
        window
            .eval(&source)
            .map_err(|_| "evaluate main link IPC".into())
    })?;
    if !receive
        .recv_timeout(Duration::from_secs(15))
        .map_err(|_| "main link IPC timed out")?
    {
        return Err("main link validation/native opener failed".into());
    }
    let deadline = Instant::now() + Duration::from_secs(15);
    loop {
        let observed = ["open_external_link", "open_external_url"]
            .iter()
            .all(|command| {
                std::fs::read_to_string(
                    directory.join(format!("reasonix-native-link-{command}.receipt")),
                )
                .is_ok_and(|value| value == nonce)
            });
        if observed {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err("actual browser loopback receipt timed out".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}
