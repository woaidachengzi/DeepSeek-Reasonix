use super::*;
use std::io::{BufReader, Cursor};

fn frame(sequence: u64, kind: &str) -> Vec<u8> {
    format!(
        "data: {}\n",
        serde_json::json!({
            "protocolVersion":1,"sequence":sequence,"eventKind":kind,
            "sessionId":"owned","payload":{"kind":kind,"text":"中🙂"}
        })
    )
    .into_bytes()
}

fn delivered(read: EventRead) -> BridgeEvent {
    match read {
        EventRead::Event(event) => event,
        _ => panic!("expected one admitted event"),
    }
}

#[test]
fn tiny_read_chunks_preserve_unicode_comments_and_sse_metadata() {
    let mut bytes = b": heartbeat\r\n\r\nid: 1\nevent: text\ndata:\n".to_vec();
    bytes.extend(frame(1, "text"));
    let mut reader = BufReader::with_capacity(3, Cursor::new(bytes));
    let mut cursor = EventCursor::new(0);
    for _ in 0..5 {
        assert!(matches!(cursor.next(&mut reader), EventRead::Ignored));
        assert_eq!(cursor.after_sequence(), 0);
    }
    let event = delivered(cursor.next(&mut reader));
    assert_eq!(event.payload["text"], "中🙂");
    assert_eq!(cursor.after_sequence(), 1);
    assert!(matches!(cursor.next(&mut reader), EventRead::Reconnect));
}

#[test]
fn duplicate_frames_are_ignored_and_terminal_frames_advance_the_global_cursor() {
    let mut bytes = frame(7, "text");
    bytes.extend(frame(7, "text"));
    bytes.extend(frame(6, "reasoning"));
    bytes.extend(frame(8, "terminal_output"));
    bytes.extend(frame(9, "turn_done"));
    let mut reader = Cursor::new(bytes);
    let mut cursor = EventCursor::new(6);
    assert_eq!(delivered(cursor.next(&mut reader)).sequence, 7);
    assert!(matches!(cursor.next(&mut reader), EventRead::Ignored));
    assert!(matches!(cursor.next(&mut reader), EventRead::Ignored));
    assert_eq!(
        delivered(cursor.next(&mut reader)).event_kind,
        "terminal_output"
    );
    assert_eq!(cursor.after_sequence(), 8);
    assert_eq!(delivered(cursor.next(&mut reader)).event_kind, "turn_done");
    assert_eq!(cursor.after_sequence(), 9);
}

#[test]
fn gap_requests_one_snapshot_then_stops_without_advancing_or_reading_more() {
    let mut bytes = frame(41, "text");
    bytes.extend(frame(43, "turn_done"));
    bytes.extend(frame(44, "text"));
    let mut reader = Cursor::new(bytes);
    let mut cursor = EventCursor::new(40);
    assert_eq!(delivered(cursor.next(&mut reader)).sequence, 41);
    assert!(matches!(
        cursor.next(&mut reader),
        EventRead::ResyncRequired
    ));
    let position = reader.position();
    for _ in 0..3 {
        assert!(matches!(cursor.next(&mut reader), EventRead::Stopped));
        assert_eq!(cursor.after_sequence(), 41);
        assert_eq!(reader.position(), position);
    }
}

#[test]
fn oversized_lines_stop_before_unbounded_copy_and_cannot_be_replayed_in_place() {
    let mut reader = BufReader::with_capacity(8, Cursor::new(vec![b'x'; 1024]));
    let mut cursor = EventCursor::new(7);
    assert!(matches!(
        cursor.next_with_limit(&mut reader, 64),
        EventRead::ResyncRequired
    ));
    assert!(reader.get_ref().position() <= 72);
    assert_eq!(cursor.after_sequence(), 7);
    assert!(matches!(
        cursor.next_with_limit(&mut reader, 64),
        EventRead::Stopped
    ));
    assert_eq!(MAX_EVENT_LINE_BYTES, MAX_BRIDGE_RESPONSE_BODY_BYTES + 8);
}

#[test]
fn line_budget_accepts_exact_boundary_and_rejects_one_more_byte() {
    assert_eq!(
        line(&mut Cursor::new(b"abc\n"), 4).unwrap(),
        Some(b"abc\n".to_vec())
    );
    assert_eq!(line(&mut Cursor::new(b"abcd\n"), 4), Err(LineError::Limit));
    assert_eq!(
        line(&mut Cursor::new(b"abc"), 4),
        Err(LineError::Unavailable)
    );
    assert_eq!(line(&mut Cursor::new(b""), 4).unwrap(), None);
}

#[test]
fn eof_or_incomplete_frame_reconnects_from_the_last_complete_sequence() {
    let mut cursor = EventCursor::new(0);
    let mut first = frame(1, "text");
    let partial = frame(2, "turn_done");
    first.extend(&partial[..partial.len() - 1]);
    let mut reader = Cursor::new(first);
    assert_eq!(delivered(cursor.next(&mut reader)).sequence, 1);
    assert!(matches!(cursor.next(&mut reader), EventRead::Reconnect));
    assert_eq!(cursor.after_sequence(), 1);
    assert_eq!(
        delivered(cursor.next(&mut Cursor::new(partial))).sequence,
        2
    );
}

#[test]
fn malformed_json_requires_a_snapshot_but_unknown_protocol_is_not_forwarded() {
    let mut cursor = EventCursor::new(0);
    assert!(matches!(
        cursor.next(&mut Cursor::new(b"data: {broken}\n")),
        EventRead::ResyncRequired
    ));
    let mut wrong = frame(1, "text");
    let text = String::from_utf8(wrong.clone())
        .unwrap()
        .replace("\"protocolVersion\":1", "\"protocolVersion\":99");
    wrong = text.into_bytes();
    let mut cursor = EventCursor::new(0);
    assert!(matches!(
        cursor.next(&mut Cursor::new(wrong)),
        EventRead::Ignored
    ));
    assert_eq!(cursor.after_sequence(), 0);
}

#[test]
fn headers_accept_close_delimited_stream_and_leave_first_event_unread() {
    let mut bytes = b"HTTP/1.0 200 OK\r\nContent-Type: text/event-stream\r\n\r\n".to_vec();
    bytes.extend(frame(1, "text"));
    let mut reader = BufReader::with_capacity(3, Cursor::new(bytes));
    assert_eq!(read_headers(&mut reader), Ok(()));
    let mut cursor = EventCursor::new(0);
    assert_eq!(delivered(cursor.next(&mut reader)).sequence, 1);
    assert_eq!(
        read_headers(&mut Cursor::new(b"HTTP/1.1 409 Conflict\r\n")),
        Err(EventStreamError::ResyncRequired)
    );
}

#[test]
fn headers_bound_both_one_line_and_the_total_and_reject_chunk_framing() {
    let mut huge_line = b"HTTP/1.0 200 OK\r\nX: ".to_vec();
    huge_line.extend(vec![b'x'; MAX_HEADER_LINE_BYTES]);
    huge_line.extend(b"\r\n\r\n");
    assert_eq!(
        read_headers(&mut Cursor::new(huge_line)),
        Err(EventStreamError::ResyncRequired)
    );
    let mut many_lines = b"HTTP/1.0 200 OK\r\n".to_vec();
    for _ in 0..MAX_HEADER_BYTES / 128 + 1 {
        many_lines.extend(b"X: ");
        many_lines.extend(vec![b'x'; 123]);
        many_lines.extend(b"\r\n");
    }
    many_lines.extend(b"\r\n");
    let mut reader = BufReader::with_capacity(8, Cursor::new(many_lines));
    assert_eq!(
        read_headers(&mut reader),
        Err(EventStreamError::ResyncRequired)
    );
    assert!(reader.get_ref().position() <= (MAX_HEADER_BYTES + 8) as u64);
    assert_eq!(
        read_headers(&mut Cursor::new(
            b"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n"
        )),
        Err(EventStreamError::ResyncRequired)
    );
    for broken in [
        b"HTTP/1.0 2000 Wrong\r\n\r\n".as_slice(),
        b"HTTP/1.0 200 OK\r\nX: unfinished",
    ] {
        assert_eq!(
            read_headers(&mut Cursor::new(broken)),
            Err(EventStreamError::Unavailable)
        );
    }
}

#[test]
fn production_connection_uses_close_delimited_sse_and_reconnect_cursor() {
    use std::{io::Write, net::TcpListener, thread, time::Duration};
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let address = listener.local_addr().unwrap();
    let server = thread::spawn(move || {
        for after in [0, 1] {
            let (stream, _) = listener.accept().unwrap();
            stream
                .set_read_timeout(Some(Duration::from_secs(2)))
                .unwrap();
            let mut reader = BufReader::new(stream);
            let first = line(&mut reader, MAX_HEADER_LINE_BYTES).unwrap().unwrap();
            assert_eq!(
                String::from_utf8(first).unwrap(),
                format!("GET /v1/events?afterSequence={after} HTTP/1.0\r\n")
            );
            let mut saw_close = false;
            loop {
                let header = line(&mut reader, MAX_HEADER_LINE_BYTES).unwrap().unwrap();
                if header == b"\r\n" {
                    break;
                }
                saw_close |= header == b"Connection: close\r\n";
            }
            assert!(saw_close);
            let mut stream = reader.into_inner();
            stream
                .write_all(b"HTTP/1.0 200 OK\r\nContent-Type: text/event-stream\r\n\r\n")
                .unwrap();
            stream.write_all(&frame(1, "text")).unwrap();
            if after == 1 {
                stream.write_all(&frame(2, "terminal_output")).unwrap();
            }
        }
    });
    let mut cursor = EventCursor::new(0);
    let mut first = super::super::open_event_stream(address, "fixture-only", 0).unwrap();
    assert_eq!(delivered(cursor.next(&mut first)).sequence, 1);
    assert!(matches!(cursor.next(&mut first), EventRead::Reconnect));
    let mut second =
        super::super::open_event_stream(address, "fixture-only", cursor.after_sequence()).unwrap();
    assert!(matches!(cursor.next(&mut second), EventRead::Ignored));
    assert_eq!(delivered(cursor.next(&mut second)).sequence, 2);
    assert!(matches!(cursor.next(&mut second), EventRead::Reconnect));
    server.join().unwrap();
}

// Drip indefinitely without a newline, cancelling from inside fill_buf. The
// guard makes a missing cancellation check fail promptly, without real sleeps.
struct CancellingDrip<'a> {
    chunks: Vec<&'static [u8]>,
    chunk: usize,
    offset: usize,
    fills: usize,
    cancelled: &'a std::cell::Cell<bool>,
}
impl std::io::Read for CancellingDrip<'_> {
    fn read(&mut self, output: &mut [u8]) -> std::io::Result<usize> {
        let available = self.fill_buf()?;
        let count = available.len().min(output.len());
        output[..count].copy_from_slice(&available[..count]);
        self.consume(count);
        Ok(count)
    }
}
impl std::io::BufRead for CancellingDrip<'_> {
    fn fill_buf(&mut self) -> std::io::Result<&[u8]> {
        self.fills += 1;
        assert!(
            self.fills < 12,
            "reader ignored cancellation while data kept arriving"
        );
        if self.fills == 3 {
            self.cancelled.set(true);
        }
        Ok(&self.chunks[self.chunk][self.offset..])
    }
    fn consume(&mut self, amount: usize) {
        self.offset += amount;
        if self.offset == self.chunks[self.chunk].len() {
            self.offset = 0;
            self.chunk = (self.chunk + 1).min(self.chunks.len() - 1);
        }
    }
}

#[test]
fn cancelling_a_dripping_line_or_header_stops_on_the_next_fill_without_resync() {
    let stop = std::cell::Cell::new(false);
    let mut body = CancellingDrip {
        chunks: vec![b"x"],
        chunk: 0,
        offset: 0,
        fills: 0,
        cancelled: &stop,
    };
    let mut cursor = EventCursor::new(7);
    assert!(matches!(
        cursor.next_cancellable(&mut body, &|| stop.get()),
        EventRead::Stopped
    ));
    assert_eq!(body.fills, 3);
    assert_eq!(cursor.after_sequence(), 7);
    assert!(!cursor.needs_snapshot);
    stop.set(false);
    let mut headers = CancellingDrip {
        chunks: vec![b"HTTP/1.0 200 OK\r\n", b"X: ", b"x"],
        chunk: 0,
        offset: 0,
        fills: 0,
        cancelled: &stop,
    };
    assert_eq!(
        read_headers_cancellable(&mut headers, &|| stop.get()),
        Err(EventStreamError::Cancelled)
    );
    assert_eq!(headers.fills, 3);
}

#[test]
fn cancellation_before_read_does_zero_work_and_pending_events_or_resync_do_not_emit() {
    let stop = std::cell::Cell::new(true);
    let mut reader = CancellingDrip {
        chunks: vec![b"x"],
        chunk: 0,
        offset: 0,
        fills: 0,
        cancelled: &stop,
    };
    let mut cursor = EventCursor::new(0);
    assert!(matches!(
        cursor.next_cancellable(&mut reader, &|| stop.get()),
        EventRead::Stopped
    ));
    assert_eq!(reader.fills, 0);
    assert_eq!(
        read_headers_cancellable(&mut reader, &|| stop.get()),
        Err(EventStreamError::Cancelled)
    );
    assert_eq!(reader.fills, 0);

    let mut emissions = 0;
    for sequence in [1, 3] {
        stop.set(false);
        let mut cursor = EventCursor::new(0);
        let pending =
            cursor.next_cancellable(&mut Cursor::new(frame(sequence, "text")), &|| stop.get());
        assert!(matches!(
            &pending,
            EventRead::Event(_) | EventRead::ResyncRequired
        ));
        // This is the same post-read admission used by forward_events.
        stop.set(true);
        match pending.if_active(&|| stop.get()) {
            EventRead::Event(_) | EventRead::ResyncRequired => emissions += 1,
            EventRead::Stopped => {}
            _ => panic!("cancelled delivery did not stop"),
        }
    }
    assert_eq!(emissions, 0);
}

#[test]
fn already_cancelled_connection_never_connects_or_reads_headers() {
    use std::net::TcpListener;
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    listener.set_nonblocking(true).unwrap();
    let result = super::super::open_event_stream_cancellable(
        listener.local_addr().unwrap(),
        "fixture-only",
        0,
        &|| true,
    );
    assert!(matches!(result, Err(EventStreamError::Cancelled)));
    assert!(
        matches!(listener.accept(), Err(error) if error.kind() == std::io::ErrorKind::WouldBlock)
    );
}
