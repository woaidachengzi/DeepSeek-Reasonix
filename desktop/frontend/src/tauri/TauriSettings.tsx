import { useState, useCallback, useEffect, useRef, lazy, Suspense } from "react";
import { Check, ArrowLeft, ArrowUp, ArrowDown, Search, X, Keyboard, Globe, Palette, Info, RefreshCw, ExternalLink, Key, Eye, EyeOff, Server, Database, SlidersHorizontal, Activity, Cable, Monitor, PanelTop, Type, ShieldCheck, Power, Bell, Volume2, Play, ChevronDown, ChartNoAxesColumn, Box, Sparkles, Users, Webhook, Package } from "lucide-react";
import { tauriPreviewRuntimeInfo, tauriProviderSummary, setTauriDefaultModel, setTauriModelRole, tauriDesktopPreferences, setTauriDesktopApproval, tauriPlatformInfo, getTauriCloseBehavior, setTauriCloseBehavior, keychainSave, keychainDelete, openTauriExternalURL, tauriMessageFrom, tauriUsageStats, type TauriToolApprovalMode, type TauriBridgeStatus, type TauriCloseBehavior, type TauriPreviewProfileStatus, type TauriPreviewRuntimeInfo, type TauriProviderSummary, type TauriSessionShadowReport } from "../lib/tauriBridge";
import { THEME_STYLES, type Theme, type ThemeStyle } from "../lib/theme";
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
import { getTauriNotificationsEnabled, setTauriNotificationsEnabled, getTauriProgressMode, setTauriProgressMode, type TauriProgressMode } from "./tauriPreferences";
import { TAURI_STATUS_BAR_ITEM_IDS, setTauriStatusBarPreferences, useTauriStatusBarPreferences, type TauriStatusBarItemId } from "./tauriStatusBarPreferences";
import { getSuccessPreference, setSuccessPreference, getAttentionPreference, setAttentionPreference, getNotificationVolume, setNotificationVolume, playSuccessChime, playAttentionChime, type SoundWavPref } from "../lib/sound";

interface TauriSettingsProps {
  onClose: () => void;
  onProviderSummaryChange?: (summary: TauriProviderSummary) => void;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
  workspaceRoot?: string;
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

const SETTINGS_GROUPS = [
  { label: "偏好设置", items: [{ id: "general", label: "通用", description: "桌面与会话体验", icon: SlidersHorizontal }] },
  { label: "模型", items: [
    { id: "model", label: "模型偏好", description: "新对话的默认与分工模型", icon: Globe },
    { id: "providers", label: "模型服务", description: "配置提供方与钥匙串凭据", icon: Cable },
    { id: "stats", label: "用量统计", description: "查看 Preview 的模型用量", icon: ChartNoAxesColumn },
  ] },
  { label: "集成与连接", items: [{ id: "mcp", label: "MCP 与工具", description: "管理工具服务器", icon: Server }] },
  { label: "能力扩展", items: [{ id: "skills", label: "Agent Skills", description: "管理技能与来源", icon: Sparkles }, { id: "subagents", label: "子智能体", description: "设置模型、并行限制与覆盖", icon: Users }, { id: "plugins", label: "插件", description: "查看已安装插件及启用状态", icon: Package }] },
  { label: "记忆与上下文", items: [{ id: "memory", label: "记忆", description: "管理说明文档与已保存的事实", icon: Database }] },
  { label: "安全与执行", items: [
    { id: "permissions", label: "权限", description: "写入决策与工具规则", icon: ShieldCheck },
    { id: "sandbox", label: "沙盒", description: "命令隔离与文件写入范围", icon: Box },
    { id: "network", label: "网络", description: "代理与直连设置", icon: Globe },
  ] },
  { label: "自动化与开发者", items: [{ id: "hooks", label: "Hooks", description: "管理事件触发的本地命令", icon: Webhook }, { id: "diagnostics", label: "运行诊断", description: "桥接状态与会话目录检查", icon: Activity }] },
  { label: "应用", items: [
    { id: "appearance", label: "外观", description: "主题、阅读布局与字体", icon: Palette },
    { id: "shortcuts", label: "快捷键", description: "查看键盘操作", icon: Keyboard },
    { id: "data", label: "数据", description: "Preview 配置与导入", icon: Database },
    { id: "about", label: "关于", description: "版本与运行信息", icon: Info },
  ] },
] as const;

const SETTINGS_TITLES: Record<TauriSettingsTab, { title: string; description: string }> = {
  general: { title: "通用", description: "设置桌面体验和会话显示。" },
  model: { title: "模型偏好", description: "设置新对话使用的默认模型与模型分工。" },
  providers: { title: "模型服务", description: "添加和编辑模型服务，管理钥匙串凭据。" },
  stats: { title: "用量统计", description: "查看 Preview 资料中已记录的 token 用量。" },
  mcp: { title: "MCP 与工具", description: "连接并管理工作区可用的工具。" },
  skills: { title: "Agent Skills", description: "管理当前工作区可发现的技能与来源。" },
  plugins: { title: "插件", description: "查看 Preview 资料中的插件包与贡献。" },
  subagents: { title: "子智能体", description: "管理子智能体的运行默认值与按名称覆盖。" },
  hooks: { title: "Hooks", description: "配置会话与工具事件触发时运行的本地命令。" },
  memory: { title: "记忆", description: "查看与编辑工作区说明文档，管理已保存的事实。" },
  permissions: { title: "权限", description: "设置工具的默认写入决策和规则。" },
  sandbox: { title: "沙盒", description: "设置命令隔离、网络访问和文件写入范围。" },
  network: { title: "网络", description: "设置 Preview 普通 HTTP 请求的代理方式。" },
  diagnostics: { title: "运行诊断", description: "检查本地服务、会话目录和 Preview 的连接状态。" },
  appearance: { title: "外观", description: "调整主题、阅读布局和字体。" },
  shortcuts: { title: "快捷键", description: "查看 Preview 中可用的键盘操作。" },
  data: { title: "数据与迁移", description: "查看 Preview 配置与会话导入。" },
  about: { title: "关于", description: "版本、构建及运行环境。" },
};

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

export function TauriSettings({ onClose, onProviderSummaryChange, currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession, workspaceRoot, initialTab = "general", profile, onRefreshProfile, onImportStableProfile, onImportStableProjectFolders, onScanUnclaimedSessions, importBusy, bridgeStatus, catalogAudit, catalogAuditError, sessionPageSource, hostError, onRestartBridge, onRefreshCatalogAudit }: TauriSettingsProps) {
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
      value => { if (request === loadRequest.current) { setApprovalMode(value.defaultToolApprovalMode); setApprovalError(""); setApprovalLoading(false); } },
      () => { if (request === loadRequest.current) { setApprovalError("无法读取默认审批设置"); setApprovalLoading(false); } },
    );
  }, [updateProviderSummary]);

  useEffect(() => {
    void loadSettings();
    return () => { loadRequest.current += 1; };
  }, [loadSettings]);

  const handleAppearanceChange = (next: TauriAppearance) => {
    setAppearance(next);
    applyTauriAppearance(next);
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

  const query = navQuery.trim().toLocaleLowerCase();
  const visibleGroups = SETTINGS_GROUPS.map(group => ({
    ...group,
    items: group.items.filter(item => !query || `${group.label} ${item.label} ${item.description}`.toLocaleLowerCase().includes(query)),
  })).filter(group => group.items.length > 0);

  return (
    <section className="tauri-settings-overlay" aria-label="设置">
      <aside className="tauri-settings-sidebar">
        <div className="tauri-settings-titlebar" data-tauri-drag-region />
        <button type="button" className="tauri-settings-back" onClick={onClose}><ArrowLeft size={17} /><span>返回工作区</span></button>
        <label className="tauri-settings-search"><Search size={16} aria-hidden="true" /><input type="search" aria-label="搜索设置" placeholder="搜索设置" value={navQuery} onChange={event => setNavQuery(event.target.value)} />{navQuery && <button type="button" aria-label="清除设置搜索" onClick={() => setNavQuery("")}><X size={14} aria-hidden="true" /></button>}</label>
        <nav className="tauri-settings-nav" aria-label="设置分类">
          {visibleGroups.map(group => <div className="tauri-settings-nav-group" key={group.label}>
            <div className="tauri-settings-nav-label">{group.label}</div>
            {group.items.map(item => { const Icon = item.icon; return <button key={item.id} type="button" aria-current={tab === item.id ? "page" : undefined} className={`tauri-settings-nav-item${tab === item.id ? " is-active" : ""}`} onClick={() => setTab(item.id)} title={item.description}>
              <Icon size={17} /><span>{item.label}</span>
            </button>; })}
          </div>)}
          {visibleGroups.length === 0 && <div className="tauri-settings-nav-empty" role="status">没有匹配的设置</div>}
        </nav>
      </aside>
      <div className="tauri-settings-panel">
        <div className="tauri-settings-content" data-tab={tab} key={tab}>
          <div className="tauri-settings-page-heading"><h1>{SETTINGS_TITLES[tab].title}</h1><p>{SETTINGS_TITLES[tab].description}</p></div>
          {tab === "general" ? <GeneralSettings notificationsEnabled={notificationsEnabled} onNotificationsChange={enabled => { setNotificationsEnabled(enabled); setTauriNotificationsEnabled(enabled); }} appearance={appearance} onAppearanceChange={handleAppearanceChange} conversationWidth={conversationWidth} onConversationWidthChange={handleConversationWidthChange} textSize={textSize} onTextSizeChange={handleTextSizeChange} progressMode={progressMode} onProgressModeChange={next => { setProgressMode(next); setTauriProgressMode(next); }} closeBehavior={closeBehavior} onCloseBehaviorChange={handleCloseBehaviorChange} closeLoading={closeLoading} closeSaving={closeSaving} closeError={closeError} approvalMode={approvalMode} onApprovalChange={handleApprovalChange} approvalLoading={approvalLoading} approvalSaving={approvalSaving} approvalError={approvalError} platform={platform} /> : tab === "shortcuts" ? <ShortcutSettings /> : tab === "appearance" ? <AppearanceSettings appearance={appearance} onChange={handleAppearanceChange} conversationWidth={conversationWidth} onConversationWidthChange={handleConversationWidthChange} textSize={textSize} onTextSizeChange={handleTextSizeChange} fontFamily={fontFamily} onFontFamilyChange={handleFontFamilyChange} monoFontFamily={monoFontFamily} onMonoFontFamilyChange={handleMonoFontFamilyChange} customFontName={customFontName} onCustomFontChange={handleCustomFontChange} customMonoFontName={customMonoFontName} onCustomMonoFontChange={handleCustomMonoFontChange} /> : <>
            {(tab === "model" || tab === "providers") && (modelLoading ? <div className="tauri-settings-loading">加载中…</div> : <>{modelLoadError && <SettingsLoadError onRetry={loadSettings} />}{providerSummary && (tab === "model" ? <ModelPreferenceSettings providerSummary={providerSummary} onModelChange={handleModelChange} onRoleChange={handleModelRoleChange} saving={modelSaving} error={modelSaveError} onOpenProviders={() => setTab("providers")} /> : <ProviderSettings providerSummary={providerSummary} onProviderSummaryChange={updateProviderSummary} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />)}</>)}
            {tab === "stats" && <Suspense fallback={<div className="tauri-settings-loading">加载中…</div>}><TauriUsageStatsPanel loadStats={tauriUsageStats} sources={["all", "desktop-tauri"]} /></Suspense>}
            {tab === "mcp" && <TauriMCPSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "skills" && <TauriSkillsSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "plugins" && <TauriPluginSettings currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "subagents" && <TauriSubagentSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "hooks" && <TauriHooksSettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "memory" && <TauriMemorySettings workspaceRoot={workspaceRoot} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "permissions" && <TauriPermissionsSettings currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "sandbox" && <TauriSandboxSettings currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "network" && <TauriNetworkSettings currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "diagnostics" && <DiagnosticsSettings bridgeStatus={bridgeStatus} catalogAudit={catalogAudit} catalogAuditError={catalogAuditError} sessionPageSource={sessionPageSource} hostError={hostError} busy={Boolean(importBusy)} onRestartBridge={onRestartBridge} onRefreshCatalogAudit={onRefreshCatalogAudit} onOpenData={() => setTab("data")} onOpenProviders={() => setTab("providers")} />}
            {tab === "data" && <DataSettings profile={profile} busy={Boolean(importBusy)} onRefreshProfile={onRefreshProfile} onImportStableProfile={onImportStableProfile} onImportStableProjectFolders={onImportStableProjectFolders} onScanUnclaimedSessions={onScanUnclaimedSessions} onClose={onClose} />}
            {tab === "about" && (aboutLoading ? <div className="tauri-settings-loading">加载中…</div> : <>{aboutLoadError && <SettingsLoadError onRetry={loadSettings} />}{runtimeInfo && <AboutSettings runtimeInfo={runtimeInfo} platform={platform} onRefresh={loadSettings} />}</>)}
          </>}
        </div>
      </div>
    </section>
  );
}

function GeneralSettings({ notificationsEnabled, onNotificationsChange, appearance, onAppearanceChange, conversationWidth, onConversationWidthChange, textSize, onTextSizeChange, progressMode, onProgressModeChange, closeBehavior, onCloseBehaviorChange, closeLoading, closeSaving, closeError, approvalMode, onApprovalChange, approvalLoading, approvalSaving, approvalError, platform }: { notificationsEnabled: boolean; onNotificationsChange: (enabled: boolean) => void; appearance: TauriAppearance; onAppearanceChange: (next: TauriAppearance) => void; conversationWidth: ConversationWidth; onConversationWidthChange: (next: ConversationWidth) => void; textSize: TextSize; onTextSizeChange: (next: TextSize) => void; progressMode: TauriProgressMode; onProgressModeChange: (next: TauriProgressMode) => void; closeBehavior: TauriCloseBehavior; onCloseBehaviorChange: (behavior: TauriCloseBehavior) => void; closeLoading: boolean; closeSaving: boolean; closeError: string; approvalMode: TauriToolApprovalMode; onApprovalChange: (mode: TauriToolApprovalMode) => void; approvalLoading: boolean; approvalSaving: boolean; approvalError: string; platform: string }) {
  const [soundExpanded, setSoundExpanded] = useState(false);
  const [statusItemsExpanded, setStatusItemsExpanded] = useState(false);
  const [successSound, setSuccessSound] = useState<SoundWavPref>(getSuccessPreference);
  const [attentionSound, setAttentionSound] = useState<SoundWavPref>(getAttentionPreference);
  const [soundVolume, setSoundVolume] = useState(getNotificationVolume);
  const statusBar = useTauriStatusBarPreferences();
  const toggleStatusItem = (id: TauriStatusBarItemId) => setTauriStatusBarPreferences({ ...statusBar, items: statusBar.items.includes(id) ? statusBar.items.filter(item => item !== id) : [...statusBar.items, id] });
  const moveStatusItem = (id: TauriStatusBarItemId, offset: number) => {
    const items = [...statusBar.items];
    const index = items.indexOf(id);
    const target = index + offset;
    if (index < 0 || target < 0 || target >= items.length) return;
    [items[index], items[target]] = [items[target], items[index]];
    setTauriStatusBarPreferences({ ...statusBar, items });
  };
  return <div className="tauri-settings-section tauri-settings-general">
    <h3>桌面与显示</h3><p className="tauri-settings-section-description">调整界面的显示方式。</p>
    <div className="tauri-settings-field"><Monitor className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">外观模式<small>选择界面亮暗模式。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="外观模式">{(["auto", "light", "dark"] as const).map(option => <button key={option} type="button" role="radio" aria-checked={appearance.mode === option} className={`tauri-settings-radio${appearance.mode === option ? " is-active" : ""}`} onClick={() => onAppearanceChange({ ...appearance, mode: option })}>{option === "auto" ? "跟随系统" : option === "light" ? "浅色" : "深色"}</button>)}</div></div>
    <h3>会话体验</h3><p className="tauri-settings-section-description">选择任务运行时与完成后的阅读方式。</p>
    <div className="tauri-settings-field"><PanelTop className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">对话宽度<small>调整消息内容的最大宽度。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="通用对话宽度">{(["standard", "full"] as const).map(option => <button key={option} type="button" role="radio" aria-checked={conversationWidth === option} className={`tauri-settings-radio${conversationWidth === option ? " is-active" : ""}`} onClick={() => onConversationWidthChange(option)}>{option === "standard" ? "标准" : "宽屏"}</button>)}</div></div>
    <div className="tauri-settings-field"><Type className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">对话字号<small>调整正文的阅读大小。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="通用对话字号">{TEXT_SIZES.map(size => <button key={size} type="button" role="radio" aria-checked={textSize === size} className={`tauri-settings-radio${textSize === size ? " is-active" : ""}`} onClick={() => onTextSizeChange(size)}>{TEXT_SIZE_LABELS[size]}</button>)}</div></div>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">会话体验<small>选择过程更新的默认展开状态，仍可逐项手动折叠。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="会话体验">{(["standard", "deep"] as const).map(option => <button key={option} type="button" role="radio" aria-checked={progressMode === option} className={`tauri-settings-radio${progressMode === option ? " is-active" : ""}`} onClick={() => onProgressModeChange(option)}>{option === "standard" ? "标准" : "深入"}</button>)}</div></div>
    <div className="tauri-settings-field"><PanelTop className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">底部信息栏样式<small>切换工作区底部状态信息的显示方式。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="底部信息栏样式">{(["icon", "text"] as const).map(option => <button key={option} type="button" role="radio" aria-checked={statusBar.style === option} className={`tauri-settings-radio${statusBar.style === option ? " is-active" : ""}`} onClick={() => setTauriStatusBarPreferences({ ...statusBar, style: option })}>{option === "icon" ? "图标版" : "文字版"}</button>)}</div></div>
    <div className="tauri-settings-field tauri-settings-status-items"><Activity className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">信息栏显示项<small>上下文和缓存来自会话快照；token 用量仅统计当前同步阶段收到的事件。</small></span><div className="tauri-settings-status-items__control"><button type="button" className="tauri-settings-status-items__toggle" aria-expanded={statusItemsExpanded} aria-controls="tauri-status-items-list" onClick={() => setStatusItemsExpanded(expanded => !expanded)}>已显示 {statusBar.items.length}/{TAURI_STATUS_BAR_ITEM_IDS.length} 项<ChevronDown size={15} aria-hidden="true" /></button>{statusItemsExpanded && <div className="tauri-settings-status-items__list" id="tauri-status-items-list">{[...statusBar.items, ...TAURI_STATUS_BAR_ITEM_IDS.filter(id => !statusBar.items.includes(id))].map(id => {
      const index = statusBar.items.indexOf(id);
      const label = { workspace: "工作区", model: "默认模型", session: "当前会话", observed_tokens: "已观测 token", turn_tokens: "本轮 token", context: "上下文", compact: "压缩阈值", cache_hit: "会话缓存命中", bridge: "本地服务" }[id];
      return <div className="tauri-settings-status-items__row" key={id}><label><input type="checkbox" checked={index >= 0} onChange={() => toggleStatusItem(id)} />{label}</label><div><button type="button" aria-label={`上移${label}`} disabled={index <= 0} onClick={() => moveStatusItem(id, -1)}><ArrowUp size={14} /></button><button type="button" aria-label={`下移${label}`} disabled={index < 0 || index >= statusBar.items.length - 1} onClick={() => moveStatusItem(id, 1)}><ArrowDown size={14} /></button></div></div>;
    })}</div>}</div></div>
    <h3>系统行为</h3><p className="tauri-settings-section-description">控制窗口、工具审批与通知。</p>
    {platform === "darwin" && <div className="tauri-settings-field"><Power className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">关闭窗口时<small>选择关闭主窗口后的运行方式。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="关闭窗口时">{(["keep_running", "quit"] as const).map(option => <button key={option} type="button" role="radio" aria-checked={closeBehavior === option} className={`tauri-settings-radio${closeBehavior === option ? " is-active" : ""}`} disabled={closeLoading || closeSaving} onClick={() => onCloseBehaviorChange(option)}>{option === "keep_running" ? "保持后台运行" : "退出 Reasonix"}</button>)}</div></div>}
    {platform === "darwin" && closeError && <p className="tauri-diagnostic-error" role="alert">{closeError}</p>}
    <div className="tauri-settings-field"><ShieldCheck className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">新会话默认审批<small>只影响之后创建的会话；Yolo 会跳过工具审批，计划与沙盒限制仍生效。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="新会话默认审批">{(["ask", "auto", "yolo"] as const).map(mode => <button key={mode} type="button" role="radio" aria-checked={approvalMode === mode} className={`tauri-settings-radio${approvalMode === mode ? " is-active" : ""}`} disabled={approvalLoading || approvalSaving} onClick={() => onApprovalChange(mode)}>{mode === "ask" ? "询问" : mode === "auto" ? "自动" : "Yolo"}</button>)}</div></div>
    {approvalError && <p className="tauri-diagnostic-error" role="alert">{approvalError}</p>}
    <label className="tauri-settings-toggle"><Bell className="tauri-settings-field-icon" size={18} /><span><strong>桌面通知</strong><small>回复完成或失败时发送系统通知。</small></span><input type="checkbox" checked={notificationsEnabled} onChange={event => onNotificationsChange(event.target.checked)} /></label>
    <div className="tauri-settings-sound">
      <button type="button" className="tauri-settings-sound-toggle" aria-expanded={soundExpanded} onClick={() => setSoundExpanded(open => !open)}><Volume2 className="tauri-settings-field-icon" size={18} /><span className="tauri-settings-field-label">声音<small>设置完成和需要回答时的提醒音。</small></span><span>{successSound === "off" && attentionSound === "off" ? "已关闭" : "已自定义"}</span><ChevronDown size={15} aria-hidden="true" /></button>
      {soundExpanded && <div className="tauri-settings-sound-body">
        <label className="tauri-settings-sound-row">通知音量 <input type="range" min={0} max={100} value={soundVolume} aria-label="通知音量" onChange={event => setSoundVolume(setNotificationVolume(Number(event.target.value)))} /><output>{soundVolume}%</output></label>
        <TauriSoundOption label="回复完成" value={successSound} onChange={next => { setSuccessSound(next); setSuccessPreference(next); playSuccessChime(); }} onPreview={playSuccessChime} />
        <TauriSoundOption label="需要回答" value={attentionSound} onChange={next => { setAttentionSound(next); setAttentionPreference(next); playAttentionChime(); }} onPreview={playAttentionChime} />
      </div>}
    </div>
  </div>;
}

const SOUND_OPTIONS: { value: SoundWavPref; label: string }[] = [
  { value: "off", label: "关闭" }, { value: "synth", label: "合成音" },
  { value: "positive", label: "清亮" }, { value: "correct", label: "确认" },
  { value: "start", label: "开始" }, { value: "back", label: "轻柔" },
];

function TauriSoundOption({ label, value, onChange, onPreview }: { label: string; value: SoundWavPref; onChange: (next: SoundWavPref) => void; onPreview: () => void }) {
  return <div className="tauri-settings-sound-row"><label>{label}<select aria-label={`${label}提示音`} value={value} onChange={event => onChange(event.target.value as SoundWavPref)}>{SOUND_OPTIONS.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label><button type="button" aria-label={`试听${label}提示音`} disabled={value === "off"} onClick={onPreview}><Play size={14} /></button></div>;
}

function ShortcutSettings() {
  const modifier = /Mac/i.test(navigator.platform) ? "⌘" : "Ctrl+";
  return <div className="tauri-settings-section">
    <h3>工作区快捷键</h3>
    <div className="tauri-settings-shortcuts">
      {[["新建对话", `${modifier}N`], ["打开设置", `${modifier},`], ["运行状态", `${modifier}.`], ["工作区文件", `${modifier}B`], ["刷新当前对话", `${modifier}R`], ["发送消息", `${modifier}Enter`], ["关闭面板", "Esc"]].map(([label, keys]) => <div key={label}><span>{label}</span><kbd>{keys}</kbd></div>)}
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
  return <div className="tauri-settings-load-error" role="alert">读取设置失败。<button type="button" className="tauri-settings-button" onClick={onRetry}><RefreshCw size={13} />重试</button></div>;
}

function AppearanceSettings({ appearance, onChange, conversationWidth, onConversationWidthChange, textSize, onTextSizeChange, fontFamily, onFontFamilyChange, monoFontFamily, onMonoFontFamilyChange, customFontName, onCustomFontChange, customMonoFontName, onCustomMonoFontChange }: {
  appearance: TauriAppearance;
  onChange: (next: TauriAppearance) => void;
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
}) {
  return (
    <div className="tauri-settings-section">
      <h3>外观</h3>
      <p>更改会立即应用，并在下次启动时保留。</p>
      <div className="tauri-settings-field">
        <span className="tauri-settings-field-label">模式</span>
        <div className="tauri-settings-radio-group" role="radiogroup" aria-label="外观模式">
          {(["auto", "light", "dark"] as const satisfies readonly Theme[]).map(option => (
            <button key={option} type="button" role="radio" aria-checked={appearance.mode === option} className={`tauri-settings-radio${appearance.mode === option ? " is-active" : ""}`} onClick={() => onChange({ ...appearance, mode: option })}>
              {appearance.mode === option && <Check size={13} />}
              <span>{option === "light" ? "浅色" : option === "dark" ? "深色" : "跟随系统"}</span>
            </button>
          ))}
        </div>
      </div>
      <div className="tauri-settings-style-heading">配色风格</div>
      <div className="tauri-settings-style-grid" role="radiogroup" aria-label="配色风格">
        {THEME_STYLES.map(style => (
          <button key={style} type="button" role="radio" aria-checked={appearance.style === style} className={`tauri-settings-style${appearance.style === style ? " is-active" : ""}`} onClick={() => onChange({ ...appearance, style })}>
            <span className="tauri-settings-style-swatches" data-style={style} aria-hidden="true"><i /><i /><i /></span>
            <span>{STYLE_LABELS[style]}</span>
            {appearance.style === style && <Check size={13} />}
          </button>
        ))}
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
  const [restarting, setRestarting] = useState(false);
  const [checking, setChecking] = useState(false);
  const [auditRefreshError, setAuditRefreshError] = useState("");
  const [restartResult, setRestartResult] = useState<"ok" | "failed" | "">("");
  const sourceLabel = sessionPageSource === "identity" ? "持久身份目录" : sessionPageSource === "partial_identity" ? "已核验身份目录（存在差异）" : sessionPageSource === "identity_unverified" ? "未核验身份目录（只读）" : sessionPageSource === "legacy" ? "本地兼容目录" : sessionPageSource === "cached" ? "上次审计快照" : "暂不可用";
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
    setAuditRefreshError("");
    try {
      await onRefreshCatalogAudit();
    } catch {
      setAuditRefreshError("会话目录检查失败，请重试。");
    } finally {
      setChecking(false);
    }
  };
  return <div className="tauri-settings-section tauri-settings-diagnostics">
    <h3>本地服务</h3>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">桥接服务<small>负责运行 Preview 会话及连接模型服务。</small></span><span className={`tauri-settings-badge${bridgeStatus?.running ? " is-ready" : ""}`}>{bridgeStatus?.running ? `运行中 · 协议 v${bridgeStatus.protocolVersion ?? "?"}` : "未连接"}</span></div>
    {onRestartBridge && <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" onClick={() => void restart()} disabled={busy || restarting}>重启桥接服务</button></div>}
    {restartResult === "ok" && <p className="tauri-settings-data__notice" role="status">桥接服务已重启。</p>}
    {restartResult === "failed" && <p className="tauri-diagnostic-error" role="alert">{hostError || "桥接服务未能重启，请稍后重试。"}</p>}
    <h3>会话目录</h3>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">当前侧栏数据源<small>切换到身份目录前会进行安全核验。</small></span><span className="tauri-settings-value">{sourceLabel}</span></div>
    {catalogAudit ? <div className="tauri-settings-audit">
      <div><span>旧目录</span><strong>{catalogAudit.legacyCount}</strong></div><div><span>身份目录</span><strong>{catalogAudit.directoryCount}</strong></div><div><span>匹配</span><strong>{catalogAudit.matchedCount}</strong></div><div><span>未认领文件</span><strong>{catalogAudit.unclaimedTranscripts}</strong></div>
      <p>{catalogAudit.legacyMatchesDirectory ? "旧目录中可见会话与身份目录一致。" : `差异：身份库缺项 ${catalogAudit.missingFromDirectory}、标题 ${catalogAudit.titleMismatches}、工作区 ${catalogAudit.workspaceMismatches}、顺序 ${catalogAudit.orderMismatches}、磁盘状态 ${catalogAudit.physicalStateMismatches}、盘点错误 ${catalogAudit.inventoryErrors}。`}</p>
    </div> : <p>{catalogAuditError || "正在检查会话目录…"}</p>}
    {catalogAudit && catalogAuditError && <p className="tauri-diagnostic-error" role="alert">{catalogAuditError}</p>}
    {auditRefreshError && <p className="tauri-diagnostic-error" role="alert">{auditRefreshError}</p>}
    <div className="tauri-settings-actions">
      {onRefreshCatalogAudit && <button type="button" className="tauri-settings-button" onClick={() => void refreshAudit()} disabled={busy || checking}><RefreshCw size={13} />重新检查</button>}
      <button type="button" className="tauri-settings-button" onClick={onOpenData}>打开数据设置</button>
      <button type="button" className="tauri-settings-button" onClick={onOpenProviders}>打开模型服务</button>
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
  const [linkError, setLinkError] = useState(false);
  return (
    <div className="tauri-settings-section">
      <h3>关于 Reasonix Tauri Preview</h3>
      {runtimeInfo ? (
        <div className="tauri-settings-about">
          <div className="tauri-settings-about-row"><span>平台</span><span>{platform === "darwin" ? "macOS" : platform === "windows" ? "Windows" : platform === "linux" ? "Linux" : "未知"}</span></div>
          <div className="tauri-settings-about-row"><span>Preview 版本</span><span>v{runtimeInfo.previewVersion}</span></div>
          <div className="tauri-settings-about-row"><span>稳定版基线</span><span>v{runtimeInfo.stableVersion}</span></div>
          <div className="tauri-settings-about-row"><span>Tauri</span><span>v{runtimeInfo.tauriVersion}</span></div>
          <div className="tauri-settings-about-row"><span>桥接协议</span><span>v{runtimeInfo.bridgeProtocolVersion}</span></div>
          <div className="tauri-settings-about-row"><span>构建时间</span><span>{runtimeInfo.previewBuild}</span></div>
          <div className="tauri-settings-about-row"><span>Sidecar</span><span>{runtimeInfo.sidecarInstanceId ?? "未运行"}</span></div>
        </div>
      ) : (
        <p>正在读取版本信息…</p>
      )}
      <div className="tauri-settings-actions">
        <button type="button" className="tauri-settings-button" onClick={onRefresh}><RefreshCw size={14} /> 刷新</button>
        <a className="tauri-settings-button" href="https://github.com/esengine/DeepSeek-Reasonix" onClick={event => {
          event.preventDefault();
          setLinkError(false);
          void openTauriExternalURL(event.currentTarget.href).catch(() => setLinkError(true));
        }}><ExternalLink size={14} /> GitHub</a>
      </div>
      {linkError && <p role="alert">无法在系统浏览器中打开链接</p>}
    </div>
  );
}
