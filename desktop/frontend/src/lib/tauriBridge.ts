import { invoke, isTauri } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import { formatAttachmentRefForSubmit } from "./attachmentDisplay";
import type {
  BridgeAnswerQuestionRequest,
  BridgeApprovalRequest,
  BridgeAskAnswer,
  BridgeAttachFileRequest,
  BridgeAttachment,
  BridgeEvent,
  BridgeHistoryMessage,
  BridgeMCPInteractionAnswerRequest,
  BridgeProjectFolder,
  BridgeProviderSummaryResponse,
  BridgeSession,
  BridgeWorkspaceListResponse,
  BridgeWorkspaceFileResponse,
  BridgeWorkspaceChangesResponse,
  BridgeWorkspaceChangeDetailResponse,
} from "./bridgeProtocol.generated";

export interface TauriBridgeStatus {
  running: boolean;
  protocolVersion?: number;
  sidecarInstanceId?: string;
}

// The wire mirrors come from the generated schema; the snapshot below is a
// host-owned command payload and stays hand-written.
export type TauriBridgeSession = BridgeSession;
export type TauriBridgeEvent = BridgeEvent;
export type TauriProviderSummary = BridgeProviderSummaryResponse;
export type TauriBridgeAttachment = BridgeAttachment;
export type TauriWorkspaceEntry = BridgeWorkspaceListResponse["entries"][number];
export type TauriWorkspaceList = BridgeWorkspaceListResponse;
export type TauriWorkspaceFilePreview = BridgeWorkspaceFileResponse["preview"];
export type TauriWorkspaceChanges = BridgeWorkspaceChangesResponse["changes"];
export type TauriWorkspaceChangeDetail = BridgeWorkspaceChangeDetailResponse["detail"];

/** Exposes only user-visible answer deltas; reasoning and other event text stay private. */
export function tauriAssistantTextDelta(event: Pick<TauriBridgeEvent, "eventKind" | "payload">): string {
  if (event.eventKind !== "text" || event.payload.kind !== "text") return "";
  return typeof event.payload.text === "string" ? event.payload.text : "";
}

export interface TauriBridgeSnapshot {
  sequence: number;
  session: TauriBridgeSession;
}

export interface TauriBridgeHistory {
  sequence: number;
  session: TauriBridgeSession;
  messages: BridgeHistoryMessage[];
  startIndex: number;
  totalMessages: number;
}

export interface TauriPreviewProfileStatus {
  previewHome: string;
  previewConfigExists: boolean;
  stableConfig?: string;
  stableConfigExists: boolean;
  importAvailable: boolean;
  managedProfile: boolean;
  projectFoldersImportAvailable?: boolean;
  projectFoldersFileExists?: boolean;
}

export interface TauriProfileImportResult {
  importedConfig: string;
  backupConfig: string;
}

export interface TauriProjectFoldersImportResult {
  importedFile: string;
  projectCount: number;
}

export interface TauriPreviewRuntimeInfo {
  stableVersion: string;
  stableCommit: string;
  previewVersion: string;
  tauriVersion: string;
  previewBuild: string;
  bridgeProtocolVersion: number;
  sidecarInstanceId?: string;
}

export interface TauriWorkbenchSession {
  sessionId: string;
  title?: string;
  workspaceRoot?: string;
  state?: string;
  missing?: boolean;
}

export interface TauriWorkbenchSessionPage {
  sessions: TauriWorkbenchSession[];
  nextCursor?: { position: number; id: string; snapshotId: string } | null;
  total: number;
  source: "identity" | "legacy";
}

/** Read-only, count-only comparison of the legacy sidebar catalog and SQLite. */
export interface TauriSessionShadowReport {
  legacyCount: number;
  directoryCount: number;
  matchedCount: number;
  directoryOnlyCount: number;
  missingFromDirectory: number;
  titleMismatches: number;
  workspaceMismatches: number;
  orderMismatches: number;
  missingTranscripts: number;
  physicalStateMismatches: number;
  unclaimedTranscripts: number;
  inventoryErrors: number;
  legacyMatchesDirectory: boolean;
}

export interface TauriSessionPreview {
  sessionId: string;
  title?: string;
  firstUser?: string;
}

// Keep Tauri detection and all Tauri-specific imports here. The established
// bridge.ts remains on its Wails path until each feature family has a complete
// Tauri equivalent; partially swapping its 500+ binding surface would turn a
// usable Wails build into a browser mock at the first unported method.
export function isTauriDesktop(): boolean {
  return isTauri();
}

function requireTauri(): void {
  if (!isTauriDesktop()) throw new Error("Tauri desktop host is unavailable");
}

export async function tauriBridgeStatus(): Promise<TauriBridgeStatus> {
  requireTauri();
  return invoke<TauriBridgeStatus>("bridge_status");
}

export async function restartTauriBridge(): Promise<TauriBridgeStatus> {
  requireTauri();
  return invoke<TauriBridgeStatus>("restart_bridge");
}

export async function tauriPreviewProfileStatus(): Promise<TauriPreviewProfileStatus> {
  requireTauri();
  return invoke<TauriPreviewProfileStatus>("preview_profile_status");
}

export async function tauriPreviewRuntimeInfo(): Promise<TauriPreviewRuntimeInfo> {
  requireTauri();
  return invoke<TauriPreviewRuntimeInfo>("preview_runtime_info");
}

export async function tauriProviderSummary(): Promise<TauriProviderSummary> {
  requireTauri();
  return invoke<TauriProviderSummary>("provider_summary");
}

export async function setTauriDefaultModel(model: string): Promise<TauriProviderSummary> {
  requireTauri();
  return invoke<TauriProviderSummary>("set_default_model", { request: { model } });
}

/** One MCP server as the renderer may see it. Credentials are write-only: the
 *  bridge returns the credential key names a server expects, never a value. */
export interface TauriMCPServer {
  name: string;
  type: string;
  source: string;
  scope: "project" | "global" | "other";
  configPath: string;
  command?: string;
  args?: string[];
  url?: string;
  envKeys?: string[];
  headerKeys?: string[];
  startupTimeoutSeconds?: number;
  callTimeoutSeconds?: number;
  autoStart?: boolean;
  tier?: string;
  managedByPackage?: boolean;
}

export interface TauriMCPServerInput {
  scope: "project" | "global";
  name: string;
  type?: string;
  command?: string;
  args?: string[];
  /** Credentials are write-only. Omit a field to keep the stored value; send an
   *  empty object to clear it. */
  env?: Record<string, string>;
  url?: string;
  headers?: Record<string, string>;
  autoStart?: boolean;
  tier?: string;
}

export interface TauriMCPServerMutation {
  protocolVersion: number;
  status: "saved" | "removed";
  configPath?: string;
  server?: TauriMCPServer;
  servers: TauriMCPServer[];
}

export async function tauriMCPServers(workspaceRoot?: string): Promise<TauriMCPServer[]> {
  requireTauri();
  return invoke<TauriMCPServer[]>("list_mcp_servers", { workspaceRoot });
}

export async function saveTauriMCPServer(
  server: TauriMCPServerInput,
  workspaceRoot?: string,
): Promise<TauriMCPServerMutation> {
  requireTauri();
  return invoke<TauriMCPServerMutation>("save_mcp_server", { request: server, workspaceRoot });
}

export async function deleteTauriMCPServer(
  name: string,
  workspaceRoot?: string,
): Promise<TauriMCPServerMutation> {
  requireTauri();
  return invoke<TauriMCPServerMutation>("delete_mcp_server", { request: { name }, workspaceRoot });
}

export async function importTauriStableProfile(): Promise<TauriProfileImportResult> {
  requireTauri();
  return invoke<TauriProfileImportResult>("import_stable_profile", { confirmed: true });
}

export async function importTauriStableProjectFolders(): Promise<TauriProjectFoldersImportResult> {
  requireTauri();
  return invoke<TauriProjectFoldersImportResult>("import_stable_project_folders", { confirmed: true });
}

export async function tauriWorkbenchSessions(): Promise<TauriWorkbenchSession[]> {
  requireTauri();
  return invoke<TauriWorkbenchSession[]>("workbench_sessions");
}

export async function tauriWorkbenchProjectFolders(): Promise<BridgeProjectFolder[]> {
  requireTauri();
  return invoke<BridgeProjectFolder[]>("workbench_project_folders");
}

export async function rememberTauriWorkbenchProjectFolder(root: string): Promise<BridgeProjectFolder[]> {
  requireTauri();
  return invoke<BridgeProjectFolder[]>("remember_workbench_project_folder", { root });
}

export async function renameTauriWorkbenchProjectFolder(root: string, title: string): Promise<BridgeProjectFolder[]> {
  requireTauri();
  return invoke<BridgeProjectFolder[]>("rename_workbench_project_folder", { root, title });
}

export async function tauriWorkbenchSessionPage(
  cursor?: { position: number; id: string; snapshotId: string },
  limit = 200,
): Promise<TauriWorkbenchSessionPage> {
  requireTauri();
  return invoke<TauriWorkbenchSessionPage>("workbench_session_page", { limit, cursor });
}

export async function tauriWorkspaceRootsAvailability(roots: string[]): Promise<(boolean | null)[]> {
  requireTauri();
  return invoke<(boolean | null)[]>("workspace_roots_availability", { roots });
}

export async function tauriImportLegacySessionCatalog(): Promise<number> {
  requireTauri();
  return invoke<number>("bridge_import_legacy_session_catalog");
}

export async function tauriSessionCatalogShadow(): Promise<TauriSessionShadowReport> {
  requireTauri();
  return invoke<TauriSessionShadowReport>("bridge_session_catalog_shadow");
}

export async function tauriSessionPreviews(sessionIds: string[]): Promise<TauriSessionPreview[]> {
  requireTauri();
  return invoke<TauriSessionPreview[]>("bridge_session_previews", { sessionIds });
}

export async function backfillTauriWorkbenchTitles(titles: { sessionId: string; title: string }[]): Promise<TauriWorkbenchSession[]> {
  requireTauri();
  return invoke<TauriWorkbenchSession[]>("backfill_workbench_titles", { titles });
}

export async function rememberTauriWorkbenchSession(
  sessionId: string,
  workspaceRoot?: string,
  title?: string,
): Promise<TauriWorkbenchSession[]> {
  requireTauri();
  return invoke<TauriWorkbenchSession[]>("remember_workbench_session", {
    request: { sessionId, workspaceRoot, title },
  });
}

export async function forgetTauriWorkbenchSession(sessionId: string): Promise<TauriWorkbenchSession[]> {
  requireTauri();
  return invoke<TauriWorkbenchSession[]>("forget_workbench_session", { sessionId });
}

/**
 * The native picker gives the bridge a path only after an explicit user
 * selection. It does not expose filesystem APIs to the webview.
 */
export async function chooseTauriWorkspaceRoot(): Promise<string | null> {
  requireTauri();
  const selected = await openDialog({
    directory: true,
    multiple: false,
    title: "Choose Reasonix workspace",
  });
  return typeof selected === "string" ? selected : null;
}

export async function chooseTauriAttachmentFiles(): Promise<string[]> {
  requireTauri();
  const selected = await openDialog({
    directory: false,
    multiple: true,
    title: "Add files to this conversation",
  });
  if (typeof selected === "string") return [selected];
  return Array.isArray(selected) ? selected : [];
}

export async function openTauriBridgeSession(sessionId: string, workspaceRoot?: string): Promise<TauriBridgeSession> {
  requireTauri();
  return invoke<TauriBridgeSession>("bridge_open_session", { request: { sessionId, workspaceRoot } });
}

export async function switchTauriBridgeSession(sessionId: string, workspaceRoot?: string): Promise<TauriBridgeSession> {
  requireTauri();
  return invoke<TauriBridgeSession>("bridge_switch_session", { request: { sessionId, workspaceRoot } });
}

export async function renameTauriBridgeSession(sessionId: string, title: string): Promise<TauriBridgeSession> {
  requireTauri();
  return invoke<TauriBridgeSession>("bridge_rename_session", { request: { sessionId, title } });
}

export interface TauriBridgeDeletedSession {
  protocolVersion: number;
  deleted: boolean;
  sessionId: string;
}

/** Removes the conversation and its durable artifacts. The host may only delete
 *  the session the bridge owns, so callers switch to the target first. */
export async function deleteTauriBridgeSession(sessionId: string): Promise<TauriBridgeDeletedSession> {
  requireTauri();
  return invoke<TauriBridgeDeletedSession>("bridge_delete_session", { request: { sessionId } });
}

export async function tauriBridgeSnapshot(sessionId: string): Promise<TauriBridgeSnapshot> {
  requireTauri();
  return invoke<TauriBridgeSnapshot>("bridge_session_snapshot", { request: { sessionId } });
}

export async function tauriBridgeHistory(sessionId: string): Promise<TauriBridgeHistory> {
  requireTauri();
  return invoke<TauriBridgeHistory>("bridge_session_history", { request: { sessionId } });
}

export async function submitTauriBridge(sessionId: string, input: string): Promise<TauriBridgeSession> {
  requireTauri();
  return invoke<TauriBridgeSession>("bridge_submit", { request: { sessionId, input } });
}

export async function attachTauriFile(sessionId: string, path: string): Promise<TauriBridgeAttachment> {
  requireTauri();
  const request: BridgeAttachFileRequest = { sessionId, path };
  return invoke<TauriBridgeAttachment>("bridge_attach_file", { request });
}

export async function tauriWorkspace(sessionId: string, path = ""): Promise<TauriWorkspaceList> {
  requireTauri();
  return invoke<TauriWorkspaceList>("bridge_workspace", { request: { sessionId, path } });
}

export async function tauriWorkspaceFile(sessionId: string, path: string): Promise<TauriWorkspaceFilePreview> {
  requireTauri();
  const response = await invoke<BridgeWorkspaceFileResponse>("bridge_workspace_file", { request: { sessionId, path } });
  return response.preview;
}

export async function tauriWorkspaceChanges(sessionId: string): Promise<TauriWorkspaceChanges> {
  requireTauri();
  const response = await invoke<BridgeWorkspaceChangesResponse>("bridge_workspace_changes", { request: { sessionId } });
  return response.changes;
}

export async function tauriWorkspaceChangeDetail(sessionId: string, path: string): Promise<TauriWorkspaceChangeDetail> {
  requireTauri();
  const response = await invoke<BridgeWorkspaceChangeDetailResponse>("bridge_workspace_change_detail", { request: { sessionId, path } });
  return response.detail;
}

export async function cancelTauriBridge(sessionId: string): Promise<TauriBridgeSession> {
  requireTauri();
  return invoke<TauriBridgeSession>("bridge_cancel", { request: { sessionId } });
}

export async function approveTauriBridge(sessionId: string, id: string, allow: boolean): Promise<TauriBridgeSession> {
  requireTauri();
  const request: BridgeApprovalRequest & { sessionId: string } = { sessionId, id, allow };
  return invoke<TauriBridgeSession>("bridge_approve", { request });
}

export async function answerTauriQuestion(
  sessionId: string,
  id: string,
  answers: readonly BridgeAskAnswer[],
): Promise<TauriBridgeSession> {
  requireTauri();
  const request: BridgeAnswerQuestionRequest & { sessionId: string } = { sessionId, id, answers: [...answers] };
  return invoke<TauriBridgeSession>("bridge_answer_question", { request });
}

export async function answerTauriMCPInteraction(
  sessionId: string,
  id: string,
  action: "accept" | "decline" | "cancel",
  content?: Record<string, unknown>,
): Promise<TauriBridgeSession> {
  requireTauri();
  const request: BridgeMCPInteractionAnswerRequest & { sessionId: string } = { sessionId, id, action, content };
  return invoke<TauriBridgeSession>("bridge_answer_mcp_interaction", { request });
}

export async function replayTauriPendingPrompts(sessionId: string): Promise<TauriBridgeSession> {
  requireTauri();
  return invoke<TauriBridgeSession>("bridge_replay_pending_prompts", { request: { sessionId } });
}

export async function startTauriBridgeEvents(afterSequence: number): Promise<void> {
  requireTauri();
  return invoke<void>("bridge_start_events", { afterSequence });
}

export function onTauriBridgeEvent(callback: (event: TauriBridgeEvent) => void): Promise<UnlistenFn> {
  requireTauri();
  return listen<TauriBridgeEvent>("bridge:event", ({ payload }) => callback(payload));
}

export function onTauriBridgeConnectionError(callback: (message: string) => void): Promise<UnlistenFn> {
  requireTauri();
  return listen<string>("bridge:connection-error", ({ payload }) => callback(payload));
}

export function onTauriBridgeConnectionRestored(callback: () => void): Promise<UnlistenFn> {
  requireTauri();
  return listen("bridge:connection-restored", callback);
}

export function onTauriBridgeResyncRequired(callback: () => void): Promise<UnlistenFn> {
  requireTauri();
  return listen("bridge:resync-required", callback);
}

export async function tauriPlatformInfo(): Promise<string> {
  requireTauri();
  return invoke<string>("platform_info");
}

export interface TauriApprovalPrompt {
  kind: "approval";
  id: string;
  tool: string;
  subject: string;
  reason: string;
}

export interface TauriAskOption {
  label: string;
  description?: string;
}

export interface TauriAskQuestion {
  id: string;
  header?: string;
  prompt: string;
  options: TauriAskOption[];
  multi: boolean;
}

export interface TauriAskPrompt {
  kind: "ask";
  id: string;
  questions: TauriAskQuestion[];
}

export interface TauriMCPPrompt {
  kind: "mcp";
  id: string;
  server: string;
  mode: string;
  message: string;
  url?: string;
}

export type TauriPendingPrompt = TauriApprovalPrompt | TauriAskPrompt | TauriMCPPrompt;

function objectValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

/** Only navigable web URLs may be rendered from an MCP server's event payload. */
export function tauriSafeMCPURL(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  try {
    const target = new URL(value.trim());
    if ((target.protocol !== "https:" && target.protocol !== "http:") || !target.hostname || target.username || target.password) return undefined;
    return target.href;
  } catch {
    return undefined;
  }
}

/** Converts the opaque v1 event payload into the three actionable prompt cards. */
export function tauriPromptFromEvent(event: Pick<TauriBridgeEvent, "eventKind" | "payload">): TauriPendingPrompt | null {
  const payload = event.payload;
  const promptKind = stringValue(payload.promptKind);
  const approval = objectValue(payload.approval);
  if (approval && (event.eventKind === "approval_request" || promptKind === "approval" || promptKind === stringValue(approval.kind))) {
    const id = stringValue(approval.id) || stringValue(payload.promptId) || stringValue(payload.itemId);
    if (!id) return null;
    return { kind: "approval", id, tool: stringValue(approval.tool) || "工具", subject: stringValue(approval.subject), reason: stringValue(approval.reason) };
  }
  const ask = objectValue(payload.ask);
  if (ask && (event.eventKind === "ask_request" || promptKind === "ask")) {
    const questions = Array.isArray(ask.questions) ? ask.questions.flatMap(raw => {
      const question = objectValue(raw);
      if (!question || !stringValue(question.id) || !stringValue(question.prompt)) return [];
      const options = Array.isArray(question.options) ? question.options.flatMap(rawOption => {
        const option = objectValue(rawOption);
        if (!option || !stringValue(option.label)) return [];
        return [{ label: stringValue(option.label), description: stringValue(option.description) || undefined }];
      }) : [];
      return [{ id: stringValue(question.id), header: stringValue(question.header) || undefined, prompt: stringValue(question.prompt), options, multi: question.multi === true }];
    }) : [];
    const id = stringValue(ask.id) || stringValue(payload.promptId) || stringValue(payload.itemId);
    return id && questions.length > 0 ? { kind: "ask", id, questions } : null;
  }
  const mcp = objectValue(payload.mcpInteraction);
  if (mcp && (event.eventKind === "mcp_interaction" || promptKind === "mcp")) {
    const id = stringValue(mcp.id) || stringValue(payload.promptId) || stringValue(payload.itemId);
    return id ? { kind: "mcp", id, server: stringValue(mcp.server) || "MCP 服务", mode: stringValue(mcp.mode), message: stringValue(mcp.message), url: tauriSafeMCPURL(mcp.url) } : null;
  }
  return null;
}

export function tauriPromptAnsweredId(event: Pick<TauriBridgeEvent, "eventKind" | "payload">): string {
  if (event.eventKind !== "prompt_answered") return "";
  return stringValue(event.payload.promptId) || stringValue(event.payload.itemId);
}

// Pure helpers shared by TauriSessionPreview and tests.
// Exported so the contract is exercised by deterministic tests instead of
// relying on component internals.

export function newTauriSessionId(): string {
  return `tauri-${crypto.randomUUID()}`;
}

export function tauriMessageFrom(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function tauriEventSummary(event: { payload: unknown }): string {
  const payload = JSON.stringify(event.payload);
  return payload.length > 500 ? `${payload.slice(0, 497)}...` : payload;
}

export function tauriComposerInput(
  prompt: string,
  attachments: readonly Pick<TauriBridgeAttachment, "path">[],
): string {
  return [prompt.trim(), ...attachments.map(formatAttachmentRefForSubmit)]
    .filter(Boolean)
    .join("\n\n");
}

/** Mirrors the bridge's title rules: non-empty, at most 120 Unicode characters,
 *  no control characters. The Rust host and the Go runtime reject the same input. */
export const TAURI_TITLE_MAX_CHARS = 120;

export function tauriTitleError(title: string): string {
  const trimmed = title.trim();
  if (!trimmed) return "对话名称不能为空";
  if (Array.from(trimmed).length > TAURI_TITLE_MAX_CHARS) {
    return `对话名称不能超过 ${TAURI_TITLE_MAX_CHARS} 个字符`;
  }
  if (/\p{Cc}/u.test(trimmed)) return "对话名称不能包含控制字符";
  return "";
}

/** The stored title when it is safe to display, otherwise the session fallback. */
export function tauriSessionTitle(storedTitle: string | undefined, fallback: string): string {
  const trimmed = (storedTitle ?? "").trim();
  return tauriTitleError(trimmed) === "" ? trimmed : fallback;
}

/** What to show when a turn ends: empty on success, the reason on failure.
 *  A failed turn used to end silently, leaving the composer on its running
 *  state with no explanation; the wire event carries status and err. */
export function tauriTurnFailure(event: Pick<TauriBridgeEvent, "eventKind" | "payload">): string {
  if (event.eventKind !== "turn_done") return "";
  if (event.payload.status !== "failed") return "";
  const reason = typeof event.payload.err === "string" ? event.payload.err.trim() : "";
  return reason || "本轮未完成：Agent 没有返回结果";
}

// ── Keychain API ──────────────────────────────────────────────────────

/** Save a secret to the system keychain. */
export async function keychainSave(key: string, value: string): Promise<void> {
  requireTauri();
  await invoke<void>("keychain_save", { key, value });
}

/** Load a secret from the system keychain. Returns null if not found. */
export async function keychainLoad(key: string): Promise<string | null> {
  requireTauri();
  return invoke<string | null>("keychain_load", { key });
}

/** Delete a secret from the system keychain. Returns true if deleted. */
export async function keychainDelete(key: string): Promise<boolean> {
  requireTauri();
  return invoke<boolean>("keychain_delete", { key });
}
