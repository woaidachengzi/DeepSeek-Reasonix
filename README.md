<p align="center">
  <img src="docs/logo-ghost-wave-effect.svg" alt="Reasonix" width="360"/>
</p>

# Reasonix Tauri

[简体中文](./README.zh-CN.md) · [Tauri host](./desktop/tauri/README.md) · [Migration checklist](./docs/tauri/E_MIGRATION_CHECKLIST.md)

An independently maintained Tauri desktop adaptation of Reasonix. This branch focuses on the desktop experience, configuration management, and native packaging for macOS, Windows, and Linux, while retaining the existing Go agent engine.

Repository: [woaidachengzi/DeepSeek-Reasonix](https://github.com/woaidachengzi/DeepSeek-Reasonix) · Branch: `experiment/tauri`

## Maintenance and project scope

This adaptation is independently developed and maintained by [woaidachengzi](https://github.com/woaidachengzi). The Tauri migration and the improvements described below are work on this fork; the original Go engine and inherited features come from upstream Reasonix.

The desktop application is **Reasonix Tauri Preview**, version `0.1.0`, with its own application identity `io.reasonix.desktop.preview`. It does not replace the stable Wails application. A Preview package is not a statement that every migration or release gate has passed.

## Work on this branch

- **Tauri desktop host:** a Rust shell supervises the Go sidecar, native windows, menus, and desktop services; the frontend uses a typed bridge.
- **Desktop workbench:** projects and sessions, model selection, streamed conversations, Markdown, attachments, and session lifecycle management.
- **Model and configuration pages:** provider/model settings, credential save/delete, and consistent model-preference and model-service page backgrounds across themes and layouts.
- **Remote management:** SSH authentication and host-fingerprint confirmation, SFTP browsing/editing/path operations, and Serve status/log/start/stop controls with a system-browser controller entry.
- **Bot and management migration:** account management, pairing approvals, and settings surfaces for permissions, plugins, MCP, hooks, memory, subagents, data, and diagnostics. Migration and real-service acceptance remain incremental.
- **Native packages:** Windows x64 NSIS and native Linux ARM64 deb/AppImage previews, plus platform-specific tray and window-close handling.
- **Profile isolation and verification:** Preview-managed data is separate from the stable profile; source identity, regression tests, package hashes, and acceptance records accompany the migration.

These are implemented areas, not a claim of complete Wails feature parity. Native remote tabs, remaining bot workflows, and package-level management-page acceptance are tracked in the [E migration checklist](./docs/tauri/E_MIGRATION_CHECKLIST.md). Automatic updates are outside the current migration scope.

## Platform status

Status as of 2026-10-07:

| Platform | Preview package | Current evidence |
| --- | --- | --- |
| macOS Apple Silicon | Development-signed app / DMG | Preview builds and selected package checks completed; native window and release gates remain open |
| Windows x64 | NSIS installer `.exe` | Built and package contents checked; the full native manual matrix remains unrecorded |
| Ubuntu 26.04 ARM64 | `.deb` / `.AppImage` | Native build, 224 Rust tests passed / 1 ignored, package audit completed; maintainer confirmed manual acceptance |
| Linux x64 / other distributions | Native build support and manual x64 packaging workflow | Not validated by the Ubuntu ARM64 result |

See [Windows acceptance](./docs/tauri/WINDOWS_PREVIEW_ACCEPTANCE.md), [Linux acceptance](./docs/tauri/LINUX_PREVIEW_ACCEPTANCE.md), and [Ubuntu package hashes](./docs/tauri/evidence/2026-10-07-linux-arm64/README.md).

Current packages are local preview handoffs, not a published or signed stable release channel. Upstream npm packages, websites, downloads, and signing statements do not describe this fork's Tauri packages.

## Try a preview

Use the package handed off for your exact OS and architecture, and verify its supplied SHA256 before running it. Install/launch **Reasonix Tauri Preview**, then configure a provider and model in the application.

For the Ubuntu ARM64 handoff, open a terminal in the package directory:

```bash
sha256sum -c SHA256SUMS
sudo apt install "./Reasonix Tauri Preview_0.1.0_arm64.deb"
```

AppImage alternative:

```bash
chmod +x "./Reasonix Tauri Preview_0.1.0_aarch64.AppImage"
"./Reasonix Tauri Preview_0.1.0_aarch64.AppImage"
# If FUSE is unavailable:
APPIMAGE_EXTRACT_AND_RUN=1 "./Reasonix Tauri Preview_0.1.0_aarch64.AppImage"
```

Packages do not include the maintainer's personal API keys or configuration. Default model entries are not saved credentials; supply your own provider credentials. Linux system credential storage requires an available session D-Bus and Secret Service.

The default managed Preview profile is isolated from the stable `~/.reasonix` profile. Existing data is imported only through an explicit action. An explicitly supplied `REASONIX_HOME` overrides the default; do not point a test build at your stable data without a backup.

## Build this branch

Prerequisites: Node 24+, pnpm 10.34.5, Go matching `go.mod` (currently toolchain 1.26.6), Rust 1.89+, and your platform's native build/WebView dependencies. The local frontend Tauri CLI is used; Wails CLI is not required for the Tauri build.

```bash
git clone --branch experiment/tauri https://github.com/woaidachengzi/DeepSeek-Reasonix.git
cd DeepSeek-Reasonix
pnpm --dir desktop/frontend install --frozen-lockfile
pnpm --dir desktop/frontend tauri:dev
```

macOS package:

```bash
pnpm --dir desktop/frontend tauri:build
```

Windows native build, from the repository root after installing Visual Studio C++ Build Tools:

```powershell
powershell -ExecutionPolicy Bypass -File desktop/tauri/scripts/build-windows.ps1
```

Native GNU Linux build, on the same architecture after preparing GTK/WebKitGTK and other required libraries:

```bash
bash desktop/tauri/scripts/build-linux.sh
# Debian package only:
bash desktop/tauri/scripts/build-linux.sh deb
```

Linux packages are generated under `desktop/tauri/target/<native Rust triple>/release/bundle/`. This packaging path does not support macOS-to-Linux or cross-architecture Linux builds. See the [Tauri host guide](./desktop/tauri/README.md) for details and Windows cross-build prerequisites.

The inherited CLI can still be built with `make build`; legacy Wails instructions are in [desktop/README.md](./desktop/README.md). These are separate from the Tauri Preview build.

## Documentation

- [Tauri host, development, and profile boundaries](./desktop/tauri/README.md)
- [E migration scope and remaining work](./docs/tauri/E_MIGRATION_CHECKLIST.md)
- [API surface audit and release gates](./docs/tauri/API_SURFACE_AUDIT.md)
- [Windows preview acceptance](./docs/tauri/WINDOWS_PREVIEW_ACCEPTANCE.md)
- [Linux preview acceptance](./docs/tauri/LINUX_PREVIEW_ACCEPTANCE.md)
- Inherited engine documentation: [Guide](./docs/GUIDE.md) · [CLI](./docs/CLI.md) · [ACP](./docs/ACP.md) · [Extensions](./docs/EXTENSIONS.md) · [Configuration paths](./docs/CONFIG_PATHS.md)

Engine and legacy desktop documents may describe upstream/Wails behavior; use the Tauri migration and acceptance records for this Preview's actual scope.

## Project origin and license

This fork is based on [esengine/DeepSeek-Reasonix](https://github.com/esengine/DeepSeek-Reasonix). Thanks to the upstream authors and contributors for the original engine and project foundation. The retained project logo is credited to [Bernardxu123](https://github.com/Bernardxu123).

Upstream contributors are historical contributors to that foundation, not a list of developers working on this Tauri adaptation. This README does not display an upstream contribution leaderboard or donation destination as belonging to this fork.

The original copyright and MIT license notice are retained in [LICENSE](./LICENSE).
