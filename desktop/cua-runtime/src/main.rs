// Private adapter over the source-pinned MIT Cua SDK. No network listener.
mod identity;
mod request;
use cua_driver_sdk::CuaDriver;
use serde_json::{json, Value};
use std::io::{self, BufRead, Read, Write};
const PROTOCOL: u32 = 1;
const SOURCE_REVISION: &str = env!("REASONIX_CUA_SOURCE_REVISION");
const MAX_REQUEST: usize = 64 * 1024;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let driver = CuaDriver::create(None)?;
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
            Ok(plan)
        });
        let mut reply = match plan {
            Err(error) => {
                json!({"protocol":PROTOCOL,"id":sequence,"driver":"cua","version":version,"is_error":true,"completion":"not_started","error_code":"reasonix_target_refused","error":error,"result":{},"images":[]})
            }
            Ok((name, args)) => match driver.call_tool(name.into(), args.to_string()).await {
                Ok(result) => {
                    let structured = result
                        .structured_json
                        .as_deref()
                        .and_then(|v| serde_json::from_str::<Value>(v).ok())
                        .unwrap_or(Value::Null);
                    json!({"protocol":PROTOCOL,"id":sequence,"driver":"cua","version":version,"result":structured,"text":result.text,
                        "images":result.images.iter().map(|i|json!({"mime_type":i.mime_type,"data_base64":i.data_base64})).collect::<Vec<_>>(),
                        "action":result.action,"is_error":result.is_error,"error_code":result.error_code})
                }
                Err(error) => {
                    json!({"protocol":PROTOCOL,"id":sequence,"driver":"cua","version":version,"is_error":true,"completion":"unknown","error_code":"reasonix_driver_error","error":error.to_string(),"result":{},"images":[]})
                }
            },
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
    driver.shutdown().await?;
    Ok(())
}
