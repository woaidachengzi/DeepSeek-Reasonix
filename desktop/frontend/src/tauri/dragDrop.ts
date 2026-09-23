import type { DragDropEvent } from "@tauri-apps/api/window";

import type { TauriBridgeAttachment } from "../lib/tauriBridge";

export interface TauriDragDropOptions {
  sessionId?: string;
  attachFile: (sessionId: string, path: string) => Promise<TauriBridgeAttachment>;
  addAttachment: (attachment: TauriBridgeAttachment) => void;
  setDragging: (dragging: boolean) => void;
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
  if (event.type === "over") {
    options.setDragging(true);
    return;
  }
  if (event.type !== "drop") {
    options.setDragging(false);
    return;
  }

  options.setDragging(false);
  if (!options.sessionId || event.paths.length === 0) return;
  for (const path of event.paths) {
    try {
      options.addAttachment(await options.attachFile(options.sessionId, path));
    } catch {
      // A single unreadable file must not discard other dropped files.
    }
  }
}
