//! Release benchmark of the production HTTP decoder and generated history DTO.
//! cargo run --release --locked --example history_decode_bench
#[path = "../src/bridge/http_response.rs"]
mod http_response;
#[path = "../src/protocol_generated.rs"]
mod protocol;

use protocol::BridgeHistoryResponse;
use serde_json::{json, Value};
use std::{
    alloc::{GlobalAlloc, Layout, System},
    cell::Cell,
    hint::black_box,
    time::Instant,
};

struct Metered;
#[derive(Clone, Copy, Default)]
struct Allocation {
    calls: u64,
    bytes: u64,
}
thread_local! { static METER: Cell<Option<Allocation>> = const { Cell::new(None) }; }
fn allocated(size: usize) {
    let _ = METER.try_with(|meter| {
        if let Some(mut count) = meter.get() {
            count.calls += 1;
            count.bytes += size as u64;
            meter.set(Some(count));
        }
    });
}
unsafe impl GlobalAlloc for Metered {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        let result = unsafe { System.alloc(layout) };
        if !result.is_null() {
            allocated(layout.size());
        }
        result
    }
    unsafe fn alloc_zeroed(&self, layout: Layout) -> *mut u8 {
        let result = unsafe { System.alloc_zeroed(layout) };
        if !result.is_null() {
            allocated(layout.size());
        }
        result
    }
    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        unsafe { System.dealloc(ptr, layout) };
    }
    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, size: usize) -> *mut u8 {
        let result = unsafe { System.realloc(ptr, layout, size) };
        if !result.is_null() {
            allocated(size);
        }
        result
    }
}
#[global_allocator]
static ALLOCATOR: Metered = Metered;
fn speech_error(_: u16, _: &str) -> Option<String> {
    None
}

fn decode(response: &[u8], direct: bool) -> BridgeHistoryResponse {
    if direct {
        http_response::parse_json_response_as(response, speech_error).unwrap()
    } else {
        let value: Value = http_response::parse_json_response_as(response, speech_error).unwrap();
        serde_json::from_value(value).unwrap()
    }
}
fn timing(response: &[u8], direct: bool, iterations: usize) -> u128 {
    let start = Instant::now();
    for _ in 0..iterations {
        black_box(decode(black_box(response), direct));
    }
    start.elapsed().as_nanos() / iterations as u128
}
fn allocation(response: &[u8], direct: bool) -> Allocation {
    METER.with(|meter| meter.set(Some(Allocation::default())));
    black_box(decode(response, direct));
    METER.with(|meter| meter.replace(None).unwrap())
}
fn fixture(chars: usize) -> Vec<u8> {
    let text = "中文🙂 a quote: \"\n"
        .chars()
        .cycle()
        .take(chars)
        .collect::<String>();
    let messages=(0..200).map(|i|json!({"role":if i%2==0 {"user"}else{"assistant"},"content":text,"truncated":false,"createdAtMs":i,"workDurationMs":250,"forkToken":"fixture-token"})).collect::<Vec<_>>();
    let body=serde_json::to_vec(&json!({
        "protocolVersion":1,"sequence":41,"startIndex":9800,"totalMessages":10000,
        "session":{"id":"fixture","title":"synthetic","path":"/fixture/a.jsonl","model":"fixture-model","state":"idle"},"messages":messages
    })).unwrap();
    let mut response = b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n".to_vec();
    response.extend_from_slice(&body);
    response
}
fn main() {
    let mut results = Vec::new();
    for (name, chars, iterations) in [("small_text", 64, 300), ("large_text", 16000, 8)] {
        let response = fixture(chars);
        assert_eq!(
            serde_json::to_value(decode(&response, false)).unwrap(),
            serde_json::to_value(decode(&response, true)).unwrap()
        );
        for _ in 0..3 {
            black_box(decode(&response, false));
            black_box(decode(&response, true));
        }
        let mut old = Vec::new();
        let mut direct = Vec::new();
        for trial in 0..7 {
            // Alternate order to avoid crediting one route with warm-cache bias.
            if trial % 2 == 0 {
                old.push(timing(&response, false, iterations));
                direct.push(timing(&response, true, iterations));
            } else {
                direct.push(timing(&response, true, iterations));
                old.push(timing(&response, false, iterations));
            }
        }
        old.sort_unstable();
        direct.sort_unstable();
        let before = allocation(&response, false);
        let after = allocation(&response, true);
        results.push(json!({"case":name,"messages":200,"characters_per_message":chars,"http_bytes":response.len(),
            "old_median_ns":old[3],"direct_median_ns":direct[3],"speedup":old[3] as f64/direct[3] as f64,
            "old_alloc_calls":before.calls,"direct_alloc_calls":after.calls,
            "old_allocated_bytes":before.bytes,"direct_allocated_bytes":after.bytes,
            "allocation_measure":"current-thread cumulative requested bytes, including realloc; excludes input fixture and is not peak RSS",
            "samples_per_route":7,"iterations_per_sample":iterations,"contract_equal":true}));
    }
    println!("{}",serde_json::to_string_pretty(&json!({"scope":"production HTTP decode plus generated history DTO, excluding network/Go projection/Tauri IPC","profile":"release","results":results})).unwrap());
}
