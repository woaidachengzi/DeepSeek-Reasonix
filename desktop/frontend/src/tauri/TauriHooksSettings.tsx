import { useEffect, useRef, useState } from "react";
import { changeTauriHooksSettings, tauriHooksSettings, tauriMessageFrom, type TauriHooksSettings as HooksView } from "../lib/tauriBridge";

type HookScope = "global" | "project";

function parseHooksEditor(text: string, events: string[]): Record<string, unknown> {
  const parsed: unknown = JSON.parse(text);
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("请输入包含 hooks 对象的 JSON。");
  const root = parsed as Record<string, unknown>;
  const hooks = root.hooks;
  if (!hooks || typeof hooks !== "object" || Array.isArray(hooks)) throw new Error("hooks 必须是按事件分组的对象。");
  const valid = new Set(events);
  for (const [event, entries] of Object.entries(hooks)) {
    if (!valid.has(event)) throw new Error(`未知的 Hook 事件：${event}`);
    if (!Array.isArray(entries)) throw new Error(`${event} 必须是数组。`);
    for (const entry of entries) {
      if (!entry || typeof entry !== "object" || Array.isArray(entry) || typeof (entry as Record<string, unknown>).command !== "string" || !(entry as Record<string, unknown>).command?.toString().trim()) {
        throw new Error(`${event} 中的每项都需要非空 command。`);
      }
    }
  }
  return hooks as Record<string, unknown>;
}

function formatHooks(hooks: Record<string, unknown>): string {
  return JSON.stringify({ hooks }, null, 2);
}

export function TauriHooksSettings({ workspaceRoot = "", currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  workspaceRoot?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const [scope, setScope] = useState<HookScope>("global");
  const [view, setView] = useState<HooksView | null>(null);
  const [text, setText] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pendingApply, setPendingApply] = useState(false);
  const busyRef = useRef(false);

  const reload = async (nextScope: HookScope) => {
    setLoading(true);
    setError("");
    try {
      const next = await tauriHooksSettings(nextScope, workspaceRoot);
      setView(next);
      setText(formatHooks(next.hooks));
      setNotice("");
    } catch (err) { setView(null); setError(tauriMessageFrom(err)); }
    finally { setLoading(false); }
  };

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    void tauriHooksSettings(scope, workspaceRoot).then(next => {
      if (!active) return;
      setView(next);
      setText(formatHooks(next.hooks));
    }).catch(err => { if (active) { setView(null); setError(tauriMessageFrom(err)); } })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [scope, workspaceRoot]);

  const dirty = Boolean(view && text !== formatHooks(view.hooks));
  const format = () => {
    if (!view) return;
    try { setText(formatHooks(parseHooksEditor(text, view.events))); setError(""); }
    catch (err) { setError(tauriMessageFrom(err)); }
  };
  const save = async () => {
    if (!view || busyRef.current) return;
    let hooks: Record<string, unknown>;
    try { hooks = parseHooksEditor(text, view.events); }
    catch (err) { setError(tauriMessageFrom(err)); return; }
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const next = await changeTauriHooksSettings({ scope, workspaceRoot, revision: view.revision, hooks });
      setView(next);
      setText(formatHooks(next.hooks));
      setPendingApply(true);
      setNotice("Hooks 已保存；新会话会读取这些命令。仅添加你信任的命令。");
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { busyRef.current = false; setBusy(false); }
  };
  const applyCurrent = async () => {
    if (!onApplyToCurrentSession || currentSessionState !== "idle" || currentSessionHasAttachments || busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      if (await onApplyToCurrentSession()) {
        setPendingApply(false);
        setNotice("当前会话已重新载入 Hooks 设置。");
      } else setError("当前会话未能更新，请在运行诊断中重试。");
    } catch { setError("当前会话未能更新，请在运行诊断中重试。"); }
    finally { busyRef.current = false; setBusy(false); }
  };

  return <div className="tauri-settings-section tauri-hooks-settings">
    <h3>配置范围</h3><p>Hooks 会在相应事件触发时运行本地命令。项目 Hooks 在项目会话中先于全局 Hooks 运行。</p>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">范围<small>项目范围使用当前选中的工作区。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="Hooks 范围">{(["global", "project"] as const).map(value => <button key={value} type="button" role="radio" aria-checked={scope === value} className={`tauri-settings-radio${scope === value ? " is-active" : ""}`} disabled={busy || (value === "project" && !workspaceRoot)} onClick={() => { if (!dirty || window.confirm("放弃尚未保存的 Hooks 编辑？")) setScope(value); }}>{value === "global" ? "全局" : "当前项目"}</button>)}</div></div>
    {scope === "project" && !workspaceRoot && <p role="status">请先打开项目会话，再编辑项目 Hooks。</p>}
    {loading ? <div className="tauri-settings-loading">加载中…</div> : view ? <>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">配置文件<small>保存后由 Reasonix 核心在新会话中加载。</small></span><code className="tauri-hooks-path">{view.path}</code></div>
      <h3>Hooks JSON</h3><p>按事件填写命令数组。支持 command、match、description、timeout（毫秒）与 cwd；现有条目的其他字段会保留。</p>
      <div className="tauri-hooks-toolbar"><button type="button" className="tauri-settings-button" disabled={busy} onClick={format}>格式化并检查</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void navigator.clipboard?.writeText(text).then(() => setNotice("已复制 JSON。"), () => setError("复制失败。"))}>复制</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void navigator.clipboard?.readText().then(value => { setText(value); setError(""); }, () => setError("粘贴失败。"))}>粘贴</button></div>
      <textarea className="tauri-hooks-editor" aria-label="Hooks JSON" spellCheck={false} value={text} disabled={busy} onChange={event => { setText(event.target.value); setError(""); setNotice(""); }} />
      <p>可用事件：{view.events.join("、")}</p>
      <div className="tauri-settings-actions"><span role="status">{dirty ? "有未保存修改" : "已保存"}</span><button type="button" className="tauri-settings-button" disabled={busy || !dirty} onClick={() => { setText(formatHooks(view.hooks)); setError(""); }}>放弃修改</button><button type="button" className="tauri-settings-button" disabled={busy || !dirty} onClick={() => void save()}>保存 Hooks</button><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => { if (!dirty || window.confirm("放弃尚未保存的 Hooks 编辑？")) void reload(scope); }}>重新读取</button>{pendingApply && currentSessionState && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>应用到当前会话</button>}</div>
    </> : <button type="button" className="tauri-settings-button" onClick={() => void reload(scope)}>重试读取</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
