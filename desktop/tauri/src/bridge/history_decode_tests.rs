use super::{http_response, BridgeHistoryResponse};
use serde_json::{json, Value};

fn wire(body: &[u8]) -> Vec<u8> {
    let mut response = b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n".to_vec();
    response.extend_from_slice(body);
    response
}

fn fixture() -> Value {
    json!({
        "protocolVersion":1,"sequence":42,"startIndex":100,"totalMessages":102,
        "session":{"id":"fixture","title":"中文 history","path":"/fixtures/private.jsonl","model":"fixture-model","state":"idle"},
        "messages":[
            {"role":"user","content":"quote: \"x\"\n中文 🙂","truncated":false,"createdAtMs":7},
            {"role":"assistant","content":"answer","truncated":true,"workDurationMs":125,"forkToken":"hash",
             "turnUsage":{"cacheHitTokens":3,"cacheMissTokens":2,"complete":true,"estimated":false,"inputTokens":5,"outputTokens":4,"reasoningTokens":1,"requestCount":1,"totalTokens":9}}
        ]
    })
}

#[test]
fn direct_history_decode_preserves_display_contract_and_legacy_value_callers() {
    let mut body = fixture();
    body["futureAdditiveField"] = json!({"ignored":[1,2,3]});
    let response = wire(&serde_json::to_vec(&body).unwrap());
    let old_value: Value =
        http_response::parse_json_response_as(&response, crate::speech_input::safe_speech_error)
            .unwrap();
    let old: BridgeHistoryResponse = serde_json::from_value(old_value.clone()).unwrap();
    let direct: BridgeHistoryResponse =
        http_response::parse_json_response_as(&response, crate::speech_input::safe_speech_error)
            .unwrap();
    assert_eq!(
        serde_json::to_value(direct).unwrap(),
        serde_json::to_value(old).unwrap()
    );
    assert_eq!(old_value, body);
}

#[test]
fn direct_history_decode_keeps_chunk_trailers_and_first_value_compatibility() {
    let body = serde_json::to_vec(&fixture()).unwrap();
    let mut framed = b"diagnostic prefix\n".to_vec();
    framed.extend_from_slice(&body);
    framed.extend_from_slice(b"\n--- diagnostic suffix");
    let mut response = b"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n".to_vec();
    for chunk in framed.chunks(17) {
        response.extend_from_slice(format!("{:x};fixture=yes\r\n", chunk.len()).as_bytes());
        response.extend_from_slice(chunk);
        response.extend_from_slice(b"\r\n");
    }
    response.extend_from_slice(b"0\r\nX-Fixture: done\r\n\r\n");
    let direct: BridgeHistoryResponse =
        http_response::parse_json_response_as(&response, crate::speech_input::safe_speech_error)
            .unwrap();
    assert_eq!(direct.messages.len(), 2);
    assert_eq!(direct.sequence, 42);
    assert!(direct.messages[0].content.contains("中文 🙂"));
    response.extend_from_slice(b"unexpected-data");
    assert!(
        http_response::parse_json_response_as::<BridgeHistoryResponse>(
            &response,
            crate::speech_input::safe_speech_error
        )
        .is_err()
    );
}

#[test]
fn direct_history_decode_preserves_safe_status_errors() {
    for (status, code, expected) in [
        (
            409,
            "session_missing",
            "desktop bridge request failed with status 409 (session_missing)",
        ),
        (
            429,
            "runtime_capacity",
            "desktop bridge request failed with status 429 (runtime_capacity)",
        ),
        (
            401,
            "unknown",
            "desktop bridge request failed with status 401",
        ),
        (
            502,
            "unknown",
            "desktop bridge request failed with status 502",
        ),
    ] {
        let response=format!("HTTP/1.1 {status} Error\r\n\r\n{{\"error\":{{\"code\":\"{code}\",\"message\":\"PRIVATE TRANSCRIPT\"}}}}");
        let direct = http_response::parse_json_response_as::<BridgeHistoryResponse>(
            response.as_bytes(),
            crate::speech_input::safe_speech_error,
        )
        .unwrap_err();
        let old = http_response::parse_json_response_as::<Value>(
            response.as_bytes(),
            crate::speech_input::safe_speech_error,
        )
        .unwrap_err();
        assert_eq!(direct, expected);
        assert_eq!(direct, old);
        assert!(!direct.contains("PRIVATE"));
    }
}

#[test]
fn direct_history_decode_refuses_missing_invalid_and_duplicate_known_fields() {
    let mut null_messages = fixture();
    null_messages["messages"] = Value::Null;
    let mut negative_sequence = fixture();
    negative_sequence["sequence"] = json!(-1);
    for body in [json!({}), null_messages, negative_sequence] {
        assert!(
            http_response::parse_json_response_as::<BridgeHistoryResponse>(
                &wire(&serde_json::to_vec(&body).unwrap()),
                crate::speech_input::safe_speech_error,
            )
            .is_err()
        );
    }
    let duplicate = serde_json::to_string(&fixture()).unwrap().replace(
        "\"protocolVersion\":1",
        "\"protocolVersion\":1,\"protocolVersion\":2",
    );
    let error = http_response::parse_json_response_as::<BridgeHistoryResponse>(
        &wire(duplicate.as_bytes()),
        crate::speech_input::safe_speech_error,
    )
    .unwrap_err();
    assert!(error.contains("duplicate field"), "{error}");
}

#[test]
fn direct_history_decode_keeps_body_and_http_byte_caps() {
    let oversized_body = vec![b' '; http_response::MAX_BRIDGE_RESPONSE_BODY_BYTES + 1];
    let response = wire(&oversized_body);
    let error = http_response::parse_json_response_as::<BridgeHistoryResponse>(
        &response,
        crate::speech_input::safe_speech_error,
    )
    .unwrap_err();
    assert_eq!(error, "desktop bridge response exceeds the size limit");
    let oversized_http = vec![0; http_response::MAX_BRIDGE_HTTP_RESPONSE_BYTES + 1];
    assert_eq!(
        http_response::parse_json_response_as::<BridgeHistoryResponse>(
            &oversized_http,
            crate::speech_input::safe_speech_error
        )
        .unwrap_err(),
        error
    );
}
