//! Opt-in packaged desktop-tool acceptance against a runner-owned local fixture.
use crate::{
    bridge::{BridgeSupervisor, OpenSessionRequest, SessionRequest, SubmitRequest},
    data_profile::PreviewProfile,
};
use std::{
    path::Path,
    time::{Duration, Instant},
};
use tauri::{AppHandle, Manager};

pub fn run(app: &AppHandle, directory: &Path) -> Result<(), String> {
    let root = directory
        .canonicalize()
        .map_err(|_| "resolve computer-use fixture root")?;
    let home = app
        .state::<PreviewProfile>()
        .home()
        .canonicalize()
        .map_err(|_| "resolve acceptance profile")?;
    if !home.starts_with(&root) || home == root {
        return Err(
            "computer-use acceptance requires a profile inside the runner's temporary directory"
                .into(),
        );
    }
    let supervisor = app.state::<BridgeSupervisor>();
    let providers = supervisor.provider_summary()?;
    if providers.default_model != "local/desktop-qa" {
        return Err("computer-use acceptance requires the isolated fixture model".into());
    }
    let id = format!("native-computer-use-{}", std::process::id());
    supervisor.switch_session(OpenSessionRequest {
        session_id: id.clone(),
        workspace_root: Some(root.join("qa-workspace").to_string_lossy().into_owned()),
        model_ref: Some("local/desktop-qa".into()),
        effort: None,
    })?;
    supervisor.submit(SubmitRequest {
        session_id: id.clone(),
        input: "desktop acceptance: operate only the runner-owned Reasonix Control Fixture".into(),
    })?;
    let deadline = Instant::now() + Duration::from_secs(90);
    loop {
        let snapshot = supervisor.snapshot(SessionRequest {
            session_id: id.clone(),
        })?;
        // A scoped app-access prompt pauses the turn. Let the operator answer
        // the real UI rather than treating a pending approval as completion.
        if snapshot.session.state == "idle" {
            let fixture: serde_json::Value = serde_json::from_slice(
                &std::fs::read(root.join("fixture-state.json"))
                    .map_err(|_| "read fixture state")?,
            )
            .map_err(|_| "decode fixture state")?;
            let success = fixture["clicks"] == 2
                && fixture["text"] == "Desktop acceptance passed ✅"
                && fixture["scroll_y"].as_f64().unwrap_or(0.0) > 0.0;
            std::fs::write(
                root.join("computer-use-acceptance.json"),
                serde_json::json!({"ok":success,"sessionId":id,"fixture":fixture}).to_string(),
            )
            .map_err(|_| "write computer-use acceptance result")?;
            return if success {
                Ok(())
            } else {
                Err("fixture postconditions failed; inspect the isolated session's tool results and macOS permissions".into())
            };
        }
        if Instant::now() >= deadline {
            return Err("computer-use acceptance timed out".into());
        }
        std::thread::sleep(Duration::from_millis(100));
    }
}
