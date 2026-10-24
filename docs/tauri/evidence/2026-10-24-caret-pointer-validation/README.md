# Distant pointer clicks blocked by a browser insertion caret

Version: 0.1.57. Date: 2026-10-24.

The saved local browser session reached a filled reply draft, then paused after
three pre-input refusals. Inspection of the actual states distinguishes them:

- The first transition changed the AX tree and pixels outside the text caret.
  Refusing it remains correct; it is not classified as caret-only.
- The next two transitions had identical AX state, including focus, value/hash,
  selection, window identity/geometry and controls. Their strong pixel changes
  were a single 1x15 insertion bar inside the focused text area. JPEG changes
  stayed within the existing tightly bounded caret compression region.

The existing browser fallback handled keyboard input and verified field fill
only. Coordinate clicks were always refused when Chromium omitted native
insertion-caret bounds. Repeatedly retrying a submit click was therefore both
slow and unreliable.

## Change

Reuse the same caret classifier for optional coordinate clicks, pointer moves
and preview-confirmed clicks, only when the resolved target lies outside both
the entire focused field and the complete caret/JPEG region (plus a two-image-
pixel boundary). Resolve image pixels using host geometry and confirmation
targets using the host-owned move receipt before checking those exclusions.

No AX-state difference is ignored. A changed field value, selection, controls,
foreground, app/window identity, geometry, extra changed pixels, broad overlay,
unknown image geometry, missing preview or target inside/near the caret/field
still refuses input. AX presses and scrolls do not gain this exception. Native
pointer ownership/coverage checks, actual-position confirmation, expiration,
single-use receipts and the three-refusal pause continue to apply. The change
does not replay text, synthesize a submit shortcut, or automatically send mail.

Successful fallback results include `observation_validation=caret_only`; compact
diagnostics retain it. Tool guidance warns that `controls_changed` alone is not
evidence of caret blink.

## Validation

- A temporary test replayed the three saved frame pairs against the production
  classifier: first refused, following two accepted. The replay read private
  local snapshots only and never posted input. The temporary test was removed;
  no private screenshot, email content, account or session ID is committed.
- Durable controller tests cover coordinate click, move and confirm-click,
  main/negative display origins, downscaled images, correct target binding and
  receipt replay refusal. Other tests keep field/halo clicks, AX presses,
  scrolls, changed text/selection/controls/focus, target pixels, missing geometry,
  out-of-window coordinates and missing preview blocked without dispatch.
- Existing caret, pointer, native event-envelope, diagnostic and related
  regression tests remain in scope. Native envelope callbacks inspect events
  without posting to the desktop.

These checks establish saved-frame classification and fake-backend controller
behavior. They do not constitute a live Gmail send or model task acceptance.
No real mailbox was operated or message sent during this work. No system
permission was changed. The macOS ARM64 package uses local ad-hoc signing,
without Apple notarization.
