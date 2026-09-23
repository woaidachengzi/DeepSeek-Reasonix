#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod bridge;
mod data_profile;
mod keychain;
mod menu;
mod protocol_generated;
mod runtime_info;
mod tray;
mod window_state;
mod workbench_catalog;

use bridge::{
    AnswerMCPInteractionRequest, AnswerQuestionRequest, ApproveRequest, AttachFileRequest,
    BridgeAttachment, BridgeDeleteSessionResponse, BridgeHistory, BridgeProviderSummaryResponse,
    BridgeSession, BridgeSetDefaultModelRequest, BridgeSnapshot, BridgeStatus, BridgeSupervisor,
    BridgeWorkspaceChangeDetailResponse, BridgeWorkspaceChangesResponse,
    BridgeWorkspaceFileResponse, BridgeWorkspaceListResponse, OpenSessionRequest,
    RenameSessionRequest, SessionRequest, SubmitRequest, WorkspaceChangeDetailRequest,
    WorkspaceFileRequest, WorkspaceRequest,
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
fn restart_bridge(
    supervisor: State<'_, BridgeSupervisor>,
    keychain: State<'_, keychain::KeychainStore>,
) -> Result<BridgeStatus, String> {
    keychain.restart_bridge(&supervisor)
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
fn bridge_rename_session(
    supervisor: State<'_, BridgeSupervisor>,
    request: RenameSessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.rename_session(request)
}

#[tauri::command]
fn bridge_delete_session(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeDeleteSessionResponse, String> {
    supervisor.delete_session(request)
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
fn bridge_attach_file(
    supervisor: State<'_, BridgeSupervisor>,
    request: AttachFileRequest,
) -> Result<BridgeAttachment, String> {
    supervisor.attach_file(request)
}

#[tauri::command]
fn bridge_workspace(
    supervisor: State<'_, BridgeSupervisor>,
    request: WorkspaceRequest,
) -> Result<BridgeWorkspaceListResponse, String> {
    supervisor.workspace(request)
}

#[tauri::command]
fn bridge_workspace_file(
    supervisor: State<'_, BridgeSupervisor>,
    request: WorkspaceFileRequest,
) -> Result<BridgeWorkspaceFileResponse, String> {
    supervisor.workspace_file(request)
}

#[tauri::command]
fn bridge_workspace_changes(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeWorkspaceChangesResponse, String> {
    supervisor.workspace_changes(request)
}

#[tauri::command]
fn bridge_workspace_change_detail(
    supervisor: State<'_, BridgeSupervisor>,
    request: WorkspaceChangeDetailRequest,
) -> Result<BridgeWorkspaceChangeDetailResponse, String> {
    supervisor.workspace_change_detail(request)
}

#[tauri::command]
fn bridge_cancel(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.cancel(request)
}

#[tauri::command]
fn bridge_approve(
    supervisor: State<'_, BridgeSupervisor>,
    request: ApproveRequest,
) -> Result<BridgeSession, String> {
    supervisor.approve(request)
}

#[tauri::command]
fn bridge_answer_question(
    supervisor: State<'_, BridgeSupervisor>,
    request: AnswerQuestionRequest,
) -> Result<BridgeSession, String> {
    supervisor.answer_question(request)
}

#[tauri::command]
fn bridge_answer_mcp_interaction(
    supervisor: State<'_, BridgeSupervisor>,
    request: AnswerMCPInteractionRequest,
) -> Result<BridgeSession, String> {
    supervisor.answer_mcp_interaction(request)
}

#[tauri::command]
fn bridge_replay_pending_prompts(
    supervisor: State<'_, BridgeSupervisor>,
    request: SessionRequest,
) -> Result<BridgeSession, String> {
    supervisor.replay_pending_prompts(request)
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

#[tauri::command]
fn forget_workbench_session(
    catalog: State<'_, WorkbenchCatalog>,
    session_id: String,
) -> Result<Vec<WorkbenchSession>, String> {
    catalog.forget(&session_id)
}

#[tauri::command]
fn platform_info() -> &'static str {
    if cfg!(target_os = "macos") {
        "darwin"
    } else if cfg!(target_os = "windows") {
        "windows"
    } else {
        "linux"
    }
}

fn main() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            // When a second instance is launched, focus the existing window
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.show();
                let _ = window.set_focus();
            }
        }))
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_notification::init())
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

            let menu = menu::build_app_menu(app)
                .map_err(|e| std::io::Error::other(e.to_string()))?;
            app.set_menu(menu)?;

            tray::create_tray(app)
                .map_err(|e| std::io::Error::other(e.to_string()))?;

            // Initialize keychain store
            let keychain = keychain::KeychainStore::new();
            keychain.initialize(app.handle()).map_err(std::io::Error::other)?;

            supervisor.start().map_err(std::io::Error::other)?;
            if let Err(error) = keychain.restore_provider_api_keys(&supervisor) {
                // A native credential service can be temporarily unavailable.
                // Keep the app usable with its existing file-backed settings;
                // the provider will simply remain unavailable until the next
                // startup or a user saves the key again.
                eprintln!("Reasonix could not restore system keychain credentials: {error}");
            }
            app.manage(keychain);

            app.manage(supervisor);
            app.manage(profile);
            app.manage(window_state);
            app.manage(workbench_catalog);
            Ok(())
        })
        .on_window_event(|window, event| {
            // macOS: hide to tray on close button instead of quitting
            #[cfg(target_os = "macos")]
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                // Prevent the default close behavior
                api.prevent_close();
                // Hide the window instead
                let _ = window.hide();
            }
        })
        .on_menu_event(|app, event| {
            match event.id().as_ref() {
                "quit" => {
                    // Cmd+Q: actually quit the application
                    app.exit(0);
                }
                "about" => {
                    let version = env!("CARGO_PKG_VERSION");
                    let stable = runtime_info::STABLE_VERSION;
                    let commit = &runtime_info::STABLE_COMMIT[..8];
                    let msg = format!(
                        "Reasonix Tauri Preview\n\nVersion: {version}\nBased on Stable: {stable} ({commit})\nTauri: {}\nBridge Protocol: {}",
                        tauri::VERSION,
                        bridge::PROTOCOL_VERSION
                    );
                    let dialog = tauri_plugin_dialog::DialogExt::dialog(app);
                    let _ = dialog
                        .message(&msg)
                        .title("About Reasonix")
                        .kind(tauri_plugin_dialog::MessageDialogKind::Info)
                        .blocking_show();
                }
                "check_updates" => {
                    let version = env!("CARGO_PKG_VERSION");
                    let msg = format!(
                        "Current version: {version}\n\nAuto-update checks are performed at startup.\nTo manually check, restart the application."
                    );
                    let dialog = tauri_plugin_dialog::DialogExt::dialog(app);
                    let _ = dialog
                        .message(&msg)
                        .title("Check for Updates")
                        .kind(tauri_plugin_dialog::MessageDialogKind::Info)
                        .blocking_show();
                }
                "reload" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.eval("location.reload()");
                    }
                }
                "force_reload" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.eval("location.reload()");
                    }
                }
                "toggle_fullscreen" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.set_fullscreen(!window.is_fullscreen().unwrap_or(false));
                    }
                }
                "zoom_in" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.eval("document.body.style.zoom = (parseFloat(document.body.style.zoom || '1') + 0.1).toString()");
                    }
                }
                "zoom_out" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.eval("document.body.style.zoom = Math.max(0.5, parseFloat(document.body.style.zoom || '1') - 0.1).toString()");
                    }
                }
                "zoom_reset" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.eval("document.body.style.zoom = '1'");
                    }
                }
                "minimize" => {
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.minimize();
                    }
                }
                "hide" => {
                    let _ = app.hide();
                }
                "hide_others" => {
                    // macOS-only: hide other applications
                    #[cfg(target_os = "macos")]
                    {
                        // Tauri doesn't expose hide_others directly; we rely on
                        // the native menu accelerator to handle this via macOS.
                        // The menu item with Cmd+Alt+H triggers the system behavior.
                    }
                }
                _ => {}
            }
        })
        .invoke_handler(tauri::generate_handler![
            bridge_status,
            restart_bridge,
            bridge_open_session,
            bridge_switch_session,
            bridge_rename_session,
            bridge_delete_session,
            bridge_session_snapshot,
            bridge_session_history,
            bridge_submit,
            bridge_attach_file,
            bridge_workspace,
            bridge_workspace_file,
            bridge_workspace_changes,
            bridge_workspace_change_detail,
            bridge_cancel,
            bridge_approve,
            bridge_answer_question,
            bridge_answer_mcp_interaction,
            bridge_replay_pending_prompts,
            bridge_start_events,
            preview_profile_status,
            preview_runtime_info,
            provider_summary,
            set_default_model,
            import_stable_profile,
            workbench_sessions,
            remember_workbench_session,
            forget_workbench_session,
            platform_info,
            keychain::keychain_save,
            keychain::keychain_load,
            keychain::keychain_delete
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
