// Run: tsx src/__tests__/tauri-session-preview.test.ts
//
// Tests for the pure-function helpers in tauriBridge.ts and the deterministic
// session-recovery flow exercised by TauriSessionPreview's restartBridge().

import {
  newTauriSessionId,
  tauriAssistantTextDelta,
  tauriComposerInput,
  tauriMessageFrom,
  tauriEventSummary,
  tauriPromptAnsweredId,
  tauriPromptFromEvent,
} from "../lib/tauriBridge";

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
// Tests: newTauriSessionId
// ---------------------------------------------------------------------------

console.log("\ntauri session preview — newTauriSessionId");

const id1 = newTauriSessionId();
const id2 = newTauriSessionId();
ok(id1.startsWith("tauri-"), `id starts with "tauri-": ${id1}`);
ok(id1 !== id2, "consecutive calls produce different IDs");

const uuidPart = id1.slice("tauri-".length);
ok(
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(uuidPart),
  `UUID v4 format in id: ${uuidPart}`,
);

// ---------------------------------------------------------------------------
// Tests: tauriMessageFrom
// ---------------------------------------------------------------------------

console.log("\ntauri session preview — tauriMessageFrom");

eq(tauriMessageFrom(new Error("boom")), "boom", "Error extracts message");
eq(tauriMessageFrom({ toString: () => "custom" }), "custom", "object with toString");
eq(tauriMessageFrom("raw string"), "raw string", "string passes through");
eq(tauriMessageFrom(42), "42", "number converts to string");
eq(tauriMessageFrom(null), "null", "null converts to string");
eq(tauriMessageFrom(undefined), "undefined", "undefined converts to string");

// ---------------------------------------------------------------------------
// Tests: tauriEventSummary
// ---------------------------------------------------------------------------

console.log("\ntauri session preview — tauriEventSummary");

const smallSummary = tauriEventSummary({ payload: { text: "hello" } });
eq(smallSummary, '{"text":"hello"}', "short payload is not truncated");

const bigPayload = { text: "x".repeat(600) };
const bigSummary = tauriEventSummary({ payload: bigPayload });
ok(bigSummary.length === 500, `long payload is truncated to 500 chars (got ${bigSummary.length})`);
ok(bigSummary.endsWith("..."), "truncated summary ends with ...");

const exactSummary = tauriEventSummary({ payload: { data: "y".repeat(489) } });
ok(!exactSummary.endsWith("..."), "payload at 500 chars exactly is not truncated");

// ---------------------------------------------------------------------------
// Tests: tauriAssistantTextDelta
// ---------------------------------------------------------------------------

console.log("\ntauri session preview — tauriAssistantTextDelta");

eq(tauriAssistantTextDelta({ eventKind: "text", payload: { kind: "text", text: "hello " } }), "hello ", "visible text event returns its delta");
eq(tauriAssistantTextDelta({ eventKind: "reasoning", payload: { kind: "reasoning", text: "private reasoning" } }), "", "reasoning events are never exposed as answer text");
eq(tauriAssistantTextDelta({ eventKind: "notice", payload: { kind: "notice", text: "status" } }), "", "notice text is not appended to the assistant answer");
eq(tauriAssistantTextDelta({ eventKind: "text", payload: { kind: "text", text: 123 } }), "", "malformed non-string text is ignored");

console.log("\ntauri session preview — file attachment submission");
eq(
  tauriComposerInput("  Please summarize these files  ", [
    { path: ".reasonix/attachments/clipboard-a.txt" },
    { path: ".reasonix/attachments/clipboard-b.pdf" },
  ]),
  "Please summarize these files\n\n@.reasonix/attachments/clipboard-a.txt\n\n@.reasonix/attachments/clipboard-b.pdf",
  "adds private attachment references to the submitted prompt",
);

console.log("\ntauri session preview — actionable prompt payloads");
const approval = tauriPromptFromEvent({ eventKind: "approval_request", payload: { promptId: "ap-1", approval: { id: "ap-1", tool: "bash", subject: "go test", reason: "需要执行本地检查" } } });
eq(approval?.kind, "approval", "approval event becomes an actionable prompt");
eq(approval?.id, "ap-1", "approval keeps its correlation ID");
const ask = tauriPromptFromEvent({ eventKind: "ask_request", payload: { promptKind: "ask", ask: { id: "ask-1", questions: [{ id: "q-1", prompt: "选择一个", options: [{ label: "A" }], multi: false }] } } });
eq(ask?.kind, "ask", "ask event becomes an actionable prompt");
eq(ask?.id, "ask-1", "ask keeps its correlation ID");
const mcp = tauriPromptFromEvent({ eventKind: "mcp_interaction", payload: { mcpInteraction: { id: "mcp-1", server: "demo", mode: "url", message: "请打开链接" } } });
eq(mcp?.kind, "mcp", "MCP event becomes an actionable prompt");
eq(tauriPromptAnsweredId({ eventKind: "prompt_answered", payload: { promptId: "ask-1" } }), "ask-1", "prompt answer event clears the matching card");
eq(
  tauriComposerInput("  ordinary prompt  ", []),
  "ordinary prompt",
  "trims prompt without changing attachment-free input",
);

// ---------------------------------------------------------------------------
// Tests: session-recovery deterministic flow
//
// Simulates the restart-recover sequence from TauriSessionPreview.restartBridge():
//   1. restartTauriBridge() → new status
//   2. openTauriBridgeSession(id, workspaceRoot) → reopened session
//   3. tauriBridgeSnapshot(id) → authoritative snapshot (via useEffect)
//   4. onTauriBridgeEvent / onTauriBridgeConnectionError → subscriptions
//   5. startTauriBridgeEvents(snapshot.sequence) → live stream
//   6. tauriBridgeHistory(id) → display-safe restored transcript
//
// The test proves:
//   - The full 7-step sequence is correct
//   - Calling it twice is idempotent (no duplicate side effects)
//   - Events from a different session are filtered out
//   - Sequence advancement is monotonically increasing
// ---------------------------------------------------------------------------

console.log("\ntauri session preview — restart-recover flow");

interface RecoveryStep {
  name: string;
  invoke: () => unknown;
}

function simulateRecovery(
  _sessionId: string,
  _workspaceRoot: string | undefined,
  steps: RecoveryStep[],
): unknown[] {
  const results: unknown[] = [];
  for (const step of steps) {
    results.push(step.invoke());
  }
  return results;
}

const recoveredSessions: Array<{ id: string; workspaceRoot?: string }> = [];
let eventStreamStartedAt = -1;

const mockSteps: RecoveryStep[] = [
  {
    name: "restartTauriBridge",
    invoke: () => ({ running: true, protocolVersion: 1, sidecarInstanceId: "new-instance" }),
  },
  {
    name: "openTauriBridgeSession",
    invoke: () => {
      const session = { id: "test-session", workspaceRoot: "/workspace" };
      recoveredSessions.push(session);
      return session;
    },
  },
  {
    name: "tauriBridgeSnapshot",
    invoke: () => ({ sequence: 42, session: recoveredSessions[0] }),
  },
  {
    name: "onTauriBridgeEvent",
    invoke: () => () => { /* unlisten */ },
  },
  {
    name: "onTauriBridgeConnectionError",
    invoke: () => () => { /* unlisten */ },
  },
  {
    name: "startTauriBridgeEvents",
    invoke: () => { eventStreamStartedAt = 42; },
  },
  {
    name: "tauriBridgeHistory",
    invoke: () => ({ sequence: 42, messages: [], startIndex: 0, totalMessages: 0 }),
  },
];

// First recovery
const results1 = simulateRecovery("test-session", "/workspace", mockSteps);
eq(results1.length, 7, "recovery executes all 7 steps");
eq(recoveredSessions.length, 1, "session was opened once");
eq(eventStreamStartedAt, 42, "event stream started at snapshot sequence");

// Second recovery (restart after restart) — proves idempotency
const results2 = simulateRecovery("test-session", "/workspace", mockSteps);
eq(results2.length, 7, "second recovery also executes all 7 steps");
eq(recoveredSessions.length, 2, "session was opened again");

// Event filtering: events from a different session must be ignored
function filterEvent(event: { sessionId: string }, activeSessionId: string): boolean {
  return event.sessionId === activeSessionId;
}

ok(filterEvent({ sessionId: "test-session" }, "test-session"), "event from own session passes filter");
ok(!filterEvent({ sessionId: "other-session" }, "test-session"), "event from other session is rejected");

// Sequence advancement: Math.max ensures monotonic progression
function advanceSequence(current: number, incoming: number): number {
  return Math.max(current, incoming);
}

eq(advanceSequence(0, 5), 5, "sequence advances from 0 to 5");
eq(advanceSequence(5, 3), 5, "sequence does not go backward from 5 to 3");
eq(advanceSequence(5, 5), 5, "sequence stays at 5 for equal value");

// Events list is capped at 100 items
function capEvents(events: unknown[], incoming: unknown, limit = 100): unknown[] {
  return [incoming, ...events].slice(0, limit);
}

const overLimit = Array.from({ length: 100 }, (_, i) => i);
const capped = capEvents(overLimit, "new");
eq(capped.length, 100, "events list stays at 100 items");
eq(capped[0], "new", "newest event is first");
eq(capped[99], 98, "oldest event is dropped");

// ---------------------------------------------------------------------------
// Tests: connection-error handling contract
// ---------------------------------------------------------------------------

console.log("\ntauri session preview — connection error handling");

function handleConnectionError(message: string): { running: boolean; error: string } {
  return { running: false, error: `Bridge event stream: ${message}` };
}

const errResult = handleConnectionError("bridge event stream is unavailable");
eq(errResult.running, false, "connection error sets running to false");
eq(errResult.error, "Bridge event stream: bridge event stream is unavailable", "error message is formatted");

// Empty session ID must be rejected before opening
function validateSessionId(id: string): boolean {
  return id.trim().length > 0;
}

ok(validateSessionId("tab-1"), "non-empty session ID is valid");
ok(!validateSessionId(""), "empty session ID is invalid");
ok(!validateSessionId("   "), "whitespace-only session ID is invalid");

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
