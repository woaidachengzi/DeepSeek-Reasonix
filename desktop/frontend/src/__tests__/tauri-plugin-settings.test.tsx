import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { LocaleProvider } = await import("../lib/i18n");
const { TauriPluginSettings } = await import("../tauri/TauriPluginSettings");

const globals = globalThis as typeof globalThis & {
  __tauriPluginSettings: { protocolVersion: number; plugins: unknown[] };
  __tauriPluginDoctor: { protocolVersion: number; name: string; compatibility: string; mappedCapabilities: string[]; skippedCapabilities: { capability: string; path: string; reason: string }[]; warnings: string[]; error: string };
  __tauriBridgeCalls: { name: string; args?: { source?: string; name?: string; mode?: string; replace?: boolean; expectedName?: string; expectedRevision?: string; request?: { source?: string; mode?: string; replace?: boolean; expectedName?: string; expectedRevision?: string } } }[];
};
globals.__tauriPluginSettings = { protocolVersion: 1, plugins: [{
  name: "sample", description: "Sample plugin", version: "1.0", source: "remote", updateSource: "https://github.com/acme/sample", root: "/preview/plugins/sample", manifestKind: "claude", enabled: true,
  status: "ready", issue: "", warningCount: 1, skills: 1, agents: 0, commands: 0, hooks: 1, mcpServers: 0, runtime: false, revision: "r1",
}] };
globals.__tauriPluginDoctor = {
  protocolVersion: 1, name: "sample", compatibility: "partial", mappedCapabilities: ["Hooks"],
  skippedCapabilities: [{ capability: "WebFetch hook semantics", path: "hooks/hooks.json", reason: "required prompt input is unavailable" }],
  warnings: ["one unsupported matcher was skipped"], error: "",
};

const root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(<LocaleProvider><TauriPluginSettings /></LocaleProvider>); });
const reviewUpdate = [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.trim() === "Updates");
if (!reviewUpdate) throw new Error("installed plugin update action was not rendered for a copied remote plugin");
await act(async () => { reviewUpdate.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
if (!document.body.textContent?.includes("Replace sample")) throw new Error("update review did not identify the installed plugin");
if (!globals.__tauriBridgeCalls.some(call => call.name === "plan_plugin_install" && call.args?.replace && call.args.expectedName === "sample" && call.args.expectedRevision === "r1")) throw new Error("plugin update plan did not bind the reviewed plugin revision");
const cancelUpdate = [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.trim() === "Cancel");
await act(async () => { cancelUpdate?.click(); });
const sourceInput = document.getElementById("tauri-plugin-source") as HTMLInputElement | null;
if (!sourceInput) throw new Error("plugin source input was not rendered");
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")?.set?.call(sourceInput, "/Users/test/plugins/dev-plugin");
  sourceInput.dispatchEvent(new dom.window.InputEvent("input", { bubbles: true, inputType: "insertText", data: "/Users/test/plugins/dev-plugin" }));
  sourceInput.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
});
const linkInput = [...document.querySelectorAll<HTMLInputElement>("input[type=checkbox]")].find(input => input.closest("label")?.textContent?.includes("Developer mode: link source folder"));
if (!linkInput) throw new Error(`local linked install option was not rendered (input=${sourceInput.value}, labels=${[...document.querySelectorAll("label")].map(label => label.textContent).join(" | ")})`);
await act(async () => { linkInput.click(); });
const preview = [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.trim() === "Preview install");
if (!preview) throw new Error("plugin preview action was not rendered");
await act(async () => { preview.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
if (!globals.__tauriBridgeCalls.some(call => call.name === "plan_plugin_install" && call.args?.mode === "link")) throw new Error("link mode was not included in the reviewed install plan");
const diagnose = [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.trim() === "Doctor");
if (!diagnose) throw new Error("plugin diagnostics action was not rendered");
await act(async () => { diagnose.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
if (!document.body.textContent?.includes("Partially compatible")) throw new Error("compatibility status was not rendered");
if (!document.body.textContent?.includes("Hooks")) throw new Error("mapped capabilities were not rendered");
if (!document.body.textContent?.includes("required prompt input is unavailable")) throw new Error("skipped capability reason was not rendered");
if (!document.body.textContent?.includes("one unsupported matcher was skipped")) throw new Error("runtime or parser warning was not rendered");
if (!globals.__tauriBridgeCalls.some(call => call.name === "plugin_doctor" && call.args?.name === "sample")) throw new Error("diagnostics did not use the authenticated Tauri command");
const install = [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.trim() === "Install reviewed plugins");
if (!install) throw new Error("reviewed plugin install action was not rendered");
await act(async () => { install.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
if (!globals.__tauriBridgeCalls.some(call => call.name === "install_plugin" && call.args?.request?.mode === "link")) throw new Error("install mode was not preserved from review to install");
const updateAgain = [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.trim() === "Updates");
if (!updateAgain) throw new Error("updated plugin entry lost its update action");
await act(async () => { updateAgain.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
const updateInstall = [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.trim() === "Confirm");
if (!updateInstall) throw new Error("reviewed plugin update confirmation was not rendered");
await act(async () => { updateInstall.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
if (!globals.__tauriBridgeCalls.some(call => call.name === "install_plugin" && call.args?.request?.replace && call.args.request.expectedName === "sample" && call.args.request.expectedRevision === "r2")) throw new Error("plugin update confirmation did not preserve the reviewed target revision");
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")?.set?.call(sourceInput, "https://github.com/acme/replacement");
  sourceInput.dispatchEvent(new dom.window.InputEvent("input", { bubbles: true, inputType: "insertText", data: "https://github.com/acme/replacement" }));
  sourceInput.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
});
const replaceToggle = [...document.querySelectorAll<HTMLInputElement>("input[type=checkbox]")].find(input => input.closest("label")?.textContent?.includes("Overwrite same-name plugin"));
if (!replaceToggle) throw new Error("same-name plugin replacement option was not rendered");
await act(async () => { replaceToggle.click(); });
const replacementSelect = document.querySelector<HTMLSelectElement>(".tauri-plugin-replace-target select");
if (replacementSelect?.value !== "sample") throw new Error("replacement target was not selected from the installed Preview plugins");
const previewReplacement = [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.trim() === "Preview install");
await act(async () => { previewReplacement?.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
if (!globals.__tauriBridgeCalls.some(call => call.name === "plan_plugin_install" && call.args?.source === "https://github.com/acme/replacement" && call.args.mode === "copy" && call.args.replace && call.args.expectedName === "sample" && call.args.expectedRevision === "r2")) throw new Error("same-name replacement review was not bound to the installed plugin revision");
const confirmReplacement = [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.trim() === "Confirm");
await act(async () => { confirmReplacement?.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
if (!globals.__tauriBridgeCalls.some(call => call.name === "install_plugin" && call.args?.request?.source === "https://github.com/acme/replacement" && call.args.request.replace && call.args.request.expectedName === "sample" && call.args.request.expectedRevision === "r2")) throw new Error(`same-name replacement confirmation did not preserve its source and revision guard: ${JSON.stringify(globals.__tauriBridgeCalls.slice(-5))}`);
await act(async () => { root.unmount(); });
process.stdout.write("tauri plugin settings diagnostics: OK\n");
