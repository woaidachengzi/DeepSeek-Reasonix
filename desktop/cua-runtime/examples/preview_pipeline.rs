//! Offline benchmark producer. Never creates a driver, captures or sends input.
//! Build: cargo build --release --locked --offline --example preview_pipeline
#[path = "../src/preview.rs"]
mod preview;

use serde::Deserialize;
use serde_json::json;
use std::io::{self, BufRead, Write};

#[derive(Deserialize)]
struct Fixture {
    mime_type: String,
    data_base64: String,
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut input = io::stdin().lock().lines();
    let fixture: Fixture = serde_json::from_str(&input.next().ok_or("missing fixture")??)?;
    let mut output = io::stdout().lock();
    for (id, line) in input.enumerate() {
        let mode = line?;
        if !matches!(mode.as_str(), "legacy" | "rust") {
            return Err("unsupported benchmark mode".into());
        }
        let thumbnail = if mode == "rust" {
            preview::for_observation(
                "observe",
                std::iter::once((fixture.mime_type.as_str(), fixture.data_base64.as_str())),
            )
        } else {
            None
        };
        let mut reply = json!({
            "protocol":1,"id":id + 1,"driver":"cua","version":"fixture",
            "source_revision":"fixture","source_fingerprint":"fixture",
            "result":{"mode":"snapshot","effect":"unverifiable"},
            "text":"Offline benchmark observation",
            "images":[{"mime_type":fixture.mime_type,"data_base64":fixture.data_base64}],
            "is_error":false
        });
        if let Some(thumbnail) = thumbnail {
            reply["preview"] = serde_json::to_value(thumbnail)?;
        }
        output.write_all(&preview::encode_reply(&mut reply, 15 * 1024 * 1024)?)?;
        output.write_all(b"\n")?;
        output.flush()?;
    }
    Ok(())
}
