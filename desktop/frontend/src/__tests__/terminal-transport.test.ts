import assert from "node:assert/strict";
import { TerminalTransport, terminalBase64, terminalBytes } from "../lib/terminalTransport";

const id = "a".repeat(32);
const second = "b".repeat(32);
const tick = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); };
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: unknown) => void; const promise = new Promise<T>((ok, fail) => { resolve = ok; reject = fail; }); return { promise, resolve, reject }; }
const encode = (value: string) => terminalBase64(new TextEncoder().encode(value));
const snapshot = (text = "", start = 0, terminal = id) => ({ protocolVersion: 1, output: { id: terminal, start, end: start + new TextEncoder().encode(text).length, data: encode(text) } });
const session = (terminal = id) => ({ id: terminal, title: "Shell", cwd: "/owned-test", shell: "/bin/sh", createdAt: 1, running: true });
const workspace = (ids = [id]) => ({ protocolVersion: 1, workspace: { available: true, readOnly: false, sessions: ids.map(session), shells: [{ id: "sh", label: "sh" }] } });
function fixture() {
  const calls: { command: string; request: Record<string, unknown> }[] = [];
  let handler: (command: string, request: Record<string, unknown>) => Promise<unknown> = async command => command === "bridge_terminal_workspace" ? workspace() : snapshot();
  let frame!: Parameters<ConstructorParameters<typeof TerminalTransport>[1]>[0];
  let connection!: Parameters<ConstructorParameters<typeof TerminalTransport>[1]>[1];
  let disposed = 0;
  let counter = 0;
  const transport = new TerminalTransport(async <T>(command: string, request: Record<string, unknown>) => {
    calls.push({ command, request }); return await handler(command, request) as T;
  }, async (cb, restore) => { frame = cb; connection = restore; return () => { disposed++; }; }, () => `owned-${++counter}`);
  const received: { id: string; bytes: number[]; reset?: boolean }[] = [];
  const errors: Error[] = [];
  transport.onOutput(value => received.push({ id: value.id, bytes: [...terminalBytes(value.data)], reset: value.reset }));
  transport.onError(error => errors.push(error));
  const release = transport.retain("first");
  return { transport, calls, received, errors, release, setHandler(value: typeof handler) { handler = value; }, send(text: string, start: number, terminal = id, owner = "first") {
    frame({ protocolVersion: 1, sessionId: owner, eventKind: "terminal_output", payload: snapshot(text, start, terminal).output });
  }, frame: (value: Parameters<typeof frame>[0]) => frame(value), connection: (connected?: boolean) => connection(connected), disposed: () => disposed };
}

{
  const f = fixture();
  await f.transport.workspace("first");
  const gate = deferred<unknown>();
  f.setHandler(async command => command === "bridge_terminal_input" ? gate.promise : {});
  const first = f.transport.write("first", id, "你");
  const next = f.transport.write("first", id, "\r");
  await tick();
  assert.equal(f.calls.filter(call => call.command === "bridge_terminal_input").length, 1, "inputs never overlap native workers");
  gate.resolve({}); await first; await next;
  const writes = f.calls.filter(call => call.command === "bridge_terminal_input");
  assert.deepEqual(writes.map(call => new TextDecoder().decode(terminalBytes(String(call.request.data)))), ["你", "\r"]);
  assert.notEqual(writes[0].request.requestId, writes[1].request.requestId);
  f.release();
}
{
  const f = fixture(); await f.transport.workspace("first");
  const gate = deferred<unknown>();
  f.setHandler(async () => gate.promise);
  const outcomes = [];
  for (let i = 0; i < 4; i++) outcomes.push(f.transport.write("first", id, "x".repeat(64 * 1024)).then(() => "ok", () => "cancelled"));
  await assert.rejects(f.transport.write("first", id, "x"), /队列已满/);
  await assert.rejects(f.transport.write("first", id, "你".repeat(22000)), /过长/);
  await tick(); f.release(); gate.resolve({});
  assert.deepEqual(await Promise.all(outcomes), ["cancelled", "cancelled", "cancelled", "cancelled"]);
  assert.equal(f.calls.filter(call => call.command === "bridge_terminal_input").length, 1, "owner release cancels unsent copied inputs");
}
{
  const f = fixture();
  f.setHandler(async command => command === "bridge_terminal_workspace" ? workspace() : snapshot("abc"));
  await f.transport.workspace("first");
  f.send("abc", 0); f.send("bcde", 1); f.send("foreign", 5, id, "other");
  assert.equal(new TextDecoder().decode(Uint8Array.from(f.received.flatMap(item => item.bytes))), "abcde", "replayed offsets are sliced, foreign owners ignored");
  const gate = deferred<unknown>(); f.setHandler(async () => gate.promise);
  f.send("i", 8); await tick(); f.send("jk", 9);
  gate.resolve(snapshot("abcdefghij")); await tick();
  assert.equal(new TextDecoder().decode(Uint8Array.from(f.received.flatMap(item => item.bytes))), "abcdefghijk", "snapshot merges buffered live tail without duplication");
  f.setHandler(async () => snapshot("tail", 100)); f.connection(); await tick();
  assert.equal(f.received[f.received.length - 1]?.reset, true, "a trimmed gap replaces xterm history");
  assert.equal(new TextDecoder().decode(Uint8Array.from(f.received[f.received.length - 1].bytes)), "tail");
  f.release();
}
{
  const f = fixture(); await f.transport.workspace("first");
  const gate = deferred<unknown>(); f.setHandler(async () => gate.promise);
  f.send("late", 10); await tick();
  const before = f.received.length;
  f.release(); const release = f.transport.retain("second");
  gate.resolve(snapshot("old snapshot")); await tick();
  assert.equal(f.received.length, before, "late snapshot cannot cross an owner generation");
  assert.equal(f.disposed(), 1); release(); await tick(); assert.equal(f.disposed(), 2);
}
{
  const f = fixture(); await f.transport.workspace("first");
  const gate = deferred<unknown>(); f.setHandler(async () => gate.promise);
  const first = f.transport.write("first", id, "first").catch(() => {});
  const queued = f.transport.write("first", id, "unsent").catch(() => {});
  await tick(); f.connection(false); gate.resolve({}); await first; await queued;
  assert.equal(f.calls.filter(call => call.command === "bridge_terminal_input").length, 1, "disconnect drops queued input instead of replaying commands");
  assert.throws(() => f.transport.write("first", id, "blocked"), /切换/);
  f.setHandler(async command => command === "bridge_terminal_workspace" ? workspace() : snapshot());
  f.connection(true); await tick(); await f.transport.workspace("first");
  await f.transport.write("first", id, "new explicit input");
  f.release();
}
{
  const f = fixture(); await f.transport.workspace("first");
  f.setHandler(async command => { if (command === "bridge_terminal_input") throw new Error("uncertain delivery"); return {}; });
  await assert.rejects(f.transport.write("first", id, "never blindly retry"), /uncertain/);
  await f.transport.mutate("first", id, "bridge_terminal_close");
  assert.equal(f.calls.filter(call => call.command === "bridge_terminal_close").length, 1, "failed input cannot prevent explicit close");
  const count = f.calls.length;
  f.send("late output after close", 0); await tick();
  assert.equal(f.calls.length, count, "closed terminal cannot be resurrected by a delayed output event");
  f.release();
}
{
  const f = fixture();
  const gate = deferred<unknown>();
  f.setHandler(async command => command === "bridge_terminal_create" ? gate.promise : snapshot("completed\r\n"));
  const create = f.transport.create("first", ".", "sh");
  await tick();
  f.frame({ protocolVersion: 1, sessionId: "first", eventKind: "terminal_exit", payload: { id, exitCode: -1, removed: false } });
  gate.resolve({ protocolVersion: 1, terminal: session() });
  assert.equal((await create).running, false, "exit before create response cannot publish a phantom running shell");
  f.release();
}
{
  const f = fixture(); await f.transport.workspace("first");
  const before = f.received.length;
  f.frame({ protocolVersion: 1, sessionId: "first", eventKind: "terminal_output", payload: { id, data: encode("bad"), start: 0, end: 1 } });
  assert.equal(f.received.length, before);
  assert.equal(f.errors.length, 1, "invalid raw byte length fails closed without painting");
  f.setHandler(async () => snapshot("wrong", 0, second));
  f.connection(); await tick();
  assert.equal(f.received.length, before, "foreign snapshot identity fails closed");
  assert.equal(f.errors.length, 2);
  f.release();
}
{
  const ready = deferred<() => void>(); let disposed = 0; const errors: unknown[] = [];
  const client = new TerminalTransport(async <T>() => workspace() as T, async () => ready.promise);
  client.onError(error => errors.push(error));
  const release = client.retain("first"); release(); ready.resolve(() => { disposed++; }); await tick();
  assert.equal(disposed, 1, "late native subscription is disposed after unmount");
  assert.equal(errors.length, 0);
}
console.log("Terminal transport FIFO, budgets, offset recovery, disconnection and ownership tests passed");
