//! Native close/restore/quit while the packaged Go core is actually streaming.
//! The runner configures a bounded loopback provider in its private profile.

use std::{
    path::Path,
    sync::{
        atomic::{AtomicBool, Ordering},
        Arc,
    },
    time::{Duration, Instant},
};

use tauri::{AppHandle, EventId, Listener, Manager};

use crate::{
    bridge::{BridgeEvent, BridgeSupervisor, OpenSessionRequest, SessionRequest, SubmitRequest},
    host_preferences::{CloseBehavior, HostPreferences},
    native_menu_smoke,
    native_window_smoke::{expected_geometry, on_main, verify_geometry, wait_for},
};

const BACKGROUND_SESSION: &str = "native-background-task";
const QUIT_SESSION: &str = "native-quit-task";
const BEFORE_CLOSE: &str = "native-before-close ";
const AFTER_CLOSE: &str = "native-after-close ";
const COMPLETED: &str = "native-completed";
const BEFORE_QUIT: &str = "native-before-quit";

struct EventListener {
    app: AppHandle,
    id: EventId,
}
impl Drop for EventListener {
    fn drop(&mut self) {
        self.app.unlisten(self.id);
    }
}

fn wait_task(
    stage: &'static str,
    mut predicate: impl FnMut() -> Result<bool, String>,
) -> Result<(), String> {
    let deadline = Instant::now() + Duration::from_secs(10);
    loop {
        if predicate()? {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err(format!("native task timeout: {stage}"));
        }
        std::thread::sleep(Duration::from_millis(50));
    }
}

fn running(supervisor: &BridgeSupervisor, id: &str) -> Result<bool, String> {
    supervisor
        .snapshot(SessionRequest {
            session_id: id.into(),
        })
        .map(|snapshot| snapshot.session.state == "running")
        .map_err(|_| "read native task state".into())
}

fn submit(app: &AppHandle, supervisor: &BridgeSupervisor, id: &str) -> Result<(), String> {
    // Explicitly hand off the previous idle controller. Open deliberately
    // rejects a different session even after its task has completed.
    supervisor
        .switch_session(OpenSessionRequest {
            session_id: id.into(),
            workspace_root: None,
        })
        .map_err(|error| format!("switch native task session {id}: {error}"))?;
    // Follow the renderer's ordinary snapshot/replay subscription sequence.
    // A Rust listener alone does not start the bridge's SSE forwarder.
    let snapshot = supervisor
        .snapshot(SessionRequest {
            session_id: id.into(),
        })
        .map_err(|_| "snapshot native task before subscription")?;
    supervisor
        .start_events(app.clone(), snapshot.sequence)
        .map_err(|_| "subscribe native task events")?;
    supervisor
        .submit(SubmitRequest {
            session_id: id.into(),
            input: "native lifecycle acceptance".into(),
        })
        .map_err(|_| "submit native task")?;
    Ok(())
}

pub fn host_death(app: &AppHandle, directory: &Path) -> Result<(), String> {
    const ID: &str = "native-host-death-task";
    let started = Arc::new(AtomicBool::new(false));
    let observed = Arc::clone(&started);
    let id = app.listen("bridge:event", move |event| {
        if serde_json::from_str::<BridgeEvent>(event.payload()).is_ok_and(|envelope| {
            envelope.protocol_version == u64::from(crate::bridge::PROTOCOL_VERSION)
                && envelope.session_id == ID
                && envelope.event_kind == "text"
                && envelope.payload.get("text").and_then(|text| text.as_str()) == Some(BEFORE_CLOSE)
        }) {
            observed.store(true, Ordering::SeqCst);
        }
    });
    let _listener = EventListener {
        app: app.clone(),
        id,
    };
    let supervisor = app.state::<BridgeSupervisor>();
    submit(app, &supervisor, ID)?;
    wait_task("host-death actual streamed task", || {
        Ok(started.load(Ordering::SeqCst) && running(&supervisor, ID)?)
    })?;
    std::fs::write(directory.join("reasonix-native-task-host-death.json"),
        serde_json::json!({"phase":"task-host-death", "hostPid": std::process::id(), "taskRunning":true, "observedText":true}).to_string())
        .map_err(|_| "publish host-death task state")?;
    // The external runner must kill its own confirmed host. A timeout exits as
    // a failed probe, never as successful native termination acceptance.
    std::thread::sleep(Duration::from_secs(30));
    Err("external host termination did not arrive".into())
}

pub fn run(app: &AppHandle, directory: &Path, phase: &str) -> Result<(), String> {
    let expected = expected_geometry(directory)?;
    verify_geometry(app, "normal window before background task", expected)?;
    let started = Arc::new(AtomicBool::new(false));
    let progressed = Arc::new(AtomicBool::new(false));
    let quitting = Arc::new(AtomicBool::new(false));
    let (first, second, third) = (
        Arc::clone(&started),
        Arc::clone(&progressed),
        Arc::clone(&quitting),
    );
    let id = app.listen("bridge:event", move |event| {
        let Ok(envelope) = serde_json::from_str::<BridgeEvent>(event.payload()) else {
            return;
        };
        if envelope.protocol_version != u64::from(crate::bridge::PROTOCOL_VERSION)
            || envelope.event_kind != "text"
        {
            return;
        }
        match (
            envelope.session_id.as_str(),
            envelope.payload.get("text").and_then(|text| text.as_str()),
        ) {
            (BACKGROUND_SESSION, Some(BEFORE_CLOSE)) => first.store(true, Ordering::SeqCst),
            (BACKGROUND_SESSION, Some(AFTER_CLOSE)) => second.store(true, Ordering::SeqCst),
            (QUIT_SESSION, Some(BEFORE_QUIT)) => third.store(true, Ordering::SeqCst),
            _ => {}
        }
    });
    let _listener = EventListener {
        app: app.clone(),
        id,
    };
    let supervisor = app.state::<BridgeSupervisor>();
    submit(app, &supervisor, BACKGROUND_SESSION)?;
    wait_task("first real streamed event", || {
        Ok(started.load(Ordering::SeqCst) && running(&supervisor, BACKGROUND_SESSION)?)
    })
    .map_err(|error| {
        let state = supervisor
            .snapshot(SessionRequest {
                session_id: BACKGROUND_SESSION.into(),
            })
            .map(|snapshot| snapshot.session.state)
            .unwrap_or("unavailable".into());
        format!(
            "{error}; observedText={}, taskState={state}",
            started.load(Ordering::SeqCst)
        )
    })?;
    on_main(app, |handle, window| {
        handle
            .state::<HostPreferences>()
            .set_close_behavior(CloseBehavior::KeepRunning)?;
        window
            .close()
            .map_err(|_| "close window during native task".into())
    })?;
    wait_for(app, "hidden window during task", |window| {
        window
            .is_visible()
            .map(|visible| !visible)
            .map_err(|_| "read visibility".into())
    })?;
    if !running(&supervisor, BACKGROUND_SESSION)? {
        return Err("closing window interrupted the native task".into());
    }
    std::fs::write(directory.join("reasonix-native-task-hidden.json"), b"{}")
        .map_err(|_| "publish hidden task window")?;
    wait_task("stream progress after native close", || {
        Ok(progressed.load(Ordering::SeqCst) && running(&supervisor, BACKGROUND_SESSION)?)
    })?;
    wait_for(
        app,
        "task progressed while window remained hidden",
        |window| {
            window
                .is_visible()
                .map(|visible| !visible)
                .map_err(|_| "read visibility".into())
        },
    )?;
    std::fs::write(directory.join("reasonix-native-task-progress.json"), b"{}")
        .map_err(|_| "publish background task progress")?;
    wait_task("background task completed with actual history", || {
        let history = supervisor
            .history(SessionRequest {
                session_id: BACKGROUND_SESSION.into(),
            })
            .map_err(|_| "read completed native task history")?;
        Ok(history.session.state == "idle"
            && history.messages.iter().any(|message| {
                message.role == "assistant"
                    && message.content == format!("{BEFORE_CLOSE}{AFTER_CLOSE}{COMPLETED}")
            }))
    })?;
    native_menu_smoke::invoke(app, "Show Reasonix")?;
    verify_geometry(
        app,
        "native menu restores completed task window",
        expected_geometry(directory)?,
    )?;

    submit(app, &supervisor, QUIT_SESSION)?;
    wait_task("active stream before native quit", || {
        Ok(quitting.load(Ordering::SeqCst) && running(&supervisor, QUIT_SESSION)?)
    })?;
    // The runner must also observe a real host exit, upstream disconnection
    // and sidecar/readiness cleanup. A result marker alone is not success.
    std::fs::write(
        directory.join("reasonix-native-window-result.json"),
        serde_json::json!({"phase":phase,"ok":true}).to_string(),
    )
    .map_err(|_| "publish native task acceptance")?;
    native_menu_smoke::invoke(app, "Quit Reasonix")
}
