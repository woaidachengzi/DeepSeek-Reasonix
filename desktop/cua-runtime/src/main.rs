// Private adapter over the source-pinned MIT Cua SDK. No network listener.
mod browser;
mod identity;
mod request;
use cua_driver_sdk::{
    ConfiguredDriverOptions, CuaDriver, RuntimeAuthorizationOptions, SessionPermissionMode,
    TrustedSessionOptions,
};
use serde_json::{json, Value};
use std::io::{self, BufRead, Read, Write};
use std::sync::Arc;
const PROTOCOL: u32 = 1;
const SOURCE_REVISION: &str = env!("REASONIX_CUA_SOURCE_REVISION");
// Go's JSON encoder can escape a 16 KiB text field into 96 KiB on the wire.
const MAX_REQUEST: usize = 128 * 1024;

fn create_driver(
) -> Result<(Arc<CuaDriver>, Arc<browser::BrowserConsent>), cua_driver_sdk::DriverError> {
    let consent = Arc::new(browser::BrowserConsent::default());
    let driver = CuaDriver::create_configured_with_authorization_host(
        ConfiguredDriverOptions {
            claude_code_compatibility: false,
            authorization: RuntimeAuthorizationOptions {
                allowed_modes: vec![SessionPermissionMode::Standard],
                compatibility_mode: SessionPermissionMode::Standard,
                compatibility_capability_manifest_path: None,
                compatibility_bounded_manifest_path: None,
                unrestricted_acknowledged: false,
                max_session_ttl_seconds: 86400,
                max_idle_ttl_seconds: 3600,
            },
        },
        consent.clone(),
    )?;
    Ok((driver, consent))
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let (driver, consent) = create_driver()?;
    let session = driver.create_trusted_session(TrustedSessionOptions {
        public_session: "reasonix_browser".into(),
        mode: SessionPermissionMode::Standard,
        ttl_seconds: 28800,
        idle_ttl_seconds: 1800,
        capability_manifest_path: None,
        bounded_manifest_path: None,
    })?;
    let mut bindings = browser::Bindings::default();
    let version = driver.metadata().await?.driver_version;
    let stdin = io::stdin();
    let mut input = stdin.lock();
    let mut stdout = io::stdout().lock();
    let mut sequence = 0;
    loop {
        let mut line = Vec::new();
        let read = input
            .by_ref()
            .take((MAX_REQUEST + 1) as u64)
            .read_until(b'\n', &mut line)?;
        if read == 0 {
            break;
        }
        if line.len() > MAX_REQUEST || !line.ends_with(b"\n") {
            break;
        }
        let envelope: request::Envelope = match serde_json::from_slice(&line) {
            Ok(value) => value,
            Err(_) => break,
        };
        if envelope.protocol != PROTOCOL || envelope.id <= sequence {
            break;
        }
        sequence = envelope.id;
        let plan = envelope.plan().and_then(|plan| {
            if let Some(target) = &envelope.approved_target {
                identity::verify(target)?;
            }
            bindings.check(&envelope)?;
            Ok(plan)
        });
        let mut reply = match plan {
            Err(error) => {
                json!({"protocol":PROTOCOL,"id":sequence,"driver":"cua","version":version,"is_error":true,"completion":"not_started","error_code":"reasonix_target_refused","error":error,"result":{},"images":[]})
            }
            Ok((name, args)) => {
                consent.arm(&envelope);
                let result = if envelope.request.is_browser() {
                    session.call_tool(name.into(), args.to_string()).await
                } else {
                    driver.call_tool(name.into(), args.to_string()).await
                };
                consent.clear();
                match result {
                    Ok(result) => {
                        let structured = result
                            .structured_json
                            .as_deref()
                            .and_then(|v| serde_json::from_str::<Value>(v).ok())
                            .unwrap_or(Value::Null);
                        bindings.record(&envelope, &structured);
                        let refused = structured["status"] == "refused"
                            || result.action.as_ref().is_some_and(|action| {
                                serde_json::to_value(action).is_ok_and(|v| v["effect"] == "refused")
                            });
                        json!({"protocol":PROTOCOL,"id":sequence,"driver":"cua","version":version,"result":structured,"text":result.text,
                        "images":result.images.iter().map(|i|json!({"mime_type":i.mime_type,"data_base64":i.data_base64})).collect::<Vec<_>>(),
                        "action":result.action,"is_error":result.is_error || refused,"error_code":result.error_code})
                    }
                    Err(error) => {
                        json!({"protocol":PROTOCOL,"id":sequence,"driver":"cua","version":version,"is_error":true,"completion":"unknown","error_code":"reasonix_driver_error","error":error.to_string(),"result":{},"images":[]})
                    }
                }
            }
        };
        reply["source_revision"] = json!(SOURCE_REVISION);
        let encoded = serde_json::to_vec(&reply)?;
        if encoded.len() > 15 * 1024 * 1024 {
            break;
        }
        stdout.write_all(&encoded)?;
        stdout.write_all(b"\n")?;
        stdout.flush()?;
    }
    session.close();
    driver.shutdown().await?;
    Ok(())
}
