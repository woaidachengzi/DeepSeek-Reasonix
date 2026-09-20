import { invoke, isTauri } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";

export interface TauriBridgeStatus {
  running: boolean;
  protocolVersion?: number;
  sidecarInstanceId?: string;
}

export interface TauriBridgeSession {
  id: string;
  path: string;
  workspaceRoot?: string;
  state: "idle" | "running" | "paused";
}

export interface TauriBridgeSnapshot {
  sequence: number;
  session: TauriBridgeSession;
}

export interface TauriBridgeEvent {
  protocolVersion: number;
  sequence: number;
  eventKind: string;
  sessionId: string;
  tabId?: string;
  payload: Record<string, unknown>;
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

export async function openTauriBridgeSession(sessionId: string, workspaceRoot?: string): Promise<TauriBridgeSession> {
  requireTauri();
  return invoke<TauriBridgeSession>("bridge_open_session", { request: { sessionId, workspaceRoot } });
}

export async function tauriBridgeSnapshot(sessionId: string): Promise<TauriBridgeSnapshot> {
  requireTauri();
  return invoke<TauriBridgeSnapshot>("bridge_session_snapshot", { request: { sessionId } });
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
