// Run: tsx src/__tests__/tauri-drag-drop.test.ts
// Exercises the same helper installed on Tauri's native window listener.

import type { DragDropEvent } from "@tauri-apps/api/window";
import { handleTauriDragDropEvent } from "../tauri/dragDrop";

let passed = 0;
let failed = 0;

function eq(actual: unknown, expected: unknown, label: string) {
  if (actual === expected) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}\n`);
    failed += 1;
  }
}

function dragEvent(type: DragDropEvent["type"], paths: string[] = []): DragDropEvent {
  if (type === "drop") return { type, paths, position: { type: "Physical", x: 0, y: 0 } } as DragDropEvent;
  if (type === "over") return { type, position: { type: "Physical", x: 0, y: 0 } } as DragDropEvent;
  return { type } as DragDropEvent;
}

async function run() {
  const attachments: string[] = [];
  const dragging: boolean[] = [];
  const attachedPaths: string[] = [];
  const options = {
    sessionId: "session-1",
    setDragging: (value: boolean) => { dragging.push(value); },
    addAttachment: (attachment: { path: string }) => { attachments.push(attachment.path); },
    attachFile: async (_sessionId: string, path: string) => {
      attachedPaths.push(path);
      if (path.includes("unreadable")) throw new Error("permission denied");
      return { path, name: path.split("/").pop() ?? path, size: 1, isImage: false };
    },
  };

  await handleTauriDragDropEvent(dragEvent("over"), options);
  eq(dragging[dragging.length - 1], true, "drag over enables visual feedback");

  await handleTauriDragDropEvent(dragEvent("leave"), options);
  eq(dragging[dragging.length - 1], false, "drag leave clears visual feedback");

  await handleTauriDragDropEvent(dragEvent("drop", ["/tmp/one.txt", "/tmp/unreadable.txt", "/tmp/二.md"]), options);
  eq(dragging[dragging.length - 1], false, "drop clears visual feedback");
  eq(attachedPaths.length, 3, "drop attempts every selected path");
  eq(attachments.length, 2, "one failed attachment does not discard successful files");
  eq(attachments[1], "/tmp/二.md", "unicode path reaches the real attachment callback");

  const noSessionPaths: string[] = [];
  await handleTauriDragDropEvent(dragEvent("drop", ["/tmp/ignored.txt"]), {
    ...options,
    sessionId: undefined,
    attachFile: async (_sessionId, path) => {
      noSessionPaths.push(path);
      return { path, name: path, size: 1, isImage: false };
    },
  });
  eq(noSessionPaths.length, 0, "drop without an active session does not attach files");

  process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
  if (failed > 0) process.exit(1);
}

void run();
