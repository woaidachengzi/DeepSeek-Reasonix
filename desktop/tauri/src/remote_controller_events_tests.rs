use super::super::tests::{controller, ID};
use super::*;
use std::{io::Read, net::TcpListener, sync::mpsc, thread};

const SELECTED: &str = "/remote/中文 +&.jsonl";
fn owner() -> BridgeRemoteControllerView {
    serde_json::from_value(controller()).unwrap()
}
fn envelope() -> Value {
    json!({"protocolVersion":1,"controller":controller(),"sessionPath":SELECTED,"event":{"kind":"text","sessionPath":SELECTED,"text":"owned"}})
}
fn fixture(
    headers: &str,
    body: Vec<u8>,
) -> (Result<SessionEvents, String>, thread::JoinHandle<()>) {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let address = listener.local_addr().unwrap();
    let headers = headers.to_string();
    let task = thread::spawn(move || {
        let (socket, _) = listener.accept().unwrap();
        socket
            .set_read_timeout(Some(Duration::from_secs(3)))
            .unwrap();
        let mut reader = BufReader::new(socket);
        let mut request = String::new();
        let mut length = 0;
        loop {
            let mut text = String::new();
            reader.read_line(&mut text).unwrap();
            request.push_str(&text);
            if let Some(value) = text.strip_prefix("Content-Length: ") {
                length = value.trim().parse().unwrap();
            }
            if text == "\r\n" {
                break;
            }
        }
        assert!(request.starts_with(&format!(
            "POST /v1/remote/controllers/{ID}/session-events HTTP/1.0\r\n"
        )));
        assert!(request.contains("Authorization: Bearer owned-secret\r\n"));
        assert!(!request.contains("?"));
        let mut bytes = vec![0; length];
        reader.read_exact(&mut bytes).unwrap();
        assert_eq!(
            serde_json::from_slice::<Value>(&bytes).unwrap(),
            json!({"sessionPath":SELECTED})
        );
        let socket = reader.get_mut();
        let _ = socket
            .write_all(headers.as_bytes())
            .and_then(|_| socket.write_all(&body));
    });
    (
        SessionEvents::open(
            address,
            "owned-secret",
            &format!("/v1/remote/controllers/{ID}/session-events"),
            owner(),
            SELECTED.into(),
        ),
        task,
    )
}
const HEADERS: &str = "HTTP/1.0 200 OK\r\nContent-Type: text/event-stream; charset=utf-8\r\n\r\n";

#[test]
fn remote_event_transport_fixed_post_multiline_and_terminal_eof() {
    let data = serde_json::to_string_pretty(&envelope()).unwrap();
    let body = format!(
        ": connected\r\n\r\nid: ignored\r\nretry: 1\r\n{}\r\n",
        data.lines()
            .map(|v| format!("data: {v}\r\n"))
            .collect::<String>()
    );
    let (stream, task) = fixture(HEADERS, body.into_bytes());
    let mut stream = stream.unwrap();
    let result = stream.next().unwrap();
    assert_eq!(result.event["text"], "owned");
    assert_eq!(result.controller.id, ID);
    assert_eq!(stream.next().err().as_deref(), Some(FAILED));
    assert!(stream.cancellation.stopped.load(Ordering::Acquire));
    task.join().unwrap();
}

#[test]
fn remote_event_transport_rejects_http_and_frame_scope() {
    for headers in ["HTTP/1.0 401 PRIVATE\r\n\r\n", "HTTP/1.0 200 OK\r\n\r\n", "HTTP/1.0 200 OK\r\nContent-Type: application/json\r\n\r\n", "HTTP/1.0 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n", "HTTP/1.0 200 OK\r\nContent-Type: text/event-stream\r\nContent-Length: 0\r\n\r\n", "HTTP/1.0 200 OK\r\nContent-Type: text/event-stream\r\nContent-Type: text/event-stream\r\n\r\n"] {
        let (stream, task) = fixture(headers, Vec::new());
        assert_eq!(stream.err().as_deref(), Some(FAILED));
        task.join().unwrap();
    }
    for field in [
        "version",
        "controller",
        "workspace",
        "name",
        "write",
        "path",
        "eventPath",
        "event",
    ] {
        let mut value = envelope();
        match field {
            "version" => value["protocolVersion"] = json!(2),
            "controller" => value["controller"]["id"] = json!("BBBBBBBBBBBBBBBBBBBBBA"),
            "workspace" => value["controller"]["workspace"] = json!("/other"),
            "name" => value["controller"]["name"] = json!("other"),
            "write" => value["controller"]["readOnly"] = json!(false),
            "path" => value["sessionPath"] = json!("/other"),
            "eventPath" => value["event"]["sessionPath"] = json!("/other"),
            _ => value["event"] = json!([]),
        }
        let (stream, task) = fixture(HEADERS, format!("data: {value}\n\n").into_bytes());
        assert_eq!(
            stream.unwrap().next().err().as_deref(),
            Some(FAILED),
            "{field}"
        );
        task.join().unwrap();
    }
}

#[test]
fn remote_event_transport_bounded_lines_comments_and_truncation() {
    for body in [
        b"data: {}".to_vec(),
        b"data: {}\n".to_vec(),
        b"data: \xff\n\n".to_vec(),
        vec![b'a'; FRAME_MAX + 1],
        format!(
            "{}\n{}\n\n",
            ":".repeat(FRAME_MAX / 2),
            ":".repeat(FRAME_MAX / 2)
        )
        .into_bytes(),
    ] {
        let (stream, task) = fixture(HEADERS, body);
        assert_eq!(stream.unwrap().next().err().as_deref(), Some(FAILED));
        task.join().unwrap();
    }
    let client = RemoteControllerClient::new("127.0.0.1:1".parse().unwrap(), "owned".into());
    for path in ["", "bad\n", "bad\0"] {
        assert_eq!(
            client
                .session_events(SessionViewRequest {
                    controller_id: ID.into(),
                    session_path: path.into()
                })
                .err()
                .as_deref(),
            Some(INVALID)
        );
    }
}

#[test]
fn remote_event_transport_requires_native_catalogue_membership() {
    let (client, task) = super::super::tests::fixture(json!({
        "protocolVersion":1, "controller":controller(), "sessions":[]
    }));
    assert_eq!(
        client
            .session_events(SessionViewRequest {
                controller_id: ID.into(),
                session_path: SELECTED.into()
            })
            .err()
            .as_deref(),
        Some(FAILED)
    );
    let (headers, body) = task.join().unwrap();
    assert!(headers.starts_with(&format!(
        "GET /v1/remote/controllers/{ID}/sessions HTTP/1.1"
    )));
    assert_eq!(body, Value::Null);
}

#[test]
fn remote_event_transport_cancellation_unblocks_reader_and_fences_buffer() {
    let (stream, task) = fixture(HEADERS, format!("data: {}\n\n", envelope()).into_bytes());
    let mut stream = stream.unwrap();
    stream.cancellation().close();
    assert_eq!(stream.next().err().as_deref(), Some(FAILED));
    task.join().unwrap();

    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let address = listener.local_addr().unwrap();
    let (ready_tx, ready_rx) = mpsc::channel();
    let task = thread::spawn(move || {
        let (socket, _) = listener.accept().unwrap();
        socket
            .set_read_timeout(Some(Duration::from_secs(3)))
            .unwrap();
        let mut reader = BufReader::new(socket);
        let mut length = 0usize;
        loop {
            let mut header = String::new();
            assert!(reader.read_line(&mut header).unwrap() > 0);
            if let Some(value) = header.strip_prefix("Content-Length: ") {
                length = value.trim().parse().unwrap();
            }
            if header == "\r\n" {
                break;
            }
        }
        assert!(length < 4096);
        reader.read_exact(&mut vec![0; length]).unwrap();
        let mut socket = reader.into_inner();
        socket.write_all(HEADERS.as_bytes()).unwrap();
        ready_tx.send(()).unwrap();
        // Read until the native cancellation shuts down its own socket.
        socket
            .set_read_timeout(Some(Duration::from_secs(3)))
            .unwrap();
        while socket.read(&mut [0; 4096]).unwrap() != 0 {}
    });
    let mut stream = SessionEvents::open(
        address,
        "owned",
        &format!("/v1/remote/controllers/{ID}/session-events"),
        owner(),
        SELECTED.into(),
    )
    .unwrap();
    ready_rx.recv_timeout(Duration::from_secs(3)).unwrap();
    let cancel = stream.cancellation();
    let (done_tx, done_rx) = mpsc::channel();
    let reader = thread::spawn(move || {
        done_tx.send(stream.next().err()).unwrap();
    });
    cancel.close();
    assert_eq!(
        done_rx
            .recv_timeout(Duration::from_secs(2))
            .unwrap()
            .as_deref(),
        Some(FAILED)
    );
    reader.join().unwrap();
    task.join().unwrap();
}

#[test]
fn remote_event_open_cancellation_covers_catalogue_and_sse_headers() {
    for phase in ["catalogue", "catalogue-body", "sse"] {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let address = listener.local_addr().unwrap();
        let (waiting_tx, waiting_rx) = mpsc::channel();
        let server = thread::spawn(move || {
            for current in ["catalogue", "sse"] {
                let (socket, _) = listener.accept().unwrap();
                socket
                    .set_read_timeout(Some(Duration::from_secs(3)))
                    .unwrap();
                let mut reader = BufReader::new(socket);
                let mut headers = String::new();
                let mut length = 0usize;
                loop {
                    let mut header = String::new();
                    assert!(reader.read_line(&mut header).unwrap() > 0);
                    headers.push_str(&header);
                    if let Some(value) = header.strip_prefix("Content-Length: ") {
                        length = value.trim().parse().unwrap();
                    }
                    if header == "\r\n" {
                        break;
                    }
                }
                assert!(length < 4096);
                reader.read_exact(&mut vec![0; length]).unwrap();
                assert!(headers.starts_with(if current == "catalogue" {
                    "GET "
                } else {
                    "POST "
                }));
                if current == phase || (current == "catalogue" && phase == "catalogue-body") {
                    if phase == "catalogue-body" {
                        reader.get_mut().write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 1048576\r\nConnection: close\r\n\r\n{").unwrap();
                    }
                    waiting_tx.send(()).unwrap();
                    // Withhold HTTP headers or leave a JSON body unfinished.
                    let result = reader.read(&mut [0; 1]);
                    assert!(
                        matches!(result, Ok(0))
                            || result.is_err_and(|err| matches!(
                                err.kind(),
                                std::io::ErrorKind::ConnectionReset
                                    | std::io::ErrorKind::ConnectionAborted
                            ))
                    );
                    return;
                }
                let catalogue = json!({"protocolVersion":1,"controller":controller(),"sessions":[{
                    "name":"owned","path":SELECTED,"title":"owned","turns":1,
                    "current":false,"running":false,"takenOver":false,"mtimeMilli":1
                }]})
                .to_string();
                write!(
                    reader.get_mut(),
                    "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    catalogue.len(),
                    catalogue
                )
                .unwrap();
            }
        });
        let cancellation = EventCancellation::default();
        let worker_cancel = cancellation.clone();
        let (done_tx, done_rx) = mpsc::channel();
        let worker = thread::spawn(move || {
            let client = RemoteControllerClient::new(address, "owned".into());
            let result = client.session_events_cancellable(
                SessionViewRequest {
                    controller_id: ID.into(),
                    session_path: SELECTED.into(),
                },
                worker_cancel,
            );
            done_tx.send(result.err()).unwrap();
        });
        waiting_rx.recv_timeout(Duration::from_secs(3)).unwrap();
        cancellation.close();
        assert_eq!(
            done_rx
                .recv_timeout(Duration::from_secs(2))
                .unwrap()
                .as_deref(),
            Some(FAILED),
            "{phase}"
        );
        worker.join().unwrap();
        server.join().unwrap();
        assert!(cancellation.socket.lock().unwrap().is_none());
    }
}

#[test]
fn remote_event_open_precancel_zero_dispatch_and_failed_open_releases_socket() {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    listener.set_nonblocking(true).unwrap();
    let client = RemoteControllerClient::new(listener.local_addr().unwrap(), "owned".into());
    let cancellation = EventCancellation::default();
    cancellation.close();
    assert_eq!(
        client
            .session_events_cancellable(
                SessionViewRequest {
                    controller_id: ID.into(),
                    session_path: SELECTED.into()
                },
                cancellation.clone()
            )
            .err()
            .as_deref(),
        Some(FAILED)
    );
    assert_eq!(
        listener.accept().err().unwrap().kind(),
        std::io::ErrorKind::WouldBlock
    );
    assert!(cancellation.socket.lock().unwrap().is_none());

    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let address = listener.local_addr().unwrap();
    let task = thread::spawn(move || {
        let (mut socket, _) = listener.accept().unwrap();
        socket
            .set_read_timeout(Some(Duration::from_secs(3)))
            .unwrap();
        let size = socket.read(&mut [0; 4096]).unwrap();
        assert!(size > 0);
        socket.write_all(b"HTTP/1.0 403 PRIVATE\r\n\r\n").unwrap();
    });
    let cancellation = EventCancellation::default();
    assert_eq!(
        SessionEvents::open_cancellable(
            address,
            "owned",
            &format!("/v1/remote/controllers/{ID}/session-events"),
            owner(),
            SELECTED.into(),
            cancellation.clone()
        )
        .err()
        .as_deref(),
        Some(FAILED)
    );
    task.join().unwrap();
    assert!(cancellation.socket.lock().unwrap().is_none());
}
