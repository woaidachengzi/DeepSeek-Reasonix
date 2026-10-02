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
// A native signal can arrive while an older pending() snapshot is in flight.
// Its empty result must not swallow the only wake for the newly queued click.
let releaseEmpty!: () => void;
const emptySnapshot = new Promise<typeof queued>(resolve => { releaseEmpty = () => resolve([]); });
let pendingQueries = 0;
let racedOpens = 0;
let racedAcks = 0;
let racedQueue = [click];
const emptyRace = new NotificationClickPump({
  ...deps,
  pending: async () => ++pendingQueries === 1 ? emptySnapshot : racedQueue,
  open: async () => { racedOpens += 1; return true; },
  acknowledge: async () => { racedAcks += 1; racedQueue = []; },
});
const emptyInFlight = emptyRace.wake();
await ticks();
await emptyRace.wake();
releaseEmpty();
await emptyInFlight;
await ticks();
assert.equal(racedOpens, 1, "a native wake during an empty snapshot opens the newly queued click");
assert.equal(racedAcks, 1, "the raced click is acknowledged once without a readiness change");

// A coalesced event must not create a tight retry loop after storage failure.
let releaseFailure!: () => void;
const failedSnapshot = new Promise<typeof queued>((_resolve, reject) => { releaseFailure = () => reject(new Error("private-storage-error")); });
let failureQueries = 0;
let failureReports = 0;
const failureRace = new NotificationClickPump({
  ...deps,
  pending: async () => { failureQueries += 1; return failedSnapshot; },
  failed: () => { failureReports += 1; },
});
const failureInFlight = failureRace.wake();
await failureRace.wake();
releaseFailure();
await failureInFlight;
await ticks();
assert.equal(failureQueries, 1, "a failed query waits for a later real wake");
assert.equal(failureReports, 1);

// The native queue never exceeds 32, but a new click can refill a slot while
// this consumer is draining its first batch. Its coalesced signal must not
// leave that click waiting forever once the per-batch limit is reached.
let batchQueue = Array.from({ length: 32 }, (_, index) => ({ ...click, token: `batch-${index}` }));
const batchAcknowledged: string[] = [];
const batchOpensBefore = opens;
let batchQueries = 0;
let batch!: NotificationClickPump;
batch = new NotificationClickPump({
  ...deps,
  pending: async () => { batchQueries++; assert.ok(batchQueue.length <= 32); return [...batchQueue]; },
  acknowledge: async token => {
    assert.equal(batchQueue.shift()?.token, token);
    batchAcknowledged.push(token);
    if (batchAcknowledged.length === 1) {
      batchQueue.push({ ...click, token: "batch-refill" });
      await batch.wake();
    }
  },
});
await batch.wake();
await ticks();
assert.equal(batchAcknowledged.length, 33, "a click refilling the bounded native queue is consumed after the first batch");
assert.equal(opens - batchOpensBefore, 33, "each refilled click is opened before acknowledgement");
assert.equal(new Set(batchAcknowledged).size, 33, "each click is acknowledged once");
assert.equal(batchQueue.length, 0);
assert.equal(batchQueries, 34, "the continued batch stops after one empty query");
console.log("Tauri notification click lifecycle passed");
