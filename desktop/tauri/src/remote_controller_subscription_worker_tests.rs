use super::super::tests::{controller, ID};
use super::*;
use std::{
    io::{BufRead, BufReader, Read, Write},
    net::{SocketAddr, TcpListener},
    sync::{mpsc, Arc, Mutex},
    thread,
    time::Duration,
};

const SELECTED: &str = "/remote/selected.jsonl";
fn fixture(
    body: &'static str,
    wait: bool,
) -> (SocketAddr, thread::JoinHandle<()>, mpsc::Receiver<()>) {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let address = listener.local_addr().unwrap();
    let (sent_tx, sent_rx) = mpsc::channel();
    let task = thread::spawn(move || {
        for event_stream in [false, true] {
            let (socket, _) = listener.accept().unwrap();
            socket
                .set_read_timeout(Some(Duration::from_secs(3)))
                .unwrap();
            let mut reader = BufReader::new(socket);
            let mut length = 0usize;
            loop {
                let mut line = String::new();
                assert!(reader.read_line(&mut line).unwrap() > 0);
                if let Some(value) = line.strip_prefix("Content-Length: ") {
                    length = value.trim().parse().unwrap();
                }
                if line == "\r\n" {
                    break;
                }
            }
            assert!(length < 4096);
            reader.read_exact(&mut vec![0; length]).unwrap();
            if !event_stream {
                let catalogue = json!({"protocolVersion":1,"controller":controller(),"sessions":[{
                    "path":SELECTED,"name":"owned","title":"owned","current":false,"running":false,
                    "takenOver":false,"turns":1,"mtimeMilli":1
                }]})
                .to_string();
                write!(
                    reader.get_mut(),
                    "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    catalogue.len(),
                    catalogue
                )
                .unwrap();
            } else {
                let frame = json!({"protocolVersion":1,"controller":controller(),"sessionPath":SELECTED,"event":{
                    "kind":"text","text":body,"sessionPath":SELECTED,"seq":1,"turnId":"owned-turn","apiKey":"PRIVATE"
                }});
                write!(
                    reader.get_mut(),
                    "HTTP/1.0 200 OK\r\nContent-Type: text/event-stream\r\n\r\ndata: {frame}\n\n"
                )
                .unwrap();
                sent_tx.send(()).unwrap();
                if wait {
                    let closed = reader.read(&mut [0; 1]);
                    assert!(
                        matches!(closed, Ok(0))
                            || closed.is_err_and(|error| matches!(
                                error.kind(),
                                std::io::ErrorKind::ConnectionReset
                                    | std::io::ErrorKind::ConnectionAborted
                            ))
                    );
                }
            }
        }
    });
    (address, task, sent_rx)
}
fn operation(registry: &RemoteSubscriptions, generation: u64) -> SubscriptionOperation {
    registry
        .reserve(
            "main",
            "owned-sidecar",
            SubscribeRequest {
                controller_id: ID.into(),
                session_path: SELECTED.into(),
                surface_id: "owned-surface".into(),
                generation,
            },
        )
        .unwrap()
}
fn registry() -> RemoteSubscriptions {
    let registry = RemoteSubscriptions::default();
    registry.set_owner(Some("owned-sidecar"));
    registry
}

#[test]
fn native_subscription_worker_ready_before_projected_events_and_terminal_cleanup() {
    let (address, server, _sent) = fixture("owned answer", false);
    let registry = registry();
    let operation = operation(&registry, 1);
    let expected = operation.identity().subscription_id;
    let values = Mutex::new(Vec::new());
    run_with(
        RemoteControllerClient::new(address, "owned".into()),
        operation,
        |identity, state| {
            assert_eq!(identity.subscription_id, expected);
            values.lock().unwrap().push(state.to_string());
            Ok(())
        },
        |identity, frame| {
            assert_eq!(identity.subscription_id, expected);
            let value = serde_json::to_string(&SessionEvent {
                protocol_version: 1,
                subscription: identity,
                frame,
            })
            .unwrap();
            assert!(!value.contains("PRIVATE"));
            assert_eq!(frame.event["text"], "owned answer");
            values.lock().unwrap().push("event".into());
            Ok(())
        },
    );
    assert_eq!(
        *values.lock().unwrap(),
        ["opening", "ready", "event", "ended"]
    );
    registry.close("main", &expected).unwrap(); // Finished subscription close is idempotent.
    server.join().unwrap();
}

#[test]
fn native_subscription_worker_old_generation_zero_dispatch_and_precise_close() {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    listener.set_nonblocking(true).unwrap();
    let registry = registry();
    let old = operation(&registry, 1);
    let current = operation(&registry, 2);
    let old_id = old.identity().subscription_id;
    run_with(
        RemoteControllerClient::new(listener.local_addr().unwrap(), "owned".into()),
        old,
        |_, _| panic!("stale status"),
        |_, _| panic!("stale event"),
    );
    assert_eq!(
        listener.accept().err().unwrap().kind(),
        std::io::ErrorKind::WouldBlock
    );
    registry.close("main", &old_id).unwrap();
    assert_eq!(current.with_current(|_| true).unwrap(), Some(true));
    registry
        .close("main", &current.identity().subscription_id)
        .unwrap();
    assert_eq!(
        current
            .with_terminal(|_| panic!("closed terminal"))
            .unwrap(),
        None::<()>
    );
    for field in ["url", "token", "controllerId", "generation"] {
        let mut input = json!({"subscriptionId":old_id});
        input[field] = json!("private");
        assert!(serde_json::from_value::<UnsubscribeRequest>(input).is_err());
    }
}

#[test]
fn native_subscription_worker_unsubscribe_cancels_open_reader_without_late_status() {
    assert_reader_revoked(false);
}

#[test]
fn native_subscription_worker_owner_exit_cancels_open_reader_without_late_status() {
    assert_reader_revoked(true);
}

fn assert_reader_revoked(owner_exit: bool) {
    let (address, server, sent) = fixture("owned answer", true);
    let registry = registry();
    let operation = operation(&registry, 1);
    let id = operation.identity().subscription_id;
    let (ready_tx, ready_rx) = mpsc::channel();
    let statuses = Arc::new(Mutex::new(Vec::new()));
    let observed = Arc::clone(&statuses);
    let worker = thread::spawn(move || {
        run_with(
            RemoteControllerClient::new(address, "owned".into()),
            operation,
            |_, state| {
                observed.lock().unwrap().push(state);
                if state == "ready" {
                    ready_tx.send(()).unwrap();
                }
                Ok(())
            },
            |_, _| Ok(()),
        )
    });
    ready_rx.recv_timeout(Duration::from_secs(3)).unwrap();
    sent.recv_timeout(Duration::from_secs(3)).unwrap();
    if owner_exit {
        registry.owner_revocation("owned-sidecar").revoke();
    } else {
        registry.close("main", &id).unwrap();
    }
    worker.join().unwrap();
    server.join().unwrap();
    assert_eq!(*statuses.lock().unwrap(), ["opening", "ready"]);
}
