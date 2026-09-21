import { useEffect, useState } from "react";
import { Activity, ArrowUp, Check, ChevronDown, FileText, FolderOpen, MessageSquare, Paperclip, Pencil, Plus, Sparkles, Square, Trash2, X } from "lucide-react";
import type { UnlistenFn } from "@tauri-apps/api/event";
import { Markdown } from "../components/Markdown";
import { parseAttachmentRefsForDisplay } from "../lib/attachmentDisplay";
import logoWordmark from "../assets/logo-wordmark.svg";
import {
  TAURI_TITLE_MAX_CHARS,
  cancelTauriBridge,
  attachTauriFile,
  chooseTauriAttachmentFiles,
  chooseTauriWorkspaceRoot,
  deleteTauriBridgeSession,
  forgetTauriWorkbenchSession,
  importTauriStableProfile,
  newTauriSessionId,
  onTauriBridgeConnectionError,
  onTauriBridgeEvent,
  openTauriBridgeSession,
  rememberTauriWorkbenchSession,
  renameTauriBridgeSession,
  restartTauriBridge,
  setTauriDefaultModel,
  startTauriBridgeEvents,
  submitTauriBridge,
  switchTauriBridgeSession,
  tauriBridgeHistory,
  tauriBridgeSnapshot,
  tauriBridgeStatus,
  tauriAssistantTextDelta,
  tauriComposerInput,
  tauriEventSummary,
  tauriMessageFrom,
  tauriProviderSummary,
  tauriPreviewProfileStatus,
  tauriPreviewRuntimeInfo,
  tauriSessionTitle,
  tauriTitleError,
  tauriTurnFailure,
  tauriWorkbenchSessions,
  type TauriBridgeEvent,
  type TauriBridgeAttachment,
  type TauriBridgeHistory,
  type TauriBridgeSession,
  type TauriBridgeStatus,
  type TauriPreviewProfileStatus,
  type TauriPreviewRuntimeInfo,
  type TauriProviderSummary,
} from "../lib/tauriBridge";
import "./tauriChatWorkspace.css";

interface WorkbenchSessionTab {
  sessionId: string;
  title?: string;
  workspaceRoot?: string;
}

function sessionLabel(sessionId: string): string {
  const suffix = sessionId.replace(/^tauri-/, "").slice(0, 7);
  return `对话 ${suffix}`;
}

/** A stored title is authoritative; anything the bridge would reject falls back
 *  to the session-derived label instead of rendering bad host state. */
function displayTitle(storedTitle: string | undefined, sessionId: string): string {
  return tauriSessionTitle(storedTitle, sessionLabel(sessionId));
}

interface SessionRowProps {
  tab: WorkbenchSessionTab;
  active: boolean;
  busy: boolean;
  switchingBlocked: boolean;
  onActivate: () => void;
  onDelete: () => void;
}

/** One recent conversation. The confirm step lives in the row so only the row
 *  the user armed changes shape, and leaving the row disarms it. */
function SessionRow({ tab, active, busy, switchingBlocked, onActivate, onDelete }: SessionRowProps) {
  const [confirming, setConfirming] = useState(false);
  if (confirming) {
    return (
      <div className="tauri-session-delete" role="group" aria-label="确认删除对话">
        <span className="tauri-session-delete__question">删除“{displayTitle(tab.title, tab.sessionId)}”？</span>
        <span className="tauri-session-delete__actions">
          <button type="button" className="tauri-session-delete__confirm" disabled={busy} onClick={onDelete}>删除</button>
          <button type="button" className="tauri-session-delete__cancel" disabled={busy} onClick={() => setConfirming(false)}>取消</button>
        </span>
      </div>
    );
  }
  return (
    <div className={`tauri-session-row${active ? " is-active" : ""}`}>
      <button
        type="button"
        className="tauri-sidebar__session"
        disabled={busy || active || switchingBlocked}
        onClick={onActivate}
        title={tab.workspaceRoot ? `${tab.sessionId}\n${tab.workspaceRoot}` : tab.sessionId}
      >
        <MessageSquare size={15} aria-hidden="true" />
        <span>{displayTitle(tab.title, tab.sessionId)}</span>
      </button>
      <button
        type="button"
        className="tauri-session-row__delete"
        aria-label={`删除对话 ${displayTitle(tab.title, tab.sessionId)}`}
        title="删除对话"
        disabled={busy || switchingBlocked}
        onClick={() => setConfirming(true)}
      >
        <Trash2 size={13} aria-hidden="true" />
      </button>
    </div>
  );
}

export function TauriSessionPreview() {
  const [session, setSession] = useState<TauriBridgeSession | null>(null);
  const [tabs, setTabs] = useState<WorkbenchSessionTab[]>([]);
  const [workspaceRoot, setWorkspaceRoot] = useState("");
  const [prompt, setPrompt] = useState("");
  const [attachments, setAttachments] = useState<TauriBridgeAttachment[]>([]);
  const [status, setStatus] = useState<TauriBridgeStatus | null>(null);
  const [profile, setProfile] = useState<TauriPreviewProfileStatus | null>(null);
  const [runtimeInfo, setRuntimeInfo] = useState<TauriPreviewRuntimeInfo | null>(null);
  const [providerSummary, setProviderSummary] = useState<TauriProviderSummary | null>(null);
  const [profileNotice, setProfileNotice] = useState("");
  const [events, setEvents] = useState<TauriBridgeEvent[]>([]);
  const [history, setHistory] = useState<TauriBridgeHistory | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState("");
  const [liveText, setLiveText] = useState("");
  const [sequence, setSequence] = useState(0);
  const [streamRevision, setStreamRevision] = useState(0);
  const [streamReady, setStreamReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [diagnosticsOpen, setDiagnosticsOpen] = useState(false);
  const [titleEditing, setTitleEditing] = useState(false);
  const [titleDraft, setTitleDraft] = useState("");
  const [error, setError] = useState("");
  const switchingBlocked = Boolean(session && session.state !== "idle");

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "n" && !busy && !switchingBlocked) {
        event.preventDefault();
        void createSession();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [busy, switchingBlocked]);

  useEffect(() => {
    void tauriBridgeStatus().then(setStatus).catch(error => setError(tauriMessageFrom(error)));
    void tauriPreviewProfileStatus().then(setProfile).catch(error => setError(tauriMessageFrom(error)));
    void tauriPreviewRuntimeInfo().then(setRuntimeInfo).catch(error => setError(tauriMessageFrom(error)));
    void tauriProviderSummary().then(setProviderSummary).catch(error => setError(tauriMessageFrom(error)));
    void tauriWorkbenchSessions().then(setTabs).catch(error => setError(tauriMessageFrom(error)));
  }, []);

  useEffect(() => {
    if (!session) return;
    let active = true;
    let offEvent: UnlistenFn | undefined;
    let offError: UnlistenFn | undefined;
    setHistoryLoading(true);
    setHistoryError("");

    void (async () => {
      try {
        const snapshot = await tauriBridgeSnapshot(session.id);
        if (!active) return;
        setSession(snapshot.session);
        setSequence(snapshot.sequence);
        offEvent = await onTauriBridgeEvent(event => {
          if (event.sessionId !== session.id) return;
          setSequence(previous => Math.max(previous, event.sequence));
          setEvents(previous => [event, ...previous].slice(0, 100));
          const textDelta = tauriAssistantTextDelta(event);
          if (textDelta) setLiveText(previous => previous + textDelta);
          if (event.eventKind === "turn_done") {
            const failure = tauriTurnFailure(event);
            void Promise.all([tauriBridgeSnapshot(event.sessionId), tauriBridgeHistory(event.sessionId)])
              .then(([latest, latestHistory]) => {
                if (!active) return;
                setSession(latest.session);
                setHistory(latestHistory);
                setHistoryError("");
                setHistoryLoading(false);
                setLiveText("");
                // A failed turn must keep its reason on screen; only a
                // completed turn clears a previous message.
                setError(failure);
              })
              .catch(error => {
                if (!active) return;
                const message = tauriMessageFrom(error);
                setHistoryError(message);
                if (!failure) setError(message);
              });
          }
        });
        offError = await onTauriBridgeConnectionError(message => {
          if (!active) return;
          setStreamReady(false);
          setStatus({ running: false });
          setError(`本地桥接事件流：${message}`);
        });
        await startTauriBridgeEvents(snapshot.sequence);
        if (!active) return;
        setError("");
        setStreamReady(true);
        try {
          const latestHistory = await tauriBridgeHistory(session.id);
          if (!active) return;
          setHistory(latestHistory);
          setHistoryError("");
        } catch (historyCause) {
          if (!active) return;
          const message = tauriMessageFrom(historyCause);
          setHistoryError(message);
          setError(message);
        } finally {
          if (active) setHistoryLoading(false);
        }
      } catch (error) {
        if (!active) return;
        setStreamReady(false);
        const message = tauriMessageFrom(error);
        setHistoryLoading(false);
        setHistoryError(message);
        setError(message);
      }
    })();

    return () => {
      active = false;
      offEvent?.();
      offError?.();
    };
  }, [session?.id, streamRevision]);

  async function rememberSession(next: TauriBridgeSession) {
    try {
      setTabs(await rememberTauriWorkbenchSession(next.id, next.workspaceRoot ?? undefined, tauriSessionTitle(next.title, "") || undefined));
    } catch (cause) {
      setError(`对话已打开，但无法保存到最近对话：${tauriMessageFrom(cause)}`);
    }
  }

  async function activateSession(id: string, root?: string) {
    if (!id) return setError("缺少会话 ID");
    setBusy(true);
    setError("");
    try {
      const next = session && session.id !== id
        ? await switchTauriBridgeSession(id, root)
        : await openTauriBridgeSession(id, root);
      setEvents([]);
      setHistory(null);
      setAttachments([]);
      setHistoryLoading(true);
      setHistoryError("");
      setLiveText("");
      setSequence(0);
      setStreamReady(false);
      setTitleEditing(false);
      setSession(next);
      setWorkspaceRoot(next.workspaceRoot ?? root ?? "");
      await rememberSession(next);
      setStreamRevision(previous => previous + 1);
      setStatus(await tauriBridgeStatus());
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  function beginTitleEdit() {
    if (!session || busy || switchingBlocked) return;
    setTitleDraft(displayTitle(session.title, session.id));
    setTitleEditing(true);
  }

  async function saveTitle() {
    if (!session) return;
    const title = titleDraft.trim();
    const titleError = tauriTitleError(title);
    if (titleError) {
      setError(titleError);
      return;
    }
    setBusy(true);
    setError("");
    try {
      const renamed = await renameTauriBridgeSession(session.id, title);
      setSession(renamed);
      await rememberSession(renamed);
      setTitleEditing(false);
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  // Deleting is a two-step confirmation, scoped to the row that asked for it.
  // The bridge only removes the session it owns, so an inactive conversation is
  // switched to first — silently, without disturbing the open transcript — and
  // only then swept.
  async function deleteSession(target: WorkbenchSessionTab) {
    if (busy || switchingBlocked) return;
    const isOpen = session?.id === target.sessionId;
    const sessionToRestore = session;
    if (!isOpen && (session?.state === "running" || session?.state === "paused")) {
      setError("请先停止正在生成的对话，再删除其他会话。");
      return;
    }
    setBusy(true);
    setError("");
    let switchedToTarget = false;
    let deleted = false;
    let operationError = "";
    try {
      if (!isOpen) {
        await switchTauriBridgeSession(target.sessionId, target.workspaceRoot);
        switchedToTarget = true;
      }
      await deleteTauriBridgeSession(target.sessionId);
      deleted = true;
      if (isOpen) setSession(null);
      setTabs(await forgetTauriWorkbenchSession(target.sessionId));
    } catch (cause) {
      operationError = deleted
        ? `对话已删除，但最近对话列表更新失败：${tauriMessageFrom(cause)}`
        : tauriMessageFrom(cause);
    } finally {
      // The bridge owns one controller. Deleting an inactive row temporarily
      // switches that controller, so restore the user's open session even if
      // deletion or catalog cleanup fails.
      if (switchedToTarget && sessionToRestore) {
        try {
          setSession(await switchTauriBridgeSession(sessionToRestore.id, sessionToRestore.workspaceRoot));
        } catch (cause) {
          setSession(null);
          const restoreError = `删除后无法恢复原对话：${tauriMessageFrom(cause)}`;
          operationError = operationError ? `${operationError}；${restoreError}` : restoreError;
        }
      }
      if (operationError) setError(operationError);
      setBusy(false);
    }
  }

  async function createSession() {
    await activateSession(newTauriSessionId(), workspaceRoot.trim() || undefined);
  }

  async function chooseWorkspaceRoot() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const selected = await chooseTauriWorkspaceRoot();
      if (selected) setWorkspaceRoot(selected);
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function addAttachments() {
    if (!session || !streamReady || busy || session.state === "running") return;
    setBusy(true);
    setError("");
    try {
      const selected = await chooseTauriAttachmentFiles();
      if (selected.length === 0) return;
      const added: TauriBridgeAttachment[] = [];
      let firstError = "";
      for (const path of selected) {
        try {
          added.push(await attachTauriFile(session.id, path));
        } catch (cause) {
          firstError ||= tauriMessageFrom(cause);
        }
      }
      if (added.length > 0) setAttachments(previous => [...previous, ...added]);
      if (firstError) setError(`部分文件未能添加：${firstError}`);
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function submit() {
    const input = tauriComposerInput(prompt, attachments);
    if (!session || !streamReady || !input) return;
    setBusy(true);
    setError("");
    setLiveText("");
    try {
      setSession(await submitTauriBridge(session.id, input));
      setPrompt("");
      setAttachments([]);
      setHistory(await tauriBridgeHistory(session.id));
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function cancel() {
    if (!session) return;
    setBusy(true);
    setError("");
    try {
      setSession(await cancelTauriBridge(session.id));
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function restartBridge() {
    setBusy(true);
    setError("");
    setStreamReady(false);
    try {
      setStatus(await restartTauriBridge());
      if (session) {
        const reopened = await openTauriBridgeSession(session.id, session.workspaceRoot);
        setSession(reopened);
        await rememberSession(reopened);
        setEvents([]);
        setHistory(null);
        setAttachments([]);
        setLiveText("");
        setSequence(0);
        setStreamRevision(previous => previous + 1);
      }
    } catch (cause) {
      setStatus({ running: false });
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function importStableProfile() {
    if (!profile?.importAvailable || busy) return;
    const confirmed = window.confirm(
      `将 ${profile.stableConfig ?? "稳定版配置"} 复制到隔离的 Tauri Preview 配置目录？\n\n操作前会创建带时间戳的备份。不会复制会话、缓存、插件或 .env 文件，也不会修改稳定版。`,
    );
    if (!confirmed) return;
    setBusy(true);
    setError("");
    setProfileNotice("");
    try {
      const result = await importTauriStableProfile();
      setProfileNotice(`已导入：${result.importedConfig}\n备份：${result.backupConfig}`);
      setProfile(await tauriPreviewProfileStatus());
      setProviderSummary(await tauriProviderSummary());
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function refreshHistory() {
    if (!session || busy) return;
    setBusy(true);
    setError("");
    setHistoryLoading(true);
    setHistoryError("");
    try {
      setHistory(await tauriBridgeHistory(session.id));
    } catch (cause) {
      const message = tauriMessageFrom(cause);
      setHistoryError(message);
      setError(message);
    } finally {
      setHistoryLoading(false);
      setBusy(false);
    }
  }

  async function changeDefaultModel(model: string) {
    if (!model || busy) return;
    setBusy(true);
    setError("");
    try {
      setProviderSummary(await setTauriDefaultModel(model));
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function selectStarterPrompt(value: string) {
    setPrompt(value);
    if (!session) await createSession();
  }

  const currentWorkspace = workspaceRoot || session?.workspaceRoot || "";

  return (
    <main className="tauri-shell">
      <aside className="tauri-sidebar" aria-label="会话导航">
        <div className="tauri-sidebar__brand"><img src={logoWordmark} alt="Reasonix" draggable={false} /><span>PREVIEW</span></div>
        <button className="tauri-sidebar__new" type="button" onClick={() => void createSession()} disabled={busy || switchingBlocked}>
          <Plus size={17} aria-hidden="true" /><span>新建对话</span><kbd>⌘ N</kbd>
        </button>
        <div className="tauri-sidebar__section-title">最近对话</div>
        <nav className="tauri-sidebar__sessions">
          {tabs.length === 0 ? <p className="tauri-sidebar__empty">还没有对话，开始一个新话题吧。</p> : tabs.map(tab => (
            <SessionRow
              key={tab.sessionId}
              tab={tab}
              active={session?.id === tab.sessionId}
              busy={busy}
              switchingBlocked={switchingBlocked}
              onActivate={() => void activateSession(tab.sessionId, tab.workspaceRoot)}
              onDelete={() => void deleteSession(tab)}
            />
          )) }
        </nav>
        <div className="tauri-sidebar__footer">
          <span className={`tauri-health${status?.running ? " is-ready" : ""}`}><i />{status?.running ? "本地运行正常" : "正在连接本地服务…"}</span>
          <button type="button" className="tauri-sidebar__diagnostics" onClick={() => setDiagnosticsOpen(true)}><Activity size={15} />运行状态与设置</button>
        </div>
      </aside>

      <section className="tauri-main">
        <header className="tauri-topbar">
          <div className="tauri-topbar__title">
            <div className="tauri-topbar__title-row">
              {session && titleEditing ? <form className="tauri-session-title-edit" onSubmit={event => { event.preventDefault(); void saveTitle(); }}>
                <input autoFocus maxLength={TAURI_TITLE_MAX_CHARS} value={titleDraft} onChange={event => setTitleDraft(event.target.value)} aria-label="对话名称" onKeyDown={event => { if (event.key === "Escape") setTitleEditing(false); }} />
                <button type="submit" className="tauri-session-title-edit__action" aria-label="保存对话名称" disabled={busy}><Check size={14} /></button>
                <button type="button" className="tauri-session-title-edit__action" aria-label="取消重命名" disabled={busy} onClick={() => setTitleEditing(false)}><X size={14} /></button>
              </form> : <>
                <strong>{session ? displayTitle(session.title, session.id) : "新对话"}</strong>
                {session && <button type="button" className="tauri-title-rename" aria-label="重命名对话" title="重命名对话" disabled={busy || switchingBlocked} onClick={beginTitleEdit}><Pencil size={13} /></button>}
              </>}
            </div>
            <span>{session?.state === "running" ? "正在生成" : session ? "本地会话" : "Reasonix Preview"}</span>
          </div>
          <div className="tauri-topbar__actions">
            <button type="button" className="tauri-workspace-button" onClick={() => void chooseWorkspaceRoot()} disabled={busy || session?.state === "running"} title={currentWorkspace || "选择工作区（用于新对话）"}>
              <FolderOpen size={15} /><span>{currentWorkspace || "选择工作区"}</span><ChevronDown size={13} />
            </button>
            <label className="tauri-model-picker" title="此默认模型用于新对话；工作区配置可能覆盖它">
              <span>模型</span>
              <select value={providerSummary?.defaultModel ?? ""} onChange={event => void changeDefaultModel(event.target.value)} disabled={busy || !providerSummary?.providers.some(provider => provider.configured && provider.models.length > 0)}>
                {!providerSummary?.defaultModel && <option value="" disabled>未选择</option>}
                {providerSummary?.defaultModel && <option value={providerSummary.defaultModel}>{providerSummary.defaultModel} · 当前</option>}
                {providerSummary?.providers.filter(provider => provider.configured).flatMap(provider => provider.models.map(model => {
                  const ref = `${provider.name}/${model}`;
                  return ref === providerSummary.defaultModel ? null : <option key={ref} value={ref}>{provider.displayName || provider.name} / {model}</option>;
                }))}
              </select>
              <ChevronDown size={13} aria-hidden="true" />
            </label>
            <button type="button" className="tauri-icon-button" aria-label="打开运行状态与设置" onClick={() => setDiagnosticsOpen(true)}><Activity size={17} /></button>
          </div>
        </header>

        <div className="tauri-conversation">
          {session && !history && historyLoading ? <div className="tauri-loading"><span /><p>正在载入对话…</p></div> : session && !history && historyError ? <div className="tauri-loading tauri-history-error"><p>无法载入对话记录</p><p>{historyError}</p><button type="button" className="tauri-diagnostic-action" onClick={() => void refreshHistory()} disabled={busy}>重新加载</button></div> : history?.messages.length || liveText || session?.state === "running" ? <div className="tauri-transcript">
            {history && history.startIndex > 0 && <p className="tauri-history-note">当前显示最近 {history.messages.length} 条，共 {history.totalMessages} 条可见消息</p>}
            {history?.messages.map((message, index) => {
              const display = message.role === "user" ? parseAttachmentRefsForDisplay(message.content) : null;
              return <article key={`${history.startIndex + index}-${message.role}`} className={`tauri-message is-${message.role}`}>
                <div className="tauri-message__avatar" aria-hidden="true">{message.role === "user" ? "你" : <Sparkles size={16} />}</div>
                <div className="tauri-message__content">
                  <div className="tauri-message__role">{message.role === "user" ? "你" : "Reasonix"}</div>
                  <Markdown text={display?.text ?? message.content} cacheKey={`${history.session.id}:${history.startIndex + index}`} />
                  {display && display.attachments.length > 0 && <div className="tauri-message__attachments">{display.attachments.map(attachment => <span key={attachment.path} title={attachment.path}><Paperclip size={13} />{attachment.name}</span>)}</div>}
                  {message.truncated && <small>为保护界面性能，这条历史内容已截断。</small>}
                </div>
              </article>;
            })}
            {liveText && <article className="tauri-message is-assistant tauri-message--live"><div className="tauri-message__avatar" aria-hidden="true"><Sparkles size={16} /></div><div className="tauri-message__content"><div className="tauri-message__role">Reasonix</div><Markdown text={liveText} streaming cacheKey={`${session?.id ?? "live"}:stream`} /></div></article>}
            {session?.state === "running" && !liveText && <div className="tauri-thinking" role="status"><span /><span /><span />Reasonix 正在思考…</div>}
          </div> : <section className="tauri-welcome">
            <div className="tauri-welcome__mark"><Sparkles size={24} /></div>
            <p className="tauri-welcome__eyebrow">REASONIX · TAURI PREVIEW</p>
            <h1>把复杂的事，<br /><span>一步步想清楚。</span></h1>
            <p className="tauri-welcome__copy">代码、想法或待办——从一个问题开始，Reasonix 会陪你一起拆解。</p>
            <div className="tauri-starters" aria-label="开始方式">
              <button type="button" onClick={() => void selectStarterPrompt("帮我梳理一下这个项目的结构和关键模块")} disabled={busy}><span className="tauri-starters__icon"><FolderOpen size={16} /></span><span><b>理解一个代码库</b><small>梳理项目结构与关键模块</small></span><ArrowUp size={14} /></button>
              <button type="button" onClick={() => void selectStarterPrompt("帮我把这个想法拆解成清晰、可执行的步骤")} disabled={busy}><span className="tauri-starters__icon"><Sparkles size={16} /></span><span><b>拆解一个想法</b><small>从目标整理到可执行计划</small></span><ArrowUp size={14} /></button>
              <button type="button" onClick={() => void selectStarterPrompt("请帮我检查这段内容，指出问题并给出改进建议")} disabled={busy}><span className="tauri-starters__icon"><Check size={16} /></span><span><b>检查并改进</b><small>发现问题并给出具体建议</small></span><ArrowUp size={14} /></button>
            </div>
            {!session && <button className="tauri-welcome__start" type="button" onClick={() => void createSession()} disabled={busy}><Plus size={16} />开始新对话</button>}
          </section>}
        </div>

        <footer className="tauri-composer-area">
          {error && <p className="tauri-error" role="alert">{error}</p>}
          <div className="tauri-composer">
            {attachments.length > 0 && <div className="tauri-composer__attachments" aria-label="已添加文件">{attachments.map((attachment, index) => <div className="tauri-composer__attachment" key={`${attachment.path}-${index}`} title={attachment.path}>
              <span className="tauri-composer__attachment-icon"><FileText size={15} /></span><span className="tauri-composer__attachment-name">{attachment.name}</span><small>{attachment.size < 1024 ? `${attachment.size} B` : `${(attachment.size / 1024).toFixed(1)} KB`}</small>
              <button type="button" onClick={() => setAttachments(previous => previous.filter((_, itemIndex) => itemIndex !== index))} disabled={busy} aria-label={`移除文件 ${attachment.name}`}><X size={13} /></button>
            </div>)}</div>}
            <textarea value={prompt} onChange={event => setPrompt(event.target.value)} onKeyDown={event => {
              if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) { event.preventDefault(); void submit(); }
            }} placeholder={session ? "继续聊聊你的问题…（⌘/Ctrl + Enter 发送）" : "先开始一个新对话，再输入你的问题"} disabled={busy || !session || !streamReady} rows={3} />
            <div className="tauri-composer__bottom"><span>{session?.workspaceRoot ? `工作区 · ${session.workspaceRoot}` : session ? "当前会话使用默认工作区" : "Preview 配置与稳定版相互隔离"}</span>
              <div className="tauri-composer__actions">
                <button className="tauri-attach-button" type="button" onClick={() => void addAttachments()} disabled={busy || !session || !streamReady || session.state === "running"} aria-label="添加文件" title="从本机选择文件并附加到消息"><Paperclip size={16} /><span>添加文件</span></button>
                {session?.state === "running" ? <button className="tauri-send-button is-stop" type="button" onClick={() => void cancel()} disabled={busy} aria-label="停止生成"><Square size={15} fill="currentColor" /></button> : <button className="tauri-send-button" type="button" onClick={() => void submit()} disabled={busy || !streamReady || (!prompt.trim() && attachments.length === 0)} aria-label="发送消息"><ArrowUp size={18} /></button>}
              </div>
            </div>
          </div>
          <p className="tauri-composer-hint">Reasonix 可能会出错，请核对重要信息。<button type="button" onClick={() => setDiagnosticsOpen(true)}>预览版说明</button></p>
        </footer>
      </section>

      {diagnosticsOpen && <>
        <button className="tauri-diagnostics__scrim" aria-label="关闭运行状态面板" type="button" onClick={() => setDiagnosticsOpen(false)} />
        <aside className="tauri-diagnostics" aria-label="运行状态与设置">
          <header className="tauri-diagnostics__header"><div><p>TAURI PREVIEW</p><h2>运行状态与设置</h2></div><button type="button" className="tauri-icon-button" onClick={() => setDiagnosticsOpen(false)} aria-label="关闭"><X size={17} /></button></header>
          <div className="tauri-diagnostics__body">
            <section className="tauri-diagnostic-card"><div className="tauri-diagnostic-card__heading"><h3>本地服务</h3><span className={`tauri-health${status?.running ? " is-ready" : ""}`}><i />{status?.running ? `运行中 · 协议 v${status.protocolVersion ?? "?"}` : "正在连接"}</span></div><button type="button" className="tauri-diagnostic-action" onClick={() => void restartBridge()} disabled={busy}>重启桥接服务{session ? "并恢复当前会话" : ""}</button></section>
            <section className="tauri-diagnostic-card">
              <h3>预览配置</h3>
              {profile ? <><p>配置与会话保存在独立预览目录：</p><code>{profile.previewHome}</code>{profile.importAvailable ? <><p>检测到稳定版配置。复制前会创建备份，不会改动稳定版。</p><button type="button" className="tauri-diagnostic-action" onClick={() => void importStableProfile()} disabled={busy}>复制稳定版配置（先备份）</button></> : <p>{profile.previewConfigExists ? "预览配置已存在，不会覆盖。" : profile.managedProfile ? "未发现可复制的稳定版配置。" : "检测到自定义 REASONIX_HOME，已停用自动导入。"}</p>}</> : <p>正在检查隔离配置…</p>}
              {profileNotice && <p className="tauri-diagnostic-notice">{profileNotice}</p>}
            </section>
            <section className="tauri-diagnostic-card">
              <div className="tauri-diagnostic-card__heading"><h3>模型提供方</h3><button type="button" onClick={() => void tauriProviderSummary().then(setProviderSummary).catch(cause => setError(tauriMessageFrom(cause)))} disabled={busy}>刷新</button></div>
              {!providerSummary ? <p>正在读取 Preview 配置…</p> : <><p>默认模型只影响新对话；工作区 <code>reasonix.toml</code> 可能覆盖用户默认值。</p>{providerSummary.providers.length === 0 ? <p>当前没有配置提供方。</p> : <ul>{providerSummary.providers.map(provider => <li key={provider.name}><strong>{provider.displayName || provider.name}</strong><span>{provider.kind} · {provider.modelCount} 个模型 · {provider.configured ? "已就绪" : "缺少 API Key"}</span></li>)}</ul>}<small>密钥、环境变量名和服务端点不会传到界面。</small></>}
            </section>
            <details className="tauri-diagnostic-card tauri-runtime-details"><summary>构建与版本详情</summary>{!runtimeInfo ? <p>正在读取构建信息…</p> : <dl><div><dt>稳定版基线</dt><dd>v{runtimeInfo.stableVersion} · {runtimeInfo.stableCommit.slice(0, 12)}</dd></div><div><dt>Preview / Tauri</dt><dd>v{runtimeInfo.previewVersion} · v{runtimeInfo.tauriVersion}</dd></div><div><dt>宿主构建时间</dt><dd>{runtimeInfo.previewBuild}</dd></div><div><dt>桥接协议</dt><dd>v{runtimeInfo.bridgeProtocolVersion}</dd></div><div><dt>Sidecar</dt><dd>{runtimeInfo.sidecarInstanceId ?? "未运行"}</dd></div>{session && <div><dt>会话 ID</dt><dd>{session.id}</dd></div>}{session && <div><dt>会话路径</dt><dd>{session.path}</dd></div>}{history && <div><dt>历史记录</dt><dd>{history.totalMessages} 条</dd></div>}<div><dt>事件游标</dt><dd>{sequence} · 最近 {events.length} 个事件</dd></div></dl>}</details>
            <details className="tauri-diagnostic-card tauri-event-details"><summary>桥接事件日志</summary>{events.length === 0 ? <p>开始一个对话后，这里会显示桥接事件。</p> : <ol>{events.map(event => <li key={event.sequence}><b>#{event.sequence} · {event.eventKind}</b><pre>{tauriEventSummary(event)}</pre></li>)}</ol>}</details>
            {session && <button type="button" className="tauri-diagnostic-action" onClick={() => void refreshHistory()} disabled={busy}>刷新当前对话记录</button>}
            <p className="tauri-diagnostics__note">会话切换仅在当前回复结束后启用，避免中断正在进行的请求。</p>
          </div>
        </aside>
      </>}
    </main>
  );
}
