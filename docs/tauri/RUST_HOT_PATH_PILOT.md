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
