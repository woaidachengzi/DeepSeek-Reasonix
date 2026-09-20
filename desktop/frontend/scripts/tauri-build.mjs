import { mkdirSync, readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(scriptDirectory, "../../..");
const tauriDirectory = join(repositoryRoot, "desktop", "tauri");
const sidecarDirectory = join(tauriDirectory, "binaries");
const tauriBinary = join(
  repositoryRoot,
  "desktop",
  "frontend",
  "node_modules",
  ".bin",
  process.platform === "win32" ? "tauri.cmd" : "tauri",
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
const targetIndex = bundleArguments.findIndex(argument => argument === "--target");
const requestedTarget = targetIndex === -1 ? hostTriple.stdout.trim() : bundleArguments[targetIndex + 1];
if (!requestedTarget) throw new Error("--target requires a target triple");
if (requestedTarget !== hostTriple.stdout.trim()) {
  throw new Error(
    `cross-target Tauri bundling is not supported yet: requested ${requestedTarget}, host is ${hostTriple.stdout.trim()}`,
  );
}

const extension = process.platform === "win32" ? ".exe" : "";
const bridgeBinary = join(
  sidecarDirectory,
  `reasonix-desktop-bridge-${requestedTarget}${extension}`,
);
mkdirSync(sidecarDirectory, { recursive: true });
const bridgeBuild = spawnSync("go", ["build", "-trimpath", "-o", bridgeBinary, "./cmd/reasonix-desktop-bridge"], {
  cwd: repositoryRoot,
  stdio: "inherit",
});
if (bridgeBuild.error) throw bridgeBuild.error;
if (bridgeBuild.status !== 0) process.exit(bridgeBuild.status ?? 1);

const tauri = spawnSync(tauriBinary, ["build", ...bundleArguments], {
  cwd: tauriDirectory,
  stdio: "inherit",
});
if (tauri.error) throw tauri.error;
if (tauri.status !== 0) process.exit(tauri.status ?? 1);

// A local preview has no Developer ID certificate. Make its app bundle
// internally consistent so macOS can run it after the user clears quarantine.
// Official builds set APPLE_SIGNING_IDENTITY and keep Tauri's Developer ID
// signature for notarization instead.
if (process.platform === "darwin" && !process.env.APPLE_SIGNING_IDENTITY) {
  const { productName } = JSON.parse(readFileSync(join(tauriDirectory, "tauri.conf.json"), "utf8"));
  if (typeof productName !== "string" || !productName.trim()) {
    throw new Error("tauri.conf.json must define a productName before macOS signing");
  }
  const appBundle = join(tauriDirectory, "target", "release", "bundle", "macos", `${productName}.app`);
  const signing = spawnSync("codesign", ["--force", "--deep", "-s", "-", appBundle], {
    stdio: "inherit",
  });
  if (signing.error) throw signing.error;
  if (signing.status !== 0) process.exit(signing.status ?? 1);
}
