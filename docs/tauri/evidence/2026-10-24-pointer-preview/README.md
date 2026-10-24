# Actual cursor preview before clicking

Version: 0.1.56. Date: 2026-10-24.

The optional two-step path separates pointer positioning from clicking:

1. `move` accepts the existing image/window coordinate contract and a fresh
   app-scoped observation. It posts only a WindowServer mouse-moved event.
2. The native gateway reads the actual global cursor position, bounds arrival
   checks to 100 ms, captures the approved window, and rechecks cursor/window
   ownership. It does not report a preview based only on requested coordinates.
3. The model and floating preview receive a red crosshair at that actual
   position. `pointer_preview` reports global screen points and image pixels.
   The original cursor-free JPEG remains the canonical receipt snapshot; the
   marked image is a separate presentation copy with identical dimensions.
4. After visually checking the marker, the model calls `confirm_click` with
   only PID and the move receipt. Host-only pointer data pins the target.
   Current state, receipt age, window identity, foreground status, coverage,
   physical button state and actual cursor position are checked again. The
   final cursor check runs after the ownership preflight. No new move event is
   sent: confirmation posts down/up only, or refuses without a click.

Moving the pointer more than one global point, replacing the receipt,
moving the window, changing the page, expiry or cancellation invalidates
confirmation. An uncertain dispatched click cannot replay the old receipt.
The feature retains the existing interactive app-access consent boundary.

Tool guidance uses previews for first screenshot positioning, changed display,
zoom or window geometry, and missed/refused clicks. Subsequent reliable targets
can use ordinary clicks to avoid another model round for every action. Two
unsuccessful corrected previews should end exploration with diagnostics.
Movement alone does not reset the existing refused-input retry budget.
Compact diagnostic exports retain actual pointer and image mapping values.

## Validation scope

- Controller fixtures cover negative screen origins, downscaled screenshots,
  actual versus requested position, raw versus marked images, host-stamped
  confirmation targets, changed pages/windows, missing previews, expiry,
  cancellation, invalid position/image and single-use confirmation receipts.
- The native test runner compiles the production gateway. Its injected post
  callback checks event types and routing without posting to the desktop:
  move emits one mouse-moved event; confirmation emits down/up without moving;
  moved/unreadable cursors and failed ownership/access checks emit zero clicks.
- Frontend integration covers preview action labels, consent, collapse/stop,
  task ownership and session isolation. Diagnostic and agent tests cover
  pointer facts and the refusal budget.

No new macOS permission was granted. No real Chrome/Gmail mailbox was operated,
and no message was sent. These checks establish the controller and event
contract, not model target-recognition accuracy or live browser acceptance.
The macOS ARM64 package uses local ad-hoc signing, without Apple notarization.
