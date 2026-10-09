use super::super::tests::{controller, fixture, ID};
use super::*;

fn request() -> SessionCancelRequest {
    SessionCancelRequest {
        controller_id: ID.into(),
        session_path: "/remote/selected.jsonl".into(),
        runtime_epoch: "controller-instance".into(),
        turn_id: "selected-turn".into(),
    }
}
fn response() -> Value {
    json!({"protocolVersion":1,"controller":controller(),"receipt":{"protocolVersion":1,"sessionPath":"/remote/selected.jsonl","runtimeEpoch":"controller-instance","turnId":"selected-turn","cancelled":true,"private":"PRIVATE replay"},"token":"PRIVATE token"})
}

#[test]
fn remote_cancel_ipc_single_fixed_route_scoped_receipt_and_private_stripping() {
    for extra in ["url", "token", "workspace", "localSessionId", "action"] {
        let mut value = json!({"controllerId":ID,"sessionPath":"/remote/selected.jsonl","runtimeEpoch":"controller-instance","turnId":"selected-turn"});
        value[extra] = json!("PRIVATE");
        assert!(serde_json::from_value::<SessionCancelRequest>(value).is_err());
    }
    let (client, task) = fixture(response());
    let result = client.session_cancel(request()).unwrap();
    let (headers, body) = task.join().unwrap();
    assert!(headers.starts_with(&format!(
        "POST /v1/remote/controllers/{ID}/session-cancel HTTP/1.1"
    )));
    assert!(!headers.contains("Idempotency-Key"));
    assert_eq!(
        body,
        json!({"sessionPath":"/remote/selected.jsonl","runtimeEpoch":"controller-instance","turnId":"selected-turn"})
    );
    assert!(result.controller.read_only);
    assert!(!serde_json::to_string(&result).unwrap().contains("PRIVATE"));
}

#[test]
fn remote_cancel_ipc_wrong_receipts_remain_unknown_without_diagnostics() {
    for kind in [
        "version",
        "controller",
        "write",
        "path",
        "epoch",
        "turn",
        "cancelled",
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
            "turn" => value["receipt"]["turnId"] = json!("old"),
            "cancelled" => value["receipt"]["cancelled"] = json!(false),
            "receipt-version" => value["receipt"]["protocolVersion"] = json!(2),
            "missing" => value = json!({"PRIVATE":"key/endpoint"}),
            _ => unreachable!(),
        }
        let (client, task) = fixture(value);
        assert_eq!(
            client.session_cancel(request()).unwrap_err(),
            UNKNOWN,
            "{kind}"
        );
        task.join().unwrap();
    }
}

#[test]
fn remote_cancel_ipc_invalid_scopes_dispatch_nothing() {
    let client = RemoteControllerClient::new("127.0.0.1:1".parse().unwrap(), "owned".into());
    for kind in ["handle", "path", "epoch", "turn", "control", "budget"] {
        let mut input = request();
        match kind {
            "handle" => input.controller_id = "../other".into(),
            "path" => input.session_path.clear(),
            "epoch" => input.runtime_epoch.clear(),
            "turn" => input.turn_id.clear(),
            "control" => input.turn_id = "bad\nturn".into(),
            "budget" => input.runtime_epoch = "x".repeat(4097),
            _ => unreachable!(),
        }
        assert_eq!(client.session_cancel(input).unwrap_err(), INVALID, "{kind}");
    }
    assert_eq!(client.session_cancel(request()).unwrap_err(), UNKNOWN);
}
