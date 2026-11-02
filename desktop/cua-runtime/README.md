# Reasonix Cua edition

The macOS and GNU/Linux integration links the MIT Cua Driver Rust SDK from
[`trycua/cua` at `e754cfec69c41400eb2aabde716c27d15607b2f0`](https://github.com/trycua/cua/tree/e754cfec69c41400eb2aabde716c27d15607b2f0/libs/cua-driver/rust).
The linked driver reports version `0.38.0`. No reverse engineering or proprietary
Cua Spaces code is used.

## Build

Requires macOS 14+, Apple Command Line Tools, Rust, Go, Python 3.12+, Node 24 and
the frontend dependencies used by the regular Tauri build. From the repository:

```sh
python3 desktop/cua-runtime/scripts/build.py --bundle
```

Or use `pnpm tauri:build:cua` in `desktop/frontend`. To reuse an existing clean
upstream checkout at the exact pinned commit, pass `--source /path/to/cua`.
The script checks the revision, exports that commit into ignored
`desktop/cua-source`, builds with the committed Cargo.lock, generates notices,
and bundles both helpers in `Contents/MacOS`. The regular native build remains
available through `tauri:build` and `tauri.takeover.conf.json`.

Compatibility adjustments: sibling upstream crates receive an explicit
`workspace = "../.."` Cargo manifest entry; `apple-metal` stays at `0.6.0`, as in
the upstream lock, for the supported Command Line Tools SDK; our executable adds
`/usr/lib/swift` to its runtime search paths. Two Linux parent-module aliases use
equivalent fully qualified imports for Ubuntu's Rust 1.93. The patch's before/after
hashes ship in `reasonix-source-patches.json`. The Linux AT-SPI insert/fallback
lengths also use UTF-8 byte counts, as required by the
[GNOME interface](https://gnome.pages.gitlab.gnome.org/at-spi2-core/libatspi/method.EditableText.insert_text.html),
instead of character counts that truncate Chinese input. These two MIT source
patches are applied only after a clean pinned export and fail if the expected
source statements change. No MPL source files are modified.

The pinned SDK backports upstream
[capture encoding optimization `8fbdf83c1e`](https://github.com/trycua/cua/commit/8fbdf83c1e05bf0a8b84a6199fe697cc58f0e24f)
across three MIT runtime files.
macOS capture retains raw RGBA pixels until the final resize/encode, avoiding
an intermediate PNG encode/decode. Reasonix continues to request default PNG;
the screenshot dimensions and `capture_id` requirement for coordinate input are
unchanged. The SDK's optional JPEG path does not mint a `capture_id` and is not
exposed in the Reasonix tool schema. The base SDK revision stays pinned above.
`upstream.json` records the backport commit, patch checksum and each file's
before/after checksum. The build verifies all of them before applying the patch
to a clean export; the manifest, patch and audit records ship with the notices.
New files have a null input checksum and must be absent before patching. Patches
are applied in manifest order; the Wayland shortcut patch precedes libei routing.

## Upstream review (2026-10-31)

This is a selective backport onto the pinned SDK, not a full upgrade to upstream
Reasonix Studio or Cua Spaces. The reviewed candidates are accounted for below.

| Upstream change | Local disposition |
| --- | --- |
| [Pinned memory confirmation `4227c20b83`](https://github.com/esengine/DeepSeek-Reasonix/commit/4227c20b83741c792b35b8842b6c086fd129ed43) | Applied to the shared remember policy, including headless modes. |
| [Capture encoding `8fbdf83c1e`](https://github.com/trycua/cua/commit/8fbdf83c1e05bf0a8b84a6199fe697cc58f0e24f) | Backported; Reasonix still requests PNG with capture IDs. |
| [Post-action wait `41591b1b39`](https://github.com/trycua/cua/commit/41591b1b3969325142b7508e92417faa9575a18b) | Backported; macOS background native input skips synchronous reporting, foreground retains the default wait. |
| [Finder lookup `5bc914abfd`](https://github.com/trycua/cua/commit/5bc914abfd3b0a4015d10dc72f4cf04cc6675db4) | Backported to the SDK. No new app-launch action is exposed. |
| [Wayland punctuation `64cddca307`](https://github.com/trycua/cua/commit/64cddca307bef011b94c5a313a2bfe0becf7578d) | Backported; preserves shortcut modifiers without implicit Shift. |
| [libei monitor regions `1e0da1a964`](https://github.com/trycua/cua/commit/1e0da1a9649e52673104c357e1f70267b8b4a425) | Backported; portable region tests run on macOS, native Linux acceptance remains required. |
| [Contract cache `6a48bfc0ad`](https://github.com/trycua/cua/commit/6a48bfc0ade6d8226e7d0866eeafc3dde40a235b) | Backported; immutable schema generation runs once per process. |
| [Synthetic scroll readback `f92b01906f`](https://github.com/trycua/cua/commit/f92b01906f) | Backported inside the SDK. The host still exposes only trusted CDP pointer input. |
| [Stale MCP removal `f98edeec7b`](https://github.com/esengine/DeepSeek-Reasonix/commit/f98edeec7b99b3dc1d88e15105e8c32b4ddbce5e) | Adapted to local config/runtime APIs; validates source readability and retains plugin ownership restrictions. |
| [Fork context projection `02bf796dcc`](https://github.com/esengine/DeepSeek-Reasonix/commit/02bf796dcc) | Already covered by `CopyValidContextProjection` in all local branch paths. |
| [Summary reset recovery `8094bcb3a8`](https://github.com/esengine/DeepSeek-Reasonix/commit/8094bcb3a8) | Already covered by bounded `StreamAuxiliary` recovery. |
| [Browser tab reservations `d1852af99f`](https://github.com/esengine/DeepSeek-Reasonix/commit/d1852af99ff26187813529ba0e89d95e1f0780b5) | Not applicable: local computer use binds existing Chromium tabs; the upstream Go browser session/open API is absent. |
| [Orphan import indexes `b3bea0d076`](https://github.com/esengine/DeepSeek-Reasonix/commit/b3bea0d076999a59c475fdd6758465edf028757d) | Not applicable: the newer Studio session-import architecture is absent. |
| Built-in browser element picker | Not ported: the Electron browser surface is absent from this Tauri/external-Chromium route. |

The existing Spaces, Bench and model-artifact exclusions in `upstream.json`
remain in place. Linux desktop behavior requires its own native build and
session acceptance; a passing portable routing test is not that acceptance.

```sh
python3 -m unittest discover -s desktop/cua-runtime/scripts -p 'test_*.py'
cargo test --manifest-path desktop/cua-runtime/Cargo.toml --locked --bin reasonix-cua-runtime
python3 desktop/cua-runtime/scripts/test_sdk.py
```

`test_sdk.py` creates a temporary workspace containing only the selected native
platform and its path dependencies, retaining the SDK skills and Wayland helper
resources used by compile-time includes. It rejects registry dependencies not
present in the shipped lock and fails if a selected regression matches no tests.
The runner executes the complete contract, core and native platform library
suites; the wrapper separately tests patch-fingerprint mismatch before dispatch
and the host action boundary. The explicit Linux Preview
workflow also builds the Cua Debian variant and runs the native libei fake-server
and Wayland shortcut regressions. That workflow does not replace live compositor
acceptance on a GNOME/KDE desktop.

## Issue review (2026-11-03)

Version `0.1.69` retains the pinned `0.38.0` base. `upstream.json` distinguishes
upstream backports from locally authored fixes: each local fix has an issue URL,
an identity, a patch checksum and exact input/output file checksums. Both kinds
are reproduced from the clean export and shipped with the notices. The CDP
change is taken from the reviewed **draft, unmerged** PR head below; it is not
represented as a released upstream fix.

| Report | Local disposition |
| --- | --- |
| [Cua #6133: AXPress timeout](https://github.com/trycua/cua/issues/6133) | A background `-25204` is an uncertain dispatch. Neither the element selection fallback nor the pixel AX-hit-test fallback may send another mutation; the result requires fresh observation. Foreground behavior remains separately authorized. |
| [Cua #6299: large CDP frames](https://github.com/trycua/cua/issues/6299), [draft PR #6307](https://github.com/trycua/cua/pull/6307) | Backport draft head `bf4be9e7379addd9449639128c22b311663c04f7`: both frame and aggregate-message budgets are 64 MiB. Repeated 20 MiB semantic and DOM-reference snapshots retain the same binding. Larger replies still fail at the explicit cap. |
| [Cua #6296: UTM input](https://github.com/trycua/cua/issues/6296) | Recognize UTM as an input forwarder. Refuse background text and modifier chords before focus or input. Explicit foreground delivery requires an exact window and uses the existing physical HID route. Screen Sharing retains its refusal identity. |
| [Cua #6269: Gecko duplicate input](https://github.com/trycua/cua/issues/6269) | Known Firefox/Zen bundle IDs refuse native background text before either actuator. An uncertain AX insertion never triggers a second keyboard insertion. No live UTM/Zen compatibility claim is made. |
| [Cua #6270: complex-window query reachability](https://github.com/trycua/cua/issues/6270) | Match during the AX walk: unmatched rows do not consume the returned-row budget. Matches and ancestors share that budget, with at most 20,000 raw visits, 128 raw levels and the existing deadline. Anonymous wrappers do not consume semantic depth. Sparse snapshot slots preserve original element tokens; omitted slots refuse. A partial tree or zero matches does not prove absence. |
| [Reasonix #14838: huge-context compaction](https://github.com/esengine/DeepSeek-Reasonix/issues/14838) | Keep every strictly reducing checkpoint even while above the provider ceiling. At most four summaries run per recovery preflight; later preflights resume the saved digest. Sampling remains blocked until safe. Cancellation, task budget and later failure preserve progress; summary usage is charged once. Canonical history is unchanged. |
| [Reasonix #14878: HTTP history size](https://github.com/esengine/DeepSeek-Reasonix/issues/14878) | Optional bounded history pages precede projection/JSON encoding; old no-parameter array clients remain compatible. The built-in web view loads recent history first and prepends older pages with a session-generation fence. Controller snapshots and mirrored session loading still read the underlying history in full; this is not a streaming storage implementation. See `internal/serve/HISTORY_PAGING.md`. |
| [Cua #6250: recordings contain secrets](https://github.com/trycua/cua/issues/6250), [#6258: Electron debug signal](https://github.com/trycua/cua/issues/6258) | Recording and arbitrary SDK page/script/native move/drag routes remain unreachable through both the host schema and adapter plan. Regression tests verify rejection before starting a worker or dispatching to the SDK. No recording/redaction feature or Electron debug-signal activation is added. |

The macOS verification passed 1,825 default SDK library tests (67 contract,
1,107 core, 651 macOS; eight upstream tests remained ignored), 15 adapter tests
and 11 source-export tests. SDK tests exercise production transport, routing,
AX ownership and snapshot logic without typing into user applications.
Native Linux/Windows acceptance,
live UTM/Zen delivery, and a performance claim for every complex macOS app are
outside the evidence collected on this Mac. An individual AX call still uses
the SDK's native timeout; the query deadline is not a promise that every native
call finishes within that many milliseconds.

## Upstream review (2026-11-02)

Version `0.1.68` retains the same `0.38.0` base and now carries 14 audited
backports. The new batch adapts the following MIT changes instead of replacing
our app-scoped protocol with an upstream nightly or the separate Spaces SDK.

| Upstream change | Local disposition |
| --- | --- |
| [Runtime version probes `aaa4f586b1`](https://github.com/esengine/DeepSeek-Reasonix/commit/aaa4f586b1624e15de4836c613da4bc96d6c506e) / stable 1.39.11 | Applied to shared effect classification, legacy readers and nested substitutions. Only exact bare Node/Python version flags are readers; Python `-v` and extra arguments require the writer policy. |
| [Private serve passwords `2bd960b09d`](https://github.com/esengine/DeepSeek-Reasonix/commit/2bd960b09da298735275d927bfbd0b0f99f3f09a) | Adapted for both serve/web startup and hash generation, using existing owner-only single-line token-file checks. An invalid file never falls back to argv. |
| [Reasoning-only final recovery `860651f866`](https://github.com/esengine/DeepSeek-Reasonix/commit/860651f86601845c407a628780efbab6097cae80) | Already covered by `recoverNoDeliverable`, including DeepSeek `stop`, internal callers, tool rounds, cancellation and budget limits. Local recovery remains one additional round, not the upstream two. |
| [Background job guidance `d56abf84d1`](https://github.com/esengine/DeepSeek-Reasonix/commit/d56abf84d18d2b4ac57bca590741af17ab2f9dc9) | Adapted to actual local lifecycle: launch/output/wait explain that timeout is not failure and results must be collected. This controller drains completion notes on the next turn; it has no Studio completion observer that automatically starts a turn, so no automatic wakeup is promised. |
| [macOS frontmost app `337c62c9a4`](https://github.com/trycua/cua/commit/337c62c9a41e340b772b40615624c5f83fa49c43) | Backported; query WindowServer instead of stale AppKit activation state in the private child. |
| [Incremental element counts `e12ccfe29b`](https://github.com/trycua/cua/commit/e12ccfe29b5d9f354210a6a48880cbd4a8a42535) | Backported; both changed and unchanged reads satisfy output validation. The host still requests a complete observation. |
| [Snapshot token lifecycle `0cdcec94bc`](https://github.com/trycua/cua/commit/0cdcec94bc9988d41087d470b32f1fad2ba28691) | Backported; retired-token refusals identify known current snapshots, and reread bare row numbers fail closed. Full snapshot tokens remain the recommended input. |
| [Linux spare keycode `039fa6486d`](https://github.com/trycua/cua/commit/039fa6486d8959ecd5c9c901a707b3511d03024d) | Backported, including spare-batch and keyboard-plan regressions. |
| [Linux focus restore `4e3b6ba1f2`](https://github.com/trycua/cua/commit/4e3b6ba1f296fb9d6e9fb760395a7d521865d8d2) | Backported with a context-only adaptation for the pinned sleep calls; existing settle timings remain unchanged. Destroyed saved windows are not reactivated. |
| [Portal capture refusal `9416cc6bc4`](https://github.com/trycua/cua/commit/9416cc6bc42b2042c78ad0ae5e3b834c7dbdbeed) | Backported with public capture-path private-bus regressions. A reachable portal refusal is terminal. The Reasonix host still exposes no full-display capture action. |
| [Shared DevTools probe `61efedd411`](https://github.com/trycua/cua/commit/61efedd41149342c02f6ab9608fc3364cfe03585) | Backported; loopback listener and returned URL attestation remain mandatory. |

The local Unicode fixes also cover semantic-router descriptions, recovery
argument summaries and MCP startup failure summaries using the existing
grapheme-aware text utility. Credential redaction still precedes truncation.
CLI download sizes now carry to the next unit when one-decimal rounding would
display 1024.0 of the previous unit. Serve help and config guidance include the
password-file route in all shipped CLI languages.

The Linux SDK runner also selects keycode/focus and screenshot-portal tests,
including the public capture integration test on its own D-Bus daemon. Native
Linux compilation and live X11/Wayland behavior cannot be certified on this Mac;
the explicit Linux Preview workflow is prepared for those checks, not claimed
as executed here. Upstream's macOS frontmost-app E2E fixture is retained as
source, but it is not part of the minimal SDK unit workspace.

New Studio Office preview/model-health UI, Electron 45 packaging, queue/skill-card
presentation and Spaces/Volume/VM features do not map to this Tauri branch's
existing surfaces and are outside this reliability backport. They were reviewed
as separate features, not silently treated as implemented. This frontend already
has its own global reduced-motion rule; no Studio animation classes are copied.

## GNU/Linux build and scope

Build natively on x64 or ARM64 GNU/Linux with Rust, Go, Python 3.12+, Node 24+
and the regular Linux Preview WebKitGTK/GTK prerequisites. Cua additionally
needs X11/XTest/XInput development libraries, libxkbcommon, PipeWire development
headers and libclang for the PipeWire bindings. On Ubuntu these include
`libx11-dev libxtst-dev libxi-dev libxrandr-dev libxfixes-dev libxcomposite-dev
libxext-dev libxkbcommon-dev libpipewire-0.3-dev libclang-dev`.

```sh
python3 desktop/cua-runtime/scripts/build.py --bundle
# Optional: build only a deb package
python3 desktop/cua-runtime/scripts/build.py --bundle --bundles deb
```

Linux uses `tauri.cua.linux.conf.json` and its existing Preview data identity;
it does not use the macOS takeover identity. Both the bridge and private Cua
runtime are bundled. Normal Linux packages without that runtime do not register
a nonfunctional computer-control tool. `REASONIX_COMPUTER_USE_ENGINE=native`
disables Cua on Linux, where there is no legacy native implementation.

Run the app as the logged-in desktop user, with the session's display and D-Bus
environment. Settings report X11 and AT-SPI independently. Native Wayland is
experimental in the pinned upstream SDK, remains off by default, and requires
explicit `CUA_DRIVER_RS_ENABLE_WAYLAND=1` at application launch. Portal input
and PipeWire capture support are compiled in; this does not imply that every
compositor supports per-window capture or background input. Missing compositor
helpers, portal consent, unsupported surfaces or an unverified window cause
refusals. Do not fall back to full-desktop screenshots or uncontrolled input.
The pinned WinRects helper explicitly refuses GNOME 50+ due to reported
session-ending crashes. Keep that guard and helper version validation intact;
Ubuntu 26.04 GNOME 50 needs XWayland-compatible applications for this preview.
Settings detect GNOME 50+ and display the limitation. XWayland support and
native Wayland support require separate live acceptance. Native `move` is not
exposed by this adapter. On macOS, upstream window-scoped
`move_cursor` operates an agent overlay and does not move the physical pointer;
our private worker has no overlay host. Linux also lacks a window-scoped input
route for this action. Screenshot-coordinate clicks use their `capture_id`
directly, without moving a cursor first. Browser ref-scoped hover remains
available through `browser_pointer`.

Linux app approval binds a same-user process to its kernel start ticks and a
SHA-256 fingerprint of boot identity, PID namespace and executable path/device/
inode. The process directory is pinned while reading identity. Go checks before
and after consent; Rust checks independently before SDK dispatch. The existing
`launch_time` wire field carries exact kernel start ticks on Linux, not Unix
time. Both languages share a fingerprint and malformed/zombie-stat test corpus.

## Host contract

### Background delivery and foreground confirmation

App and browser-profile access do not authorize changing the user's foreground
app. Delivery defaults to background. A foreground request pauses for a fresh
human decision bound to the exact app incarnation, window and action. Neither
YOLO nor remembered/session grants can answer it. An allow applies once; a
denial suppresses further foreground prompts for that task/window. Cancellation
and process identity are checked again after the prompt. The private Rust
envelope independently requires `foreground_approved`; that field is not in the
model schema and cannot be supplied inside a model request.

On macOS, background native click, typing, key, hotkey and scroll calls use the
SDK's `post_action_wait: "none"`. The input result returns without waiting for
window-change reporting; the detached watch retains its full deadline and
focus-suppression lease. The result explicitly says changes were not yet
observed, and late changes may appear on the next action. Foreground delivery
retains the default wait. This private adapter choice is not model-controlled;
application success still requires observing the resulting state.

On macOS, establishing a new browser debugging connection can activate Chrome.
`browser_prepare` therefore requires explicit foreground delivery plus both
profile consent and single-action foreground consent. This also avoids relying
on upstream's activate-and-restore guard on another Space. Once connected,
ordinary browser actions remain in background. Browser reads and DOM text
insertion reject an unnecessary foreground request rather than asking to front
the app for an operation that does not use that delivery mode.

The parallel acceptance mode (`background` in the local QA host) holds a
separate editable fixture in front while the Cua worker controls disposable
Chrome in background. It observes app-activation events, cursor displacement
and real key events in that fixture. It never restores focus during the measured
phase. These checks cover the tested browser workflow; they do not establish a
universal no-activation guarantee for third-party native apps and their dialogs.

The Go bridge discovers only a bundled sibling `reasonix-cua-runtime` executable.
Its presence selects the Cua `computer_use` schema. Setting
`REASONIX_COMPUTER_USE_ENGINE=native` before launch selects the existing engine.
The Cua worker exchanges bounded JSON lines over private pipes: no daemon,
network listener, shell commands, or shared driver session is started.

Every Agent.Run owns a cancellable worker lifetime. Task end/stop kills and reaps
the worker, discarding its tokens. Transport interruption is an uncertain action;
the adapter refuses automatic restart/replay in that task. A new task starts a
fresh worker and must observe again. Version and pinned source revision accompany
the diagnostic evidence. The private envelope and each reply also carry the
SHA-256 of `upstream.json`, binding both helpers to the same reviewed patch set.
The worker refuses a mismatched host fingerprint before SDK dispatch; the host
rejects mismatched replies and never replays uncertain input. Older workers
reject the new private envelope field before dispatch as well.

The host resolves app PID/bundle ID/launch time before consent, checks again after
consent, and the Rust worker independently checks the approved app incarnation.
The host resolves the exact PID directly rather than reading
`NSWorkspace.runningApplications`: that catalog updates only through a Cocoa
main run loop, which the long-running Go bridge does not run. This keeps newly
launched/restarted applications available without reusing an old process grant.
Only app/window-scoped reads and allowlisted native or typed browser input actions are exposed.
The existing app consent, workspace restriction, stop, and screenshot PiP remain.
`effect=confirmed` means the driver's stated evidence was verified; it never means
an email or whole task succeeded. Read the resulting application state.

## Typed browser route (0.1.60)

For Chromium pages, prefer the typed DOM route; native AX and screenshot input
remain available for browsers that cannot attach and for other applications:

1. `apps`, then `windows` with a selected native PID; the host requests app access.
2. `browser_observe` with that PID/window ID binds the native window and lists its
   tabs. A setup/consent refusal is explicit and performs no setup automatically.
3. If needed, `browser_prepare` for that PID/window requests a **separate** fresh
   human decision through `browser_profile_access`. Approval describes reading
   logged-in pages, foregrounding, opening the fixed debugging settings page and
   enabling remote debugging (the setting may remain after task end). The browser
   may also present its own consent. A denial remains task-local and cannot be
   overridden by YOLO or remembered generic grants. There is no profile copy,
   browser restart, arbitrary profile path or model-chosen endpoint.
4. Bind again, then call `browser_observe` with returned `target_id`/`tab_id` to
   obtain semantic DOM refs. Optional `query`, scoped `ref` and `continuation`
   bound observation; `include_screenshot` defaults to false and captures only
   that tab's viewport through CDP when requested.
5. `browser_click` requires a current DOM ref and uses trusted CDP input; no
   synthetic-JS fallback is enabled. `browser_type` inserts text at the caret;
   `replace=true` replaces the field, and empty text with replace clears it.
   Observe the postcondition to establish application success. Never repeat an
   uncertain submit without checking whether the previous one completed.
6. `browser_navigate` navigates the exact bound tab to an absolute HTTP(S) URL
   without embedded credentials, or `about:blank`. All previous DOM refs become
   invalid; observe the destination before further input. Internal browser pages,
   local file URLs and JavaScript/data URLs are not navigation destinations.
7. `browser_select` requires a native HTML select ref and exactly one option
   `label` or `value`. Empty values are preserved. Input/change events are
   synthetic; verify that the page accepted them. An already-selected option is
   reported as `no_change=true`, not dispatched input.
8. `browser_pointer` takes a current DOM `ref` and `gesture` of `hover`,
   `right_click`, `double_click`, `scroll` or `drag`. Scroll takes nonzero
   `delta_x`/`delta_y` in CSS pixels (absolute value at most 10000); drag takes a
   `destination_ref` in the same frame. The origin must declare the appropriate
   pointer/scroll capability. Only trusted CDP input is exposed, with background
   delivery by default and explicit foreground delivery after inspecting a
   refusal. Native screenshot coordinates are not accepted here.

Example after receiving the real target/tab/ref values:

```json
{"action":"browser_type","pid":42,"window_id":11,"target_id":"RETURNED_TARGET","tab_id":"RETURNED_TAB","ref":"RETURNED_EDITABLE_REF","text":"本地测试","replace":true}
```

The Rust wrapper uses a private SDK standard-mode trusted session. Only a
host-approved prepare call can satisfy the upstream residual consent callback,
which checks exact PID, window, endpoint owner and the live native process
incarnation. The attested consent resource/digest is never forwarded to a model
or diagnostic log. Bindings additionally tie every returned target/tab ID to
that same approved native process incarnation and window. Rebinding replaces old
IDs; a new host task cannot reuse them. SDK session/attachment limits (30 minutes
idle, 8 hours total) also apply. This adapter does not launch isolated browsers
in production, so task cancellation cannot orphan a driver-launched browser.

Native screenshot pixels and `element_token` are distinct from DOM refs and
cannot be passed to browser actions. Older DOM trees and tab images use the
existing bounded model-history projection. Partial browser input and failed
focus cleanup remain uncertain outcomes, even when upstream labels its result
as refused; there is no automatic replay.

No arbitrary JavaScript, legacy `page`, `run_steps`, clipboard, downloads,
file-upload or output-file API is exposed. Background AX/CDP input may leave the
physical mouse stationary. The original whale feedback belongs to the native
engine; a Cua background action is not drawn as a fake physical cursor.
Screenshots continue to appear in the existing PiP.

The regular `/Applications/Reasonix Tauri.app` is not necessarily the Cua edition.
The bundle must contain both `reasonix-desktop-bridge` and
`reasonix-cua-runtime` in `Contents/MacOS`; source support or successful tests
alone do not switch a running native-engine installation to Cua.

## Source and notices

The root Cua MIT notice, upstream Rust notices and Inter SIL OFL notice are
retained in `licenses/`. The build additionally generates a complete target
dependency inventory and exact crate notices. Unmodified MPL-2.0 UniFFI source
archives are distributed alongside the executable, with Cargo.lock checksums;
crates without root notice files also include their complete source archive.
These files ship under `Contents/Resources/cua-notices/` in the app/DMG. The build
refuses incomplete inventories and dependencies requiring a separate GPL/FSL/BUSL
review. Do not describe all transitive dependencies as MIT.

## Acceptance

`python3 desktop/qa/computeruse/build_cua_acceptance.py` prepares an AppKit and
WKWebView fixture. Grant only this temporary fixture Accessibility and Screen
Recording through macOS Settings after explicit user approval; never transfer a
grant from Reasonix or modify TCC databases. Rebuilding an ad-hoc app changes its
identity and may require removing/re-adding its temporary entries. Remove both
entries and quit the fixture when finished.

On 2026-10-24 the actual Cua SDK filled and read back native Chinese/ASCII text,
activated a native button, captured the fixture window, and clicked by screenshot
coordinates on both displays, including a negative-X secondary display. A local
WKWebView independently read back the Chinese input and its changed button text.
These checks used only the local fixture; Gmail/Douyin/WPS were not exercised.
Go race tests additionally cover worker reuse/isolation across concurrent tasks, cancellation/reaping,
invalid replies, no replay after uncertainty, app consent and sandbox refusals.

For real DOM acceptance, the same final QA bundle also includes a `browser`
command. It launches a fresh Chrome profile and a local HTML fixture, exercises
host-profile refusal, exact-tab binding, Chinese replacement/readback, clearing,
click/readback, CDP viewport screenshot and cross-window refusal, then terminates
only its own disposable browser. It never uses existing browser accounts.
Grant the QA host Accessibility only after explicit consent; if the OS requires
another grant, stop and obtain it separately. The test does not edit TCC or grant
permissions. Results are under the ignored QA target's `browser-results` folder.

On 2026-10-25 the expanded local Chrome acceptance passed 18 checks, including
label/empty-value selection, a selection no-op, all five trusted pointer gestures,
navigation readback, and rejection of old refs after navigation. Every new route
also refused another native window before dispatch. Temporary QA Accessibility
permission was enabled after user approval and switched off after acceptance;
Screen Recording was not granted. See the
[candidate evidence](../../docs/tauri/evidence/2026-10-25-cua-browser-actions/README.md).
Go and Rust share a wire-contract corpus so that request validation covers both
sides, including empty selection values and bounded gesture arguments.
