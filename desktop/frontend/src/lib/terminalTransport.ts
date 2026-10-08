import type { BridgeTerminalOutputResponse, BridgeTerminalOutputView, BridgeTerminalSessionResponse, BridgeTerminalWorkspaceResponse } from "./bridgeProtocol.generated";
import type { TerminalExitEvent, TerminalOutputEvent } from "./bridge";

type Invoke = <T>(command: string, request: Record<string, unknown>) => Promise<T>;
type Frame = { protocolVersion: number; sessionId: string; eventKind: string; payload: unknown };
type OutputState = { end: number | null; pending: BridgeTerminalOutputView[]; recovering?: Promise<void> };
type Owner = { id: string; live: boolean; connected: boolean; outputs: Map<string, OutputState>; exits: Map<string, TerminalExitEvent>; queues: Map<string, Queue>; ready: Promise<void>; off?: () => void };
type Job = { run: () => Promise<void>; resolve: () => void; reject: (error: unknown) => void; bytes: number };
type Queue = { jobs: Job[]; bytes: number; running: boolean; closed: boolean };
const stale = () => new Error("终端所属会话已切换，请在当前会话重新打开终端。");
const busy = () => new Error("终端输入队列已满，请等待正在发送的输入完成后重试。");

export function terminalBytes(data: string): Uint8Array {
  const value = atob(data);
  return Uint8Array.from(value, char => char.charCodeAt(0));
}
export function terminalBase64(bytes: Uint8Array): string {
  // Do not spread a 128 KiB snapshot into the JS argument stack.
  let value = "";
  for (const byte of bytes) value += String.fromCharCode(byte);
  return btoa(value);
}
function output(value: BridgeTerminalOutputView, limit = 128 * 1024): Uint8Array {
  if (!value || !/^[a-f0-9]{32}$/.test(value.id) || !Number.isSafeInteger(value.start) || !Number.isSafeInteger(value.end) || value.start < 0 || value.end < value.start || typeof value.data !== "string" || value.data.length > Math.ceil(limit / 3) * 4) throw new Error("终端输出格式错误，请重新加载终端。");
  const bytes = terminalBytes(value.data);
  if (bytes.length > limit || bytes.length !== value.end - value.start) throw new Error("终端输出长度错误，请重新加载终端。");
  return bytes;
}

/** One renderer owner; raw byte offsets never enter the Agent transcript. */
export class TerminalTransport {
  private owner?: Owner;
  private outputs = new Set<(event: TerminalOutputEvent) => void>();
  private exits = new Set<(event: TerminalExitEvent) => void>();
  private errors = new Set<(error: Error) => void>();
  private refreshes = new Set<() => void>();
  constructor(private invoke: Invoke, private listen: (frame: (value: Frame) => void, reconnect: (connected?: boolean) => void) => Promise<() => void>, private requestId: () => string = () => crypto.randomUUID()) {}
  onOutput(cb: (event: TerminalOutputEvent) => void) { this.outputs.add(cb); return () => { this.outputs.delete(cb); }; }
  onExit(cb: (event: TerminalExitEvent) => void) { this.exits.add(cb); return () => { this.exits.delete(cb); }; }
  onError(cb: (error: Error) => void) { this.errors.add(cb); return () => { this.errors.delete(cb); }; }
  onRefresh(cb: () => void) { this.refreshes.add(cb); return () => { this.refreshes.delete(cb); }; }
  retain(id: string): () => void {
    this.dispose();
    const owner: Owner = { id, live: true, connected: true, outputs: new Map(), exits: new Map(), queues: new Map(), ready: Promise.resolve() };
    this.owner = owner;
    owner.ready = this.listen(value => { if (this.current(owner)) this.frame(owner, value); }, (connected = true) => {
      if (!this.current(owner)) return;
      owner.connected = connected;
      if (!connected) {
        for (const queue of owner.queues.values()) this.drop(queue, stale());
        this.report(owner, new Error("终端连接已断开，已取消待发送输入；请等待连接恢复后重试。"));
        return;
      }
      // Coalesced by snapshot() per terminal. Metadata refresh does not replace
      // the painted xterm or select a different terminal.
      for (const terminal of owner.outputs.keys()) void this.snapshot(owner, terminal).catch(error => this.report(owner, error));
      this.refreshes.forEach(cb => cb());
    }).then(off => { if (this.current(owner)) owner.off = off; else off(); });
    // Async listener rejection must be observable by commands, but never become
    // an unhandled rejection if the dock unmounts before issuing a command.
    void owner.ready.catch(error => this.report(owner, error));
    return () => { if (this.owner === owner) this.dispose(); };
  }
  private current(owner: Owner) { return owner.live && this.owner === owner; }
  private check(owner: Owner) { if (!this.current(owner) || !owner.connected) throw stale(); }
  private capture(id: string) { const owner = this.owner; if (!owner || owner.id !== id || !owner.live) throw stale(); return owner; }
  private report(owner: Owner, value: unknown) { if (this.current(owner)) this.errors.forEach(cb => cb(value instanceof Error ? value : new Error(String(value)))); }
  private drop(queue: Queue, error: unknown) { queue.closed = true; for (const job of queue.jobs.splice(0)) { queue.bytes -= job.bytes; job.reject(error); } }
  private dispose() {
    const owner = this.owner;
    if (!owner) return;
    owner.live = false;
    owner.off?.();
    for (const queue of owner.queues.values()) this.drop(queue, stale());
    for (const id of owner.outputs.keys()) this.exits.forEach(cb => cb({ id, removed: true, exitCode: -1 }));
    this.owner = undefined;
  }
  private state(owner: Owner, id: string) {
    let state = owner.outputs.get(id);
    if (!state) {
      if (owner.outputs.size >= 10) throw busy();
      state = { end: null, pending: [] };
      owner.outputs.set(id, state);
    }
    return state;
  }
  private emit(id: string, bytes: Uint8Array, reset = false) { this.outputs.forEach(cb => cb({ id, data: terminalBase64(bytes), reset })); }
  private accept(state: OutputState, value: BridgeTerminalOutputView): boolean {
    const bytes = output(value, 8192);
    if (state.end === null || value.start > state.end) return false;
    if (value.end <= state.end) return true; // reconnect/replay duplicate
    this.emit(value.id, bytes.subarray(state.end - value.start));
    state.end = value.end;
    return true;
  }
  private frame(owner: Owner, frame: Frame) {
    if (frame.protocolVersion !== 1 || frame.sessionId !== owner.id) return;
    try {
      if (frame.eventKind === "terminal_output") {
        const value = frame.payload as BridgeTerminalOutputView;
        output(value, 8192);
        if (owner.exits.get(value.id)?.removed) return;
        if (owner.queues.get(value.id)?.closed) return;
        const state = this.state(owner, value.id);
        if (!state.recovering && this.accept(state, value)) return;
        state.pending.push(value);
        // Keep at most 128 KiB of staging, including frames arriving while a
        // snapshot is in flight. A later snapshot repairs the trimmed prefix.
        while (state.pending.length > 16) state.pending.shift();
        void this.snapshot(owner, value.id).catch(error => this.report(owner, error));
      } else if (frame.eventKind === "terminal_exit") {
        const event = frame.payload as TerminalExitEvent;
        if (!event || !/^[a-f0-9]{32}$/.test(event.id) || !Number.isInteger(event.exitCode) || event.exitCode < -2147483648 || event.exitCode > 4294967295 || typeof event.removed !== "boolean") return;
        const queue = owner.queues.get(event.id);
        if (queue) this.drop(queue, stale());
        owner.exits.set(event.id, event);
        while (owner.exits.size > 10) owner.exits.delete(owner.exits.keys().next().value!);
        this.exits.forEach(cb => cb(event));
        if (event.removed) owner.outputs.delete(event.id);
      }
    } catch (error) { this.report(owner, error); }
  }
  private snapshot(owner: Owner, id: string): Promise<void> {
    const state = this.state(owner, id);
    if (state.recovering) return state.recovering;
    const request = (async () => {
      await owner.ready;
      this.check(owner);
      // One bounded retry covers an event beyond the first snapshot response.
      // Never poll unchanged state indefinitely on an unavailable bridge.
      for (let attempt = 0; attempt < 2; attempt++) {
        const result = await this.invoke<BridgeTerminalOutputResponse>("bridge_terminal_output", { sessionId: owner.id, terminalId: id });
        this.check(owner);
        if (owner.outputs.get(id) !== state) throw stale();
        if (result.protocolVersion !== 1 || result.output.id !== id) throw new Error("终端快照不匹配，请重新加载终端。");
        const bytes = output(result.output);
        if (state.end === null || result.output.start > state.end) this.emit(id, bytes, true);
        else if (result.output.end > state.end) this.emit(id, bytes.subarray(state.end - result.output.start));
        state.end = Math.max(state.end ?? 0, result.output.end);
        const pending = state.pending.splice(0).sort((a, b) => a.start - b.start);
        for (const value of pending) if (!this.accept(state, value)) state.pending.push(value);
        if (state.pending.length === 0) return;
      }
      throw new Error("终端输出存在缺口，请重新加载终端以恢复最近输出。");
    })();
    state.recovering = request;
    void request.finally(() => { if (state.recovering === request) state.recovering = undefined; }).catch(() => {});
    return request;
  }
  async workspace(id: string) {
    const owner = this.capture(id);
    await owner.ready;
    this.check(owner);
    const result = await this.invoke<BridgeTerminalWorkspaceResponse>("bridge_terminal_workspace", { sessionId: id });
    this.check(owner);
    if (result.protocolVersion !== 1 || result.workspace.sessions.length > 10) throw new Error("终端列表不匹配，请重新加载终端。");
    const ids = new Set(result.workspace.sessions.map(item => item.id));
    for (const terminal of owner.outputs.keys()) if (!ids.has(terminal)) {
      owner.outputs.delete(terminal);
      const queue = owner.queues.get(terminal);
      if (queue) this.drop(queue, stale());
      owner.queues.delete(terminal);
      this.exits.forEach(cb => cb({ id: terminal, exitCode: -1, removed: true }));
    }
    await Promise.all(result.workspace.sessions.map(item => this.snapshot(owner, item.id)));
    this.check(owner);
    for (const item of result.workspace.sessions) {
      const queue = owner.queues.get(item.id);
      if (item.running && !owner.exits.has(item.id) && queue?.closed && !queue.running && !queue.jobs.length) owner.queues.delete(item.id);
    }
    return { ...result.workspace, sessions: result.workspace.sessions.filter(item => !owner.exits.get(item.id)?.removed)
      .map(item => { const exit = owner.exits.get(item.id); return exit ? { ...item, running: false, exitCode: exit.exitCode } : item; }) };
  }
  async create(id: string, path: string, shellId: string) {
    const owner = this.capture(id);
    await owner.ready;
    this.check(owner);
    const result = await this.invoke<BridgeTerminalSessionResponse>("bridge_terminal_create", { sessionId: id, requestId: this.requestId(), path, shellId });
    this.check(owner);
    if (result.protocolVersion !== 1) throw new Error("终端响应不匹配，请重新加载终端。");
    // The shell may print its prompt before the create response reaches JS.
    await this.snapshot(owner, result.terminal.id);
    const exit = owner.exits.get(result.terminal.id);
    if (exit?.removed) throw stale();
    return exit ? { ...result.terminal, running: false, exitCode: exit.exitCode } : result.terminal;
  }
  async text(id: string, terminalId: string) {
    const owner = this.capture(id);
    await owner.ready;
    this.check(owner);
    const result = await this.invoke<BridgeTerminalOutputResponse>("bridge_terminal_output", { sessionId: id, terminalId });
    this.check(owner);
    if (result.protocolVersion !== 1 || result.output.id !== terminalId) throw stale();
    return new TextDecoder().decode(output(result.output));
  }
  mutate(id: string, terminalId: string, command: string, fields: Record<string, unknown> = {}, bytes = 0): Promise<void> {
    const owner = this.capture(id);
    this.check(owner);
    if (!owner.outputs.has(terminalId)) return Promise.reject(stale());
    if (owner.exits.has(terminalId) && command !== "bridge_terminal_close") return Promise.reject(stale());
    let queue = owner.queues.get(terminalId);
    if (!queue) { queue = { jobs: [], bytes: 0, running: false, closed: false }; owner.queues.set(terminalId, queue); }
    if (queue.closed && command !== "bridge_terminal_close") return Promise.reject(stale());
    if (queue.bytes + bytes > 256 * 1024 || queue.jobs.length >= 1024) return Promise.reject(busy());
    const request = { sessionId: id, terminalId, requestId: this.requestId(), ...fields };
    const owned = queue;
    if (command === "bridge_terminal_close") this.drop(owned, stale());
    return new Promise<void>((resolve, reject) => {
      owned.bytes += bytes;
      owned.jobs.push({ bytes, resolve, reject, run: async () => {
        await owner.ready;
        this.check(owner);
        await this.invoke(command, request);
        this.check(owner);
        if (command === "bridge_terminal_close") {
          owner.exits.set(terminalId, { id: terminalId, exitCode: -1, removed: true });
          while (owner.exits.size > 10) owner.exits.delete(owner.exits.keys().next().value!);
          owner.outputs.delete(terminalId);
          owner.queues.delete(terminalId);
          this.exits.forEach(cb => cb({ id: terminalId, exitCode: -1, removed: true }));
        }
      } });
      void this.drain(owner, owned);
    });
  }
  private async drain(owner: Owner, queue: Queue) {
    if (queue.running) return;
    queue.running = true;
    try {
      while (queue.jobs.length) {
        const job = queue.jobs.shift()!;
        try { this.check(owner); await job.run(); job.resolve(); }
        catch (error) { job.reject(error); this.drop(queue, error); }
        finally { queue.bytes -= job.bytes; }
      }
    } finally { queue.running = false; }
  }
  write(id: string, terminalId: string, data: string) {
    const bytes = new TextEncoder().encode(data);
    if (!bytes.length) return Promise.resolve();
    if (bytes.length > 64 * 1024) return Promise.reject(new Error("单次终端输入过长，请分段粘贴后重试。"));
    return this.mutate(id, terminalId, "bridge_terminal_input", { data: terminalBase64(bytes) }, bytes.length);
  }
}
