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
        if crate::native_edit_smoke::evaluate(app, script.clone())? {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err(format!("rendered message copy did not settle: {stage}"));
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}

pub fn run(app: &AppHandle, directory: &Path) -> Result<(), String> {
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
    while app
        .clipboard()
        .read_text()
        .map_err(|_| "read copied native text")?
        != text
    {
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
    Ok(())
}
