import { useEffect, useState } from "react";
import { deleteTauriProviderConfig, discoverTauriProviderModels, installTauriProviderPreset, resetTauriProviderPreset, saveTauriProviderConfig, tauriMessageFrom, tauriProviderConfigs, tauriProviderSummary, type TauriProviderConfig, type TauriProviderConfigInput, type TauriProviderPreset, type TauriProviderSummary } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";

const EMPTY: TauriProviderConfigInput = { name: "", displayName: "", kind: "openai", baseUrl: "", modelsUrl: "", clearModelsUrl: false, modelsUrlSet: false, noProxy: false, contextWindow: 0, responsesMode: "", balanceUrl: "", clearBalanceUrl: false, balanceUrlSet: false, models: [], default: "", useApiKey: true };

export function TauriProviderEditor({ onSummaryChange }: { onSummaryChange: (summary: TauriProviderSummary) => void }) {
  const t = useT();
  const [configs, setConfigs] = useState<TauriProviderConfig[]>([]);
  const [presets, setPresets] = useState<TauriProviderPreset[]>([]);
  const [selectedPreset, setSelectedPreset] = useState("");
  const [resetArmed, setResetArmed] = useState("");
  const [presetQuery, setPresetQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<TauriProviderConfigInput | null>(null);
  const [editingExisting, setEditingExisting] = useState(false);
  const [modelText, setModelText] = useState("");
  const [saving, setSaving] = useState(false);
  const [discoveringModels, setDiscoveringModels] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  useEffect(() => {
    let active = true;
    void tauriProviderConfigs().then(view => { if (active) { setConfigs(view.providers); setPresets(view.presets || []); } }).catch(err => { if (active) setError(tauriMessageFrom(err)); }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, []);

  const startEdit = (provider?: TauriProviderConfig) => {
    setError("");
    setMessage("");
    setEditing(provider ? { ...provider, baseUrl: "", modelsUrl: "", clearModelsUrl: false, balanceUrl: "", clearBalanceUrl: false, useApiKey: false } : { ...EMPTY });
    setEditingExisting(Boolean(provider));
    setModelText(provider?.models.join("\n") ?? "");
  };

  const save = async () => {
    if (!editing || saving) return;
    const models = modelText.split(/\r?\n|,/).map(value => value.trim()).filter(Boolean);
    if (!/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$/.test(editing.name)) { setError(t("settings.previewProvider.error.invalidName")); return; }
    if (models.length === 0 || new Set(models).size !== models.length) { setError(t("settings.previewProvider.error.modelsRequired")); return; }
    if (!editingExisting && configs.some(provider => provider.name === editing.name)) { setError(t("settings.previewProvider.error.duplicateName")); return; }
    if (!editingExisting && !editing.baseUrl.trim()) { setError(t("settings.previewProvider.error.endpointRequired")); return; }
    if (editing.baseUrl.trim()) {
      try {
        const endpoint = new URL(editing.baseUrl.trim());
        const loopback = endpoint.hostname === "localhost" || endpoint.hostname.endsWith(".localhost") || endpoint.hostname === "127.0.0.1" || endpoint.hostname === "[::1]";
        if (!endpoint.hostname || endpoint.username || endpoint.password || endpoint.search || endpoint.hash || (endpoint.protocol !== "https:" && !(endpoint.protocol === "http:" && loopback))) {
          setError(t("settings.previewProvider.error.secureEndpoint")); return;
        }
      } catch { setError(t("settings.previewProvider.error.invalidEndpoint")); return; }
    }
    if (editing.balanceUrl.trim()) {
      try {
        const endpoint = new URL(editing.balanceUrl.trim());
        const loopback = endpoint.hostname === "localhost" || endpoint.hostname.endsWith(".localhost") || endpoint.hostname === "127.0.0.1" || endpoint.hostname === "[::1]";
        if (!endpoint.hostname || endpoint.username || endpoint.password || endpoint.search || endpoint.hash || (endpoint.protocol !== "https:" && !(endpoint.protocol === "http:" && loopback))) {
          setError(t("settings.previewProvider.error.secureEndpoint")); return;
        }
      } catch { setError(t("settings.previewProvider.error.invalidEndpoint")); return; }
    }
    if (editing.modelsUrl.trim()) {
      try {
        const endpoint = new URL(editing.modelsUrl.trim());
        const loopback = endpoint.hostname === "localhost" || endpoint.hostname.endsWith(".localhost") || endpoint.hostname === "127.0.0.1" || endpoint.hostname === "[::1]";
        if (!endpoint.hostname || endpoint.username || endpoint.password || endpoint.search || endpoint.hash || (endpoint.protocol !== "https:" && !(endpoint.protocol === "http:" && loopback))) {
          setError(t("settings.previewProvider.error.secureEndpoint")); return;
        }
      } catch { setError(t("settings.previewProvider.error.invalidEndpoint")); return; }
    }
    const input: TauriProviderConfigInput = { name: editing.name, displayName: editing.displayName, kind: editing.kind, baseUrl: editing.baseUrl, modelsUrl: editing.modelsUrl, clearModelsUrl: editing.clearModelsUrl, modelsUrlSet: editing.modelsUrlSet, noProxy: editing.noProxy, contextWindow: editing.contextWindow, responsesMode: editing.responsesMode, balanceUrl: editing.balanceUrl, clearBalanceUrl: editing.clearBalanceUrl, balanceUrlSet: editing.balanceUrlSet, models, default: models.includes(editing.default) ? editing.default : models[0], useApiKey: editing.useApiKey };
    setSaving(true);
    setError("");
    try {
      const view = await saveTauriProviderConfig(input);
      setConfigs(view.providers);
      setPresets(view.presets || []);
      setEditing(null);
      setMessage(t("settings.previewProvider.message.saved"));
      try { onSummaryChange(await tauriProviderSummary()); }
      catch { setError(t("settings.previewProvider.message.refreshFailed")); }
    } catch (err) {
      setError(tauriMessageFrom(err));
    } finally {
      setSaving(false);
    }
  };

  const discoverModels = async () => {
    if (!editing || !editingExisting || discoveringModels || saving) return;
    const provider = configs.find(candidate => candidate.name === editing.name);
    if (!provider) return;
    setDiscoveringModels(true);
    setError("");
    setMessage("");
    try {
      const result = await discoverTauriProviderModels(provider);
      const currentModels = modelText.split(/\r?\n|,/).map(value => value.trim()).filter(Boolean);
      const merged = [...new Set([...currentModels, ...result.models])];
      if (result.models.length === 0) {
        setMessage(t("settings.previewProvider.discoveryEmpty"));
      } else {
        setModelText(merged.join("\n"));
        setMessage(t("settings.previewProvider.discoverySuccess", { count: result.models.length }));
      }
    } catch (err) {
      setError(tauriMessageFrom(err));
    } finally {
      setDiscoveringModels(false);
    }
  };

  const remove = async (provider: TauriProviderConfig) => {
    if (saving || !provider.removable || !window.confirm(t("settings.previewProvider.confirmDelete", { name: provider.displayName || provider.name }))) return;
    setSaving(true);
    setError(""); setMessage("");
    try {
      const view = await deleteTauriProviderConfig(provider);
      setConfigs(view.providers);
      setPresets(view.presets || []);
      setEditing(null);
      setMessage(t("settings.previewProvider.message.deleted"));
      try { onSummaryChange(await tauriProviderSummary()); }
      catch { setError(t("settings.previewProvider.message.deleteRefreshFailed")); }
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSaving(false); }
  };

  const changePreset = async (preset: TauriProviderPreset, action: "add" | "reset") => {
    if (saving || (action === "add" ? preset.status !== "available" && preset.status !== "partial" : preset.status !== "installed_modified")) return;
    setSaving(true);
    setError(""); setMessage("");
    try {
      const view = action === "reset" ? await resetTauriProviderPreset(preset) : await installTauriProviderPreset(preset);
      setConfigs(view.providers);
      setPresets(view.presets || []);
      setSelectedPreset("");
      setResetArmed("");
      setMessage(action === "reset" ? t("settings.previewProvider.message.presetRestored", { name: preset.label }) : t("settings.previewProvider.message.presetAdded", { name: preset.label }));
      try { onSummaryChange(await tauriProviderSummary()); }
      catch { setError(t("settings.previewProvider.message.presetRefreshFailed")); }
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSaving(false); }
  };

  const visiblePresets = presets.filter(preset => `${preset.label} ${preset.description} ${preset.group} ${preset.routes.map(route => route.name).join(" ")}`.toLocaleLowerCase().includes(presetQuery.trim().toLocaleLowerCase()));
  const reviewedPreset = presets.find(preset => preset.id === selectedPreset);

  const presetStatus = (status: TauriProviderPreset["status"]): string => ({
    installed: t("settings.addProvider.addedBadge"),
    installed_modified: t("settings.addProvider.modifiedBadge"),
    partial: t("settings.addProvider.partialBadge"),
    name_conflict: t("settings.addProvider.nameConflictBadge"),
    available: t("settings.previewProvider.available"),
  })[status];

  return <div className="tauri-provider-editor">
    <div className="tauri-settings-model-header"><strong>{t("settings.previewProvider.presets")}</strong><span className="tauri-settings-hint">{t("settings.previewProvider.presetsHint")}</span></div>
    <input className="tauri-settings-input tauri-provider-preset-search" type="search" aria-label={t("settings.previewProvider.searchAria")} placeholder={t("settings.previewProvider.searchPlaceholder")} value={presetQuery} onChange={event => setPresetQuery(event.target.value)} />
    {loading ? <p className="tauri-settings-hint">{t("settings.previewProvider.loadingPresets")}</p> : <div className="tauri-provider-presets">{visiblePresets.map(preset => <button key={preset.id} type="button" className={`tauri-provider-preset${selectedPreset === preset.id ? " is-selected" : ""}`} onClick={() => { setSelectedPreset(preset.id); setResetArmed(""); }} aria-pressed={selectedPreset === preset.id} disabled={saving}><strong>{preset.label}{preset.recommended && <em>{t("settings.previewProvider.recommended")}</em>}</strong><small>{preset.group || t("settings.models.services")} · {t("settings.previewProvider.routeCount", { count: preset.routes.length })} · {presetStatus(preset.status)}</small></button>)}{visiblePresets.length === 0 && <p>{t("settings.previewProvider.emptyPresets")}</p>}</div>}
    {reviewedPreset && <div className="tauri-provider-preset-review"><h4>{t(reviewedPreset.status === "installed_modified" ? "settings.previewProvider.reviewRestore" : "settings.previewProvider.reviewAdd")}{reviewedPreset.label}</h4><p>{reviewedPreset.description}</p>{reviewedPreset.routes.map(route => <div key={route.name} className="tauri-provider-preset-route"><strong>{route.name}</strong><span>{route.kind} · {route.baseUrl}</span><small>{t("settings.previewProvider.modelCountDefault", { count: route.models.length, model: route.default || route.models[0] || t("common.none") })}</small></div>)}{reviewedPreset.status === "name_conflict" && <p role="alert">{t("settings.previewProvider.nameConflict")}</p>}{reviewedPreset.status === "installed_modified" && <p>{t("settings.previewProvider.restoreHint")}</p>}<div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={saving || reviewedPreset.status === "installed" || reviewedPreset.status === "name_conflict"} onClick={() => { if (reviewedPreset.status === "installed_modified") { if (resetArmed === reviewedPreset.id) void changePreset(reviewedPreset, "reset"); else setResetArmed(reviewedPreset.id); } else void changePreset(reviewedPreset, "add"); }}>{reviewedPreset.status === "partial" ? t("settings.addProvider.completeBundleAction") : reviewedPreset.status === "installed_modified" ? resetArmed === reviewedPreset.id ? t("settings.previewProvider.confirmResetPreset") : t("settings.previewProvider.resetPreset") : reviewedPreset.status === "installed" ? t("settings.addProvider.alreadyAddedAction") : t("settings.previewProvider.addPreset")}</button><button type="button" className="tauri-settings-button" onClick={() => { setSelectedPreset(""); setResetArmed(""); }} disabled={saving}>{t("common.collapse")}</button></div></div>}
    <div className="tauri-settings-model-header"><strong>{t("settings.previewProvider.serviceConfig")}</strong><button type="button" className="tauri-settings-button" onClick={() => startEdit()} disabled={saving}>{t("settings.previewProvider.addServiceAction")}</button></div>
    <p className="tauri-settings-hint">{t("settings.previewProvider.protocolHint")}</p>
    {loading ? <p className="tauri-settings-hint">{t("common.loading")}</p> : configs.map(provider => <div className="tauri-provider-editor-row" key={provider.name}>
      <span><strong>{provider.displayName || provider.name}</strong><small>{provider.kind} · {t("settings.previewProvider.modelCount", { count: provider.models.length })}</small></span>
      <button type="button" className="tauri-settings-button" onClick={() => startEdit(provider)} disabled={saving || !["openai", "anthropic", "responses"].includes(provider.kind)}>{t("common.edit")}</button>
      {provider.removable && <button type="button" className="tauri-settings-button" onClick={() => void remove(provider)} disabled={saving}>{t("common.delete")}</button>}
    </div>)}
    {editing && <div className="tauri-provider-editor-form">
      <h4>{t(editingExisting ? "settings.previewProvider.editService" : "settings.previewProvider.addService")}</h4>
      <label>{t("settings.previewProvider.identifier")}<input value={editing.name} disabled={editingExisting || saving} onChange={event => setEditing({ ...editing, name: event.target.value })} placeholder={t("settings.previewProvider.identifierPlaceholder")} /></label>
      <label>{t("settings.previewProvider.displayName")}<input value={editing.displayName} disabled={saving} onChange={event => setEditing({ ...editing, displayName: event.target.value })} placeholder={t("settings.previewProvider.displayNamePlaceholder")} /></label>
      <label>{t("settings.previewProvider.protocol")}<select value={editing.kind} disabled={editingExisting || saving} onChange={event => setEditing({ ...editing, kind: event.target.value })}><option value="openai">OpenAI Chat</option><option value="anthropic">Anthropic Messages</option><option value="responses">Responses</option></select></label>
      <label>{t("settings.previewProvider.endpoint")}<input value={editing.baseUrl} disabled={saving} onChange={event => setEditing({ ...editing, baseUrl: event.target.value })} placeholder={editingExisting ? t("settings.previewProvider.keepEndpoint") : "https://example.com/v1"} /></label>
      <label>{t("settings.providerBalanceUrl")}<input value={editing.balanceUrl} disabled={saving || editing.clearBalanceUrl} onChange={event => setEditing({ ...editing, balanceUrl: event.target.value })} placeholder={editing.balanceUrlSet && editingExisting ? t("settings.previewProvider.keepBalanceUrl") : t("settings.balanceUrlPlaceholder")} /></label>
      <p className="tauri-settings-hint">{editing.balanceUrlSet && editingExisting ? t("settings.previewProvider.balanceUrlKeepHint") : t("settings.previewProvider.balanceUrlHint")}</p>
      {editingExisting && editing.balanceUrlSet && <label className="tauri-provider-editor-checkbox"><input type="checkbox" checked={editing.clearBalanceUrl} disabled={saving} onChange={event => setEditing({ ...editing, clearBalanceUrl: event.target.checked, balanceUrl: "" })} />{t("settings.previewProvider.clearBalanceUrl")}</label>}
      <label>{t("settings.previewProvider.models")}<textarea rows={4} value={modelText} disabled={saving || discoveringModels} onChange={event => setModelText(event.target.value)} /></label>
      {editingExisting && <button type="button" className="tauri-settings-button" onClick={() => void discoverModels()} disabled={saving || discoveringModels}>{discoveringModels ? t("settings.previewProvider.discoveringModels") : t("settings.previewProvider.discoverModels")}</button>}
      {modelText.trim() && <label>{t("settings.previewProvider.defaultModel")}<select value={editing.default && modelText.split(/\r?\n|,/).map(value => value.trim()).includes(editing.default) ? editing.default : modelText.split(/\r?\n|,/).map(value => value.trim()).filter(Boolean)[0] ?? ""} disabled={saving} onChange={event => setEditing({ ...editing, default: event.target.value })}>{modelText.split(/\r?\n|,/).map(value => value.trim()).filter(Boolean).map(model => <option key={model} value={model}>{model}</option>)}</select></label>}
      <details className="tauri-provider-advanced"><summary>{t("settings.previewProvider.advanced")}</summary><div className="tauri-provider-advanced__body">
        <label>{t("settings.providerContextWindow")}<input type="number" min={0} max={10000000} value={editing.contextWindow} disabled={saving} onChange={event => setEditing({ ...editing, contextWindow: Number(event.target.value) || 0 })} /></label>
        <label className="tauri-provider-editor-checkbox"><input type="checkbox" checked={editing.noProxy} disabled={saving} onChange={event => setEditing({ ...editing, noProxy: event.target.checked })} />{t("settings.providerNoProxy")}</label><p className="tauri-settings-hint">{t("settings.providerNoProxyHint")}</p>
        {editing.kind === "responses" && <label>{t("settings.previewProvider.responsesMode")}<select value={editing.responsesMode || ""} disabled={saving} onChange={event => setEditing({ ...editing, responsesMode: event.target.value })}><option value="">{t("settings.previewProvider.responsesAuto")}</option><option value="stateless">{t("settings.previewProvider.responsesStateless")}</option><option value="stateful">{t("settings.previewProvider.responsesStateful")}</option></select></label>}
        <label>{t("settings.previewProvider.modelsEndpoint")}<input value={editing.modelsUrl} disabled={saving || editing.clearModelsUrl} placeholder={editing.modelsUrlSet && editingExisting ? t("settings.previewProvider.keepModelsEndpoint") : t("settings.previewProvider.modelsEndpointPlaceholder")} onChange={event => setEditing({ ...editing, modelsUrl: event.target.value })} /></label>
        {editingExisting && editing.modelsUrlSet && <label className="tauri-provider-editor-checkbox"><input type="checkbox" checked={editing.clearModelsUrl} disabled={saving} onChange={event => setEditing({ ...editing, clearModelsUrl: event.target.checked, modelsUrl: "" })} />{t("settings.previewProvider.clearModelsEndpoint")}</label>}
      </div></details>
      {!editingExisting && <label className="tauri-provider-editor-checkbox"><input type="checkbox" checked={editing.useApiKey} disabled={saving} onChange={event => setEditing({ ...editing, useApiKey: event.target.checked })} />{t("settings.previewProvider.requiresAPIKey")}</label>}
      <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" onClick={() => void save()} disabled={saving}>{saving ? t("settings.previewProvider.saving") : t("settings.previewProvider.saveService")}</button><button type="button" className="tauri-settings-button" onClick={() => setEditing(null)} disabled={saving}>{t("common.cancel")}</button></div>
    </div>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {message && <p className="tauri-settings-hint" role="status">{message}</p>}
  </div>;
}
