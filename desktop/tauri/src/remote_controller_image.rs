use super::*;
use base64::engine::general_purpose::STANDARD;
use std::io::Cursor;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct SessionImageRequest {
    pub controller_id: String,
    pub session_path: String,
    pub source: String,
}

impl RemoteControllerClient {
    pub fn session_image(
        &self,
        request: SessionImageRequest,
    ) -> Result<BridgeRemoteControllerSessionImageResponse, String> {
        if request.session_path.is_empty()
            || !clean(&request.session_path, 32768)
            || request.source.trim().is_empty()
            || request.source.len() > 22370645
            || request.source.contains('\0')
        {
            return Err(INVALID.into());
        }
        let route = format!("{}/session-image", path(&request.controller_id)?);
        let response: BridgeRemoteControllerSessionImageResponse = self.request(
            "POST",
            &route,
            Some(json!(BridgeRemoteControllerSessionImageRequest {
                session_path: request.session_path.clone(),
                source: request.source
            })),
            40,
        )?;
        if !view(&response.controller)
            || response.controller.id != request.controller_id
            || response.view.protocol_version != 1
            || response.view.session_path != request.session_path
            || response.view.workspace != response.controller.workspace
            || !valid_image(&response.view.image)
            || serde_json::to_vec(&response).map_or(true, |v| v.len() > 12 << 20)
        {
            return Err(FAILED.into());
        }
        Ok(response)
    }
}

fn valid_image(image: &BridgeRemoteControllerImage) -> bool {
    if image.filename.as_ref().is_some_and(|v| !clean(v, 4096))
        || image.size.is_some_and(|v| v > 16 << 20)
    {
        return false;
    }
    if let Some(code) = &image.error_code {
        return image.url.is_empty()
            && image.mime.is_none()
            && image.filename.is_none()
            && image.size.is_none()
            && matches!(
                code.as_str(),
                "blocked-remote"
                    | "proxy-config"
                    | "fetch-failed"
                    | "not-found"
                    | "forbidden"
                    | "not-a-file"
                    | "too-large"
                    | "changed-file"
                    | "invalid-image"
                    | "unsupported-type"
            );
    }
    if image.mime.as_deref() != Some("image/png") {
        return false;
    }
    let Some(encoded) = image.url.strip_prefix("data:image/png;base64,") else {
        return false;
    };
    if encoded.len() > ((8 << 20) * 4 / 3) + 4 {
        return false;
    }
    let Ok(bytes) = STANDARD.decode(encoded) else {
        return false;
    };
    if bytes.len() > 8 << 20 || STANDARD.encode(&bytes) != encoded {
        return false;
    }
    let mut decoder = png::Decoder::new(Cursor::new(bytes));
    decoder.set_limits(png::Limits { bytes: 16 << 20 });
    let Ok(mut reader) = decoder.read_info() else {
        return false;
    };
    let info = reader.info();
    if info.width == 0
        || info.height == 0
        || info.width > 1200
        || info.height > 1200
        || info.animation_control.is_some()
    {
        return false;
    }
    let Some(size) = reader.output_buffer_size() else {
        return false;
    };
    if size > 16 << 20 {
        return false;
    }
    let mut pixels = vec![0; size];
    reader.next_frame(&mut pixels).is_ok() && reader.finish().is_ok()
}

#[cfg(test)]
#[path = "remote_controller_image_tests.rs"]
mod tests;
