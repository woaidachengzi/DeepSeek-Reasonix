import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";
import type { ThemePackView } from "../lib/themePack";
import { TauriThemeGallery } from "../tauri/TauriThemeGallery";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  HTMLElement: dom.window.HTMLElement,
  Event: dom.window.Event,
  MouseEvent: dom.window.MouseEvent,
  IS_REACT_ACT_ENVIRONMENT: true,
});
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });

const pluginTheme: ThemePackView = {
  id: "plugin:sample:neon-night",
  name: "Neon Night",
  pluginName: "sample",
  baseStyle: "graphite",
  builtin: false,
  kind: "plugin",
  active: false,
  hasBackground: false,
  tokens: { dark: { accent: "#8b5cf6" } },
  recipes: { density: "comfortable", corners: "soft" },
};
let applied = "";
const root = createRoot(document.getElementById("root")!);
await act(async () => {
  root.render(<LocaleProvider><TauriThemeGallery
    mode="dark"
    baseStyle="graphite"
    activeThemeId=""
    userThemes={[pluginTheme]}
    error=""
    onBack={() => undefined}
    onApply={async id => { applied = id; }}
    onSaveTheme={async () => null}
    onDeleteTheme={async () => false}
    onImportTheme={async () => null}
    onExportTheme={async () => true}
  /></LocaleProvider>);
});

const card = [...document.querySelectorAll<HTMLButtonElement>("[role=option]")].find(button => button.textContent?.includes("Neon Night"));
if (!card) throw new Error("plugin theme card was not rendered");
await act(async () => { card.click(); });
if (![...document.querySelectorAll<HTMLElement>("[role=group]")].some(group => /plugin|插件/i.test(group.getAttribute("aria-label") ?? "") && group.textContent?.includes("Neon Night"))) throw new Error("plugin theme was not grouped in the gallery");
if (document.querySelector(".theme-gallery__detail .btn--secondary")) {
  throw new Error("plugin themes must remain read-only");
}
await act(async () => { document.querySelector<HTMLButtonElement>(".theme-gallery__apply")?.click(); });
if (applied !== pluginTheme.id) throw new Error(`plugin theme apply sent ${applied || "no id"}`);
await act(async () => { root.unmount(); });
process.stdout.write("tauri plugin themes: OK\n");
