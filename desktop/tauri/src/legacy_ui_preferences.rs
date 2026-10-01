//! Explicit, read-only access to allowlisted preferences at the old shared
//! origin. The inert hidden document never runs the product UI or receives a
//! capability; migration writes and its rollback journal live in the new origin.
use std::{collections::BTreeMap, sync::mpsc, time::Duration};

use block2::RcBlock;
use objc2::runtime::AnyObject;
use objc2_foundation::{NSError, NSString};
use objc2_web_kit::WKWebView;
use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Manager, WebviewUrl, WebviewWindow, WebviewWindowBuilder};

const LABEL: &str = "legacy-ui-preference-reader";
const SOURCE: &str = "tauri://localhost";
const LIMIT: usize = 262_144;
const KEYS: &str = include_str!("../../frontend/src/tauri/legacyUiPreferenceKeys.json");
static READER: std::sync::Mutex<()> = std::sync::Mutex::new(());

#[derive(Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Snapshot {
    source: String,
    values: BTreeMap<String, String>,
}

fn validate(raw: &str) -> Result<Snapshot, String> {
    if raw.len() > LIMIT {
        return Err(
            "Old UI preferences are too large; select your workspace and preferences manually"
                .into(),
        );
    }
    let snapshot: Snapshot = serde_json::from_str(raw)
        .map_err(|_| "Old UI preferences could not be read; retry the preview")?;
    let keys: Vec<String> = serde_json::from_str(KEYS).expect("compiled UI preference keys");
    if snapshot.source != SOURCE
        || snapshot
            .values
            .iter()
            .any(|(key, value)| !keys.contains(key) || value.len() > 65_536 || value.contains('\0'))
    {
        return Err("Old UI preferences are invalid; select your preferences manually".into());
    }
    Ok(snapshot)
}

fn main<T: Send + 'static>(
    app: &AppHandle,
    action: impl FnOnce(&AppHandle) -> Result<T, String> + Send + 'static,
) -> Result<T, String> {
    let (send, receive) = mpsc::sync_channel(1);
    let handle = app.clone();
    app.run_on_main_thread(move || {
        let _ = send.try_send(action(&handle));
    })
    .map_err(|_| "Could not schedule UI preference preview; retry")?;
    receive
        .recv_timeout(Duration::from_secs(5))
        .map_err(|_| "UI preference preview timed out; retry")?
}

struct HiddenReader(WebviewWindow);
impl Drop for HiddenReader {
    fn drop(&mut self) {
        // Destroy bypasses the user's main-window CloseRequested policy.
        let _ = self.0.destroy();
    }
}

pub fn read(app: &AppHandle) -> Result<Snapshot, String> {
    let _guard = READER
        .try_lock()
        .map_err(|_| "Another UI preference preview is active; wait and retry")?;
    if cfg!(debug_assertions) {
        return Err("Old UI preferences can only be previewed in the packaged macOS app".into());
    }
    let (loaded, wait_load) = mpsc::sync_channel(1);
    let _reader = main(app, move |handle| {
        let asset = handle
            .asset_resolver()
            .get_for_scheme("/legacy-ui-store.html".into(), false)
            .ok_or("Old UI preference reader is missing; rebuild the Preview package")?;
        if !String::from_utf8_lossy(&asset.bytes)
            .contains("<title>Legacy UI preference reader</title>")
        {
            return Err("Old UI preference reader is invalid; rebuild the Preview package".into());
        }
        WebviewWindowBuilder::new(
            handle,
            LABEL,
            WebviewUrl::App("legacy-ui-store.html".into()),
        )
        .visible(false)
        .focused(false)
        .on_navigation(|url| url.as_str() == "tauri://localhost/legacy-ui-store.html")
        .on_page_load(move |webview, payload| {
            if payload.event() == tauri::webview::PageLoadEvent::Finished
                && webview.label() == LABEL
                && payload.url().as_str() == "tauri://localhost/legacy-ui-store.html"
            {
                let _ = loaded.try_send(());
            }
        })
        .build()
        .map(HiddenReader)
        .map_err(|_| "Could not open old UI preference preview; retry".into())
    })?;
    wait_load
        .recv_timeout(Duration::from_secs(10))
        .map_err(|_| "Old UI preference preview did not load; retry")?;
    let (send, receive) = mpsc::sync_channel(1);
    let script = format!(
        r#"(() => {{
      if (location.origin !== '{SOURCE}' || document.title !== 'Legacy UI preference reader'
          || document.body.children.length !== 0) throw new Error('origin');
      const values = {{}}; for (const key of {KEYS}) {{
        const value = localStorage.getItem(key); if (value !== null) values[key] = value;
      }} return JSON.stringify({{ source: '{SOURCE}', values }});
    }})()"#
    );
    main(app, move |handle| {
        let window = handle
            .get_webview_window(LABEL)
            .ok_or("UI preference reader is unavailable; retry")?;
        if window
            .url()
            .map_err(|_| "UI preference reader URL is unavailable")?
            .as_str()
            != "tauri://localhost/legacy-ui-store.html"
        {
            return Err("UI preference reader origin changed; close and retry".into());
        }
        window.with_webview(move |platform| {
            // SAFETY: Tauri supplies this live WKWebView on the main thread;
            // the callback copies only a bounded NSString before returning.
            let view = unsafe { &*platform.inner().cast::<WKWebView>() };
            let completed = RcBlock::new(move |value: *mut AnyObject, error: *mut NSError| {
                let result = if !error.is_null() {
                    Err("Old UI preferences are unavailable; retry or choose preferences manually".into())
                } else {
                    unsafe { value.as_ref() }.and_then(|value| value.downcast_ref::<NSString>())
                        .filter(|value| value.length() <= LIMIT)
                        .ok_or_else(|| "Old UI preference preview is invalid; choose preferences manually".to_string())
                        .and_then(|value| validate(&value.to_string()))
                };
                let _ = send.try_send(result);
            });
            unsafe { view.evaluateJavaScript_completionHandler(&NSString::from_str(&script), Some(&completed)); }
        }).map_err(|_| "Could not read old UI preferences; retry".into())
    })?;
    receive
        .recv_timeout(Duration::from_secs(5))
        .map_err(|_| "Old UI preference read timed out; retry".to_string())?
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn snapshot_rejects_other_origins_unknown_keys_and_unbounded_values() {
        assert!(validate(
            r#"{"source":"tauri://localhost","values":{"tauri-progress-mode":"deep"}}"#
        )
        .is_ok());
        for raw in [
            r#"{"source":"https://foreign","values":{}}"#,
            r#"{"source":"tauri://localhost","values":{"apiKey":"secret"}}"#,
            r#"{"source":"tauri://localhost","values":{"tauri-progress-mode":"\u0000"}}"#,
            r#"{"source":"tauri://localhost","values":{},"extra":true}"#,
        ] {
            assert!(validate(raw).is_err());
        }
        assert!(validate(&serde_json::json!({"source":SOURCE,"values":{"tauri-progress-mode":"x".repeat(65_537)}}).to_string()).is_err());
    }
}
