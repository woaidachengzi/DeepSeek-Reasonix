import { useEffect, useRef, useState, type FormEvent } from "react";
import { changeTauriPermissionSettings, tauriMessageFrom, tauriPermissionSettings, type TauriPermissionList, type TauriPermissionMode, type TauriPermissionSettings } from "../lib/tauriBridge";

const RULE_LISTS: { id: TauriPermissionList; title: string; hint: string }[] = [
  { id: "deny", title: "拒绝", hint: "优先级最高，匹配后直接拒绝。" },
  { id: "ask", title: "询问", hint: "匹配后请求你确认。" },
  { id: "allow", title: "允许", hint: "匹配后直接允许。" },
];

export function TauriPermissionsSettings({ currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const [view, setView] = useState<TauriPermissionSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pendingApply, setPendingApply] = useState(false);
  const [drafts, setDrafts] = useState<Record<TauriPermissionList, string>>({ allow: "", ask: "", deny: "" });
  const busyRef = useRef(false);

  const reload = () => {
    setLoading(true);
    setError("");
    void tauriPermissionSettings().then(setView).catch(err => setError(tauriMessageFrom(err))).finally(() => setLoading(false));
  };
  useEffect(reload, []);

  const change = async (action: "mode" | "add" | "remove", list?: TauriPermissionList, rule?: string, mode?: TauriPermissionMode) => {
    if (busyRef.current) return;
    const normalizedRule = rule?.trim() ?? "";
    if (action !== "mode" && !normalizedRule) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const next = action === "mode"
        ? await changeTauriPermissionSettings({ action, mode: mode! })
        : await changeTauriPermissionSettings({ action, list: list!, rule: normalizedRule });
      setView(next);
      setPendingApply(true);
      setNotice("已保存到 Preview 配置；新会话会读取更新后的规则。");
      if (action === "add" && list) setDrafts(previous => ({ ...previous, [list]: "" }));
    } catch (err) {
      setError(tauriMessageFrom(err));
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
  };

  const submitRule = (event: FormEvent<HTMLFormElement>, list: TauriPermissionList) => {
    event.preventDefault();
    void change("add", list, drafts[list]);
  };

  const applyCurrent = async () => {
    if (!onApplyToCurrentSession || busyRef.current || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      const applied = await onApplyToCurrentSession();
      if (applied) {
        setPendingApply(false);
        setNotice("当前会话已重新载入配置。");
      } else setError("当前会话未能更新，请在运行诊断中重试。");
    } catch { setError("当前会话未能更新，请在运行诊断中重试。"); }
    finally { busyRef.current = false; setBusy(false); }
  };

  return <div className="tauri-settings-section tauri-permissions-settings">
    <h3>默认写入决策</h3>
    <p>没有规则匹配时，写入类工具使用下方决策；只读工具默认允许。工作区配置可能覆盖全局规则。</p>
    {loading ? <div className="tauri-settings-loading">加载中…</div> : view ? <>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">权限模式<small>拒绝规则优先于询问规则，询问规则优先于允许规则。</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label="默认写入决策">{(["ask", "allow", "deny"] as const).map(mode => <button key={mode} type="button" role="radio" aria-checked={view.mode === mode} className={`tauri-settings-radio${view.mode === mode ? " is-active" : ""}`} disabled={busy} onClick={() => void change("mode", undefined, undefined, mode)}>{mode === "ask" ? "询问" : mode === "allow" ? "允许" : "拒绝"}</button>)}</div></div>
      <h3>权限规则</h3>
      <p>输入工具名或 <code>ToolName(glob)</code>，例如 <code>Bash(git status:*)</code>。按“拒绝 → 询问 → 允许”的顺序判定。</p>
      <div className="tauri-permissions-rules">{RULE_LISTS.map(list => <section key={list.id} className="tauri-permissions-rule-card" aria-label={`${list.title}规则`}>
        <h4>{list.title}</h4><p>{list.hint}</p>
        <div className="tauri-permissions-rule-list">{view[list.id].length ? view[list.id].map(rule => <div className="tauri-permissions-rule" key={rule}><code>{rule}</code><button type="button" aria-label={`移除${list.title}规则 ${rule}`} disabled={busy} onClick={() => void change("remove", list.id, rule)}>移除</button></div>) : <span>暂无规则</span>}</div>
        <form onSubmit={event => submitRule(event, list.id)}><input aria-label={`新增${list.title}规则`} value={drafts[list.id]} maxLength={2048} disabled={busy} placeholder="ToolName 或 ToolName(glob)" onChange={event => setDrafts(previous => ({ ...previous, [list.id]: event.target.value }))} /><button type="submit" className="tauri-settings-button" disabled={busy || !drafts[list.id].trim()}>添加</button></form>
      </section>)}</div>
      {pendingApply && currentSessionState && onApplyToCurrentSession && <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>应用到当前会话</button></div>}
    </> : <button type="button" className="tauri-settings-button" onClick={reload}>重试读取</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
