// Loader hook for tsx-based component tests: replaces the Tauri adapter with a
// stub so a component can mount under jsdom without a real Tauri host. The stub
// records calls, which is what lets a test assert the UI reached the bridge.
//
// Keep this list in step with the adapter functions the mounted component uses.
export async function load(url, context, nextLoad) {
  if (url.endsWith("/lib/tauriBridge.ts")) {
    return {
      format: "module",
      shortCircuit: true,
      source: STUB_SOURCE,
    };
  }
  return nextLoad(url, context);
}

const STUB_SOURCE = `
const calls = [];
globalThis.__tauriBridgeCalls = calls;

function record(name, args) {
  calls.push({ name, args });
}

export function tauriBridgeCalls() {
  return calls;
}

export const TAURI_TITLE_MAX_CHARS = 120;

export function tauriTitleError(title) {
  const trimmed = title.trim();
  if (!trimmed) return "对话名称不能为空";
  if (Array.from(trimmed).length > TAURI_TITLE_MAX_CHARS) {
    return \`对话名称不能超过 \${TAURI_TITLE_MAX_CHARS} 个字符\`;
  }
  if (/\\p{Cc}/u.test(trimmed)) return "对话名称不能包含控制字符";
  return "";
}

export function tauriSessionTitle(storedTitle, fallback) {
  const trimmed = (storedTitle ?? "").trim();
  return tauriTitleError(trimmed) === "" ? trimmed : fallback;
}

export function tauriTurnFailure(event) {
  if (event.eventKind !== "turn_done") return "";
  if (event.payload.status !== "failed") return "";
  const reason = typeof event.payload.err === "string" ? event.payload.err.trim() : "";
  return reason || "本轮未完成：Agent 没有返回结果";
}

export function tauriMessageFrom(error) {
  return error instanceof Error ? error.message : String(error);
}

export function isTauriDesktop() { return true; }

export function newTauriSessionId() { return "tauri-stub-session"; }

export function tauriBridgeStatus() { record("bridge_status"); return Promise.resolve({ running: true, protocolVersion: 1 }); }
export function restartTauriBridge() { record("restart_bridge"); return Promise.resolve({ running: true, protocolVersion: 1 }); }
export function previewProfileStatus() { return Promise.resolve({ previewHome: "/tmp", previewConfigExists: true, stableConfigExists: false, importAvailable: false, managedProfile: true }); }
export function tauriPreviewProfileStatus() { return previewProfileStatus(); }
export function tauriPreviewRuntimeInfo() { return Promise.resolve({ stableVersion: "1.38.3", stableCommit: "test", previewVersion: "0.1.0", tauriVersion: "2", previewBuild: "test-build", bridgeProtocolVersion: 1 }); }
export function tauriProviderSummary() { return Promise.resolve({ protocolVersion: 1, providers: [] }); }
export function setTauriDefaultModel() { return Promise.resolve({ protocolVersion: 1, providers: [] }); }
export function importTauriStableProfile() { return Promise.resolve({ importedConfig: "", backupConfig: "" }); }
export function chooseTauriWorkspaceRoot() { return Promise.resolve(null); }
export function chooseTauriAttachmentFiles() { return Promise.resolve([]); }

export function tauriWorkbenchSessions() {
  record("workbench_sessions");
  return Promise.resolve((globalThis.__workbenchSessions ?? []).slice());
}

export function rememberTauriWorkbenchSession(sessionId, workspaceRoot, title) {
  record("remember_workbench_session", { sessionId, workspaceRoot, title });
  const existing = (globalThis.__workbenchSessions ?? []).find(entry => entry.sessionId === sessionId);
  const next = (globalThis.__workbenchSessions ?? []).filter(entry => entry.sessionId !== sessionId);
  next.push({ sessionId, workspaceRoot, title: title ?? existing?.title });
  globalThis.__workbenchSessions = next;
  return Promise.resolve(next.slice());
}

export function forgetTauriWorkbenchSession(sessionId) {
  record("forget_workbench_session", { sessionId });
  globalThis.__workbenchSessions = (globalThis.__workbenchSessions ?? []).filter(entry => entry.sessionId !== sessionId);
  return Promise.resolve(globalThis.__workbenchSessions.slice());
}

function storedTitle(sessionId) {
  return (globalThis.__workbenchSessions ?? []).find(entry => entry.sessionId === sessionId)?.title;
}

export function openTauriBridgeSession(sessionId, workspaceRoot) {
  record("bridge_open_session", { sessionId, workspaceRoot });
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle", title: storedTitle(sessionId), workspaceRoot });
}

export function switchTauriBridgeSession(sessionId, workspaceRoot) {
  record("bridge_switch_session", { sessionId, workspaceRoot });
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle", title: storedTitle(sessionId), workspaceRoot });
}

export function renameTauriBridgeSession(sessionId, title) {
  record("bridge_rename_session", { sessionId, title });
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle", title });
}

export function deleteTauriBridgeSession(sessionId) {
  record("bridge_delete_session", { sessionId });
  const failure = globalThis.__deleteFailure;
  if (failure) return Promise.reject(new Error(failure));
  return Promise.resolve({ protocolVersion: 1, deleted: true, sessionId });
}

export function tauriBridgeSnapshot(sessionId) {
  return Promise.resolve({ sequence: 0, session: { id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle" } });
}

export function tauriBridgeHistory(sessionId) {
  return Promise.resolve({ sequence: 0, session: { id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle" }, messages: [], startIndex: 0, totalMessages: 0 });
}

export function submitTauriBridge(sessionId, input) {
  record("bridge_submit", { sessionId, input });
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "running" });
}

export function attachTauriFile(sessionId, path) {
  record("bridge_attach_file", { sessionId, path });
  return Promise.resolve({ path: ".reasonix/attachments/a.txt", name: "a.txt", size: 1, isImage: false });
}

export function cancelTauriBridge(sessionId) {
  record("bridge_cancel", { sessionId });
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle" });
}

export function startTauriBridgeEvents() { record("bridge_start_events"); return Promise.resolve(); }
export function onTauriBridgeEvent() { return Promise.resolve(() => {}); }
export function onTauriBridgeConnectionError() { return Promise.resolve(() => {}); }

export function tauriAssistantTextDelta() { return ""; }
export function tauriComposerInput(prompt, attachments) { return prompt.trim(); }
export function tauriEventSummary(event) { return JSON.stringify(event.payload); }
`;
