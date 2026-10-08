//! Opt-in WKWebView IPC checks, independent of the clipboard probe.
//! Positive pixels require an owned external fixture or its established handle.
//! Neither this probe nor its receipt establishes SSH/Serve provenance by itself.
use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use serde::Deserialize;
use std::{
    io::Read,
    path::Path,
    time::{Duration, Instant},
};
use tauri::AppHandle;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Fixture {
    controller_id: Option<String>,
    attach: Option<OwnedAttach>,
    session_path: String,
    source: String,
    width: u32,
    height: u32,
}
#[derive(Deserialize, serde::Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct OwnedAttach {
    name: String,
    workspace: String,
    fingerprint: String,
}
impl Fixture {
    fn valid(&self) -> bool {
        let owner = match (&self.controller_id, &self.attach) {
            (Some(id), None) => URL_SAFE_NO_PAD
                .decode(id)
                .is_ok_and(|v| v.len() == 16 && URL_SAFE_NO_PAD.encode(v) == *id),
            (None, Some(attach)) => {
                attach.name == "owned-image"
                    && attach.workspace.starts_with("/private/tmp/")
                    && attach.workspace.len() <= 4096
                    && !attach.workspace.chars().any(char::is_control)
                    && attach.fingerprint.starts_with("SHA256:")
                    && attach.fingerprint.len() <= 128
                    && !attach.fingerprint.chars().any(char::is_control)
            }
            _ => false,
        };
        owner
            && self.session_path.starts_with('/')
            && self.session_path.len() <= 32768
            && !self.session_path.chars().any(char::is_control)
            && !self.source.trim().is_empty()
            && self.source.len() <= 2048
            && !self.source.chars().any(char::is_control)
            && (1..=1200).contains(&self.width)
            && (1..=1200).contains(&self.height)
    }
}
fn check(app: &AppHandle, expression: &str, stage: &str, timeout: u64) -> Result<(), String> {
    let deadline = Instant::now() + Duration::from_secs(timeout);
    loop {
        if crate::native_edit_smoke::evaluate(
            app,
            format!("(() => ({expression}) ? 'edit-ok' : 'edit-pending')()"),
        )? {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err(format!("remote image native probe timed out: {stage}"));
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
        Err("remote image native action failed".into())
    }
}
fn fixture(directory: &Path) -> Result<Fixture, String> {
    let deadline = Instant::now() + Duration::from_secs(20);
    let path = directory.join("reasonix-native-remote-image-control.json");
    loop {
        if path.exists() {
            let file = std::fs::File::open(&path).map_err(|_| "open remote image fixture")?;
            let metadata = file
                .metadata()
                .map_err(|_| "inspect remote image fixture")?;
            if !metadata.is_file() || metadata.len() > 65536 || path.is_symlink() {
                return Err("invalid remote image fixture".into());
            }
            let mut bytes = Vec::new();
            file.take(65537)
                .read_to_end(&mut bytes)
                .map_err(|_| "read remote image fixture")?;
            let fixture: Fixture =
                serde_json::from_slice(&bytes).map_err(|_| "invalid remote image fixture")?;
            if bytes.len() > 65536 || !fixture.valid() {
                return Err("invalid remote image fixture".into());
            }
            return Ok(fixture);
        }
        if Instant::now() >= deadline {
            return Err("remote image fixture not supplied".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}
pub fn run(app: &AppHandle, directory: &Path, positive: bool) -> Result<(), String> {
    check(app, "location.protocol === 'reasonix-preview:' && document.querySelector('.tauri-shell') && document.querySelector('.tauri-sidebar__new')?.disabled === false && typeof window.__TAURI_INTERNALS__?.invoke === 'function'", "trusted initialized page", 15)?;
    let work = (|| {
        act(
            app,
            r#"window.__reasonixRemoteImageProbe = {done:false,ok:false}; (async () => {
          const invoke = request => window.__TAURI_INTERNALS__.invoke('bridge_remote_controller_session_image',{request});
          const base={controllerId:'AAAAAAAAAAAAAAAAAAAAAA',sessionPath:'/owned/session.jsonl',source:'owned.png'};
          const invalid='invalid remote controller request; select a saved host and workspace';
          const unavailable='remote controller is unavailable; reconnect the saved SSH host and reopen the workspace';
          async function rejected(request, match){try{await invoke(request);return false;}catch(error){return match(String(error));}}
          const empty=await rejected({...base,source:''}, error=>error===invalid);
          const extra=await rejected({...base,workspace:'/not-a-grant'},error=>error.includes('unknown field'));
          const missing=await rejected(base,error=>error===unavailable);
          window.__reasonixRemoteImageProbe={done:true,ok:empty&&extra&&missing};
        })().catch(()=>{window.__reasonixRemoteImageProbe={done:true,ok:false};})"#,
        )?;
        check(
            app,
            "window.__reasonixRemoteImageProbe?.done === true",
            "IPC boundary completion",
            15,
        )?;
        check(
            app,
            "window.__reasonixRemoteImageProbe?.ok === true",
            "IPC boundary assertions",
            1,
        )?;
        if positive {
            let input = fixture(directory)?;
            let request = serde_json::json!({"controllerId":input.controller_id,"sessionPath":input.session_path,"source":input.source});
            let attach =
                serde_json::to_string(&input.attach).map_err(|_| "encode owned attach fixture")?;
            let script = format!(
                r#"window.__reasonixRemoteImageProbe={{done:false,ok:false}}; (async()=>{{
              const request={request};const attach={attach};const invoke=(command,request)=>window.__TAURI_INTERNALS__.invoke(command,{{request}});let handle;let pixelsOK=false;
              try{{
              if(attach){{const connection=await invoke('connect_remote_host',{{name:attach.name,trustFingerprint:attach.fingerprint}});if(connection.status!=='connected')throw new Error('connect');
                const owned=await invoke('bridge_remote_controller_attach',{{name:attach.name,workspace:attach.workspace}});handle=owned.controller.id;request.controllerId=handle;}}
              const result=await invoke('bridge_remote_controller_session_image',request);
              const image=result.view.image;
              if(result.protocolVersion!==1||result.view.protocolVersion!==1||result.controller.id!==request.controllerId||result.controller.readOnly!==true||result.view.sessionPath!==request.sessionPath||result.view.workspace!==result.controller.workspace||image.mime!=='image/png'||image.errorCode||image.openHref||!image.url.startsWith('data:image/png;base64,'))throw new Error('invalid');
              const node=document.createElement('img');node.id='reasonix-native-remote-image-probe';node.src=image.url;node.style.maxWidth='240px';document.body.append(node);await node.decode();
              pixelsOK=node.naturalWidth==={width}&&node.naturalHeight==={height}&&node.getBoundingClientRect().width>0;
              }}finally{{if(handle)await invoke('bridge_remote_controller_close',{{controllerId:handle}});if(attach)await invoke('disconnect_remote_host',{{name:attach.name}});}}
              window.__reasonixRemoteImageProbe={{done:true,ok:pixelsOK}};
            }})().catch(()=>{{window.__reasonixRemoteImageProbe={{done:true,ok:false}};}})"#,
                width = input.width,
                height = input.height
            );
            act(app, &script)?;
            check(
                app,
                "window.__reasonixRemoteImageProbe?.done === true",
                "pixels completion",
                45,
            )?;
            check(
                app,
                "window.__reasonixRemoteImageProbe?.ok === true",
                "actual PNG decoding",
                1,
            )?;
        }
        std::fs::write(directory.join("reasonix-native-remote-image-result.json"), serde_json::json!({
            "ok":true,"registeredWebViewIPC":true,"unknownFieldRejected":true,"invalidSourceRejected":true,
            "unknownHandleRejected":true,"positivePixels":positive,"clipboardTouched":false,
            "sshServeProvenance":"external fixture must verify separately","sharedTranscriptUI":false
        }).to_string()).map_err(|_| "write remote image native receipt")?;
        Ok(())
    })();
    let cleanup = act(app, "document.getElementById('reasonix-native-remote-image-probe')?.remove(); delete window.__reasonixRemoteImageProbe");
    work.and(cleanup)
}

// Snapshot only the owned main WKWebView, never the user's screen/other apps.
fn screenshot(app: &AppHandle, path: std::path::PathBuf) -> Result<(), String> {
    use block2::RcBlock;
    use objc2_app_kit::{NSBitmapImageFileType, NSBitmapImageRep, NSImage};
    use objc2_foundation::{NSDictionary, NSError};
    use objc2_web_kit::WKWebView;
    let (send, receive) = std::sync::mpsc::sync_channel(1);
    crate::native_window_smoke::on_main(app, move |_, window| {
        window
            .with_webview(move |platform| {
                // SAFETY: Tauri schedules this live owned WKWebView on main; the
                // completion converts AppKit pixels on main, sending only bytes.
                let webview = unsafe { &*platform.inner().cast::<WKWebView>() };
                let completed = RcBlock::new(move |image: *mut NSImage, error: *mut NSError| {
                    let bytes = (|| {
                        if !error.is_null() {
                            return None;
                        }
                        let image = unsafe { image.as_ref() }?;
                        let tiff = image.TIFFRepresentation()?;
                        let bitmap = NSBitmapImageRep::imageRepWithData(&tiff)?;
                        // SAFETY: PNG uses an empty, correctly typed dictionary.
                        let png = unsafe {
                            bitmap.representationUsingType_properties(
                                NSBitmapImageFileType::PNG,
                                &NSDictionary::new(),
                            )
                        }?;
                        let bytes = png.to_vec();
                        (bytes.len() <= 16 << 20 && bytes.starts_with(b"\x89PNG\r\n\x1a\n"))
                            .then_some(bytes)
                    })();
                    let _ = send.try_send(bytes);
                });
                unsafe {
                    webview.takeSnapshotWithConfiguration_completionHandler(None, &completed);
                }
            })
            .map_err(|_| "schedule owned WebView snapshot".to_string())
    })?;
    let bytes = receive
        .recv_timeout(Duration::from_secs(10))
        .map_err(|_| "owned WebView snapshot timed out")?
        .ok_or("owned WebView snapshot unavailable")?;
    std::fs::write(path, bytes).map_err(|_| "save owned WebView snapshot".to_string())
}

// Exercise mounted production settings/lease/Transcript/Markdown/ImageViewer,
// not a standalone image node or a renderer import mocked by a test harness.
pub fn history(app: &AppHandle, directory: &Path) -> Result<(), String> {
    run(app, directory, false)?;
    // A failed later UI stage must not leave the earlier boundary receipt.
    std::fs::remove_file(directory.join("reasonix-native-remote-image-result.json"))
        .map_err(|_| "retire boundary receipt")?;
    let input = fixture(directory)?;
    let attach = input
        .attach
        .ok_or("shared history requires owned SSH fixture")?;
    // Rendered QA needs an actual active/key WKWebView. Background/off-display
    // package startup checks are not sufficient for observer/paint behavior.
    // This helper only activates this owned window; it never uses clipboard.
    crate::native_edit_smoke::focus(app)?;
    let attach_json = serde_json::to_string(&attach).map_err(|_| "encode owned UI fixture")?;
    let path = serde_json::to_string(&input.session_path).map_err(|_| "encode owned UI path")?;
    let work = (|| {
        act(app, "window.__reasonixRemoteRenderErrorCount=0; window.__reasonixRemoteRenderError=()=>{window.__reasonixRemoteRenderErrorCount++}; window.addEventListener('error',window.__reasonixRemoteRenderError); window.addEventListener('unhandledrejection',window.__reasonixRemoteRenderError)")?;
        act(
            app,
            &format!(
                r#"window.__reasonixRemoteImageProbe={{done:false,connectOK:false,settingsOK:false,rowFound:false,statusOK:false,workspaceOK:false}};
          (async()=>{{const result=await window.__TAURI_INTERNALS__.invoke('connect_remote_host',{{request:{{name:'owned-image',trustFingerprint:{attach_json}.fingerprint}}}});
          window.__reasonixRemoteImageProbe.connectOK=result.status==='connected';
          const settings=await window.__TAURI_INTERNALS__.invoke('remote_settings');
          window.__reasonixRemoteImageProbe.settingsOK=true;
          const host=settings.hosts.find(host=>host.name==='owned-image');
          window.__reasonixRemoteImageProbe.rowFound=!!host;
          window.__reasonixRemoteImageProbe.statusOK=host?.connection?.status==='connected';
          window.__reasonixRemoteImageProbe.workspaceOK=host?.workspace==={attach_json}.workspace;
          }})().catch(()=>{{}}).finally(()=>{{window.__reasonixRemoteImageProbe.done=true;}})"#
            ),
        )?;
        check(
            app,
            "window.__reasonixRemoteImageProbe?.done",
            "owned UI settings response",
            20,
        )?;
        for (field, stage) in [
            ("connectOK", "owned SSH connect receipt"),
            ("settingsOK", "native remote settings IPC"),
            ("rowFound", "native owned host row"),
            ("statusOK", "native settings connection state"),
            ("workspaceOK", "native settings workspace"),
        ] {
            check(
                app,
                &format!("window.__reasonixRemoteImageProbe?.{field} === true"),
                stage,
                1,
            )?;
        }
        act(
            app,
            "document.querySelector('.tauri-sidebar__footer .tauri-sidebar__diagnostics').click()",
        )?;
        check(
            app,
            "document.querySelector('.tauri-settings-nav')",
            "settings mount",
            10,
        )?;
        act(app, "Array.from(document.querySelectorAll('.tauri-settings-nav-item')).find(node=>['远程 SSH','Remote SSH'].includes(node.textContent.trim())).click()")?;
        check(
            app,
            "document.querySelector('.tauri-settings-content[data-tab=remote]')",
            "remote settings navigation",
            10,
        )?;
        check(app, "Array.from(document.querySelectorAll('.tauri-remote-host-card strong')).some(node=>node.textContent === 'owned-image')", "owned host settings row", 15)?;
        check(
            app,
            "document.querySelector('.tauri-remote-serve')",
            "connected owned host UI",
            15,
        )?;
        act(
            app,
            "document.querySelector('.tauri-remote-serve summary').click()",
        )?;
        check(app, "document.querySelector('.tauri-remote-serve button[aria-expanded]')?.disabled === false", "Serve UI ready", 15)?;
        act(
            app,
            "document.querySelector('.tauri-remote-serve button[aria-expanded]').click()",
        )?;
        screenshot(app, directory.join("reasonix-native-remote-catalogue.png"))?;
        check(
            app,
            "document.querySelector('.tauri-remote-serve button[aria-expanded=true]')",
            "shared sessions toggle",
            5,
        )?;
        check(
            app,
            "document.querySelector('.tauri-remote-sessions')",
            "shared sessions mount",
            10,
        )?;
        check(
            app,
            "document.querySelector('.tauri-remote-sessions[aria-busy=false]')",
            "shared sessions request completion",
            30,
        )?;
        screenshot(app, directory.join("reasonix-native-remote-catalogue.png"))?;
        check(
            app,
            "!document.querySelector('.tauri-remote-sessions [role=alert]')",
            "shared sessions request success",
            1,
        )?;
        let row = format!("Array.from(document.querySelectorAll('.tauri-remote-sessions-list li')).find(row=>row.querySelector('code')?.title==={path})");
        check(
            app,
            &format!("({row})?.querySelector('button')?.disabled === false"),
            "owned saved session catalogue",
            30,
        )?;
        act(app, &format!("({row}).querySelector('button').click()"))?;
        check(app, "document.querySelector('.tauri-remote-history[aria-busy=false] .tauri-remote-history-viewport')", "shared history mount", 15)?;
        // Reveal the settings section; this does not write the nested Transcript
        // scroller or bypass its generation-aware viewport writer.
        act(
            app,
            "document.querySelector('.tauri-remote-history').scrollIntoView({block:'start'})",
        )?;
        let pixels = format!("(() => {{const image=document.querySelector('.tauri-remote-history-viewport img');return image?.src.startsWith('data:image/png;base64,')&&image.naturalWidth==={}&&image.naturalHeight==={}&&image.getBoundingClientRect().width>0;}})()", input.width, input.height);
        check(app, &pixels, "shared Markdown PNG pixels", 20)?;
        screenshot(app, directory.join("reasonix-native-remote-history.png"))?;
        act(app, "window.__reasonixRemoteImageURL=document.querySelector('.tauri-remote-history-viewport img').src; window.__reasonixRemoteImageOverflow=document.body.style.overflow; document.querySelector('.tauri-remote-history-viewport img').click()")?;
        check(app, "document.querySelector('.image-viewer-backdrop[role=dialog] img')?.src === window.__reasonixRemoteImageURL", "shared ImageViewer preview", 10)?;
        let preview_pixels = format!("(()=>{{const node=document.querySelector('.image-viewer-backdrop[role=dialog]');const image=node?.querySelector('img');return node&&getComputedStyle(node).opacity==='1'&&image?.naturalWidth==={}&&image.naturalHeight==={};}})()", input.width, input.height);
        check(app, &preview_pixels, "visible shared preview pixels", 10)?;
        screenshot(app, directory.join("reasonix-native-remote-preview.png"))?;
        act(
            app,
            "document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}))",
        )?;
        check(app, "!document.querySelector('.image-viewer-backdrop') && document.body.style.overflow === window.__reasonixRemoteImageOverflow", "preview Escape and scroll lock cleanup", 5)?;
        check(
            app,
            "!!document.querySelector('.tauri-remote-history-viewport img')",
            "Escape preserves underlying history",
            1,
        )?;
        act(
            app,
            "document.querySelector('.tauri-remote-history-viewport img').click()",
        )?;
        check(app, "document.querySelector('.image-viewer-backdrop[role=dialog] img')?.src === window.__reasonixRemoteImageURL", "reopened shared preview", 5)?;
        act(
            app,
            "document.querySelector('.tauri-remote-history-header button').click()",
        )?;
        check(app, "!document.querySelector('.tauri-remote-history') && !document.querySelector('.image-viewer-backdrop') && document.body.style.overflow === window.__reasonixRemoteImageOverflow", "history closes owned preview", 5)?;
        act(app, "document.querySelector('.tauri-remote-serve button[aria-expanded]').click(); document.querySelector('.tauri-settings-back').click()")?;
        check(app, "!document.querySelector('.tauri-settings-overlay') && !document.querySelector('.tauri-remote-sessions')", "settings lease unmount", 5)?;
        check(app, "window.__reasonixRemoteRenderErrorCount === 0", "no uncaught shared UI errors", 1)?;
        Ok(())
    })();
    if work.is_err() {
        let _ = screenshot(app, directory.join("reasonix-native-remote-failure.png"));
        let mut states = serde_json::Map::new();
        for (name, expression) in [
            ("documentVisible", "document.visibilityState==='visible'"),
            ("documentFocused", "document.hasFocus()"),
            ("questionInDOM", "document.querySelector('.tauri-remote-history-viewport')?.textContent.includes('owned screenshot')"),
            ("answerInDOM", "document.querySelector('.tauri-remote-history-viewport')?.textContent.includes('Owned image response')"),
            ("hasMarkdown", "!!document.querySelector('.tauri-remote-history-viewport .md')"),
            ("hasMarkdownFallback", "!!document.querySelector('.tauri-remote-history-viewport [data-transcript-selection-source-fallback]')"),
            ("hasImage", "!!document.querySelector('.tauri-remote-history-viewport img')"),
            ("hasImagePlaceholder", "!!document.querySelector('.tauri-remote-history-viewport .md-image-placeholder')"),
            ("hasImageRefusal", "!!document.querySelector('.tauri-remote-history-viewport .md-image-fallback')"),
            ("messageVisible", "(()=>{const node=document.querySelector('.tauri-remote-history-viewport .msg');if(!node)return false;const css=getComputedStyle(node);return css.opacity==='1'&&css.visibility==='visible'&&node.getBoundingClientRect().height>0})()"),
            ("bodyVisible", "(()=>{const node=document.querySelector('.tauri-remote-history-viewport .msg__body');if(!node)return false;const css=getComputedStyle(node);return css.opacity==='1'&&css.visibility==='visible'&&node.getBoundingClientRect().height>0})()"),
            ("uncaughtError", "window.__reasonixRemoteRenderErrorCount>0"),
        ] {
            let state = crate::native_edit_smoke::evaluate(app, format!("({expression}) ? 'edit-ok' : 'edit-pending'"));
            states.insert(name.into(), state.map(serde_json::Value::Bool).unwrap_or(serde_json::Value::Null));
        }
        let _ = std::fs::write(
            directory.join("reasonix-native-remote-render-states.json"),
            serde_json::Value::Object(states).to_string(),
        );
    }
    let cleanup = (|| {
        act(app, "window.__reasonixRemoteImageCleanup=false; (async()=>{await window.__TAURI_INTERNALS__.invoke('disconnect_remote_host',{request:{name:'owned-image'}});window.__reasonixRemoteImageCleanup=true;})().catch(()=>{})")?;
        check(
            app,
            "window.__reasonixRemoteImageCleanup === true",
            "owned UI SSH disconnect",
            10,
        )?;
        act(app, "window.removeEventListener('error',window.__reasonixRemoteRenderError); window.removeEventListener('unhandledrejection',window.__reasonixRemoteRenderError); delete window.__reasonixRemoteRenderError; delete window.__reasonixRemoteRenderErrorCount; delete window.__reasonixRemoteImageProbe; delete window.__reasonixRemoteImageURL; delete window.__reasonixRemoteImageOverflow; delete window.__reasonixRemoteImageCleanup")
    })();
    work.and(cleanup)?;
    std::fs::write(directory.join("reasonix-native-remote-image-result.json"), serde_json::json!({
        "ok":true,"registeredWebViewIPC":true,"unknownFieldRejected":true,"invalidSourceRejected":true,
        "unknownHandleRejected":true,"positivePixels":true,"clipboardTouched":false,"sharedTranscriptUI":true,
        "previewOpened":true,"escapeClosed":true,"historyClosedPreview":true,"settingsLeaseUnmounted":true,
        "sshServeProvenance":"external fixture must verify separately"
    }).to_string()).map_err(|_| "write shared history receipt".to_string())
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn native_remote_image_fixture_is_narrow_and_bounded() {
        let valid = serde_json::json!({"controllerId":"AAAAAAAAAAAAAAAAAAAAAA","sessionPath":"/owned/session.jsonl","source":"owned.png","width":160,"height":100});
        assert!(serde_json::from_value::<Fixture>(valid.clone())
            .unwrap()
            .valid());
        for field in ["token", "url", "workspace", "localSessionId"] {
            let mut extra = valid.clone();
            extra[field] = serde_json::json!("private");
            assert!(serde_json::from_value::<Fixture>(extra).is_err());
        }
        for field in ["controllerId", "sessionPath", "source"] {
            let mut bad = valid.clone();
            bad[field] = serde_json::json!("");
            assert!(!serde_json::from_value::<Fixture>(bad).unwrap().valid());
        }
        let mut bad = valid;
        bad["width"] = serde_json::json!(1201);
        assert!(!serde_json::from_value::<Fixture>(bad).unwrap().valid());

        let owned = serde_json::json!({"attach":{"name":"owned-image","workspace":"/private/tmp/owned/workspace","fingerprint":"SHA256:owned"},"sessionPath":"/private/tmp/owned/session.jsonl","source":"owned.png","width":16,"height":10});
        assert!(serde_json::from_value::<Fixture>(owned.clone())
            .unwrap()
            .valid());
        let mut both = owned.clone();
        both["controllerId"] = serde_json::json!("AAAAAAAAAAAAAAAAAAAAAA");
        assert!(!serde_json::from_value::<Fixture>(both).unwrap().valid());
        let mut neither = owned.clone();
        neither["attach"] = serde_json::Value::Null;
        assert!(!serde_json::from_value::<Fixture>(neither).unwrap().valid());
        for (field, value) in [
            ("name", "real-host"),
            ("workspace", "/Users/real"),
            ("fingerprint", "SHA256:bad\n"),
        ] {
            let mut bad = owned.clone();
            bad["attach"][field] = serde_json::json!(value);
            assert!(!serde_json::from_value::<Fixture>(bad).unwrap().valid());
        }
        let mut extra = owned;
        extra["attach"]["token"] = serde_json::json!("private");
        assert!(serde_json::from_value::<Fixture>(extra).is_err());
    }
}
