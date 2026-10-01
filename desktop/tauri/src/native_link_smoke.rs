//! Opt-in main WKWebView IPC -> production external browser/application entry.
//! Only private canaries are used; no renderer command is added.
use std::{
    path::Path,
    sync::{
        atomic::{AtomicBool, Ordering},
        mpsc, Mutex,
    },
    time::{Duration, Instant},
};
use tauri::{AppHandle, Manager, Webview, WebviewUrl, WebviewWindow, WebviewWindowBuilder};

#[derive(Default)]
pub struct LinkSmokeState {
    loaded: AtomicBool,
    active: Mutex<Option<Context>>,
}
struct Context {
    nonce: String,
    send: mpsc::SyncSender<Receipt>,
}
struct Receipt {
    label: String,
    status: String,
}

pub fn observe(webview: &Webview, payload: &tauri::webview::PageLoadPayload<'_>) {
    if !matches!(
        std::env::var("REASONIX_TAURI_NATIVE_WINDOW_SMOKE").as_deref(),
        Ok("external-browser" | "external-app-failure" | "external-terminal" | "document-scope")
    ) || !(webview.label() == "main"
        || (webview.label() == "native-document-denied"
            && std::env::var("REASONIX_TAURI_NATIVE_WINDOW_SMOKE").as_deref()
                == Ok("document-scope")))
        || payload.url().scheme() != "tauri"
        || payload.url().host_str() != Some("localhost")
    {
        return;
    }
    let Some(state) = webview.app_handle().try_state::<LinkSmokeState>() else {
        return;
    };
    if payload.event() == tauri::webview::PageLoadEvent::Finished && webview.label() == "main" {
        state.loaded.store(true, Ordering::SeqCst);
    }
    if payload.event() == tauri::webview::PageLoadEvent::Started {
        let Ok(active) = state.active.lock() else {
            return;
        };
        let Some(context) = active.as_ref() else {
            return;
        };
        if payload.url().as_str().len() > 4096 {
            return;
        }
        let pairs: std::collections::HashMap<_, _> = payload.url().query_pairs().collect();
        if pairs.get("native_link_nonce").map(|value| value.as_ref())
            == Some(context.nonce.as_str())
        {
            if let Some(status) = pairs.get("native_link_result") {
                if matches!(
                    status.as_ref(),
                    "ok" | "failed" | "unexpected-native-access"
                ) {
                    let _ = context.send.try_send(Receipt {
                        label: webview.label().into(),
                        status: status.to_string(),
                    });
                }
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

struct HiddenWindow(WebviewWindow);
impl Drop for HiddenWindow {
    fn drop(&mut self) {
        let _ = self.0.destroy();
    }
}

fn document_script(directory: &Path, nonce: &str, denied: bool) -> Result<String, String> {
    let args = serde_json::json!({
        "document": directory.join("document 中文\n\"$.md"),
        "executable": directory.join("executable-canary.sh"),
        "alias": directory.join("alias.md"),
    });
    Ok(r#"(async () => {
        const nonce = __NONCE__, denied = __DENIED__, paths = __PATHS__;
        if (new URL(location.href).searchParams.get('native_link_nonce') === nonce && new URL(location.href).searchParams.has('native_link_result')) return;
        const report = result => {
            const receipt = new URL(location.href);
            receipt.searchParams.set('native_link_nonce', nonce);
            receipt.searchParams.set('native_link_result', result);
            location.replace(receipt.href);
        };
        try {
            const deadline = Date.now() + 12000;
            while (!window.__TAURI_INTERNALS__?.invoke) {
                if (Date.now() >= deadline) throw new Error('IPC unavailable');
                await new Promise(resolve => setTimeout(resolve, 25));
            }
            const invoke = window.__TAURI_INTERNALS__.invoke;
            const rejected = async (command, args, expected) => {
                try { await invoke(command, args); throw new Error('accepted forbidden action: ' + command); }
                catch (error) { if (!String(error).includes(expected)) throw error; }
            };
            if (denied) {
                const actions = [
                    ['open_local_path', {path: paths.document}],
                    ['reveal_local_path', {path: paths.document}],
                    ['save_local_path_as', {path: paths.document}],
                    ['local_path_openers', {}],
                    ['set_preferred_external_opener', {id: 'finder'}],
                    ['open_local_path_with', {path: paths.document, id: 'finder'}],
                    ['workspace_external_openers', {sessionId: 'private-denied-' + nonce}],
                    ['open_workspace_external', {sessionId: 'private-denied-' + nonce, id: 'finder'}],
                    ['desktop_preferences', {}],
                    ['bridge_status', {}],
                ];
                for (const [command, args] of actions) {
                    // Extra renderer input cannot impersonate the injected
                    // native caller window used by each command's guard.
                    await rejected(command, {...args, window: 'main'}, 'require the main window');
                }
            } else {
                for (const path of [paths.executable, paths.alias]) {
                    await rejected('open_local_path', {path}, 'cannot open an executable target');
                    await rejected('open_local_path_with', {path, id: 'finder'}, 'cannot open an executable target');
                }
                const catalog = await invoke('local_path_openers');
                for (const id of ['finder', 'terminal']) {
                    if (!catalog.openers.some(item => item.id === id && item.iconDataUrl.startsWith('data:image/png;base64,'))) throw new Error('native catalog missing');
                }
            }
            report('ok');
        } catch (error) { report(String(error).includes('accepted forbidden action: desktop_preferences') ? 'unexpected-native-access' : 'failed'); }
    })();"#.replace("__NONCE__", &serde_json::to_string(nonce).map_err(|_| "encode scope nonce")?)
        .replace("__DENIED__", if denied { "true" } else { "false" })
        .replace("__PATHS__", &args.to_string()))
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
    let application_failure = std::env::var("REASONIX_TAURI_NATIVE_WINDOW_SMOKE").as_deref()
        == Ok("external-app-failure");
    let terminal =
        std::env::var("REASONIX_TAURI_NATIVE_WINDOW_SMOKE").as_deref() == Ok("external-terminal");
    let document_scope =
        std::env::var("REASONIX_TAURI_NATIVE_WINDOW_SMOKE").as_deref() == Ok("document-scope");
    let port = if application_failure || terminal || document_scope {
        0
    } else {
        control
            .get("port")
            .and_then(|value| value.as_u64())
            .filter(|port| *port > 0 && *port <= u16::MAX as u64)
            .ok_or("invalid loopback port")?
    };
    let (send, receive) = mpsc::sync_channel(1);
    *app.state::<LinkSmokeState>()
        .active
        .lock()
        .map_err(|_| "link probe lock failed")? = Some(Context {
        nonce: nonce.clone(),
        send,
    });
    let _scope = Scope(app.clone());
    let deadline = Instant::now() + Duration::from_secs(12);
    while !app.state::<LinkSmokeState>().loaded.load(Ordering::SeqCst) {
        if Instant::now() >= deadline {
            return Err("main link document load timed out".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
    let source = if document_scope {
        document_script(directory, &nonce, false)?
    } else if application_failure {
        let expected = std::path::PathBuf::from(std::env::var_os("HOME").ok_or("private HOME missing")?)
            .join("Applications/Ghostty.app").canonicalize().map_err(|_| "private invalid bundle missing")?;
        let selected = crate::opener_catalog::selected_opener(crate::opener_catalog::installed_openers(), "ghostty")?;
        if selected.target != expected {
            return Err("Ghostty catalog does not resolve to the private fixture; no application was operated".into());
        }
        let path = directory.join("document 中文\n\"$.md");
        let path = path.to_str().ok_or("private document is not UTF-8")?;
        r#"(async () => {
            const receipt = new URL(location.href);
            receipt.searchParams.set('native_link_nonce', __NONCE__);
            const invoke = window.__TAURI_INTERNALS__.invoke;
            const rejected = async id => {
                try { await invoke('open_local_path_with', {path: __PATH__, id}); return ''; }
                catch (error) { return String(error); }
            };
            try {
                const missing = await rejected('reasonix-not-installed-canary');
                const broken = await rejected('ghostty');
                receipt.searchParams.set('native_link_result',
                    missing.includes('no longer installed') && broken.includes('cannot open selected application; choose an installed application and retry')
                    ? 'ok' : 'failed');
            } catch { receipt.searchParams.set('native_link_result', 'failed'); }
            location.replace(receipt.href);
        })();"#.replace("__PATH__", &serde_json::to_string(path).map_err(|_| "encode private document path")?)
    } else if terminal {
        let expected = std::path::Path::new("/System/Applications/Utilities/Terminal.app")
            .canonicalize().map_err(|_| "system Terminal missing")?;
        let selected = crate::opener_catalog::selected_opener(crate::opener_catalog::installed_openers(), "terminal")?;
        if selected.target != expected {
            return Err("Terminal catalog target is not the system application".into());
        }
        let root = directory.join("workspace 中文\n\"$");
        let file = root.join("document.md");
        let root = serde_json::to_string(root.to_str().ok_or("invalid private workspace path")?).map_err(|_| "encode workspace path")?;
        let file = serde_json::to_string(file.to_str().ok_or("invalid private document path")?).map_err(|_| "encode document path")?;
        r#"(async () => {
            const receipt = new URL(location.href);
            receipt.searchParams.set('native_link_nonce', __NONCE__);
            try {
                const invoke = window.__TAURI_INTERNALS__.invoke;
                await invoke('open_local_path_with', {path: __ROOT__, id: 'terminal'});
                await invoke('open_local_path_with', {path: __FILE__, id: 'terminal'});
                receipt.searchParams.set('native_link_result', 'ok');
            } catch { receipt.searchParams.set('native_link_result', 'failed'); }
            location.replace(receipt.href);
        })();"#.replace("__ROOT__", &root).replace("__FILE__", &file)
    } else { r#"(async () => {
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
    })();"#.to_string() }
        .replace("__NONCE__", &serde_json::to_string(&nonce).map_err(|_| "encode link nonce")?)
        .replace("__BASE__", &serde_json::to_string(&format!("http://127.0.0.1:{port}/{nonce}")).map_err(|_| "encode loopback URL")?);
    crate::native_window_smoke::on_main(app, move |_, window| {
        window
            .eval(&source)
            .map_err(|_| "evaluate main link IPC".into())
    })?;
    let main_receipt = receive
        .recv_timeout(Duration::from_secs(15))
        .map_err(|_| "main link IPC timed out")?;
    if main_receipt.label != "main" || main_receipt.status != "ok" {
        return Err("main link validation/native opener failed".into());
    }
    if document_scope {
        let script = document_script(directory, &nonce, true)?;
        let _hidden = HiddenWindow(crate::native_window_smoke::on_main(
            app,
            move |handle, _| {
                WebviewWindowBuilder::new(
                    handle,
                    "native-document-denied",
                    WebviewUrl::App("index.html".into()),
                )
                .visible(false)
                .focused(false)
                .initialization_script(script)
                .build()
                .map_err(|_| "create document caller scope probe".into())
            },
        )?);
        let hidden_receipt = receive
            .recv_timeout(Duration::from_secs(15))
            .map_err(|_| "hidden document scope IPC timed out")?;
        if hidden_receipt.label != "native-document-denied" || hidden_receipt.status != "ok" {
            return Err(format!(
                "hidden document/native caller scope was not rejected: {}",
                hidden_receipt.status
            ));
        }
        return Ok(());
    }
    if application_failure {
        return Ok(());
    }
    if terminal {
        let deadline = Instant::now() + Duration::from_secs(15);
        loop {
            if std::fs::read_to_string(directory.join("reasonix-native-terminal.receipt"))
                .is_ok_and(|value| value == nonce)
            {
                return Ok(());
            }
            if Instant::now() >= deadline {
                return Err("actual Terminal shell cwd receipt timed out".into());
            }
            std::thread::sleep(Duration::from_millis(25));
        }
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
