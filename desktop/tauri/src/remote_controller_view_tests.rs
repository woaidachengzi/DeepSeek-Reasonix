use super::*;

fn snapshot() -> Value {
    json!({"protocolVersion":1,"sessionPath":"/remote/中文 &+.jsonl","readOnly":true,"ownership":"saved","current":false,"modelRef":"","label":"","history":[{"id":"owned-user","role":"user","content":"你好"},{"id":"owned-answer","role":"assistant","content":"answer","reasoning":"thought","toolCalls":[{"id":"t","name":"read","arguments":"{}"}],"serverSearch":[{"id":"s","results":[{"title":"title","url":"https://example.invalid"}],"raw":{"secret":"private-replay"}}],"credentials":{"token":"private-config"}}],"token":"private-extra"})
}
fn response() -> Value {
    let mut value = json!({"protocolVersion":1,"controller":tests::controller(),"view":snapshot()});
    value["view"]["history"][1]["serverSearch"][0]["sources_status"] = json!("not_provided");
    value
}
fn request() -> SessionViewRequest {
    SessionViewRequest {
        controller_id: tests::ID.into(),
        session_path: "/remote/中文 &+.jsonl".into(),
    }
}

#[test]
fn remote_session_view_ipc_fixed_route_typed_privacy_and_unknown_request_rejection() {
    for extra in [
        "url",
        "token",
        "modelRef",
        "workspace",
        "sessionId",
        "action",
    ] {
        let mut value = json!({"controllerId":tests::ID,"sessionPath":"/remote/a.jsonl"});
        value[extra] = json!("not-allowed");
        assert!(serde_json::from_value::<SessionViewRequest>(value).is_err());
    }
    let client = RemoteControllerClient::new("127.0.0.1:1".parse().unwrap(), "owned".into());
    for p in ["", "bad\npath", "bad\u{1b}path"] {
        let mut req = request();
        req.session_path = p.into();
        assert_eq!(client.session_view(req).unwrap_err(), INVALID);
    }
    let mut req = request();
    req.controller_id = "a/../b".into();
    assert_eq!(client.session_view(req).unwrap_err(), INVALID);
    let (client, task) = tests::fixture(response());
    let view = client.session_view(request()).unwrap();
    assert_eq!(view.view.history[0].id, "owned-user");
    assert_eq!(view.view.history[1].id, "owned-answer");
    assert_eq!(view.view.history[1].reasoning.as_deref(), Some("thought"));
    assert_eq!(
        view.view.history[1].server_search.as_ref().unwrap()[0]
            .sources_status
            .as_deref(),
        Some("not_provided")
    );
    let wire = serde_json::to_value(&view).unwrap();
    assert_eq!(
        wire["view"]["history"][1]["serverSearch"][0]["sources_status"],
        "not_provided"
    );
    assert!(wire["view"]["history"][1]["serverSearch"][0]
        .get("sourcesStatus")
        .is_none());
    let serialized = serde_json::to_string(&view).unwrap();
    assert!(!serialized.contains("private-") && !serialized.contains("credentials"));
    let (headers, body) = task.join().unwrap();
    assert!(headers.starts_with(&format!(
        "POST /v1/remote/controllers/{}/session-view HTTP/1.1",
        tests::ID
    )));
    assert!(headers.contains("Authorization: Bearer owned-bridge-auth\r\n"));
    assert_eq!(body, json!({"sessionPath":"/remote/中文 &+.jsonl"}));
}

#[test]
fn remote_session_view_ipc_rejects_wrong_session_runtime_and_budget() {
    let mut candidates = Vec::new();
    for value in [
        json!(""),
        json!("bad\nidentity"),
        json!("x".repeat(4097)),
        Value::Null,
    ] {
        let mut candidate = response();
        candidate["view"]["history"][0]["id"] = value;
        candidates.push(candidate);
    }
    let mut candidate = response();
    candidate["view"]["history"][1]["id"] = json!("owned-user");
    candidates.push(candidate);
    let mut candidate = response();
    candidate["view"]["history"][0]
        .as_object_mut()
        .unwrap()
        .remove("id");
    candidates.push(candidate);
    for (field, value) in [
        ("sessionPath", json!("/other")),
        ("protocolVersion", json!(2)),
        ("readOnly", json!(false)),
        ("ownership", json!("unknown")),
        ("modelRef", json!("local/model")),
        ("history", Value::Null),
    ] {
        let mut candidate = response();
        candidate["view"][field] = value;
        candidates.push(candidate);
    }
    let mut candidate = response();
    candidate["view"]["history"][0]["role"] = json!("unexpected");
    candidates.push(candidate);
    let mut candidate = response();
    candidate["controller"]["id"] = json!("BBBBBBBBBBBBBBBBBBBBBA");
    candidates.push(candidate);
    let state = json!({"schemaVersion":1,"runtimeEpoch":"owned-epoch","revision":1,"phase":"idle","running":false,"turnId":"","turnStatus":"","turnEventSeq":0,"pendingPrompt":false,"cancelRequested":false,"cancellable":false,"backgroundJobs":0,"activity":""});
    let mut owned = response();
    owned["view"]["ownership"] = json!("serve");
    owned["view"]["modelRef"] = json!("remote/model");
    owned["view"]["runtimeState"] = state;
    for (field, value) in [
        ("phase", json!("unknown")),
        ("turnStatus", json!("unknown")),
        ("revision", json!(9007199254740992u64)),
        ("backgroundJobs", json!(-1)),
    ] {
        let mut candidate = owned.clone();
        candidate["view"]["runtimeState"][field] = value;
        candidates.push(candidate);
    }
    for candidate in candidates {
        let (client, task) = tests::fixture(candidate);
        assert_eq!(client.session_view(request()).unwrap_err(), FAILED);
        task.join().unwrap();
    }
    let (client, task) = tests::fixture(owned);
    assert_eq!(
        client.session_view(request()).unwrap().view.model_ref,
        "remote/model"
    );
    task.join().unwrap();
}
