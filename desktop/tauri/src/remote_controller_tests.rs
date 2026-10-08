use super::*;
use std::{
    io::{BufRead, BufReader, Read, Write},
    net::TcpListener,
    thread,
};

const ID: &str = "AAAAAAAAAAAAAAAAAAAAAA";
fn controller() -> Value {
    json!({"id":ID,"name":"owned","workspace":"/remote/actual","readOnly":true})
}
fn row() -> Value {
    json!({"name":"a","path":"/remote/a.jsonl","title":"中文\n第二行","turns":2,"current":true,"running":false,"takenOver":false,"mtimeMilli":1})
}
fn fixture(response: Value) -> (RemoteControllerClient, thread::JoinHandle<(String, Value)>) {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let address = listener.local_addr().unwrap();
    let task = thread::spawn(move || {
        let (stream, _) = listener.accept().unwrap();
        stream
            .set_read_timeout(Some(Duration::from_secs(3)))
            .unwrap();
        let mut reader = BufReader::new(stream);
        let mut headers = String::new();
        loop {
            let mut line = String::new();
            reader.read_line(&mut line).unwrap();
            assert!(!line.is_empty());
            if line == "\r\n" {
                break;
            }
            headers.push_str(&line);
            assert!(headers.len() < 16384);
        }
        let size: usize = headers
            .lines()
            .find_map(|v| v.strip_prefix("Content-Length: "))
            .unwrap()
            .parse()
            .unwrap();
        assert!(size <= 12288);
        let mut body = vec![0; size];
        reader.read_exact(&mut body).unwrap();
        let value = if body.is_empty() {
            Value::Null
        } else {
            serde_json::from_slice(&body).unwrap()
        };
        let text = response.to_string();
        write!(
            reader.into_inner(),
            "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            text.len(),
            text
        )
        .unwrap();
        (headers, value)
    });
    (
        RemoteControllerClient::new(address, "owned-bridge-auth".into()),
        task,
    )
}
#[test]
fn remote_ipc_rejects_extra_fields_paths_and_secondary_windows() {
    assert!(ensure_main_window("main").is_ok());
    for label in ["settings", "", "main-2"] {
        assert!(ensure_main_window(label).is_err());
    }
    for extra in [
        "url",
        "token",
        "sessionPath",
        "action",
        "command",
        "workspaceRoot",
    ] {
        let mut input = json!({"name":"owned","workspace":"/remote"});
        input[extra] = json!("forbidden");
        assert!(serde_json::from_value::<AttachRequest>(input).is_err());
    }
    let client = RemoteControllerClient::new("127.0.0.1:1".parse().unwrap(), "owned".into());
    for id in [
        "",
        "a/../b",
        "AAAAAAAAAAAAAAAAAAAAAB",
        "AAAAAAAAAAAAAAAAAAAAAA?secret",
        "a\r\nInjected: yes",
    ] {
        assert_eq!(
            client
                .sessions(HandleRequest {
                    controller_id: id.into()
                })
                .unwrap_err(),
            INVALID
        );
        assert_eq!(
            client
                .close(HandleRequest {
                    controller_id: id.into()
                })
                .unwrap_err(),
            INVALID
        );
    }
    for (name, workspace) in [
        ("_bad", "/p"),
        ("owned", "\n"),
        ("owned", "/p\x1b"),
        ("a.b", "/p"),
    ] {
        assert_eq!(
            client
                .attach(AttachRequest {
                    name: name.into(),
                    workspace: workspace.into()
                })
                .unwrap_err(),
            INVALID
        );
    }
}
#[test]
fn remote_ipc_uses_only_fixed_routes_and_safe_dtos() {
    let (client, task) =
        fixture(json!({"protocolVersion":1,"controller":controller(),"token":"private-unknown"}));
    let response = client
        .attach(AttachRequest {
            name: "owned".into(),
            workspace: "/remote/requested".into(),
        })
        .unwrap();
    let (headers, body) = task.join().unwrap();
    assert!(headers.starts_with("POST /v1/remote/controllers HTTP/1.1"));
    assert!(headers.contains("Authorization: Bearer owned-bridge-auth"));
    assert_eq!(
        body,
        json!({"name":"owned","workspace":"/remote/requested"})
    );
    assert!(!serde_json::to_string(&response)
        .unwrap()
        .contains("private-unknown"));
    let (client, task) =
        fixture(json!({"protocolVersion":1,"controller":controller(),"sessions":[row()]}));
    let response = client
        .sessions(HandleRequest {
            controller_id: ID.into(),
        })
        .unwrap();
    assert_eq!(response.sessions[0].title, "中文\n第二行");
    let (headers, body) = task.join().unwrap();
    assert!(headers.starts_with(&format!(
        "GET /v1/remote/controllers/{ID}/sessions HTTP/1.1"
    )));
    assert!(body.is_null());
    let (client, task) = fixture(json!({"protocolVersion":1,"closed":true}));
    client
        .close(HandleRequest {
            controller_id: ID.into(),
        })
        .unwrap();
    assert!(task
        .join()
        .unwrap()
        .0
        .starts_with(&format!("DELETE /v1/remote/controllers/{ID} HTTP/1.1")));
}
#[test]
fn remote_ipc_rejects_wrong_scope_version_and_catalogue() {
    for kind in [
        "version",
        "handle",
        "write",
        "duplicate",
        "current",
        "count",
        "escape",
    ] {
        let mut value = json!({"protocolVersion":1,"controller":controller(),"sessions":[row()]});
        match kind {
            "version" => value["protocolVersion"] = json!(2),
            "handle" => value["controller"]["id"] = json!("BBBBBBBBBBBBBBBBBBBBBA"),
            "write" => value["controller"]["readOnly"] = json!(false),
            "duplicate" => value["sessions"].as_array_mut().unwrap().push(row()),
            "current" => {
                let mut other = row();
                other["path"] = json!("/remote/b");
                value["sessions"].as_array_mut().unwrap().push(other);
            }
            "count" => value["sessions"][0]["turns"] = json!(MAX_JS + 1),
            "escape" => value["sessions"][0]["title"] = json!("private-escape\u{1b}"),
            _ => unreachable!(),
        }
        let (client, task) = fixture(value);
        assert_eq!(
            client
                .sessions(HandleRequest {
                    controller_id: ID.into()
                })
                .unwrap_err(),
            FAILED
        );
        task.join().unwrap();
    }
}
