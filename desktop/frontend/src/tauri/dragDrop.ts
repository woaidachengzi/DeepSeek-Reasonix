import type { DragDropEvent } from "@tauri-apps/api/window";
import type { UnlistenFn } from "@tauri-apps/api/event";

import type { TauriBridgeAttachment } from "../lib/tauriBridge";

export interface TauriDragDropOptions {
  sessionId?: string;
  queuePendingPath?: (path: string) => void;
  attachFile: (sessionId: string, path: string) => Promise<TauriBridgeAttachment>;
  addAttachment: (attachment: TauriBridgeAttachment) => void;
  setDragging: (dragging: boolean) => void;
  isCurrent?: () => boolean;
}

/** Release a listener whose registration finished after its owner was retired. */
export async function retainTauriDragDropListener(
  registration: Promise<UnlistenFn>,
  isCurrent: () => boolean,
): Promise<UnlistenFn | undefined> {
  const unlisten = await registration;
  if (isCurrent()) return unlisten;
  unlisten();
  return undefined;
}

/**
 * Handles the payload delivered by Tauri's native window listener. Keeping it
 * separate from the component makes the production event path directly
 * testable, including partial failures during multi-file drops.
 */
export async function handleTauriDragDropEvent(
  event: DragDropEvent,
  options: TauriDragDropOptions,
): Promise<void> {
  const isCurrent = options.isCurrent ?? (() => true);
  if (!isCurrent()) return;
  if (event.type === "over") {
    options.setDragging(Boolean(options.sessionId || options.queuePendingPath));
    return;
  }
  if (event.type !== "drop") {
    options.setDragging(false);
    return;
  }

  options.setDragging(false);
  if (event.paths.length === 0 || (!options.sessionId && !options.queuePendingPath)) return;
  for (const path of event.paths) {
    if (!isCurrent()) return;
    if (!options.sessionId) {
      options.queuePendingPath?.(path);
      continue;
    }
    try {
      const attachment = await options.attachFile(options.sessionId, path);
      if (!isCurrent()) return;
      options.addAttachment(attachment);
    } catch {
      // A single unreadable file must not discard other dropped files.
    }
  }
}
