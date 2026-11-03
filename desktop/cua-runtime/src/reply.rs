//! Borrowed worker envelopes, retaining the original compact JSON wire shape.
use serde::{ser::SerializeSeq, Serialize, Serializer};
use serde_json::Value;

#[derive(Clone, Copy)]
pub struct Context<'a> {
    pub protocol: u32,
    pub id: u64,
    pub version: &'a str,
    pub source_revision: &'a str,
    pub source_fingerprint: &'a str,
}

// Field order matches serde_json::Value's sorted object keys, including images.
#[derive(Clone, Copy, Serialize)]
pub struct Image<'a> {
    pub data_base64: &'a str,
    pub mime_type: &'a str,
}

pub struct Images<I>(pub I);

pub trait ImageBytes: Serialize {
    fn image_bytes(&self) -> usize;
}

impl<'a, I> ImageBytes for Images<I>
where
    I: Clone + ExactSizeIterator<Item = Image<'a>>,
{
    fn image_bytes(&self) -> usize {
        self.0.clone().fold(0usize, |size, image| {
            size.saturating_add(image.data_base64.len())
                .saturating_add(image.mime_type.len())
        })
    }
}

impl<'a, I> Serialize for Images<I>
where
    I: Clone + ExactSizeIterator<Item = Image<'a>>,
{
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        let mut sequence = serializer.serialize_seq(Some(self.0.len()))?;
        for image in self.0.clone() {
            sequence.serialize_element(&image)?;
        }
        sequence.end()
    }
}

pub struct SuccessData<'a, I> {
    pub result: &'a Value,
    pub text: &'a str,
    pub images: I,
    pub action: Option<&'a Value>,
    pub is_error: bool,
    pub error_code: Option<&'a str>,
    pub preview: Option<&'a Value>,
}

#[derive(Serialize)]
pub struct Success<'a, I: Serialize> {
    action: Option<&'a Value>,
    driver: &'static str,
    error_code: Option<&'a str>,
    id: u64,
    images: I,
    is_error: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    preview: Option<Option<&'a Value>>,
    protocol: u32,
    result: &'a Value,
    source_fingerprint: &'a str,
    source_revision: &'a str,
    text: &'a str,
    version: &'a str,
}

impl<'a, I: Serialize> Success<'a, I> {
    pub fn new(context: Context<'a>, data: SuccessData<'a, I>) -> Self {
        Self {
            action: data.action,
            driver: "cua",
            error_code: data.error_code,
            id: context.id,
            images: data.images,
            is_error: data.is_error,
            preview: Some(data.preview),
            protocol: context.protocol,
            result: data.result,
            source_fingerprint: context.source_fingerprint,
            source_revision: context.source_revision,
            text: data.text,
            version: context.version,
        }
    }
}

impl<I: ImageBytes> Success<'_, I> {
    pub fn encode(&mut self, limit: usize) -> serde_json::Result<Vec<u8>> {
        // A sizing hint only: escaped text and arbitrary structured data may
        // still grow the buffer. Wire limits are checked against actual bytes.
        let hint = self
            .images
            .image_bytes()
            .saturating_add(self.text.len())
            .saturating_add(1024);
        let mut encoded = Vec::with_capacity(hint.min(limit.max(128)));
        serde_json::to_writer(&mut encoded, &*self)?;
        if encoded.len() <= limit || self.preview.is_none() {
            return Ok(encoded);
        }
        // Remove only optional preview, retaining the same allocated buffer.
        self.preview = None;
        encoded.clear();
        serde_json::to_writer(&mut encoded, self)?;
        Ok(encoded)
    }
}

#[derive(Serialize)]
struct EmptyObject {}

#[derive(Serialize)]
pub struct Failure<'a> {
    completion: &'static str,
    driver: &'static str,
    error: &'a str,
    error_code: &'static str,
    id: u64,
    images: [(); 0],
    is_error: bool,
    protocol: u32,
    result: EmptyObject,
    source_fingerprint: &'a str,
    source_revision: &'a str,
    version: &'a str,
}

impl<'a> Failure<'a> {
    pub fn refused(context: Context<'a>, error: &'a str) -> Self {
        Self::new(context, error, "not_started", "reasonix_target_refused")
    }

    pub fn driver_error(context: Context<'a>, error: &'a str) -> Self {
        Self::new(context, error, "unknown", "reasonix_driver_error")
    }

    fn new(
        context: Context<'a>,
        error: &'a str,
        completion: &'static str,
        error_code: &'static str,
    ) -> Self {
        Self {
            completion,
            driver: "cua",
            error,
            error_code,
            id: context.id,
            images: [],
            is_error: true,
            protocol: context.protocol,
            result: EmptyObject {},
            source_fingerprint: context.source_fingerprint,
            source_revision: context.source_revision,
            version: context.version,
        }
    }

    pub fn encode(&self) -> serde_json::Result<Vec<u8>> {
        serde_json::to_vec(self)
    }
}

pub fn structured(raw: Option<&str>) -> Value {
    raw.and_then(|value| serde_json::from_str(value).ok())
        .unwrap_or(Value::Null)
}

pub fn refused(structured: &Value, action: Option<&Value>) -> bool {
    structured["status"] == "refused" || action.is_some_and(|value| value["effect"] == "refused")
}

#[cfg(test)]
#[path = "reply_tests.rs"]
mod tests;
