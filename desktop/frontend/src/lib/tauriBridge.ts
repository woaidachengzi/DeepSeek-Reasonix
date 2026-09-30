import { convertFileSrc, invoke, isTauri } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import { formatAttachmentRefForSubmit } from "./attachmentDisplay";
import { t } from "./i18n";
import type { CapabilityDiagnosticsReport, RuntimeDoctorReport, UsageStatsRange, UsageStatsRequest } from "./types";
import type { FrontendDiagnosticPayload } from "./frontendDiagnostics";
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
  BridgeProviderModelProbeRequest,
  BridgeProviderModelProbeResponse,
  BridgeRemoteBrowseRequest,
  BridgeRemoteBrowseResponse,
  BridgeRemoteFilePreviewRequest,
  BridgeRemoteFilePreviewResponse,
  BridgeRemoteFileSaveRequest,
  BridgeRemoteFileSaveResponse,
  BridgeRemoteDisconnectResponse,
  BridgeSetAgentPreferenceRequest,
  BridgeSession,
  BridgeSessionMetrics,
  BridgeWorkspaceListResponse,
  BridgeWorkspaceFileResponse,
  BridgeWorkspaceChangesResponse,
  BridgeWorkspaceChangeDetailResponse,
  BridgeWorkspaceFileRevertPlanResponse,
  BridgeWorkspaceFileRevertResultResponse,
  BridgeWorkspaceCheckpointsResponse,
  BridgeCodeRewindPlanResponse,
  BridgeConversationRewindPlanResponse,
  BridgeConversationRewindResultResponse,
  BridgeSessionHeadsResponse,
  BridgeSessionHeadSwitchResponse,
  BridgeCombinedRewindPlanResponse,
  BridgeCombinedRewindResultResponse,
  BridgeLegacyConversationForkResultResponse,
} from "./bridgeProtocol.generated";
import type { TerminalThemePreference } from "./terminalTheme";
import type { Theme, ThemeStyle } from "./theme";
import type { ThemePackView } from "./themePack";
import { registerTrustedThemeBackgroundURLs } from "./themePack";
import type { LangPref } from "./i18n";

export interface TauriBridgeStatus {
  running: boolean;
  protocolVersion?: number;
  sidecarInstanceId?: string;
}

// The wire mirrors come from the generated schema; the snapshot below is a
// host-owned command payload and stays hand-written.
export type TauriBridgeSession = BridgeSession;
export type TauriSessionMetrics = BridgeSessionMetrics;
export type TauriBridgeEvent = BridgeEvent;
export type TauriProviderSummary = BridgeProviderSummaryResponse;
export type TauriBridgeAttachment = BridgeAttachment;
export type TauriWorkspaceEntry = BridgeWorkspaceListResponse["entries"][number];
export type TauriWorkspaceList = BridgeWorkspaceListResponse;
export type TauriWorkspaceFilePreview = BridgeWorkspaceFileResponse["preview"];
export type TauriWorkspaceChanges = BridgeWorkspaceChangesResponse["changes"];
export type TauriWorkspaceChangeDetail = BridgeWorkspaceChangeDetailResponse["detail"];
export type TauriWorkspaceFileRevertPlan = BridgeWorkspaceFileRevertPlanResponse["plan"];
export type TauriWorkspaceFileRevertResult = BridgeWorkspaceFileRevertResultResponse["result"];
export type TauriWorkspaceCheckpoint = BridgeWorkspaceCheckpointsResponse["checkpoints"][number];
export type TauriCodeRewindPlan = BridgeCodeRewindPlanResponse["plan"];
export type TauriConversationRewindPlan = BridgeConversationRewindPlanResponse["plan"];
export type TauriConversationRewindResult = BridgeConversationRewindResultResponse["result"];
export type TauriSessionHead = BridgeSessionHeadsResponse["heads"][number];
export type TauriCombinedRewindPlan = BridgeCombinedRewindPlanResponse["plan"];
export type TauriCombinedRewindResult = BridgeCombinedRewindResultResponse["result"];
export type TauriLegacyConversationForkResult = BridgeLegacyConversationForkResultResponse["result"];

/** Exposes only user-visible answer deltas; reasoning and other event text stay private. */
export function tauriAssistantTextDelta(event: Pick<TauriBridgeEvent, "eventKind" | "payload">): string {
  if (event.eventKind !== "text" || event.payload.kind !== "text") return "";
  return typeof event.payload.text === "string" ? event.payload.text : "";
}

export interface TauriBridgeSnapshot {
  sequence: number;
  session: TauriBridgeSession;
  metrics?: BridgeSessionMetrics;
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

export interface TauriStorageSettings {
  protocolVersion: number;
  profilePath: string;
  statePath: string;
  cachePath: string;
  extensionsPath: string;
}

export interface TauriRemoteHost {
  name: string;
  host: string;
  port: number;
  user: string;
  identityFile: string;
  proxyJump: string;
  workspace: string;
  serveInstall: string;
  credentialMode: string;
  useSSHConfig: boolean;
  passwordSet: boolean;
  passphraseSet: boolean;
}

export interface TauriRemoteSettings {
  protocolVersion: number;
  configPath: string;
  sshConfigPath: string;
  hosts: TauriRemoteHost[];
}

export interface TauriRemoteSSHConfigScan {
  protocolVersion: number;
  configPath: string;
  aliases: { alias: string }[];
}

export interface TauriRemoteHostInput extends Omit<TauriRemoteHost, "passwordSet" | "passphraseSet"> {
  passwordAction?: "keep" | "replace" | "clear";
  password?: string;
  passphraseAction?: "keep" | "replace" | "clear";
  passphrase?: string;
}

export interface TauriRemoteSettingsChange {
  action: "upsert" | "remove";
  name?: string;
  host?: TauriRemoteHostInput;
}

export interface TauriRemoteConnectRequest {
  name: string;
  trustFingerprint?: string;
  password?: string;
  passphrase?: string;
}

export interface TauriRemoteConnectResponse {
  protocolVersion: number;
  status: "connected" | "host_key_confirmation" | "failed";
  host?: string;
  address?: string;
  keyType?: string;
  fingerprint?: string;
  message?: string;
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
  previewCommit: string;
  previewDirty: boolean;
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

export interface TauriWorkbenchTitleBackfillResult {
  sessions: TauriWorkbenchSession[];
  resolvedTitles: { sessionId: string; title: string }[];
}

export interface TauriPendingSessionDelete {
  id: string;
  title: string;
}

export interface TauriPendingSessionDeleteCursor {
  id: string;
}

export interface TauriPendingSessionDeletePage {
  sessions: TauriPendingSessionDelete[];
  nextCursor?: TauriPendingSessionDeleteCursor | null;
}

export interface TauriPendingSessionTitleRecovery {
  id: string;
  title: string;
  workspaceRoot?: string;
  state: "reserved" | "ready" | "missing";
}

export interface TauriWorkbenchSessionPage {
  sessions: TauriWorkbenchSession[];
  nextCursor?: { position: number; id: string; snapshotId: string; total: number } | null;
  total: number;
  source: "identity" | "partial_identity" | "identity_unverified" | "legacy" | "cached";
  unverifiedLegacySessions?: TauriWorkbenchSession[];
  shadowDirectoryCount?: number;
  unclaimedTranscriptCount?: number;
  titleMismatchCount?: number;
  missingTranscriptCount?: number;
  shadowReport?: TauriSessionShadowReport;
  catalogWarning?: string;
}

export interface TauriScanImportCandidate {
  id: string;
  file: string;
  transcriptSha256: string;
}

export interface TauriScanImportSelection {
  id: string;
  title: string;
  workspaceRoot: string;
  transcriptSha256: string;
}

export interface TauriScanImportCandidateList {
  candidates: TauriScanImportCandidate[];
  blockedCount: number;
}

export interface TauriWorkbenchProjectFolders {
  folders: BridgeProjectFolder[];
  warning?: string;
}

/** Read-only, count-only comparison of the legacy sidebar catalog and SQLite. */
export interface TauriSessionShadowReport {
  legacyCount: number;
  directoryCount: number;
  matchedCount: number;
  directoryOnlyCount: number;
  missingFromDirectory: number;
  retiredLegacyCount: number;
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

export async function tauriStorageSettings(): Promise<TauriStorageSettings> {
  requireTauri();
  return invoke<TauriStorageSettings>("storage_settings");
}

export async function tauriCapabilityDiagnostics(workspaceRoot = "", includeSessionRuntime = false): Promise<CapabilityDiagnosticsReport> {
  requireTauri();
  return invoke<CapabilityDiagnosticsReport>("capability_diagnostics", { workspaceRoot, includeSessionRuntime });
}

export async function tauriRuntimeDoctor(): Promise<RuntimeDoctorReport> {
  requireTauri();
  return invoke<RuntimeDoctorReport>("runtime_doctor");
}

export async function exportTauriFrontendDiagnostics(payload: FrontendDiagnosticPayload): Promise<boolean> {
  requireTauri();
  return invoke<boolean>("export_frontend_diagnostics", { payload });
}

export async function tauriRemoteSettings(): Promise<TauriRemoteSettings> {
  requireTauri();
  return invoke<TauriRemoteSettings>("remote_settings");
}

export interface TauriBotAdapterHealth {
  id: string;
  platform: string;
  domain?: string;
  name?: string;
  status: string;
  started_at?: string;
  last_message_at?: string;
  last_send_at?: string;
  last_error_at?: string;
  last_error?: string;
  messages: number;
  sends: number;
  send_errors: number;
  closed: boolean;
}

export interface TauriBotRuntimeStatus {
  protocolVersion: number;
  running: boolean;
  status: "stopped" | "blocked" | "starting" | "running" | "degraded" | "error" | string;
  message: string;
  connections: number;
  startedAt?: string;
  adapterHealth?: TauriBotAdapterHealth[];
  desktopBridgeAvailable: boolean;
}

export interface TauriBotChannelSettings {
  id: string;
  platform: string;
  domain?: string;
  label: string;
  enabled: boolean;
  status: string;
  credentialsSet: boolean;
  credentialMissing: boolean;
  credentialIdentity?: string;
  model?: string;
  toolApprovalMode?: string;
  workspaceRoot?: string;
  runtimeSettings: boolean;
  access?: TauriBotConnectionAccess;
}

export interface TauriBotRoute {
  connectionId: string;
  platform: "" | "qq" | "feishu" | "weixin" | "dingtalk";
  chatType: "" | "dm" | "group" | "guild" | "direct" | "thread";
  chatId: string;
  userId: string;
  threadId: string;
  model: string;
  toolApprovalMode: "" | "ask" | "auto" | "yolo";
  workspaceRoot: string;
}

export interface TauriBotConnectionAccess {
  enabled: boolean;
  allowAll: boolean;
  pairingEnabled: boolean;
  users: string[];
  groups: string[];
  approvers: string[];
  admins: string[];
}

export interface TauriBotSettings {
  protocolVersion: number;
  configPath: string;
  enabled: boolean;
  maxSteps: number;
  debounceMs: number;
  queueMode: "steer" | "followup" | "collect" | "interrupt";
  queueCap: number;
  queueDrop: "summarize" | "old" | "new";
  ignoreSelfMessages: boolean;
  selfUserIds: Record<"qq" | "feishu" | "weixin" | "dingtalk", string[]>;
  accessControlConfigured: boolean;
  pairingEnabled: boolean;
  pairingRequestTtlMinutes: number;
  pairingMaxPendingPerPlatform: number;
  allowlistEnabled: boolean;
  allowAll: boolean;
  allowlist: Record<"qq" | "feishu" | "weixin" | "dingtalk", {
    users: string[];
    groups: string[];
    approvers: string[];
    admins: string[];
  }>;
  channels: TauriBotChannelSettings[];
  routes: TauriBotRoute[];
}

export type TauriBotSettingsChange =
  | { action: "set_enabled"; enabled: boolean }
  | { action: "set_gateway_runtime"; maxSteps?: number; debounceMs?: number; queueMode?: TauriBotSettings["queueMode"]; queueCap?: number; queueDrop?: TauriBotSettings["queueDrop"]; ignoreSelfMessages?: boolean; pairingRequestTtlMinutes?: number; pairingMaxPendingPerPlatform?: number }
  | { action: "set_self_user_ids"; platform: "qq" | "feishu" | "weixin" | "dingtalk"; values: string[] }
  | { action: "set_routes"; routes: TauriBotRoute[] }
  | { action: "set_channel_enabled"; channelId: string; enabled: boolean }
  | { action: "set_pairing"; enabled: boolean }
  | { action: "set_allow_all"; enabled: boolean }
  | { action: "set_allowlist"; platform: "qq" | "feishu" | "weixin" | "dingtalk"; list: "users" | "groups" | "approvers" | "admins"; values: string[] }
  | { action: "set_channel_access_mode"; channelId: string; mode: "trusted" | "everyone" }
  | { action: "set_channel_pairing"; channelId: string; enabled: boolean }
  | { action: "set_channel_allowlist"; channelId: string; list: "users" | "groups" | "approvers" | "admins"; values: string[] }
  | { action: "set_channel_runtime"; channelId: string; model?: string; toolApprovalMode?: "" | "ask" | "auto" | "yolo"; workspaceRoot?: string }
  | { action: "set_credentials"; channelId: string; identity: string; secret: string };

export async function tauriBotSettings(): Promise<TauriBotSettings> {
  requireTauri();
  return invoke<TauriBotSettings>("bot_settings");
}

export async function changeTauriBotSettings(change: TauriBotSettingsChange): Promise<TauriBotSettings> {
  requireTauri();
  return invoke<TauriBotSettings>("change_bot_settings", { change });
}

export async function tauriBotRuntimeStatus(): Promise<TauriBotRuntimeStatus> {
  requireTauri();
  return invoke<TauriBotRuntimeStatus>("bot_runtime_status");
}

export async function scanTauriRemoteSSHConfig(): Promise<TauriRemoteSSHConfigScan> {
  requireTauri();
  return invoke<TauriRemoteSSHConfigScan>("scan_remote_ssh_config");
}

export async function changeTauriRemoteSettings(change: TauriRemoteSettingsChange): Promise<TauriRemoteSettings> {
  requireTauri();
  return invoke<TauriRemoteSettings>("change_remote_settings", { change });
}

export async function connectTauriRemoteHost(request: TauriRemoteConnectRequest): Promise<TauriRemoteConnectResponse> {
  requireTauri();
  return invoke<TauriRemoteConnectResponse>("connect_remote_host", { request });
}

export async function disconnectTauriRemoteHost(name: string): Promise<BridgeRemoteDisconnectResponse> {
  requireTauri();
  return invoke<BridgeRemoteDisconnectResponse>("disconnect_remote_host", { request: { name } });
}

export async function browseTauriRemoteHost(name: string, path?: string): Promise<BridgeRemoteBrowseResponse> {
  requireTauri();
  const request: BridgeRemoteBrowseRequest = { name, ...(path ? { path } : {}) };
  return invoke<BridgeRemoteBrowseResponse>("browse_remote_host", { request });
}

export async function previewTauriRemoteFile(name: string, path: string): Promise<BridgeRemoteFilePreviewResponse> {
  requireTauri();
  const request: BridgeRemoteFilePreviewRequest = { name, path };
  return invoke<BridgeRemoteFilePreviewResponse>("preview_remote_file", { request });
}

export async function saveTauriRemoteFile(request: BridgeRemoteFileSaveRequest): Promise<BridgeRemoteFileSaveResponse> {
  requireTauri();
  return invoke<BridgeRemoteFileSaveResponse>("save_remote_file", { request });
}

export async function tauriPreviewRuntimeInfo(): Promise<TauriPreviewRuntimeInfo> {
  requireTauri();
  return invoke<TauriPreviewRuntimeInfo>("preview_runtime_info");
}

export async function tauriProviderSummary(workspaceRoot?: string, scope: "global" | "project" = "global"): Promise<TauriProviderSummary> {
  requireTauri();
  return invoke<TauriProviderSummary>("provider_summary", { scope, ...(workspaceRoot ? { workspaceRoot } : {}) });
}

export interface TauriProviderConfig {
  name: string;
  displayName: string;
  kind: string;
  models: string[];
  default: string;
  modelsUrlSet: boolean;
  noProxy: boolean;
  contextWindow: number;
  responsesMode: string;
  balanceUrlSet: boolean;
  removable: boolean;
  revision: string;
}

export type TauriProviderConfigInput = Omit<TauriProviderConfig, "removable" | "revision"> & {
  baseUrl: string;
  modelsUrl: string;
  clearModelsUrl: boolean;
  balanceUrl: string;
  clearBalanceUrl: boolean;
  useApiKey: boolean;
};

export interface TauriProviderConfigList {
  protocolVersion: number;
  providers: TauriProviderConfig[];
  presets: TauriProviderPreset[];
}

export interface TauriSessionBalance {
  available: boolean;
  display: string;
}

export interface TauriSessionBalanceResponse {
  protocolVersion: number;
  balance: TauriSessionBalance | null;
}

export async function tauriSessionBalance(sessionId: string): Promise<TauriSessionBalanceResponse> {
  requireTauri();
  return invoke<TauriSessionBalanceResponse>("bridge_session_balance", { request: { sessionId } });
}

export interface TauriDiscoveredProviderModels {
  protocolVersion: number;
  models: string[];
}

export interface TauriProviderPresetRoute {
  name: string;
  kind: string;
  baseUrl: string;
  models: string[];
  default: string;
}

export interface TauriProviderPreset {
  id: string;
  label: string;
  description: string;
  group: string;
  recommended: boolean;
  status: "available" | "partial" | "installed" | "installed_modified" | "name_conflict";
  routes: TauriProviderPresetRoute[];
  revision: string;
}

export async function tauriProviderConfigs(): Promise<TauriProviderConfigList> {
  requireTauri();
  return invoke<TauriProviderConfigList>("provider_configs");
}

export async function discoverTauriProviderModels(provider: Pick<TauriProviderConfig, "name" | "revision">): Promise<TauriDiscoveredProviderModels> {
  requireTauri();
  return invoke<TauriDiscoveredProviderModels>("discover_provider_models", { input: { name: provider.name, revision: provider.revision } });
}

export async function testTauriProviderModel(request: BridgeProviderModelProbeRequest): Promise<BridgeProviderModelProbeResponse> {
  requireTauri();
  return invoke<BridgeProviderModelProbeResponse>("test_provider_model", { request });
}

export async function saveTauriProviderConfig(input: TauriProviderConfigInput): Promise<TauriProviderConfigList> {
  requireTauri();
  return invoke<TauriProviderConfigList>("save_provider_config", { input });
}

async function changeTauriProviderPreset(preset: TauriProviderPreset, presetAction: "add" | "reset"): Promise<TauriProviderConfigList> {
  requireTauri();
  return invoke<TauriProviderConfigList>("save_provider_config", { input: {
    presetId: preset.id, presetAction, revision: preset.revision, name: "", displayName: "", kind: "", baseUrl: "", models: [], default: "", useApiKey: false,
  } });
}

export const installTauriProviderPreset = (preset: TauriProviderPreset) => changeTauriProviderPreset(preset, "add");
export const resetTauriProviderPreset = (preset: TauriProviderPreset) => changeTauriProviderPreset(preset, "reset");

export async function deleteTauriProviderConfig(provider: TauriProviderConfig): Promise<TauriProviderConfigList> {
  requireTauri();
  const { name, displayName, kind, models, default: defaultModel, revision } = provider;
  return invoke<TauriProviderConfigList>("delete_provider_config", { input: { name, displayName, kind, models, default: defaultModel, revision } });
}

export async function tauriUsageStats(request: UsageStatsRequest): Promise<UsageStatsRange> {
  requireTauri();
  return invoke<UsageStatsRange>("usage_stats", { request });
}

export type TauriPermissionMode = "ask" | "allow" | "deny";
export type TauriPermissionList = "allow" | "ask" | "deny";
export interface TauriPermissionSettings {
  protocolVersion: number;
  scope: "global" | "project";
  mode: TauriPermissionMode;
  allow: string[];
  ask: string[];
  deny: string[];
  projectOverrides: { mode: boolean; allow: boolean; ask: boolean; deny: boolean };
}
export type TauriPermissionScope = "global" | "project";
export type TauriPermissionChange = ({ action: "mode"; mode: TauriPermissionMode } | { action: "add" | "remove"; list: TauriPermissionList; rule: string }) & { scope?: TauriPermissionScope; workspaceRoot?: string };

export async function tauriPermissionSettings(workspaceRoot?: string): Promise<TauriPermissionSettings> {
  requireTauri();
  return invoke<TauriPermissionSettings>("permission_settings", { workspaceRoot: workspaceRoot ?? "" });
}

export async function changeTauriPermissionSettings(change: TauriPermissionChange): Promise<TauriPermissionSettings> {
  requireTauri();
  return invoke<TauriPermissionSettings>("change_permission_settings", { change });
}

export interface TauriSecretsSettings {
  protocolVersion: number;
  filterSubprocessEnv: boolean;
  protectSensitiveFiles: boolean;
}

export interface TauriSecretsSettingsChange {
  filterSubprocessEnv?: boolean;
  protectSensitiveFiles?: boolean;
}

export async function tauriSecretsSettings(): Promise<TauriSecretsSettings> {
  requireTauri();
  return invoke<TauriSecretsSettings>("secrets_settings");
}

export async function changeTauriSecretsSettings(change: TauriSecretsSettingsChange): Promise<TauriSecretsSettings> {
  requireTauri();
  return invoke<TauriSecretsSettings>("change_secrets_settings", { change });
}

export interface TauriSandboxSettings {
  protocolVersion: number;
  bash: "enforce" | "off";
  network: boolean;
  workspaceRoot: string;
  allowWrite: string[];
  platform: string;
  shell: string;
  resolvedShell: string;
  effectiveShell?: string;
  shellReloadRequired?: boolean;
  shellCapabilities?: Array<{ id: string; variant?: string; available: boolean; path?: string; source?: string; reason?: string }>;
  gitCapability?: { id: string; variant?: string; available: boolean; path?: string; source?: string; reason?: string } | null;
  effectiveWriteRoots: string[];
  effectiveRootsError: string;
}

export type TauriSandboxChange = Pick<TauriSandboxSettings, "bash" | "network" | "workspaceRoot" | "allowWrite"> & { shell?: string };

export async function tauriSandboxSettings(workspaceRoot?: string, sessionId?: string): Promise<TauriSandboxSettings> {
  requireTauri();
  return invoke<TauriSandboxSettings>("sandbox_settings", workspaceRoot || sessionId ? { workspaceRoot, sessionId } : undefined);
}

export async function changeTauriSandboxSettings(change: TauriSandboxChange): Promise<TauriSandboxSettings> {
  requireTauri();
  return invoke<TauriSandboxSettings>("change_sandbox_settings", { change });
}

export interface TauriNetworkSettings {
  protocolVersion: number;
  proxyMode: "auto" | "env" | "custom" | "off";
  noProxy: string;
  proxyType: string;
  proxyServer: string;
  proxyPort: number;
  proxyUsername: string;
  proxyUrlSet: boolean;
  proxyPasswordSet: boolean;
}

export type TauriNetworkSecretAction = "keep" | "replace" | "clear";
export interface TauriNetworkChange extends Omit<TauriNetworkSettings, "protocolVersion" | "proxyUrlSet" | "proxyPasswordSet"> {
  proxyUrlAction: TauriNetworkSecretAction;
  proxyUrl: string;
  proxyPasswordAction: TauriNetworkSecretAction;
  proxyPassword: string;
}

export async function tauriNetworkSettings(): Promise<TauriNetworkSettings> {
  requireTauri();
  return invoke<TauriNetworkSettings>("network_settings");
}

export async function changeTauriNetworkSettings(change: TauriNetworkChange): Promise<TauriNetworkSettings> {
  requireTauri();
  return invoke<TauriNetworkSettings>("change_network_settings", { change });
}

export interface TauriSkillItem {
  name: string;
  description: string;
  invocation: string;
  scope: string;
  sourcePath: string;
  runAs: string;
  enabled: boolean;
  globalEnabled?: boolean;
  requires: string[];
  archiveRevision?: string;
}

export interface TauriArchivedSkill {
  name: string;
  scope: "global" | "project";
  archiveId: string;
  path: string;
  revision: string;
}

export interface TauriSkillSource {
  path: string;
  scope: string;
  status: string;
  enabled: boolean;
  globalEnabled?: boolean;
  configured: boolean;
  configuredGlobal?: boolean;
  configuredProject?: boolean;
  skillCount: number;
}

export interface TauriSkillsSettings {
  protocolVersion: number;
  allowImplicitInvocation: boolean;
  globalAllowImplicitInvocation?: boolean;
  projectOverrides?: { implicit: boolean; skills: boolean; sources: boolean };
  skills: TauriSkillItem[];
  sources: TauriSkillSource[];
  archivedSkills: TauriArchivedSkill[];
}

export interface TauriSkillArchiveRequest {
  name: string;
  scope: "global" | "project";
  workspaceRoot: string;
  archiveId: string;
  revision: string;
}

export interface TauriSkillArchiveResult {
  protocolVersion: number;
  backupPath: string;
  settings: TauriSkillsSettings;
}

export async function archiveTauriSkill(request: TauriSkillArchiveRequest): Promise<TauriSkillArchiveResult> {
  requireTauri();
  return invoke<TauriSkillArchiveResult>("archive_skill", { request });
}

export async function restoreTauriSkill(request: TauriSkillArchiveRequest): Promise<TauriSkillArchiveResult> {
  requireTauri();
  return invoke<TauriSkillArchiveResult>("restore_skill", { request });
}

export interface TauriSkillsChange {
  workspaceRoot: string;
  scope?: "global" | "project";
  action: "implicit" | "skill" | "source" | "add_source" | "remove_source";
  enabled: boolean;
  name: string;
  path: string;
}

export async function tauriSkillsSettings(workspaceRoot = ""): Promise<TauriSkillsSettings> {
  requireTauri();
  return invoke<TauriSkillsSettings>("skills_settings", { workspaceRoot });
}

export async function changeTauriSkillsSettings(change: TauriSkillsChange): Promise<TauriSkillsSettings> {
  requireTauri();
  return invoke<TauriSkillsSettings>("change_skills_settings", { change });
}

export interface TauriSkillInstallRequest {
  source: string;
  scope: "global" | "project";
  workspaceRoot: string;
  planId: string;
  acceptRisk: boolean;
}

export interface TauriSkillInstallPlan {
  protocolVersion: number;
  planId: string;
  actions: { name: string; target: string; riskLevel: "low" | "medium" | "high" }[];
  warningCount: number;
  warnings: string[];
}

export interface TauriSkillInstallResult {
  protocolVersion: number;
  status: "done" | "partial" | "failed";
  failedNames: string[];
  settings: TauriSkillsSettings;
}

export async function planTauriSkillInstall(request: TauriSkillInstallRequest): Promise<TauriSkillInstallPlan> {
  requireTauri();
  return invoke<TauriSkillInstallPlan>("plan_skill_install", { request });
}

export async function installTauriSkill(request: TauriSkillInstallRequest): Promise<TauriSkillInstallResult> {
  requireTauri();
  return invoke<TauriSkillInstallResult>("install_skill", { request });
}

export interface TauriPluginItem {
  name: string;
  description: string;
  version: string;
  source: "local" | "remote" | "package" | "unknown";
  updateSource?: string;
  root: string;
  manifestKind: string;
  enabled: boolean;
  linked: boolean;
  status: "ready" | "invalid";
  issue: string;
  warningCount: number;
  skills: number;
  agents: number;
  commands: number;
  hooks: number;
  hookDetails?: Array<{ event: string; match?: string; contextFile?: string; description?: string }>;
  mcpServers: number;
  runtime: boolean;
  revision: string;
}

export interface TauriPluginSettings {
  protocolVersion: number;
  plugins: TauriPluginItem[];
}

export interface TauriPluginDoctorView {
  protocolVersion: number;
  name: string;
  compatibility: string;
  mappedCapabilities: string[];
  skippedCapabilities: { capability: string; path: string; reason: string }[];
  warnings: string[];
  error: string;
}

export async function tauriPluginSettings(): Promise<TauriPluginSettings> {
  requireTauri();
  return invoke<TauriPluginSettings>("plugin_settings");
}

export async function changeTauriPluginSettings(change: { name: string; revision: string; enabled: boolean }): Promise<TauriPluginSettings> {
  requireTauri();
  return invoke<TauriPluginSettings>("change_plugin_settings", { change });
}

export async function tauriPluginDoctor(name: string): Promise<TauriPluginDoctorView> {
  requireTauri();
  return invoke<TauriPluginDoctorView>("plugin_doctor", { name });
}

export interface TauriPluginInstallPlanAction {
  name: string;
  version: string;
  manifestKind: string;
  riskLevel: "low" | "medium" | "high";
  skills: number;
  agents: number;
  commands: number;
  hooks: number;
  mcpServers: number;
  prompts: number;
  themes: number;
  runtime: boolean;
  runtimeCommand: string;
  intercepts: string[];
  replaces: string[];
}

export interface TauriPluginInstallPlan {
  protocolVersion: number;
  planId: string;
  mode: "copy" | "link";
  actions: TauriPluginInstallPlanAction[];
  warningCount: number;
  warnings: string[];
}

export interface TauriPluginOperationResult {
  protocolVersion: number;
  status: "done" | "partial" | "failed";
  failedNames: string[];
  settings: TauriPluginSettings;
}

export async function planTauriPluginInstall(source: string, mode: "copy" | "link" = "copy", update?: { name: string; revision: string }): Promise<TauriPluginInstallPlan> {
  requireTauri();
  return invoke<TauriPluginInstallPlan>("plan_plugin_install", { source, mode, replace: Boolean(update), expectedName: update?.name ?? "", expectedRevision: update?.revision ?? "" });
}

export async function installTauriPlugin(request: { source: string; mode: "copy" | "link"; planId: string; acceptRisk: boolean; replace?: boolean; expectedName?: string; expectedRevision?: string }): Promise<TauriPluginOperationResult> {
  requireTauri();
  return invoke<TauriPluginOperationResult>("install_plugin", { request });
}

export async function removeTauriPlugin(request: { name: string; revision: string }): Promise<TauriPluginOperationResult> {
  requireTauri();
  return invoke<TauriPluginOperationResult>("remove_plugin", { request });
}

export async function chooseTauriPluginDirectory(): Promise<string | null> {
  requireTauri();
  const selected = await openDialog({ directory: true, multiple: false, title: "选择插件目录" });
  return typeof selected === "string" ? selected : null;
}

export interface TauriSubagentProfile {
  name: string;
  description: string;
  scope: string;
  invocation: string;
  configuredModel: string;
  configuredEffort: string;
  invocationMode?: string;
  editable?: boolean;
  editReason?: string;
  revision?: string;
  body?: string;
  color?: string;
  model?: string;
  effort?: string;
  allowedTools?: string[];
  readOnly?: boolean;
}

export interface TauriSubagentProfileInput {
  name: string;
  description: string;
  systemPrompt: string;
  color: string;
  model: string;
  effort: string;
  allowedTools: string[];
  readOnly: boolean;
}

export interface TauriSubagentSettings {
  protocolVersion: number;
  defaultModel: string;
  subagentModel: string;
  subagentEffort: string;
  maxDepth: number;
  maxConcurrency: number;
  maxParallelWriters: number;
  modelRefs: string[];
  modelEfforts: Record<string, string[]>;
  profiles: TauriSubagentProfile[];
}

export interface TauriSubagentChange {
  workspaceRoot: string;
  action: "model" | "effort" | "depth" | "concurrency" | "writers" | "profile_model" | "profile_effort" | "create_profile" | "update_profile" | "delete_profile";
  name: string;
  value: string;
  number: number;
  scope?: "global" | "project";
  revision?: string;
  profile?: TauriSubagentProfileInput;
}

export async function tauriSubagentSettings(workspaceRoot = ""): Promise<TauriSubagentSettings> {
  requireTauri();
  return invoke<TauriSubagentSettings>("subagent_settings", { workspaceRoot });
}

export async function changeTauriSubagentSettings(change: TauriSubagentChange): Promise<TauriSubagentSettings> {
  requireTauri();
  return invoke<TauriSubagentSettings>("change_subagent_settings", { change });
}

export async function tryTauriSubagentProfile(workspaceRoot: string, input: TauriSubagentProfileInput, task: string): Promise<string> {
  requireTauri();
  return invoke<string>("try_subagent_profile", { workspaceRoot, input, task });
}

export interface TauriSubagentTryStatus {
  protocolVersion: number;
  running: boolean;
  output: string;
}

export async function tauriSubagentProfileTryStatus(): Promise<TauriSubagentTryStatus> {
  requireTauri();
  return invoke<TauriSubagentTryStatus>("subagent_profile_try_status");
}

export async function cancelTauriSubagentProfileTry(): Promise<void> {
  requireTauri();
  await invoke("cancel_subagent_profile_try");
}

export interface TauriHooksSettings {
  protocolVersion: number;
  scope: "global" | "project";
  path: string;
  projectRoot: string;
  revision: string;
  hooks: Record<string, unknown>;
  events: string[];
}

export interface TauriHooksChange {
  scope: "global" | "project";
  workspaceRoot: string;
  revision: string;
  hooks: Record<string, unknown>;
}

export async function tauriHooksSettings(scope: "global" | "project", workspaceRoot = ""): Promise<TauriHooksSettings> {
  requireTauri();
  return invoke<TauriHooksSettings>("hooks_settings", { scope, workspaceRoot });
}

export async function changeTauriHooksSettings(change: TauriHooksChange): Promise<TauriHooksSettings> {
  requireTauri();
  return invoke<TauriHooksSettings>("change_hooks_settings", { change });
}

export interface TauriMemoryDoc { path: string; scope: string; body: string; revision: string }
export interface TauriMemoryFact { id: string; revision: number; createdAt: string; updatedAt: string; name: string; title: string; description: string; type: string; scope: string; body: string; freshness: string }
export interface TauriMemoryArchive extends TauriMemoryFact { path: string; archivedAt: string }
export interface TauriMemoryRecallHit { id: string; revision: number; name: string; title?: string; type: string; scope: string; score: number; freshness: string; reason: string; snippet: string }
export interface TauriMemoryRecall { query: string; hits: TauriMemoryRecallHit[]; omitted: number; charBudget: number; usedChars: number; suppressed?: string }
export interface TauriMemorySettings {
  protocolVersion: number;
  workspaceRoot: string;
  storeDir: string;
  globalStoreDir: string;
  docs: TauriMemoryDoc[];
  facts: TauriMemoryFact[];
  archives: TauriMemoryArchive[];
  revisions: TauriMemoryFact[];
  diagnostics: string[];
  lastRecall: TauriMemoryRecall;
}
export interface TauriMemoryChange {
  workspaceRoot: string;
  action: "save_doc" | "quick_add" | "archive" | "restore" | "load_revisions" | "restore_revision" | "save_fact";
  path: string;
  revision: string;
  body: string;
  scope: string;
  factId: string;
  factRevision: number;
  historyRevision: number;
  name?: string;
  title?: string;
  description?: string;
  type?: string;
}
export interface TauriMemorySuggestion { id: string; name: string; title: string; description: string; type: string; scope: string; body: string; reason: string; evidence: string[] }
export interface TauriSkillSuggestion { id: string; name: string; description: string; scope: string; body: string; reason: string; evidence: string[] }
export interface TauriMemorySuggestions { memories: TauriMemorySuggestion[]; skills: TauriSkillSuggestion[]; generatedAt: string; available: boolean; source: string }
export interface TauriMemorySuggestionAcceptance { path: string; suggestions: TauriMemorySuggestions }
export async function tauriMemorySettings(workspaceRoot: string): Promise<TauriMemorySettings> {
  requireTauri();
  return invoke<TauriMemorySettings>("memory_settings", { workspaceRoot });
}
export async function changeTauriMemorySettings(change: TauriMemoryChange): Promise<TauriMemorySettings> {
  requireTauri();
  return invoke<TauriMemorySettings>("change_memory_settings", { change });
}
export async function tauriMemorySuggestions(workspaceRoot: string): Promise<TauriMemorySuggestions> {
  requireTauri();
  return invoke<TauriMemorySuggestions>("memory_suggestions", { workspaceRoot });
}
export async function acceptTauriMemorySuggestion(workspaceRoot: string, kind: "memory" | "skill", id: string): Promise<TauriMemorySuggestionAcceptance> {
  requireTauri();
  return invoke<TauriMemorySuggestionAcceptance>("accept_memory_suggestion", { request: { workspaceRoot, kind, id } });
}

export type TauriToolApprovalMode = "ask" | "auto" | "yolo";
export interface TauriDesktopPreferences {
  protocolVersion: number;
  defaultToolApprovalMode: TauriToolApprovalMode;
  language: LangPref;
  displayCurrency: "" | "CNY" | "USD";
  terminalTheme: TerminalThemePreference;
  theme: Theme;
  themeStyle: ThemeStyle | "";
  appearanceConfigured: boolean;
}

export async function tauriDesktopPreferences(): Promise<TauriDesktopPreferences> {
  requireTauri();
  return invoke<TauriDesktopPreferences>("desktop_preferences");
}

export async function setTauriDesktopApproval(mode: TauriToolApprovalMode): Promise<TauriDesktopPreferences> {
  requireTauri();
  return invoke<TauriDesktopPreferences>("set_desktop_approval", { mode });
}

export async function setTauriDesktopTerminalTheme(theme: TerminalThemePreference): Promise<TauriDesktopPreferences> {
  requireTauri();
  return invoke<TauriDesktopPreferences>("set_desktop_terminal_theme", { theme });
}

export async function setTauriDesktopAppearance(theme: Theme, style: ThemeStyle): Promise<TauriDesktopPreferences> {
  requireTauri();
  return invoke<TauriDesktopPreferences>("set_desktop_appearance", { theme, style });
}

export async function setTauriDesktopLanguage(language: LangPref): Promise<TauriDesktopPreferences> {
  requireTauri();
  return invoke<TauriDesktopPreferences>("set_desktop_language", { language });
}

export async function setTauriDesktopCurrency(currency: "" | "CNY" | "USD"): Promise<TauriDesktopPreferences> {
  requireTauri();
  return invoke<TauriDesktopPreferences>("set_desktop_currency", { currency });
}

export async function tauriZoomFactor(): Promise<number> {
  requireTauri();
  return invoke<number>("get_zoom_factor");
}

export async function setTauriZoomFactor(factor: number): Promise<number> {
  requireTauri();
  return invoke<number>("set_zoom_factor", { factor });
}

export async function tauriActiveThemeId(): Promise<string> {
  requireTauri();
  return invoke<string>("get_active_theme_id");
}

export async function setTauriActiveThemeId(id: string): Promise<string> {
  requireTauri();
  return invoke<string>("set_active_theme_id", { id });
}

interface TauriUserThemeRecord {
  id: string;
  name: string;
  author?: string;
  description?: string;
  license?: string;
  baseStyle: string;
  tokens: ThemePackView["tokens"];
  density: string;
  corners: string;
  background?: ThemePackView["background"];
  taskBackground?: ThemePackView["taskBackground"];
  backgroundPath?: string;
  taskBackgroundPath?: string;
  kind?: ThemePackView["kind"];
  pluginName?: string;
}

function userThemeView(theme: TauriUserThemeRecord): ThemePackView {
  const backgroundUrl = theme.backgroundPath ? convertFileSrc(theme.backgroundPath) : "";
  const taskBackgroundUrl = theme.taskBackgroundPath ? convertFileSrc(theme.taskBackgroundPath) : "";
  registerTrustedThemeBackgroundURLs([backgroundUrl, taskBackgroundUrl]);
  return {
    id: theme.id,
    name: theme.name,
    author: theme.author,
    description: theme.description,
    license: theme.license,
    baseStyle: theme.baseStyle,
    builtin: false,
    kind: theme.kind ?? "user",
    active: false,
    pluginName: theme.pluginName,
    hasBackground: Boolean((backgroundUrl && theme.background) || (taskBackgroundUrl && theme.taskBackground)),
    backgroundUrl: backgroundUrl || undefined,
    taskBackgroundUrl: taskBackgroundUrl || undefined,
    background: theme.background,
    taskBackground: theme.taskBackground,
    tokens: theme.tokens,
    recipes: { density: theme.density, corners: theme.corners },
  };
}

export async function tauriUserThemes(): Promise<ThemePackView[]> {
  requireTauri();
  const themes = await invoke<TauriUserThemeRecord[]>("list_user_themes");
  return themes.map(userThemeView);
}

export async function tauriPluginThemes(): Promise<ThemePackView[]> {
  requireTauri();
  const themes = await invoke<TauriUserThemeRecord[]>("list_plugin_themes");
  return themes.map(userThemeView);
}

export async function saveTauriUserTheme(theme: Pick<ThemePackView, "id" | "name" | "baseStyle" | "tokens" | "recipes"> & Partial<Pick<ThemePackView, "author" | "description" | "license" | "background" | "taskBackground">> & {
  backgroundDataUrl?: string;
  taskBackgroundDataUrl?: string;
  clearBackground?: boolean;
  clearTaskBackground?: boolean;
}): Promise<ThemePackView> {
  requireTauri();
  const saved = await invoke<TauriUserThemeRecord>("save_user_theme", {
    theme: {
      id: theme.id,
      name: theme.name,
      author: theme.author,
      description: theme.description,
      license: theme.license,
      baseStyle: theme.baseStyle,
      tokens: theme.tokens,
      density: theme.recipes.density ?? "comfortable",
      corners: theme.recipes.corners ?? "soft",
      background: theme.background,
      taskBackground: theme.taskBackground,
      backgroundAssetDataUrl: theme.backgroundDataUrl,
      taskBackgroundAssetDataUrl: theme.taskBackgroundDataUrl,
      clearBackground: theme.clearBackground,
      clearTaskBackground: theme.clearTaskBackground,
    },
  });
  return userThemeView(saved);
}

export async function deleteTauriUserTheme(id: string): Promise<void> {
  requireTauri();
  return invoke<void>("delete_user_theme", { id });
}

export async function importTauriUserTheme(): Promise<ThemePackView | null> {
  requireTauri();
  const imported = await invoke<TauriUserThemeRecord | null>("import_user_theme");
  return imported ? userThemeView(imported) : null;
}

export async function exportTauriUserTheme(id: string): Promise<boolean> {
  requireTauri();
  return invoke<boolean>("export_user_theme", { id });
}

export async function setTauriDefaultModel(model: string, scope: "global" | "project" = "global", workspaceRoot?: string): Promise<TauriProviderSummary> {
  requireTauri();
  return invoke<TauriProviderSummary>("set_default_model", { request: { model, scope, ...(workspaceRoot ? { workspaceRoot } : {}) } });
}

export async function setTauriModelRole(role: "planner" | "vision" | "search", model: string, scope: "global" | "project" = "global", workspaceRoot?: string): Promise<TauriProviderSummary> {
  requireTauri();
  return invoke<TauriProviderSummary>("set_model_role", { request: { role, model, scope, ...(workspaceRoot ? { workspaceRoot } : {}) } });
}

export async function setTauriAgentPreference(request: Pick<BridgeSetAgentPreferenceRequest, "reasoningLanguage" | "compactRatioPercent">, scope: "global" | "project" = "global", workspaceRoot?: string): Promise<TauriProviderSummary> {
  requireTauri();
  if (Object.keys(request).length !== 1) throw new Error("Choose one agent preference at a time.");
  return invoke<TauriProviderSummary>("set_agent_preferences", { request: { ...request, scope, ...(workspaceRoot ? { workspaceRoot } : {}) } });
}

/** One MCP server as the renderer may see it. Credentials are write-only: the
 *  bridge returns the credential key names a server expects, never a value. */
export interface TauriMCPServer {
  name: string;
  enabled: boolean;
  runtimeStatus?: "connected" | "failed" | "initializing";
  toolCount?: number;
  toolList?: { name: string; description?: string }[];
  mcpProtocolVersion?: string;
  mcpSessionState?: string;
  reconnectAttempts?: number;
  errorKind?: string;
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
  toolTimeoutSeconds?: Record<string, number>;
  autoStart?: boolean;
  tier?: string;
  managedByPackage?: boolean;
  nativeOAuthEligible?: boolean;
  authenticationSaved?: boolean;
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
  startupTimeoutSeconds?: number;
  callTimeoutSeconds?: number;
  toolTimeoutSeconds?: Record<string, number>;
  autoStart?: boolean;
  tier?: string;
}

export interface TauriMCPServerMutation {
  protocolVersion: number;
  status: "saved" | "removed" | "activated";
  configPath?: string;
  server?: TauriMCPServer;
  servers: TauriMCPServer[];
}

export interface TauriMCPRuntimeActionResponse {
  protocolVersion: number;
  name: string;
  action: "connect" | "disconnect";
  toolCount: number;
}

export interface TauriMCPClearAuthResponse {
  protocolVersion: number;
  name: string;
  changed: boolean;
}

export interface TauriMCPOAuthFlow {
  protocolVersion: number;
  flowId: string;
  name: string;
  status: "pending" | "complete" | "failed" | "canceled";
}

export async function tauriMCPServers(workspaceRoot?: string): Promise<TauriMCPServer[]> {
  requireTauri();
  return invoke<TauriMCPServer[]>("list_mcp_servers", { workspaceRoot });
}

export async function tauriMCPRuntimeAction(
  sessionId: string,
  name: string,
  action: "connect" | "disconnect",
): Promise<TauriMCPRuntimeActionResponse> {
  requireTauri();
  return invoke<TauriMCPRuntimeActionResponse>("mcp_runtime_action", { request: { sessionId, name, action } });
}

export async function clearTauriMCPAuthentication(sessionId: string, name: string): Promise<TauriMCPClearAuthResponse> {
  requireTauri();
  return invoke<TauriMCPClearAuthResponse>("clear_mcp_authentication", { request: { sessionId, name } });
}

export async function startTauriMCPOAuth(sessionId: string, name: string): Promise<TauriMCPOAuthFlow> {
  requireTauri();
  return invoke<TauriMCPOAuthFlow>("start_mcp_oauth", { request: { sessionId, name } });
}

export async function tauriMCPOAuthStatus(sessionId: string, flowId: string): Promise<TauriMCPOAuthFlow> {
  requireTauri();
  return invoke<TauriMCPOAuthFlow>("mcp_oauth_status", { request: { sessionId, flowId } });
}

export async function cancelTauriMCPOAuth(sessionId: string, flowId: string): Promise<TauriMCPOAuthFlow> {
  requireTauri();
  return invoke<TauriMCPOAuthFlow>("cancel_mcp_oauth", { request: { sessionId, flowId } });
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

export async function setTauriMCPServerEnabled(
  name: string,
  enabled: boolean,
  workspaceRoot?: string,
): Promise<TauriMCPServerMutation> {
  requireTauri();
  return invoke<TauriMCPServerMutation>("set_mcp_server_enabled", { request: { name, enabled }, workspaceRoot });
}

export interface TauriMCPMarketplaceEntry {
  name: string;
  suggestedName: string;
  title?: string;
  description?: string;
  version?: string;
  installable: boolean;
  unavailableReason?: string;
  transport?: "stdio" | "http" | "sse";
  command?: string;
  args?: string[];
  url?: string;
}

export interface TauriMCPMarketplace {
  protocolVersion: number;
  servers: TauriMCPMarketplaceEntry[];
  cached: boolean;
  warning?: string;
}

export async function searchTauriMCPMarketplace(query: string): Promise<TauriMCPMarketplace> {
  requireTauri();
  return invoke<TauriMCPMarketplace>("search_mcp_marketplace", { query });
}

export async function resolveTauriMCPMarketplace(name: string): Promise<TauriMCPMarketplaceEntry> {
  requireTauri();
  return invoke<TauriMCPMarketplaceEntry>("resolve_mcp_marketplace", { name });
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

export async function tauriWorkbenchProjectFolders(): Promise<TauriWorkbenchProjectFolders> {
  requireTauri();
  return invoke<TauriWorkbenchProjectFolders>("workbench_project_folders");
}

export async function rememberTauriWorkbenchProjectFolder(root: string): Promise<TauriWorkbenchProjectFolders> {
  requireTauri();
  return invoke<TauriWorkbenchProjectFolders>("remember_workbench_project_folder", { root });
}

export async function renameTauriWorkbenchProjectFolder(root: string, title: string): Promise<TauriWorkbenchProjectFolders> {
  requireTauri();
  return invoke<TauriWorkbenchProjectFolders>("rename_workbench_project_folder", { root, title });
}

export async function tauriWorkbenchSessionPage(
  cursor?: { position: number; id: string; snapshotId: string; total: number },
  limit = 200,
): Promise<TauriWorkbenchSessionPage> {
  requireTauri();
  return invoke<TauriWorkbenchSessionPage>("workbench_session_page", { limit, cursor });
}

export async function tauriScanUnclaimedSessions(): Promise<TauriScanImportCandidateList> {
  requireTauri();
  return invoke<TauriScanImportCandidateList>("scan_unclaimed_workbench_sessions");
}

export async function tauriImportUnclaimedSessions(selected: TauriScanImportSelection[]): Promise<string[]> {
  requireTauri();
  return invoke<string[]>("import_unclaimed_workbench_sessions", { selected });
}

export async function tauriPendingSessionDeletesPage(
  cursor?: TauriPendingSessionDeleteCursor | null,
): Promise<TauriPendingSessionDeletePage> {
  requireTauri();
  return invoke<TauriPendingSessionDeletePage>("bridge_pending_session_deletes_page", { cursor: cursor ?? null });
}

export async function tauriPendingSessionTitleRecoveries(): Promise<TauriPendingSessionTitleRecovery[]> {
  requireTauri();
  return invoke<TauriPendingSessionTitleRecovery[]>("bridge_pending_session_title_recoveries");
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

export async function backfillTauriWorkbenchTitles(titles: { sessionId: string; title: string }[]): Promise<TauriWorkbenchTitleBackfillResult> {
  requireTauri();
  return invoke<TauriWorkbenchTitleBackfillResult>("backfill_workbench_titles", { titles });
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

export async function chooseTauriSkillSourceDirectory(): Promise<string | null> {
  requireTauri();
  const selected = await openDialog({ directory: true, multiple: false, title: "选择技能来源目录" });
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

export async function setTauriBridgeSessionModel(sessionId: string, model: string): Promise<TauriBridgeSession> {
  requireTauri();
  return invoke<TauriBridgeSession>("bridge_set_session_model", { sessionId, request: { model } });
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

export async function tauriWorkspaceFileRevertPreview(sessionId: string, path: string): Promise<TauriWorkspaceFileRevertPlan> {
  requireTauri();
  const response = await invoke<BridgeWorkspaceFileRevertPlanResponse>("bridge_workspace_file_revert_preview", { request: { sessionId, path } });
  return response.plan;
}

export async function tauriWorkspaceFileRevertCommit(sessionId: string, planId: string, resolution: "" | "overwrite_checkpoint"): Promise<TauriWorkspaceFileRevertResult> {
  requireTauri();
  const response = await invoke<BridgeWorkspaceFileRevertResultResponse>("bridge_workspace_file_revert_commit", { request: { sessionId, planId, resolution } });
  return response.result;
}

export async function tauriWorkspaceFileRevertUndo(sessionId: string, transactionId: string): Promise<TauriWorkspaceFileRevertResult> {
  requireTauri();
  const response = await invoke<BridgeWorkspaceFileRevertResultResponse>("bridge_workspace_file_revert_undo", { request: { sessionId, transactionId } });
  return response.result;
}

export async function tauriWorkspaceCheckpoints(sessionId: string): Promise<TauriWorkspaceCheckpoint[]> {
  requireTauri();
  const response = await invoke<BridgeWorkspaceCheckpointsResponse>("bridge_workspace_checkpoints", { request: { sessionId } });
  return response.checkpoints;
}

export async function tauriCodeRewindPreview(sessionId: string, turn: number): Promise<TauriCodeRewindPlan> {
  requireTauri();
  const response = await invoke<BridgeCodeRewindPlanResponse>("bridge_code_rewind_preview", { request: { sessionId, turn } });
  return response.plan;
}

export async function tauriCodeRewindCommit(sessionId: string, planId: string, confirmPartialCoverage: boolean): Promise<TauriWorkspaceFileRevertResult> {
  requireTauri();
  const response = await invoke<BridgeWorkspaceFileRevertResultResponse>("bridge_code_rewind_commit", { request: { sessionId, planId, confirmPartialCoverage } });
  return response.result;
}

export async function tauriConversationRewindPreview(sessionId: string, turn: number): Promise<TauriConversationRewindPlan> {
  requireTauri();
  const response = await invoke<BridgeConversationRewindPlanResponse>("bridge_conversation_rewind_preview", { request: { sessionId, turn } });
  return response.plan;
}

export async function tauriConversationRewindCommit(sessionId: string, planId: string): Promise<TauriConversationRewindResult> {
  requireTauri();
  const response = await invoke<BridgeConversationRewindResultResponse>("bridge_conversation_rewind_commit", { request: { sessionId, planId } });
  return response.result;
}

export async function tauriConversationRewindUndo(sessionId: string, headId: string): Promise<TauriConversationRewindResult> {
  requireTauri();
  const response = await invoke<BridgeConversationRewindResultResponse>("bridge_conversation_rewind_undo", { request: { sessionId, headId } });
  return response.result;
}

export async function tauriSessionHeads(sessionId: string): Promise<TauriSessionHead[]> {
  requireTauri();
  const response = await invoke<BridgeSessionHeadsResponse>("bridge_session_heads", { request: { sessionId } });
  return response.heads;
}

export async function tauriSessionHeadSwitch(sessionId: string, headId: string): Promise<void> {
  requireTauri();
  const response = await invoke<BridgeSessionHeadSwitchResponse>("bridge_session_head_switch", { request: { sessionId, headId } });
  if (!response.ok) throw new Error("conversation version switch failed");
}

export async function tauriCombinedRewindPreview(sessionId: string, turn: number): Promise<TauriCombinedRewindPlan> {
  requireTauri();
  const response = await invoke<BridgeCombinedRewindPlanResponse>("bridge_combined_rewind_preview", { request: { sessionId, turn } });
  return response.plan;
}

export async function tauriCombinedRewindCommit(sessionId: string, planId: string, confirmPartialCoverage: boolean): Promise<TauriCombinedRewindResult> {
  requireTauri();
  const response = await invoke<BridgeCombinedRewindResultResponse>("bridge_combined_rewind_commit", { request: { sessionId, planId, confirmPartialCoverage } });
  return response.result;
}

export async function tauriLegacyForkPreview(sessionId: string, turn: number): Promise<TauriConversationRewindPlan> {
  requireTauri();
  const response = await invoke<BridgeConversationRewindPlanResponse>("bridge_legacy_fork_preview", { request: { sessionId, turn } });
  return response.plan;
}

export async function tauriLegacyForkCommit(sessionId: string, planId: string): Promise<TauriLegacyConversationForkResult> {
  requireTauri();
  const response = await invoke<BridgeLegacyConversationForkResultResponse>("bridge_legacy_fork_commit", { request: { sessionId, planId } });
  return response.result;
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

export type TauriCloseBehavior = "keep_running" | "quit";

export async function getTauriCloseBehavior(): Promise<TauriCloseBehavior> {
  requireTauri();
  return invoke<TauriCloseBehavior>("get_close_behavior");
}

export async function setTauriCloseBehavior(behavior: TauriCloseBehavior): Promise<TauriCloseBehavior> {
  requireTauri();
  return invoke<TauriCloseBehavior>("set_close_behavior", { behavior });
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

/** Open a web link in the system browser after host-side URL validation. */
export async function openTauriExternalURL(url: string): Promise<void> {
  requireTauri();
  await invoke<void>("open_external_url", { url });
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
  const message = error instanceof Error ? error.message : String(error);
  const localizedErrors: Record<string, Parameters<typeof t>[0]> = {
    "旧版项目文件夹清单暂不可用；当前仅显示 Tauri 本地保存的文件夹。": "settings.data.legacyProjectFoldersUnavailable",
    "Tauri 本地项目文件夹清单暂不可用；当前仅显示旧版项目来源。": "settings.data.tauriProjectFoldersUnavailable",
    "旧版与 Tauri 本地项目文件夹清单都暂不可用，已保存的空项目文件夹可能未显示。": "settings.data.allProjectFoldersUnavailable",
    "旧会话兼容目录无法读取；当前仅显示未核验的持久目录，不能据此操作会话。修复目录后重新检查会话目录。": "settings.data.legacySessionCatalogUnavailable",
    "审核导入只在隔离的 Preview profile 中开放": "settings.data.previewProfileRequired",
    "每项都需要明确确认标题和项目归属": "settings.data.confirmTitleAndProject",
    "插件文件缺失或格式不兼容": "settings.plugins.packageInvalid",
  };
  const key = localizedErrors[message];
  return key ? t(key) : message;
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

/** Delete a secret from the system keychain. Returns true if deleted. */
export async function keychainDelete(key: string): Promise<boolean> {
  requireTauri();
  return invoke<boolean>("keychain_delete", { key });
}
