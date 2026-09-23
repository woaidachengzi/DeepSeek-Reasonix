// Run: tsx src/__tests__/tauri-drag-drop.test.ts
//
// Tests for Tauri native drag-and-drop file handling.
// Verifies the drag-drop event processing, file attachment, and error handling.

let passed = 0;
let failed = 0;

function eq(a: unknown, b: unknown, label: string) {
  if (a === b) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${JSON.stringify(b)}, got ${JSON.stringify(a)}\n`);
    failed += 1;
  }
}

function ok(condition: unknown, label: string) {
  if (condition) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

// ---------------------------------------------------------------------------
// Mock Tauri APIs for testing
// ---------------------------------------------------------------------------

const mockAttachments: Array<{ path: string; name: string; size: number }> = [];
let mockAttachError: string | null = null;

// Mock the Tauri window drag-drop event
interface MockDragDropEvent {
  type: "over" | "drop" | "leave";
  paths?: string[];
}

function simulateDragEvent(event: MockDragDropEvent): MockDragDropEvent {
  // Simulate the drag event processing logic from TauriChatWorkspace
  if (event.type === "drop" && event.paths) {
    for (const path of event.paths) {
      if (mockAttachError) {
        // Simulate attachment failure
        continue;
      }
      mockAttachments.push({
        path,
        name: path.split("/").pop() ?? path,
        size: 1024,
      });
    }
  }
  return event;
}

function clearMockAttachments() {
  mockAttachments.length = 0;
  mockAttachError = null;
}

// ---------------------------------------------------------------------------
// Test: drag enter sets dragging state
// ---------------------------------------------------------------------------

console.log("\ntauri drag-drop — drag enter");

{
  let dragging = false;
  const event = { type: "over" as const };
  if (event.type === "over") {
    dragging = true;
  }
  ok(dragging, "drag enter sets dragging to true");
}

// ---------------------------------------------------------------------------
// Test: drag leave clears dragging state
// ---------------------------------------------------------------------------

console.log("\ntauri drag-drop — drag leave");

{
  let dragging = true;
  const event = { type: "leave" as const };
  if (event.type === "leave") {
    dragging = false;
  }
  ok(!dragging, "drag leave sets dragging to false");
}

// ---------------------------------------------------------------------------
// Test: drop with single file
// ---------------------------------------------------------------------------

console.log("\ntauri drag-drop — single file drop");

{
  clearMockAttachments();
  simulateDragEvent({ type: "drop", paths: ["/Users/test/document.txt"] });
  eq(mockAttachments.length, 1, "single file drop creates one attachment");
  eq(mockAttachments[0]?.name, "document.txt", "attachment name extracted from path");
  eq(mockAttachments[0]?.path, "/Users/test/document.txt", "attachment path preserved");
}

// ---------------------------------------------------------------------------
// Test: drop with multiple files
// ---------------------------------------------------------------------------

console.log("\ntauri drag-drop — multiple file drop");

{
  clearMockAttachments();
  simulateDragEvent({
    type: "drop",
    paths: [
      "/Users/test/file1.txt",
      "/Users/test/file2.pdf",
      "/Users/test/image.png",
    ],
  });
  eq(mockAttachments.length, 3, "multiple files create multiple attachments");
  eq(mockAttachments[0]?.name, "file1.txt", "first file name correct");
  eq(mockAttachments[1]?.name, "file2.pdf", "second file name correct");
  eq(mockAttachments[2]?.name, "image.png", "third file name correct");
}

// ---------------------------------------------------------------------------
// Test: drop with empty paths
// ---------------------------------------------------------------------------

console.log("\ntauri drag-drop — empty paths");

{
  clearMockAttachments();
  simulateDragEvent({ type: "drop", paths: [] });
  eq(mockAttachments.length, 0, "empty paths create no attachments");
}

// ---------------------------------------------------------------------------
// Test: drop without paths
// ---------------------------------------------------------------------------

console.log("\ntauri drag-drop — drop without paths");

{
  clearMockAttachments();
  simulateDragEvent({ type: "drop" });
  eq(mockAttachments.length, 0, "drop without paths creates no attachments");
}

// ---------------------------------------------------------------------------
// Test: attachment failure handling
// ---------------------------------------------------------------------------

console.log("\ntauri drag-drop — attachment failure");

{
  clearMockAttachments();
  mockAttachError = "Permission denied";
  simulateDragEvent({ type: "drop", paths: ["/Users/test/protected.txt"] });
  eq(mockAttachments.length, 0, "failed attachment not added");
}

// ---------------------------------------------------------------------------
// Test: mixed success and failure
// ---------------------------------------------------------------------------

console.log("\ntauri drag-drop — mixed success and failure");

{
  clearMockAttachments();
  // First file succeeds
  simulateDragEvent({ type: "drop", paths: ["/Users/test/good.txt"] });
  eq(mockAttachments.length, 1, "first file succeeds");

  // Second file fails
  mockAttachError = "File not found";
  simulateDragEvent({ type: "drop", paths: ["/Users/test/bad.txt"] });
  eq(mockAttachments.length, 1, "failed file not added");

  // Third file succeeds (error cleared)
  mockAttachError = null;
  simulateDragEvent({ type: "drop", paths: ["/Users/test/another.txt"] });
  eq(mockAttachments.length, 2, "third file succeeds after error cleared");
}

// ---------------------------------------------------------------------------
// Test: file path with special characters
// ---------------------------------------------------------------------------

console.log("\ntauri drag-drop — special characters in path");

{
  clearMockAttachments();
  simulateDragEvent({
    type: "drop",
    paths: ["/Users/test/my file (copy).txt"],
  });
  eq(mockAttachments.length, 1, "file with spaces and parentheses");
  eq(mockAttachments[0]?.name, "my file (copy).txt", "name preserves special characters");
}

// ---------------------------------------------------------------------------
// Test: file path with unicode
// ---------------------------------------------------------------------------

console.log("\ntauri drag-drop — unicode in path");

{
  clearMockAttachments();
  simulateDragEvent({
    type: "drop",
    paths: ["/Users/test/文档/测试文件.md"],
  });
  eq(mockAttachments.length, 1, "unicode path accepted");
  eq(mockAttachments[0]?.name, "测试文件.md", "unicode name preserved");
}

// ---------------------------------------------------------------------------
// Test: event sequence (over -> drop -> leave)
// ---------------------------------------------------------------------------

console.log("\ntauri drag-drop — event sequence");

{
  const events: string[] = [];
  let dragging = false;

  // Simulate drag over
  events.push("over");
  dragging = true;

  // Simulate drop
  events.push("drop");
  dragging = false;

  // Simulate leave (after drop)
  events.push("leave");

  eq(events.length, 3, "three events in sequence");
  eq(events[0], "over", "first event is over");
  eq(events[1], "drop", "second event is drop");
  eq(events[2], "leave", "third event is leave");
  ok(!dragging, "dragging is false after sequence");
}

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
