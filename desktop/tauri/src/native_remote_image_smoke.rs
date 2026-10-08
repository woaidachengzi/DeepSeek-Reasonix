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
