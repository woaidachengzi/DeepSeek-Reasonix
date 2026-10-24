# macOS desktop control

Reasonix exposes a `computer_use` image-capable model tool on macOS 14+ builds
with Cgo enabled. It uses Apple APIs directly; no Node runtime, downloaded
plugin, or third-party helper is needed. Other platforms and CGO_ENABLED=0 builds
do not register this tool.

The implementation lives in `internal/computeruse`; `internal/tool/builtin`
adapts it to the normal approval, execution, and structured image-result path.
Both workspace and ordinary boot paths disable the capability when the workspace
shell sandbox is enforced. It is a write-capable tool, including for observation,
and is therefore excluded from read-only agents. It does not change permission
policy, grant system access, or prompt for macOS permissions automatically.

## Build and use

Open Settings → Integrations & connections → Computer control (the first item).
This page reads native component availability, Accessibility and Screen Recording
status from the bridge process without prompting, capturing, or sending input.
It links to the fixed macOS permission panes and the existing sandbox, action
approval and model settings. Opening a system pane does not grant permission;
refresh the status after authorizing the app. Opening this page does not change
the workspace sandbox or tool approval policy.

Build the Tauri app normally on macOS with `CGO_ENABLED=1`. ScreenCaptureKit is
loaded dynamically so older systems can still start the application without
advertising this capability. Run the app from a stable installation path. In
System Settings → Privacy & Security, grant the running Reasonix application
Accessibility and Screen Recording, then restart it if macOS requests that.
Development/test executables can have a different TCC permission owner from the
packaged app; a test binary's permission result does not prove the app is ready.

The macOS default workspace shell sandbox is enforced, so this capability is
hidden by default. To intentionally allow desktop control, set `[sandbox]`
`bash = "off"` in the intended profile or project. This also runs shell commands
without the OS sandbox; the ordinary per-tool permission policy still applies.
Use a separate profile for acceptance testing rather than changing a user's
working profile.

Use a model with image input, or configure the existing image-understanding
path. Screenshots are returned through the existing structured image channel,
not embedded in the text transcript.

Typical model workflow:

The normal lean tool surface routes this through
`use_capability(action="call", capability_id="tool:computer_use", arguments={...})`.
Inspect that capability to obtain its action schema.

1. `status` reports permission readiness without triggering a system prompt.
2. `apps` lists running regular applications and their PIDs.
3. `observe` with a selected PID returns one visible window, accessibility
   elements, window bounds in points, screenshot, and an observation receipt.
4. Use that receipt for one `activate`, `click`, `type`, `key`, or `scroll`.
   Activation is explicit; input never silently switches the target application.
5. Read the result and observe again whenever the state or outcome is unclear.

An inactive window hidden by Stage Manager can return metadata without an
image (`capture_available=false`). Activate it, then observe the full window.

An accessibility click uses the returned element path. A coordinate click uses
window-local points; screenshots may be downscaled to at most 1920 pixels on
the longest side. Convert from image pixels using the returned window bounds.
Input requires the selected window to be active. Named keys include return,
tab, escape, backspace, space, arrows, and common command-letter shortcuts.
Typing is Unicode-aware and does not overwrite the user's clipboard.
Coordinate clicks use normal foreground WindowServer routing. Before posting,
the native host checks PostEvent access, the physical left button, the frontmost
mouse-down window at the exact point, its CG owner/ID/bounds, and the active
AX focused window's owner/bounds. AppKit's public `windowNumberAtPoint` query
distinguishes input-covering windows from click-through visual surfaces; a
window-list rectangle alone cannot do so. A different or unverifiable mouse
target refuses all mouse events. Remote web AX leaves need not expose an
`AXWindow` attribute. Headless Go hosts lazily use `NSApplicationLoad`, which
preserves a prohibited activation policy and creates no windows. The mouse
pointer moves to the selected point; the tool does not restore it to another
application afterwards. AX element presses remain app-targeted.

Scrolling uses the selected window's vertical accessibility scroller, at an
optional observed element path or the scroll area containing its center.
Negative delta moves down, positive moves up; some native controls use page
granularity. Windows without an accessible scroller report unsupported instead
of sending a wheel event without a window target. Scrolling does not move the pointer.

## Observation and dispatch contract

A receipt is controller-owned, PID/window-bound, short-lived, and consumed before
input. Fresh accessibility state and screenshot pixels are compared before
execution. Changed state returns a new observation without sending input.
JPEG metadata is excluded from comparison. A blinking insertion caret's small
AX-reported render region (including neighboring compression blocks) is excluded
too; focus, selection, text value and geometry still invalidate observations.
No whole text field or general animation region is ignored.
The model receives `state_changed=true`, `dispatched=false`, and the new image
as a normal observation result, so it can inspect the change before deciding
on its next action. This is not an automatic input retry.
Activation compares app/window identity and launch time rather than pixels;
inactive-window animations must not prevent bringing an app forward.
Desktop operations are serialized across controllers in the current process.
Other processes and physical users can still change the UI between observation
and dispatch; there is no claim of a system-wide atomic UI transaction.

`dispatched` means an input was sent, not that its intended effect completed.
`dispatch_method=window_server_mouse` identifies a coordinate click sent through
WindowServer. `pointer_target_changed`, `pointer_target_unverified`,
`pointer_busy`, and `input_permission_missing` mean no mouse input was posted;
resolve that condition and obtain a fresh observation before another attempt.
`dispatch_unknown` means the operation may have happened. Such outcomes require
fresh observation, never automatic replay. All computer-use errors explicitly
opt out of the engine's generic transient-tool retries.

Accessibility traversal is bounded and indicates truncation. Secure text values
are omitted from the accessibility data; screenshots still show the selected
window as the user sees it. Capture is app-window scoped, not a full-desktop
recording. This initial implementation does not provide drag-and-drop,
Record & Replay or Windows/Linux desktop input.

## Verification

Run:

```
go test -race ./internal/computeruse ./internal/tool/builtin -run TestComputerUse
go test ./internal/boot ./internal/agent ./cmd/reasonix-desktop-bridge
CGO_ENABLED=0 go test ./internal/computeruse
```

The native status/catalog test runs the real Cocoa bridge without input,
permission prompts, screen capture, or launching another app. Controller tests
verify stale pixels, receipt ownership/expiry, uncertain dispatch, no replay,
window binding, cancellation, and serialization using a deterministic backend.
These tests do not establish real-input success. Interactive acceptance needs
an authorized app instance with TCC grants and an isolated test window.

For packaged acceptance, close other Preview instances and run on macOS:

```
python3 desktop/qa/computeruse/run.py --app /absolute/path/Reasonix\ Tauri\ Preview.app
```

This builds a disposable Cocoa fixture, starts a deterministic loopback model,
and runs the normal packaged model/tool chain in a private core profile. It
checks AX button press plus an independent coordinate-button counter increment,
coordinate focus from a non-text initial responder, Chinese/emoji typing, Command-A,
replacement typing, and scrolling against the fixture's independently recorded
state. It keeps logs in a newly created temporary directory and closes its
fixture and Preview instance afterwards. It does not grant TCC permissions.
Use LaunchServices (`open --env ...`) when launching packaged tests: a direct
shell launch can attribute permissions to the terminal/host instead of the app.
Ad-hoc rebuilds may invalidate previous macOS grants; reselect the final bundle
in System Settings if its native status reports access unavailable.
If TCC logs report a code-requirement/hash mismatch despite enabled switches,
restarting System Settings and removing/re-adding that app's old entries may
be needed. This changes system permissions and requires the operator's explicit
authorization; the acceptance runner never does it automatically.

Packaged acceptance passed on macOS 27.0.1 on 2026-10-23 through the normal
`use_capability` model/tool route: screenshot delivery to the vision model,
background activation, AX button press (counter 1), coordinate field focus,
Chinese/emoji typing, Command-A replacement (`Desktop acceptance passed ✅`),
and `scroll` moving the native view by 91 points. The fixture recorded its own
postconditions. This verifies that fixture and host, not every macOS application.


The older fixture started with a text field eligible for initial focus and only
counted its AX button press. That could not independently prove coordinate-click
delivery. The current fixture requires two counter increments (one AX press and
one coordinate click) and starts with a non-text responder. The updated native
gateway tests inspect real Core Graphics event objects with a non-posting test
callback; they verify refusal sends zero events, mouse-target ownership,
input-covering versus click-through overlays, negative origins and move/down/up
envelopes. A read-only worker-thread check verifies headless AppKit startup and
real mouse-target lookup without creating a window or posting events. They do not establish
real WindowServer delivery or Chromium acceptance without the packaged app's
TCC grants and a completed interactive fixture run.

For a smaller native-path acceptance, run
`python3 desktop/qa/computeruse/build_pointer_acceptance.py`. The builder only
compiles and signs a disposable app; its `--preflight` launch reads permissions
and posts no events. After the operator explicitly authorizes that exact app's
Accessibility access, launch it through LaunchServices. It exercises the
production native preflight/gateway against its own native button and inline,
nonpersistent WKWebView page on at most two displays. Independent counters and
bounds are written to `result.json`. It never requests permissions itself,
loads a user's browser profile, or sends messages. Remove the temporary
Accessibility entry afterwards. This tests the native gateway, not the full
agent/approval chain or Chromium specifically.

On 2026-10-24 the disposable fixture independently counted one native and one
WKWebView click on each of two displays, including the display at negative X.
The temporary permission was then removed and read-only preflight confirmed
both Accessibility and PostEvent unavailable. See the scoped
[acceptance record](../tauri/evidence/2026-10-24-pointer-dual-display/README.md).
