//! Standalone, opt-in benchmark for the production terminal router.
//!
//! Build this file as a scratch Cargo binary with the same serde/serde_json/
//! base64 versions as the host, and set REASONIX_TERMINAL_BASELINE_RS to the
//! archived terminal.rs with its opening inner docs changed to ordinary comments.
//! It includes both real routers; the unused HTTP bridge is deliberately inert.
//! No allocator or benchmark code enters the application or its test binary.
#![allow(dead_code)]

#[path = "protocol_generated.rs"]
mod protocol_generated;
#[path = "terminal.rs"]
mod terminal;
mod baseline {
    include!(env!("REASONIX_TERMINAL_BASELINE_RS"));
}

mod bridge {
    use serde_json::Value;
    use std::{net::SocketAddr, time::Duration};
    pub(crate) fn session_path_component(_: &str) -> Result<String, String> {
        panic!("nonterminal frames must not enter terminal validation")
    }
    pub(crate) fn request_json_with_timeout(
        _: SocketAddr,
        _: &str,
        _: &str,
        _: &str,
        _: Option<Value>,
        _: Option<&str>,
        _: Duration,
    ) -> Result<Value, String> {
        panic!("the benchmark must not access the network")
    }
}

use protocol_generated::BridgeEvent;
use std::{
    alloc::{GlobalAlloc, Layout, System},
    hint::black_box,
    sync::atomic::{AtomicBool, AtomicUsize, Ordering},
    time::Instant,
};

struct CountingAllocator;
static TRACK: AtomicBool = AtomicBool::new(false);
static ALLOCATIONS: AtomicUsize = AtomicUsize::new(0);
static BYTES: AtomicUsize = AtomicUsize::new(0);

fn count(bytes: usize) {
    if TRACK.load(Ordering::Relaxed) {
        ALLOCATIONS.fetch_add(1, Ordering::Relaxed);
        BYTES.fetch_add(bytes, Ordering::Relaxed);
    }
}

unsafe impl GlobalAlloc for CountingAllocator {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        count(layout.size());
        // Safety: forward the allocator contract unchanged to System.
        unsafe { System.alloc(layout) }
    }
    unsafe fn alloc_zeroed(&self, layout: Layout) -> *mut u8 {
        count(layout.size());
        // Safety: forward the allocator contract unchanged to System.
        unsafe { System.alloc_zeroed(layout) }
    }
    unsafe fn dealloc(&self, pointer: *mut u8, layout: Layout) {
        // Safety: pointer and layout are from this System-backed allocator.
        unsafe { System.dealloc(pointer, layout) }
    }
    unsafe fn realloc(&self, pointer: *mut u8, layout: Layout, size: usize) -> *mut u8 {
        count(size);
        // Safety: retain the caller's valid allocation and reallocation layout.
        unsafe { System.realloc(pointer, layout, size) }
    }
}

#[global_allocator]
static ALLOCATOR: CountingAllocator = CountingAllocator;
type Route = fn(&BridgeEvent) -> Option<Result<BridgeEvent, String>>;

fn invoke(route: Route, event: &BridgeEvent, iterations: usize) {
    for _ in 0..iterations {
        assert!(black_box(route)(black_box(event)).is_none());
    }
}

fn allocation_counts(route: Route, event: &BridgeEvent) -> (usize, usize) {
    ALLOCATIONS.store(0, Ordering::Relaxed);
    BYTES.store(0, Ordering::Relaxed);
    TRACK.store(true, Ordering::Relaxed);
    invoke(route, event, 10);
    TRACK.store(false, Ordering::Relaxed);
    (
        ALLOCATIONS.load(Ordering::Relaxed) / 10,
        BYTES.load(Ordering::Relaxed) / 10,
    )
}

fn sample(route: Route, event: &BridgeEvent, iterations: usize) -> f64 {
    let start = Instant::now();
    invoke(route, event, iterations);
    start.elapsed().as_secs_f64() * 1e9 / iterations as f64
}

fn main() {
    let mut cases = Vec::new();
    for (kind, bytes, iterations) in [
        ("text", 256, 500_000),
        ("tool_progress", 64 << 10, 10_000),
        ("tool_result", 1 << 20, 1_000),
    ] {
        let event: BridgeEvent = serde_json::from_value(serde_json::json!({
            "protocolVersion": 1, "sequence": 71, "sessionId": "owned", "tabId": "tab-a",
            "eventKind": kind, "payload": {"kind": kind, "tool": {"id":"fixture", "name":"fixture", "output":"x".repeat(bytes)}}
        })).unwrap();
        invoke(baseline::terminal_event, &event, 100);
        invoke(terminal::terminal_event, &event, 100);
        let (old_allocations, old_bytes) = allocation_counts(baseline::terminal_event, &event);
        let (new_allocations, new_bytes) = allocation_counts(terminal::terminal_event, &event);
        assert_eq!((new_allocations, new_bytes), (0, 0));
        assert!(old_allocations > 0 && old_bytes >= bytes);
        let mut before = Vec::new();
        let mut after = Vec::new();
        for sample_index in 0..7 {
            // Alternate order to avoid consistently favoring one temperature.
            if sample_index % 2 == 0 {
                before.push(sample(baseline::terminal_event, &event, iterations));
                after.push(sample(terminal::terminal_event, &event, iterations));
            } else {
                after.push(sample(terminal::terminal_event, &event, iterations));
                before.push(sample(baseline::terminal_event, &event, iterations));
            }
        }
        before.sort_by(f64::total_cmp);
        after.sort_by(f64::total_cmp);
        cases.push(serde_json::json!({
            "kind":kind, "payload_text_bytes":bytes, "iterations_per_sample":iterations,
            "samples":7, "baseline_ns_per_call_median":before[3], "current_ns_per_call_median":after[3],
            "baseline_ns_per_call_samples":before, "current_ns_per_call_samples":after,
            "baseline_allocations_per_call":old_allocations, "current_allocations_per_call":new_allocations,
            "baseline_allocated_bytes_per_call":old_bytes, "current_allocated_bytes_per_call":new_bytes
        }));
    }
    println!(
        "{}",
        serde_json::to_string_pretty(&serde_json::json!({
            "scope":"router only; event construction, JSON decode/IPC and UI rendering excluded",
            "latency_allocation_tracking":false, "allocation_count_iterations":10, "cases":cases
        }))
        .unwrap()
    );
}
