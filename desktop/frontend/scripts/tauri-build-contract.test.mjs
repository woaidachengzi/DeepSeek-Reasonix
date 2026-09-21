// Run: node scripts/tauri-build-contract.test.mjs
//
// Verifies the tauri-build.mjs script's key contracts without actually
// building: sidecar path naming, signing logic, and environment checks.

import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

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
contains(scriptSource, 'cross-target', "has cross-target error message");
contains(scriptSource, 'not supported yet', "cross-target is explicitly rejected");

// ---------------------------------------------------------------------------
// Contract 5: macOS ad-hoc signing
// ---------------------------------------------------------------------------

console.log("\ntauri-build contract — macOS signing");

// When no APPLE_SIGNING_IDENTITY is set, the script must ad-hoc sign
// the .app bundle so macOS will run it after quarantine clearance.
contains(scriptSource, 'APPLE_SIGNING_IDENTITY', "checks for signing identity");
contains(scriptSource, 'codesign', "invokes codesign for ad-hoc signing");
contains(scriptSource, '--force', "codesign uses --force flag");
contains(scriptSource, '--deep', "codesign uses --deep flag");
contains(scriptSource, '"-s"', 'codesign uses ad-hoc signing flag'); contains(scriptSource, '"-"', 'codesign uses ad-hoc identity');
contains(scriptSource, '.app', "signs the .app bundle");

// Official builds (with APPLE_SIGNING_IDENTITY) skip ad-hoc signing.
contains(scriptSource, '!process.env.APPLE_SIGNING_IDENTITY', "ad-hoc signing only when no identity");

// ---------------------------------------------------------------------------
// Contract 6: Tauri config references
// ---------------------------------------------------------------------------

console.log("\ntauri-build contract — Tauri config");

// The script reads productName from tauri.conf.json for the .app bundle path.
contains(scriptSource, 'tauri.conf.json', "reads tauri.conf.json");
contains(scriptSource, 'productName', "uses productName for bundle path");

// The build uses the local Tauri CLI from node_modules.
contains(scriptSource, '.bin', "uses local Tauri CLI from node_modules/.bin");

// ---------------------------------------------------------------------------
// Contract 7: tauri.conf.json bundle config
// ---------------------------------------------------------------------------

console.log("\ntauri-build contract — tauri.conf.json bundle config");

const tauriConf = JSON.parse(
  readFileSync(resolve(__dirname, "../../tauri/tauri.conf.json"), "utf8"),
);
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

ok(
  mainWindowCapability.permissions?.includes("dialog:allow-open"),
  "workspace picker has only the dialog open permission it needs",
);
ok(
  !mainWindowCapability.permissions?.includes("dialog:default"),
  "workspace picker does not receive dialog save or message permissions",
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
