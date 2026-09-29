import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, Event: dom.window.Event, MouseEvent: dom.window.MouseEvent, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });

let passed = 0;
let failed = 0;
function ok(value: unknown, label: string) {
  if (value) { process.stdout.write(`  PASS  ${label}\n`); passed += 1; }
  else { process.stdout.write(`  FAIL  ${label}\n`); failed += 1; }
}
const settle = () => new Promise<void>(resolve => setTimeout(resolve, 0));
function typeInto(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const prototype = input instanceof dom.window.HTMLTextAreaElement ? dom.window.HTMLTextAreaElement.prototype : dom.window.HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(prototype, "value")?.set?.call(input, value);
  input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  input.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
}
async function main() {
  const React = await import("react");
  const { act } = React;
  const { createRoot } = await import("react-dom/client");
  const { LocaleProvider } = await import("../lib/i18n");
  const { TauriSubagentSettings } = await import("../tauri/TauriSubagentSettings");
  const usedCommands: string[] = [];
  (globalThis as typeof globalThis & { __subagentProfiles?: unknown[] }).__subagentProfiles = [
    { name: "review", description: "Read only review", scope: "builtin", invocation: "/review", configuredModel: "", configuredEffort: "", invocationMode: "manual", allowedTools: [] },
    { name: "security-review", description: "Security review", scope: "builtin", invocation: "/security-review", configuredModel: "demo/override", configuredEffort: "low", invocationMode: "manual", allowedTools: [] },
    { name: "project-check", description: "Project check", scope: "project", invocation: "/project-check", invocationMode: "manual", editable: true, revision: "rev-1", allowedTools: ["read_file"] },
    { name: "external-review", description: "External profile", scope: "custom", invocation: "/external-review", invocationMode: "manual", readOnly: true },
  ];
  const root = createRoot(document.getElementById("root")!);
  await act(async () => { root.render(React.createElement(LocaleProvider, null, React.createElement(TauriSubagentSettings, { workspaceRoot: "/tmp/preview-project", onUseInChat: command => usedCommands.push(command) }))); await settle(); });
  ok(Boolean(document.querySelector(".tauri-subagent-profile-group")) && document.querySelectorAll(".tauri-subagent-profile").length === 4, "discovered profiles are grouped and rendered even when optional tool metadata is missing");
  const builtinEffectiveValues = [...document.querySelectorAll(".tauri-subagent-profile--builtin .tauri-subagent-effective-value")].map(node => node.textContent?.trim() ?? "");
  ok(builtinEffectiveValues.includes("Effective: demo/m") && builtinEffectiveValues.includes("Effective: demo/override") && builtinEffectiveValues.includes("Effective: auto"), "built-in model and effort selectors show inherited and overridden effective values");
  await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-subagent-profile--custom .tauri-subagent-invocation button:last-child")?.click(); await settle(); });
  ok(usedCommands[0] === "/project-check ", "Use in chat returns the slash command with a trailing task space without submitting it");
  ok(!((globalThis as typeof globalThis & { __tauriBridgeCalls: { name: string }[] }).__tauriBridgeCalls).some(call => call.name === "submit"), "using a profile does not submit a chat turn");
  await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-subagent-toolbar button")?.click(); await settle(); });

  const editor = document.querySelector(".tauri-subagent-editor");
  if (!editor) throw new Error("subagent editor did not open");
  const inputs = editor.querySelectorAll<HTMLInputElement>("input");
  const textareas = editor.querySelectorAll<HTMLTextAreaElement>("textarea");
  const task = document.querySelector<HTMLInputElement>("#tauri-subagent-try-task");
  await act(async () => {
    if (inputs[0]) typeInto(inputs[0], "unsaved-reviewer");
    if (inputs[1]) typeInto(inputs[1], "Inspect one file safely");
    if (textareas[0]) typeInto(textareas[0], "Review the provided task. Never modify files.");
    if (textareas[1]) typeInto(textareas[1], "read_file, search");
    if (task) typeInto(task, "Summarize the current project state");
    await settle();
  });
  await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-subagent-try-row button")?.click(); await settle(); });
  const calls = (globalThis as typeof globalThis & { __tauriBridgeCalls: { name: string; args?: Record<string, unknown> }[] }).__tauriBridgeCalls;
  const runCall = calls.find(call => call.name === "try_subagent_profile");
  const runInput = runCall?.args?.input as { name?: string; systemPrompt?: string; allowedTools?: string[] } | undefined;
  ok(runCall?.args?.workspaceRoot === "/tmp/preview-project" && runCall.args.task === "Summarize the current project state", "try request uses the active project and one-off task");
  ok(runInput?.name === "unsaved-reviewer" && runInput.systemPrompt === "Review the provided task. Never modify files." && runInput.allowedTools?.join(",") === "read_file,search", "try request uses the unsaved profile form values");
  ok(!calls.some(call => call.name === "change_subagent_settings"), "trying the profile does not persist the draft");
  ok(document.body.textContent?.includes("Preview result"), "successful try output is shown in the editor");

  let rejectTry: ((reason: Error) => void) | undefined;
  const globals = globalThis as typeof globalThis & { __subagentTryPending?: () => Promise<string>; __subagentTryCancel?: () => void; __subagentTryStatus?: { protocolVersion: number; running: boolean; output: string } };
  globals.__subagentTryPending = () => new Promise<string>((_resolve, reject) => { rejectTry = reject; });
  globals.__subagentTryCancel = () => rejectTry?.(new Error("subagent try run was cancelled"));
  globals.__subagentTryStatus = { protocolVersion: 1, running: true, output: "Live partial response" };
  await act(async () => { if (task) typeInto(task, "Long running task"); await settle(); });
  await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-subagent-try-row button")?.click(); await settle(); });
  ok(document.body.textContent?.includes("Live partial response"), "try output updates while the isolated subagent is still running");
  await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-subagent-try-row button")?.click(); await settle(); });
  ok(calls.some(call => call.name === "cancel_subagent_profile_try"), "cancel button reaches the independent bridge cancellation request");
  ok(calls.some(call => call.name === "subagent_profile_try_status"), "live output polls the authenticated Preview bridge status");
  ok(!document.querySelector('[role="alert"]')?.textContent?.includes("cancelled"), "expected cancellation is not rendered as a run error");
  globals.__subagentTryPending = undefined;
  globals.__subagentTryCancel = undefined;
  globals.__subagentTryStatus = undefined;

  await act(async () => { root.unmount(); });
  process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
  if (failed > 0) process.exitCode = 1;
}

await main();
