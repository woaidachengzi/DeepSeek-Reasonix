// Run: node --import ./scripts/tauri-bridge-stub-register.mjs --import tsx src/__tests__/tauri-remote-settings.test.tsx

import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", {
  url: "http://localhost/",
  pretendToBeVisual: true,
});
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  HTMLElement: dom.window.HTMLElement,
  Event: dom.window.Event,
  MouseEvent: dom.window.MouseEvent,
  IS_REACT_ACT_ENVIRONMENT: true,
});
dom.window.confirm = () => true;
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });

let passed = 0;
let failed = 0;
function ok(value: unknown, label: string) {
  if (value) { process.stdout.write(`  PASS  ${label}\n`); passed += 1; }
  else { process.stdout.write(`  FAIL  ${label}\n`); failed += 1; }
}
function settle() { return new Promise<void>(resolve => setTimeout(resolve, 0)); }
function click(label: string) {
  const button = [...document.querySelectorAll<HTMLButtonElement>("button")].find(item => item.textContent?.trim() === label);
  button?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
  return Boolean(button);
}
function typeInto(input: HTMLInputElement, value: string) {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")?.set?.call(input, value);
  input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
}

async function main() {
  (globalThis as typeof globalThis & { __remoteHosts?: unknown[] }).__remoteHosts = [];
  (globalThis as typeof globalThis & { __remoteSSHAliases?: string[] }).__remoteSSHAliases = ["gpu"];
  const React = await import("react");
  const { act } = React;
  const { createRoot } = await import("react-dom/client");
  const { LocaleProvider } = await import("../lib/i18n");
  const { TauriRemoteSettings } = await import("../tauri/TauriRemoteSettings");
  const root = createRoot(document.getElementById("root")!);
  await act(async () => { root.render(React.createElement(LocaleProvider, null, React.createElement(TauriRemoteSettings))); await settle(); });
  ok(document.body.textContent?.includes("No remote hosts configured"), "empty host list is visible");
  await act(async () => { click("Scan SSH config"); await settle(); });
  ok(document.body.textContent?.includes("gpu"), "SSH alias scan returns importable aliases");
  await act(async () => { click("gpu"); await settle(); });
  const name = document.querySelector<HTMLInputElement>("#remote-host-name");
  const address = document.querySelector<HTMLInputElement>("#remote-host-address");
  ok(name?.value === "gpu" && address?.value === "gpu", "importing an alias pre-fills its SSH name and address");
  const password = document.querySelector<HTMLInputElement>('[aria-label="SSH password"]');
  const passphrase = document.querySelector<HTMLInputElement>('[aria-label="Private-key passphrase"]');
  if (password) typeInto(password, "private-password");
  if (passphrase) typeInto(passphrase, "private-passphrase");
  await act(async () => { click("Save"); await settle(); await settle(); });
  const calls = (globalThis as typeof globalThis & {
    __tauriBridgeCalls: { name: string; args: { change?: { host?: { name?: string; host?: string; workspace?: string; useSSHConfig?: boolean } }; request?: { trustFingerprint?: string; password?: string; passphrase?: string } } }[];
  }).__tauriBridgeCalls;
  const save = calls.find(call => call.name === "change_remote_settings")?.args.change?.host;
  ok(save?.name === "gpu" && save.host === "gpu" && save.useSSHConfig === true, "saving the imported alias persists the SSH-config flag");
  ok(save?.passwordAction === "replace" && save.password === "private-password" && save.passphraseAction === "replace" && save.passphrase === "private-passphrase", "new SSH credentials are sent only in the save request");
  ok(!document.body.textContent?.includes("private-password") && !document.body.textContent?.includes("private-passphrase"), "credential text is never rendered back into the settings page");
  ok(document.body.textContent?.includes("gpu"), "saved host appears in the list");
  (globalThis as typeof globalThis & { __remoteConnectResults?: unknown[] }).__remoteConnectResults = [
    { protocolVersion: 1, status: "host_key_confirmation", host: "gpu", address: "gpu.example:22", keyType: "ssh-ed25519", fingerprint: "SHA256:trusted-test" },
    { protocolVersion: 1, status: "connected", host: "gpu", fingerprint: "SHA256:trusted-test" },
  ];
  await act(async () => { click("Connect"); await settle(); await settle(); });
  ok(document.body.textContent?.includes("SHA256:trusted-test") && document.body.textContent?.includes("Trust fingerprint and connect"), "first connection requires explicit host-key confirmation");
  await act(async () => { click("Trust fingerprint and connect"); await settle(); await settle(); });
  const connectCalls = calls.filter(call => call.name === "connect_remote_host");
  ok(connectCalls.length === 2 && connectCalls[1]?.args.request?.trustFingerprint === "SHA256:trusted-test", "host key is trusted only after confirming the displayed fingerprint");
  ok(document.body.textContent?.includes("Connection verified"), "verified connection status is shown on the host card");
  await act(async () => { click("Disconnect"); await settle(); });
  (globalThis as typeof globalThis & { __remoteConnectResults?: unknown[] }).__remoteConnectResults = [
    { protocolVersion: 1, status: "failed", message: "authentication_failed" },
    { protocolVersion: 1, status: "connected", host: "gpu", fingerprint: "SHA256:trusted-test" },
  ];
  await act(async () => { click("Connect"); await settle(); });
  ok(document.querySelector(".tauri-remote-credential-prompt") !== null, "authentication failure offers an explicit temporary credential prompt");
  const passwordPrompt = document.querySelector<HTMLInputElement>("#remote-password-gpu");
  const passphrasePrompt = document.querySelector<HTMLInputElement>("#remote-passphrase-gpu");
  if (passwordPrompt) typeInto(passwordPrompt, "one-time-password");
  if (passphrasePrompt) typeInto(passphrasePrompt, "one-time-passphrase");
  const configWritesBeforeCredentialRetry = calls.filter(call => call.name === "change_remote_settings").length;
  await act(async () => { click("Retry with credentials"); await settle(); await settle(); });
  const finalConnectCall = calls.filter(call => call.name === "connect_remote_host").at(-1);
  ok(finalConnectCall?.args.request?.password === "one-time-password" && finalConnectCall.args.request.passphrase === "one-time-passphrase", "temporary password and key passphrase are sent only with the retry request");
  ok(calls.filter(call => call.name === "change_remote_settings").length === configWritesBeforeCredentialRetry, "temporary SSH credentials are not written to host settings");
  ok(!document.querySelector("#remote-password-gpu") && document.body.textContent?.includes("Connection verified"), "successful retry clears the temporary credential form");
  (globalThis as typeof globalThis & { __remoteBrowseResults?: unknown[] }).__remoteBrowseResults = [
    { protocolVersion: 1, path: "/srv/workspace", parentPath: "/srv", entries: [{ name: "project", path: "/srv/workspace/project", isDir: true, symlink: false, size: 0, modTime: 0 }, { name: "README.md", path: "/srv/workspace/README.md", isDir: false, symlink: false, size: 12, modTime: 0 }], truncated: false },
    { protocolVersion: 1, path: "/srv/workspace/project", parentPath: "/srv/workspace", entries: [], truncated: false },
  ];
  (globalThis as typeof globalThis & { __remotePreviewResults?: unknown[] }).__remotePreviewResults = [
    { protocolVersion: 1, path: "/srv/workspace/README.md", kind: "text", content: "# Remote project\nPreview text", revision: "original-revision", truncated: false },
  ];
  await act(async () => { click("Edit"); await settle(); });
  await act(async () => { click("Browse folders"); await settle(); await settle(); });
  ok(document.body.textContent?.includes("README.md") && document.body.textContent?.includes("project"), "remote SFTP browser shows files and navigable folders");
  const remoteFile = document.querySelector<HTMLButtonElement>(".tauri-remote-browser-entry--file");
  await act(async () => { remoteFile?.click(); await settle(); });
  ok(document.body.textContent?.includes("# Remote project") && document.body.textContent?.includes("Text file"), "selecting a remote text file shows its bounded preview");
  const previewCall = (globalThis as typeof globalThis & { __tauriBridgeCalls: { name: string; args: { name?: string; path?: string } }[] }).__tauriBridgeCalls.find(call => call.name === "preview_remote_file");
  ok(previewCall?.args.name === "gpu" && previewCall.args.path === "/srv/workspace/README.md", "remote preview reads only the selected file from the connected host");
  const remoteEditor = document.querySelector<HTMLTextAreaElement>(".tauri-remote-file-preview textarea");
  if (remoteEditor) {
    Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")?.set?.call(remoteEditor, "# Updated remotely\nSaved text");
    remoteEditor.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  }
  await act(async () => { click("Save remote file"); await settle(); });
  const saveCall = (globalThis as typeof globalThis & { __tauriBridgeCalls: { name: string; args: { request?: { name?: string; path?: string; revision?: string; content?: string } } }[] }).__tauriBridgeCalls.find(call => call.name === "save_remote_file");
  ok(saveCall?.args.request?.name === "gpu" && saveCall.args.request.path === "/srv/workspace/README.md" && saveCall.args.request.revision === "original-revision" && saveCall.args.request.content === "# Updated remotely\nSaved text", "saving sends the original revision and edited text to the connected host");
  ok(document.body.textContent?.includes("Remote file saved."), "successful remote save is confirmed");
  const savedEditor = document.querySelector<HTMLTextAreaElement>(".tauri-remote-file-preview textarea");
  (globalThis as typeof globalThis & { __remoteSaveError?: Error }).__remoteSaveError = new Error("remote_file_changed");
  if (savedEditor) {
    Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")?.set?.call(savedEditor, "unsaved draft");
    savedEditor.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  }
  await act(async () => { click("Save remote file"); await settle(); });
  ok(document.body.textContent?.includes("file may have changed") && document.querySelector<HTMLTextAreaElement>(".tauri-remote-file-preview textarea")?.value === "unsaved draft", "a conflict keeps the user's unsaved draft and asks them to reload");
  delete (globalThis as typeof globalThis & { __remoteSaveError?: Error }).__remoteSaveError;
  let discardPrompts = 0;
  dom.window.confirm = () => { discardPrompts += 1; return false; };
  await act(async () => { click("Browse folders"); await settle(); });
  ok(discardPrompts === 1 && document.querySelector<HTMLTextAreaElement>(".tauri-remote-file-preview textarea")?.value === "unsaved draft" && document.body.textContent?.includes("/srv/workspace/README.md"), "declining a browser switch preserves the unsaved remote draft");
  dom.window.confirm = () => true;
  const projectFolder = document.querySelector<HTMLButtonElement>(".tauri-remote-browser-entry:not(.tauri-remote-browser-entry--file)");
  await act(async () => { projectFolder?.click(); await settle(); await settle(); });
  ok(document.body.textContent?.includes("/srv/workspace/project"), "remote browser navigates into a selected folder");
  await act(async () => { click("Choose this folder"); await settle(); });
  ok(document.querySelector<HTMLInputElement>("#remote-host-workspace")?.value === "/srv/workspace/project", "choosing a remote folder fills the host workspace field");
  await act(async () => { click("Save"); await settle(); await settle(); });
  const latestSave = calls.filter(call => call.name === "change_remote_settings").at(-1)?.args.change?.host;
  ok(latestSave?.workspace === "/srv/workspace/project", "selected remote workspace is saved to the host settings");
  await act(async () => { click("Disconnect"); await settle(); await settle(); });
  ok(calls.some(call => call.name === "disconnect_remote_host") && document.body.textContent?.includes("Connect"), "disconnect releases the live remote session and restores the connect action");
  await act(async () => { click("Delete"); await settle(); });
  ok(document.querySelector('[role="alertdialog"]') !== null, "removing a host requires confirmation");
  await act(async () => { click("Cancel"); await settle(); });
  ok(document.body.textContent?.includes("gpu") && !document.querySelector('[role="alertdialog"]'), "cancel leaves the host configured");
  await act(async () => { click("Delete"); await settle(); });
  const dialogDelete = document.querySelector<HTMLButtonElement>("[role=alertdialog] button:last-child");
  await act(async () => { dialogDelete?.click(); await settle(); await settle(); });
  ok(document.querySelectorAll(".tauri-remote-host-card").length === 0, "confirmed removal deletes the saved host");
  await act(async () => { root.unmount(); });
  process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
  if (failed > 0) process.exitCode = 1;
}

await main();
