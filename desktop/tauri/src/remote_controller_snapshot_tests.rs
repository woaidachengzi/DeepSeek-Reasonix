use super::*;
use std::{
    io::{BufRead, BufReader, Read, Write},
    net::TcpListener,
    sync::mpsc,
    thread,
    time::Duration,
};
const ID: &str = "AAAAAAAAAAAAAAAAAAAAAA";
fn subscription(registry: &RemoteSubscriptions, generation: u64) -> SubscriptionOperation {
    registry
        .reserve(
            "main",
            "owned",
            SubscribeRequest {
                controller_id: ID.into(),
                session_path: "/remote/selected.jsonl".into(),
                surface_id: "surface".into(),
                generation,
            },
        )
        .unwrap()
}
fn registry() -> RemoteSubscriptions {
    let registry = RemoteSubscriptions::default();
    registry.set_owner(Some("owned"));
    registry
}
fn request(operation: &SubscriptionOperation) -> SnapshotRequest {
    SnapshotRequest {
        subscription_id: operation.identity().subscription_id,
        continuation: None,
    }
}
fn ready(operation: &SubscriptionOperation) {
    assert!(matches!(operation.with_ready(|_| Ok(())), Ok(Some(Ok(())))));
}

#[test]
fn snapshot_requires_ready_exact_owner_main_and_single_read() {
    let registry = registry();
    let subscription = subscription(&registry, 1);
    assert!(registry
        .reserve_snapshot("main", "owned", request(&subscription))
        .is_err());
    assert!(matches!(
        subscription.with_ready(|_| Err(())),
        Ok(Some(Err(())))
    ));
    assert!(registry
        .reserve_snapshot("main", "owned", request(&subscription))
        .is_err());
    ready(&subscription);
    assert!(registry
        .reserve_snapshot("settings", "owned", request(&subscription))
        .is_err());
    assert!(registry
        .reserve_snapshot("main", "other-owner", request(&subscription))
        .is_err());
    let snapshot = registry
        .reserve_snapshot("main", "owned", request(&subscription))
        .unwrap();
    assert!(registry
        .reserve_snapshot("main", "owned", request(&subscription))
        .is_err());
    assert_eq!(
        snapshot
            .with_current(|identity| identity.generation)
            .unwrap(),
        1
    );
    drop(snapshot);
    assert!(!subscription.cancellation().is_closed());
    assert!(registry
        .reserve_snapshot("main", "owned", request(&subscription))
        .is_ok());
    for key in [
        "controllerId",
        "sessionPath",
        "generation",
        "surfaceId",
        "sidecarInstanceId",
        "url",
        "token",
        "after",
    ] {
        let mut input = json!({"subscriptionId":subscription.identity().subscription_id});
        input[key] = json!("forbidden");
        assert!(serde_json::from_value::<SnapshotRequest>(input).is_err());
    }
}

#[test]
fn snapshot_retirement_zero_old_publication_or_dispatch() {
    for action in ["replace", "close", "clear", "owner", "observer"] {
        let registry = registry();
        let live = subscription(&registry, 1);
        ready(&live);
        let snapshot = registry
            .reserve_snapshot("main", "owned", request(&live))
            .unwrap();
        let replacement = if action == "replace" {
            Some(subscription(&registry, 2))
        } else {
            None
        };
        match action {
            "close" => registry
                .close("main", &live.identity().subscription_id)
                .unwrap(),
            "clear" => registry.clear(),
            "owner" => registry.set_owner(Some("next-owner")),
            "observer" => registry.owner_revocation("owned").revoke(),
            _ => {}
        }
        assert!(snapshot.cancellation.is_closed(), "{action}");
        assert!(snapshot
            .with_current(|_| panic!("stale publication"))
            .is_err());
        let client = RemoteControllerClient::new("127.0.0.1:1".parse().unwrap(), "private".into());
        assert!(snapshot.read(client).is_err());
        drop(snapshot);
        if let Some(next) = replacement {
            ready(&next);
            assert!(registry
                .reserve_snapshot("main", "owned", request(&next))
                .is_ok());
        }
    }
}

#[test]
fn snapshot_result_preserves_subscription_scope_and_narrow_wire() {
    let registry = registry();
    let live = subscription(&registry, 1);
    ready(&live);
    let snapshot = registry
        .reserve_snapshot("main", "owned", request(&live))
        .unwrap();
    let response = json!({"protocolVersion":1,"controller":super::super::super::tests::controller(),"projection":{"protocolVersion":1,"sessionPath":"/remote/selected.jsonl","readOnly":true,"initial":true,"history":[{"id":"question-id","role":"user","content":"question"}],"userSuffix":[],"replayAfterSeq":0,"replay":{"events":[],"floorSeq":1,"latestSeq":0,"nextAfterSeq":0,"hasMore":false}}});
    let (client, task) = super::super::super::tests::fixture(response);
    let result = snapshot.read(client).unwrap();
    assert_eq!(
        result.subscription.subscription_id,
        live.identity().subscription_id
    );
    assert_eq!(result.subscription.sidecar_instance_id, "owned");
    assert_eq!(result.subscription.generation, 1);
    assert_eq!(result.snapshot.projection.history[0].id, "question-id");
    let (_, body) = task.join().unwrap();
    assert_eq!(body, json!({"sessionPath":"/remote/selected.jsonl"}));
}

#[test]
fn snapshot_generation_replacement_interrupts_actual_pending_body() {
    let registry = registry();
    let live = subscription(&registry, 1);
    ready(&live);
    let snapshot = registry
        .reserve_snapshot("main", "owned", request(&live))
        .unwrap();
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let address = listener.local_addr().unwrap();
    let (entered_tx, entered_rx) = mpsc::channel();
    let (closed_tx, closed_rx) = mpsc::channel();
    let server = thread::spawn(move || {
        let (socket, _) = listener.accept().unwrap();
        socket
            .set_read_timeout(Some(Duration::from_secs(3)))
            .unwrap();
        let mut reader = BufReader::new(socket);
        let mut length = 0;
        loop {
            let mut line = String::new();
            reader.read_line(&mut line).unwrap();
            assert!(!line.is_empty());
            if line == "\r\n" {
                break;
            }
            if let Some(value) = line.strip_prefix("Content-Length: ") {
                length = value.trim().parse::<usize>().unwrap();
            }
        }
        let mut body = vec![0; length];
        reader.read_exact(&mut body).unwrap();
        reader
            .get_mut()
            .write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 4\r\nConnection: close\r\n\r\n{")
            .unwrap();
        reader.get_mut().flush().unwrap();
        entered_tx.send(()).unwrap();
        let mut byte = [0];
        let result = reader.read(&mut byte);
        closed_tx
            .send(result.is_err() || matches!(result, Ok(0)))
            .unwrap();
    });
    let client = RemoteControllerClient::new(address, "owned-auth".into());
    let (done_tx, done_rx) = mpsc::channel();
    let worker = thread::spawn(move || {
        done_tx.send(snapshot.read(client).is_err()).unwrap();
    });
    entered_rx.recv_timeout(Duration::from_secs(3)).unwrap();
    let next = subscription(&registry, 2);
    ready(&next);
    assert!(done_rx.recv_timeout(Duration::from_secs(3)).unwrap());
    assert!(closed_rx.recv_timeout(Duration::from_secs(3)).unwrap());
    worker.join().unwrap();
    server.join().unwrap();
    assert!(!next.cancellation().is_closed());
    assert!(registry
        .reserve_snapshot("main", "owned", request(&next))
        .is_ok());
}
