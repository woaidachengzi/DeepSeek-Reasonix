use serde::Deserialize;
use serde_json::{json, Value};

#[derive(Debug, Clone, PartialEq, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Target {
    pub pid: i32,
    pub name: String,
    pub bundle_id: String,
    pub launch_time: f64,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Envelope {
    pub protocol: u32,
    pub id: u64,
    pub request: Request,
    pub approved_target: Option<Target>,
    #[serde(default)]
    pub browser_profile_approved: bool,
    #[serde(default)]
    pub foreground_approved: bool,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Request {
    pub action: String,
    #[serde(default)]
    pub pid: i32,
    #[serde(default)]
    pub window_id: u32,
    pub element_token: Option<String>,
    pub capture_id: Option<String>,
    pub x: Option<f64>,
    pub y: Option<f64>,
    pub text: Option<String>,
    pub key: Option<String>,
    pub keys: Option<Vec<String>>,
    pub direction: Option<String>,
    pub amount: Option<u32>,
    pub query: Option<String>,
    pub delivery_mode: Option<String>,
    pub target_id: Option<String>,
    pub tab_id: Option<String>,
    pub r#ref: Option<String>,
    pub continuation: Option<String>,
    #[serde(default)]
    pub include_screenshot: bool,
    #[serde(default)]
    pub replace: bool,
    pub url: Option<String>,
    pub label: Option<String>,
    pub value: Option<String>,
    pub gesture: Option<String>,
    pub delta_x: Option<f64>,
    pub delta_y: Option<f64>,
    pub destination_ref: Option<String>,
}

impl Envelope {
    pub fn plan(&self) -> Result<(&'static str, Value), String> {
        let r = &self.request;
        let foreground = r.delivery_mode.as_deref() == Some("foreground");
        if foreground != self.foreground_approved || foreground && self.approved_target.is_none() {
            return Err("foreground input requires fresh single-action host approval".into());
        }
        // A new Chrome DevTools connection can activate Chrome independently
        // of our input. Do not use upstream's activate-then-restore guard as a
        // promise of no interruption, even when the window is on another Space.
        if cfg!(target_os = "macos")
            && r.action == "browser_prepare"
            && self.browser_profile_approved
            && !foreground
        {
            return Err("macOS browser connection setup may activate the browser; pause and request foreground with fresh single-action confirmation, then use background for ordinary page work".into());
        }
        r.validate_browser_operation_fields()?;
        if r.action == "status" {
            return Ok(("check_permissions", json!({"prompt":false})));
        }
        if r.action == "apps" {
            return Ok(("list_apps", json!({})));
        }
        let target = self
            .approved_target
            .as_ref()
            .ok_or("missing approved app")?;
        if target.pid < 1
            || target.pid != r.pid
            || target.name.is_empty()
            || target.bundle_id.is_empty()
            || !target.launch_time.is_finite()
            || target.launch_time <= 0.
        {
            return Err("approved app identity is invalid".into());
        }
        if r.action == "windows" {
            return Ok(("list_windows", json!({"pid":r.pid})));
        }
        if r.window_id == 0 {
            return Err("select an exact window_id".into());
        }
        if r.is_browser() {
            return self.browser_plan();
        }
        if r.target_id.is_some()
            || r.tab_id.is_some()
            || r.r#ref.is_some()
            || r.continuation.is_some()
            || r.include_screenshot
            || r.replace
        {
            return Err("browser fields require a browser action".into());
        }
        if r.action == "observe" {
            if r.query.as_ref().is_some_and(|v| v.len() > 256) {
                return Err("query too long".into());
            }
            let mut args = json!({"pid":r.pid,"window_id":r.window_id,"tree_format":"both","max_elements":250,"max_dimension":1920,"timeout_ms":1000});
            if let Some(query) = &r.query {
                args["query"] = json!(query);
            }
            return Ok(("get_window_state", args));
        }
        self.input_plan()
    }

    fn input_plan(&self) -> Result<(&'static str, Value), String> {
        let r = &self.request;
        let mode = r.delivery_mode.as_deref().unwrap_or("background");
        if !matches!(mode, "background" | "foreground") {
            return Err("invalid delivery mode".into());
        }
        let mut args = json!({"target":{"kind":"window","pid":r.pid,"window_id":r.window_id},"delivery_mode":mode});
        let point = match (r.x, r.y) {
            (Some(x), Some(y)) if x.is_finite() && y.is_finite() && x >= 0. && y >= 0. => {
                Some((x, y))
            }
            (None, None) => None,
            _ => return Err("invalid screenshot coordinates".into()),
        };
        if let Some((x, y)) = point {
            args["x"] = json!(x);
            args["y"] = json!(y);
        }
        if let Some(token) = &r.element_token {
            if token.is_empty() || token.len() > 128 {
                return Err("invalid element token".into());
            }
            args["element_token"] = json!(token);
        }
        if let Some(capture) = &r.capture_id {
            if capture.is_empty() || capture.len() > 256 || r.action != "click" || point.is_none() {
                return Err("invalid capture id".into());
            }
            args["capture_id"] = json!(capture);
        }
        let name = match r.action.as_str() {
            "click" => {
                if r.element_token.is_some() == point.is_some() {
                    return Err("choose element or pixels".into());
                }
                args["delivery_mode"] = json!(mode);
                "click"
            }
            "type" => {
                let text = r
                    .text
                    .as_ref()
                    .filter(|v| !v.is_empty() && v.len() <= 16384)
                    .ok_or("invalid text")?;
                args["text"] = json!(text);
                "type_text"
            }
            "key" => {
                let key = r
                    .key
                    .as_ref()
                    .filter(|v| !v.is_empty() && v.len() <= 64)
                    .ok_or("invalid key")?;
                args["key"] = json!(key);
                "press_key"
            }
            "hotkey" => {
                let keys = r
                    .keys
                    .as_ref()
                    .filter(|v| {
                        (2..=4).contains(&v.len())
                            && v.iter().all(|s| !s.is_empty() && s.len() <= 64)
                    })
                    .ok_or("invalid keys")?;
                args["keys"] = json!(keys);
                "hotkey"
            }
            "scroll" => {
                if point.is_none()
                    || !matches!(r.direction.as_deref(), Some("up" | "down"))
                    || !r.amount.is_some_and(|n| (1..=20).contains(&n))
                {
                    return Err("invalid scroll".into());
                }
                args["direction"] = json!(r.direction);
                args["amount"] = json!(r.amount);
                "scroll"
            }
            "move" => {
                if cfg!(target_os = "linux") {
                    return Err("app-scoped native move is unavailable on Linux; use browser_pointer hover for an observed browser ref".into());
                }
                if point.is_none() {
                    return Err("move needs a screenshot point".into());
                }
                "move_cursor"
            }
            _ => return Err("tool is outside the Reasonix Cua adapter".into()),
        };
        Ok((name, args))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    fn envelope(request: Value) -> Envelope {
        serde_json::from_value(json!({"protocol":1,"id":1,"request":request,"approved_target":{"pid":42,"name":"Fixture","bundle_id":"test.fixture","launch_time":100.}})).unwrap()
    }
    #[test]
    fn foreground_requires_one_action_private_host_approval() {
        let mut e = envelope(
            json!({"action":"key","pid":42,"window_id":11,"key":"tab","delivery_mode":"foreground"}),
        );
        assert!(e
            .plan()
            .unwrap_err()
            .contains("single-action host approval"));
        e.foreground_approved = true;
        let (_, args) = e.plan().unwrap();
        assert_eq!(args["delivery_mode"], "foreground");
        assert!(args.get("foreground_approved").is_none());
        e.request.delivery_mode = None;
        assert!(
            e.plan().is_err(),
            "grant leaked into a later background request"
        );
        let mut prep = envelope(
            json!({"action":"browser_prepare","pid":42,"window_id":11,"delivery_mode":"foreground"}),
        );
        prep.browser_profile_approved = true;
        assert!(
            prep.plan().is_err(),
            "profile approval granted foreground access"
        );
        prep.foreground_approved = true;
        assert!(prep.plan().is_ok());
        assert!(serde_json::from_value::<Request>(
            json!({"action":"key","foreground_approved":true})
        )
        .is_err());
    }
    #[test]
    fn only_explicit_window_and_observed_tokens_reach_the_sdk() {
        let e = envelope(
            json!({"action":"click","pid":42,"window_id":11,"element_token":"s12345678:2"}),
        );
        let (name, args) = e.plan().unwrap();
        assert_eq!(name, "click");
        assert_eq!(
            args["target"],
            json!({"kind":"window","pid":42,"window_id":11})
        );
        assert_eq!(args["delivery_mode"], "background");
        for bad in [
            json!({"action":"click","pid":99,"window_id":11,"x":1,"y":2}),
            json!({"action":"click","pid":42,"window_id":0,"x":1,"y":2}),
            json!({"action":"click","pid":42,"window_id":11,"x":1,"y":2,"element_token":"s12345678:2"}),
            json!({"action":"shell_execute","pid":42,"window_id":11}),
        ] {
            assert!(envelope(bad).plan().is_err());
        }
    }
    #[test]
    fn agent_cannot_supply_reserved_driver_or_file_arguments() {
        for key in [
            "session",
            "target",
            "scope",
            "screenshot_out_file",
            "_approved",
            "script",
            "browser_profile_approved",
            "profile",
            "endpoint",
        ] {
            let mut v = json!({"action":"observe","pid":42,"window_id":11});
            v[key] = json!("untrusted");
            assert!(serde_json::from_value::<Request>(v).is_err());
        }
        let (_, args) = envelope(json!({"action":"observe","pid":42,"window_id":11}))
            .plan()
            .unwrap();
        assert_eq!(args["max_dimension"], 1920);
        assert!(args.get("screenshot_out_file").is_none());
    }

    #[tokio::test]
    async fn adapter_arguments_match_the_actual_pinned_sdk_inventory() {
        let (driver, _) = crate::create_driver().unwrap();
        let inventory: Value =
            serde_json::from_str(&driver.list_tools_json().await.unwrap()).unwrap();
        let tools = inventory["tools"].as_array().unwrap();
        for fields in [
            json!({"action":"status"}),
            json!({"action":"apps"}),
            json!({"action":"windows"}),
            json!({"action":"observe","query":"field"}),
            json!({"action":"click","element_token":"s00000001:1"}),
            json!({"action":"click","x":100.,"y":80.,"capture_id":"capture_test"}),
            json!({"action":"type","element_token":"s00000001:1","text":"test"}),
            json!({"action":"key","element_token":"s00000001:1","key":"tab"}),
            json!({"action":"hotkey","keys":["command","a"]}),
            json!({"action":"scroll","x":100.,"y":80.,"direction":"down","amount":2}),
            json!({"action":"move","x":100.,"y":80.}),
            json!({"action":"browser_observe"}),
            json!({"action":"browser_observe","target_id":"b1","tab_id":"t1","ref":"p1:e1","query":"field","include_screenshot":true,"continuation":"next"}),
            json!({"action":"browser_click","target_id":"b1","tab_id":"t1","ref":"p1:e1"}),
            json!({"action":"browser_type","target_id":"b1","tab_id":"t1","ref":"p1:e1","text":"test","replace":true}),
            json!({"action":"browser_navigate","target_id":"b1","tab_id":"t1","url":"https://example.test"}),
            json!({"action":"browser_select","target_id":"b1","tab_id":"t1","ref":"p1:e1","value":""}),
            json!({"action":"browser_pointer","target_id":"b1","tab_id":"t1","ref":"p1:e1","gesture":"hover"}),
            json!({"action":"browser_pointer","target_id":"b1","tab_id":"t1","ref":"p1:e1","gesture":"scroll","delta_y":350.}),
            json!({"action":"browser_pointer","target_id":"b1","tab_id":"t1","ref":"p1:e1","gesture":"drag","destination_ref":"p1:e2"}),
        ] {
            let unavailable_move = cfg!(target_os = "linux") && fields["action"] == "move";
            let mut request = fields;
            request["pid"] = json!(42);
            request["window_id"] = json!(11);
            let planned = envelope(request).plan();
            if unavailable_move {
                assert!(planned.unwrap_err().contains("app-scoped native move"));
                continue;
            }
            let (name, args) = planned.unwrap();
            let schema = &tools.iter().find(|tool| tool["name"] == name).unwrap()["inputSchema"];
            let properties = schema["properties"].as_object().unwrap();
            for key in args.as_object().unwrap().keys() {
                assert!(properties.contains_key(key), "{name} does not accept {key}");
            }
            if let Some(required) = schema["required"].as_array() {
                for key in required {
                    if key == "session" && name.starts_with("browser_") {
                        // TrustedSession supplies its private transport session.
                        assert!(args.get("session").is_none());
                        continue;
                    }
                    assert!(
                        args.get(key.as_str().unwrap()).is_some(),
                        "{name} lacks {key}"
                    );
                }
            }
        }
        let mut prep = envelope(json!({"action":"browser_prepare","pid":42,"window_id":11}));
        prep.browser_profile_approved = true;
        if cfg!(target_os = "macos") {
            assert!(prep
                .plan()
                .unwrap_err()
                .contains("connection setup may activate"));
            prep.request.delivery_mode = Some("foreground".into());
            prep.foreground_approved = true;
        }
        let (name, args) = prep.plan().unwrap();
        let schema = &tools.iter().find(|tool| tool["name"] == name).unwrap()["inputSchema"];
        assert!(args
            .as_object()
            .unwrap()
            .keys()
            .all(|key| schema["properties"].get(key).is_some()));
        let session = driver
            .create_trusted_session(cua_driver_sdk::TrustedSessionOptions {
                public_session: "reasonix_browser_test".into(),
                mode: cua_driver_sdk::SessionPermissionMode::Standard,
                ttl_seconds: 60,
                idle_ttl_seconds: 60,
                capability_manifest_path: None,
                bounded_manifest_path: None,
            })
            .unwrap();
        let reply = session
            .call_tool(
                "get_browser_state".into(),
                json!({"pid":2147483647,"window_id":1}).to_string(),
            )
            .await
            .unwrap();
        assert!(
            !reply.text.contains("declare an explicit session"),
            "{}",
            reply.text
        );
        let state: Value = serde_json::from_str(&reply.structured_json.unwrap()).unwrap();
        assert_eq!(state["status"], "refused");
        let expected = if cfg!(target_os = "linux") {
            "browser_binding_stale"
        } else {
            "browser_route_unavailable"
        };
        assert_eq!(state["refusal"]["code"], expected);
        session.close();
        driver.shutdown().await.unwrap();
    }
}
