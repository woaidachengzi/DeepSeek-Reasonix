use super::super::tests::{controller, fixture, ID};
use super::*;

fn request(kind: &str, answer: Value) -> SessionPromptRequest {
    SessionPromptRequest {
        controller_id: ID.into(),
        session_path: "/remote/selected.jsonl".into(),
        runtime_epoch: "controller-instance".into(),
        turn_id: "turn".into(),
        prompt_id: "prompt".into(),
        prompt_runtime_epoch: "".into(),
        kind: kind.into(),
        answer,
    }
}
fn response(kind: &str) -> Value {
    json!({"protocolVersion":1,"controller":controller(),"receipt":{"protocolVersion":1,"sessionPath":"/remote/selected.jsonl","runtimeEpoch":"controller-instance","turnId":"turn","promptId":"prompt","promptRuntimeEpoch":"","kind":kind,"resolved":true,"private":"PRIVATE answer"},"token":"PRIVATE key"})
}

#[test]
fn remote_prompt_ipc_five_kinds_use_single_fixed_route_and_private_typed_receipt() {
    for (kind, answer) in [
        ("ask", json!({"questions":[]})),
        ("approval", json!({"allow":false})),
        ("approval", json!({"allow":true,"persist":true})),
        ("plan", json!({"action":"revise_plan","feedback":"修改"})),
        ("recovery", json!({"action":"continue_task"})),
        ("mcp", json!({"action":"accept","content":{"answer":true}})),
    ] {
        let (client, task) = fixture(response(kind));
        let result = client
            .session_prompt(request(kind, answer.clone()))
            .unwrap();
        let (headers, body) = task.join().unwrap();
        assert!(headers.starts_with(&format!(
            "POST /v1/remote/controllers/{ID}/session-prompt HTTP/1.1"
        )));
        assert!(!headers.contains("Idempotency-Key"));
        assert_eq!(
            body,
            json!({"sessionPath":"/remote/selected.jsonl","runtimeEpoch":"controller-instance","turnId":"turn","promptId":"prompt","promptRuntimeEpoch":"","kind":kind,"answer":answer})
        );
        assert!(result.controller.read_only && result.receipt.resolved);
        assert!(!serde_json::to_string(&result).unwrap().contains("PRIVATE"));
    }
    for extra in ["url", "token", "workspace", "localSessionId", "model"] {
        let mut value = json!({"controllerId":ID,"sessionPath":"/remote/selected.jsonl","runtimeEpoch":"controller-instance","turnId":"turn","promptId":"prompt","promptRuntimeEpoch":"","kind":"approval","answer":{"allow":false}});
        value[extra] = json!("PRIVATE");
        assert!(serde_json::from_value::<SessionPromptRequest>(value).is_err());
    }
}

#[test]
fn remote_prompt_ipc_mismatched_or_incomplete_receipts_are_unknown() {
    for field in [
        "outer-version",
        "controller",
        "readOnly",
        "protocolVersion",
        "sessionPath",
        "runtimeEpoch",
        "turnId",
        "promptId",
        "promptRuntimeEpoch",
        "kind",
        "resolved",
        "missing",
    ] {
        let mut value = response("approval");
        match field {
            "outer-version" => value["protocolVersion"] = json!(2),
            "controller" => value["controller"]["id"] = json!("BBBBBBBBBBBBBBBBBBBBBA"),
            "readOnly" => value["controller"]["readOnly"] = json!(false),
            "protocolVersion" => value["receipt"][field] = json!(2),
            "resolved" => value["receipt"][field] = json!(false),
            "missing" => value = json!({"PRIVATE":"key/endpoint"}),
            _ => value["receipt"][field] = json!("other"),
        }
        let (client, task) = fixture(value);
        assert_eq!(
            client
                .session_prompt(request("approval", json!({"allow":false})))
                .unwrap_err(),
            UNKNOWN,
            "{field}"
        );
        task.join().unwrap();
    }
}

#[test]
fn remote_prompt_ipc_invalid_union_identity_and_budgets_dispatch_nothing() {
    let client = RemoteControllerClient::new("127.0.0.1:1".parse().unwrap(), "owned".into());
    for (kind, value) in [
        ("ask", json!({})),
        ("ask", json!({"questions":null})),
        ("ask", json!({"questions":[],"allow":true})),
        (
            "ask",
            json!({"questions":[{"questionId":"q","selected":[],"private":"x"}]}),
        ),
        (
            "ask",
            json!({"questions":[{"questionId":"q","selected":[]},{"questionId":"q","selected":[]}]}),
        ),
        (
            "ask",
            json!({"questions":[{"questionId":"q","selected":["x".repeat(8193)]}]}),
        ),
        ("approval", json!({})),
        ("approval", json!({"allow":null})),
        ("approval", json!({"allow":"true"})),
        ("approval", json!({"allow":false,"session":true})),
        (
            "approval",
            json!({"allow":true,"session":true,"persist":true}),
        ),
        ("approval", json!({"allow":true,"content":null})),
        ("plan", json!({"action":"continue"})),
        (
            "plan",
            json!({"action":"revise_plan","feedback":"x".repeat(4097)}),
        ),
        ("recovery", json!({"action":"stop"})),
        ("recovery", json!({"action":"revise","feedback":"\0"})),
        ("mcp", json!({"action":"decline","content":{}})),
        ("mcp", json!({"action":"accept","content":[]})),
        (
            "mcp",
            json!({"action":"accept","content":{"value":"x".repeat(65537)}}),
        ),
        ("unknown", json!({})),
    ] {
        assert_eq!(
            client.session_prompt(request(kind, value)).unwrap_err(),
            INVALID,
            "{kind}"
        );
    }
    for kind in [
        "handle",
        "path",
        "epoch",
        "turn",
        "prompt",
        "routing",
        "path-budget",
        "answer-budget",
    ] {
        let mut req = request("approval", json!({"allow":false}));
        match kind {
            "handle" => req.controller_id = "invalid".into(),
            "path" => req.session_path.clear(),
            "epoch" => req.runtime_epoch.clear(),
            "turn" => req.turn_id.clear(),
            "prompt" => req.prompt_id.clear(),
            "routing" => req.prompt_runtime_epoch = "bad\nidentity".into(),
            "path-budget" => req.session_path = "x".repeat(32769),
            "answer-budget" => {
                req.kind = "ask".into();
                req.answer =
                    json!({"questions":[{"questionId":"q","selected":["x".repeat(131073)]}]});
            }
            _ => unreachable!(),
        }
        assert_eq!(client.session_prompt(req).unwrap_err(), INVALID, "{kind}");
    }
    // A syntactically valid network dispatch with no confirmation is unknown,
    // not an invalid-input rejection or a fallback into local execution.
    assert_eq!(
        client
            .session_prompt(request("approval", json!({"allow":false})))
            .unwrap_err(),
        UNKNOWN
    );
}
