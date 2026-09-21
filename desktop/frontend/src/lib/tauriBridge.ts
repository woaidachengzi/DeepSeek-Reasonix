import { invoke, isTauri } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import type {
  BridgeEvent,
  BridgeHistoryMessage,
  BridgeProviderSummaryResponse,
  BridgeSession,
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
}

export interface TauriProfileImportResult {
  importedConfig: string;
  backupConfig: string;
}

export interface TauriPreviewRuntimeInfo {
  stableVersion: string;
  stableCommit: string;
  previewVersion: string;
  tauriVersion: string;
  bridgeProtocolVersion: number;
  sidecarInstanceId?: string;
}

export interface TauriWorkbenchSession {
  sessionId: string;
  workspaceRoot?: string;
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

export async function importTauriStableProfile(): Promise<TauriProfileImportResult> {
  requireTauri();
  return invoke<TauriProfileImportResult>("import_stable_profile", { confirmed: true });
}

export async function tauriWorkbenchSessions(): Promise<TauriWorkbenchSession[]> {
  requireTauri();
  return invoke<TauriWorkbenchSession[]>("workbench_sessions");
}

export async function rememberTauriWorkbenchSession(
  sessionId: string,
  workspaceRoot?: string,
): Promise<TauriWorkbenchSession[]> {
  requireTauri();
  return invoke<TauriWorkbenchSession[]>("remember_workbench_session", {
    request: { sessionId, workspaceRoot },
  });
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

export async function openTauriBridgeSession(sessionId: string, workspaceRoot?: string): Promise<TauriBridgeSession> {
  requireTauri();
  return invoke<TauriBridgeSession>("bridge_open_session", { request: { sessionId, workspaceRoot } });
}

export async function switchTauriBridgeSession(sessionId: string, workspaceRoot?: string): Promise<TauriBridgeSession> {
  requireTauri();
  return invoke<TauriBridgeSession>("bridge_switch_session", { request: { sessionId, workspaceRoot } });
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

export async function cancelTauriBridge(sessionId: string): Promise<TauriBridgeSession> {
  requireTauri();
  return invoke<TauriBridgeSession>("bridge_cancel", { request: { sessionId } });
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
