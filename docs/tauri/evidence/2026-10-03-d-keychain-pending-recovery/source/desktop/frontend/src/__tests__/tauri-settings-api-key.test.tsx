// Run: tsx src/__tests__/tauri-settings-api-key.test.tsx
import assert from "node:assert/strict";
import { mock } from "node:test";
import { registerHooks } from "node:module";
import { JSDOM } from "jsdom";

registerHooks({ resolve(specifier, context, nextResolve) {
  if (specifier.endsWith(".css")) return nextResolve("./asset-stub-for-tests.ts", { ...context, parentURL: import.meta.url });
  return nextResolve(specifier, context);
} });

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { url: "http://localhost/" });
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  Element: dom.window.Element,
  HTMLElement: dom.window.HTMLElement,
  Event: dom.window.Event,
  CustomEvent: dom.window.CustomEvent,
  MouseEvent: dom.window.MouseEvent,
  localStorage: dom.window.localStorage,
  IS_REACT_ACT_ENVIRONMENT: true,
  isTauri: true,
});
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
let copiedPath = "";
let copiedHooksPath = "";
let exportedFrontendDiagnostics: unknown;
Object.defineProperty(dom.window.navigator, "clipboard", { value: { writeText: async (value: string) => { copiedPath = value; } }, configurable: true });
class TestResizeObserver { observe() {} unobserve() {} disconnect() {} }
globalThis.ResizeObserver = TestResizeObserver as unknown as typeof ResizeObserver;

let keyPresent = false;
const envCredentialPresent = true;
let saveGate: Promise<void> | null = null;
let releaseSave: (() => void) | undefined;
let failSummary = false;
let failSave = false;
let failImport = false;
let missingLegacy = false;
const importRequests: Record<string, unknown>[] = [];
let failRuntime = false;
let failNotificationPermission = false;
const notificationPermissionRequests: boolean[] = [];
let permissionGate: Promise<void> | null = null;
let releasePermission: (() => void) | undefined;
let failStorage = false;
const calls: string[] = [];
const capabilityDiagnosticsCalls: { workspaceRoot?: string; includeSessionRuntime?: boolean }[] = [];
let openedURL = "";
let closeBehavior = "keep_running";
let zoomFactor = 1;
let approvalMode = "auto";
let desktopLanguage = "zh";
let displayCurrency = "";
let terminalTheme = "auto";
let desktopTheme = "auto";
let desktopThemeStyle = "";
let activeThemeId = "";
let userThemes: Array<Record<string, unknown>> = [];
let savedThemePayload: Record<string, unknown> | undefined;
let appearanceConfigured = false;
let defaultModel = "";
let plannerModel = "";
let visionModel = "";
let webSearchModel = "";
let reasoningLanguage: "auto" | "zh" | "en" = "auto";
let compactRatioPercent = 80;
let projectModelPreferences = { defaultModel: "demo/new", plannerModel: "", visionModel: "", webSearchModel: "auto", reasoningLanguage: "zh" as "auto" | "zh" | "en", compactRatioPercent: 70 };
let lastModelPreferenceRequest: Record<string, unknown> | undefined;
const permissions = { protocolVersion: 1, scope: "global" as "global" | "project", mode: "ask", allow: [] as string[], ask: [] as string[], deny: [] as string[], projectOverrides: { mode: false, allow: false, ask: false, deny: false } };
const secretsSettings = { protocolVersion: 1, filterSubprocessEnv: false, protectSensitiveFiles: false };
let sandbox = { protocolVersion: 1, bash: "enforce", network: true, workspaceRoot: "", allowWrite: [] as string[], platform: "darwin", shell: "auto", resolvedShell: "bash", shellCapabilities: [{ id: "bash", available: true, path: "/bin/bash" }, { id: "zsh", available: true, path: "/bin/zsh" }, { id: "sh", available: true, path: "/bin/sh" }], gitCapability: { id: "git", available: true, path: "/usr/bin/git" }, effectiveWriteRoots: ["/preview/project"], effectiveRootsError: "" };
let sandboxQueryRoot = "";
let sandboxQuerySession = "";
let network = { protocolVersion: 1, proxyMode: "auto", noProxy: "", proxyType: "socks5", proxyServer: "", proxyPort: 0, proxyUsername: "", proxyUrlSet: false, proxyPasswordSet: false };
let skills = { protocolVersion: 1, allowImplicitInvocation: true, globalAllowImplicitInvocation: true, projectOverrides: { implicit: false, skills: false, sources: false }, skills: [{ name: "demo", description: "A test skill", invocation: "/demo", scope: "global", sourcePath: "/tmp/skills/demo/SKILL.md", runAs: "inline", enabled: true, requires: ["mcp-server:github"], archiveRevision: "" }], sources: [{ path: "/tmp/skills", scope: "custom", status: "ok", enabled: true, configured: true, skillCount: 1 }], archivedSkills: [] as { name: string; scope: "global" | "project"; archiveId: string; path: string; revision: string }[] };
let failSkills = false;
let plugins = { protocolVersion: 1, plugins: [{ name: "sample", description: "A test plugin", version: "1.0", source: "local", root: "/preview/plugins/sample", manifestKind: "reasonix", enabled: true, status: "ready", issue: "", warningCount: 0, skills: 1, agents: 0, commands: 2, hooks: 1, hookDetails: [{ event: "PreToolUse", match: "Bash", description: "Review shell commands", contextFile: "hooks/policy.md" }], mcpServers: 1, runtime: false, revision: "a".repeat(64) }] };
let installedPluginSource = "";
let removedPluginName = "";
let lastSkillScope = "";
let installedSkillSource = "";
let installedSkillScope = "";
let archivedSkillRevision = "";
let restoredSkillArchiveId = "";
let subagents = { protocolVersion: 1, defaultModel: "demo/m", subagentModel: "", subagentEffort: "", maxDepth: 2, maxConcurrency: 6, maxParallelWriters: 3, modelRefs: ["demo/m"], modelEfforts: { "demo/m": ["auto", "low", "high"] }, profiles: [{ name: "reviewer", description: "Reviews code", scope: "global", invocation: "/reviewer", invocationMode: "manual", configuredModel: "", configuredEffort: "" }] };
let savedSubagentProfile: { action?: string; scope?: string; profile?: { name: string; description: string; systemPrompt: string } } | undefined;
let hooks = { protocolVersion: 1, scope: "global", path: "/preview/settings.json", projectRoot: "", revision: "r1", hooks: {} as Record<string, unknown>, events: ["PreToolUse", "Stop"] };
let memory = { protocolVersion: 1, workspaceRoot: "/preview/project", storeDir: "/preview/memory", globalStoreDir: "/preview/global-memory", docs: [{ path: "/preview/project/AGENTS.md", scope: "project", body: "Old instruction.\n", revision: "r1" }], facts: [{ id: "memory-fact", revision: 2, createdAt: "2026-09-01T00:00:00Z", updatedAt: "2026-09-02T00:00:00Z", name: "project-fact", title: "Project fact", description: "Current description", type: "project", scope: "project", body: "Current body", freshness: "fresh" }], archives: [] as unknown[], revisions: [] as unknown[], diagnostics: [] as string[], lastRecall: { query: "language preference", hits: [{ id: "memory-fact", revision: 2, name: "project-fact", title: "Project fact", type: "project", scope: "project", score: 0.9, freshness: "fresh", reason: "matched recent context", snippet: "Reply in Chinese." }], omitted: 0, charBudget: 480, usedChars: 18 } };
let memorySuggestions = { memories: [{ id: "memory-pref", name: "language-pref", title: "Language preference", description: "Reply in Chinese by default", type: "feedback", scope: "project", body: "Use Chinese unless English is requested.", reason: "future-facing preference", evidence: ["session-a: 以后请始终用中文回复"] }], skills: [], generatedAt: "2026-09-28T00:00:00Z", available: true, source: "local-history" };
let memoryNote = "";
let allowDeleteProvider = false;
let deletedProviderName = "";
let deletedProviderRevision = "";
let savedProviderInput: { name: string; displayName: string; kind: string; baseUrl: string; modelsUrl?: string; clearModelsUrl?: boolean; noProxy?: boolean; contextWindow?: number; responsesMode?: string; balanceUrl?: string; clearBalanceUrl?: boolean; models: string[]; default: string; useApiKey: boolean } | undefined;
let savedPresetId = "";
let presetInstalled = false;
let presetModified = false;
let savedPresetAction = "";
let lastProbeRequest: { name: string; model: string; apiKey?: string } | null = null;
const providerPreset = () => ({ id: "mimo-api", label: "MiMo API", description: "MiMo direct API", group: "Xiaomi", recommended: false, status: presetInstalled ? presetModified ? "installed_modified" : "installed" : "available", revision: "r1", routes: [{ name: "mimo-api", kind: "openai", baseUrl: "https://api.xiaomimimo.com/v1", models: ["mimo-v2.5-pro"], default: "mimo-v2.5-pro" }] });
const summary = (scope = "global") => ({
  protocolVersion: 1,
  scope,
  defaultModel: scope === "project" ? projectModelPreferences.defaultModel : defaultModel,
  plannerModel: scope === "project" ? projectModelPreferences.plannerModel : plannerModel,
  visionModel: scope === "project" ? projectModelPreferences.visionModel : visionModel,
  webSearchModel: scope === "project" ? projectModelPreferences.webSearchModel : webSearchModel,
  reasoningLanguage: scope === "project" ? projectModelPreferences.reasoningLanguage : reasoningLanguage,
  compactRatioPercent: scope === "project" ? projectModelPreferences.compactRatioPercent : compactRatioPercent,
  providers: [{ name: "demo", displayName: "Demo", kind: "openai", modelCount: 2, models: ["m", "new"], visionModels: ["m"], searchModels: ["m"], requiresKey: true, configured: keyPresent || envCredentialPresent }],
});

(dom.window as unknown as { __TAURI_INTERNALS__: { invoke: (command: string) => Promise<unknown> } }).__TAURI_INTERNALS__ = {
  async invoke(command: string, args?: { payload?: unknown; scope?: string; source?: string; workspaceRoot?: string; currency?: string; url?: string; behavior?: string; mode?: string; theme?: string | { id?: string; [key: string]: unknown }; request?: { model?: string; role?: string; reasoningLanguage?: "auto" | "zh" | "en"; compactRatioPercent?: number; source?: string; scope?: string; workspaceRoot?: string; name?: string; apiKey?: string; planId?: string; revision?: string; acceptRisk?: boolean; kind?: string; id?: string }; input?: { name: string; displayName: string; kind: string; baseUrl: string; modelsUrl?: string; clearModelsUrl?: boolean; noProxy?: boolean; contextWindow?: number; responsesMode?: string; balanceUrl?: string; clearBalanceUrl?: boolean; models: string[]; default: string; useApiKey: boolean; revision?: string; presetId?: string; presetAction?: string }; change?: { action?: string; enabled?: boolean; name?: string; path?: string; mode?: string; list?: "allow" | "ask" | "deny"; rule?: string; bash?: string; network?: boolean; workspaceRoot?: string; allowWrite?: string[]; proxyMode?: string; noProxy?: string; proxyType?: string; proxyServer?: string; proxyPort?: number; proxyUsername?: string; proxyUrlAction?: string; proxyPasswordAction?: string; value?: string; number?: number; revision?: string; hooks?: Record<string, unknown>; scope?: string; body?: string; profile?: { name: string; description: string; systemPrompt: string }; filterSubprocessEnv?: boolean; protectSensitiveFiles?: boolean } }) {
    calls.push(command);
    switch (command) {
      case "plugin:clipboard-manager|write_text": copiedPath = (args as { text?: string })?.text ?? ""; copiedHooksPath = copiedPath; return;
      case "plugin:clipboard-manager|read_text": return copiedPath;
      case "capability_diagnostics":
        capabilityDiagnosticsCalls.push({ workspaceRoot: args?.workspaceRoot, includeSessionRuntime: (args as { includeSessionRuntime?: boolean } | undefined)?.includeSessionRuntime });
        return {
          schema_version: 1,
          root: args?.workspaceRoot ?? "<workspace>",
          live: false,
          summary: { errors: 0, warnings: 0, infos: 0, instructions: 0, skills: 0, commands: 0, hooks: 0, plugins: 0, mcp_servers: 0 },
          instructions: { docs: [] },
          skills: { roots: [], entries: [], winners: 0, shadowed: 0 },
          commands: { roots: [], entries: [], winners: 0, shadowed: 0 },
          hooks: { trusted_project: false, project_defines_hooks: false, sources: [], entries: [] },
          plugins: { packages: [] },
          mcp: { servers: (args as { includeSessionRuntime?: boolean } | undefined)?.includeSessionRuntime ? [{ name: "github", effective: true, transport: "stdio", start_intent: "automatic", runtime_status: "connected", tool_count: 1, tools: [{ name: "search_issues" }] }] : [] },
          issues: [],
        };
      case "runtime_doctor": return { text: "runtime status: unavailable\\n", publishedGeneration: 0, allowResume: true, cleanRollback: true, hasIrreversible: false, noOpRebuilds: 0, fullRebuilds: 0, subgraphRebuilds: 0, staleDrops: 0, admissionRejected: 0, runtimeOwnerFallbacks: 0 };
      case "export_frontend_diagnostics": exportedFrontendDiagnostics = args?.payload; return true;
      case "preview_runtime_info":
        if (failRuntime) { failRuntime = false; throw new Error("runtime unavailable"); }
        return { previewVersion: "1", previewCommit: "unknown", previewDirty: false, stableVersion: "1", stableCommit: "test", tauriVersion: "2", bridgeProtocolVersion: 1, previewBuild: "test" };
      case "provider_summary":
        if (failSummary) { failSummary = false; throw new Error("summary unavailable"); }
        return summary(args?.scope ?? "global");
      case "test_provider_model": lastProbeRequest = args?.request as { name: string; model: string; apiKey?: string }; return { protocolVersion: 1, latencyMillis: 123 };
      case "provider_configs": return { protocolVersion: 1, providers: [{ name: "demo", displayName: "Demo", kind: "openai", models: ["m"], default: "m", modelsUrlSet: false, noProxy: false, contextWindow: 0, responsesMode: "", balanceUrlSet: false, removable: allowDeleteProvider, revision: "r1" }], presets: [providerPreset()] };
      case "save_provider_config": {
        if (args?.input?.presetId) { savedPresetId = args.input.presetId; savedPresetAction = args.input.presetAction ?? ""; presetInstalled = true; presetModified = savedPresetAction !== "reset"; return { protocolVersion: 1, providers: [{ name: "demo", displayName: "Demo", kind: "openai", models: ["m"], default: "m", removable: false, revision: "r1" }], presets: [providerPreset()] }; }
        savedProviderInput = args?.input;
        return { protocolVersion: 1, providers: [{ ...args?.input, removable: false }], presets: [providerPreset()] };
      }
      case "delete_provider_config": deletedProviderName = args?.input?.name ?? ""; deletedProviderRevision = args?.input?.revision ?? ""; return { protocolVersion: 1, providers: [] };
      case "usage_stats": return { protocolVersion: 1, from: "2026-09-01", to: "2026-09-27", tokens: 42, requests: 1, turns: 1, cacheHit: 0, cacheMiss: 42, activeDays: 1, topModel: "demo/m", topProvider: "demo", daily: [], models: [], providers: [] };
      case "storage_settings":
        if (failStorage) { failStorage = false; throw new Error("storage unavailable"); }
        return { protocolVersion: 1, profilePath: "/preview/home", statePath: "/preview/state", cachePath: "/preview/cache", extensionsPath: "/preview/home/plugins" };
      case "permission_settings": return { ...permissions, scope: args?.workspaceRoot ? "project" : "global" };
      case "secrets_settings": return { ...secretsSettings };
      case "change_secrets_settings": {
        if (typeof args?.change?.filterSubprocessEnv === "boolean") secretsSettings.filterSubprocessEnv = args.change.filterSubprocessEnv;
        if (typeof args?.change?.protectSensitiveFiles === "boolean") secretsSettings.protectSensitiveFiles = args.change.protectSensitiveFiles;
        return { ...secretsSettings };
      }
      case "change_permission_settings": {
        const change = args?.change;
        if (change?.action === "mode") permissions.mode = change.mode ?? permissions.mode;
        else if (change?.list && change.rule) {
          if (change.action === "add") permissions[change.list] = [...permissions[change.list], change.rule];
          else permissions[change.list] = permissions[change.list].filter(rule => rule !== change.rule);
        }
        return { ...permissions };
      }
      case "sandbox_settings": sandboxQueryRoot = args?.workspaceRoot ?? ""; sandboxQuerySession = args?.sessionId ?? ""; return { ...sandbox, effectiveShell: "zsh", shellReloadRequired: true };
      case "change_sandbox_settings": sandbox = { ...sandbox, ...args?.change }; return { ...sandbox };
      case "network_settings": return { ...network };
      case "change_network_settings": network = { ...network, ...args?.change }; return { ...network };
      case "skills_settings": if (failSkills) { failSkills = false; throw new Error("skill inventory unavailable"); } return { ...skills };
      case "plan_skill_install": return { protocolVersion: 1, planId: "sha256:skill-review", warningCount: 0, warnings: [], actions: [{ name: "reviewed-skill", target: "/preview/home/skills/reviewed-skill/SKILL.md", riskLevel: "medium" }] };
      case "install_skill": installedSkillSource = args?.request?.source ?? ""; installedSkillScope = args?.request?.scope ?? ""; skills = { ...skills, skills: [...skills.skills, { ...skills.skills[0], name: "reviewed-skill", invocation: "/reviewed-skill", sourcePath: "/preview/home/skills/reviewed-skill/SKILL.md", archiveRevision: "sha256:reviewed-revision" }] }; return { protocolVersion: 1, status: "done", failedNames: [], settings: { ...skills } };
      case "archive_skill": archivedSkillRevision = args?.request?.revision ?? ""; skills = { ...skills, skills: skills.skills.filter(item => item.name !== args?.request?.name), archivedSkills: [{ name: "reviewed-skill", scope: "global", archiveId: "reviewed-skill--aabbccddeeffaabbccddeeff", path: "/preview/home/removed-skills/reviewed-skill--aabbccddeeffaabbccddeeff", revision: "sha256:backup-revision" }] }; return { protocolVersion: 1, backupPath: skills.archivedSkills[0].path, settings: { ...skills } };
      case "restore_skill": restoredSkillArchiveId = args?.request?.archiveId ?? ""; skills = { ...skills, skills: [...skills.skills, { ...skills.skills[0], name: "reviewed-skill", invocation: "/reviewed-skill", sourcePath: "/preview/home/skills/reviewed-skill/SKILL.md", archiveRevision: "sha256:reviewed-revision" }], archivedSkills: [] }; return { protocolVersion: 1, backupPath: "", settings: { ...skills } };
      case "plugin_settings": return { ...plugins };
      case "change_plugin_settings": plugins = { ...plugins, plugins: plugins.plugins.map(item => item.name === args?.change?.name ? { ...item, enabled: Boolean(args.change.enabled), revision: "b".repeat(64) } : item) }; return { ...plugins };
      case "plan_plugin_install": return { protocolVersion: 1, planId: "sha256:reviewed", warningCount: 0, warnings: [], actions: [{ name: "planned", version: "1.0", manifestKind: "reasonix", riskLevel: "high", skills: 1, agents: 0, commands: 0, hooks: 1, mcpServers: 0, prompts: 0, themes: 0, runtime: true, runtimeCommand: "plugin-runtime", intercepts: ["input.receive"], replaces: [] }] };
      case "install_plugin": installedPluginSource = args?.request?.source ?? ""; plugins = { ...plugins, plugins: [...plugins.plugins, { ...plugins.plugins[0], name: "planned", revision: "c".repeat(64) }] }; return { protocolVersion: 1, status: "done", failedNames: [], settings: { ...plugins } };
      case "remove_plugin": removedPluginName = args?.request?.name ?? ""; plugins = { ...plugins, plugins: plugins.plugins.filter(item => item.name !== removedPluginName) }; return { protocolVersion: 1, status: "done", failedNames: [], settings: { ...plugins } };
      case "change_skills_settings": {
        lastSkillScope = args?.change?.scope ?? "";
        if (args?.change?.action === "implicit") {
          skills.allowImplicitInvocation = Boolean(args.change.enabled);
          if (args.change.scope === "project") skills.projectOverrides.implicit = true;
          else skills.globalAllowImplicitInvocation = Boolean(args.change.enabled);
        }
        if (args?.change?.action === "skill" && args.change.name) skills.skills = skills.skills.map(item => item.name === args.change?.name ? { ...item, enabled: Boolean(args.change?.enabled) } : item);
        return { ...skills };
      }
      case "subagent_settings": return { ...subagents };
      case "change_subagent_settings": {
        if (args?.change?.action === "depth") subagents.maxDepth = args.change.number ?? subagents.maxDepth;
        if (args?.change?.action === "model") subagents.subagentModel = args.change.value ?? subagents.subagentModel;
        if (args?.change?.action === "create_profile") savedSubagentProfile = args.change;
        return { ...subagents };
      }
      case "hooks_settings": return { ...hooks, scope: args?.scope ?? "global" };
      case "change_hooks_settings": hooks = { ...hooks, revision: "r2", hooks: args?.change?.hooks ?? {} }; return { ...hooks };
      case "memory_settings": return { ...memory };
      case "memory_suggestions": return { ...memorySuggestions };
      case "accept_memory_suggestion": memorySuggestions = { ...memorySuggestions, memories: [], skills: [] }; return { path: "/preview/memory/language-pref.md", suggestions: { ...memorySuggestions } };
      case "change_memory_settings": {
        if (args?.change?.action === "save_doc") memory = { ...memory, docs: memory.docs.map(doc => doc.path === args.change?.path ? { ...doc, body: args.change?.body ?? "", revision: "r2" } : doc) };
        if (args?.change?.action === "quick_add") memoryNote = args.change.body ?? "";
        if (args?.change?.action === "save_fact") memory = { ...memory, facts: memory.facts.map(fact => fact.id === args.change?.factId ? { ...fact, name: args.change.name ?? fact.name, title: args.change.title ?? fact.title, description: args.change.description ?? fact.description, type: args.change.type ?? fact.type, body: args.change.body ?? fact.body, revision: fact.revision + 1 } : fact) };
        if (args?.change?.action === "load_revisions") memory = { ...memory, revisions: [{ ...memory.facts[0], revision: 1, body: "Previous body", updatedAt: "2026-09-01T00:00:00Z" }] };
        if (args?.change?.action === "restore_revision") memory = { ...memory, facts: memory.facts.map(fact => fact.id === args.change?.factId ? { ...fact, revision: 3, body: "Previous body" } : fact), revisions: [] };
        return { ...memory };
      }
      case "set_default_model": {
        lastModelPreferenceRequest = { ...args?.request };
        if (args?.request?.scope === "project") projectModelPreferences.defaultModel = args.request.model ?? "";
        else defaultModel = args?.request?.model ?? "";
        return summary(args?.request?.scope ?? "global");
      }
      case "set_model_role": {
        lastModelPreferenceRequest = { ...args?.request };
        const target = args?.request?.scope === "project" ? projectModelPreferences : { plannerModel, visionModel, webSearchModel };
        if (args?.request?.role === "planner") target.plannerModel = args.request.model ?? "";
        if (args?.request?.role === "vision") target.visionModel = args.request.model ?? "";
        if (args?.request?.role === "search") target.webSearchModel = args.request.model ?? "";
        if (args?.request?.scope !== "project") { plannerModel = target.plannerModel; visionModel = target.visionModel; webSearchModel = target.webSearchModel; }
        return summary(args?.request?.scope ?? "global");
      }
      case "set_agent_preferences": {
        lastModelPreferenceRequest = { ...args?.request };
        const target = args?.request?.scope === "project" ? projectModelPreferences : { reasoningLanguage, compactRatioPercent };
        if (args?.request?.reasoningLanguage) target.reasoningLanguage = args.request.reasoningLanguage;
        if (typeof args?.request?.compactRatioPercent === "number") target.compactRatioPercent = args.request.compactRatioPercent;
        if (args?.request?.scope !== "project") { reasoningLanguage = target.reasoningLanguage; compactRatioPercent = target.compactRatioPercent; }
        return summary(args?.request?.scope ?? "global");
      }
      case "platform_info": return "win32";
      case "get_close_behavior": return closeBehavior;
      case "set_close_behavior": closeBehavior = args?.behavior ?? closeBehavior; return closeBehavior;
      case "get_zoom_factor": return zoomFactor;
      case "set_zoom_factor": zoomFactor = args?.factor ?? zoomFactor; return zoomFactor;
      case "desktop_preferences": return { protocolVersion: 1, defaultToolApprovalMode: approvalMode, language: desktopLanguage, displayCurrency, terminalTheme, theme: desktopTheme, themeStyle: desktopThemeStyle, appearanceConfigured };
      case "get_active_theme_id": return activeThemeId;
      case "set_active_theme_id": activeThemeId = args?.id ?? ""; return activeThemeId;
      case "list_user_themes": return userThemes.map(theme => ({ ...theme }));
      case "save_user_theme": {
        const payload = args?.theme && typeof args.theme === "object" ? args.theme : {};
        savedThemePayload = payload;
        userThemes = [...userThemes.filter(theme => theme.id !== payload.id), { ...payload, density: payload.density ?? "comfortable", corners: payload.corners ?? "soft" }];
        return { ...userThemes[userThemes.length - 1] };
      }
      case "delete_user_theme": userThemes = userThemes.filter(theme => theme.id !== args?.id); return;
      case "import_user_theme": {
        const imported = { id: "user-imported", name: "Imported Theme", baseStyle: "slate", tokens: { light: { accent: "#336699" }, dark: {} }, density: "compact", corners: "round" };
        userThemes = [...userThemes, imported];
        return imported;
      }
      case "export_user_theme": return true;
      case "set_desktop_approval": approvalMode = args?.mode ?? approvalMode; return { protocolVersion: 1, defaultToolApprovalMode: approvalMode };
      case "set_desktop_terminal_theme": terminalTheme = typeof args?.theme === "string" ? args.theme : terminalTheme; return { protocolVersion: 1, defaultToolApprovalMode: approvalMode, terminalTheme };
      case "set_desktop_appearance": desktopTheme = typeof args?.theme === "string" ? args.theme : desktopTheme; desktopThemeStyle = args?.style ?? desktopThemeStyle; appearanceConfigured = true; return { protocolVersion: 1, defaultToolApprovalMode: approvalMode, terminalTheme, theme: desktopTheme, themeStyle: desktopThemeStyle, appearanceConfigured };
      case "set_desktop_language": desktopLanguage = args?.language ?? desktopLanguage; return { protocolVersion: 1, defaultToolApprovalMode: approvalMode, language: desktopLanguage, displayCurrency, terminalTheme, theme: desktopTheme, themeStyle: desktopThemeStyle, appearanceConfigured };
      case "set_desktop_currency": displayCurrency = args?.currency ?? displayCurrency; return { protocolVersion: 1, defaultToolApprovalMode: approvalMode, language: desktopLanguage, displayCurrency, terminalTheme, theme: desktopTheme, themeStyle: desktopThemeStyle, appearanceConfigured };
      case "open_external_url": openedURL = args?.url ?? ""; return;
      case "notification_permission": {
        notificationPermissionRequests.push(args?.request === true);
        if (permissionGate) await permissionGate;
        if (failNotificationPermission) throw new Error("private-notification-platform-error");
        return { permission: "denied", clickSupported: true };
      }
      case "keychain_save":
        if (failSave) throw new Error("secret-in-error-message");
        if (saveGate) await saveGate;
        keyPresent = true;
        return;
      case "keychain_import_legacy": {
        importRequests.push(args ?? {});
        if (failImport) throw new Error("secret-in-migration-error");
        if (keyPresent) throw { code: "existing_credential" };
        if (missingLegacy) throw { code: "missing_legacy_credential" };
        if (saveGate) await saveGate;
        keyPresent = true;
        return;
      }
      case "keychain_delete": {
        const deleted = keyPresent;
        keyPresent = false;
        return deleted;
      }
      default: throw new Error(`unexpected command: ${command}`);
    }
  },
};

const React = await import("react");
const { act } = React;
const { renderToString } = await import("react-dom/server");
const { createRoot } = await import("react-dom/client");
const { LocaleProvider } = await import("../lib/i18n");
await import("../lib/tauriBridge");
const { TauriSettings } = await import("../tauri/TauriSettings");
let parentConfigured: boolean | undefined;
let applyCalls = 0;
let profileImportCalls = 0;
let restartCalls = 0;
let auditRefreshCalls = 0;
const sessionModelChanges: string[] = [];

renderToString(<LocaleProvider><TauriSettings onClose={() => {}} /></LocaleProvider>);
assert.equal(calls.length, 0, "rendering settings does not start bridge work");

const root = createRoot(document.getElementById("root")!);
await act(async () => { root.render(<LocaleProvider><TauriSettings initialTab="appearance" workspaceRoot="/preview/project" onClose={() => {}} onProviderSummaryChange={value => { parentConfigured = value.providers[0]?.configured; }} currentSessionId="session-preview" currentSessionState="idle" currentSessionModelRef="demo/m" onCurrentSessionModelChange={async model => { sessionModelChanges.push(model); return true; }} onApplyToCurrentSession={async () => { applyCalls += 1; return true; }} bridgeStatus={{ running: true, protocolVersion: 1 }} catalogAudit={{ legacyCount: 3, directoryCount: 3, matchedCount: 3, directoryOnlyCount: 0, missingFromDirectory: 0, titleMismatches: 0, workspaceMismatches: 0, orderMismatches: 0, missingTranscripts: 0, physicalStateMismatches: 0, unclaimedTranscripts: 0, inventoryErrors: 0, legacyMatchesDirectory: true }} sessionPageSource="identity" onRestartBridge={async () => { restartCalls += 1; return true; }} onRefreshCatalogAudit={async () => { auditRefreshCalls += 1; return; }} profile={{ stableConfig: "/stable/config.toml", previewHome: "/preview/home", previewConfigExists: false, stableConfigExists: true, importAvailable: true, managedProfile: true }} onImportStableProfile={async () => { profileImportCalls += 1; return "已备份并导入配置"; }} onScanUnclaimedSessions={() => {}} /></LocaleProvider>); });
assert.equal(calls.filter(call => call === "provider_summary").length, 1, `settings load once after mount (${JSON.stringify(calls)})`);
assert.equal(parentConfigured, true, "the model picker outside settings receives the initial summary");
assert.equal(document.documentElement.lang, "zh-CN", "saved desktop language is restored when settings load");

function click(label: string | string[]) {
  const labels = Array.isArray(label) ? label : [label];
  const button = [...document.querySelectorAll<HTMLButtonElement>("button")].find(candidate => labels.includes(candidate.textContent?.trim() ?? ""));
  assert.ok(button, `missing button: ${labels.join(" / ")}`);
  button.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
}

function enterKey(value: string) {
  const input = document.querySelector<HTMLInputElement>(".tauri-settings-apikey input");
  assert.ok(input);
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(input, value);
  input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
}

function visibleText() { return document.body.textContent ?? ""; }

assert.match(visibleText(), /视觉风格/, "appearance controls are available without opening the model tab");
await act(async () => { click(["通用", "General"]); });
const notificationEvents = [...document.querySelectorAll<HTMLInputElement>(".tauri-settings-notification-events input[type=checkbox]")];
assert.equal(notificationEvents.length, 3, "General exposes separate completion, approval, and question notifications");
assert.ok(notificationEvents.every(input => input.checked), "the three notification event types default to enabled");
assert.match(document.querySelector('[data-testid="system-notification-permission"]')?.textContent ?? "", /已被系统拒绝/, "Settings reports actual OS denial instead of claiming permission is granted");
assert.ok(notificationPermissionRequests.every(request => !request), "mounting Settings only reads authorization");
const masterNotificationToggle = document.querySelector<HTMLInputElement>(".tauri-settings-toggle input");
assert.ok(masterNotificationToggle);
await act(async () => { masterNotificationToggle.click(); });
permissionGate = new Promise(resolve => { releasePermission = resolve; });
await act(async () => { masterNotificationToggle.click(); masterNotificationToggle.click(); });
assert.equal(notificationPermissionRequests.filter(Boolean).length, 1, "enabling sends one explicit authorization request");
assert.equal(masterNotificationToggle.disabled, true, "the authorization operation cannot be duplicated");
await act(async () => { releasePermission?.(); });
permissionGate = null;
assert.equal(masterNotificationToggle.checked, true, "the saved notification preference remains independent of OS authorization");
assert.match(document.querySelector('[data-testid="system-notification-permission"]')?.textContent ?? "", /已被系统拒绝/);
await act(async () => { masterNotificationToggle.click(); });
failNotificationPermission = true;
await act(async () => { masterNotificationToggle.click(); });
assert.match(document.querySelector('[data-testid="system-notification-permission"]')?.textContent ?? "", /暂不可用/);
assert.doesNotMatch(visibleText(), /private-notification-platform-error/, "native permission errors do not expose diagnostics");
failNotificationPermission = false;

await act(async () => { notificationEvents[1].click(); });
assert.equal(JSON.parse(localStorage.getItem("tauri-desktop-notification-events") ?? "{}").approval_request, false, "approval notifications persist separately from other event notifications");
await act(async () => { click("English"); });
assert.equal(desktopLanguage, "en", "English desktop language is persisted through the Preview profile bridge");
assert.equal(document.documentElement.lang, "en", "English updates the live UI locale");
await act(async () => { click("CNY"); });
assert.equal(displayCurrency, "CNY", "CNY display currency is persisted through the Preview profile bridge");
await act(async () => { click("USD"); });
assert.equal(displayCurrency, "USD", "USD display currency is persisted through the Preview profile bridge");
await act(async () => { click("Auto (recommended)"); });
assert.equal(displayCurrency, "", "display currency can return to automatic selection");
await act(async () => { click(["外观", "Appearance"]); });
assert.match(visibleText(), /Appearance/, "Appearance heading follows the selected English locale");
assert.match(visibleText(), /Display zoom/, "display zoom controls follow the selected English locale");
assert.match(visibleText(), /Custom terminal colors/, "Appearance includes the custom terminal palette editor");
assert.doesNotMatch(visibleText(), /配色风格|对话宽度|界面字体/, "Appearance settings do not retain Chinese-only controls in English");
await act(async () => { document.querySelector<HTMLElement>(".tauri-terminal-palette > summary")?.click(); });
await act(async () => { document.querySelector<HTMLInputElement>(".tauri-terminal-palette__toggle input")?.click(); });
assert.equal(JSON.parse(localStorage.getItem("reasonix-terminal-palette-v1") ?? "{}").enabled, true, "custom terminal palette preference persists locally");
await act(async () => { click("Restore default colors"); });
assert.equal(JSON.parse(localStorage.getItem("reasonix-terminal-palette-v1") ?? "{}").enabled, false, "reset disables custom colors and persists the reset");
await act(async () => { click("Configure"); });
assert.match(document.querySelector(".typography-settings__header")?.textContent ?? "", /Detailed typography/, "the typography editor follows the selected English locale");
await act(async () => { document.querySelector<HTMLButtonElement>(".typography-settings__back")?.click(); });
await act(async () => { click(["通用", "General"]); });
await act(async () => { click(["快捷键", "Shortcuts"]); });
assert.equal(document.querySelector(".tauri-shortcuts-heading h3")?.textContent, "Keyboard shortcuts", "shortcut settings follow the selected English locale");
assert.match(visibleText(), /Refresh conversation/, "shortcut actions and descriptions follow the selected English locale");
await act(async () => { click(["数据位置", "Storage & paths"]); });
assert.match(visibleText(), /Preview data/, "Preview profile controls follow the selected English locale");
assert.match(visibleText(), /Review unclaimed sessions/, "the transcript review action follows the selected English locale");
await act(async () => { click(["通用", "General"]); });
await act(async () => { click("中文"); });
assert.equal(desktopLanguage, "zh", "desktop language is persisted through the Preview profile bridge");
assert.equal(document.documentElement.lang, "zh-CN", "desktop language updates the live UI locale");
await act(async () => { click("自动（跟随系统）"); });
assert.equal(desktopLanguage, "", "desktop language can return to following the system");
assert.equal(document.documentElement.lang, "en", "auto follows the English test system locale");
assert.deepEqual([...document.querySelectorAll(".tauri-settings-general > .settings-section > .settings-section__head .settings-section__title")].map(heading => heading.textContent), ["Desktop & language", "Conversation experience", "System behavior"], "General sections follow the active locale");
await act(async () => { click("中文"); });
await act(async () => { click(["外观", "Appearance"]); });
await act(async () => { document.querySelector<HTMLButtonElement>('[aria-label="终端主题"] button:nth-child(3)')?.click(); });
assert.equal(terminalTheme, "dark", "terminal theme is saved through the Preview config bridge");
assert.equal(document.documentElement.getAttribute("data-terminal-theme"), "dark", "terminal appearance updates immediately");
await act(async () => { document.querySelector<HTMLButtonElement>('[aria-label="终端主题"] button:first-child')?.click(); });
assert.equal(terminalTheme, "auto", "terminal theme can return to following the app");
assert.equal(document.documentElement.hasAttribute("data-terminal-theme"), false, "auto terminal appearance follows the app theme");
assert.equal(document.querySelector(".tauri-settings-zoom__control output")?.textContent, "100%", "appearance reads the persisted host zoom factor");
await act(async () => { document.querySelector<HTMLButtonElement>('[aria-label="放大界面"], [aria-label="放大显示缩放"]')?.click(); });
assert.equal(zoomFactor, 1.05, "display zoom is applied through the Tauri host");
assert.equal(document.querySelector(".tauri-settings-zoom__control output")?.textContent, "105%", "display zoom reports the host-applied value");
await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-settings-zoom__reset")?.click(); });
assert.equal(zoomFactor, 1, "display zoom can be reset to the default");
assert.ok(document.querySelector(".tauri-settings-sidebar .tauri-settings-nav"), "settings use a full-page sidebar");
const settingsSearch = document.querySelector<HTMLInputElement>('input[aria-label="搜索设置"]');
assert.ok(settingsSearch, "the sidebar has settings search");
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(settingsSearch, "快捷键");
  settingsSearch.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
assert.equal(document.querySelectorAll(".tauri-settings-nav-item").length, 1, "search filters navigation items");
assert.match(document.querySelector(".tauri-settings-nav-item")?.textContent ?? "", /快捷键/);
assert.match(document.querySelector(".tauri-settings-nav-item small")?.textContent ?? "", /按键与帮助/, "search exposes the matched setting's purpose");
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(settingsSearch, "");
  settingsSearch.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(settingsSearch, "关闭窗口时");
  settingsSearch.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
assert.equal(document.querySelectorAll(".tauri-settings-nav-item").length, 1, "search matches General settings by a field label");
assert.match(document.querySelector(".tauri-settings-nav-item")?.textContent ?? "", /通用/, "field search navigates to its settings page");
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(settingsSearch, "");
  settingsSearch.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("深色"); });
assert.equal(document.documentElement.getAttribute("data-theme"), "dark");
assert.equal(localStorage.getItem("tauri-theme"), "dark");
assert.equal(desktopTheme, "dark", "appearance mode is persisted through the Preview profile bridge");
await act(async () => { click("自动"); });
assert.equal(document.documentElement.hasAttribute("data-theme"), false);
await act(async () => { click("柔雾极光"); });
assert.equal(document.documentElement.getAttribute("data-theme-style"), "aurora");
assert.equal(localStorage.getItem("tauri-theme-style"), "aurora");
assert.equal(desktopThemeStyle, "aurora", "appearance style is persisted through the Preview profile bridge");
await act(async () => { click("浏览主题"); });
assert.match(visibleText(), /主题画廊/);
await act(async () => { click("创建自定义主题"); });
const userThemeName = document.querySelector<HTMLInputElement>('.tauri-theme-editor input:not([type="color"])');
assert.ok(userThemeName, "custom theme editor includes a name field");
Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(userThemeName, "My Preview Theme");
await act(async () => { userThemeName.dispatchEvent(new dom.window.Event("input", { bubbles: true })); });
const themeRecipeSelects = [...document.querySelectorAll<HTMLSelectElement>(".tauri-theme-editor__recipes select")];
assert.equal(themeRecipeSelects.length, 2, "the theme editor exposes density and corner recipes");
await act(async () => { themeRecipeSelects[0]!.value = "compact"; themeRecipeSelects[0]!.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
await act(async () => { themeRecipeSelects[1]!.value = "round"; themeRecipeSelects[1]!.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
assert.equal(document.querySelectorAll(".tauri-theme-editor__token").length, 18, "the editor exposes every host-validated semantic color token");
const sidebarToken = [...document.querySelectorAll<HTMLElement>(".tauri-theme-editor__token")].find(label => label.querySelector("code")?.textContent === "sidebar")?.querySelector<HTMLInputElement>('input[type="text"]');
assert.ok(sidebarToken, "custom themes expose the sidebar color token");
Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(sidebarToken, "#445566AA");
await act(async () => { sidebarToken.dispatchEvent(new dom.window.Event("input", { bubbles: true })); });
const sceneImageInputs = [...document.querySelectorAll<HTMLInputElement>(".tauri-theme-editor__scene input[type=file]")];
assert.equal(sceneImageInputs.length, 2, "custom theme editor exposes separate home and workspace image controls");
const pngHeader = new Uint8Array(24);
pngHeader.set([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a], 0);
pngHeader.set([0x49, 0x48, 0x44, 0x52], 12);
pngHeader.set([0, 0, 0, 1, 0, 0, 0, 1], 16);
const imageFile = new dom.window.File([pngHeader.buffer], "background.png", { type: "image/png" });
Object.defineProperty(sceneImageInputs[0], "files", { configurable: true, value: [imageFile] });
await act(async () => {
  sceneImageInputs[0]!.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  await new Promise(resolve => setTimeout(resolve, 25));
});
await act(async () => {
  document.querySelector<HTMLFormElement>(".tauri-theme-editor")!.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
  await new Promise(resolve => setTimeout(resolve, 0));
});
assert.ok((savedThemePayload?.backgroundAssetDataUrl as string | undefined)?.startsWith("data:image/png;base64,"), "uploaded home background reaches the native theme storage command");
assert.equal((savedThemePayload?.background as { image?: string } | undefined)?.image, "background.webp", "home scene settings accompany the uploaded image");
assert.equal(userThemes.length, 1, "custom theme is saved in the native host preferences");
assert.equal(userThemes[0]?.id, "user-my-preview-theme");
assert.equal(userThemes[0]?.density, "compact", "theme density is persisted by the host");
assert.equal(userThemes[0]?.corners, "round", "theme corner recipe is persisted by the host");
assert.equal((userThemes[0]?.tokens as { dark?: Record<string, string> }).dark?.sidebar, "#445566AA", "semantic theme colors persist through the host bridge");
assert.match(visibleText(), /My Preview Theme/, "the saved theme appears in the gallery");
await act(async () => { click("编辑主题"); });
const editedThemeName = document.querySelector<HTMLInputElement>('.tauri-theme-editor input:not([type="color"])');
assert.ok(editedThemeName, "a saved custom theme can be reopened for editing");
Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(editedThemeName, "My Edited Theme");
await act(async () => { editedThemeName.dispatchEvent(new dom.window.Event("input", { bubbles: true })); });
await act(async () => {
  document.querySelector<HTMLFormElement>(".tauri-theme-editor")!.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
  await new Promise(resolve => setTimeout(resolve, 0));
});
assert.equal(userThemes.length, 1, "editing replaces the existing custom theme instead of duplicating it");
assert.equal(userThemes[0]?.id, "user-my-preview-theme", "editing preserves the theme identity");
assert.match(visibleText(), /My Edited Theme/);
await act(async () => { click("应用主题"); });
assert.equal(activeThemeId, "user-my-preview-theme", "saved custom themes can be applied through the host preference bridge");
await act(async () => { click("浏览主题"); });
await act(async () => { [...document.querySelectorAll<HTMLButtonElement>(".theme-gallery-card")].find(button => button.textContent?.includes("My Edited Theme"))?.click(); });
await act(async () => { click("删除自定义主题"); });
await act(async () => { click("要删除这个自定义主题吗？"); });
assert.equal(userThemes.length, 0, "custom themes can be removed from native host preferences");
await act(async () => { document.querySelector<HTMLButtonElement>(".theme-gallery__back")?.click(); });
await act(async () => { click("浏览主题"); });
await act(async () => { click("导入"); });
assert.equal(userThemes.at(-1)?.id, "user-imported", "theme package import returns a persisted user theme");
assert.match(visibleText(), /Imported Theme/);
await act(async () => { click("导出"); });
assert.ok(calls.includes("export_user_theme"), "theme export reaches the native package writer command");
await act(async () => { document.querySelector<HTMLButtonElement>(".theme-gallery__back")?.click(); });
await act(async () => { click("全宽"); });
assert.equal(document.documentElement.getAttribute("data-conversation-width"), "full");
assert.equal(localStorage.getItem("reasonix-conv-width"), "full");
await act(async () => { click("大"); });
assert.equal(document.documentElement.getAttribute("data-text-size"), "large");
assert.equal(localStorage.getItem("reasonix-text-size"), "large");
await act(async () => { click("进入设置"); });
assert.ok(document.querySelector(".typography-settings__workspace"), "Preview opens the stable region typography editor");
assert.match(document.querySelector(".typography-settings__header")?.textContent ?? "", /详细排版设置/, "the Preview editor follows the selected Chinese locale");
await act(async () => { document.querySelectorAll<HTMLButtonElement>(".typography-settings__region")[2]?.click(); });
const followGlobal = document.querySelector<HTMLInputElement>(".typography-settings__follow input");
assert.ok(followGlobal?.checked, "conversation typography initially follows the global setting");
await act(async () => { followGlobal.click(); });
assert.equal(document.documentElement.style.getPropertyValue("--typography-conversation-size"), "14px", "conversation text changes immediately");
assert.equal(JSON.parse(localStorage.getItem("reasonix-region-typography-v1")!).conversation.followGlobal, false, "region typography persists for later launches");
await act(async () => { document.querySelector<HTMLButtonElement>('.typography-settings__back')?.click(); });
assert.ok(document.querySelector(".tauri-settings-typography-entry"), "the region editor returns to Preview appearance settings");
await act(async () => { click("进入设置"); });
await act(async () => { document.querySelectorAll<HTMLButtonElement>(".typography-settings__region")[2]?.click(); });
assert.equal(document.querySelector<HTMLInputElement>(".typography-settings__follow input")?.checked, false, "region typography survives reopening the editor");
await act(async () => { click("全部恢复默认"); });
assert.equal(document.documentElement.style.getPropertyValue("--typography-conversation-size"), "", "restoring defaults clears the rendered override");
await act(async () => { document.querySelector<HTMLButtonElement>('.typography-settings__back')?.click(); });
await act(async () => { click(["存储", "存储与路径", "Storage & paths"]); });
assert.match(visibleText(), /\/stable\/config.toml/, "import shows the stable source before copying");
assert.match(visibleText(), /\/preview\/home/, "import shows the isolated target before copying");
assert.match(visibleText(), /退出 Preview.*Wails/, "import explains how to return to the stable app");
const statePathInput = () => document.querySelector<HTMLInputElement>('input[aria-label="状态目录"], input[aria-label="State directory"], input[aria-label="狀態目錄"]');
const cachePathInput = () => document.querySelector<HTMLInputElement>('input[aria-label="缓存"], input[aria-label="Cache"], input[aria-label="快取"]');
assert.equal(statePathInput()?.value, "/preview/state", "storage page reads the effective core state directory");
assert.equal(cachePathInput()?.value, "/preview/cache", "storage page reads the effective core cache directory");
await act(async () => { document.querySelector<HTMLButtonElement>('[aria-label="复制状态目录"], [aria-label="Copy State directory"], [aria-label="複製狀態目錄"]')?.click(); });
assert.equal(copiedPath, "/preview/state", "storage path copy uses the displayed core path");
failStorage = true;
await act(async () => { document.querySelector<HTMLButtonElement>('.tauri-storage-settings .tauri-settings-data__heading button')?.click(); });
assert.match(document.querySelector('.tauri-storage-settings [role="alert"]')?.textContent ?? "", /storage unavailable/, "storage refresh exposes bridge failures");
assert.equal(statePathInput(), null, "failed refresh does not leave stale core directories");
assert.ok(document.querySelector<HTMLInputElement>('input[aria-label="新对话默认工作区"], input[aria-label="Default workspace for new chats"], input[aria-label="新對話的預設工作區"]'), "the local default workspace remains editable when the core directory query fails");
await act(async () => { document.querySelector<HTMLButtonElement>('.tauri-storage-settings .tauri-settings-data__heading button')?.click(); });
assert.equal(statePathInput()?.value, "/preview/state", "storage paths recover after retry");
assert.match(visibleText(), /\/preview\/home/, "data settings show the isolated preview directory");
await act(async () => { click("复制正式版配置（先备份）"); });
assert.equal(profileImportCalls, 1);
assert.match(visibleText(), /已备份并导入配置/, "data settings surface the import result");
await act(async () => { click(["通用", "General"]); });
assert.deepEqual([...document.querySelectorAll(".tauri-settings-general > .settings-section > .settings-section__head .settings-section__title")].map(heading => heading.textContent), ["桌面与语言", "会话体验", "系统行为"], "General follows the stable settings section order");
assert.ok(document.querySelector('[aria-label="关闭窗口时"]'), "close-to-tray behavior remains available on Windows");
assert.equal(document.querySelector('.tauri-settings-general [aria-label="外观模式"]'), null, "appearance controls are kept in the Appearance page");
await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-settings-sound-toggle")?.click(); });
const musicPresetSelect = document.querySelector<HTMLSelectElement>('select[aria-label="音乐预设"]');
assert.ok(musicPresetSelect, "Preview exposes the running-turn music presets");
await act(async () => {
  musicPresetSelect.value = "classic";
  musicPresetSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
});
assert.equal(localStorage.getItem("generativeMusicPreset"), "classic", "the music preset is saved for later turns");
assert.match(document.querySelector(".tauri-settings-sound-toggle")?.textContent ?? "", /自定义/, "the sound summary includes running-turn music");
await act(async () => {
  musicPresetSelect.value = "off";
  musicPresetSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
});
assert.equal(localStorage.getItem("generativeMusicPreset"), "off", "music can be disabled independently of notification sounds");
await act(async () => { document.querySelector<HTMLButtonElement>(".tauri-settings-sound-toggle")?.click(); });
const statusEditorToggle = document.querySelector<HTMLButtonElement>(".status-bar-items-editor__toggle");
assert.ok(statusEditorToggle, "Preview uses the full status bar item editor");
await act(async () => { statusEditorToggle.click(); });
assert.equal(document.querySelectorAll('[data-statusbar-setting-item]').length, 17, "only metrics with a Preview data source are offered");
assert.ok(document.querySelector('[data-statusbar-setting-item="turn_output_tokens"]'), "reported output tokens can be enabled");
assert.ok(document.querySelector('[data-statusbar-setting-item="turn_cache_tokens"]'), "reported cache tokens can be enabled");
const bridgeStatusToggle = document.querySelector<HTMLInputElement>('[data-statusbar-setting-item="bridge"] input[type="checkbox"]');
assert.ok(bridgeStatusToggle);
await act(async () => { bridgeStatusToggle.click(); });
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.status-bar.v1")!).items.includes("bridge"), false, "hidden Preview item is saved");
const workspaceMoveDown = document.querySelector<HTMLButtonElement>('[data-statusbar-setting-item="workspace"] .status-bar-item-row__actions > :last-child button');
assert.ok(workspaceMoveDown);
await act(async () => { workspaceMoveDown.click(); });
assert.deepEqual(JSON.parse(localStorage.getItem("reasonix.tauri.status-bar.v1")!).items.slice(0, 2), ["model", "workspace"], "Preview status order is saved");
const restoreStatusItems = [...document.querySelectorAll<HTMLButtonElement>(".status-bar-items-editor__action")].find(button => /恢复默认|Restore default/.test(button.textContent ?? ""));
assert.ok(restoreStatusItems);
await act(async () => { restoreStatusItems.click(); });
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.status-bar.v1")!).items.length, 17, "Preview status items can be restored");
assert.equal(document.querySelector('[aria-label="桌面风格"] [aria-checked="true"]')?.textContent, "工作台");
await act(async () => { click("创作"); });
assert.equal(localStorage.getItem("reasonix.tauri.desktop-layout.v1"), "creation", "desktop layout survives restart");
assert.equal(document.querySelector('[aria-label="桌面风格"] [aria-checked="true"]')?.textContent, "创作", "desktop layout updates immediately");
assert.equal(document.querySelector(".tauri-settings-overlay")?.getAttribute("data-desktop-layout"), "creation", "settings navigation follows the selected desktop layout");
await act(async () => { click("工作台"); });
assert.equal(localStorage.getItem("reasonix.tauri.desktop-layout.v1"), "workbench", "desktop layout can be restored");
assert.equal(document.querySelector(".tauri-settings-overlay")?.getAttribute("data-desktop-layout"), "workbench", "settings navigation restores the workbench layout");
await act(async () => { click("退出 Reasonix"); });
assert.equal(closeBehavior, "quit", "close-window behavior is saved through the native host");
assert.equal(document.querySelector('[aria-label="关闭窗口时"] [aria-checked="true"]')?.textContent, "退出 Reasonix");
await act(async () => { click("Yolo"); });
assert.equal(approvalMode, "yolo", "new-session approval default is saved through the bridge");
assert.equal(document.querySelector('[aria-label="新会话默认审批"] [aria-checked="true"]')?.textContent, "Yolo");
const notificationToggle = document.querySelector<HTMLInputElement>(".tauri-settings-toggle input");
assert.ok(notificationToggle, "general settings include desktop notifications");
await act(async () => { notificationToggle.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true })); });
assert.equal(localStorage.getItem("tauri-desktop-notifications"), "off", "notification preference is saved");
const soundToggle = document.querySelector<HTMLButtonElement>('.tauri-settings-sound-toggle');
assert.ok(soundToggle);
await act(async () => { soundToggle.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true })); });
const successSelect = document.querySelector<HTMLSelectElement>('select[aria-label="生成完成"]');
const attentionSelect = document.querySelector<HTMLSelectElement>('select[aria-label="等待操作"]');
const volumeSlider = document.querySelector<HTMLInputElement>('input[aria-label="通知音量"]');
assert.ok(successSelect && attentionSelect && volumeSlider, "sound controls are available in General");
await act(async () => { successSelect.value = "synth"; successSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
await act(async () => { attentionSelect.value = "positive"; attentionSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(volumeSlider, "42");
  volumeSlider.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  volumeSlider.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
});
assert.equal(localStorage.getItem("notificationSoundSuccess"), "synth");
assert.equal(localStorage.getItem("notificationSoundAttention"), "positive");
assert.equal(localStorage.getItem("notificationSoundVolume"), "42");

await act(async () => { click(["快捷键", "Shortcuts"]); });
const settingsShortcutKey = document.querySelector<HTMLButtonElement>('[data-tauri-shortcut-action="settings"]');
assert.ok(settingsShortcutKey);
await act(async () => { settingsShortcutKey.click(); });
assert.equal(document.activeElement, settingsShortcutKey, "Preview shortcut recorder receives keyboard focus");
for (const composition of [{ isComposing: true }, { keyCode: 229 }]) {
  for (const key of ["q", "Escape"]) {
    const before = localStorage.getItem("reasonix.tauri.shortcuts.v1");
    const event = new dom.window.KeyboardEvent("keydown", { key, ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true, ...composition });
    await act(async () => { settingsShortcutKey.dispatchEvent(event); });
    assert.equal(settingsShortcutKey.classList.contains("is-recording"), true, "IME keys do not finish shortcut recording");
    assert.equal(localStorage.getItem("reasonix.tauri.shortcuts.v1"), before, "IME keys do not replace the saved binding");
    assert.equal(event.defaultPrevented, false, "shortcut recording leaves composition to the input method");
  }
}
await act(async () => { settingsShortcutKey.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "q", ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true })); });
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.shortcuts.v1")!).settings.key, "q", "shortcut recording saves to Preview preferences");
const newSessionShortcutKey = document.querySelector<HTMLButtonElement>('[data-tauri-shortcut-action="new_session"]');
assert.ok(newSessionShortcutKey);
await act(async () => { newSessionShortcutKey.click(); });
await act(async () => { newSessionShortcutKey.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "q", ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true })); });
assert.match(document.querySelector('[role="alert"]')?.textContent ?? "", /冲突/, "conflicting Preview shortcuts are rejected");
const resetSettingsShortcut = settingsShortcutKey.closest(".tauri-shortcut-row")?.querySelector<HTMLButtonElement>(".tauri-shortcut-reset");
assert.ok(resetSettingsShortcut);
await act(async () => { resetSettingsShortcut.click(); });
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.shortcuts.v1")!).settings, undefined, "a shortcut can be reset individually");

const originalPlatform = dom.window.navigator.platform;
Object.defineProperty(dom.window.navigator, "platform", { value: "MacIntel", configurable: true });
await act(async () => { settingsShortcutKey.click(); });
await act(async () => { settingsShortcutKey.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "c", metaKey: true, bubbles: true, cancelable: true })); });
assert.match(document.querySelector('[role="alert"]')?.textContent ?? "", /macOS.*换一个快捷键/, "macOS native menu conflict explains how to resolve it");
assert.equal(settingsShortcutKey.classList.contains("is-recording"), true, "native rejection keeps recording available for a replacement chord");
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.shortcuts.v1")!).settings, undefined, "native conflict is not persisted");
await act(async () => { settingsShortcutKey.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "q", metaKey: true, shiftKey: true, bubbles: true, cancelable: true })); });
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.shortcuts.v1")!).settings.key, "q", "a non-native replacement chord is saved");
assert.equal(document.querySelector('[role="alert"]'), null, "successful replacement clears the error");
await act(async () => { newSessionShortcutKey.click(); });
await act(async () => { newSessionShortcutKey.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: ",", metaKey: true, bubbles: true, cancelable: true })); });
await act(async () => { resetSettingsShortcut.click(); });
assert.match(document.querySelector('[role="alert"]')?.textContent ?? "", /冲突/, "individual reset reports a default binding already assigned to another action");
assert.equal(JSON.parse(localStorage.getItem("reasonix.tauri.shortcuts.v1")!).settings.key, "q", "failed reset preserves the saved binding");
const resetAllShortcuts = document.querySelector<HTMLButtonElement>(".tauri-shortcuts-heading button");
assert.ok(resetAllShortcuts);
await act(async () => { resetAllShortcuts.click(); });
assert.equal(localStorage.getItem("reasonix.tauri.shortcuts.v1"), null, "reset all resolves custom/default collisions");
assert.equal(document.querySelector('[role="alert"]'), null, "reset all clears conflict feedback");
Object.defineProperty(dom.window.navigator, "platform", { value: originalPlatform, configurable: true });

await act(async () => { click(["通用", "General"]); });
await act(async () => { click("English"); });
await act(async () => { click(["模型偏好", "Model preferences"]); });
assert.match(visibleText(), /Model assignment/, "model preference content follows the selected English locale");
assert.doesNotMatch(visibleText(), /模型分工|默认模型|规划模型/, "model preferences do not retain Chinese-only labels in English");
const activeSessionModel = document.querySelector<HTMLSelectElement>("#tauri-settings-current-session-model");
assert.ok(activeSessionModel, "the model page exposes a distinct current-session model selector");
await act(async () => { activeSessionModel.value = "demo/new"; activeSessionModel.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
assert.deepEqual(sessionModelChanges, ["demo/new"], "current-session model selection uses its own immediate apply path");
const defaultModelSelect = document.querySelector<HTMLSelectElement>("#tauri-settings-default-model");
assert.ok(defaultModelSelect, "default model has its own settings page");
await act(async () => {
  defaultModelSelect.value = "demo/m";
  defaultModelSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
});
assert.equal(defaultModel, "demo/m", "default model is persisted through the bridge");
const plannerSelect = document.querySelector<HTMLSelectElement>("#tauri-settings-planner-model");
const visionSelect = document.querySelector<HTMLSelectElement>("#tauri-settings-vision-model");
assert.ok(plannerSelect && visionSelect, "role models have controls in model preferences");
await act(async () => { plannerSelect.value = "demo/m"; plannerSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
assert.equal(plannerModel, "demo/m", "planner model is persisted through the bridge");
await act(async () => { visionSelect.value = "auto"; visionSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
assert.equal(visionModel, "auto", "vision routing is persisted through the bridge");
const searchSelect = document.querySelector<HTMLSelectElement>("#tauri-settings-search-model");
assert.ok(searchSelect, "web search has a model assignment control");
await act(async () => { searchSelect.value = "demo/m"; searchSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
assert.equal(webSearchModel, "demo/m", "web search model is persisted through the bridge");
const projectScope = document.querySelector<HTMLButtonElement>('[aria-label="Model preference scope"] [role="radio"][aria-checked="false"]');
assert.ok(projectScope, "model preferences can be scoped to the current workspace");
await act(async () => { projectScope.click(); await new Promise(resolve => setTimeout(resolve, 0)); });
assert.equal(document.querySelector<HTMLSelectElement>("#tauri-settings-default-model")?.value, "demo/new", "switching to project scope loads that workspace's effective model preferences");
const projectDefaultModelSelect = document.querySelector<HTMLSelectElement>("#tauri-settings-default-model");
assert.ok(projectDefaultModelSelect);
await act(async () => { projectDefaultModelSelect.value = "demo/m"; projectDefaultModelSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
assert.equal(projectModelPreferences.defaultModel, "demo/m", "project default model is persisted independently");
assert.equal(defaultModel, "demo/m", "saving a project default does not change the global default");
assert.equal(lastModelPreferenceRequest?.scope, "project", "project model writes include their scope");
assert.equal(lastModelPreferenceRequest?.workspaceRoot, "/preview/project", "project model writes are bound to the active workspace");
const projectPlannerSelect = document.querySelector<HTMLSelectElement>("#tauri-settings-planner-model");
assert.ok(projectPlannerSelect);
await act(async () => { projectPlannerSelect.value = "demo/m"; projectPlannerSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
assert.equal(projectModelPreferences.plannerModel, "demo/m", "project role model saves through the scoped bridge request");
assert.equal(plannerModel, "demo/m", "saving a project role model does not change the global role model");
await act(async () => { click("Chinese"); });
assert.equal(projectModelPreferences.reasoningLanguage, "zh", "project agent preferences save through the project scope");
assert.equal(reasoningLanguage, "auto", "saving a project agent preference does not change the global preference");
const globalScope = document.querySelector<HTMLButtonElement>('[aria-label="Model preference scope"] [role="radio"][aria-checked="false"]');
assert.ok(globalScope);
await act(async () => { globalScope.click(); });
const advancedModelAssignments = document.querySelector<HTMLDetailsElement>(".tauri-model-settings__advanced");
assert.ok(advancedModelAssignments, "advanced subagent assignments are grouped under the model preferences page");
assert.equal(advancedModelAssignments.open, false, "advanced assignments stay collapsed by default");
await act(async () => { advancedModelAssignments.open = true; });
const defaultSubagentModel = advancedModelAssignments.querySelector<HTMLSelectElement>("select[aria-label='Default subagent model']");
assert.ok(defaultSubagentModel, "advanced assignments expose the saved default subagent model");
await act(async () => { defaultSubagentModel.value = "demo/m"; defaultSubagentModel.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
assert.equal(subagents.subagentModel, "demo/m", "advanced model assignment saves through the existing authenticated settings bridge");
const ratioInput = document.querySelector<HTMLInputElement>("#tauri-settings-compact-ratio-custom");
assert.ok(document.querySelector('[role="radiogroup"][aria-label="Thinking language"]') && ratioInput, "agent language and compaction settings are exposed with real controls");
await act(async () => { click("Chinese"); });
assert.equal(reasoningLanguage, "zh", "reasoning language is persisted through the bridge");
await act(async () => {
  ratioInput.focus();
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(ratioInput, "75.5");
  ratioInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  ratioInput.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
});
assert.equal(compactRatioPercent, 75.5, "fractional automatic compaction threshold is persisted through the bridge");
assert.ok(calls.includes("set_agent_preferences"), "agent preferences use the authenticated Tauri bridge command");
await act(async () => { click(["General", "通用"]); });
await act(async () => { click("中文"); });
await act(async () => { click(["用量统计", "Usage stats"]); });
assert.ok(document.querySelector(".usage-stats"), "Preview renders the stable usage chart panel");
assert.ok(calls.includes("usage_stats"), "usage statistics read through the Tauri bridge");
await act(async () => { click(["权限", "Permissions"]); });
assert.match(visibleText(), /默认写入决策/, "permission editor is available in settings");
await act(async () => { [...document.querySelectorAll<HTMLButtonElement>('.tauri-permissions-settings [role="radio"]')].find(button => /deny|阻止写操作/.test(button.textContent ?? ""))?.click(); });
assert.equal(permissions.mode, "deny", "writer fallback mode is persisted through the bridge");
const secretProtectionChecks = [...document.querySelectorAll<HTMLInputElement>(".tauri-permissions-settings .tauri-settings-checkbox-field input")];
assert.equal(secretProtectionChecks.length, 2, "sensitive data protections are exposed alongside permissions");
await act(async () => { secretProtectionChecks[0].click(); });
assert.equal(secretsSettings.filterSubprocessEnv, true, "credential environment filtering persists through the bridge");
await act(async () => { secretProtectionChecks[1].click(); });
assert.equal(secretsSettings.protectSensitiveFiles, true, "credential file protection persists through the bridge");
assert.ok(calls.includes("change_secrets_settings"), "sensitive data protection uses the authenticated settings bridge");
const denyRuleInput = document.querySelector<HTMLInputElement>('input[aria-label="新增阻止规则"], input[aria-label="Add Block rule"], input[aria-label="新增阻止規則"]');
assert.ok(denyRuleInput);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(denyRuleInput, "Bash(rm:*)");
  denyRuleInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("添加"); });
assert.deepEqual(permissions.deny, ["Bash(rm:*)"], "permission rule is persisted through the bridge");
await act(async () => { click(["通用", "General"]); });
await act(async () => { click("English"); });
await act(async () => { click(["Permissions", "权限"]); });
assert.match(visibleText(), /Default writer decision/, "permission settings follow the active English locale");
assert.ok(document.querySelector<HTMLInputElement>('input[aria-label="Add Block rule"]'), "permission rule labels are localized for assistive technology");
await act(async () => { click(["General", "通用"]); });
await act(async () => { click("中文"); });
await act(async () => { click(["权限", "Permissions"]); });
await act(async () => { click(["沙盒", "沙箱", "Sandbox"]); });
assert.match(visibleText(), /沙箱与工作区/, "sandbox editor is available in settings");
assert.equal(sandboxQueryRoot, "/preview/project", "sandbox effective roots are queried for the current workspace");
assert.match(visibleText(), /实际可写根/, "sandbox shows core-computed write roots");
assert.match(visibleText(), /Shell 检测|Shell 偵測/, "sandbox reports the detected host shell inventory");
assert.match(visibleText(), /\/bin\/bash/, "sandbox shows the resolved executable path from the host capability inventory");
assert.match(visibleText(), /\/usr\/bin\/git/, "sandbox shows Git from the same host capability inventory");
assert.equal(sandboxQuerySession, "session-preview", "sandbox requests the bound shell for the exact active session");
assert.match(visibleText(), /当前会话使用/, "sandbox displays the active controller's bound shell");
assert.match(visibleText(), /重新加载当前会话/, "sandbox offers the stable-style reload action when the saved shell differs");
const shellSelect = document.querySelector<HTMLSelectElement>('select[aria-label="Shell 解释器"], select[aria-label="Shell interpreter"]');
assert.ok(shellSelect, "shell preference is shown");
await act(async () => { shellSelect.value = "bash"; shellSelect.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
await act(async () => { click(["不受限运行", "不受限執行", "Unconfined"]); });
await act(async () => { click("保存沙盒设置"); });
assert.equal(sandbox.bash, "off", "sandbox mode is persisted through the bridge");
assert.equal(sandbox.shell, "bash", "shell preference is persisted through the bridge");
await act(async () => { click(["通用", "General"]); });
await act(async () => { click("English"); });
await act(async () => { click(["Sandbox", "沙盒", "沙箱"]); });
assert.match(visibleText(), /Shell interpreter/, "sandbox settings follow the selected English locale");
assert.ok(document.querySelector<HTMLInputElement>('input[aria-label="Add writable directory"]'), "sandbox directory controls localize assistive labels");
await act(async () => { click(["General", "通用"]); });
await act(async () => { click("中文"); });
await act(async () => { click(["网络", "Network"]); });
assert.match(visibleText(), /代理模式/, "network editor is available in settings");
await act(async () => { click("关闭"); });
await act(async () => { click("保存网络设置"); });
assert.equal(network.proxyMode, "off", "proxy mode is persisted through the bridge");
network.proxyUrlSet = true;
network.proxyPasswordSet = true;
await act(async () => { click(["通用", "General"]); });
await act(async () => { click("English"); });
await act(async () => { click(["Network", "网络"]); });
assert.match(visibleText(), /Direct skips proxies; custom uses fields below/, "network mode guidance follows the selected English locale");
await act(async () => { click("custom"); });
assert.ok(document.querySelector<HTMLInputElement>('input[aria-label="Proxy URL"]'), "network secret controls localize their accessible labels");
assert.ok(document.querySelector<HTMLInputElement>('input[aria-label="Password"]'), "proxy password accessible label follows the active locale");
assert.equal(document.querySelector<HTMLInputElement>('input[aria-label="Proxy URL"]')?.getAttribute("placeholder"), "Configured (hidden)", "configured proxy URL stays hidden while its status is localized");
assert.equal(document.querySelector<HTMLInputElement>('input[aria-label="Password"]')?.getAttribute("placeholder"), "Configured (hidden)", "configured proxy password stays hidden while its status is localized");
await act(async () => { click(["通用", "General"]); });
await act(async () => { click("English"); });
await act(async () => { click(["Agent Skills", "Agent Skills"]); });
assert.match(visibleText(), /Install skills/, "skill controls follow the selected English locale");
assert.match(visibleText(), /Skill sources/, "skill source inventory follows the selected English locale");
assert.ok(document.querySelector('input[aria-label="Skill installation source"]'), "skill install source has a localized accessible name");
await act(async () => { click(["General", "通用"]); });
await act(async () => { click("中文"); });
await act(async () => { click(["Agent Skills", "Agent Skills"]); });
assert.match(visibleText(), /A test skill/, "discovered skills appear in settings");
assert.match(visibleText(), /1 个技能 · 可读取/, "source inventory count and status are shown");
const sourceInventoryToggle = document.querySelector<HTMLButtonElement>('[data-skill-source="/tmp/skills"]');
assert.ok(sourceInventoryToggle, "a skill source can expand its discovered entries");
await act(async () => { sourceInventoryToggle.click(); });
assert.match(document.querySelector(".tauri-skill-source-inventory__items")?.textContent ?? "", /A test skill/, "source expansion browses the skills found under that directory");
assert.match(visibleText(), /声明依赖：mcp-server:github/, "skill capabilities retain their declared requirement IDs");
assert.match(visibleText(), /当前会话已就绪：mcp-server:github/, "a configured connected server is identified as ready for this matching session");
assert.ok(capabilityDiagnosticsCalls.some(call => call.workspaceRoot === "/preview/project" && call.includeSessionRuntime === true), "skill readiness is checked against the current session Host without starting MCP");
const implicitSwitch = document.querySelector<HTMLInputElement>('input[aria-label="允许自动调用技能"]');
assert.ok(implicitSwitch);
await act(async () => { implicitSwitch.click(); });
assert.equal(skills.allowImplicitInvocation, false, "implicit skill invocation is persisted through the bridge");
assert.equal(lastSkillScope, "global", "skill settings default to global scope");
await act(async () => { click("当前项目"); });
await act(async () => { implicitSwitch.click(); });
assert.equal(lastSkillScope, "project", "project skill settings use the selected workspace scope");
await act(async () => { click("全局"); });
assert.equal(implicitSwitch.checked, false, "global scope shows its saved value after a project override");
failSkills = true;
await act(async () => { click("刷新发现结果"); });
assert.match(visibleText(), /skill inventory unavailable/, "failed refresh shows the read error");
assert.doesNotMatch(visibleText(), /A test skill/, "failed refresh does not keep a stale inventory");
await act(async () => { click("重试读取"); });
assert.match(visibleText(), /A test skill/, "retry loads the current inventory");
const skillInstallSource = document.querySelector<HTMLInputElement>('input[aria-label="技能安装来源"]');
assert.ok(skillInstallSource);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(skillInstallSource, "/preview/install-skill");
  skillInstallSource.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("预览技能安装"); });
assert.match(visibleText(), /reviewed-skill/, "skill name and destination are reviewed before installation");
assert.equal(installedSkillSource, "", "review does not install a skill");
await act(async () => { click("安装已预览技能"); });
assert.equal(installedSkillSource, "/preview/install-skill", "reviewed source reaches the native installer");
assert.equal(installedSkillScope, "global", "skill install uses the selected scope");
await act(async () => { click("移入备份区"); });
assert.equal(archivedSkillRevision, "", "skill removal requires an explicit second click");
await act(async () => { click("确认备份移除"); });
assert.equal(archivedSkillRevision, "sha256:reviewed-revision", "removal carries the reviewed directory revision");
assert.match(visibleText(), /已备份的技能/, "the backed-up skill remains discoverable for restoration");
await act(async () => { click("当前项目"); });
assert.doesNotMatch(visibleText(), /已备份的技能/, "global backups are hidden while editing the project scope");
await act(async () => { click("全局"); });
await act(async () => { click("恢复"); });
assert.equal(restoredSkillArchiveId, "reviewed-skill--aabbccddeeffaabbccddeeff", "restore targets the selected backup identity");
assert.doesNotMatch(visibleText(), /已备份的技能/, "restored skill leaves the backup list");
await act(async () => { click(["插件", "Plugins"]); });
assert.match(visibleText(), /A test plugin/, "installed plugin is listed in settings");
assert.match(visibleText(), /1 技能/, "plugin contribution counts are shown");
await act(async () => { click(["通用", "General"]); });
await act(async () => { click("English"); });
await act(async () => { click(["Plugins", "插件"]); });
assert.match(visibleText(), /Installed plugins/, "plugin settings follow the selected English locale");
assert.match(visibleText(), /Review plugin packages/, "plugin guidance follows the selected English locale");
assert.ok(document.querySelector('input[aria-label="Search plugins"]'), "plugin search has a localized accessible name");
await act(async () => { click(["General", "通用"]); });
await act(async () => { click("中文"); });
await act(async () => { click(["插件", "Plugins"]); });
const pluginSwitch = document.querySelector<HTMLInputElement>('input[aria-label="启用插件 sample"]');
assert.ok(pluginSwitch);
await act(async () => { pluginSwitch.click(); });
assert.equal(plugins.plugins[0].enabled, false, "plugin activation is saved through the bridge");
const pluginSource = document.querySelector<HTMLInputElement>('#tauri-plugin-source');
assert.ok(pluginSource);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(pluginSource, "/preview/plugin-source");
  pluginSource.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("预览安装"); });
assert.match(visibleText(), /完整信任运行时/, "high risk plugin plan is shown before installation");
const installButton = [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent?.trim() === "安装已预览插件");
assert.ok(installButton?.disabled, "high risk plan requires explicit acknowledgement");
const riskAcknowledgement = document.querySelector<HTMLInputElement>('.tauri-plugin-plan__ack input');
assert.ok(riskAcknowledgement);
await act(async () => { riskAcknowledgement.click(); });
await act(async () => { click("安装已预览插件"); });
assert.equal(installedPluginSource, "/preview/plugin-source", "approved plugin source is sent to the bridge");
await act(async () => { click("移除"); });
assert.match(visibleText(), /确认移除/, "plugin removal requires a second explicit click");
await act(async () => { click("确认移除"); });
assert.equal(removedPluginName, "sample", "plugin removal targets the reviewed registration");
await act(async () => { click(["通用", "General"]); });
await act(async () => { click("English"); });
await act(async () => { click(["Subagents", "子智能体"]); });
assert.match(visibleText(), /Runtime defaults/, "subagent defaults follow the selected English locale");
assert.match(visibleText(), /Discoverable subagents/, "subagent profiles follow the selected English locale");
assert.ok(document.querySelector('input[aria-label="Search subagents"]'), "subagent search has a localized accessible name");
await act(async () => { click(["General", "通用"]); });
await act(async () => { click("中文"); });
await act(async () => { click(["子智能体", "Subagents"]); });
assert.match(visibleText(), /Reviews code/, "discoverable subagent profiles appear in settings");
await act(async () => { click("1 层"); });
assert.equal(subagents.maxDepth, 1, "subagent delegation depth is persisted through the bridge");
await act(async () => { click("新建档案"); });
const setProfileField = async (selector: string, value: string) => {
  const input = document.querySelector<HTMLInputElement | HTMLTextAreaElement>(selector);
  assert.ok(input, `missing profile field: ${selector}`);
  await act(async () => {
    const proto = input instanceof dom.window.HTMLTextAreaElement ? dom.window.HTMLTextAreaElement.prototype : dom.window.HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(input, value);
    input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  });
};
await setProfileField('input[aria-label="档案名称"]', "preview-review");
await setProfileField('input[aria-label="档案说明"]', "Review changes");
await setProfileField('textarea[aria-label="档案系统提示词"]', "Review carefully.");
await act(async () => { click("保存档案"); });
assert.equal(savedSubagentProfile?.action, "create_profile", "profile editor submits a create action");
assert.equal(savedSubagentProfile?.scope, "project", "new profile defaults to the current workspace");
assert.deepEqual(savedSubagentProfile?.profile, { name: "preview-review", description: "Review changes", systemPrompt: "Review carefully.", color: "", model: "", effort: "", allowedTools: [], readOnly: false }, "profile content reaches the bridge");
await act(async () => { click("Hooks"); });
assert.match(visibleText(), /配置文件/, "hooks editor displays its source path");
await act(async () => { click(["通用", "General"]); });
await act(async () => { click("English"); });
await act(async () => { click(["Hooks", "钩子"]); });
assert.match(visibleText(), /Configuration scope/, "Hooks settings follow the selected English locale");
assert.match(visibleText(), /Format and validate/, "Hooks editor actions follow the selected English locale");
assert.match(visibleText(), /Hooks from installed plugins/, "Hooks page shows imported plugin hook diagnostics");
assert.match(visibleText(), /PreToolUse.*Bash/s, "plugin hook event and matcher are visible");
assert.match(visibleText(), /Review shell commands/, "plugin hook description is visible");
assert.doesNotMatch(visibleText(), /private-command|secret-value/, "plugin hook command strings are not shown");
assert.ok(document.querySelector('textarea[aria-label="Hooks JSON"]'), "Hooks editor has a localized accessible name");
const pluginJump = document.querySelector<HTMLButtonElement>(".tauri-hooks-plugin-heading .tauri-settings-actions button:last-child");
assert.ok(pluginJump);
await act(async () => { pluginJump.click(); });
assert.match(visibleText(), /Installed plugins|已安装插件/, "plugin hook inventory links to plugin management");
await act(async () => { click(["General", "通用"]); });
await act(async () => { click("English"); });
await act(async () => { click(["Hooks", "钩子"]); });
await act(async () => { click(["General", "通用"]); });
await act(async () => { click("中文"); });
await act(async () => { click(["钩子", "Hooks"]); });
Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: async () => { throw new Error("WebView clipboard unavailable"); } } });
await act(async () => { click("复制路径"); });
assert.equal(copiedHooksPath, "/preview/settings.json", "hooks path copy uses the current bridge path");
const hooksEditor = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="Hooks JSON"]');
assert.ok(hooksEditor);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(hooksEditor, '{"hooks":{"Stop":[{"command":"echo done"}]}}');
  hooksEditor.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("保存 Hooks"); });
assert.deepEqual(hooks.hooks, { Stop: [{ command: "echo done" }] }, "hooks JSON is saved through the bridge");
await act(async () => { click(["模型服务", "Model services"]); });
assert.match(visibleText(), /已就绪/, ".env credential configures the provider");
assert.match(visibleText(), /服务配置/, "provider configuration is editable from settings");
await act(async () => { enterKey("one-time-probe-key"); });
await act(async () => { click("测试连接"); });
assert.deepEqual(lastProbeRequest, { name: "demo", model: "m", apiKey: "one-time-probe-key" }, "provider probe sends the selected model and optional key only for this request");
assert.match(visibleText(), /连接成功 · 123 ms/, "successful provider probe reports latency");
assert.equal(document.querySelector<HTMLInputElement>(".tauri-settings-apikey input")?.value, "one-time-probe-key", "testing does not silently save or clear the draft key");
await act(async () => { click("编辑"); });
assert.match(visibleText(), /余额查询 URL/, "provider editor exposes the optional balance endpoint");
await act(async () => { click("取消"); });
await act(async () => { click("更新"); });
assert.match(visibleText(), /Preview 版本|Preview version/, "updates page shows the current Preview release");
assert.match(visibleText(), /尚未配置更新验证公钥|no updater verification key is configured/, "updates page states why signed self-update is unavailable");
await act(async () => { click("前往官网下载"); });
assert.equal(openedURL, "https://reasonix.io/?download=desktop#start", "official download opens through the native URL handler");
await act(async () => { click(["通用", "General"]); });
await act(async () => { click("English"); });
await act(async () => { click(["Model services", "模型服务"]); });
assert.match(visibleText(), /Provider presets/, "provider presets follow the selected locale");
assert.match(visibleText(), /Service configuration/, "provider editor follows the selected locale");
assert.ok(document.querySelector('input[aria-label="Search provider presets"]'), "preset search has a localized accessible label");
await act(async () => { click(["General", "通用"]); });
await act(async () => { click("中文"); });
await act(async () => { click(["模型服务", "Model services"]); });
assert.match(visibleText(), /服务配置/, "provider settings return to Simplified Chinese");
const presetButton = [...document.querySelectorAll<HTMLButtonElement>(".tauri-provider-preset")].find(button => button.textContent?.includes("MiMo API"));
assert.ok(presetButton);
await act(async () => { presetButton.click(); });
assert.match(visibleText(), /api\.xiaomimimo\.com\/v1/, "preset endpoint is reviewed before installation");
await act(async () => { click("添加预设"); });
assert.equal(savedPresetId, "mimo-api", "reviewed preset ID reaches the bridge");
await act(async () => { presetButton.click(); });
await act(async () => { click("恢复预设配置"); });
assert.notEqual(savedPresetAction, "reset", "reset requires another explicit click");
await act(async () => { click("确认恢复预设"); });
assert.equal(savedPresetAction, "reset", "preset reset reaches the bridge after confirmation");
await act(async () => { click("编辑"); });
assert.equal(document.querySelector<HTMLInputElement>('.tauri-provider-editor-form input[placeholder="留空保留现有端点"]')?.value, "", "existing endpoint is not sent back to the WebView");
await act(async () => { click("取消"); });
await act(async () => { click("添加服务"); });
const providerNameInput = document.querySelector<HTMLInputElement>('.tauri-provider-editor-form input[placeholder="例如 my-provider"]');
const providerURLInput = document.querySelector<HTMLInputElement>('.tauri-provider-editor-form input[placeholder="https://example.com/v1"]');
const providerBalanceURLInput = document.querySelectorAll<HTMLInputElement>(".tauri-provider-editor-form label input")[3];
const providerModelsInput = document.querySelector<HTMLTextAreaElement>('.tauri-provider-editor-form textarea');
const providerProtocolInput = document.querySelector<HTMLSelectElement>('.tauri-provider-editor-form label select');
assert.ok(providerNameInput && providerURLInput && providerBalanceURLInput && providerModelsInput);
assert.ok(providerProtocolInput);
const providerAdvanced = document.querySelector<HTMLDetailsElement>(".tauri-provider-advanced");
assert.ok(providerAdvanced);
await act(async () => { providerAdvanced.querySelector("summary")?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true })); });
const contextWindowInput = providerAdvanced.querySelector<HTMLInputElement>('input[type="number"]');
const noProxyInput = providerAdvanced.querySelector<HTMLInputElement>('input[type="checkbox"]');
assert.ok(contextWindowInput && noProxyInput);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(providerNameInput, "new-provider");
  providerNameInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(providerURLInput, "https://provider.example/v1");
  providerURLInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(providerBalanceURLInput, "https://provider.example/account/balance");
  providerBalanceURLInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(providerModelsInput, "chat-model");
  providerModelsInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(contextWindowInput, "65536");
  contextWindowInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  noProxyInput.click();
  Object.getOwnPropertyDescriptor(dom.window.HTMLSelectElement.prototype, "value")!.set!.call(providerProtocolInput, "responses");
  providerProtocolInput.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
});
const responsesModeInput = providerAdvanced.querySelector<HTMLSelectElement>("select");
const modelsEndpointInput = [...providerAdvanced.querySelectorAll<HTMLInputElement>('input:not([type="checkbox"]):not([type="number"])')][0];
assert.ok(responsesModeInput && modelsEndpointInput);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLSelectElement.prototype, "value")!.set!.call(responsesModeInput, "stateful");
  responsesModeInput.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(modelsEndpointInput, "https://provider.example/models");
  modelsEndpointInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("保存服务"); });
assert.equal(savedProviderInput?.name, "new-provider", "provider creation reaches the native command");
assert.equal(savedProviderInput?.models[0], "chat-model", "model IDs reach the native command");
assert.equal(savedProviderInput?.balanceUrl, "https://provider.example/account/balance", "balance endpoint reaches the native command");
assert.equal(savedProviderInput?.contextWindow, 65536, "provider context window reaches the native command");
assert.equal(savedProviderInput?.noProxy, true, "provider proxy bypass setting reaches the native command");
assert.equal(savedProviderInput?.kind, "responses", "the selected provider protocol reaches the native command");
assert.equal(savedProviderInput?.responsesMode, "stateful", "Responses API context mode reaches the native command");
assert.equal(savedProviderInput?.modelsUrl, "https://provider.example/models", "provider-specific model discovery endpoint reaches the native command");
assert.equal(savedProviderInput?.useApiKey, true, "remote services default to credential support");
await act(async () => { enterKey("secret-input-value"); });

mock.timers.enable({ apis: ["setTimeout"] });
saveGate = new Promise(resolve => { releaseSave = resolve; });
await act(async () => {
  click("保存到钥匙串");
  click("保存到钥匙串");
  click("删除");
});
assert.equal(calls.filter(call => call === "keychain_save").length, 1, "repeated save starts one operation");
assert.equal(calls.filter(call => call === "keychain_delete").length, 0, "delete cannot race with save");
assert.match(visibleText(), /正在处理钥匙串请求.*完成或取消/, "pending native authorization has recovery guidance");
await act(async () => { mock.timers.tick(60_000); });
assert.match(visibleText(), /正在处理钥匙串请求/, "pending guidance does not expire before the native operation completes");
assert.equal(document.querySelector<HTMLInputElement>(".tauri-settings-apikey input")?.value, "secret-input-value", "pending native authorization preserves the draft");
assert.equal(calls.filter(call => call === "keychain_save").length, 1, "waiting never retries or duplicates a native write");
await act(async () => { releaseSave?.(); });
saveGate = null;
assert.match(visibleText(), /已保存到钥匙串/);
assert.doesNotMatch(visibleText(), /正在处理钥匙串请求/, "completion replaces pending guidance");
assert.equal(document.querySelector<HTMLInputElement>(".tauri-settings-apikey input")?.value, "", "successful save clears the entered secret");
assert.equal(calls.filter(call => call === "provider_summary").length, 6, "save refreshes the provider summary");
assert.equal(parentConfigured, true, "the model picker outside settings receives the saved status");
assert.match(visibleText(), /应用到当前会话/, "saved key offers an explicit active-session update");
assert.equal(document.querySelector(".tauri-settings-apikey-status")?.getAttribute("role"), "status");
await act(async () => { mock.timers.tick(2001); });
assert.doesNotMatch(visibleText(), /已保存到钥匙串/, "ordinary success still expires");
mock.timers.reset();
await act(async () => { click("应用到当前会话"); });
assert.equal(applyCalls, 1, "active-session update is user initiated");
assert.doesNotMatch(visibleText(), /应用到当前会话/, "successful update clears the pending action");

await act(async () => { click(["删除", "Delete"]); });
assert.match(visibleText(), /钥匙串密钥已删除；其他凭据仍可用/, "delete reports the keychain scope when .env remains");
assert.match(visibleText(), /已就绪/, "refreshed provider remains configured by .env");
assert.equal(calls.filter(call => call === "provider_summary").length, 7, "delete refreshes the provider summary");
assert.equal(parentConfigured, true, "the model picker outside settings keeps the .env readiness");

await act(async () => { click("删除"); });
assert.match(visibleText(), /钥匙串中没有密钥/, "repeated delete does not claim a key was removed");

await act(async () => { enterKey("secret-input-value"); });
mock.timers.enable({ apis: ["setTimeout"] });
failSummary = true;
await act(async () => { click("保存到钥匙串"); });
assert.match(visibleText(), /已保存到钥匙串；配置状态刷新失败/, "refresh failure does not misreport a successful save");
await act(async () => { mock.timers.tick(2001); });
assert.match(visibleText(), /已保存到钥匙串；配置状态刷新失败/, "partial success recovery instructions remain visible");
assert.match(visibleText(), /关闭并重新打开设置/);
mock.timers.reset();
assert.doesNotMatch(visibleText(), /secret-input-value|secret-in-error-message/, "status never includes secrets");

await act(async () => { enterKey("secret-input-value"); });
mock.timers.enable({ apis: ["setTimeout"] });
failSave = true;
await act(async () => { click("保存到钥匙串"); });
assert.match(visibleText(), /保存失败/);
await act(async () => { mock.timers.tick(2001); });
assert.match(visibleText(), /保存失败/, "credential failure stays visible after the success timeout");
assert.equal(document.querySelector(".tauri-settings-apikey-status")?.getAttribute("role"), "alert", "credential refusal is announced as an error");
mock.timers.reset();
assert.doesNotMatch(visibleText(), /secret-in-error-message/, "error details do not expose secrets");

// Import is explicit, never transmits a credential, and keeps an unsaved input.
failSave = false;
const summaryCountBeforeImport = calls.filter(call => call === "provider_summary").length;
await act(async () => { click("迁移旧凭据"); });
assert.equal(keyPresent, true, "import refusal preserves an already saved credential");
assert.match(visibleText(), /已有钥匙串凭据.*迁移不会覆盖它/);
assert.match(visibleText(), /填写新密钥并保存到钥匙串/);
assert.doesNotMatch(visibleText(), /系统授权/, "existing credential refusal does not imply an OS permission failure");
assert.doesNotMatch(visibleText(), /密钥保存失败/, "the next explicit action replaces previous failure feedback");
assert.equal(calls.filter(call => call === "provider_summary").length, summaryCountBeforeImport, "failed migration does not report a saved key");
assert.doesNotMatch(visibleText(), /secret-in-migration-error/, "migration errors hide sensitive host diagnostics");
assert.equal(document.querySelector<HTMLInputElement>(".tauri-settings-apikey input")?.value, "secret-input-value", "failed migration preserves the entered value");
await act(async () => { click("删除"); });
await act(async () => { click("应用到当前会话"); });
const saveCountBeforeImport = calls.filter(call => call === "keychain_save").length;
const deleteCountBeforeImport = calls.filter(call => call === "keychain_delete").length;
const importCountBeforeGate = importRequests.length;
saveGate = new Promise(resolve => { releaseSave = resolve; });
await act(async () => {
  click("迁移旧凭据");
  click("迁移旧凭据");
  click("保存到钥匙串");
  click("删除");
});
assert.equal(importRequests.length, importCountBeforeGate + 1, "repeated migration starts one operation");
assert.equal(calls.filter(call => call === "keychain_save").length, saveCountBeforeImport, "save cannot race with migration");
assert.equal(calls.filter(call => call === "keychain_delete").length, deleteCountBeforeImport, "delete cannot race with migration");
assert.deepEqual(importRequests.at(-1), { provider: "demo" }, "only the selected provider identity crosses IPC");
await act(async () => { releaseSave?.(); });
saveGate = null;
assert.equal(keyPresent, true, "migration updates native credential readiness");
assert.equal(document.querySelector<HTMLInputElement>(".tauri-settings-apikey input")?.value, "secret-input-value", "migration never discards an unsaved input");
assert.match(visibleText(), /已保存到钥匙串/);
assert.match(visibleText(), /应用到当前会话/, "migrated credential offers the active-session update");
await act(async () => { click("删除"); });
failImport = true;
await act(async () => { click("迁移旧凭据"); });
assert.equal(keyPresent, false, "failed import leaves the destination empty");
assert.match(visibleText(), /迁移失败/);
assert.doesNotMatch(visibleText(), /secret-in-migration-error/);
failImport = false;
missingLegacy = true;
const summaryBeforeMissing = calls.filter(call => call === "provider_summary").length;
await act(async () => { click("迁移旧凭据"); });
assert.match(visibleText(), /未找到旧 Preview 凭据.*填写密钥并保存到钥匙串/);
assert.equal(keyPresent, false);
assert.equal(calls.filter(call => call === "provider_summary").length, summaryBeforeMissing);
assert.equal(document.querySelector<HTMLInputElement>(".tauri-settings-apikey input")?.value, "secret-input-value");
missingLegacy = false;

await act(async () => { click(["运行诊断", "诊断", "Diagnostics"]); });
assert.match(visibleText(), /持久身份目录/, "diagnostics show the actual sidebar data source");
assert.match(visibleText(), /运行中 · 协议 v1/, "diagnostics show bridge status");
assert.match(visibleText(), /能力诊断/, "diagnostics include the shared capability report");
assert.ok(capabilityDiagnosticsCalls.some(call => call.workspaceRoot === "/preview/project" && call.includeSessionRuntime === false), "capability diagnostics use the selected workspace and remain static until runtime is requested");
assert.ok(calls.includes("runtime_doctor"), "diagnostics also load the shared process-wide Runtime Doctor report");
await act(async () => { click("通用"); });
await act(async () => { click("English"); });
await act(async () => { click("Diagnostics"); });
assert.match(visibleText(), /Persistent identity catalog/, "diagnostics labels follow the active language");
const diagnosticsControl = document.querySelector<HTMLElement>('[data-testid="frontend-diagnostics"]');
assert.ok(diagnosticsControl, "Tauri diagnostics expose the privacy-filtered frontend recorder");
await act(async () => { diagnosticsControl?.querySelector<HTMLButtonElement>(".frontend-diagnostics__toggle")?.click(); });
assert.equal(diagnosticsControl?.querySelector(".frontend-diagnostics__toggle")?.getAttribute("aria-checked"), "true", "the frontend recorder can start in Preview");
await act(async () => { document.querySelector<HTMLElement>('[data-testid="frontend-diagnostics"] .frontend-diagnostics__toggle')?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true })); await Promise.resolve(); });
assert.ok(calls.includes("export_frontend_diagnostics"), "the Tauri recorder exports through its native save command");
assert.equal((exportedFrontendDiagnostics as { schemaVersion?: number } | undefined)?.schemaVersion, 2, "the exported trace preserves the sanitized diagnostics schema");
assert.doesNotMatch(JSON.stringify(exportedFrontendDiagnostics), /secret-input-value|PRIVATE_TEXT|PRIVATE_TOKEN/, "Tauri export does not include form values or private text");
assert.match(visibleText(), /Running · protocol v1/, "bridge status is localized with its protocol value");
await act(async () => { click("General"); });
await act(async () => { click("中文"); });
await act(async () => { click(["运行诊断", "诊断", "Diagnostics"]); });
await act(async () => { click("重新检查"); });
assert.equal(auditRefreshCalls, 1, "diagnostics refresh the catalog audit");
await act(async () => { click("重启桥接服务"); });
assert.equal(restartCalls, 1, "diagnostics restart the existing bridge");
assert.match(visibleText(), /桥接服务已重启/, "restart outcome is shown in settings");

await act(async () => { click("关于"); });
assert.match(visibleText(), /关于 Reasonix Tauri Preview/, "About page uses the locale catalog");
const githubLink = document.querySelector<HTMLAnchorElement>('.tauri-settings-about a[href^="https://github.com/"]')
  ?? document.querySelector<HTMLAnchorElement>('.tauri-settings-actions a[href^="https://github.com/"]');
assert.ok(githubLink, "About page has a GitHub link");
const githubClick = new dom.window.MouseEvent("click", { bubbles: true, cancelable: true });
await act(async () => { githubLink.dispatchEvent(githubClick); });
assert.equal(githubClick.defaultPrevented, true, "GitHub link does not navigate the WebView");
assert.equal(openedURL, "https://github.com/esengine/DeepSeek-Reasonix", "GitHub link opens through the native host");

await act(async () => { root.unmount(); });
failSummary = true;
const retryRoot = createRoot(document.getElementById("root")!);
await act(async () => { retryRoot.render(<LocaleProvider><TauriSettings onClose={() => {}} /></LocaleProvider>); });
assert.match(visibleText(), /桌面风格/, "general settings remain available when provider loading fails");
await act(async () => { click(["模型服务", "Model services"]); });
assert.match(visibleText(), /读取设置失败/, "failed provider loading is actionable");
await act(async () => { click("重试"); });
assert.match(visibleText(), /已就绪/, "retry recovers the model settings");
await act(async () => { click("关于"); });
failRuntime = true;
await act(async () => { click("刷新"); });
assert.match(visibleText(), /读取设置失败/, "runtime refresh failure is visible");
await act(async () => { click(["模型服务", "Model services"]); });
assert.match(visibleText(), /已就绪/, "runtime failure does not block model settings");
await act(async () => { retryRoot.unmount(); });
const memoryRoot = createRoot(document.getElementById("root")!);
await act(async () => { memoryRoot.render(<LocaleProvider><TauriSettings initialTab="memory" workspaceRoot="/preview/project" onClose={() => {}} /></LocaleProvider>); });
assert.match(visibleText(), /Old instruction/, "memory document is read through the bridge");
assert.match(visibleText(), /language preference/, "latest automatic memory recall is visible");
assert.match(visibleText(), /Reply in Chinese\./, "latest recalled facts and their snippets are shown");
await act(async () => { click(["通用", "General"]); });
await act(async () => { click("English"); });
await act(async () => { click(["Memory", "记忆"]); });
assert.match(visibleText(), /Instruction documents/, "memory settings follow the selected English locale");
assert.match(visibleText(), /Saved facts/, "memory facts follow the selected English locale");
assert.ok(document.querySelector('textarea[aria-label="Memory document"]'), "memory editor has a localized accessible name");
await act(async () => { click(["General", "通用"]); });
await act(async () => { click("中文"); });
await act(async () => { click(["记忆", "Memory"]); });
assert.match(visibleText(), /Project fact/, "saved memory facts are shown");
await act(async () => { click("扫描近期会话"); });
assert.match(visibleText(), /Reply in Chinese by default/, "memory suggestions are explicitly scanned and previewed");
await act(async () => { click("接受并保存为记忆"); });
assert.deepEqual(memorySuggestions.memories, [], "accepted suggestion refreshes from the Preview bridge");
assert.ok(calls.includes("memory_suggestions") && calls.includes("accept_memory_suggestion"), "suggestions use the authenticated Tauri bridge commands");
await act(async () => { click("版本历史"); });
assert.equal(memory.revisions.length, 1, "memory history is loaded through the Preview bridge");
assert.match(visibleText(), /版本 1/, "the prior revision and timestamp are shown");
await act(async () => { click("恢复此版本"); });
assert.equal(memory.facts[0].revision, 3, "restoring a memory revision saves a new active revision");
assert.equal(memory.facts[0].body, "Previous body", "restoring a memory revision restores the selected content");
await act(async () => { click("编辑事实"); });
const factBody = document.querySelector<HTMLTextAreaElement>('.tauri-memory-fact textarea');
assert.ok(factBody, "saved memory fact has a structured editor");
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(factBody, "Edited fact body");
  factBody.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("保存事实"); });
assert.equal(memory.facts[0].revision, 4, "editing a memory fact creates a new revision");
assert.equal(memory.facts[0].body, "Edited fact body", "structured fact edits are sent to the Preview bridge");
const memoryEditor = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="记忆文档"]');
assert.ok(memoryEditor);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLTextAreaElement.prototype, "value")!.set!.call(memoryEditor, "New instruction.");
  memoryEditor.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("保存文档"); });
assert.equal(memory.docs[0].body, "New instruction.", "memory document edit is persisted through the bridge");
const noteInput = document.querySelector<HTMLInputElement>('input[aria-label="记忆笔记"]');
assert.ok(noteInput);
await act(async () => {
  Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(noteInput, "Remember this.");
  noteInput.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
});
await act(async () => { click("追加"); });
assert.equal(memoryNote, "Remember this.", "quick memory note is saved through the bridge");
await act(async () => { memoryRoot.unmount(); });
allowDeleteProvider = true;
(dom.window as unknown as { confirm: () => boolean }).confirm = () => true;
const { TauriProviderEditor } = await import("../tauri/TauriProviderEditor");
const providerRoot = createRoot(document.getElementById("root")!);
await act(async () => { providerRoot.render(<LocaleProvider><TauriProviderEditor onSummaryChange={() => {}} /></LocaleProvider>); });
await act(async () => { click(["删除", "Delete"]); });
assert.equal(deletedProviderName, "demo", "confirmed custom provider deletion reaches the bridge");
assert.equal(deletedProviderRevision, "r1", "deletion carries the configuration revision shown to the user");
assert.match(visibleText(), /模型服务已删除|Model service deleted/, "provider deletion reports the new-session effect");
await act(async () => { providerRoot.unmount(); });
console.log("tauri settings API key flow passed");
