use super::*;
use serde_json::json;

fn context() -> Context<'static> {
    Context {
        protocol: 1,
        id: 7,
        version: "fixture <>&\n界",
        source_revision: "revision",
        source_fingerprint: "fingerprint",
    }
}

// The original 0.1.72 Value construction and optional-preview cap fallback.
fn legacy_success(
    context: Context<'_>,
    data: &SuccessData<'_, impl Serialize>,
    limit: usize,
) -> Vec<u8> {
    let mut value = json!({
        "protocol":context.protocol,"id":context.id,"driver":"cua","version":context.version,
        "result":data.result,"text":data.text,"images":data.images,"action":data.action,
        "is_error":data.is_error,"error_code":data.error_code,"preview":data.preview,
        "source_revision":context.source_revision,"source_fingerprint":context.source_fingerprint
    });
    let encoded = serde_json::to_vec(&value).unwrap();
    if encoded.len() <= limit {
        return encoded;
    }
    value.as_object_mut().unwrap().remove("preview");
    serde_json::to_vec(&value).unwrap()
}

#[test]
fn success_envelopes_match_every_original_byte_including_null_fields() {
    let empty = String::new();
    let first = "A".repeat(64 << 10);
    let second = "B/+=<>&\n界".repeat(32);
    let image_sets = [
        vec![],
        vec![("image/png", empty.as_str())],
        vec![
            ("image/png", first.as_str()),
            ("image/jpeg", second.as_str()),
        ],
    ];
    for raw in [
        None,
        Some("malformed"),
        Some("null"),
        Some("[]"),
        Some(r#"{"z":[1,2],"status":"refused","a":{"mode":"snapshot"}}"#),
    ] {
        let result = structured(raw);
        for action in [
            None,
            Some(json!({"route":"targeted","effect":"refused","delivery":{"b":2,"a":1}})),
            Some(json!({"effect":"unverifiable","route":"browser"})),
        ] {
            for preview in [
                None,
                Some(
                    json!({"source_image_index":0,"mime_type":"image/jpeg","data_base64":"tiny","width":1,"height":2}),
                ),
            ] {
                for images in &image_sets {
                    let data = || SuccessData {
                        result: &result,
                        text: "original text \"quotes\"\n界",
                        images: Images(images.iter().map(|(mime, data)| Image {
                            data_base64: data,
                            mime_type: mime,
                        })),
                        action: action.as_ref(),
                        is_error: refused(&result, action.as_ref()),
                        error_code: None,
                        preview: preview.as_ref(),
                    };
                    let expected = legacy_success(context(), &data(), usize::MAX);
                    let mut reply = Success::new(context(), data());
                    assert_eq!(reply.encode(usize::MAX).unwrap(), expected);
                    let parsed: Value = serde_json::from_slice(&expected).unwrap();
                    assert!(
                        parsed.get("action").is_some()
                            && parsed.get("error_code").is_some()
                            && parsed.get("preview").is_some()
                    );
                    assert!(parsed.get("completion").is_none() && parsed.get("error").is_none());
                }
            }
        }
    }
}

#[test]
fn preview_boundary_removes_only_the_optional_field() {
    let result = json!({"capture_id":"capture","mode":"snapshot"});
    let original = "A".repeat(5000);
    for preview in [
        None,
        Some(json!({"data_base64":"B".repeat(1000),"source_image_index":0})),
    ] {
        let data = || SuccessData {
            result: &result,
            text: "completed action",
            images: Images(std::iter::once(Image {
                data_base64: &original,
                mime_type: "image/png",
            })),
            action: None,
            is_error: false,
            error_code: Some("retained-code"),
            preview: preview.as_ref(),
        };
        let complete = legacy_success(context(), &data(), usize::MAX);
        let removed = legacy_success(context(), &data(), 0);
        for limit in [
            0,
            removed.len() - 1,
            removed.len(),
            complete.len() - 1,
            complete.len(),
            complete.len() + 1,
        ] {
            let expected = legacy_success(context(), &data(), limit);
            let mut reply = Success::new(context(), data());
            let actual = reply.encode(limit).unwrap();
            assert_eq!(actual, expected, "limit={limit}");
            let parsed: Value = serde_json::from_slice(&actual).unwrap();
            assert_eq!(parsed["images"][0]["data_base64"], original);
            assert_eq!(parsed["result"], result);
            assert_eq!(parsed["error_code"], "retained-code");
            assert_eq!(actual.len() > limit, expected.len() > limit);
        }
    }
}

#[test]
fn actual_transport_cap_keeps_original_facts() {
    const LIMIT: usize = 15 << 20;
    let original = "A".repeat(LIMIT);
    let result = json!({"status":"ok","capture_id":"never-truncate"});
    let data = || SuccessData {
        result: &result,
        text: "already dispatched",
        images: Images(std::iter::once(Image {
            data_base64: &original,
            mime_type: "image/png",
        })),
        action: None,
        is_error: false,
        error_code: None,
        preview: None,
    };
    let mut reply = Success::new(context(), data());
    let actual = reply.encode(LIMIT).unwrap();
    assert!(
        actual.len() > LIMIT,
        "caller must still stop on an oversized full reply"
    );
    assert_eq!(actual, legacy_success(context(), &data(), LIMIT));
    assert!(serde_json::from_slice::<Value>(&actual)
        .unwrap()
        .get("preview")
        .is_none());
}

#[test]
fn real_limit_keeps_exact_fits_and_drops_only_preview_when_one_byte_over() {
    const LIMIT: usize = 15 << 20;
    let result = json!({"capture_id":"complete"});
    fn make_data<'a>(
        result: &'a Value,
        data: &'a str,
    ) -> SuccessData<'a, Images<std::iter::Once<Image<'a>>>> {
        SuccessData {
            result,
            text: "complete",
            images: Images(std::iter::once(Image {
                data_base64: data,
                mime_type: "image/png",
            })),
            action: None,
            is_error: false,
            error_code: None,
            preview: None,
        }
    }
    let overhead = legacy_success(context(), &make_data(&result, ""), usize::MAX).len();
    for wanted in [LIMIT - 1, LIMIT, LIMIT + 1] {
        let image = "A".repeat(wanted - overhead);
        let mut reply = Success::new(context(), make_data(&result, &image));
        let actual = reply.encode(LIMIT).unwrap();
        assert_eq!(
            actual,
            legacy_success(context(), &make_data(&result, &image), LIMIT)
        );
        let value: Value = serde_json::from_slice(&actual).unwrap();
        assert_eq!(value.get("preview").is_some(), wanted <= LIMIT);
        assert_eq!(value["images"][0]["data_base64"], image);
    }
}

#[test]
fn refusal_and_driver_error_preserve_their_distinct_field_sets() {
    let context = context();
    for (reply, completion, code) in [
        (
            Failure::refused(context, "reason \"quoted\"\n界"),
            "not_started",
            "reasonix_target_refused",
        ),
        (
            Failure::driver_error(context, "reason \"quoted\"\n界"),
            "unknown",
            "reasonix_driver_error",
        ),
    ] {
        let original = json!({"protocol":context.protocol,"id":context.id,"driver":"cua","version":context.version,"is_error":true,"completion":completion,"error_code":code,"error":"reason \"quoted\"\n界","result":{},"images":[],"source_revision":context.source_revision,"source_fingerprint":context.source_fingerprint});
        let actual = reply.encode().unwrap();
        assert_eq!(actual, serde_json::to_vec(&original).unwrap());
        for field in ["action", "text", "preview"] {
            assert!(original.get(field).is_none());
        }
    }
}

#[test]
fn malformed_structured_data_and_refusal_detection_are_unchanged() {
    for raw in [None, Some("bad json"), Some("{\"status\":"), Some("null")] {
        assert_eq!(structured(raw), Value::Null);
    }
    assert!(refused(&json!({"status":"refused"}), None));
    assert!(refused(&Value::Null, Some(&json!({"effect":"refused"}))));
    assert!(!refused(
        &json!({"status":"ok"}),
        Some(&json!({"effect":"unverifiable"}))
    ));
    assert!(!refused(&json!(["refused"]), None));
}
