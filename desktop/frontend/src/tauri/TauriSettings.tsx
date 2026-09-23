import { useState, useCallback, useEffect, useRef } from "react";
import { X, Check, ChevronRight, Globe, Palette, Info, RefreshCw, ExternalLink, Key, Eye, EyeOff } from "lucide-react";
import { tauriPreviewRuntimeInfo, tauriProviderSummary, setTauriDefaultModel, tauriPlatformInfo, keychainSave, keychainDelete, type TauriPreviewRuntimeInfo, type TauriProviderSummary } from "../lib/tauriBridge";

interface TauriSettingsProps {
  onClose: () => void;
  onProviderSummaryChange?: (summary: TauriProviderSummary) => void;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}

type SettingsTab = "general" | "model" | "about";

export function TauriSettings({ onClose, onProviderSummaryChange, currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: TauriSettingsProps) {
  const [tab, setTab] = useState<SettingsTab>("general");
  const [runtimeInfo, setRuntimeInfo] = useState<TauriPreviewRuntimeInfo | null>(null);
  const [providerSummary, setProviderSummaryState] = useState<TauriProviderSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [platform, setPlatform] = useState("");
  const loadRequest = useRef(0);
  const [theme, setTheme] = useState<"light" | "dark" | "system">(() => {
    const saved = localStorage.getItem("tauri-theme");
    return (saved as "light" | "dark" | "system") || "system";
  });

  const updateProviderSummary = useCallback((summary: TauriProviderSummary) => {
    setProviderSummaryState(summary);
    onProviderSummaryChange?.(summary);
  }, [onProviderSummaryChange]);

  const loadSettings = useCallback(async () => {
    const request = ++loadRequest.current;
    setLoading(true);
    try {
      const [info, providers, plat] = await Promise.all([
        tauriPreviewRuntimeInfo(),
        tauriProviderSummary(),
        tauriPlatformInfo(),
      ]);
      if (request === loadRequest.current) {
        setRuntimeInfo(info);
        updateProviderSummary(providers);
        setPlatform(plat);
      }
    } catch {
      // Non-fatal
    } finally {
      if (request === loadRequest.current) setLoading(false);
    }
  }, [updateProviderSummary]);

  useEffect(() => {
    void loadSettings();
    return () => { loadRequest.current += 1; };
  }, [loadSettings]);

  const handleThemeChange = (newTheme: "light" | "dark" | "system") => {
    setTheme(newTheme);
    localStorage.setItem("tauri-theme", newTheme);
    document.documentElement.setAttribute("data-theme", newTheme === "system" ? "" : newTheme);
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
            <Palette size={15} /><span>通用</span><ChevronRight size={13} />
          </button>
          <button type="button" className={`tauri-settings-nav-item${tab === "model" ? " is-active" : ""}`} onClick={() => setTab("model")}>
            <Globe size={15} /><span>模型</span><ChevronRight size={13} />
          </button>
          <button type="button" className={`tauri-settings-nav-item${tab === "about" ? " is-active" : ""}`} onClick={() => setTab("about")}>
            <Info size={15} /><span>关于</span><ChevronRight size={13} />
          </button>
        </nav>

        <div className="tauri-settings-content">
          {loading ? <div className="tauri-settings-loading">加载中…</div> : <>
            {tab === "general" && <GeneralSettings theme={theme} onThemeChange={handleThemeChange} platform={platform} />}
            {tab === "model" && <ModelSettings providerSummary={providerSummary} onModelChange={handleModelChange} onProviderSummaryChange={updateProviderSummary} currentSessionState={currentSessionState} currentSessionHasAttachments={currentSessionHasAttachments} onApplyToCurrentSession={onApplyToCurrentSession} />}
            {tab === "about" && <AboutSettings runtimeInfo={runtimeInfo} onRefresh={loadSettings} />}
          </>}
        </div>
      </div>
    </div>
  );
}

function GeneralSettings({ theme, onThemeChange, platform }: { theme: string; onThemeChange: (t: "light" | "dark" | "system") => void; platform: string }) {
  return (
    <div className="tauri-settings-section">
      <h3>外观</h3>
      <div className="tauri-settings-field">
        <label>主题</label>
        <div className="tauri-settings-radio-group">
          {(["light", "dark", "system"] as const).map(option => (
            <button key={option} type="button" className={`tauri-settings-radio${theme === option ? " is-active" : ""}`} onClick={() => onThemeChange(option)}>
              {theme === option && <Check size={13} />}
              <span>{option === "light" ? "浅色" : option === "dark" ? "深色" : "跟随系统"}</span>
            </button>
          ))}
        </div>
      </div>
      <div className="tauri-settings-field">
        <label>平台</label>
        <span className="tauri-settings-value">{platform === "darwin" ? "macOS" : platform === "windows" ? "Windows" : "Linux"}</span>
      </div>
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

function AboutSettings({ runtimeInfo, onRefresh }: { runtimeInfo: TauriPreviewRuntimeInfo | null; onRefresh: () => void }) {
  return (
    <div className="tauri-settings-section">
      <h3>关于 Reasonix Tauri Preview</h3>
      {runtimeInfo ? (
        <div className="tauri-settings-about">
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
        <a className="tauri-settings-button" href="https://github.com/esengine/DeepSeek-Reasonix" target="_blank" rel="noreferrer"><ExternalLink size={14} /> GitHub</a>
      </div>
    </div>
  );
}
