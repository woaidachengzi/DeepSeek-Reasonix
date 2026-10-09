use super::super::tests::{controller, fixture, ID};
use super::*;

fn request() -> SessionSubmitRequest {
    SessionSubmitRequest {
        controller_id: ID.into(),
        session_path: "/remote/selected.jsonl".into(),
        runtime_epoch: "controller-instance".into(),
        revision: 7,
        text: "用户问题\n/new".into(),
    }
}
fn response() -> Value {
    json!({"protocolVersion":1,"controller":controller(),"receipt":{"protocolVersion":1,"sessionPath":"/remote/selected.jsonl","runtimeEpoch":"controller-instance","revision":7,"accepted":true,"private":"PRIVATE replay"},"token":"PRIVATE token"})
}

#[test]
fn remote_submit_ipc_single_fixed_route_scoped_receipt_and_private_stripping() {
    for extra in [
        "url",
        "token",
        "workspace",
        "localSessionId",
        "action",
        "model",
    ] {
        let mut value = json!({"controllerId":ID,"sessionPath":"/remote/selected.jsonl","runtimeEpoch":"controller-instance","revision":7,"text":"question"});
        value[extra] = json!("PRIVATE");
        assert!(serde_json::from_value::<SessionSubmitRequest>(value).is_err());
    }
    let (client, task) = fixture(response());
    let result = client.session_submit(request()).unwrap();
    let (headers, body) = task.join().unwrap();
    assert!(headers.starts_with(&format!(
        "POST /v1/remote/controllers/{ID}/session-submit HTTP/1.1"
    )));
    assert!(!headers.contains("Idempotency-Key"));
    assert_eq!(
        body,
        json!({"sessionPath":"/remote/selected.jsonl","runtimeEpoch":"controller-instance","revision":7,"text":"用户问题\n/new"})
    );
    assert!(result.controller.read_only);
    assert!(!serde_json::to_string(&result).unwrap().contains("PRIVATE"));
}

#[test]
fn remote_submit_ipc_wrong_receipts_remain_unknown_without_diagnostics() {
    for kind in [
        "version",
        "controller",
        "write",
        "path",
        "epoch",
        "revision",
        "accepted",
        "receipt-version",
        "missing",
    ] {
        let mut value = response();
        match kind {
            "version" => value["protocolVersion"] = json!(2),
            "controller" => value["controller"]["id"] = json!("BBBBBBBBBBBBBBBBBBBBBA"),
            "write" => value["controller"]["readOnly"] = json!(false),
            "path" => value["receipt"]["sessionPath"] = json!("/other"),
            "epoch" => value["receipt"]["runtimeEpoch"] = json!("old"),
            "revision" => value["receipt"]["revision"] = json!(6),
            "accepted" => value["receipt"]["accepted"] = json!(false),
            "receipt-version" => value["receipt"]["protocolVersion"] = json!(2),
            "missing" => value = json!({"PRIVATE":"key/endpoint"}),
            _ => unreachable!(),
        }
        let (client, task) = fixture(value);
        assert_eq!(
            client.session_submit(request()).unwrap_err(),
            UNKNOWN,
            "{kind}"
        );
        task.join().unwrap();
    }
}

#[test]
fn remote_submit_ipc_invalid_inputs_dispatch_nothing() {
    let client = RemoteControllerClient::new("127.0.0.1:1".parse().unwrap(), "owned".into());
    for kind in [
        "handle",
        "path",
        "epoch",
        "revision",
        "unsafe",
        "control",
        "budget",
        "empty",
        "nul",
        "text-budget",
        "encoded-budget",
    ] {
        let mut input = request();
        match kind {
            "handle" => input.controller_id = "../other".into(),
            "path" => input.session_path.clear(),
            "epoch" => input.runtime_epoch.clear(),
            "revision" => input.revision = 0,
            "unsafe" => input.revision = MAX_JS + 1,
            "control" => input.runtime_epoch = "bad\ninstance".into(),
            "budget" => input.runtime_epoch = "x".repeat(4097),
            "empty" => input.text = " \n ".into(),
            "nul" => input.text = "bad\0text".into(),
            "text-budget" => input.text = "x".repeat((512 << 10) + 1),
            "encoded-budget" => input.text = "\u{0001}".repeat(200 << 10),
            _ => unreachable!(),
        }
        assert_eq!(client.session_submit(input).unwrap_err(), INVALID, "{kind}");
    }
    assert_eq!(client.session_submit(request()).unwrap_err(), UNKNOWN);
}
