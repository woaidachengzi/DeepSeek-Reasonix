import { Component, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
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
import { handleTauriDragDropEvent, retainTauriDragDropListener } from "./dragDrop";

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
  backfillTauriWorkbenchTitles,
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
  deleteTauriMCPServer,
  saveTauriMCPServer,
  tauriMCPServers,
  tauriMessageFrom,
  tauriPromptAnsweredId,
  tauriPromptFromEvent,
  tauriProviderSummary,
  tauriPlatformInfo,
  tauriPreviewProfileStatus,
  tauriPreviewRuntimeInfo,
  tauriSessionTitle,
  tauriWorkbenchSessionPage,
  tauriSessionPreviews,
  tauriSafeMCPURL,
  tauriTitleError,
  tauriTurnFailure,
  tauriImportLegacySessionCatalog,
  tauriSessionCatalogShadow,
  type TauriMCPServer,
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
  type TauriSessionShadowReport,
} from "../lib/tauriBridge";
import { groupWorkbenchSessions, titleFromFirstUser } from "./workbenchSessions";
import { sessionLifecycleFailure, sessionLifecycleNotice } from "./sessionLifecycleError";
import {
  emptyMCPDraft,
  mcpCredentialHint,
  mcpDraftForEditing,
  mcpDraftToInput,
  mcpTransportSummary,
  type MCPDraft,
} from "./tauriMCPServers";
import "./tauriChatWorkspace.css";

interface WorkbenchSessionTab {
  sessionId: string;
  title?: string;
  workspaceRoot?: string;
}

/** A stored title is authoritative; anything the bridge would reject falls back
 *  to the catalog title or a neutral label instead of rendering bad host state. */
function displayTitle(storedTitle: string | undefined, catalogTitle?: string): string {
  return tauriSessionTitle(storedTitle, tauriSessionTitle(catalogTitle, "新对话"));
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
        <span className="tauri-session-delete__question">删除“{displayTitle(tab.title)}”？</span>
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
        <span>{displayTitle(tab.title)}</span>
      </button>
      <button
        type="button"
        className="tauri-session-row__delete"
        aria-label={`删除对话 ${displayTitle(tab.title)}`}
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
  const safeURL = tauriSafeMCPURL(prompt.url);
  const linkHint = prompt.mode !== "url" ? "这是一个外部服务请求" : safeURL ? "打开链接完成后再继续" : "链接不可安全打开，请检查服务配置";
  return <section className="tauri-prompt-card" aria-live="polite" aria-label="MCP 服务请求">
    <div className="tauri-prompt-card__heading"><Sparkles size={17} /><div><strong>{prompt.server} 请求你的操作</strong><span>{linkHint}</span></div></div>
    {prompt.message && <p className="tauri-prompt-card__subject">{prompt.message}</p>}
    {safeURL && <a className="tauri-prompt-card__link" href={safeURL} target="_blank" rel="noreferrer">打开外部链接（{new URL(safeURL).host}）</a>}
    <div className="tauri-prompt-card__actions"><button type="button" className="tauri-prompt-card__allow" onClick={() => onMCPAction("accept")} disabled={busy}>接受并继续</button><button type="button" className="tauri-prompt-card__deny" onClick={() => onMCPAction("decline")} disabled={busy}>拒绝</button><button type="button" className="tauri-prompt-card__cancel" onClick={() => onMCPAction("cancel")} disabled={busy}>取消</button></div>
  </section>;
}

export function TauriSessionPreview() {
  const [session, setSession] = useState<TauriBridgeSession | null>(null);
  const [tabs, setTabs] = useState<WorkbenchSessionTab[]>([]);
  const [sessionPageCursor, setSessionPageCursor] = useState<{ position: number; id: string } | null>(null);
  const [sessionPageSource, setSessionPageSource] = useState<"identity" | "legacy">("identity");
  const [sessionPageLoading, setSessionPageLoading] = useState(false);
  const [sessionPageError, setSessionPageError] = useState("");
  const [collapsedProjects, setCollapsedProjects] = useState<Record<string, boolean>>({});
  const [workspaceRoot, setWorkspaceRoot] = useState("");
  const [prompt, setPrompt] = useState("");
  const [attachments, setAttachments] = useState<TauriBridgeAttachment[]>([]);
  const [status, setStatus] = useState<TauriBridgeStatus | null>(null);
  const [profile, setProfile] = useState<TauriPreviewProfileStatus | null>(null);
  const [runtimeInfo, setRuntimeInfo] = useState<TauriPreviewRuntimeInfo | null>(null);
  const [providerSummary, setProviderSummary] = useState<TauriProviderSummary | null>(null);
  const [profileNotice, setProfileNotice] = useState("");
  const [mcpServers, setMcpServers] = useState<TauriMCPServer[]>([]);
  const [mcpNotice, setMcpNotice] = useState("");
  const [mcpDraft, setMcpDraft] = useState<MCPDraft | null>(null);
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
  const [catalogAudit, setCatalogAudit] = useState<TauriSessionShadowReport | null>(null);
  const [catalogAuditError, setCatalogAuditError] = useState("");
  const catalogAuditRequestRef = useRef(0);
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
  const turnEpochRef = useRef(0);
  const submitInFlightRef = useRef(false);
  const workspaceEpochRef = useRef(0);
  const workspaceListRequestRef = useRef(0);
  const workspaceChangesRequestRef = useRef(0);
  const workspacePreviewRequestRef = useRef(0);
  const workspaceDetailRequestRef = useRef(0);
  const [activeQuestion, setActiveQuestion] = useState<number | null>(null);
  const switchingBlocked = Boolean(session && session.state !== "idle");
  const [platform, setPlatform] = useState<string>("");
  // Optimistic user message: displayed immediately after submit, cleared when history loads
  const [pendingUserMessage, setPendingUserMessage] = useState<string | null>(null);
  // Drag-and-drop state
  const [dragging, setDragging] = useState(false);
  const projectGroups = useMemo(() => groupWorkbenchSessions(tabs), [tabs]);
  const activeCatalogTitle = tabs.find(tab => tab.sessionId === session?.id)?.title;

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
    void tauriMCPServers().then(setMcpServers).catch(error => setMcpNotice(tauriMessageFrom(error)));
    let active = true;
    void (async () => {
      try {
        // One-time, idempotent migration. Prefer the identity directory after
        // import, but retain the JSON catalog as an initial-load fallback.
        try { await tauriImportLegacySessionCatalog(); } catch { /* keep legacy catalog usable while the bridge is unavailable */ }
        if (!active) return;
        const page = await tauriWorkbenchSessionPage();
        if (!active) return;
        setTabs(page.sessions);
        setSessionPageCursor(page.nextCursor ?? null);
        setSessionPageSource(page.source);
        const missing = page.sessions.filter(tab => !tauriSessionTitle(tab.title, "")).map(tab => tab.sessionId);
        for (let offset = 0; active && offset < missing.length; offset += 50) {
          try {
            const previews = await tauriSessionPreviews(missing.slice(offset, offset + 50));
            if (!active) return;
            const titles = previews.flatMap(preview => {
              const title = tauriSessionTitle(preview.title, "") || titleFromFirstUser(preview.firstUser ?? "");
              return title ? [{ sessionId: preview.sessionId, title }] : [];
            });
            if (titles.length > 0) {
              await backfillTauriWorkbenchTitles(titles);
              if (active) setTabs(previous => previous.map(tab => {
                const title = titles.find(item => item.sessionId === tab.sessionId)?.title;
                return title ? { ...tab, title } : tab;
              }));
            }
          } catch {
            // Legacy enrichment is best-effort. The catalog remains usable,
            // and another launch can retry this batch without changing order.
          }
        }
        await refreshCatalogAudit(() => active);
      } catch (cause) {
        if (active) setError(`读取最近对话失败：${tauriMessageFrom(cause)}`);
      }
    })();
    void tauriPlatformInfo().then(setPlatform).catch(() => {});
    return () => { active = false; };
  }, []);

  async function loadMoreWorkbenchSessions() {
    const cursor = sessionPageCursor;
    if (!cursor || sessionPageRequestRef.current) return;
    sessionPageRequestRef.current = true;
    setSessionPageLoading(true);
    setSessionPageError("");
    try {
      const page = await tauriWorkbenchSessionPage(cursor);
      setTabs(previous => {
        const known = new Set(previous.map(tab => tab.sessionId));
        return [...previous, ...page.sessions.filter(tab => !known.has(tab.sessionId))];
      });
      setSessionPageCursor(page.nextCursor ?? null);
      setSessionPageSource(page.source);
      const missing = page.sessions.filter(tab => !tauriSessionTitle(tab.title, "")).map(tab => tab.sessionId);
      for (let offset = 0; offset < missing.length; offset += 50) {
        try {
          const previews = await tauriSessionPreviews(missing.slice(offset, offset + 50));
          const titles = previews.flatMap(preview => {
            const title = tauriSessionTitle(preview.title, "") || titleFromFirstUser(preview.firstUser ?? "");
            return title ? [{ sessionId: preview.sessionId, title }] : [];
          });
          if (titles.length > 0) {
            await backfillTauriWorkbenchTitles(titles);
            setTabs(previous => previous.map(tab => {
              const title = titles.find(item => item.sessionId === tab.sessionId)?.title;
              return title ? { ...tab, title } : tab;
            }));
          }
        } catch {
          // Keep the page visible; title enrichment can be retried on a later launch.
        }
      }
    } catch (cause) {
      setSessionPageError(tauriMessageFrom(cause));
    } finally {
      sessionPageRequestRef.current = false;
      setSessionPageLoading(false);
    }
  }

  // Tauri drag-and-drop file handler
  useLayoutEffect(() => {
    let active = true;
    let unlisten: UnlistenFn | undefined;
    const canAttach = Boolean(session && streamReady && !busy && session.state === "idle");
    void (async () => {
      try {
        unlisten = await retainTauriDragDropListener(getCurrentWindow().onDragDropEvent(async (event) => {
          if (!active) return;
          await handleTauriDragDropEvent(event.payload, {
            sessionId: canAttach ? session?.id : undefined,
            attachFile: attachTauriFile,
            addAttachment: attached => setAttachments(previous => [...previous, attached]),
            setDragging,
            isCurrent: () => active,
          });
        }), () => active);
      } catch {
        // Drag-drop not supported in browser mode
      }
    })();
    return () => { active = false; unlisten?.(); };
  }, [session?.id, session?.state, streamReady, busy]);

  useEffect(() => {
    if (!session) return;
    let active = true;
    let offEvent: UnlistenFn | undefined;
    let offError: UnlistenFn | undefined;
    let offRestored: UnlistenFn | undefined;
    let offResync: UnlistenFn | undefined;
    let resyncRequested = false;
    let streamDisconnected = true;
    let startRequested = false;
    setStreamReady(false);
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
          if (event.eventKind === "turn_started") {
            turnEpochRef.current += 1;
            setSession(previous => previous ? { ...previous, state: "running" } : previous);
            setLiveText("");
          }
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
            const completionEpoch = ++turnEpochRef.current;
            const completionIsCurrent = () => active && turnEpochRef.current === completionEpoch;
            setPendingPrompt(null);
            setPromptSelections({});
            setSession(previous => previous ? { ...previous, state: "running" } : previous);
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
              // failure must not prevent state reconciliation. The core can
              // still be in its finishing window while TurnDone fans out, so
              // only an authoritative idle snapshot may reopen admission.
              let completionError = tauriTurnFailure(event);
              let latestState: TauriBridgeSession["state"] | undefined;
              let snapshotError = "";
              for (let attempt = 0; attempt < 5; attempt += 1) {
                if (attempt > 0) await new Promise(resolve => setTimeout(resolve, 100));
                if (!completionIsCurrent()) return;
                try {
                  const latest = await tauriBridgeSnapshot(event.sessionId);
                  if (!completionIsCurrent()) return;
                  latestState = latest.session.state;
                  setSession(latest.session);
                  snapshotError = "";
                  if (latestState === "idle") break;
                } catch (cause) {
                  if (!completionIsCurrent()) return;
                  snapshotError = tauriMessageFrom(cause);
                }
              }
              if (latestState !== "idle") {
                const stateError = snapshotError
                  ? `无法确认当前会话状态：${snapshotError}；请刷新当前对话状态与记录`
                  : "回合结束事件已到达，但会话尚未空闲；请稍后刷新当前对话状态与记录";
                completionError = [completionError, stateError].filter(Boolean).join("；");
              }
              try {
                const latestHistory = await tauriBridgeHistory(event.sessionId);
                if (!completionIsCurrent()) return;
                setHistory(latestHistory);
                setPendingUserMessage(null); // Clear optimistic message
                setHistoryError("");
              } catch (historyError) {
                if (!completionIsCurrent()) return;
                const message = tauriMessageFrom(historyError);
                setHistoryError(message);
                completionError ||= message;
              } finally {
                if (!completionIsCurrent()) return;
                setHistoryLoading(false);
                setLiveText("");
                // A failed turn must keep its reason on screen; only a
                // completed turn clears a previous message. Do not erase a
                // newer event-stream outage while transcript refresh finishes.
                setError(previous => streamDisconnected && previous.startsWith("本地桥接事件流：") ? previous : completionError);
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
          if (!active || resyncRequested || !startRequested) return;
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
        startRequested = true;
        await startTauriBridgeEvents(snapshot.sequence);
        if (!active || resyncRequested) return;
        // Re-emit an approval/ask/MCP prompt after the listener is live. This
        // is what makes a paused historical session actionable after restart.
        const replayEpoch = turnEpochRef.current;
        try {
          const replayed = await replayTauriPendingPrompts(session.id);
          if (active && turnEpochRef.current === replayEpoch) setSession(replayed);
        } catch (replayError) {
          if (active && turnEpochRef.current === replayEpoch) setError(tauriMessageFrom(replayError));
        }
        if (!active) return;
        if (!streamDisconnected && turnEpochRef.current === replayEpoch) setError("");
        const historyEpoch = turnEpochRef.current;
        try {
          const latestHistory = await tauriBridgeHistory(session.id);
          if (!active) return;
          if (turnEpochRef.current === historyEpoch) {
            setHistory(latestHistory);
            setHistoryError("");
          }
        } catch (historyCause) {
          if (!active) return;
          if (turnEpochRef.current === historyEpoch) {
            const message = tauriMessageFrom(historyCause);
            setHistoryError(message);
            setError(message);
          }
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

  // Existing rows keep their sidebar position on reopen; only brand-new
  // sessions are prepended (matches WorkbenchCatalog::remember).
  async function rememberSession(next: TauriBridgeSession) {
    try {
      setTabs(await rememberTauriWorkbenchSession(next.id, next.workspaceRoot ?? undefined, tauriSessionTitle(next.title, "") || undefined));
      await refreshCatalogAudit();
    } catch (cause) {
      setError(`对话已打开，但无法保存到最近对话：${tauriMessageFrom(cause)}`);
    }
  }

  async function refreshCatalogAudit(isActive: () => boolean = () => true) {
    const request = ++catalogAuditRequestRef.current;
    setCatalogAudit(null);
    setCatalogAuditError("");
    try {
      const report = await tauriSessionCatalogShadow();
      if (request === catalogAuditRequestRef.current && isActive()) setCatalogAudit(report);
    } catch (cause) {
      if (request === catalogAuditRequestRef.current && isActive()) {
        setCatalogAuditError(tauriMessageFrom(cause));
      }
    }
  }

  function invalidateWorkspaceRequests() {
    workspaceEpochRef.current += 1;
    setWorkspaceLoading(false);
    setWorkspaceChangesLoading(false);
    setWorkspacePreviewLoading(false);
    setWorkspaceChangeDetailLoading(false);
  }

  async function activateSession(id: string, root?: string) {
    if (!id) return setError("缺少会话 ID");
    setBusy(true);
    setError("");
    try {
      const next = session && session.id !== id
        ? await switchTauriBridgeSession(id, root)
        : await openTauriBridgeSession(id, root);
      invalidateWorkspaceRequests();
      setEvents([]);
      setHistory(null);
      setAttachments([]);
      setDragging(false);
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
      turnEpochRef.current += 1;
      setSession(next);
      setWorkspaceRoot(next.workspaceRoot ?? root ?? "");
      await rememberSession(next);
      setStreamRevision(previous => previous + 1);
      setStatus(await tauriBridgeStatus());
    } catch (cause) {
      setError(sessionLifecycleNotice(cause) ?? tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  function beginTitleEdit() {
    if (!session || busy || switchingBlocked) return;
    setTitleDraft(displayTitle(session.title, activeCatalogTitle));
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
    invalidateWorkspaceRequests();
    setWorkspaceOpen(false);
    setDragging(false);
    let switchedToTarget = false;
    let deleted = false;
    let operationError = "";
    try {
      if (!isOpen) {
        try {
          await switchTauriBridgeSession(target.sessionId, target.workspaceRoot);
          switchedToTarget = true;
        } catch (cause) {
          // Missing and interrupted sessions cannot be reopened. The bridge
          // DELETE endpoint can retire a missing identity or resume its fenced
          // deletion without recreating a transcript.
          const failure = sessionLifecycleFailure(cause);
          if (failure !== "missing" && failure !== "deleting") throw cause;
        }
      }
      // For an interrupted deletion, avoid switching the single bridge
      // controller. DELETE resumes cleanup without recreating the transcript.
      await deleteTauriBridgeSession(target.sessionId);
      deleted = true;
      if (isOpen) {
        turnEpochRef.current += 1;
        setSession(null);
      }
      setTabs(await forgetTauriWorkbenchSession(target.sessionId));
      await refreshCatalogAudit();
    } catch (cause) {
      const userMessage = sessionLifecycleNotice(cause) ?? tauriMessageFrom(cause);
      operationError = deleted
        ? `对话已删除，但最近对话列表更新失败：${userMessage}`
        : userMessage;
    } finally {
      // The bridge owns one controller. Deleting an inactive row temporarily
      // switches that controller, so restore the user's open session even if
      // deletion or catalog cleanup fails.
      if (switchedToTarget && sessionToRestore) {
        try {
          turnEpochRef.current += 1;
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

  async function createSession(root = workspaceRoot) {
    await activateSession(newTauriSessionId(), root.trim() || undefined);
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
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceListRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceListRequestRef.current === request;
    workspacePreviewRequestRef.current += 1;
    setWorkspaceLoading(true);
    setWorkspacePreviewLoading(false);
    setWorkspacePreview(null);
    setWorkspaceError("");
    try {
      const listing = await tauriWorkspace(sessionID, path);
      if (!isCurrent()) return;
      setWorkspacePath(listing.path);
      setWorkspaceEntries(listing.entries);
      setWorkspaceTruncated(listing.truncated);
    } catch (cause) {
      if (isCurrent()) setWorkspaceError(tauriMessageFrom(cause));
    } finally {
      if (isCurrent()) setWorkspaceLoading(false);
    }
  }

  async function loadWorkspaceChanges() {
    if (!session) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceChangesRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceChangesRequestRef.current === request;
    workspaceDetailRequestRef.current += 1;
    setWorkspaceChangesLoading(true);
    setWorkspaceChangeDetailLoading(false);
    setWorkspaceChangeDetail(null);
    setWorkspaceError("");
    try {
      const changes = await tauriWorkspaceChanges(sessionID);
      if (!isCurrent()) return;
      setWorkspaceChanges(changes);
    } catch (cause) {
      if (isCurrent()) setWorkspaceError(tauriMessageFrom(cause));
    } finally {
      if (isCurrent()) setWorkspaceChangesLoading(false);
    }
  }

  async function loadWorkspaceChangeDetail(path: string) {
    if (!session) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceDetailRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceDetailRequestRef.current === request;
    setWorkspaceChangeDetailLoading(true);
    setWorkspaceChangeDetail(null);
    setWorkspaceError("");
    try {
      const detail = await tauriWorkspaceChangeDetail(sessionID, path);
      if (isCurrent()) {
        setWorkspaceChangeDetail(detail);
        setWorkspaceChangeDetailPath(path);
      }
    } catch (cause) {
      if (isCurrent()) setWorkspaceError(tauriMessageFrom(cause));
    } finally {
      if (isCurrent()) setWorkspaceChangeDetailLoading(false);
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
      workspaceChangesRequestRef.current += 1;
      setWorkspaceChangesLoading(false);
      setWorkspaceChangeDetail(null);
      void loadWorkspace(workspacePath);
    } else {
      workspaceListRequestRef.current += 1;
      workspacePreviewRequestRef.current += 1;
      setWorkspaceLoading(false);
      setWorkspacePreviewLoading(false);
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
    const epoch = workspaceEpochRef.current;
    const request = ++workspacePreviewRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspacePreviewRequestRef.current === request;
    setWorkspacePreviewLoading(true);
    setWorkspacePreview(null);
    setWorkspaceError("");
    try {
      const preview = await tauriWorkspaceFile(sessionID, entry.path);
      if (isCurrent()) setWorkspacePreview(preview);
    } catch (cause) {
      if (isCurrent()) setWorkspaceError(tauriMessageFrom(cause));
    } finally {
      if (isCurrent()) setWorkspacePreviewLoading(false);
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
    if (busy || session?.state === "paused" || !input || submitInFlightRef.current) return;
    // Typing may open a session asynchronously; send waits for that session
    // and for the event stream, same gate the send button already uses.
    if (!session || !streamReady || session.state !== "idle") return;
    submitInFlightRef.current = true;
    const sessionId = session.id;
    const userText = prompt.trim();
    const submitEpoch = ++turnEpochRef.current;
    setBusy(true);
    setError("");
    setLiveText("");
    // Show user message immediately (optimistic update)
    setPendingUserMessage(userText || null);
    try {
      const submitted = await submitTauriBridge(sessionId, input);
      if (sessionId !== session?.id) return;
      if (turnEpochRef.current === submitEpoch) setSession(submitted);
      if (!tauriSessionTitle(submitted.title, "") && !tabs.find(tab => tab.sessionId === sessionId)?.title) {
        const title = titleFromFirstUser(input);
        if (title) {
          try {
            setTabs(await backfillTauriWorkbenchTitles([{ sessionId, title }]));
          } catch {
            // The turn was accepted; catalog enrichment must not report it as a failed send.
          }
        }
      }
      setPrompt("");
      setAttachments([]);
      // Fetch history after a short delay to let the backend settle
      await new Promise(resolve => setTimeout(resolve, 100));
      try {
        const latestHistory = await tauriBridgeHistory(sessionId);
        if (sessionId === session?.id && turnEpochRef.current === submitEpoch) {
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
      submitInFlightRef.current = false;
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
      invalidateWorkspaceRequests();
      setDragging(false);
      setStreamReady(false);
      setStatus(await restartTauriBridge());
      if (session) {
        const reopened = await openTauriBridgeSession(session.id, session.workspaceRoot);
        turnEpochRef.current += 1;
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
    const refreshEpoch = ++turnEpochRef.current;
    const sessionId = session.id;
    setBusy(true);
    setError("");
    setHistoryLoading(true);
    setHistoryError("");
    let refreshError = "";
    try {
      try {
        const latest = await tauriBridgeSnapshot(sessionId);
        if (turnEpochRef.current !== refreshEpoch) return;
        setSession(latest.session);
      } catch (cause) {
        if (turnEpochRef.current !== refreshEpoch) return;
        refreshError = `刷新会话状态失败：${tauriMessageFrom(cause)}`;
      }
      try {
        const latestHistory = await tauriBridgeHistory(sessionId);
        if (turnEpochRef.current !== refreshEpoch) return;
        setHistory(latestHistory);
      } catch (cause) {
        if (turnEpochRef.current !== refreshEpoch) return;
        const message = tauriMessageFrom(cause);
        setHistoryError(message);
        refreshError = [refreshError, message].filter(Boolean).join("；");
      }
      setError(refreshError);
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

  async function refreshMCPServers() {
    try {
      setMcpServers(await tauriMCPServers(currentWorkspace || undefined));
      setMcpNotice("");
    } catch (cause) {
      setMcpNotice(tauriMessageFrom(cause));
    }
  }

  /** Credentials are write-only: an empty field keeps the stored value, so an
   *  edit never requires retyping a token and never reveals one. */
  async function saveMCPServer() {
    if (!mcpDraft) return;
    let input;
    try {
      input = mcpDraftToInput(mcpDraft);
    } catch (cause) {
      setMcpNotice(tauriMessageFrom(cause));
      return;
    }
    setBusy(true);
    try {
      const result = await saveTauriMCPServer(input, currentWorkspace || undefined);
      setMcpServers(result.servers);
      setMcpDraft(null);
      setMcpNotice("");
    } catch (cause) {
      setMcpNotice(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function removeMCPServer(server: TauriMCPServer) {
    setBusy(true);
    try {
      const result = await deleteTauriMCPServer(server.name, currentWorkspace || undefined);
      setMcpServers(result.servers);
      if (mcpDraft?.name === server.name) setMcpDraft(null);
      setMcpNotice("");
    } catch (cause) {
      setMcpNotice(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="tauri-shell" data-platform={platform}>
      <aside className="tauri-sidebar" aria-label="会话导航">
        <div className="tauri-sidebar__drag" data-tauri-drag-region aria-hidden="true" />
        <div className="tauri-sidebar__brand"><img src={logoWordmark} alt="Reasonix" draggable={false} /><span>PREVIEW</span></div>
        <button className="tauri-sidebar__new" type="button" onClick={() => void createSession()} disabled={busy || switchingBlocked}>
          <Plus size={17} aria-hidden="true" /><span>新建对话</span><kbd>⌘ N</kbd>
        </button>
        <div className="tauri-sidebar__section-title">项目</div>
        <nav className="tauri-sidebar__sessions">
          {projectGroups.length === 0 ? <p className="tauri-sidebar__empty">还没有对话，开始一个新话题吧。</p> : projectGroups.map(group => group.root ? (
            <section className="tauri-project-group" key={group.key} aria-label={group.label}>
              <div className="tauri-project-group__heading">
                <button type="button" className="tauri-project-group__toggle" aria-label={`${collapsedProjects[group.key] ? "展开" : "收起"} ${group.label}`} aria-expanded={!collapsedProjects[group.key]} onClick={() => setCollapsedProjects(previous => ({ ...previous, [group.key]: !previous[group.key] }))}>
                  {collapsedProjects[group.key] ? <ChevronRight size={13} /> : <ChevronDown size={13} />}
                </button>
                <button type="button" className="tauri-project-group__select" title={group.root} aria-label={`切换到项目 ${group.label}`} disabled={busy || switchingBlocked} onClick={() => { const latest = group.sessions[0]; if (latest && latest.sessionId !== session?.id) void activateSession(latest.sessionId, latest.workspaceRoot); }}><FolderOpen size={14} /><span>{group.label}</span><small>{group.sessions.length}</small></button>
                <button type="button" className="tauri-project-group__new" aria-label={`在 ${group.label} 中新建对话`} title="在此项目新建对话" disabled={busy || switchingBlocked} onClick={() => void createSession(group.root || "")}><Plus size={14} /></button>
              </div>
              {!collapsedProjects[group.key] && group.sessions.map(tab => <SessionRow key={tab.sessionId} tab={tab} active={session?.id === tab.sessionId} busy={busy} switchingBlocked={switchingBlocked} onActivate={() => void activateSession(tab.sessionId, tab.workspaceRoot)} onDelete={() => void deleteSession(tab)} />)}
            </section>
          ) : group.sessions.map(tab => <SessionRow key={tab.sessionId} tab={tab} active={session?.id === tab.sessionId} busy={busy} switchingBlocked={switchingBlocked} onActivate={() => void activateSession(tab.sessionId, tab.workspaceRoot)} onDelete={() => void deleteSession(tab)} />))}
          {sessionPageCursor && <button type="button" className="tauri-sidebar__load-more" onClick={() => void loadMoreWorkbenchSessions()} disabled={sessionPageLoading} aria-label="加载更多会话">
            {sessionPageLoading ? "正在加载…" : "加载更多会话"}
          </button>}
          {sessionPageError && <p className="tauri-sidebar__page-error" role="alert">加载失败：{sessionPageError}</p>}
          {sessionPageSource === "legacy" && <p className="tauri-sidebar__page-note">当前使用本地兼容目录</p>}
        </nav>
        <div className="tauri-sidebar__footer">
          <span className={`tauri-health${status?.running ? " is-ready" : ""}`}><i />{status?.running ? "本地运行正常" : "正在连接本地服务…"}</span>
          <button type="button" className="tauri-sidebar__diagnostics" onClick={() => setSettingsOpen(true)}><Settings size={15} />设置</button>
          <button type="button" className="tauri-sidebar__diagnostics" onClick={() => setDiagnosticsOpen(true)}><Activity size={15} />运行状态</button>
        </div>
      </aside>

      <section className="tauri-main">
        <header className="tauri-topbar" data-tauri-drag-region>
          <div className="tauri-topbar__title">
            <div className="tauri-topbar__title-row">
              {session && titleEditing ? <form className="tauri-session-title-edit" onSubmit={event => { event.preventDefault(); void saveTitle(); }}>
                <input autoFocus maxLength={TAURI_TITLE_MAX_CHARS} value={titleDraft} onChange={event => setTitleDraft(event.target.value)} aria-label="对话名称" onKeyDown={event => { if (event.key === "Escape") setTitleEditing(false); }} />
                <button type="submit" className="tauri-session-title-edit__action" aria-label="保存对话名称" disabled={busy}><Check size={14} /></button>
                <button type="button" className="tauri-session-title-edit__action" aria-label="取消重命名" disabled={busy} onClick={() => setTitleEditing(false)}><X size={14} /></button>
              </form> : <>
                <strong data-tauri-drag-region>{session ? displayTitle(session.title, activeCatalogTitle) : "新对话"}</strong>
                {session && <button type="button" className="tauri-title-rename" aria-label="重命名对话" title="重命名对话" disabled={busy || switchingBlocked} onClick={beginTitleEdit}><Pencil size={13} /></button>}
              </>}
            </div>
            <span data-tauri-drag-region>{session?.state === "running" ? "正在生成" : session?.state === "paused" ? "等待你的操作" : session ? "本地会话" : "Reasonix Preview"}</span>
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
            <textarea value={prompt} onChange={event => {
              const next = event.target.value;
              setPrompt(next);
              // Drafting must not wait on a session or the event stream; open
              // one on the first keystroke so send has an id when it is ready.
              // Do not flip `busy` here — that would disable the textarea mid-word.
              if (!session && next && !createSessionInFlightRef.current) {
                createSessionInFlightRef.current = true;
                void createSession().finally(() => { createSessionInFlightRef.current = false; });
              }
            }} onKeyDown={event => {
              if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) { event.preventDefault(); void submit(); }
            }} placeholder={session?.state === "paused" ? "请先完成上方确认…" : session ? "继续聊聊你的问题…（⌘/Ctrl + Enter 发送）" : "输入问题，开始新对话…（⌘/Ctrl + Enter 发送）"} disabled={session?.state === "paused"} rows={3} />
            <div className="tauri-composer__bottom"><span>{session?.workspaceRoot ? `工作区 · ${session.workspaceRoot}` : session ? "当前会话使用默认工作区" : "Preview 配置与稳定版相互隔离"}</span>
              <div className="tauri-composer__actions">
                <button className="tauri-attach-button" type="button" onClick={() => void addAttachments()} disabled={busy || !session || !streamReady || session.state !== "idle"} aria-label="添加文件" title="从本机选择文件并附加到消息"><Paperclip size={16} /><span>添加文件</span></button>
                {session?.state === "running" ? <button className="tauri-send-button is-stop" type="button" onClick={() => void cancel()} disabled={busy} aria-label="停止生成"><Square size={15} fill="currentColor" /></button> : <button className="tauri-send-button" type="button" onClick={() => void submit()} disabled={busy || !session || !streamReady || session.state === "paused" || (!prompt.trim() && attachments.length === 0)} aria-label="发送消息"><ArrowUp size={18} /></button>}
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
            <section className="tauri-diagnostic-card" aria-label="会话目录迁移检查">
              <div className="tauri-diagnostic-card__heading"><h3>会话目录迁移检查</h3><span>{catalogAudit ? "影子比对" : "检查中"}</span></div>
              {catalogAudit ? <>
                <p>旧目录 {catalogAudit.legacyCount} 条 · 身份目录 {catalogAudit.directoryCount} 条；当前侧栏仍使用旧目录。</p>
                <p>{catalogAudit.legacyMatchesDirectory ? "旧目录条目与身份目录一致" : `差异：身份库缺项 ${catalogAudit.missingFromDirectory}、标题 ${catalogAudit.titleMismatches}、工作区 ${catalogAudit.workspaceMismatches}、顺序 ${catalogAudit.orderMismatches}、磁盘状态 ${catalogAudit.physicalStateMismatches}、未登记文件 ${catalogAudit.unclaimedTranscripts}、盘点错误 ${catalogAudit.inventoryErrors}`}</p>
                {catalogAudit.directoryOnlyCount > 0 && <p>身份目录新增项：{catalogAudit.directoryOnlyCount} 条</p>}
                {catalogAudit.missingTranscripts > 0 && <p>transcript 缺失：{catalogAudit.missingTranscripts} 条</p>}
              </> : <p>{catalogAuditError || "正在分页读取身份目录并与旧目录比较…"}</p>}
            </section>
            <section className="tauri-diagnostic-card">
              <h3>预览配置</h3>
              {profile ? <><p>配置与会话保存在独立预览目录：</p><code>{profile.previewHome}</code>{profile.importAvailable ? <><p>检测到稳定版配置。复制前会创建备份，不会改动稳定版。</p><button type="button" className="tauri-diagnostic-action" onClick={() => void importStableProfile()} disabled={busy}>复制稳定版配置（先备份）</button></> : <p>{profile.previewConfigExists ? "预览配置已存在，不会覆盖。" : profile.managedProfile ? "未发现可复制的稳定版配置。" : "检测到自定义 REASONIX_HOME，已停用自动导入。"}</p>}</> : <p>正在检查隔离配置…</p>}
              {profileNotice && <p className="tauri-diagnostic-notice">{profileNotice}</p>}
            </section>
            <section className="tauri-diagnostic-card">
              <div className="tauri-diagnostic-card__heading"><h3>模型提供方</h3><button type="button" onClick={() => void tauriProviderSummary().then(setProviderSummary).catch(cause => setError(tauriMessageFrom(cause)))} disabled={busy}>刷新</button></div>
              {!providerSummary ? <p>正在读取 Preview 配置…</p> : <><p>默认模型只影响新对话；工作区 <code>reasonix.toml</code> 可能覆盖用户默认值。</p>{providerSummary.providers.length === 0 ? <p>当前没有配置提供方。</p> : <ul>{providerSummary.providers.map(provider => <li key={provider.name}><strong>{provider.displayName || provider.name}</strong><span>{provider.kind} · {provider.modelCount} 个模型 · {provider.configured ? "已就绪" : "缺少 API Key"}</span></li>)}</ul>}<small>密钥、环境变量名和服务端点不会传到界面。</small></>}
            </section>
            <section className="tauri-diagnostic-card">
              <div className="tauri-diagnostic-card__heading"><h3>MCP 服务器</h3><span><button type="button" onClick={() => void refreshMCPServers()} disabled={busy}>刷新</button><button type="button" onClick={() => setMcpDraft(emptyMCPDraft(currentWorkspace ? "project" : "global"))} disabled={busy || mcpDraft !== null}>添加</button></span></div>
              {mcpNotice && <p className="tauri-diagnostic-error" role="alert">{mcpNotice}</p>}
              <p>项目级写入工作区的 <code>reasonix.toml</code>，全局级写入用户配置。密钥只写不回传。</p>
              {mcpServers.length === 0 ? <p>还没有配置 MCP 服务器。</p> : <ul className="tauri-mcp-list">{mcpServers.map(server => <li key={server.name}>
                <div className="tauri-mcp-list__row"><strong>{server.name}</strong><span className="tauri-mcp-list__meta">{server.scope === "project" ? "项目" : server.scope === "global" ? "全局" : server.scope} · {server.type}</span></div>
                <code className="tauri-mcp-list__transport">{mcpTransportSummary(server) || "—"}</code>
                {mcpCredentialHint(server) && <small>{mcpCredentialHint(server)}</small>}
                {server.managedByPackage && <small>由已安装插件包管理，不能在此修改。</small>}
                <div className="tauri-mcp-list__actions"><button type="button" onClick={() => setMcpDraft(mcpDraftForEditing(server))} disabled={busy || server.managedByPackage}>编辑</button><button type="button" onClick={() => void removeMCPServer(server)} disabled={busy || server.managedByPackage}>删除</button></div>
              </li>)}</ul>}
              {mcpDraft && <form className="tauri-mcp-form" onSubmit={event => { event.preventDefault(); void saveMCPServer(); }}>
                <label>名称<input value={mcpDraft.name} onChange={event => setMcpDraft({ ...mcpDraft, name: event.target.value })} disabled={mcpDraft.editing} aria-label="MCP 服务器名称" /></label>
                <label>作用域<select value={mcpDraft.scope} onChange={event => setMcpDraft({ ...mcpDraft, scope: event.target.value === "project" ? "project" : "global" })} disabled={mcpDraft.editing}><option value="project">项目（写入工作区 reasonix.toml）</option><option value="global">全局（写入用户配置）</option></select></label>
                <label>传输<select value={mcpDraft.type} onChange={event => setMcpDraft({ ...mcpDraft, type: event.target.value === "http" ? "http" : event.target.value === "sse" ? "sse" : "stdio" })}><option value="stdio">stdio</option><option value="http">http</option><option value="sse">sse</option></select></label>
                {mcpDraft.type === "stdio" ? <>
                  <label>命令<input value={mcpDraft.command} onChange={event => setMcpDraft({ ...mcpDraft, command: event.target.value })} placeholder="uvx" aria-label="MCP 服务器命令" /></label>
                  <label>参数<input value={mcpDraft.args} onChange={event => setMcpDraft({ ...mcpDraft, args: event.target.value })} placeholder="mcp-server-time" aria-label="MCP 服务器参数" /></label>
                </> : <label>URL<input value={mcpDraft.url} onChange={event => setMcpDraft({ ...mcpDraft, url: event.target.value })} placeholder="https://…" aria-label="MCP 服务器 URL" /></label>}
                <label>环境变量<textarea value={mcpDraft.env} onChange={event => setMcpDraft({ ...mcpDraft, env: event.target.value })} rows={2} placeholder="KEY=value，每行一个；留空保持原值" aria-label="MCP 服务器环境变量" /></label>
                <label>请求头<textarea value={mcpDraft.headers} onChange={event => setMcpDraft({ ...mcpDraft, headers: event.target.value })} rows={2} placeholder="Header=value，每行一个；留空保持原值" aria-label="MCP 服务器请求头" /></label>
                <div className="tauri-mcp-form__actions"><button type="submit" disabled={busy}>{mcpDraft.editing ? "保存修改" : "添加服务器"}</button><button type="button" onClick={() => setMcpDraft(null)} disabled={busy}>取消</button></div>
              </form>}
            </section>
            <details className="tauri-diagnostic-card tauri-runtime-details"><summary>构建与版本详情</summary>{!runtimeInfo ? <p>正在读取构建信息…</p> : <dl><div><dt>稳定版基线</dt><dd>v{runtimeInfo.stableVersion} · {runtimeInfo.stableCommit.slice(0, 12)}</dd></div><div><dt>Preview / Tauri</dt><dd>v{runtimeInfo.previewVersion} · v{runtimeInfo.tauriVersion}</dd></div><div><dt>宿主构建时间</dt><dd>{runtimeInfo.previewBuild}</dd></div><div><dt>桥接协议</dt><dd>v{runtimeInfo.bridgeProtocolVersion}</dd></div><div><dt>Sidecar</dt><dd>{runtimeInfo.sidecarInstanceId ?? "未运行"}</dd></div>{session && <div><dt>会话 ID</dt><dd>{session.id}</dd></div>}{session && <div><dt>会话路径</dt><dd>{session.path}</dd></div>}{history && <div><dt>历史记录</dt><dd>{history.totalMessages} 条</dd></div>}<div><dt>事件游标</dt><dd>{sequence} · 最近 {events.length} 个事件</dd></div></dl>}</details>
            <details className="tauri-diagnostic-card tauri-event-details"><summary>桥接事件日志</summary>{events.length === 0 ? <p>开始一个对话后，这里会显示桥接事件。</p> : <ol>{events.map(event => <li key={event.sequence}><b>#{event.sequence} · {event.eventKind}</b><pre>{tauriEventSummary(event)}</pre></li>)}</ol>}</details>
            {session && <button type="button" className="tauri-diagnostic-action" onClick={() => void refreshHistory()} disabled={busy}>刷新当前对话状态与记录</button>}
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
