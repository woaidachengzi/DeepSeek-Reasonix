// Run: node scripts/tauri-build-contract.test.mjs
//
// Verifies the tauri-build.mjs script's key contracts without actually
// building: sidecar path naming, signing logic, and environment checks.

import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { resolveBuildRunner, resolveBuildTarget } from "./tauri-build-target.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
const scriptSource = readFileSync(resolve(__dirname, "tauri-build.mjs"), "utf8");

let passed = 0;
let failed = 0;

function eq(a, b, label) {
  if (a === b) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${JSON.stringify(b)}, got ${JSON.stringify(a)}\n`);
    failed += 1;
  }
}

function ok(condition, label) {
  if (condition) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

function contains(haystack, needle, label) {
  ok(haystack.includes(needle), label);
}

function notContains(haystack, needle, label) {
  ok(!haystack.includes(needle), label);
}

// ---------------------------------------------------------------------------
// Contract 1: Node version gate
// ---------------------------------------------------------------------------

console.log("\ntauri-build contract — Node version gate");
contains(scriptSource, 'process.versions.node', "checks process.versions.node");
contains(scriptSource, '< 24', "requires Node 24+");

// ---------------------------------------------------------------------------
// Contract 2: Rust host triple detection
// ---------------------------------------------------------------------------

console.log("\ntauri-build contract — Rust host triple");
contains(scriptSource, 'rustc', "invokes rustc for host triple");
contains(scriptSource, '--print', "uses rustc --print");
contains(scriptSource, 'host-tuple', "requests host-tuple from rustc");

// ---------------------------------------------------------------------------
// Contract 3: Sidecar path naming convention
// ---------------------------------------------------------------------------

console.log("\ntauri-build contract — sidecar path naming");

// The sidecar binary must follow Tauri's naming convention:
//   binaries/<name>-<target-triple>[.exe]
// This is how Tauri resolves the sidecar at runtime.
contains(scriptSource, 'reasonix-desktop-bridge-', "sidecar name starts with reasonix-desktop-bridge-");
contains(scriptSource, 'requestedTarget', "sidecar path includes target triple");
contains(scriptSource, 'sidecarDirectory', "sidecar is placed in binaries/ directory");

// The bridge binary is built from the root module, not desktop/.
contains(scriptSource, './cmd/reasonix-desktop-bridge', "go build targets the correct package");

// ---------------------------------------------------------------------------
// Contract 4: Cross-compilation guard
// ---------------------------------------------------------------------------

console.log("\ntauri-build contract — cross-compilation guard");
const windowsTarget = resolveBuildTarget(["--target", "x86_64-pc-windows-msvc"], "aarch64-apple-darwin");
eq(windowsTarget.extension, ".exe", "Windows sidecar extension follows target, not host");
eq(windowsTarget.goEnv.GOOS, "windows", "Windows sidecar is cross-compiled for Windows");
eq(windowsTarget.goEnv.GOARCH, "amd64", "Rust x64 maps to Go amd64");
eq(windowsTarget.goEnv.CGO_ENABLED, "0", "Windows sidecar does not require a host C toolchain");
eq(resolveBuildTarget(["--target=aarch64-pc-windows-msvc"], "aarch64-apple-darwin").goEnv.GOARCH, "arm64", "inline ARM64 target maps to Go arm64");
eq(resolveBuildTarget([], "aarch64-apple-darwin").extension, "", "native macOS build remains unchanged");
eq(resolveBuildRunner(windowsTarget, "aarch64-apple-darwin", "darwin"), "cargo-xwin", "Windows cross-build on macOS uses cargo-xwin");
eq(resolveBuildRunner(resolveBuildTarget(["--target=aarch64-pc-windows-msvc"], "x86_64-pc-windows-msvc"), "x86_64-pc-windows-msvc", "win32"), null, "Windows architecture switch retains the native MSVC toolchain");
eq(resolveBuildRunner(windowsTarget, windowsTarget.target, "win32"), null, "native Windows does not require cargo-xwin");
for (const [target, arch] of [["x86_64-unknown-linux-gnu", "amd64"], ["aarch64-unknown-linux-gnu", "arm64"]]) {
  const nativeLinux = resolveBuildTarget([], target);
  eq(nativeLinux.extension, "", "native Linux sidecar has no Windows extension");
  eq(nativeLinux.goEnv.GOOS, "linux", "native Linux ignores an inherited Go target OS");
  eq(nativeLinux.goEnv.GOARCH, arch, "native Linux Go architecture matches Rust");
  eq(nativeLinux.goEnv.CGO_ENABLED, "0", "Linux sidecar does not add a libc dependency");
  eq(resolveBuildRunner(nativeLinux, target, "linux"), null, "native Linux retains cargo");
}
for (const args of [["--target"], ["--target", "--debug"], ["--target=x86_64-unknown-linux-gnu"]]) {
  let rejected = false;
  try { resolveBuildTarget(args, "aarch64-apple-darwin"); } catch { rejected = true; }
  ok(rejected, `rejects invalid or unsupported target ${args.join(" ")}`);
}
for (const target of ["x86_64-unknown-linux-musl", "armv7-unknown-linux-gnu"]) {
  let rejected = false;
  try { resolveBuildTarget([], target); } catch { rejected = true; }
  ok(rejected, `rejects unsupported native Linux host ${target}`);
}

// ---------------------------------------------------------------------------
// Contract 5: macOS ad-hoc signing
// ---------------------------------------------------------------------------

console.log("\ntauri-build contract — macOS signing");

// When no APPLE_SIGNING_IDENTITY is set, Tauri must ad-hoc sign before
// bundling so the .app inside the DMG has the same valid signature.
contains(scriptSource, 'APPLE_SIGNING_IDENTITY', "checks for signing identity");
contains(scriptSource, 'codesign', "invokes codesign for ad-hoc signing");
contains(scriptSource, '...(adHocMacOSBuild ? { APPLE_SIGNING_IDENTITY: "-" } : {})', "Tauri signs before bundling the DMG");
contains(scriptSource, '--verify', "checks the packaged app signature");
contains(scriptSource, '--deep', "checks nested binaries");
contains(scriptSource, '--strict', "uses strict signature verification");
contains(scriptSource, '.app', "checks the .app bundle");

// Official builds (with APPLE_SIGNING_IDENTITY) keep their identity.
contains(scriptSource, '!process.env.APPLE_SIGNING_IDENTITY', "ad-hoc signing only when no identity");

// ---------------------------------------------------------------------------
// Contract 6: Tauri config references
// ---------------------------------------------------------------------------

console.log("\ntauri-build contract — Tauri config");

// The script reads productName from tauri.conf.json for the .app bundle path.
contains(scriptSource, 'tauri.conf.json', "reads tauri.conf.json");
contains(scriptSource, 'productName', "uses productName for bundle path");

// The build uses the local Tauri CLI from node_modules.
contains(scriptSource, '"@tauri-apps", "cli", "tauri.js"', "uses the local Tauri JS entrypoint on all platforms");
contains(scriptSource, 'spawnSync(process.execPath', "runs the CLI with Node instead of executing a Windows cmd shim");

// ---------------------------------------------------------------------------
// Contract 7: tauri.conf.json bundle config
// ---------------------------------------------------------------------------

console.log("\ntauri-build contract — tauri.conf.json bundle config");

const tauriConf = JSON.parse(
  readFileSync(resolve(__dirname, "../../tauri/tauri.conf.json"), "utf8"),
);
const windowsConf = JSON.parse(readFileSync(resolve(__dirname, "../../tauri/tauri.windows.conf.json"), "utf8"));
eq(windowsConf.bundle.windows.nsis.installMode, "currentUser", "Preview installs without replacing a machine-wide stable installation");
eq(windowsConf.bundle.targets[0], "nsis", "Windows defaults to the cross-buildable NSIS installer");
ok(windowsConf.bundle.icon.some(path => path.endsWith(".ico")), "Windows has an ICO resource");
const linuxConf = JSON.parse(readFileSync(resolve(__dirname, "../../tauri/tauri.linux.conf.json"), "utf8"));
eq(JSON.stringify(linuxConf.bundle.targets), JSON.stringify(["deb", "appimage"]), "Linux produces Debian and AppImage packages");
ok(linuxConf.bundle.icon.every(path => path.endsWith(".png")), "Linux packaging uses a PNG icon, not macOS ICNS");
ok(linuxConf.bundle.linux.deb.depends.includes("libayatana-appindicator3-1"), "Debian package declares its tray runtime dependency");
const linuxBuildSource = readFileSync(resolve(__dirname, "../../tauri/scripts/build-linux.sh"), "utf8");
contains(linuxBuildSource, 'uname -s', "Linux script rejects non-Linux hosts before building");
contains(linuxBuildSource, 'rustc --print host-tuple', "Linux script builds the native Rust target");
contains(linuxBuildSource, 'pkg-config --exists', "Linux script checks GTK/WebKit and D-Bus prerequisites");
contains(linuxBuildSource, 'sha256sum', "Linux script prints package verification hashes");
const tauriConfigDirectory = resolve(__dirname, "../../tauri");
const mainWindowCapability = JSON.parse(
  readFileSync(resolve(tauriConfigDirectory, "capabilities/main-window.json"), "utf8"),
);

eq(tauriConf.bundle?.active, true, "bundle.active is true");
eq(tauriConf.bundle?.targets, "all", "bundle.targets is 'all'");
ok(
  tauriConf.bundle?.externalBin?.some(p => p.includes("reasonix-desktop-bridge")),
  "externalBin includes reasonix-desktop-bridge",
);
ok(
  tauriConf.bundle?.icon?.length > 0,
  "bundle has icon files",
);
ok(
  tauriConf.build?.frontendDist,
  "build.frontendDist is set",
);
eq(
  tauriConf.build?.frontendDist,
  "../frontend/dist",
  "frontendDist points to ../frontend/dist",
);

const expectedWebviewPermissions = [
  "core:event:allow-listen",
  "core:event:allow-unlisten",
  "core:window:allow-start-dragging",
  "dialog:allow-open",
  "clipboard-manager:allow-read-text",
  "clipboard-manager:allow-write-text",
];
eq(
  JSON.stringify([...mainWindowCapability.permissions].sort()),
  JSON.stringify(expectedWebviewPermissions.sort()),
  "WebView has only event-listener, drag, file-picker, and text clipboard permissions",
);

// Tauri decodes the configured PNG into an RGBA buffer at application launch.
// A 16-bit RGBA PNG looks like a 1024px icon to macOS tools, but supplies twice
// the byte length Tauri expects and aborts the app during startup.
console.log("\ntauri-build contract — launch icon format");

const pngIcon = tauriConf.bundle?.icon?.find(icon => icon.endsWith(".png"));
ok(Boolean(pngIcon), "bundle includes a PNG launch icon");

if (pngIcon) {
  const iconData = readFileSync(resolve(tauriConfigDirectory, pngIcon));
  const pngSignature = "89504e470d0a1a0a";

  eq(iconData.subarray(0, 8).toString("hex"), pngSignature, "launch icon is a PNG");
  eq(iconData.toString("ascii", 12, 16), "IHDR", "launch icon has an IHDR header");
  eq(iconData.readUInt32BE(16), 1024, "launch icon width is 1024px");
  eq(iconData.readUInt32BE(20), 1024, "launch icon height is 1024px");
  eq(iconData[24], 8, "launch icon uses 8-bit channels");
  eq(iconData[25], 6, "launch icon uses RGBA color type");
}

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
