import type { BridgeRemoteControllerRequest, BridgeRemoteControllerView, BridgeRemoteControllerSession, BridgeRemoteControllerResponse, BridgeRemoteControllerSessionsResponse, BridgeRemoteControllerCloseResponse, BridgeRemoteControllerSessionView, BridgeRemoteControllerSessionViewResponse } from "./bridgeProtocol.generated";
import type { BridgeRemoteControllerImage, BridgeRemoteControllerSessionImageResponse } from "./bridgeProtocol.generated";
import type { BridgeRemoteControllerSessionCancelRequest, BridgeRemoteControllerSessionCancelReceipt, BridgeRemoteControllerSessionCancelResponse } from "./bridgeProtocol.generated";
import type { BridgeRemoteControllerSessionSubmitRequest, BridgeRemoteControllerSessionSubmitReceipt, BridgeRemoteControllerSessionSubmitResponse } from "./bridgeProtocol.generated";

export interface RemoteControllerAPI {
  attach(request: BridgeRemoteControllerRequest): Promise<BridgeRemoteControllerResponse>;
  sessions(id: string): Promise<BridgeRemoteControllerSessionsResponse>;
  sessionView(id: string, sessionPath: string): Promise<BridgeRemoteControllerSessionViewResponse>;
  sessionImage(id: string, sessionPath: string, source: string): Promise<BridgeRemoteControllerSessionImageResponse>;
  sessionCancel?(id:string, scope:BridgeRemoteControllerSessionCancelRequest):Promise<BridgeRemoteControllerSessionCancelResponse>;
  sessionSubmit?(id:string, input:BridgeRemoteControllerSessionSubmitRequest):Promise<BridgeRemoteControllerSessionSubmitResponse>;
  close(id: string): Promise<BridgeRemoteControllerCloseResponse>;
}
interface Entry {
  refs: number;
  retired: boolean;
  barrier: Promise<void>;
  ready: Promise<BridgeRemoteControllerView>;
  closing?: Promise<void>;
}
export interface RemoteControllerLease {
  ready: Promise<BridgeRemoteControllerView>;
  sessions(): Promise<BridgeRemoteControllerSession[]>;
  sessionView(sessionPath: string): Promise<BridgeRemoteControllerSessionView>;
  sessionImage(sessionPath: string, source: string): Promise<BridgeRemoteControllerImage>;
  sessionCancel?(scope:BridgeRemoteControllerSessionCancelRequest):Promise<BridgeRemoteControllerSessionCancelReceipt>;
  sessionSubmit?(input:BridgeRemoteControllerSessionSubmitRequest):Promise<BridgeRemoteControllerSessionSubmitReceipt>;
  release(): void;
}
const FAILED = "remote controller connection changed; reconnect the host and reopen the workspace";
export const REMOTE_STOP_UNKNOWN = "remote stop outcome is unknown; refresh the selected session and do not automatically retry";
export const REMOTE_SEND_UNKNOWN = "remote send outcome is unknown; refresh the selected session and do not automatically retry";
const encoder = new TextEncoder();
function identifier(value: string, limit: number): boolean {
  return typeof value === "string" && value.length > 0 && encoder.encode(value).length <= limit && !/\p{Cc}/u.test(value);
}

// One owner pool for all React mounts. A late attach must close before the next
// same-scope attach can reuse its backend handle; an old finally cannot delete
// a newer entry. No local RuntimeManager or renderer URL is involved.
export class RemoteControllerPool {
  private entries = new Map<string, Entry>();
  constructor(private api: RemoteControllerAPI) {}
  acquire(name: string, workspace: string): RemoteControllerLease {
    const key = JSON.stringify([name,workspace]);
    let entry = this.entries.get(key);
    if (!entry || entry.closing) {
      const predecessor = entry?.closing ?? Promise.resolve();
      let next: Entry;
      const opening = predecessor.then(async () => {
        if (next.retired || !next.refs) throw new Error(FAILED);
        const response = await this.api.attach({ name,workspace });
        const view = response.controller;
        if (response.protocolVersion !== 1 || view.name !== name || !view.id || view.readOnly !== true) throw new Error(FAILED);
        return view;
      });
      next = {refs:0,retired:false,barrier:predecessor,ready:opening};
      this.entries.set(key,next); entry = next;
    }
    const owner = entry;
    ++owner.refs;
    let released = false;
    const ready = owner.ready.then(view => { if (released || owner.retired) throw new Error(FAILED); return view; });
    const assertLive = () => { if (released || owner.retired) throw new Error(FAILED); };
    const assertController = (actual: BridgeRemoteControllerView, expected: BridgeRemoteControllerView) => {
      assertLive();
      if (actual.id !== expected.id || actual.name !== expected.name || actual.workspace !== expected.workspace || actual.readOnly !== true) throw new Error(FAILED);
    };
    // A mount can unmount before consuming ready. Rejections still propagate
    // to its consumer but must not become detached/unhandled promise errors.
    void ready.catch(() => {});
    return {
      ready,
      sessions:async () => {
        const view = await ready;
        assertLive();
        const response = await this.api.sessions(view.id);
        assertController(response.controller,view);
        if (response.protocolVersion !== 1) throw new Error(FAILED);
        return response.sessions;
      },
      sessionView:async sessionPath => {
        if (!identifier(sessionPath,32768)) throw new Error(FAILED);
        const view = await ready;
        assertLive();
        const response = await this.api.sessionView(view.id,sessionPath);
        assertController(response.controller,view);
        const snapshot = response.view;
        if (response.protocolVersion !== 1 || snapshot.protocolVersion !== 1 || snapshot.sessionPath !== sessionPath || snapshot.readOnly !== true || !Array.isArray(snapshot.history) || snapshot.history.length > 100000) throw new Error(FAILED);
        const identities = new Set<string>();
        for (const message of snapshot.history) {
          if (!identifier(message.id,4096) || identities.has(message.id)) throw new Error(FAILED);
          identities.add(message.id);
        }
        return snapshot;
      },
      sessionImage:async (sessionPath,source) => {
        if (!identifier(sessionPath,32768) || typeof source !== "string" || !source.trim() || encoder.encode(source).length > 22370645 || source.includes("\0")) throw new Error(FAILED);
        const view = await ready;
        assertLive();
        const response = await this.api.sessionImage(view.id,sessionPath,source);
        assertController(response.controller,view);
        const image = response.view.image;
        if (response.protocolVersion !== 1 || response.view.protocolVersion !== 1 || response.view.sessionPath !== sessionPath || response.view.workspace !== view.workspace
          || typeof image.url !== "string" || image.url.length > 11184900 || image.filename !== undefined && (typeof image.filename !== "string" || encoder.encode(image.filename).length > 4096 || /\p{Cc}/u.test(image.filename))
          || image.size !== undefined && (!Number.isSafeInteger(image.size) || image.size < 0 || image.size > 16777216)) throw new Error(FAILED);
        if (image.errorCode !== undefined) {
          if (image.url || image.mime !== undefined || image.filename !== undefined || image.size !== undefined || !["blocked-remote","proxy-config","fetch-failed","not-found","forbidden","not-a-file","too-large","changed-file","invalid-image","unsupported-type"].includes(image.errorCode)) throw new Error(FAILED);
          return {url:"",errorCode:image.errorCode};
        }
        if (image.mime !== "image/png" || !/^data:image\/png;base64,[A-Za-z0-9+/]+={0,2}$/.test(image.url)) throw new Error(FAILED);
        // Explicit projection, even for test/alternate native adapters: an
        // unexpected remote opener or private field cannot become a UI grant.
        return {url:image.url,filename:image.filename,mime:image.mime,size:image.size};
      },
      ...(this.api.sessionCancel ? {sessionCancel:async (input:BridgeRemoteControllerSessionCancelRequest) => {
        const scope={sessionPath:input.sessionPath,runtimeEpoch:input.runtimeEpoch,turnId:input.turnId};
        if(!identifier(scope.sessionPath,32768)||!identifier(scope.runtimeEpoch,4096)||!identifier(scope.turnId,4096))throw new Error(FAILED);
        const view=await ready;assertLive();
        try {
          const response=await this.api.sessionCancel!(view.id,scope);
          assertController(response.controller,view);
          const receipt=response.receipt;
          if(response.protocolVersion!==1||receipt.protocolVersion!==1||receipt.sessionPath!==scope.sessionPath||receipt.runtimeEpoch!==scope.runtimeEpoch||receipt.turnId!==scope.turnId||receipt.cancelled!==true)throw new Error(REMOTE_STOP_UNKNOWN);
          return {protocolVersion:1,sessionPath:scope.sessionPath,runtimeEpoch:scope.runtimeEpoch,turnId:scope.turnId,cancelled:true};
        } catch {throw new Error(REMOTE_STOP_UNKNOWN);}
      }} : {}),
      ...(this.api.sessionSubmit ? {sessionSubmit:async (input:BridgeRemoteControllerSessionSubmitRequest) => {
        const message={sessionPath:input.sessionPath,runtimeEpoch:input.runtimeEpoch,revision:input.revision,text:input.text};
        if(!identifier(message.sessionPath,32768)||!identifier(message.runtimeEpoch,4096)||!Number.isSafeInteger(message.revision)||message.revision<1||typeof message.text!=="string"||!message.text.trim()||encoder.encode(message.text).length>524288||message.text.includes("\0"))throw new Error(FAILED);
        const view=await ready;assertLive();
        try {
          const response=await this.api.sessionSubmit!(view.id,message);
          assertController(response.controller,view);
          const receipt=response.receipt;
          if(response.protocolVersion!==1||receipt.protocolVersion!==1||receipt.sessionPath!==message.sessionPath||receipt.runtimeEpoch!==message.runtimeEpoch||receipt.revision!==message.revision||receipt.accepted!==true)throw new Error(REMOTE_SEND_UNKNOWN);
          return {protocolVersion:1,sessionPath:message.sessionPath,runtimeEpoch:message.runtimeEpoch,revision:message.revision,accepted:true};
        } catch {throw new Error(REMOTE_SEND_UNKNOWN);}
      }} : {}),
      release:() => {
        if (released) return;
        released = true;
        if (--owner.refs !== 0 || owner.closing) return;
        // An attach error has no handle to close. A close error is different:
        // retain its rejected barrier until explicit SSH reset, avoiding ABA
        // reuse after an uncertain native HTTP timeout.
        owner.closing = owner.barrier.then(() => owner.ready.then(view => this.api.close(view.id).then(response => { if (response.protocolVersion !== 1 || !response.closed) throw new Error(FAILED); }),() => {}));
        void owner.closing.then(() => { if (this.entries.get(key) === owner) this.entries.delete(key); },() => {});
      },
    };
  }
  invalidateHost(name: string): void {
    for (const [key,entry] of this.entries) {
      if (JSON.parse(key)[0] !== name) continue;
      entry.retired = true;
      // Only called after an explicit SSH operation revoked backend owners.
      // In-flight old closes target old random IDs, never new connections.
      this.entries.delete(key);
    }
  }
}
