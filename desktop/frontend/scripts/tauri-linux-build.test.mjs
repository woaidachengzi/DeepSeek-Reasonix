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
  // CI requires a real sidecar instead of silently skipping lifecycle tests.
  const workflow = readFileSync(resolve(dirname(sourceScript), "../../../.github/workflows/tauri-linux-preview.yml"), "utf8");
  const nativeStep = workflow.match(/      - name: Run native Linux Rust regression tests\n([\s\S]*?)(?=      - name:|$)/)?.[1];
  assert.ok(nativeStep, "Linux preview workflow has a native regression step");
  const bridgePath = nativeStep.match(/REASONIX_TAURI_BRIDGE_TEST_BIN: (.+)/)?.[1];
  assert.equal(bridgePath, "${{ github.workspace }}/desktop/tauri/binaries/reasonix-desktop-bridge-x86_64-unknown-linux-gnu", "CI tests use the x64 sidecar produced by packaging");
  const nativeRun = nativeStep.match(/        run: \|\n([\s\S]*)/)?.[1].replace(/^          /gm, "");
  assert.ok(nativeRun, "native test step includes an executable-sidecar preflight");
  assert.match(nativeRun, /^test -x "\$REASONIX_TAURI_BRIDGE_TEST_BIN"$/m, "CI rejects a missing or non-executable sidecar before Cargo runs");
  for (const [tool, body] of [
    ["xvfb-run", '[[ "$1" == -a ]] || exit 21\nshift\nexec "$@"'],
    ["dbus-run-session", '[[ "$1" == -- ]] || exit 22\nshift\nexec "$@"'],
    ["cargo", '[[ "$CI" == true && -x "$REASONIX_TAURI_BRIDGE_TEST_BIN" ]] || exit 23\nprintf "%s\\n" "$@" > "$QA_BUILD_LOG"'],
  ]) {
    writeFileSync(join(fixture, tool), `#!/bin/bash\n${body}\n`);
    chmodSync(join(fixture, tool), 0o755);
  }
  const fixtureBridge = bridgePath.replace("${{ github.workspace }}", fixture);
  mkdirSync(dirname(fixtureBridge), { recursive: true });
  const runNative = () => spawnSync("bash", ["-e", "-c", nativeRun], {
    env: { ...process.env, PATH: `${fixture}:${process.env.PATH}`, CI: "true", REASONIX_TAURI_BRIDGE_TEST_BIN: fixtureBridge, QA_BUILD_LOG: log }, encoding: "utf8",
  });
  assert.notEqual(runNative().status, 0, "missing built sidecar cannot pass the native CI step");
  writeFileSync(fixtureBridge, "#!/bin/bash\nexit 0\n"); chmodSync(fixtureBridge, 0o755);
  assert.equal(runNative().status, 0, "native CI step forwards the real sidecar through Xvfb and D-Bus");
  assert.deepEqual(readFileSync(log, "utf8").trim().split("\n"), ["test", "--locked", "--manifest-path", "desktop/tauri/Cargo.toml", "--bin", "reasonix-tauri"]);
  console.log("PASS Linux native build preflight, package selection, target arguments, artifact discovery and failure propagation");
  console.log("PASS Linux preview CI sidecar selection, missing-binary preflight and native test arguments");
} finally {
  rmSync(fixture, { recursive: true, force: true });
}
