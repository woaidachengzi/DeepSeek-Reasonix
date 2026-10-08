import { existsSync, lstatSync, mkdirSync, readFileSync, realpathSync, renameSync, symlinkSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { resolveBuildRunner, resolveBuildTarget, resolveBundleDirectory } from "./tauri-build-target.mjs";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(scriptDirectory, "../../..");
const tauriDirectory = join(repositoryRoot, "desktop", "tauri");
const sidecarDirectory = join(tauriDirectory, "binaries");
const tauriBinary = join(
  repositoryRoot,
  "desktop",
  "frontend",
  "node_modules",
  "@tauri-apps", "cli", "tauri.js",
);

if (Number(process.versions.node.split(".")[0]) < 24) {
  throw new Error(`Node 24 or newer is required; found ${process.version}`);
}

const hostTriple = spawnSync("rustc", ["--print", "host-tuple"], { encoding: "utf8" });
if (hostTriple.error) throw hostTriple.error;
if (hostTriple.status !== 0 || !hostTriple.stdout.trim()) {
  throw new Error("could not determine the Rust host target triple");
}

const bundleArguments = process.argv.slice(2);
if (bundleArguments[0] === "--") bundleArguments.shift();
const { target: requestedTarget, windows: windowsBuild, extension, goEnv } = resolveBuildTarget(bundleArguments, hostTriple.stdout.trim());
const bridgeBinary = join(
  sidecarDirectory,
  `reasonix-desktop-bridge-${requestedTarget}${extension}`,
);
mkdirSync(sidecarDirectory, { recursive: true });
const bridgeBuild = spawnSync("go", ["build", "-trimpath", "-o", bridgeBinary, "./cmd/reasonix-desktop-bridge"], {
  cwd: repositoryRoot,
  env: { ...process.env, ...goEnv },
  stdio: "inherit",
});
if (bridgeBuild.error) throw bridgeBuild.error;
if (bridgeBuild.status !== 0) process.exit(bridgeBuild.status ?? 1);

const sourceCommit = spawnSync("git", ["rev-parse", "--verify", "HEAD"], {
  cwd: repositoryRoot,
  encoding: "utf8",
});
const previewCommit = sourceCommit.status === 0 && /^[0-9a-f]{40,64}$/i.test(sourceCommit.stdout.trim())
  ? sourceCommit.stdout.trim()
  : "unknown";
const sourceStatus = spawnSync("git", ["status", "--porcelain", "--untracked-files=normal"], {
  cwd: repositoryRoot,
  encoding: "utf8",
});
const previewDirty = previewCommit !== "unknown" && (sourceStatus.status !== 0 || sourceStatus.stdout.trim() !== "");
console.log(`Preview source: ${previewCommit}${previewDirty ? " (uncommitted changes)" : ""}`);
// Vite empties dist before building. Keep the tracked placeholder so a local
// package build does not leave a clean source checkout marked as modified.
const frontendPlaceholder = join(repositoryRoot, "desktop", "frontend", "dist", ".gitkeep");
const placeholderBytes = existsSync(frontendPlaceholder) ? readFileSync(frontendPlaceholder) : null;
// Tauri must sign the app before bundling the DMG. Signing only the standalone
// .app after `tauri build` leaves the copy already sealed in the DMG invalid.
const macOSBuild = requestedTarget.endsWith("-apple-darwin");
const adHocMacOSBuild = macOSBuild && !process.env.APPLE_SIGNING_IDENTITY;
const bundleDirectory = resolveBundleDirectory(tauriDirectory, bundleArguments, requestedTarget, process.env.CARGO_TARGET_DIR);
// Keep development bundles out of app search. The compatibility alias retains
// Tauri's normal output path; the actual directory is excluded by Spotlight.
if (macOSBuild) {
  const macosDirectory = join(bundleDirectory, "macos");
  const storageDirectory = join(bundleDirectory, "macos.noindex");
  mkdirSync(bundleDirectory, { recursive: true });
  if (existsSync(macosDirectory) && !lstatSync(macosDirectory).isSymbolicLink()) {
    if (existsSync(storageDirectory)) throw new Error("both macOS bundle directories exist; preserve them and resolve the conflict before building");
    renameSync(macosDirectory, storageDirectory);
  }
  mkdirSync(storageDirectory, { recursive: true });
  if (!existsSync(macosDirectory)) symlinkSync("macos.noindex", macosDirectory, "dir");
  if (realpathSync(macosDirectory) !== realpathSync(storageDirectory)) throw new Error("macOS bundle alias points outside its expected storage directory");
}
const defaultRunner = resolveBuildRunner({ target: requestedTarget, windows: windowsBuild }, hostTriple.stdout.trim(), process.platform);
if (defaultRunner && !bundleArguments.some(arg => arg === "--runner" || arg.startsWith("--runner="))) {
  bundleArguments.push("--runner", defaultRunner);
}
const tauri = spawnSync(process.execPath, [tauriBinary, "build", ...bundleArguments], {
  cwd: tauriDirectory,
  env: {
    ...process.env,
    REASONIX_PREVIEW_COMMIT: previewCommit,
    REASONIX_PREVIEW_DIRTY: previewDirty ? "1" : "0",
    ...(adHocMacOSBuild ? { APPLE_SIGNING_IDENTITY: "-" } : {}),
  },
  stdio: "inherit",
});
if (placeholderBytes !== null) {
  mkdirSync(dirname(frontendPlaceholder), { recursive: true });
  writeFileSync(frontendPlaceholder, placeholderBytes);
}
if (tauri.error) throw tauri.error;
if (tauri.status !== 0) process.exit(tauri.status ?? 1);

// Verify the exact .app Tauri placed into the bundle. A local preview uses
// ad-hoc signing; official builds keep their Developer ID signature.
if (macOSBuild) {
  const { productName } = JSON.parse(readFileSync(join(tauriDirectory, "tauri.conf.json"), "utf8"));
  if (typeof productName !== "string" || !productName.trim()) {
    throw new Error("tauri.conf.json must define a productName before macOS signature verification");
  }
  const appBundle = join(bundleDirectory, "macos", `${productName}.app`);
  const signing = spawnSync("codesign", ["--verify", "--deep", "--strict", appBundle], {
    stdio: "inherit",
  });
  if (signing.error) throw signing.error;
  if (signing.status !== 0) process.exit(signing.status ?? 1);
}
