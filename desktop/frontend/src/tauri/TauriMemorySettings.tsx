import { useEffect, useState } from "react";
import { changeTauriMemorySettings, tauriMemorySettings, tauriMessageFrom, type TauriMemoryChange, type TauriMemoryDoc, type TauriMemorySettings } from "../lib/tauriBridge";

const scopeLabel: Record<string, string> = { user: "全局", project: "项目", local: "当前工作区", ancestor: "上级目录" };

export function TauriMemorySettings({ workspaceRoot = "", currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  workspaceRoot?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const [view, setView] = useState<TauriMemorySettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [selectedPath, setSelectedPath] = useState("");
  const [editor, setEditor] = useState("");
  const [note, setNote] = useState("");
  const [scope, setScope] = useState<"user" | "project" | "local">("project");
  const [showArchives, setShowArchives] = useState(false);
  const [pendingApply, setPendingApply] = useState(false);

  const pick = (doc: TauriMemoryDoc) => { setSelectedPath(doc.path); setEditor(doc.body); setError(""); };
  const adopt = (next: TauriMemorySettings) => {
    setView(next);
    const doc = next.docs.find(item => item.path === selectedPath) ?? next.docs[0];
    if (doc) pick(doc);
  };
  useEffect(() => {
    let active = true;
    setLoading(true); setError(""); setView(null); setSelectedPath("");
    if (!workspaceRoot) { setLoading(false); return; }
    void tauriMemorySettings(workspaceRoot).then(next => { if (active) { setView(next); const doc = next.docs[0]; if (doc) { setSelectedPath(doc.path); setEditor(doc.body); } } })
      .catch(err => { if (active) setError(tauriMessageFrom(err)); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [workspaceRoot]);
  const selected = view?.docs.find(doc => doc.path === selectedPath);
  const dirty = Boolean(selected && editor !== selected.body);
  const reload = async () => {
    if (dirty && !window.confirm("放弃尚未保存的记忆文档修改？")) return;
    setLoading(true); setError("");
    try { adopt(await tauriMemorySettings(workspaceRoot)); setNotice("已重新读取记忆。"); }
    catch (err) { setError(tauriMessageFrom(err)); }
    finally { setLoading(false); }
  };
  const mutate = async (change: Omit<TauriMemoryChange, "workspaceRoot">, success: string) => {
    if (busy) return;
    setBusy(true); setError(""); setNotice("");
    try {
      const next = await changeTauriMemorySettings({ workspaceRoot, ...change });
      adopt(next); setPendingApply(true); setNotice(success);
      if (change.action === "quick_add") setNote("");
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setBusy(false); }
  };
  const base = { path: "", revision: "", body: "", scope: "", factId: "", factRevision: 0 };
  const apply = async () => {
    if (!onApplyToCurrentSession || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    setBusy(true); setError("");
    try {
      if (await onApplyToCurrentSession()) { setPendingApply(false); setNotice("当前会话已重新载入记忆。"); }
      else setError("当前会话未能更新，请在运行诊断中重试。");
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setBusy(false); }
  };

  if (!workspaceRoot) return <div className="tauri-settings-section"><p role="status">请先打开项目会话，再管理记忆。</p></div>;
  return <div className="tauri-settings-section tauri-memory-settings">
    <p>这些说明文档与事实会由 Reasonix 核心在新会话中读取。修改后，运行中的会话需要重新载入。</p>
    <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={loading || busy} onClick={() => void reload()}>重新读取</button>{pendingApply && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void apply()}>应用到当前会话</button>}</div>
    {loading && <div className="tauri-settings-loading">加载中…</div>}
    {view && <>
      <h3>说明文档</h3><p>编辑全局或项目指令。保存会检查磁盘版本，避免覆盖其他窗口的修改。</p>
      <div className="tauri-memory-doc-list">{view.docs.map(doc => <button key={doc.path} type="button" className={`tauri-memory-doc${selectedPath === doc.path ? " is-active" : ""}`} title={doc.path} onClick={() => { if (!dirty || window.confirm("放弃尚未保存的修改？")) pick(doc); }}><strong>{scopeLabel[doc.scope] ?? doc.scope}</strong><small>{doc.path}</small></button>)}</div>
      {selected && <><div className="tauri-memory-path">{selected.path}</div><textarea aria-label="记忆文档" className="tauri-memory-editor" value={editor} disabled={busy} onChange={event => { setEditor(event.target.value); setError(""); }} /><div className="tauri-settings-actions"><span role="status">{dirty ? "有未保存修改" : "已保存"}</span><button type="button" className="tauri-settings-button" disabled={!dirty || busy} onClick={() => setEditor(selected.body)}>放弃修改</button><button type="button" className="tauri-settings-button" disabled={!dirty || busy} onClick={() => void mutate({ ...base, action: "save_doc", path: selected.path, revision: selected.revision, body: editor }, "文档已保存，新会话会读取更新。")}>保存文档</button></div></>}
      <h3>快速追加</h3><p>把一条简短说明追加到所选范围的指令文档。</p>
      <div className="tauri-memory-note-row"><select aria-label="记忆范围" value={scope} disabled={busy} onChange={event => setScope(event.target.value as typeof scope)}><option value="user">全局</option><option value="project">项目</option><option value="local">当前工作区</option></select><input aria-label="记忆笔记" value={note} maxLength={4096} disabled={busy} placeholder="写下要记住的约定…" onChange={event => setNote(event.target.value)} /><button type="button" className="tauri-settings-button" disabled={busy || !note.trim() || dirty} onClick={() => void mutate({ ...base, action: "quick_add", scope, body: note }, "笔记已追加，新会话会读取更新。")}>追加</button></div>
      <h3>已保存的事实</h3><p>事实由会话提取并保存到当前 Preview 资料中。归档后可恢复。</p>
      {view.facts.length === 0 ? <p>暂无已保存的事实。</p> : <div className="tauri-memory-facts">{view.facts.map(fact => <article key={fact.id} className="tauri-memory-fact"><div><strong>{fact.title || fact.name || fact.id}</strong><small>{scopeLabel[fact.scope] ?? fact.scope} · {fact.freshness}</small></div><p>{fact.description || fact.body}</p><button type="button" className="tauri-settings-button" disabled={busy || dirty} onClick={() => { if (window.confirm(`归档“${fact.title || fact.name || fact.id}”？`)) void mutate({ ...base, action: "archive", factId: fact.id, factRevision: fact.revision }, "事实已归档。"); }}>归档</button></article>)}</div>}
      <button type="button" className="tauri-settings-button" onClick={() => setShowArchives(value => !value)}>{showArchives ? "收起归档" : `查看归档（${view.archives.length}）`}</button>
      {showArchives && (view.archives.length ? <div className="tauri-memory-facts">{view.archives.map(item => <article key={item.path} className="tauri-memory-fact"><div><strong>{item.title || item.name || item.id}</strong><small>{item.archivedAt || item.scope}</small></div><p>{item.description || item.body}</p><button type="button" className="tauri-settings-button" disabled={busy || dirty} onClick={() => void mutate({ ...base, action: "restore", path: item.path }, "事实已恢复。")}>恢复</button></article>)}</div> : <p>暂无归档。</p>)}
      {view.diagnostics.length > 0 && <><h3>读取提示</h3>{view.diagnostics.map((item, index) => <p key={index}>{item}</p>)}</>}
    </>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
