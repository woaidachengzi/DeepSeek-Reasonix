import { useState, useCallback } from "react";
import { X, Check, ChevronRight, Globe, Palette, Info, RefreshCw, ExternalLink } from "lucide-react";
import { tauriPreviewRuntimeInfo, tauriProviderSummary, setTauriDefaultModel, tauriPlatformInfo, type TauriPreviewRuntimeInfo, type TauriProviderSummary } from "../lib/tauriBridge";

interface TauriSettingsProps {
  onClose: () => void;
}

type SettingsTab = "general" | "model" | "about";

export function TauriSettings({ onClose }: TauriSettingsProps) {
  const [tab, setTab] = useState<SettingsTab>("general");
  const [runtimeInfo, setRuntimeInfo] = useState<TauriPreviewRuntimeInfo | null>(null);
  const [providerSummary, setProviderSummaryState] = useState<TauriProviderSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [platform, setPlatform] = useState("");
  const [theme, setTheme] = useState<"light" | "dark" | "system">(() => {
    const saved = localStorage.getItem("tauri-theme");
    return (saved as "light" | "dark" | "system") || "system";
  });

  const loadSettings = useCallback(async () => {
    setLoading(true);
    try {
      const [info, providers, plat] = await Promise.all([
        tauriPreviewRuntimeInfo(),
        tauriProviderSummary(),
        tauriPlatformInfo(),
      ]);
      setRuntimeInfo(info);
      setProviderSummaryState(providers);
      setPlatform(plat);
    } catch {
      // Non-fatal
    } finally {
      setLoading(false);
    }
  }, []);

  // Load on mount
  useState(() => { void loadSettings(); });

  const handleThemeChange = (newTheme: "light" | "dark" | "system") => {
    setTheme(newTheme);
    localStorage.setItem("tauri-theme", newTheme);
    document.documentElement.setAttribute("data-theme", newTheme === "system" ? "" : newTheme);
  };

  const handleModelChange = async (model: string) => {
    try {
      const updated = await setTauriDefaultModel(model);
      setProviderSummaryState(updated);
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
            {tab === "model" && <ModelSettings providerSummary={providerSummary} onModelChange={handleModelChange} />}
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

function ModelSettings({ providerSummary, onModelChange }: { providerSummary: TauriProviderSummary | null; onModelChange: (m: string) => void }) {
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
            </div>
          ))}
        </div>
      )}
      <p className="tauri-settings-hint">密钥和环境变量名不会显示在界面中。</p>
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
