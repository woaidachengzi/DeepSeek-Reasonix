//! Selected-session host transport with cancellation and native payload
//! projection. The subscription worker owns admission to the renderer queue.
use super::*;
use std::{
    io::{BufRead, BufReader, Write},
    net::{Shutdown, TcpStream},
    sync::{
        atomic::{AtomicBool, Ordering},
        Arc, Mutex,
    },
};

const FRAME_MAX: usize = 9 << 20;

#[derive(Clone, Default)]
pub(crate) struct EventCancellation {
    socket: Arc<Mutex<Option<Arc<TcpStream>>>>,
    stopped: Arc<AtomicBool>,
}
impl EventCancellation {
    pub(crate) fn is_closed(&self) -> bool {
        self.stopped.load(Ordering::Acquire)
    }
    pub(crate) fn close(&self) {
        self.stopped.store(true, Ordering::Release);
        if let Ok(socket) = self.socket.lock() {
            if let Some(socket) = socket.as_ref() {
                let _ = socket.shutdown(Shutdown::Both);
            }
        }
    }
    fn attach(&self, socket: &TcpStream) -> Result<EventSocketLease, String> {
        let mut current = self.socket.lock().map_err(|_| FAILED.to_string())?;
        if self.stopped.load(Ordering::Acquire) || current.is_some() {
            return Err(FAILED.into());
        }
        let socket = Arc::new(socket.try_clone().map_err(|_| FAILED.to_string())?);
        *current = Some(Arc::clone(&socket));
        Ok(EventSocketLease {
            cancellation: self.clone(),
            socket,
        })
    }
}

struct EventSocketLease {
    cancellation: EventCancellation,
    socket: Arc<TcpStream>,
}
impl Drop for EventSocketLease {
    fn drop(&mut self) {
        if let Ok(mut current) = self.cancellation.socket.lock() {
            if current
                .as_ref()
                .is_some_and(|socket| Arc::ptr_eq(socket, &self.socket))
            {
                *current = None;
            }
        }
        let _ = self.socket.shutdown(Shutdown::Both);
    }
}

pub(crate) struct SessionEvents {
    reader: BufReader<TcpStream>,
    cancellation: EventCancellation,
    controller: BridgeRemoteControllerView,
    session_path: String,
    _socket: EventSocketLease,
}
impl Drop for SessionEvents {
    fn drop(&mut self) {
        self.cancellation.close();
    }
}

// Unlike the local ledger stream, this uses HTTP/1.0 close-delimited framing:
// no chunk-size lines can accidentally be interpreted as SSE metadata. The
// producer sends heartbeats every 10s; a silent/broken transport fails closed.
impl RemoteControllerClient {
    #[allow(dead_code)] // Registered IPC + native payload projection follows.
    pub(crate) fn session_events(
        &self,
        request: SessionViewRequest,
    ) -> Result<SessionEvents, String> {
        self.session_events_cancellable(request, EventCancellation::default())
    }
    #[allow(dead_code)]
    pub(crate) fn session_events_cancellable(
        &self,
        request: SessionViewRequest,
        cancellation: EventCancellation,
    ) -> Result<SessionEvents, String> {
        if cancellation.stopped.load(Ordering::Acquire) {
            return Err(FAILED.into());
        }
        let route = format!("{}/session-events", path(&request.controller_id)?);
        if request.session_path.is_empty() || !clean(&request.session_path, 32768) {
            return Err(INVALID.into());
        }
        let catalogue = self.sessions_connected(
            HandleRequest {
                controller_id: request.controller_id,
            },
            |socket| cancellation.attach(socket),
        )?;
        if cancellation.stopped.load(Ordering::Acquire) {
            return Err(FAILED.into());
        }
        if !catalogue
            .sessions
            .iter()
            .any(|row| row.path == request.session_path)
        {
            return Err(FAILED.into());
        }
        SessionEvents::open_cancellable(
            self.address,
            &self.token,
            &route,
            catalogue.controller,
            request.session_path,
            cancellation,
        )
    }
}

fn line(reader: &mut impl BufRead, max: usize) -> Result<Option<Vec<u8>>, String> {
    let mut result = Vec::new();
    loop {
        let bytes = reader.fill_buf().map_err(|_| FAILED.to_string())?;
        if bytes.is_empty() {
            return if result.is_empty() {
                Ok(None)
            } else {
                Err(FAILED.into())
            };
        }
        let end = bytes.iter().position(|b| *b == b'\n');
        let take = end.map_or(bytes.len(), |n| n + 1);
        if result.len().saturating_add(take) > max {
            return Err(FAILED.into());
        }
        result.extend_from_slice(&bytes[..take]);
        reader.consume(take);
        if end.is_some() {
            return Ok(Some(result));
        }
    }
}

impl SessionEvents {
    #[cfg(test)]
    fn open(
        address: SocketAddr,
        token: &str,
        route: &str,
        controller: BridgeRemoteControllerView,
        session_path: String,
    ) -> Result<Self, String> {
        Self::open_cancellable(
            address,
            token,
            route,
            controller,
            session_path,
            EventCancellation::default(),
        )
    }
    fn open_cancellable(
        address: SocketAddr,
        token: &str,
        route: &str,
        controller: BridgeRemoteControllerView,
        session_path: String,
        cancellation: EventCancellation,
    ) -> Result<Self, String> {
        if cancellation.stopped.load(Ordering::Acquire) {
            return Err(FAILED.into());
        }
        if !address.ip().is_loopback()
            || token.is_empty()
            || token.len() > 256
            || !token.bytes().all(|b| b.is_ascii_graphic())
        {
            return Err(FAILED.into());
        }
        let mut socket = TcpStream::connect_timeout(&address, Duration::from_secs(1))
            .map_err(|_| FAILED.to_string())?;
        let socket_lease = cancellation.attach(&socket)?;
        socket
            .set_read_timeout(Some(Duration::from_secs(35)))
            .map_err(|_| FAILED.to_string())?;
        socket
            .set_write_timeout(Some(Duration::from_secs(5)))
            .map_err(|_| FAILED.to_string())?;
        let body = serde_json::to_vec(&BridgeRemoteControllerSessionViewRequest {
            session_path: session_path.clone(),
        })
        .map_err(|_| FAILED.to_string())?;
        let header = format!("POST {route} HTTP/1.0\r\nHost: {address}\r\nAuthorization: Bearer {token}\r\nAccept: text/event-stream\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n", body.len());
        socket
            .write_all(header.as_bytes())
            .and_then(|_| socket.write_all(&body))
            .map_err(|_| FAILED.to_string())?;
        let mut reader = BufReader::new(socket);
        let status = line(&mut reader, 8192)?.ok_or_else(|| FAILED.to_string())?;
        let status = std::str::from_utf8(&status).map_err(|_| FAILED.to_string())?;
        let mut parts = status.split_whitespace();
        if !matches!(parts.next(), Some("HTTP/1.0" | "HTTP/1.1")) || parts.next() != Some("200") {
            return Err(FAILED.into());
        }
        let mut budget = status.len();
        let mut content_type = false;
        loop {
            let header = line(&mut reader, 8192)?.ok_or_else(|| FAILED.to_string())?;
            budget += header.len();
            if budget > 64 << 10 {
                return Err(FAILED.into());
            }
            if header == b"\r\n" {
                break;
            }
            let header = std::str::from_utf8(&header).map_err(|_| FAILED.to_string())?;
            let (key, value) = header.split_once(':').ok_or_else(|| FAILED.to_string())?;
            match key.to_ascii_lowercase().as_str() {
                "content-type" => {
                    if content_type
                        || !value
                            .trim()
                            .split(';')
                            .next()
                            .is_some_and(|v| v.trim().eq_ignore_ascii_case("text/event-stream"))
                    {
                        return Err(FAILED.into());
                    }
                    content_type = true;
                }
                "transfer-encoding" | "content-length" => return Err(FAILED.into()),
                _ => {}
            }
        }
        if !content_type {
            return Err(FAILED.into());
        }
        reader
            .get_ref()
            .set_read_timeout(Some(Duration::from_secs(25)))
            .map_err(|_| FAILED.to_string())?;
        if cancellation.stopped.load(Ordering::Acquire) {
            return Err(FAILED.into());
        }
        Ok(Self {
            reader,
            cancellation,
            controller,
            session_path,
            _socket: socket_lease,
        })
    }

    #[allow(dead_code)]
    pub(crate) fn cancellation(&self) -> EventCancellation {
        self.cancellation.clone()
    }

    #[allow(dead_code)]
    pub(crate) fn next(&mut self) -> Result<BridgeRemoteControllerSessionEvent, String> {
        let result = self.read_event();
        if result.is_err() {
            self.cancellation.close();
        }
        result
    }
    fn read_event(&mut self) -> Result<BridgeRemoteControllerSessionEvent, String> {
        let mut data = Vec::new();
        let mut budget = 0usize;
        let mut has_data = false;
        loop {
            if self.cancellation.stopped.load(Ordering::Acquire) {
                return Err(FAILED.into());
            }
            let raw = line(&mut self.reader, FRAME_MAX)?.ok_or_else(|| FAILED.to_string())?;
            budget = budget.saturating_add(raw.len());
            if budget > FRAME_MAX {
                return Err(FAILED.into());
            }
            let text = std::str::from_utf8(&raw)
                .map_err(|_| FAILED.to_string())?
                .trim_end_matches('\n')
                .trim_end_matches('\r');
            if text.is_empty() {
                budget = 0;
                if !has_data {
                    continue;
                }
                let mut frame: BridgeRemoteControllerSessionEvent =
                    serde_json::from_slice(&data).map_err(|_| FAILED.to_string())?;
                if frame.protocol_version != 1
                    || !view(&frame.controller)
                    || frame.controller.id != self.controller.id
                    || frame.controller.name != self.controller.name
                    || frame.controller.workspace != self.controller.workspace
                    || frame.session_path != self.session_path
                    || !frame.event.is_object()
                    || frame.event.get("sessionPath").and_then(Value::as_str)
                        != Some(self.session_path.as_str())
                    || self.cancellation.stopped.load(Ordering::Acquire)
                {
                    return Err(FAILED.into());
                }
                frame.event = super::event_payload::project(&frame.event)?;
                // The worker still checks registry ownership before enqueue;
                // the renderer fences events already queued for an old owner.
                return Ok(frame);
            }
            let (field, value) = text.split_once(':').unwrap_or((text, ""));
            if field == "data" {
                if has_data {
                    data.push(b'\n');
                }
                data.extend_from_slice(value.strip_prefix(' ').unwrap_or(value).as_bytes());
                has_data = true;
            }
        }
    }
}

#[cfg(test)]
#[path = "remote_controller_events_tests.rs"]
mod tests;
