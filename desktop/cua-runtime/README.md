# Reasonix Cua edition

The first macOS integration links the MIT Cua Driver Rust SDK from
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
`/usr/lib/swift` to its runtime search paths. Upstream behavior source is unchanged.

## Host contract

The Go bridge discovers only a bundled sibling `reasonix-cua-runtime` executable.
Its presence selects the Cua `computer_use` schema. Setting
`REASONIX_COMPUTER_USE_ENGINE=native` before launch selects the existing engine.
The Cua worker exchanges bounded JSON lines over private pipes: no daemon,
network listener, shell commands, or shared driver session is started.

Every Agent.Run owns a cancellable worker lifetime. Task end/stop kills and reaps
the worker, discarding its tokens. Transport interruption is an uncertain action;
the adapter refuses automatic restart/replay in that task. A new task starts a
fresh worker and must observe again. Version and pinned source revision accompany
the diagnostic evidence; responses with a mismatched revision are rejected.

The host resolves app PID/bundle ID/launch time before consent, checks again after
consent, and the Rust worker independently checks the approved app incarnation.
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
