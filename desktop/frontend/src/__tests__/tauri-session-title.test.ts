// Run: tsx src/__tests__/tauri-session-title.test.ts
//
// Tests the session-title rules shared by the rename form, the workbench tab
// labels and the remembered-session catalog. These must reject exactly what the
// Rust host (workbench_catalog.rs) and the Go bridge (desktopbridge.RenameSession)
// reject, so the UI never sends a title that the backend refuses.

import {
  TAURI_TITLE_MAX_CHARS,
  tauriSessionTitle,
  tauriTitleError,
  tauriTurnFailure,
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
// Tests: limit agrees with the bridge schema
// ---------------------------------------------------------------------------

console.log("\ntauri session title — shared limit");

// docs/tauri/protocol/v1.schema.json: renameSessionRequest.title maxLength 120.
eq(TAURI_TITLE_MAX_CHARS, 120, "title limit matches the wire schema");

// ---------------------------------------------------------------------------
// Tests: tauriTitleError accepts a valid title
// ---------------------------------------------------------------------------

console.log("\ntauri session title — accepted titles");

eq(tauriTitleError("Release notes"), "", "ordinary title is accepted");
eq(tauriTitleError("  发布说明  "), "", "surrounding whitespace is trimmed, not rejected");
eq(tauriTitleError("界".repeat(TAURI_TITLE_MAX_CHARS)), "", "120 CJK characters are accepted");
eq(tauriTitleError("x".repeat(TAURI_TITLE_MAX_CHARS)), "", "120 ASCII characters are accepted");

// ---------------------------------------------------------------------------
// Tests: tauriTitleError rejects what the backend rejects
// ---------------------------------------------------------------------------

console.log("\ntauri session title — rejected titles");

eq(tauriTitleError(""), "对话名称不能为空", "empty title is rejected");
eq(tauriTitleError("   "), "对话名称不能为空", "whitespace-only title is rejected");
eq(
  tauriTitleError("界".repeat(TAURI_TITLE_MAX_CHARS + 1)),
  `对话名称不能超过 ${TAURI_TITLE_MAX_CHARS} 个字符`,
  "121 characters are rejected",
);
// Counting must be by code point: 120 astral-plane characters are 240 UTF-16
// units, and the Go/Rust sides count runes/chars, not units.
eq(
  tauriTitleError("😀".repeat(TAURI_TITLE_MAX_CHARS)),
  "",
  "120 astral-plane characters are accepted",
);
ok(
  tauriTitleError("😀".repeat(TAURI_TITLE_MAX_CHARS + 1)) !== "",
  "121 astral-plane characters are rejected",
);
eq(tauriTitleError("line\nbreak"), "对话名称不能包含控制字符", "control characters are rejected");
eq(tauriTitleError("tab\there"), "对话名称不能包含控制字符", "tab is rejected");
eq(tauriTitleError("del\x7fchar"), "对话名称不能包含控制字符", "DEL is rejected");

// ---------------------------------------------------------------------------
// Tests: tauriSessionTitle never renders a title the bridge would refuse
// ---------------------------------------------------------------------------

console.log("\ntauri session title — display fallback");

eq(tauriSessionTitle("Release notes", "对话 abc1234"), "Release notes", "stored title wins over the fallback");
eq(tauriSessionTitle("  Release notes  ", "对话 abc1234"), "Release notes", "stored title is trimmed for display");
eq(tauriSessionTitle(undefined, "对话 abc1234"), "对话 abc1234", "missing title falls back to the session label");
eq(tauriSessionTitle("", "对话 abc1234"), "对话 abc1234", "empty title falls back");
eq(tauriSessionTitle("   ", "对话 abc1234"), "对话 abc1234", "whitespace-only title falls back");
eq(
  tauriSessionTitle("界".repeat(TAURI_TITLE_MAX_CHARS + 1), "对话 abc1234"),
  "对话 abc1234",
  "over-long stored title falls back instead of rendering bad host state",
);
eq(tauriSessionTitle("bad\nname", "对话 abc1234"), "对话 abc1234", "stored title with a control character falls back");

// The catalog write path uses an empty fallback: the result must be either a
// valid title or undefined (never an invalid title that the host would reject).
eq(tauriSessionTitle("Release notes", "") || undefined, "Release notes", "catalog write keeps a valid title");
eq(tauriSessionTitle("bad\nname", "") || undefined, undefined, "catalog write drops an invalid title");
eq(tauriSessionTitle(undefined, "") || undefined, undefined, "catalog write drops a missing title");

// ---------------------------------------------------------------------------
// Tests: failed turns must surface a reason
// ---------------------------------------------------------------------------

console.log("\ntauri session title — turn failure reporting");

eq(
  tauriTurnFailure({ eventKind: "turn_done", payload: { status: "completed" } }),
  "",
  "a completed turn reports nothing",
);
eq(
  tauriTurnFailure({
    eventKind: "turn_done",
    payload: { status: "failed", err: "deepseek-flash · Authentication failed (HTTP 401)" },
  }),
  "deepseek-flash · Authentication failed (HTTP 401)",
  "a failed turn surfaces the provider error",
);
eq(
  tauriTurnFailure({ eventKind: "turn_done", payload: { status: "failed" } }),
  "本轮未完成：Agent 没有返回结果",
  "a failed turn without an err still reports a reason",
);
eq(
  tauriTurnFailure({ eventKind: "turn_done", payload: { status: "failed", err: "   " } }),
  "本轮未完成：Agent 没有返回结果",
  "a blank err falls back to the generic reason",
);
eq(
  tauriTurnFailure({ eventKind: "turn_done", payload: { status: "failed", err: 42 } }),
  "本轮未完成：Agent 没有返回结果",
  "a non-string err falls back instead of rendering an object",
);
eq(
  tauriTurnFailure({ eventKind: "text", payload: { status: "failed", err: "ignored" } }),
  "",
  "only turn_done can report a turn failure",
);

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
