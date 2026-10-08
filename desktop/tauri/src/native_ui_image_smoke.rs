//! Opt-in packaged WKWebView image Paste and persisted history acceptance.
//! Only fixed booleans/counters are recorded, never clipboard or transcript bytes.
use std::{
    io::Read,
    path::Path,
    time::{Duration, Instant},
};
use tauri::AppHandle;

fn check(app: &AppHandle, expression: &str, stage: &str) -> Result<(), String> {
    let script = format!("(() => (({expression}) ? 'edit-ok' : 'edit-pending'))()");
    let deadline = Instant::now() + Duration::from_secs(15);
    loop {
        if crate::native_edit_smoke::evaluate(app, script.clone())? {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err(format!("native image state did not settle: {stage}"));
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}

fn act(app: &AppHandle, script: &str) -> Result<(), String> {
    if crate::native_edit_smoke::evaluate(
        app,
        format!("(() => {{ {script}; return 'edit-ok'; }})()"),
    )? {
        Ok(())
    } else {
        Err("native image UI action failed".into())
    }
}

fn nonce(directory: &Path) -> Result<String, String> {
    let file = std::fs::File::open(directory.join("reasonix-native-clipboard-control.json"))
        .map_err(|_| "image fixture control missing")?;
    let mut data = Vec::new();
    file.take(8193)
        .read_to_end(&mut data)
        .map_err(|_| "read image fixture control")?;
    if data.len() > 8192 {
        return Err("image fixture control exceeds bounds".into());
    }
    let control: serde_json::Value =
        serde_json::from_slice(&data).map_err(|_| "invalid image fixture control")?;
    let nonce = control
        .get("nonce")
        .and_then(|v| v.as_str())
        .ok_or("image fixture nonce missing")?;
    if nonce.len() != 32 || !nonce.bytes().all(|v| v.is_ascii_hexdigit()) {
        return Err("invalid image fixture nonce".into());
    }
    Ok(nonce.into())
}

fn pending_files(directory: &Path) -> Result<usize, String> {
    let entries =
        std::fs::read_dir(directory).map_err(|_| "inspect owned image staging directory")?;
    let mut count = 0;
    for entry in entries {
        let entry = entry.map_err(|_| "read owned image staging entry")?;
        if entry.file_name().to_string_lossy().starts_with("pasted-image-") {
            count += 1;
        }
    }
    Ok(count)
}

fn image_loaded_expression(selector: &str) -> String {
    format!("(() => {{ const images = [...document.querySelectorAll({})]; return images.length > 0 && images.every(image => image.complete && image.naturalWidth === 64 && image.naturalHeight === 40 && image.src.startsWith('data:image/') && image.getBoundingClientRect().width > 0); }})()", serde_json::to_string(selector).unwrap())
}

fn ready(app: &AppHandle) -> Result<(), String> {
    check(app, "location.protocol === 'reasonix-preview:' && document.querySelector('.tauri-shell') && document.querySelector('.tauri-composer textarea') && document.querySelector('.tauri-sidebar__new')?.disabled === false", "trusted initialized application")
}

pub fn paste(app: &AppHandle, directory: &Path) -> Result<(), String> {
    let nonce = nonce(directory)?;
    ready(app)?;
    crate::native_edit_smoke::focus(app)?;
    act(app, "window.__reasonixImagePasteCount = 0; const editor = document.querySelector('.tauri-composer textarea'); editor.addEventListener('paste', () => window.__reasonixImagePasteCount++); editor.focus()")?;
    check(
        app,
        "document.activeElement === document.querySelector('.tauri-composer textarea')",
        "native focused composer",
    )?;
    crate::native_menu_smoke::edit(app, "Paste")?;
    check(
        app,
        &image_loaded_expression(".tauri-composer__attachment-icon img"),
        "draft native image preview",
    )?;
    check(app, "window.__reasonixImagePasteCount === 1 && document.querySelector('.tauri-composer textarea').value === '' && document.querySelectorAll('.tauri-sidebar__session').length === 0", "one native paste without text or early session")?;
    if pending_files(directory)? != 1 {
        return Err("draft image staging ownership differs".into());
    }
    act(
        app,
        "document.querySelector('.tauri-composer__attachment button').click()",
    )?;
    check(
        app,
        "!document.querySelector('.tauri-composer__attachment')",
        "remove draft image",
    )?;
    let deadline = Instant::now() + Duration::from_secs(5);
    while pending_files(directory)? != 0 {
        if Instant::now() >= deadline {
            return Err("removed native image temp file survived".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
    act(
        app,
        "document.querySelector('.tauri-composer textarea').focus()",
    )?;
    crate::native_menu_smoke::edit(app, "Paste")?;
    check(
        app,
        &image_loaded_expression(".tauri-composer__attachment-icon img"),
        "second draft preview",
    )?;
    let prompt = serde_json::to_string(&format!("reasonix-native-image-{nonce}"))
        .map_err(|_| "encode owned image prompt")?;
    act(app, &format!("const editor = document.querySelector('.tauri-composer textarea'); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(editor, {prompt}); editor.dispatchEvent(new Event('input', {{bubbles:true}}))"))?;
    check(
        app,
        "document.querySelector('.tauri-send-button')?.disabled === false",
        "image send enabled",
    )?;
    act(app, "document.querySelector('.tauri-send-button').click()")?;
    check(app, "[...document.querySelectorAll('.tauri-message.is-assistant .md')].some(node => node.textContent.includes('native-image-accepted')) && !document.querySelector('.tauri-send-button.is-stop')", "real core provider completed")?;
    check(
        app,
        &image_loaded_expression(".tauri-message__attachments img"),
        "persisted user image",
    )?;
    check(
        app,
        &image_loaded_expression(".tauri-message.is-assistant .md img"),
        "assistant workspace Markdown image",
    )?;
    check(app, "!document.querySelector('.tauri-composer__attachment') && document.querySelector('.tauri-composer textarea').value === ''", "accepted send clears composer")?;
    if pending_files(directory)? != 0 {
        return Err("accepted native send left staging files".into());
    }
    act(
        app,
        "document.querySelector('.tauri-composer textarea').focus()",
    )?;
    crate::native_menu_smoke::edit(app, "Paste")?;
    check(
        app,
        &image_loaded_expression(".tauri-composer__attachment-icon img"),
        "existing session pasted preview",
    )?;
    if pending_files(directory)? != 0 {
        return Err("existing session copy left staging files".into());
    }
    // Leave this copied-but-unsent image in the composer. Reopen must not replay it.
    std::fs::write(
        directory.join("reasonix-native-image-paste-result.json"),
        serde_json::json!({
            "ok":true,"nativeMenuPaste":true,"draftPreview":true,"draftRemovalCleanup":true,
            "imageSend":true,"historyImage":true,"markdownImage":true,"existingSessionPreview":true,
            "stagingCleanup":true,"noEarlySession":true
        })
        .to_string(),
    )
    .map_err(|_| "record native image receipt")?;
    Ok(())
}

pub fn reopen(app: &AppHandle, directory: &Path) -> Result<(), String> {
    ready(app)?;
    check(app, "document.querySelectorAll('.tauri-sidebar__session').length === 1 && !document.querySelector('.tauri-sidebar__session').disabled", "persisted image session entry")?;
    act(
        app,
        "document.querySelector('.tauri-sidebar__session').click()",
    )?;
    check(
        app,
        &image_loaded_expression(".tauri-message__attachments img"),
        "reopened user attachment",
    )?;
    check(
        app,
        &image_loaded_expression(".tauri-message.is-assistant .md img"),
        "reopened local Markdown image",
    )?;
    check(app, "document.querySelector('.tauri-composer textarea').value === '' && !document.querySelector('.tauri-composer__attachment') && !document.querySelector('.tauri-send-button.is-stop')", "no image draft replay or provider resubmit")?;
    if pending_files(directory)? != 0 {
        return Err("reopen found leaked image staging".into());
    }
    std::fs::write(directory.join("reasonix-native-image-reopen-result.json"), serde_json::json!({
        "ok":true,"historyImage":true,"markdownImage":true,"noDraftReplay":true,"stagingCleanup":true
    }).to_string()).map_err(|_| "record reopened image receipt")?;
    Ok(())
}
