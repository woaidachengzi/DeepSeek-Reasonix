//! Opt-in actual rendered message menu -> native clipboard regression.
//! The caller owns a private profile, fixed loopback provider and clipboard backup.
use std::{
    io::Read,
    path::Path,
    time::{Duration, Instant},
};
use tauri::{AppHandle, Manager};
use tauri_plugin_clipboard_manager::ClipboardExt;

fn check(app: &AppHandle, script: String, stage: &str) -> Result<(), String> {
    let deadline = Instant::now() + Duration::from_secs(15);
    loop {
        if crate::native_edit_smoke::evaluate(app, script.clone())
            .map_err(|error| format!("{stage}: {error}"))? {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err(format!("rendered message copy did not settle: {stage}"));
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}

// Opt-in fixed-schema observations only: no transcript/editor content is returned.
fn observe_selection(app: &AppHandle, directory: &Path, name: &str) -> Result<(), String> {
    use block2::RcBlock;
    use objc2::runtime::AnyObject;
    use objc2_foundation::{NSError, NSString};
    use objc2_web_kit::WKWebView;
    let (send, receive) = std::sync::mpsc::sync_channel(1);
    crate::native_window_smoke::on_main(app, move |_, window| {
        window.with_webview(move |platform| {
            // SAFETY: Tauri's live main WKWebView is evaluated on its UI thread.
            let webview = unsafe { &*platform.inner().cast::<WKWebView>() };
            let completed = RcBlock::new(move |value: *mut AnyObject, error: *mut NSError| {
                let result = if !error.is_null() {
                    Err("selection observation evaluation failed".to_string())
                } else {
                    let text = unsafe { value.as_ref() }
                        .and_then(|value| value.downcast_ref::<NSString>())
                        .map(|value| value.to_string());
                    text.filter(|value| value.len() <= 4096)
                        .ok_or_else(|| "selection observation invalid".to_string())
                };
                let _ = send.try_send(result);
            });
            let script = r#"JSON.stringify((() => {
              const selection = document.getSelection();
              const transcript = document.querySelector('.tauri-transcript');
              const action = document.querySelector('.transcript-selection-action');
              return {oldShortcutDefaultPrevented: window.__reasonixOldShortcutProbe?.defaultPrevented ?? null,
                oldShortcutSelectionCollapsed: window.__reasonixOldShortcutProbe?.selectionCollapsed ?? null,
                frameCallback: window.__reasonixSelectionFrame === true,
                documentVisible: document.visibilityState === 'visible',
                documentFocused: document.hasFocus(),
                selectionCollapsed: !selection || selection.isCollapsed,
                selectionLength: selection?.toString().length || 0,
                anchorInTranscript: !!transcript?.contains(selection?.anchorNode),
                focusInTranscript: !!transcript?.contains(selection?.focusNode),
                editorFocused: document.activeElement === document.querySelector('.tauri-composer textarea'),
                settingsPresent: !!document.querySelector('.tauri-settings-overlay'),
                modalPresent: !!document.querySelector('[aria-modal=true]'),
                actionPresent: !!action,
                actionOpen: action?.dataset.state === 'open',
                actionDisabled: !!action?.querySelector('button:disabled')};
            })())"#;
            unsafe { webview.evaluateJavaScript_completionHandler(&NSString::from_str(script), Some(&completed)); }
        }).map_err(|_| "schedule selection observation".into())
    })?;
    let text = receive.recv_timeout(Duration::from_secs(5))
        .map_err(|_| "selection observation timed out")??;
    let value: serde_json::Value = serde_json::from_str(&text)
        .map_err(|_| "selection observation malformed")?;
    std::fs::write(directory.join(name), value.to_string())
        .map_err(|_| "record selection observation".into())
}

fn fixture_nonce(directory: &Path) -> Result<String, String> {
    let file = std::fs::File::open(directory.join("reasonix-native-clipboard-control.json"))
        .map_err(|_| "message clipboard fixture missing")?;
    let mut bytes = Vec::new();
    file.take(8193)
        .read_to_end(&mut bytes)
        .map_err(|_| "read message clipboard fixture")?;
    if bytes.len() > 8192 {
        return Err("message clipboard fixture exceeds bounds".into());
    }
    let value: serde_json::Value =
        serde_json::from_slice(&bytes).map_err(|_| "invalid clipboard fixture")?;
    let nonce = value
        .get("nonce")
        .and_then(|v| v.as_str())
        .ok_or("clipboard nonce missing")?;
    if nonce.len() != 32 || !nonce.bytes().all(|v| v.is_ascii_hexdigit()) {
        return Err("clipboard nonce invalid".into());
    }
    Ok(nonce.to_string())
}

pub fn run(app: &AppHandle, directory: &Path, send_selection: bool) -> Result<(), String> {
    let nonce = fixture_nonce(directory)?;
    let text = format!("reasonix-native-clipboard-{nonce}");
    let literal = serde_json::to_string(&text).map_err(|_| "encode clipboard fixture")?;
    check(app, "document.querySelector('.tauri-composer textarea') && document.querySelector('[data-surface=transcript]') ? 'edit-ok' : 'edit-pending'".into(), "application and lazy menu")?;
    app.state::<crate::host_preferences::HostPreferences>()
        .set_close_behavior(crate::host_preferences::CloseBehavior::Quit)?;
    let prepare = format!(
        r#"(() => {{
      const editor = document.querySelector('.tauri-composer textarea');
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(editor, {literal});
      editor.dispatchEvent(new Event('input', {{bubbles:true}}));
      return 'edit-ok';
    }})()"#
    );
    if !crate::native_edit_smoke::evaluate(app, prepare)? {
        return Err("prepare owned message".into());
    }
    check(app, "document.querySelector('.tauri-send-button')?.disabled === false ? 'edit-ok' : 'edit-pending'".into(), "enabled send")?;
    if !crate::native_edit_smoke::evaluate(
        app,
        "document.querySelector('.tauri-send-button').click(); 'edit-ok'".into(),
    )? {
        return Err("submit owned message".into());
    }
    check(app, format!("[...document.querySelectorAll('.tauri-message.is-assistant .md')].some(node => node.textContent.trim() === {literal}) ? 'edit-ok' : 'edit-pending'"), "actual provider reply")?;
    let select = format!(
        r#"(() => {{
      const node = [...document.querySelectorAll('.tauri-message.is-assistant .md')].find(node => node.textContent.trim() === {literal});
      const details = node.closest('details'); if (details) details.open = true;
      const range = document.createRange(); range.selectNodeContents(node);
      const selected = document.getSelection(); selected.removeAllRanges(); selected.addRange(range);
      if (selected.toString() !== {literal}) return 'edit-pending';
      const bounds = node.getBoundingClientRect();
      if (!bounds.width || !bounds.height) return 'edit-pending';
      const event = new MouseEvent('contextmenu', {{bubbles:true, cancelable:true, clientX:bounds.left+2, clientY:bounds.top+2}});
      node.dispatchEvent(event); return event.defaultPrevented ? 'edit-ok' : 'edit-pending';
    }})()"#
    );
    // Do not retry the menu action: a failed claim is a failure, not a focus workaround.
    if !crate::native_edit_smoke::evaluate(app, select)? {
        return Err("actual message context menu was not claimed".into());
    }
    check(
        app,
        "document.querySelector('.context-menu [role=menuitem]') ? 'edit-ok' : 'edit-pending'"
            .into(),
        "Copy menu",
    )?;
    if !crate::native_edit_smoke::evaluate(
        app,
        "document.querySelector('.context-menu [role=menuitem]').click(); 'edit-ok'".into(),
    )? {
        return Err("activate rendered Copy".into());
    }
    let deadline = Instant::now() + Duration::from_secs(5);
    // The click dispatches async IPC. An initially empty/nontext pasteboard
    // cannot be read as text yet; wait for the one submitted write, never
    // resubmit the menu action or accept another value.
    while app.clipboard().read_text().ok().as_deref() != Some(text.as_str()) {
        if Instant::now() >= deadline {
            return Err("rendered menu did not write exact native text".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
    let count = crate::native_window_smoke::on_main(app, |_, _| {
        Ok(objc2_app_kit::NSPasteboard::generalPasteboard().changeCount())
    })?;
    std::fs::write(
        directory.join("reasonix-native-clipboard-owned.json"),
        serde_json::json!({"nonce":nonce,"count":count}).to_string(),
    )
    .map_err(|_| "record owned clipboard generation")?;
    check(app, "document.getSelection()?.isCollapsed && !document.querySelector('.context-menu') ? 'edit-ok' : 'edit-pending'".into(), "successful copy release")?;
    if send_selection {
        if !crate::native_edit_smoke::evaluate(app, "document.dispatchEvent(new KeyboardEvent('keydown', {key:'k',metaKey:true,shiftKey:true,bubbles:true,cancelable:true})); 'edit-ok'".into())? {
            return Err("open actual shortcut settings".into());
        }
        check(app, "document.querySelector('[data-tauri-shortcut-action=add_selection]') ? 'edit-ok' : 'edit-pending'".into(), "selection settings row")?;
        if !crate::native_edit_smoke::evaluate(app, "document.querySelector('[data-tauri-shortcut-action=add_selection]').click(); 'edit-ok'".into())? {
            return Err("record actual selection shortcut".into());
        }
        check(app, "document.querySelector('[data-tauri-shortcut-action=add_selection].is-recording') ? 'edit-ok' : 'edit-pending'".into(), "shortcut recorder ready")?;
        if !crate::native_edit_smoke::evaluate(app, "document.querySelector('[data-tauri-shortcut-action=add_selection]').dispatchEvent(new KeyboardEvent('keydown', {key:'l',metaKey:true,shiftKey:true,bubbles:true,cancelable:true})); 'edit-ok'".into())? {
            return Err("save actual selection shortcut".into());
        }
        check(app, "(() => { const saved = JSON.parse(localStorage.getItem('reasonix.tauri.shortcuts.v1') || '{}').add_selection; return saved?.key === 'l' && saved.meta === true && saved.shift === true && !document.querySelector('[data-tauri-shortcut-action=add_selection].is-recording') ? 'edit-ok' : 'edit-pending'; })()".into(), "selection shortcut saved")?;
        if !crate::native_edit_smoke::evaluate(app, "document.dispatchEvent(new KeyboardEvent('keydown', {key:'Escape',bubbles:true,cancelable:true})); 'edit-ok'".into())? {
            return Err("close shortcut settings".into());
        }
        check(app, "!document.querySelector('.tauri-settings-overlay') ? 'edit-ok' : 'edit-pending'".into(), "shortcut settings closed")?;
    }
    crate::native_window_smoke::record(app, "selection-before-request")?;
    if !crate::native_edit_smoke::evaluate(app, "window.__reasonixSelectionFrame = false; requestAnimationFrame(() => { window.__reasonixSelectionFrame = true; }); 'edit-ok'".into())? {
        return Err("register selection frame observation".into());
    }
    let select_for_chat = format!(
        r#"(() => {{
      const editor = document.querySelector('.tauri-composer textarea');
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(editor, 'draft-after-copy');
      editor.dispatchEvent(new Event('input', {{bubbles:true}}));
      const message = [...document.querySelectorAll('.tauri-message.is-assistant .md')].find(node => node.textContent.trim() === {literal});
      if (!message) return 'edit-pending';
      const range = document.createRange(); range.selectNodeContents(message);
      const selection = document.getSelection(); selection.removeAllRanges(); selection.addRange(range);
      message.dispatchEvent(new MouseEvent('pointerup', {{button:0,bubbles:true}}));
      return 'edit-ok';
    }})()"#
    );
    if !crate::native_edit_smoke::evaluate(app, select_for_chat)? {
        return Err("select actual message for Add to Chat".into());
    }
    crate::native_window_smoke::record(app, "selection-requested")?;
    let action_result = check(app, "document.querySelector('.transcript-selection-action[data-state=open] button:not(:disabled)') ? 'edit-ok' : 'edit-pending'".into(), "Add to Chat action");
    // Capture the first terminal observation before the runner exits/cleans up.
    observe_selection(app, directory, "reasonix-native-selection-observation.json")?;
    crate::native_window_smoke::record(app, if action_result.is_ok() { "selection-action-ready" } else { "selection-action-failed" })?;
    action_result?;
    if !crate::native_edit_smoke::evaluate(
        app,
        "document.querySelector('.transcript-selection-action button').click(); 'edit-ok'".into(),
    )? {
        return Err("activate rendered Add to Chat".into());
    }
    check(
        app,
        format!(
            r#"(() => {{
      const editor = document.querySelector('.tauri-composer textarea');
      const cards = document.querySelectorAll('.tauri-composer .composer-context__name');
      return cards.length === 1 && cards[0].textContent === {literal}
        && editor.value === 'draft-after-copy' && document.activeElement === editor
        && document.getSelection().isCollapsed ? 'edit-ok' : 'edit-pending';
    }})()"#
        ),
        "quoted reference and unchanged draft",
    )?;
    let reselect = format!(
        r#"(() => {{
      const message = [...document.querySelectorAll('.tauri-message.is-assistant .md')].find(node => node.textContent.trim() === {literal});
      const range = document.createRange(); range.selectNodeContents(message);
      const selection = document.getSelection(); selection.removeAllRanges(); selection.addRange(range);
      message.dispatchEvent(new MouseEvent('pointerup', {{button:0,bubbles:true}}));
      return 'edit-ok';
    }})()"#
    );
    if !crate::native_edit_smoke::evaluate(app, reselect)? {
        return Err("reselect actual message for selection shortcut".into());
    }
    check(app, "document.querySelector('.transcript-selection-action[data-state=open] button:not(:disabled)') ? 'edit-ok' : 'edit-pending'".into(), "selection shortcut armed")?;
    if send_selection {
        let old_inactive = crate::native_edit_smoke::evaluate(app, "(() => { const event = new KeyboardEvent('keydown', {key:'l',metaKey:true,bubbles:true,cancelable:true}); document.dispatchEvent(event); const probe = {defaultPrevented:event.defaultPrevented, selectionCollapsed:!document.getSelection() || document.getSelection().isCollapsed}; window.__reasonixOldShortcutProbe = probe; return !probe.defaultPrevented && !probe.selectionCollapsed ? 'edit-ok' : 'edit-pending'; })()".into())?;
        // Freeze both subconditions from the same dispatch, including failures;
        // keep the first action observation in its distinct original file.
        observe_selection(app, directory, "reasonix-native-old-shortcut-observation.json")?;
        if !old_inactive {
            return Err("old shortcut inactivity or retained selection assertion failed after remap".into());
        }
    }
    let shortcut = format!("(() => {{ const event = new KeyboardEvent('keydown', {{key:'l',metaKey:true,shiftKey:{send_selection},bubbles:true,cancelable:true}}); document.dispatchEvent(event); return event.defaultPrevented ? 'edit-ok' : 'edit-pending'; }})()");
    if !crate::native_edit_smoke::evaluate(app, shortcut)? {
        return Err("rendered selection Cmd+L was not claimed".into());
    }
    check(app, "document.getSelection().isCollapsed && document.querySelectorAll('.tauri-composer .composer-context__name').length === 1 ? 'edit-ok' : 'edit-pending'".into(), "selection shortcut deduplicates")?;
    if app.clipboard().read_text().ok().as_deref() != Some(text.as_str()) {
        return Err("Add to Chat changed native clipboard".into());
    }
    let after_count = crate::native_window_smoke::on_main(app, |_, _| {
        Ok(objc2_app_kit::NSPasteboard::generalPasteboard().changeCount())
    })?;
    if after_count != count {
        return Err("Add to Chat changed clipboard generation".into());
    }
    std::fs::write(directory.join("reasonix-native-selection-add-result.json"),
        serde_json::json!({"ok":true,"selectionReference":true,"draftPreserved":true,"composerFocused":true,"selectionShortcut":true,"deduplicated":true,"clipboardTextPreserved":true,"clipboardGenerationPreserved":true}).to_string())
        .map_err(|_| "record Add to Chat result")?;
    if send_selection {
        check(app, "document.querySelector('.tauri-send-button:not(.is-stop):not(:disabled)') ? 'edit-ok' : 'edit-pending'".into(), "second turn ready")?;
        if !crate::native_edit_smoke::evaluate(
            app,
            "document.querySelector('.tauri-send-button').click(); 'edit-ok'".into(),
        )? {
            return Err("submit actual selection reference".into());
        }
        let answer = serde_json::to_string(&format!("reasonix-selection-accepted-{nonce}"))
            .map_err(|_| "encode quoted response")?;
        check(app, format!("[...document.querySelectorAll('.tauri-message.is-assistant .md')].some(node => node.textContent.trim() === {answer}) ? 'edit-ok' : 'edit-pending'"), "actual quoted provider response")?;
        check(app, format!("[...document.querySelectorAll('.tauri-message__selection summary')].some(node => node.textContent === {literal}) && !document.querySelector('.tauri-composer .composer-context__name') ? 'edit-ok' : 'edit-pending'"), "accepted reference displayed and consumed")?;
        std::fs::write(directory.join("reasonix-native-selection-send-result.json"),
            serde_json::json!({"ok":true,"providerResponse":true,"historyReference":true,"draftConsumed":true,"configuredShortcut":true,"oldBindingInactive":true}).to_string())
            .map_err(|_| "record selection submit")?;
    }
    Ok(())
}

pub fn reopen(app: &AppHandle, directory: &Path) -> Result<(), String> {
    crate::native_window_smoke::record(app, "selection-reopen-before-storage")?;
    check(app, "(() => { const saved = JSON.parse(localStorage.getItem('reasonix.tauri.shortcuts.v1') || '{}').add_selection; return saved?.key === 'l' && saved.meta === true && saved.shift === true ? 'edit-ok' : 'edit-pending'; })()".into(), "reopened configured selection shortcut")?;
    let nonce = fixture_nonce(directory)?;
    let text = serde_json::to_string(&format!("reasonix-native-clipboard-{nonce}"))
        .map_err(|_| "encode reopened selection")?;
    let answer = serde_json::to_string(&format!("reasonix-selection-accepted-{nonce}"))
        .map_err(|_| "encode reopened response")?;
    check(app, "document.querySelectorAll('.tauri-sidebar__session').length === 1 && !document.querySelector('.tauri-sidebar__session').disabled ? 'edit-ok' : 'edit-pending'".into(), "persisted session entry")?;
    if !crate::native_edit_smoke::evaluate(
        app,
        "document.querySelector('.tauri-sidebar__session').click(); 'edit-ok'".into(),
    )? {
        return Err("reopen actual persisted session".into());
    }
    check(
        app,
        format!(
            r#"(() => {{
      const transcript = document.querySelector('.tauri-transcript');
      return transcript && [...transcript.querySelectorAll('.tauri-message.is-assistant .md')].some(node => node.textContent.trim() === {answer})
        && [...transcript.querySelectorAll('.tauri-message__selection summary')].some(node => node.textContent === {text})
        && !transcript.textContent.includes('reasonix-selected-chat-context')
        && !transcript.textContent.includes('The JSON array below')
        && document.querySelector('.tauri-composer textarea')?.value === ''
        && !document.querySelector('.tauri-composer .composer-context__name') ? 'edit-ok' : 'edit-pending';
    }})()"#
        ),
        "reopened quoted history without protocol text or draft replay",
    )?;
    std::fs::write(directory.join("reasonix-native-selection-reopen-result.json"),
        serde_json::json!({"ok":true,"historyReference":true,"protocolTextHidden":true,"noDraftReplay":true,"configuredShortcutPersisted":true}).to_string())
        .map_err(|_| "record reopened reference")?;
    Ok(())
}
