import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { resolveMacOSProductName } from "./tauri-build-config.mjs";

test("signature verification follows the actual build flavor and config order", () => {
  const directory = mkdtempSync(join(tmpdir(), "reasonix-build-config-"));
  try {
    const write = (name, config) => writeFileSync(join(directory, name), JSON.stringify(config));
    write("tauri.conf.json", { productName: "Reasonix Tauri Preview" });
    write("tauri.takeover.conf.json", { productName: "Reasonix Tauri" });
    assert.equal(resolveMacOSProductName(directory, []), "Reasonix Tauri Preview");
    write("tauri.macos.conf.json", { productName: "Platform" });
    assert.equal(resolveMacOSProductName(directory, []), "Platform");
    for (const args of [["--config", "tauri.takeover.conf.json"], ["-c", join(directory, "tauri.takeover.conf.json")], ["--config=tauri.takeover.conf.json"]]) {
      assert.equal(resolveMacOSProductName(directory, args), "Reasonix Tauri");
    }
    assert.equal(resolveMacOSProductName(directory, ["--config", "tauri.takeover.conf.json", "--config", '{"bundle":{"active":true}}']), "Reasonix Tauri");
    assert.equal(resolveMacOSProductName(directory, ["-c", "tauri.takeover.conf.json", '--config={"productName":"Last flavor"}']), "Last flavor");
    assert.equal(resolveMacOSProductName(directory, ["--", "--config", '{"productName":"Runner argument"}']), "Platform");
    for (const args of [["--config"], ["--config", "--debug"], ["--config", "missing.json"], ["--config", '{"productName":null}'], ["--config", '{"productName":"../Other"}']]) {
      assert.throws(() => resolveMacOSProductName(directory, args));
    }
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
