use super::*;
use std::{
    io::{BufRead, BufReader, Read, Write},
    net::{TcpListener, TcpStream},
    sync::mpsc,
    thread,
    time::Instant,
};

const ID: &str = "0123456789abcdef0123456789abcdef";

fn accept(listener: &TcpListener) -> TcpStream {
    listener.set_nonblocking(true).unwrap();
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        match listener.accept() {
            Ok((stream, _)) => {
                stream.set_nonblocking(false).unwrap();
                stream
                    .set_read_timeout(Some(Duration::from_secs(2)))
                    .unwrap();
                return stream;
            }
            Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                assert!(
                    Instant::now() < deadline,
                    "owned fixture timed out waiting for a request"
                );
                thread::sleep(Duration::from_millis(5));
            }
            Err(_) => panic!("owned fixture accept failed"),
        }
    }
}

fn read_request(stream: TcpStream) -> (TcpStream, String, Value) {
    let mut reader = BufReader::new(stream);
    let mut headers = String::new();
    loop {
        let mut line = String::new();
        reader.read_line(&mut line).unwrap();
        if line == "\r\n" {
            break;
        }
        assert!(!line.is_empty() && headers.len() + line.len() < 16 << 10);
        headers.push_str(&line);
    }
    let length: usize = headers
        .lines()
        .find_map(|line| line.strip_prefix("Content-Length: "))
        .unwrap()
        .parse()
        .unwrap();
    assert!(length <= 96 << 10);
    let mut body = vec![0; length];
    reader.read_exact(&mut body).unwrap();
    let body = if body.is_empty() {
        Value::Null
    } else {
        serde_json::from_slice(&body).unwrap()
    };
    (reader.into_inner(), headers, body)
}
fn respond(mut stream: TcpStream, value: Value) {
    let text = value.to_string();
    write!(
        stream,
        "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
        text.len(),
        text
    )
    .unwrap();
}
fn session() -> Value {
    json!({"id":ID,"title":"sh","shell":"sh","cwd":"/owned","createdAt":1,"running":true})
}

#[test]
fn terminal_ipc_rejects_unknown_commands_header_injection_and_secondary_window() {
    assert!(ensure_main_window("main").is_ok());
    for label in ["settings", "", "main-2"] {
        assert!(ensure_main_window(label).is_err());
    }
    for id in ["", "owned/../other", "owned\r\nHeader: injected", " owned "] {
        assert!(terminal_id(id).is_err());
    }
    for id in ["", "request\r\nHeader: injected", "a/b", &"x".repeat(129)] {
        assert!(request_id(id).is_err());
    }
    for extra in ["program", "args", "env", "workspaceRoot", "command"] {
        let mut request =
            json!({"sessionId":"owned","requestId":"opaque-request","path":".","shellId":"sh"});
        request[extra] = json!("forbidden");
        assert!(serde_json::from_value::<CreateRequest>(request).is_err());
    }
    assert!(serde_json::from_value::<InputRequest>(
        json!({"sessionId":"owned","terminalId":ID,"data":"YQ=="})
    )
    .is_err());
    let client = TerminalClient::new("127.0.0.1:1".parse().unwrap(), "fixture-only".into());
    for data in [
        "",
        "YR==",
        "YQ==\n",
        &STANDARD.encode(vec![0; MAX_INPUT + 1]),
    ] {
        assert!(client
            .input(InputRequest {
                session_id: "owned".into(),
                terminal_id: ID.into(),
                request_id: "opaque".into(),
                data: data.into()
            })
            .unwrap_err()
            .contains("invalid terminal request"));
    }
    assert!(client
        .resize(ResizeRequest {
            session_id: "owned".into(),
            terminal_id: ID.into(),
            request_id: "opaque".into(),
            cols: 1001,
            rows: 24
        })
        .is_err());
    assert!(client
        .rename(RenameRequest {
            session_id: "owned".into(),
            terminal_id: ID.into(),
            request_id: "opaque".into(),
            title: "private\nlabel".into()
        })
        .is_err());
    assert!(client
        .create(CreateRequest {
            session_id: "owned".into(),
            request_id: "opaque".into(),
            path: ".".into(),
            shell_id: "/bin/sh -c anything".into()
        })
        .is_err());
}

#[test]
fn terminal_output_checks_bytes_offsets_budgets_and_signed_exit() {
    let bytes = [0, 0xff, 0x1b, b'[', b'3', b'1', b'm', 0xe4];
    let good = BridgeTerminalOutputView {
        id: ID.into(),
        data: STANDARD.encode(bytes),
        start: 123,
        end: 131,
    };
    assert!(validate_output(&good, MAX_EVENT).is_ok());
    for broken in [
        BridgeTerminalOutputView {
            end: 132,
            ..good.clone()
        },
        BridgeTerminalOutputView {
            start: 132,
            ..good.clone()
        },
        BridgeTerminalOutputView {
            data: "private-not-base64".into(),
            ..good.clone()
        },
        BridgeTerminalOutputView {
            end: MAX_JS_INTEGER + 1,
            ..good.clone()
        },
    ] {
        let error = validate_output(&broken, MAX_EVENT).unwrap_err();
        assert_eq!(error, BAD_RESPONSE);
    }
    assert!(validate_output(
        &BridgeTerminalOutputView {
            id: ID.into(),
            data: STANDARD.encode(vec![0; MAX_EVENT + 1]),
            start: 0,
            end: (MAX_EVENT + 1) as u64
        },
        MAX_EVENT
    )
    .is_err());
    for code in [-1_i64, 7, 4_294_967_295] {
        let event: BridgeEvent = serde_json::from_value(json!({"protocolVersion":1,"sequence":2,"sessionId":"owned","eventKind":"terminal_exit","payload":{"id":ID,"exitCode":code,"removed":false,"kind":"text","text":"not an assistant answer"}})).unwrap();
        let clean = terminal_event(&event).unwrap().unwrap();
        assert_eq!(clean.payload["exitCode"], code);
        assert!(clean.payload.get("text").is_none());
    }
    let mut event: BridgeEvent = serde_json::from_value(json!({"protocolVersion":1,"sequence":3,"sessionId":"owned","eventKind":"terminal_output","payload":good})).unwrap();
    assert!(terminal_event(&event).unwrap().is_ok());
    event.payload["end"] = json!(132);
    assert!(terminal_event(&event).unwrap().is_err());
    event.event_kind = "text".into();
    assert!(terminal_event(&event).is_none());
}

#[test]
fn terminal_http_retry_keeps_exact_opaque_identity_and_binary_payload() {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let address = listener.local_addr().unwrap();
    let (tx, rx) = mpsc::channel();
    let server = thread::spawn(move || {
        let mut captured = Vec::new();
        for attempt in 0..2 {
            let (stream, headers, body) = read_request(accept(&listener));
            captured.push((headers, body));
            if attempt == 0 {
                drop(stream);
            } else {
                respond(
                    stream,
                    json!({"protocolVersion":1,"terminalId":ID,"accepted":true}),
                );
            }
        }
        tx.send(captured).unwrap();
    });
    let client = TerminalClient::new(address, "owned-private-fixture-token".into());
    let bytes = [0, 0xff, 0x1b, b'[', b'0', b'm', 0xe4, 0xb8];
    let response = client
        .input(InputRequest {
            session_id: "owned".into(),
            terminal_id: ID.into(),
            request_id: "input-opaque-123".into(),
            data: STANDARD.encode(bytes),
        })
        .unwrap();
    assert!(response.accepted);
    let requests = rx.recv_timeout(Duration::from_secs(5)).unwrap();
    assert_eq!(requests[0], requests[1]);
    assert!(requests[0].0.starts_with(&format!(
        "POST /v1/sessions/owned/terminal/{ID}/input HTTP/1.1"
    )));
    assert!(requests[0]
        .0
        .contains("Authorization: Bearer owned-private-fixture-token\r\n"));
    assert!(requests[0]
        .0
        .contains("X-Reasonix-Request-ID: input-opaque-123\r\n"));
    assert_eq!(
        STANDARD
            .decode(requests[0].1["data"].as_str().unwrap())
            .unwrap(),
        bytes
    );
    assert_eq!(requests[0].1.as_object().unwrap().len(), 1);
    server.join().unwrap();
}

#[test]
fn terminal_workspace_and_snapshot_responses_fail_closed() {
    for response in [
        json!({"protocolVersion":99,"workspace":{"available":true,"readOnly":false,"sessions":[],"shells":[]}}),
        json!({"protocolVersion":1,"workspace":{"available":true,"readOnly":false,"sessions":[session(),session()],"shells":[]}}),
        json!({"protocolVersion":1,"workspace":{"available":true,"readOnly":false,"sessions":[],"shells":[{"id":"/bin/sh","label":"not a stable ID"}]}}),
    ] {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let address = listener.local_addr().unwrap();
        let server = thread::spawn(move || {
            let (stream, _, _) = read_request(accept(&listener));
            respond(stream, response);
        });
        assert!(TerminalClient::new(address, "fixture".into())
            .workspace(WorkspaceRequest {
                session_id: "owned".into()
            })
            .is_err());
        server.join().unwrap();
    }
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let address = listener.local_addr().unwrap();
    let server = thread::spawn(move || {
        let (stream, _, _) = read_request(accept(&listener));
        respond(
            stream,
            json!({"protocolVersion":1,"output":{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","data":"YQ==","start":0,"end":1}}),
        );
    });
    assert!(TerminalClient::new(address, "fixture".into())
        .output(TargetRequest {
            session_id: "owned".into(),
            terminal_id: ID.into()
        })
        .is_err());
    server.join().unwrap();
}

#[test]
fn terminal_http_errors_carry_only_safe_recovery_codes() {
    for (status, code) in [
        (400, "terminal_invalid_request"),
        (404, "terminal_not_found"),
        (409, "terminal_busy"),
        (503, "terminal_unavailable"),
    ] {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let address = listener.local_addr().unwrap();
        let server = thread::spawn(move || {
            let (mut stream, _, _) = read_request(accept(&listener));
            let body = json!({"protocolVersion":1,"error":{"code":code,"message":"private-terminal-input-and-path-do-not-forward"}}).to_string();
            write!(
                stream,
                "HTTP/1.1 {status} Error\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                body.len()
            )
            .unwrap();
        });
        let error = request_json_with_timeout(
            address,
            "fixture-only",
            "GET",
            "/owned",
            None,
            None,
            Duration::from_secs(2),
        )
        .unwrap_err();
        assert!(error.contains(code));
        assert!(!error.contains("private-terminal"));
        server.join().unwrap();
    }
}

#[cfg(unix)]
struct OwnedBridge {
    child: std::process::Child,
    client: Option<TerminalClient>,
}
#[cfg(unix)]
impl Drop for OwnedBridge {
    fn drop(&mut self) {
        if let Some(client) = &self.client {
            let _ = client.request::<Value>("POST", "/v1:shutdown", None, None);
        }
        let deadline = Instant::now() + Duration::from_secs(4);
        while Instant::now() < deadline {
            if self.child.try_wait().ok().flatten().is_some() {
                return;
            }
            thread::sleep(Duration::from_millis(10));
        }
        // Exact child created by this fixture; never a process-name/PID sweep.
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

#[cfg(unix)]
#[test]
#[ignore = "requires REASONIX_TERMINAL_BRIDGE_TEST_BIN pointing at a newly built isolated sidecar"]
fn actual_bridge_pty_end_to_end() {
    use std::{
        fs,
        process::{Command, Stdio},
    };
    let binary = std::env::var_os("REASONIX_TERMINAL_BRIDGE_TEST_BIN")
        .expect("explicit test sidecar required");
    let profile = tempfile::tempdir().unwrap();
    let home = profile.path().join("home");
    let project = profile.path().join("project");
    fs::create_dir(&home).unwrap();
    fs::create_dir(&project).unwrap();
    fs::write(profile.path().join("config.toml"), "default_model = \"fixture/ordinary\"\n[[providers]]\nname=\"fixture\"\nkind=\"openai\"\nbase_url=\"http://127.0.0.1:1/v1\"\nmodels=[\"ordinary\",\"other\"]\ndefault=\"ordinary\"\nno_proxy=true\n").unwrap();
    let ready = profile.path().join("ready.json");
    let token = "terminal-fixture-private-token-only-123456";
    let mut child = Command::new(binary)
        .args(["--ready-file"])
        .arg(&ready)
        .args(["--launch-id", "terminal-owned-launch"])
        .env_clear()
        .env("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")
        .env("HOME", &home)
        .env("REASONIX_HOME", profile.path())
        .env("REASONIX_STATE_HOME", profile.path())
        .env("REASONIX_CACHE_HOME", profile.path().join("cache"))
        .env("REASONIX_CREDENTIALS_STORE", "file")
        .env("SHELL", "/bin/sh")
        .env("ENV", "")
        .env("BASH_ENV", "")
        .env("ZDOTDIR", &home)
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .expect("spawn owned sidecar");
    writeln!(child.stdin.take().unwrap(), "{token}").unwrap();
    let mut bridge = OwnedBridge {
        child,
        client: None,
    };
    let deadline = Instant::now() + Duration::from_secs(8);
    let address = loop {
        if let Ok(bytes) = fs::read(&ready) {
            let ready: Value = serde_json::from_slice(&bytes).unwrap();
            assert_eq!(ready["launchId"], "terminal-owned-launch");
            assert_eq!(ready["protocolVersion"], 1);
            let address: SocketAddr = ready["address"].as_str().unwrap().parse().unwrap();
            assert!(address.ip().is_loopback());
            break address;
        }
        assert!(
            bridge.child.try_wait().unwrap().is_none(),
            "owned sidecar exited before readiness"
        );
        assert!(Instant::now() < deadline, "owned sidecar startup timed out");
        thread::sleep(Duration::from_millis(10));
    };
    let client = TerminalClient::new(address, token.into());
    bridge.client = Some(client.clone());
    client
        .request::<Value>(
            "POST",
            "/v1/sessions:open",
            Some(json!({"sessionId":"rust-terminal-owned","workspaceRoot":project})),
            Some("open-owned"),
        )
        .unwrap();
    let create = CreateRequest {
        session_id: "rust-terminal-owned".into(),
        request_id: "create-owned".into(),
        path: ".".into(),
        shell_id: "sh".into(),
    };
    let created = client.create(create.clone()).unwrap();
    let id = created.terminal.id;
    assert_eq!(
        client.create(create).unwrap().terminal.id,
        id,
        "create retry duplicated a PTY"
    );
    client
        .resize(ResizeRequest {
            session_id: "rust-terminal-owned".into(),
            terminal_id: id.clone(),
            request_id: "resize-owned".into(),
            cols: 100,
            rows: 30,
        })
        .unwrap();
    let input = InputRequest{session_id:"rust-terminal-owned".into(),terminal_id:id.clone(),request_id:"input-owned".into(),data:STANDARD.encode(b"stty -echo\nprintf '%s\\n' 'rust-''pty-accepted'\nprintf 'owned-shell:%s\\n' \"$$\"\nstty size\n")};
    client.input(input.clone()).unwrap();
    client.input(input).unwrap();
    let read = || {
        client
            .output(TargetRequest {
                session_id: "rust-terminal-owned".into(),
                terminal_id: id.clone(),
            })
            .unwrap()
            .output
    };
    let deadline = Instant::now() + Duration::from_secs(5);
    let initial = loop {
        let snapshot = read();
        let output =
            String::from_utf8_lossy(&STANDARD.decode(&snapshot.data).unwrap()).into_owned();
        if output.contains("rust-pty-accepted") && output.contains("30 100") {
            assert_eq!(
                output.matches("rust-pty-accepted").count(),
                1,
                "input replay executed twice"
            );
            break output;
        }
        assert!(
            Instant::now() < deadline,
            "actual PTY execution/geometry missing"
        );
        thread::sleep(Duration::from_millis(10));
    };
    let pid = initial
        .split("owned-shell:")
        .find_map(|fragment| {
            let digits: String = fragment.chars().take_while(char::is_ascii_digit).collect();
            (!digits.is_empty()).then_some(digits)
        })
        .expect("PID printed by the owned shell");
    assert!(pid.parse::<u32>().unwrap() > 1);
    client
        .rename(RenameRequest {
            session_id: "rust-terminal-owned".into(),
            terminal_id: id.clone(),
            request_id: "rename-owned".into(),
            title: "保留终端".into(),
        })
        .unwrap();
    client
        .request::<Value>(
            "POST",
            "/v1/sessions/rust-terminal-owned/model",
            Some(json!({"model":"fixture/other"})),
            Some("model-owned"),
        )
        .unwrap();
    client
        .input(InputRequest {
            session_id: "rust-terminal-owned".into(),
            terminal_id: id.clone(),
            request_id: "after-model-owned".into(),
            data: STANDARD.encode(b"printf 'retained-shell:%s\\n' \"$$\"\n"),
        })
        .unwrap();
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        let bytes = STANDARD.decode(read().data).unwrap();
        let output = String::from_utf8_lossy(&bytes);
        if output.contains(&format!("retained-shell:{pid}")) {
            break;
        }
        assert!(
            Instant::now() < deadline,
            "model replacement lost actual PTY"
        );
        thread::sleep(Duration::from_millis(10));
    }
    let workspace = client
        .workspace(WorkspaceRequest {
            session_id: "rust-terminal-owned".into(),
        })
        .unwrap();
    assert_eq!(workspace.workspace.sessions.len(), 1);
    assert_eq!(workspace.workspace.sessions[0].title, "保留终端");
    let history: Value = client
        .request(
            "GET",
            "/v1/sessions/rust-terminal-owned/history",
            None,
            None,
        )
        .unwrap();
    assert!(
        history["messages"].as_array().unwrap().is_empty(),
        "terminal output leaked into model history"
    );
    client
        .close(CloseRequest {
            session_id: "rust-terminal-owned".into(),
            terminal_id: id.clone(),
            request_id: "close-owned".into(),
        })
        .unwrap();
    assert!(client
        .output(TargetRequest {
            session_id: "rust-terminal-owned".into(),
            terminal_id: id
        })
        .is_err());
    // Read-only probe of ONLY the PID printed by our own shell, never kill it.
    let deadline = Instant::now() + Duration::from_secs(3);
    while Command::new("/bin/kill")
        .args(["-0", &pid])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .unwrap()
        .success()
    {
        assert!(
            Instant::now() < deadline,
            "owned terminal process survived Close"
        );
        thread::sleep(Duration::from_millis(10));
    }

    // Terminate only a second fixture shell from inside that shell. Unix Wait
    // reports -1 for signal termination; the actual Go→Rust response must not
    // fail deserialization as the old unsigned exit-code mirror did.
    let signalled = client
        .create(CreateRequest {
            session_id: "rust-terminal-owned".into(),
            request_id: "create-signal-owned".into(),
            path: ".".into(),
            shell_id: "sh".into(),
        })
        .unwrap()
        .terminal
        .id;
    client
        .input(InputRequest {
            session_id: "rust-terminal-owned".into(),
            terminal_id: signalled.clone(),
            request_id: "signal-self-owned".into(),
            data: STANDARD.encode(b"kill -KILL $$\n"),
        })
        .unwrap();
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        let workspace = client
            .workspace(WorkspaceRequest {
                session_id: "rust-terminal-owned".into(),
            })
            .unwrap();
        let shell = workspace
            .workspace
            .sessions
            .iter()
            .find(|terminal| terminal.id == signalled)
            .unwrap();
        if !shell.running {
            assert_eq!(shell.exit_code, Some(-1));
            break;
        }
        assert!(
            Instant::now() < deadline,
            "owned self-terminated shell did not report its signed exit code"
        );
        thread::sleep(Duration::from_millis(10));
    }
    client
        .close(CloseRequest {
            session_id: "rust-terminal-owned".into(),
            terminal_id: signalled,
            request_id: "close-signal-owned".into(),
        })
        .unwrap();
}
