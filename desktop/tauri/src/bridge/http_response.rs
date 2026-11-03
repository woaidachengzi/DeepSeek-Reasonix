//! One bounded HTTP response decoder for typed and legacy Value callers.
use serde::de::DeserializeOwned;
use serde_json::Value;

pub(crate) const MAX_BRIDGE_RESPONSE_BODY_BYTES: usize = 32 * 1024 * 1024;
pub(crate) const MAX_BRIDGE_HTTP_RESPONSE_BYTES: usize = 64 * 1024 * 1024;

pub(crate) fn parse_json_response_as<T: DeserializeOwned>(
    response: &[u8],
    safe_speech_error: fn(u16, &str) -> Option<String>,
) -> Result<T, String> {
    if response.len() > MAX_BRIDGE_HTTP_RESPONSE_BYTES {
        return Err("desktop bridge response exceeds the size limit".to_string());
    }
    let boundary = b"\r\n\r\n";
    let Some(index) = response
        .windows(boundary.len())
        .position(|window| window == boundary)
    else {
        return Err("desktop bridge returned an invalid HTTP response".to_string());
    };
    let headers = std::str::from_utf8(&response[..index]).map_err(|error| error.to_string())?;
    let status = headers
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse::<u16>().ok())
        .ok_or_else(|| "desktop bridge returned an invalid HTTP status".to_string())?;
    let body = &response[index + boundary.len()..];
    let transfer_encoding = headers.lines().skip(1).find_map(|line| {
        let (name, value) = line.split_once(':')?;
        name.trim()
            .eq_ignore_ascii_case("transfer-encoding")
            .then_some(value.trim())
    });
    let decoded_body = transfer_encoding
        .map(|encoding| {
            if encoding.eq_ignore_ascii_case("chunked") {
                decode_chunked_body(body)
            } else {
                Err("desktop bridge returned an unsupported transfer encoding".to_string())
            }
        })
        .transpose()?;
    if transfer_encoding.is_none() && body.len() > MAX_BRIDGE_RESPONSE_BODY_BYTES {
        return Err("desktop bridge response exceeds the size limit".to_string());
    }
    let raw = decoded_body.as_deref().unwrap_or(body);
    if !(200..300).contains(&status) {
        if let Ok(parsed) = serde_json::from_slice::<Value>(first_json_value(raw)) {
            if let Some(code) = parsed.pointer("/error/code").and_then(Value::as_str) {
                if let Some(message) = safe_speech_error(status, code) {
                    return Err(message);
                }
            }
        }
        // Remote bootstrap explicitly publishes a bounded, credential-redacted
        // diagnostic. This exception must not expose other endpoints' messages.
        if status == 502 {
            if let Ok(parsed) = serde_json::from_slice::<Value>(first_json_value(raw)) {
                if parsed.pointer("/error/code").and_then(Value::as_str)
                    == Some("remote_serve_start_failed")
                {
                    if let Some(detail) = parsed.pointer("/error/detail").and_then(Value::as_str) {
                        if !detail.trim().is_empty() && detail.chars().count() <= 2049 {
                            return Err(format!(
                                "desktop bridge request failed with status 502 (remote_serve_start_failed): {}",
                                detail.trim()
                            ));
                        }
                    }
                }
            }
        }
        // Carry only a small allowlist of recovery codes across the Tauri
        // String error boundary. Never forward the bridge's free-text message:
        // it may contain a private transcript or workspace path.
        if matches!(status, 400 | 404 | 409 | 422 | 429 | 503) {
            let parsed = serde_json::from_slice::<Value>(first_json_value(raw)).ok();
            let code = parsed
                .as_ref()
                .and_then(|value| value.pointer("/error/code"))
                .and_then(Value::as_str);
            let allowed = matches!(
                (status, code),
                (
                    409,
                    Some(
                        "session_missing"
                            | "session_deleting"
                            | "session_deleted"
                            | "resync_required"
                            | "workspace_file_ambiguous"
                            | "terminal_busy"
                            | "settings_apply_failed"
                            | "steer_unavailable",
                    ),
                ) | (400, Some("terminal_invalid_request" | "model_unavailable"))
                    | (404, Some("workspace_file_not_found" | "terminal_not_found"))
                    | (422, Some("workspace_file_unavailable"))
                    | (503, Some("terminal_unavailable"))
                    | (429, Some("runtime_capacity"))
            );
            if allowed {
                let code = code.unwrap();
                return Err(format!(
                    "desktop bridge request failed with status {status} ({code})"
                ));
            }
        }
        return Err(format!(
            "desktop bridge request failed with status {status}"
        ));
    }
    // Some local proxies prepend diagnostics or append a separator after the
    // JSON body. Extract exactly the first balanced object/array so neither
    // form produces serde_json's misleading "trailing characters" error.
    serde_json::from_slice(first_json_value(raw)).map_err(|error| error.to_string())
}

fn first_json_value(raw: &[u8]) -> &[u8] {
    let Some(start) = raw.iter().position(|&b| b == b'{' || b == b'[') else {
        return raw;
    };
    let mut depth = 0usize;
    let mut in_string = false;
    let mut escaped = false;
    for (offset, &byte) in raw[start..].iter().enumerate() {
        if in_string {
            if escaped {
                escaped = false;
            } else if byte == b'\\' {
                escaped = true;
            } else if byte == b'"' {
                in_string = false;
            }
            continue;
        }
        match byte {
            b'"' => in_string = true,
            b'{' | b'[' => depth += 1,
            b'}' | b']' => {
                depth = depth.saturating_sub(1);
                if depth == 0 {
                    return &raw[start..=start + offset];
                }
            }
            _ => {}
        }
    }
    &raw[start..]
}

fn decode_chunked_body(mut body: &[u8]) -> Result<Vec<u8>, String> {
    let mut decoded = Vec::new();
    loop {
        let line_end = body
            .windows(2)
            .position(|window| window == b"\r\n")
            .ok_or_else(|| "desktop bridge returned an invalid chunked response".to_string())?;
        let line = std::str::from_utf8(&body[..line_end]).map_err(|error| error.to_string())?;
        let size = usize::from_str_radix(line.split(';').next().unwrap_or_default().trim(), 16)
            .map_err(|error| error.to_string())?;
        body = &body[line_end + 2..];

        if size == 0 {
            loop {
                let trailer_end = body
                    .windows(2)
                    .position(|window| window == b"\r\n")
                    .ok_or_else(|| {
                        "desktop bridge returned an invalid chunked response trailer".to_string()
                    })?;
                let trailer = &body[..trailer_end];
                body = &body[trailer_end + 2..];
                if trailer.is_empty() {
                    if !body.is_empty() {
                        return Err(
                            "desktop bridge returned data after the chunked response".to_string()
                        );
                    }
                    return Ok(decoded);
                }
                if !trailer.contains(&b':') {
                    return Err(
                        "desktop bridge returned an invalid chunked response trailer".to_string(),
                    );
                }
            }
        }

        if decoded.len().saturating_add(size) > MAX_BRIDGE_RESPONSE_BODY_BYTES {
            return Err("desktop bridge response exceeds the size limit".to_string());
        }
        let chunk_end = size
            .checked_add(2)
            .ok_or_else(|| "desktop bridge returned an invalid chunk size".to_string())?;
        if body.len() < chunk_end || body.get(size..chunk_end) != Some(&b"\r\n"[..]) {
            return Err("desktop bridge returned an invalid chunked response".to_string());
        }
        decoded.extend_from_slice(&body[..size]);
        body = &body[chunk_end..];
    }
}
