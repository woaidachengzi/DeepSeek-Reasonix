import { Component, lazy, Suspense, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Activity, ArrowUp, Check, ChevronDown, ChevronRight, Eye, FileText, FolderOpen, FolderTree, GitBranch, Keyboard, MessageSquare, Paperclip, PanelLeftClose, PanelLeftOpen, Pencil, Plus, Search, Settings, Sparkles, Square, Trash2, X } from "lucide-react";
import type { UnlistenFn } from "@tauri-apps/api/event";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { sendTauriSystemNotification } from "../lib/tauriBridge";
import { useTauriNotificationClicks } from "./tauriNotifications";
import { Markdown } from "../components/Markdown";
const TranscriptSelectionMenu = lazy(() => import("../components/TranscriptSelectionMenu").then(module => ({ default: module.TranscriptSelectionMenu })));
import { ExternalOpener } from "../components/ExternalOpener";
import { tauriExternalOpenerBridge } from "./tauriExternalOpener";
import { CopyButton } from "../components/CopyButton";
import { ComposerContextCard } from "../components/ComposerContextCard";
import { formatSelectedTextContext, normalizeSelectedText, selectedTextSnippet, splitSelectedTextContext, type SelectedTextReference } from "../lib/selectedTextContext";
import { onTauriOpenSettings } from "../lib/tauriBridge";
import { CommandPalette, type PaletteItem } from "../components/CommandPalette";
import { ShortcutsCheatsheet, type ShortcutCheatsheetItem } from "../components/ShortcutsCheatsheet";
import { QuestionJumpBar } from "../components/QuestionJumpBar";
import { parseAttachmentRefsForDisplay } from "../lib/attachmentDisplay";
import { compactQuestionText, type QuestionAnchor } from "../lib/transcriptGrouping";
import { LocaleProvider, useI18n, useT, type DictKey } from "../lib/i18n";
import { ToastProvider, useToast } from "../lib/toast";
import { applyTextSize, getTextSize, nextTextSize, DEFAULT_TEXT_SIZE } from "../lib/textSize";
import { playSuccessChime, playAttentionChime, shouldPlayAttentionChimeForEvent } from "../lib/sound";
import { generativeMusic, isGenerativeMusicEnabled } from "../lib/generative-music";
import logoWordmark from "../assets/logo-wordmark.svg";
import { TAURI_SHORTCUT_LABELS, TauriSettings, type TauriSettingsTab } from "./TauriSettings";
import { detectShortcutPlatform, formatShortcutCombo, type ShortcutSection } from "../lib/keyboardShortcuts";
import { defaultTauriShortcut, getTauriShortcut, isTauriCompositionKey, matchesTauriShortcut, TAURI_SHORTCUT_ACTIONS, TAURI_SHORTCUT_TABS, useTauriShortcuts, type TauriShortcutAction } from "./tauriKeyboardShortcuts";
import { TauriStatusBar } from "./TauriStatusBar";
import { useTauriDesktopLayout } from "./tauriDesktopLayout";
import { getTauriDefaultWorkspace, setTauriDefaultWorkspace } from "./tauriDefaultWorkspace";
import { observeTauriUsage, type TauriObservedUsage } from "./tauriObservedUsage";
import { isTauriNotificationEnabled, getTauriProgressMode, getTauriSidebarVisible, setTauriSidebarVisible, TAURI_PROGRESS_MODE_CHANGED, type TauriNotificationKind } from "./tauriPreferences";
import { handleTauriDragDropEvent, retainTauriDragDropListener } from "./dragDrop";
import { formatTauriWorkDuration, groupTauriHistory, type IndexedHistoryMessage } from "./historyPresentation";
import { applyTerminalThemePreference, onTerminalThemePreferenceChange } from "../lib/terminalTheme";
import { applyTauriAppearance, readTauriAppearance } from "./tauriAppearance";
import { applyThemePack } from "../lib/themePack";
import { tauriThemePackById } from "./tauriThemeCatalog";
import { tauriWorkspaceRecoveryCopy } from "./tauriWorkspaceRecoveryCopy";

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


function UserMessageContent({ text }: { text: string }) {
  const context = splitSelectedTextContext(text);
  return <>
    <Markdown text={parseAttachmentRefsForDisplay(context.submitText).text} />
    {context.entries.map((entry, index) => <details key={index} className="tauri-message__selection">
      <summary>{selectedTextSnippet(entry.text)}</summary>
      <Markdown text={entry.text} />
    </details>)}
  </>;
}

function HistoryMessageArticle({ entry, sessionId, questionId }: {
  entry: IndexedHistoryMessage;
  sessionId: string;
  questionId?: string;
}) {
  const { message, index } = entry;
  const display = message.role === "user" ? parseAttachmentRefsForDisplay(splitSelectedTextContext(message.content).submitText) : null;
  const created = message.createdAtMs && Number.isSafeInteger(message.createdAtMs) ? new Date(message.createdAtMs) : null;
  const createdAt = created && !Number.isNaN(created.getTime()) ? created : null;
  return <MessageErrorBoundary index={index}>
    <article id={questionId} data-tauri-question-anchor={questionId} className={`tauri-message is-${message.role}`}>
      {message.role !== "user" && <div className="tauri-message__avatar" aria-hidden="true"><Sparkles size={16} /></div>}
      <div className="tauri-message__content">
        {message.role !== "user" && <div className="tauri-message__role">Reasonix</div>}
        {message.role === "user" ? <UserMessageContent text={message.content ?? ""} /> : <Markdown text={message.content ?? ""} cacheKey={`${sessionId}:${index}`} />}
        {display && display.attachments.length > 0 && <div className="tauri-message__attachments">{display.attachments.map(attachment => <span key={attachment.path} title={attachment.path}><Paperclip size={13} />{attachment.name}</span>)}</div>}
        {message.truncated && <small>为保护界面性能，这条历史内容已截断。</small>}
      </div>
      {message.role === "user" && <div className="tauri-message__meta">
        {createdAt && <time dateTime={createdAt.toISOString()}>{createdAt.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time>}
        <CopyButton text={message.content} label="复制消息" showInlineLabel={false} />
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
  setTauriTrayLocale,
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
  setTauriBridgeSessionModel,
  startTauriBridgeEvents,
  submitTauriBridge,
  switchTauriBridgeSession,
  tauriWorkspace,
  tauriWorkspaceFile,
  tauriWorkspaceChanges,
  tauriWorkspaceChangeDetail,
  tauriWorkspaceFileRevertPreview,
  tauriWorkspaceFileRevertCommit,
  tauriWorkspaceFileRevertUndo,
  tauriWorkspaceCheckpoints,
  tauriCodeRewindPreview,
  tauriCodeRewindCommit,
  tauriConversationRewindPreview,
  tauriConversationRewindCommit,
  tauriConversationRewindUndo,
  tauriLegacyForkPreview,
  tauriLegacyForkCommit,
  tauriSessionHeads,
  tauriSessionHeadSwitch,
  tauriCombinedRewindPreview,
  tauriCombinedRewindCommit,
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
  tauriPendingSessionDeletesPage,
  tauriPendingSessionTitleRecoveries,
  tauriWorkbenchSessionPage,
  tauriScanUnclaimedSessions,
  tauriImportUnclaimedSessions,
  tauriWorkbenchProjectFolders,
  tauriWorkspaceRootsAvailability,
  tauriSessionPreviews,
  tauriSafeMCPURL,
  openTauriExternalURL,
  tauriTitleError,
  tauriTurnFailure,
  tauriImportLegacySessionCatalog,
  tauriSessionCatalogShadow,
  tauriDesktopPreferences,
  tauriActiveThemeId,
  tauriUserThemes,
  type TauriBridgeEvent,
  type TauriBridgeAttachment,
  type TauriBridgeHistory,
  type TauriBridgeSession,
  type TauriSessionMetrics,
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
  type TauriWorkspaceFileRevertPlan,
  type TauriWorkspaceCheckpoint,
  type TauriCodeRewindPlan,
  type TauriConversationRewindPlan,
  type TauriSessionHead,
  type TauriCombinedRewindPlan,
  type TauriPreviewProfileStatus,
  type TauriPreviewRuntimeInfo,
  type TauriProviderSummary,
  type TauriSessionShadowReport,
} from "../lib/tauriBridge";
import { filterWorkbenchProjectGroups, groupWorkbenchSessions, needsFirstMessageTitle, titleFromFirstUser, workbenchProjectKey, type WorkbenchProjectFolder } from "./workbenchSessions";
import { sessionLifecycleFailure, sessionLifecycleNotice } from "./sessionLifecycleError";
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
  onOpenExternalURL: (url: string) => void;
}

function PromptCard({ prompt, busy, selections, onApproval, onAskSelection, onAskSubmit, onMCPAction, onOpenExternalURL }: PromptCardProps) {
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
    {safeURL && <a className="tauri-prompt-card__link" href={safeURL} onClick={event => { event.preventDefault(); onOpenExternalURL(safeURL); }}>打开外部链接（{new URL(safeURL).host}）</a>}
    <div className="tauri-prompt-card__actions"><button type="button" className="tauri-prompt-card__allow" onClick={() => onMCPAction("accept")} disabled={busy}>接受并继续</button><button type="button" className="tauri-prompt-card__deny" onClick={() => onMCPAction("decline")} disabled={busy}>拒绝</button><button type="button" className="tauri-prompt-card__cancel" onClick={() => onMCPAction("cancel")} disabled={busy}>取消</button></div>
  </section>;
}

export function TauriSessionPreview() {
  const { locale, pref: languagePref, setPref: setLanguagePref } = useI18n();
  const t = useT();
  const { showToast } = useToast();
  useEffect(() => {
    let active = true;
    void setTauriTrayLocale(locale).catch(() => {
      if (active) showToast(locale === "en"
        ? "Could not update the tray language. Restart Preview and retry."
        : "托盘语言未能更新，请重启 Preview 后重试。", "warn");
    });
    return () => { active = false; };
  }, [locale, showToast]);
  const notificationFailureShown = useRef(false);
  const notificationLocale = useRef(locale);
  useLayoutEffect(() => { notificationLocale.current = locale; }, [locale]);
  const recoveryCopy = tauriWorkspaceRecoveryCopy[locale];
  const languagePrefRef = useRef(languagePref);
  languagePrefRef.current = languagePref;
  const initialLanguagePrefRef = useRef(languagePref);
  const desktopLayout = useTauriDesktopLayout();
  const shortcutOverrides = useTauriShortcuts();
  const shortcutPlatform = detectShortcutPlatform();
  const newSessionShortcutLabel = formatShortcutCombo(shortcutOverrides.new_session ?? defaultTauriShortcut("new_session", shortcutPlatform), shortcutPlatform);
  const sendShortcutLabel = formatShortcutCombo(shortcutOverrides.send_message ?? defaultTauriShortcut("send_message", shortcutPlatform), shortcutPlatform);
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
  const [sessionPageInitialLoading, setSessionPageInitialLoading] = useState(true);
  const [sessionPageNotice, setSessionPageNotice] = useState("");
  const [sessionPageError, setSessionPageError] = useState("");
  const [workspaceAvailability, setWorkspaceAvailability] = useState<Record<string, boolean | null>>({});
  const [collapsedProjects, setCollapsedProjects] = useState<Record<string, boolean>>(loadCollapsedProjectGroups);
  const [sessionSearch, setSessionSearch] = useState("");
  const [defaultWorkspace, setDefaultWorkspace] = useState(getTauriDefaultWorkspace);
  const [workspaceRoot, setWorkspaceRoot] = useState(getTauriDefaultWorkspace);
  const [editingProjectRoot, setEditingProjectRoot] = useState<string | null>(null);
  const [projectTitleDraft, setProjectTitleDraft] = useState("");
  const [prompt, setPrompt] = useState("");
  const [selectedTextDrafts, setSelectedTextDrafts] = useState<Record<string, SelectedTextReference[]>>({});
  const selectedTextSequence = useRef(0);
  const selectedTexts = session ? selectedTextDrafts[session.id] ?? [] : [];
  const addSelectedText = useCallback((text: string) => {
    if (!session?.id) return;
    const normalized = normalizeSelectedText(text);
    if (!normalized.text) return;
    if (normalized.truncated) showToast(t("composer.selectedTextTruncated"), "warn");
    const reference = { id: `chat-selection-${++selectedTextSequence.current}`, text: normalized.text };
    const sessionId = session.id;
    setSelectedTextDrafts(previous => {
      const current = previous[sessionId] ?? [];
      return current.some(item => item.text === reference.text) ? previous : { ...previous, [sessionId]: [...current, reference] };
    });
    composerRef.current?.focus();
  }, [session?.id, showToast, t]);
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
  const [events, setEvents] = useState<TauriBridgeEvent[]>([]);
  const [observedUsage, setObservedUsage] = useState<TauriObservedUsage | null>(null);
  const [sessionMetrics, setSessionMetrics] = useState<{ sessionId: string; metrics: TauriSessionMetrics } | null>(null);
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
  const [settingsTab, setSettingsTab] = useState<TauriSettingsTab>("general");
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  const [shortcutsHelpOpen, setShortcutsHelpOpen] = useState(false);
  const [progressMode, setProgressMode] = useState(getTauriProgressMode);
  const [sidebarVisible, setSidebarVisible] = useState(getTauriSidebarVisible);
  const [workspaceOpen, setWorkspaceOpen] = useState(false);
  const [workspacePath, setWorkspacePath] = useState("");
  const [workspaceEntries, setWorkspaceEntries] = useState<TauriWorkspaceEntry[]>([]);
  const [workspaceLoading, setWorkspaceLoading] = useState(false);
  const [workspaceError, setWorkspaceError] = useState("");
  const [workspaceTruncated, setWorkspaceTruncated] = useState(false);
  const [workspacePreview, setWorkspacePreview] = useState<TauriWorkspaceFilePreview | null>(null);
  const [workspacePreviewLoading, setWorkspacePreviewLoading] = useState(false);
  const [workspaceView, setWorkspaceView] = useState<"files" | "changes" | "checkpoints">("files");
  const [workspaceChanges, setWorkspaceChanges] = useState<TauriWorkspaceChanges | null>(null);
  const [workspaceChangesLoading, setWorkspaceChangesLoading] = useState(false);
  const [workspaceChangeDetail, setWorkspaceChangeDetail] = useState<TauriWorkspaceChangeDetail | null>(null);
  const [workspaceChangeDetailPath, setWorkspaceChangeDetailPath] = useState("");
  const [workspaceChangeDetailLoading, setWorkspaceChangeDetailLoading] = useState(false);
  const [workspaceFileRevertPlan, setWorkspaceFileRevertPlan] = useState<TauriWorkspaceFileRevertPlan | null>(null);
  const [workspaceFileRevertBusy, setWorkspaceFileRevertBusy] = useState(false);
  const [workspaceFileRevertMessage, setWorkspaceFileRevertMessage] = useState("");
  const [workspaceFileRevertUndo, setWorkspaceFileRevertUndo] = useState<{ sessionId: string; transactionId: string; path: string } | null>(null);
  const [workspaceCheckpoints, setWorkspaceCheckpoints] = useState<TauriWorkspaceCheckpoint[]>([]);
  const [workspaceCheckpointsLoading, setWorkspaceCheckpointsLoading] = useState(false);
  const [codeRewindPlan, setCodeRewindPlan] = useState<TauriCodeRewindPlan | null>(null);
  const [codeRewindCoverageConfirmed, setCodeRewindCoverageConfirmed] = useState(false);
  const [conversationRewindPlan, setConversationRewindPlan] = useState<TauriConversationRewindPlan | null>(null);
  const [legacyForkPlan, setLegacyForkPlan] = useState<TauriConversationRewindPlan | null>(null);
  const [conversationRewindUndo, setConversationRewindUndo] = useState<{ sessionId: string; headId: string } | null>(null);
  const [sessionHeads, setSessionHeads] = useState<TauriSessionHead[]>([]);
  const [combinedRewindPlan, setCombinedRewindPlan] = useState<TauriCombinedRewindPlan | null>(null);
  const [combinedCoverageConfirmed, setCombinedCoverageConfirmed] = useState(false);
  const [combinedRewindUndo, setCombinedRewindUndo] = useState<{ sessionId: string; transactionId: string; headId: string } | null>(null);
  const [titleEditing, setTitleEditing] = useState(false);
  const [titleDraft, setTitleDraft] = useState("");
  const [error, setError] = useState("");
  const [pendingPrompt, setPendingPrompt] = useState<TauriPendingPrompt | null>(null);
  const [promptSelections, setPromptSelections] = useState<Record<string, string[]>>({});
  const conversationRef = useRef<HTMLDivElement>(null);
  const composerRef = useRef<HTMLTextAreaElement>(null);
  const turnEpochRef = useRef(0);
  const attentionChimeSeenRef = useRef(new Set<string>());
  const notificationPromptSeenRef = useRef(new Set<string>());
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
  const workspaceFileRevertRequestRef = useRef(0);
  const workspaceCheckpointsRequestRef = useRef(0);
  const [activeQuestion, setActiveQuestion] = useState<number | null>(null);
  const switchingBlocked = Boolean(session && session.state !== "idle");
  useTauriNotificationClicks({
    canOpen: click => click.sessionId === session?.id || (!busy && !switchingBlocked && !isReadOnlyWorkbenchSource(sessionPageSource)),
    open: async target => {
      setSettingsOpen(false); setDiagnosticsOpen(false); setWorkspaceOpen(false);
      if (target.sessionId === session?.id) return true;
      return activateSession(target.sessionId, target.workspaceRoot ?? undefined);
    },
    unavailable: () => showToast(t("composer.sessionContextReadFailed"), "warn"),
    failed: () => showToast(t("notifications.openFailed"), "warn"),
  }, { busy, blocked: switchingBlocked, source: sessionPageSource, sessionId: session?.id });
  const [platform, setPlatform] = useState<string>("");
  // Optimistic user message: displayed immediately after submit, cleared when history loads
  const [pendingUserMessage, setPendingUserMessage] = useState<string | null>(null);
  // Drag-and-drop state
  const [dragging, setDragging] = useState(false);
  const pendingSessionDeleteIDs = useMemo(() => new Set(pendingSessionDeletes.map(item => item.id)), [pendingSessionDeletes]);
  const visibleTabs = useMemo(() => tabs.filter(tab => !pendingSessionDeleteIDs.has(tab.sessionId)), [tabs, pendingSessionDeleteIDs]);
  const shortcutHelpItems = useMemo<ShortcutCheatsheetItem[]>(() => TAURI_SHORTCUT_ACTIONS.map(action => {
    const section: ShortcutSection = action === "show_shortcuts" ? "help"
      : action === "workspace_files" || action === "diagnostics" ? "tools"
        : action === "new_session" || action === "send_message" || action.startsWith("goto_session_") ? "session"
          : action === "settings" || action === "command_palette" || action === "close_panel" ? "global" : "view";
    const label = TAURI_SHORTCUT_LABELS[action];
    return { action, section, labelKey: label.label, descriptionKey: label.description, combo: getTauriShortcut(action, shortcutPlatform) };
  }), [shortcutOverrides, shortcutPlatform]);
  const commandPaletteItems = useMemo<PaletteItem[]>(() => {
    const commandGroup = t("palette.group.commands");
    const sessionGroup = t("palette.group.sessions");
    const openSettingsTab = (tab: TauriSettingsTab) => {
      setSettingsTab(tab);
      setDiagnosticsOpen(false);
      setSettingsOpen(true);
    };
    const settingsTabs: Array<{ tab: TauriSettingsTab; label: DictKey; keywords: string[] }> = [
      { tab: "general", label: "settings.tab.general", keywords: ["general", "通用"] },
      { tab: "appearance", label: "settings.tab.appearance", keywords: ["appearance", "theme", "外观", "主题"] },
      { tab: "model", label: "settings.models.preferences", keywords: ["model", "模型"] },
      { tab: "providers", label: "settings.models.services", keywords: ["provider", "service", "模型服务"] },
      { tab: "stats", label: "settings.modelTab.stats", keywords: ["usage", "stats", "用量", "统计"] },
      { tab: "bots", label: "settings.tab.bots", keywords: ["bot", "机器人"] },
      { tab: "mcp", label: "settings.tab.mcp", keywords: ["mcp", "tools", "工具"] },
      { tab: "remote", label: "settings.tab.remote", keywords: ["ssh", "remote", "远程"] },
      { tab: "skills", label: "settings.tab.skills", keywords: ["skills", "技能"] },
      { tab: "subagents", label: "settings.tab.subagents", keywords: ["subagent", "子智能体"] },
      { tab: "plugins", label: "settings.tab.plugins", keywords: ["plugin", "插件"] },
      { tab: "hooks", label: "settings.tab.hooks", keywords: ["hooks", "钩子"] },
      { tab: "memory", label: "settings.tab.memory", keywords: ["memory", "记忆"] },
      { tab: "permissions", label: "settings.tab.permissions", keywords: ["permissions", "权限"] },
      { tab: "sandbox", label: "settings.tab.sandbox", keywords: ["sandbox", "沙箱"] },
      { tab: "network", label: "settings.tab.network", keywords: ["network", "proxy", "网络", "代理"] },
      { tab: "data", label: "settings.tab.storage", keywords: ["storage", "data", "存储"] },
      { tab: "shortcuts", label: "settings.tab.shortcuts", keywords: ["shortcuts", "快捷键"] },
      { tab: "updates", label: "settings.tab.updates", keywords: ["updates", "更新"] },
      { tab: "about", label: "settings.about.navLabel", keywords: ["about", "版本"] },
    ];
    const commands: PaletteItem[] = [
      { id: "tauri-command-new-session", group: commandGroup, title: t("palette.cmd.newSession"), icon: <Plus size={15} />, compact: true, keywords: ["new", "新建"], run: () => { setSettingsOpen(false); setDiagnosticsOpen(false); if (!busy && !switchingBlocked) void createSession(); } },
      { id: "tauri-command-settings", group: commandGroup, title: t("palette.cmd.settings"), icon: <Settings size={15} />, compact: true, keywords: ["settings", "设置"], run: () => openSettingsTab("general") },
      { id: "tauri-command-shortcuts", group: commandGroup, title: t("shortcuts.action.showShortcuts"), icon: <Keyboard size={15} />, compact: true, keywords: ["shortcuts", "keyboard", "快捷键"], run: () => setShortcutsHelpOpen(true) },
      { id: "tauri-command-files", group: commandGroup, title: t("workspace.filesTab"), icon: <FolderTree size={15} />, compact: true, keywords: ["files", "workspace", "文件", "工作区"], run: () => { if (!session || busy) return; setSettingsOpen(false); setDiagnosticsOpen(false); setWorkspaceView("files"); setWorkspaceOpen(true); void loadWorkspace(workspacePath); } },
      { id: "tauri-command-diagnostics", group: commandGroup, title: t("settings.tab.diagnostics"), icon: <Activity size={15} />, compact: true, keywords: ["diagnostics", "运行状态", "诊断"], run: () => { setSettingsOpen(false); setDiagnosticsOpen(true); } },
      ...settingsTabs.map(({ tab, label, keywords }) => ({
        id: `tauri-settings-${tab}`,
        group: commandGroup,
        title: t(label),
        icon: <Settings size={15} />,
        compact: true,
        keywords,
        run: () => openSettingsTab(tab),
      })),
    ];
    const sessions: PaletteItem[] = (busy || switchingBlocked || isReadOnlyWorkbenchSource(sessionPageSource) ? [] : visibleTabs.filter(tab => !isMissingWorkbenchSession(tab) && !tab.deletionInterrupted)).map(tab => ({
      id: `tauri-session-${tab.sessionId}`,
      group: sessionGroup,
      title: displayTitle(tab.title),
      hint: tab.workspaceRoot,
      keywords: [tab.title ?? "", tab.workspaceRoot ?? "", "session", "会话"],
      run: () => {
        if (isReadOnlyWorkbenchSource(sessionPageSource)) return;
        setSettingsOpen(false);
        setDiagnosticsOpen(false);
        void activateSession(tab.sessionId, tab.workspaceRoot);
      },
    }));
    return [...commands, ...sessions];
  }, [t, busy, switchingBlocked, visibleTabs, session, sessionPageSource, defaultWorkspace, workspacePath]);
  const projectGroups = useMemo(() => groupWorkbenchSessions(visibleTabs, projectFolders, platform), [visibleTabs, projectFolders, platform]);
  const searchedProjectGroups = useMemo(() => filterWorkbenchProjectGroups(projectGroups, sessionSearch), [projectGroups, sessionSearch]);
  const searchingSessions = sessionSearch.trim().length > 0;
  const projectHistoryMayBeIncomplete = sessionPageCursor !== null || sessionPageSource === "legacy";
  const projectHistoryUnavailableHint = sessionPageSource === "legacy"
    ? "持久会话目录未核验，无法确认此项目是否还有历史会话；重新检查目录后再选择"
    : "还有未加载的会话页；加载更多以确认此项目是否为空";
  const projectRootsKey = projectGroups.flatMap(group => group.root ? [group.root] : []).join("\u0000");
  const activeCatalogTitle = tabs.find(tab => tab.sessionId === session?.id)?.title;

  useEffect(() => {
    const onProgressModeChanged = () => setProgressMode(getTauriProgressMode());
    window.addEventListener(TAURI_PROGRESS_MODE_CHANGED, onProgressModeChanged);
    return () => window.removeEventListener(TAURI_PROGRESS_MODE_CHANGED, onProgressModeChanged);
  }, []);

  useEffect(() => {
    let active = true;
    let changedLocally = false;
    const initialAppearance = readTauriAppearance();
    const unsubscribe = onTerminalThemePreferenceChange(() => { changedLocally = true; });
    void Promise.all([tauriDesktopPreferences(), tauriActiveThemeId(), tauriUserThemes()]).then(([preferences, themeId, userThemes]) => {
      if (!active) return;
      if (!changedLocally) applyTerminalThemePreference(preferences.terminalTheme);
      if (preferences.appearanceConfigured) {
        const currentAppearance = readTauriAppearance();
        if (currentAppearance.mode === initialAppearance.mode && currentAppearance.style === initialAppearance.style) {
          applyTauriAppearance({ mode: preferences.theme, style: preferences.themeStyle || "graphite" });
        }
      }
      const themePack = tauriThemePackById(themeId) ?? userThemes.find(theme => theme.id === themeId) ?? null;
      if (themePack) applyThemePack(themePack);
      if (languagePrefRef.current === initialLanguagePrefRef.current) setLanguagePref(preferences.language);
    }).catch(() => {});
    return () => { active = false; unsubscribe(); };
  }, [setLanguagePref]);

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
    let active = true;
    let unlisten: UnlistenFn | undefined;
    void onTauriOpenSettings(() => {
      if (active) { setSettingsTab("general"); setSettingsOpen(true); }
    }).then(off => { if (active) unlisten = off; else off(); }).catch(() => {});
    return () => { active = false; unlisten?.(); };
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (isTauriCompositionKey(event) || event.defaultPrevented || (event.target instanceof HTMLElement && event.target.closest(".tauri-shortcut-key.is-recording"))) return;
      const shortcutPlatform = detectShortcutPlatform();
      if (matchesTauriShortcut(event, "command_palette", shortcutPlatform)) {
        event.preventDefault();
        setCommandPaletteOpen(open => !open);
        return;
      }
      // The palette owns Escape and navigation keys; don't also trigger
      // shortcuts in the workspace underneath its modal surface.
      if (commandPaletteOpen) return;
      if (shortcutsHelpOpen) return;
      if (matchesTauriShortcut(event, "show_shortcuts", shortcutPlatform)) {
        event.preventDefault();
        setShortcutsHelpOpen(true);
        return;
      }
      // Escape: Close open panels (no modifier required)
      if (event.key === "Escape") {
        if (settingsOpen) { setSettingsOpen(false); return; }
        if (diagnosticsOpen) { setDiagnosticsOpen(false); return; }
        if (workspaceOpen) { setWorkspaceOpen(false); return; }
      }
      if (matchesTauriShortcut(event, "close_panel", shortcutPlatform) && (settingsOpen || diagnosticsOpen || workspaceOpen)) {
        event.preventDefault();
        if (settingsOpen) setSettingsOpen(false);
        else if (diagnosticsOpen) setDiagnosticsOpen(false);
        else setWorkspaceOpen(false);
      }
      else if (matchesTauriShortcut(event, "new_session", shortcutPlatform) && !busy && !switchingBlocked) {
        event.preventDefault();
        void createSession();
      }
      else if (matchesTauriShortcut(event, "settings", shortcutPlatform)) {
        event.preventDefault();
        setSettingsOpen(true);
      }
      else if (matchesTauriShortcut(event, "diagnostics", shortcutPlatform)) {
        event.preventDefault();
        setDiagnosticsOpen(true);
      }
      else if (matchesTauriShortcut(event, "toggle_sidebar", shortcutPlatform)) {
        event.preventDefault();
        setSidebarVisible(previous => {
          const next = !previous;
          setTauriSidebarVisible(next);
          return next;
        });
      }
      else if (matchesTauriShortcut(event, "workspace_files", shortcutPlatform) && session) {
        event.preventDefault();
        toggleWorkspace();
      }
      else if (matchesTauriShortcut(event, "refresh_session", shortcutPlatform) && session && !busy) {
        event.preventDefault();
        void refreshHistory();
      }
      else if (matchesTauriShortcut(event, "open_appearance", shortcutPlatform)) {
        event.preventDefault();
        setSettingsTab("appearance");
        setSettingsOpen(true);
      }
      else if (matchesTauriShortcut(event, "open_model_preferences", shortcutPlatform)) {
        event.preventDefault();
        setSettingsTab("model");
        setSettingsOpen(true);
      }
      else if (matchesTauriShortcut(event, "open_model_services", shortcutPlatform)) {
        event.preventDefault();
        setSettingsTab("providers");
        setSettingsOpen(true);
      }
      else if (matchesTauriShortcut(event, "open_usage_stats", shortcutPlatform)) {
        event.preventDefault();
        setSettingsTab("stats");
        setSettingsOpen(true);
      }
      else if (matchesTauriShortcut(event, "text_size_increase", shortcutPlatform)) {
        event.preventDefault();
        applyTextSize(nextTextSize(getTextSize(), 1));
      }
      else if (matchesTauriShortcut(event, "text_size_decrease", shortcutPlatform)) {
        event.preventDefault();
        applyTextSize(nextTextSize(getTextSize(), -1));
      }
      else if (matchesTauriShortcut(event, "text_size_reset", shortcutPlatform)) {
        event.preventDefault();
        applyTextSize(DEFAULT_TEXT_SIZE);
      }
      else {
        const sessionShortcut = TAURI_SHORTCUT_ACTIONS.find(action => action.startsWith("goto_session_")
          && matchesTauriShortcut(event, action, shortcutPlatform));
        if (sessionShortcut) {
          event.preventDefault();
          if (!settingsOpen && !diagnosticsOpen && !workspaceOpen && !busy && !switchingBlocked) {
            const position = Number(sessionShortcut.slice("goto_session_".length)) - 1;
            const target = visibleTabs.filter(tab => !isMissingWorkbenchSession(tab))[position];
            if (target && target.sessionId !== session?.id) void activateSession(target.sessionId, target.workspaceRoot);
          }
          return;
        }
        const settingsTarget = Object.entries(TAURI_SHORTCUT_TABS).find(([action]) =>
          matchesTauriShortcut(event, action as TauriShortcutAction, shortcutPlatform),
        );
        if (settingsTarget) {
          event.preventDefault();
          setSettingsTab(settingsTarget[1] as TauriSettingsTab);
          setSettingsOpen(true);
        }
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [busy, switchingBlocked, session, settingsOpen, diagnosticsOpen, workspaceOpen, commandPaletteOpen, shortcutsHelpOpen, visibleTabs]);

  useEffect(() => {
    void tauriBridgeStatus().then(setStatus).catch(error => setError(tauriMessageFrom(error)));
    void tauriPreviewProfileStatus().then(setProfile).catch(error => setError(tauriMessageFrom(error)));
    void tauriPreviewRuntimeInfo().then(setRuntimeInfo).catch(error => setError(tauriMessageFrom(error)));
    void tauriProviderSummary().then(setProviderSummary).catch(error => setError(tauriMessageFrom(error)));
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
        setSessionPageCatalogWarning(page.catalogWarning ? tauriMessageFrom(page.catalogWarning) : "");
        setSessionPageDirectoryCount(page.shadowDirectoryCount ?? (page.source === "identity" ? page.total : null));
        setSessionPageUnclaimedCount(page.unclaimedTranscriptCount ?? null);
        setSessionPageTitleMismatchCount(page.titleMismatchCount ?? null);
        setSessionPageMissingTranscriptCount(page.missingTranscriptCount ?? null);
        const missing = isIdentityPageSource(page.source)
          ? page.sessions.filter(tab => needsFirstMessageTitle(tab.title)).map(tab => tab.sessionId)
          : [];
        for (let offset = 0; active && offset < missing.length; offset += 50) {
          try {
            const previews = await tauriSessionPreviews(missing.slice(offset, offset + 50));
            if (!active) return;
            const titles = previews.flatMap(preview => {
              const stored = tauriSessionTitle(preview.title, "");
              const title = needsFirstMessageTitle(stored) ? titleFromFirstUser(preview.firstUser ?? "") : stored;
              return title ? [{ sessionId: preview.sessionId, title }] : [];
            });
            if (titles.length > 0) {
              const result = await backfillTauriWorkbenchTitles(titles);
              const resolved = new Map(result.resolvedTitles.map(item => [item.sessionId, item.title]));
              if (active) setTabs(previous => previous.map(tab => {
                const title = resolved.get(tab.sessionId);
                return title && needsFirstMessageTitle(tab.title) ? { ...tab, title } : tab;
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
          const message = `读取最近对话失败：${tauriMessageFrom(cause)}${importNotice}`;
          setError(message);
          setSessionPageError(message);
        }
      } finally {
        if (active) setSessionPageInitialLoading(false);
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
      setSessionPageCatalogWarning(page.catalogWarning ? tauriMessageFrom(page.catalogWarning) : "");
      setSessionPageDirectoryCount(page.shadowDirectoryCount ?? (page.source === "identity" ? page.total : null));
      setSessionPageUnclaimedCount(page.unclaimedTranscriptCount ?? null);
      setSessionPageTitleMismatchCount(page.titleMismatchCount ?? null);
      setSessionPageMissingTranscriptCount(page.missingTranscriptCount ?? null);
      const missing = isIdentityPageSource(page.source)
        ? page.sessions.filter(tab => needsFirstMessageTitle(tab.title)).map(tab => tab.sessionId)
        : [];
      for (let offset = 0; offset < missing.length; offset += 50) {
        try {
          const previews = await tauriSessionPreviews(missing.slice(offset, offset + 50));
          const titles = previews.flatMap(preview => {
            const stored = tauriSessionTitle(preview.title, "");
            const title = needsFirstMessageTitle(stored) ? titleFromFirstUser(preview.firstUser ?? "") : stored;
            return title ? [{ sessionId: preview.sessionId, title }] : [];
          });
          if (titles.length > 0) {
            const result = await backfillTauriWorkbenchTitles(titles);
            const resolved = new Map(result.resolvedTitles.map(item => [item.sessionId, item.title]));
            if (revision !== sessionPageRevisionRef.current) return;
            setTabs(previous => previous.map(tab => {
              const title = resolved.get(tab.sessionId);
              return title && needsFirstMessageTitle(tab.title) ? { ...tab, title } : tab;
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
        setSessionPageCatalogWarning(page.catalogWarning ? tauriMessageFrom(page.catalogWarning) : "");
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
    if (session?.state === "running" && isGenerativeMusicEnabled()) generativeMusic.start();
    else generativeMusic.stop();
  }, [session?.state]);
  useEffect(() => () => generativeMusic.stop(), []);

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
    setObservedUsage(null);
    setSessionMetrics(null);

    void (async () => {
      try {
        const snapshot = await tauriBridgeSnapshot(session.id);
        if (!active) return;
        setSession(snapshot.session);
        setSessionMetrics(snapshot.metrics ? { sessionId: session.id, metrics: snapshot.metrics } : null);
        setSequence(snapshot.sequence);
        let lastSequence = snapshot.sequence;
        offEvent = await onTauriBridgeEvent(event => {
          if (!active || resyncRequested || event.sessionId !== session.id || event.sequence <= lastSequence) return;
          const notify = (kind: TauriNotificationKind, failed = false) => {
            if (!isTauriNotificationEnabled(kind)) return;
            void sendTauriSystemNotification({ sessionId: session.id, kind, language: notificationLocale.current, failed }).then(() => {
              if (active) notificationFailureShown.current = false;
            }).catch(() => {
              if (active && !notificationFailureShown.current) {
                notificationFailureShown.current = true;
                showToast(t("notifications.deliveryFailed"), "warn");
              }
            });
          };
          lastSequence = event.sequence;
          setSequence(previous => Math.max(previous, event.sequence));
          setEvents(previous => [event, ...previous].slice(0, 100));
          if (event.eventKind === "usage" || event.eventKind === "turn_started" || event.eventKind === "stream_attempt" || event.eventKind === "turn_done") {
            setObservedUsage(previous => observeTauriUsage(previous, event));
          }
          if (event.eventKind === "turn_started") {
            turnEpochRef.current += 1;
            if (isGenerativeMusicEnabled()) generativeMusic.start();
            // Prompt ids may restart when a controller is rebuilt between turns.
            attentionChimeSeenRef.current.clear();
            notificationPromptSeenRef.current.clear();
            setSession(previous => previous ? { ...previous, state: "running" } : previous);
            setLiveText("");
          }
          const incomingPrompt = tauriPromptFromEvent(event);
          if (incomingPrompt) {
            if (incomingPrompt.kind === "approval" || incomingPrompt.kind === "ask") {
              const notificationKind = incomingPrompt.kind === "approval" ? "approval_request" : "ask_request";
              const soundEvent = incomingPrompt.kind === "approval"
                ? { kind: "approval_request", tabId: session.id, approval: { id: incomingPrompt.id } }
                : { kind: "ask_request", tabId: session.id, ask: { id: incomingPrompt.id } };
              if (shouldPlayAttentionChimeForEvent(soundEvent, attentionChimeSeenRef.current)) playAttentionChime();
              const notificationKey = `${notificationKind}:${incomingPrompt.id}`;
              if (!notificationPromptSeenRef.current.has(notificationKey)) {
                notificationPromptSeenRef.current.add(notificationKey);
                notify(notificationKind);
              }
            }
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
          if (event.eventKind === "text" || event.eventKind === "reasoning" || event.eventKind === "tool_dispatch") generativeMusic.playTokenNote();
          if (event.eventKind === "turn_done") {
            generativeMusic.stop();
            if (!tauriTurnFailure(event)) playSuccessChime();
            const completionEpoch = ++turnEpochRef.current;
            const completionIsCurrent = () => active && turnEpochRef.current === completionEpoch;
            setPendingPrompt(null);
            setPromptSelections({});
            setSession(previous => previous ? { ...previous, state: "running" } : previous);
            const failure = tauriTurnFailure(event);
            notify("turn_done", Boolean(failure));
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
                  setSessionMetrics(latest.metrics ? { sessionId: event.sessionId, metrics: latest.metrics } : null);
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
      setSessionPageCatalogWarning(page.catalogWarning ? tauriMessageFrom(page.catalogWarning) : "");
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
      setProjectFoldersWarning(result.warning ? tauriMessageFrom(result.warning) : "");
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
        setSessionPageCatalogWarning(page.catalogWarning ? tauriMessageFrom(page.catalogWarning) : "");
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
          setSessionPageCatalogWarning(page.catalogWarning ? tauriMessageFrom(page.catalogWarning) : "");
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
    workspaceFileRevertRequestRef.current += 1;
    workspaceCheckpointsRequestRef.current += 1;
    setWorkspaceFileRevertPlan(null);
    setCodeRewindPlan(null);
    setCodeRewindCoverageConfirmed(false);
    setConversationRewindPlan(null);
    setLegacyForkPlan(null);
    setConversationRewindUndo(null);
    setSessionHeads([]);
    setWorkspaceCheckpoints([]);
    setCombinedRewindPlan(null);
    setCombinedCoverageConfirmed(false);
    setCombinedRewindUndo(null);
    setWorkspaceFileRevertMessage("");
    setWorkspaceFileRevertUndo(null);
    setWorkspaceFileRevertBusy(false);
    setWorkspaceCheckpointsLoading(false);
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
      return false;
    }
    if (!id) { setError("缺少会话 ID"); return false; }
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
          setProjectFoldersWarning(result.warning ? tauriMessageFrom(result.warning) : "");
        } catch (cause) {
          setError(`对话已打开，但无法保存项目文件夹：${tauriMessageFrom(cause)}`);
        }
      }
      await rememberSession(next);
      await reloadPendingSessionTitleRecoveries();
      setStreamRevision(previous => previous + 1);
      setStatus(await tauriBridgeStatus());
      return true;
    } catch (cause) {
      setError(sessionLifecycleNotice(cause) ?? tauriMessageFrom(cause));
      return false;
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

  function createSession(root = defaultWorkspace) {
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

  async function chooseDefaultWorkspace(): Promise<string | null> {
    if (busy) return null;
    setBusy(true);
    setError("");
    try {
      const selected = await chooseTauriWorkspaceRoot();
      if (selected) {
        setTauriDefaultWorkspace(selected);
        setDefaultWorkspace(selected);
        setWorkspaceRoot(selected);
        try {
          const result = await rememberTauriWorkbenchProjectFolder(selected);
          setProjectFolders(result.folders.map(folder => ({ root: folder.root, title: folder.title })));
          setProjectFoldersWarning(result.warning ? tauriMessageFrom(result.warning) : "");
        } catch (cause) {
          setProjectFoldersWarning(`默认工作区已保存，但项目列表未更新：${tauriMessageFrom(cause)}`);
        }
      }
      return selected;
    } catch (cause) {
      setError(tauriMessageFrom(cause));
      throw cause;
    } finally {
      setBusy(false);
    }
  }

  function clearDefaultWorkspace() {
    setTauriDefaultWorkspace("");
    setDefaultWorkspace("");
    if (!session) setWorkspaceRoot("");
  }

  async function saveProjectTitle(root: string) {
    if (!root || busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await renameTauriWorkbenchProjectFolder(root, projectTitleDraft);
      setProjectFolders(result.folders.map(folder => ({ root: folder.root, title: folder.title })));
      setProjectFoldersWarning(result.warning ? tauriMessageFrom(result.warning) : "");
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
    workspaceFileRevertRequestRef.current += 1;
    setWorkspaceFileRevertPlan(null);
    setWorkspaceFileRevertMessage("");
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

  useEffect(() => {
    if (!workspaceOpen || workspaceView !== "changes") return;
    if (!session || session.state !== "idle") {
      workspaceChangesRequestRef.current += 1;
      workspaceDetailRequestRef.current += 1;
      workspaceFileRevertRequestRef.current += 1;
      setWorkspaceChangesLoading(false);
      setWorkspaceChangeDetailLoading(false);
      setWorkspaceFileRevertBusy(false);
      setWorkspaceChanges(null);
      setWorkspaceChangeDetail(null);
      setWorkspaceFileRevertPlan(null);
      return;
    }
    void loadWorkspaceChanges();
    return () => { workspaceChangesRequestRef.current += 1; };
  }, [workspaceOpen, workspaceView, session?.id, session?.state]);

  async function loadWorkspaceCheckpoints() {
    if (!session) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceCheckpointsRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceCheckpointsRequestRef.current === request;
    setWorkspaceCheckpointsLoading(true);
    setCodeRewindPlan(null);
    setCodeRewindCoverageConfirmed(false);
    setConversationRewindPlan(null);
    setLegacyForkPlan(null);
    setCombinedRewindPlan(null);
    setCombinedCoverageConfirmed(false);
    setWorkspaceError("");
    try {
      const [checkpoints, heads] = await Promise.all([tauriWorkspaceCheckpoints(sessionID), tauriSessionHeads(sessionID)]);
      if (isCurrent()) {
        setWorkspaceCheckpoints(checkpoints);
        setSessionHeads(heads);
      }
    } catch (cause) {
      if (isCurrent()) {
        setWorkspaceCheckpoints([]);
        setSessionHeads([]);
        setWorkspaceError(tauriMessageFrom(cause));
      }
    } finally {
      if (isCurrent()) setWorkspaceCheckpointsLoading(false);
    }
  }

  useEffect(() => {
    if (!workspaceOpen || workspaceView !== "checkpoints") return;
    if (!session || session.state !== "idle") {
      workspaceCheckpointsRequestRef.current += 1;
      workspaceFileRevertRequestRef.current += 1;
      setWorkspaceCheckpointsLoading(false);
      setWorkspaceFileRevertBusy(false);
      setWorkspaceCheckpoints([]);
      setCodeRewindPlan(null);
      setCodeRewindCoverageConfirmed(false);
      setConversationRewindPlan(null);
      setLegacyForkPlan(null);
      setCombinedRewindPlan(null);
      setCombinedCoverageConfirmed(false);
      return;
    }
    void loadWorkspaceCheckpoints();
    return () => { workspaceCheckpointsRequestRef.current += 1; };
  }, [workspaceOpen, workspaceView, session?.id, session?.state]);

  async function previewCodeRewind(turn: number) {
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setCodeRewindPlan(null);
    setCodeRewindCoverageConfirmed(false);
    setConversationRewindPlan(null);
    setLegacyForkPlan(null);
    setCombinedRewindPlan(null);
    setWorkspaceFileRevertMessage("");
    try {
      const plan = await tauriCodeRewindPreview(sessionID, turn);
      if (isCurrent()) setCodeRewindPlan(plan);
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.failed} ${tauriMessageFrom(cause)}`);
    } finally {
      if (isCurrent()) setWorkspaceFileRevertBusy(false);
    }
  }

  async function commitCodeRewind() {
    const plan = codeRewindPlan;
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy || !plan?.canFiles || !plan.planId) return;
    if (plan.requiresCoverageConfirmation && !codeRewindCoverageConfirmed) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setBusy(true);
    setWorkspaceFileRevertMessage("");
    try {
      const result = await tauriCodeRewindCommit(sessionID, plan.planId, codeRewindCoverageConfirmed);
      if (!isCurrent()) return;
      setCodeRewindPlan(null);
      if (result.ok) {
        setWorkspaceFileRevertUndo(result.undoAvailable && result.transactionId
          ? { sessionId: sessionID, transactionId: result.transactionId, path: `${recoveryCopy.tab} · ${plan.fileCount}` }
          : null);
        await loadWorkspaceCheckpoints();
        setWorkspaceFileRevertMessage(recoveryCopy.done);
      } else {
        setWorkspaceFileRevertMessage(recoveryCopy.failed);
      }
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.failed} ${tauriMessageFrom(cause)}`);
    } finally {
      setWorkspaceFileRevertBusy(false);
      setBusy(false);
    }
  }

  async function reloadConversationAfterRewind(sessionID: string) {
    const epoch = ++turnEpochRef.current;
    setHistory(null);
    setHistoryLoading(true);
    setHistoryError("");
    try {
      const [snapshot, latestHistory] = await Promise.all([
        tauriBridgeSnapshot(sessionID), tauriBridgeHistory(sessionID),
      ]);
      if (turnEpochRef.current !== epoch) return;
      setSession(snapshot.session);
      setSessionMetrics(snapshot.metrics ? { sessionId: sessionID, metrics: snapshot.metrics } : null);
      setSequence(previous => Math.max(previous, snapshot.sequence, latestHistory.sequence));
      setHistory(latestHistory);
      setHistoryError("");
      setPendingUserMessage(null);
      setLiveText("");
    } finally {
      if (turnEpochRef.current === epoch) setHistoryLoading(false);
    }
  }

  async function previewConversationRewind(turn: number) {
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setCodeRewindPlan(null);
    setConversationRewindPlan(null);
    setLegacyForkPlan(null);
    setCombinedRewindPlan(null);
    setWorkspaceFileRevertMessage("");
    try {
      const plan = await tauriConversationRewindPreview(sessionID, turn);
      if (isCurrent()) setConversationRewindPlan(plan);
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.conversation.failed} ${tauriMessageFrom(cause)}`);
    } finally {
      if (isCurrent()) setWorkspaceFileRevertBusy(false);
    }
  }

  async function previewLegacyFork(turn: number) {
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setCodeRewindPlan(null);
    setConversationRewindPlan(null);
    setLegacyForkPlan(null);
    setCombinedRewindPlan(null);
    setWorkspaceFileRevertMessage("");
    try {
      const plan = await tauriLegacyForkPreview(sessionID, turn);
      if (isCurrent()) setLegacyForkPlan(plan);
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.legacyFork.failed} ${tauriMessageFrom(cause)}`);
    } finally {
      if (isCurrent()) setWorkspaceFileRevertBusy(false);
    }
  }

  async function commitLegacyFork() {
    const plan = legacyForkPlan;
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy || !plan?.canConversation || !plan.planId) return;
    const sessionID = session.id;
    const root = session.workspaceRoot;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setBusy(true);
    setWorkspaceFileRevertMessage("");
    try {
      const result = await tauriLegacyForkCommit(sessionID, plan.planId);
      if (!isCurrent()) return;
      setLegacyForkPlan(null);
      if (!result.ok || !result.sessionId) {
        setWorkspaceFileRevertMessage(`${recoveryCopy.legacyFork.failed} ${result.error || ""}`);
        return;
      }
      const nextPageRequest = ++catalogAuditRequestRef.current;
      await reloadFirstWorkbenchSessionPage(nextPageRequest, isCurrent);
      if (!isCurrent()) return;
      setBusy(false);
      setWorkspaceFileRevertBusy(false);
      await activateSession(result.sessionId, root);
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.legacyFork.failed} ${tauriMessageFrom(cause)}`);
    } finally {
      if (isCurrent()) {
        setWorkspaceFileRevertBusy(false);
        setBusy(false);
      }
    }
  }

  async function commitConversationRewind() {
    const plan = conversationRewindPlan;
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy || !plan?.canConversation || !plan.planId) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setBusy(true);
    setWorkspaceFileRevertMessage("");
    try {
      const result = await tauriConversationRewindCommit(sessionID, plan.planId);
      if (!isCurrent()) return;
      setConversationRewindPlan(null);
      if (!result.ok || !result.conversationForked || !result.headId) {
        setWorkspaceFileRevertMessage(`${recoveryCopy.conversation.failed} ${result.error || ""}`);
        return;
      }
      setConversationRewindUndo({ sessionId: sessionID, headId: result.headId });
      setWorkspaceFileRevertMessage(recoveryCopy.conversation.done);
      try {
        await reloadConversationAfterRewind(sessionID);
      } catch (cause) {
        setHistoryError(tauriMessageFrom(cause));
      }
      await loadWorkspaceCheckpoints();
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.conversation.failed} ${tauriMessageFrom(cause)}`);
    } finally {
      if (isCurrent()) {
        setWorkspaceFileRevertBusy(false);
        setBusy(false);
      }
    }
  }

  async function undoConversationRewind() {
    const undo = conversationRewindUndo;
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy || !undo || undo.sessionId !== session.id) return;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setBusy(true);
    try {
      const result = await tauriConversationRewindUndo(undo.sessionId, undo.headId);
      if (!isCurrent()) return;
      if (!result.ok) {
        setWorkspaceFileRevertMessage(`${recoveryCopy.conversation.failed} ${result.error || ""}`);
        return;
      }
      setConversationRewindUndo(null);
      setWorkspaceFileRevertMessage(recoveryCopy.conversation.undone);
      try {
        await reloadConversationAfterRewind(undo.sessionId);
      } catch (cause) {
        setHistoryError(tauriMessageFrom(cause));
      }
      await loadWorkspaceCheckpoints();
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.conversation.failed} ${tauriMessageFrom(cause)}`);
    } finally {
      if (isCurrent()) {
        setWorkspaceFileRevertBusy(false);
        setBusy(false);
      }
    }
  }

  async function previewCombinedRewind(turn: number) {
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setCodeRewindPlan(null);
    setConversationRewindPlan(null);
    setLegacyForkPlan(null);
    setCombinedRewindPlan(null);
    setCombinedCoverageConfirmed(false);
    setWorkspaceFileRevertMessage("");
    try {
      const plan = await tauriCombinedRewindPreview(sessionID, turn);
      if (isCurrent()) setCombinedRewindPlan(plan);
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.combined.failed} ${tauriMessageFrom(cause)}`);
    } finally {
      if (isCurrent()) setWorkspaceFileRevertBusy(false);
    }
  }

  async function commitCombinedRewind() {
    const plan = combinedRewindPlan;
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy || !plan?.canFiles || !plan.canConversation || !plan.planId || (plan.requiresCoverageConfirmation && !combinedCoverageConfirmed)) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setBusy(true);
    setWorkspaceFileRevertMessage("");
    try {
      const result = await tauriCombinedRewindCommit(sessionID, plan.planId, combinedCoverageConfirmed);
      if (!isCurrent()) return;
      setCombinedRewindPlan(null);
      if (!result.ok) {
        setWorkspaceFileRevertMessage(`${recoveryCopy.combined.failed} ${result.error || result.conflicts.join(", ")}`);
        return;
      }
      if (!result.conversationForked || !result.headId) {
        setWorkspaceFileRevertMessage(recoveryCopy.combined.failed);
        return;
      }
      setWorkspaceFileRevertUndo(null);
      if (result.partial) {
        setCombinedRewindUndo(null);
        setConversationRewindUndo({ sessionId: sessionID, headId: result.headId });
        setWorkspaceFileRevertMessage(recoveryCopy.combined.partial);
      } else {
        setConversationRewindUndo(null);
        setCombinedRewindUndo(result.undoAvailable && result.transactionId
          ? { sessionId: sessionID, transactionId: result.transactionId, headId: result.headId }
          : null);
        setWorkspaceFileRevertMessage(recoveryCopy.combined.done);
      }
      try {
        await reloadConversationAfterRewind(sessionID);
      } catch (cause) {
        setHistoryError(tauriMessageFrom(cause));
      }
      await loadWorkspaceCheckpoints();
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.combined.failed} ${tauriMessageFrom(cause)}`);
    } finally {
      if (isCurrent()) {
        setWorkspaceFileRevertBusy(false);
        setBusy(false);
      }
    }
  }

  async function undoCombinedRewind() {
    const undo = combinedRewindUndo;
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy || !undo || undo.sessionId !== session.id) return;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setBusy(true);
    try {
      const result = await tauriWorkspaceFileRevertUndo(undo.sessionId, undo.transactionId);
      if (!isCurrent()) return;
      if (!result.ok) {
        setWorkspaceFileRevertMessage(`${recoveryCopy.combined.undoFailed} ${result.error || ""}`);
        return;
      }
      setCombinedRewindUndo(null);
      try {
        await reloadConversationAfterRewind(undo.sessionId);
      } catch (cause) {
        setHistoryError(tauriMessageFrom(cause));
      }
      try {
        const heads = await tauriSessionHeads(undo.sessionId);
        if (isCurrent()) setWorkspaceFileRevertMessage(heads.some(head => head.id === undo.headId && head.selected)
          ? recoveryCopy.combined.undoFilesOnly : recoveryCopy.combined.undone);
      } catch {
        if (isCurrent()) setWorkspaceFileRevertMessage(recoveryCopy.combined.undoSelectionUnknown);
      }
      await loadWorkspaceCheckpoints();
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.combined.undoFailed} ${tauriMessageFrom(cause)}`);
    } finally {
      if (isCurrent()) {
        setWorkspaceFileRevertBusy(false);
        setBusy(false);
      }
    }
  }

  async function switchSessionHead(headID: string) {
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setBusy(true);
    setWorkspaceFileRevertMessage("");
    try {
      await tauriSessionHeadSwitch(sessionID, headID);
      if (!isCurrent()) return;
      setConversationRewindUndo(null);
      setCombinedRewindUndo(null);
      setConversationRewindPlan(null);
      setCodeRewindPlan(null);
      setCombinedRewindPlan(null);
      setWorkspaceFileRevertMessage(recoveryCopy.heads.switched);
      try {
        await reloadConversationAfterRewind(sessionID);
      } catch (cause) {
        setHistoryError(tauriMessageFrom(cause));
      }
      await loadWorkspaceCheckpoints();
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.heads.failed} ${tauriMessageFrom(cause)}`);
    } finally {
      if (isCurrent()) {
        setWorkspaceFileRevertBusy(false);
        setBusy(false);
      }
    }
  }

  async function loadWorkspaceChangeDetail(path: string) {
    if (!session) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceDetailRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceDetailRequestRef.current === request;
    workspaceFileRevertRequestRef.current += 1;
    setWorkspaceFileRevertPlan(null);
    setWorkspaceFileRevertMessage("");
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

  async function previewWorkspaceFileRevert(path: string) {
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setWorkspaceFileRevertMessage("");
    try {
      const plan = await tauriWorkspaceFileRevertPreview(sessionID, path);
      if (isCurrent()) setWorkspaceFileRevertPlan(plan);
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.fileRevert.previewFailed} ${tauriMessageFrom(cause)}`);
    } finally {
      if (isCurrent()) setWorkspaceFileRevertBusy(false);
    }
  }

  async function commitWorkspaceFileRevert() {
    const plan = workspaceFileRevertPlan;
    if (!session || session.state !== "idle" || busy || workspaceFileRevertBusy || !plan?.canFiles || !plan.planId) return;
    const sessionID = session.id;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setBusy(true);
    setWorkspaceFileRevertMessage("");
    try {
      const result = await tauriWorkspaceFileRevertCommit(sessionID, plan.planId, plan.conflicts?.length ? "overwrite_checkpoint" : "");
      if (!isCurrent()) return;
      setWorkspaceFileRevertPlan(null);
      if (result.ok) {
        setWorkspaceFileRevertUndo(result.undoAvailable && result.transactionId
          ? { sessionId: sessionID, transactionId: result.transactionId, path: plan.path }
          : null);
        await loadWorkspaceChanges();
        setWorkspaceFileRevertMessage(recoveryCopy.fileRevert.done);
      } else {
        setWorkspaceFileRevertMessage(recoveryCopy.fileRevert.failed);
      }
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.fileRevert.failed} ${tauriMessageFrom(cause)}`);
    } finally {
      setWorkspaceFileRevertBusy(false);
      setBusy(false);
    }
  }

  async function undoWorkspaceFileRevert() {
    const undo = workspaceFileRevertUndo;
    if (!session || session.id !== undo?.sessionId || session.state !== "idle" || busy || workspaceFileRevertBusy) return;
    const epoch = workspaceEpochRef.current;
    const request = ++workspaceFileRevertRequestRef.current;
    const isCurrent = () => workspaceEpochRef.current === epoch && workspaceFileRevertRequestRef.current === request;
    setWorkspaceFileRevertBusy(true);
    setBusy(true);
    setWorkspaceFileRevertMessage("");
    try {
      const result = await tauriWorkspaceFileRevertUndo(undo.sessionId, undo.transactionId);
      if (!isCurrent()) return;
      if (result.ok) {
        setWorkspaceFileRevertUndo(null);
        if (workspaceView === "checkpoints") await loadWorkspaceCheckpoints();
        else await loadWorkspaceChanges();
        setWorkspaceFileRevertMessage(recoveryCopy.fileRevert.undone);
      } else {
        setWorkspaceFileRevertMessage(recoveryCopy.fileRevert.undoFailed);
      }
    } catch (cause) {
      if (isCurrent()) setWorkspaceFileRevertMessage(`${recoveryCopy.fileRevert.undoFailed} ${tauriMessageFrom(cause)}`);
    } finally {
      setWorkspaceFileRevertBusy(false);
      setBusy(false);
    }
  }

  function toggleWorkspace() {
    if (!session || busy) return;
    const nextOpen = !workspaceOpen;
    setWorkspaceOpen(nextOpen);
    if (nextOpen) {
      if (workspaceView === "files") void loadWorkspace(workspacePath);
    }
  }

  function showWorkspaceView(view: "files" | "changes" | "checkpoints") {
    workspaceFileRevertRequestRef.current += 1;
    setWorkspaceFileRevertPlan(null);
    setCodeRewindPlan(null);
    setCodeRewindCoverageConfirmed(false);
    setConversationRewindPlan(null);
    setLegacyForkPlan(null);
    setCombinedRewindPlan(null);
    setCombinedCoverageConfirmed(false);
    setWorkspaceFileRevertMessage("");
    setWorkspaceView(view);
    setWorkspaceError("");
    if (view === "files") {
      workspaceChangesRequestRef.current += 1;
      setWorkspaceChangesLoading(false);
      setWorkspaceChangeDetail(null);
      void loadWorkspace(workspacePath);
    } else if (view === "changes") {
      workspaceListRequestRef.current += 1;
      workspacePreviewRequestRef.current += 1;
      setWorkspaceLoading(false);
      setWorkspacePreviewLoading(false);
      setWorkspacePreview(null);
    } else {
      workspaceListRequestRef.current += 1;
      workspaceChangesRequestRef.current += 1;
      workspacePreviewRequestRef.current += 1;
      workspaceDetailRequestRef.current += 1;
      setWorkspaceLoading(false);
      setWorkspaceChangesLoading(false);
      setWorkspacePreviewLoading(false);
      setWorkspaceChangeDetailLoading(false);
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

  function insertSubagentInvocation(command: string) {
    setPrompt(previous => {
      const current = previous.trimEnd();
      return current ? `${current}\n\n${command}` : command;
    });
    setSettingsOpen(false);
    window.requestAnimationFrame(() => composerRef.current?.focus());
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
    const text = [prompt.trim(), formatSelectedTextContext(selectedTexts)].filter(Boolean).join("\n\n");
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
      if (needsFirstMessageTitle(submitted.title) && needsFirstMessageTitle(tabs.find(tab => tab.sessionId === sessionId)?.title)) {
        let firstUser = firstTurn ? input : "";
        if (!firstTurn) {
          try { firstUser = (await tauriSessionPreviews([sessionId]))[0]?.firstUser ?? ""; }
          catch { /* Title enrichment must not fail an accepted turn. */ }
        }
        const title = titleFromFirstUser(firstUser);
        if (title) {
          try {
            const result = await backfillTauriWorkbenchTitles([{ sessionId, title }]);
            const resolvedTitle = result.resolvedTitles.find(item => item.sessionId === sessionId)?.title;
            if (isIdentityPageSource(sessionPageSource)) {
              if (resolvedTitle) setTabs(previous => previous.map(tab => tab.sessionId === sessionId && needsFirstMessageTitle(tab.title) ? { ...tab, title: resolvedTitle } : tab));
              await refreshCatalogAudit();
            } else {
              setTabs(previous => preserveWorkbenchLifecycle(result.sessions, previous));
            }
          } catch {
            // The turn was accepted; catalog enrichment must not report it as a failed send.
          }
        }
      }
      setPrompt("");
      setSelectedTextDrafts(previous => {
        const remaining = (previous[sessionId] ?? []).filter(item => !selectedTexts.some(sent => sent.id === item.id));
        return { ...previous, [sessionId]: remaining };
      });
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
        setSessionMetrics(current.metrics ? { sessionId: session.id, metrics: current.metrics } : null);
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

  async function refreshProfile() {
    setProfile(await tauriPreviewProfileStatus());
  }

  async function importStableProfile(): Promise<string> {
    if (!profile?.importAvailable || busy) return "";
    const confirmed = window.confirm(
      t("settings.data.confirmProfileImport", { path: profile.stableConfig ?? t("settings.data.stableProfile"), target: profile.previewHome }),
    );
    if (!confirmed) return "";
    setBusy(true);
    setError("");
    try {
      const result = await importTauriStableProfile();
      const [profileStatus, providers] = await Promise.allSettled([tauriPreviewProfileStatus(), tauriProviderSummary()]);
      if (profileStatus.status === "fulfilled") setProfile(profileStatus.value);
      if (providers.status === "fulfilled") setProviderSummary(providers.value);
      const refreshWarning = profileStatus.status === "rejected" || providers.status === "rejected" ? `\n${t("settings.data.profileRefreshWarning")}` : "";
      return `${t("settings.data.profileImported", { path: result.importedConfig })}\n${t("settings.data.backupCreated", { path: result.backupConfig })}${refreshWarning}`;
    } catch (cause) {
      throw cause;
    } finally {
      setBusy(false);
    }
  }

  async function importStableProjectFolders(): Promise<string> {
    if (!profile?.projectFoldersImportAvailable || busy) return "";
    const confirmed = window.confirm(
      t("settings.data.confirmFoldersImport"),
    );
    if (!confirmed) return "";
    setBusy(true);
    setError("");
    try {
      const result = await importTauriStableProjectFolders();
      const [folders, profileStatus] = await Promise.allSettled([tauriWorkbenchProjectFolders(), tauriPreviewProfileStatus()]);
      if (folders.status === "fulfilled") {
        setProjectFolders(folders.value.folders.map(folder => ({ root: folder.root, title: folder.title })));
        setProjectFoldersWarning(folders.value.warning ? tauriMessageFrom(folders.value.warning) : "");
      }
      if (profileStatus.status === "fulfilled") setProfile(profileStatus.value);
      const refreshWarning = folders.status === "rejected" || profileStatus.status === "rejected" ? `\n${t("settings.data.foldersRefreshWarning")}` : "";
      return `${t("settings.data.foldersImported", { count: result.projectCount })}\n${result.importedFile}${refreshWarning}`;
    } catch (cause) {
      throw cause;
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
        setSessionMetrics(latest.metrics ? { sessionId, metrics: latest.metrics } : null);
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

  async function changeCurrentSessionModel(model: string): Promise<boolean> {
    if (!session || !model || busy || session.state !== "idle" || attachments.length > 0) return false;
    setBusy(true);
    setError("");
    try {
      const updated = await setTauriBridgeSessionModel(session.id, model);
      setSession(updated);
      await rememberSession(updated);
      try {
        const snapshot = await tauriBridgeSnapshot(updated.id);
        setSessionMetrics(snapshot.metrics ? { sessionId: updated.id, metrics: snapshot.metrics } : null);
      } catch {
        // The controller replacement succeeded; a metrics refresh can be retried separately.
      }
      return true;
    } catch (cause) {
      setError(tauriMessageFrom(cause));
      return false;
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

  return (
    <main className="tauri-shell" data-platform={platform} data-desktop-layout={desktopLayout} data-sidebar-hidden={sidebarVisible ? undefined : "true"}>
      <aside className="tauri-sidebar" aria-label="会话导航">
        <div className="tauri-sidebar__drag" data-tauri-drag-region aria-hidden="true" />
        <div className="tauri-sidebar__brand"><img src={logoWordmark} alt="Reasonix" draggable={false} /><span>PREVIEW</span></div>
        <button className="tauri-sidebar__new" type="button" onClick={() => void createSession()} disabled={busy || isReadOnlyWorkbenchSource(sessionPageSource) || switchingBlocked}>
          <Plus size={17} aria-hidden="true" /><span>新建对话</span><kbd>{newSessionShortcutLabel}</kbd>
        </button>
        <div className="tauri-sidebar__search">
          <Search size={15} aria-hidden="true" />
          <input type="search" aria-label="搜索会话和项目" placeholder="搜索会话或项目" value={sessionSearch} onChange={event => setSessionSearch(event.target.value)} />
          {sessionSearch && <button type="button" aria-label="清除会话搜索" onClick={() => setSessionSearch("")}><X size={14} /></button>}
        </div>
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
        {searchingSessions && sessionPageCursor && <p className="tauri-sidebar__page-note" role="status">仅搜索已加载的会话；加载更多后可找到更早的对话。</p>}
        {projectFoldersWarning && <div className="tauri-pending-deletes__error" role="status">
          <span>{projectFoldersWarning}</span>
          <button type="button" onClick={() => void reloadWorkbenchProjectFolders()} disabled={busy}>重试读取项目文件夹</button>
        </div>}
        <nav className="tauri-sidebar__sessions">
          {visibleTabs.length === 0 && sessionPageInitialLoading && <p className="tauri-sidebar__empty" role="status">正在加载对话…</p>}
          {visibleTabs.length === 0 && !sessionPageInitialLoading && sessionPageError && <p className="tauri-sidebar__empty" role="status">会话列表暂不可用。</p>}
          {searchedProjectGroups.length === 0 && !sessionPageInitialLoading && !sessionPageError ? <p className="tauri-sidebar__empty">{searchingSessions ? "没有匹配的会话或项目。" : "还没有对话，开始一个新话题吧。"}</p> : searchedProjectGroups.map(group => group.root ? (
            <section className="tauri-project-group" key={group.key} aria-label={group.label}>
              <div className="tauri-project-group__heading">
                <button type="button" className="tauri-project-group__toggle" aria-label={`${!searchingSessions && collapsedProjects[group.key] ? "展开" : "收起"} ${group.label}`} aria-expanded={searchingSessions || !collapsedProjects[group.key]} disabled={searchingSessions} onClick={() => setCollapsedProjects(previous => {
                  const next = { ...previous };
                  if (next[group.key]) delete next[group.key];
                  else next[group.key] = true;
                  return next;
                })}>
                  {!searchingSessions && collapsedProjects[group.key] ? <ChevronRight size={13} /> : <ChevronDown size={13} />}
                </button>
                {editingProjectRoot === group.root ? <form className="tauri-project-title-edit" onSubmit={event => { event.preventDefault(); void saveProjectTitle(group.root!); }}>
                  <input autoFocus maxLength={1024} value={projectTitleDraft} aria-label={`重命名项目 ${group.label}`} onChange={event => setProjectTitleDraft(event.target.value)} onKeyDown={event => { if (event.key === "Escape") setEditingProjectRoot(null); }} />
                  <button type="submit" aria-label="保存项目名称" disabled={busy}><Check size={13} /></button>
                  <button type="button" aria-label="取消重命名项目" disabled={busy} onClick={() => setEditingProjectRoot(null)}><X size={13} /></button>
                </form> : <button type="button" className={`tauri-project-group__select${activeProjectKey === group.key ? " is-active" : ""}`} aria-current={activeProjectKey === group.key ? "location" : undefined} title={projectGroupHasUnloadedSessions(group, projectHistoryMayBeIncomplete) ? projectHistoryUnavailableHint : workspaceAvailability[group.root] === false ? `${group.root}\n工作区不可用；已有会话仍可打开` : group.root} aria-label={`切换到项目 ${group.label}${projectGroupHasUnloadedSessions(group, projectHistoryMayBeIncomplete) ? sessionPageSource === "legacy" ? "（持久目录未核验，先重新检查）" : "（历史会话尚未加载，先加载更多）" : workspaceAvailability[group.root] === false ? "（工作区不可用）" : ""}`} disabled={busy || isReadOnlyWorkbenchSource(sessionPageSource) || switchingBlocked || projectGroupHasUnloadedSessions(group, projectHistoryMayBeIncomplete) || (group.sessions.length > 0 && !group.sessions.some(tab => !isMissingWorkbenchSession(tab)))} onClick={() => { const latest = group.sessions.find(tab => !isMissingWorkbenchSession(tab)); if (latest) { if (latest.sessionId !== session?.id) void activateSession(latest.sessionId, latest.workspaceRoot); } else setWorkspaceRoot(group.root || ""); }}><FolderOpen size={14} /><span>{group.label}</span>{projectGroupHasUnloadedSessions(group, projectHistoryMayBeIncomplete) && <small className="tauri-project-group__unloaded">{sessionPageSource === "legacy" ? "目录未核验" : "历史未加载"}</small>}{workspaceAvailability[group.root] === false && <small className="tauri-project-group__unavailable">工作区不可用</small>}<small>{group.sessions.length}</small></button>}
                <button type="button" className="tauri-project-group__rename" aria-label={`重命名项目 ${group.label}`} title="重命名项目" disabled={busy || switchingBlocked || editingProjectRoot !== null} onClick={() => { setEditingProjectRoot(group.root || null); setProjectTitleDraft(group.title ?? ""); }}><Pencil size={12} /></button>
                <button type="button" className="tauri-project-group__new" aria-label={`在 ${group.label} 中新建对话`} title={workspaceAvailability[group.root] === false ? "工作区不可用，无法在此处新建对话" : "在此项目新建对话"} disabled={busy || isReadOnlyWorkbenchSource(sessionPageSource) || switchingBlocked || workspaceAvailability[group.root] === false} onClick={() => void createSession(group.root || "")}><Plus size={14} /></button>
              </div>
              {(searchingSessions || !collapsedProjects[group.key]) && group.sessions.map(tab => <SessionRow key={tab.sessionId} tab={tab} active={session?.id === tab.sessionId} busy={busy || isReadOnlyWorkbenchSource(sessionPageSource)} switchingBlocked={switchingBlocked} onActivate={() => void activateSession(tab.sessionId, tab.workspaceRoot)} onDelete={() => void deleteSession(tab)} />)}
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
          <button type="button" className="tauri-sidebar__diagnostics" onClick={() => { setSettingsTab("general"); setSettingsOpen(true); }}><Settings size={15} />设置</button>
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
            {session && <ExternalOpener key={session.id} tabId={session.id} dismissSignal={settingsOpen ? 1 : 0} bridge={tauriExternalOpenerBridge} />}
            <button type="button" className="tauri-icon-button" aria-label={t("shortcuts.action.commandPalette")} title={`${t("shortcuts.action.commandPalette")} (${formatShortcutCombo(shortcutOverrides.command_palette ?? defaultTauriShortcut("command_palette", shortcutPlatform), shortcutPlatform)})`} onClick={() => setCommandPaletteOpen(true)}><Search size={17} /></button>
            <button type="button" className="tauri-icon-button tauri-sidebar-toggle" aria-label={t(sidebarVisible ? "settings.tauriShortcut.hideSidebar" : "settings.tauriShortcut.showSidebar")} title={`${t(sidebarVisible ? "settings.tauriShortcut.hideSidebar" : "settings.tauriShortcut.showSidebar")} (${formatShortcutCombo(shortcutOverrides.toggle_sidebar ?? defaultTauriShortcut("toggle_sidebar", shortcutPlatform), shortcutPlatform)})`} aria-pressed={!sidebarVisible} onClick={() => setSidebarVisible(previous => { const next = !previous; setTauriSidebarVisible(next); return next; })}>
              {sidebarVisible ? <PanelLeftClose size={17} /> : <PanelLeftOpen size={17} />}
            </button>
            <button type="button" className="tauri-workspace-button" onClick={() => { void chooseDefaultWorkspace().catch(() => {}); }} disabled={busy || session?.state === "running"} title={defaultWorkspace ? `新对话默认工作区：${defaultWorkspace}` : "选择工作区（用于新对话）"}>
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
              : <details className="tauri-progress" key={`progress-${item.entries[0].index}-${progressMode}`} open={progressMode === "deep"}>
                <summary><ChevronRight size={14} aria-hidden="true" /><span>{item.active ? "正在处理" : formatTauriWorkDuration(item.durationMs) ?? "过程记录"}</span><small>{item.entries.length} 条过程更新</small></summary>
                <div className="tauri-progress__messages">{item.entries.map(entry => <HistoryMessageArticle key={entry.index} entry={entry} sessionId={history!.session.id} />)}</div>
              </details>)}
            {/* Optimistic user message: shown immediately after submit, before history loads */}
            {pendingUserMessage && <article className="tauri-message is-user"><div className="tauri-message__content"><UserMessageContent text={pendingUserMessage} /></div></article>}
            {liveText && <details className="tauri-progress tauri-progress--live" key={`live-progress-${progressMode}`} open={progressMode === "deep"}><summary><ChevronRight size={14} aria-hidden="true" /><span>正在处理</span><small>展开查看当前输出</small></summary><div className="tauri-progress__messages"><article className="tauri-message is-assistant tauri-message--live"><div className="tauri-message__avatar" aria-hidden="true"><Sparkles size={16} /></div><div className="tauri-message__content"><div className="tauri-message__role">Reasonix</div><Markdown text={liveText} streaming cacheKey={`${session?.id ?? "live"}:stream`} /></div></article></div></details>}
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

        {pendingPrompt && <PromptCard prompt={pendingPrompt} busy={busy} selections={promptSelections} onApproval={allow => void answerApproval(allow)} onAskSelection={selectPromptOption} onAskSubmit={() => void answerAsk()} onMCPAction={action => void answerMCP(action)} onOpenExternalURL={url => void openTauriExternalURL(url).catch(() => setError("无法在系统浏览器中打开链接"))} />}

        <footer className="tauri-composer-area">
          {error && <p className="tauri-error" role="alert">{error}</p>}
          <div className={`tauri-composer${dragging ? " is-dragging" : ""}`}>
            {selectedTexts.length > 0 && <div className="composer-context" aria-label={t("composer.selectedText")}>
              {selectedTexts.map(reference => <ComposerContextCard key={reference.id} variant="selection"
                name={selectedTextSnippet(reference.text)} meta={t("composer.selectedText")}
                icon={<MessageSquare size={20} />} tooltipLabel={<Markdown text={reference.text} />}
                removeLabel={t("composer.removeSelectedText")}
                onRemove={() => { if (session) setSelectedTextDrafts(previous => ({ ...previous, [session.id]: (previous[session.id] ?? []).filter(item => item.id !== reference.id) })); composerRef.current?.focus(); }}
              />)}
            </div>}
            {attachments.length > 0 && <div className="tauri-composer__attachments" aria-label="已添加文件">{attachments.map((attachment, index) => <div className="tauri-composer__attachment" key={`${attachment.path}-${index}`} title={attachment.path}>
              <span className="tauri-composer__attachment-icon"><FileText size={15} /></span><span className="tauri-composer__attachment-name">{attachment.name}</span><small>{attachment.size < 1024 ? `${attachment.size} B` : `${(attachment.size / 1024).toFixed(1)} KB`}</small>
              <button type="button" onClick={() => setAttachments(previous => previous.filter((_, itemIndex) => itemIndex !== index))} disabled={busy} aria-label={`移除文件 ${attachment.name}`}><X size={13} /></button>
            </div>)}</div>}
            {draftAttachmentPaths.length > 0 && <div className="tauri-composer__attachments" aria-label="待发送文件">{draftAttachmentPaths.map((path, index) => <div className="tauri-composer__attachment" key={`${path}-${index}`} title={path}>
              <span className="tauri-composer__attachment-icon"><FileText size={15} /></span><span className="tauri-composer__attachment-name">{path.split(/[\\/]/).pop() || path}</span><small>待发送</small>
              <button type="button" onClick={() => setDraftAttachmentPaths(previous => previous.filter((_, itemIndex) => itemIndex !== index))} disabled={busy} aria-label={`移除待发送文件 ${path.split(/[\\/]/).pop() || path}`}><X size={13} /></button>
            </div>)}</div>}
            <textarea ref={composerRef} value={prompt} onChange={event => setPrompt(event.target.value)} onKeyDown={event => {
              const native = event.nativeEvent;
              if (isTauriCompositionKey(native)) return;
              if (matchesTauriShortcut(native, "composer_newline", detectShortcutPlatform())) {
                event.preventDefault();
                const textarea = event.currentTarget;
                const start = textarea.selectionStart;
                const end = textarea.selectionEnd;
                const next = prompt.slice(0, start) + "\n" + prompt.slice(end);
                setPrompt(next);
                requestAnimationFrame(() => {
                  textarea.setSelectionRange(start + 1, start + 1);
                });
                return;
              }
              if (matchesTauriShortcut(native, "send_message", detectShortcutPlatform())) { event.preventDefault(); void submit(); }
            }} placeholder={session?.state === "paused" ? "请先完成上方确认…" : session ? `继续聊聊你的问题…（${sendShortcutLabel} 发送）` : `输入问题，开始新对话…（${sendShortcutLabel} 发送）`} disabled={session?.state === "paused"} rows={3} />
            <div className="tauri-composer__bottom"><span>{session?.workspaceRoot ? `当前对话工作区 · ${session.workspaceRoot}` : session ? "当前对话使用默认工作区" : currentWorkspace ? `${currentWorkspace === defaultWorkspace ? "新对话默认工作区" : "新对话工作区"} · ${currentWorkspace}` : "Preview 配置与稳定版相互隔离"}</span>
              <div className="tauri-composer__actions">
                <button className="tauri-attach-button" type="button" onClick={() => void addAttachments()} disabled={busy || isReadOnlyWorkbenchSource(sessionPageSource) || Boolean(session && (!streamReady || session.state !== "idle"))} aria-label="添加文件" title="从本机选择文件并附加到消息"><Paperclip size={16} /><span>添加文件</span></button>
                {session?.state === "running" ? <button className="tauri-send-button is-stop" type="button" onClick={() => void cancel()} disabled={busy} aria-label="停止生成"><Square size={15} fill="currentColor" /></button> : <button className="tauri-send-button" type="button" onClick={() => void submit()} disabled={busy || isReadOnlyWorkbenchSource(sessionPageSource) || Boolean(session && (!streamReady || session.state === "paused")) || (!prompt.trim() && selectedTexts.length === 0 && attachments.length === 0 && draftAttachmentPaths.length === 0)} aria-label="发送消息"><ArrowUp size={18} /></button>}
              </div>
            </div>
          </div>
          <p className="tauri-composer-hint">Reasonix 可能会出错，请核对重要信息。<button type="button" onClick={() => setDiagnosticsOpen(true)}>预览版说明</button></p>
        </footer>
        <TauriStatusBar workspace={currentWorkspace} sessionId={session?.id} model={providerSummary?.defaultModel} sessionState={session?.state} bridgeRunning={status?.running} observedUsage={observedUsage?.sessionId === session?.id ? observedUsage : null} sessionMetrics={sessionMetrics && sessionMetrics.sessionId === session?.id ? sessionMetrics.metrics : null} />
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
            <section className="tauri-diagnostic-card"><h3>预览配置</h3><p>配置与会话保存在独立 Preview 目录，导入与备份入口位于设置的存储页。</p><button type="button" className="tauri-diagnostic-action" onClick={() => { setDiagnosticsOpen(false); setSettingsTab("data"); setSettingsOpen(true); }}>打开存储设置</button></section>
            <section className="tauri-diagnostic-card">
              <div className="tauri-diagnostic-card__heading"><h3>模型提供方</h3><button type="button" onClick={() => void tauriProviderSummary().then(setProviderSummary).catch(cause => setError(tauriMessageFrom(cause)))} disabled={busy}>刷新</button></div>
              {!providerSummary ? <p>正在读取 Preview 配置…</p> : <><p>默认模型只影响新对话；工作区 <code>reasonix.toml</code> 可能覆盖用户默认值。</p>{providerSummary.providers.length === 0 ? <p>当前没有配置提供方。</p> : <ul>{providerSummary.providers.map(provider => <li key={provider.name}><strong>{provider.displayName || provider.name}</strong><span>{provider.kind} · {provider.modelCount} 个模型 · {provider.configured ? "已就绪" : "缺少 API Key"}</span></li>)}</ul>}<small>密钥、环境变量名和服务端点不会传到界面。</small></>}
            </section>
            <section className="tauri-diagnostic-card">
              <h3>MCP 服务器</h3>
              <p>在设置中管理全局和当前项目的 MCP 服务器。</p>
              <button type="button" className="tauri-diagnostic-action" onClick={() => { setDiagnosticsOpen(false); setSettingsTab("mcp"); setSettingsOpen(true); }}>打开 MCP 设置</button>
            </section>
            <details className="tauri-diagnostic-card tauri-runtime-details"><summary>构建与版本详情</summary>{!runtimeInfo ? <p>正在读取构建信息…</p> : <dl><div><dt>稳定版基线</dt><dd>v{runtimeInfo.stableVersion} · {runtimeInfo.stableCommit.slice(0, 12)}</dd></div><div><dt>Preview / Tauri</dt><dd>v{runtimeInfo.previewVersion} · v{runtimeInfo.tauriVersion}</dd></div><div><dt>Preview 源码</dt><dd>{runtimeInfo.previewCommit === "unknown" ? "未知" : runtimeInfo.previewCommit.slice(0, 12)}{runtimeInfo.previewDirty && "（含未提交改动）"}</dd></div><div><dt>宿主构建时间</dt><dd>{runtimeInfo.previewBuild}</dd></div><div><dt>桥接协议</dt><dd>v{runtimeInfo.bridgeProtocolVersion}</dd></div><div><dt>Sidecar</dt><dd>{runtimeInfo.sidecarInstanceId ?? "未运行"}</dd></div>{session && <div><dt>会话 ID</dt><dd>{session.id}</dd></div>}{session && <div><dt>会话路径</dt><dd>{session.path}</dd></div>}{history && <div><dt>历史记录</dt><dd>{history.totalMessages} 条</dd></div>}<div><dt>事件游标</dt><dd>{sequence} · 最近 {events.length} 个事件</dd></div></dl>}</details>
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
            <div><p>WORKSPACE</p><h2>{workspaceView === "files" ? "文件" : workspaceView === "changes" ? "变更" : recoveryCopy.tab}</h2><span title={currentWorkspace}>{workspaceView === "files" ? (workspacePath ? `/${workspacePath}` : "工作区根目录") : workspaceView === "changes" ? (workspaceChanges?.gitBranch ? `Git · ${workspaceChanges.gitBranch}` : "Git 工作区") : currentWorkspace}</span></div>
            <button type="button" className="tauri-icon-button" onClick={() => setWorkspaceOpen(false)} aria-label="关闭"><X size={17} /></button>
          </header>
          <div className="tauri-workspace-drawer__body">
            <div className="tauri-workspace-drawer__toolbar">
              <button type="button" className={`tauri-diagnostic-action${workspaceView === "files" ? " is-active" : ""}`} onClick={() => showWorkspaceView("files")} disabled={workspaceView === "files" && workspaceLoading}><FolderOpen size={13} /> 文件</button>
              <button type="button" className={`tauri-diagnostic-action${workspaceView === "changes" ? " is-active" : ""}`} onClick={() => showWorkspaceView("changes")} disabled={workspaceView === "changes" && workspaceChangesLoading}><GitBranch size={13} /> 变更</button>
              <button type="button" className={`tauri-diagnostic-action${workspaceView === "checkpoints" ? " is-active" : ""}`} onClick={() => showWorkspaceView("checkpoints")} disabled={workspaceView === "checkpoints" && workspaceCheckpointsLoading}><FileText size={13} /> {recoveryCopy.tab}</button>
              {workspaceView === "files" ? <><button type="button" className="tauri-diagnostic-action" onClick={() => void loadWorkspace(workspacePath)} disabled={workspaceLoading}>刷新</button><button type="button" className="tauri-diagnostic-action" onClick={() => void loadWorkspace(workspaceParent(workspacePath))} disabled={workspaceLoading || !workspacePath}>返回上级</button></> : <button type="button" className="tauri-diagnostic-action" onClick={() => void (workspaceView === "changes" ? loadWorkspaceChanges() : loadWorkspaceCheckpoints())} disabled={workspaceView === "changes" ? workspaceChangesLoading : workspaceCheckpointsLoading}>刷新</button>}
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
            </> : workspaceView === "changes" ? <>
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
                {workspaceChanges?.files.find(change => change.path === workspaceChangeDetailPath)?.canSessionRevert && <button type="button" className="tauri-diagnostic-action" onClick={() => void previewWorkspaceFileRevert(workspaceChangeDetailPath)} disabled={workspaceFileRevertBusy || busy || session?.state !== "idle"}>{t("workspace.revertSessionFileShort")}</button>}
                {workspaceChangeDetail.binary ? <p className="tauri-workspace-preview__binary">这是二进制变更，无法显示文本差异。</p> : <pre>{workspaceChangeDetail.diff || "（没有可显示的文本差异）"}</pre>}
              </section>}
              {workspaceFileRevertBusy && <p className="tauri-workspace-drawer__note">{recoveryCopy.fileRevert.working}</p>}
              {workspaceFileRevertMessage && <p role="status" className="tauri-workspace-drawer__note">{workspaceFileRevertMessage}</p>}
              {workspaceFileRevertUndo && workspaceFileRevertUndo.sessionId === session?.id && <section className="tauri-workspace-revert" aria-label={recoveryCopy.fileRevert.undo}>
                <strong>{recoveryCopy.fileRevert.undo}</strong><code>{workspaceFileRevertUndo.path}</code>
                <p>{recoveryCopy.fileRevert.undoDescription}</p>
                <div><button type="button" className="tauri-diagnostic-action" onClick={() => void undoWorkspaceFileRevert()} disabled={workspaceFileRevertBusy || busy || session?.state !== "idle"}>{recoveryCopy.fileRevert.undo}</button></div>
              </section>}
              {workspaceFileRevertPlan && <section className="tauri-workspace-revert" aria-label={recoveryCopy.fileRevert.review}>
                <strong>{recoveryCopy.fileRevert.review}</strong>
                <code>{workspaceFileRevertPlan.path}</code>
                <p>{t("workspace.revertSessionFile")}</p>
                {!workspaceFileRevertPlan.canFiles && <p role="alert">{recoveryCopy.fileRevert.unavailable} {workspaceFileRevertPlan.disabledReason || ""}</p>}
                {workspaceFileRevertPlan.conflicts && workspaceFileRevertPlan.conflicts.length > 0 && <p role="alert">{recoveryCopy.fileRevert.conflictNotice} {workspaceFileRevertPlan.conflicts.join(", ")}</p>}
                <div><button type="button" className="tauri-diagnostic-action" onClick={() => setWorkspaceFileRevertPlan(null)} disabled={workspaceFileRevertBusy}>{recoveryCopy.fileRevert.cancel}</button>{workspaceFileRevertPlan.canFiles && <button type="button" className="tauri-diagnostic-action" onClick={() => void commitWorkspaceFileRevert()} disabled={workspaceFileRevertBusy || busy || session?.state !== "idle"}>{workspaceFileRevertPlan.conflicts?.length ? recoveryCopy.fileRevert.overwrite : recoveryCopy.fileRevert.confirm}</button>}</div>
              </section>}
              <p className="tauri-workspace-drawer__hint">变更来自 Git 或本轮会话检查点；点击文件查看受限差异。</p>
            </> : <>
              {sessionHeads.length > 0 && <section className="tauri-workspace-revert" aria-label={recoveryCopy.heads.title}>
                <strong>{recoveryCopy.heads.title}</strong>
                <ul className="tauri-workspace-revert__files tauri-session-heads">{sessionHeads.map(head => <li key={head.id}>
                  <span>{head.name || (head.kind === "main" ? recoveryCopy.heads.main : head.kind === "rewind" ? recoveryCopy.heads.rewind : head.kind === "concurrent" ? recoveryCopy.heads.concurrent : recoveryCopy.heads.fork)} · {head.preview || head.id} · {head.messageCount}</span>
                  {head.selected ? <small>{recoveryCopy.heads.current}</small> : <button type="button" className="tauri-diagnostic-action" onClick={() => void switchSessionHead(head.id)} disabled={busy || workspaceFileRevertBusy || session?.state !== "idle"}>{recoveryCopy.heads.switch}</button>}
                </li>)}</ul>
              </section>}
              {workspaceCheckpointsLoading ? <div className="tauri-workspace-drawer__loading">{t("workspace.loadingChanges")}</div> : workspaceCheckpoints.length === 0 ? <div className="tauri-workspace-drawer__empty">{recoveryCopy.empty}</div> : <ul className="tauri-workspace-drawer__entries">
                {[...workspaceCheckpoints].reverse().map(checkpoint => <li key={checkpoint.turn}><button type="button" className="tauri-workspace-entry tauri-workspace-change-entry" onClick={() => void previewCodeRewind(checkpoint.turn)} disabled={busy || workspaceFileRevertBusy || session?.state !== "idle"}>
                  <span className="tauri-workspace-entry__icon"><FileText size={15} /></span><span className="tauri-workspace-entry__name">#{checkpoint.turn} · {checkpoint.prompt || "—"}</span><small>{checkpoint.turnFileCount} {t("workspace.filesTab")}</small>
                </button><button type="button" className="tauri-diagnostic-action" onClick={() => void previewConversationRewind(checkpoint.turn)} disabled={busy || workspaceFileRevertBusy || session?.state !== "idle"}>{recoveryCopy.conversation.action}</button><button type="button" className="tauri-diagnostic-action" onClick={() => void previewLegacyFork(checkpoint.turn)} disabled={busy || workspaceFileRevertBusy || session?.state !== "idle"}>{recoveryCopy.legacyFork.action}</button><button type="button" className="tauri-diagnostic-action" onClick={() => void previewCombinedRewind(checkpoint.turn)} disabled={busy || workspaceFileRevertBusy || session?.state !== "idle"}>{recoveryCopy.combined.action}</button></li>)}
              </ul>}
              {workspaceFileRevertBusy && <p className="tauri-workspace-drawer__note">{recoveryCopy.working}</p>}
              {workspaceFileRevertMessage && <p role="status" className="tauri-workspace-drawer__note">{workspaceFileRevertMessage}</p>}
              {codeRewindPlan && <section className="tauri-workspace-revert" aria-label={recoveryCopy.review}>
                <strong>{recoveryCopy.review} · #{codeRewindPlan.turn}</strong>
                <p>{recoveryCopy.description}</p>
                <p>{codeRewindPlan.fileCount} {t("workspace.filesTab")}{codeRewindPlan.filesTruncated ? ` · ${codeRewindPlan.files.length}/${codeRewindPlan.fileCount}` : ""}</p>
                {codeRewindPlan.files.length > 0 && <ul className="tauri-workspace-revert__files">{codeRewindPlan.files.map(path => <li key={path}><code>{path}</code></li>)}</ul>}
                {!codeRewindPlan.canFiles && <p role="alert">{recoveryCopy.unavailable} {codeRewindPlan.disabledReason || codeRewindPlan.conflicts.join(", ")}</p>}
                {codeRewindPlan.requiresCoverageConfirmation && <><p role="alert">{recoveryCopy.coverageWarning} {codeRewindPlan.coverageGaps.join(", ")}</p><label className="tauri-workspace-revert__confirmation"><input type="checkbox" checked={codeRewindCoverageConfirmed} onChange={event => setCodeRewindCoverageConfirmed(event.target.checked)} />{recoveryCopy.coverageConfirm}</label></>}
                <div><button type="button" className="tauri-diagnostic-action" onClick={() => setCodeRewindPlan(null)} disabled={workspaceFileRevertBusy}>{recoveryCopy.fileRevert.cancel}</button>{codeRewindPlan.canFiles && <button type="button" className="tauri-diagnostic-action" onClick={() => void commitCodeRewind()} disabled={workspaceFileRevertBusy || busy || session?.state !== "idle" || (codeRewindPlan.requiresCoverageConfirmation && !codeRewindCoverageConfirmed)}>{recoveryCopy.confirm}</button>}</div>
              </section>}
              {conversationRewindPlan && <section className="tauri-workspace-revert" aria-label={recoveryCopy.conversation.review}>
                <strong>{recoveryCopy.conversation.review} · #{conversationRewindPlan.turn}</strong>
                <p>{recoveryCopy.conversation.description}</p>
                {!conversationRewindPlan.canConversation && <p role="alert">{recoveryCopy.conversation.unavailable} {conversationRewindPlan.disabledReason || ""}</p>}
                <div><button type="button" className="tauri-diagnostic-action" onClick={() => setConversationRewindPlan(null)} disabled={workspaceFileRevertBusy}>{recoveryCopy.fileRevert.cancel}</button>{conversationRewindPlan.canConversation && <button type="button" className="tauri-diagnostic-action" onClick={() => void commitConversationRewind()} disabled={workspaceFileRevertBusy || busy || session?.state !== "idle"}>{recoveryCopy.conversation.confirm}</button>}</div>
              </section>}
              {legacyForkPlan && <section className="tauri-workspace-revert" aria-label={recoveryCopy.legacyFork.review}>
                <strong>{recoveryCopy.legacyFork.review} · #{legacyForkPlan.turn}</strong>
                <p>{recoveryCopy.legacyFork.description}</p>
                {!legacyForkPlan.canConversation && <p role="alert">{recoveryCopy.legacyFork.unavailable} {legacyForkPlan.disabledReason || ""}</p>}
                <div><button type="button" className="tauri-diagnostic-action" onClick={() => setLegacyForkPlan(null)} disabled={workspaceFileRevertBusy}>{recoveryCopy.fileRevert.cancel}</button>{legacyForkPlan.canConversation && <button type="button" className="tauri-diagnostic-action" onClick={() => void commitLegacyFork()} disabled={workspaceFileRevertBusy || busy || session?.state !== "idle"}>{recoveryCopy.legacyFork.confirm}</button>}</div>
              </section>}
              {combinedRewindPlan && <section className="tauri-workspace-revert" aria-label={recoveryCopy.combined.review}>
                <strong>{recoveryCopy.combined.review} · #{combinedRewindPlan.turn}</strong>
                <p>{recoveryCopy.combined.description}</p>
                <p>{combinedRewindPlan.fileCount} {t("workspace.filesTab")}{combinedRewindPlan.filesTruncated ? ` · ${combinedRewindPlan.files.length}/${combinedRewindPlan.fileCount}` : ""}</p>
                {combinedRewindPlan.files.length > 0 && <ul className="tauri-workspace-revert__files">{combinedRewindPlan.files.map(path => <li key={path}><code>{path}</code></li>)}</ul>}
                {(!combinedRewindPlan.canFiles || !combinedRewindPlan.canConversation) && <p role="alert">{recoveryCopy.combined.unavailable} {combinedRewindPlan.disabledReason || combinedRewindPlan.conflicts.join(", ")}</p>}
                {combinedRewindPlan.requiresCoverageConfirmation && <><p role="alert">{recoveryCopy.coverageWarning} {combinedRewindPlan.coverageGaps.join(", ")}</p><label className="tauri-workspace-revert__confirmation"><input type="checkbox" checked={combinedCoverageConfirmed} onChange={event => setCombinedCoverageConfirmed(event.target.checked)} />{recoveryCopy.coverageConfirm}</label></>}
                <div><button type="button" className="tauri-diagnostic-action" onClick={() => setCombinedRewindPlan(null)} disabled={workspaceFileRevertBusy}>{recoveryCopy.fileRevert.cancel}</button>{combinedRewindPlan.canFiles && combinedRewindPlan.canConversation && <button type="button" className="tauri-diagnostic-action" onClick={() => void commitCombinedRewind()} disabled={workspaceFileRevertBusy || busy || session?.state !== "idle" || (combinedRewindPlan.requiresCoverageConfirmation && !combinedCoverageConfirmed)}>{recoveryCopy.combined.confirm}</button>}</div>
              </section>}
              {conversationRewindUndo?.sessionId === session?.id && <section className="tauri-workspace-revert" aria-label={recoveryCopy.conversation.undo}><strong>{recoveryCopy.conversation.undo}</strong><p>{recoveryCopy.conversation.undoDescription}</p><div><button type="button" className="tauri-diagnostic-action" onClick={() => void undoConversationRewind()} disabled={workspaceFileRevertBusy || busy || session?.state !== "idle"}>{recoveryCopy.conversation.undo}</button></div></section>}
              {combinedRewindUndo?.sessionId === session?.id && <section className="tauri-workspace-revert" aria-label={recoveryCopy.combined.undo}><strong>{recoveryCopy.combined.undo}</strong><p>{recoveryCopy.combined.undoDescription}</p><div><button type="button" className="tauri-diagnostic-action" onClick={() => void undoCombinedRewind()} disabled={workspaceFileRevertBusy || busy || session?.state !== "idle"}>{recoveryCopy.combined.undo}</button></div></section>}
              {workspaceFileRevertUndo?.sessionId === session?.id && <section className="tauri-workspace-revert" aria-label={recoveryCopy.fileRevert.undo}><strong>{recoveryCopy.fileRevert.undo}</strong><p>{recoveryCopy.fileRevert.undoDescription}</p><div><button type="button" className="tauri-diagnostic-action" onClick={() => void undoWorkspaceFileRevert()} disabled={workspaceFileRevertBusy || busy || session?.state !== "idle"}>{recoveryCopy.fileRevert.undo}</button></div></section>}
            </>}
          </div>
        </aside>
      </>}

      <CommandPalette open={commandPaletteOpen} onClose={() => setCommandPaletteOpen(false)} items={commandPaletteItems} placeholder={t("palette.placeholder")} emptyText={t("palette.empty")} />
      <Suspense fallback={null}><TranscriptSelectionMenu
        enabled={Boolean(session && !settingsOpen && !diagnosticsOpen && !workspaceOpen && !commandPaletteOpen)}
        resetKey={session?.id ?? "tauri-empty"}
        onAddToChat={addSelectedText}
      /></Suspense>
      <ShortcutsCheatsheet open={shortcutsHelpOpen} platform={shortcutPlatform} onClose={() => setShortcutsHelpOpen(false)} t={t} items={shortcutHelpItems} />
      {settingsOpen && <TauriSettings key={settingsTab} initialTab={settingsTab} workspaceRoot={currentWorkspace || undefined} defaultWorkspace={defaultWorkspace} onChooseDefaultWorkspace={chooseDefaultWorkspace} onClearDefaultWorkspace={clearDefaultWorkspace} profile={profile} importBusy={busy} bridgeStatus={status} catalogAudit={catalogAudit} catalogAuditError={catalogAuditError} sessionPageSource={sessionPageSource} hostError={error} onRestartBridge={restartBridge} onRefreshCatalogAudit={() => refreshCatalogAudit()} onRefreshProfile={refreshProfile} onImportStableProfile={importStableProfile} onImportStableProjectFolders={importStableProjectFolders} onScanUnclaimedSessions={() => void openScanImportReview()} onClose={() => setSettingsOpen(false)} onProviderSummaryChange={setProviderSummary} currentSessionId={session?.id} currentSessionState={session?.state} currentSessionModelRef={session?.modelRef} onCurrentSessionModelChange={changeCurrentSessionModel} currentSessionHasAttachments={attachments.length > 0} onApplyToCurrentSession={restartBridge} onUseSubagentInChat={insertSubagentInvocation} />}
    </main>
  );
}

/** The standalone Tauri entry owns the same localization context as Wails. */
export function TauriSessionApp() {
  return (
    <LocaleProvider>
      <ToastProvider><TauriSessionPreview /></ToastProvider>
    </LocaleProvider>
  );
}
