import { useEffect, useRef, useState } from "react";
import { changeTauriNetworkSettings, tauriMessageFrom, tauriNetworkSettings, type TauriNetworkChange, type TauriNetworkSettings } from "../lib/tauriBridge";

const MODES = [
  { value: "auto", label: "自动" },
  { value: "env", label: "环境变量" },
  { value: "custom", label: "自定义" },
  { value: "off", label: "直连" },
] as const;
const TYPES = ["http", "https", "socks5", "socks5h"] as const;

function draftFromView(view: TauriNetworkSettings): TauriNetworkChange {
  return {
    proxyMode: view.proxyMode,
    noProxy: view.noProxy,
    proxyType: view.proxyType || "socks5",
    proxyServer: view.proxyServer,
    proxyPort: view.proxyPort,
    proxyUsername: view.proxyUsername,
    proxyUrlAction: "keep",
    proxyUrl: "",
    proxyPasswordAction: "keep",
    proxyPassword: "",
  };
}

export function TauriNetworkSettings({ currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const [view, setView] = useState<TauriNetworkSettings | null>(null);
  const [draft, setDraft] = useState<TauriNetworkChange | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pendingApply, setPendingApply] = useState(false);
  const busyRef = useRef(false);

  const reload = () => {
    setLoading(true);
    setError("");
    void tauriNetworkSettings().then(next => { setView(next); setDraft(draftFromView(next)); })
      .catch(err => setError(tauriMessageFrom(err))).finally(() => setLoading(false));
  };
  useEffect(reload, []);

  const save = async () => {
    if (!draft || busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const next = await changeTauriNetworkSettings(draft);
      setView(next);
      setDraft(draftFromView(next));
      setPendingApply(true);
      setNotice("网络设置已保存；新会话会读取新配置。");
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const applyCurrent = async () => {
    if (!onApplyToCurrentSession || busyRef.current || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      if (await onApplyToCurrentSession()) {
        setPendingApply(false);
        setNotice("当前会话已重新载入网络配置。");
      } else setError("当前会话未能更新，请在运行诊断中重试。");
    } catch { setError("当前会话未能更新，请在运行诊断中重试。"); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const dirty = Boolean(view && draft && JSON.stringify(draft) !== JSON.stringify(draftFromView(view)));
  const overrideURL = Boolean(view && draft && (draft.proxyUrlAction === "replace" || (view.proxyUrlSet && draft.proxyUrlAction === "keep")));
  return <div className="tauri-settings-section tauri-network-settings">
    <h3>代理模式</h3>
    <p>控制模型服务和其他普通 HTTP 请求的出站代理。自动模式目前使用系统环境变量。</p>
    {loading ? <div className="tauri-settings-loading">加载中…</div> : view && draft ? <>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">连接方式<small>直连会绕过代理；自定义会使用下方代理配置。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="代理模式">{MODES.map(mode => <button type="button" role="radio" aria-checked={draft.proxyMode === mode.value} className={`tauri-settings-radio${draft.proxyMode === mode.value ? " is-active" : ""}`} key={mode.value} disabled={busy} onClick={() => setDraft({ ...draft, proxyMode: mode.value })}>{mode.label}</button>)}</div></div>
      {draft.proxyMode === "custom" && <>
        <h3>自定义代理</h3>
        <p>代理 URL 优先于下方的服务器、端口和凭据。已有 URL 和密码不会回显。</p>
        <div className="tauri-settings-field"><span className="tauri-settings-field-label">代理 URL<small>{view.proxyUrlSet && draft.proxyUrlAction === "keep" ? "已设置；留空会保留原值。" : "例如 socks5://127.0.0.1:7890"}</small></span><div className="tauri-network-secret"><input className="tauri-settings-input" aria-label="代理 URL" value={draft.proxyUrl} disabled={busy} maxLength={4096} placeholder={view.proxyUrlSet && draft.proxyUrlAction === "keep" ? "已设置（不回显）" : "socks5://127.0.0.1:7890"} onChange={event => setDraft({ ...draft, proxyUrl: event.target.value, proxyUrlAction: event.target.value ? "replace" : "keep" })} />{(view.proxyUrlSet || draft.proxyUrlAction === "replace") && <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setDraft({ ...draft, proxyUrl: "", proxyUrlAction: "clear" })}>清除</button>}</div></div>
        <div className="tauri-settings-field"><span className="tauri-settings-field-label">代理类型</span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="代理类型">{TYPES.map(type => <button type="button" role="radio" aria-checked={draft.proxyType === type} className={`tauri-settings-radio${draft.proxyType === type ? " is-active" : ""}`} key={type} disabled={busy || overrideURL} onClick={() => setDraft({ ...draft, proxyType: type })}>{type.toUpperCase()}</button>)}</div></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="tauri-proxy-server">代理服务器</label><div className="tauri-network-inline"><input id="tauri-proxy-server" className="tauri-settings-input" value={draft.proxyServer} disabled={busy || overrideURL} maxLength={2048} placeholder="127.0.0.1" onChange={event => setDraft({ ...draft, proxyServer: event.target.value })} /><input className="tauri-settings-input" aria-label="代理端口" inputMode="numeric" value={draft.proxyPort || ""} disabled={busy || overrideURL} placeholder="7890" onChange={event => setDraft({ ...draft, proxyPort: Number(event.target.value) || 0 })} /></div></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="tauri-proxy-username">代理用户名</label><input id="tauri-proxy-username" className="tauri-settings-input" value={draft.proxyUsername} disabled={busy || overrideURL} maxLength={1024} onChange={event => setDraft({ ...draft, proxyUsername: event.target.value })} /></div>
        <div className="tauri-settings-field"><span className="tauri-settings-field-label">代理密码<small>{view.proxyPasswordSet && draft.proxyPasswordAction === "keep" ? "已设置；留空会保留原值。" : "可使用环境变量表达式。"}</small></span><div className="tauri-network-secret"><input className="tauri-settings-input" aria-label="代理密码" type="password" value={draft.proxyPassword} disabled={busy || overrideURL} maxLength={4096} placeholder={view.proxyPasswordSet && draft.proxyPasswordAction === "keep" ? "已设置（不回显）" : "密码或 ${VAR}"} onChange={event => setDraft({ ...draft, proxyPassword: event.target.value, proxyPasswordAction: event.target.value ? "replace" : "keep" })} />{(view.proxyPasswordSet || draft.proxyPasswordAction === "replace") && <button type="button" className="tauri-settings-button" disabled={busy || overrideURL} onClick={() => setDraft({ ...draft, proxyPassword: "", proxyPasswordAction: "clear" })}>清除</button>}</div></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="tauri-no-proxy">不走代理的地址</label><input id="tauri-no-proxy" className="tauri-settings-input" value={draft.noProxy} disabled={busy} maxLength={4096} placeholder="localhost,127.0.0.1,.local" onChange={event => setDraft({ ...draft, noProxy: event.target.value })} /></div>
      </>}
      <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy || !dirty} onClick={() => setDraft(draftFromView(view))}>取消修改</button><button type="button" className="tauri-settings-button" disabled={busy || !dirty} onClick={() => void save()}>保存网络设置</button>{pendingApply && currentSessionState && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" disabled={busy || dirty || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>应用到当前会话</button>}</div>
    </> : <button type="button" className="tauri-settings-button" onClick={reload}>重试读取</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
