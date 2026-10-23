import type { TauriBridgeEvent } from "../lib/tauriBridge";

export interface ComputerUsePreview {
  appName: string;
  bundleId: string;
  pid: number;
  windowId: number;
  action: string;
  image: string;
}

export interface ComputerUsePreviewState {
  sessionId: string;
  turnId: string;
  ended: boolean;
  preview: ComputerUsePreview | null;
}

const actions = new Set(["observe", "activate", "click", "type", "key", "scroll"]);

export function computerUsePreviewFromEvent(event: TauriBridgeEvent): ComputerUsePreview | null {
  if (event.eventKind !== "tool_progress") return null;
  const tool = event.payload.tool as Record<string, unknown> | undefined;
  const p = tool?.computerUsePreview as Record<string, unknown> | undefined;
  if (tool?.name !== "computer_use" || !p || typeof p.appName !== "string" || !p.appName || p.appName.length > 256
    || typeof p.bundleId !== "string" || p.bundleId.length > 256
    || !Number.isSafeInteger(p.pid) || (p.pid as number) <= 0 || !Number.isSafeInteger(p.windowId)
    || typeof p.action !== "string" || !actions.has(p.action)
    || typeof p.image !== "string" || p.image.length > 132_000 || !/^data:image\/jpeg;base64,[A-Za-z0-9+/]+={0,2}$/.test(p.image)) return null;
  return p as unknown as ComputerUsePreview;
}

// The owner is the session plus durable turn id. Terminal state rejects late
// images, and screenshots are never projected from saved transcript content.
export function advanceComputerUsePreview(state: ComputerUsePreviewState | null, event: TauriBridgeEvent): ComputerUsePreviewState | null {
  const turnId = event.payload.turnId;
  if (typeof turnId !== "string" || !turnId) return state;
  const same = state?.sessionId === event.sessionId && state.turnId === turnId;
  if (event.eventKind === "turn_started") return same ? state : { sessionId: event.sessionId, turnId, ended: false, preview: null };
  if (event.eventKind === "turn_done") return state && !same ? state : { sessionId: event.sessionId, turnId, ended: true, preview: null };
  if (state && (!same || state.ended)) return state;
  const preview = computerUsePreviewFromEvent(event);
  return preview ? { sessionId: event.sessionId, turnId, ended: false, preview } : state;
}

export function clampPreviewPosition(x: number, y: number, width: number, height: number, viewportWidth: number, viewportHeight: number) {
  return { x: Math.max(8, Math.min(x, viewportWidth - width - 8)), y: Math.max(8, Math.min(y, viewportHeight - height - 8)) };
}
