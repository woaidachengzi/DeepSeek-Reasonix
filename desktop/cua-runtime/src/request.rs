use serde::Deserialize;
use serde_json::{json, Value};

#[derive(Debug, Deserialize)]
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
}

impl Envelope {
    pub fn plan(&self) -> Result<(&'static str, Value), String> {
        let r = &self.request;
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
        let driver = cua_driver_sdk::CuaDriver::create(None).unwrap();
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
        ] {
            let mut request = fields;
            request["pid"] = json!(42);
            request["window_id"] = json!(11);
            let (name, args) = envelope(request).plan().unwrap();
            let schema = &tools.iter().find(|tool| tool["name"] == name).unwrap()["inputSchema"];
            let properties = schema["properties"].as_object().unwrap();
            for key in args.as_object().unwrap().keys() {
                assert!(properties.contains_key(key), "{name} does not accept {key}");
            }
            if let Some(required) = schema["required"].as_array() {
                for key in required {
                    assert!(
                        args.get(key.as_str().unwrap()).is_some(),
                        "{name} lacks {key}"
                    );
                }
            }
        }
        driver.shutdown().await.unwrap();
    }
}
