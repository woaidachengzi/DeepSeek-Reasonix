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
Scrolling uses the selected window's vertical accessibility scroller, at an
optional observed element path or the scroll area containing its center.
Negative delta moves down, positive moves up; some native controls use page
granularity. Windows without an accessible scroller report unsupported instead
of sending a wheel event without a window target. The desktop pointer is not moved.

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
`dispatch_unknown` means the operation may have happened. Such outcomes require
fresh observation, never automatic replay. All computer-use errors explicitly
opt out of the engine's generic transient-tool retries.

Accessibility traversal is bounded and indicates truncation. Secure text values
are omitted from the accessibility data; screenshots still show the selected
window as the user sees it. Capture is app-window scoped, not a full-desktop
recording. This initial implementation does not provide drag-and-drop,
Record & Replay, Windows/Linux desktop input, or a dedicated settings page.

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
checks AX button press, coordinate focus, Chinese/emoji typing, Command-A,
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
