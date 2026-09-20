#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod bridge;

use bridge::{
    BridgeSession, BridgeSnapshot, BridgeStatus, BridgeSupervisor, OpenSessionRequest,
    SessionRequest, SubmitRequest,
};
use tauri::{Manager, State};

#[tauri::command]
fn bridge_status(supervisor: State<'_, BridgeSupervisor>) -> BridgeStatus {
    supervisor.status()
}

#[tauri::command]
fn restart_bridge(supervisor: State<'_, BridgeSupervisor>) -> Result<BridgeStatus, String> {
    supervisor.restart()
}

#[tauri::command]
fn bridge_open_session(
    supervisor: State<'_, BridgeSupervisor>,
    request: OpenSessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.open_session(request)
}

#[tauri::command]
fn bridge_session_snapshot(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeSnapshot, String> {
    supervisor.snapshot(request)
}

#[tauri::command]
fn bridge_submit(
    supervisor: State<'_, BridgeSupervisor>,
    request: SubmitRequest,
) -> Result<BridgeSession, String> {
    supervisor.submit(request)
}

#[tauri::command]
fn bridge_cancel(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.cancel(request)
}

#[tauri::command]
fn bridge_start_events(
    app: tauri::AppHandle,
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<(), String> {
    supervisor.start_events(app)
}

fn main() {
    let supervisor = BridgeSupervisor::from_environment().unwrap_or_else(|error| panic!("{error}"));
    let app = tauri::Builder::default()
        .manage(supervisor)
        .setup(|app| {
            app.state::<BridgeSupervisor>()
                .start()
                .map_err(std::io::Error::other)?;
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            bridge_status,
            restart_bridge,
            bridge_open_session,
            bridge_session_snapshot,
            bridge_submit,
            bridge_cancel,
            bridge_start_events
        ])
        .build(tauri::generate_context!())
        .expect("failed to build Reasonix Tauri host");
    app.run(|app, event| {
        if matches!(event, tauri::RunEvent::Exit) {
            let _ = app.state::<BridgeSupervisor>().stop();
        }
    });
}
