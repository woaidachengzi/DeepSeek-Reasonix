import { useState, useCallback, useEffect, useRef, lazy, Suspense, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { Check, ArrowLeft, Search, X, Keyboard, Globe, Languages, Palette, Info, RefreshCw, ExternalLink, Key, Eye, EyeOff, Server, Database, SlidersHorizontal, Activity, Cable, Monitor, PanelTop, ShieldCheck, Power, Bell, Volume2, Play, ChevronDown, ChartNoAxesColumn, Box, Sparkles, Users, Webhook, Package } from "lucide-react";
import { tauriPreviewRuntimeInfo, tauriProviderSummary, setTauriDefaultModel, setTauriModelRole, tauriDesktopPreferences, setTauriDesktopApproval, setTauriDesktopTerminalTheme, setTauriDesktopAppearance, setTauriDesktopLanguage, tauriPlatformInfo, getTauriCloseBehavior, setTauriCloseBehavior, tauriZoomFactor, setTauriZoomFactor, keychainSave, keychainDelete, openTauriExternalURL, tauriMessageFrom, tauriUsageStats, type TauriToolApprovalMode, type TauriBridgeStatus, type TauriCloseBehavior, type TauriPreviewProfileStatus, type TauriPreviewRuntimeInfo, type TauriProviderSummary, type TauriSessionShadowReport } from "../lib/tauriBridge";
import { applyTerminalThemePreference, normalizeTerminalThemePreference, type TerminalThemePreference } from "../lib/terminalTheme";
import { THEME_STYLES, type Theme, type ThemeStyle } from "../lib/theme";
import { useI18n, useT, type DictKey, type LangPref, type Translator } from "../lib/i18n";
import { applyConversationWidth, getCachedConversationWidth, type ConversationWidth } from "../lib/conversationWidth";
import { applyTextSize, getTextSize, TEXT_SIZES, type TextSize } from "../lib/textSize";
import { applyFontFamily, applyMonoFontFamily, FONT_FAMILIES, MONO_FONT_FAMILIES, getCustomFontName, getCustomMonoFontName, getFontFamily, getMonoFontFamily, setCustomFontName, setCustomMonoFontName, type FontFamily, type MonoFontFamily } from "../lib/fontFamily";
import { applyTauriAppearance, readTauriAppearance, type TauriAppearance } from "./tauriAppearance";
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
import { StatusBarItemsEditor } from "../components/StatusBarItemsEditor";
import { TypographySettings } from "../components/TypographySettings";
import { comboFromKeyboardEvent, detectShortcutPlatform, formatShortcutCombo } from "../lib/keyboardShortcuts";
import { TAURI_SHORTCUT_ACTIONS, getTauriShortcut, isValidTauriShortcut, resetTauriShortcuts, setTauriShortcut, tauriShortcutConflict, useTauriShortcuts, type TauriShortcutAction } from "./tauriKeyboardShortcuts";
import "../components/SettingsPanel.css";
import { getTauriNotificationsEnabled, setTauriNotificationsEnabled, getTauriProgressMode, setTauriProgressMode, type TauriProgressMode } from "./tauriPreferences";
import { TAURI_STATUS_BAR_ITEM_IDS, setTauriStatusBarPreferences, useTauriStatusBarPreferences, type TauriStatusBarItemId } from "./tauriStatusBarPreferences";
import { setTauriDesktopLayout, useTauriDesktopLayout } from "./tauriDesktopLayout";
import { getSuccessPreference, setSuccessPreference, getAttentionPreference, setAttentionPreference, getNotificationVolume, setNotificationVolume, playSuccessChime, playAttentionChime, type SoundWavPref } from "../lib/sound";
import { generativeMusic, getGenerativePreset, setGenerativePreset, type GenerativePreset } from "../lib/generative-music";

interface TauriSettingsProps {
  onClose: () => void;
  onProviderSummaryChange?: (summary: TauriProviderSummary) => void;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
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

export type TauriSettingsTab = "general" | "appearance" | "model" | "providers" | "stats" | "mcp" | "skills" | "subagents" | "plugins" | "hooks" | "memory" | "permissions" | "sandbox" | "network" | "diagnostics" | "data" | "shortcuts" | "about";

const TauriUsageStatsPanel = lazy(() => import("../components/UsageStatsPanel").then(module => ({ default: module.UsageStatsPanel })));

const SETTINGS_GROUPS = (t: Translator) => [
  { label: t("settings.navGroup.preferences"), items: [{ id: "general", label: t("settings.tab.general"), description: t("settings.tabSub.general"), icon: SlidersHorizontal }] },
  { label: t("settings.tab.models"), items: [
    { id: "model", label: t("settings.models.preferences"), description: t("settings.tabSub.models"), icon: Globe },
    { id: "providers", label: t("settings.models.services"), description: t("settings.tabSub.providers"), icon: Cable },
    { id: "stats", label: t("settings.modelTab.stats"), description: t("settings.tabSub.models"), icon: ChartNoAxesColumn },
  ] },
  { label: t("settings.navGroup.connections"), items: [{ id: "mcp", label: t("settings.tab.mcp"), description: t("settings.tabSub.mcp"), icon: Server }] },
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
    { id: "about", label: t("settings.about.navLabel"), description: t("settings.about.hint"), icon: Info },
  ] },
] as const;

const SETTINGS_TITLES = (t: Translator): Record<TauriSettingsTab, { title: string; description: string }> => ({
  general: { title: t("settings.tab.general"), description: t("settings.tabSub.general") },
  model: { title: t("settings.models.preferences"), description: t("settings.tabSub.models") },
  providers: { title: t("settings.models.services"), description: t("settings.tabSub.providers") },
  stats: { title: t("settings.modelTab.stats"), description: t("settings.tabSub.models") },
  mcp: { title: t("settings.tab.mcp"), description: t("settings.tabSub.mcp") },
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
  about: { title: t("settings.about.title"), description: t("settings.about.hint") },
});

const STYLE_LABELS: Record<ThemeStyle, string> = {
  graphite: "石墨",
  aurora: "极光",
  slate: "岩蓝",
  carbon: "碳黑",
  nocturne: "夜曲",
  amber: "琥珀",
};

const TEXT_SIZE_LABELS: Record<TextSize, string> = {
  small: "小",
  default: "默认",
  large: "大",
  xlarge: "更大",
  xxlarge: "最大",
};

const TAURI_STATUS_BAR_LABEL_KEYS: Record<TauriStatusBarItemId, DictKey> = {
  workspace: "settings.statusBarItem.workspace", model: "settings.statusBarItem.model", session: "settings.statusBarItem.session",
  observed_tokens: "settings.statusBarItem.observedTokens", turn_tokens: "settings.statusBarItem.turnTokens", context: "settings.statusBarItem.context",
  compact: "settings.statusBarItem.compact", cache_hit: "settings.statusBarItem.cacheHit", bridge: "settings.statusBarItem.bridge",
};

const TAURI_SHORTCUT_LABELS: Record<TauriShortcutAction, { label: string; description: string }> = {
  new_session: { label: "新建对话", description: "回到新对话并选择工作区。" },
  settings: { label: "打开设置", description: "打开设置页。" },
  diagnostics: { label: "运行状态", description: "查看本地服务与运行信息。" },
  workspace_files: { label: "工作区文件", description: "打开当前对话的文件面板。" },
  refresh_session: { label: "刷新当前对话", description: "重新读取当前对话的历史记录。" },
  send_message: { label: "发送消息", description: "在输入框中发送消息；只能使用带主修饰键的 Enter。" },
};

const FONT_LABELS: Record<FontFamily, string> = {
  system: "系统默认",
  yahei: "微软雅黑",
  pingfang: "苹方",
  noto: "思源黑体",
  custom: "自定义字体",
};

const MONO_FONT_LABELS: Record<MonoFontFamily, string> = {
  system: "系统默认",
  cascadia: "Cascadia Code",
  jetbrains: "JetBrains Mono",
  sfmono: "SF Mono",
  custom: "自定义等宽字体",
};

export function TauriSettings({ onClose, onProviderSummaryChange, currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession, workspaceRoot, defaultWorkspace, onChooseDefaultWorkspace, onClearDefaultWorkspace, initialTab = "general", profile, onRefreshProfile, onImportStableProfile, onImportStableProjectFolders, onScanUnclaimedSessions, importBusy, bridgeStatus, catalogAudit, catalogAuditError, sessionPageSource, hostError, onRestartBridge, onRefreshCatalogAudit }: TauriSettingsProps) {
  const desktopLayout = useTauriDesktopLayout();
  const t = useT();
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
  const languageDirty = useRef(false);
  const loadRequest = useRef(0);
  const [appearance, setAppearance] = useState<TauriAppearance>(readTauriAppearance);
  const [conversationWidth, setConversationWidth] = useState<ConversationWidth>(getCachedConversationWidth);
  const [textSize, setTextSize] = useState<TextSize>(getTextSize);
  const [fontFamily, setFontFamily] = useState<FontFamily>(getFontFamily);
  const [monoFontFamily, setMonoFontFamily] = useState<MonoFontFamily>(getMonoFontFamily);
  const [customFontName, setCustomFontNameState] = useState(getCustomFontName);
  const [customMonoFontName, setCustomMonoFontNameState] = useState(getCustomMonoFontName);
  const [notificationsEnabled, setNotificationsEnabled] = useState(getTauriNotificationsEnabled);
  const [progressMode, setProgressMode] = useState(getTauriProgressMode);

  const updateProviderSummary = useCallback((summary: TauriProviderSummary) => {
    setProviderSummaryState(summary);
    onProviderSummaryChange?.(summary);
  }, [onProviderSummaryChange]);

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
      () => { if (request === loadRequest.current) { setCloseError("无法读取关闭窗口设置"); setCloseLoading(false); } },
    );
    void tauriDesktopPreferences().then(
      value => { if (request === loadRequest.current) { setApprovalMode(value.defaultToolApprovalMode); setApprovalError(""); setTerminalTheme(normalizeTerminalThemePreference(value.terminalTheme)); applyTerminalThemePreference(value.terminalTheme); setTerminalThemeError(""); if (value.appearanceConfigured && !appearanceDirty.current) { const next = { mode: value.theme, style: THEME_STYLES.includes(value.themeStyle as ThemeStyle) ? value.themeStyle as ThemeStyle : "graphite" }; setAppearance(next); applyTauriAppearance(next); } if (!languageDirty.current) setLanguagePref(value.language); setApprovalLoading(false); } },
      () => { if (request === loadRequest.current) { setApprovalError("无法读取默认审批设置"); setApprovalLoading(false); } },
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
    } catch {
      setAppearance(previous);
      applyTauriAppearance(previous);
      setAppearanceError("无法保存外观设置，请重试");
    } finally {
      setAppearanceSaving(false);
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
      setLanguageError("无法保存语言设置，请重试");
    } finally {
      setLanguageSaving(false);
    }
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

  const handleModelChange = async (model: string) => {
    if (modelSaving) return;
    setModelSaving(true);
    setModelSaveError("");
    try {
      const updated = await setTauriDefaultModel(model);
      updateProviderSummary(updated);
    } catch {
      setModelSaveError("默认模型保存失败，请重试");
    } finally {
      setModelSaving(false);
    }
  };

  const handleModelRoleChange = async (role: "planner" | "vision" | "search", model: string) => {
    if (modelSaving) return;
    setModelSaving(true);
    setModelSaveError("");
    try {
      updateProviderSummary(await setTauriModelRole(role, model));
    } catch {
      setModelSaveError("模型分工保存失败，请检查服务与模型能力后重试");
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
      setCloseError("保存失败，请重试");
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
      setApprovalError("默认审批设置保存失败，请重试");
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
      setTerminalThemeError("终端主题保存失败");
    } finally {
      setTerminalThemeSaving(false);
    }
  };

  const query = navQuery.trim().toLocaleLowerCase();
  const settingGroups = SETTINGS_GROUPS(t);
  const settingTitles = SETTINGS_TITLES(t);
  const visibleGroups = settingGroups.map(group => ({
    ...group,
    items: group.items.filter(item => !query || `${group.label} ${item.label} ${item.description}`.toLocaleLowerCase().includes(query)),
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
          {tab === "general" ? <GeneralSettings languagePref={languagePref} onLanguageChange={handleLanguageChange} languageSaving={languageSaving} languageError={languageError} notificationsEnabled={notificationsEnabled} onNotificationsChange={enabled => { setNotificationsEnabled(enabled); setTauriNotificationsEnabled(enabled); }} progressMode={progressMode} onProgressModeChange={next => { setProgressMode(next); setTauriProgressMode(next); }} closeBehavior={closeBehavior} onCloseBehaviorChange={handleCloseBehaviorChange} closeLoading={closeLoading} closeSaving={closeSaving} closeError={closeError} approvalMode={approvalMode} onApprovalChange={handleApprovalChange} approvalLoading={approvalLoading} approvalSaving={approvalSaving} approvalError={approvalError} platform={platform} currentSessionState={currentSessionState} /> : tab === "shortcuts" ? <ShortcutSettings /> : tab === "appearance" ? <AppearanceSettings appearance={appearance} onChange={handleAppearanceChange} appearanceSaving={appearanceSaving} appearanceError={appearanceError} conversationWidth={conversationWidth} onConversationWidthChange={handleConversationWidthChange} textSize={textSize} onTextSizeChange={handleTextSizeChange} fontFamily={fontFamily} onFontFamilyChange={handleFontFamilyChange} monoFontFamily={monoFontFamily} onMonoFontFamilyChange={handleMonoFontFamilyChange} customFontName={customFontName} onCustomFontChange={handleCustomFontChange} customMonoFontName={customMonoFontName} onCustomMonoFontChange={handleCustomMonoFontChange} terminalTheme={terminalTheme} onTerminalThemeChange={handleTerminalThemeChange} terminalThemeSaving={terminalThemeSaving} terminalThemeError={terminalThemeError} /> : <>
            {(tab === "model" || tab === "providers") && (modelLoading ? <div className="tauri-settings-loading">加载中…</div> : <>{modelLoadError && <SettingsLoadError onRetry={loadSettings} />}{providerSummary && (tab === "model" ? <ModelPreferenceSettings providerSummary={providerSummary} onModelChange={handleModelChange} onRoleChange={handleModelRoleChange} saving={modelSaving} error={modelSaveError} onOpenProviders={() => setTab("providers")} /> : <ProviderSettings providerSummary={providerSummary} onProviderSummaryChange={updateProviderSummary} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />)}</>)}
            {tab === "stats" && <Suspense fallback={<div className="tauri-settings-loading">加载中…</div>}><TauriUsageStatsPanel loadStats={tauriUsageStats} sources={["all", "desktop-tauri"]} /></Suspense>}
            {tab === "mcp" && <TauriMCPSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "skills" && <TauriSkillsSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "plugins" && <TauriPluginSettings currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "subagents" && <TauriSubagentSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "hooks" && <TauriHooksSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "memory" && <TauriMemorySettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "permissions" && <TauriPermissionsSettings currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "sandbox" && <TauriSandboxSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "network" && <TauriNetworkSettings currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "diagnostics" && <DiagnosticsSettings bridgeStatus={bridgeStatus} catalogAudit={catalogAudit} catalogAuditError={catalogAuditError} sessionPageSource={sessionPageSource} hostError={hostError} busy={Boolean(importBusy)} onRestartBridge={onRestartBridge} onRefreshCatalogAudit={onRefreshCatalogAudit} onOpenData={() => setTab("data")} onOpenProviders={() => setTab("providers")} />}
            {tab === "data" && <><TauriStorageSettings workspaceRoot={workspaceRoot} defaultWorkspace={defaultWorkspace} onChooseDefaultWorkspace={onChooseDefaultWorkspace} onClearDefaultWorkspace={onClearDefaultWorkspace} /><DataSettings profile={profile} busy={Boolean(importBusy)} onRefreshProfile={onRefreshProfile} onImportStableProfile={onImportStableProfile} onImportStableProjectFolders={onImportStableProjectFolders} onScanUnclaimedSessions={onScanUnclaimedSessions} onClose={onClose} /></>}
            {tab === "about" && (aboutLoading ? <div className="tauri-settings-loading">{t("common.loading")}</div> : <>{aboutLoadError && <SettingsLoadError onRetry={loadSettings} />}{runtimeInfo && <AboutSettings runtimeInfo={runtimeInfo} platform={platform} onRefresh={loadSettings} />}</>)}
          </>}
        </div>
      </div>
    </section>
  );
}

function GeneralSettings({ languagePref, onLanguageChange, languageSaving, languageError, notificationsEnabled, onNotificationsChange, progressMode, onProgressModeChange, closeBehavior, onCloseBehaviorChange, closeLoading, closeSaving, closeError, approvalMode, onApprovalChange, approvalLoading, approvalSaving, approvalError, platform, currentSessionState }: { languagePref: LangPref; onLanguageChange: (language: LangPref) => void; languageSaving: boolean; languageError: string; notificationsEnabled: boolean; onNotificationsChange: (enabled: boolean) => void; progressMode: TauriProgressMode; onProgressModeChange: (next: TauriProgressMode) => void; closeBehavior: TauriCloseBehavior; onCloseBehaviorChange: (behavior: TauriCloseBehavior) => void; closeLoading: boolean; closeSaving: boolean; closeError: string; approvalMode: TauriToolApprovalMode; onApprovalChange: (mode: TauriToolApprovalMode) => void; approvalLoading: boolean; approvalSaving: boolean; approvalError: string; platform: string; currentSessionState?: "idle" | "running" | "paused" }) {
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
  return <div className="tauri-settings-section tauri-settings-general">
    <h3>{t("settings.general.sectionAppearance")}</h3><p className="tauri-settings-section-description">{t("settings.general.desktopHint")}</p>
    <div className="tauri-settings-field"><Monitor className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.desktopLayoutStyle")}<small>{t("settings.desktopLayoutStyleHint")}</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.desktopLayoutStyle")}>{(["workbench", "creation"] as const).map(option => <button key={option} type="button" role="radio" aria-checked={desktopLayout === option} className={`tauri-settings-radio${desktopLayout === option ? " is-active" : ""}`} onClick={() => setTauriDesktopLayout(option)}>{t(option === "workbench" ? "settings.desktopLayoutStyle.workbench" : "settings.desktopLayoutStyle.creation")}</button>)}</div></div>
    <div className="tauri-settings-field"><Languages className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.language")}<small>{t("settings.languageHint")}</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.language")}>{(["", "zh", "en"] as const satisfies readonly LangPref[]).map(option => <button key={option || "auto"} type="button" role="radio" aria-checked={languagePref === option} disabled={languageSaving} className={`tauri-settings-radio${languagePref === option ? " is-active" : ""}`} onClick={() => onLanguageChange(option)}>{languagePref === option && <Check size={13} />}<span>{option === "" ? t("settings.langAuto") : option === "zh" ? "中文" : "English"}</span></button>)}</div></div>
    {languageError && <p className="tauri-diagnostic-error" role="alert">{languageError}</p>}
    <h3>{t("settings.general.sectionConversation")}</h3><p className="tauri-settings-section-description">{t("settings.sessionExperienceHint")}</p>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.sessionExperience")}<small>{progressMode === "standard" ? t("settings.sessionExperience.standardHint") : t("settings.sessionExperience.deepHint")}</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.sessionExperience")}>{(["standard", "deep"] as const).map(option => <button key={option} type="button" role="radio" aria-checked={progressMode === option} className={`tauri-settings-radio${progressMode === option ? " is-active" : ""}`} onClick={() => onProgressModeChange(option)}>{t(option === "standard" ? "settings.sessionExperience.standard" : "settings.sessionExperience.deep")}</button>)}</div></div>
    <h3>{t("settings.general.sectionSystem")}</h3><p className="tauri-settings-section-description">{t("settings.general.sectionSystemHint")}</p>
    {platform === "darwin" && <div className="tauri-settings-field"><Power className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.closeBehavior")}<small>{t("settings.closeBehaviorHint")}</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.closeBehavior")}>{(["keep_running", "quit"] as const).map(option => <button key={option} type="button" role="radio" aria-checked={closeBehavior === option} className={`tauri-settings-radio${closeBehavior === option ? " is-active" : ""}`} disabled={closeLoading || closeSaving} onClick={() => onCloseBehaviorChange(option)}>{t(option === "keep_running" ? "settings.closeBehavior.background" : "settings.closeBehavior.quit")}</button>)}</div></div>}
    {platform === "darwin" && closeError && <p className="tauri-diagnostic-error" role="alert">{closeError}</p>}
    <div className="tauri-settings-field"><ShieldCheck className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.defaultToolApprovalMode")}<small>{t("settings.defaultToolApprovalModeHint")}</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.defaultToolApprovalMode")}>{(["ask", "auto", "yolo"] as const).map(mode => <button key={mode} type="button" role="radio" aria-checked={approvalMode === mode} className={`tauri-settings-radio${approvalMode === mode ? " is-active" : ""}`} disabled={approvalLoading || approvalSaving} onClick={() => onApprovalChange(mode)}>{t(mode === "ask" ? "settings.defaultToolApprovalMode.ask" : mode === "auto" ? "settings.defaultToolApprovalMode.auto" : "settings.defaultToolApprovalMode.yolo")}</button>)}</div></div>
    {approvalError && <p className="tauri-diagnostic-error" role="alert">{approvalError}</p>}
    <label className="tauri-settings-toggle"><Bell className="tauri-settings-field-icon" size={18} /><span><strong>{t("settings.general.notifications")}</strong><small>{t("settings.general.notificationsHint")}</small></span><input type="checkbox" checked={notificationsEnabled} onChange={event => onNotificationsChange(event.target.checked)} /></label>
    <div className="tauri-settings-sound">
      <button type="button" className="tauri-settings-sound-toggle" aria-expanded={soundExpanded} aria-label={soundExpanded ? t("settings.soundCollapse") : t("settings.soundExpand")} onClick={() => setSoundExpanded(open => !open)}><Volume2 className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.sound")}<small>{t("settings.soundHint")}</small></span><span>{musicPreset === "off" && successSound === "off" && attentionSound === "off" ? t("settings.soundStatus.allOff") : t("settings.soundStatus.custom")}</span><ChevronDown size={15} aria-hidden="true" /></button>
      {soundExpanded && <div className="tauri-settings-sound-body">
        <div className="tauri-settings-sound-row"><label>{t("settings.generativeMusicPreset")}<select aria-label={t("settings.generativeMusicPreset")} value={musicPreset} onChange={event => changeMusicPreset(event.target.value as GenerativePreset)}>{musicPresets(t).map(option => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label><button type="button" aria-label={t("settings.generativeMusicPreview")} disabled={musicPreset === "off" || typeof AudioContext === "undefined"} onClick={() => { if (musicPreset !== "off") generativeMusic.playPreview(musicPreset); }}><Play size={14} /></button></div>
        <label className="tauri-settings-sound-row">{t("settings.notificationVolume")} <input type="range" min={0} max={100} value={soundVolume} aria-label={t("settings.notificationVolume")} onChange={event => setSoundVolume(setNotificationVolume(Number(event.target.value)))} /><output>{soundVolume}%</output></label>
        <TauriSoundOption t={t} label={t("settings.notificationSoundSuccess")} value={successSound} onChange={next => { setSuccessSound(next); setSuccessPreference(next); playSuccessChime(); }} onPreview={playSuccessChime} />
        <TauriSoundOption t={t} label={t("settings.notificationSoundAttention")} value={attentionSound} onChange={next => { setAttentionSound(next); setAttentionPreference(next); playAttentionChime(); }} onPreview={playAttentionChime} />
      </div>}
    </div>
    <div className="tauri-settings-field"><PanelTop className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.statusBarStyle")}<small>{t("settings.statusBarStyleHint")}</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.statusBarStyle")}>{(["icon", "text"] as const).map(option => <button key={option} type="button" role="radio" aria-checked={statusBar.style === option} className={`tauri-settings-radio${statusBar.style === option ? " is-active" : ""}`} onClick={() => setTauriStatusBarPreferences({ ...statusBar, style: option })}>{t(option === "icon" ? "settings.statusBarStyle.icon" : "settings.statusBarStyle.text")}</button>)}</div></div>
    <div className="tauri-settings-field tauri-settings-status-items"><Activity className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">{t("settings.statusBarItems")}<small>{t("settings.statusBarItemsHint")}</small></span><StatusBarItemsEditor items={statusBar.items} availableItems={TAURI_STATUS_BAR_ITEM_IDS} busy={false} onChange={items => setTauriStatusBarPreferences({ ...statusBar, items })} itemLabel={id => t(TAURI_STATUS_BAR_LABEL_KEYS[id])} /></div>
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
      setFeedback(action === "send_message" ? "发送消息需要主修饰键 + Enter。" : "请使用包含 ⌘ 或 Ctrl 的组合键。");
      return;
    }
    const conflict = tauriShortcutConflict(action, combo, platform);
    if (conflict) {
      setFeedback(`与“${TAURI_SHORTCUT_LABELS[conflict].label}”的快捷键冲突。`);
      return;
    }
    setTauriShortcut(action, combo, platform);
    setRecording(null); setFeedback("");
  };
  return <div className="tauri-settings-section">
    <div className="tauri-shortcuts-heading"><div><h3>工作区快捷键</h3><p>点击按键录入新组合；设置只保存在 Preview，并立即生效。</p></div><button type="button" className="tauri-settings-button" onClick={() => { resetTauriShortcuts(); setRecording(null); setFeedback(""); }} disabled={Object.keys(overrides).length === 0}>全部恢复默认</button></div>
    {feedback && <p className="tauri-diagnostic-error" role="alert">{feedback}</p>}
    <div className="tauri-settings-shortcuts">
      {TAURI_SHORTCUT_ACTIONS.map(action => {
        const info = TAURI_SHORTCUT_LABELS[action];
        const keys = formatShortcutCombo(getTauriShortcut(action, platform), platform);
        const active = recording === action;
        return <div key={action} className="tauri-shortcut-row"><span><strong>{info.label}</strong><small>{info.description}</small></span><div className="tauri-shortcut-row__actions"><button type="button" className={`tauri-shortcut-key${active ? " is-recording" : ""}`} data-tauri-shortcut-action={action} aria-label={active ? `正在录入${info.label}` : `${info.label}：${keys}`} aria-pressed={active} onClick={event => { setRecording(action); setFeedback(""); event.currentTarget.focus(); }} onBlur={() => { if (active) setRecording(null); }} onKeyDown={event => { if (active) record(action, event); }}>{active ? "请按组合键" : <kbd>{keys}</kbd>}</button><button type="button" className="tauri-shortcut-reset" disabled={!overrides[action]} onClick={() => { setTauriShortcut(action, null, platform); setFeedback(""); }}>重置</button></div></div>;
      })}
      <div className="tauri-shortcut-row"><span><strong>关闭面板</strong><small>保留系统常用的 Esc 操作，不能改键。</small></span><kbd>Esc</kbd></div>
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
    <div className="tauri-settings-data__heading"><div><h3>预览数据</h3><p>Preview 使用独立的配置和会话目录。</p></div>{onRefreshProfile && <button type="button" className="tauri-settings-button" onClick={() => void run(onRefreshProfile)} disabled={disabled}><RefreshCw size={13} />刷新</button>}</div>
    {profile ? <>
      <div className="tauri-settings-data__path"><span>配置目录</span><code>{profile.previewHome}</code></div>
      <div className="tauri-settings-data__actions">
        <h4>从正式版导入</h4>
        {profile.importAvailable ? <><p>复制稳定版配置前会创建带时间戳的备份，不会修改稳定版。</p><button type="button" className="tauri-settings-button" onClick={() => onImportStableProfile && void run(onImportStableProfile)} disabled={disabled || !onImportStableProfile}>复制稳定版配置（先备份）</button></> : <p>{profile.previewConfigExists ? "预览配置已存在，不会覆盖。" : profile.managedProfile ? "未发现可复制的稳定版配置。" : "检测到自定义 REASONIX_HOME，已停用自动导入。"}</p>}
        {profile.projectFoldersImportAvailable ? <><p>可单独导入正式版保存的项目文件夹名称和路径。</p><button type="button" className="tauri-settings-button" onClick={() => onImportStableProjectFolders && void run(onImportStableProjectFolders)} disabled={disabled || !onImportStableProjectFolders}>导入旧版项目文件夹</button></> : <p>{profile.projectFoldersFileExists ? "当前配置目录中已有项目文件夹清单。" : profile.managedProfile ? "没有可导入的项目文件夹清单。" : "自定义 REASONIX_HOME 下停用项目文件夹导入。"}</p>}
      </div>
      {profile.managedProfile && onScanUnclaimedSessions && <div className="tauri-settings-data__actions"><h4>会话目录</h4><p>审核尚未登记的 transcript，确认标题和项目后导入。</p><button type="button" className="tauri-settings-button" onClick={() => { onClose(); onScanUnclaimedSessions(); }} disabled={disabled}>扫描并审核未认领会话</button></div>}
    </> : <p className="tauri-settings-loading">尚未读取到预览配置。可点击刷新重试。</p>}
    {notice && <p className="tauri-settings-data__notice" role="status">{notice}</p>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
  </div>;
}

function SettingsLoadError({ onRetry }: { onRetry: () => void }) {
  const t = useT();
  return <div className="tauri-settings-load-error" role="alert">{t("settings.about.readFailed")}<button type="button" className="tauri-settings-button" onClick={onRetry}><RefreshCw size={13} />{t("settings.about.retry")}</button></div>;
}

function AppearanceSettings({ appearance, onChange, appearanceSaving, appearanceError, conversationWidth, onConversationWidthChange, textSize, onTextSizeChange, fontFamily, onFontFamilyChange, monoFontFamily, onMonoFontFamilyChange, customFontName, onCustomFontChange, customMonoFontName, onCustomMonoFontChange, terminalTheme, onTerminalThemeChange, terminalThemeSaving, terminalThemeError }: {
  appearance: TauriAppearance;
  onChange: (next: TauriAppearance) => void;
  appearanceSaving: boolean;
  appearanceError: string;
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
  const [typographyOpen, setTypographyOpen] = useState(false);

  if (typographyOpen) {
    return <TypographySettings onBack={() => setTypographyOpen(false)} scrollContainerSelector=".tauri-settings-content" fixedChinese />;
  }

  return (
    <div className="tauri-settings-section">
      <h3>外观</h3>
      <p>更改会立即应用，并在下次启动时保留。</p>
      <div className="tauri-settings-field">
        <span className="tauri-settings-field-label">模式</span>
        <div className="tauri-settings-radio-group" role="radiogroup" aria-label="外观模式">
          {(["auto", "light", "dark"] as const satisfies readonly Theme[]).map(option => (
            <button key={option} type="button" role="radio" aria-checked={appearance.mode === option} disabled={appearanceSaving} className={`tauri-settings-radio${appearance.mode === option ? " is-active" : ""}`} onClick={() => onChange({ ...appearance, mode: option })}>
              {appearance.mode === option && <Check size={13} />}
              <span>{option === "light" ? "浅色" : option === "dark" ? "深色" : "跟随系统"}</span>
            </button>
          ))}
        </div>
      </div>
      <div className="tauri-settings-style-heading">配色风格</div>
      <div className="tauri-settings-style-grid" role="radiogroup" aria-label="配色风格">
        {THEME_STYLES.map(style => (
          <button key={style} type="button" role="radio" aria-checked={appearance.style === style} disabled={appearanceSaving} className={`tauri-settings-style${appearance.style === style ? " is-active" : ""}`} onClick={() => onChange({ ...appearance, style })}>
            <span className="tauri-settings-style-swatches" data-style={style} aria-hidden="true"><i /><i /><i /></span>
            <span>{STYLE_LABELS[style]}</span>
            {appearance.style === style && <Check size={13} />}
          </button>
        ))}
      </div>
      {appearanceError && <p className="tauri-settings-zoom__error" role="alert">{appearanceError}</p>}
      <div className="tauri-settings-field tauri-settings-terminal-theme">
        <span className="tauri-settings-field-label">终端主题</span>
        <div className="tauri-settings-terminal-theme__controls">
          <div className="tauri-settings-radio-group" role="radiogroup" aria-label="终端主题">
            {(["auto", "light", "dark"] as const).map(option => (
              <button key={option} type="button" role="radio" aria-checked={terminalTheme === option} disabled={terminalThemeSaving} className={`tauri-settings-radio${terminalTheme === option ? " is-active" : ""}`} onClick={() => onTerminalThemeChange(option)}>
                {terminalTheme === option && <Check size={13} />}
                <span>{option === "auto" ? "跟随应用" : option === "light" ? "浅色" : "深色"}</span>
              </button>
            ))}
          </div>
          {terminalThemeError && <small className="tauri-settings-zoom__error" role="alert">{terminalThemeError}</small>}
        </div>
      </div>
      <div className="tauri-settings-style-heading">阅读布局</div>
      <div className="tauri-settings-field">
        <span className="tauri-settings-field-label">对话宽度</span>
        <div className="tauri-settings-radio-group" role="radiogroup" aria-label="对话宽度">
          {(["standard", "full"] as const).map(option => (
            <button key={option} type="button" role="radio" aria-checked={conversationWidth === option} className={`tauri-settings-radio${conversationWidth === option ? " is-active" : ""}`} onClick={() => onConversationWidthChange(option)}>
              {conversationWidth === option && <Check size={13} />}
              <span>{option === "standard" ? "标准" : "宽屏"}</span>
            </button>
          ))}
        </div>
      </div>
      <div className="tauri-settings-field">
        <span className="tauri-settings-field-label">对话字号</span>
        <div className="tauri-settings-radio-group" role="radiogroup" aria-label="对话字号">
          {TEXT_SIZES.map(size => (
            <button key={size} type="button" role="radio" aria-checked={textSize === size} className={`tauri-settings-radio${textSize === size ? " is-active" : ""}`} onClick={() => onTextSizeChange(size)}>
              {textSize === size && <Check size={13} />}
              <span>{TEXT_SIZE_LABELS[size]}</span>
            </button>
          ))}
        </div>
      </div>
      <DisplayZoomSetting />
      <div className="tauri-settings-field">
        <label htmlFor="tauri-settings-font">界面字体</label>
        <select id="tauri-settings-font" className="tauri-settings-select" value={fontFamily} onChange={event => onFontFamilyChange(event.target.value as FontFamily)}>
          {FONT_FAMILIES.map(font => <option key={font} value={font}>{FONT_LABELS[font]}</option>)}
        </select>
      </div>
      {fontFamily === "custom" && <div className="tauri-settings-field">
        <label htmlFor="tauri-settings-custom-font">字体名称</label>
        <input id="tauri-settings-custom-font" className="tauri-settings-input" value={customFontName} onChange={event => onCustomFontChange(event.target.value)} placeholder="输入已安装字体的名称" />
      </div>}
      <div className="tauri-settings-field">
        <label htmlFor="tauri-settings-mono-font">代码字体</label>
        <select id="tauri-settings-mono-font" className="tauri-settings-select" value={monoFontFamily} onChange={event => onMonoFontFamilyChange(event.target.value as MonoFontFamily)}>
          {MONO_FONT_FAMILIES.map(font => <option key={font} value={font}>{MONO_FONT_LABELS[font]}</option>)}
        </select>
      </div>
      {monoFontFamily === "custom" && <div className="tauri-settings-field">
        <label htmlFor="tauri-settings-custom-mono-font">等宽字体名称</label>
        <input id="tauri-settings-custom-mono-font" className="tauri-settings-input" value={customMonoFontName} onChange={event => onCustomMonoFontChange(event.target.value)} placeholder="输入已安装等宽字体的名称" />
      </div>}
      <div className="tauri-settings-field tauri-settings-typography-entry">
        <div><span className="tauri-settings-field-label">分区排版</span><p>分别设置界面、对话、输入框、代码和辅助文字的字体与字号。</p></div>
        <button type="button" className="tauri-settings-button" onClick={() => setTypographyOpen(true)}>自定义排版</button>
      </div>
    </div>
  );
}

function DisplayZoomSetting() {
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
    }).catch(() => { if (active) setError("无法读取显示缩放设置"); }).finally(() => { if (active) setLoading(false); });
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
      if (sequence === request.current) { setPercent(savedPercent.current); setError("显示缩放保存失败"); }
    }).finally(() => { if (sequence === request.current) setSaving(false); });
  };

  return (
    <div className="tauri-settings-field tauri-settings-zoom">
      <div className="tauri-settings-zoom__copy"><span className="tauri-settings-field-label">显示缩放</span><small>调整整个窗口的显示比例，菜单缩放也会同步。</small>{error && <span className="tauri-settings-zoom__error" role="alert">{error}</span>}</div>
      <div className="tauri-settings-zoom__control">
        <button type="button" aria-label="缩小界面" disabled={loading || percent <= 50} onClick={() => update(percent - 5)}>−</button>
        <input aria-label="显示缩放" type="range" min="50" max="200" step="5" value={percent} disabled={loading} onChange={event => update(Number(event.target.value))} />
        <button type="button" aria-label="放大界面" disabled={loading || percent >= 200} onClick={() => update(percent + 5)}>+</button>
        <output>{percent}%</output>
        <button type="button" className="tauri-settings-zoom__reset" disabled={loading || percent === 100} onClick={() => update(100)}>100%</button>
        {saving && <small aria-live="polite">保存中</small>}
      </div>
    </div>
  );
}

function DiagnosticsSettings({ bridgeStatus, catalogAudit, catalogAuditError, sessionPageSource, hostError, busy, onRestartBridge, onRefreshCatalogAudit, onOpenData, onOpenProviders }: {
  bridgeStatus?: TauriBridgeStatus | null;
  catalogAudit?: TauriSessionShadowReport | null;
  catalogAuditError?: string;
  sessionPageSource?: string;
  hostError?: string;
  busy: boolean;
  onRestartBridge?: () => Promise<boolean>;
  onRefreshCatalogAudit?: () => Promise<void>;
  onOpenData: () => void;
  onOpenProviders: () => void;
}) {
  const t = useT();
  const [restarting, setRestarting] = useState(false);
  const [checking, setChecking] = useState(false);
  const [auditRefreshError, setAuditRefreshError] = useState(false);
  const [restartResult, setRestartResult] = useState<"ok" | "failed" | "">("");
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
  </div>;
}

function ModelPreferenceSettings({ providerSummary, onModelChange, onRoleChange, saving, error, onOpenProviders }: {
  providerSummary: TauriProviderSummary;
  onModelChange: (model: string) => void;
  onRoleChange: (role: "planner" | "vision" | "search", model: string) => void;
  saving: boolean;
  error: string;
  onOpenProviders: () => void;
}) {
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
    if (ref === "auto") return "自动选择";
    if (!ready) return "连接不可用";
    const provider = available.find(item => ref.startsWith(`${item.name}/`));
    return provider?.displayName || provider?.name || "连接不可用";
  };
  return <div className="tauri-settings-section tauri-model-settings">
    <h3>模型分工</h3>
    <p>新会话读取 Preview 资料中的模型偏好。工作区的 <code>reasonix.toml</code> 可以覆盖这里的设置。</p>
    {available.length === 0 ? <div className="tauri-settings-empty">没有已就绪且提供模型列表的服务。请先配置模型服务。</div> : <>
      <div className="tauri-model-settings__head" aria-hidden="true"><span>模型用途</span><span>使用模型</span><span>连接</span></div>
      <div className="tauri-settings-field tauri-model-settings__row">
        <label className="tauri-settings-field-label" htmlFor="tauri-settings-default-model">默认模型<small>主会话运行任务时使用。</small></label>
        <select id="tauri-settings-default-model" className="tauri-settings-select" value={currentAvailable ? providerSummary.defaultModel : ""} disabled={saving} onChange={event => { if (event.target.value) onModelChange(event.target.value); }}>
          <option value="">选择模型…</option>
          {available.map(provider => <optgroup key={provider.name} label={provider.displayName || provider.name}>{provider.models.map(model => <option key={model} value={`${provider.name}/${model}`}>{model}</option>)}</optgroup>)}
        </select>
        <span className="tauri-model-settings__connection">{connection(providerSummary.defaultModel, currentAvailable, "未指定")}</span>
      </div>
      {!currentAvailable && providerSummary.defaultModel && <p className="tauri-model-settings__stale">当前默认模型 {providerSummary.defaultModel} 暂不可用，请选择已就绪的模型。</p>}
      <div className="tauri-settings-field tauri-model-settings__row">
        <label className="tauri-settings-field-label" htmlFor="tauri-settings-planner-model">规划模型<small>用于 Plan、审批或目标启动时的规划步骤。</small></label>
        <select id="tauri-settings-planner-model" className="tauri-settings-select" value={providerSummary.plannerModel} disabled={saving} onChange={event => onRoleChange("planner", event.target.value)}>
          <option value="">不指定</option>
          {!plannerAvailable && <option value={providerSummary.plannerModel} disabled>{providerSummary.plannerModel}（不可用）</option>}
          {available.map(provider => <optgroup key={provider.name} label={provider.displayName || provider.name}>{provider.models.map(model => <option key={model} value={`${provider.name}/${model}`}>{model}</option>)}</optgroup>)}
        </select>
        <span className="tauri-model-settings__connection">{connection(providerSummary.plannerModel, plannerAvailable, "跟随主会话")}</span>
      </div>
      <div className="tauri-settings-field tauri-model-settings__row">
        <label className="tauri-settings-field-label" htmlFor="tauri-settings-vision-model">图片理解模型<small>当前模型无法处理图片时使用的备用模型。</small></label>
        <select id="tauri-settings-vision-model" className="tauri-settings-select" value={providerSummary.visionModel} disabled={saving} onChange={event => onRoleChange("vision", event.target.value)}>
          <option value="">不指定</option><option value="auto">自动选择</option>
          {!visionAvailable && <option value={providerSummary.visionModel} disabled>{providerSummary.visionModel}（不可用）</option>}
          {available.map(provider => provider.visionModels.length > 0 && <optgroup key={provider.name} label={provider.displayName || provider.name}>{provider.visionModels.map(model => <option key={model} value={`${provider.name}/${model}`}>{model}</option>)}</optgroup>)}
        </select>
        <span className="tauri-model-settings__connection">{connection(providerSummary.visionModel, visionAvailable, "无")}</span>
      </div>
      {visionRefs.length === 0 && <p className="tauri-settings-hint">当前没有确认支持图片输入的已就绪模型；可以使用自动选择。</p>}
      <div className="tauri-settings-field tauri-model-settings__row">
        <label className="tauri-settings-field-label" htmlFor="tauri-settings-search-model">网页搜索模型<small>负责使用原生搜索协议获取网页来源。</small></label>
        <select id="tauri-settings-search-model" className="tauri-settings-select" value={providerSummary.webSearchModel || "auto"} disabled={saving} onChange={event => onRoleChange("search", event.target.value)}>
          <option value="auto">自动选择</option>
          {!searchAvailable && <option value={providerSummary.webSearchModel} disabled>{providerSummary.webSearchModel}（不可用）</option>}
          {available.map(provider => provider.searchModels.length > 0 && <optgroup key={provider.name} label={provider.displayName || provider.name}>{provider.searchModels.map(model => <option key={model} value={`${provider.name}/${model}`}>{model}</option>)}</optgroup>)}
        </select>
        <span className="tauri-model-settings__connection">{connection(providerSummary.webSearchModel || "auto", searchAvailable, "自动选择")}</span>
      </div>
      {searchRefs.length === 0 && <p className="tauri-settings-hint">当前没有已就绪且支持原生网页搜索的模型；自动模式仍会按运行时规则查找。</p>}
    </>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" onClick={onOpenProviders}>管理模型服务</button></div>
  </div>;
}

function ProviderSettings({ providerSummary, onProviderSummaryChange, currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  providerSummary: TauriProviderSummary;
  onProviderSummaryChange: (summary: TauriProviderSummary) => void;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const [apiKeyInputs, setApiKeyInputs] = useState<Record<string, string>>({});
  const [showApiKey, setShowApiKey] = useState<Record<string, boolean>>({});
  const [keyStatus, setKeyStatus] = useState<Record<string, { message: string; error: boolean } | null>>({});
  const [keyBusy, setKeyBusy] = useState(false);
  const [pendingApply, setPendingApply] = useState<Record<string, boolean>>({});
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

  const handleApiKeyAction = async (providerName: string, action: "save" | "delete") => {
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
      else deleted = await keychainDelete(`api_key_${providerName}`);
      if (action === "save" && mounted.current) {
        setApiKeyInputs(prev => ({ ...prev, [providerName]: prev[providerName] === key ? "" : prev[providerName] }));
      }
      if (mounted.current && (action === "save" || deleted)) {
        setPendingApply(prev => ({ ...prev, [providerName]: true }));
      }
      try {
        const summary = await tauriProviderSummary();
        if (mounted.current) onProviderSummaryChange(summary);
        const configured = summary.providers.find(provider => provider.name === providerName)?.configured;
        showStatus(providerName, action === "save" ? "已保存到钥匙串" : !deleted ? "钥匙串中没有密钥" : configured ? "钥匙串密钥已删除；其他凭据仍可用" : "钥匙串密钥已删除");
      } catch {
        showStatus(providerName, action === "save" ? "已保存到钥匙串；配置状态刷新失败" : deleted ? "钥匙串密钥已删除；配置状态刷新失败" : "钥匙串中没有密钥；配置状态刷新失败", true);
      }
    } catch {
      showStatus(providerName, action === "save" ? "保存失败" : "删除失败", true);
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
      showStatus(providerName, applied ? "当前会话已更新" : "当前会话未能更新，请在诊断面板重试", !applied);
    } catch {
      showStatus(providerName, "当前会话未能更新，请在诊断面板重试", true);
    } finally {
      keyBusyRef.current = false;
      if (mounted.current) setKeyBusy(false);
    }
  };

  return (
    <div className="tauri-settings-section">
      <h3>模型提供方</h3>
      <p className="tauri-settings-hint">在这里配置服务并保存所需凭据。默认模型在“模型偏好”中选择。</p>
      <TauriProviderEditor onSummaryChange={onProviderSummaryChange} />
      {providerSummary.providers.length === 0 ? (
        <div className="tauri-settings-empty">当前没有模型服务。可通过上方的“添加服务”配置 Preview 独立资料。</div>
      ) : (
        <div className="tauri-settings-model-list">
          {providerSummary.providers.map(provider => (
            <div key={provider.name} className="tauri-settings-model-card">
              <div className="tauri-settings-model-header">
                <strong>{provider.displayName || provider.name}</strong>
                <span className={`tauri-settings-badge${provider.configured ? " is-ready" : ""}`}>{provider.configured ? "已就绪" : "未配置"}</span>
              </div>
              <p className="tauri-settings-model-meta">{provider.kind} · {provider.modelCount} 个模型</p>
              {provider.requiresKey && (
                <div className="tauri-settings-apikey">
                  <div className="tauri-settings-apikey-input">
                    <Key size={13} />
                    <input
                      type={showApiKey[provider.name] ? "text" : "password"}
                      placeholder={provider.configured ? "已配置（钥匙串或其他凭据来源）" : "输入 API Key…"}
                      value={apiKeyInputs[provider.name] ?? ""}
                      onChange={e => setApiKeyInputs(prev => ({ ...prev, [provider.name]: e.target.value }))}
                    />
                    <button type="button" onClick={() => setShowApiKey(prev => ({ ...prev, [provider.name]: !prev[provider.name] }))}>
                      {showApiKey[provider.name] ? <EyeOff size={13} /> : <Eye size={13} />}
                    </button>
                  </div>
                  <div className="tauri-settings-apikey-actions">
                    <button type="button" className="tauri-settings-button" onClick={() => void handleApiKeyAction(provider.name, "save")} disabled={keyBusy || !apiKeyInputs[provider.name]}>
                      保存到钥匙串
                    </button>
                    <button type="button" className="tauri-settings-button tauri-settings-button--danger" onClick={() => void handleApiKeyAction(provider.name, "delete")} disabled={keyBusy}>
                      删除
                    </button>
                    {pendingApply[provider.name] && currentSessionState && onApplyToCurrentSession && (
                      <button type="button" className="tauri-settings-button" onClick={() => void handleApplyToCurrentSession(provider.name)} disabled={keyBusy || currentSessionState !== "idle" || currentSessionHasAttachments} title={currentSessionState !== "idle" ? "请等待当前回合结束" : currentSessionHasAttachments ? "请先处理待发送的附件" : undefined}>
                        应用到当前会话
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
      <p className="tauri-settings-hint">在此保存的 API Key 存储在系统钥匙串中，对新会话生效；当前会话可在诊断面板重启桥接服务后继续使用。删除钥匙串密钥后，其他凭据来源仍可能使提供方保持就绪。</p>
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
