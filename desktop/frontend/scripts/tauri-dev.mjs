import { mkdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawn, spawnSync } from "node:child_process";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(scriptDirectory, "../../..");
const tauriDirectory = join(repositoryRoot, "desktop", "tauri");
const sidecarDirectory = join(tauriDirectory, "target", "sidecar-dev");
const bridgeBinary = join(sidecarDirectory, process.platform === "win32" ? "reasonix-desktop-bridge.exe" : "reasonix-desktop-bridge");
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

mkdirSync(sidecarDirectory, { recursive: true });
const build = spawnSync("go", ["build", "-o", bridgeBinary, "./cmd/reasonix-desktop-bridge"], {
  cwd: repositoryRoot,
  stdio: "inherit",
});
if (build.error) throw build.error;
if (build.status !== 0) process.exit(build.status ?? 1);

// The bridge executable is generated under Tauri's ignored target directory,
// then passed only to the host process. The WebView never receives this path,
// its token, or the bridge's loopback address.
const tauri = spawn(tauriBinary, ["dev", ...process.argv.slice(2)], {
  cwd: tauriDirectory,
  env: { ...process.env, REASONIX_DESKTOP_BRIDGE_BIN: bridgeBinary },
  stdio: "inherit",
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => tauri.kill(signal));
}
tauri.on("error", error => {
  throw error;
});
tauri.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  process.exit(code ?? 1);
});
