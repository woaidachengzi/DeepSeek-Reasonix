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
export function tauriPreviewRuntimeInfo() { return Promise.resolve({ stableVersion: "1.38.3", stableCommit: "test", previewVersion: "0.1.0", previewCommit: "unknown", previewDirty: false, tauriVersion: "2", previewBuild: "test-build", bridgeProtocolVersion: 1 }); }
export function openTauriExternalURL(url) { record("open_external_url", { url }); return Promise.resolve(); }
export function tauriProviderSummary() { return Promise.resolve({ protocolVersion: 1, defaultModel: "", plannerModel: "", visionModel: "", webSearchModel: "", providers: [] }); }
export function tauriProviderConfigs() { return Promise.resolve({ protocolVersion: 1, providers: [] }); }
export function saveTauriProviderConfig(input) { record("save_provider_config", { input }); return Promise.resolve({ protocolVersion: 1, providers: [input] }); }
export function installTauriProviderPreset(preset) { record("save_provider_config", { input: { presetId: preset.id, revision: preset.revision } }); return Promise.resolve({ protocolVersion: 1, providers: [], presets: [] }); }
export function resetTauriProviderPreset(preset) { record("save_provider_config", { input: { presetId: preset.id, presetAction: "reset", revision: preset.revision } }); return Promise.resolve({ protocolVersion: 1, providers: [], presets: [] }); }
export function deleteTauriProviderConfig(provider) { record("delete_provider_config", { provider }); return Promise.resolve({ protocolVersion: 1, providers: [] }); }
export function tauriUsageStats(request) { record("usage_stats", { request }); return Promise.resolve({ protocolVersion: 1, from: "2026-09-01", to: "2026-09-27", tokens: 0, requests: 0, turns: 0, cacheHit: 0, cacheMiss: 0, activeDays: 0, topModel: "", topProvider: "", daily: [], models: [], providers: [] }); }
export function tauriPermissionSettings() { record("permission_settings"); return Promise.resolve({ protocolVersion: 1, mode: "ask", allow: [], ask: [], deny: [] }); }
export function changeTauriPermissionSettings(change) { record("change_permission_settings", { change }); return Promise.resolve({ protocolVersion: 1, mode: change.mode ?? "ask", allow: [], ask: [], deny: [] }); }
export function tauriSandboxSettings() { record("sandbox_settings"); return Promise.resolve({ protocolVersion: 1, bash: "enforce", network: true, workspaceRoot: "", allowWrite: [], platform: "darwin" }); }
export function changeTauriSandboxSettings(change) { record("change_sandbox_settings", { change }); return Promise.resolve({ protocolVersion: 1, ...change, platform: "darwin" }); }
export function tauriNetworkSettings() { record("network_settings"); return Promise.resolve({ protocolVersion: 1, proxyMode: "auto", noProxy: "", proxyType: "socks5", proxyServer: "", proxyPort: 0, proxyUsername: "", proxyUrlSet: false, proxyPasswordSet: false }); }
export function changeTauriNetworkSettings(change) { record("change_network_settings", { change }); return Promise.resolve({ protocolVersion: 1, ...change, proxyUrlSet: change.proxyUrlAction === "replace", proxyPasswordSet: change.proxyPasswordAction === "replace" }); }
export function tauriSkillsSettings(workspaceRoot) { record("skills_settings", { workspaceRoot }); return Promise.resolve({ protocolVersion: 1, allowImplicitInvocation: true, skills: [], sources: [] }); }
export function changeTauriSkillsSettings(change) { record("change_skills_settings", { change }); return Promise.resolve({ protocolVersion: 1, allowImplicitInvocation: change.action === "implicit" ? change.enabled : true, skills: [], sources: [] }); }
export function planTauriSkillInstall(request) { record("plan_skill_install", { request }); return Promise.resolve({ protocolVersion: 1, planId: "sha256:stub", actions: [], warningCount: 0, warnings: [] }); }
export function installTauriSkill(request) { record("install_skill", { request }); return Promise.resolve({ protocolVersion: 1, status: "done", failedNames: [], settings: { protocolVersion: 1, allowImplicitInvocation: true, skills: [], sources: [] } }); }
export function archiveTauriSkill(request) { record("archive_skill", { request }); return Promise.resolve({ protocolVersion: 1, backupPath: "/preview/removed-skills/skill--stub", settings: { protocolVersion: 1, allowImplicitInvocation: true, skills: [], sources: [], archivedSkills: [] } }); }
export function restoreTauriSkill(request) { record("restore_skill", { request }); return Promise.resolve({ protocolVersion: 1, backupPath: "", settings: { protocolVersion: 1, allowImplicitInvocation: true, skills: [], sources: [], archivedSkills: [] } }); }
export function tauriPluginSettings() { record("plugin_settings"); return Promise.resolve({ protocolVersion: 1, plugins: [] }); }
export function changeTauriPluginSettings(change) { record("change_plugin_settings", { change }); return Promise.resolve({ protocolVersion: 1, plugins: [] }); }
export function planTauriPluginInstall(source) { record("plan_plugin_install", { source }); return Promise.resolve({ protocolVersion: 1, planId: "sha256:stub", actions: [], warningCount: 0, warnings: [] }); }
export function installTauriPlugin(request) { record("install_plugin", { request }); return Promise.resolve({ protocolVersion: 1, status: "done", failedNames: [], settings: { protocolVersion: 1, plugins: [] } }); }
export function removeTauriPlugin(request) { record("remove_plugin", { request }); return Promise.resolve({ protocolVersion: 1, status: "done", failedNames: [], settings: { protocolVersion: 1, plugins: [] } }); }
export function chooseTauriPluginDirectory() { record("choose_plugin_directory"); return Promise.resolve(null); }
export function tauriSubagentSettings(workspaceRoot) { record("subagent_settings", { workspaceRoot }); return Promise.resolve({ protocolVersion: 1, defaultModel: "demo/m", subagentModel: "", subagentEffort: "", maxDepth: 2, maxConcurrency: 6, maxParallelWriters: 3, modelRefs: ["demo/m"], modelEfforts: { "demo/m": ["auto", "low"] }, profiles: [] }); }
export function changeTauriSubagentSettings(change) { record("change_subagent_settings", { change }); return Promise.resolve({ protocolVersion: 1, defaultModel: "demo/m", subagentModel: change.action === "model" ? change.value : "", subagentEffort: "", maxDepth: change.action === "depth" ? change.number : 2, maxConcurrency: 6, maxParallelWriters: 3, modelRefs: ["demo/m"], modelEfforts: { "demo/m": ["auto", "low"] }, profiles: [] }); }
export function tauriHooksSettings(scope, workspaceRoot) { record("hooks_settings", { scope, workspaceRoot }); return Promise.resolve({ protocolVersion: 1, scope, path: "/preview/settings.json", projectRoot: scope === "project" ? workspaceRoot : "", revision: "test", hooks: {}, events: ["PreToolUse", "Stop"] }); }
export function changeTauriHooksSettings(change) { record("change_hooks_settings", { change }); return Promise.resolve({ protocolVersion: 1, scope: change.scope, path: "/preview/settings.json", projectRoot: change.scope === "project" ? change.workspaceRoot : "", revision: "test-2", hooks: change.hooks, events: ["PreToolUse", "Stop"] }); }
export function tauriMemorySettings(workspaceRoot) { record("memory_settings", { workspaceRoot }); return Promise.resolve({ protocolVersion: 1, workspaceRoot, storeDir: "/preview/memory", globalStoreDir: "/preview/global-memory", docs: [], facts: [], archives: [], diagnostics: [] }); }
export function changeTauriMemorySettings(change) { record("change_memory_settings", { change }); return tauriMemorySettings(change.workspaceRoot); }
export function setTauriDefaultModel() { return Promise.resolve({ protocolVersion: 1, providers: [] }); }
export function setTauriModelRole(role, model) { record("set_model_role", { role, model }); return tauriProviderSummary(); }
export function tauriDesktopPreferences() { return Promise.resolve({ protocolVersion: 1, defaultToolApprovalMode: "auto" }); }
export function setTauriDesktopApproval(mode) { record("set_desktop_approval", { mode }); return Promise.resolve({ protocolVersion: 1, defaultToolApprovalMode: mode }); }
export function getTauriCloseBehavior() { return Promise.resolve("keep_running"); }
export function setTauriCloseBehavior(behavior) { record("set_close_behavior", { behavior }); return Promise.resolve(behavior); }
export function keychainSave(key, value) { record("keychain_save", { key, value }); return Promise.resolve(); }
export function keychainDelete(key) { record("keychain_delete", { key }); return Promise.resolve(true); }
export function importTauriStableProfile() { return Promise.resolve({ importedConfig: "", backupConfig: "" }); }
export function importTauriStableProjectFolders() {
  record("import_stable_project_folders");
  return Promise.resolve({ importedFile: "/tmp/desktop-projects.json", projectCount: 0 });
}
export function chooseTauriWorkspaceRoot() { return Promise.resolve(null); }
export function chooseTauriSkillSourceDirectory() { return Promise.resolve(null); }
export function chooseTauriAttachmentFiles() { return Promise.resolve([]); }

export function tauriWorkbenchSessions() {
  record("workbench_sessions");
  return Promise.resolve((globalThis.__workbenchSessions ?? []).slice());
}

export function tauriWorkbenchProjectFolders() {
  record("workbench_project_folders");
  return Promise.resolve({ folders: (globalThis.__savedProjectFolders ?? []).slice() });
}

export function rememberTauriWorkbenchProjectFolder(root) {
  record("remember_workbench_project_folder", { root });
  const folders = globalThis.__savedProjectFolders ?? [];
  if (!folders.some(folder => folder.root === root)) folders.push({ root });
  globalThis.__savedProjectFolders = folders;
  return Promise.resolve({ folders: folders.slice() });
}

export function renameTauriWorkbenchProjectFolder(root, title) {
  record("rename_workbench_project_folder", { root, title });
  const folders = globalThis.__savedProjectFolders ?? [];
  const folder = folders.find(item => item.root === root);
  if (folder) folder.title = title;
  else folders.push({ root, title });
  globalThis.__savedProjectFolders = folders;
  return Promise.resolve({ folders: folders.slice() });
}

export function tauriWorkbenchSessionPage(cursor, limit = 200) {
  record("workbench_session_page", { cursor, limit });
  const pages = globalThis.__workbenchPages;
  if (Array.isArray(pages) && pages.length > 0) {
    const page = pages.shift();
    return page instanceof Error ? Promise.reject(page) : Promise.resolve(page);
  }
  const sessions = (globalThis.__workbenchSessions ?? []).slice();
  return Promise.resolve({ sessions, nextCursor: null, total: sessions.length, source: "identity" });
}

export function tauriPendingSessionDeletesPage(cursor, limit = 200) {
  record("bridge_pending_session_deletes_page", { cursor, limit });
  return Promise.resolve({ sessions: (globalThis.__pendingSessionDeletes ?? []).slice(), nextCursor: null });
}

export function tauriPendingSessionTitleRecoveries() {
  record("bridge_pending_session_title_recoveries");
  return Promise.resolve((globalThis.__pendingSessionTitleRecoveries ?? []).slice());
}

export function tauriWorkspaceRootsAvailability(roots) {
  record("workspace_roots_availability", { roots });
  const unavailable = new Set(globalThis.__unavailableWorkspaceRoots ?? []);
  return Promise.resolve(roots.map(root => !unavailable.has(root)));
}

export function tauriImportLegacySessionCatalog() {
  record("bridge_import_legacy_session_catalog");
  return Promise.resolve((globalThis.__workbenchSessions ?? []).length);
}

export function tauriScanUnclaimedSessions() {
  record("bridge_scan_unclaimed_sessions");
  return Promise.resolve(globalThis.__scanImportCandidates ?? { candidates: [], blockedCount: 0 });
}

export function tauriImportUnclaimedSessions(selected) {
  record("bridge_import_unclaimed_sessions", { selected });
  return Promise.resolve(selected.map(item => item.id));
}

export function tauriSessionCatalogShadow() {
  record("bridge_session_catalog_shadow");
  const responses = globalThis.__sessionCatalogShadowResponses;
  if (Array.isArray(responses) && responses.length > 0) {
    const response = responses.shift();
    return response instanceof Error ? Promise.reject(response) : Promise.resolve(response);
  }
  if (globalThis.__failSessionCatalogShadow) {
    globalThis.__failSessionCatalogShadow = false;
    return Promise.reject(new Error("session catalog shadow unavailable"));
  }
  const count = (globalThis.__workbenchSessions ?? []).length;
  return Promise.resolve({
    legacyCount: count,
    directoryCount: count,
    matchedCount: count,
    directoryOnlyCount: 0,
    missingFromDirectory: 0,
    titleMismatches: 0,
    workspaceMismatches: 0,
    orderMismatches: 0,
    missingTranscripts: 0,
    physicalStateMismatches: 0,
    unclaimedTranscripts: 0,
    inventoryErrors: 0,
    legacyMatchesDirectory: true,
  });
}

export function tauriSessionPreviews(sessionIds) {
  record("bridge_session_previews", { sessionIds });
  return Promise.resolve(sessionIds.map(sessionId => ({ sessionId, firstUser: globalThis.__previewFirstUsers?.[sessionId] ?? "" })));
}

export function backfillTauriWorkbenchTitles(titles) {
  record("backfill_workbench_titles", { titles });
  const resolvedTitles = titles.map(item => ({
    ...item,
    title: globalThis.__backfillTitleOverrides?.[item.sessionId] ?? item.title,
  }));
  const byId = new Map(resolvedTitles.map(item => [item.sessionId, item.title]));
  globalThis.__workbenchSessions = (globalThis.__workbenchSessions ?? []).map(entry => ({
    ...entry,
    title: [undefined, "", "新的会话", "新对话", "新建对话"].includes(entry.title)
      ? byId.get(entry.sessionId) ?? entry.title
      : entry.title,
  }));
  return Promise.resolve({ sessions: globalThis.__workbenchSessions.slice(), resolvedTitles });
}

export function rememberTauriWorkbenchSession(sessionId, workspaceRoot, title) {
  record("remember_workbench_session", { sessionId, workspaceRoot, title });
  const list = globalThis.__workbenchSessions ?? [];
  const existing = list.find(entry => entry.sessionId === sessionId);
  if (existing) {
    globalThis.__workbenchSessions = list.map(entry =>
      entry.sessionId === sessionId
        ? { ...entry, workspaceRoot, title: title ?? entry.title }
        : entry
    );
    return Promise.resolve(globalThis.__workbenchSessions.slice());
  }
  globalThis.__workbenchSessions = [{ sessionId, workspaceRoot, title }, ...list];
  return Promise.resolve(globalThis.__workbenchSessions.slice());
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
  globalThis.__pendingSessionTitleRecoveries = (globalThis.__pendingSessionTitleRecoveries ?? []).filter(entry => entry.id !== sessionId);
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle", title: globalThis.__recoveredTitles?.[sessionId] ?? storedTitle(sessionId), workspaceRoot });
}

export function switchTauriBridgeSession(sessionId, workspaceRoot) {
  record("bridge_switch_session", { sessionId, workspaceRoot });
  const failure = globalThis.__switchFailure;
  if (failure) return Promise.reject(new Error(failure));
  globalThis.__pendingSessionTitleRecoveries = (globalThis.__pendingSessionTitleRecoveries ?? []).filter(entry => entry.id !== sessionId);
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle", title: globalThis.__recoveredTitles?.[sessionId] ?? storedTitle(sessionId), workspaceRoot });
}

export function renameTauriBridgeSession(sessionId, title) {
  record("bridge_rename_session", { sessionId, title });
  return Promise.resolve({ id: sessionId, path: "/tmp/" + sessionId + ".jsonl", state: "idle", title });
}

export function deleteTauriBridgeSession(sessionId) {
  record("bridge_delete_session", { sessionId });
  const failure = globalThis.__deleteFailure;
  if (failure) return Promise.reject(new Error(failure));
  globalThis.__pendingSessionDeletes = (globalThis.__pendingSessionDeletes ?? []).filter(entry => entry.id !== sessionId);
  globalThis.__pendingSessionTitleRecoveries = (globalThis.__pendingSessionTitleRecoveries ?? []).filter(entry => entry.id !== sessionId);
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

export function tauriMCPServers(workspaceRoot) {
  record("list_mcp_servers", { workspaceRoot });
  return Promise.resolve((globalThis.__mcpServers ?? []).slice());
}

export function saveTauriMCPServer(server, workspaceRoot) {
  record("save_mcp_server", { server, workspaceRoot });
  const list = (globalThis.__mcpServers ?? []).filter(entry => entry.name !== server.name);
  const saved = {
    name: server.name,
    enabled: (globalThis.__mcpServers ?? []).find(entry => entry.name === server.name)?.enabled ?? true,
    type: server.type ?? "stdio",
    source: server.scope === "project" ? "project_config" : "user_config",
    scope: server.scope,
    configPath: "/tmp/config.toml",
    command: server.command,
    args: server.args,
    url: server.url,
    envKeys: server.env ? Object.keys(server.env) : (globalThis.__mcpServers ?? []).find(entry => entry.name === server.name)?.envKeys,
    headerKeys: server.headers ? Object.keys(server.headers) : (globalThis.__mcpServers ?? []).find(entry => entry.name === server.name)?.headerKeys,
  };
  list.push(saved);
  globalThis.__mcpServers = list;
  return Promise.resolve({ protocolVersion: 1, status: "saved", configPath: saved.configPath, server: saved, servers: list.slice() });
}

export function deleteTauriMCPServer(name, workspaceRoot) {
  record("delete_mcp_server", { name, workspaceRoot });
  const list = (globalThis.__mcpServers ?? []).filter(entry => entry.name !== name);
  globalThis.__mcpServers = list;
  return Promise.resolve({ protocolVersion: 1, status: "removed", servers: list.slice() });
}

export function setTauriMCPServerEnabled(name, enabled, workspaceRoot) {
  record("set_mcp_server_enabled", { name, enabled, workspaceRoot });
  const list = (globalThis.__mcpServers ?? []).map(entry => entry.name === name ? { ...entry, enabled } : entry);
  globalThis.__mcpServers = list;
  return Promise.resolve({ protocolVersion: 1, status: "activated", server: list.find(entry => entry.name === name), servers: list.slice() });
}

export function searchTauriMCPMarketplace(query) {
  record("search_mcp_marketplace", { query });
  return Promise.resolve({ protocolVersion: 1, cached: false, servers: [{ name: "io.example/remote", suggestedName: "remote", title: "Remote", description: "Example remote server", version: "1.0", installable: true, transport: "http", url: "https://mcp.example.test/mcp" }] });
}

export function resolveTauriMCPMarketplace(name) {
  record("resolve_mcp_marketplace", { name });
  return Promise.resolve({ name, suggestedName: "remote", title: "Remote", installable: true, transport: "http", url: "https://mcp.example.test/mcp" });
}

export function tauriAssistantTextDelta() { return ""; }
export function tauriComposerInput(prompt, attachments) { return prompt.trim(); }
export function tauriEventSummary(event) { return JSON.stringify(event.payload); }
`;
