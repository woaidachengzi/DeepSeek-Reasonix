import { useState, useCallback, useEffect, useRef, lazy, Suspense, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from "react";
import { Check, ArrowLeft, Search, X, Keyboard, Globe, Languages, Palette, Info, RefreshCw, ExternalLink, Key, Eye, EyeOff, Server, Database, SlidersHorizontal, Activity, Cable, Monitor, PanelTop, ShieldCheck, Power, Bell, Volume2, Play, ChevronDown, ChartNoAxesColumn, Box, Sparkles, Users, Webhook, Package, Bot } from "lucide-react";
import { tauriPreviewRuntimeInfo, tauriProviderSummary, setTauriDefaultModel, setTauriModelRole, setTauriAgentPreference, testTauriProviderModel, tauriDesktopPreferences, tauriActiveThemeId, setTauriActiveThemeId, tauriUserThemes, tauriPluginThemes, saveTauriUserTheme, deleteTauriUserTheme, importTauriUserTheme, exportTauriUserTheme, setTauriDesktopApproval, setTauriDesktopTerminalTheme, setTauriDesktopAppearance, setTauriDesktopLanguage, setTauriDesktopCurrency, tauriPlatformInfo, getTauriCloseBehavior, setTauriCloseBehavior, tauriZoomFactor, setTauriZoomFactor, keychainSave, keychainDelete, keychainImportLegacy, openTauriExternalURL, tauriMessageFrom, tauriUsageStats, tauriCapabilityDiagnostics, tauriRuntimeDoctor, type TauriToolApprovalMode, type TauriBridgeStatus, type TauriCloseBehavior, type TauriPreviewProfileStatus, type TauriPreviewRuntimeInfo, type TauriProviderSummary, type TauriSessionShadowReport } from "../lib/tauriBridge";
import { applyTerminalThemePreference, normalizeTerminalThemePreference, getCustomTerminalPalette, setCustomTerminalPalette, DEFAULT_TERMINAL_PALETTE, type TerminalPalette, type TerminalPaletteKey, type TerminalThemePreference } from "../lib/terminalTheme";
import { THEME_STYLES, type Theme, type ThemeStyle } from "../lib/theme";
import { useI18n, useT, type DictKey, type LangPref, type Translator } from "../lib/i18n";
import { applyConversationWidth, getCachedConversationWidth, type ConversationWidth } from "../lib/conversationWidth";
import { applyTextSize, getTextSize, TEXT_SIZES, type TextSize } from "../lib/textSize";
import { applyFontFamily, applyMonoFontFamily, FONT_FAMILIES, MONO_FONT_FAMILIES, getCustomFontName, getCustomMonoFontName, getFontFamily, getMonoFontFamily, setCustomFontName, setCustomMonoFontName, type FontFamily, type MonoFontFamily } from "../lib/fontFamily";
import { applyTauriAppearance, readTauriAppearance, type TauriAppearance } from "./tauriAppearance";
import { applyThemePack, clearThemePack, type ThemePackView } from "../lib/themePack";
import { TauriThemeGallery, type UserThemeSaveInput } from "./TauriThemeGallery";
import { tauriThemePackById } from "./tauriThemeCatalog";
import { TauriMCPSettings } from "./TauriMCPSettings";
import { TauriProviderEditor } from "./TauriProviderEditor";
import { TauriPermissionsSettings } from "./TauriPermissionsSettings";
import { TauriSandboxSettings } from "./TauriSandboxSettings";
import { TauriNetworkSettings } from "./TauriNetworkSettings";
import { TauriSkillsSettings } from "./TauriSkillsSettings";
import { TauriPluginSettings } from "./TauriPluginSettings";
import { TauriSubagentSettings } from "./TauriSubagentSettings";
import { TauriHooksSettings } from "./TauriHooksSettings";
import { TauriMemorySettings } from "./TauriMemorySettings";
import { TauriStorageSettings } from "./TauriStorageSettings";
import { TauriRemoteSettings } from "./TauriRemoteSettings";
import { TauriBotSettings } from "./TauriBotSettings";
import { StatusBarItemsEditor } from "../components/StatusBarItemsEditor";
import { TypographySettings } from "../components/TypographySettings";
import { SettingsSection } from "../components/SettingsForm";
import { SettingsOptions } from "../components/SettingsOptions";
import { comboFromKeyboardEvent, detectShortcutPlatform, formatShortcutCombo } from "../lib/keyboardShortcuts";
import { TAURI_SHORTCUT_ACTIONS, getTauriShortcut, isValidTauriShortcut, resetTauriShortcuts, setTauriShortcut, tauriShortcutConflict, useTauriShortcuts, type TauriShortcutAction } from "./tauriKeyboardShortcuts";
import "../components/SettingsPanel.css";
import "../components/CompactRatioSettings.css";
import { getTauriNotificationsEnabled, getTauriNotificationEvents, setTauriNotificationEvent, setTauriNotificationsEnabled, getTauriProgressMode, setTauriProgressMode, type TauriNotificationKind, type TauriProgressMode } from "./tauriPreferences";
import { TAURI_STATUS_BAR_ITEM_IDS, setTauriStatusBarPreferences, useTauriStatusBarPreferences, type TauriStatusBarItemId } from "./tauriStatusBarPreferences";
import { setTauriDesktopLayout, useTauriDesktopLayout } from "./tauriDesktopLayout";
import { COMPACT_RATIO_MAX_PERCENT, COMPACT_RATIO_MIN_PERCENT } from "../lib/compactRatio";
import { getSuccessPreference, setSuccessPreference, getAttentionPreference, setAttentionPreference, getNotificationVolume, setNotificationVolume, playSuccessChime, playAttentionChime, type SoundWavPref } from "../lib/sound";
import { generativeMusic, getGenerativePreset, setGenerativePreset, type GenerativePreset } from "../lib/generative-music";

interface TauriSettingsProps {
  onClose: () => void;
  onProviderSummaryChange?: (summary: TauriProviderSummary) => void;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionId?: string;
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
  onUseSubagentInChat?: (command: string) => void;
  currentSessionModelRef?: string;
  onCurrentSessionModelChange?: (model: string) => Promise<boolean>;
  workspaceRoot?: string;
  defaultWorkspace?: string;
  onChooseDefaultWorkspace?: () => Promise<string | null>;
  onClearDefaultWorkspace?: () => void;
  initialTab?: TauriSettingsTab;
  profile?: TauriPreviewProfileStatus | null;
  onRefreshProfile?: () => Promise<void>;
  onImportStableProfile?: () => Promise<string>;
  onImportStableProjectFolders?: () => Promise<string>;
  onScanUnclaimedSessions?: () => void;
  importBusy?: boolean;
  bridgeStatus?: TauriBridgeStatus | null;
  catalogAudit?: TauriSessionShadowReport | null;
  catalogAuditError?: string;
  sessionPageSource?: string;
  hostError?: string;
  onRestartBridge?: () => Promise<boolean>;
  onRefreshCatalogAudit?: () => Promise<void>;
}

export type TauriSettingsTab = "general" | "appearance" | "model" | "providers" | "stats" | "bots" | "mcp" | "remote" | "skills" | "subagents" | "plugins" | "hooks" | "memory" | "permissions" | "sandbox" | "network" | "diagnostics" | "data" | "shortcuts" | "updates" | "about";

const TauriUsageStatsPanel = lazy(() => import("../components/UsageStatsPanel").then(module => ({ default: module.UsageStatsPanel })));
const TauriCapabilityDiagnosticsPage = lazy(() => import("../components/DiagnosticsSettingsPage").then(module => ({ default: module.DiagnosticsSettingsPage })));

const SETTINGS_GROUPS = (t: Translator) => [
  { label: t("settings.navGroup.preferences"), items: [{ id: "general", label: t("settings.tab.general"), description: t("settings.tabSub.general"), searchTerms: [
    "settings.desktopLayoutStyle", "settings.language", "settings.currency", "settings.sessionExperience",
    "settings.closeBehavior", "settings.defaultToolApprovalMode", "settings.general.notifications",
    "settings.sound", "settings.statusBarStyle", "settings.statusBarItems",
  ].map(key => t(key as DictKey)).join(" "), icon: SlidersHorizontal }] },
  { label: t("settings.tab.models"), items: [
    { id: "model", label: t("settings.models.preferences"), description: t("settings.tabSub.models"), icon: Globe },
    { id: "providers", label: t("settings.models.services"), description: t("settings.tabSub.providers"), icon: Cable },
    { id: "stats", label: t("settings.modelTab.stats"), description: t("settings.tabSub.models"), icon: ChartNoAxesColumn },
  ] },
  { label: t("settings.navGroup.connections"), items: [{ id: "bots", label: t("settings.tab.bots"), description: t("settings.tabSub.bots"), icon: Bot }, { id: "mcp", label: t("settings.tab.mcp"), description: t("settings.tabSub.mcp"), icon: Server }, { id: "remote", label: t("settings.tab.remote"), description: t("settings.tabSub.remote"), icon: Cable }] },
  { label: t("settings.navGroup.capabilities"), items: [{ id: "skills", label: t("settings.tab.skills"), description: t("settings.tabSub.skills"), icon: Sparkles }, { id: "subagents", label: t("settings.tab.subagents"), description: t("subagents.tabHint"), icon: Users }, { id: "plugins", label: t("settings.tab.plugins"), description: t("settings.tabSub.plugins"), icon: Package }] },
  { label: t("settings.navGroup.context"), items: [{ id: "memory", label: t("settings.tab.memory"), description: t("settings.tabSub.memory"), icon: Database }] },
  { label: t("settings.navGroup.automation"), items: [{ id: "hooks", label: t("settings.tab.hooks"), description: t("settings.tabSub.hooks"), icon: Webhook }, { id: "diagnostics", label: t("settings.tab.diagnostics"), description: t("settings.tabSub.diagnostics"), icon: Activity }] },
  { label: t("settings.navGroup.security"), items: [
    { id: "permissions", label: t("settings.tab.permissions"), description: t("settings.tabSub.permissions"), icon: ShieldCheck },
    { id: "sandbox", label: t("settings.tab.sandbox"), description: t("settings.tabSub.sandbox"), icon: Box },
    { id: "network", label: t("settings.tab.network"), description: t("settings.tabSub.network"), icon: Globe },
  ] },
  { label: t("settings.navGroup.application"), items: [
    { id: "appearance", label: t("settings.tab.appearance"), description: t("settings.tabSub.appearance"), icon: Palette },
    { id: "shortcuts", label: t("settings.tab.shortcuts"), description: t("settings.tabSub.shortcuts"), icon: Keyboard },
    { id: "data", label: t("settings.tab.storage"), description: t("settings.tabSub.storage"), icon: Database },
    { id: "updates", label: t("settings.tab.updates"), description: t("settings.tabSub.updates"), icon: RefreshCw },
    { id: "about", label: t("settings.about.navLabel"), description: t("settings.about.hint"), icon: Info },
  ] },
] as const;

const SETTINGS_TITLES = (t: Translator): Record<TauriSettingsTab, { title: string; description: string }> => ({
  general: { title: t("settings.tab.general"), description: t("settings.tabSub.general") },
  model: { title: t("settings.models.preferences"), description: t("settings.tabSub.models") },
  providers: { title: t("settings.models.services"), description: t("settings.tabSub.providers") },
  stats: { title: t("settings.modelTab.stats"), description: t("settings.tabSub.models") },
  bots: { title: t("settings.tab.bots"), description: t("settings.pageDesc.bots") },
  mcp: { title: t("settings.tab.mcp"), description: t("settings.tabSub.mcp") },
  remote: { title: t("settings.tab.remote"), description: t("settings.tabSub.remote") },
  skills: { title: t("settings.tab.skills"), description: t("settings.tabSub.skills") },
  plugins: { title: t("settings.tab.plugins"), description: t("settings.tabSub.plugins") },
  subagents: { title: t("settings.tab.subagents"), description: t("subagents.tabHint") },
  hooks: { title: t("settings.tab.hooks"), description: t("settings.tabSub.hooks") },
  memory: { title: t("settings.tab.memory"), description: t("settings.tabSub.memory") },
  permissions: { title: t("settings.tab.permissions"), description: t("settings.tabSub.permissions") },
  sandbox: { title: t("settings.tab.sandbox"), description: t("settings.tabSub.sandbox") },
  network: { title: t("settings.tab.network"), description: t("settings.tabSub.network") },
  diagnostics: { title: t("settings.tab.diagnostics"), description: t("settings.tabSub.diagnostics") },
  appearance: { title: t("settings.tab.appearance"), description: t("settings.tabSub.appearance") },
  shortcuts: { title: t("settings.tab.shortcuts"), description: t("settings.tabSub.shortcuts") },
  data: { title: t("settings.tab.storage"), description: t("settings.tabSub.storage") },
  updates: { title: t("settings.tab.updates"), description: t("settings.tabSub.updates") },
  about: { title: t("settings.about.title"), description: t("settings.about.hint") },
});

const STYLE_LABEL_KEYS: Record<ThemeStyle, DictKey> = {
  graphite: "settings.style.graphite.zh",
  aurora: "settings.style.aurora.zh",
  slate: "settings.style.slate.zh",
  carbon: "settings.style.carbon.zh",
  nocturne: "settings.style.nocturne.zh",
  amber: "settings.style.amber.zh",
};

const TEXT_SIZE_LABEL_KEYS: Record<TextSize, DictKey> = {
  small: "settings.textSizeSmall",
  default: "settings.textSizeDefault",
  large: "settings.textSizeLarge",
  xlarge: "settings.textSizeXLarge",
  xxlarge: "settings.textSizeXXLarge",
};

const TAURI_STATUS_BAR_LABEL_KEYS: Record<TauriStatusBarItemId, DictKey> = {
  workspace: "settings.statusBarItem.workspace", model: "settings.statusBarItem.model", balance: "settings.statusBarItem.balance", session: "settings.statusBarItem.session",
  observed_tokens: "settings.statusBarItem.observedTokens", turn_tokens: "settings.statusBarItem.turnTokens", turn_output_tokens: "settings.statusBarItem.turnOutputTokens", turn_cache_tokens: "settings.statusBarItem.turnCacheTokens", turn_cost: "settings.statusBarItem.turnCost", session_turns: "settings.statusBarItem.sessionTurns", context: "settings.statusBarItem.context",
  turn_tps: "status.tpsLabel",
  session_cost: "settings.statusBarItem.sessionCost", compact: "settings.statusBarItem.compact", cache: "status.cacheLabel", cache_avg: "status.cacheAvgLabel", bridge: "settings.statusBarItem.bridge",
};

const TAURI_COMPACT_RATIO_PRESETS = [
  [70, "settings.compactRatioPreset.70", "settings.compactRatioPresetEffect.70"],
  [80, "settings.compactRatioPreset.80", "settings.compactRatioPresetEffect.80"],
  [85, "settings.compactRatioPreset.85", "settings.compactRatioPresetEffect.85"],
] as const satisfies readonly (readonly [number, DictKey, DictKey])[];

export const TAURI_SHORTCUT_LABELS: Record<TauriShortcutAction, { label: DictKey; description?: DictKey }> = {
  new_session: { label: "shortcuts.action.newSession", description: "shortcuts.desc.newSession" },
  close_panel: { label: "shortcuts.action.closeTab", description: "shortcuts.desc.closeTab" },
  settings: { label: "shortcuts.action.settings", description: "shortcuts.desc.settings" },
  command_palette: { label: "shortcuts.action.commandPalette", description: "shortcuts.desc.commandPalette" },
  show_shortcuts: { label: "shortcuts.action.showShortcuts", description: "shortcuts.desc.showShortcuts" },
  diagnostics: { label: "settings.tab.diagnostics", description: "settings.tabSub.diagnostics" },
  toggle_sidebar: { label: "settings.tauriShortcut.toggleSidebar", description: "settings.tauriShortcut.toggleSidebarHint" },
  workspace_files: { label: "workspace.filesTab" },
  refresh_session: { label: "settings.tauriShortcut.refreshSession", description: "settings.tauriShortcut.refreshSessionHint" },
  send_message: { label: "shortcuts.action.composerSend", description: "shortcuts.desc.composerSend" },
  composer_newline: { label: "shortcuts.action.composerNewline", description: "shortcuts.desc.composerNewline" },
  open_appearance: { label: "settings.tab.appearance", description: "settings.tabSub.appearance" },
  open_model_preferences: { label: "settings.models.preferences", description: "settings.tabSub.models" },
  open_model_services: { label: "settings.models.services", description: "settings.tabSub.providers" },
  open_usage_stats: { label: "settings.modelTab.stats", description: "settings.pageDesc.model-stats" },
  open_general: { label: "settings.tab.general", description: "settings.tabSub.general" },
  open_bots: { label: "settings.tab.bots", description: "settings.tabSub.bots" },
  open_mcp: { label: "settings.tab.mcp", description: "settings.tabSub.mcp" },
  open_remote: { label: "settings.tab.remote", description: "settings.tabSub.remote" },
  open_skills: { label: "settings.tab.skills", description: "settings.tabSub.skills" },
  open_plugins: { label: "settings.tab.plugins", description: "settings.tabSub.plugins" },
  open_subagents: { label: "settings.tab.subagents", description: "subagents.tabHint" },
  open_hooks: { label: "settings.tab.hooks", description: "settings.tabSub.hooks" },
  open_memory: { label: "settings.tab.memory", description: "settings.tabSub.memory" },
  open_permissions: { label: "settings.tab.permissions", description: "settings.tabSub.permissions" },
  open_sandbox: { label: "settings.tab.sandbox", description: "settings.tabSub.sandbox" },
  open_network: { label: "settings.tab.network", description: "settings.tabSub.network" },
  open_storage: { label: "settings.tab.storage", description: "settings.tabSub.storage" },
  open_shortcuts: { label: "settings.tab.shortcuts", description: "settings.tabSub.shortcuts" },
  open_updates: { label: "settings.tab.updates", description: "settings.tabSub.updates" },
  open_about: { label: "settings.about.navLabel", description: "settings.about.hint" },
  goto_session_1: { label: "shortcuts.action.topicGoto1", description: "shortcuts.desc.topicGoto" },
  goto_session_2: { label: "shortcuts.action.topicGoto2", description: "shortcuts.desc.topicGoto" },
  goto_session_3: { label: "shortcuts.action.topicGoto3", description: "shortcuts.desc.topicGoto" },
  goto_session_4: { label: "shortcuts.action.topicGoto4", description: "shortcuts.desc.topicGoto" },
  goto_session_5: { label: "shortcuts.action.topicGoto5", description: "shortcuts.desc.topicGoto" },
  goto_session_6: { label: "shortcuts.action.topicGoto6", description: "shortcuts.desc.topicGoto" },
  goto_session_7: { label: "shortcuts.action.topicGoto7", description: "shortcuts.desc.topicGoto" },
  goto_session_8: { label: "shortcuts.action.topicGoto8", description: "shortcuts.desc.topicGoto" },
  goto_session_9: { label: "shortcuts.action.topicGoto9", description: "shortcuts.desc.topicGoto" },
  text_size_increase: { label: "shortcuts.action.textSizeIncrease", description: "shortcuts.desc.textSizeIncrease" },
  text_size_decrease: { label: "shortcuts.action.textSizeDecrease", description: "shortcuts.desc.textSizeDecrease" },
  text_size_reset: { label: "shortcuts.action.textSizeReset", description: "shortcuts.desc.textSizeReset" },
};

const FONT_LABEL_KEYS: Record<FontFamily, DictKey> = {
  system: "settings.fontFamilySystem",
  yahei: "settings.fontFamilyYaHei",
  pingfang: "settings.fontFamilyPingFang",
  noto: "settings.fontFamilyNoto",
  custom: "settings.fontFamilyCustomName",
};

const MONO_FONT_LABEL_KEYS: Record<MonoFontFamily, DictKey> = {
  system: "settings.monoFontFamilySystem",
  cascadia: "settings.monoFontFamilyCascadia",
  jetbrains: "settings.monoFontFamilyJetBrains",
  sfmono: "settings.monoFontFamilySFMono",
  custom: "settings.monoFontFamilyCustomName",
};

export function TauriSettings({ onClose, onProviderSummaryChange, currentSessionState, currentSessionId, currentSessionModelRef, onCurrentSessionModelChange, currentSessionHasAttachments, onApplyToCurrentSession, onUseSubagentInChat, workspaceRoot, defaultWorkspace, onChooseDefaultWorkspace, onClearDefaultWorkspace, initialTab = "general", profile, onRefreshProfile, onImportStableProfile, onImportStableProjectFolders, onScanUnclaimedSessions, importBusy, bridgeStatus, catalogAudit, catalogAuditError, sessionPageSource, hostError, onRestartBridge, onRefreshCatalogAudit }: TauriSettingsProps) {
  const desktopLayout = useTauriDesktopLayout();
  const t = useT();
  const translatorRef = useRef(t);
  translatorRef.current = t;
  const { pref: languagePref, setPref: setLanguagePref } = useI18n();
  const [tab, setTab] = useState<TauriSettingsTab>(initialTab);
  const [navQuery, setNavQuery] = useState("");
  const [runtimeInfo, setRuntimeInfo] = useState<TauriPreviewRuntimeInfo | null>(null);
  const [providerSummary, setProviderSummaryState] = useState<TauriProviderSummary | null>(null);
  const [modelLoading, setModelLoading] = useState(true);
  const [aboutLoading, setAboutLoading] = useState(true);
  const [modelLoadError, setModelLoadError] = useState(false);
  const [modelSaveError, setModelSaveError] = useState("");
  const [modelSaving, setModelSaving] = useState(false);
  const [aboutLoadError, setAboutLoadError] = useState(false);
  const [platform, setPlatform] = useState("");
  const [closeBehavior, setCloseBehavior] = useState<TauriCloseBehavior>("keep_running");
  const [closeLoading, setCloseLoading] = useState(true);
  const [closeSaving, setCloseSaving] = useState(false);
  const [closeError, setCloseError] = useState("");
  const [approvalMode, setApprovalMode] = useState<TauriToolApprovalMode>("auto");
  const [approvalLoading, setApprovalLoading] = useState(true);
  const [approvalSaving, setApprovalSaving] = useState(false);
  const [approvalError, setApprovalError] = useState("");
  const [terminalTheme, setTerminalTheme] = useState<TerminalThemePreference>("auto");
  const [terminalThemeSaving, setTerminalThemeSaving] = useState(false);
  const [terminalThemeError, setTerminalThemeError] = useState("");
  const [appearanceSaving, setAppearanceSaving] = useState(false);
  const [appearanceError, setAppearanceError] = useState("");
  const appearanceDirty = useRef(false);
  const [languageSaving, setLanguageSaving] = useState(false);
  const [languageError, setLanguageError] = useState("");
  const [displayCurrency, setDisplayCurrency] = useState<"" | "CNY" | "USD">("");
  const [currencySaving, setCurrencySaving] = useState(false);
  const [currencyError, setCurrencyError] = useState("");
  const languageDirty = useRef(false);
  const loadRequest = useRef(0);
  const [appearance, setAppearance] = useState<TauriAppearance>(readTauriAppearance);
  const [activeThemeId, setActiveThemeId] = useState("");
  const [userThemes, setUserThemes] = useState<ThemePackView[]>([]);
  const [conversationWidth, setConversationWidth] = useState<ConversationWidth>(getCachedConversationWidth);
  const [textSize, setTextSize] = useState<TextSize>(getTextSize);
  const [fontFamily, setFontFamily] = useState<FontFamily>(getFontFamily);
  const [monoFontFamily, setMonoFontFamily] = useState<MonoFontFamily>(getMonoFontFamily);
  const [customFontName, setCustomFontNameState] = useState(getCustomFontName);
  const [customMonoFontName, setCustomMonoFontNameState] = useState(getCustomMonoFontName);
  const [notificationsEnabled, setNotificationsEnabled] = useState(getTauriNotificationsEnabled);
  const [notificationEvents, setNotificationEvents] = useState(getTauriNotificationEvents);
  const [progressMode, setProgressMode] = useState(getTauriProgressMode);

  const updateProviderSummary = useCallback((summary: TauriProviderSummary) => {
    setProviderSummaryState(summary);
    onProviderSummaryChange?.(summary);
  }, [onProviderSummaryChange]);

  const refreshPluginThemes = async () => {
    try {
      const contributedThemes = await tauriPluginThemes();
      setUserThemes(current => [...current.filter(theme => theme.kind !== "plugin"), ...contributedThemes]);
    } catch {
      // The plugin catalog is optional; keep saved and bundled themes usable.
    }
  };

  const loadSettings = useCallback(() => {
    const request = ++loadRequest.current;
    setModelLoading(true);
    setAboutLoading(true);
    void tauriPreviewRuntimeInfo().then(
      info => {
        if (request !== loadRequest.current) return;
        setRuntimeInfo(info);
        setAboutLoadError(false);
      },
      () => { if (request === loadRequest.current) setAboutLoadError(true); },
    ).finally(() => { if (request === loadRequest.current) setAboutLoading(false); });
    void tauriProviderSummary().then(
      providers => {
        if (request !== loadRequest.current) return;
        updateProviderSummary(providers);
        setModelLoadError(false);
      },
      () => { if (request === loadRequest.current) setModelLoadError(true); },
    ).finally(() => { if (request === loadRequest.current) setModelLoading(false); });
    void tauriPlatformInfo().then(
      plat => { if (request === loadRequest.current) setPlatform(plat); },
      () => { /* Platform is optional on the About page. */ },
    );
    void getTauriCloseBehavior().then(
      behavior => { if (request === loadRequest.current) { setCloseBehavior(behavior); setCloseError(""); setCloseLoading(false); } },
      () => { if (request === loadRequest.current) { setCloseError(translatorRef.current("settings.errorUnknown")); setCloseLoading(false); } },
    );
    void Promise.all([tauriDesktopPreferences(), tauriActiveThemeId(), tauriUserThemes(), tauriPluginThemes().catch(() => [])]).then(
      ([value, themeId, savedThemes, contributedThemes]) => { if (request === loadRequest.current) { setApprovalMode(value.defaultToolApprovalMode); setApprovalError(""); setTerminalTheme(normalizeTerminalThemePreference(value.terminalTheme)); applyTerminalThemePreference(value.terminalTheme); setTerminalThemeError(""); if (value.appearanceConfigured && !appearanceDirty.current) { const next = { mode: value.theme, style: THEME_STYLES.includes(value.themeStyle as ThemeStyle) ? value.themeStyle as ThemeStyle : "graphite" }; setAppearance(next); applyTauriAppearance(next); } const themes = [...savedThemes, ...contributedThemes]; setUserThemes(themes); setActiveThemeId(themeId); const pack = tauriThemePackById(themeId) ?? themes.find(theme => theme.id === themeId) ?? null; if (pack) applyThemePack(pack); else clearThemePack(); if (!languageDirty.current) setLanguagePref(value.language); setDisplayCurrency(value.displayCurrency); setCurrencyError(""); setApprovalLoading(false); } },
      () => { if (request === loadRequest.current) { setApprovalError(translatorRef.current("settings.errorUnknown")); setApprovalLoading(false); } },
    );
  }, [setLanguagePref, updateProviderSummary]);

  useEffect(() => {
    void loadSettings();
    return () => { loadRequest.current += 1; };
  }, [loadSettings]);

  const handleAppearanceChange = async (next: TauriAppearance) => {
    if (appearanceSaving) return;
    const previous = appearance;
    appearanceDirty.current = true;
    setAppearance(next);
    applyTauriAppearance(next);
    setAppearanceSaving(true);
    setAppearanceError("");
    try {
      const saved = await setTauriDesktopAppearance(next.mode, next.style);
      const persisted = { mode: saved.theme, style: THEME_STYLES.includes(saved.themeStyle as ThemeStyle) ? saved.themeStyle as ThemeStyle : "graphite" };
      setAppearance(persisted);
      applyTauriAppearance(persisted);
      const activePack = tauriThemePackById(activeThemeId) ?? userThemes.find(theme => theme.id === activeThemeId) ?? null;
      if (activePack) applyThemePack(activePack);
    } catch {
      setAppearance(previous);
      applyTauriAppearance(previous);
      const previousPack = tauriThemePackById(activeThemeId) ?? userThemes.find(theme => theme.id === activeThemeId) ?? null;
      if (previousPack) applyThemePack(previousPack);
      setAppearanceError(t("settings.errorUnknown"));
    } finally {
      setAppearanceSaving(false);
    }
  };

  const handleThemeApply = async (id: string) => {
    if (appearanceSaving) return false;
    const previousThemeId = activeThemeId;
    const previousAppearance = appearance;
    setAppearanceSaving(true);
    setAppearanceError("");
    try {
      if (THEME_STYLES.includes(id as ThemeStyle)) {
        const updated = await setTauriDesktopAppearance(appearance.mode, id as ThemeStyle);
        let saved: string;
        try {
          saved = await setTauriActiveThemeId("");
        } catch (error) {
          await setTauriDesktopAppearance(previousAppearance.mode, previousAppearance.style).catch(() => undefined);
          throw error;
        }
        const next = { mode: updated.theme, style: updated.themeStyle as ThemeStyle };
        setActiveThemeId(saved);
        setAppearance(next);
        applyTauriAppearance(next);
        clearThemePack();
      } else {
        const pack = tauriThemePackById(id) ?? userThemes.find(theme => theme.id === id) ?? null;
        if (!pack) throw new Error("unknown theme");
        const saved = await setTauriActiveThemeId(id);
        setActiveThemeId(saved);
        applyThemePack(pack);
      }
      return true;
    } catch {
      await Promise.all([
        setTauriActiveThemeId(previousThemeId).catch(() => previousThemeId),
        setTauriDesktopAppearance(previousAppearance.mode, previousAppearance.style).catch(() => undefined),
      ]);
      setActiveThemeId(previousThemeId);
      setAppearance(previousAppearance);
      applyTauriAppearance(previousAppearance);
      const previousPack = tauriThemePackById(previousThemeId) ?? userThemes.find(theme => theme.id === previousThemeId) ?? null;
      if (previousPack) applyThemePack(previousPack); else clearThemePack();
      setAppearanceError(t("settings.errorUnknown"));
      return false;
    } finally {
      setAppearanceSaving(false);
    }
  };

  const handleUserThemeSave = async (theme: UserThemeSaveInput): Promise<ThemePackView | null> => {
    try {
      const saved = await saveTauriUserTheme(theme);
      setUserThemes(current => [...current.filter(item => item.id !== saved.id), saved]);
      setAppearanceError("");
      return saved;
    } catch {
      setAppearanceError(t("settings.themeGallery.themeSaveFailed"));
      return null;
    }
  };

  const handleUserThemeDelete = async (id: string): Promise<boolean> => {
    try {
      await deleteTauriUserTheme(id);
      setUserThemes(current => current.filter(theme => theme.id !== id));
      if (activeThemeId === id) {
        setActiveThemeId("");
        applyTauriAppearance(appearance);
        clearThemePack();
      }
      setAppearanceError("");
      return true;
    } catch {
      setAppearanceError(t("settings.themeGallery.themeDeleteFailed"));
      return false;
    }
  };

  const handleUserThemeImport = async (): Promise<ThemePackView | null> => {
    try {
      const imported = await importTauriUserTheme();
      if (imported) setUserThemes(current => [...current, imported]);
      setAppearanceError("");
      return imported;
    } catch {
      setAppearanceError(t("settings.themeGallery.themeImportFailed"));
      return null;
    }
  };

  const handleUserThemeExport = async (id: string): Promise<boolean> => {
    try {
      const exported = await exportTauriUserTheme(id);
      setAppearanceError("");
      return exported;
    } catch {
      setAppearanceError(t("settings.themeGallery.themeExportFailed"));
      return false;
    }
  };

  const handleLanguageChange = async (next: LangPref) => {
    if (languageSaving || next === languagePref) return;
    const previous = languagePref;
    languageDirty.current = true;
    setLanguagePref(next);
    setLanguageSaving(true);
    setLanguageError("");
    try {
      const saved = await setTauriDesktopLanguage(next);
      setLanguagePref(saved.language);
    } catch {
      setLanguagePref(previous);
      setLanguageError(t("settings.errorUnknown"));
    } finally {
      setLanguageSaving(false);
    }
  };

  const handleDisplayCurrencyChange = async (next: "" | "CNY" | "USD") => {
    if (currencySaving || next === displayCurrency) return;
    const previous = displayCurrency;
    setDisplayCurrency(next);
    setCurrencySaving(true);
    setCurrencyError("");
    try {
      const saved = await setTauriDesktopCurrency(next);
      setDisplayCurrency(saved.displayCurrency);
    } catch {
      setDisplayCurrency(previous);
      setCurrencyError(t("settings.currencySaveFailed"));
    } finally { setCurrencySaving(false); }
  };

  const handleConversationWidthChange = (next: ConversationWidth) => {
    setConversationWidth(applyConversationWidth(next));
  };

  const handleTextSizeChange = (next: TextSize) => {
    setTextSize(next);
    applyTextSize(next);
  };

  const handleFontFamilyChange = (next: FontFamily) => {
    setFontFamily(next);
    applyFontFamily(next);
  };

  const handleMonoFontFamilyChange = (next: MonoFontFamily) => {
    setMonoFontFamily(next);
    applyMonoFontFamily(next);
  };

  const handleCustomFontChange = (name: string) => {
    setCustomFontNameState(name);
    setCustomFontName(name);
    applyFontFamily("custom");
  };

  const handleCustomMonoFontChange = (name: string) => {
    setCustomMonoFontNameState(name);
    setCustomMonoFontName(name);
    applyMonoFontFamily("custom");
  };

  const handleModelChange = async (model: string, scope: "global" | "project", root?: string) => {
    if (modelSaving) return undefined;
    setModelSaving(true);
    setModelSaveError("");
    try {
      const updated = await setTauriDefaultModel(model, scope, root);
      if (scope === "global") updateProviderSummary(updated);
      return updated;
    } catch {
      setModelSaveError(t("settings.errorUnknown"));
      return undefined;
    } finally {
      setModelSaving(false);
    }
  };

  const handleModelRoleChange = async (role: "planner" | "vision" | "search", model: string, scope: "global" | "project", root?: string) => {
    if (modelSaving) return undefined;
    setModelSaving(true);
    setModelSaveError("");
    try {
      const updated = await setTauriModelRole(role, model, scope, root);
      if (scope === "global") updateProviderSummary(updated);
      return updated;
    } catch {
      setModelSaveError(t("settings.errorUnknown"));
      return undefined;
    } finally {
      setModelSaving(false);
    }
  };

  const handleAgentPreferenceChange = async (request: { reasoningLanguage?: "auto" | "zh" | "en"; compactRatioPercent?: number }, scope: "global" | "project", root?: string) => {
    if (modelSaving) return undefined;
    setModelSaving(true);
    setModelSaveError("");
    try {
      const updated = await setTauriAgentPreference(request, scope, root);
      if (scope === "global") updateProviderSummary(updated);
      return updated;
    } catch {
      setModelSaveError(t("settings.errorUnknown"));
      return undefined;
    } finally {
      setModelSaving(false);
    }
  };

  const handleCloseBehaviorChange = async (behavior: TauriCloseBehavior) => {
    if (closeSaving || behavior === closeBehavior) return;
    setCloseSaving(true);
    setCloseError("");
    try {
      setCloseBehavior(await setTauriCloseBehavior(behavior));
    } catch {
      setCloseError(t("settings.errorUnknown"));
    } finally {
      setCloseSaving(false);
    }
  };

  const handleApprovalChange = async (mode: TauriToolApprovalMode) => {
    if (approvalSaving || mode === approvalMode) return;
    setApprovalSaving(true);
    setApprovalError("");
    try {
      const updated = await setTauriDesktopApproval(mode);
      setApprovalMode(updated.defaultToolApprovalMode);
    } catch {
      setApprovalError(t("settings.errorUnknown"));
    } finally {
      setApprovalSaving(false);
    }
  };

  const handleTerminalThemeChange = async (theme: TerminalThemePreference) => {
    if (terminalThemeSaving || theme === terminalTheme) return;
    const previous = terminalTheme;
    setTerminalTheme(theme);
    setTerminalThemeSaving(true);
    setTerminalThemeError("");
    applyTerminalThemePreference(theme);
    try {
      const updated = await setTauriDesktopTerminalTheme(theme);
      const saved = normalizeTerminalThemePreference(updated.terminalTheme);
      setTerminalTheme(saved);
      applyTerminalThemePreference(saved);
    } catch {
      setTerminalTheme(previous);
      applyTerminalThemePreference(previous);
      setTerminalThemeError(t("settings.errorUnknown"));
    } finally {
      setTerminalThemeSaving(false);
    }
  };

  const query = navQuery.trim().toLocaleLowerCase();
  const settingGroups = SETTINGS_GROUPS(t);
  const settingTitles = SETTINGS_TITLES(t);
  const visibleGroups = settingGroups.map(group => ({
    ...group,
    items: group.items.filter(item => !query || `${group.label} ${item.label} ${item.description} ${"searchTerms" in item ? item.searchTerms : ""}`.toLocaleLowerCase().includes(query)),
  })).filter(group => group.items.length > 0);

  return (
    <section className="tauri-settings-overlay" data-desktop-layout={desktopLayout} aria-label={t("settings.title")}>
      <aside className="tauri-settings-sidebar">
        <div className="tauri-settings-titlebar" data-tauri-drag-region />
        <button type="button" className="tauri-settings-back" onClick={onClose}><ArrowLeft size={17} /><span>{t("settings.backToWorkspace")}</span></button>
        <label className="tauri-settings-search"><Search size={16} aria-hidden="true" /><input type="search" aria-label={t("settings.searchPlaceholder")} placeholder={t("settings.searchPlaceholder")} value={navQuery} onChange={event => setNavQuery(event.target.value)} />{navQuery && <button type="button" aria-label={t("settings.searchClear")} onClick={() => setNavQuery("")}><X size={14} aria-hidden="true" /></button>}</label>
        <nav className="tauri-settings-nav" aria-label={t("settings.title")}>
          {visibleGroups.map(group => <div className="tauri-settings-nav-group" key={group.label}>
            <div className="tauri-settings-nav-label">{group.label}</div>
            {group.items.map(item => { const Icon = item.icon; return <button key={item.id} type="button" aria-current={tab === item.id ? "page" : undefined} className={`tauri-settings-nav-item${tab === item.id ? " is-active" : ""}`} onClick={() => setTab(item.id)} title={item.description}>
              <span className="tauri-settings-nav-item-main"><Icon size={17} aria-hidden="true" /><span>{item.label}</span></span>
              {query && <small>{item.description}</small>}
            </button>; })}
          </div>)}
          {visibleGroups.length === 0 && <div className="tauri-settings-nav-empty" role="status">{t("settings.searchNoResults")}</div>}
        </nav>
      </aside>
      <div className="tauri-settings-panel">
        <div className="tauri-settings-content" data-tab={tab} key={tab}>
          <div className="tauri-settings-page-heading"><h1>{settingTitles[tab].title}</h1><p>{settingTitles[tab].description}</p></div>
          {tab === "general" ? <GeneralSettings languagePref={languagePref} onLanguageChange={handleLanguageChange} languageSaving={languageSaving} languageError={languageError} displayCurrency={displayCurrency} onDisplayCurrencyChange={handleDisplayCurrencyChange} currencySaving={currencySaving} currencyError={currencyError} notificationsEnabled={notificationsEnabled} onNotificationsChange={enabled => { setNotificationsEnabled(enabled); setTauriNotificationsEnabled(enabled); }} notificationEvents={notificationEvents} onNotificationEventChange={kind => { const next = setTauriNotificationEvent(kind, !notificationEvents[kind]); setNotificationEvents(next); }} progressMode={progressMode} onProgressModeChange={next => { setProgressMode(next); setTauriProgressMode(next); }} closeBehavior={closeBehavior} onCloseBehaviorChange={handleCloseBehaviorChange} closeLoading={closeLoading} closeSaving={closeSaving} closeError={closeError} approvalMode={approvalMode} onApprovalChange={handleApprovalChange} approvalLoading={approvalLoading} approvalSaving={approvalSaving} approvalError={approvalError} currentSessionState={currentSessionState} /> : tab === "shortcuts" ? <ShortcutSettings /> : tab === "appearance" ? <AppearanceSettings appearance={appearance} onChange={handleAppearanceChange} appearanceSaving={appearanceSaving} appearanceError={appearanceError} activeThemeId={activeThemeId} userThemes={userThemes} onRefreshPluginThemes={refreshPluginThemes} onThemeApply={handleThemeApply} onUserThemeSave={handleUserThemeSave} onUserThemeDelete={handleUserThemeDelete} onUserThemeImport={handleUserThemeImport} onUserThemeExport={handleUserThemeExport} conversationWidth={conversationWidth} onConversationWidthChange={handleConversationWidthChange} textSize={textSize} onTextSizeChange={handleTextSizeChange} fontFamily={fontFamily} onFontFamilyChange={handleFontFamilyChange} monoFontFamily={monoFontFamily} onMonoFontFamilyChange={handleMonoFontFamilyChange} customFontName={customFontName} onCustomFontChange={handleCustomFontChange} customMonoFontName={customMonoFontName} onCustomMonoFontChange={handleCustomMonoFontChange} terminalTheme={terminalTheme} onTerminalThemeChange={handleTerminalThemeChange} terminalThemeSaving={terminalThemeSaving} terminalThemeError={terminalThemeError} /> : <>
            {(tab === "model" || tab === "providers") && (modelLoading ? <div className="tauri-settings-loading">{t("settings.loading")}</div> : <>{modelLoadError && <SettingsLoadError onRetry={loadSettings} />}{providerSummary && (tab === "model" ? <ModelPreferenceSettings providerSummary={providerSummary} workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionModelRef={currentSessionModelRef} onCurrentSessionModelChange={onCurrentSessionModelChange} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} onModelChange={handleModelChange} onRoleChange={handleModelRoleChange} onAgentPreferenceChange={handleAgentPreferenceChange} saving={modelSaving} error={modelSaveError} onOpenProviders={() => setTab("providers")} /> : <ProviderSettings providerSummary={providerSummary} onProviderSummaryChange={updateProviderSummary} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />)}</>)}
            {tab === "stats" && <Suspense fallback={<div className="tauri-settings-loading">{t("settings.loading")}</div>}><TauriUsageStatsPanel loadStats={tauriUsageStats} sources={["all", "desktop-tauri"]} /></Suspense>}
            {tab === "bots" && <TauriBotSettings />}
            {tab === "mcp" && <TauriMCPSettings workspaceRoot={workspaceRoot} sessionId={currentSessionId} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "remote" && <TauriRemoteSettings />}
            {tab === "skills" && <TauriSkillsSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "plugins" && <TauriPluginSettings currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "subagents" && <TauriSubagentSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} onUseInChat={onUseSubagentInChat} />}
            {tab === "hooks" && <TauriHooksSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} onOpenPlugins={() => setTab("plugins")} />}
            {tab === "memory" && <TauriMemorySettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "permissions" && <TauriPermissionsSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "sandbox" && <TauriSandboxSettings workspaceRoot={workspaceRoot} sessionId={currentSessionId} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "network" && <TauriNetworkSettings currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "diagnostics" && <DiagnosticsSettings bridgeStatus={bridgeStatus} catalogAudit={catalogAudit} catalogAuditError={catalogAuditError} sessionPageSource={sessionPageSource} hostError={hostError} workspaceRoot={workspaceRoot} busy={Boolean(importBusy)} onRestartBridge={onRestartBridge} onRefreshCatalogAudit={onRefreshCatalogAudit} onOpenData={() => setTab("data")} onOpenProviders={() => setTab("providers")} onNavigateSettings={next => setTab(next)} />}
            {tab === "data" && <><TauriStorageSettings workspaceRoot={workspaceRoot} defaultWorkspace={defaultWorkspace} onChooseDefaultWorkspace={onChooseDefaultWorkspace} onClearDefaultWorkspace={onClearDefaultWorkspace} /><DataSettings profile={profile} busy={Boolean(importBusy)} onRefreshProfile={onRefreshProfile} onImportStableProfile={onImportStableProfile} onImportStableProjectFolders={onImportStableProjectFolders} onScanUnclaimedSessions={onScanUnclaimedSessions} onClose={onClose} /></>}
            {tab === "about" && (aboutLoading ? <div className="tauri-settings-loading">{t("common.loading")}</div> : <>{aboutLoadError && <SettingsLoadError onRetry={loadSettings} />}{runtimeInfo && <AboutSettings runtimeInfo={runtimeInfo} platform={platform} onRefresh={loadSettings} />}</>)}
            {tab === "updates" && runtimeInfo && <TauriUpdatesSettings runtimeInfo={runtimeInfo} />}
          </>}
        </div>
      </div>
    </section>
  );
}

function TauriSettingsChoice<T extends string>({ ariaLabel, value, options, disabled, onChange }: { ariaLabel: string; value: T; options: Array<{ value: T; label: ReactNode }>; disabled?: boolean; onChange: (value: T) => void }) {
  return <SettingsOptions layout="field" className="tauri-settings-general-options" role="radiogroup" aria-label={ariaLabel}>
    {options.map(option => <button key={option.value} type="button" role="radio" aria-checked={value === option.value} className={`set-seg__btn${value === option.value ? " set-seg__btn--on" : ""}`} disabled={disabled} onClick={() => onChange(option.value)}>{option.label}</button>)}
  </SettingsOptions>;
}

function GeneralSettings({ languagePref, onLanguageChange, languageSaving, languageError, displayCurrency, onDisplayCurrencyChange, currencySaving, currencyError, notificationsEnabled, onNotificationsChange, notificationEvents, onNotificationEventChange, progressMode, onProgressModeChange, closeBehavior, onCloseBehaviorChange, closeLoading, closeSaving, closeError, approvalMode, onApprovalChange, approvalLoading, approvalSaving, approvalError, currentSessionState }: { languagePref: LangPref; onLanguageChange: (language: LangPref) => void; languageSaving: boolean; languageError: string; displayCurrency: "" | "CNY" | "USD"; onDisplayCurrencyChange: (currency: "" | "CNY" | "USD") => void; currencySaving: boolean; currencyError: string; notificationsEnabled: boolean; onNotificationsChange: (enabled: boolean) => void; notificationEvents: Record<TauriNotificationKind, boolean>; onNotificationEventChange: (kind: TauriNotificationKind) => void; progressMode: TauriProgressMode; onProgressModeChange: (next: TauriProgressMode) => void; closeBehavior: TauriCloseBehavior; onCloseBehaviorChange: (behavior: TauriCloseBehavior) => void; closeLoading: boolean; closeSaving: boolean; closeError: string; approvalMode: TauriToolApprovalMode; onApprovalChange: (mode: TauriToolApprovalMode) => void; approvalLoading: boolean; approvalSaving: boolean; approvalError: string; currentSessionState?: "idle" | "running" | "paused" }) {
  const t = useT();
  const desktopLayout = useTauriDesktopLayout();
  const [soundExpanded, setSoundExpanded] = useState(false);
  const [successSound, setSuccessSound] = useState<SoundWavPref>(getSuccessPreference);
  const [attentionSound, setAttentionSound] = useState<SoundWavPref>(getAttentionPreference);
  const [soundVolume, setSoundVolume] = useState(getNotificationVolume);
  const [musicPreset, setMusicPreset] = useState<GenerativePreset>(getGenerativePreset);
  const statusBar = useTauriStatusBarPreferences();
  const changeMusicPreset = (next: GenerativePreset) => {
    setMusicPreset(next);
    setGenerativePreset(next);
    if (next === "off" || currentSessionState !== "running") generativeMusic.stop();
    else if (generativeMusic.isRunning) generativeMusic.setPreset(next);
    else generativeMusic.start(next);
    if (next !== "off" && typeof AudioContext !== "undefined") generativeMusic.playPreview(next);
  };
  return <div className="tauri-settings-section tauri-settings-general settings-page settings-page--general">
    <SettingsSection title={t("settings.general.sectionAppearance")} description={t("settings.general.desktopHint")}>
    <div className="tauri-settings-field"><Monitor className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.desktopLayoutStyle")}<small>{t("settings.desktopLayoutStyleHint")}</small></span><TauriSettingsChoice ariaLabel={t("settings.desktopLayoutStyle")} value={desktopLayout} options={[{ value: "workbench", label: t("settings.desktopLayoutStyle.workbench") }, { value: "creation", label: t("settings.desktopLayoutStyle.creation") }]} onChange={setTauriDesktopLayout} /></div>
    <div className="tauri-settings-field"><Languages className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.language")}<small>{t("settings.languageHint")}</small></span><TauriSettingsChoice ariaLabel={t("settings.language")} value={languagePref} disabled={languageSaving} options={[{ value: "", label: t("settings.langAuto") }, { value: "zh", label: "中文" }, { value: "en", label: "English" }]} onChange={onLanguageChange} /></div>
    {languageError && <p className="tauri-diagnostic-error" role="alert">{languageError}</p>}
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.currency")}<small>{t("settings.currencyHint")}</small></span><TauriSettingsChoice ariaLabel={t("settings.currency")} value={displayCurrency} disabled={currencySaving} options={[{ value: "", label: t("settings.currencyAuto") }, { value: "CNY", label: "CNY" }, { value: "USD", label: "USD" }]} onChange={onDisplayCurrencyChange} /></div>
    {currencyError && <p className="tauri-diagnostic-error" role="alert">{currencyError}</p>}
    </SettingsSection>
    <SettingsSection title={t("settings.general.sectionConversation")} description={t("settings.sessionExperienceHint")}>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.sessionExperience")}<small>{progressMode === "standard" ? t("settings.sessionExperience.standardHint") : t("settings.sessionExperience.deepHint")}</small></span><TauriSettingsChoice ariaLabel={t("settings.sessionExperience")} value={progressMode} options={[{ value: "standard", label: t("settings.sessionExperience.standard") }, { value: "deep", label: t("settings.sessionExperience.deep") }]} onChange={onProgressModeChange} /></div>
    </SettingsSection>
    <SettingsSection title={t("settings.general.sectionSystem")} description={t("settings.general.sectionSystemHint")}>
    <div className="tauri-settings-field"><Power className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.closeBehavior")}<small>{t("settings.closeBehaviorHint")}</small></span><TauriSettingsChoice ariaLabel={t("settings.closeBehavior")} value={closeBehavior} disabled={closeLoading || closeSaving} options={[{ value: "keep_running", label: t("settings.closeBehavior.background") }, { value: "quit", label: t("settings.closeBehavior.quit") }]} onChange={onCloseBehaviorChange} /></div>
    {closeError && <p className="tauri-diagnostic-error" role="alert">{closeError}</p>}
    <div className="tauri-settings-field"><ShieldCheck className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.defaultToolApprovalMode")}<small>{t("settings.defaultToolApprovalModeHint")}</small></span><TauriSettingsChoice ariaLabel={t("settings.defaultToolApprovalMode")} value={approvalMode} disabled={approvalLoading || approvalSaving} options={[{ value: "ask", label: t("settings.defaultToolApprovalMode.ask") }, { value: "auto", label: t("settings.defaultToolApprovalMode.auto") }, { value: "yolo", label: t("settings.defaultToolApprovalMode.yolo") }]} onChange={onApprovalChange} /></div>
    {approvalError && <p className="tauri-diagnostic-error" role="alert">{approvalError}</p>}
    <label className="tauri-settings-toggle"><Bell className="tauri-settings-field-icon" size={18} /><span><strong>{t("settings.general.notifications")}</strong><small>{t("settings.general.notificationsHint")}</small></span><input type="checkbox" checked={notificationsEnabled} onChange={event => onNotificationsChange(event.target.checked)} /></label>
    <div className="tauri-settings-notification-events" aria-label={t("settings.notificationEvents")}>
      <h4>{t("settings.notificationEvents")}</h4>
      {(["turn_done", "approval_request", "ask_request"] as const).map(kind => <label className="tauri-settings-toggle" key={kind}><span><strong>{t(`settings.notificationEvents.${kind}` as DictKey)}</strong></span><input type="checkbox" checked={notificationEvents[kind]} onChange={() => onNotificationEventChange(kind)} /></label>)}
    </div>
    <div className="tauri-settings-sound">
      <button type="button" className="tauri-settings-sound-toggle" aria-expanded={soundExpanded} aria-label={soundExpanded ? t("settings.soundCollapse") : t("settings.soundExpand")} onClick={() => setSoundExpanded(open => !open)}><Volume2 className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.sound")}<small>{t("settings.soundHint")}</small></span><span>{musicPreset === "off" && successSound === "off" && attentionSound === "off" ? t("settings.soundStatus.allOff") : t("settings.soundStatus.custom")}</span><ChevronDown size={15} aria-hidden="true" /></button>
      {soundExpanded && <div className="tauri-settings-sound-body">
        <div className="tauri-settings-sound-row"><label>{t("settings.generativeMusicPreset")}<select aria-label={t("settings.generativeMusicPreset")} value={musicPreset} onChange={event => changeMusicPreset(event.target.value as GenerativePreset)}>{musicPresets(t).map(option => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label><button type="button" aria-label={t("settings.generativeMusicPreview")} disabled={musicPreset === "off" || typeof AudioContext === "undefined"} onClick={() => { if (musicPreset !== "off") generativeMusic.playPreview(musicPreset); }}><Play size={14} /></button></div>
        <label className="tauri-settings-sound-row">{t("settings.notificationVolume")} <input type="range" min={0} max={100} value={soundVolume} aria-label={t("settings.notificationVolume")} onChange={event => setSoundVolume(setNotificationVolume(Number(event.target.value)))} /><output>{soundVolume}%</output></label>
        <TauriSoundOption t={t} label={t("settings.notificationSoundSuccess")} value={successSound} onChange={next => { setSuccessSound(next); setSuccessPreference(next); playSuccessChime(); }} onPreview={playSuccessChime} />
        <TauriSoundOption t={t} label={t("settings.notificationSoundAttention")} value={attentionSound} onChange={next => { setAttentionSound(next); setAttentionPreference(next); playAttentionChime(); }} onPreview={playAttentionChime} />
      </div>}
    </div>
    <div className="tauri-settings-field"><PanelTop className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.statusBarStyle")}<small>{t("settings.statusBarStyleHint")}</small></span><TauriSettingsChoice ariaLabel={t("settings.statusBarStyle")} value={statusBar.style} options={[{ value: "icon", label: t("settings.statusBarStyle.icon") }, { value: "text", label: t("settings.statusBarStyle.text") }]} onChange={style => setTauriStatusBarPreferences({ ...statusBar, style })} /></div>
    <div className="tauri-settings-field tauri-settings-status-items"><Activity className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.statusBarItems")}<small>{t("settings.statusBarItemsHint")}</small></span><StatusBarItemsEditor items={statusBar.items} availableItems={TAURI_STATUS_BAR_ITEM_IDS} busy={false} onChange={items => setTauriStatusBarPreferences({ ...statusBar, items })} itemLabel={id => t(TAURI_STATUS_BAR_LABEL_KEYS[id])} /></div>
    </SettingsSection>
  </div>;
}

const SOUND_OPTION_KEYS: Record<SoundWavPref, DictKey> = {
  off: "settings.notificationSound.off", synth: "settings.notificationSound.synth",
  positive: "settings.notificationSound.positive", correct: "settings.notificationSound.correct",
  start: "settings.notificationSound.start", back: "settings.notificationSound.back",
};

const MUSIC_PRESET_KEYS: Record<GenerativePreset, DictKey> = {
  off: "settings.generativeMusic.off", ethereal: "settings.generativeMusic.presets.ethereal",
  classic: "settings.generativeMusic.presets.classic", digital: "settings.generativeMusic.presets.digital",
  retro: "settings.generativeMusic.presets.retro",
};

function musicPresets(t: Translator) {
  return (Object.entries(MUSIC_PRESET_KEYS) as [GenerativePreset, DictKey][]).map(([value, key]) => ({ value, label: t(key) }));
}

function TauriSoundOption({ t, label, value, onChange, onPreview }: { t: Translator; label: string; value: SoundWavPref; onChange: (next: SoundWavPref) => void; onPreview: () => void }) {
  return <div className="tauri-settings-sound-row"><label>{label}<select aria-label={label} value={value} onChange={event => onChange(event.target.value as SoundWavPref)}>{(Object.entries(SOUND_OPTION_KEYS) as [SoundWavPref, DictKey][]).map(([optionValue, key]) => <option key={optionValue} value={optionValue}>{t(key)}</option>)}</select></label><button type="button" aria-label={`${t("settings.notificationSoundPreview")} · ${label}`} disabled={value === "off"} onClick={onPreview}><Play size={14} /></button></div>;
}

function ShortcutSettings() {
  const t = useT();
  const platform = detectShortcutPlatform();
  const overrides = useTauriShortcuts();
  const [recording, setRecording] = useState<TauriShortcutAction | null>(null);
  const [feedback, setFeedback] = useState("");
  const record = (action: TauriShortcutAction, event: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (event.key === "Escape") {
      event.preventDefault(); event.stopPropagation();
      setRecording(null); setFeedback("");
      return;
    }
    if (event.key === "Tab") { setRecording(null); return; }
    const combo = comboFromKeyboardEvent(event.nativeEvent);
    if (!combo) return;
    event.preventDefault(); event.stopPropagation();
    if (!isValidTauriShortcut(action, combo)) {
      setFeedback(action === "send_message"
        ? t("settings.shortcutsEnterOnly", { action: t(TAURI_SHORTCUT_LABELS[action].label) })
        : t("settings.tauriShortcut.primaryModifier"));
      return;
    }
    const conflict = tauriShortcutConflict(action, combo, platform);
    if (conflict) {
      setFeedback(t("settings.shortcutsConflict", {
        action: t(TAURI_SHORTCUT_LABELS[action].label),
        conflict: t(TAURI_SHORTCUT_LABELS[conflict].label),
      }));
      return;
    }
    setTauriShortcut(action, combo, platform);
    setRecording(null); setFeedback("");
  };
  return <div className="tauri-settings-section">
    <div className="tauri-shortcuts-heading"><div><h3>{t("settings.shortcutsTitle")}</h3><p>{t("settings.shortcutsHint")}</p></div><button type="button" className="tauri-settings-button" onClick={() => { resetTauriShortcuts(); setRecording(null); setFeedback(""); }} disabled={Object.keys(overrides).length === 0}>{t("settings.shortcutsResetAll")}</button></div>
    {feedback && <p className="tauri-diagnostic-error" role="alert">{feedback}</p>}
    <div className="tauri-settings-shortcuts">
      {TAURI_SHORTCUT_ACTIONS.map(action => {
        const info = TAURI_SHORTCUT_LABELS[action];
        const label = t(info.label);
        const keys = formatShortcutCombo(getTauriShortcut(action, platform), platform);
        const active = recording === action;
        return <div key={action} className="tauri-shortcut-row"><span><strong>{label}</strong>{info.description && <small>{t(info.description)}</small>}</span><div className="tauri-shortcut-row__actions"><button type="button" className={`tauri-shortcut-key${active ? " is-recording" : ""}`} data-tauri-shortcut-action={action} aria-label={active ? `${t("settings.shortcutsRecording")} · ${label}` : `${label}: ${keys}`} aria-pressed={active} onClick={event => { setRecording(action); setFeedback(""); event.currentTarget.focus(); }} onBlur={() => { if (active) setRecording(null); }} onKeyDown={event => { if (active) record(action, event); }}>{active ? t("settings.shortcutsRecording") : <kbd>{keys}</kbd>}</button><button type="button" className="tauri-shortcut-reset" disabled={!overrides[action]} onClick={() => { setTauriShortcut(action, null, platform); setFeedback(""); }}>{t("settings.shortcutsReset")}</button></div></div>;
      })}
      <div className="tauri-shortcut-row"><span><strong>{t("settings.tauriShortcut.closePanel")}</strong></span><kbd>Esc</kbd></div>
    </div>
  </div>;
}

function DataSettings({ profile, busy, onRefreshProfile, onImportStableProfile, onImportStableProjectFolders, onScanUnclaimedSessions, onClose }: {
  profile?: TauriPreviewProfileStatus | null;
  busy: boolean;
  onRefreshProfile?: () => Promise<void>;
  onImportStableProfile?: () => Promise<string>;
  onImportStableProjectFolders?: () => Promise<string>;
  onScanUnclaimedSessions?: () => void;
  onClose: () => void;
}) {
  const t = useT();
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [actionBusy, setActionBusy] = useState(false);
  const run = async (action: () => Promise<string | void>) => {
    if (busy || actionBusy) return;
    setActionBusy(true);
    setNotice("");
    setError("");
    try {
      const result = await action();
      if (result) setNotice(result);
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setActionBusy(false);
    }
  };
  const disabled = busy || actionBusy;
  return <div className="tauri-settings-section tauri-settings-data">
    <div className="tauri-settings-data__heading"><div><h3>{t("settings.data.previewTitle")}</h3><p>{t("settings.data.previewDescription")}</p></div>{onRefreshProfile && <button type="button" className="tauri-settings-button" onClick={() => void run(onRefreshProfile)} disabled={disabled}><RefreshCw size={13} />{t("settings.data.refresh")}</button>}</div>
    {profile ? <>
      <div className="tauri-settings-data__path"><span>{t("settings.data.configDirectory")}</span><code>{profile.previewHome}</code></div>
      {profile.stableConfig && <div className="tauri-settings-data__path"><span>{t("settings.data.stableConfig")}</span><code>{profile.stableConfig}</code></div>}
      <div className="tauri-settings-data__actions">
        <h4>{t("settings.data.importTitle")}</h4>
        {profile.importAvailable ? <><p>{t("settings.data.importProfileHint")}</p><button type="button" className="tauri-settings-button" onClick={() => onImportStableProfile && void run(onImportStableProfile)} disabled={disabled || !onImportStableProfile}>{t("settings.data.importProfile")}</button></> : <p>{profile.previewConfigExists ? t("settings.data.profileExists") : profile.managedProfile ? t("settings.data.stableConfigMissing") : t("settings.data.customHomeDisabled")}</p>}
        {profile.projectFoldersImportAvailable ? <><p>{t("settings.data.importFoldersHint")}</p><button type="button" className="tauri-settings-button" onClick={() => onImportStableProjectFolders && void run(onImportStableProjectFolders)} disabled={disabled || !onImportStableProjectFolders}>{t("settings.data.importFolders")}</button></> : <p>{profile.projectFoldersFileExists ? t("settings.data.foldersExist") : profile.managedProfile ? t("settings.data.foldersMissing") : t("settings.data.customHomeFoldersDisabled")}</p>}
        {profile.managedProfile && <p>{t("settings.data.rollbackHint")}</p>}
      </div>
      {profile.managedProfile && onScanUnclaimedSessions && <div className="tauri-settings-data__actions"><h4>{t("settings.data.sessionsTitle")}</h4><p>{t("settings.data.sessionsHint")}</p><button type="button" className="tauri-settings-button" onClick={() => { onClose(); onScanUnclaimedSessions(); }} disabled={disabled}>{t("settings.data.reviewSessions")}</button></div>}
    </> : <p className="tauri-settings-loading">{t("settings.data.notLoaded")}</p>}
    {notice && <p className="tauri-settings-data__notice" role="status">{notice}</p>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
  </div>;
}

function SettingsLoadError({ onRetry }: { onRetry: () => void }) {
  const t = useT();
  return <div className="tauri-settings-load-error" role="alert">{t("settings.about.readFailed")}<button type="button" className="tauri-settings-button" onClick={onRetry}><RefreshCw size={13} />{t("settings.about.retry")}</button></div>;
}

function AppearanceSettings({ appearance, onChange, appearanceSaving, appearanceError, activeThemeId, userThemes, onRefreshPluginThemes, onThemeApply, onUserThemeSave, onUserThemeDelete, onUserThemeImport, onUserThemeExport, conversationWidth, onConversationWidthChange, textSize, onTextSizeChange, fontFamily, onFontFamilyChange, monoFontFamily, onMonoFontFamilyChange, customFontName, onCustomFontChange, customMonoFontName, onCustomMonoFontChange, terminalTheme, onTerminalThemeChange, terminalThemeSaving, terminalThemeError }: {
  appearance: TauriAppearance;
  onChange: (next: TauriAppearance) => void;
  appearanceSaving: boolean;
  appearanceError: string;
  activeThemeId: string;
  userThemes: ThemePackView[];
  onRefreshPluginThemes: () => Promise<void>;
  onThemeApply: (id: string) => Promise<boolean>;
  onUserThemeSave: (theme: Pick<ThemePackView, "id" | "name" | "baseStyle" | "tokens" | "recipes">) => Promise<ThemePackView | null>;
  onUserThemeDelete: (id: string) => Promise<boolean>;
  onUserThemeImport: () => Promise<ThemePackView | null>;
  onUserThemeExport: (id: string) => Promise<boolean>;
  conversationWidth: ConversationWidth;
  onConversationWidthChange: (next: ConversationWidth) => void;
  textSize: TextSize;
  onTextSizeChange: (next: TextSize) => void;
  fontFamily: FontFamily;
  onFontFamilyChange: (next: FontFamily) => void;
  monoFontFamily: MonoFontFamily;
  onMonoFontFamilyChange: (next: MonoFontFamily) => void;
  customFontName: string;
  onCustomFontChange: (name: string) => void;
  customMonoFontName: string;
  onCustomMonoFontChange: (name: string) => void;
  terminalTheme: TerminalThemePreference;
  onTerminalThemeChange: (theme: TerminalThemePreference) => void;
  terminalThemeSaving: boolean;
  terminalThemeError: string;
}) {
  const t = useT();
  const [typographyOpen, setTypographyOpen] = useState(false);
  const [themeGalleryOpen, setThemeGalleryOpen] = useState(false);
  const [customPalette, setCustomPalette] = useState(getCustomTerminalPalette);

  if (typographyOpen) {
    return <TypographySettings onBack={() => setTypographyOpen(false)} scrollContainerSelector=".tauri-settings-content" />;
  }
  if (themeGalleryOpen) {
    return <TauriThemeGallery mode={appearance.mode} baseStyle={appearance.style} activeThemeId={activeThemeId} userThemes={userThemes} error={appearanceError} onBack={() => setThemeGalleryOpen(false)} onApply={async id => { if (await onThemeApply(id)) setThemeGalleryOpen(false); }} onSaveTheme={onUserThemeSave} onDeleteTheme={onUserThemeDelete} onImportTheme={onUserThemeImport} onExportTheme={onUserThemeExport} />;
  }

  return (
    <div className="tauri-settings-section tauri-settings-appearance">
      <div className="tauri-settings-style-heading">{t("settings.appearance.sectionTheme")}</div>
      <div className="tauri-settings-theme-gallery-entry">
        <span className="tauri-settings-field-label">{t("settings.themeGallery.title")}<small>{t("settings.themeGallery.subtitle")}</small></span>
        <button type="button" className="btn btn--secondary" onClick={() => { setThemeGalleryOpen(true); void onRefreshPluginThemes(); }}>{t("settings.themeGallery.browse")} <ArrowLeft size={14} className="tauri-settings-theme-gallery-entry__arrow" /></button>
      </div>
      <div className="tauri-settings-field">
        <Palette className="tauri-settings-field-icon" size={18} />
        <span className="tauri-settings-field-label">{t("settings.theme")}<small>{t("settings.appearance.themeHint")}</small></span>
        <div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.theme")}>
          {(["auto", "light", "dark"] as const satisfies readonly Theme[]).map(option => (
            <button key={option} type="button" role="radio" aria-checked={appearance.mode === option} disabled={appearanceSaving} className={`tauri-settings-radio${appearance.mode === option ? " is-active" : ""}`} onClick={() => onChange({ ...appearance, mode: option })}>
              {appearance.mode === option && <Check size={13} />}
              <span>{option === "light" ? t("settings.themeLight") : option === "dark" ? t("settings.themeDark") : t("settings.themeAuto")}</span>
            </button>
          ))}
        </div>
      </div>
      <div className="tauri-settings-style-heading tauri-settings-style-heading--field">{t("settings.themeStyle")}<small>{t("settings.appearance.styleHint")}</small></div>
      <div className="tauri-settings-style-grid" role="radiogroup" aria-label={t("settings.themeStyle")}>
        {THEME_STYLES.map(style => (
            <button key={style} type="button" role="radio" aria-checked={!activeThemeId && appearance.style === style} disabled={appearanceSaving} className={`tauri-settings-style${!activeThemeId && appearance.style === style ? " is-active" : ""}`} onClick={() => onThemeApply(style)}>
            <span className="tauri-settings-style-swatches" data-style={style} aria-hidden="true"><i /><i /><i /></span>
            <span>{t(STYLE_LABEL_KEYS[style])}</span>
            {appearance.style === style && <Check size={13} />}
          </button>
        ))}
      </div>
      {appearanceError && <p className="tauri-settings-zoom__error" role="alert">{appearanceError}</p>}
      <div className="tauri-settings-field tauri-settings-terminal-theme">
        <PanelTop className="tauri-settings-field-icon" size={18} />
        <span className="tauri-settings-field-label">{t("settings.terminalTheme")}<small>{t("settings.appearance.terminalHint")}</small></span>
        <div className="tauri-settings-terminal-theme__controls">
          <div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.terminalTheme")}>
            {(["auto", "light", "dark"] as const).map(option => (
              <button key={option} type="button" role="radio" aria-checked={terminalTheme === option} disabled={terminalThemeSaving} className={`tauri-settings-radio${terminalTheme === option ? " is-active" : ""}`} onClick={() => onTerminalThemeChange(option)}>
                {terminalTheme === option && <Check size={13} />}
              <span>{option === "auto" ? t("settings.terminalThemeAuto") : option === "light" ? t("settings.terminalThemeLight") : t("settings.terminalThemeDark")}</span>
              </button>
            ))}
          </div>
          {terminalThemeError && <small className="tauri-settings-zoom__error" role="alert">{terminalThemeError}</small>}
        </div>
      </div>
      <details className="tauri-terminal-palette">
        <summary><span><strong>{t("settings.terminalPalette.title")}</strong><small>{t("settings.terminalPalette.hint")}</small></span><ChevronDown size={15} aria-hidden="true" /></summary>
        <div className="tauri-terminal-palette__body">
          <label className="tauri-terminal-palette__toggle"><input type="checkbox" checked={customPalette.enabled} onChange={event => {
            const next = { ...customPalette, enabled: event.target.checked };
            setCustomPalette(next);
            setCustomTerminalPalette(next.colors, next.enabled);
          }} /><span>{t("settings.terminalPalette.enable")}</span></label>
          <div className="tauri-terminal-palette__grid">
            {(Object.keys(DEFAULT_TERMINAL_PALETTE) as TerminalPaletteKey[]).map(key => <label key={key} className="tauri-terminal-palette__color">
              <span>{key}</span><input aria-label={key} type="color" value={customPalette.colors[key]} onChange={event => {
                const colors: TerminalPalette = { ...customPalette.colors, [key]: event.target.value };
                const next = { ...customPalette, colors };
                setCustomPalette(next);
                setCustomTerminalPalette(colors, next.enabled);
              }} />
            </label>)}
          </div>
          <button type="button" className="tauri-settings-button" onClick={() => {
            const next = { enabled: false, colors: { ...DEFAULT_TERMINAL_PALETTE } };
            setCustomPalette(next);
            setCustomTerminalPalette(next.colors, false);
          }}>{t("settings.terminalPalette.reset")}</button>
        </div>
      </details>
      <div className="tauri-settings-style-heading">{t("settings.appearance.sectionConversation")}</div>
      <div className="tauri-settings-field">
        <Monitor className="tauri-settings-field-icon" size={18} />
        <span className="tauri-settings-field-label">{t("settings.conversationWidth")}<small>{t("settings.appearance.widthHint")}</small></span>
        <div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.conversationWidth")}>
          {(["standard", "full"] as const).map(option => (
            <button key={option} type="button" role="radio" aria-checked={conversationWidth === option} className={`tauri-settings-radio${conversationWidth === option ? " is-active" : ""}`} onClick={() => onConversationWidthChange(option)}>
              {conversationWidth === option && <Check size={13} />}
              <span>{option === "standard" ? t("settings.conversationWidthStandard") : t("settings.conversationWidthFull")}</span>
            </button>
          ))}
        </div>
      </div>
      <div className="tauri-settings-field">
        <PanelTop className="tauri-settings-field-icon" size={18} />
        <span className="tauri-settings-field-label">{t("settings.textSize")}<small>{t("settings.appearance.textSizeHint")}</small></span>
        <div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.textSize")}>
          {TEXT_SIZES.map(size => (
            <button key={size} type="button" role="radio" aria-checked={textSize === size} className={`tauri-settings-radio${textSize === size ? " is-active" : ""}`} onClick={() => onTextSizeChange(size)}>
              {textSize === size && <Check size={13} />}
              <span>{t(TEXT_SIZE_LABEL_KEYS[size])}</span>
            </button>
          ))}
        </div>
      </div>
      <DisplayZoomSetting />
      <div className="tauri-settings-style-heading">{t("settings.appearance.sectionTypography")}</div>
      <div className="tauri-settings-field">
        <Languages className="tauri-settings-field-icon" size={18} />
        <label htmlFor="tauri-settings-font" className="tauri-settings-field-label">{t("settings.fontFamily")}<small>{t("settings.appearance.fontHint")}</small></label>
        <select id="tauri-settings-font" className="tauri-settings-select" value={fontFamily} onChange={event => onFontFamilyChange(event.target.value as FontFamily)}>
          {FONT_FAMILIES.map(font => <option key={font} value={font}>{t(FONT_LABEL_KEYS[font])}</option>)}
        </select>
      </div>
      {fontFamily === "custom" && <div className="tauri-settings-field">
        <span className="tauri-settings-field-icon" />
        <label htmlFor="tauri-settings-custom-font" className="tauri-settings-field-label">{t("settings.fontFamilyCustomName")}</label>
        <input id="tauri-settings-custom-font" className="tauri-settings-input" value={customFontName} onChange={event => onCustomFontChange(event.target.value)} placeholder={t("settings.fontFamilyCustomPlaceholder")} />
      </div>}
      <div className="tauri-settings-field">
        <Keyboard className="tauri-settings-field-icon" size={18} />
        <label htmlFor="tauri-settings-mono-font" className="tauri-settings-field-label">{t("settings.monoFontFamily")}<small>{t("settings.appearance.monoFontHint")}</small></label>
        <select id="tauri-settings-mono-font" className="tauri-settings-select" value={monoFontFamily} onChange={event => onMonoFontFamilyChange(event.target.value as MonoFontFamily)}>
          {MONO_FONT_FAMILIES.map(font => <option key={font} value={font}>{t(MONO_FONT_LABEL_KEYS[font])}</option>)}
        </select>
      </div>
      {monoFontFamily === "custom" && <div className="tauri-settings-field">
        <span className="tauri-settings-field-icon" />
        <label htmlFor="tauri-settings-custom-mono-font" className="tauri-settings-field-label">{t("settings.monoFontFamilyCustomName")}</label>
        <input id="tauri-settings-custom-mono-font" className="tauri-settings-input" value={customMonoFontName} onChange={event => onCustomMonoFontChange(event.target.value)} placeholder={t("settings.monoFontFamilyCustomPlaceholder")} />
      </div>}
      <div className="tauri-settings-field tauri-settings-typography-entry">
        <div><span className="tauri-settings-field-label">{t("settings.typography.title")}</span><p>{t("settings.typography.entrySummary")}</p></div>
        <button type="button" className="tauri-settings-button" onClick={() => setTypographyOpen(true)}>{t("settings.typography.open")}</button>
      </div>
    </div>
  );
}

function DisplayZoomSetting() {
  const t = useT();
  const translatorRef = useRef(t);
  translatorRef.current = t;
  const [percent, setPercent] = useState(100);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const savedPercent = useRef(100);
  const saveQueue = useRef<Promise<void>>(Promise.resolve());
  const request = useRef(0);

  useEffect(() => {
    let active = true;
    void tauriZoomFactor().then(factor => {
      const next = Math.round(factor * 100);
      savedPercent.current = next;
      if (active) { setPercent(next); setError(""); }
    }).catch(() => { if (active) setError(translatorRef.current("settings.errorUnknown")); }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; request.current += 1; };
  }, []);

  const update = (nextValue: number) => {
    const next = Math.max(50, Math.min(200, Math.round(nextValue / 5) * 5));
    const sequence = ++request.current;
    setPercent(next);
    setError("");
    setSaving(true);
    const task = saveQueue.current.catch(() => undefined).then(() => setTauriZoomFactor(next / 100));
    saveQueue.current = task.then(() => undefined, () => undefined);
    void task.then(factor => {
      savedPercent.current = Math.round(factor * 100);
      if (sequence === request.current) setPercent(savedPercent.current);
    }).catch(() => {
      if (sequence === request.current) { setPercent(savedPercent.current); setError(t("settings.errorUnknown")); }
    }).finally(() => { if (sequence === request.current) setSaving(false); });
  };

  return (
    <div className="tauri-settings-field tauri-settings-zoom">
      <div className="tauri-settings-zoom__copy"><span className="tauri-settings-field-label">{t("settings.displayZoom")}</span>{error && <span className="tauri-settings-zoom__error" role="alert">{error}</span>}</div>
      <div className="tauri-settings-zoom__control">
        <button type="button" aria-label={t("settings.displayZoomDecrease")} disabled={loading || percent <= 50} onClick={() => update(percent - 5)}>−</button>
        <input aria-label={t("settings.displayZoom")} type="range" min="50" max="200" step="5" value={percent} disabled={loading} onChange={event => update(Number(event.target.value))} />
        <button type="button" aria-label={t("settings.displayZoomIncrease")} disabled={loading || percent >= 200} onClick={() => update(percent + 5)}>+</button>
        <output>{percent}%</output>
        <button type="button" className="tauri-settings-zoom__reset" aria-label={t("settings.displayZoomReset")} disabled={loading || percent === 100} onClick={() => update(100)}>100%</button>
        {saving && <small aria-live="polite">{t("common.loading")}</small>}
      </div>
    </div>
  );
}

function DiagnosticsSettings({ bridgeStatus, catalogAudit, catalogAuditError, sessionPageSource, hostError, workspaceRoot, busy, onRestartBridge, onRefreshCatalogAudit, onOpenData, onOpenProviders, onNavigateSettings }: {
  bridgeStatus?: TauriBridgeStatus | null;
  catalogAudit?: TauriSessionShadowReport | null;
  catalogAuditError?: string;
  sessionPageSource?: string;
  hostError?: string;
  workspaceRoot?: string;
  busy: boolean;
  onRestartBridge?: () => Promise<boolean>;
  onRefreshCatalogAudit?: () => Promise<void>;
  onOpenData: () => void;
  onOpenProviders: () => void;
  onNavigateSettings: (tab: TauriSettingsTab) => void;
}) {
  const t = useT();
  const [restarting, setRestarting] = useState(false);
  const [checking, setChecking] = useState(false);
  const [auditRefreshError, setAuditRefreshError] = useState(false);
  const [restartResult, setRestartResult] = useState<"ok" | "failed" | "">("");
  const loadCapabilities = useCallback((includeRuntime: boolean) => tauriCapabilityDiagnostics(workspaceRoot ?? "", includeRuntime), [workspaceRoot]);
  const sourceLabel = t(sessionPageSource === "identity" ? "settings.diagnostics.source.identity" : sessionPageSource === "partial_identity" ? "settings.diagnostics.source.partialIdentity" : sessionPageSource === "identity_unverified" ? "settings.diagnostics.source.unverified" : sessionPageSource === "legacy" ? "settings.diagnostics.source.legacy" : sessionPageSource === "cached" ? "settings.diagnostics.source.cached" : "settings.diagnostics.source.unavailable");
  const restart = async () => {
    if (!onRestartBridge || restarting) return;
    setRestarting(true);
    setRestartResult("");
    try {
      setRestartResult(await onRestartBridge() ? "ok" : "failed");
    } catch {
      setRestartResult("failed");
    } finally {
      setRestarting(false);
    }
  };
  const refreshAudit = async () => {
    if (!onRefreshCatalogAudit || checking) return;
    setChecking(true);
    setAuditRefreshError(false);
    try {
      await onRefreshCatalogAudit();
    } catch {
      setAuditRefreshError(true);
    } finally {
      setChecking(false);
    }
  };
  return <div className="tauri-settings-section tauri-settings-diagnostics">
    <h3>{t("settings.diagnostics.localService")}</h3>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.diagnostics.bridge")}<small>{t("settings.diagnostics.bridgeHint")}</small></span><span className={`tauri-settings-badge${bridgeStatus?.running ? " is-ready" : ""}`}>{bridgeStatus?.running ? t("settings.diagnostics.bridgeRunning", { protocol: bridgeStatus.protocolVersion ?? "?" }) : t("settings.diagnostics.notConnected")}</span></div>
    {onRestartBridge && <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" onClick={() => void restart()} disabled={busy || restarting}>{t("settings.diagnostics.restartBridge")}</button></div>}
    {restartResult === "ok" && <p className="tauri-settings-data__notice" role="status">{t("settings.diagnostics.bridgeRestarted")}</p>}
    {restartResult === "failed" && <p className="tauri-diagnostic-error" role="alert">{hostError || t("settings.diagnostics.bridgeRestartFailed")}</p>}
    <h3>{t("settings.diagnostics.sessionCatalog")}</h3>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.diagnostics.activeSource")}<small>{t("settings.diagnostics.sourceHint")}</small></span><span className="tauri-settings-value">{sourceLabel}</span></div>
    {catalogAudit ? <div className="tauri-settings-audit">
      <div><span>{t("settings.diagnostics.legacyCatalog")}</span><strong>{catalogAudit.legacyCount}</strong></div><div><span>{t("settings.diagnostics.identityCatalog")}</span><strong>{catalogAudit.directoryCount}</strong></div><div><span>{t("settings.diagnostics.matched")}</span><strong>{catalogAudit.matchedCount}</strong></div><div><span>{t("settings.diagnostics.unclaimed")}</span><strong>{catalogAudit.unclaimedTranscripts}</strong></div>
      <p>{catalogAudit.legacyMatchesDirectory ? t("settings.diagnostics.catalogConsistent") : t("settings.diagnostics.catalogDifferences", { missing: catalogAudit.missingFromDirectory, titles: catalogAudit.titleMismatches, workspaces: catalogAudit.workspaceMismatches, order: catalogAudit.orderMismatches, disk: catalogAudit.physicalStateMismatches, errors: catalogAudit.inventoryErrors })}</p>
    </div> : <p>{catalogAuditError || t("settings.diagnostics.catalogLoading")}</p>}
    {catalogAudit && catalogAuditError && <p className="tauri-diagnostic-error" role="alert">{catalogAuditError}</p>}
    {auditRefreshError && <p className="tauri-diagnostic-error" role="alert">{t("settings.diagnostics.catalogCheckFailed")}</p>}
    <div className="tauri-settings-actions">
      {onRefreshCatalogAudit && <button type="button" className="tauri-settings-button" onClick={() => void refreshAudit()} disabled={busy || checking}><RefreshCw size={13} />{t("settings.diagnostics.recheck")}</button>}
      <button type="button" className="tauri-settings-button" onClick={onOpenData}>{t("settings.diagnostics.openData")}</button>
      <button type="button" className="tauri-settings-button" onClick={onOpenProviders}>{t("settings.diagnostics.openProviders")}</button>
    </div>
    <h3>{t("settings.diagnostics.capabilityTitle")}</h3>
    <Suspense fallback={<div className="tauri-settings-loading">{t("common.loading")}</div>}>
      <TauriCapabilityDiagnosticsPage capabilityDiagnostics={loadCapabilities} runtimeDoctorProvider={tauriRuntimeDoctor} onNavigate={tab => onNavigateSettings(tab as TauriSettingsTab)} />
    </Suspense>
  </div>;
}

function ModelPreferenceSettings({ providerSummary: initialProviderSummary, workspaceRoot, currentSessionState, currentSessionModelRef, onCurrentSessionModelChange, currentSessionHasAttachments, onApplyToCurrentSession, onModelChange, onRoleChange, onAgentPreferenceChange, saving, error, onOpenProviders }: {
  providerSummary: TauriProviderSummary;
  workspaceRoot?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionModelRef?: string;
  onCurrentSessionModelChange?: (model: string) => Promise<boolean>;
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
  onModelChange: (model: string, scope: "global" | "project", workspaceRoot?: string) => Promise<TauriProviderSummary | undefined>;
  onRoleChange: (role: "planner" | "vision" | "search", model: string, scope: "global" | "project", workspaceRoot?: string) => Promise<TauriProviderSummary | undefined>;
  onAgentPreferenceChange: (request: { reasoningLanguage?: "auto" | "zh" | "en"; compactRatioPercent?: number }, scope: "global" | "project", workspaceRoot?: string) => Promise<TauriProviderSummary | undefined>;
  saving: boolean;
  error: string;
  onOpenProviders: () => void;
}) {
  const t = useT();
  const [scope, setScope] = useState<"global" | "project">("global");
  const [providerSummary, setProviderSummary] = useState(initialProviderSummary);
  const [scopeLoading, setScopeLoading] = useState(false);
  const [scopeLoadError, setScopeLoadError] = useState(false);
  const [sessionModelSaving, setSessionModelSaving] = useState(false);
  const [sessionModelError, setSessionModelError] = useState("");
  const [sessionModelNotice, setSessionModelNotice] = useState("");
  const [compactRatioDraft, setCompactRatioDraft] = useState(String(providerSummary.compactRatioPercent));
  const [compactRatioCustomEditing, setCompactRatioCustomEditing] = useState(false);
  const compactRatioCustomInputRef = useRef<HTMLInputElement>(null);
  const compactRatioCancelBlurRef = useRef(false);
  useEffect(() => {
    if (scope === "global") {
      setProviderSummary(initialProviderSummary);
      setScopeLoadError(false);
    }
  }, [initialProviderSummary, scope]);
  useEffect(() => {
    if (scope !== "project") return;
    if (!workspaceRoot) {
      setScope("global");
      return;
    }
    let active = true;
    setScopeLoading(true);
    setScopeLoadError(false);
    void tauriProviderSummary(workspaceRoot, "project").then(summary => {
      if (active) setProviderSummary(summary);
    }).catch(() => {
      if (active) setScopeLoadError(true);
    }).finally(() => {
      if (active) setScopeLoading(false);
    });
    return () => { active = false; };
  }, [scope, workspaceRoot]);
  const saveScopedSummary = async (save: () => Promise<TauriProviderSummary | undefined>) => {
    const updated = await save();
    if (updated) setProviderSummary(updated);
  };
  const compactRatioPreset = TAURI_COMPACT_RATIO_PRESETS.find(([percent]) => Math.abs(providerSummary.compactRatioPercent - percent) < 0.0001);
  const compactRatioDraftPercent = Number(compactRatioDraft);
  const compactRatioDraftValid = compactRatioDraft !== ""
    && Number.isFinite(compactRatioDraftPercent)
    && compactRatioDraftPercent >= COMPACT_RATIO_MIN_PERCENT
    && compactRatioDraftPercent <= COMPACT_RATIO_MAX_PERCENT
    && Math.abs(compactRatioDraftPercent * 10 - Math.round(compactRatioDraftPercent * 10)) < 0.0001;
  useEffect(() => {
    if (!compactRatioCustomEditing) setCompactRatioDraft(compactRatioPreset ? "" : String(providerSummary.compactRatioPercent));
  }, [compactRatioCustomEditing, compactRatioPreset, providerSummary.compactRatioPercent]);
  const available = providerSummary.providers.filter(provider => provider.configured && provider.models.length > 0);
  const modelRefs = available.flatMap(provider => provider.models.map(model => `${provider.name}/${model}`));
  const visionRefs = available.flatMap(provider => provider.visionModels.map(model => `${provider.name}/${model}`));
  const searchRefs = available.flatMap(provider => provider.searchModels.map(model => `${provider.name}/${model}`));
  const currentAvailable = modelRefs.includes(providerSummary.defaultModel);
  const plannerAvailable = !providerSummary.plannerModel || modelRefs.includes(providerSummary.plannerModel);
  const visionAvailable = !providerSummary.visionModel || providerSummary.visionModel === "auto" || visionRefs.includes(providerSummary.visionModel);
  const searchAvailable = !providerSummary.webSearchModel || providerSummary.webSearchModel === "auto" || searchRefs.includes(providerSummary.webSearchModel);
  const connection = (ref: string, ready: boolean, empty: string) => {
    if (!ref) return empty;
    if (ref === "auto") return t("settings.connectionAutomatic");
    if (!ready) return t("settings.modelPrefs.connectionUnavailable");
    const provider = available.find(item => ref.startsWith(`${item.name}/`));
    return provider?.displayName || provider?.name || t("settings.modelPrefs.connectionUnavailable");
  };
  const changeCurrentSessionModel = async (model: string) => {
    if (!model || !onCurrentSessionModelChange || sessionModelSaving) return;
    setSessionModelSaving(true);
    setSessionModelError("");
    setSessionModelNotice("");
    try {
      if (await onCurrentSessionModelChange(model)) setSessionModelNotice(t("settings.modelPrefs.sessionModelChanged"));
      else setSessionModelError(t("settings.modelPrefs.sessionModelChangeFailed"));
    } catch {
      setSessionModelError(t("settings.modelPrefs.sessionModelChangeFailed"));
    } finally {
      setSessionModelSaving(false);
    }
  };
  const selectCompactRatioPreset = (percent: number) => {
    if (document.activeElement === compactRatioCustomInputRef.current) {
      compactRatioCancelBlurRef.current = true;
      compactRatioCustomInputRef.current?.blur();
    }
    setCompactRatioCustomEditing(false);
    if (Math.abs(providerSummary.compactRatioPercent - percent) >= 0.0001) void saveScopedSummary(() => onAgentPreferenceChange({ compactRatioPercent: percent }, scope, scope === "project" ? workspaceRoot : undefined));
  };
  const focusCompactRatioCustom = () => {
    setCompactRatioCustomEditing(true);
    requestAnimationFrame(() => compactRatioCustomInputRef.current?.focus());
  };
  const commitCompactRatioDraft = (rawValue: string) => {
    const percent = Number(rawValue);
    const valid = rawValue !== ""
      && Number.isFinite(percent)
      && percent >= COMPACT_RATIO_MIN_PERCENT
      && percent <= COMPACT_RATIO_MAX_PERCENT
      && Math.abs(percent * 10 - Math.round(percent * 10)) < 0.0001;
    if (!valid && rawValue !== "") return;
    if (!valid || saving) {
      setCompactRatioCustomEditing(false);
      return;
    }
    if (Math.abs(percent - providerSummary.compactRatioPercent) >= 0.0001) void saveScopedSummary(() => onAgentPreferenceChange({ compactRatioPercent: percent }, scope, scope === "project" ? workspaceRoot : undefined));
    setCompactRatioCustomEditing(false);
  };
  return <div className="tauri-settings-section tauri-model-settings model-preferences">
    <SettingsSection className="tauri-model-settings__section" title={t("settings.modelAssignment")} description={t("settings.modelPrefs.description", { config: "reasonix.toml" })}>
    <div className="tauri-settings-field tauri-model-settings__scope">
      <span className="tauri-settings-field-label">{t("settings.modelPrefs.scopeLabel")}<small>{t("settings.modelPrefs.scopeHint")}</small></span>
      <TauriSettingsChoice ariaLabel={t("settings.modelPrefs.scopeLabel")} value={scope} disabled={saving || scopeLoading || !workspaceRoot} options={[{ value: "global", label: t("settings.skills.scope.global") }, ...(workspaceRoot ? [{ value: "project" as const, label: t("settings.skills.scope.project") }] : [])]} onChange={next => { setScopeLoadError(false); setScope(next); }} />
    </div>
    {scopeLoadError && <p className="tauri-diagnostic-error" role="alert">{t("settings.modelPrefs.scopeLoadError")}</p>}
    {currentSessionState !== undefined && onCurrentSessionModelChange && <>
      <div className="tauri-settings-field tauri-model-settings__current-session">
        <label className="tauri-settings-field-label" htmlFor="tauri-settings-current-session-model">{t("settings.modelPrefs.currentSessionModel")}<small>{t("settings.modelPrefs.currentSessionModelHint")}</small></label>
        <select id="tauri-settings-current-session-model" className="tauri-settings-select" value={currentSessionModelRef || ""} disabled={saving || sessionModelSaving || currentSessionState !== "idle" || currentSessionHasAttachments} onChange={event => void changeCurrentSessionModel(event.target.value)}>
          {!currentSessionModelRef && <option value="">{t("settings.modelPrefs.sessionModelUnavailable")}</option>}
          {currentSessionModelRef && !modelRefs.includes(currentSessionModelRef) && <option value={currentSessionModelRef} disabled>{currentSessionModelRef} ({t("settings.modelPrefs.unavailable")})</option>}
          {available.map(provider => <optgroup key={provider.name} label={provider.displayName || provider.name}>{provider.models.map(model => <option key={model} value={`${provider.name}/${model}`}>{model}</option>)}</optgroup>)}
        </select>
        <span className="tauri-model-settings__connection">{sessionModelSaving ? t("common.loading") : connection(currentSessionModelRef || "", Boolean(currentSessionModelRef && modelRefs.includes(currentSessionModelRef)), t("settings.modelPrefs.sessionModelUnavailable"))}</span>
      </div>
      {sessionModelError && <p className="tauri-diagnostic-error" role="alert">{sessionModelError}</p>}
      {sessionModelNotice && <p className="tauri-settings-data__notice" role="status">{sessionModelNotice}</p>}
    </>}
    {available.length === 0 ? <div className="tauri-settings-empty">{t("settings.modelPrefs.noAvailableProviders")}</div> : <>
      <div className="tauri-model-settings__head" aria-hidden="true"><span>{t("settings.modelPurpose")}</span><span>{t("settings.modelUsage")}</span><span>{t("settings.modelConnection")}</span></div>
      <div className="tauri-settings-field tauri-model-settings__row">
        <label className="tauri-settings-field-label" htmlFor="tauri-settings-default-model">{t("settings.defaultModel")}<small>{t("settings.defaultModelHint")}</small></label>
        <select id="tauri-settings-default-model" className="tauri-settings-select" value={currentAvailable ? providerSummary.defaultModel : ""} disabled={saving || scopeLoading || scopeLoadError} onChange={event => { if (event.target.value) void saveScopedSummary(() => onModelChange(event.target.value, scope, scope === "project" ? workspaceRoot : undefined)); }}>
          <option value="">{t("settings.modelPrefs.choose")}</option>
          {available.map(provider => <optgroup key={provider.name} label={provider.displayName || provider.name}>{provider.models.map(model => <option key={model} value={`${provider.name}/${model}`}>{model}</option>)}</optgroup>)}
        </select>
        <span className="tauri-model-settings__connection">{connection(providerSummary.defaultModel, currentAvailable, t("common.none"))}</span>
      </div>
      {!currentAvailable && providerSummary.defaultModel && <p className="tauri-model-settings__stale">{t("settings.modelPrefs.staleDefault", { model: providerSummary.defaultModel })}</p>}
      <div className="tauri-settings-field tauri-model-settings__row">
        <label className="tauri-settings-field-label" htmlFor="tauri-settings-planner-model">{t("settings.plannerModel")}<small>{t("settings.modelPrefs.plannerHint")}</small></label>
        <select id="tauri-settings-planner-model" className="tauri-settings-select" value={providerSummary.plannerModel} disabled={saving || scopeLoading || scopeLoadError} onChange={event => void saveScopedSummary(() => onRoleChange("planner", event.target.value, scope, scope === "project" ? workspaceRoot : undefined))}>
          <option value="">{t("settings.plannerNone")}</option>
          {!plannerAvailable && <option value={providerSummary.plannerModel} disabled>{providerSummary.plannerModel} ({t("settings.modelPrefs.unavailable")})</option>}
          {available.map(provider => <optgroup key={provider.name} label={provider.displayName || provider.name}>{provider.models.map(model => <option key={model} value={`${provider.name}/${model}`}>{model}</option>)}</optgroup>)}
        </select>
        <span className="tauri-model-settings__connection">{connection(providerSummary.plannerModel, plannerAvailable, t("settings.connectionFollowSession"))}</span>
      </div>
      <div className="tauri-settings-field tauri-model-settings__row">
        <label className="tauri-settings-field-label" htmlFor="tauri-settings-vision-model">{t("settings.imageUnderstandingModel")}<small>{t("settings.modelPrefs.visionHint")}</small></label>
        <select id="tauri-settings-vision-model" className="tauri-settings-select" value={providerSummary.visionModel} disabled={saving || scopeLoading || scopeLoadError} onChange={event => void saveScopedSummary(() => onRoleChange("vision", event.target.value, scope, scope === "project" ? workspaceRoot : undefined))}>
          <option value="">{t("common.none")}</option><option value="auto">{t("settings.connectionAutomatic")}</option>
          {!visionAvailable && <option value={providerSummary.visionModel} disabled>{providerSummary.visionModel} ({t("settings.modelPrefs.unavailable")})</option>}
          {available.map(provider => provider.visionModels.length > 0 && <optgroup key={provider.name} label={provider.displayName || provider.name}>{provider.visionModels.map(model => <option key={model} value={`${provider.name}/${model}`}>{model}</option>)}</optgroup>)}
        </select>
        <span className="tauri-model-settings__connection">{connection(providerSummary.visionModel, visionAvailable, t("common.none"))}</span>
      </div>
      {visionRefs.length === 0 && <p className="tauri-settings-hint">{t("settings.modelPrefs.noVisionModels")}</p>}
      <div className="tauri-settings-field tauri-model-settings__row">
        <label className="tauri-settings-field-label" htmlFor="tauri-settings-search-model">{t("settings.webSearchModel")}<small>{t("settings.modelPrefs.searchHint")}</small></label>
        <select id="tauri-settings-search-model" className="tauri-settings-select" value={providerSummary.webSearchModel || "auto"} disabled={saving || scopeLoading || scopeLoadError} onChange={event => void saveScopedSummary(() => onRoleChange("search", event.target.value, scope, scope === "project" ? workspaceRoot : undefined))}>
          <option value="auto">{t("settings.connectionAutomatic")}</option>
          {!searchAvailable && <option value={providerSummary.webSearchModel} disabled>{providerSummary.webSearchModel} ({t("settings.modelPrefs.unavailable")})</option>}
          {available.map(provider => provider.searchModels.length > 0 && <optgroup key={provider.name} label={provider.displayName || provider.name}>{provider.searchModels.map(model => <option key={model} value={`${provider.name}/${model}`}>{model}</option>)}</optgroup>)}
        </select>
        <span className="tauri-model-settings__connection">{connection(providerSummary.webSearchModel || "auto", searchAvailable, t("settings.connectionAutomatic"))}</span>
      </div>
      {searchRefs.length === 0 && <p className="tauri-settings-hint">{t("settings.modelPrefs.noSearchModels")}</p>}
    </>}
    <details className="tauri-model-settings__advanced">
      <summary>{t("settings.subagents.defaultsTitle")}<ChevronDown size={15} aria-hidden="true" /></summary>
      <p>{t("settings.subagents.defaultsDescription")}</p>
      <TauriSubagentSettings defaultsOnly workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />
    </details>
    </SettingsSection>
    <SettingsSection className="tauri-model-settings__section tauri-model-settings__section--behavior" title={t("settings.modelPrefs.agentBehaviorTitle")} description={t("settings.modelPrefs.agentBehaviorHint")}>
    <div className="tauri-settings-field">
      <span className="tauri-settings-field-label">{t("settings.reasoningLanguage")}<small>{t("settings.reasoningLanguageHint")}</small></span>
      <TauriSettingsChoice ariaLabel={t("settings.reasoningLanguage")} value={providerSummary.reasoningLanguage} disabled={saving || scopeLoading || scopeLoadError} options={(["auto", "zh", "en"] as const).map(language => ({ value: language, label: t(`settings.reasoningLanguage.${language}` as DictKey) }))} onChange={reasoningLanguage => void saveScopedSummary(() => onAgentPreferenceChange({ reasoningLanguage }, scope, scope === "project" ? workspaceRoot : undefined))} />
    </div>
    <div className="tauri-settings-field tauri-model-settings__ratio-field">
      <div className="tauri-settings-field-label">{t("settings.compactRatio")}<small>{t("settings.compactRatioHint")}</small></div>
      <div className="compact-ratio-controls">
        <fieldset className="compact-ratio-choice-list">
          <legend className="sr-only">{t("settings.compactRatio")}</legend>
          {TAURI_COMPACT_RATIO_PRESETS.map(([percent, labelKey, effectKey]) => {
            const selected = Math.abs(providerSummary.compactRatioPercent - percent) < 0.0001;
            const [valueLabel, name] = t(labelKey).split(" · ");
            return <div key={percent} className="compact-ratio-choice" data-selected={selected || undefined}>
              <label className="compact-ratio-choice__row" onClick={() => { if (selected && !saving) selectCompactRatioPreset(percent); }}>
                <input type="radio" name="tauri-settings-compact-ratio" value={percent} checked={selected} disabled={saving || scopeLoading || scopeLoadError} aria-label={t(labelKey)} onChange={() => selectCompactRatioPreset(percent)} />
                <span className="compact-ratio-choice__name">{name}</span>
                <span className="compact-ratio-choice__percent">{valueLabel}</span>
                <span className="compact-ratio-choice__effect">— {t(effectKey)}</span>
                {percent === 80 && <span className="compact-ratio-choice__badge">{t("settings.recommended")}</span>}
              </label>
            </div>;
          })}
          <div className="compact-ratio-choice" data-selected={!compactRatioPreset || undefined}>
            <div className="compact-ratio-choice__row compact-ratio-choice__row--custom">
              <input id="tauri-settings-compact-ratio-custom-choice" type="radio" name="tauri-settings-compact-ratio" value="custom" checked={!compactRatioPreset} disabled={saving} aria-label={t("settings.compactRatioCustomOption")} onChange={focusCompactRatioCustom} />
              <label className="compact-ratio-choice__name" htmlFor="tauri-settings-compact-ratio-custom-choice">{t("settings.typography.customized")}</label>
              <label className="compact-ratio-choice__inline-input" htmlFor="tauri-settings-compact-ratio-custom">
                <input
                  ref={compactRatioCustomInputRef}
                  id="tauri-settings-compact-ratio-custom"
                  type="number"
                  min={COMPACT_RATIO_MIN_PERCENT}
                  max={COMPACT_RATIO_MAX_PERCENT}
                  step={0.1}
                  inputMode="decimal"
                  value={compactRatioDraft}
                  placeholder={t("settings.compactRatioCustomPlaceholder")}
                  disabled={saving}
                  aria-label={t("settings.compactRatioCustomAria")}
                  aria-invalid={compactRatioDraft !== "" && !compactRatioDraftValid}
                  onFocus={() => setCompactRatioCustomEditing(true)}
                  onInput={event => setCompactRatioDraft(event.currentTarget.value)}
                  onBlur={event => {
                    if (compactRatioCancelBlurRef.current) { compactRatioCancelBlurRef.current = false; return; }
                    commitCompactRatioDraft(event.currentTarget.value);
                  }}
                  onKeyDown={event => {
                    if (event.key === "Enter") { event.preventDefault(); event.currentTarget.blur(); }
                    if (event.key === "Escape") {
                      event.preventDefault();
                      compactRatioCancelBlurRef.current = true;
                      setCompactRatioDraft(compactRatioPreset ? "" : String(providerSummary.compactRatioPercent));
                      setCompactRatioCustomEditing(false);
                      event.currentTarget.blur();
                    }
                  }}
                />
                <span aria-hidden="true">%</span>
              </label>
            </div>
          </div>
        </fieldset>
        <p className="tauri-settings-hint">{t("settings.compactRatioCustomHint")}</p>
      </div>
    </div>
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" onClick={onOpenProviders}>{t("settings.webSearchModelConnections")}</button></div>
    </SettingsSection>
  </div>;
}

function ProviderSettings({ providerSummary, onProviderSummaryChange, currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  providerSummary: TauriProviderSummary;
  onProviderSummaryChange: (summary: TauriProviderSummary) => void;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const t = useT();
  const [apiKeyInputs, setApiKeyInputs] = useState<Record<string, string>>({});
  const [showApiKey, setShowApiKey] = useState<Record<string, boolean>>({});
  const [keyStatus, setKeyStatus] = useState<Record<string, { message: string; error: boolean } | null>>({});
  const [keyBusy, setKeyBusy] = useState(false);
  const [pendingApply, setPendingApply] = useState<Record<string, boolean>>({});
  const [probeModels, setProbeModels] = useState<Record<string, string>>({});
  const [probeBusy, setProbeBusy] = useState<Record<string, boolean>>({});
  const [probeStatus, setProbeStatus] = useState<Record<string, { message: string; error: boolean } | null>>({});
  const keyBusyRef = useRef(false);
  const statusTimers = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  const mounted = useRef(false);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      for (const timer of statusTimers.current.values()) clearTimeout(timer);
      statusTimers.current.clear();
    };
  }, []);

  const showStatus = (providerName: string, message: string, error = false) => {
    if (!mounted.current) return;
    const previousTimer = statusTimers.current.get(providerName);
    if (previousTimer) clearTimeout(previousTimer);
    setKeyStatus(prev => ({ ...prev, [providerName]: { message, error } }));
    const timer = setTimeout(() => {
      statusTimers.current.delete(providerName);
      setKeyStatus(prev => ({ ...prev, [providerName]: null }));
    }, 2000);
    statusTimers.current.set(providerName, timer);
  };

  const handleApiKeyAction = async (providerName: string, action: "save" | "delete" | "import") => {
    const key = apiKeyInputs[providerName];
    if (keyBusyRef.current || (action === "save" && !key)) return;
    keyBusyRef.current = true;
    setKeyBusy(true);
    const previousTimer = statusTimers.current.get(providerName);
    if (previousTimer) clearTimeout(previousTimer);
    statusTimers.current.delete(providerName);
    setKeyStatus(prev => ({ ...prev, [providerName]: null }));
    try {
      let deleted = false;
      if (action === "save") await keychainSave(`api_key_${providerName}`, key);
      else if (action === "import") await keychainImportLegacy(providerName);
      else deleted = await keychainDelete(`api_key_${providerName}`);
      if (action === "save" && mounted.current) {
        setApiKeyInputs(prev => ({ ...prev, [providerName]: prev[providerName] === key ? "" : prev[providerName] }));
      }
      if (mounted.current && (action !== "delete" || deleted)) {
        setPendingApply(prev => ({ ...prev, [providerName]: true }));
      }
      try {
        const summary = await tauriProviderSummary();
        if (mounted.current) onProviderSummaryChange(summary);
        const configured = summary.providers.find(provider => provider.name === providerName)?.configured;
        showStatus(providerName, action !== "delete" ? t("settings.previewProvider.keySaved") : !deleted ? t("settings.previewProvider.keyMissing") : configured ? t("settings.previewProvider.keyDeletedOther") : t("settings.previewProvider.keyDeleted"));
      } catch {
        showStatus(providerName, action !== "delete" ? t("settings.previewProvider.keySavedRefreshFailed") : deleted ? t("settings.previewProvider.keyDeletedRefreshFailed") : t("settings.previewProvider.keyMissingRefreshFailed"), true);
      }
    } catch {
      showStatus(providerName, action === "save" ? t("settings.models.keySaveFailed") : action === "import" ? t("settings.previewProvider.keyImportFailed") : t("settings.previewProvider.keyDeleteFailed"), true);
    } finally {
      keyBusyRef.current = false;
      if (mounted.current) setKeyBusy(false);
    }
  };

  const handleApplyToCurrentSession = async (providerName: string) => {
    if (!onApplyToCurrentSession || !pendingApply[providerName] || keyBusyRef.current) return;
    keyBusyRef.current = true;
    setKeyBusy(true);
    try {
      const applied = await onApplyToCurrentSession();
      if (applied && mounted.current) {
        setPendingApply({});
      }
      showStatus(providerName, applied ? t("settings.permission.applied") : t("settings.permission.applyFailed"), !applied);
    } catch {
      showStatus(providerName, t("settings.permission.applyFailed"), true);
    } finally {
      keyBusyRef.current = false;
      if (mounted.current) setKeyBusy(false);
    }
  };

  const handleProviderProbe = async (provider: TauriProviderSummary["providers"][number]) => {
    const model = probeModels[provider.name] || provider.models[0] || "";
    if (!model || probeBusy[provider.name]) return;
    setProbeBusy(prev => ({ ...prev, [provider.name]: true }));
    setProbeStatus(prev => ({ ...prev, [provider.name]: null }));
    try {
      const result = await testTauriProviderModel({ name: provider.name, model, apiKey: apiKeyInputs[provider.name] || undefined });
      setProbeStatus(prev => ({ ...prev, [provider.name]: { message: t("settings.previewProvider.probeSuccess", { latency: result.latencyMillis }), error: false } }));
    } catch {
      setProbeStatus(prev => ({ ...prev, [provider.name]: { message: t("settings.previewProvider.probeFailed"), error: true } }));
    } finally {
      setProbeBusy(prev => ({ ...prev, [provider.name]: false }));
    }
  };

  return (
    <div className="tauri-settings-section">
      <h3>{t("settings.models.services")}</h3>
      <p className="tauri-settings-hint">{t("settings.previewProvider.pageHint")}</p>
      <TauriProviderEditor onSummaryChange={onProviderSummaryChange} />
      {providerSummary.providers.length === 0 ? (
        <div className="tauri-settings-empty">{t("settings.previewProvider.noServices")}</div>
      ) : (
        <div className="tauri-settings-model-list">
          {providerSummary.providers.map(provider => (
            <div key={provider.name} className="tauri-settings-model-card">
              <div className="tauri-settings-model-header">
                <strong>{provider.displayName || provider.name}</strong>
                <span className={`tauri-settings-badge${provider.configured ? " is-ready" : ""}`}>{provider.configured ? t("settings.previewProvider.ready") : t("settings.previewProvider.notConfigured")}</span>
              </div>
              <p className="tauri-settings-model-meta">{provider.kind} · {t("settings.previewProvider.modelCount", { count: provider.modelCount })}</p>
              <div className="tauri-settings-provider-probe">
                <label>{t("settings.previewProvider.probeModel")}
                  <select className="tauri-settings-input" aria-label={t("settings.previewProvider.probeModelFor", { name: provider.displayName || provider.name })} value={probeModels[provider.name] || provider.models[0] || ""} disabled={probeBusy[provider.name] || provider.models.length === 0} onChange={event => setProbeModels(prev => ({ ...prev, [provider.name]: event.target.value }))}>
                    {provider.models.length === 0 && <option value="">{t("settings.modelPrefs.choose")}</option>}
                    {provider.models.map(model => <option key={model} value={model}>{model}</option>)}
                  </select>
                </label>
                <button type="button" className="tauri-settings-button" disabled={probeBusy[provider.name] || provider.models.length === 0} onClick={() => void handleProviderProbe(provider)}>{probeBusy[provider.name] ? t("settings.previewProvider.probeRunning") : t("settings.previewProvider.probe")}</button>
                {probeStatus[provider.name] && <span role={probeStatus[provider.name]?.error ? "alert" : "status"} className={`tauri-settings-provider-probe-status${probeStatus[provider.name]?.error ? " is-error" : ""}`}>{probeStatus[provider.name]?.message}</span>}
              </div>
              {provider.requiresKey && (
                <div className="tauri-settings-apikey">
                  <div className="tauri-settings-apikey-input">
                    <Key size={13} />
                    <input
                      type={showApiKey[provider.name] ? "text" : "password"}
                      aria-label={t("settings.previewProvider.apiKeyFor", { name: provider.displayName || provider.name })}
                      placeholder={provider.configured ? t("settings.previewProvider.credentialConfigured") : t("settings.previewProvider.enterAPIKey")}
                      value={apiKeyInputs[provider.name] ?? ""}
                      onChange={e => setApiKeyInputs(prev => ({ ...prev, [provider.name]: e.target.value }))}
                    />
                    <button type="button" aria-label={t(showApiKey[provider.name] ? "settings.previewProvider.hideAPIKey" : "settings.previewProvider.showAPIKey")} onClick={() => setShowApiKey(prev => ({ ...prev, [provider.name]: !prev[provider.name] }))}>
                      {showApiKey[provider.name] ? <EyeOff size={13} /> : <Eye size={13} />}
                    </button>
                  </div>
                  <div className="tauri-settings-apikey-actions">
                    <button type="button" className="tauri-settings-button" onClick={() => void handleApiKeyAction(provider.name, "save")} disabled={keyBusy || !apiKeyInputs[provider.name]}>
                      {t("settings.previewProvider.saveKey")}
                    </button>
                    <button type="button" className="tauri-settings-button" onClick={() => void handleApiKeyAction(provider.name, "import")} disabled={keyBusy}>
                      {t("settings.previewProvider.importLegacyKey")}
                    </button>
                    <button type="button" className="tauri-settings-button tauri-settings-button--danger" onClick={() => void handleApiKeyAction(provider.name, "delete")} disabled={keyBusy}>
                      {t("common.delete")}
                    </button>
                    {pendingApply[provider.name] && currentSessionState && onApplyToCurrentSession && (
                      <button type="button" className="tauri-settings-button" onClick={() => void handleApplyToCurrentSession(provider.name)} disabled={keyBusy || currentSessionState !== "idle" || currentSessionHasAttachments} title={currentSessionState !== "idle" ? t("common.busyHint") : currentSessionHasAttachments ? t("settings.previewProvider.resolveAttachments") : undefined}>
                        {t("settings.permission.applyCurrent")}
                      </button>
                    )}
                    {keyStatus[provider.name] && <span className={`tauri-settings-apikey-status${keyStatus[provider.name]?.error ? " is-error" : " is-success"}`}>{keyStatus[provider.name]?.message}</span>}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
      <p className="tauri-settings-hint">{t("settings.previewProvider.keychainHint")}</p>
    </div>
  );
}

function AboutSettings({ runtimeInfo, platform, onRefresh }: { runtimeInfo: TauriPreviewRuntimeInfo | null; platform: string; onRefresh: () => void }) {
  const t = useT();
  const [linkError, setLinkError] = useState(false);
  return (
    <div className="tauri-settings-section">
      <h3>{t("settings.about.title")}</h3>
      {runtimeInfo ? (
        <div className="tauri-settings-about">
          <div className="tauri-settings-about-row"><span>{t("settings.about.platform")}</span><span>{platform === "darwin" ? "macOS" : platform === "windows" ? "Windows" : platform === "linux" ? "Linux" : t("settings.about.unknownPlatform")}</span></div>
          <div className="tauri-settings-about-row"><span>{t("settings.about.previewVersion")}</span><span>v{runtimeInfo.previewVersion}</span></div>
          <div className="tauri-settings-about-row"><span>{t("settings.about.stableBaseline")}</span><span>v{runtimeInfo.stableVersion}</span></div>
          <div className="tauri-settings-about-row"><span>{t("settings.about.tauri")}</span><span>v{runtimeInfo.tauriVersion}</span></div>
          <div className="tauri-settings-about-row"><span>{t("settings.about.bridgeProtocol")}</span><span>v{runtimeInfo.bridgeProtocolVersion}</span></div>
          <div className="tauri-settings-about-row"><span>{t("settings.about.buildTime")}</span><span>{runtimeInfo.previewBuild}</span></div>
          <div className="tauri-settings-about-row"><span>{t("settings.about.sidecar")}</span><span>{runtimeInfo.sidecarInstanceId ?? t("settings.about.notRunning")}</span></div>
        </div>
      ) : (
        <p>{t("settings.about.loading")}</p>
      )}
      <div className="tauri-settings-actions">
        <button type="button" className="tauri-settings-button" onClick={onRefresh}><RefreshCw size={14} /> {t("settings.about.refresh")}</button>
        <a className="tauri-settings-button" href="https://github.com/esengine/DeepSeek-Reasonix" onClick={event => {
          event.preventDefault();
          setLinkError(false);
          void openTauriExternalURL(event.currentTarget.href).catch(() => setLinkError(true));
        }}><ExternalLink size={14} /> {t("settings.about.github")}</a>
      </div>
      {linkError && <p role="alert">{t("settings.about.openLinkFailed")}</p>}
    </div>
  );
}

function TauriUpdatesSettings({ runtimeInfo }: { runtimeInfo: TauriPreviewRuntimeInfo }) {
  const t = useT();
  const [linkError, setLinkError] = useState(false);
  const open = (url: string) => {
    setLinkError(false);
    void openTauriExternalURL(url).catch(() => setLinkError(true));
  };
  return (
    <div className="tauri-settings-section tauri-updates-settings">
      <h3>{t("settings.tab.updates")}</h3>
      <p>{t("settings.tabSub.updates")}</p>
      <div className="tauri-settings-about">
        <div className="tauri-settings-about-row"><span>{t("settings.about.previewVersion")}</span><span>v{runtimeInfo.previewVersion}</span></div>
        <div className="tauri-settings-about-row"><span>{t("settings.about.stableBaseline")}</span><span>v{runtimeInfo.stableVersion}</span></div>
      </div>
      <div className="tauri-updates-settings-notice" role="note">{t("settings.updates.previewUpdaterUnavailable")}</div>
      <div className="tauri-settings-actions">
        <button type="button" className="tauri-settings-button" onClick={() => open("https://reasonix.io/?download=desktop#start")}><ExternalLink size={14} /> {t("updater.officialDownload")}</button>
        <button type="button" className="tauri-settings-button" onClick={() => open("https://reasonix.io/changelog/")}><ExternalLink size={14} /> {t("changelog.openWeb")}</button>
        <button type="button" className="tauri-settings-button" onClick={() => open("https://github.com/esengine/DeepSeek-Reasonix/issues/new/choose")}><ExternalLink size={14} /> {t("feedback.submitIssue")}</button>
      </div>
      {linkError && <p role="alert">{t("settings.about.openLinkFailed")}</p>}
    </div>
  );
}
