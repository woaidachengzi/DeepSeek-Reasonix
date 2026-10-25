//! Typed browser routes only: no raw CDP endpoints, JS, profiles or session IDs.
use crate::request::{Envelope, Request, Target};
use cua_driver_sdk::{
    DriverAuthorizationAction, DriverAuthorizationDecision, DriverAuthorizationHost,
    DriverAuthorizationHostError, DriverAuthorizationRequest,
};
use serde_json::{json, Value};
use std::collections::{HashMap, HashSet};
use std::sync::Mutex;

impl Request {
    pub fn is_browser(&self) -> bool {
        matches!(
            self.action.as_str(),
            "browser_prepare"
                | "browser_observe"
                | "browser_click"
                | "browser_type"
                | "browser_navigate"
                | "browser_select"
                | "browser_pointer"
        )
    }

    pub fn validate_browser_operation_fields(&self) -> Result<(), String> {
        if self.action == "browser_navigate" {
            let raw = self.url.as_deref().ok_or("navigation needs a URL")?;
            let parsed = url::Url::parse(raw).map_err(|_| "invalid navigation URL")?;
            let authority = raw
                .split_once("://")
                .map(|(_, rest)| rest.split(['/', '?', '#']).next().unwrap_or(""));
            if raw.len() > 8192
                || raw
                    .chars()
                    .any(|c| c.is_ascii_control() || c == ' ' || c == '\\')
                || !parsed.username().is_empty()
                || parsed.password().is_some()
                || !(matches!(parsed.scheme(), "http" | "https")
                    && parsed.host_str().is_some()
                    && authority.is_some_and(|s| !s.is_empty() && !s.contains('@'))
                    && raw
                        .to_ascii_lowercase()
                        .starts_with(&format!("{}://", parsed.scheme()))
                    || raw == "about:blank")
            {
                return Err("navigation requires an absolute http/https URL without credentials, or about:blank".into());
            }
        } else if self.url.is_some() {
            return Err("url requires browser_navigate".into());
        }
        if self.action == "browser_select" {
            if self.label.is_some() == self.value.is_some()
                || self.label.as_ref().is_some_and(|s| s.len() > 16384)
                || self.value.as_ref().is_some_and(|s| s.len() > 16384)
            {
                return Err("selection requires exactly one bounded label or value".into());
            }
        } else if self.label.is_some() || self.value.is_some() {
            return Err("label and value require browser_select".into());
        }
        if self.action != "browser_pointer" {
            return if self.gesture.is_some()
                || self.delta_x.is_some()
                || self.delta_y.is_some()
                || self.destination_ref.is_some()
            {
                Err("gesture, deltas and destination_ref require browser_pointer".into())
            } else {
                Ok(())
            };
        }
        match self.gesture.as_deref() {
            Some("scroll") => {
                if self.destination_ref.is_some()
                    || self.delta_x.is_none() && self.delta_y.is_none()
                    || [self.delta_x, self.delta_y]
                        .into_iter()
                        .flatten()
                        .any(|v| !v.is_finite() || v.abs() > 10000.)
                    || ![self.delta_x, self.delta_y]
                        .into_iter()
                        .flatten()
                        .any(|v| v != 0.)
                {
                    return Err("scroll needs nonzero finite CSS pixel deltas within 10000 and no destination".into());
                }
            }
            Some("drag") => {
                if !self
                    .destination_ref
                    .as_ref()
                    .is_some_and(|s| !s.is_empty() && s.len() <= 256)
                    || self.delta_x.is_some()
                    || self.delta_y.is_some()
                {
                    return Err("drag needs a destination_ref and no deltas".into());
                }
            }
            Some("hover" | "right_click" | "double_click") => {
                if self.delta_x.is_some()
                    || self.delta_y.is_some()
                    || self.destination_ref.is_some()
                {
                    return Err("this pointer gesture accepts no deltas or destination".into());
                }
            }
            _ => return Err("unsupported browser gesture".into()),
        }
        Ok(())
    }
}

impl Envelope {
    pub fn browser_plan(&self) -> Result<(&'static str, Value), String> {
        let r = &self.request;
        if r.x.is_some()
            || r.y.is_some()
            || r.element_token.is_some()
            || r.capture_id.is_some()
            || r.key.is_some()
            || r.keys.is_some()
            || r.direction.is_some()
            || r.amount.is_some()
        {
            return Err("browser actions require DOM refs, not native input arguments".into());
        }
        for (value, max) in [
            (&r.target_id, 128),
            (&r.tab_id, 128),
            (&r.r#ref, 256),
            (&r.continuation, 4096),
            (&r.query, 256),
        ] {
            if value
                .as_ref()
                .is_some_and(|v| v.is_empty() || v.len() > max)
            {
                return Err("invalid browser argument length".into());
            }
        }
        if r.target_id.is_some() != r.tab_id.is_some() {
            return Err("target_id and tab_id must be paired".into());
        }
        let mode = r.delivery_mode.as_deref().unwrap_or("background");
        if !matches!(mode, "background" | "foreground") {
            return Err("invalid delivery mode".into());
        }
        if r.action != "browser_type" && (r.text.is_some() || r.replace) {
            return Err("text and replace require browser_type".into());
        }
        if r.action == "browser_prepare" {
            if !self.browser_profile_approved
                || r.target_id.is_some()
                || r.r#ref.is_some()
                || r.continuation.is_some()
                || r.query.is_some()
                || r.include_screenshot
            {
                return Err(
                    "browser attachment requires separate host approval for this exact window"
                        .into(),
                );
            }
            return Ok((
                "browser_prepare",
                json!({"pid":r.pid,"window_id":r.window_id,
                "strategy":{"kind":"existing_profile"},"delivery_mode":mode}),
            ));
        }
        let mut args = json!({});
        if let (Some(target), Some(tab)) = (&r.target_id, &r.tab_id) {
            args["target_id"] = json!(target);
            args["tab_id"] = json!(tab);
        } else if r.action == "browser_observe"
            && r.r#ref.is_none()
            && r.continuation.is_none()
            && r.query.is_none()
            && !r.include_screenshot
        {
            return Ok((
                "get_browser_state",
                json!({"pid":r.pid,"window_id":r.window_id}),
            ));
        } else {
            return Err("bind browser first, then select an observed tab".into());
        }
        if r.action == "browser_observe" {
            args["snapshot_format"] = json!("semantic_v2");
            args["include_screenshot"] = json!(r.include_screenshot);
            for (key, value) in [
                ("scope_ref", &r.r#ref),
                ("continuation", &r.continuation),
                ("query", &r.query),
            ] {
                if let Some(value) = value {
                    args[key] = json!(value);
                }
            }
            return Ok(("get_browser_state", args));
        }
        if r.query.is_some() || r.continuation.is_some() || r.include_screenshot {
            return Err("snapshot fields require browser_observe".into());
        }
        if r.action == "browser_navigate" {
            if r.r#ref.is_some() || r.delivery_mode.is_some() {
                return Err("navigation requires an observed target/tab and URL only".into());
            }
            args["url"] = json!(r.url);
            return Ok(("browser_navigate", args));
        }
        args["ref"] = json!(r
            .r#ref
            .as_ref()
            .ok_or("browser input needs an observed DOM ref")?);
        match r.action.as_str() {
            "browser_click" => {
                args["delivery_mode"] = json!(mode);
                args["input_route"] = json!("trusted");
                Ok(("browser_click", args))
            }
            "browser_type" => {
                // Go omits empty text, including the explicit clear operation.
                let text = r.text.as_deref().unwrap_or("");
                if text.len() > 16384 || (text.is_empty() && !r.replace) {
                    return Err("invalid browser text".into());
                }
                args["text"] = json!(text);
                args["mode"] = json!("insert_text");
                args["replace"] = json!(r.replace);
                Ok(("browser_type", args))
            }
            "browser_select" => {
                if r.delivery_mode.is_some() {
                    return Err("selection does not support delivery_mode".into());
                }
                if let Some(label) = &r.label {
                    args["label"] = json!(label);
                }
                if let Some(value) = &r.value {
                    args["value"] = json!(value);
                }
                Ok(("browser_select", args))
            }
            "browser_pointer" => {
                args["action"] = json!(r.gesture);
                args["input_route"] = json!("trusted");
                args["delivery_mode"] = json!(mode);
                if let Some(delta) = r.delta_x {
                    args["delta_x"] = json!(delta);
                }
                if let Some(delta) = r.delta_y {
                    args["delta_y"] = json!(delta);
                }
                if let Some(destination) = &r.destination_ref {
                    args["destination_ref"] = json!(destination);
                }
                Ok(("browser_pointer", args))
            }
            _ => Err("unsupported browser action".into()),
        }
    }
}

struct Binding {
    app: Target,
    window: u32,
    tabs: HashSet<String>,
}
#[derive(Default)]
pub struct Bindings {
    targets: HashMap<String, Binding>,
}
impl Bindings {
    pub fn check(&self, e: &Envelope) -> Result<(), String> {
        if !e.request.is_browser() {
            return Ok(());
        }
        if let Some(id) = &e.request.target_id {
            let binding = self
                .targets
                .get(id)
                .ok_or("browser binding is not from this task; bind again")?;
            if Some(&binding.app) != e.approved_target.as_ref()
                || binding.window != e.request.window_id
                || !e
                    .request
                    .tab_id
                    .as_ref()
                    .is_some_and(|tab| binding.tabs.contains(tab))
            {
                return Err("browser target/tab does not belong to the approved app window".into());
            }
        }
        Ok(())
    }
    pub fn record(&mut self, e: &Envelope, result: &Value) {
        if e.request.action != "browser_observe"
            || e.request.target_id.is_some()
            || result["status"] != "ok"
            || result["mode"] != "bind"
        {
            return;
        }
        let Some(id) = result["target_id"].as_str().filter(|s| s.len() <= 128) else {
            return;
        };
        let Some(app) = &e.approved_target else {
            return;
        };
        // Rebinding a window replaces its old IDs. Cap retained native windows.
        self.targets
            .retain(|_, b| b.app != *app || b.window != e.request.window_id);
        if self.targets.len() >= 16 {
            self.targets.clear();
        }
        let tabs = result["tabs"]
            .as_array()
            .into_iter()
            .flatten()
            .take(256)
            .filter_map(|tab| {
                tab["tab_id"]
                    .as_str()
                    .filter(|s| s.len() <= 128)
                    .map(str::to_owned)
            })
            .collect();
        self.targets.insert(
            id.to_owned(),
            Binding {
                app: app.clone(),
                window: e.request.window_id,
                tabs,
            },
        );
    }
}

// Only the host-controlled envelope authorizes setup. Model JSON cannot set it.
// Clear the short-lived grant after each SDK call, even when prepare is refused.
#[derive(Default)]
pub struct BrowserConsent {
    approved: Mutex<Option<(Target, u32)>>,
}
impl BrowserConsent {
    pub fn arm(&self, e: &Envelope) {
        *self.approved.lock().unwrap() =
            if e.browser_profile_approved && e.request.action == "browser_prepare" {
                e.approved_target
                    .clone()
                    .map(|app| (app, e.request.window_id))
            } else {
                None
            };
    }
    pub fn clear(&self) {
        *self.approved.lock().unwrap() = None;
    }
    fn permits(&self, request: &DriverAuthorizationRequest) -> bool {
        let guard = self.approved.lock().unwrap();
        guard.as_ref().is_some_and(|(app, window)| {
            matches_resource(app, *window, request) && crate::identity::verify(app).is_ok()
        })
    }
}
#[async_trait::async_trait]
impl DriverAuthorizationHost for BrowserConsent {
    async fn authorize(
        &self,
        request: DriverAuthorizationRequest,
    ) -> Result<DriverAuthorizationDecision, DriverAuthorizationHostError> {
        Ok(DriverAuthorizationDecision {
            action: if self.permits(&request) {
                DriverAuthorizationAction::Allow
            } else {
                DriverAuthorizationAction::Deny
            },
            request_digest: request.request_digest,
        })
    }
}

fn matches_resource(app: &Target, window: u32, request: &DriverAuthorizationRequest) -> bool {
    if request.adapter_id != "browser_prepare.existing_profile"
        || request.permission_mode != "standard"
    {
        return false;
    }
    let Ok(resource) = serde_json::from_str::<Value>(&request.resource_json) else {
        return false;
    };
    resource["pid"].as_i64() == Some(i64::from(app.pid))
        && resource["endpoint_owner_pid"].as_i64() == Some(i64::from(app.pid))
        && resource["window_id"].as_u64() == Some(u64::from(window))
}

#[cfg(test)]
mod tests {
    use super::*;
    fn envelope(request: Value) -> Envelope {
        serde_json::from_value(json!({"protocol":1,"id":1,"request":request,
            "approved_target":{"pid":42,"name":"Fixture","bundle_id":"test.fixture","launch_time":100.}})).unwrap()
    }
    #[test]
    fn browser_operation_wire_contract_matches_the_host() {
        let cases: Value = serde_json::from_str(include_str!(
            "../../../internal/computeruse/testdata/cua_browser_operations.json"
        ))
        .unwrap();
        for case in cases.as_array().unwrap() {
            let planned = envelope(case["request"].clone()).plan();
            assert_eq!(
                planned.is_ok(),
                case["valid"].as_bool().unwrap(),
                "{}: {:?}",
                case["name"],
                planned
            );
            if case["valid"] == true && case["request"]["action"] == "browser_pointer" {
                let (tool, args) = planned.unwrap();
                assert_eq!(tool, "browser_pointer");
                assert_eq!(args["input_route"], "trusted");
                assert_eq!(args["action"], case["request"]["gesture"]);
                assert!(args.get("x").is_none() && args.get("y").is_none());
            }
        }
    }
    #[test]
    fn dom_routes_do_not_accept_native_pixels_or_authority_overrides() {
        let e = envelope(json!({"action":"browser_prepare","pid":42,"window_id":11}));
        assert!(e.plan().is_err());
        let mut approved = e;
        approved.browser_profile_approved = true;
        let (_, args) = approved.plan().unwrap();
        assert_eq!(args["strategy"], json!({"kind":"existing_profile"}));
        for bad in [
            json!({"action":"browser_click","pid":42,"window_id":11,"x":1,"y":2}),
            json!({"action":"browser_type","pid":42,"window_id":11,"target_id":"b1","tab_id":"t1","ref":"p1:e1","text":""}),
            json!({"action":"browser_observe","pid":42,"window_id":11,"target_id":"b1"}),
            json!({"action":"browser_observe","pid":42,"window_id":11,"include_screenshot":true}),
            json!({"action":"browser_click","pid":42,"window_id":11,"target_id":"b1","tab_id":"t1","ref":"p1:e1","continuation":"x"}),
        ] {
            assert!(envelope(bad).plan().is_err());
        }
        let (_,args) = envelope(json!({"action":"browser_type","pid":42,"window_id":11,"target_id":"b1","tab_id":"t1","ref":"p1:e1","replace":true})).plan().unwrap();
        assert_eq!(args["text"], "");
        assert_eq!(args["replace"], true);
        assert!(args.get("session").is_none());
    }
    #[test]
    fn browser_ids_are_bound_to_native_window_and_process_incarnation() {
        let bind = envelope(json!({"action":"browser_observe","pid":42,"window_id":11}));
        let result = json!({"status":"ok","mode":"bind","target_id":"b1","tabs":[{"tab_id":"t1"}]});
        let mut bindings = Bindings::default();
        bindings.record(&bind, &result);
        let mut click = envelope(
            json!({"action":"browser_click","pid":42,"window_id":11,"target_id":"b1","tab_id":"t1","ref":"p1:e1"}),
        );
        assert!(bindings.check(&click).is_ok());
        click.request.window_id = 12;
        assert!(bindings.check(&click).is_err());
        click.request.window_id = 11;
        click.request.tab_id = Some("other_tab".into());
        assert!(bindings.check(&click).is_err());
        click.request.tab_id = Some("t1".into());
        click.approved_target.as_mut().unwrap().launch_time = 101.;
        assert!(bindings.check(&click).is_err());
        click.approved_target.as_mut().unwrap().launch_time = 100.;
        bindings.record(
            &bind,
            &json!({"status":"ok","mode":"bind","target_id":"b2","tabs":[{"tab_id":"t2"}]}),
        );
        assert!(bindings.check(&click).is_err());
    }
    #[test]
    fn profile_consent_does_not_cover_other_residual_operations_or_windows() {
        let e = envelope(json!({"action":"browser_prepare","pid":42,"window_id":11}));
        let app = e.approved_target.as_ref().unwrap();
        let mut request = DriverAuthorizationRequest {
            schema: "cua-driver-authorization-request-v1".into(),
            nonce: "nonce".into(),
            generation: 1,
            daemon_instance: "private".into(),
            permission_mode: "standard".into(),
            managed_policy_sha256: None,
            user_policy_sha256: None,
            adapter_id: "browser_prepare.existing_profile".into(),
            risk_class: "R2".into(),
            public_session: "reasonix_browser".into(),
            transport_session: "private".into(),
            resource_json: json!({"pid":42,"window_id":11,"endpoint_owner_pid":42}).to_string(),
            human_summary: "test".into(),
            expires_unix_ms: u64::MAX,
            request_digest: "digest".into(),
        };
        assert!(matches_resource(app, 11, &request));
        assert!(!matches_resource(app, 12, &request));
        request.resource_json =
            json!({"pid":42,"window_id":11,"endpoint_owner_pid":99}).to_string();
        assert!(!matches_resource(app, 11, &request));
        request.adapter_id = "browser_set_input_files".into();
        assert!(!matches_resource(app, 11, &request));
        let consent = BrowserConsent::default();
        consent.arm(&e); // no host profile approval
        assert!(!consent.permits(&request));
        consent.clear();
    }
}
