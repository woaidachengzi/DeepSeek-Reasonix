import { Component, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Activity, ArrowUp, Check, ChevronDown, ChevronRight, Copy, Eye, FileText, FolderOpen, FolderTree, GitBranch, MessageSquare, Paperclip, Pencil, Plus, Settings, Sparkles, Square, Trash2, X } from "lucide-react";
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
import { formatTauriWorkDuration, groupTauriHistory, type IndexedHistoryMessage } from "./historyPresentation";

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

function HistoryMessageArticle({ entry, sessionId, questionId }: {
  entry: IndexedHistoryMessage;
  sessionId: string;
  questionId?: string;
}) {
  const { message, index } = entry;
  const display = message.role === "user" ? parseAttachmentRefsForDisplay(message.content) : null;
  const created = message.createdAtMs && Number.isSafeInteger(message.createdAtMs) ? new Date(message.createdAtMs) : null;
  const createdAt = created && !Number.isNaN(created.getTime()) ? created : null;
  return <MessageErrorBoundary index={index}>
    <article id={questionId} data-tauri-question-anchor={questionId} className={`tauri-message is-${message.role}`}>
      {message.role !== "user" && <div className="tauri-message__avatar" aria-hidden="true"><Sparkles size={16} /></div>}
      <div className="tauri-message__content">
        {message.role !== "user" && <div className="tauri-message__role">Reasonix</div>}
        <Markdown text={display?.text ?? message.content ?? ""} cacheKey={`${sessionId}:${index}`} />
        {display && display.attachments.length > 0 && <div className="tauri-message__attachments">{display.attachments.map(attachment => <span key={attachment.path} title={attachment.path}><Paperclip size={13} />{attachment.name}</span>)}</div>}
        {message.truncated && <small>为保护界面性能，这条历史内容已截断。</small>}
      </div>
      {message.role === "user" && <div className="tauri-message__meta">
        {createdAt && <time dateTime={createdAt.toISOString()}>{createdAt.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time>}
        <button type="button" aria-label="复制消息" title="复制消息" onClick={() => { void navigator.clipboard?.writeText(message.content).catch(() => {}); }}><Copy size={14} /></button>
      </div>}
    </article>
  </MessageErrorBoundary>;
}

const COLLAPSED_PROJECTS_STORAGE_KEY = "reasonix.tauri.workbench.collapsed-projects.v1";

function loadCollapsedProjectGroups(): Record<string, boolean> {
  if (typeof window === "undefined") return {};
  try {
    const raw = window.localStorage.getItem(COLLAPSED_PROJECTS_STORAGE_KEY);
    if (!raw) return {};
    const decoded: unknown = JSON.parse(raw);
    if (!decoded || typeof decoded !== "object" || Array.isArray(decoded)) return {};
    return Object.fromEntries(Object.entries(decoded)
      .filter(([root, collapsed]) => root.length > 0 && root.length <= 4096 && collapsed === true)
      .slice(-1000));
  } catch {
    return {};
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
  importTauriStableProjectFolders,
  importTauriStableProfile,
  newTauriSessionId,
  onTauriBridgeConnectionError,
  onTauriBridgeConnectionRestored,
  onTauriBridgeEvent,
  onTauriBridgeResyncRequired,
  openTauriBridgeSession,
  rememberTauriWorkbenchSession,
  rememberTauriWorkbenchProjectFolder,
  renameTauriWorkbenchProjectFolder,
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
  tauriPendingSessionDeletesPage,
  tauriPendingSessionTitleRecoveries,
  tauriWorkbenchSessionPage,
  tauriScanUnclaimedSessions,
  tauriImportUnclaimedSessions,
  tauriWorkbenchProjectFolders,
  tauriWorkspaceRootsAvailability,
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
  type TauriPendingSessionDelete,
  type TauriPendingSessionDeleteCursor,
  type TauriPendingSessionTitleRecovery,
  type TauriWorkbenchSessionPage,
  type TauriScanImportCandidate,
  type TauriScanImportSelection,
  type TauriWorkspaceEntry,
  type TauriWorkspaceFilePreview,
  type TauriWorkspaceChanges,
  type TauriWorkspaceChangeDetail,
  type TauriPreviewProfileStatus,
  type TauriPreviewRuntimeInfo,
  type TauriProviderSummary,
  type TauriSessionShadowReport,
} from "../lib/tauriBridge";
import { groupWorkbenchSessions, titleFromFirstUser, workbenchProjectKey, type WorkbenchProjectFolder } from "./workbenchSessions";
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
  state?: string;
  missing?: boolean;
  deletionInterrupted?: boolean;
  titleRecoveryPending?: boolean;
}

function isIdentityPageSource(source: TauriWorkbenchSessionPage["source"] | "unavailable"): boolean {
  return source === "identity" || source === "partial_identity";
}

function isReadOnlyWorkbenchSource(source: TauriWorkbenchSessionPage["source"] | "unavailable"): boolean {
  return source === "cached" || source === "identity_unverified" || source === "unavailable";
}

function isPaginatableIdentityPageSource(source: TauriWorkbenchSessionPage["source"]): boolean {
  return isIdentityPageSource(source) || source === "identity_unverified";
}

function projectGroupHasUnloadedSessions(group: { sessions: readonly WorkbenchSessionTab[] }, historyMayBeIncomplete: boolean): boolean {
  return historyMayBeIncomplete && !group.sessions.some(tab => !isMissingWorkbenchSession(tab));
}

function workbenchPageSourceLabel(source: TauriWorkbenchSessionPage["source"]): string {
  if (source === "identity" || source === "partial_identity") return "当前会话列表已重新核验";
  if (source === "identity_unverified") return "当前显示未核验的持久目录（只读）";
  if (source === "cached") return "当前仍显示上次 shadow 审计快照（只读）";
  return "当前仍显示本地兼容目录";
}

function isSessionShadowSafeToPage(report: TauriSessionShadowReport): boolean {
  return report.missingFromDirectory === 0
    && report.workspaceMismatches === 0
    && report.orderMismatches === 0
    && report.physicalStateMismatches === 0
    && report.inventoryErrors === 0;
}

function sessionShadowDifferenceSummary(report: TauriSessionShadowReport): string {
  const differences = [
    report.missingFromDirectory > 0 ? `旧目录有 ${report.missingFromDirectory} 条未登记到身份库` : "",
    report.directoryOnlyCount > 0 ? `身份目录有 ${report.directoryOnlyCount} 条未见于旧目录` : "",
    report.titleMismatches > 0 ? `标题差异 ${report.titleMismatches} 条` : "",
    report.workspaceMismatches > 0 ? `工作区差异 ${report.workspaceMismatches} 条` : "",
    report.orderMismatches > 0 ? `顺序差异 ${report.orderMismatches} 条` : "",
    report.missingTranscripts > 0 ? `transcript 缺失 ${report.missingTranscripts} 条` : "",
    report.physicalStateMismatches > 0 ? `磁盘状态差异 ${report.physicalStateMismatches} 条` : "",
    report.unclaimedTranscripts > 0 ? `未认领 transcript ${report.unclaimedTranscripts} 个` : "",
    report.inventoryErrors > 0 ? `盘点错误 ${report.inventoryErrors} 项` : "",
    report.retiredLegacyCount > 0 ? `旧目录中有 ${report.retiredLegacyCount} 条为待删除或已删除会话` : "",
  ].filter(Boolean);
  if (differences.length > 0) return differences.join("；");
  return report.legacyMatchesDirectory
    ? "审计报告显示目录一致，但分页读取未通过同轮校验；请重新检查"
    : "审计报告发现差异，但未返回可展示的差异类别；请重新检查";
}

function preserveWorkbenchLifecycle(
  sessions: WorkbenchSessionTab[],
  previous: readonly WorkbenchSessionTab[],
): WorkbenchSessionTab[] {
  const byId = new Map(previous.map(session => [session.sessionId, session]));
  return sessions.map(session => {
    const prior = byId.get(session.sessionId);
    return prior ? {
      ...session,
      state: session.state ?? prior.state,
      missing: session.missing ?? prior.missing,
      deletionInterrupted: session.deletionInterrupted ?? prior.deletionInterrupted,
    } : session;
  });
}

function isMissingWorkbenchSession(session: WorkbenchSessionTab): boolean {
  return session.missing === true || session.state === "missing";
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
  const missing = isMissingWorkbenchSession(tab);
  const deletionInterrupted = tab.deletionInterrupted === true;
  const titleRecoveryPending = tab.titleRecoveryPending === true;
  if (confirming) {
    return (
      <div className="tauri-session-delete" role="group" aria-label="确认删除对话">
        <span className="tauri-session-delete__question">{deletionInterrupted ? `继续删除“${displayTitle(tab.title)}”？` : titleRecoveryPending ? `删除“${displayTitle(tab.title)}”并放弃未完成的改名？` : `删除“${displayTitle(tab.title)}”？`}</span>
        <span className="tauri-session-delete__actions">
          <button type="button" className="tauri-session-delete__confirm" disabled={busy} onClick={onDelete}>{deletionInterrupted ? "继续删除" : "删除"}</button>
          <button type="button" className="tauri-session-delete__cancel" disabled={busy} onClick={() => setConfirming(false)}>取消</button>
        </span>
      </div>
    );
  }
  return (
    <div className={`tauri-session-row${active ? " is-active" : ""}`}>
      <button
        type="button"
        className={`tauri-sidebar__session${missing ? " is-missing" : ""}`}
        disabled={busy || active || switchingBlocked || missing || deletionInterrupted}
        onClick={onActivate}
        aria-label={deletionInterrupted ? `${displayTitle(tab.title)}（删除未完成）` : missing ? `${displayTitle(tab.title)}（文件缺失）` : titleRecoveryPending ? `${displayTitle(tab.title)}（标题恢复待完成）` : undefined}
        title={deletionInterrupted ? "上次删除被中断；点击右侧按钮并确认后继续删除。" : missing ? "会话文件缺失，无法打开。可删除这条失效记录。" : titleRecoveryPending ? "上次改名被中断；打开此会话会核对侧车并尝试完成或撤销改名。" : tab.workspaceRoot ? `${tab.sessionId}\n${tab.workspaceRoot}` : tab.sessionId}
      >
        <MessageSquare size={15} aria-hidden="true" />
        <span>{displayTitle(tab.title)}</span>
        {deletionInterrupted && <small className="tauri-session-recovery">删除未完成</small>}
        {titleRecoveryPending && <small className="tauri-session-recovery">标题待恢复</small>}
        {missing && <small className="tauri-session-missing">文件缺失</small>}
      </button>
      <button
        type="button"
        className="tauri-session-row__delete"
        aria-label={`${deletionInterrupted ? "继续删除" : "删除对话"} ${displayTitle(tab.title)}`}
        title={deletionInterrupted ? "继续删除已开始的删除操作" : "删除对话"}
        disabled={busy || (switchingBlocked && !deletionInterrupted)}
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
  const [unverifiedLegacyTabs, setUnverifiedLegacyTabs] = useState<WorkbenchSessionTab[]>([]);
  const [pendingSessionDeletes, setPendingSessionDeletes] = useState<TauriPendingSessionDelete[]>([]);
  const [pendingSessionDeleteCursor, setPendingSessionDeleteCursor] = useState<TauriPendingSessionDeleteCursor | null>(null);
  const [pendingSessionDeleteLoading, setPendingSessionDeleteLoading] = useState(true);
  const [pendingSessionDeleteError, setPendingSessionDeleteError] = useState("");
  const [pendingSessionTitleRecoveries, setPendingSessionTitleRecoveries] = useState<TauriPendingSessionTitleRecovery[]>([]);
  const [pendingSessionTitleRecoveryError, setPendingSessionTitleRecoveryError] = useState("");
  const [projectFolders, setProjectFolders] = useState<WorkbenchProjectFolder[]>([]);
  const [projectFoldersWarning, setProjectFoldersWarning] = useState("");
  const [sessionPageCursor, setSessionPageCursor] = useState<{ position: number; id: string; snapshotId: string; total: number } | null>(null);
  const [sessionPageSource, setSessionPageSource] = useState<TauriWorkbenchSessionPage["source"] | "unavailable">("unavailable");
  const [sessionPageDirectoryCount, setSessionPageDirectoryCount] = useState<number | null>(null);
  const [sessionPageUnclaimedCount, setSessionPageUnclaimedCount] = useState<number | null>(null);
  const [sessionPageTitleMismatchCount, setSessionPageTitleMismatchCount] = useState<number | null>(null);
  const [sessionPageMissingTranscriptCount, setSessionPageMissingTranscriptCount] = useState<number | null>(null);
  const [sessionPageLoading, setSessionPageLoading] = useState(false);
  const [sessionPageNotice, setSessionPageNotice] = useState("");
  const [sessionPageError, setSessionPageError] = useState("");
  const [workspaceAvailability, setWorkspaceAvailability] = useState<Record<string, boolean | null>>({});
  const [collapsedProjects, setCollapsedProjects] = useState<Record<string, boolean>>(loadCollapsedProjectGroups);
  const [workspaceRoot, setWorkspaceRoot] = useState("");
  const [editingProjectRoot, setEditingProjectRoot] = useState<string | null>(null);
  const [projectTitleDraft, setProjectTitleDraft] = useState("");
  const [prompt, setPrompt] = useState("");
  const [attachments, setAttachments] = useState<TauriBridgeAttachment[]>([]);
  const [draftAttachmentPaths, setDraftAttachmentPaths] = useState<string[]>([]);
  const [status, setStatus] = useState<TauriBridgeStatus | null>(null);
  const [profile, setProfile] = useState<TauriPreviewProfileStatus | null>(null);
  const [scanImportOpen, setScanImportOpen] = useState(false);
  const [scanImportCandidates, setScanImportCandidates] = useState<TauriScanImportCandidate[]>([]);
  const [scanImportDrafts, setScanImportDrafts] = useState<Record<string, { selected: boolean; title: string; workspaceRoot: string }>>({});
  const [scanImportBlockedCount, setScanImportBlockedCount] = useState(0);
  const [scanImportLoading, setScanImportLoading] = useState(false);
  const [scanImportError, setScanImportError] = useState("");
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
  const [sessionPageCatalogWarning, setSessionPageCatalogWarning] = useState("");
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
  const composerRef = useRef<HTMLTextAreaElement>(null);
  const turnEpochRef = useRef(0);
  const submitInFlightRef = useRef(false);
  const pendingDraftSubmissionRef = useRef<{ sessionId: string; text: string; paths: string[]; workspaceRoot?: string } | null>(null);
  const sessionPageRequestRef = useRef(false);
  const sessionPageRevisionRef = useRef(0);
  const pendingDeleteRequestRef = useRef(0);
  const pendingTitleRecoveryRequestRef = useRef(0);
  const projectRootsRequestRef = useRef(0);
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
  const pendingSessionDeleteIDs = useMemo(() => new Set(pendingSessionDeletes.map(item => item.id)), [pendingSessionDeletes]);
  const visibleTabs = useMemo(() => tabs.filter(tab => !pendingSessionDeleteIDs.has(tab.sessionId)), [tabs, pendingSessionDeleteIDs]);
  const projectGroups = useMemo(() => groupWorkbenchSessions(visibleTabs, projectFolders, platform), [visibleTabs, projectFolders, platform]);
  const projectHistoryMayBeIncomplete = sessionPageCursor !== null || sessionPageSource === "legacy";
  const projectHistoryUnavailableHint = sessionPageSource === "legacy"
    ? "持久会话目录未核验，无法确认此项目是否还有历史会话；重新检查目录后再选择"
    : "还有未加载的会话页；加载更多以确认此项目是否为空";
  const projectRootsKey = projectGroups.flatMap(group => group.root ? [group.root] : []).join("\u0000");
  const activeCatalogTitle = tabs.find(tab => tab.sessionId === session?.id)?.title;

  useEffect(() => {
    try {
      const collapsed = Object.entries(collapsedProjects)
        .filter(([root, value]) => root.length > 0 && root.length <= 4096 && value)
        .slice(-1000);
      window.localStorage.setItem(COLLAPSED_PROJECTS_STORAGE_KEY, JSON.stringify(Object.fromEntries(collapsed)));
    } catch {
      // The sidebar remains usable when WebView storage is unavailable or full.
    }
  }, [collapsedProjects]);

  useEffect(() => {
    const roots = projectRootsKey ? projectRootsKey.split("\u0000") : [];
    if (roots.length === 0) return;
    const request = ++projectRootsRequestRef.current;
    void (async () => {
      for (let offset = 0; offset < roots.length; offset += 200) {
        try {
          const batch = roots.slice(offset, offset + 200);
          const availability = await tauriWorkspaceRootsAvailability(batch);
          if (request !== projectRootsRequestRef.current) return;
          setWorkspaceAvailability(previous => ({
            ...previous,
            ...Object.fromEntries(batch.map((root, index) => [root, availability[index] ?? null])),
          }));
        } catch {
          // Workspace availability is informational; a failed probe must not block session access.
          return;
        }
      }
    })();
    return () => { projectRootsRequestRef.current += 1; };
  }, [projectRootsKey]);

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

  const historyPresentation = useMemo(() => history
    ? groupTauriHistory(
      history.messages,
      history.startIndex,
      Boolean(session && session.state !== "idle" && history.session.state !== "idle" && !pendingUserMessage),
    )
    : [], [history, session?.state, pendingUserMessage]);

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
      let importFailure = "";
      try {
        await reloadWorkbenchProjectFolders(() => active);
        // One-time, idempotent migration. Prefer the identity directory after
        // import, but retain the JSON catalog as an initial-load fallback.
        try {
          await tauriImportLegacySessionCatalog();
        } catch (cause) {
          importFailure = tauriMessageFrom(cause);
        }
        if (!active) return;
        void reloadPendingSessionDeletes(() => active);
        void reloadPendingSessionTitleRecoveries(() => active);
        const page = await tauriWorkbenchSessionPage();
        if (!active) return;
        setTabs(page.sessions);
        setUnverifiedLegacyTabs(page.unverifiedLegacySessions ?? []);
        setSessionPageCursor(page.nextCursor ?? null);
        setSessionPageSource(page.source);
        setSessionPageCatalogWarning(page.catalogWarning ?? "");
        setSessionPageDirectoryCount(page.shadowDirectoryCount ?? (page.source === "identity" ? page.total : null));
        setSessionPageUnclaimedCount(page.unclaimedTranscriptCount ?? null);
        setSessionPageTitleMismatchCount(page.titleMismatchCount ?? null);
        setSessionPageMissingTranscriptCount(page.missingTranscriptCount ?? null);
        const missing = isIdentityPageSource(page.source)
          ? page.sessions.filter(tab => !tauriSessionTitle(tab.title, "")).map(tab => tab.sessionId)
          : [];
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
        if (page.shadowReport) {
          setCatalogAudit(page.shadowReport);
          setCatalogAuditError("");
        } else {
          await refreshCatalogAudit(() => active);
        }
        if (active && importFailure) {
          setSessionPageNotice(`最近会话列表已加载；启动时旧会话目录导入失败：${importFailure}`);
        }
      } catch (cause) {
        if (active) {
          const importNotice = importFailure ? `；旧会话目录导入也失败：${importFailure}` : "";
          setError(`读取最近对话失败：${tauriMessageFrom(cause)}${importNotice}`);
        }
      }
    })();
    void tauriPlatformInfo().then(setPlatform).catch(() => {});
    return () => { active = false; };
  }, []);

  async function loadMoreWorkbenchSessions() {
    const cursor = sessionPageCursor;
    if (!cursor || sessionPageRequestRef.current) return;
    const revision = sessionPageRevisionRef.current;
    sessionPageRequestRef.current = true;
    setSessionPageLoading(true);
    setSessionPageNotice("");
    setSessionPageError("");
    try {
      const page = await tauriWorkbenchSessionPage(cursor);
      if (revision !== sessionPageRevisionRef.current) return;
      setTabs(previous => {
        const known = new Set(previous.map(tab => tab.sessionId));
        return [...previous, ...page.sessions.filter(tab => !known.has(tab.sessionId))];
      });
      setSessionPageCursor(page.nextCursor ?? null);
      setSessionPageSource(page.source);
      setSessionPageCatalogWarning(page.catalogWarning ?? "");
      setSessionPageDirectoryCount(page.shadowDirectoryCount ?? (page.source === "identity" ? page.total : null));
      setSessionPageUnclaimedCount(page.unclaimedTranscriptCount ?? null);
      setSessionPageTitleMismatchCount(page.titleMismatchCount ?? null);
      setSessionPageMissingTranscriptCount(page.missingTranscriptCount ?? null);
      const missing = isIdentityPageSource(page.source)
        ? page.sessions.filter(tab => !tauriSessionTitle(tab.title, "")).map(tab => tab.sessionId)
        : [];
      for (let offset = 0; offset < missing.length; offset += 50) {
        try {
          const previews = await tauriSessionPreviews(missing.slice(offset, offset + 50));
          const titles = previews.flatMap(preview => {
            const title = tauriSessionTitle(preview.title, "") || titleFromFirstUser(preview.firstUser ?? "");
            return title ? [{ sessionId: preview.sessionId, title }] : [];
          });
          if (titles.length > 0) {
            await backfillTauriWorkbenchTitles(titles);
            if (revision !== sessionPageRevisionRef.current) return;
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
      if (revision !== sessionPageRevisionRef.current) return;
      // A rejected continuation may mean the directory changed after the
      // previous page. Restart from a freshly shadow-verified first page so
      // we never leave the sidebar stranded on a stale cursor or mix snapshots.
      try {
        const page = await tauriWorkbenchSessionPage();
        if (revision !== sessionPageRevisionRef.current) return;
        setTabs(page.sessions);
        setUnverifiedLegacyTabs(page.unverifiedLegacySessions ?? []);
        setSessionPageCursor(page.nextCursor ?? null);
        setSessionPageSource(page.source);
        setSessionPageCatalogWarning(page.catalogWarning ?? "");
        setSessionPageDirectoryCount(page.shadowDirectoryCount ?? (page.source === "identity" ? page.total : null));
        setSessionPageUnclaimedCount(page.unclaimedTranscriptCount ?? null);
        setSessionPageTitleMismatchCount(page.titleMismatchCount ?? null);
        setSessionPageMissingTranscriptCount(page.missingTranscriptCount ?? null);
        setSessionPageError("");
      } catch (restartCause) {
        if (revision === sessionPageRevisionRef.current) {
          setTabs([]);
          setUnverifiedLegacyTabs([]);
          setSessionPageCursor(null);
          setSessionPageSource("unavailable");
          setSessionPageCatalogWarning("");
          setSessionPageDirectoryCount(null);
          setSessionPageUnclaimedCount(null);
          setSessionPageTitleMismatchCount(null);
          setSessionPageMissingTranscriptCount(null);
          setSessionPageError(`${tauriMessageFrom(cause)}；重新读取失败：${tauriMessageFrom(restartCause)}`);
        }
      }
    } finally {
      sessionPageRequestRef.current = false;
      setSessionPageLoading(false);
    }
  }

  // Tauri drag-and-drop file handler
  useLayoutEffect(() => {
    let active = true;
    let unlisten: UnlistenFn | undefined;
    const canAttach = !busy && !isReadOnlyWorkbenchSource(sessionPageSource) &&
      (!session || (streamReady && session.state === "idle"));
    void (async () => {
      try {
        unlisten = await retainTauriDragDropListener(getCurrentWindow().onDragDropEvent(async (event) => {
          if (!active) return;
          await handleTauriDragDropEvent(event.payload, {
            sessionId: canAttach ? session?.id : undefined,
            queuePendingPath: canAttach && !session
              ? path => setDraftAttachmentPaths(previous => [...previous, path])
              : undefined,
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
  }, [session?.id, session?.state, streamReady, busy, sessionPageSource]);

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
          void abandonUnsentDraft(session.id, `无法开始新对话：${message}`);
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
        void abandonUnsentDraft(session.id, `无法开始新对话：${message}`);
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

  useEffect(() => {
    const pending = pendingDraftSubmissionRef.current;
    if (!pending || !session || !streamReady || pending.sessionId !== session.id) return;
    pendingDraftSubmissionRef.current = null;
    void submitPreparedInput(pending.sessionId, pending.text, [], pending.paths, true, pending.workspaceRoot);
  }, [session?.id, streamReady]);

  async function abandonUnsentDraft(sessionId: string, message: string) {
    if (pendingDraftSubmissionRef.current?.sessionId !== sessionId) return;
    pendingDraftSubmissionRef.current = null;
    try {
      await deleteTauriBridgeSession(sessionId);
      setSession(previous => previous?.id === sessionId ? null : previous);
      setStreamReady(false);
      setError(message);
    } catch (cause) {
      setError(`${message}；空会话清理失败：${tauriMessageFrom(cause)}`);
    } finally {
      submitInFlightRef.current = false;
      setBusy(false);
    }
  }

  // Existing rows keep their sidebar position on reopen; only brand-new
  // sessions are prepended (matches WorkbenchCatalog::remember).
  async function rememberSession(
    next: TauriBridgeSession,
    failureMessage = "对话已打开，但无法保存到最近对话",
  ) {
    try {
      const updated = await rememberTauriWorkbenchSession(next.id, next.workspaceRoot ?? undefined, tauriSessionTitle(next.title, "") || undefined);
      sessionPageRevisionRef.current += 1;
      if (!isIdentityPageSource(sessionPageSource)) {
        setTabs(previous => preserveWorkbenchLifecycle(updated, previous));
        setUnverifiedLegacyTabs([]);
        setSessionPageSource("legacy");
        setSessionPageDirectoryCount(null);
        setSessionPageUnclaimedCount(null);
        setSessionPageTitleMismatchCount(null);
        setSessionPageMissingTranscriptCount(null);
      }
      await refreshCatalogAudit(() => true, true);
    } catch (cause) {
      setError(`${failureMessage}：${tauriMessageFrom(cause)}`);
    }
  }

  async function reloadFirstWorkbenchSessionPage(request: number, isActive: () => boolean): Promise<TauriWorkbenchSessionPage["source"] | null> {
    // A fresh guarded first page replaces the previous snapshot. Invalidate
    // any continuation already in flight before it can append old rows.
    sessionPageRevisionRef.current += 1;
    setSessionPageNotice("");
    try {
      const page = await tauriWorkbenchSessionPage();
      if (request !== catalogAuditRequestRef.current || !isActive()) return null;
      sessionPageRevisionRef.current += 1;
      setTabs(page.sessions);
      setUnverifiedLegacyTabs(page.unverifiedLegacySessions ?? []);
      setSessionPageCursor(page.nextCursor ?? null);
      setSessionPageSource(page.source);
      setSessionPageCatalogWarning(page.catalogWarning ?? "");
      setSessionPageDirectoryCount(page.shadowDirectoryCount ?? (page.source === "identity" ? page.total : null));
      setSessionPageUnclaimedCount(page.unclaimedTranscriptCount ?? null);
      setSessionPageTitleMismatchCount(page.titleMismatchCount ?? null);
      setSessionPageMissingTranscriptCount(page.missingTranscriptCount ?? null);
      setCatalogAudit(page.shadowReport ?? null);
      setCatalogAuditError(!page.shadowReport
        ? "会话目录 shadow 检查不可用；重新检查会话目录可重试"
        : page.source === "legacy" ? "首屏未通过身份目录分页校验，已回退到兼容目录" : "");
      setSessionPageError("");
      return page.source;
    } catch (cause) {
      if (request === catalogAuditRequestRef.current && isActive()) {
        sessionPageRevisionRef.current += 1;
        setTabs([]);
        setUnverifiedLegacyTabs([]);
        setSessionPageCursor(null);
        setSessionPageSource("unavailable");
        setSessionPageCatalogWarning("");
        setSessionPageDirectoryCount(null);
        setSessionPageUnclaimedCount(null);
        setSessionPageTitleMismatchCount(null);
        setSessionPageMissingTranscriptCount(null);
        setCatalogAudit(null);
        setCatalogAuditError("会话目录 shadow 检查不可用；重新检查会话目录可重试");
        setSessionPageError(tauriMessageFrom(cause));
      }
      return null;
    }
  }

  async function reloadPendingSessionDeletes(
    isActive: () => boolean = () => true,
    cursor: TauriPendingSessionDeleteCursor | null = null,
    append = false,
  ) {
    const request = ++pendingDeleteRequestRef.current;
    setPendingSessionDeleteLoading(true);
    setPendingSessionDeleteError("");
    try {
      const page = await tauriPendingSessionDeletesPage(cursor);
      if (!isActive() || request !== pendingDeleteRequestRef.current) return;
      setPendingSessionDeletes(previous => {
        const combined = append ? [...previous, ...page.sessions] : page.sessions;
        const seen = new Set<string>();
        return combined.filter(entry => {
          if (seen.has(entry.id)) return false;
          seen.add(entry.id);
          return true;
        });
      });
      setPendingSessionDeleteCursor(page.nextCursor ?? null);
      setPendingSessionDeleteError("");
    } catch (cause) {
      if (!isActive() || request !== pendingDeleteRequestRef.current) return;
      setPendingSessionDeleteError(tauriMessageFrom(cause));
    } finally {
      if (isActive() && request === pendingDeleteRequestRef.current) {
        setPendingSessionDeleteLoading(false);
      }
    }
  }

  async function retryPendingSessionDeleteCheck() {
    await reloadPendingSessionDeletes();
  }

  async function loadMorePendingSessionDeletes() {
    if (!pendingSessionDeleteCursor || pendingSessionDeleteLoading) return;
    await reloadPendingSessionDeletes(() => true, pendingSessionDeleteCursor, true);
  }

  async function reloadPendingSessionTitleRecoveries(isActive: () => boolean = () => true) {
    const request = ++pendingTitleRecoveryRequestRef.current;
    try {
      const entries = await tauriPendingSessionTitleRecoveries();
      if (!isActive() || request !== pendingTitleRecoveryRequestRef.current) return;
      setPendingSessionTitleRecoveries(entries);
      setPendingSessionTitleRecoveryError("");
    } catch (cause) {
      if (!isActive() || request !== pendingTitleRecoveryRequestRef.current) return;
      setPendingSessionTitleRecoveryError(tauriMessageFrom(cause));
    }
  }

  async function retryPendingSessionTitleRecoveryCheck() {
    await reloadPendingSessionTitleRecoveries();
  }

  async function reloadWorkbenchProjectFolders(isActive: () => boolean = () => true) {
    try {
      const result = await tauriWorkbenchProjectFolders();
      if (!isActive()) return;
      setProjectFolders(result.folders.map(folder => ({ root: folder.root, title: folder.title })));
      setProjectFoldersWarning(result.warning ?? "");
    } catch (cause) {
      if (!isActive()) return;
      setProjectFoldersWarning(`读取项目文件夹失败：${tauriMessageFrom(cause)}`);
    }
  }

  async function retryWorkbenchSessionDirectory() {
    if (sessionPageRequestRef.current) return;
    sessionPageRequestRef.current = true;
    const request = ++catalogAuditRequestRef.current;
    setSessionPageLoading(true);
    setSessionPageNotice("");
    setSessionPageError("");
    try {
      let importFailure = "";
      try {
        // The import is idempotent and bounded; retry it when the user asks
        // for a fresh directory check after a transient bridge failure.
        await tauriImportLegacySessionCatalog();
      } catch (cause) {
        importFailure = tauriMessageFrom(cause);
      }
      const source = await reloadFirstWorkbenchSessionPage(request, () => true);
      if (importFailure && request === catalogAuditRequestRef.current) {
        if (source) {
          setSessionPageNotice(`${workbenchPageSourceLabel(source)}；旧会话目录导入重试失败：${importFailure}`);
        } else {
          setSessionPageError(previous => previous
            ? `${previous}；旧会话目录导入重试失败：${importFailure}`
            : `旧会话目录导入重试失败：${importFailure}`);
        }
      }
    } finally {
      if (request === catalogAuditRequestRef.current) {
        sessionPageRequestRef.current = false;
        setSessionPageLoading(false);
      }
    }
  }

  async function refreshCatalogAudit(isActive: () => boolean = () => true, refreshVisiblePage = false) {
    const request = ++catalogAuditRequestRef.current;
    setCatalogAudit(null);
    setCatalogAuditError("");
    if (refreshVisiblePage) setSessionPageNotice("");

    if (refreshVisiblePage) {
      sessionPageRevisionRef.current += 1;
      const replaceWithPage = (page: TauriWorkbenchSessionPage) => {
        sessionPageRevisionRef.current += 1;
        setTabs(page.sessions);
        setUnverifiedLegacyTabs(page.unverifiedLegacySessions ?? []);
        setSessionPageCursor(page.nextCursor ?? null);
        setSessionPageSource(page.source);
        setSessionPageCatalogWarning(page.catalogWarning ?? "");
        setSessionPageDirectoryCount(page.shadowDirectoryCount ?? null);
        setSessionPageUnclaimedCount(page.unclaimedTranscriptCount ?? null);
        setSessionPageTitleMismatchCount(page.titleMismatchCount ?? null);
        setSessionPageMissingTranscriptCount(page.missingTranscriptCount ?? null);
      };
      try {
        const desiredPages = Math.max(1, Math.ceil(tabs.length / 200));
        let page = await tauriWorkbenchSessionPage();
        if (request !== catalogAuditRequestRef.current || !isActive()) return;
        let report = page.shadowReport ?? null;
        setCatalogAudit(report);
        setCatalogAuditError(report ? "" : "会话目录 shadow 检查不可用；重新检查会话目录可重试");
        if (!isPaginatableIdentityPageSource(page.source)) {
          replaceWithPage(page);
          if (page.source === "legacy" && report) {
            setCatalogAuditError("首屏未通过身份目录分页校验，已回退到兼容目录");
          }
          return;
        }

        const sessions = [...page.sessions];
        let cursor = page.nextCursor ?? null;
        let pagesRead = 1;
        while (cursor && pagesRead < desiredPages) {
          page = await tauriWorkbenchSessionPage(cursor);
          if (request !== catalogAuditRequestRef.current || !isActive()) return;
          report = page.shadowReport ?? report;
          if (!isPaginatableIdentityPageSource(page.source)) {
            if (report) setCatalogAudit(report);
            setCatalogAuditError("续页期间身份目录不可用，正在重新读取当前安全来源");
            await reloadFirstWorkbenchSessionPage(request, isActive);
            return;
          }
          const known = new Set(sessions.map(tab => tab.sessionId));
          sessions.push(...page.sessions.filter(tab => !known.has(tab.sessionId)));
          cursor = page.nextCursor ?? null;
          pagesRead += 1;
        }
        if (request === catalogAuditRequestRef.current && isActive()) {
          sessionPageRevisionRef.current += 1;
          setTabs(previous => preserveWorkbenchLifecycle(sessions, previous));
          setSessionPageCursor(cursor);
          setSessionPageSource(page.source);
          setSessionPageCatalogWarning(page.catalogWarning ?? "");
          setSessionPageDirectoryCount(page.shadowDirectoryCount ?? page.total);
          setSessionPageUnclaimedCount(page.unclaimedTranscriptCount ?? null);
          setSessionPageTitleMismatchCount(page.titleMismatchCount ?? null);
          setSessionPageMissingTranscriptCount(page.missingTranscriptCount ?? null);
          setCatalogAudit(report);
          setCatalogAuditError(report ? "" : "会话目录 shadow 检查不可用；重新检查会话目录可重试");
        }
      } catch (cause) {
        if (request === catalogAuditRequestRef.current && isActive()) {
          setSessionPageError(tauriMessageFrom(cause));
          await reloadFirstWorkbenchSessionPage(request, isActive);
        }
      }
      return;
    }

    try {
      const report = await tauriSessionCatalogShadow();
      if (request === catalogAuditRequestRef.current && isActive()) {
        setCatalogAudit(report);
        if (!isSessionShadowSafeToPage(report) && isIdentityPageSource(sessionPageSource)) {
          // Structural or physical divergence stops identity paging. Title
          // drift, verified missing rows, and unclaimed files remain visible
          // as partial source with separate counts.
          await reloadFirstWorkbenchSessionPage(request, isActive);
        }
      }
    } catch (cause) {
      if (request === catalogAuditRequestRef.current && isActive()) {
        setCatalogAuditError(tauriMessageFrom(cause));
        // An unavailable audit is not evidence that the current page is still
        // safe. Re-read through the guarded first-page command; it chooses JSON
        // whenever the current shadow check is incomplete or divergent.
        await reloadFirstWorkbenchSessionPage(request, isActive);
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
    if (isReadOnlyWorkbenchSource(sessionPageSource)) {
      setError(sessionPageSource === "unavailable"
        ? "会话目录尚未通过检查；读取成功后才能打开会话。"
        : "当前会话目录为只读来源；重新检查并核验通过后才能打开会话。");
      return;
    }
    if (!id) return setError("缺少会话 ID");
    setBusy(true);
    setError("");
    try {
      const next = await switchTauriBridgeSession(id, root);
      invalidateWorkspaceRequests();
      setEvents([]);
      setHistory(null);
      setAttachments([]);
      setDraftAttachmentPaths([]);
      if (!session) setPrompt("");
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
      const openedRoot = next.workspaceRoot ?? root;
      if (openedRoot?.trim()) {
        try {
          const result = await rememberTauriWorkbenchProjectFolder(openedRoot);
          setProjectFolders(result.folders.map(folder => ({ root: folder.root, title: folder.title })));
          setProjectFoldersWarning(result.warning ?? "");
        } catch (cause) {
          setError(`对话已打开，但无法保存项目文件夹：${tauriMessageFrom(cause)}`);
        }
      }
      await rememberSession(next);
      await reloadPendingSessionTitleRecoveries();
      setStreamRevision(previous => previous + 1);
      setStatus(await tauriBridgeStatus());
    } catch (cause) {
      setError(sessionLifecycleNotice(cause) ?? tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  function beginTitleEdit() {
    if (!session || busy || switchingBlocked || isReadOnlyWorkbenchSource(sessionPageSource)) return;
    setTitleDraft(displayTitle(session.title, activeCatalogTitle));
    setTitleEditing(true);
  }

  async function saveTitle() {
    if (!session) return;
    if (isReadOnlyWorkbenchSource(sessionPageSource)) {
      setError("当前会话目录为只读来源；重新检查并核验通过后才能重命名会话。");
      return;
    }
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
      await rememberSession(renamed, "标题已保存，但同步到会话列表失败；请重新检查会话目录");
      setTitleEditing(false);
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  // Deleting is a two-step confirmation, scoped to the row that asked for it.
  // Ordinary inactive conversations switch to the bridge's owned controller
  // before deletion. A pending manual title cannot always be reopened: the
  // bridge atomically verifies that intent before fencing explicit deletion.
  async function deleteSession(target: WorkbenchSessionTab) {
    const isOpen = session?.id === target.sessionId;
    // A pending delete comes from the identity recovery list. Retrying a
    // different ID does not switch the single active controller.
    const retryingInterruptedDelete = target.deletionInterrupted === true && !isOpen;
    if (isReadOnlyWorkbenchSource(sessionPageSource) && !retryingInterruptedDelete) {
      setError("当前会话目录为只读来源；重新检查并核验通过后才能删除会话。");
      return;
    }
    if (busy || (switchingBlocked && !retryingInterruptedDelete)) return;
    const sessionToRestore = session;
    if (!retryingInterruptedDelete && !isOpen && (session?.state === "running" || session?.state === "paused")) {
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
      if (!isOpen && !target.deletionInterrupted && !target.titleRecoveryPending && !target.missing && target.state !== "missing") {
        try {
          await switchTauriBridgeSession(target.sessionId, target.workspaceRoot);
          switchedToTarget = true;
        } catch (cause) {
          // Missing, deleting, and tombstoned sessions cannot be reopened.
          // DELETE can retire/retry them without recreating a transcript, and
          // is idempotent for a tombstone left in the legacy host catalog.
          const failure = sessionLifecycleFailure(cause);
          if (failure !== "missing" && failure !== "deleting" && failure !== "deleted") throw cause;
        }
      }
      // For an interrupted deletion, avoid switching the single bridge
      // controller. DELETE resumes cleanup without recreating the transcript.
      await deleteTauriBridgeSession(target.sessionId);
      deleted = true;
      if (target.deletionInterrupted) {
        setPendingSessionDeletes(previous => previous.filter(item => item.id !== target.sessionId));
      }
      if (isOpen) {
        turnEpochRef.current += 1;
        setSession(null);
      }
      const updated = await forgetTauriWorkbenchSession(target.sessionId);
      sessionPageRevisionRef.current += 1;
      if (!isIdentityPageSource(sessionPageSource)) {
        setTabs(previous => preserveWorkbenchLifecycle(updated, previous));
        setUnverifiedLegacyTabs([]);
        setSessionPageSource("legacy");
        setSessionPageDirectoryCount(null);
        setSessionPageUnclaimedCount(null);
        setSessionPageTitleMismatchCount(null);
        setSessionPageMissingTranscriptCount(null);
      }
      await refreshCatalogAudit(() => true, true);
    } catch (cause) {
      const userMessage = sessionLifecycleNotice(cause) ?? tauriMessageFrom(cause);
      operationError = deleted
        ? `对话已删除，但最近对话列表更新失败：${userMessage}`
        : userMessage;
    } finally {
      await reloadPendingSessionDeletes();
      await reloadPendingSessionTitleRecoveries();
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

  function createSession(root = workspaceRoot) {
    if (isReadOnlyWorkbenchSource(sessionPageSource)) {
      setError("当前会话目录为只读来源；重新检查并核验通过后才能新建会话。");
      return;
    }
    if (busy || switchingBlocked) return;
    invalidateWorkspaceRequests();
    turnEpochRef.current += 1;
    setSession(null);
    setHistory(null);
    setHistoryError("");
    setHistoryLoading(false);
    setStreamReady(false);
    setEvents([]);
    setLiveText("");
    setPendingUserMessage(null);
    setPendingPrompt(null);
    setPromptSelections({});
    setAttachments([]);
    setDraftAttachmentPaths([]);
    setPrompt("");
    setError("");
    setTitleEditing(false);
    setWorkspaceOpen(false);
    setDragging(false);
    setWorkspaceRoot(root.trim());
    composerRef.current?.focus();
  }

  async function chooseWorkspaceRoot() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const selected = await chooseTauriWorkspaceRoot();
      if (selected) {
        setWorkspaceRoot(selected);
        const result = await rememberTauriWorkbenchProjectFolder(selected);
        setProjectFolders(result.folders.map(folder => ({ root: folder.root, title: folder.title })));
        setProjectFoldersWarning(result.warning ?? "");
      }
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function saveProjectTitle(root: string) {
    if (!root || busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await renameTauriWorkbenchProjectFolder(root, projectTitleDraft);
      setProjectFolders(result.folders.map(folder => ({ root: folder.root, title: folder.title })));
      setProjectFoldersWarning(result.warning ?? "");
      setEditingProjectRoot(null);
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
    if (busy || isReadOnlyWorkbenchSource(sessionPageSource) || (session && (!streamReady || session.state !== "idle"))) return;
    setBusy(true);
    setError("");
    try {
      const selected = await chooseTauriAttachmentFiles();
      if (selected.length === 0) return;
      if (!session) {
        setDraftAttachmentPaths(previous => [...previous, ...selected]);
        return;
      }
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
    const text = prompt.trim();
    if (busy || submitInFlightRef.current || isReadOnlyWorkbenchSource(sessionPageSource) ||
        (!text && attachments.length === 0 && draftAttachmentPaths.length === 0)) return;
    if (session && (!streamReady || session.state !== "idle")) return;
    submitInFlightRef.current = true;
    setBusy(true);
    setError("");
    if (!session) {
      const sessionId = newTauriSessionId();
      const root = workspaceRoot.trim() || undefined;
      try {
        // The new session is opened only after the first Send. The event
        // listener must become ready before its turn is submitted.
        const opened = await switchTauriBridgeSession(sessionId, root);
        pendingDraftSubmissionRef.current = {
          sessionId, text, paths: [...draftAttachmentPaths], workspaceRoot: root,
        };
        setSession(opened);
        setStreamReady(false);
      } catch (cause) {
        setError(tauriMessageFrom(cause));
        submitInFlightRef.current = false;
        setBusy(false);
      }
      return;
    }
    await submitPreparedInput(session.id, text, attachments, [], false, session.workspaceRoot);
  }

  async function submitPreparedInput(
    sessionId: string,
    text: string,
    readyAttachments: TauriBridgeAttachment[],
    pendingPaths: string[],
    firstTurn: boolean,
    root?: string,
  ) {
    const submitEpoch = ++turnEpochRef.current;
    setLiveText("");
    setPendingUserMessage(text || null);
    try {
      const prepared = [...readyAttachments];
      let attachmentError = "";
      for (const path of pendingPaths) {
        try {
          prepared.push(await attachTauriFile(sessionId, path));
        } catch (cause) {
          attachmentError ||= tauriMessageFrom(cause);
        }
      }
      const input = tauriComposerInput(text, prepared);
      if (!input) {
        if (firstTurn) {
          await deleteTauriBridgeSession(sessionId);
          setSession(null);
          setStreamReady(false);
        }
        setError(attachmentError ? `文件未能添加：${attachmentError}` : "请输入消息或添加文件");
        setPendingUserMessage(null);
        return;
      }
      const submitted = await submitTauriBridge(sessionId, input);
      if (sessionId !== session?.id) return;
      if (turnEpochRef.current === submitEpoch) setSession(submitted);
      if (firstTurn) await rememberSession({ ...submitted, workspaceRoot: root });
      if (!tauriSessionTitle(submitted.title, "") && !tabs.find(tab => tab.sessionId === sessionId)?.title) {
        const title = titleFromFirstUser(input);
        if (title) {
          try {
            const updated = await backfillTauriWorkbenchTitles([{ sessionId, title }]);
            const resolvedTitle = updated.find(tab => tab.sessionId === sessionId)?.title ?? title;
            if (isIdentityPageSource(sessionPageSource)) {
              setTabs(previous => previous.map(tab => tab.sessionId === sessionId ? { ...tab, title: resolvedTitle } : tab));
              await refreshCatalogAudit();
            } else {
              setTabs(previous => preserveWorkbenchLifecycle(updated, previous));
            }
          } catch {
            // The turn was accepted; catalog enrichment must not report it as a failed send.
          }
        }
      }
      setPrompt("");
      setAttachments([]);
      setDraftAttachmentPaths([]);
      if (attachmentError) setError(`部分文件未能添加：${attachmentError}`);
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

  async function importStableProjectFolders() {
    if (!profile?.projectFoldersImportAvailable || busy) return;
    const confirmed = window.confirm(
      "将稳定版保存的项目文件夹名称和路径复制到隔离的 Tauri Preview？只导入文件夹清单，不导入会话、topic 或其他配置；不会修改稳定版。",
    );
    if (!confirmed) return;
    setBusy(true);
    setError("");
    setProfileNotice("");
    try {
      const result = await importTauriStableProjectFolders();
      const folderResult = await tauriWorkbenchProjectFolders();
      setProjectFolders(folderResult.folders.map(folder => ({ root: folder.root, title: folder.title })));
      setProjectFoldersWarning(folderResult.warning ?? "");
      setProfile(await tauriPreviewProfileStatus());
      setProfileNotice(`已导入 ${result.projectCount} 个项目文件夹：\n${result.importedFile}`);
    } catch (cause) {
      setError(tauriMessageFrom(cause));
    } finally {
      setBusy(false);
    }
  }

  async function openScanImportReview() {
    if (!profile?.managedProfile || busy || scanImportLoading) return;
    setScanImportOpen(true);
    setScanImportLoading(true);
    setScanImportError("");
    try {
      const result = await tauriScanUnclaimedSessions();
      setScanImportCandidates(result.candidates);
      setScanImportBlockedCount(result.blockedCount);
      setScanImportDrafts(Object.fromEntries(result.candidates.map(candidate => [
        candidate.id,
        { selected: false, title: "", workspaceRoot: "" },
      ])));
    } catch (cause) {
      setScanImportError(tauriMessageFrom(cause));
    } finally {
      setScanImportLoading(false);
    }
  }

  async function applyScanImportReview() {
    if (!profile?.managedProfile || busy || scanImportLoading) return;
    const selected: TauriScanImportSelection[] = scanImportCandidates.flatMap(candidate => {
      const draft = scanImportDrafts[candidate.id];
      return draft?.selected ? [{
        id: candidate.id,
        title: draft.title,
        workspaceRoot: draft.workspaceRoot,
        transcriptSha256: candidate.transcriptSha256,
      }] : [];
    });
    if (selected.length === 0) return;
    const confirmed = window.confirm(
      `将这些会话加入隔离的 Preview 身份目录？\n\n${selected.map(item => `${item.id} · ${item.title || "无标题"} · ${item.workspaceRoot || "不归属项目"}`).join("\n")}\n\n每个 transcript 会在写入前重新核对 SHA-256；源文件内容不会改写。`,
    );
    if (!confirmed) return;
    setScanImportLoading(true);
    setScanImportError("");
    try {
      const imported = await tauriImportUnclaimedSessions(selected);
      setScanImportOpen(false);
      setSessionPageNotice(`已审核导入 ${imported.length} 个未认领会话。`);
      await retryWorkbenchSessionDirectory();
    } catch (cause) {
      setScanImportError(tauriMessageFrom(cause));
    } finally {
      setScanImportLoading(false);
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
    composerRef.current?.focus();
  }

  const currentWorkspace = workspaceRoot || session?.workspaceRoot || "";
  const activeProjectKey = workbenchProjectKey(currentWorkspace, platform);

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
        <button className="tauri-sidebar__new" type="button" onClick={() => void createSession()} disabled={busy || isReadOnlyWorkbenchSource(sessionPageSource) || switchingBlocked}>
          <Plus size={17} aria-hidden="true" /><span>新建对话</span><kbd>⌘ N</kbd>
        </button>
        {pendingSessionDeletes.length > 0 && <>
          <div className="tauri-sidebar__section-title">待完成删除</div>
          <section className="tauri-pending-deletes" aria-label="待完成删除">
            {pendingSessionDeletes.map(item => {
              const tab: WorkbenchSessionTab = { sessionId: item.id, title: item.title, state: "deleting", deletionInterrupted: true };
              return <SessionRow key={item.id} tab={tab} active={false} busy={busy} switchingBlocked={switchingBlocked && session?.id === item.id} onActivate={() => {}} onDelete={() => void deleteSession(tab)} />;
            })}
            {pendingSessionDeleteCursor && <button
              className="tauri-sidebar__load-more"
              type="button"
              onClick={() => void loadMorePendingSessionDeletes()}
              disabled={pendingSessionDeleteLoading || busy}
            >{pendingSessionDeleteLoading ? "正在加载待完成删除…" : "加载更多待完成删除"}</button>}
          </section>
        </>}
        {pendingSessionDeleteLoading && <p className="tauri-sidebar__page-note" role="status">正在检查待完成删除…</p>}
        {pendingSessionDeleteError && <div className="tauri-pending-deletes__error" role="alert">
          <span>读取待完成删除失败：{pendingSessionDeleteError}</span>
          <button type="button" onClick={() => void retryPendingSessionDeleteCheck()} disabled={busy}>重新检查</button>
        </div>}
        {pendingSessionTitleRecoveries.length > 0 && <>
          <div className="tauri-sidebar__section-title">待恢复标题</div>
          <section className="tauri-pending-deletes" aria-label="待恢复标题">
            <p className="tauri-sidebar__page-note">打开会话以核对上次改名；当前会话需先切换。也可确认删除该会话并放弃未完成的改名。</p>
            {pendingSessionTitleRecoveries.map(item => {
              const tab: WorkbenchSessionTab = {
                sessionId: item.id, title: item.title, workspaceRoot: item.workspaceRoot,
                state: item.state, missing: item.state === "missing", titleRecoveryPending: true,
              };
              return <SessionRow key={item.id} tab={tab} active={session?.id === item.id} busy={busy || sessionPageSource === "cached"} switchingBlocked={switchingBlocked} onActivate={() => void activateSession(item.id, item.workspaceRoot)} onDelete={() => void deleteSession(tab)} />;
            })}
          </section>
        </>}
        {pendingSessionTitleRecoveryError && <div className="tauri-pending-deletes__error" role="alert">
          <span>读取待恢复标题失败：{pendingSessionTitleRecoveryError}</span>
          <button type="button" onClick={() => void retryPendingSessionTitleRecoveryCheck()} disabled={busy}>重新检查</button>
        </div>}
        <div className="tauri-sidebar__section-title">项目</div>
        {sessionPageCursor && <p className="tauri-sidebar__page-note" role="status">项目组会话数按当前已加载页统计；继续加载后数量可能变化。</p>}
        {projectFoldersWarning && <div className="tauri-pending-deletes__error" role="status">
          <span>{projectFoldersWarning}</span>
          <button type="button" onClick={() => void reloadWorkbenchProjectFolders()} disabled={busy}>重试读取项目文件夹</button>
        </div>}
        <nav className="tauri-sidebar__sessions">
          {projectGroups.length === 0 ? <p className="tauri-sidebar__empty">还没有对话，开始一个新话题吧。</p> : projectGroups.map(group => group.root ? (
            <section className="tauri-project-group" key={group.key} aria-label={group.label}>
              <div className="tauri-project-group__heading">
                <button type="button" className="tauri-project-group__toggle" aria-label={`${collapsedProjects[group.key] ? "展开" : "收起"} ${group.label}`} aria-expanded={!collapsedProjects[group.key]} onClick={() => setCollapsedProjects(previous => {
                  const next = { ...previous };
                  if (next[group.key]) delete next[group.key];
                  else next[group.key] = true;
                  return next;
                })}>
                  {collapsedProjects[group.key] ? <ChevronRight size={13} /> : <ChevronDown size={13} />}
                </button>
                {editingProjectRoot === group.root ? <form className="tauri-project-title-edit" onSubmit={event => { event.preventDefault(); void saveProjectTitle(group.root!); }}>
                  <input autoFocus maxLength={1024} value={projectTitleDraft} aria-label={`重命名项目 ${group.label}`} onChange={event => setProjectTitleDraft(event.target.value)} onKeyDown={event => { if (event.key === "Escape") setEditingProjectRoot(null); }} />
                  <button type="submit" aria-label="保存项目名称" disabled={busy}><Check size={13} /></button>
                  <button type="button" aria-label="取消重命名项目" disabled={busy} onClick={() => setEditingProjectRoot(null)}><X size={13} /></button>
                </form> : <button type="button" className={`tauri-project-group__select${activeProjectKey === group.key ? " is-active" : ""}`} aria-current={activeProjectKey === group.key ? "location" : undefined} title={projectGroupHasUnloadedSessions(group, projectHistoryMayBeIncomplete) ? projectHistoryUnavailableHint : workspaceAvailability[group.root] === false ? `${group.root}\n工作区不可用；已有会话仍可打开` : group.root} aria-label={`切换到项目 ${group.label}${projectGroupHasUnloadedSessions(group, projectHistoryMayBeIncomplete) ? sessionPageSource === "legacy" ? "（持久目录未核验，先重新检查）" : "（历史会话尚未加载，先加载更多）" : workspaceAvailability[group.root] === false ? "（工作区不可用）" : ""}`} disabled={busy || isReadOnlyWorkbenchSource(sessionPageSource) || switchingBlocked || projectGroupHasUnloadedSessions(group, projectHistoryMayBeIncomplete) || (group.sessions.length > 0 && !group.sessions.some(tab => !isMissingWorkbenchSession(tab)))} onClick={() => { const latest = group.sessions.find(tab => !isMissingWorkbenchSession(tab)); if (latest) { if (latest.sessionId !== session?.id) void activateSession(latest.sessionId, latest.workspaceRoot); } else setWorkspaceRoot(group.root || ""); }}><FolderOpen size={14} /><span>{group.label}</span>{projectGroupHasUnloadedSessions(group, projectHistoryMayBeIncomplete) && <small className="tauri-project-group__unloaded">{sessionPageSource === "legacy" ? "目录未核验" : "历史未加载"}</small>}{workspaceAvailability[group.root] === false && <small className="tauri-project-group__unavailable">工作区不可用</small>}<small>{group.sessions.length}</small></button>}
                <button type="button" className="tauri-project-group__rename" aria-label={`重命名项目 ${group.label}`} title="重命名项目" disabled={busy || switchingBlocked || editingProjectRoot !== null} onClick={() => { setEditingProjectRoot(group.root || null); setProjectTitleDraft(group.title ?? ""); }}><Pencil size={12} /></button>
                <button type="button" className="tauri-project-group__new" aria-label={`在 ${group.label} 中新建对话`} title={workspaceAvailability[group.root] === false ? "工作区不可用，无法在此处新建对话" : "在此项目新建对话"} disabled={busy || isReadOnlyWorkbenchSource(sessionPageSource) || switchingBlocked || workspaceAvailability[group.root] === false} onClick={() => void createSession(group.root || "")}><Plus size={14} /></button>
              </div>
              {!collapsedProjects[group.key] && group.sessions.map(tab => <SessionRow key={tab.sessionId} tab={tab} active={session?.id === tab.sessionId} busy={busy || isReadOnlyWorkbenchSource(sessionPageSource)} switchingBlocked={switchingBlocked} onActivate={() => void activateSession(tab.sessionId, tab.workspaceRoot)} onDelete={() => void deleteSession(tab)} />)}
            </section>
          ) : group.sessions.map(tab => <SessionRow key={tab.sessionId} tab={tab} active={session?.id === tab.sessionId} busy={busy || isReadOnlyWorkbenchSource(sessionPageSource)} switchingBlocked={switchingBlocked} onActivate={() => void activateSession(tab.sessionId, tab.workspaceRoot)} onDelete={() => void deleteSession(tab)} />))}
          {sessionPageSource === "identity_unverified" && <p className="tauri-sidebar__page-note" role="status">
            正在只读显示未完成 shadow 核验的持久身份目录（{sessionPageDirectoryCount ?? tabs.length} 条）；不能据此打开、重命名、删除或新建会话。原因：{sessionPageCatalogWarning ? "旧会话兼容目录不可读" : catalogAudit ? sessionShadowDifferenceSummary(catalogAudit) : "shadow 报告不可用"}。请先重新检查并处理差异。
          </p>}
          {unverifiedLegacyTabs.length > 0 && <section className="tauri-pending-deletes" aria-label="仅在旧兼容目录中的未核验会话">
            <div className="tauri-sidebar__section-title">仅在旧目录中（只读）</div>
            {unverifiedLegacyTabs.map(tab => <SessionRow key={tab.sessionId} tab={tab} active={false} busy switchingBlocked={switchingBlocked} onActivate={() => {}} onDelete={() => {}} />)}
          </section>}
          {sessionPageCursor && <button type="button" className="tauri-sidebar__load-more" onClick={() => void loadMoreWorkbenchSessions()} disabled={sessionPageLoading} aria-label="加载更多会话">
            {sessionPageLoading ? "正在加载…" : "加载更多会话"}
          </button>}
          {sessionPageError && <p className="tauri-sidebar__page-error" role="alert">加载失败：{sessionPageError}</p>}
          {sessionPageCatalogWarning && <p className="tauri-sidebar__page-note" role="alert">{sessionPageCatalogWarning}</p>}
          {sessionPageNotice && <p className="tauri-sidebar__page-note" role="status">{sessionPageNotice}</p>}
          <button type="button" className="tauri-sidebar__load-more" aria-label="重新检查会话目录" onClick={() => void retryWorkbenchSessionDirectory()} disabled={busy || sessionPageLoading}>
            {sessionPageLoading ? "正在重新检查…" : "重新检查会话目录"}
          </button>
          {sessionPageSource === "legacy" && <>
            <p className="tauri-sidebar__page-note" role="status">
              当前使用本地兼容目录，最多 50 条；{sessionPageDirectoryCount !== null && sessionPageDirectoryCount !== visibleTabs.length && <>身份目录本次盘点为 {sessionPageDirectoryCount} 条（仅是审计数量，无法从当前回退列表翻页），</>}
              持久会话目录未通过校验，列表可能不完整。{catalogAudit ? `原因：${sessionShadowDifferenceSummary(catalogAudit)}。` : "本次未能读取 shadow 诊断，暂时无法确定回退原因。"}
              点击“重新检查会话目录”会重试旧目录导入并重新核验；未认领 transcript 不会自动导入，差异详情见“运行状态”。
            </p>
          </>}
          {sessionPageSource === "partial_identity" && <p className="tauri-sidebar__page-note" role="status">
            当前分页显示已登记且通过身份结构、工作区与磁盘状态核验的会话。
            {sessionPageTitleMismatchCount ? `其中 ${sessionPageTitleMismatchCount} 条标题与本地兼容目录不同，当前显示持久身份目录标题。` : ""}
            {sessionPageMissingTranscriptCount ? `其中 ${sessionPageMissingTranscriptCount} 条已确认 transcript 缺失；对应行会标记“文件缺失”、禁止打开，但可显式删除失效记录。` : ""}
            {sessionPageUnclaimedCount ? `另发现 ${sessionPageUnclaimedCount} 个未认领 transcript，尚未导入本目录；这些文件不会自动分配或认领 ID，需经审核导入。` : ""}
          </p>}
          {sessionPageSource === "cached" && <p className="tauri-sidebar__page-note" role="status">当前无法完成实时目录校验，分页来自本次运行中上一次审计的快照，共 {sessionPageDirectoryCount ?? tabs.length} 条；{catalogAudit && !isSessionShadowSafeToPage(catalogAudit) ? `当时已发现目录差异：${sessionShadowDifferenceSummary(catalogAudit)}。` : "该快照当时通过目录结构核验。"}{sessionPageUnclaimedCount ? `另报告 ${sessionPageUnclaimedCount} 个未认领 transcript。` : ""}内容可能已过期且当前只读。服务恢复后点击“重新检查会话目录”，完成实时校验后才能操作。</p>}
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
                {session && <button type="button" className="tauri-title-rename" aria-label="重命名对话" title="重命名对话" disabled={busy || switchingBlocked || isReadOnlyWorkbenchSource(sessionPageSource)} onClick={beginTitleEdit}><Pencil size={13} /></button>}
              </>}
            </div>
            <span data-tauri-drag-region>{session?.state === "running" ? "正在生成" : session?.state === "paused" ? "等待你的操作" : session ? "本地会话" : "Reasonix Preview"}</span>
          </div>
          <div className="tauri-topbar__actions">
            <button type="button" className="tauri-workspace-button" onClick={() => void chooseWorkspaceRoot()} disabled={busy || session?.state === "running"} title={currentWorkspace ? `新对话默认工作区：${currentWorkspace}` : "选择工作区（用于新对话）"}>
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
            {historyPresentation.map(item => item.kind === "message"
              ? <HistoryMessageArticle key={`message-${item.entry.index}`} entry={item.entry} sessionId={history!.session.id} questionId={item.entry.message.role === "user" ? `tauri-question-${item.entry.index}` : undefined} />
              : <details className="tauri-progress" key={`progress-${item.entries[0].index}`}>
                <summary><ChevronRight size={14} aria-hidden="true" /><span>{item.active ? "正在处理" : formatTauriWorkDuration(item.durationMs) ?? "过程记录"}</span><small>{item.entries.length} 条过程更新</small></summary>
                <div className="tauri-progress__messages">{item.entries.map(entry => <HistoryMessageArticle key={entry.index} entry={entry} sessionId={history!.session.id} />)}</div>
              </details>)}
            {/* Optimistic user message: shown immediately after submit, before history loads */}
            {pendingUserMessage && <article className="tauri-message is-user"><div className="tauri-message__content"><Markdown text={pendingUserMessage} /></div></article>}
            {liveText && <details className="tauri-progress tauri-progress--live"><summary><ChevronRight size={14} aria-hidden="true" /><span>正在处理</span><small>展开查看当前输出</small></summary><div className="tauri-progress__messages"><article className="tauri-message is-assistant tauri-message--live"><div className="tauri-message__avatar" aria-hidden="true"><Sparkles size={16} /></div><div className="tauri-message__content"><div className="tauri-message__role">Reasonix</div><Markdown text={liveText} streaming cacheKey={`${session?.id ?? "live"}:stream`} /></div></article></div></details>}
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
            {!session && <button className="tauri-welcome__start" type="button" onClick={() => composerRef.current?.focus()} disabled={busy}><Plus size={16} />开始新对话</button>}
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
            {draftAttachmentPaths.length > 0 && <div className="tauri-composer__attachments" aria-label="待发送文件">{draftAttachmentPaths.map((path, index) => <div className="tauri-composer__attachment" key={`${path}-${index}`} title={path}>
              <span className="tauri-composer__attachment-icon"><FileText size={15} /></span><span className="tauri-composer__attachment-name">{path.split(/[\\/]/).pop() || path}</span><small>待发送</small>
              <button type="button" onClick={() => setDraftAttachmentPaths(previous => previous.filter((_, itemIndex) => itemIndex !== index))} disabled={busy} aria-label={`移除待发送文件 ${path.split(/[\\/]/).pop() || path}`}><X size={13} /></button>
            </div>)}</div>}
            <textarea ref={composerRef} value={prompt} onChange={event => setPrompt(event.target.value)} onKeyDown={event => {
              if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) { event.preventDefault(); void submit(); }
            }} placeholder={session?.state === "paused" ? "请先完成上方确认…" : session ? "继续聊聊你的问题…（⌘/Ctrl + Enter 发送）" : "输入问题，开始新对话…（⌘/Ctrl + Enter 发送）"} disabled={session?.state === "paused"} rows={3} />
            <div className="tauri-composer__bottom"><span>{session?.workspaceRoot ? `当前对话工作区 · ${session.workspaceRoot}` : session ? "当前对话使用默认工作区" : currentWorkspace ? `新对话默认工作区 · ${currentWorkspace}` : "Preview 配置与稳定版相互隔离"}</span>
              <div className="tauri-composer__actions">
                <button className="tauri-attach-button" type="button" onClick={() => void addAttachments()} disabled={busy || isReadOnlyWorkbenchSource(sessionPageSource) || Boolean(session && (!streamReady || session.state !== "idle"))} aria-label="添加文件" title="从本机选择文件并附加到消息"><Paperclip size={16} /><span>添加文件</span></button>
                {session?.state === "running" ? <button className="tauri-send-button is-stop" type="button" onClick={() => void cancel()} disabled={busy} aria-label="停止生成"><Square size={15} fill="currentColor" /></button> : <button className="tauri-send-button" type="button" onClick={() => void submit()} disabled={busy || isReadOnlyWorkbenchSource(sessionPageSource) || Boolean(session && (!streamReady || session.state === "paused")) || (!prompt.trim() && attachments.length === 0 && draftAttachmentPaths.length === 0)} aria-label="发送消息"><ArrowUp size={18} /></button>}
              </div>
            </div>
          </div>
          <p className="tauri-composer-hint">Reasonix 可能会出错，请核对重要信息。<button type="button" onClick={() => setDiagnosticsOpen(true)}>预览版说明</button></p>
        </footer>
      </section>

      {scanImportOpen && <>
        <button className="tauri-scan-import__scrim" type="button" aria-label="关闭未认领会话审核" onClick={() => !scanImportLoading && setScanImportOpen(false)} />
        <section className="tauri-scan-import" role="dialog" aria-modal="true" aria-labelledby="tauri-scan-import-title">
          <header className="tauri-scan-import__header">
            <div><p>PREVIEW SESSION IMPORT</p><h2 id="tauri-scan-import-title">审核未认领会话</h2></div>
            <button type="button" className="tauri-icon-button" aria-label="关闭" onClick={() => setScanImportOpen(false)} disabled={scanImportLoading}><X size={17} /></button>
          </header>
          <div className="tauri-scan-import__body">
            <p>文件名可确定的 ID 会固定显示。逐项确认是否导入，并填写标题和项目路径；项目路径留空表示不归属项目。提交前会重新核对文件指纹。</p>
            {scanImportLoading && <p role="status">正在核对会话目录…</p>}
            {scanImportError && <p className="tauri-scan-import__error" role="alert">{scanImportError}</p>}
            {scanImportBlockedCount > 0 && <p className="tauri-scan-import__note">{scanImportBlockedCount} 个扫描文件因命名、文件状态或冲突未进入审核清单。</p>}
            {!scanImportLoading && scanImportCandidates.length === 0 && !scanImportError && <p className="tauri-scan-import__empty">没有可审核的未认领 transcript。</p>}
            <div className="tauri-scan-import__list">
              {scanImportCandidates.map(candidate => {
                const draft = scanImportDrafts[candidate.id] ?? { selected: false, title: "", workspaceRoot: "" };
                return <article className="tauri-scan-import__item" key={candidate.id}>
                  <label className="tauri-scan-import__choice">
                    <input type="checkbox" checked={draft.selected} onChange={event => setScanImportDrafts(previous => ({ ...previous, [candidate.id]: { ...draft, selected: event.target.checked } }))} />
                    <span><strong>{candidate.id}</strong><small>{candidate.file} · SHA-256 {candidate.transcriptSha256.slice(0, 12)}…</small></span>
                  </label>
                  <label>标题<input value={draft.title} onChange={event => setScanImportDrafts(previous => ({ ...previous, [candidate.id]: { ...draft, title: event.target.value } }))} maxLength={120} placeholder="留空表示确认无标题" /></label>
                  <label>项目路径<input value={draft.workspaceRoot} onChange={event => setScanImportDrafts(previous => ({ ...previous, [candidate.id]: { ...draft, workspaceRoot: event.target.value } }))} maxLength={4096} placeholder="绝对路径；留空表示不归属项目" /></label>
                </article>;
              })}
            </div>
          </div>
          <footer className="tauri-scan-import__footer">
            <span>已选 {Object.values(scanImportDrafts).filter(item => item.selected).length} 项</span>
            <button type="button" onClick={() => setScanImportOpen(false)} disabled={scanImportLoading}>取消</button>
            <button type="button" className="is-primary" onClick={() => void applyScanImportReview()} disabled={scanImportLoading || !Object.values(scanImportDrafts).some(item => item.selected)}>审核并导入</button>
          </footer>
        </section>
      </>}

      {diagnosticsOpen && <>
        <button className="tauri-diagnostics__scrim" aria-label="关闭运行状态面板" type="button" onClick={() => setDiagnosticsOpen(false)} />
        <aside className="tauri-diagnostics" aria-label="运行状态与设置">
          <header className="tauri-diagnostics__header"><div><p>TAURI PREVIEW</p><h2>运行状态与设置</h2></div><button type="button" className="tauri-icon-button" onClick={() => setDiagnosticsOpen(false)} aria-label="关闭"><X size={17} /></button></header>
          <div className="tauri-diagnostics__body">
            <section className="tauri-diagnostic-card"><div className="tauri-diagnostic-card__heading"><h3>本地服务</h3><span className={`tauri-health${status?.running ? " is-ready" : ""}`}><i />{status?.running ? `运行中 · 协议 v${status.protocolVersion ?? "?"}` : "正在连接"}</span></div><button type="button" className="tauri-diagnostic-action" onClick={() => void restartBridge()} disabled={busy}>重启桥接服务{session ? "并恢复当前会话" : ""}</button></section>
            <section className="tauri-diagnostic-card" aria-label="会话目录迁移检查">
              <div className="tauri-diagnostic-card__heading"><h3>会话目录迁移检查</h3><span>{catalogAudit ? "影子比对" : "检查中"}</span></div>
              {catalogAudit ? <>
                <p>旧目录 {catalogAudit.legacyCount} 条 · 身份目录 {catalogAudit.directoryCount} 条；当前侧栏数据源：{sessionPageSource === "identity" ? "持久身份目录" : sessionPageSource === "partial_identity" ? "已核验身份目录（存在差异）" : sessionPageSource === "identity_unverified" ? "未核验身份目录（只读）" : sessionPageSource === "legacy" ? "本地兼容目录" : sessionPageSource === "cached" ? "上次审计快照" : "暂不可用"}。</p>
                <p>{catalogAudit.legacyMatchesDirectory ? "旧目录中可见会话与身份目录一致" : `差异：身份库缺项 ${catalogAudit.missingFromDirectory}、标题 ${catalogAudit.titleMismatches}、工作区 ${catalogAudit.workspaceMismatches}、顺序 ${catalogAudit.orderMismatches}、磁盘状态 ${catalogAudit.physicalStateMismatches}、未登记文件 ${catalogAudit.unclaimedTranscripts}、盘点错误 ${catalogAudit.inventoryErrors}`}{catalogAudit.retiredLegacyCount > 0 && <>；另有 {catalogAudit.retiredLegacyCount} 条待删除或已删除记录由生命周期状态解释</>}</p>
                {catalogAudit.directoryOnlyCount > 0 && <p>身份目录新增项：{catalogAudit.directoryOnlyCount} 条</p>}
                {catalogAudit.missingTranscripts > 0 && <p>transcript 缺失：{catalogAudit.missingTranscripts} 条</p>}
              </> : <p>{catalogAuditError || "正在分页读取身份目录并与旧目录比较…"}</p>}
              {profile?.managedProfile && <button type="button" className="tauri-diagnostic-action" onClick={() => void openScanImportReview()} disabled={busy || scanImportLoading}>扫描并审核未认领 transcript</button>}
            </section>
            <section className="tauri-diagnostic-card">
              <h3>预览配置</h3>
              {profile ? <><p>配置与会话保存在独立预览目录：</p><code>{profile.previewHome}</code>{profile.importAvailable ? <><p>检测到稳定版配置。复制前会创建备份，不会改动稳定版。</p><button type="button" className="tauri-diagnostic-action" onClick={() => void importStableProfile()} disabled={busy}>复制稳定版配置（先备份）</button></> : <p>{profile.previewConfigExists ? "预览配置已存在，不会覆盖。" : profile.managedProfile ? "未发现可复制的稳定版配置。" : "检测到自定义 REASONIX_HOME，已停用自动导入。"}</p>}
                {profile.projectFoldersImportAvailable ? <><p>可单独导入稳定版保存的实际项目文件夹；只复制根路径和显示名称。</p><button type="button" className="tauri-diagnostic-action" onClick={() => void importStableProjectFolders()} disabled={busy}>导入旧版项目文件夹</button></> : profile.projectFoldersFileExists ? <p>当前配置目录中已有项目文件夹清单。</p> : <p>{profile.managedProfile ? "没有可导入的稳定版项目文件夹清单。" : "自定义 REASONIX_HOME 下停用稳定版项目文件夹导入。"}</p>}
              </> : <p>正在检查隔离配置…</p>}
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
