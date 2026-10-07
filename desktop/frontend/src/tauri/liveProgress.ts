import type { TauriBridgeEvent } from "../lib/tauriBridge";

export type LiveTool = { id: string; name: string; state: "running" | "done" | "failed" };
export type LiveProgress = { phase: "thinking" | "answering" | "tool" | "finishing"; tools: LiveTool[] };
export const EMPTY_LIVE_PROGRESS: LiveProgress = { phase: "thinking", tools: [] };

/** Project public execution metadata only. Never render reasoning, arguments, or raw outputs. */
export function advanceLiveProgress(previous: LiveProgress, event: TauriBridgeEvent): LiveProgress {
  if (event.payload.kind !== event.eventKind) return previous;
  if (event.eventKind === "turn_started") return EMPTY_LIVE_PROGRESS;
  if (event.eventKind === "text") return previous.phase === "answering" ? previous : { ...previous, phase: "answering" };
  if (event.eventKind === "reasoning") return previous.phase === "thinking" ? previous : { ...previous, phase: "thinking" };
  if (event.eventKind === "turn_done") return { ...previous, phase: "finishing" };
  if (event.eventKind !== "tool_dispatch" && event.eventKind !== "tool_result") return previous;
  const raw = event.payload.tool;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return previous;
  const tool = raw as Record<string, unknown>;
  if (typeof tool.id !== "string" || !tool.id) return previous;
  const existing = previous.tools.find(item => item.id === tool.id);
  const item: LiveTool = {
    id: tool.id,
    name: typeof tool.name === "string" && tool.name ? tool.name.slice(0, 120) : existing?.name ?? "工具",
    state: event.eventKind === "tool_result" ? tool.err ? "failed" : "done" : existing?.state ?? "running",
  };
  const tools = existing
    ? previous.tools.map(value => value.id === item.id ? item : value)
    : [...previous.tools, item].slice(-30);
  return { phase: event.eventKind === "tool_dispatch" ? "tool" : "thinking", tools };
}

export function liveProgressLabel(progress: LiveProgress, paused: boolean): string {
  if (paused) return "等待你的确认…";
  if (progress.phase === "answering") return "正在回答…";
  if (progress.phase === "finishing") return "正在整理回答…";
  if (progress.phase === "tool") return "正在执行工具…";
  return "正在思考…";
}
