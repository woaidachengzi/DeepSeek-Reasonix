//! Bounded local SSE framing and the global replay-cursor admission boundary.
use super::{http_response::MAX_BRIDGE_RESPONSE_BODY_BYTES, BridgeEvent, PROTOCOL_VERSION};
use std::io::BufRead;

// Match normal bridge JSON bodies, allowing the single-line SSE prefix/CRLF.
const MAX_EVENT_LINE_BYTES: usize = MAX_BRIDGE_RESPONSE_BODY_BYTES + 8;
// Match the existing selected-session remote SSE header budgets.
const MAX_HEADER_LINE_BYTES: usize = 8 << 10;
const MAX_HEADER_BYTES: usize = 64 << 10;

#[derive(Debug, PartialEq, Eq)]
pub(super) enum EventStreamError {
    ResyncRequired,
    Unavailable,
    Cancelled,
}

#[derive(Debug, PartialEq, Eq)]
enum LineError {
    Unavailable,
    Limit,
    Cancelled,
}

#[cfg(test)]
fn line(reader: &mut impl BufRead, limit: usize) -> Result<Option<Vec<u8>>, LineError> {
    line_cancellable(reader, limit, &|| false)
}

fn line_cancellable(
    reader: &mut impl BufRead,
    limit: usize,
    cancelled: &impl Fn() -> bool,
) -> Result<Option<Vec<u8>>, LineError> {
    let mut result = Vec::new();
    loop {
        if cancelled() {
            return Err(LineError::Cancelled);
        }
        let available = reader.fill_buf();
        if cancelled() {
            return Err(LineError::Cancelled);
        }
        let bytes = available.map_err(|_| LineError::Unavailable)?;
        if bytes.is_empty() {
            return if result.is_empty() {
                Ok(None)
            } else {
                Err(LineError::Unavailable)
            };
        }
        let end = bytes.iter().position(|byte| *byte == b'\n');
        let take = end.map_or(bytes.len(), |index| index + 1);
        let wanted = result.len().saturating_add(take);
        if wanted > limit {
            return Err(LineError::Limit);
        }
        if wanted > result.capacity() {
            let capacity = result.capacity().saturating_mul(2).max(wanted).min(limit);
            result.reserve_exact(capacity - result.len());
        }
        result.extend_from_slice(&bytes[..take]);
        reader.consume(take);
        if cancelled() {
            return Err(LineError::Cancelled);
        }
        if end.is_some() {
            return Ok(Some(result));
        }
    }
}

fn header_line(
    reader: &mut impl BufRead,
    remaining: usize,
    cancelled: &impl Fn() -> bool,
) -> Result<Vec<u8>, EventStreamError> {
    match line_cancellable(reader, MAX_HEADER_LINE_BYTES.min(remaining), cancelled) {
        Ok(Some(value)) => Ok(value),
        Err(LineError::Limit) => Err(EventStreamError::ResyncRequired),
        Err(LineError::Cancelled) => Err(EventStreamError::Cancelled),
        _ => Err(EventStreamError::Unavailable),
    }
}

#[cfg(test)]
pub(super) fn read_headers(reader: &mut impl BufRead) -> Result<(), EventStreamError> {
    read_headers_cancellable(reader, &|| false)
}

pub(super) fn read_headers_cancellable(
    reader: &mut impl BufRead,
    cancelled: &impl Fn() -> bool,
) -> Result<(), EventStreamError> {
    let status = header_line(reader, MAX_HEADER_BYTES, cancelled)?;
    let status_text = std::str::from_utf8(&status).map_err(|_| EventStreamError::Unavailable)?;
    let mut words = status_text.split_whitespace();
    if !matches!(words.next(), Some("HTTP/1.0" | "HTTP/1.1")) {
        return Err(EventStreamError::Unavailable);
    }
    match words.next() {
        Some("409") => return Err(EventStreamError::ResyncRequired),
        Some("200") => {}
        _ => return Err(EventStreamError::Unavailable),
    }
    let mut total = status.len();
    loop {
        let header = header_line(reader, MAX_HEADER_BYTES - total, cancelled)?;
        total = total.saturating_add(header.len());
        if total > MAX_HEADER_BYTES {
            return Err(EventStreamError::ResyncRequired);
        }
        if header == b"\r\n" || header == b"\n" {
            return Ok(());
        }
        let header = std::str::from_utf8(&header).map_err(|_| EventStreamError::Unavailable)?;
        let (name, value) = header
            .split_once(':')
            .ok_or(EventStreamError::Unavailable)?;
        // The request uses HTTP/1.0 close-delimited SSE, like the remote
        // subscription path. Never interpret chunk-size bytes as SSE fields.
        if name.trim().eq_ignore_ascii_case("transfer-encoding") && !value.trim().is_empty() {
            return Err(EventStreamError::ResyncRequired);
        }
    }
}

pub(super) enum EventRead {
    Event(BridgeEvent),
    Ignored,
    Reconnect,
    ResyncRequired,
    Stopped,
}

impl EventRead {
    // Reading/decoding may finish just as a stream generation is cancelled.
    // Both pending delivery and pending resync must be suppressed at admission.
    pub(super) fn if_active(self, cancelled: &impl Fn() -> bool) -> Self {
        if cancelled() {
            Self::Stopped
        } else {
            self
        }
    }
}

pub(super) struct EventCursor {
    after: u64,
    needs_snapshot: bool,
}

impl EventCursor {
    pub(super) fn new(after: u64) -> Self {
        Self {
            after,
            needs_snapshot: false,
        }
    }

    pub(super) fn after_sequence(&self) -> u64 {
        self.after
    }

    fn resync(&mut self) -> EventRead {
        self.needs_snapshot = true;
        EventRead::ResyncRequired
    }

    #[cfg(test)]
    pub(super) fn next(&mut self, reader: &mut impl BufRead) -> EventRead {
        self.next_cancellable(reader, &|| false)
    }

    pub(super) fn next_cancellable(
        &mut self,
        reader: &mut impl BufRead,
        cancelled: &impl Fn() -> bool,
    ) -> EventRead {
        self.next_bounded(reader, MAX_EVENT_LINE_BYTES, cancelled)
    }

    #[cfg(test)]
    fn next_with_limit(&mut self, reader: &mut impl BufRead, limit: usize) -> EventRead {
        self.next_bounded(reader, limit, &|| false)
    }

    fn next_bounded(
        &mut self,
        reader: &mut impl BufRead,
        limit: usize,
        cancelled: &impl Fn() -> bool,
    ) -> EventRead {
        if self.needs_snapshot || cancelled() {
            return EventRead::Stopped;
        }
        let line = match line_cancellable(reader, limit, cancelled) {
            Ok(Some(line)) => line,
            Err(LineError::Limit) => return self.resync().if_active(cancelled),
            Err(LineError::Cancelled) => return EventRead::Stopped,
            Ok(None) | Err(LineError::Unavailable) => {
                return EventRead::Reconnect.if_active(cancelled)
            }
        };
        let Some(data) = line.strip_prefix(b"data:") else {
            return EventRead::Ignored; // SSE comments, metadata and blank lines.
        };
        let data = data.strip_prefix(b" ").unwrap_or(data);
        if data.iter().all(u8::is_ascii_whitespace) {
            return EventRead::Ignored; // Empty data heartbeats carry no sequence.
        }
        let event = match serde_json::from_slice::<BridgeEvent>(data) {
            Ok(event) => event,
            Err(_) => return self.resync().if_active(cancelled),
        };
        if cancelled() {
            return EventRead::Stopped;
        }
        if event.protocol_version != u64::from(PROTOCOL_VERSION) {
            return EventRead::Ignored; // Keep unsupported-protocol rejection.
        }
        if event.sequence <= self.after {
            return EventRead::Ignored; // Replay must never duplicate a UI mutation.
        }
        if self.after.checked_add(1) != Some(event.sequence) {
            return self.resync().if_active(cancelled); // A slow Go subscriber can lose live frames.
        }
        // Includes terminal frames, even though they route to another channel.
        self.after = event.sequence;
        EventRead::Event(event).if_active(cancelled)
    }
}

#[cfg(test)]
#[path = "event_stream_tests.rs"]
mod tests;
