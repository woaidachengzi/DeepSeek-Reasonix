//! macOS initial visibility follows the Wails hidden -> DOM-ready presentation.
//! Readiness and Show intent have one owner; reload/late events cannot reopen.
use std::sync::Mutex;
use tauri::{AppHandle, Manager, Webview};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum Intent {
    Startup,
    Explicit,
}

struct Gate {
    runtime: bool,
    page: bool,
    closed: bool,
    pending: Option<Intent>,
}
impl Default for Gate {
    fn default() -> Self {
        Self {
            runtime: false,
            page: false,
            closed: false,
            pending: Some(Intent::Startup),
        }
    }
}
impl Gate {
    fn can_present(&self) -> bool {
        self.runtime && self.page && !self.closed
    }
    fn take_ready(&mut self) -> Option<Intent> {
        if self.can_present() {
            self.pending.take()
        } else {
            None
        }
    }
    fn request(&mut self) -> Option<Intent> {
        if self.closed {
            return None;
        }
        self.pending = Some(Intent::Explicit);
        self.take_ready()
    }
    fn finish(&mut self, runtime: bool) -> Option<Intent> {
        if runtime {
            self.runtime = true;
        } else {
            self.page = true;
        }
        self.take_ready()
    }
    fn close(&mut self) {
        self.closed = true;
        self.pending = None;
    }
}
#[derive(Default)]
pub struct InitialPresentation(Mutex<Gate>);

fn present(app: &AppHandle, intent: Intent) {
    if intent == Intent::Startup {
        crate::native_window_smoke::observe_restore(app, false);
    }
    crate::tray::present_main_window(app, intent == Intent::Explicit);
}

/// Called on the native UI thread by the ordinary Show/Dock/tray/instance path.
pub fn request(app: &AppHandle) {
    if let Some(state) = app.try_state::<InitialPresentation>() {
        let action = state.0.lock().unwrap_or_else(|e| e.into_inner()).request();
        if let Some(intent) = action {
            present(app, intent);
        }
    }
}
pub fn runtime_ready(app: &AppHandle) {
    let state = app.state::<InitialPresentation>();
    let action = state
        .0
        .lock()
        .unwrap_or_else(|e| e.into_inner())
        .finish(true);
    if let Some(intent) = action {
        present(app, intent);
    }
}
pub fn observe(webview: &Webview, payload: &tauri::webview::PageLoadPayload<'_>) {
    let app = webview.app_handle();
    let trusted = crate::ui_origin::matches(app, payload.url());
    #[cfg(debug_assertions)]
    let trusted = trusted
        || app.config().build.dev_url.as_ref().is_some_and(|url| {
            url.origin() == payload.url().origin()
                && payload.url().username().is_empty()
                && payload.url().password().is_none()
        });
    if webview.label() != "main"
        || !trusted
        || payload.event() != tauri::webview::PageLoadEvent::Finished
    {
        return;
    }
    if let Some(state) = app.try_state::<InitialPresentation>() {
        let action = state
            .0
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .finish(false);
        if let Some(intent) = action {
            present(app, intent);
        }
    }
}
pub fn close(app: &AppHandle) {
    if let Some(state) = app.try_state::<InitialPresentation>() {
        state.0.lock().unwrap_or_else(|e| e.into_inner()).close();
    }
}

/// Recheck inside the queued native action, after an exit may have cancelled it.
pub fn can_present(app: &AppHandle) -> bool {
    app.try_state::<InitialPresentation>().is_some_and(|state| {
        state
            .0
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .can_present()
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn both_readiness_orders_present_once_and_reload_does_not_reopen() {
        for runtime_first in [true, false] {
            let mut gate = Gate::default();
            assert_eq!(gate.finish(runtime_first), None);
            assert_eq!(gate.finish(!runtime_first), Some(Intent::Startup));
            assert_eq!(gate.finish(false), None);
            assert_eq!(gate.finish(true), None);
        }
    }
    #[test]
    fn early_explicit_requests_coalesce_and_override_startup_intent() {
        let mut gate = Gate::default();
        assert_eq!(gate.request(), None);
        assert_eq!(gate.finish(true), None);
        assert_eq!(gate.request(), None);
        assert_eq!(gate.finish(false), Some(Intent::Explicit));
        assert_eq!(gate.finish(false), None);
        assert_eq!(gate.request(), Some(Intent::Explicit));
    }
    #[test]
    fn exit_before_readiness_cancels_late_events_and_requests() {
        let mut gate = Gate::default();
        gate.finish(true);
        gate.request();
        gate.close();
        assert_eq!(gate.finish(false), None);
        assert_eq!(gate.request(), None);
    }
    #[test]
    fn exit_cancels_an_already_consumed_presentation() {
        let mut gate = Gate::default();
        gate.finish(true);
        assert_eq!(gate.finish(false), Some(Intent::Startup));
        assert!(gate.can_present());
        gate.close();
        assert!(!gate.can_present());
        assert_eq!(gate.request(), None);
    }
}
