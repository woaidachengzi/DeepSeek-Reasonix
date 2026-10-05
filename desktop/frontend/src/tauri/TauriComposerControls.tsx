import { useEffect, useRef, useState } from "react";
import { Check, ChevronDown, Search, Settings, ShieldAlert, ShieldCheck, ShieldQuestion, Brain } from "lucide-react";
import { setTauriDesktopApproval, tauriDesktopPreferences, tauriMessageFrom, tauriSessionApprovalMode, tauriProviderSummary, type TauriProviderSummary, type TauriToolApprovalMode } from "../lib/tauriBridge";

const MODES = [
  { id: "ask", label: "需要审批", hint: "按权限规则请求确认，执行前由你审批。", icon: ShieldQuestion },
  { id: "auto", label: "自动审批", hint: "自动批准常规工具；显式询问、拒绝规则和沙箱仍生效。", icon: ShieldCheck },
  { id: "yolo", label: "完全权限 · Yolo", hint: "跳过常规工具询问；拒绝规则和沙箱仍生效。", icon: ShieldAlert },
] as const;

export function TauriComposerControls({ sessionId, workspaceRoot, effort = "auto", onEffortChange, model, providers, disabled, refreshToken, onModelChange, onBusyChange, onError, onOpenModels, onProvidersChange }: {
  sessionId?: string;
  workspaceRoot?: string;
  effort?: string;
  onEffortChange?: (effort: string) => Promise<boolean>;
  model: string;
  providers: TauriProviderSummary | null;
  disabled: boolean;
  refreshToken: boolean;
  onModelChange: (model: string) => Promise<unknown>;
  onBusyChange: (busy: boolean) => void;
  onError: (error: string) => void;
  onOpenModels: () => void;
  onProvidersChange?: (summary: TauriProviderSummary) => void;
}) {
  const [menu, setMenu] = useState<"permission" | "model" | "effort" | null>(null);
  const [mode, setMode] = useState<TauriToolApprovalMode | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [query, setQuery] = useState("");
  const [retry, setRetry] = useState(0);
  const [loadError, setLoadError] = useState("");
  const [catalog, setCatalog] = useState<TauriProviderSummary | null>(null);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [modelsError, setModelsError] = useState("");
  const [reasoningCatalog, setReasoningCatalog] = useState<TauriProviderSummary | null>(null);
  const [reasoningError, setReasoningError] = useState("");
  const [reasoningLoading, setReasoningLoading] = useState(true);
  useEffect(() => {
    let active = true;
    setReasoningLoading(true); setReasoningError(""); setReasoningCatalog(null);
    void tauriProviderSummary(workspaceRoot || undefined, workspaceRoot ? "project" : "global").then(summary => {
      if (active) setReasoningCatalog(summary);
    }).catch(error => { if (active) setReasoningError(tauriMessageFrom(error)); })
      .finally(() => { if (active) setReasoningLoading(false); });
    return () => { active = false; };
  }, [model, workspaceRoot, refreshToken, retry]);
  const root = useRef<HTMLDivElement>(null);
  const operation = useRef(false);
  const focusAfterSelection = useRef(false);
  useEffect(() => {
    if (!saving && !menu && focusAfterSelection.current) {
      focusAfterSelection.current = false;
      root.current?.querySelector<HTMLButtonElement>('[data-picker="effort"]')?.focus();
    }
  }, [saving, menu]);
  useEffect(() => {
    let active = true;
    setLoading(true); setMode(null); setLoadError("");
    const request = sessionId ? tauriSessionApprovalMode(sessionId) : tauriDesktopPreferences().then(value => value.defaultToolApprovalMode);
    void request.then(value => { if (active) setMode(value); }).catch(error => { if (active) setLoadError(tauriMessageFrom(error)); }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [sessionId, refreshToken, retry]);
  useEffect(() => {
    if (!menu) return;
    const close = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node)) setMenu(null); };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") { setMenu(null); root.current?.querySelector<HTMLButtonElement>(`[data-picker="${menu}"]`)?.focus(); }
    };
    document.addEventListener("pointerdown", close); document.addEventListener("keydown", escape);
    return () => { document.removeEventListener("pointerdown", close); document.removeEventListener("keydown", escape); };
  }, [menu]);
  const selectMode = async (next: TauriToolApprovalMode) => {
    if (disabled || operation.current || loading) return;
    if (next === mode) { setMenu(null); return; }
    operation.current = true; setSaving(true); onBusyChange(true);
    try {
      const updated = sessionId ? await tauriSessionApprovalMode(sessionId, next) : (await setTauriDesktopApproval(next)).defaultToolApprovalMode;
      setMode(updated); setMenu(null);
    } catch (error) { onError(tauriMessageFrom(error)); }
    finally { operation.current = false; setSaving(false); onBusyChange(false); }
  };
  useEffect(() => {
    if (menu !== "model") return;
    let active = true;
    setModelsLoading(true); setModelsError("");
    void tauriProviderSummary().then(summary => {
      if (!active) return;
      setCatalog(summary); onProvidersChange?.(summary);
    }).catch(error => { if (active) setModelsError(tauriMessageFrom(error)); })
      .finally(() => { if (active) setModelsLoading(false); });
    return () => { active = false; };
  }, [menu, onProvidersChange]);
  const selected = MODES.find(item => item.id === mode);
  const Icon = selected?.icon ?? ShieldQuestion;
  const modelLabel = model.split("/").slice(1).join("/") || model || "选择模型";
  const listedProviders = (catalog ?? providers)?.providers ?? [];
  const filter = query.trim().toLowerCase();
  const providerName = model.split("/")[0];
  const capability = reasoningCatalog?.providers.find(item => item.name === providerName)?.reasoning?.find(item => item.model === modelLabel);
  const levels = capability?.levels ?? [];
  const label = (value: string) => ({auto:"自动",none:"不思考",disabled:"关闭思考",enabled:"开启思考",adaptive:"自适应",low:"低",medium:"中",high:"高",xhigh:"很高",max:"最高",ultra:"极高"})[value] ?? value;
  const selectEffort = async (next: string) => {
    if (!onEffortChange || disabled || operation.current || reasoningLoading || !levels.includes(next)) return;
    operation.current = true; setSaving(true);
    try { if (await onEffortChange(next)) { focusAfterSelection.current = true; setMenu(null); } }
    catch (error) { onError(tauriMessageFrom(error)); }
    finally { operation.current = false; setSaving(false); }
  };
  return <div className="tauri-composer-controls" ref={root}>
    <div className="tauri-composer-controls__permission">
      <button type="button" data-picker="permission" className="tauri-composer-picker is-permission" aria-label="审批权限" aria-expanded={menu === "permission"} title={sessionId ? "当前对话的工具审批模式" : "新对话默认工具审批模式"} disabled={disabled || saving} onClick={() => setMenu(menu === "permission" ? null : "permission")}><Icon size={18} /><span>{loading ? "读取权限…" : selected?.label ?? "权限不可用"}</span><ChevronDown size={14} /></button>
      {menu === "permission" && <div className="tauri-composer-popover is-permission" aria-label="审批模式">
        <small>{sessionId ? "应用于当前对话" : "用于新对话"}</small>
        {MODES.map(item => <button type="button" key={item.id} aria-pressed={mode === item.id} disabled={disabled || saving || loading || !mode} onClick={() => void selectMode(item.id)}><item.icon size={19} /><span>{item.label}<small>{item.hint}</small></span>{mode === item.id && <Check size={17} />}</button>)}
        {loadError && <><p role="alert">{loadError}</p><button type="button" disabled={loading || saving || disabled} onClick={() => setRetry(value => value + 1)}>重新读取权限</button></>}
      </div>}
    </div>
    <div className="tauri-composer-controls__selection"><div className="tauri-composer-controls__model">
      <button type="button" data-picker="model" className="tauri-composer-picker is-model" aria-label="选择模型" aria-expanded={menu === "model"} disabled={disabled || saving} title={sessionId ? `当前对话：${model}` : `新对话默认模型：${model}（工作区配置可能覆盖）`} onClick={() => { setMenu(menu === "model" ? null : "model"); setQuery(""); }}><span>{modelLabel}</span><ChevronDown size={14} /></button>
      {menu === "model" && <div className="tauri-composer-popover is-model" aria-label="模型列表">
        <small>{sessionId ? "切换当前对话模型" : "新对话默认模型 · 工作区配置可能覆盖"}</small>
        <label className="tauri-composer-model-search"><Search size={15} /><input aria-label="搜索模型" placeholder="搜索模型或提供商" value={query} onChange={event => setQuery(event.target.value)} /></label>
        <div className="tauri-composer-model-list">{modelsLoading ? <p role="status">正在刷新模型…</p> : listedProviders.map(provider => {
          const models = provider.models.filter(id => `${provider.displayName || provider.name} ${id}`.toLowerCase().includes(filter));
          return models.length > 0 && <section key={provider.name}><h4>{provider.displayName || provider.name}</h4>{models.map(id => {
            const ref = `${provider.name}/${id}`;
            return <button type="button" key={ref} aria-pressed={ref === model} disabled={disabled || saving || modelsLoading || Boolean(modelsError) || !provider.configured} title={provider.configured ? undefined : "请到管理模型服务填写并保存 API Key"} onClick={() => { if (operation.current) return; operation.current = true; void onModelChange(ref).then(result => { if (result !== false) setMenu(null); }).catch(error => onError(tauriMessageFrom(error))).finally(() => { operation.current = false; }); }}><span>{id}{!provider.configured && <small>请填写 API Key</small>}</span>{ref === model && <Check size={16} />}</button>;
          })}</section>;
        })}{!modelsLoading && !listedProviders.some(provider => provider.models.some(id => `${provider.displayName || provider.name} ${id}`.toLowerCase().includes(filter))) && <p>{listedProviders.length ? "没有匹配的模型" : "尚无模型，请先添加模型服务。"}</p>}</div>
        {modelsError && <p role="alert">模型刷新失败：{modelsError}。请关闭列表后重新打开。</p>}
        <button className="tauri-composer-model-manage" type="button" disabled={disabled || saving} onClick={() => { setMenu(null); onOpenModels(); }}><Settings size={15} />管理模型服务</button>
      </div>}
    </div>
    {onEffortChange && <div className="tauri-composer-controls__effort">
      <button type="button" data-picker="effort" className="tauri-composer-picker is-effort" aria-label="思考强度" aria-expanded={menu === "effort"} disabled={disabled || saving || reasoningLoading} title="选择当前模型的思考强度" onClick={() => setMenu(menu === "effort" ? null : "effort")}><Brain size={16}/><span>{reasoningLoading ? "读取…" : levels.length ? label(effort) : reasoningError ? "读取失败" : "模型默认"}</span><ChevronDown size={14}/></button>
      {menu === "effort" && <div className="tauri-composer-popover is-effort" aria-label="思考强度选项">
        <small>{sessionId ? "应用于当前对话的后续消息" : "应用于这次新对话"}</small>
        {levels.map(level => <button type="button" key={level} aria-pressed={effort === level} disabled={disabled || saving} onClick={() => void selectEffort(level)}><span>{label(level)}<small>{level === "auto" ? `使用模型默认设置${capability?.default && capability.default !== "auto" ? `（${label(capability.default)}）` : ""}` : ["none","disabled"].includes(level) ? "关闭推理，更快响应" : ["enabled","adaptive"].includes(level) ? "由模型控制思考过程" : level === "low" ? "较快响应，适合简单任务" : level === "max" ? "更多思考时间，用量可能增加" : "更充分思考，适合复杂任务"}</small></span>{effort === level && <Check size={16}/>}</button>)}
        {!reasoningError && !levels.length && <p>当前模型未声明可调节的思考等级，将使用模型默认设置。</p>}
        {reasoningError && <><p role="alert">无法读取思考选项：{reasoningError}</p><button type="button" onClick={() => setRetry(value => value + 1)}>重新读取</button></>}
      </div>}
    </div>}
    </div>
  </div>;
}
