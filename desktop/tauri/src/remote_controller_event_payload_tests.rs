use super::*;

fn base(kind: &str) -> Value {
    json!({"kind":kind,"sessionPath":"/remote/selected.jsonl","turnId":"owned-turn","seq":1})
}
fn minimal(shape: &Shape) -> Value {
    match shape.kind.as_str() {
        "string" => json!("owned"),
        "boolean" => json!(false),
        "integer" | "unsigned" | "number" => json!(0),
        "array" => json!([]),
        "map" | "json" => json!({}),
        "object" => Value::Object(
            shape
                .fields
                .iter()
                .filter(|(_, field)| !field.optional)
                .map(|(key, field)| (key.clone(), minimal(&field.shape)))
                .collect(),
        ),
        _ => panic!("unknown generated shape"),
    }
}

#[test]
fn native_event_projection_preserves_all_go_kinds_and_known_nested_fields() {
    let contract: Contract =
        serde_json::from_str(include_str!("remote_event_contract.generated.json")).unwrap();
    let mut full = minimal(&contract.event);
    for (key, field) in &contract.event.fields {
        full[key] = minimal(&field.shape);
    }
    full["turnId"] = json!("owned-turn");
    full["itemId"] = json!("owned-item");
    full["promptId"] = json!("owned-prompt");
    full["sessionPath"] = json!("/remote/selected.jsonl");
    full["status"] = json!("in_progress");
    full["runtimeState"]["phase"] = json!("executing");
    full["runtimeState"]["turnStatus"] = json!("in_progress");
    for kind in &contract.kinds {
        full["kind"] = json!(kind);
        if ["steer", "user_message_admitted", "host_input_admitted"].contains(&kind.as_str()) {
            full["messageId"] = json!("owned-message");
        } else {
            full.as_object_mut().unwrap().remove("messageId");
        }
        let expected = if ["user_message_admitted", "host_input_admitted"].contains(&kind.as_str())
        {
            let mut expected = full.clone();
            expected.as_object_mut().unwrap().retain(|key, _| {
                [
                    "kind",
                    "messageId",
                    "turnId",
                    "seq",
                    "status",
                    "sessionPath",
                    "sessionCurrent",
                ]
                .contains(&key.as_str())
            });
            expected
        } else {
            full.clone()
        };
        assert_eq!(
            project(&full).unwrap_or_else(|_| panic!("generated fixture rejected for {kind}")),
            expected,
            "{kind}"
        );
    }
    assert!(contract.kinds.len() >= 32);
}

#[test]
fn native_admission_identity_preserves_both_origins_without_body_or_authority() {
    for kind in ["user_message_admitted", "host_input_admitted"] {
        let mut frame = base(kind);
        frame["messageId"] = json!("canonical-input");
        frame["sessionCurrent"] = json!(true);
        let expected = frame.clone();
        frame["text"] = json!("private instructions");
        frame["detail"] = json!("private detail");
        frame["itemId"] = json!("approval-identity");
        assert_eq!(project(&frame).unwrap(), expected);
        frame.as_object_mut().unwrap().remove("messageId");
        assert_eq!(project(&frame).err().as_deref(), Some(FAILED));
        for bad in [json!(""), json!("bad\n"), json!(12), Value::Null] {
            frame["messageId"] = bad;
            assert_eq!(project(&frame).err().as_deref(), Some(FAILED));
        }
    }
}

#[test]
fn native_steer_message_identity_is_narrow_and_legacy_optional() {
    let mut steer = base("steer");
    assert!(project(&steer).unwrap().get("messageId").is_none());
    steer["itemId"] = json!("owned-inbox");
    steer["messageId"] = json!("owned-message");
    let projected = project(&steer).unwrap();
    assert_eq!(projected["messageId"], "owned-message");
    assert_eq!(projected["itemId"], "owned-inbox");
    for bad in [
        json!("bad\n"),
        json!("x".repeat(4097)),
        json!(12),
        Value::Null,
    ] {
        steer["messageId"] = bad;
        assert_eq!(project(&steer).err().as_deref(), Some(FAILED));
    }
    steer["messageId"] = json!("owned-message");
    steer["kind"] = json!("text");
    assert_eq!(project(&steer).err().as_deref(), Some(FAILED));
}

#[test]
fn native_event_projection_drops_unknowns_without_returning_private_errors() {
    let contract: Contract =
        serde_json::from_str(include_str!("remote_event_contract.generated.json")).unwrap();
    let mut value = base("tool_result");
    value["tool"] = minimal(&contract.event.fields["tool"].shape);
    value["tool"]["id"] = json!("owned-tool");
    value["tool"]["token"] = json!("PRIVATE");
    value["token"] = json!("PRIVATE");
    value["tool"]["profile"] = json!({"model":"owned","effort":"high","apiKey":"PRIVATE"});
    let result = project(&value).unwrap();
    assert!(!result.to_string().contains("PRIVATE"));
    assert_eq!(
        result["tool"]["profile"],
        json!({"model":"owned","effort":"high"})
    );
    for field in ["kind", "tool"] {
        let mut broken = value.clone();
        broken[field] = json!("PRIVATE");
        assert_eq!(project(&broken).err().as_deref(), Some(FAILED));
    }
}

#[test]
fn native_event_projection_checks_types_numbers_ids_and_required_payloads() {
    for kind in [
        "tool_dispatch",
        "approval_request",
        "ask_request",
        "mcp_interaction",
        "usage",
        "workspace_changed",
        "read_status",
        "stream_attempt",
        "completion_summary",
    ] {
        assert_eq!(project(&base(kind)).err().as_deref(), Some(FAILED));
    }
    for (key, value) in [
        ("seq", json!(MAX_JS + 1)),
        ("seq", json!(-1)),
        ("seq", json!(1.5)),
        ("seq", Value::Null),
        ("sessionCurrent", json!("true")),
        ("turnId", json!("bad\n")),
        ("promptId", json!("x".repeat(4097))),
        ("text", json!([])),
        ("runtimeState", json!({})),
        ("kind", json!("unknown_kind")),
        ("status", json!("unknown_status")),
    ] {
        let mut frame = base("text");
        frame[key] = value;
        assert_eq!(project(&frame).err().as_deref(), Some(FAILED), "{key}");
    }
    let mut frame = base("text");
    frame["text"] = json!("allowed\nmultiline\ttext");
    frame["runtimeState"] = Value::Null;
    assert!(project(&frame).unwrap().get("runtimeState").is_none());
}

#[test]
fn native_event_projection_preserves_purpose_json_but_bounds_every_field() {
    let mut frame = base("mcp_interaction");
    frame["mcpInteraction"] = json!({"id":"owned-prompt","server":"owned","mode":"form","message":"owned","requestedSchema":{"type":"object","properties":{"token":{"type":"string"}}}});
    assert_eq!(project(&frame).unwrap(), frame); // A schema property name is data, not host auth.
    let mut nested = json!("leaf");
    for _ in 0..65 {
        nested = json!([nested]);
    }
    frame["mcpInteraction"]["requestedSchema"] = nested;
    assert_eq!(project(&frame).err().as_deref(), Some(FAILED));
    let mut frame = base("text");
    frame["unknown"] = json!(vec![false; 100001]);
    assert_eq!(project(&frame).err().as_deref(), Some(FAILED));
}
