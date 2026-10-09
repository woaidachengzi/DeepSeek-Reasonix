use super::super::tests as fixture;
use super::*;

fn request() -> SessionProjectionRequest {
    SessionProjectionRequest {
        controller_id: fixture::ID.into(),
        session_path: "/remote/中文 &+.jsonl".into(),
        continuation: None,
    }
}
fn response() -> Value {
    json!({"protocolVersion":1,"controller":fixture::controller(),"projection":{
        "protocolVersion":1,"sessionPath":"/remote/中文 &+.jsonl","readOnly":true,"initial":true,
        "history":[{"id":"old-user","role":"user","content":"old","secret":"private-config"}],
        "userSuffix":[{"id":"question-id","role":"user","content":"new"}],
        "activeTurnId":"turn-id","turnStatus":"in_progress","replayAfterSeq":0,
        "replay":{"events":[{"kind":"steer","text":"guidance","itemId":"inbox-id","messageId":"saved-guidance-id","turnId":"turn-id","seq":1,"sessionPath":"/remote/中文 &+.jsonl","status":"in_progress","secret":"private-event"}],"floorSeq":1,"latestSeq":2,"nextAfterSeq":1,"hasMore":true,"runtimeEpoch":"epoch"}},"nextPage":fixture::ID,"token":"private-extra"})
}

#[test]
fn snapshot_transport_fixed_route_identity_and_privacy() {
    let (client, task) = fixture::fixture(response());
    let result = client.session_projection(request()).unwrap();
    assert_eq!(result.projection.history[0].id, "old-user");
    assert_eq!(result.projection.user_suffix[0].id, "question-id");
    assert_eq!(
        result.projection.replay.events[0]["messageId"],
        "saved-guidance-id"
    );
    assert_eq!(result.projection.replay.events[0]["itemId"], "inbox-id");
    assert!(!serde_json::to_string(&result).unwrap().contains("private-"));
    let (headers, body) = task.join().unwrap();
    assert!(headers.starts_with(&format!(
        "POST /v1/remote/controllers/{}/session-projection HTTP/1.1",
        fixture::ID
    )));
    assert!(headers.contains("Authorization: Bearer owned-bridge-auth\r\n"));
    assert_eq!(body, json!({"sessionPath":"/remote/中文 &+.jsonl"}));
}

#[test]
fn snapshot_continuation_transmits_only_host_handle_and_path() {
    let mut value = response();
    value["projection"]["initial"] = json!(false);
    value["projection"]["history"] = json!([]);
    value["projection"]["userSuffix"] = json!([]);
    value["projection"]["replay"]["events"][0]["seq"] = json!(2);
    value["projection"]["replay"]["nextAfterSeq"] = json!(2);
    value["projection"]["replay"]["hasMore"] = json!(false);
    value.as_object_mut().unwrap().remove("nextPage");
    let (client, task) = fixture::fixture(value);
    let mut req = request();
    req.continuation = Some(fixture::ID.into());
    let result = client.session_projection(req).unwrap();
    assert!(!result.projection.initial);
    assert_eq!(result.projection.replay.next_after_seq, 2);
    let (_, body) = task.join().unwrap();
    assert_eq!(
        body,
        json!({"sessionPath":"/remote/中文 &+.jsonl","continuation":fixture::ID})
    );
}

#[test]
fn snapshot_invalid_request_zero_dispatch() {
    for key in [
        "url",
        "token",
        "history",
        "after",
        "pageToken",
        "generation",
    ] {
        let mut value = json!({"controllerId":fixture::ID,"sessionPath":"owned"});
        value[key] = json!("forbidden");
        assert!(serde_json::from_value::<SessionProjectionRequest>(value).is_err());
    }
    let client = RemoteControllerClient::new("127.0.0.1:1".parse().unwrap(), "owned".into());
    for continuation in ["", "a/../b", "AAAAAAAAAAAAAAAAAAAAAB", "bad\npath"] {
        let mut req = request();
        req.continuation = Some(continuation.into());
        assert_eq!(client.session_projection(req).unwrap_err(), INVALID);
    }
    for path in ["", "bad\npath"] {
        let mut req = request();
        req.session_path = path.into();
        assert_eq!(client.session_projection(req).unwrap_err(), INVALID);
    }
}

#[test]
fn snapshot_rejects_mixed_scope_status_identity_and_cut() {
    let edits = [
        ("/controller/id", json!("AQEBAQEBAQEBAQEBAQEBAQ")),
        ("/projection/sessionPath", json!("other")),
        ("/projection/readOnly", json!(false)),
        ("/projection/initial", json!(false)),
        ("/projection/history/0/role", json!("system")),
        ("/projection/userSuffix/0/id", json!("old-user")),
        ("/projection/userSuffix/0/role", json!("assistant")),
        ("/projection/activeTurnId", json!("")),
        ("/projection/turnStatus", json!("completed")),
        ("/projection/replay/floorSeq", json!(0)),
        ("/projection/replay/latestSeq", json!(MAX_JS + 1)),
        ("/projection/replay/nextAfterSeq", json!(2)),
        ("/projection/replay/events/0/seq", json!(2)),
        ("/projection/replay/events/0/turnId", json!("other")),
        ("/projection/replay/events/0/sessionPath", json!("other")),
        ("/projection/replay/events/0/kind", json!("unknown")),
        ("/projection/replay/events/0/status", json!("completed")),
        ("/projection/replay/events/0/status", json!("")),
        ("/projection/replay/events", json!([])),
        ("/projection/replay/events", json!(vec![json!({}); 513])),
        ("/nextPage", json!("bad")),
    ];
    for (pointer, replacement) in edits {
        let mut value = response();
        *value.pointer_mut(pointer).unwrap() = replacement;
        let (client, task) = fixture::fixture(value);
        assert_eq!(
            client.session_projection(request()).unwrap_err(),
            FAILED,
            "{pointer}"
        );
        task.join().unwrap();
    }
    let mut value = response();
    value["projection"]["replay"]["events"][0]
        .as_object_mut()
        .unwrap()
        .remove("status");
    let (client, task) = fixture::fixture(value);
    assert_eq!(client.session_projection(request()).unwrap_err(), FAILED);
    task.join().unwrap();
    let mut value = response();
    value["projection"]["pageToken"] = json!("must-stay-host-only");
    let (client, task) = fixture::fixture(value);
    assert_eq!(client.session_projection(request()).unwrap_err(), FAILED);
    task.join().unwrap();
}
