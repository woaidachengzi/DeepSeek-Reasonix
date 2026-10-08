import assert from "node:assert/strict";
import { tauriMessageFrom } from "../lib/tauriBridge";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { LocaleProvider, preloadLocale } from "../lib/i18n";

const cases = [
  [404, "workspace_file_not_found", /未找到.*完整相对路径/],
  [409, "workspace_file_ambiguous", /多个同名文件.*完整相对路径/],
  [422, "workspace_file_unavailable", /无法查找或读取.*重试/],
] as const;
Object.defineProperty(globalThis, "navigator", { value: { language: "zh-CN" }, configurable: true });
await preloadLocale("zh");
renderToStaticMarkup(createElement(LocaleProvider, { children: null }));
for (const [status, code, expected] of cases) {
  const error = `desktop bridge request failed with status ${status} (${code})`;
  assert.match(tauriMessageFrom(error), expected);
  assert.match(tauriMessageFrom(new Error(error)), expected);
}
assert.equal(tauriMessageFrom("desktop bridge request failed with status 500"), "desktop bridge request failed with status 500", "unknown errors retain their diagnostics");
console.log("workspace reference errors: localized recovery hints OK");
