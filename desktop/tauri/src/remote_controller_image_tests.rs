use super::super::tests::{controller, fixture, ID};
use super::*;

fn pixels(width: u32) -> String {
    let mut bytes = Vec::new();
    {
        let mut encoder = png::Encoder::new(&mut bytes, width, 1);
        encoder.set_color(png::ColorType::Rgba);
        encoder.set_depth(png::BitDepth::Eight);
        let mut writer = encoder.write_header().unwrap();
        writer
            .write_image_data(&vec![255; (width * 4) as usize])
            .unwrap();
    }
    format!("data:image/png;base64,{}", STANDARD.encode(bytes))
}
fn response() -> Value {
    json!({"protocolVersion":1,"controller":controller(),"view":{"protocolVersion":1,"sessionPath":"/remote/selected.jsonl","workspace":"/remote/actual","image":{"url":pixels(1),"mime":"image/png","size":100,"openHref":"file:///private-extra","config":"private-extra"}},"token":"private-extra"})
}
fn request() -> SessionImageRequest {
    SessionImageRequest {
        controller_id: ID.into(),
        session_path: "/remote/selected.jsonl".into(),
        source: "screen.png".into(),
    }
}
#[test]
fn remote_image_ipc_fixed_route_private_fields_and_narrow_request() {
    for extra in ["workspace", "url", "token", "localSessionId", "action"] {
        let mut value =
            json!({"controllerId":ID,"sessionPath":"/remote/selected.jsonl","source":"image.png"});
        value[extra] = json!("private");
        assert!(serde_json::from_value::<SessionImageRequest>(value).is_err());
    }
    let (client, task) = fixture(response());
    let result = client.session_image(request()).unwrap();
    let (headers, body) = task.join().unwrap();
    assert!(headers.starts_with(&format!(
        "POST /v1/remote/controllers/{ID}/session-image HTTP/1.1"
    )));
    assert_eq!(
        body,
        json!({"sessionPath":"/remote/selected.jsonl","source":"screen.png"})
    );
    let serialized = serde_json::to_string(&result).unwrap();
    assert!(!serialized.contains("private-extra") && !serialized.contains("openHref"));
}
#[test]
fn remote_image_ipc_rejects_scope_uri_dimensions_and_diagnostics() {
    for kind in [
        "version",
        "path",
        "workspace",
        "controller",
        "write",
        "url",
        "mime",
        "dimension",
        "encoding",
        "size",
        "filename",
        "diagnostic",
        "mixed",
    ] {
        let mut value = response();
        match kind {
            "version" => value["view"]["protocolVersion"] = json!(2),
            "path" => value["view"]["sessionPath"] = json!("/other"),
            "workspace" => value["view"]["workspace"] = json!("/other"),
            "controller" => value["controller"]["id"] = json!("BBBBBBBBBBBBBBBBBBBBBA"),
            "write" => value["controller"]["readOnly"] = json!(false),
            "url" => value["view"]["image"]["url"] = json!("file:///private"),
            "mime" => value["view"]["image"]["mime"] = json!("image/svg+xml"),
            "dimension" => value["view"]["image"]["url"] = json!(pixels(1201)),
            "encoding" => value["view"]["image"]["url"] = json!("data:image/png;base64,not-png"),
            "size" => value["view"]["image"]["size"] = json!((16 << 20) + 1),
            "filename" => value["view"]["image"]["filename"] = json!("private\nname"),
            "diagnostic" => value["view"]["image"] = json!({"url":"","errorCode":"PRIVATE ERROR"}),
            "mixed" => value["view"]["image"]["errorCode"] = json!("forbidden"),
            _ => unreachable!(),
        }
        let (client, task) = fixture(value);
        assert_eq!(
            client.session_image(request()).unwrap_err(),
            FAILED,
            "{kind}"
        );
        task.join().unwrap();
    }
    let mut value = response();
    value["view"]["image"] = json!({"url":"","errorCode":"forbidden"});
    let (client, task) = fixture(value);
    assert_eq!(
        client
            .session_image(request())
            .unwrap()
            .view
            .image
            .error_code
            .as_deref(),
        Some("forbidden")
    );
    task.join().unwrap();
}
#[test]
fn remote_image_ipc_invalid_sources_dispatch_zero_requests() {
    let client = RemoteControllerClient::new("127.0.0.1:1".parse().unwrap(), "owned".into());
    for source in [
        "".to_string(),
        " ".into(),
        "bad\0source".into(),
        "x".repeat(22370646),
    ] {
        let mut input = request();
        input.source = source;
        assert_eq!(client.session_image(input).unwrap_err(), INVALID);
    }
    let mut input = request();
    input.controller_id = "not-a-handle".into();
    assert_eq!(client.session_image(input).unwrap_err(), INVALID);
}
