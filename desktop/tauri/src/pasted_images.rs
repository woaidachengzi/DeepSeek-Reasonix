use std::{collections::HashMap, io::Write, sync::Mutex};

use base64::{engine::general_purpose::STANDARD, Engine};
use serde::Serialize;
use tempfile::NamedTempFile;

const MAX_BYTES: usize = 16 << 20;
const MAX_PENDING: usize = 20;

#[derive(Default)]
pub struct PastedImages(Mutex<HashMap<String, NamedTempFile>>);

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StagedImage {
    pub token: String,
    pub path: String,
    pub size: usize,
    pub preview_url: String,
}

impl PastedImages {
    pub fn stage(&self, extension: &str, bytes: &[u8]) -> Result<StagedImage, String> {
        validate_image(extension, bytes)?;
        let mut files = self.0.lock().map_err(|_| "image staging is unavailable")?;
        if files.len() >= MAX_PENDING {
            return Err(
                "at most 20 pasted images can be pending; remove an image and retry".into(),
            );
        }
        let mut file = tempfile::Builder::new()
            .prefix("pasted-image-")
            .suffix(&format!(".{extension}"))
            .tempfile()
            .map_err(|_| "unable to prepare a private image file")?;
        file.write_all(bytes)
            .map_err(|_| "unable to save pasted image")?;
        file.flush().map_err(|_| "unable to save pasted image")?;
        let path = file.path().to_string_lossy().into_owned();
        let token = file
            .path()
            .file_name()
            .unwrap()
            .to_string_lossy()
            .into_owned();
        files.insert(token.clone(), file);
        Ok(StagedImage {
            token,
            path,
            size: bytes.len(),
            preview_url: format!("data:{};base64,{}", match extension {
                "jpg" | "jpeg" => "image/jpeg",
                "gif" => "image/gif",
                "webp" => "image/webp",
                _ => "image/png",
            }, STANDARD.encode(bytes)),
        })
    }

    pub fn discard(&self, token: &str) -> Result<(), String> {
        // Only files this host owns can be removed. No renderer-supplied path
        // is ever opened or deleted, and dropping the state cleans up on exit.
        self.0
            .lock()
            .map_err(|_| "image staging is unavailable")?
            .remove(token);
        Ok(())
    }
}

pub fn decode_data_url(value: &str) -> Result<(String, Vec<u8>), String> {
    if value.len() > MAX_BYTES.div_ceil(3) * 4 + 64 {
        return Err("pasted image exceeds 16 MiB".into());
    }
    let (header, encoded) = value.split_once(",").ok_or("invalid image data URL")?;
    let extension = match header {
        "data:image/png;base64" => "png",
        "data:image/jpeg;base64" => "jpg",
        "data:image/webp;base64" => "webp",
        "data:image/gif;base64" => "gif",
        _ => return Err("pasted image must be PNG, JPEG, GIF, or WebP".into()),
    };
    let bytes = STANDARD
        .decode(encoded)
        .map_err(|_| "invalid pasted image encoding")?;
    validate_image(extension, &bytes)?;
    Ok((extension.into(), bytes))
}

fn validate_image(extension: &str, bytes: &[u8]) -> Result<(), String> {
    if bytes.is_empty() || bytes.len() > MAX_BYTES {
        return Err("pasted image is empty or exceeds 16 MiB".into());
    }
    if extension == "gif" {
        if bytes.len() < 10 || (!bytes.starts_with(b"GIF87a") && !bytes.starts_with(b"GIF89a")) {
            return Err("invalid GIF image".into());
        }
        let width = u16::from_le_bytes([bytes[6], bytes[7]]);
        let height = u16::from_le_bytes([bytes[8], bytes[9]]);
        if width == 0 || height == 0 || width > 8192 || height > 8192 {
            return Err("image dimensions must be between 1 and 8192 pixels".into());
        }
        return Ok(());
    }
    super::validate_theme_image(&format!("pasted.{extension}"), bytes)
        .map_err(|_| "invalid PNG, JPEG, or WebP image (maximum 8192 pixels per side)".into())
}

pub fn encode_rgba(width: u32, height: u32, rgba: &[u8]) -> Result<Vec<u8>, String> {
    let length = (width as usize)
        .checked_mul(height as usize)
        .and_then(|n| n.checked_mul(4));
    if width == 0
        || height == 0
        || width > 8192
        || height > 8192
        || length != Some(rgba.len())
        || rgba.len() > 64 << 20
    {
        return Err("clipboard image is too large or has invalid dimensions".into());
    }
    let mut bytes = Vec::new();
    {
        let mut encoder = png::Encoder::new(&mut bytes, width, height);
        encoder.set_color(png::ColorType::Rgba);
        encoder.set_depth(png::BitDepth::Eight);
        let mut writer = encoder
            .write_header()
            .map_err(|_| "unable to encode clipboard image")?;
        writer
            .write_image_data(rgba)
            .map_err(|_| "unable to encode clipboard image")?;
    }
    validate_image("png", &bytes)?;
    Ok(bytes)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn staging_is_private_bounded_and_token_owned() {
        let images = PastedImages::default();
        let bytes = encode_rgba(1, 1, &[255, 0, 0, 255]).unwrap();
        let staged = images.stage("png", &bytes).unwrap();
        assert_eq!(std::fs::read(&staged.path).unwrap(), bytes);
        assert_eq!(staged.preview_url, format!("data:image/png;base64,{}", STANDARD.encode(&bytes)));
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            assert_eq!(
                std::fs::metadata(&staged.path)
                    .unwrap()
                    .permissions()
                    .mode()
                    & 0o777,
                0o600
            );
        }
        images.discard(&staged.path).unwrap();
        assert!(
            std::path::Path::new(&staged.path).exists(),
            "a path is not a deletion token"
        );
        images.discard(&staged.token).unwrap();
        assert!(!std::path::Path::new(&staged.path).exists());
        for _ in 0..MAX_PENDING {
            images.stage("png", &bytes).unwrap();
        }
        assert!(images.stage("png", &bytes).is_err());
    }

    #[test]
    fn decoding_rejects_disguised_or_oversized_data() {
        let bytes = encode_rgba(1, 1, &[0, 0, 0, 255]).unwrap();
        let url = format!("data:image/png;base64,{}", STANDARD.encode(&bytes));
        assert_eq!(decode_data_url(&url).unwrap().1, bytes);
        assert!(decode_data_url("data:image/png;base64,c2VjcmV0").is_err());
        assert!(decode_data_url(&url.replace("image/png", "image/jpeg")).is_err());
        assert!(decode_data_url("data:image/svg+xml;base64,PHN2Zz4=").is_err());
        assert!(encode_rgba(8193, 1, &[0; 4]).is_err());
        assert!(encode_rgba(1, 1, &[0; 3]).is_err());
        assert!(decode_data_url(&"x".repeat(MAX_BYTES.div_ceil(3) * 4 + 65)).is_err());
        let staged = PastedImages::default().stage("png", &bytes).unwrap();
        assert!(
            !std::path::Path::new(&staged.path).exists(),
            "host exit drops its temporary images"
        );
    }
}
