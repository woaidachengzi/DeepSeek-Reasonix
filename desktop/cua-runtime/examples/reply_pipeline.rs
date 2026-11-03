//! Offline reply-only benchmark/worker: no driver, capture, input or thumbnails.
#[allow(dead_code)]
#[path = "../src/reply.rs"]
mod reply;

use serde::Deserialize;
use serde_json::{json, Value};
use std::alloc::{GlobalAlloc, Layout, System};
use std::io::{self, BufRead, Write};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::time::Instant;

struct Counter;
static COUNTING: AtomicBool = AtomicBool::new(false);
static ALLOCATIONS: AtomicU64 = AtomicU64::new(0);
static BYTES: AtomicU64 = AtomicU64::new(0);

unsafe impl GlobalAlloc for Counter {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        if COUNTING.load(Ordering::Relaxed) {
            ALLOCATIONS.fetch_add(1, Ordering::Relaxed);
            BYTES.fetch_add(layout.size() as u64, Ordering::Relaxed);
        }
        // Safety: forward the caller's allocation contract unchanged.
        unsafe { System.alloc(layout) }
    }
    unsafe fn alloc_zeroed(&self, layout: Layout) -> *mut u8 {
        if COUNTING.load(Ordering::Relaxed) {
            ALLOCATIONS.fetch_add(1, Ordering::Relaxed);
            BYTES.fetch_add(layout.size() as u64, Ordering::Relaxed);
        }
        // Safety: forward the caller's zeroed allocation contract unchanged.
        unsafe { System.alloc_zeroed(layout) }
    }
    unsafe fn realloc(&self, pointer: *mut u8, old: Layout, new_size: usize) -> *mut u8 {
        if COUNTING.load(Ordering::Relaxed) {
            ALLOCATIONS.fetch_add(1, Ordering::Relaxed);
            BYTES.fetch_add(new_size as u64, Ordering::Relaxed);
        }
        // Safety: retain the caller's valid allocation and reallocation layout.
        unsafe { System.realloc(pointer, old, new_size) }
    }
    unsafe fn dealloc(&self, pointer: *mut u8, layout: Layout) {
        // Safety: pointer and layout came from this System-backed allocator.
        unsafe { System.dealloc(pointer, layout) }
    }
}

#[global_allocator]
static ALLOCATOR: Counter = Counter;
const LIMIT: usize = 15 << 20;

#[derive(Deserialize)]
struct FixtureImage {
    mime_type: String,
    data_base64: String,
}

#[derive(Deserialize)]
struct Fixture {
    images: Vec<FixtureImage>,
    #[serde(default)]
    structured_json: Option<String>,
    #[serde(default)]
    text: String,
    #[serde(default)]
    action: Option<Value>,
    #[serde(default)]
    is_error: bool,
    #[serde(default)]
    error_code: Option<String>,
    #[serde(default)]
    preview: Option<Value>,
}

fn context(id: u64) -> reply::Context<'static> {
    reply::Context {
        protocol: 1,
        id,
        version: "fixture",
        source_revision: "fixture",
        source_fingerprint: "fixture",
    }
}

fn encode(fixture: &Fixture, id: u64, route: &str) -> serde_json::Result<Vec<u8>> {
    let structured = reply::structured(fixture.structured_json.as_deref());
    let refused = reply::refused(&structured, fixture.action.as_ref());
    match route {
        // Original 0.1.72 construction, retaining the same full image strings.
        "legacy" => {
            let context = context(id);
            let mut value = json!({
                "protocol":context.protocol,"id":context.id,"driver":"cua","version":context.version,
                "result":structured,"text":fixture.text,
                "images":fixture.images.iter().map(|image|json!({"mime_type":image.mime_type,"data_base64":image.data_base64})).collect::<Vec<_>>(),
                "action":fixture.action,"is_error":fixture.is_error || refused,
                "error_code":fixture.error_code,"preview":fixture.preview,
                "source_revision":context.source_revision,"source_fingerprint":context.source_fingerprint
            });
            let encoded = serde_json::to_vec(&value)?;
            if encoded.len() <= LIMIT {
                return Ok(encoded);
            }
            value.as_object_mut().unwrap().remove("preview");
            serde_json::to_vec(&value)
        }
        "borrowed" => {
            let mut value = reply::Success::new(
                context(id),
                reply::SuccessData {
                    result: &structured,
                    text: &fixture.text,
                    images: reply::Images(fixture.images.iter().map(|image| reply::Image {
                        data_base64: &image.data_base64,
                        mime_type: &image.mime_type,
                    })),
                    action: fixture.action.as_ref(),
                    is_error: fixture.is_error || refused,
                    error_code: fixture.error_code.as_deref(),
                    preview: fixture.preview.as_ref(),
                },
            );
            value.encode(LIMIT)
        }
        _ => Err(<serde_json::Error as serde::ser::Error>::custom(
            "unknown reply route",
        )),
    }
}

fn benchmark(
    route: &str,
    size: usize,
    iterations: usize,
) -> Result<(), Box<dyn std::error::Error>> {
    if iterations == 0 || size > 8 << 20 {
        return Err("positive iterations and at most 8MiB fixture required".into());
    }
    let fixture = Fixture {
        images: if size == 0 {
            vec![]
        } else {
            vec![FixtureImage {
                mime_type: "image/png".into(),
                data_base64: "A".repeat(size),
            }]
        },
        structured_json: Some(
            r#"{"mode":"snapshot","capture_id":"synthetic","status":"ok"}"#.into(),
        ),
        text: "Synthetic reply only".into(),
        action: None,
        is_error: false,
        error_code: None,
        preview: None,
    };
    let baseline = encode(&fixture, 1, "legacy")?;
    assert_eq!(baseline, encode(&fixture, 1, "borrowed")?);
    let encoded_bytes = baseline.len();
    std::hint::black_box(encode(&fixture, 1, route)?);
    ALLOCATIONS.store(0, Ordering::Relaxed);
    BYTES.store(0, Ordering::Relaxed);
    let allocation_iterations = iterations.min(10);
    COUNTING.store(true, Ordering::Relaxed);
    for _ in 0..allocation_iterations {
        std::hint::black_box(encode(&fixture, 1, route)?);
    }
    COUNTING.store(false, Ordering::Relaxed);
    let allocations = ALLOCATIONS.load(Ordering::Relaxed);
    let bytes = BYTES.load(Ordering::Relaxed);
    // Time separately: enabled atomic counters would penalize routes that
    // allocate more and conflate instrumentation cost with serialization work.
    let started = Instant::now();
    for _ in 0..iterations {
        std::hint::black_box(encode(&fixture, 1, route)?);
    }
    let elapsed = started.elapsed();
    println!(
        "{}",
        json!({"route":route,"base64_bytes":size,"iterations":iterations,"allocation_iterations":allocation_iterations,"ns_per_op":elapsed.as_nanos() as f64 / iterations as f64,"rust_allocations_per_op":allocations as f64 / allocation_iterations as f64,"rust_allocated_bytes_per_op":bytes as f64 / allocation_iterations as f64,"encoded_bytes":encoded_bytes})
    );
    Ok(())
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let arguments: Vec<_> = std::env::args().collect();
    if arguments.get(1).is_some_and(|arg| arg == "--bench") {
        if arguments.len() != 5 {
            return Err(
                "usage: reply_pipeline --bench legacy|borrowed BASE64_BYTES ITERATIONS".into(),
            );
        }
        return benchmark(&arguments[2], arguments[3].parse()?, arguments[4].parse()?);
    }
    let mut input = io::stdin().lock().lines();
    let fixture: Fixture = serde_json::from_str(&input.next().ok_or("missing fixture")??)?;
    let mut output = io::stdout().lock();
    for (index, line) in input.enumerate() {
        let encoded = encode(&fixture, index as u64 + 1, &line?)?;
        if encoded.len() > LIMIT {
            break;
        }
        output.write_all(&encoded)?;
        output.write_all(b"\n")?;
        output.flush()?;
    }
    Ok(())
}
