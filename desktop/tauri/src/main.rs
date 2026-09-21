#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod bridge;
mod data_profile;
mod protocol_generated;
mod runtime_info;
mod window_state;
mod workbench_catalog;

use bridge::{
    BridgeHistory, BridgeProviderSummaryResponse, BridgeSession, BridgeSetDefaultModelRequest,
    BridgeSnapshot, BridgeStatus, BridgeSupervisor, OpenSessionRequest, SessionRequest,
    SubmitRequest,
};
use data_profile::{PreviewProfile, PreviewProfileStatus, ProfileImportResult};
use runtime_info::PreviewRuntimeInfo;
use tauri::{Manager, State};
use window_state::PreviewWindowState;
use workbench_catalog::{WorkbenchCatalog, WorkbenchSession};

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
fn bridge_switch_session(
    supervisor: State<'_, BridgeSupervisor>,
    request: OpenSessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.switch_session(request)
}

#[tauri::command]
fn bridge_session_snapshot(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeSnapshot, String> {
    supervisor.snapshot(request)
}

#[tauri::command]
fn bridge_session_history(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeHistory, String> {
    supervisor.history(request)
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
    after_sequence: Option<u64>,
) -> Result<(), String> {
    supervisor.start_events(app, after_sequence.unwrap_or(0))
}

#[tauri::command]
fn preview_profile_status(profile: State<'_, PreviewProfile>) -> PreviewProfileStatus {
    profile.status()
}

#[tauri::command]
fn preview_runtime_info(supervisor: State<'_, BridgeSupervisor>) -> PreviewRuntimeInfo {
    PreviewRuntimeInfo::from_status(supervisor.status())
}

#[tauri::command]
fn provider_summary(
    supervisor: State<'_, BridgeSupervisor>,
) -> Result<BridgeProviderSummaryResponse, String> {
    supervisor.provider_summary()
}

#[tauri::command]
fn set_default_model(
    supervisor: State<'_, BridgeSupervisor>,
    request: BridgeSetDefaultModelRequest,
) -> Result<BridgeProviderSummaryResponse, String> {
    supervisor.set_default_model(request)
}

#[tauri::command]
fn import_stable_profile(
    profile: State<'_, PreviewProfile>,
    confirmed: bool,
) -> Result<ProfileImportResult, String> {
    if !confirmed {
        return Err("stable config import requires explicit confirmation".into());
    }
    profile.import_stable_config()
}

#[tauri::command]
fn workbench_sessions(
    catalog: State<'_, WorkbenchCatalog>,
) -> Result<Vec<WorkbenchSession>, String> {
    catalog.list()
}

#[tauri::command]
fn remember_workbench_session(
    catalog: State<'_, WorkbenchCatalog>,
    request: WorkbenchSession,
) -> Result<Vec<WorkbenchSession>, String> {
    catalog.remember(request)
}

fn main() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_shell::init())
        .setup(|app| {
            let window_state = PreviewWindowState::for_app(app)?;
            let workbench_catalog =
                WorkbenchCatalog::for_app(app).map_err(std::io::Error::other)?;
            if let Some(window) = app.get_webview_window("main") {
                window_state.restore(&window);
            }
            let profile =
                data_profile::configure_preview_profile(app).map_err(std::io::Error::other)?;
            let supervisor = BridgeSupervisor::from_environment(app.handle().clone());
            supervisor.start().map_err(std::io::Error::other)?;
            app.manage(supervisor);
            app.manage(profile);
            app.manage(window_state);
            app.manage(workbench_catalog);
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            bridge_status,
            restart_bridge,
            bridge_open_session,
            bridge_switch_session,
            bridge_session_snapshot,
            bridge_session_history,
            bridge_submit,
            bridge_cancel,
            bridge_start_events,
            preview_profile_status,
            preview_runtime_info,
            provider_summary,
            set_default_model,
            import_stable_profile,
            workbench_sessions,
            remember_workbench_session
        ])
        .build(tauri::generate_context!())
        .expect("failed to build Reasonix Tauri host");
    app.run(|app, event| {
        if matches!(event, tauri::RunEvent::Exit) {
            if let Some(window) = app.get_webview_window("main") {
                let _ = app.state::<PreviewWindowState>().save(&window);
            }
            let _ = app.state::<BridgeSupervisor>().stop();
        }
    });
}
