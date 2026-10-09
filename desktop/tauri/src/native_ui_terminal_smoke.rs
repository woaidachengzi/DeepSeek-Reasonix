//! Opt-in real WKWebView -> registered IPC -> packaged Go -> PTY acceptance.
//! DOM InputEvent exercises xterm's input handler, not a physical keyboard or
//! clipboard. Only fresh runner-owned profiles and working directories are used.
use crate::bridge::{BridgeSupervisor, OpenSessionRequest, RenameSessionRequest, SessionRequest};
use crate::terminal::{TargetRequest, WorkspaceRequest};
use base64::{engine::general_purpose::STANDARD, Engine};
use std::{
    path::Path,
    time::{Duration, Instant},
};
use tauri::{AppHandle, Manager};

const FIRST: &str = "terminal-ui-first";
const SECOND: &str = "terminal-ui-second";

fn check(app: &AppHandle, expression: &str, stage: &str) -> Result<(), String> {
    let script = format!("(() => (({expression}) ? 'edit-ok' : 'edit-pending'))()");
    let deadline = Instant::now() + Duration::from_secs(10);
    loop {
        if crate::native_edit_smoke::evaluate(app, script.clone())? {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err(format!("terminal UI did not settle: {stage}"));
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}
fn act(app: &AppHandle, script: &str) -> Result<(), String> {
    if crate::native_edit_smoke::evaluate(
        app,
        format!("(() => {{ {script}; return 'edit-ok'; }})()"),
    )? {
        Ok(())
    } else {
        Err("terminal UI action failed".into())
    }
}
fn click(app: &AppHandle, selector: &str) -> Result<(), String> {
    act(
        app,
        &format!(
            "document.querySelector({}).click()",
            serde_json::to_string(selector).unwrap()
        ),
    )
}
fn select(app: &AppHandle, title: &str) -> Result<(), String> {
    let title = serde_json::to_string(title).map_err(|_| "encode terminal session title")?;
    act(app, &format!("[...document.querySelectorAll('.tauri-sidebar__session')].find(node => node.textContent.trim() === {title}).click()"))?;
    check(app, &format!("document.querySelector('.tauri-topbar__title strong')?.textContent.trim() === {title} && document.querySelector('[aria-label=\"切换终端面板\"]')?.disabled === false"), "active owned conversation")
}
fn new_terminal(app: &AppHandle, session_id: &str) -> Result<String, String> {
    click(app, ".terminal-panel__actions .terminal-icon-button")?;
    check(app, "document.querySelector('.terminal-session__close') && document.querySelector('.xterm-helper-textarea') && document.querySelector('.xterm-screen')?.getBoundingClientRect().height > 0", "created visible xterm")?;
    let response = app
        .state::<BridgeSupervisor>()
        .terminal_client()?
        .workspace(WorkspaceRequest {
            session_id: session_id.into(),
        })?;
    if response.workspace.sessions.len() != 1 || !response.workspace.sessions[0].running {
        return Err("native UI did not create exactly one PTY".into());
    }
    Ok(response.workspace.sessions[0].id.clone())
}
fn input(app: &AppHandle, command: &str) -> Result<(), String> {
    let literal = serde_json::to_string(command).map_err(|_| "encode owned terminal input")?;
    // Uses the production xterm textarea's InputEvent listener. No direct IPC,
    // native client input, model submission or mock is used for this action.
    act(app, &format!("const editor=document.querySelector('.xterm-helper-textarea'); editor.focus(); editor.dispatchEvent(new InputEvent('input', {{data:{literal},inputType:'insertText',bubbles:true,cancelable:true}}))"))
}
fn owned_pid(
    app: &AppHandle,
    session_id: &str,
    terminal_id: &str,
    cwd: &Path,
) -> Result<u32, String> {
    // Split the marker across printf calls so driver echo cannot satisfy it.
    input(app, "if test -t 0 && test -t 1; then printf 'owned-'; printf 'terminal:%s\\n' \"$$\"; fi; printf '\\033[32m中文-output\\033[0m\\n'; pwd\r")?;
    let client = app.state::<BridgeSupervisor>().terminal_client()?;
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        let response = client.output(TargetRequest {
            session_id: session_id.into(),
            terminal_id: terminal_id.into(),
        })?;
        let bytes = STANDARD
            .decode(response.output.data)
            .map_err(|_| "decode native PTY receipt")?;
        let text = String::from_utf8_lossy(&bytes);
        if let Some(pid) = text
            .split("owned-terminal:")
            .nth(1)
            .and_then(|suffix| suffix.lines().next())
            .and_then(|line| line.trim().parse::<u32>().ok())
            .filter(|pid| *pid > 1)
        {
            if text.contains(&format!("{}\r\n", cwd.display()))
                && text.contains("\x1b[32m中文-output\x1b[0m")
            {
                if let Err(error) = check(app, &format!("document.querySelector('.xterm-rows')?.textContent.includes('owned-terminal:{pid}') && document.querySelector('.xterm-rows')?.textContent.includes('中文-output')"), "real PTY bytes painted in xterm") {
                    // Fixed booleans only: no terminal text, DOM dump, profile
                    // data, environment or model response enters diagnostics.
                    let mut diagnostics = serde_json::Map::new();
                    for (name, expression) in [
                        ("rowsPresent", "!!document.querySelector('.xterm-rows')"),
                        ("rowsHaveText", "!!document.querySelector('.xterm-rows')?.textContent.trim()"),
                        ("screenVisible", "document.querySelector('.xterm-screen')?.getBoundingClientRect().height > 0"),
                        ("terminalMarkerPainted", "!!document.querySelector('.xterm-rows')?.textContent.includes('owned-terminal:')"),
                        ("utf8Painted", "!!document.querySelector('.xterm-rows')?.textContent.includes('中文-output')"),
                        ("panelOpen", "document.querySelector('.tauri-terminal-drawer')?.hidden === false"),
                        ("documentVisible", "document.visibilityState === 'visible'"),
                        ("documentFocused", "document.hasFocus()"),
                        ("terminalErrorVisible", "!!document.querySelector('.terminal-panel [role=alert]')"),
                    ] {
                        let value = crate::native_edit_smoke::evaluate(app, format!("(() => (({expression}) ? 'edit-ok' : 'edit-pending'))()"));
                        diagnostics.insert(name.into(), value.map(serde_json::Value::Bool).unwrap_or(serde_json::Value::Null));
                    }
                    std::fs::write(cwd.parent().ok_or("terminal diagnostic directory missing")?.join("reasonix-native-terminal-paint-diagnostic.json"), serde_json::Value::Object(diagnostics).to_string()).map_err(|_| "write fixed terminal paint diagnostic")?;
                    return Err(error);
                }
                return Ok(pid);
            }
        }
        if Instant::now() >= deadline {
            return Err("native xterm input did not execute in a real owned TTY".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}
fn gone(pid: u32) -> Result<(), String> {
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        // Signal 0 is read-only; only a PID printed by this owned shell is used.
        let result = std::process::Command::new("/bin/kill")
            .args(["-0", &pid.to_string()])
            .output()
            .map_err(|_| "probe owned terminal PID")?;
        if !result.status.success() {
            return Ok(());
        }
        if Instant::now() >= deadline {
            return Err("owned terminal PID survived cleanup".into());
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}
pub fn run(app: &AppHandle, directory: &Path) -> Result<(), String> {
    check(app, "location.protocol === 'reasonix-preview:' && document.querySelector('.tauri-shell') && document.querySelector('.tauri-sidebar__new')?.disabled === false", "trusted initialized app")?;
    // WKWebView may suspend xterm animation-frame paints while fully occluded.
    // Require the owned native window to be active before asserting rendering,
    // exactly as the existing image-history acceptance does; no clipboard.
    crate::native_edit_smoke::focus(app)?;
    check(
        app,
        "document.visibilityState === 'visible'",
        "visible owned terminal page",
    )?;
    let workspace = directory.join("terminal-workspace 中文");
    std::fs::create_dir(&workspace).map_err(|_| "create private terminal workspace")?;
    let supervisor = app.state::<BridgeSupervisor>();
    for (id, title) in [
        (FIRST, "Terminal acceptance A"),
        (SECOND, "Terminal acceptance B"),
    ] {
        // switch_session also admits the first open, while preserving the
        // bridge's single-controller contract for the second fixture session.
        supervisor
            .switch_session(OpenSessionRequest {
                session_id: id.into(),
                workspace_root: Some(workspace.to_string_lossy().into_owned()),
                model_ref: None,
                effort: None,
            })
            .map_err(|error| format!("seed terminal fixture {id}: {error}"))?;
        supervisor
            .rename_session(RenameSessionRequest {
                session_id: id.into(),
                title: title.into(),
            })
            .map_err(|error| format!("name terminal fixture {id}: {error}"))?;
    }
    check(app, "location.protocol === 'reasonix-preview:' && document.querySelector('.tauri-shell') && document.querySelector('.tauri-sidebar__new')?.disabled === false", "trusted initialized app")?;
    click(
        app,
        ".tauri-sidebar__load-more[aria-label='重新检查会话目录']",
    )?;
    check(app, "[...document.querySelectorAll('.tauri-sidebar__session')].some(node => node.textContent.trim() === 'Terminal acceptance A')", "registered session list")?;
    select(app, "Terminal acceptance A")?;
    click(app, "[aria-label='切换终端面板']")?;
    check(app, "document.querySelector('.terminal-panel__actions .terminal-icon-button')?.disabled === false", "lazy terminal IPC capability")?;
    let before = supervisor.terminal_client()?.workspace(WorkspaceRequest {
        session_id: FIRST.into(),
    })?;
    if !before.workspace.sessions.is_empty() {
        return Err("opening panel executed a shell".into());
    }
    act(app, "const select=document.querySelector('.terminal-shell-select'); const setter=Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value').set; setter.call(select,'sh'); select.dispatchEvent(new Event('change',{bubbles:true}))")?;
    let terminal = new_terminal(app, FIRST)?;
    let first_pid = owned_pid(app, FIRST, &terminal, &workspace)?;
    if !supervisor
        .history(SessionRequest {
            session_id: FIRST.into(),
        })?
        .messages
        .is_empty()
    {
        return Err("terminal output entered model history".into());
    }
    click(app, "[aria-label='切换终端面板']")?;
    check(
        app,
        "document.querySelector('.tauri-terminal-drawer')?.hidden === true",
        "collapse without closing PTY",
    )?;
    click(app, "[aria-label='切换终端面板']")?;
    check(app, &format!("document.querySelector('.xterm-rows')?.textContent.includes('owned-terminal:{first_pid}')"), "reopen without duplicate creation")?;
    let after = supervisor.terminal_client()?.workspace(WorkspaceRequest {
        session_id: FIRST.into(),
    })?;
    if after.workspace.sessions.len() != 1 || after.workspace.sessions[0].id != terminal {
        return Err("reopen replaced the owned PTY".into());
    }
    click(
        app,
        ".terminal-panel__actions .terminal-icon-button:nth-of-type(2)",
    )?;
    check(
        app,
        "document.querySelectorAll('.composer-context__item--selection').length === 1",
        "explicit output-to-chat card",
    )?;
    // The card intentionally shows only a 72-character snippet. Inspect its
    // production tooltip, not the truncated label, for the executed output.
    act(app, "document.querySelector('.composer-context__label').closest('.tooltip-trigger').dispatchEvent(new MouseEvent('mouseover',{bubbles:true,relatedTarget:null}))")?;
    check(app, &format!("[...document.querySelectorAll('[role=tooltip]')].some(node => node.textContent.includes('owned-terminal:{first_pid}') && node.textContent.includes('中文-output') && !node.textContent.includes('\\u001b'))"), "complete sanitized output-to-chat preview")?;
    act(app, "document.querySelector('.composer-context__label').closest('.tooltip-trigger').dispatchEvent(new MouseEvent('mouseout',{bubbles:true,relatedTarget:document.body}))")?;
    if !supervisor
        .history(SessionRequest {
            session_id: FIRST.into(),
        })?
        .messages
        .is_empty()
    {
        return Err("explicit context action submitted a model message".into());
    }
    click(app, ".terminal-session__close")?;
    check(
        app,
        "!document.querySelector('.xterm-helper-textarea')",
        "close visible terminal",
    )?;
    gone(first_pid)?;
    let switch_terminal = new_terminal(app, FIRST)?;
    let switch_pid = owned_pid(app, FIRST, &switch_terminal, &workspace)?;
    select(app, "Terminal acceptance B")?;
    check(app, "!document.querySelector('.tauri-terminal-drawer') && !document.querySelector('.xterm-helper-textarea')", "conversation switch revokes old UI")?;
    gone(switch_pid)?;
    click(app, "[aria-label='切换终端面板']")?;
    check(app, "document.querySelector('.terminal-panel__actions .terminal-icon-button')?.disabled === false", "second conversation terminal capability")?;
    act(app, "const select=document.querySelector('.terminal-shell-select'); select.value='sh'; select.dispatchEvent(new Event('change',{bubbles:true}))")?;
    let shutdown_terminal = new_terminal(app, SECOND)?;
    let shutdown_pid = owned_pid(app, SECOND, &shutdown_terminal, &workspace)?;
    // Normal app shutdown, executed by native_window_smoke after this returns,
    // must clean this third shell. The Python runner verifies its disappearance.
    std::fs::write(
        directory.join("reasonix-native-integrated-terminal-result.json"),
        serde_json::json!({
            "ok": true, "realWebViewIPC": true, "xtermInputEvent": true, "realTTY": true,
            "utf8AnsiPainted": true, "explicitCreateOnly": true, "collapseSamePTY": true,
            "explicitContextOnly": true, "closePIDGone": true, "switchPIDGone": true,
            "shutdownPID": shutdown_pid, "noModelHistory": true
        })
        .to_string(),
    )
    .map_err(|_| "write native terminal receipt")?;
    Ok(())
}
