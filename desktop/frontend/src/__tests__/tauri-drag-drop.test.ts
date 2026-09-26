// Run: tsx src/__tests__/tauri-drag-drop.test.ts
// Exercises the same helper installed on Tauri's native window listener.

import type { DragDropEvent } from "@tauri-apps/api/window";
import { handleTauriDragDropEvent, retainTauriDragDropListener } from "../tauri/dragDrop";

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
  await handleTauriDragDropEvent(dragEvent("over"), { ...options, sessionId: undefined });
  eq(dragging[dragging.length - 1], false, "drag over an unavailable composer does not show attachment feedback");

  const draftPaths: string[] = [];
  const draftOptions = { ...options, sessionId: undefined, queuePendingPath: (path: string) => { draftPaths.push(path); } };
  await handleTauriDragDropEvent(dragEvent("over"), draftOptions);
  eq(dragging[dragging.length - 1], true, "draft composer accepts file drops without opening a session");
  await handleTauriDragDropEvent(dragEvent("drop", ["/tmp/draft-a.txt", "/tmp/draft-b.txt"]), draftOptions);
  eq(draftPaths.length, 2, "draft drop queues files until the first send");
  eq(attachedPaths.length, 3, "draft drop does not copy files into a session");

  let current = true;
  let resolveFirst: ((attachment: { path: string; name: string; size: number; isImage: boolean }) => void) | undefined;
  const lateAttachments: string[] = [];
  const latePaths: string[] = [];
  const pendingDrop = handleTauriDragDropEvent(dragEvent("drop", ["/tmp/old.txt", "/tmp/second.txt"]), {
    sessionId: "session-1",
    isCurrent: () => current,
    setDragging: () => {},
    addAttachment: attachment => { lateAttachments.push(attachment.path); },
    attachFile: async (_sessionId, path) => {
      latePaths.push(path);
      return new Promise(resolve => { resolveFirst = resolve; });
    },
  });
  current = false;
  resolveFirst?.({ path: "/tmp/old.txt", name: "old.txt", size: 1, isImage: false });
  await pendingDrop;
  eq(lateAttachments.length, 0, "attachment resolving after session change is not added to the new composer");
  eq(latePaths.length, 1, "session change prevents the rest of a multi-file drop from starting");

  const draggingBeforeStaleEvent = dragging.length;
  await handleTauriDragDropEvent(dragEvent("over"), { ...options, isCurrent: () => false });
  eq(dragging.length, draggingBeforeStaleEvent, "retired native listener cannot update drag feedback");

  let finishRegistration: ((unlisten: () => void) => void) | undefined;
  const registration = new Promise<() => void>(resolve => { finishRegistration = resolve; });
  let stillMounted = true;
  let unlistenCount = 0;
  const retained = retainTauriDragDropListener(registration, () => stillMounted);
  stillMounted = false;
  finishRegistration?.(() => { unlistenCount += 1; });
  eq(await retained, undefined, "late native listener registration is not retained after cleanup");
  eq(unlistenCount, 1, "late native listener registration is immediately removed");

  process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
  if (failed > 0) process.exit(1);
}

void run();
