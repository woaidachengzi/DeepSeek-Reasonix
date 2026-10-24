use std::io::Write;
use std::path::Path;
use tauri::State;
use tauri_plugin_dialog::DialogExt;

use crate::bridge::{BridgeSupervisor, SessionRequest};

fn encode_report(payload: &serde_json::Value) -> Result<Vec<u8>, String> {
    if payload
        .get("schemaVersion")
        .and_then(serde_json::Value::as_u64)
        != Some(1)
        || !payload
            .get("entries")
            .is_some_and(serde_json::Value::is_array)
    {
        return Err("AI diagnostic report is invalid; restart the local service and retry".into());
    }
    let bytes = serde_json::to_vec_pretty(payload).map_err(|error| error.to_string())?;
    if bytes.len() > 8 << 20 {
        return Err("AI diagnostic report exceeds the 8 MiB export limit".into());
    }
    Ok(bytes)
}

fn write_report(path: &Path, bytes: &[u8]) -> Result<(), String> {
    let parent = path.parent().ok_or("choose a writable export folder")?;
    let mut temporary = tempfile::NamedTempFile::new_in(parent)
        .map_err(|_| "cannot prepare AI log export; choose a writable folder")?;
    temporary
        .write_all(bytes)
        .and_then(|_| temporary.as_file().sync_all())
        .map_err(|_| "cannot write AI log export; check free space and retry")?;
    temporary
        .persist(path)
        .map_err(|_| "cannot save AI log export; choose a writable file")?;
    Ok(())
}

#[tauri::command]
pub async fn export_ai_trace(
    app: tauri::AppHandle,
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
    detailed: bool,
) -> Result<bool, String> {
    // Read through the authenticated bridge and its exact live session owner.
    // The renderer cannot supply file paths, arbitrary content or credentials.
    let session_id = request.session_id.clone();
    let mut payload = supervisor.ai_trace(request, detailed)?;
    let object = payload
        .as_object_mut()
        .ok_or("AI diagnostic report is not an object; restart the local service and retry")?;
    object.insert(
        "host".into(),
        serde_json::json!({
            "version": app.package_info().version.to_string(),
            "commit": env!("REASONIX_PREVIEW_COMMIT"),
            "dirty": env!("REASONIX_PREVIEW_DIRTY") == "1",
            "platform": std::env::consts::OS,
            "sessionId": session_id,
        }),
    );
    let bytes = encode_report(&payload)?;
    let (sender, receiver) = std::sync::mpsc::channel();
    app.dialog()
        .file()
        .add_filter("AI diagnostic JSON", &["json"])
        .set_file_name("reasonix-ai-diagnostics.json")
        .save_file(move |selection| {
            let _ = sender.send(selection);
        });
    let selected = tauri::async_runtime::spawn_blocking(move || {
        crate::receive_file_dialog_selection(receiver)
    })
    .await
    .map_err(|_| "file dialog task failed; select a file again")??;
    let Some(path) = crate::selected_theme_path(selected)? else {
        return Ok(false);
    };
    write_report(&path, &bytes)?;
    Ok(true)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn bounded_report_and_atomic_destination() {
        assert!(encode_report(&serde_json::json!({"schemaVersion":2,"entries":[]})).is_err());
        assert!(encode_report(
            &serde_json::json!({"schemaVersion":1,"entries":["x".repeat(8 << 20)]})
        )
        .is_err());
        let bytes = encode_report(&serde_json::json!({"schemaVersion":1,"entries":[]})).unwrap();
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("trace.json");
        std::fs::write(&path, "old").unwrap();
        write_report(&path, &bytes).unwrap();
        assert_eq!(std::fs::read(path).unwrap(), bytes);
        assert!(write_report(&dir.path().join("missing/trace.json"), &bytes).is_err());
    }

    #[cfg(unix)]
    #[test]
    fn chosen_symlink_does_not_overwrite_target() {
        let dir = tempfile::tempdir().unwrap();
        let target = dir.path().join("original");
        let chosen = dir.path().join("trace.json");
        std::fs::write(&target, "keep").unwrap();
        std::os::unix::fs::symlink(&target, &chosen).unwrap();
        write_report(&chosen, b"{}").unwrap();
        assert_eq!(std::fs::read_to_string(target).unwrap(), "keep");
        assert_eq!(std::fs::read_to_string(chosen).unwrap(), "{}");
    }
}
