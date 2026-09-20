#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod bridge;

use bridge::{BridgeStatus, BridgeSupervisor};
use tauri::{Manager, State};

#[tauri::command]
fn bridge_status(supervisor: State<'_, BridgeSupervisor>) -> BridgeStatus {
    supervisor.status()
}

#[tauri::command]
fn restart_bridge(supervisor: State<'_, BridgeSupervisor>) -> Result<BridgeStatus, String> {
    supervisor.restart()
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
        .invoke_handler(tauri::generate_handler![bridge_status, restart_bridge])
        .build(tauri::generate_context!())
        .expect("failed to build Reasonix Tauri host");
    app.run(|app, event| {
        if matches!(event, tauri::RunEvent::Exit) {
            let _ = app.state::<BridgeSupervisor>().stop();
        }
    });
}
