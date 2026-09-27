import { useState, useCallback, useEffect, useRef } from "react";
import { X, Check, ChevronRight, Globe, Palette, Info, RefreshCw, ExternalLink, Key, Eye, EyeOff, Server, Database, SlidersHorizontal } from "lucide-react";
import { tauriPreviewRuntimeInfo, tauriProviderSummary, setTauriDefaultModel, tauriPlatformInfo, keychainSave, keychainDelete, openTauriExternalURL, tauriMessageFrom, type TauriPreviewProfileStatus, type TauriPreviewRuntimeInfo, type TauriProviderSummary } from "../lib/tauriBridge";
import { THEME_STYLES, type Theme, type ThemeStyle } from "../lib/theme";
import { applyConversationWidth, getCachedConversationWidth, type ConversationWidth } from "../lib/conversationWidth";
import { applyTextSize, getTextSize, TEXT_SIZES, type TextSize } from "../lib/textSize";
import { applyFontFamily, applyMonoFontFamily, FONT_FAMILIES, MONO_FONT_FAMILIES, getCustomFontName, getCustomMonoFontName, getFontFamily, getMonoFontFamily, setCustomFontName, setCustomMonoFontName, type FontFamily, type MonoFontFamily } from "../lib/fontFamily";
import { applyTauriAppearance, readTauriAppearance, type TauriAppearance } from "./tauriAppearance";
import { TauriMCPSettings } from "./TauriMCPSettings";
import { getTauriNotificationsEnabled, setTauriNotificationsEnabled } from "./tauriPreferences";

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
}

export type TauriSettingsTab = "general" | "appearance" | "model" | "mcp" | "data" | "about";

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

export function TauriSettings({ onClose, onProviderSummaryChange, currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession, workspaceRoot, initialTab = "appearance", profile, onRefreshProfile, onImportStableProfile, onImportStableProjectFolders, onScanUnclaimedSessions, importBusy }: TauriSettingsProps) {
  const [tab, setTab] = useState<TauriSettingsTab>(initialTab);
  const [runtimeInfo, setRuntimeInfo] = useState<TauriPreviewRuntimeInfo | null>(null);
  const [providerSummary, setProviderSummaryState] = useState<TauriProviderSummary | null>(null);
  const [modelLoading, setModelLoading] = useState(true);
  const [aboutLoading, setAboutLoading] = useState(true);
  const [modelLoadError, setModelLoadError] = useState(false);
  const [aboutLoadError, setAboutLoadError] = useState(false);
  const [platform, setPlatform] = useState("");
  const loadRequest = useRef(0);
  const [appearance, setAppearance] = useState<TauriAppearance>(readTauriAppearance);
  const [conversationWidth, setConversationWidth] = useState<ConversationWidth>(getCachedConversationWidth);
  const [textSize, setTextSize] = useState<TextSize>(getTextSize);
  const [fontFamily, setFontFamily] = useState<FontFamily>(getFontFamily);
  const [monoFontFamily, setMonoFontFamily] = useState<MonoFontFamily>(getMonoFontFamily);
  const [customFontName, setCustomFontNameState] = useState(getCustomFontName);
  const [customMonoFontName, setCustomMonoFontNameState] = useState(getCustomMonoFontName);
  const [notificationsEnabled, setNotificationsEnabled] = useState(getTauriNotificationsEnabled);

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
    try {
      const updated = await setTauriDefaultModel(model);
      updateProviderSummary(updated);
    } catch {
      // Non-fatal
    }
  };

  return (
    <div className="tauri-settings-overlay">
      <div className="tauri-settings-scrim" onClick={onClose} />
      <div className="tauri-settings-panel">
        <header className="tauri-settings-header">
          <h2>设置</h2>
          <button type="button" className="tauri-icon-button" onClick={onClose} aria-label="关闭"><X size={17} /></button>
        </header>

        <nav className="tauri-settings-nav">
          <button type="button" className={`tauri-settings-nav-item${tab === "general" ? " is-active" : ""}`} onClick={() => setTab("general")}>
            <SlidersHorizontal size={15} /><span>通用</span><ChevronRight size={13} />
          </button>
          <button type="button" className={`tauri-settings-nav-item${tab === "appearance" ? " is-active" : ""}`} onClick={() => setTab("appearance")}>
            <Palette size={15} /><span>外观</span><ChevronRight size={13} />
          </button>
          <button type="button" className={`tauri-settings-nav-item${tab === "model" ? " is-active" : ""}`} onClick={() => setTab("model")}>
            <Globe size={15} /><span>模型</span><ChevronRight size={13} />
          </button>
          <button type="button" className={`tauri-settings-nav-item${tab === "mcp" ? " is-active" : ""}`} onClick={() => setTab("mcp")}>
            <Server size={15} /><span>MCP</span><ChevronRight size={13} />
          </button>
          <button type="button" className={`tauri-settings-nav-item${tab === "data" ? " is-active" : ""}`} onClick={() => setTab("data")}>
            <Database size={15} /><span>数据</span><ChevronRight size={13} />
          </button>
          <button type="button" className={`tauri-settings-nav-item${tab === "about" ? " is-active" : ""}`} onClick={() => setTab("about")}>
            <Info size={15} /><span>关于</span><ChevronRight size={13} />
          </button>
        </nav>

        <div className="tauri-settings-content">
          {tab === "general" ? <GeneralSettings notificationsEnabled={notificationsEnabled} onNotificationsChange={enabled => { setNotificationsEnabled(enabled); setTauriNotificationsEnabled(enabled); }} /> : tab === "appearance" ? <AppearanceSettings appearance={appearance} onChange={handleAppearanceChange} conversationWidth={conversationWidth} onConversationWidthChange={handleConversationWidthChange} textSize={textSize} onTextSizeChange={handleTextSizeChange} fontFamily={fontFamily} onFontFamilyChange={handleFontFamilyChange} monoFontFamily={monoFontFamily} onMonoFontFamilyChange={handleMonoFontFamilyChange} customFontName={customFontName} onCustomFontChange={handleCustomFontChange} customMonoFontName={customMonoFontName} onCustomMonoFontChange={handleCustomMonoFontChange} /> : <>
            {tab === "model" && (modelLoading ? <div className="tauri-settings-loading">加载中…</div> : <>{modelLoadError && <SettingsLoadError onRetry={loadSettings} />}{providerSummary && <ModelSettings providerSummary={providerSummary} onModelChange={handleModelChange} onProviderSummaryChange={updateProviderSummary} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}</>)}
            {tab === "mcp" && <TauriMCPSettings workspaceRoot={workspaceRoot} />}
            {tab === "data" && <DataSettings profile={profile} busy={Boolean(importBusy)} onRefreshProfile={onRefreshProfile} onImportStableProfile={onImportStableProfile} onImportStableProjectFolders={onImportStableProjectFolders} onScanUnclaimedSessions={onScanUnclaimedSessions} onClose={onClose} />}
            {tab === "about" && (aboutLoading ? <div className="tauri-settings-loading">加载中…</div> : <>{aboutLoadError && <SettingsLoadError onRetry={loadSettings} />}{runtimeInfo && <AboutSettings runtimeInfo={runtimeInfo} platform={platform} onRefresh={loadSettings} />}</>)}
          </>}
        </div>
      </div>
    </div>
  );
}

function GeneralSettings({ notificationsEnabled, onNotificationsChange }: { notificationsEnabled: boolean; onNotificationsChange: (enabled: boolean) => void }) {
  const modifier = /Mac/i.test(navigator.platform) ? "⌘" : "Ctrl+";
  return <div className="tauri-settings-section tauri-settings-general">
    <h3>通用</h3>
    <label className="tauri-settings-toggle"><span><strong>桌面通知</strong><small>回复完成或失败时发送系统通知。</small></span><input type="checkbox" checked={notificationsEnabled} onChange={event => onNotificationsChange(event.target.checked)} /></label>
    <h3>快捷键</h3>
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

function ModelSettings({ providerSummary, onModelChange, onProviderSummaryChange, currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  providerSummary: TauriProviderSummary | null;
  onModelChange: (m: string) => void;
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

  if (!providerSummary) return <div className="tauri-settings-loading">正在读取 Provider 配置…</div>;

  return (
    <div className="tauri-settings-section">
      <h3>模型提供方</h3>
      <p className="tauri-settings-hint">默认模型用于新对话。工作区 <code>reasonix.toml</code> 可能覆盖此设置。</p>
      {providerSummary.providers.length === 0 ? (
        <div className="tauri-settings-empty">未配置任何 Provider。请编辑 <code>~/.reasonix/config.toml</code> 添加 Provider 配置。</div>
      ) : (
        <div className="tauri-settings-model-list">
          {providerSummary.providers.map(provider => (
            <div key={provider.name} className="tauri-settings-model-card">
              <div className="tauri-settings-model-header">
                <strong>{provider.displayName || provider.name}</strong>
                <span className={`tauri-settings-badge${provider.configured ? " is-ready" : ""}`}>{provider.configured ? "已就绪" : "未配置"}</span>
              </div>
              <p className="tauri-settings-model-meta">{provider.kind} · {provider.modelCount} 个模型</p>
              {provider.configured && provider.models.length > 0 && (
                <div className="tauri-settings-model-select">
                  <select
                    value={providerSummary.defaultModel?.startsWith(provider.name + "/") ? providerSummary.defaultModel : ""}
                    onChange={e => { if (e.target.value) onModelChange(e.target.value); }}
                  >
                    <option value="">选择默认模型…</option>
                    {provider.models.map(model => (
                      <option key={model} value={`${provider.name}/${model}`}>{model}</option>
                    ))}
                  </select>
                </div>
              )}
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
