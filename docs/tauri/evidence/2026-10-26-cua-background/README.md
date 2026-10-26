# macOS background-control preview 0.1.63 — 2026-10-26

Built and installed `/Applications/Reasonix Tauri.app` 0.1.63. The previous
0.1.61 app has both a verified backup and an atomic-install rollback path in
`installation.json`. The installer replaced only the app bundle. Existing
configuration, credentials and conversations were not migrated or edited by
the installer. The ordinary app reopened with its existing catalog.

## Behavior

- Cua input defaults to background. A foreground request needs a separate
  fresh human decision for that app incarnation, window and action. Ordinary
  app/profile access, YOLO and remembered grants cannot answer it.
- An allow applies to one call. Choosing **保持后台** sends no input for that
  step and suppresses further foreground prompts for that task/window.
- Go rechecks cancellation and process identity after confirmation. The private
  Rust envelope independently checks `foreground_approved`; model requests
  cannot supply that field. A later request cannot reuse an earlier allow.
- macOS browser connection setup requires explicit foreground delivery and
  both profile approval and single-action foreground approval. Chrome may
  activate for its debugging prompt; an activate-and-restore guard is not
  accepted as an uninterrupted background setup. Once connected, page work
  uses background delivery. Browser reads/text insertion reject an unnecessary
  foreground parameter rather than silently ignoring it.
- The installed Computer control page shows **默认后台操作；每次切到前台都需单独确认。**

This is a Cua-edition policy, not a universal guarantee about third-party apps.
Native apps can open dialogs or activate themselves in response to an action.
The manually selected legacy native engine is outside this preview's policy.
No global focus-restoration loop, new input-monitoring permission or hidden
desktop-input fallback was introduced.

## Actual parallel acceptance

The signed temporary QA host used the exact packaged runtime, with only code
signatures differing. Its separate native text field stayed in front while a
fresh, account-free Chrome instance ran the local page workflow in background.
All measured browser operations explicitly used background delivery; the
initial browser-connection setup completed before measurement began.

Two consecutive trials passed **21 checks each**:

| Measurement | Trial 1 | Trial 2 |
| --- | --- | --- |
| App activation/focus changes observed | 0 | 0 |
| Real pointer displacement observed | 0 | 0 |
| Key-down events received in foreground fixture | 20 | 20 |
| Independent foreground text | `reasonixparallel2026` | `reasonixparallel2026` |

Browser checks cover profile-consent refusal, exact tab, Chinese replacement,
no duplicate typing, clearing, option selection/empty/no-op, trusted hover,
right-click, double-click, scrolling, dragging, click/readback, tab image,
cross-window refusal, navigation and stale refs. While foreground input arrived,
the worker continued real browser observations/hover operations and checked
that the foreground text did not appear in the web field.

`parallel-1-acceptance.json`, `parallel-2-acceptance.json` and their JSONL evidence
retain these results. Images are represented by metadata and private token
fields are redacted. The foreground fixture observes NSWorkspace activation
events plus 20 ms polling, cursor displacement and local key events. It does
not front either app or restore focus during measurement. These are observed
results at that instrumentation resolution, not a proof of every possible OS
timing or a physical-human typing trial.

An initial trial failed: it observed no focus change but one cursor displacement
before any controlled key arrived. The source of that movement was not
attributed. It remains recorded as **failed/inconclusive**, not counted as a
pass. No driver code changed between that trial and the two passing trials;
subsequent orchestration waited for the sentry's ready state before sending
the fixed key sequence. See `initial-failed-summary.json` and its JSONL trace.

The user approved temporary QA Accessibility access and personally completed
system authentication. After testing, the QA toggle was visibly switched off
and its host/idle worker stopped. Screen Recording was not granted to QA.
The disposable browser profiles and workers were closed by the fixture.
`qa-cleanup.json` records cleanup.

## Regression and package checks

- Targeted race suites passed across computeruse, builtin, control, agent and
  desktop bridge with `-run 'Cua|ComputerUse|Foreground' -count=1`.
- Nine Rust wrapper tests passed, including independent foreground approval
  enforcement and the actual pinned SDK inventory. Go/Rust share 51 operation
  cases, including rejection of foreground parameters on background-only DOM
  routes.
- Both Python export tests passed. The updated Linux fixture runner cross-
  compiled for ARM64; no Linux VM acceptance or upgrade was performed here.
- Settings and foreground-prompt DOM tests passed; the real frontend build
  passed lint, layers, scroll/style contracts, TypeScript and all bundle budgets.
- App deep/strict signature verification passed. A read-only DMG mount matched
  all three app executables; installed hashes match that same app exactly.
  QA and bundled runtime are byte-identical after removing their signatures.
  No user config/database/JSONL files were present in the app payload.

See `validation-summary.json`, `go-tests.log`, `rust-tests.log`,
`python-tests.log`, `final-build.log` and `package-audit.json`. The DMG is:

`desktop/tauri/target/release/bundle/dmg/Reasonix Tauri_0.1.63_aarch64.dmg`

SHA-256: `5e584b940645229d3a4f327ddd32f6e5c256f4d4955db356243f493800346998`.
This is a locally signed, unnotarized ARM64 preview.

## Installed readiness and limits

Resigning invalidated the former app's TCC grants. With the user's explicit
approval, the old entries were removed and the current installed app was
re-added through System Settings. After the OS-requested restart, the app's
own Computer control page reported both Accessibility and Screen Recording
granted; About visibly showed v0.1.63. No TCC database edits, copied grants or
security-policy changes were used. See `installed-readiness.json`.

Source base is `69926dad7b55dd951770e552d93252d3580cb476` plus the dirty changes
hashed in `source-identity.json`. Source hashes were checked again after the
final build and before delivery. The runtime README and this evidence were
added afterward as documentation. The pinned Cua revision stays
`e754cfec69c41400eb2aabde716c27d15607b2f0`, driver 0.38.0.

The new foreground prompt's controller and UI were exercised through regression
tests; a new ordinary model-driven chat was not run for this preview. Chrome's
tested local workflow does not establish compatibility for every production
site, other Space, minimized window, native app or modal dialog.

## Pre-commit review

Review found that the ordinary app-access prompt is shared with the legacy
engine, so it must not promise background delivery unconditionally. Its neutral
app-scope wording was restored, and the settings hint now explicitly names Cua
in English, Simplified Chinese and Traditional Chinese. Foreground authorization
behavior did not change during review. These four copy-only changes postdate
the accepted 0.1.63 package; that installed app was not rebuilt or replaced
during review. `review-validation.json` records packaged/reviewed source hashes.

Fresh review checks passed: complete Go race suites for computeruse, builtin
and desktop bridge; approval-related race suites for control and agent; nine
Rust tests; both Python export tests; settings and foreground-prompt DOM tests;
and the full frontend build, contracts and bundle budgets. Go/Rust formatting
and Git whitespace checks passed. No blocking review findings remain. See
`review-go-core.log`, `review-go-approval.log`, `review-rust.log` and
`review-frontend-build.log`.
