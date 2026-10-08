import type { BridgeRemoteControllerRequest, BridgeRemoteControllerView, BridgeRemoteControllerSession, BridgeRemoteControllerResponse, BridgeRemoteControllerSessionsResponse, BridgeRemoteControllerCloseResponse } from "./bridgeProtocol.generated";

export interface RemoteControllerAPI {
  attach(request: BridgeRemoteControllerRequest): Promise<BridgeRemoteControllerResponse>;
  sessions(id: string): Promise<BridgeRemoteControllerSessionsResponse>;
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
  release(): void;
}
const FAILED = "remote controller connection changed; reconnect the host and reopen the workspace";

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
    // A mount can unmount before consuming ready. Rejections still propagate
    // to its consumer but must not become detached/unhandled promise errors.
    void ready.catch(() => {});
    return {
      ready,
      sessions:async () => {
        const view = await ready;
        const response = await this.api.sessions(view.id);
        if (released || owner.retired || response.protocolVersion !== 1 || response.controller.id !== view.id || response.controller.name !== view.name || response.controller.workspace !== view.workspace || response.controller.readOnly !== true) throw new Error(FAILED);
        return response.sessions;
      },
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
