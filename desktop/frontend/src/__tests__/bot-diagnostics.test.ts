import assert from "node:assert/strict";
import { parseBotDiagnostics } from "../lib/botDiagnostics";

const fixture = () => ({ protocolVersion: 1, runtimeObservationOnly: true, connections: [
  { id: "legacy:qq", configStatus: "configured", runtimeStatus: "not_observed" },
] });
const value = fixture();
const parsed = parseBotDiagnostics(value);
assert.deepEqual(parsed, value);
assert.notEqual(parsed.connections[0], value.connections[0], "return owned projection, not caller's mutable row");
assert.deepEqual(parseBotDiagnostics({ ...value, connections: [] }).connections, []);
for (const configStatus of ["disabled", "bot_disabled", "missing_credentials", "access_blocked", "configured"]) {
  for (const runtimeStatus of ["not_observed", "refreshing", "unknown", "configured", "disabled", "running", "error", "closed", "degraded"]) {
    assert.equal(parseBotDiagnostics({ ...value, connections: [{ ...value.connections[0], configStatus, runtimeStatus }] }).connections[0].runtimeStatus, runtimeStatus);
  }
}
const canary = "private-sdk-secret-canary";
for (const malformed of [null, [], {}, { ...value, protocolVersion: 2 },
  { ...value, runtimeObservationOnly: false }, { ...value, sdkError: canary },
  { ...value, connections: null }, { ...value, connections: [value.connections[0], value.connections[0]] },
  ...["", "x\n", "\u0085", "中".repeat(86)].map(id => ({ ...value, connections: [{ ...value.connections[0], id }] })),
  ...[ { secret: canary }, { runtimeStatus: canary }, { configStatus: canary } ].map(patch => ({ ...value, connections: [{ ...value.connections[0], ...patch }] })),
  { ...value, connections: Array(10_001).fill(value.connections[0]) },
]) {
  assert.throws(() => parseBotDiagnostics(malformed), error => error instanceof Error
    && error.message.includes("update the Preview") && !error.message.includes(canary));
}
console.log("PASS bot diagnostics: fixed status matrix, owned projection, strict/private/bounded response rejection");

async function bindingTest() {
  const calls: { name: string; args: unknown }[] = [];
  let response: unknown = fixture();
  Object.assign(globalThis, { isTauri: true, window: { __TAURI_INTERNALS__: {
    invoke: async (name: string, args: unknown) => { calls.push({ name, args }); return response; },
  } } });
  const { tauriBotConnectionDiagnostics } = await import("../lib/tauriBridge");
  assert.deepEqual(await tauriBotConnectionDiagnostics(), fixture());
  assert.deepEqual(calls, [{ name: "bot_connection_diagnostics", args: {} }], "fixed read-only command takes no URL, token, account or activation arguments");
  response = { ...fixture(), sdkError: canary };
  await assert.rejects(tauriBotConnectionDiagnostics(), error => error instanceof Error && !error.message.includes(canary));
  Object.assign(globalThis, { isTauri: false });
  await assert.rejects(tauriBotConnectionDiagnostics(), /host is unavailable/);
  assert.equal(calls.length, 2, "no invoke outside Tauri and no fallback activation");
  console.log("PASS bot diagnostics: actual frontend binding invokes only the fixed no-argument command and rejects invalid responses");
}
void bindingTest().catch(error => { console.error(error); process.exitCode = 1; });
