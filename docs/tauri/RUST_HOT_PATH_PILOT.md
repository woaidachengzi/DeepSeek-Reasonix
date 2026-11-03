# Rust hot-path pilot — 2026-11-03

This is a measured first migration slice on the macOS Cua edition. The stable
comparison commit is `c8c68f599209be3dc2e4259285ee1c0356574df6` (0.1.69).
The candidate implements one Go computation in Rust and removes repeat work
from the existing Go/Rust event and history paths. It does not remove the Go
core or change the canonical session writer, provider sampling or approvals.

## Scope and ownership

| Scenario | Implemented or observed |
| --- | --- |
| Screenshot preview | Read-only `observe`/`browser_observe` thumbnail work moves to the existing Rust worker. The complete model image and capture/coordinate identity remain unchanged. Go validates a small optional JPEG and keeps its previous fallback. Write actions continue the post-reply Go preview path. |
| High-frequency events | Go replay becomes a lazy bounded ring; live answer append uses an append-only builder. Rust rejects nonterminal frames before cloning their payload. |
| History decode | Rust decodes HTTP bytes directly into the generated display DTO, avoiding an intermediate JSON tree. Go still controls visibility and session state. |
| Streaming stability | Rust bounds each SSE frame/header, ignores duplicates, requests one fresh snapshot on a gap/oversize/bad frame and stops that old stream. EOF reconnects with its last complete global cursor, including terminal frames. Cooperative cancellation is checked within reads and before publish; an in-progress socket operation retains its existing one-second timeout. |
| Model/network wait | No language rewrite is claimed to reduce external generation or network time. No paid model request was used for this trial. |
| React/WebView scroll | Existing transcript/session integration checks are retained. A synthetic Node history benchmark is not a native WebView frame-time measurement, and no renderer speedup is claimed. |

## Serial offline comparisons

Apple M4 / macOS ARM64, Go and Rust release profiles recorded in
[`benchmark.json`](evidence/2026-11-03-rust-hot-paths/benchmark.json).
All data are generated fixtures. The runner builds exact current source,
uses the host's locked dependency versions, alternates old/new Rust samples
and reverses the Go route order across three rounds. There are no CPU or
allocation timing gates in CI. Raw records include source hashes and the
candidate's worktree status; that snapshot precedes the final release commit.

Screenshot pipeline medians include a persistent worker, full screenshot JSON
encoding, pipes, Go decode and thumbnail work/validation. Worker startup, SDK
capture, model requests and renderer work are excluded. MB below are cumulative
**Go allocations per operation**, excluding the Rust worker, not whole-app RSS.

| Fixture | Previous pipeline | Rust preview pipeline | Go allocated bytes |
| --- | ---: | ---: | ---: |
| 640x400 PNG | 4.20 ms | 3.60 ms | 2.51 → 0.95 MB |
| 640x400 JPEG | 6.33 ms | 4.51 ms | 2.58 → 1.00 MB |
| 1920x1080 PNG | 12.62 ms | 8.11 ms | 10.20 → 0.90 MB |
| 1920x1080 JPEG | 19.78 ms | 11.07 ms | 6.50 → 1.28 MB |
| 1080x1920 PNG | 12.55 ms | 7.42 ms | 10.18 → 0.95 MB |
| 1080x1920 JPEG | 19.89 ms | 11.61 ms | 6.62 → 1.36 MB |

Event medians cover the synthetic complete Go publish/checkpoint path with an
answer snapshot. Both versions use the same event workload; this is an
algorithm/data-structure comparison within Go, not a Go-versus-Rust claim.

| Fixture | Previous | Ring + builder | Go allocated bytes |
| --- | ---: | ---: | ---: |
| 512 deltas | 0.68 ms | 0.50 ms | 1.81 → 0.70 MB |
| 2048 deltas | 6.46 ms | 2.03 ms | 20.46 → 2.74 MB |
| 4096 deltas | 17.97 ms | 4.08 ms | 75.97 → 5.20 MB |

Rust history decoding of 200 small messages was 0.154 → 0.102 ms, with 2,030 →
615 allocations. A 5.14 MB text-heavy reply was 9.09 → 8.98 ms: effectively flat,
so this trial does not establish a general large-history latency improvement.
Nonterminal routing allocates zero bytes in the isolated Rust harness; the
previous 1 MiB tool-result fixture copied approximately 1.05 MB per routing call.
Those router figures exclude JSON decode, IPC and rendering.

## Compatibility and validation

The preview is display-only: source ≤5 MiB data URL, decoded edges ≤4096,
preview edges ≤480, JPEG body ≤40 KiB. An invalid/missing optional preview falls
back to Go. If it would exceed the 15 MiB worker reply cap, it is removed before
returning the original action result. The exact host switch
`REASONIX_CUA_RUST_PREVIEW=0` disables this slice without widening the sanitized
worker environment. Older workers remain compatible through missing-field
fallback; the pinned SDK revision/fingerprint are unchanged.

Regression tests cover payload/snapshot isolation, repeated ring wrap,
future cursors from another process lifetime, Unicode limits, malformed preview
fields and image bodies, write-action fallback, unchanged full model images,
HTTP errors/size bounds, global event gaps and cancelled slow partial reads.
The prior parallel prompt test mistook registered identities for actionable
prompts; both the old and new implementations reproduced it. It now waits for
the public owned-prompt reader and joins its fixtures during cleanup. A stale
single-runtime Rust test now asserts that a retained background owner keeps
its original workspace, and supervisor tests use isolated profiles. Production
prompt authority and queued guards were not relaxed.

## Reproduce and remaining work

```sh
python3 tools/tauri/bench-rust-hot-paths.py --output /tmp/reasonix-hot-path-bench
```

The runner is offline and requires the project's already installed toolchains
and locked dependencies. It creates only a scratch terminal benchmark and
synthetic fixtures; it does not capture the screen, send input or call models.

In 0.1.70, Go still projected the full live transcript before keeping the last
200 display rows. The follow-up below removes that repeated work while keeping
private provider messages in their existing owner. Whole-session storage,
model runtime, MCP and tool execution have not been migrated here.
Linux/Windows native acceptance and whole-application peak RSS were not measured.

## Follow-up: incremental history, 0.1.71

The real Tauri history route now asks its concrete Go runtime for a bounded
display page. A session-local opaque cursor atomically distinguishes an
unchanged transcript, an appended suffix and a prefix change. The runtime keeps
only 200 display rows, a rolling canonical-prefix hash and turn-usage carry.
An unchanged read copies the page; an append hashes/projects only new messages.
Initial reads and rewrites still scan the complete canonical transcript.
The old full projector remains the compatibility oracle and legacy-runtime
fallback. This is an algorithm improvement within Go, not another Rust rewrite:
moving raw provider data into Rust would widen the desktop display boundary.

The read fence includes session identity, persistence rewrite version and a
separate in-memory epoch. That epoch covers same-length Replace, unpersisted
tool-guard corrections, and save-time message-ID creation/renaming without
changing persistence conflict classification or the session schema. The cursor
holds a small independent identity token, not a pointer that retains the old
Session after replacement. Cached text owns its bounded string storage.

Usage includes hidden tool-call completions and earlier rows outside the page.
Blank visible users still reset usage before display filtering. Known local
no-deliverable failures retain their special accounting and are localized at
read time. A serialization error invalidates every fork token until a full
reset. Returned DTOs and nested usage counters are isolated from the cache.
Session writers remain authoritative and use immutable replacement of nested
published records; no provider reasoning, tool JSON, results or image data is
sent to the host by this cache.

### Serial HTTP comparison

The fixed comparison is the exact 0.1.70 commit
`cbf06ed4efc4ffea36c5fa1f7ff0846feda0fbc5`. The runner archives that source and
injects the same synthetic harness with a legacy constructor; the candidate
uses the real bounded controller method. Both execute the authenticated
production HTTP handler, runtime manager, controller/session history path and
JSON encoding. Runtime status/title are fixed fixture values. There is no
socket transport, Rust decoding, rendering, disk load, process startup or model
request in this measurement. Fixture sizes are raw provider-message counts;
approximately half project to display rows, and both routes return 200 rows.

Five serial rounds alternate old/new process order, with 20 measured operations
per workload. Preparation, replacing the fixture Session and prewarming are
untimed. Append includes adding two messages. Rewrite includes copying the
replacement log and Replace. Setup may leave background GC work, so the large
repeat-read improvement and bounded allocation trend are more useful than tiny
differences between cold/rewrite samples. Medians and individual samples,
toolchain, binary hashes and measured source hashes are recorded in
[`benchmark.json`](evidence/2026-11-03-incremental-history/benchmark.json).

| Raw fixture / operation | 0.1.70 | Incremental | Go allocated bytes per operation |
| --- | ---: | ---: | ---: |
| 1K / unchanged | 1.49 ms | 0.066 ms | 2.73 → 0.097 MB |
| 10K / unchanged | 12.33 ms | 0.067 ms | 25.28 → 0.097 MB |
| 50K / unchanged | 61.77 ms | 0.070 ms | 125.91 → 0.097 MB |
| 10K / append two | 12.42 ms | 0.155 ms | 25.28 → 0.103 MB |
| 50K / append two | 60.58 ms | 0.463 ms | 125.92 → 0.103 MB |
| 10K, 4KiB tool results / unchanged | 21.25 ms | 0.065 ms | 37.19 → 0.097 MB |
| 50K / initial projection | 63.53 ms | 59.42 ms | 125.92 → 115.54 MB |
| 50K / same-length rewrite | 64.37 ms | 61.19 ms | 154.72 → 144.34 MB |

Allocated MB are cumulative Go allocations, not peak RSS or retained cache
size. Cold scans and prefix rewrites remain O(total messages); this does not
establish any whole-application, WebView or model-generation speedup.

```sh
python3 tools/tauri/bench-history-page.py --output /tmp/reasonix-history-bench
```

The runner builds both binaries before timing, disables module downloads,
checks that measured source hashes did not change, and accepts Go's single-CPU
benchmark output format. Use a fresh output directory for every run.

### Verification and release boundaries

Full race suites passed for agent, control, desktopbridge and the desktop
bridge, including the existing persistence tests. Added parity coverage checks
200-row boundaries, hidden messages, cross-page usage, Unicode/legacy invalid
UTF-8, append, same-length replacement, rewind, session swapping, save-time IDs,
hash failure, localization and returned-pointer isolation. A production-handler
integration test checks exact JSON projection through append, rewrite and
replacement. Frontend history-surface and parallel-session integration tests
also passed. The full repository lint still reports its existing aggregate
struct-state/test-size ceilings; the fixed baseline reports the same totals.

The protocol, persisted session format, approval behavior and pinned Cua SDK
are unchanged. Rollback is the protected 0.1.70 package in
`desktop/tauri/target/releases.noindex/0.1.70/`, outside the bundler cleanup
directory; 0.1.71 is archived beside it. Native package checks are recorded in
that release's verification receipt. Linux/Windows native acceptance remains
outside this macOS batch.

## Follow-up: bounded cold rebuilds, 0.1.72

0.1.71 made unchanged reads and appends cheap; its first projection and prefix
rewrites still allocated a complete provider-message snapshot and constructed
display rows that were immediately evicted. A synthetic 50K cold-read CPU and
allocation profile identified full message-value copies, reflection/value
boxing and Marshal output copies as the main allocation sources. SHA-256 was
a small part of CPU time, so moving that calculation alone into Rust would
not address the measured problem.

The new Go path captures one request-local Session reader and fixes its prefix
identity/count. It copies at most 64 message values per short read lock into a
reused scratch buffer; hashing and display work happen after releasing the
lock. A reverse scan finds the last 200 actual display rows. The forward scan
still visits every message for prefix identity, total and usage, but constructs
text, tokens and usage snapshots only for that retained tail. Ordinary appends
keep the prior incremental path. This reduces allocations of full arrays, not
the total number of canonical message values visited or copied.

A prefix rewrite invalidates a chunked attempt. An independent candidate is
installed only after every chunk succeeds. At most two attempts are made,
then the original atomic Snapshot/Delta path supplies a consistent fallback.
Pure appends can advance the live Session while the reader consumes its fixed
earlier count. The request reader may retain its source Session; the resident
cache continues to hold only the small identity token and display projection.
Scratch references are cleared at return. No JSON hashing or callbacks run
while holding the Session lock, and no second canonical writer was added.

The prefix accumulator now borrows an addressable message only during encoding
and reuses a default JSON Encoder writing into the hash. Its bytes remain
exactly the original Marshal output followed by one newline; the independent
old prefix-token function remains the oracle. Encoder still buffers a complete
single message internally: this is not a streaming parser or constant-memory
claim for arbitrarily large individual messages. Encoding failure drops the
encoder/hash and retains only a static failure marker. Plain text without an
XML opening delimiter skips the wrapper regex copies; wrapper, handoff,
RawContent and legacy Memory v5 behavior retain their existing rules.

### Comparison with the already optimized 0.1.71

The exact baseline is `083cad4dc651f6c6a2fbd9a541f9a9098f0029d4`. Its archived
bounded constructor is retained, rather than replacing it with the older full
History fallback. The same 5-round, 20-operation, alternating-order HTTP harness
and timing boundaries described above are used. Candidate production source
hashes and binary hashes are recorded in
[`benchmark.json`](evidence/2026-11-03-history-rebuild/benchmark.json).

| Raw fixture / operation | 0.1.71 | 0.1.72 | Go allocated bytes per operation |
| --- | ---: | ---: | ---: |
| 1K / initial projection | 1.37 ms | 1.00 ms | 2.55 → 0.321 MB |
| 10K / initial projection | 11.72 ms | 8.66 ms | 23.22 → 0.323 MB |
| 50K / initial projection | 58.23 ms | 41.87 ms | 115.54 → 0.323 MB |
| 50K / same-length rewrite | 59.22 ms | 43.04 ms | 144.34 → 29.12 MB |
| 50K / unchanged | 0.065 ms | 0.064 ms | 0.097 → 0.097 MB |
| 50K / append two | 0.419 ms | 0.374 ms | 0.103 → 0.098 MB |
| 10K, 4KiB tool results / initial projection | 20.40 ms | 16.00 ms | 35.13 → 0.309 MB |

The 50K cold sample reduces cumulative Go allocated bytes by about 99.7% and
latency by about 28%. Rewrite includes the caller's full replacement-slice
copy, which remains necessary in that fixture and explains its larger byte
count. First projection and rewrites remain O(total messages), and the retry
fallback can still allocate an atomic full snapshot. These are in-process
synthetic HTTP measurements, not peak RSS, whole-app startup, WebView frame
time, model latency or a Go-versus-Rust benchmark.

The short 1K unchanged sample varied from 0.064 to 0.087 ms across versions.
A targeted follow-up using the same binaries, seven alternating rounds and
2,000 operations per round measured 0.0702 versus 0.0708 ms, with 625 allocations
in both versions. Both the original samples and the longer
[`warm check`](evidence/2026-11-03-history-rebuild/warm-check.json) are retained;
no small unchanged-read speedup is claimed.

```sh
python3 tools/tauri/bench-history-page.py \
  --baseline 083cad4dc651f6c6a2fbd9a541f9a9098f0029d4 \
  --output /tmp/reasonix-history-rebuild-bench
```

### Verification and remaining candidates

Full agent, control, desktopbridge and desktop-bridge race suites passed.
Additional deterministic cases cover reverse/forward chunk boundaries,
mid-build rewrites, no publication of partial candidates, bounded retries,
atomic fallback, fixed-count concurrent appends and cleared scratch references.
The original complete projector and prefix-token oracle check exact display,
usage, Unicode, privacy and fork-token parity.

The full desktop suite exposed an existing Ask/MCP fixture race: event receipt
and registered identity can precede clearing the queued flag. A test-only sink
barrier reproduces that window on 0.1.71. The fixture now waits for the exact
public scoped prompt reader and cancels/joins its goroutines before store
cleanup. Patched current and baseline tests each passed 20 race repetitions;
production prompt/permission guards and replay/owner rejection assertions are
unchanged. Existing full-repository lint baseline failures remain; scanned
finding weights did not increase from 0.1.71.

The next candidates were audited but are not implemented or assigned speedups
in this slice:

| Candidate | Next evidence needed |
| --- | --- |
| Go event publish re-parses JSON under the shared live-state lock | Mixed large tool events, parallel sessions and a mutex profile before replacing the local JSON round trip with typed deltas. |
| Rust SSE allocates a vector for each line | Complete decoder allocation/latency measurements while retaining frame caps, cancellation and resync behavior. |
| Rust Cua reply builds an intermediate JSON tree containing image strings | Typed-envelope versus current worker measurements, preserving full model images, identities, reply limits and optional-preview fallback. |

The pinned SDK, session format, provider behavior and approval policy are
unchanged. Protected 0.1.71 and 0.1.72 packages live outside the bundler cleanup
directory in `desktop/tauri/target/releases.noindex/`; each release's receipt
records package verification. Linux/Windows native acceptance and whole-app
peak memory remain unmeasured.

## Follow-up: event and reply pipelines, 0.1.73

This slice implements the three candidates identified after 0.1.72. The fixed
comparison commit is `bab3297ba129a0a8db5393e765a5638501713adb`. There is no SDK,
dependency, persisted session format, model sampling or permission-policy
change.

### Ownership and compatibility

Go Agent events still pass through the same ToWire and JSON encoding. Only
after encoding succeeds, a small typed public checkpoint update is prepared
before publication takes the shared mutex. Tool arguments/results and
reasoning are absent from that update. Publication, ledger sequencing, live
state changes and subscriber fanout remain atomic under the original lock.
Raw publishers retain the original JSON interpretation, parsed before the
lock. The wire ledger still retains the complete unchanged payload.

Text and tool names preserve JSON's per-invalid-byte UTF-8 replacement.
Preparation first takes a bounded rune prefix, avoiding a full invalid-input
rune allocation. Tool identifiers/names own their small string storage, and
answer text is copied by the existing builder. Limits, lifecycle resets,
private-content exclusion, inactive eviction, failed serialization and raw
malformed-payload behavior are checked against a frozen 0.1.72 JSON oracle.

The Rust host reuses one SSE line buffer and a local header buffer. Every
event/ignored/error/EOF/cancel path clears its line length; capacity above
64 KiB is dropped rather than retained after an exceptional large frame.
Returned BridgeEvent values own their fields. Header/frame byte limits,
global cursors, duplicate rejection, gap resync, EOF reconnect and cooperative
cancellation keep the original behavior.

The Cua worker serializes borrowed image strings directly into the reply,
avoiding the intermediate image Value trees. Small structured/action/preview
values keep their old representation. Exact compact JSON byte comparisons
check key order, source identity, full image data, null versus absent fields,
refused/error completion and original oversized-reply behavior. Crossing the
15 MiB limit still removes only optional preview, then leaves a still-oversized
original reply for the existing stop condition. No screenshot, coordinate,
input, authorization or retry decision is changed.

### Serial offline results

The runner builds everything before timing and alternates routes across five
rounds. Go event baselines are built from the exact archived source, with only
the shared public-API benchmark injected. SSE includes both actual decoders
with the host's locked serde versions. Cua legacy and borrowed constructors
are checked for identical bytes before measurement. Rust allocation counting
is separate from CPU timing. Source, runner and binary hashes, individual
samples and medians are in
[`benchmark.json`](evidence/2026-11-03-event-pipelines/benchmark.json).

Go measurements include typed Sink, wire encoding, the ledger's full payload
copy, checkpoint and optional subscriber delivery/drain. Synthetic source
setup and ring prefill are excluded. These are single-event medians; model,
socket and rendering work are excluded.

| Go event fixture | 0.1.72 | 0.1.73 |
| --- | ---: | ---: |
| Text, 1 session, no subscriber | 0.90 µs | 0.74 µs |
| 4KiB tool result, 1 session, no subscriber | 7.66 µs | 3.73 µs |
| 1MiB tool result, 1 session, no subscriber | 1.292 ms | 0.494 ms |
| 1MiB tool result, 16 sessions, 1 subscriber | 1.280 ms | 0.493 ms |

Large-event Go allocations remain approximately 2.14 MB/operation: JSON wire
and ledger copies still exist. The gain comes mainly from avoiding the local
JSON decode and shortening the shared critical section. Separate 100-operation
parallel mutex diagnostics support that interpretation; their aggregated
blocking samples are not a latency percentile or a timing gate. Raw
[baseline](evidence/2026-11-03-event-pipelines/baseline-mutex.txt) and
[candidate](evidence/2026-11-03-event-pipelines/candidate-mutex.txt) profiles are
retained separately from unprofiled timing.

SSE measurements include HTTP headers, actual Go-style id/event/data/blank
framing, BridgeEvent JSON decode and global cursor admission. Fixtures contain
public synthetic text and mixed terminal/tool frames. The 7-byte reader is a
fragmentation stress case; 8192 bytes matches the ordinary buffer scale.

| Complete SSE stream | 0.1.72 | 0.1.73 | Allocation calls per stream |
| --- | ---: | ---: | ---: |
| 10K small frames, 8192-byte reader | 4.43 ms | 3.76 ms | 110,519 → 70,007 |
| 10K small frames, 7-byte reader | 8.47 ms | 5.31 ms | 189,739 → 70,011 |
| Mixed 1MiB frames, 8192-byte reader | 2.03 ms | 1.94 ms | 11,353 → 7,225 |

Cua reply-only measurements exclude SDK dispatch, screen capture, thumbnail
calculation and model requests. The isolated Rust path includes construction
and JSON encoding; the persistent-worker pipe path adds complete Go decoding
and full image/source identity validation. Original base64 image size and
encoded output bytes are identical across routes.

| Cua reply fixture | Legacy tree | Borrowed envelope | Rust allocation requests |
| --- | ---: | ---: | ---: |
| 64KiB image, isolated Rust | 0.023 ms | 0.020 ms | 0.333 → 0.198 MB |
| 1MiB image, isolated Rust | 0.352 ms | 0.339 ms | 5.248 → 3.147 MB |
| 8MiB image, isolated Rust | 3.004 ms | 2.370 ms | 41.948 → 25.167 MB |
| 8MiB image, full warm pipe/Go decode | 17.070 ms | 16.124 ms | Go remains 8.390 MB |

The original 20-operation 1MiB pipe batches were nearly flat, at 1.476 versus
1.514 ms. A targeted same-binary follow-up with seven alternating rounds of
200 operations measured 1.451 versus 1.403 ms and 8 Go allocations in both
routes. Both sets of samples are retained in the main report and
[`pipe check`](evidence/2026-11-03-event-pipelines/pipe-check.json). No blanket
full-pipeline speedup is claimed. The larger measured benefit is reduced Rust
temporary allocation, approximately 40% at 8MiB, with a modest pipe gain there.

Go B/op and counted Rust allocation requests are cumulative, not peak RSS.
Rust realloc accounting counts the requested new capacity. These fixtures do
not establish whole-application, capture, model or WebView latency changes.

```sh
python3 tools/tauri/bench-event-pipelines.py --output /tmp/reasonix-event-bench
```

### Verification and release

Full Go race suites passed for desktopbridge, computeruse, the desktop bridge,
eventwire and billing. A pre-existing eventwire compatibility test mixed wire
serialization with a changing catalog anchor and failed identically on the
fixed baseline. It now supplies clearly synthetic CostQuote data and retains
all aliases, basis and scheduling-metadata assertions, with stronger value
checks. Production billing, prices and serializer code are unchanged.

The adapter's 28 tests include exact reply bytes and real 15MiB boundary cases;
the host's 358 tests passed with 6 existing ignored tests. SSE lifecycle tests
cover both ordinary reuse and release after large/cancelled/invalid frames.
Production clippy, frontend/build contracts and package verification are
recorded in the release receipt. No real screen capture, user input, model
request or permissions change was used. Linux/Windows native acceptance and
whole-app peak RSS remain outside this macOS batch.

The 0.1.72 rollback and 0.1.73 release are archived under
`desktop/tauri/target/releases.noindex/`, outside bundler cleanup. Completing
these three measured slices does not remove the Go core or establish that all
application performance work is finished.

## Follow-up: reply buffer capacity, 0.1.74

The borrowed envelope still grew its JSON output vector repeatedly. It now
uses a saturating image/text byte estimate plus modest metadata allowance for
initial capacity, capped by the supplied reply budget (with a small minimum).
This is only an allocation hint: escaping and structured data can exceed it,
and the original actual-byte checks remain authoritative. Preview removal
clears and reuses the first output buffer instead of allocating another one.
The complete original images and wire bytes remain unchanged.

The same-byte oracle now also covers underestimated escaping, large structured
metadata and budgets of zero/one byte. All 29 adapter tests and production
clippy passed, including the existing real 15MiB boundaries. No SDK, dependency,
approval, capture or provider behavior changed.

Five alternating rounds compare the exact 0.1.73 normal-path `to_vec` encoding
with the capacity implementation, using identical below-cap fixtures. Rust
allocation counting is separate from latency timing. The persistent pipe adds
Go decode and complete image/source validation, excluding SDK dispatch,
capture, thumbnails and models. All samples and source hashes are in
[`benchmark.json`](evidence/2026-11-03-reply-capacity/benchmark.json).

| Base64 image body | Rust requested bytes, 0.1.73 → 0.1.74 | Isolated encoding | Complete warm pipe |
| --- | ---: | ---: | ---: |
| 64KiB | 0.198 → 0.067 MB | 0.0192 → 0.0173 ms | 0.123 → 0.119 ms |
| 1MiB | 3.147 → 1.050 MB | 0.287 → 0.268 ms | 1.385 → 1.358 ms |
| 8MiB | 25.167 → 8.390 MB | 2.180 → 2.182 ms | 15.477 → 15.773 ms |

The main gain is about two-thirds fewer cumulative Rust allocation requests
for these image bodies. Large-reply latency is effectively flat; no general
pipeline or whole-app speedup is claimed. These counters are not peak RSS.
Go decode allocation remains approximately unchanged.

```sh
python3 tools/tauri/bench-reply-capacity.py --output /tmp/reasonix-reply-capacity-bench
```

The protected 0.1.73 rollback and 0.1.74 package are stored outside bundler
cleanup in `desktop/tauri/target/releases.noindex/`. Native package verification
is recorded in the archived receipt; no native input or screenshot is used.
