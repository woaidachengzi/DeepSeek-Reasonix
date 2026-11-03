//! Bounded, display-only thumbnails. The original tool images remain untouched.
use base64::{engine::general_purpose::STANDARD, Engine as _};
use image::{codecs::jpeg::JpegEncoder, GenericImageView, ImageFormat, ImageReader, Limits};
use serde::Serialize;
use std::io::Cursor;

const MAX_SOURCE_DATA_URL: usize = 5 << 20;
const MAX_SOURCE_EDGE: u32 = 4096;
const MAX_EDGE: u32 = 480;
const MAX_JPEG_BYTES: usize = 40 << 10;

pub fn enabled(setting: Option<&str>) -> bool {
    setting != Some("0")
}

#[derive(Debug, Serialize)]
pub struct Preview {
    source_image_index: usize,
    mime_type: &'static str,
    width: u32,
    height: u32,
    data_base64: String,
}

// An optional display improvement must not push a completed action over the
// existing transport cap. Remove it and serialize the original envelope first.
#[cfg(test)]
pub fn encode_reply(reply: &mut serde_json::Value, limit: usize) -> serde_json::Result<Vec<u8>> {
    let encoded = serde_json::to_vec(reply)?;
    if encoded.len() <= limit || reply.get("preview").is_none() {
        return Ok(encoded);
    }
    if let Some(object) = reply.as_object_mut() {
        object.remove("preview");
    }
    serde_json::to_vec(reply)
}

// Match the first image eligible for Go's model-image path. A bad first image
// cannot silently substitute a thumbnail from a different image in this reply.
// Only read-only observations do this work before the worker replies. Mutating
// actions retain Go's post-reply path so optional image work cannot turn an
// already dispatched action into a host timeout/unknown-delivery result.
pub fn for_observation<'a>(
    action: &str,
    images: impl Iterator<Item = (&'a str, &'a str)>,
) -> Option<Preview> {
    if !matches!(action, "observe" | "browser_observe") {
        return None;
    }
    from_images(images)
}

fn from_images<'a>(images: impl Iterator<Item = (&'a str, &'a str)>) -> Option<Preview> {
    for (index, (mime, data)) in images.enumerate() {
        if !matches!(mime, "image/png" | "image/jpeg") || data.len() > 5 << 20 {
            continue;
        }
        return from_image(index, mime, data);
    }
    None
}

fn from_image(index: usize, mime: &str, data: &str) -> Option<Preview> {
    let prefix_len = "data:".len() + mime.len() + ";base64,".len();
    if data.len().checked_add(prefix_len)? > MAX_SOURCE_DATA_URL {
        return None;
    }
    let bytes = STANDARD.decode(data).ok()?;
    let format = match mime {
        "image/png" => ImageFormat::Png,
        "image/jpeg" => ImageFormat::Jpeg,
        _ => return None,
    };
    let mut limits = Limits::default();
    limits.max_image_width = Some(MAX_SOURCE_EDGE);
    limits.max_image_height = Some(MAX_SOURCE_EDGE);
    limits.max_alloc = Some(128 << 20);
    let mut reader = ImageReader::with_format(Cursor::new(bytes), format);
    reader.limits(limits);
    let source = reader.decode().ok()?;
    let (sw, sh) = source.dimensions();
    if sw == 0 || sh == 0 || sw > MAX_SOURCE_EDGE || sh > MAX_SOURCE_EDGE {
        return None;
    }
    let (width, height) = if sw <= MAX_EDGE && sh <= MAX_EDGE {
        (sw, sh)
    } else if sw >= sh {
        (MAX_EDGE, (sh * MAX_EDGE / sw).max(1))
    } else {
        ((sw * MAX_EDGE / sh).max(1), MAX_EDGE)
    };
    let mut rgb = image::RgbImage::new(width, height);
    for y in 0..height {
        for x in 0..width {
            // Preserve the old Go nearest-neighbour sampling and alpha-on-black
            // preview semantics; coordinates always use the full original image.
            let p = source.get_pixel(x * sw / width, y * sh / height);
            let alpha = u16::from(p[3]);
            rgb.put_pixel(
                x,
                y,
                image::Rgb([
                    (u16::from(p[0]) * alpha / 255) as u8,
                    (u16::from(p[1]) * alpha / 255) as u8,
                    (u16::from(p[2]) * alpha / 255) as u8,
                ]),
            );
        }
    }
    for quality in [70, 45, 25] {
        let mut bytes = Vec::new();
        JpegEncoder::new_with_quality(&mut bytes, quality)
            .encode(rgb.as_raw(), width, height, image::ColorType::Rgb8.into())
            .ok()?;
        if bytes.len() <= MAX_JPEG_BYTES {
            return Some(Preview {
                source_image_index: index,
                mime_type: "image/jpeg",
                width,
                height,
                data_base64: STANDARD.encode(bytes),
            });
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use image::{ImageBuffer, Rgba};

    #[test]
    fn exact_host_disable_keeps_the_legacy_post_reply_fallback_available() {
        assert!(!enabled(Some("0")));
        for setting in [None, Some(""), Some("1"), Some("false")] {
            assert!(enabled(setting));
        }
    }

    fn png(width: u32, height: u32) -> String {
        let image = ImageBuffer::from_pixel(width, height, Rgba([80u8, 160, 240, 255]));
        let mut bytes = Cursor::new(Vec::new());
        image.write_to(&mut bytes, ImageFormat::Png).unwrap();
        STANDARD.encode(bytes.into_inner())
    }

    #[test]
    fn scales_landscape_portrait_and_tiny_images_without_upscaling() {
        for (sw, sh, w, h) in [(640, 400, 480, 300), (400, 640, 300, 480), (8, 6, 8, 6)] {
            let source = png(sw, sh);
            let preview = from_images(std::iter::once(("image/png", source.as_str()))).unwrap();
            assert_eq!((preview.width, preview.height), (w, h));
            assert_eq!(preview.source_image_index, 0);
            let raw = STANDARD.decode(preview.data_base64).unwrap();
            assert!(raw.len() <= MAX_JPEG_BYTES);
            let decoded = image::load_from_memory_with_format(&raw, ImageFormat::Jpeg).unwrap();
            assert_eq!(decoded.dimensions(), (w, h));
        }
    }

    #[test]
    fn preserves_original_images_and_binds_the_selected_index() {
        let source = png(640, 400);
        let before = source.clone();
        let preview = from_images(
            [
                ("image/svg+xml", "untrusted"),
                ("image/png", source.as_str()),
            ]
            .into_iter(),
        )
        .unwrap();
        assert_eq!(source, before);
        assert_eq!(preview.source_image_index, 1);
        assert!(
            from_images([("image/png", "bad"), ("image/png", source.as_str())].into_iter())
                .is_none()
        );
    }

    #[test]
    fn refuses_bad_formats_oversized_sources_and_false_mime_without_panicking() {
        assert!(from_images(std::iter::once(("image/png", "not-base64"))).is_none());
        let too_wide = png(MAX_SOURCE_EDGE + 1, 1);
        assert!(from_images(std::iter::once(("image/png", too_wide.as_str()))).is_none());
        assert!(from_images(std::iter::once(("image/jpeg", png(2, 2).as_str()))).is_none());
        assert!(from_images(std::iter::once((
            "image/png",
            "A".repeat(MAX_SOURCE_DATA_URL).as_str()
        )))
        .is_none());
    }

    #[test]
    fn optional_preview_never_turns_a_fitting_action_reply_into_transport_failure() {
        let original = serde_json::json!({"is_error":false,"result":{"effect":"unverifiable"},"images":[{"data_base64":"A".repeat(5000)}]});
        let original_bytes = serde_json::to_vec(&original).unwrap();
        let mut reply = original.clone();
        reply["preview"] = serde_json::json!({"data_base64":"B".repeat(1000)});
        assert_eq!(
            encode_reply(&mut reply, original_bytes.len()).unwrap(),
            original_bytes
        );
        assert_eq!(reply, original);
        let mut oversized = original;
        oversized["preview"] = serde_json::Value::Null;
        let encoded = encode_reply(&mut oversized, 100).unwrap();
        assert!(encoded.len() > 100, "the original cap must remain enforced");
        assert!(oversized.get("preview").is_none());
    }

    #[test]
    fn only_read_only_observations_may_spend_preview_time_before_reply() {
        for action in [
            "click",
            "type",
            "key",
            "hotkey",
            "scroll",
            "browser_click",
            "browser_type",
            "browser_prepare",
            "status",
            "unknown",
        ] {
            let images = std::iter::once_with(|| panic!("write action inspected image"));
            assert!(for_observation(action, images).is_none(), "{action}");
        }
        let image = png(8, 6);
        for action in ["observe", "browser_observe"] {
            assert!(
                for_observation(action, std::iter::once(("image/png", image.as_str()))).is_some()
            );
        }
    }

    #[test]
    fn maximum_source_dimension_has_a_bounded_display_result() {
        let source = png(MAX_SOURCE_EDGE, MAX_SOURCE_EDGE);
        let started = std::time::Instant::now();
        let result =
            for_observation("observe", std::iter::once(("image/png", source.as_str()))).unwrap();
        eprintln!(
            "maximum-source preview: {}x{}, source_base64_bytes={}, elapsed_ms={:.3}",
            MAX_SOURCE_EDGE,
            MAX_SOURCE_EDGE,
            source.len(),
            started.elapsed().as_secs_f64() * 1000.0
        );
        assert_eq!((result.width, result.height), (MAX_EDGE, MAX_EDGE));
        assert!(STANDARD.decode(result.data_base64).unwrap().len() <= MAX_JPEG_BYTES);
    }
}
