# Cua browser operation integration — 2026-10-25

This candidate adds `browser_navigate`, `browser_select` and `browser_pointer`
to Reasonix's Go model schema, validation and result handling and the Rust SDK
adapter. Pointer gestures are hover, right-click, double-click, scroll and drag.
They use observed DOM refs and trusted CDP input. All routes retain task-local
native app/window identity, browser profile approval and cancellation ownership.
Selection explicitly reports synthetic events; neither dispatch nor a successful
SDK acknowledgement establishes that a user's whole task succeeded.

## Source

The user supplied `/Users/jerry/Downloads/cua-main`. Comparing its Rust driver
scope against the project's existing pinned source covered 773 files after
normalizing the documented Cargo workspace metadata adjustment. Five files
differed: Cargo.lock, SDK lib.rs and three daemon/CLI files. The browser tools and
pointer implementations are identical. This candidate therefore keeps revision
`e754cfec69c41400eb2aabde716c27d15607b2f0`, driver version `0.38.0`.
See [source-comparison.json](source-comparison.json).

## Acceptance

- `go test -race ./internal/computeruse ./internal/tool/builtin ./internal/control
  ./internal/agent ./cmd/reasonix-desktop-bridge -run 'Cua|ComputerUse' -count=1`
  passed all five packages; [go-tests.log](go-tests.log).
- `cargo test --manifest-path desktop/cua-runtime/Cargo.toml --offline --locked`
  passed seven tests, including argument compatibility with the actual pinned
  SDK inventory; [rust-tests.log](rust-tests.log).
- Go and Rust consumed the same 49-case model-request corpus, covering valid
  operations, empty option values, gesture bounds, scheme restrictions, mixed
  native/browser arguments and unbound targets. The Go tests also preserve
  uncertain partial delivery and distinguish dispatched selection from a no-op.
- The signed temporary QA host ran the actual release runtime against a fresh
  disposable Chrome profile. All 18 local browser checks passed:
  profile refusal, exact tab, Chinese replacement/readback, no duplicate input,
  clearing, label selection, empty selection, unchanged selection, trusted hover,
  right-click, double-click, scrolling, dragging, click/readback, viewport image,
  cross-window refusal, navigation/readback, and navigation invalidating refs.
  [browser-evidence.jsonl](browser-evidence.jsonl) retains the driver results;
  private session arguments and image payloads are omitted.

The user approved temporary Accessibility access for `Reasonix Cua QA` and
completed the macOS authentication dialog. The permission was visibly switched
off after testing, and the QA application was terminated. Screen Recording was
not granted. The fixture closed only its disposable browser and removed its
temporary profile. Existing Chrome/Edge accounts were not used.

## Packaging and limits

The installed `/Applications/Reasonix Tauri.app` was inspected before this change:
it contained the bridge and Tauri host, but no `reasonix-cua-runtime`. That was
the installation state during this candidate-only acceptance. The later
[installed-app acceptance](../2026-10-25-cua-installed/README.md) records the
upgrade to Cua 0.1.61 and verification through the ordinary chat tool chain.

The Cua candidate was built with `tauri.cua.conf.json`. Its exact build output and
binary hashes are recorded in [bundle-verification.json](bundle-verification.json).
Deep/strict App signing verification, DMG checksum verification and a read-only
mount comparing all three packaged executable hashes passed. The accepted QA
runtime and bundled runtime have identical executable bytes after signatures
are removed from temporary comparison copies; their signatures differ because
the Tauri bundle additionally enables hardened runtime. The bundle includes the
runtime and existing complete third-party notices. Ad hoc
signing is for local acceptance; Apple notarization and Windows/Linux acceptance
are outside this candidate. The new operations were verified on the local test
page, not on Gmail, QQ Mail, WPS or any production account.

The build needed the installed Homebrew Node 26 because the shell default is
Node 22. The normal pinned-pnpm signature checks were retained. Restricted
network/cache failures were resolved by rerunning the build with authorized
network access; no package signature or permission checks were bypassed.
