//! Display-only feedback from authenticated, app-approved native observations.
//! Never accepts renderer coordinates or synthesizes input.
use crate::protocol_generated::BridgeEvent;
#[cfg(target_os = "macos")]
use serde::Deserialize;

#[cfg(target_os = "macos")]
#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct Cursor {
    x: f64,
    y: f64,
    screen_x: f64,
    screen_y: f64,
    #[serde(default)]
    clicked: bool,
}

#[cfg(target_os = "macos")]
impl Cursor {
    fn valid(&self) -> bool {
        [self.x, self.y, self.screen_x, self.screen_y]
            .iter()
            .all(|v| v.is_finite())
            && (0.0..1.0).contains(&self.x)
            && (0.0..1.0).contains(&self.y)
            && self.screen_x.abs() < 1_000_000.0
            && self.screen_y.abs() < 1_000_000.0
    }
}

#[cfg(target_os = "macos")]
#[derive(Clone, Debug)]
struct Frame {
    session: String,
    turn: String,
    pid: i32,
    window: i64,
    cursor: Cursor,
}

#[cfg(target_os = "macos")]
fn frame(event: &BridgeEvent) -> Option<Frame> {
    if event.event_kind != "tool_progress" {
        return None;
    }
    let turn = event.payload.get("turnId")?.as_str()?;
    let tool = event.payload.get("tool")?;
    if tool.get("name")?.as_str()? != "computer_use" {
        return None;
    }
    let preview = tool.get("computerUsePreview")?;
    let pid = i32::try_from(preview.get("pid")?.as_i64()?).ok()?;
    let window = preview.get("windowId")?.as_i64()?;
    let mut cursor: Cursor = serde_json::from_value(preview.get("cursor")?.clone()).ok()?;
    if pid <= 0
        || !(1..=u32::MAX as i64).contains(&window)
        || !cursor.valid()
        || turn.is_empty()
        || turn.len() > 256
        || event.session_id.is_empty()
        || event.session_id.len() > 256
    {
        return None;
    }
    cursor.clicked &= matches!(
        preview.get("action").and_then(|v| v.as_str()),
        Some("click" | "confirm_click")
    );
    Some(Frame {
        session: event.session_id.clone(),
        turn: turn.into(),
        pid,
        window,
        cursor,
    })
}

#[cfg(target_os = "macos")]
#[path = "computer_use_cursor/native.rs"]
mod native;

pub fn begin_stream(app: &tauri::AppHandle, after_sequence: u64) -> u64 {
    #[cfg(target_os = "macos")]
    return native::begin_stream(app, after_sequence);
    #[cfg(not(target_os = "macos"))]
    {
        let _ = (app, after_sequence);
        0
    }
}

pub fn observe(app: &tauri::AppHandle, event: &BridgeEvent, stream: u64) {
    #[cfg(target_os = "macos")]
    native::observe(app, event, stream);
    #[cfg(not(target_os = "macos"))]
    let _ = (app, event, stream);
}

pub fn clear_session(app: &tauri::AppHandle, session: &str) {
    #[cfg(target_os = "macos")]
    native::clear_session(app, session);
    #[cfg(not(target_os = "macos"))]
    let _ = (app, session);
}

#[cfg(all(test, target_os = "macos"))]
mod tests {
    use super::*;
    fn event(cursor: serde_json::Value) -> BridgeEvent {
        serde_json::from_value(serde_json::json!({"protocolVersion":1,"sequence":1,"eventKind":"tool_progress","sessionId":"a","payload":{"turnId":"one","tool":{"name":"computer_use","computerUsePreview":{"pid":42,"windowId":11,"action":"click","cursor":cursor}}}})).unwrap()
    }
    #[test]
    fn only_native_valid_cursor_samples_are_presented() {
        let point =
            serde_json::json!({"x":0.4,"y":0.2,"screenX":-1900,"screenY":-300,"clicked":true});
        let mut e = event(point.clone());
        let f = frame(&e).unwrap();
        assert!(f.cursor.clicked && f.pid == 42 && f.window == 11);
        assert_eq!(
            (&f.session, &f.turn),
            (&"a".to_string(), &"one".to_string())
        );
        e.event_kind = "tool_result".into();
        assert!(frame(&e).is_none());
        for patch in [
            serde_json::json!({"x":1.0}),
            serde_json::json!({"y":-0.1}),
            serde_json::json!({"screenX":1e99}),
        ] {
            let mut p = point.clone();
            for (k, v) in patch.as_object().unwrap() {
                p[k] = v.clone();
            }
            assert!(frame(&event(p)).is_none());
        }
        let mut e = event(point);
        e.payload["tool"]["computerUsePreview"]["action"] = "observe".into();
        assert!(!frame(&e).unwrap().cursor.clicked);
    }
}

#[cfg(all(target_os = "macos", feature = "cursor-qa"))]
pub fn acceptance_fixture(output: &str) -> Result<(), String> {
    native::acceptance_fixture(output)
}
