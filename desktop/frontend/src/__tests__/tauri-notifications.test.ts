import assert from "node:assert/strict";
import { NotificationClickPump, type NotificationClickDependencies } from "../tauri/tauriNotifications";

const click = { token: "owned-token", sessionId: "session-one" };
const target = { sessionId: "session-one", workspaceRoot: "/canonical/project" };
const ticks = async () => { for (let i = 0; i < 8; i += 1) await Promise.resolve(); };
let queued = [click];
let allowed = false;
let opens = 0;
let acks = 0;
let missing = 0;
let errors = 0;
let resolved: typeof target | null = target;
const deps: NotificationClickDependencies = {
  pending: async () => queued,
  resolve: async () => resolved,
  acknowledge: async () => { acks += 1; queued = []; },
  canOpen: () => allowed,
  open: async route => { assert.deepEqual(route, target); opens += 1; return true; },
  unavailable: () => { missing += 1; },
  failed: () => { errors += 1; },
};
const pump = new NotificationClickPump(deps);
await pump.wake();
assert.equal(acks, 0, "busy/read-only surfaces keep the native click queued");
allowed = true;
await pump.wake();
assert.equal(opens, 1);
assert.equal(acks, 1);
await pump.wake();
assert.equal(opens, 1, "an acknowledged click does not replay");
queued = [click]; resolved = null;
await pump.wake();
assert.equal(opens, 1, "missing sessions are never created or switched");
assert.equal(missing, 1);
assert.equal(acks, 2, "stale clicks do not block later valid targets");

let release!: () => void;
const gate = new Promise<void>(resolve => { release = resolve; });
queued = [click]; resolved = target;
const concurrent = new NotificationClickPump({ ...deps, resolve: async () => { await gate; return target; } });
const first = concurrent.wake();
await ticks();
void concurrent.wake(); void concurrent.wake();
release(); await first; await ticks();
assert.equal(opens, 2, "duplicate signals share one native-target navigation");
assert.equal(acks, 3);

queued = [click];
const failed = new NotificationClickPump({ ...deps, open: async () => false });
await failed.wake();
assert.equal(acks, 3, "failed navigation remains queued for a later retry");
const mismatch = new NotificationClickPump({ ...deps, resolve: async () => ({ sessionId: "other-session" }) });
await mismatch.wake();
assert.equal(errors, 1);
assert.equal(acks, 3, "a mismatched target is never consumed or opened");

let stopRelease!: () => void;
const stopGate = new Promise<void>(resolve => { stopRelease = resolve; });
const stopped = new NotificationClickPump({ ...deps, pending: async () => { await stopGate; return [click]; } });
const inFlight = stopped.wake(); stopped.stop(); stopRelease(); await inFlight;
assert.equal(opens, 2, "unmount fences late native replies before navigation");
assert.equal(acks, 3);

const ackFailure = new NotificationClickPump({ ...deps, acknowledge: async () => { throw new Error("private-native-diagnostic"); } });
await ackFailure.wake();
assert.equal(errors, 2, "ack failure is surfaced without leaking the native error");
assert.equal(queued.length, 1, "failed ack retains a recoverable click");
console.log("Tauri notification click lifecycle passed");
