import assert from "node:assert/strict";
import { sessionLifecycleFailure, sessionLifecycleNotice } from "../tauri/sessionLifecycleError";

for (const [code, kind] of [
  ["session_missing", "missing"],
  ["session_deleting", "deleting"],
  ["session_deleted", "deleted"],
] as const) {
  const marker = `desktop bridge request failed with status 409 (${code})`;
  assert.equal(sessionLifecycleFailure(marker), kind);
  const notice = sessionLifecycleNotice(new Error(marker));
  if (kind === "missing") assert.ok(notice?.includes("可删除"));
  else assert.ok(notice?.includes("不能重新打开"));
}
assert.equal(sessionLifecycleFailure("desktop bridge request failed with status 500 (session_missing)"), undefined);
assert.equal(sessionLifecycleFailure("desktop bridge request failed with status 409 (unknown)"), undefined);
assert.equal(sessionLifecycleFailure({ message: "session_missing" }), undefined);
assert.equal(sessionLifecycleNotice("desktop bridge request failed with status 409"), undefined);

process.stdout.write("tauri session lifecycle error mapping: OK\n");
