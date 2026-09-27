import { useEffect, useState } from "react";
import { deleteTauriProviderConfig, saveTauriProviderConfig, tauriMessageFrom, tauriProviderConfigs, tauriProviderSummary, type TauriProviderConfig, type TauriProviderConfigInput, type TauriProviderSummary } from "../lib/tauriBridge";

const EMPTY: TauriProviderConfigInput = { name: "", displayName: "", kind: "openai", baseUrl: "", models: [], default: "", useApiKey: true };

export function TauriProviderEditor({ onSummaryChange }: { onSummaryChange: (summary: TauriProviderSummary) => void }) {
  const [configs, setConfigs] = useState<TauriProviderConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<TauriProviderConfigInput | null>(null);
  const [editingExisting, setEditingExisting] = useState(false);
  const [modelText, setModelText] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  useEffect(() => {
    let active = true;
    void tauriProviderConfigs().then(view => { if (active) setConfigs(view.providers); }).catch(err => { if (active) setError(tauriMessageFrom(err)); }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, []);

  const startEdit = (provider?: TauriProviderConfig) => {
    setError("");
    setMessage("");
    setEditing(provider ? { ...provider, baseUrl: "", useApiKey: false } : { ...EMPTY });
    setEditingExisting(Boolean(provider));
    setModelText(provider?.models.join("\n") ?? "");
  };

  const save = async () => {
    if (!editing || saving) return;
    const models = modelText.split(/\r?\n|,/).map(value => value.trim()).filter(Boolean);
    if (!/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$/.test(editing.name)) { setError("标识只能使用字母、数字、连字符和下划线，最多 64 位。"); return; }
    if (models.length === 0 || new Set(models).size !== models.length) { setError("请填写至少一个不重复的模型 ID。"); return; }
    if (!editingExisting && configs.some(provider => provider.name === editing.name)) { setError("服务标识已存在，请在列表中选择编辑。"); return; }
    if (!editingExisting && !editing.baseUrl.trim()) { setError("新增服务需要填写端点地址。"); return; }
    if (editing.baseUrl.trim()) {
      try {
        const endpoint = new URL(editing.baseUrl.trim());
        const loopback = endpoint.hostname === "localhost" || endpoint.hostname.endsWith(".localhost") || endpoint.hostname === "127.0.0.1" || endpoint.hostname === "[::1]";
        if (!endpoint.hostname || endpoint.username || endpoint.password || endpoint.search || endpoint.hash || (endpoint.protocol !== "https:" && !(endpoint.protocol === "http:" && loopback))) {
          setError("端点必须使用 HTTPS；本机服务可使用 HTTP。请不要在地址中放入密钥或查询参数。"); return;
        }
      } catch { setError("请输入有效的服务端点地址。"); return; }
    }
    const input: TauriProviderConfigInput = { name: editing.name, displayName: editing.displayName, kind: editing.kind, baseUrl: editing.baseUrl, models, default: models.includes(editing.default) ? editing.default : models[0], useApiKey: editing.useApiKey };
    setSaving(true);
    setError("");
    try {
      const view = await saveTauriProviderConfig(input);
      setConfigs(view.providers);
      setEditing(null);
      setMessage("模型服务已保存；新会话会读取更新后的配置。");
      try { onSummaryChange(await tauriProviderSummary()); }
      catch { setError("配置已保存，但服务状态刷新失败。请重新打开设置。"); }
    } catch (err) {
      setError(tauriMessageFrom(err));
    } finally {
      setSaving(false);
    }
  };

  const remove = async (provider: TauriProviderConfig) => {
    if (saving || !provider.removable || !window.confirm(`删除模型服务“${provider.displayName || provider.name}”？新会话将不能再选择它，已保存的钥匙串凭据不会被删除。`)) return;
    setSaving(true);
    setError(""); setMessage("");
    try {
      const view = await deleteTauriProviderConfig(provider);
      setConfigs(view.providers);
      setEditing(null);
      setMessage("模型服务已删除；新会话会使用剩余的默认模型。");
      try { onSummaryChange(await tauriProviderSummary()); }
      catch { setError("服务已删除，但状态刷新失败。请重新打开设置。"); }
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSaving(false); }
  };

  return <div className="tauri-provider-editor">
    <div className="tauri-settings-model-header"><strong>服务配置</strong><button type="button" className="tauri-settings-button" onClick={() => startEdit()} disabled={saving}>添加服务</button></div>
    <p className="tauri-settings-hint">支持 OpenAI Chat、Anthropic Messages 和 Responses 协议。端点与凭据不会显示在服务列表中。</p>
    {loading ? <p className="tauri-settings-hint">正在加载…</p> : configs.map(provider => <div className="tauri-provider-editor-row" key={provider.name}>
      <span><strong>{provider.displayName || provider.name}</strong><small>{provider.kind} · {provider.models.length} 个模型</small></span>
      <button type="button" className="tauri-settings-button" onClick={() => startEdit(provider)} disabled={saving || !["openai", "anthropic", "responses"].includes(provider.kind)}>编辑</button>
      {provider.removable && <button type="button" className="tauri-settings-button" onClick={() => void remove(provider)} disabled={saving}>删除</button>}
    </div>)}
    {editing && <div className="tauri-provider-editor-form">
      <h4>{editingExisting ? "编辑模型服务" : "添加模型服务"}</h4>
      <label>服务标识<input value={editing.name} disabled={editingExisting || saving} onChange={event => setEditing({ ...editing, name: event.target.value })} placeholder="例如 my-provider" /></label>
      <label>显示名称<input value={editing.displayName} disabled={saving} onChange={event => setEditing({ ...editing, displayName: event.target.value })} placeholder="显示在模型选择器中的名称" /></label>
      <label>协议<select value={editing.kind} disabled={editingExisting || saving} onChange={event => setEditing({ ...editing, kind: event.target.value })}><option value="openai">OpenAI Chat</option><option value="anthropic">Anthropic Messages</option><option value="responses">Responses</option></select></label>
      <label>服务端点<input value={editing.baseUrl} disabled={saving} onChange={event => setEditing({ ...editing, baseUrl: event.target.value })} placeholder={editingExisting ? "留空保留现有端点" : "https://example.com/v1"} /></label>
      <label>模型 ID（每行一个）<textarea rows={4} value={modelText} disabled={saving} onChange={event => setModelText(event.target.value)} /></label>
      {modelText.trim() && <label>默认模型<select value={editing.default && modelText.split(/\r?\n|,/).map(value => value.trim()).includes(editing.default) ? editing.default : modelText.split(/\r?\n|,/).map(value => value.trim()).filter(Boolean)[0] ?? ""} disabled={saving} onChange={event => setEditing({ ...editing, default: event.target.value })}>{modelText.split(/\r?\n|,/).map(value => value.trim()).filter(Boolean).map(model => <option key={model} value={model}>{model}</option>)}</select></label>}
      {!editingExisting && <label className="tauri-provider-editor-checkbox"><input type="checkbox" checked={editing.useApiKey} disabled={saving} onChange={event => setEditing({ ...editing, useApiKey: event.target.checked })} />此服务需要 API Key（创建后可保存到系统钥匙串）</label>}
      <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" onClick={() => void save()} disabled={saving}>{saving ? "保存中…" : "保存服务"}</button><button type="button" className="tauri-settings-button" onClick={() => setEditing(null)} disabled={saving}>取消</button></div>
    </div>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {message && <p className="tauri-settings-hint" role="status">{message}</p>}
  </div>;
}
