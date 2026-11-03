//! Standalone release harness for production SSE framing and cursor admission.
//! Use the host's locked serde/serde_json versions. Set the baseline source env
//! to archived 0.1.72 event_stream.rs with inner docs made ordinary comments.
#![allow(dead_code)]

#[path = "event_stream.rs"]
mod candidate;
#[path = "http_response.rs"]
mod http_response;
#[path = "../protocol_generated.rs"]
mod protocol_generated;
mod baseline {
    include!(env!("REASONIX_SSE_BASELINE_RS"));
}

use protocol_generated::BridgeEvent;
use std::{
    alloc::{GlobalAlloc, Layout, System},
    hint::black_box,
    io::{BufReader, Cursor},
    sync::atomic::{AtomicBool, AtomicUsize, Ordering},
    time::Instant,
};
const PROTOCOL_VERSION: u8 = 1;

struct CountingAllocator;
static TRACK: AtomicBool = AtomicBool::new(false);
static ALLOCATIONS: AtomicUsize = AtomicUsize::new(0);
static BYTES: AtomicUsize = AtomicUsize::new(0);

fn count(size: usize) {
    if TRACK.load(Ordering::Relaxed) {
        ALLOCATIONS.fetch_add(1, Ordering::Relaxed);
        BYTES.fetch_add(size, Ordering::Relaxed);
    }
}

unsafe impl GlobalAlloc for CountingAllocator {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        count(layout.size());
        // Safety: forward System's allocation contract unchanged.
        unsafe { System.alloc(layout) }
    }
    unsafe fn alloc_zeroed(&self, layout: Layout) -> *mut u8 {
        count(layout.size());
        // Safety: forward System's allocation contract unchanged.
        unsafe { System.alloc_zeroed(layout) }
    }
    unsafe fn dealloc(&self, pointer: *mut u8, layout: Layout) {
        // Safety: this pointer and layout came from System.
        unsafe { System.dealloc(pointer, layout) }
    }
    unsafe fn realloc(&self, pointer: *mut u8, layout: Layout, size: usize) -> *mut u8 {
        count(size);
        // Safety: retain the caller's allocation and new layout contract.
        unsafe { System.realloc(pointer, layout, size) }
    }
}

#[global_allocator]
static ALLOCATOR: CountingAllocator = CountingAllocator;

type Decode = fn(&[u8], usize) -> (u64, usize);

fn decode_baseline(bytes: &[u8], capacity: usize) -> (u64, usize) {
    let mut reader = BufReader::with_capacity(capacity, Cursor::new(bytes));
    baseline::read_headers_cancellable(&mut reader, &|| false).unwrap();
    let mut cursor = baseline::EventCursor::new(0);
    let mut count = 0;
    loop {
        match cursor.next_cancellable(&mut reader, &|| false) {
            baseline::EventRead::Event(event) => {
                black_box(event);
                count += 1;
            }
            baseline::EventRead::Ignored => {}
            baseline::EventRead::Reconnect => return (cursor.after_sequence(), count),
            _ => panic!("valid fixture required resync or cancellation"),
        }
    }
}

fn decode_candidate(bytes: &[u8], capacity: usize) -> (u64, usize) {
    let mut reader = BufReader::with_capacity(capacity, Cursor::new(bytes));
    candidate::read_headers_cancellable(&mut reader, &|| false).unwrap();
    let mut cursor = candidate::EventCursor::new(0);
    let mut count = 0;
    loop {
        match cursor.next_cancellable(&mut reader, &|| false) {
            candidate::EventRead::Event(event) => {
                black_box(event);
                count += 1;
            }
            candidate::EventRead::Ignored => {}
            candidate::EventRead::Reconnect => return (cursor.after_sequence(), count),
            _ => panic!("valid fixture required resync or cancellation"),
        }
    }
}

fn fixture(events: usize, large_every: usize) -> Vec<u8> {
    let mut bytes =
        b"HTTP/1.0 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n".to_vec();
    for sequence in 1..=events {
        let large = large_every > 0 && sequence % large_every == 0;
        let kind = if large {
            "tool_result"
        } else if sequence % 17 == 0 {
            "terminal_output"
        } else {
            "text"
        };
        let text = if large {
            "x".repeat(1 << 20)
        } else {
            "中🙂 synthetic text".into()
        };
        let event = serde_json::json!({"protocolVersion":1,"sequence":sequence,"eventKind":kind,
            "sessionId":format!("owned-{}",sequence % 16),"payload":{"kind":kind,"text":text}});
        let frame = format!("id: {sequence}\nevent: {kind}\ndata: {event}\n\n");
        bytes.extend_from_slice(frame.as_bytes());
        if sequence % 64 == 0 {
            bytes.extend_from_slice(b": heartbeat\n\n");
        }
    }
    bytes
}

fn main() {
    let arguments: Vec<_> = std::env::args().skip(1).collect();
    let [route, iterations] = arguments.as_slice() else {
        panic!("usage: sse-bench baseline|candidate iterations");
    };
    let decode: Decode = match route.as_str() {
        "baseline" => decode_baseline,
        "candidate" => decode_candidate,
        _ => panic!("unknown route"),
    };
    let iterations: usize = iterations.parse().unwrap();
    assert!(iterations > 0);
    let mut rows = Vec::new();
    for (name, events, large_every) in [("text10K", 10_000, 0), ("mixed1MiB", 1_024, 256)] {
        let bytes = fixture(events, large_every);
        for capacity in [7, 8192] {
            let expected = (events as u64, events);
            assert_eq!(decode(&bytes, capacity), expected);
            ALLOCATIONS.store(0, Ordering::Relaxed);
            BYTES.store(0, Ordering::Relaxed);
            TRACK.store(true, Ordering::Relaxed);
            let result = decode(&bytes, capacity);
            TRACK.store(false, Ordering::Relaxed);
            assert_eq!(result, expected);
            let allocations = ALLOCATIONS.load(Ordering::Relaxed);
            let allocated_bytes = BYTES.load(Ordering::Relaxed);
            let start = Instant::now();
            for _ in 0..iterations {
                assert_eq!(black_box(decode)(black_box(&bytes), capacity), expected);
            }
            rows.push(serde_json::json!({"workload":name,"events":events,"wire_bytes":bytes.len(),
                "reader_capacity":capacity,"iterations":iterations,"ns_per_stream":start.elapsed().as_secs_f64()*1e9/iterations as f64,
                "allocation_calls_per_stream":allocations,"allocated_bytes_per_stream":allocated_bytes}));
        }
    }
    println!(
        "{}",
        serde_json::json!({"route":route,"scope":"Complete production HTTP header + SSE line + BridgeEvent JSON decode + global cursor admission. No socket, routing, renderer or model. Allocated bytes are cumulative, not RSS.","rows":rows})
    );
}
