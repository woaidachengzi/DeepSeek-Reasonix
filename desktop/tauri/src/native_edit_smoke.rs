//! Installed native edit roles against actual WKWebView textarea state.
//! Only the opt-in private-profile clipboard runner calls this probe.
use std::{
    sync::mpsc,
    time::{Duration, Instant},
};

use block2::RcBlock;
use objc2::{runtime::AnyObject, ClassType, MainThreadMarker};
use objc2_app_kit::{NSApplication, NSWindow};
use objc2_foundation::{NSError, NSObjectProtocol, NSString};
use objc2_web_kit::WKWebView;
use tauri::AppHandle;
use tauri_plugin_clipboard_manager::ClipboardExt;

use crate::{native_menu_smoke, native_window_smoke::on_main};

pub(crate) fn evaluate(app: &AppHandle, source: String) -> Result<bool, String> {
    let (send, receive) = mpsc::sync_channel(1);
    on_main(app, move |_, window| {
        window
            .with_webview(move |platform| {
                // SAFETY: Tauri provides live WKWebView/NSWindow handles on the
                // main thread. The callback only converts a fixed string result.
                let webview = unsafe { &*platform.inner().cast::<WKWebView>() };
                let completed = RcBlock::new(move |value: *mut AnyObject, error: *mut NSError| {
                    let result = if !error.is_null() {
                        Err("WKWebView edit evaluation failed".into())
                    } else {
                        let value = unsafe { value.as_ref() };
                        match value
                            .and_then(|value| value.downcast_ref::<NSString>())
                            .map(|value| value.to_string())
                            .as_deref()
                        {
                            Some("edit-ok") => Ok(true),
                            Some("edit-pending") => Ok(false),
                            _ => Err("WKWebView returned an invalid edit receipt".into()),
                        }
                    };
                    let _ = send.try_send(result);
                });
                unsafe {
                    webview.evaluateJavaScript_completionHandler(
                        &NSString::from_str(&source),
                        Some(&completed),
                    );
                }
            })
            .map_err(|_| "schedule WKWebView edit evaluation".into())
    })?;
    receive
        .recv_timeout(Duration::from_secs(5))
        .map_err(|_| "WKWebView edit receipt timed out".to_string())?
}

fn check(app: &AppHandle, stage: &str, expression: &str) -> Result<(), String> {
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        let source = format!("(() => (({expression}) ? 'edit-ok' : 'edit-pending'))()");
        if evaluate(app, source)? {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err(format!("native edit state did not settle: {stage}"));
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}

fn focus(app: &AppHandle) -> Result<(), String> {
    // Use the same asynchronous restoration path as tray/Dock/second instance.
    // AppKit activation is not established merely by ordering a window front.
    crate::tray::show_main_window(app);
    on_main(app, |_, _| {
        let application =
            NSApplication::sharedApplication(MainThreadMarker::new().ok_or("not main thread")?);
        // On macOS 14+ use the current cooperative activation request for
        // this explicit test. Older macOS keeps Tauri's existing focus path.
        if application.respondsToSelector(objc2::sel!(activate)) {
            application.activate();
        }
        Ok(())
    })?;
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        let actual = focus_state(app)?;
        if actual.0 && actual.1 && actual.2 {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err(format!("native WKWebView could not become the active key-window responder; firstResponderAccepted={}, keyWindow={}, applicationActive={}, windowVisible={}, applicationHidden={}",actual.0,actual.1,actual.2,actual.3,actual.4));
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}

fn focus_state(app: &AppHandle) -> Result<(bool, bool, bool, bool, bool), String> {
    let (send, receive) = mpsc::sync_channel(1);
    on_main(app, move |_, window| {
        window
            .with_webview(move |platform| {
                // SAFETY: These handles belong to this live main WebView; AppKit
                // window/responder operations run only in Tauri's main callback.
                let native_window = unsafe { &*platform.ns_window().cast::<NSWindow>() };
                let webview = unsafe { &*platform.inner().cast::<WKWebView>() };
                let first = native_window.makeFirstResponder(Some(webview.as_super().as_super()));
                let application = NSApplication::sharedApplication(
                    MainThreadMarker::new().expect("platform callback on main"),
                );
                let key = application
                    .keyWindow()
                    .is_some_and(|window| std::ptr::eq(&*window, native_window));
                let _ = send.try_send((
                    first,
                    key,
                    application.isActive(),
                    native_window.isVisible(),
                    application.isHidden(),
                ));
            })
            .map_err(|_| "schedule native edit focus".into())
    })?;
    receive
        .recv_timeout(Duration::from_secs(5))
        .map_err(|_| "native edit focus timed out".into())
}

fn copied(app: &AppHandle, stage: &str, value: &str) -> Result<(), String> {
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        if app
            .clipboard()
            .read_text()
            .is_ok_and(|actual| actual == value)
        {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err(format!("native {stage} did not copy selected text"));
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}

pub fn run(app: &AppHandle, nonce: &str) -> Result<(), String> {
    check(
        app,
        "main document loaded",
        "document.readyState === 'complete' && !!document.body",
    )?;
    let value = format!("reasonix-native-clipboard-{nonce}");
    let encoded = serde_json::to_string(&value).map_err(|_| "encode native edit fixture")?;
    let setup = format!(
        r#"(() => {{
        const host = document.createElement('div'); host.id = 'native-edit-fixture';
        host.style.cssText = 'position:fixed;left:32px;top:32px;width:600px;background:white;color:black;z-index:2147483647';
        const source = document.createElement('textarea'), target = document.createElement('textarea');
        source.id = 'native-edit-source'; target.id = 'native-edit-target'; source.value = {encoded};
        for (const field of [source, target]) {{ field.dataset.inputs = '0'; field.addEventListener('input', () => field.dataset.inputs = String(Number(field.dataset.inputs) + 1)); host.append(field); }}
        document.body.append(host); source.focus(); source.setSelectionRange(0, source.value.length);
        return document.activeElement === source ? 'edit-ok' : 'edit-pending';
    }})()"#
    );
    if !evaluate(app, setup)? {
        return Err("native edit fixture did not focus".into());
    }
    focus(app)?;
    check(app, "source selected", "document.activeElement?.id === 'native-edit-source' && document.activeElement.selectionStart === 0 && document.activeElement.selectionEnd === document.activeElement.value.length")?;
    // A no-op Copy must fail even though the IPC probe left the same canary.
    // Both values are unique to this run and safe for interrupted restoration.
    app.clipboard()
        .write_text(format!("{value}-denied"))
        .map_err(|_| "seed native Copy probe")?;
    native_menu_smoke::edit(app, "Copy")?;
    copied(app, "Copy", &value)?;
    if !evaluate(app, "(() => { const target = document.getElementById('native-edit-target'); target.focus(); return document.activeElement === target ? 'edit-ok' : 'edit-pending'; })()".into())? {
        return Err("native paste fixture did not focus".into());
    }
    native_menu_smoke::edit(app, "Paste")?;
    check(app, "paste", &format!("document.getElementById('native-edit-target').value === {encoded} && Number(document.getElementById('native-edit-target').dataset.inputs) === 1"))?;
    native_menu_smoke::edit(app, "Undo")?;
    check(
        app,
        "undo paste",
        "document.getElementById('native-edit-target').value === ''",
    )?;
    native_menu_smoke::edit(app, "Redo")?;
    check(
        app,
        "redo paste",
        &format!("document.getElementById('native-edit-target').value === {encoded}"),
    )?;
    native_menu_smoke::edit(app, "Select All")?;
    check(app, "select all", "document.activeElement?.id === 'native-edit-target' && document.activeElement.selectionStart === 0 && document.activeElement.selectionEnd === document.activeElement.value.length")?;
    app.clipboard()
        .write_text(format!("{value}-denied"))
        .map_err(|_| "seed native Cut probe")?;
    native_menu_smoke::edit(app, "Cut")?;
    check(
        app,
        "cut",
        "document.getElementById('native-edit-target').value === ''",
    )?;
    copied(app, "Cut", &value)?;
    native_menu_smoke::edit(app, "Undo")?;
    check(
        app,
        "undo cut",
        &format!("document.getElementById('native-edit-target').value === {encoded}"),
    )?;
    native_menu_smoke::edit(app, "Redo")?;
    check(
        app,
        "redo cut",
        "document.getElementById('native-edit-target').value === ''",
    )?;
    if !evaluate(
        app,
        "(() => { document.getElementById('native-edit-fixture').remove(); return 'edit-ok'; })()"
            .into(),
    )? {
        return Err("native edit fixture cleanup failed".into());
    }
    Ok(())
}
