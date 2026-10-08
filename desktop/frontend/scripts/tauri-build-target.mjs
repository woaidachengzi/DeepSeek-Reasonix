// Resolve both the Rust host and Go sidecar from the requested bundle target.
import { join, resolve } from "node:path";

// Cargo resolves a relative CARGO_TARGET_DIR from the invocation directory.
// Bundle relocation and signature checks must address that same candidate.
export function resolveBundleDirectory(tauriDirectory, args, target, cargoTargetDirectory) {
  const directory = resolve(tauriDirectory, cargoTargetDirectory || "target");
  const explicitTarget = args.some(arg => arg === "--target" || arg === "-t" || arg.startsWith("--target="));
  return join(directory, ...(explicitTarget ? [target] : []), args.includes("--debug") ? "debug" : "release", "bundle");
}

export function resolveBuildTarget(args, hostTriple) {
  const index = args.findIndex(arg => arg === "--target" || arg === "-t");
  const inline = args.find(arg => arg.startsWith("--target="));
  const target = inline ? inline.slice("--target=".length) : index < 0 ? hostTriple : args[index + 1];
  if (!target || target.startsWith("-")) throw new Error("--target requires a target triple");
  const windows = target.endsWith("-pc-windows-msvc");
  const linux = target.endsWith("-unknown-linux-gnu");
  const arch = target.startsWith("x86_64-") ? "amd64" : target.startsWith("aarch64-") ? "arm64" : "";
  if (target !== hostTriple && (!windows || !arch)) {
    throw new Error(`cross-target ${target} is not supported; only Windows x64/ARM64 MSVC bundles can be cross-built`);
  }
  if (windows && !arch) throw new Error(`unsupported Windows target: ${target}`);
  if (linux && !arch) throw new Error(`unsupported Linux target: ${target}; use x64 or ARM64 GNU`);
  if (target.includes("-linux-") && !linux) throw new Error(`unsupported Linux target: ${target}; GTK/WebKit bundles require a native GNU build`);
  return { target, windows, linux, extension: windows ? ".exe" : "", goEnv: windows || linux ? { GOOS: windows ? "windows" : "linux", GOARCH: arch, CGO_ENABLED: "0" } : {} };
}

// Native Windows uses MSVC even for another Windows architecture. cargo-xwin
// is only the automatic runner for Windows bundles built on a non-Windows host.
export function resolveBuildRunner(buildTarget, hostTriple, hostPlatform) {
  return buildTarget.windows && buildTarget.target !== hostTriple && hostPlatform !== "win32"
    ? "cargo-xwin" : null;
}
