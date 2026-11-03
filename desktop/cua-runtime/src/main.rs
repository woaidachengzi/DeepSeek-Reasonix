// Private adapter over the source-pinned MIT Cua SDK. No network listener.
mod browser;
mod identity;
#[cfg(test)]
mod linux_region_tests;
mod preview;
mod reply;
mod request;
use cua_driver_sdk::{
    ConfiguredDriverOptions, CuaDriver, RuntimeAuthorizationOptions, SessionPermissionMode,
    TrustedSessionOptions,
};
use std::io::{self, BufRead, Read, Write};
use std::sync::Arc;
const PROTOCOL: u32 = 1;
const SOURCE_REVISION: &str = env!("REASONIX_CUA_SOURCE_REVISION");
const SOURCE_FINGERPRINT: &str = env!("REASONIX_CUA_SOURCE_FINGERPRINT");
// Go's JSON encoder can escape a 16 KiB text field into 96 KiB on the wire.
const MAX_REQUEST: usize = 128 * 1024;
const MAX_REPLY: usize = 15 * 1024 * 1024;

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
    let rust_preview = preview::enabled(std::env::var("REASONIX_CUA_RUST_PREVIEW").ok().as_deref());
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
        let context = reply::Context {
            protocol: PROTOCOL,
            id: sequence,
            version: &version,
            source_revision: SOURCE_REVISION,
            source_fingerprint: SOURCE_FINGERPRINT,
        };
        let encoded = match plan {
            Err(error) => reply::Failure::refused(context, &error).encode()?,
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
                        // Best-effort display work only: failure must not change
                        // the outcome of an already dispatched native action.
                        let preview = if rust_preview {
                            preview::for_observation(
                                &envelope.request.action,
                                result.images.iter().map(|image| {
                                    (image.mime_type.as_str(), image.data_base64.as_str())
                                }),
                            )
                        } else {
                            None
                        };
                        let structured = reply::structured(result.structured_json.as_deref());
                        bindings.record(&envelope, &structured);
                        let action = result
                            .action
                            .as_ref()
                            .map(serde_json::to_value)
                            .transpose()?;
                        let preview = preview.as_ref().map(serde_json::to_value).transpose()?;
                        let mut reply = reply::Success::new(
                            context,
                            reply::SuccessData {
                                result: &structured,
                                text: &result.text,
                                images: reply::Images(result.images.iter().map(|image| {
                                    reply::Image {
                                        data_base64: &image.data_base64,
                                        mime_type: &image.mime_type,
                                    }
                                })),
                                action: action.as_ref(),
                                is_error: result.is_error
                                    || reply::refused(&structured, action.as_ref()),
                                error_code: result.error_code.as_deref(),
                                preview: preview.as_ref(),
                            },
                        );
                        reply.encode(MAX_REPLY)?
                    }
                    Err(error) => {
                        reply::Failure::driver_error(context, &error.to_string()).encode()?
                    }
                }
            }
        };
        if encoded.len() > MAX_REPLY {
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
