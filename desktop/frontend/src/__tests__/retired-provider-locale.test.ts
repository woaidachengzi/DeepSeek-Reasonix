import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { en } from "../locales/en";
import { zh } from "../locales/zh";
import { zhTW } from "../locales/zh-TW";

// The replaced provider editor has no callers for these captions. Do not trim
// live text or dynamic families (preset descriptions, pageDesc, notification events).
const retired = [
  "customProviders", "customProvidersHint", "customProvidersEmpty",
  "providerNamePlaceholder", "models.saveKeyFirst", "providerApiKeyEnvHint",
  "providerKeyOptional", "providerKeyPlaceholder", "testFetchModels",
  "testFetchModelsHint", "manualModels", "manualModelsHint", "visionModels",
  "visionModelsHint", "fetchModelsUpdatedForProvider", "fetchModelsAfterKeyFailed",
  "fetchModelsAfterKeyFailedForProvider", "updateKey", "updateKeyAction",
  "keyStatus", "configuredKey", "notConfiguredKey", "keySource",
].map(key => `settings.${key}`);

const keys = Object.keys(en).sort();
for (const dict of [en, zh, zhTW]) {
  assert.deepEqual(Object.keys(dict).sort(), keys, "all three locales retain identical keys");
  for (const key of retired) assert.equal(Object.prototype.hasOwnProperty.call(dict, key), false, `retired caption ${key}`);
  for (const key of ["settings.providerKey", "settings.providerRequestUrlHint", "settings.models.keySaveFailed", "settings.fetchModelsManualFallbackGeneric"] as const) {
    assert.ok(dict[key], `live provider recovery text ${key} stays available`);
    const placeholders = (value: string) => [...value.matchAll(/\{(\w+)\}/g)].map(match => match[1]).sort();
    assert.deepEqual(placeholders(dict[key]), placeholders(en[key]), `live placeholders ${key}`);
  }
}

const self = fileURLToPath(import.meta.url);
function verifyNoCallers(dir: string): void {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name !== "locales") verifyNoCallers(path);
    } else if (/\.tsx?$/.test(entry.name) && path !== self) {
      const source = readFileSync(path, "utf8");
      for (const key of retired) assert.equal(source.includes(key), false, `retired caption referenced in ${path}: ${key}`);
    }
  }
}
verifyNoCallers(resolve(dirname(self), ".."));
console.log("  PASS  retired provider captions have no callers; locale parity and live recovery copy retained");
