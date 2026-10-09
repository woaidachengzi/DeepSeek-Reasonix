import type { BridgeRemoteControllerSessionEvent, BridgeRemoteControllerView } from "./bridgeProtocol.generated";

export interface RemoteSubscriptionRequest {
  controllerId: string;
  sessionPath: string;
  surfaceId: string;
  generation: number;
}
export interface RemoteSubscriptionIdentity extends RemoteSubscriptionRequest {
  protocolVersion: number;
  subscriptionId: string;
  sidecarInstanceId: string;
}
export interface RemoteSubscriptionTransport {
  listen(name: string, callback: (payload: unknown) => void): Promise<() => void | Promise<void>>;
  subscribe(request: RemoteSubscriptionRequest): Promise<unknown>;
  unsubscribe(subscriptionId: string): Promise<void>;
}
export interface RemoteSubscriptionSink {
  state(state: "opening" | "ready" | "ended" | "failed"): void;
  event(frame: BridgeRemoteControllerSessionEvent): void;
}
const MAX_PENDING_BYTES = 9 * 1024 * 1024;
const encoder = new TextEncoder();
const record = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const clean = (value: unknown, limit: number): value is string => typeof value === "string" && value.length > 0 && encoder.encode(value).length <= limit && !/\p{Cc}/u.test(value);
const handle = (value: unknown): value is string => typeof value === "string" && /^[A-Za-z0-9_-]{21}[AQgw]$/.test(value);

// Native enqueue may beat the invoke receipt. Listen first and retain only a
// bounded queue; no callback receives an event before its exact receipt and
// ready notice. This is not a snapshot/replay projector or a write grant.
export function openRemoteSessionSubscription(transport: RemoteSubscriptionTransport, selectedController: BridgeRemoteControllerView, input: RemoteSubscriptionRequest, sink: RemoteSubscriptionSink) {
  const request = {controllerId:input.controllerId,sessionPath:input.sessionPath,surfaceId:input.surfaceId,generation:input.generation};
  const controller = {...selectedController};
  let disposed = false;
  let identity: RemoteSubscriptionIdentity | null = null;
  let phase: "opening" | "ready" | "ended" = "opening";
  let pending: {name:string;payload:Record<string,unknown>}[] = [];
  let bytes = 0;
  const listeners: (() => void | Promise<void>)[] = [];
  const releaseListener = (unlisten: () => void | Promise<void>) => {
    try {
      // Tauri unlisten returns a promise even though its SDK type says void.
      // A closing WebView can reject it after the synchronous call returns.
      void Promise.resolve(unlisten()).catch(() => {});
    } catch { /* Release the remaining listeners too. */ }
  };
  const closed = new Set<string>();
  const close = (id: string) => {
    if (closed.has(id)) return;
    closed.add(id);
    try {
      void Promise.resolve(transport.unsubscribe(id)).catch(() => {});
    } catch { /* Never retry/reopen an uncertain close. */ }
  };
  const dispose = () => {
    if (disposed) return;
    disposed = true; pending = []; bytes = 0;
    for (const unlisten of listeners.splice(0)) releaseListener(unlisten);
    if (identity) close(identity.subscriptionId);
  };
  const fail = () => {
    if (disposed) return;
    dispose(); sink.state("failed");
  };
  const matches = (value: unknown): value is RemoteSubscriptionIdentity => record(value)
    && value.protocolVersion === 1 && handle(value.subscriptionId) && clean(value.sidecarInstanceId,4096)
    && value.controllerId === request.controllerId && value.sessionPath === request.sessionPath
    && value.surfaceId === request.surfaceId && value.generation === request.generation;
  const deliver = (name: string, payload: Record<string,unknown>) => {
    if (disposed || !identity || !matches(payload.subscription)
      || payload.subscription.subscriptionId !== identity.subscriptionId
      || payload.subscription.sidecarInstanceId !== identity.sidecarInstanceId) return;
    if (name === "bridge:remote-session-state") {
      const state = payload.state;
      if (state === "opening" && phase === "opening") sink.state("opening");
      else if (state === "ready" && phase === "opening") { phase = "ready"; sink.state("ready"); }
      else if (state === "ended") { phase = "ended"; dispose(); sink.state("ended"); }
      else fail();
      return;
    }
    const frame = payload.frame;
    if (phase !== "ready" || !record(frame) || frame.protocolVersion !== 1 || frame.sessionPath !== request.sessionPath
      || !record(frame.controller) || frame.controller.id !== controller.id || frame.controller.name !== controller.name
      || frame.controller.workspace !== controller.workspace || frame.controller.readOnly !== true
      || !record(frame.event) || frame.event.sessionPath !== request.sessionPath || !clean(frame.event.kind,128)) { fail(); return; }
    // Nested payload projection belongs to the generated native contract;
    // this boundary checks owner and scope, and grants no local capabilities.
    sink.event(frame as unknown as BridgeRemoteControllerSessionEvent);
  };
  const receive = (name: string, payload: unknown) => { try {
    if (disposed || !record(payload) || payload.protocolVersion !== 1 || !matches(payload.subscription)) return;
    if (identity) { deliver(name,payload); return; }
    bytes += encoder.encode(JSON.stringify(payload)).length;
    if (pending.length >= 256 || bytes > MAX_PENDING_BYTES) { fail(); return; }
    pending.push({name,payload});
    } catch { fail(); }
  };
  const receipt = (async () => {
    try {
      if (!handle(request.controllerId) || controller.id !== request.controllerId || controller.readOnly !== true
        || !clean(request.sessionPath,32768) || !clean(request.surfaceId,128) || !Number.isSafeInteger(request.generation) || request.generation < 1) throw new Error("invalid scope");
      for (const name of ["bridge:remote-session-state","bridge:remote-session-event"]) {
        const unlisten = await transport.listen(name,payload => receive(name,payload));
        if (disposed) { releaseListener(unlisten); return null; }
        listeners.push(unlisten);
      }
      if (disposed) return null;
      const result = await transport.subscribe({...request});
      if (!matches(result)) throw new Error("invalid receipt");
      identity = {...request,protocolVersion:1,subscriptionId:result.subscriptionId,sidecarInstanceId:result.sidecarInstanceId};
      if (disposed) { close(identity.subscriptionId); return null; }
      const queue = pending; pending = []; bytes = 0;
      for (const item of queue) deliver(item.name,item.payload);
      return disposed ? null : {...identity};
    } catch { fail(); return null; }
  })();
  return {receipt,dispose};
}
