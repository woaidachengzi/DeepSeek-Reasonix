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
export function keychainSave(key, value) { record("keychain_save", { key, value }); return Promise.resolve(); }
export function keychainLoad(key) { record("keychain_load", { key }); return Promise.resolve(null); }
export function keychainDelete(key) { record("keychain_delete", { key }); return Promise.resolve(true); }
export function importTauriStableProfile() { return Promise.resolve({ importedConfig: "", backupConfig: "" }); }
export function chooseTauriWorkspaceRoot() { return Promise.resolve(null); }
export function chooseTauriAttachmentFiles() { return Promise.resolve([]); }

export function tauriWorkbenchSessions() {
  record("workbench_sessions");
  return Promise.resolve((globalThis.__workbenchSessions ?? []).slice());
}

export function tauriSessionPreviews(sessionIds) {
  record("bridge_session_previews", { sessionIds });
  return Promise.resolve(sessionIds.map(sessionId => ({ sessionId, firstUser: globalThis.__previewFirstUsers?.[sessionId] ?? "" })));
}

export function backfillTauriWorkbenchTitles(titles) {
  record("backfill_workbench_titles", { titles });
  const byId = new Map(titles.map(item => [item.sessionId, item.title]));
  globalThis.__workbenchSessions = (globalThis.__workbenchSessions ?? []).map(entry => ({ ...entry, title: entry.title ?? byId.get(entry.sessionId) }));
  return Promise.resolve(globalThis.__workbenchSessions.slice());
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
  record("bridge_session_snapshot", { sessionId });
  if (globalThis.__snapshotError) return Promise.reject(new Error("snapshot unavailable"));
  const state = globalThis.__snapshotStates?.shift() ?? globalThis.__snapshotState ?? "idle";
  const result = { sequence: globalThis.__snapshotSequence ?? 0, session: { id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state } };
  return globalThis.__snapshotGate ? globalThis.__snapshotGate.then(() => result) : Promise.resolve(result);
}

export function tauriBridgeHistory(sessionId) {
  const messages = globalThis.__tauriHistoryMessages ?? [];
  const result = { sequence: 0, session: { id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle" }, messages, startIndex: 0, totalMessages: messages.length };
  return globalThis.__historyGate ? globalThis.__historyGate.then(() => result) : Promise.resolve(result);
}

export function submitTauriBridge(sessionId, input) {
  record("bridge_submit", { sessionId, input });
  const result = { id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "running" };
  return globalThis.__submitGate ? globalThis.__submitGate.then(() => result) : Promise.resolve(result);
}

export function attachTauriFile(sessionId, path) {
  record("bridge_attach_file", { sessionId, path });
  return Promise.resolve({ path: ".reasonix/attachments/a.txt", name: "a.txt", size: 1, isImage: false });
}

export function cancelTauriBridge(sessionId) {
  record("bridge_cancel", { sessionId });
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle" });
}

export function approveTauriBridge(sessionId, id, allow) {
  record("bridge_approve", { sessionId, id, allow });
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "running" });
}

export function answerTauriQuestion(sessionId, id, answers) {
  record("bridge_answer_question", { sessionId, id, answers });
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "running" });
}

export function answerTauriMCPInteraction(sessionId, id, action, content) {
  record("bridge_answer_mcp_interaction", { sessionId, id, action, content });
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "running" });
}

export function replayTauriPendingPrompts(sessionId) {
  record("bridge_replay_pending_prompts", { sessionId });
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle" });
}

export function tauriPromptFromEvent(event) {
  if (event.eventKind !== "mcp_interaction" || !event.payload.mcpInteraction) return null;
  return { ...event.payload.mcpInteraction, kind: "mcp" };
}
export function tauriPromptAnsweredId(event) { return event.eventKind === "prompt_answered" ? event.payload.promptId ?? "" : ""; }
export function tauriSafeMCPURL(value) {
  if (typeof value !== "string") return undefined;
  try {
    const target = new URL(value.trim());
    return (target.protocol === "https:" || target.protocol === "http:") && target.hostname && !target.username && !target.password ? target.href : undefined;
  } catch { return undefined; }
}
export function tauriPlatformInfo() { return Promise.resolve("darwin"); }

export function tauriWorkspace(sessionId, path) {
  record("bridge_workspace", { sessionId, path });
  return globalThis.__workspaceHandler?.(sessionId, path) ?? Promise.resolve({ path, entries: [], truncated: false });
}
export function tauriWorkspaceFile(sessionId, path) {
  record("bridge_workspace_file", { sessionId, path });
  return globalThis.__workspaceFileHandler?.(sessionId, path) ?? Promise.resolve({ path, body: "", size: 0 });
}
export function tauriWorkspaceChanges(sessionId) {
  record("bridge_workspace_changes", { sessionId });
  return globalThis.__workspaceChangesHandler?.(sessionId) ?? Promise.resolve({ files: [], gitAvailable: false });
}
export function tauriWorkspaceChangeDetail(sessionId, path) {
  record("bridge_workspace_change_detail", { sessionId, path });
  return globalThis.__workspaceDetailHandler?.(sessionId, path) ?? Promise.resolve({ source: "" });
}

export function startTauriBridgeEvents() {
  record("bridge_start_events");
  if (globalThis.__outageOnStart) globalThis.__emitBridgeError?.("temporarily unavailable");
  else if (!globalThis.__holdStreamReady) queueMicrotask(() => globalThis.__emitBridgeRestored?.());
  return Promise.resolve();
}
export function onTauriBridgeEvent(callback) { globalThis.__emitBridgeEvent = callback; return Promise.resolve(() => { if (globalThis.__emitBridgeEvent === callback) globalThis.__emitBridgeEvent = undefined; }); }
export function onTauriBridgeConnectionError(callback) { globalThis.__emitBridgeError = callback; return Promise.resolve(() => { if (globalThis.__emitBridgeError === callback) globalThis.__emitBridgeError = undefined; }); }
export function onTauriBridgeConnectionRestored(callback) { globalThis.__emitBridgeRestored = callback; return Promise.resolve(() => { if (globalThis.__emitBridgeRestored === callback) globalThis.__emitBridgeRestored = undefined; }); }
export function onTauriBridgeResyncRequired(callback) { globalThis.__emitBridgeResync = callback; return Promise.resolve(() => { if (globalThis.__emitBridgeResync === callback) globalThis.__emitBridgeResync = undefined; }); }

export function tauriAssistantTextDelta() { return ""; }
export function tauriComposerInput(prompt, attachments) { return prompt.trim(); }
export function tauriEventSummary(event) { return JSON.stringify(event.payload); }
`;
