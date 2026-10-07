import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, chmodSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const fixture = mkdtempSync(join(tmpdir(), "reasonix-linux-build-test-"));
const sourceScript = resolve(dirname(fileURLToPath(import.meta.url)), "../../tauri/scripts/build-linux.sh");
// Exercise successful packaging in an isolated repository layout, never the
// real target directory or pre-existing developer packages.
const script = join(fixture, "desktop/tauri/scripts/build-linux.sh");
const outputRoot = join(fixture, "desktop/tauri/target");
const log = join(fixture, "pnpm-args.txt");
const mock = [
  '#!/bin/bash',
  'case "${0##*/}" in',
  '  uname) printf "%s\\n" "${QA_HOST_OS:-Linux}" ;;',
  '  rustc) printf "%s\\n" "${QA_RUST_TARGET:-x86_64-unknown-linux-gnu}" ;;',
  '  node) exit "${QA_NODE_FAILURE:-0}" ;;',
  '  pkg-config) [[ "$2" != "${QA_MISSING_LIBRARY:-}" ]] ;;',
  '  pnpm)',
  '    printf "%s\\n" "$@" > "$QA_BUILD_LOG"',
  '    [[ "${QA_BUILD_SUCCESS:-0}" == 1 ]] || exit 17',
  '    for bundle in deb appimage; do',
  '      [[ "${QA_ARTIFACT_MODE:-complete}" != none ]] || continue',
  '      [[ "${QA_ARTIFACT_MODE:-complete}" != partial || "$bundle" == deb ]] || continue',
  '      out="$QA_OUTPUT_ROOT/${QA_RUST_TARGET:-x86_64-unknown-linux-gnu}/release/bundle/$bundle"',
  '      mkdir -p "$out"',
  '      case "$bundle" in deb) suffix=deb ;; appimage) suffix=AppImage ;; esac',
  '      printf "fixture" > "$out/Reasonix Preview.$suffix"',
  '    done ;;',
  '  file) exit "${QA_FILE_FAILURE:-0}" ;;',
  '  sha256sum) exit "${QA_HASH_FAILURE:-0}" ;;',
  '  *) exit 0 ;;',
  'esac',
  '',
].join("\n");
try {
  mkdirSync(dirname(script), { recursive: true });
  mkdirSync(join(fixture, "desktop/frontend"), { recursive: true });
  writeFileSync(script, readFileSync(sourceScript));
  for (const tool of ["uname", "rustc", "cargo", "node", "pnpm", "go", "pkg-config", "file", "sha256sum"]) {
    const path = join(fixture, tool);
    writeFileSync(path, mock); chmodSync(path, 0o755);
  }
  const run = (args = [], env = {}) => spawnSync("bash", [script, ...args], {
    env: { ...process.env, PATH: `${fixture}:${process.env.PATH}`, QA_BUILD_LOG: log, QA_OUTPUT_ROOT: outputRoot, ...env }, encoding: "utf8",
  });
  for (const [env, message] of [
    [{ QA_HOST_OS: "Darwin" }, /native Linux build host/],
    [{ QA_RUST_TARGET: "x86_64-unknown-linux-musl" }, /Unsupported Linux host/],
    [{ QA_MISSING_LIBRARY: "dbus-1" }, /Missing development library: dbus-1/],
  ]) {
    const result = run([], env);
    assert.equal(result.status, 1); assert.match(result.stderr, message);
  }
  assert.equal(run([], { QA_NODE_FAILURE: "1" }).status, 1, "Node prerequisite failure stops before build");
  assert.match(run(["rpm"]).stderr, /Unsupported bundles/, "unknown bundles do not invoke Tauri");
  assert.match(run(["deb", "extra"]).stderr, /Usage:/, "extra arguments are rejected");
  for (const target of ["x86_64-unknown-linux-gnu", "aarch64-unknown-linux-gnu"]) {
    const result = run(["deb"], { QA_RUST_TARGET: target });
    assert.equal(result.status, 17, "native build preserves the pnpm failure exit code");
    assert.deepEqual(readFileSync(log, "utf8").trim().split("\n"), ["tauri:build", "--", "--target", target, "--bundles", "deb"]);
    for (const bundles of ["deb", "appimage", "deb,appimage"]) {
      rmSync(outputRoot, { recursive: true, force: true });
      const built = run([bundles], { QA_RUST_TARGET: target, QA_BUILD_SUCCESS: "1" });
      assert.equal(built.status, 0, `${target} ${bundles} finds artifacts with spaces in their names: ${built.stderr}`);
      assert.deepEqual(readFileSync(log, "utf8").trim().split("\n"), ["tauri:build", "--", "--target", target, "--bundles", bundles]);
    }
  }
  for (const [env, message] of [
    [{ QA_ARTIFACT_MODE: "none" }, /did not produce a deb package/],
    [{ QA_ARTIFACT_MODE: "partial" }, /did not produce a appimage package/],
  ]) {
    rmSync(outputRoot, { recursive: true, force: true });
    const result = run([], { QA_BUILD_SUCCESS: "1", ...env });
    assert.equal(result.status, 1); assert.match(result.stderr, message);
  }
  for (const env of [{ QA_FILE_FAILURE: "8" }, { QA_HASH_FAILURE: "9" }]) {
    const result = run(["deb"], { QA_BUILD_SUCCESS: "1", ...env });
    assert.equal(result.status, Number(Object.values(env)[0]), "artifact verification failure cannot report a successful build");
  }
  console.log("PASS Linux native build preflight, package selection, target arguments, artifact discovery and failure propagation");
} finally {
  rmSync(fixture, { recursive: true, force: true });
}
