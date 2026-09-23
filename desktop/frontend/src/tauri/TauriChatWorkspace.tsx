import { Component, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Activity, ArrowUp, Check, ChevronDown, ChevronRight, Eye, FileText, FolderOpen, FolderTree, GitBranch, MessageSquare, Paperclip, Pencil, Plus, Settings, Sparkles, Square, Trash2, X } from "lucide-react";
import type { UnlistenFn } from "@tauri-apps/api/event";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { isPermissionGranted, requestPermission, sendNotification } from "@tauri-apps/plugin-notification";
import { Markdown } from "../components/Markdown";
import { QuestionJumpBar } from "../components/QuestionJumpBar";
import { parseAttachmentRefsForDisplay } from "../lib/attachmentDisplay";
import { compactQuestionText, type QuestionAnchor } from "../lib/transcriptGrouping";
import { LocaleProvider } from "../lib/i18n";
import logoWordmark from "../assets/logo-wordmark.svg";
import { TauriSettings } from "./TauriSettings";
import { handleTauriDragDropEvent } from "./dragDrop";

/** Per-message error boundary to prevent one bad message from crashing the entire transcript. */
class MessageErrorBoundary extends Component<{ children: ReactNode; index: number }, { hasError: boolean }> {
  state = { hasError: false };
  static getDerivedStateFromError() { return { hasError: true }; }
  render() {
    if (this.state.hasError) {
      return <div style={{ padding: "8px 12px", color: "#e0696a", fontSize: "11px", border: "1px solid #343945", borderRadius: "8px", margin: "8px 0" }}>
        消息 #{this.props.index} 渲染出错
      </div>;
    }
    return this.props.children;
  }
}
import {
  TAURI_TITLE_MAX_CHARS,
  answerTauriMCPInteraction,
  answerTauriQuestion,
  approveTauriBridge,
  cancelTauriBridge,
  attachTauriFile,
  chooseTauriAttachmentFiles,
  chooseTauriWorkspaceRoot,
  deleteTauriBridgeSession,
  forgetTauriWorkbenchSession,
  importTauriStableProfile,
  newTauriSessionId,
  onTauriBridgeConnectionError,
  onTauriBridgeConnectionRestored,
  onTauriBridgeEvent,
  onTauriBridgeResyncRequired,
  openTauriBridgeSession,
  rememberTauriWorkbenchSession,
  renameTauriBridgeSession,
  restartTauriBridge,
  replayTauriPendingPrompts,
  setTauriDefaultModel,
  startTauriBridgeEvents,
  submitTauriBridge,
  switchTauriBridgeSession,
  tauriWorkspace,
  tauriWorkspaceFile,
  tauriWorkspaceChanges,
  tauriWorkspaceChangeDetail,
  tauriBridgeHistory,
  tauriBridgeSnapshot,
  tauriBridgeStatus,
  tauriAssistantTextDelta,
  tauriComposerInput,
  tauriEventSummary,
  tauriMessageFrom,
  tauriPromptAnsweredId,
  tauriPromptFromEvent,
  tauriProviderSummary,
  tauriPlatformInfo,
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
  type TauriPendingPrompt,
  type TauriWorkspaceEntry,
  type TauriWorkspaceFilePreview,
  type TauriWorkspaceChanges,
  type TauriWorkspaceChangeDetail,
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

function workspaceChangeLabel(status?: string, sources?: string[]): string {
  const normalized = (status ?? "").trim();
  if (normalized === "??") return "新增";
  if (normalized.includes("R")) return "重命名";
  if (normalized.includes("D")) return "删除";
  if (normalized.includes("A")) return "新增";
  if (normalized.includes("M")) return "修改";
  if (sources?.includes("session")) return "本轮修改";
  return normalized || "变更";
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

interface PromptCardProps {
  prompt: TauriPendingPrompt;
  busy: boolean;
  selections: Record<string, string[]>;
  onApproval: (allow: boolean) => void;
  onAskSelection: (questionId: string, label: string, multi: boolean) => void;
  onAskSubmit: () => void;
  onMCPAction: (action: "accept" | "decline" | "cancel") => void;
}

function PromptCard({ prompt, busy, selections, onApproval, onAskSelection, onAskSubmit, onMCPAction }: PromptCardProps) {
  if (prompt.kind === "approval") {
    return <section className="tauri-prompt-card" aria-live="polite" aria-label="等待权限确认">
      <div className="tauri-prompt-card__heading"><Sparkles size={17} /><div><strong>需要你的确认</strong><span>{prompt.tool}</span></div></div>
      {prompt.subject && <p className="tauri-prompt-card__subject">{prompt.subject}</p>}
      {prompt.reason && <p className="tauri-prompt-card__reason">{prompt.reason}</p>}
      <div className="tauri-prompt-card__actions"><button type="button" className="tauri-prompt-card__allow" onClick={() => onApproval(true)} disabled={busy}>允许一次</button><button type="button" className="tauri-prompt-card__deny" onClick={() => onApproval(false)} disabled={busy}>拒绝</button></div>
    </section>;
  }
  if (prompt.kind === "ask") {
    return <section className="tauri-prompt-card" aria-live="polite" aria-label="回答助手提问">
      <div className="tauri-prompt-card__heading"><Sparkles size={17} /><div><strong>请回答一个问题</strong><span>回答后助手会继续工作</span></div></div>
      <div className="tauri-prompt-card__questions">{prompt.questions.map(question => {
        const selected = selections[question.id] ?? [];
        return <fieldset key={question.id} className="tauri-prompt-card__question"><legend>{question.header || "问题"}</legend><p>{question.prompt}</p><div className="tauri-prompt-card__options">{question.options.map(option => {
          const active = selected.includes(option.label);
          return <button key={option.label} type="button" className={active ? "is-selected" : ""} onClick={() => onAskSelection(question.id, option.label, question.multi)} disabled={busy} aria-pressed={active}><b>{option.label}</b>{option.description && <small>{option.description}</small>}</button>;
        })}</div></fieldset>;
      })}</div>
      <div className="tauri-prompt-card__actions"><button type="button" className="tauri-prompt-card__allow" onClick={onAskSubmit} disabled={busy}>提交回答</button><button type="button" className="tauri-prompt-card__deny" onClick={onAskSubmit} disabled={busy}>跳过</button></div>
    </section>;
  }
  return <section className="tauri-prompt-card" aria-live="polite" aria-label="MCP 服务请求">
    <div className="tauri-prompt-card__heading"><Sparkles size={17} /><div><strong>{prompt.server} 请求你的操作</strong><span>{prompt.mode === "url" ? "打开链接完成后再继续" : "这是一个外部服务请求"}</span></div></div>
    {prompt.message && <p className="tauri-prompt-card__subject">{prompt.message}</p>}
    {prompt.url && <a className="tauri-prompt-card__link" href={prompt.url} target="_blank" rel="noreferrer">打开外部链接</a>}
    <div className="tauri-prompt-card__actions"><button type="button" className="tauri-prompt-card__allow" onClick={() => onMCPAction("accept")} disabled={busy}>接受并继续</button><button type="button" className="tauri-prompt-card__deny" onClick={() => onMCPAction("decline")} disabled={busy}>拒绝</button><button type="button" className="tauri-prompt-card__cancel" onClick={() => onMCPAction("cancel")} disabled={busy}>取消</button></div>
  </section>;
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
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [workspaceOpen, setWorkspaceOpen] = useState(false);
  const [workspacePath, setWorkspacePath] = useState("");
  const [workspaceEntries, setWorkspaceEntries] = useState<TauriWorkspaceEntry[]>([]);
  const [workspaceLoading, setWorkspaceLoading] = useState(false);
  const [workspaceError, setWorkspaceError] = useState("");
  const [workspaceTruncated, setWorkspaceTruncated] = useState(false);
  const [workspacePreview, setWorkspacePreview] = useState<TauriWorkspaceFilePreview | null>(null);
  const [workspacePreviewLoading, setWorkspacePreviewLoading] = useState(false);
  const [workspaceView, setWorkspaceView] = useState<"files" | "changes">("files");
  const [workspaceChanges, setWorkspaceChanges] = useState<TauriWorkspaceChanges | null>(null);
  const [workspaceChangesLoading, setWorkspaceChangesLoading] = useState(false);
  const [workspaceChangeDetail, setWorkspaceChangeDetail] = useState<TauriWorkspaceChangeDetail | null>(null);
  const [workspaceChangeDetailPath, setWorkspaceChangeDetailPath] = useState("");
  const [workspaceChangeDetailLoading, setWorkspaceChangeDetailLoading] = useState(false);
  const [titleEditing, setTitleEditing] = useState(false);
  const [titleDraft, setTitleDraft] = useState("");
  const [error, setError] = useState("");
  const [pendingPrompt, setPendingPrompt] = useState<TauriPendingPrompt | null>(null);
  const [promptSelections, setPromptSelections] = useState<Record<string, string[]>>({});
  const conversationRef = useRef<HTMLDivElement>(null);
  const [activeQuestion, setActiveQuestion] = useState<number | null>(null);
  const switchingBlocked = Boolean(session && session.state !== "idle");
  const [platform, setPlatform] = useState<string>("");
  // Optimistic user message: displayed immediately after submit, cleared when history loads
  const [pendingUserMessage, setPendingUserMessage] = useState<string | null>(null);
  // Drag-and-drop state
  const [dragging, setDragging] = useState(false);

  const questions = useMemo<QuestionAnchor[]>(() => {
    if (!history) return [];
    let turn = 0;
    return history.messages.flatMap((message, index) => {
      if (message.role !== "user") return [];
      const display = parseAttachmentRefsForDisplay(message.content);
      const question: QuestionAnchor = {
        id: `tauri-question-${history.startIndex + index}`,
        text: compactQuestionText(display.text),
        turn,
        loaded: true,
      };
      turn += 1;
      return [question];
    });
  }, [history]);

  useEffect(() => {
    const conversation = conversationRef.current;
    if (!conversation) return;
    const updateActiveQuestion = () => {
      const top = conversation.getBoundingClientRect().top + 24;
      let active = questions[0]?.turn ?? null;
      for (const question of questions) {
        const anchor = document.getElementById(question.id);
        if (!anchor || anchor.getBoundingClientRect().top > top) break;
        active = question.turn;
      }
      setActiveQuestion(active);
    };
    conversation.addEventListener("scroll", updateActiveQuestion, { passive: true });
    updateActiveQuestion();
    return () => conversation.removeEventListener("scroll", updateActiveQuestion);
  }, [questions]);

  function jumpToQuestion(question: QuestionAnchor) {
    const conversation = conversationRef.current;
    const anchor = document.getElementById(question.id);
    if (!conversation || !anchor) return;
    const top = anchor.getBoundingClientRect().top - conversation.getBoundingClientRect().top + conversation.scrollTop - 20;
    conversation.scrollTo({ top: Math.max(0, top), behavior: "smooth" });
    setActiveQuestion(question.turn);
  }

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      // Escape: Close open panels (no modifier required)
      if (event.key === "Escape") {
        if (settingsOpen) { setSettingsOpen(false); return; }
        if (diagnosticsOpen) { setDiagnosticsOpen(false); return; }
        if (workspaceOpen) { setWorkspaceOpen(false); return; }
      }
      const mod = event.metaKey || event.ctrlKey;
      if (!mod) return;
      const key = event.key.toLowerCase();
      // Cmd/Ctrl + N: New session
      if (key === "n" && !busy && !switchingBlocked) {
        event.preventDefault();
        void createSession();
      }
      // Cmd/Ctrl + ,: Settings
      if (key === ",") {
        event.preventDefault();
        setSettingsOpen(true);
      }
      // Cmd/Ctrl + .: Diagnostics
      if (key === ".") {
        event.preventDefault();
        setDiagnosticsOpen(true);
      }
      // Cmd/Ctrl + B: Toggle workspace panel
      if (key === "b" && session) {
        event.preventDefault();
        toggleWorkspace();
      }
      // Cmd/Ctrl + R: Refresh history
      if (key === "r" && session && !busy) {
        event.preventDefault();
        void refreshHistory();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [busy, switchingBlocked, session, settingsOpen, diagnosticsOpen, workspaceOpen]);

  useEffect(() => {
    void tauriBridgeStatus().then(setStatus).catch(error => setError(tauriMessageFrom(error)));
    void tauriPreviewProfileStatus().then(setProfile).catch(error => setError(tauriMessageFrom(error)));
    void tauriPreviewRuntimeInfo().then(setRuntimeInfo).catch(error => setError(tauriMessageFrom(error)));
    void tauriProviderSummary().then(setProviderSummary).catch(error => setError(tauriMessageFrom(error)));
    void tauriWorkbenchSessions().then(setTabs).catch(error => setError(tauriMessageFrom(error)));
    void tauriPlatformInfo().then(setPlatform).catch(() => {});
  }, []);

  // Tauri drag-and-drop file handler
  useEffect(() => {
    let unlisten: UnlistenFn | undefined;
    void (async () => {
      try {
        unlisten = await getCurrentWindow().onDragDropEvent(async (event) => {
          await handleTauriDragDropEvent(event.payload, {
            sessionId: session?.id,
            attachFile: attachTauriFile,
            addAttachment: attached => setAttachments(previous => [...previous, attached]),
            setDragging,
          });
        });
      } catch {
        // Drag-drop not supported in browser mode
      }
    })();
    return () => { unlisten?.(); };
  }, [session?.id]);

  useEffect(() => {
    if (!session) return;
    let active = true;
    let offEvent: UnlistenFn | undefined;
    let offError: UnlistenFn | undefined;
    let offRestored: UnlistenFn | undefined;
    let offResync: UnlistenFn | undefined;
    let resyncRequested = false;
    let streamDisconnected = false;
    setHistoryLoading(true);
    setHistoryError("");

    void (async () => {
      try {
        const snapshot = await tauriBridgeSnapshot(session.id);
        if (!active) return;
        setSession(snapshot.session);
        setSequence(snapshot.sequence);
        let lastSequence = snapshot.sequence;
        offEvent = await onTauriBridgeEvent(event => {
          if (!active || resyncRequested || event.sessionId !== session.id || event.sequence <= lastSequence) return;
          lastSequence = event.sequence;
          setSequence(previous => Math.max(previous, event.sequence));
          setEvents(previous => [event, ...previous].slice(0, 100));
          const incomingPrompt = tauriPromptFromEvent(event);
          if (incomingPrompt) {
            setPendingPrompt(incomingPrompt);
            setPromptSelections({});
            setSession(previous => previous ? { ...previous, state: "paused" } : previous);
          }
          const answeredPromptID = tauriPromptAnsweredId(event);
          if (answeredPromptID) {
            setPendingPrompt(previous => previous?.id === answeredPromptID ? null : previous);
            setPromptSelections({});
            setSession(previous => previous ? { ...previous, state: "running" } : previous);
          }
          const textDelta = tauriAssistantTextDelta(event);
          if (textDelta) setLiveText(previous => previous + textDelta);
          if (event.eventKind === "turn_done") {
            setPendingPrompt(null);
            setPromptSelections({});
            // Send notification if window is not focused
            void (async () => {
              try {
                const hasPermission = await isPermissionGranted();
                if (!hasPermission) {
                  const permission = await requestPermission();
                  if (permission !== "granted") return;
                }
                const failure = tauriTurnFailure(event);
                sendNotification({
                  title: "Reasonix",
                  body: failure ? `生成失败：${failure}` : "回复已完成",
                });
              } catch {
                // Notification is best-effort
              }
            })();
            void (async () => {
              // Refresh state and transcript independently. A transcript parse
              // failure must not leave a completed turn marked as running.
              let completionError = tauriTurnFailure(event);
              try {
                const latest = await tauriBridgeSnapshot(event.sessionId);
                if (!active) return;
                setSession(latest.session);
                // Ensure session state is explicitly set to idle after turn_done
                if (latest.session.state !== "idle") {
                  setSession(prev => prev ? { ...prev, state: "idle" } : prev);
                }
              } catch (snapshotError) {
                completionError ||= tauriMessageFrom(snapshotError);
                // Even on snapshot failure, mark session as idle
                setSession(prev => prev ? { ...prev, state: "idle" } : prev);
              }
              try {
                const latestHistory = await tauriBridgeHistory(event.sessionId);
                if (!active) return;
                setHistory(latestHistory);
                setPendingUserMessage(null); // Clear optimistic message
                setHistoryError("");
              } catch (historyError) {
                if (!active) return;
                const message = tauriMessageFrom(historyError);
                setHistoryError(message);
                completionError ||= message;
              } finally {
                if (!active) return;
                setHistoryLoading(false);
                setLiveText("");
                // A failed turn must keep its reason on screen; only a
                // completed turn clears a previous message.
                setError(completionError);
              }
            })();
          }
        });
        if (!active) { offEvent(); return; }
        offError = await onTauriBridgeConnectionError(message => {
          if (!active) return;
          streamDisconnected = true;
          setStreamReady(false);
          setError(`本地桥接事件流：${message}`);
        });
        if (!active) { offError(); return; }
        offRestored = await onTauriBridgeConnectionRestored(() => {
          if (!active || resyncRequested) return;
          streamDisconnected = false;
          setStreamReady(true);
          setError(previous => previous.startsWith("本地桥接事件流：") ? "" : previous);
          void tauriBridgeStatus().then(next => { if (active) setStatus(next); }).catch(() => {});
        });
        if (!active) { offRestored(); return; }
        offResync = await onTauriBridgeResyncRequired(() => {
          if (!active || resyncRequested) return;
          resyncRequested = true;
          active = false;
          setStreamReady(false);
          setLiveText("");
          setStreamRevision(previous => previous + 1);
        });
        if (!active) { offResync(); return; }
        await startTauriBridgeEvents(snapshot.sequence);
        if (!active || resyncRequested) return;
        // Re-emit an approval/ask/MCP prompt after the listener is live. This
        // is what makes a paused historical session actionable after restart.
        try {
          const replayed = await replayTauriPendingPrompts(session.id);
          if (active) setSession(replayed);
        } catch (replayError) {
          if (active) setError(tauriMessageFrom(replayError));
        }
        if (!active) return;
        if (!streamDisconnected) setError("");
        setStreamReady(!streamDisconnected);
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
        offEvent?.();
        offError?.();
        offRestored?.();
        offResync?.();
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
      offRestored?.();
      offResync?.();
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
      setPendingPrompt(null);
      setPromptSelections({});
      setHistoryLoading(true);
      setHistoryError("");
      setLiveText("");
      setPendingUserMessage(null);
      setSequence(0);
      setStreamReady(false);
      setTitleEditing(false);
      setWorkspaceOpen(false);
      setWorkspacePath("");
      setWorkspaceEntries([]);
      setWorkspacePreview(null);
      setWorkspacePreviewLoading(false);
      setWorkspaceView("files");
      setWorkspaceChanges(null);
      setWorkspaceChangeDetail(null);
      setWorkspaceChangeDetailPath("");
      setWorkspaceChangesLoading(false);
      setWorkspaceChangeDetailLoading(false);
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

  async function loadWorkspace(path = "") {
    if (!session) return;
    const sessionID = session.id;
    setWorkspaceLoading(true);
    setWorkspaceError("");
    try {
      const listing = await tauriWorkspace(sessionID, path);
      if (sessionID !== session?.id) return;
      setWorkspacePath(listing.path);
      setWorkspaceEntries(listing.entries);
      setWorkspaceTruncated(listing.truncated);
      setWorkspacePreview(null);
    } catch (cause) {
      if (sessionID === session?.id) setWorkspaceError(tauriMessageFrom(cause));
    } finally {
      if (sessionID === session?.id) setWorkspaceLoading(false);
    }
  }

  async function loadWorkspaceChanges() {
    if (!session) return;
    const sessionID = session.id;
    setWorkspaceChangesLoading(true);
    setWorkspaceError("");
    try {
      const changes = await tauriWorkspaceChanges(sessionID);
      if (sessionID !== session?.id) return;
      setWorkspaceChanges(changes);
      setWorkspaceChangeDetail(null);
    } catch (cause) {
      if (sessionID === session?.id) setWorkspaceError(tauriMessageFrom(cause));
    } finally {
      if (sessionID === session?.id) setWorkspaceChangesLoading(false);
    }
  }

  async function loadWorkspaceChangeDetail(path: string) {
    if (!session) return;
    const sessionID = session.id;
    setWorkspaceChangeDetailLoading(true);
    setWorkspaceError("");
    try {
      const detail = await tauriWorkspaceChangeDetail(sessionID, path);
      if (sessionID === session?.id) {
        setWorkspaceChangeDetail(detail);
        setWorkspaceChangeDetailPath(path);
      }
    } catch (cause) {
      if (sessionID === session?.id) setWorkspaceError(tauriMessageFrom(cause));
    } finally {
      if (sessionID === session?.id) setWorkspaceChangeDetailLoading(false);
    }
  }

  function toggleWorkspace() {
    if (!session || busy) return;
    const nextOpen = !workspaceOpen;
    setWorkspaceOpen(nextOpen);
    if (nextOpen) void loadWorkspace(workspacePath);
  }

  function showWorkspaceView(view: "files" | "changes") {
    setWorkspaceView(view);
    setWorkspaceError("");
    if (view === "files") {
      setWorkspaceChangeDetail(null);
      void loadWorkspace(workspacePath);
    } else {
      setWorkspacePreview(null);
      void loadWorkspaceChanges();
    }
  }

  function workspaceParent(path: string): string {
    const parts = path.split("/").filter(Boolean);
    parts.pop();
    return parts.join("/");
  }

  function insertWorkspacePath(path: string) {
    const reference = `@${path}`;
    setPrompt(previous => previous.trim() ? `${previous.trim()}\n\n${reference}` : reference);
  }

  function insertWorkspaceReference(entry: TauriWorkspaceEntry) {
    if (entry.isDir) {
      void loadWorkspace(entry.path);
      return;
    }
    insertWorkspacePath(entry.path);
  }

  async function previewWorkspaceFile(entry: TauriWorkspaceEntry) {
    if (entry.isDir || !session || busy) return;
    const sessionID = session.id;
    setWorkspacePreviewLoading(true);
    setWorkspaceError("");
    try {
      const preview = await tauriWorkspaceFile(sessionID, entry.path);
      if (sessionID === session?.id) setWorkspacePreview(preview);
    } catch (cause) {
      if (sessionID === session?.id) setWorkspaceError(tauriMessageFrom(cause));
    } finally {
      if (sessionID === session?.id) setWorkspacePreviewLoading(false);
    }
  }

  async function addAttachments() {
    if (!session || !streamReady || busy || session.state !== "idle") return;
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
    const sessionId = session.id;
    const userText = prompt.trim();
    setBusy(true);
    setError("");
    setLiveText("");
    // Show user message immediately (optimistic update)
    setPendingUserMessage(userText || null);
    try {
      const submitted = await submitTauriBridge(sessionId, input);
      if (sessionId !== session?.id) return;
      setSession(submitted);
      setPrompt("");
      setAttachments([]);
      // Fetch history after a short delay to let the backend settle
      await new Promise(resolve => setTimeout(resolve, 100));
      try {
        const latestHistory = await tauriBridgeHistory(sessionId);
        if (sessionId === session?.id) {
          setHistory(latestHistory);
          setPendingUserMessage(null); // Clear optimistic message when real history arrives
        }
      } catch {
        // History fetch failure after submit is non-fatal; events will update
      }
    } catch (cause) {
      setError(tauriMessageFrom(cause));
      setPendingUserMessage(null);
    } finally {
      setBusy(false);
    }
  }

  function selectPromptOption(questionID: string, label: string, multi: boolean) {
    setPromptSelections(previous => {
      const current = previous[questionID] ?? [];
      if (multi) {
        return { ...previous, [questionID]: current.includes(label) ? current.filter(item => item !== label) : [...current, label] };
      }
      return { ...previous, [questionID]: current[0] === label ? [] : [label] };
    });
  }

  async function answerApproval(allow: boolean) {
    if (!session || !pendingPrompt || pendingPrompt.kind !== "approval") return;
    setBusy(true);
    setError("");
    try {
      setSession(await approveTauriBridge(session.id, pendingPrompt.id, allow));
      setPendingPrompt(null);
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function answerAsk() {
    if (!session || !pendingPrompt || pendingPrompt.kind !== "ask") return;
    setBusy(true);
    setError("");
    try {
      const answers = pendingPrompt.questions.map(question => ({ questionId: question.id, selected: promptSelections[question.id] ?? [] }));
      setSession(await answerTauriQuestion(session.id, pendingPrompt.id, answers));
      setPendingPrompt(null);
      setPromptSelections({});
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function answerMCP(action: "accept" | "decline" | "cancel") {
    if (!session || !pendingPrompt || pendingPrompt.kind !== "mcp") return;
    setBusy(true);
    setError("");
    try {
      setSession(await answerTauriMCPInteraction(session.id, pendingPrompt.id, action));
      setPendingPrompt(null);
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function cancel() {
    if (!session) return;
    const sessionId = session.id;
    setBusy(true);
    setError("");
    try {
      const cancelled = await cancelTauriBridge(sessionId);
      setSession(cancelled);
      setLiveText("");
      setPendingPrompt(null);
      setPromptSelections({});
      // Refresh history after cancel
      try {
        const latestHistory = await tauriBridgeHistory(sessionId);
        setHistory(latestHistory);
      } catch {
        // Non-fatal
      }
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function restartBridge(): Promise<boolean> {
    if (busy) return false;
    setBusy(true);
    setError("");
    try {
      if (session) {
        const current = await tauriBridgeSnapshot(session.id);
        if (current.session.state !== "idle") {
          setError("请等待当前回合结束，再重启桥接服务。");
          return false;
        }
        if (attachments.length > 0) {
          setError("请先处理待发送的附件，再重启桥接服务。");
          return false;
        }
      }
      setStreamReady(false);
      setStatus(await restartTauriBridge());
      if (session) {
        const reopened = await openTauriBridgeSession(session.id, session.workspaceRoot);
        setSession(reopened);
        await rememberSession(reopened);
        setEvents([]);
        setHistory(null);
        setAttachments([]);
        setPendingPrompt(null);
        setPromptSelections({});
        setLiveText("");
        setSequence(0);
        setStreamRevision(previous => previous + 1);
      }
      setProviderSummary(await tauriProviderSummary());
      return true;
    } catch (cause) {
      setStatus(await tauriBridgeStatus().catch(() => ({ running: false })));
      setError(tauriMessageFrom(cause));
      return false;
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
    <main className="tauri-shell" data-platform={platform}>
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
          <button type="button" className="tauri-sidebar__diagnostics" onClick={() => setSettingsOpen(true)}><Settings size={15} />设置</button>
          <button type="button" className="tauri-sidebar__diagnostics" onClick={() => setDiagnosticsOpen(true)}><Activity size={15} />运行状态</button>
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
            <span>{session?.state === "running" ? "正在生成" : session?.state === "paused" ? "等待你的操作" : session ? "本地会话" : "Reasonix Preview"}</span>
          </div>
          <div className="tauri-topbar__actions">
            <button type="button" className="tauri-workspace-button" onClick={() => void chooseWorkspaceRoot()} disabled={busy || session?.state === "running"} title={currentWorkspace || "选择工作区（用于新对话）"}>
              <FolderOpen size={15} /><span>{currentWorkspace || "选择工作区"}</span><ChevronDown size={13} />
            </button>
            <button type="button" className="tauri-icon-button tauri-workspace-tree-button" aria-label="浏览工作区文件" title="浏览工作区文件" onClick={toggleWorkspace} disabled={busy || !session}>
              <FolderTree size={17} />
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

        <div className="tauri-conversation" ref={conversationRef}>
          {session && !history && historyLoading ? <div className="tauri-loading"><span /><p>正在载入对话…</p></div> : session && !history && historyError ? <div className="tauri-loading tauri-history-error"><p>无法载入对话记录</p><p>{historyError}</p><button type="button" className="tauri-diagnostic-action" onClick={() => void refreshHistory()} disabled={busy}>重新加载</button></div> : history?.messages.length || liveText || pendingUserMessage || session?.state === "running" ? <div className="tauri-transcript">
            {history && history.startIndex > 0 && <p className="tauri-history-note">当前显示最近 {history.messages.length} 条，共 {history.totalMessages} 条可见消息</p>}
            {history?.messages.map((message, index) => {
              const display = message.role === "user" ? parseAttachmentRefsForDisplay(message.content) : null;
              const question = message.role === "user" ? questions.find(item => item.id === `tauri-question-${history.startIndex + index}`) : undefined;
              return <MessageErrorBoundary key={`${history.startIndex + index}-${message.role}`} index={index}>
                <article id={question?.id} data-tauri-question-anchor={question?.id} className={`tauri-message is-${message.role}`}>
                  <div className="tauri-message__avatar" aria-hidden="true">{message.role === "user" ? "你" : <Sparkles size={16} />}</div>
                  <div className="tauri-message__content">
                    <div className="tauri-message__role">{message.role === "user" ? "你" : "Reasonix"}</div>
                    <Markdown text={display?.text ?? message.content ?? ""} cacheKey={`${history.session.id}:${history.startIndex + index}`} />
                    {display && display.attachments.length > 0 && <div className="tauri-message__attachments">{display.attachments.map(attachment => <span key={attachment.path} title={attachment.path}><Paperclip size={13} />{attachment.name}</span>)}</div>}
                    {message.truncated && <small>为保护界面性能，这条历史内容已截断。</small>}
                  </div>
                </article>
              </MessageErrorBoundary>;
            })}
            {/* Optimistic user message: shown immediately after submit, before history loads */}
            {pendingUserMessage && <article className="tauri-message is-user"><div className="tauri-message__avatar" aria-hidden="true">你</div><div className="tauri-message__content"><div className="tauri-message__role">你</div><Markdown text={pendingUserMessage} /></div></article>}
            {liveText && <article className="tauri-message is-assistant tauri-message--live"><div className="tauri-message__avatar" aria-hidden="true"><Sparkles size={16} /></div><div className="tauri-message__content"><div className="tauri-message__role">Reasonix</div><Markdown text={liveText || ""} streaming cacheKey={`${session?.id ?? "live"}:stream`} /></div></article>}
            {session?.state === "running" && !liveText && !pendingUserMessage && <div className="tauri-thinking" role="status"><span /><span /><span />Reasonix 正在思考…</div>}
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

        {questions.length >= 2 && <QuestionJumpBar loadedQuestions={questions} totalQuestions={questions.length} activeTurn={activeQuestion} onJump={jumpToQuestion}
          height={Math.min(240, Math.max(48, questions.length * 18 + 12))} />}

        {pendingPrompt && <PromptCard prompt={pendingPrompt} busy={busy} selections={promptSelections} onApproval={allow => void answerApproval(allow)} onAskSelection={selectPromptOption} onAskSubmit={() => void answerAsk()} onMCPAction={action => void answerMCP(action)} />}

        <footer className="tauri-composer-area">
          {error && <p className="tauri-error" role="alert">{error}</p>}
          <div className={`tauri-composer${dragging ? " is-dragging" : ""}`}>
            {attachments.length > 0 && <div className="tauri-composer__attachments" aria-label="已添加文件">{attachments.map((attachment, index) => <div className="tauri-composer__attachment" key={`${attachment.path}-${index}`} title={attachment.path}>
              <span className="tauri-composer__attachment-icon"><FileText size={15} /></span><span className="tauri-composer__attachment-name">{attachment.name}</span><small>{attachment.size < 1024 ? `${attachment.size} B` : `${(attachment.size / 1024).toFixed(1)} KB`}</small>
              <button type="button" onClick={() => setAttachments(previous => previous.filter((_, itemIndex) => itemIndex !== index))} disabled={busy} aria-label={`移除文件 ${attachment.name}`}><X size={13} /></button>
            </div>)}</div>}
            <textarea value={prompt} onChange={event => setPrompt(event.target.value)} onKeyDown={event => {
              if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) { event.preventDefault(); void submit(); }
            }} placeholder={session?.state === "paused" ? "请先完成上方确认…" : session ? "继续聊聊你的问题…（⌘/Ctrl + Enter 发送）" : "先开始一个新对话，再输入你的问题"} disabled={busy || !session || !streamReady || session.state === "paused"} rows={3} />
            <div className="tauri-composer__bottom"><span>{session?.workspaceRoot ? `工作区 · ${session.workspaceRoot}` : session ? "当前会话使用默认工作区" : "Preview 配置与稳定版相互隔离"}</span>
              <div className="tauri-composer__actions">
                <button className="tauri-attach-button" type="button" onClick={() => void addAttachments()} disabled={busy || !session || !streamReady || session.state !== "idle"} aria-label="添加文件" title="从本机选择文件并附加到消息"><Paperclip size={16} /><span>添加文件</span></button>
                {session?.state === "running" ? <button className="tauri-send-button is-stop" type="button" onClick={() => void cancel()} disabled={busy} aria-label="停止生成"><Square size={15} fill="currentColor" /></button> : <button className="tauri-send-button" type="button" onClick={() => void submit()} disabled={busy || !streamReady || session?.state === "paused" || (!prompt.trim() && attachments.length === 0)} aria-label="发送消息"><ArrowUp size={18} /></button>}
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

      {workspaceOpen && <>
        <button className="tauri-workspace-drawer__scrim" aria-label="关闭工作区文件" type="button" onClick={() => setWorkspaceOpen(false)} />
        <aside className="tauri-workspace-drawer" aria-label="工作区文件">
          <header className="tauri-workspace-drawer__header">
            <div><p>WORKSPACE</p><h2>{workspaceView === "files" ? "文件" : "变更"}</h2><span title={currentWorkspace}>{workspaceView === "files" ? (workspacePath ? `/${workspacePath}` : "工作区根目录") : (workspaceChanges?.gitBranch ? `Git · ${workspaceChanges.gitBranch}` : "Git 工作区")}</span></div>
            <button type="button" className="tauri-icon-button" onClick={() => setWorkspaceOpen(false)} aria-label="关闭"><X size={17} /></button>
          </header>
          <div className="tauri-workspace-drawer__body">
            <div className="tauri-workspace-drawer__toolbar">
              <button type="button" className={`tauri-diagnostic-action${workspaceView === "files" ? " is-active" : ""}`} onClick={() => showWorkspaceView("files")} disabled={workspaceView === "files" && workspaceLoading}><FolderOpen size={13} /> 文件</button>
              <button type="button" className={`tauri-diagnostic-action${workspaceView === "changes" ? " is-active" : ""}`} onClick={() => showWorkspaceView("changes")} disabled={workspaceView === "changes" && workspaceChangesLoading}><GitBranch size={13} /> 变更</button>
              {workspaceView === "files" ? <><button type="button" className="tauri-diagnostic-action" onClick={() => void loadWorkspace(workspacePath)} disabled={workspaceLoading}>刷新</button><button type="button" className="tauri-diagnostic-action" onClick={() => void loadWorkspace(workspaceParent(workspacePath))} disabled={workspaceLoading || !workspacePath}>返回上级</button></> : <button type="button" className="tauri-diagnostic-action" onClick={() => void loadWorkspaceChanges()} disabled={workspaceChangesLoading}>刷新</button>}
            </div>
            {workspaceError && <p className="tauri-workspace-drawer__error">{workspaceError}</p>}
            {workspaceView === "files" ? <>
              {workspaceLoading ? <div className="tauri-workspace-drawer__loading">正在读取文件…</div> : workspaceEntries.length === 0 ? <div className="tauri-workspace-drawer__empty">此目录没有可展示的文件。</div> : <ul className="tauri-workspace-drawer__entries">
                {workspaceEntries.map(entry => <li key={entry.path}><button type="button" className="tauri-workspace-entry" onClick={() => insertWorkspaceReference(entry)} onDoubleClick={() => void previewWorkspaceFile(entry)} title={entry.isDir ? `打开 ${entry.name}` : `单击插入 @${entry.path}，双击预览`}>
                  <span className="tauri-workspace-entry__icon">{entry.isDir ? <FolderOpen size={15} /> : <FileText size={15} />}</span><span className="tauri-workspace-entry__name">{entry.name}</span>{entry.isDir && <ChevronRight size={14} />}
                </button></li>)}
              </ul>}
              {workspacePreviewLoading && <div className="tauri-workspace-preview__loading">正在读取预览…</div>}
              {workspacePreview && <section className="tauri-workspace-preview" aria-label="文件预览">
                <header><div><strong>{workspacePreview.path}</strong><span>{workspacePreview.size} bytes{workspacePreview.truncated ? " · 已截断" : ""}</span></div><button type="button" className="tauri-diagnostic-action" onClick={() => insertWorkspacePath(workspacePreview.path)}><Eye size={13} /> 插入引用</button></header>
                {workspacePreview.binary ? <p className="tauri-workspace-preview__binary">这是二进制文件，已隐藏内容；仍可插入它的 @路径。</p> : <pre>{workspacePreview.body || "（空文件）"}</pre>}
              </section>}
              {workspaceTruncated && <p className="tauri-workspace-drawer__note">目录较大，仅显示前 200 项。</p>}
              <p className="tauri-workspace-drawer__hint">单击文件把 <code>@路径</code> 插入输入框，双击文件查看安全预览。</p>
            </> : <>
              {workspaceChangesLoading ? <div className="tauri-workspace-drawer__loading">正在读取变更…</div> : workspaceChanges?.files.length ? <>
                {workspaceChanges && !workspaceChanges.gitAvailable && <p className="tauri-workspace-drawer__note">Git 不可用，仅显示 Reasonix 本轮会话检查点：{workspaceChanges.gitErr || "未检测到仓库"}</p>}
                <ul className="tauri-workspace-drawer__entries">
                {workspaceChanges.files.map(change => <li key={change.path}><button type="button" className="tauri-workspace-entry tauri-workspace-change-entry" onClick={() => void loadWorkspaceChangeDetail(change.path)} title={`查看 ${change.path} 的差异`}>
                  <span className="tauri-workspace-entry__icon"><GitBranch size={15} /></span><span className="tauri-workspace-entry__name">{change.path}</span><small>{workspaceChangeLabel(change.gitStatus, change.sources)}{change.sources?.length > 1 ? " · Git+本轮" : change.sources?.includes("session") ? ` · 第${change.turns && change.turns.length > 0 ? change.turns[change.turns.length - 1] : "?"}轮` : ""}</small>
                </button></li>)}
                </ul>
              </> : <div className="tauri-workspace-drawer__empty">当前没有工作区变更。</div>}
              {workspaceChangeDetailLoading && <div className="tauri-workspace-preview__loading">正在读取差异…</div>}
              {workspaceChangeDetail && <section className="tauri-workspace-preview" aria-label="文件差异">
                <header><div><strong>{workspaceChangeDetailPath}</strong><span>{workspaceChangeDetail.source === "session" ? "本轮会话" : "Git"} · {workspaceChangeDetail.added ?? 0} 新增 · {workspaceChangeDetail.removed ?? 0} 删除{workspaceChangeDetail.truncated ? " · 已截断" : ""}</span></div><button type="button" className="tauri-diagnostic-action" onClick={() => insertWorkspacePath(workspaceChangeDetailPath)}><Eye size={13} /> 引用文件</button></header>
                {workspaceChangeDetail.binary ? <p className="tauri-workspace-preview__binary">这是二进制变更，无法显示文本差异。</p> : <pre>{workspaceChangeDetail.diff || "（没有可显示的文本差异）"}</pre>}
              </section>}
              <p className="tauri-workspace-drawer__hint">变更来自 Git 或本轮会话检查点；点击文件查看受限差异。</p>
            </>}
          </div>
        </aside>
      </>}

      {settingsOpen && <TauriSettings onClose={() => setSettingsOpen(false)} onProviderSummaryChange={setProviderSummary} currentSessionState={session?.state} currentSessionHasAttachments={attachments.length > 0} onApplyToCurrentSession={restartBridge} />}
    </main>
  );
}

/** The standalone Tauri entry owns the same localization context as Wails. */
export function TauriSessionApp() {
  return (
    <LocaleProvider>
      <TauriSessionPreview />
    </LocaleProvider>
  );
}
