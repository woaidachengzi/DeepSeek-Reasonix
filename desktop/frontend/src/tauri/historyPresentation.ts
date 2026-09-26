import type { BridgeHistoryMessage } from "../lib/bridgeProtocol.generated";

export interface IndexedHistoryMessage {
  index: number;
  message: BridgeHistoryMessage;
}

export type HistoryPresentation =
  | { kind: "message"; entry: IndexedHistoryMessage }
  | { kind: "progress"; entries: IndexedHistoryMessage[]; durationMs: number; active: boolean };

// The bridge exposes visible assistant text, not a commentary/final channel.
// Within each user turn, the last completed assistant message is the answer;
// preceding assistant messages are progress that can be folded without loss.
export function groupTauriHistory(
  messages: BridgeHistoryMessage[],
  startIndex: number,
  activeTail: boolean,
): HistoryPresentation[] {
  const result: HistoryPresentation[] = [];
  let assistants: IndexedHistoryMessage[] = [];
  const flush = (active: boolean) => {
    if (assistants.length === 0) return;
    const final = active ? null : assistants[assistants.length - 1];
    const progress = active ? assistants : assistants.slice(0, -1);
    if (progress.length > 0) {
      result.push({
        kind: "progress",
        entries: progress,
        durationMs: final?.message.workDurationMs || progress[progress.length - 1]?.message.workDurationMs || 0,
        active,
      });
    }
    if (final) result.push({ kind: "message", entry: final });
    assistants = [];
  };
  messages.forEach((message, offset) => {
    const entry = { index: startIndex + offset, message };
    if (message.role === "assistant") {
      assistants.push(entry);
    } else {
      flush(false);
      result.push({ kind: "message", entry });
    }
  });
  flush(activeTail);
  return result;
}

export function formatTauriWorkDuration(durationMs: number): string | null {
  if (!Number.isFinite(durationMs) || durationMs <= 0) return null;
  const seconds = Math.ceil(durationMs / 1000);
  if (seconds < 60) return `用时 ${seconds} 秒`;
  const minutes = Math.ceil(seconds / 60);
  if (minutes < 60) return `用时 ${minutes} 分钟`;
  return `用时 ${Math.floor(minutes / 60)} 小时 ${minutes % 60} 分钟`;
}
