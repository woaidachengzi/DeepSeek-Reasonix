import type { BridgeHistoryMessage } from "../lib/bridgeProtocol.generated";

export interface IndexedHistoryMessage {
  index: number;
  message: BridgeHistoryMessage;
}

export type HistoryPresentation =
  | { kind: "message"; entry: IndexedHistoryMessage }
  | { kind: "progress"; anchorIndex: number; entries: IndexedHistoryMessage[]; durationMs: number; active: boolean };

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
    // A timed single answer still gets its settled turn header; it has no
    // hidden process to expand. Legacy untimed single answers stay unchanged.
    if (progress.length > 0 || (final && (final.message.workDurationMs ?? 0) > 0)) {
      result.push({
        kind: "progress",
        anchorIndex: assistants[0].index,
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
  const total = Math.max(1, Math.floor(durationMs / 1000));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor(total / 60) % 60;
  const seconds = total % 60;
  return `用时 ${hours > 0 ? `${hours}小时` : ""}${total >= 60 ? `${minutes}分` : ""}${seconds}秒`;
}

/** Date-aware local clock, following Harness message chrome. Source times
 * stay authoritative; invalid/legacy missing times must not become "now". */
export function formatTauriMessageClock(timestamp: number | undefined, now = Date.now(), locale: "zh" | "zh-TW" | "en" = "zh"): string | null {
  if (!timestamp || !Number.isSafeInteger(timestamp) || timestamp <= 0) return null;
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) return null;
  const reference = new Date(now);
  const hour = String(date.getHours()).padStart(2, "0");
  const minute = String(date.getMinutes()).padStart(2, "0");
  const clock = `${hour}:${minute}`;
  const year = date.getFullYear(), month = date.getMonth() + 1, day = date.getDate();
  if (year === reference.getFullYear() && date.getMonth() === reference.getMonth() && day === reference.getDate()) return clock;
  const sameYear = year === reference.getFullYear();
  const calendar = locale === "en" ? sameYear ? `${month}/${day}` : `${year}-${month}-${day}` : `${sameYear ? "" : `${year}年`}${month}月${day}日`;
  return `${calendar} ${clock}`;
}
