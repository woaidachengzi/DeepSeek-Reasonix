# Computer-use request and input optimization acceptance

Date: 2026-10-24. macOS 27.0.1, Apple Silicon.

## Local request replay

The opt-in `TestComputerUseContextLocalReplay` read a saved 455-message session
locally. It did not call a model or publish conversation contents.
Estimated request tokens changed from 609,949 to 340,643 (44.15% less).
Image references changed from 77 to 11: two latest desktop observations plus
nine user/other images. The exact latest observations, user instructions and
attachments remain unchanged; durable history is not edited. See
`context-replay.json`. This is a local estimator, not actual provider billing,
measured response speed or a predicted reduction in email completion time.

The original slow filling phase had 26 provider requests totaling about 29m02s
within 31m03s wall time. Seventeen of 25 desktop tool calls refused input; local
desktop execution excluding the initial app-approval wait totaled about 5.35s.
The first-output wait does not isolate model reasoning from queue, network or
prefill time. These aggregate figures explain reducing repeated requests and
historical screenshots rather than changing low-level mouse delays.

## Native fill gateway

The operator explicitly authorized temporary Accessibility access for the
disposable `Reasonix Pointer QA` app. Its independent child process called the
production `RXFill` against only the parent fixture's own windows. A separate
process is necessary: an initial same-process worker-thread AX setter triggered
an AppKit/TextKit lock deadlock. That discarded trial is not passing evidence.

On each of two displays (field center X=960 and X=-960):

- The native text field independently reported the requested Chinese/emoji text.
- The first fill reported `accessibility_value`, dispatched and confirmed.
- Repeating the same complete value reported confirmed, unchanged and no dispatch.
- A secure field and non-text button refused input; the secure fixture value stayed unchanged.
- The inline nonpersistent WKWebView textarea was **not supported** by the strict
  editable-target verification and refused with no input. Web fill did not pass.

See `native-fill.json` for per-case results and production-source hashes. No
Screen Recording grant was added to this fixture, so its post-write screenshot
reported access unavailable. The independent field readback proves only the
native write/no-op contract; it does not prove screenshot delivery, Chromium
behavior, a recipient chip, full receipt/approval routing or actual mail sending.
The full controller still requires normal observation/capture readiness.

After testing, the temporary Accessibility entry was removed. A fresh read-only
preflight reported Accessibility=false, PostEvent=false and writes=0. The
existing Reasonix permissions were not changed. No real email or message was sent.

## Automated checks

Controller tests cover one-use receipts, real frame changes, bounded caret-only
differences, fill readback and verified no-ops. Real agent-loop tests enforce
three zero-input refusals despite interleaved fresh observations, preserve each
refusal's image/result, and pause with `computer_use_blocked`. Proxy tests retain
the concrete dispatch outcome and classify zero-input refusals as not started.
Request projection tests protect user attachments, tool pairing and original
history. These deterministic checks do not measure a live model's speed.
