import { useEffect, useState } from "react";
import { acceptTauriMemorySuggestion, changeTauriMemorySettings, tauriMemorySettings, tauriMemorySuggestions, tauriMessageFrom, type TauriMemoryChange, type TauriMemoryDoc, type TauriMemoryFact, type TauriMemorySettings, type TauriMemorySuggestions } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";

export function TauriMemorySettings({ workspaceRoot = "", currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  workspaceRoot?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const t = useT();
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
  const [expandedFact, setExpandedFact] = useState("");
  const [revisionHistory, setRevisionHistory] = useState<Record<string, TauriMemoryFact[]>>({});
  const [editingFact, setEditingFact] = useState("");
  const [factDraft, setFactDraft] = useState({ name: "", title: "", description: "", type: "project", body: "" });
  const [suggestions, setSuggestions] = useState<TauriMemorySuggestions | null>(null);
  const [suggestionsBusy, setSuggestionsBusy] = useState(false);
  const [acceptingSuggestion, setAcceptingSuggestion] = useState("");
  const base = { path: "", revision: "", body: "", scope: "", factId: "", factRevision: 0, historyRevision: 0 };

  const pick = (doc: TauriMemoryDoc) => { setSelectedPath(doc.path); setEditor(doc.body); setError(""); };
  const adopt = (next: TauriMemorySettings) => {
    setView(next);
    const doc = next.docs.find(item => item.path === selectedPath) ?? next.docs[0];
    if (doc) pick(doc);
  };
  useEffect(() => {
    let active = true;
    setLoading(true); setError(""); setView(null); setSelectedPath("");
    setExpandedFact(""); setRevisionHistory({});
    if (!workspaceRoot) { setLoading(false); return; }
    void tauriMemorySettings(workspaceRoot).then(next => { if (active) { setView(next); const doc = next.docs[0]; if (doc) { setSelectedPath(doc.path); setEditor(doc.body); } } })
      .catch(err => { if (active) setError(tauriMessageFrom(err)); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [workspaceRoot]);
  const selected = view?.docs.find(doc => doc.path === selectedPath);
  const dirty = Boolean(selected && editor !== selected.body);
  const toggleRevisionHistory = async (fact: TauriMemoryFact) => {
    if (expandedFact === fact.id) { setExpandedFact(""); return; }
    setExpandedFact(fact.id);
    if (revisionHistory[fact.id] || busy) return;
    setBusy(true); setError("");
    try {
      const next = await changeTauriMemorySettings({ workspaceRoot, ...base, action: "load_revisions", factId: fact.id, factRevision: fact.revision });
      setRevisionHistory(current => ({ ...current, [fact.id]: next.revisions ?? [] }));
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setBusy(false); }
  };
  const reload = async () => {
    if (dirty && !window.confirm(t("settings.memory.confirmReload"))) return;
    setLoading(true); setError("");
    try { adopt(await tauriMemorySettings(workspaceRoot)); setNotice(t("settings.memory.reloaded")); }
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
      if (change.action === "save_fact") setEditingFact("");
      if (change.action === "restore_revision") {
        setExpandedFact("");
        setRevisionHistory(current => { const updated = { ...current }; delete updated[change.factId]; return updated; });
      }
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setBusy(false); }
  };
  const apply = async () => {
    if (!onApplyToCurrentSession || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    setBusy(true); setError("");
    try {
      if (await onApplyToCurrentSession()) { setPendingApply(false); setNotice(t("settings.memory.applied")); }
      else setError(t("settings.memory.applyFailed"));
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setBusy(false); }
  };
  const scanSuggestions = async () => {
    setSuggestionsBusy(true); setError("");
    try { setSuggestions(await tauriMemorySuggestions(workspaceRoot)); }
    catch (err) { setError(tauriMessageFrom(err)); }
    finally { setSuggestionsBusy(false); }
  };
  const acceptSuggestion = async (kind: "memory" | "skill", id: string) => {
    if (acceptingSuggestion) return;
    setAcceptingSuggestion(id); setError(""); setNotice("");
    try {
      await acceptTauriMemorySuggestion(workspaceRoot, kind, id);
      setView(await tauriMemorySettings(workspaceRoot));
      setSuggestions(await tauriMemorySuggestions(workspaceRoot));
      setPendingApply(true);
      setNotice(t(kind === "memory" ? "settings.memory.suggestionMemoryAccepted" : "settings.memory.suggestionSkillAccepted"));
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setAcceptingSuggestion(""); }
  };

  if (!workspaceRoot) return <div className="tauri-settings-section"><p role="status">{t("settings.memory.projectRequired")}</p></div>;
  return <div className="tauri-settings-section tauri-memory-settings">
    <p>{t("settings.memory.description")}</p>
    <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={loading || busy} onClick={() => void reload()}>{t("settings.memory.reload")}</button>{pendingApply && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void apply()}>{t("settings.memory.applyCurrent")}</button>}</div>
    <h3>{t("settings.memory.suggestionsTitle")}</h3><p>{t("settings.memory.suggestionsDescription")}</p>
    <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={suggestionsBusy || acceptingSuggestion !== ""} onClick={() => void scanSuggestions()}>{suggestionsBusy ? t("settings.memory.suggestionsScanning") : t("settings.memory.suggestionsScan")}</button>{suggestions && <span role="status">{t("settings.memory.suggestionsCount", { memories: suggestions.memories.length, skills: suggestions.skills.length })}</span>}</div>
    {suggestions && <div className="tauri-memory-facts">{suggestions.memories.map(item => <article className="tauri-memory-fact" key={item.id}><div><strong>{item.title || item.name}</strong><small>{t("settings.memory.suggestionMemory")}{item.reason ? ` · ${item.reason}` : ""}</small></div><p>{item.description}</p><p>{item.body}</p>{item.evidence.map((line, index) => <small key={index}>{line}</small>)}<button type="button" className="tauri-settings-button" disabled={acceptingSuggestion !== ""} onClick={() => void acceptSuggestion("memory", item.id)}>{t("settings.memory.suggestionAcceptMemory")}</button></article>)}{suggestions.skills.map(item => <article className="tauri-memory-fact" key={item.id}><div><strong>{item.name}</strong><small>{t("settings.memory.suggestionSkill")} · {item.scope}</small></div><p>{item.description}</p><p>{item.reason}</p>{item.evidence.map((line, index) => <small key={index}>{line}</small>)}<button type="button" className="tauri-settings-button" disabled={acceptingSuggestion !== ""} onClick={() => void acceptSuggestion("skill", item.id)}>{t("settings.memory.suggestionAcceptSkill")}</button></article>)}{suggestions.memories.length === 0 && suggestions.skills.length === 0 && <p>{t("settings.memory.suggestionsEmpty")}</p>}</div>}
    {loading && <div className="tauri-settings-loading">{t("settings.memory.loading")}</div>}
    {view && <>
      <h3>{t("settings.memory.documentsTitle")}</h3><p>{t("settings.memory.documentsDescription")}</p>
      <div className="tauri-memory-doc-list">{view.docs.map(doc => <button key={doc.path} type="button" className={`tauri-memory-doc${selectedPath === doc.path ? " is-active" : ""}`} title={doc.path} onClick={() => { if (!dirty || window.confirm(t("settings.memory.confirmDiscard"))) pick(doc); }}><strong>{t(`settings.memory.scope.${doc.scope}` as Parameters<typeof t>[0])}</strong><small>{doc.path}</small></button>)}</div>
      {selected && <><div className="tauri-memory-path">{selected.path}</div><textarea aria-label={t("settings.memory.documentAria")} className="tauri-memory-editor" value={editor} disabled={busy} onChange={event => { setEditor(event.target.value); setError(""); }} /><div className="tauri-settings-actions"><span role="status">{dirty ? t("settings.memory.unsaved") : t("settings.memory.saved")}</span><button type="button" className="tauri-settings-button" disabled={!dirty || busy} onClick={() => setEditor(selected.body)}>{t("settings.memory.discardChanges")}</button><button type="button" className="tauri-settings-button" disabled={!dirty || busy} onClick={() => void mutate({ ...base, action: "save_doc", path: selected.path, revision: selected.revision, body: editor }, t("settings.memory.documentSaved"))}>{t("settings.memory.saveDocument")}</button></div></>}
      <h3>{t("settings.memory.quickAddTitle")}</h3><p>{t("settings.memory.quickAddDescription")}</p>
      <div className="tauri-memory-note-row"><select aria-label={t("settings.memory.scopeAria")} value={scope} disabled={busy} onChange={event => setScope(event.target.value as typeof scope)}><option value="user">{t("settings.memory.scope.user")}</option><option value="project">{t("settings.memory.scope.project")}</option><option value="local">{t("settings.memory.scope.local")}</option></select><input aria-label={t("settings.memory.noteAria")} value={note} maxLength={4096} disabled={busy} placeholder={t("settings.memory.notePlaceholder")} onChange={event => setNote(event.target.value)} /><button type="button" className="tauri-settings-button" disabled={busy || !note.trim() || dirty} onClick={() => void mutate({ ...base, action: "quick_add", scope, body: note }, t("settings.memory.noteAdded"))}>{t("settings.memory.append")}</button></div>
      <h3>{t("settings.memory.factsTitle")}</h3><p>{t("settings.memory.factsDescription")}</p>
      {view.facts.length === 0 ? <p>{t("settings.memory.noFacts")}</p> : <div className="tauri-memory-facts">{view.facts.map(fact => <article key={fact.id} className="tauri-memory-fact"><div><strong>{fact.title || fact.name || fact.id}</strong><small>{t(`settings.memory.scope.${fact.scope}` as Parameters<typeof t>[0])} · {fact.freshness} · {t("settings.memory.revisionLabel", { revision: fact.revision })}</small></div><p>{fact.description || fact.body}</p><div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy || dirty} onClick={() => { setEditingFact(editingFact === fact.id ? "" : fact.id); setFactDraft({ name: fact.name, title: fact.title, description: fact.description, type: fact.type || "project", body: fact.body }); }}>{editingFact === fact.id ? t("settings.memory.cancelEdit") : t("settings.memory.editFact")}</button><button type="button" className="tauri-settings-button" disabled={busy || dirty} onClick={() => void toggleRevisionHistory(fact)}>{expandedFact === fact.id ? t("settings.memory.hideHistory") : t("settings.memory.showHistory")}</button><button type="button" className="tauri-settings-button" disabled={busy || dirty} onClick={() => { if (window.confirm(t("settings.memory.archiveConfirm", { title: fact.title || fact.name || fact.id }))) void mutate({ ...base, action: "archive", factId: fact.id, factRevision: fact.revision }, t("settings.memory.archived")); }}>{t("settings.memory.archive")}</button></div>{editingFact === fact.id && <section className="tauri-memory-history"><label>{t("settings.memory.factName")}<input value={factDraft.name} disabled={busy} onChange={event => setFactDraft({ ...factDraft, name: event.target.value })} /></label><label>{t("settings.memory.factTitle")}<input value={factDraft.title} disabled={busy} onChange={event => setFactDraft({ ...factDraft, title: event.target.value })} /></label><label>{t("settings.memory.factDescription")}<input value={factDraft.description} disabled={busy} onChange={event => setFactDraft({ ...factDraft, description: event.target.value })} /></label><label>{t("settings.memory.factType")}<select value={factDraft.type} disabled={busy} onChange={event => setFactDraft({ ...factDraft, type: event.target.value })}><option value="user">{t("settings.memory.type.user")}</option><option value="feedback">{t("settings.memory.type.feedback")}</option><option value="project">{t("settings.memory.type.project")}</option><option value="reference">{t("settings.memory.type.reference")}</option></select></label><label>{t("settings.memory.factBody")}<textarea className="tauri-memory-editor" value={factDraft.body} disabled={busy} onChange={event => setFactDraft({ ...factDraft, body: event.target.value })} /></label><div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setEditingFact("")}>{t("settings.memory.cancelEdit")}</button><button type="button" className="tauri-settings-button" disabled={busy || dirty || !factDraft.name.trim() || !factDraft.description.trim() || !factDraft.body.trim()} onClick={() => void mutate({ ...base, action: "save_fact", factId: fact.id, factRevision: fact.revision, name: factDraft.name, title: factDraft.title, description: factDraft.description, type: factDraft.type, body: factDraft.body }, t("settings.memory.factSaved"))}>{t("settings.memory.saveFact")}</button></div></section>}{expandedFact === fact.id && <section className="tauri-memory-history" aria-label={t("settings.memory.historyAria")}><strong>{t("settings.memory.historyTitle")}</strong>{revisionHistory[fact.id]?.length ? revisionHistory[fact.id].map(revision => <div className="tauri-settings-actions" key={`${fact.id}:${revision.revision}`}><span>{t("settings.memory.revisionLabel", { revision: revision.revision })}{revision.updatedAt ? ` · ${revision.updatedAt}` : ""}</span><button type="button" className="tauri-settings-button" disabled={busy || dirty} onClick={() => void mutate({ ...base, action: "restore_revision", factId: fact.id, factRevision: fact.revision, historyRevision: revision.revision }, t("settings.memory.revisionRestored"))}>{t("settings.memory.restoreRevision")}</button></div>) : <p>{t("settings.memory.noHistory")}</p>}</section>}</article>)}</div>}
      <button type="button" className="tauri-settings-button" onClick={() => setShowArchives(value => !value)}>{showArchives ? t("settings.memory.hideArchives") : t("settings.memory.viewArchives", { count: view.archives.length })}</button>
      {showArchives && (view.archives.length ? <div className="tauri-memory-facts">{view.archives.map(item => <article key={item.path} className="tauri-memory-fact"><div><strong>{item.title || item.name || item.id}</strong><small>{item.archivedAt || item.scope}</small></div><p>{item.description || item.body}</p><button type="button" className="tauri-settings-button" disabled={busy || dirty} onClick={() => void mutate({ ...base, action: "restore", path: item.path }, t("settings.memory.restored"))}>{t("settings.memory.restore")}</button></article>)}</div> : <p>{t("settings.memory.noArchives")}</p>)}
      {view.diagnostics.length > 0 && <><h3>{t("settings.memory.diagnosticsTitle")}</h3>{view.diagnostics.map((item, index) => <p key={index}>{item}</p>)}</>}
      <h3>{t("memory.recallTitle")}</h3><p>{t("memory.recallHint")}</p>
      <div className="tauri-memory-fact"><strong>{view.lastRecall.query || t("memory.noRecallQuery")}</strong><small>{t("memory.recallBudget", { used: view.lastRecall.usedChars, budget: view.lastRecall.charBudget, omitted: view.lastRecall.omitted })}</small>
        {view.lastRecall.suppressed && <p>{t("memory.recallSuppressed", { reason: view.lastRecall.suppressed })}</p>}
        {view.lastRecall.hits.length === 0 ? <p>{t("memory.noRecallHits")}</p> : view.lastRecall.hits.map(hit => <article key={`${hit.id}:${hit.revision}`}><strong>{hit.title || hit.name}</strong><small>{Math.round(hit.score * 100)}% · {hit.scope} · {hit.freshness} · {t("memory.revision", { revision: hit.revision })}</small><p>{hit.snippet}</p><small>{hit.reason}</small></article>)}
      </div>
    </>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
