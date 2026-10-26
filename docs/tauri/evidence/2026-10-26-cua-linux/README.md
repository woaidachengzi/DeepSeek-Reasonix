# Linux Cua acceptance — 2026-10-26

Linux Cua is implemented and packaged as Preview 0.1.62. The ARM64 Ubuntu
26.04 VM was upgraded from 0.1.44 after backing up its original package and
installed payload. Installed host, bridge and runtime hashes match the accepted
deb exactly; see `installation.json`. User configuration was not migrated or
edited by this acceptance workflow.

## Accepted scope and remaining checks

- **Accepted:** X11/XWayland native GTK control on the logged-in GNOME desktop;
  browser control with sandboxed official Chromium Snap on an owned Xvfb server;
  package startup, authentication boundary, ordinary-exit cleanup and the
  packaged Linux computer-control settings page.
- **Unsupported:** GNOME Shell 50.1 native Wayland windows. The pinned upstream
  WinRects helper refuses GNOME 50+ because of reported session-ending crashes.
  Its version checks and refusal remain intact; the extension was not enabled.
  Portal/PipeWire support being compiled does not constitute live acceptance.
- **Settings visual check passed:** at 17:37 China time, after the user opened
  Settings → Computer control, the packaged app showed the bundled desktop
  control card, X11/XWayland connected, AT-SPI connected and the GNOME 50 native
  Wayland warning. No macOS permission controls appeared in that Linux status
  card. See `linux-settings.png` and `settings-visual.json`.
  The displayed workspace uses enforced isolation, so computer control is
  correctly unavailable under that policy. The app explains that unrestricted
  Bash would also remove command isolation. No sandbox policy was changed.
  Earlier Parallels input/black-display trouble required user navigation;
  it is not counted as successful remote settings navigation.
  After inspection, disposable GUI QA processes were stopped by matching both
  their private HOME and runtime directory and rechecking PID start times;
  `gui-cleanup.json` confirms none remain. The user desktop and installed app
  were not stopped or removed.
- x86_64 Linux, other compositors/distributions and a real model-driven chat
  have not been accepted here. Native pixel-coordinate `move` is deliberately
  absent on Linux; browser ref-scoped hover is available.

## Actual control checks

`native-acceptance.json` records seven checks through the actual
`computer_use` builtin → Go controller → Rust worker → pinned Cua SDK. The
fixture was a disposable GTK window on the visible GNOME XWayland desktop.
The fixture's independent oracle read back **Reasonix Linux 中文 2026** and
exactly **one** click. Same-user process identity, independent Rust identity,
window screenshot and wrong-PID refusal passed. The runtime included the final
UTF-8 byte-length fix; later wrapper changes refused Linux native `move` and
corrected a Linux-specific metadata test expectation.

`browser-acceptance.json` records 18 checks using official Chromium Snap
156.0.8078.47, real user D-Bus and a transient user-systemd scope on an isolated
Xvfb server. Chromium's sandbox and Ubuntu's AppArmor protections stayed on.
Checks cover profile-consent refusal, exact tab, Chinese replacement, select
label/empty/no-op, background trusted-pointer refusal with unchanged readback,
foreground trusted hover/right-click/double-click/scroll/drag, one trusted click,
cross-window refusal, navigation, stale-ref refusal and tab screenshots.
The local HTTP oracle confirms one click with `event.isTrusted === true`.
This does not prove native Wayland browser input or ordinary user-profile use.

The two `*-evidence.jsonl` files retain fixture requests/results without image
bytes; token fields are redacted. No real-user browser profile, credentials or
model service were used.

## Build and package verification

- Final Linux targeted race checks passed for `internal/computeruse`,
  `internal/tool/builtin`, and `cmd/reasonix-desktop-bridge`.
- All nine Linux Rust tests passed, including the actual pinned SDK inventory,
  live process identity and shared Go/Rust malformed-identity corpus.
- Frontend lint, layer/scroll/style contracts, TypeScript, Vite and bundle
  budgets passed. The targeted settings DOM test also passed, including Linux
  status, GNOME 50 warning and absence of macOS permission controls.
- Both hermetic Python source-export tests passed, including audited patches,
  helper export, dirty-source and unowned-cache refusals.
- `package-lifecycle.json`: deb and extracted AppImage AppRun each passed
  managed and explicit-profile startup (four runs), unauthenticated health
  returned 401, and ordinary RunEvent::Exit removed sidecar/readiness state.
  These probes used private Xvfb/D-Bus; portal connection warnings in that
  environment are not Wayland acceptance.
- Package audit found all dynamic dependencies resolved, both audited MIT
  source patches and original notices included, GNOME 50 guard retained, and
  no `.env`, user config/database or JSONL files in the payload. APT first
  simulated, then installed only the Reasonix update without new dependencies.

`final-package-build2.log`, `final-audit.log`, `package-lifecycle.json` and
`package-sha256.json` provide the results and hashes. Packages are in the local
ignored `desktop/tauri/target/linux-cua-20261026/final-packages/` directory.
AppImage linuxdeploy rewrites ELF RUNPATH to `$ORIGIN/../lib`, so the runtime's
whole-file hash differs from deb; `.text` and `.rodata` hashes match exactly.
The bundled bridge matches byte for byte in both artifacts.

Early parallel Rust builds exhausted this 3.3 GiB VM's memory and systemd-oomd
killed GNOME. Builds were changed to one job; final compilation finished before
the last desktop login, and no further heavy builds were run. No OOM policy,
lock-screen policy, browser sandbox or compositor guard was weakened.

## Source provenance

Reasonix base: `fb470ec77513435476785c56653220c52a1d18a4`, dirty workspace.
`SOURCE_IDENTITY.json` identifies the initial source archive. Applying
`OVERLAY-7.json` gives the final production source used for both packages.
The private guest source export excluded `.git`, so embedded Preview source
metadata says `unknown`; these artifacts must not be represented as a clean
commit build. `current-source-sha256.json` differs only for the subsequently
improved browser QA harness (debugger-readiness wait and background-refusal
check), which was used for the accepted 18-check browser run.

Cua Driver 0.38.0 remains pinned to
`e754cfec69c41400eb2aabde716c27d15607b2f0`. Two strictly matched MIT source
patches are documented with original/patched SHA-256 in the package audit:
Rust 1.93-compatible Linux imports and the AT-SPI UTF-8 byte-length correction.
MPL source was not modified. See the runtime README for build prerequisites and
the upstream AT-SPI interface reference.

## Pre-commit review

Reviewed process-incarnation checks in Go/Rust, desktop environment allowlist,
read-only readiness, Linux-only action exclusions, source export/notices,
package configuration, settings request generations and fixture evidence.
No blocking findings remained. Fresh macOS-host regression checks passed:

- Complete race suites for `internal/computeruse`, `internal/tool/builtin` and
  `cmd/reasonix-desktop-bridge` (25.591 s, 15.873 s and 76.551 s respectively).
- Eight Rust wrapper tests with `--locked --offline`.
- Both hermetic Python source-export tests.
- Computer-control settings DOM test with Node 24.21.0.
- Go formatting, Rust formatting and `git diff --check`.

The accepted Linux packages were not rebuilt during review. Their nine Linux
Rust tests, Linux targeted race checks, full frontend build and live acceptance
remain recorded above; production source did not change after those checks.
