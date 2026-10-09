import assert from "node:assert/strict";
import { RemoteControllerPool, type RemoteControllerAPI } from "../lib/remoteControllerPool";
import type { BridgeRemoteControllerResponse, BridgeRemoteControllerCloseResponse, BridgeRemoteControllerSessionsResponse, BridgeRemoteControllerSessionViewResponse, BridgeRemoteControllerSessionImageResponse } from "../lib/bridgeProtocol.generated";
import type { BridgeRemoteControllerSessionCancelRequest,BridgeRemoteControllerSessionCancelResponse } from "../lib/bridgeProtocol.generated";
import type { BridgeRemoteControllerSessionSubmitRequest,BridgeRemoteControllerSessionSubmitResponse } from "../lib/bridgeProtocol.generated";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((ok, fail) => { resolve = ok; reject = fail; });
  return { promise, resolve, reject };
}
const tick = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };
const response = (id: string, workspace = "/owned", name = "owned"): BridgeRemoteControllerResponse => ({ protocolVersion: 1, controller: { id, name, workspace, readOnly: true } });
const selected = "/remote/中文 &+?#.jsonl";
const snapshotResponse = (id: string, workspace = "/owned"): BridgeRemoteControllerSessionViewResponse => ({
  ...response(id,workspace),
  view: { protocolVersion:1,sessionPath:selected,readOnly:true,ownership:"saved",current:false,modelRef:"",label:"",history:[{id:"backend-user",role:"user",content:"question"},{id:"backend-answer",role:"assistant",content:"answer"}] },
});
function fixture() {
  const attaches: { name: string; workspace: string; gate: ReturnType<typeof deferred<BridgeRemoteControllerResponse>> }[] = [];
  const closes: { id: string; gate: ReturnType<typeof deferred<BridgeRemoteControllerCloseResponse>> }[] = [];
  const reads: { id: string; gate: ReturnType<typeof deferred<BridgeRemoteControllerSessionsResponse>> }[] = [];
  const snapshots: { id: string; sessionPath: string; gate: ReturnType<typeof deferred<BridgeRemoteControllerSessionViewResponse>> }[] = [];
  const api: RemoteControllerAPI = {
    sessionSubmit:(id,input)=>{const gate=deferred<BridgeRemoteControllerSessionSubmitResponse>();sends.push({id,input,gate});return gate.promise;},
    sessionCancel:(id,scope)=>{const gate=deferred<BridgeRemoteControllerSessionCancelResponse>();stops.push({id,scope,gate});return gate.promise;},
    sessionImage: (id,sessionPath,source) => { const gate = deferred<BridgeRemoteControllerSessionImageResponse>(); images.push({id,sessionPath,source,gate}); return gate.promise; },
    attach: request => { const gate = deferred<BridgeRemoteControllerResponse>(); attaches.push({ ...request, gate }); return gate.promise; },
    close: id => { const gate = deferred<BridgeRemoteControllerCloseResponse>(); closes.push({ id, gate }); return gate.promise; },
    sessions: id => { const gate = deferred<BridgeRemoteControllerSessionsResponse>(); reads.push({ id, gate }); return gate.promise; },
    sessionView: (id,sessionPath) => { const gate = deferred<BridgeRemoteControllerSessionViewResponse>(); snapshots.push({ id,sessionPath,gate }); return gate.promise; },
  };
  const images: {id:string;sessionPath:string;source:string;gate:ReturnType<typeof deferred<BridgeRemoteControllerSessionImageResponse>>}[] = [];
  const stops:{id:string;scope:BridgeRemoteControllerSessionCancelRequest;gate:ReturnType<typeof deferred<BridgeRemoteControllerSessionCancelResponse>>}[]=[];
  const sends:{id:string;input:BridgeRemoteControllerSessionSubmitRequest;gate:ReturnType<typeof deferred<BridgeRemoteControllerSessionSubmitResponse>>}[]=[];
  return { pool: new RemoteControllerPool(api), attaches, closes, reads, snapshots, images,stops,sends };
}

{
  const f=fixture(),lease=f.pool.acquire("owned","/owned");await tick();f.attaches[0].gate.resolve(response("send-owner"));await lease.ready;
  const input={sessionPath:selected,runtimeEpoch:"instance",revision:7,text:"用户问题\n/new"};
  const pending=lease.sessionSubmit!(input);input.text="caller-mutated";input.revision=8;await tick();
  assert.deepEqual(f.sends[0].input,{sessionPath:selected,runtimeEpoch:"instance",revision:7,text:"用户问题\n/new"});
  const receipt={protocolVersion:1,sessionPath:selected,runtimeEpoch:"instance",revision:7,accepted:true};
  f.sends[0].gate.resolve({...response("send-owner"),receipt:{...receipt,private:"PRIVATE"} as typeof receipt});
  assert.deepEqual(await pending,receipt,"send receipt strips private extras");
  for(const bad of [{...input,revision:0},{...input,revision:Number.MAX_SAFE_INTEGER+1},{...input,text:" \n "},{...input,text:"bad\0text"},{...input,text:"x".repeat(524289)}])await assert.rejects(lease.sessionSubmit!(bad),/connection changed/);
  assert.equal(f.sends.length,1,"invalid message never dispatches");
  const late=lease.sessionSubmit!({...input,revision:7});await tick();lease.release();await tick();
  f.sends[1].gate.resolve({...response("send-owner"),receipt});
  await assert.rejects(late,/outcome is unknown/);
  await assert.rejects(lease.sessionSubmit!({...input,revision:7}),/connection changed/);
  assert.equal(f.sends.length,2,"release cannot dispatch or retry");
  f.closes[0].gate.resolve({protocolVersion:1,closed:true});await tick();
}

{
  const f=fixture(),lease=f.pool.acquire("owned","/owned");await tick();f.attaches[0].gate.resolve(response("cancel-owner"));await lease.ready;
  const scope={sessionPath:selected,runtimeEpoch:"instance",turnId:"turn"};
  const pending=lease.sessionCancel!(scope);scope.turnId="caller-mutated";await tick();
  assert.equal(f.stops[0].scope.turnId,"turn","scope is immutable across ready/dispatch");
  const receipt={protocolVersion:1,sessionPath:selected,runtimeEpoch:"instance",turnId:"turn",cancelled:true};
  f.stops[0].gate.resolve({...response("cancel-owner"),receipt:{...receipt,private:"PRIVATE"} as typeof receipt});
  assert.deepEqual(await pending,receipt,"receipt strips private extras");
  const late=lease.sessionCancel!({...scope,turnId:"turn"});await tick();lease.release();await tick();
  f.stops[1].gate.resolve({...response("cancel-owner"),receipt});
  await assert.rejects(late,/outcome is unknown/,"release after dispatch cannot publish success");
  await assert.rejects(lease.sessionCancel!({...scope,turnId:"turn"}),/connection changed/);
  assert.equal(f.stops.length,2,"released operation does not dispatch or retry");
  f.closes[0].gate.resolve({protocolVersion:1,closed:true});await tick();
}
{
  const f = fixture();
  const first = f.pool.acquire("owned", "/owned");
  const shared = f.pool.acquire("owned", "/owned");
  const other = f.pool.acquire("owned", "/other");
  await tick(); assert.equal(f.attaches.length, 2, "shared mounts deduplicate, workspaces stay independent");
  f.attaches[0].gate.resolve(response("first"));
  f.attaches[1].gate.resolve(response("other", "/other"));
  await Promise.all([first.ready, shared.ready, other.ready]);
  first.release(); await tick(); assert.equal(f.closes.length, 0, "one shared consumer cannot close another's handle");
  const lateRead = shared.sessions(); await tick();
  shared.release(); await tick(); assert.equal(f.closes[0].id, "first");
  f.reads[0].gate.resolve({ ...response("first"), sessions: [] });
  await assert.rejects(lateRead, /connection changed/, "late catalogue cannot publish after release");
  f.closes[0].gate.resolve({ protocolVersion: 1, closed: true });
  other.release(); await tick(); assert.equal(f.closes[1].id, "other");
  f.closes[1].gate.resolve({ protocolVersion: 1, closed: true }); await tick();
}
{
  const f = fixture();
  const old = f.pool.acquire("owned", "/owned"); await tick(); old.release();
  const next = f.pool.acquire("owned", "/owned"); await tick();
  assert.equal(f.attaches.length, 1, "reopen waits for old pending attach and close");
  f.attaches[0].gate.resolve(response("old")); await tick();
  await assert.rejects(old.ready); assert.equal(f.closes[0].id, "old");
  assert.equal(f.attaches.length, 1, "close completion owns the reuse barrier");
  f.closes[0].gate.resolve({ protocolVersion: 1, closed: true }); await tick();
  assert.equal(f.attaches.length, 2); f.attaches[1].gate.resolve(response("new")); await next.ready;
  const shared = f.pool.acquire("owned", "/owned"); await shared.ready;
  assert.equal(f.attaches.length, 2, "old close finally cannot delete the newer owner");
  next.release(); shared.release(); await tick();
  assert.equal(f.closes[1].id, "new"); f.closes[1].gate.resolve({ protocolVersion: 1, closed: true }); await tick();
}
{
  const f = fixture();
  const old = f.pool.acquire("owned", "/owned"); await tick();
  f.attaches[0].gate.resolve(response("old")); await old.ready; old.release(); await tick();
  f.closes[0].gate.reject(new Error("uncertain close delivery")); await tick();
  const blocked = f.pool.acquire("owned", "/owned"); await assert.rejects(blocked.ready, /uncertain close/);
  assert.equal(f.attaches.length, 1, "uncertain close must not blindly reuse a handle");
  blocked.release(); f.pool.invalidateHost("owned");
  const fresh = f.pool.acquire("owned", "/owned"); await tick();
  f.attaches[1].gate.resolve(response("fresh")); await fresh.ready;
  fresh.release(); await tick(); f.closes[1].gate.resolve({ protocolVersion: 1, closed: true }); await tick();
}
{
  const f = fixture();
  const old = f.pool.acquire("owned", "/owned"); await tick();
  f.pool.invalidateHost("owned"); const fresh = f.pool.acquire("owned", "/owned"); await tick();
  f.attaches[1].gate.resolve(response("new")); await fresh.ready;
  f.attaches[0].gate.resolve(response("revoked")); await assert.rejects(old.ready);
  old.release(); await tick(); assert.equal(f.closes[0].id, "revoked", "late release closes only the old random handle");
  f.closes[0].gate.resolve({ protocolVersion: 1, closed: true }); await tick();
  const shared = f.pool.acquire("owned", "/owned"); await shared.ready; assert.equal(f.attaches.length, 2);
  fresh.release(); shared.release(); await tick(); f.closes[1].gate.resolve({ protocolVersion: 1, closed: true }); await tick();
}
{
  const f = fixture();
  const lease = f.pool.acquire("owned", "requested-alias"); await tick();
  f.attaches[0].gate.resolve(response("snapshot", "/resolved")); await lease.ready;
  const read = lease.sessionView(selected); await tick();
  assert.deepEqual({id:f.snapshots[0].id,path:f.snapshots[0].sessionPath},{id:"snapshot",path:selected});
  const expected = snapshotResponse("snapshot","/resolved"); f.snapshots[0].gate.resolve(expected);
  assert.deepEqual(await read,expected.view,"snapshot preserves backend IDs and resolved owner identity");
  lease.release(); await tick(); f.closes[0].gate.resolve({protocolVersion:1,closed:true}); await tick();
  await assert.rejects(lease.sessionView(selected),/connection changed/);
  assert.equal(f.snapshots.length,1,"released lease dispatches zero new history reads");
}
{
  const f = fixture();
  const lease = f.pool.acquire("owned", "/owned"); const shared = f.pool.acquire("owned", "/owned"); await tick();
  f.attaches[0].gate.resolve(response("fenced")); await lease.ready;
  const notDispatched = lease.sessionView(selected); lease.release();
  await assert.rejects(notDispatched,/connection changed/);
  assert.equal(f.snapshots.length,0,"release between ready and dispatch owns the request fence");
  const late = shared.sessionView(selected); await tick(); shared.release(); await tick();
  f.snapshots[0].gate.resolve(snapshotResponse("fenced"));
  await assert.rejects(late,/connection changed/,"late history cannot publish after consumer release");
  f.closes[0].gate.resolve({protocolVersion:1,closed:true}); await tick();
}
{
  const f = fixture();
  const old = f.pool.acquire("owned", "/owned"); await tick(); f.attaches[0].gate.resolve(response("old-view")); await old.ready;
  const late = old.sessionView(selected); await tick(); f.pool.invalidateHost("owned");
  const fresh = f.pool.acquire("owned", "/owned"); await tick(); f.attaches[1].gate.resolve(response("new-view")); await fresh.ready;
  f.snapshots[0].gate.resolve(snapshotResponse("old-view")); await assert.rejects(late,/connection changed/);
  const current = fresh.sessionView(selected); await tick(); f.snapshots[1].gate.resolve(snapshotResponse("new-view")); await current;
  old.release(); fresh.release(); await tick();
  for (const close of f.closes) close.gate.resolve({protocolVersion:1,closed:true}); await tick();
}
for (const tamper of [
  (r:BridgeRemoteControllerSessionViewResponse) => { r.controller.id = "other"; },
  (r:BridgeRemoteControllerSessionViewResponse) => { r.controller.name = "other"; },
  (r:BridgeRemoteControllerSessionViewResponse) => { r.controller.workspace = "/other"; },
  (r:BridgeRemoteControllerSessionViewResponse) => { r.controller.readOnly = false; },
  (r:BridgeRemoteControllerSessionViewResponse) => { r.protocolVersion = 2; },
  (r:BridgeRemoteControllerSessionViewResponse) => { r.view.protocolVersion = 2; },
  (r:BridgeRemoteControllerSessionViewResponse) => { r.view.sessionPath = "/other"; },
  (r:BridgeRemoteControllerSessionViewResponse) => { r.view.readOnly = false; },
  (r:BridgeRemoteControllerSessionViewResponse) => { r.view.history[0].id = ""; },
  (r:BridgeRemoteControllerSessionViewResponse) => { r.view.history[0].id = "bad\nidentity"; },
  (r:BridgeRemoteControllerSessionViewResponse) => { r.view.history[0].id = "x".repeat(4097); },
  (r:BridgeRemoteControllerSessionViewResponse) => { r.view.history[1].id = r.view.history[0].id; },
]) {
  const f = fixture(); const lease = f.pool.acquire("owned","/owned"); await tick(); f.attaches[0].gate.resolve(response("checked")); await lease.ready;
  const read = lease.sessionView(selected); await tick(); const invalid = snapshotResponse("checked"); tamper(invalid); f.snapshots[0].gate.resolve(invalid);
  await assert.rejects(read,/connection changed/);
  lease.release(); await tick(); f.closes[0].gate.resolve({protocolVersion:1,closed:true}); await tick();
}
{
  const f = fixture(); const lease = f.pool.acquire("owned","/owned"); await tick(); f.attaches[0].gate.resolve(response("checked")); await lease.ready;
  for (const path of ["","bad\npath","中".repeat(10923)]) await assert.rejects(lease.sessionView(path),/connection changed/);
  assert.equal(f.snapshots.length,0,"invalid path never reaches IPC");
  lease.release(); await tick(); f.closes[0].gate.resolve({protocolVersion:1,closed:true}); await tick();
}
const imageResponse = ():BridgeRemoteControllerSessionImageResponse => ({...response("image"),view:{protocolVersion:1,sessionPath:selected,workspace:"/owned",image:{url:"data:image/png;base64,aQ==",mime:"image/png",size:1}}});
{
  const f=fixture(); const lease=f.pool.acquire("owned","/owned"); await tick(); f.attaches[0].gate.resolve(response("image")); await lease.ready;
  const read=lease.sessionImage(selected,"photo.png"); await tick();
  assert.deepEqual({id:f.images[0].id,path:f.images[0].sessionPath,source:f.images[0].source},{id:"image",path:selected,source:"photo.png"});
  const valid=imageResponse(); Object.assign(valid.view.image,{openHref:"file:///private/secret",token:"PRIVATE"}); f.images[0].gate.resolve(valid);
  assert.deepEqual(await read,{url:valid.view.image.url,mime:"image/png",size:1,filename:undefined},"pixels never grant a remote opener or expose extra fields");
  const late=lease.sessionImage(selected,"late.png"); await tick(); lease.release(); f.images[1].gate.resolve(imageResponse());
  await assert.rejects(late,/connection changed/);
  await assert.rejects(lease.sessionImage(selected,"new.png"),/connection changed/); assert.equal(f.images.length,2,"released lease dispatches no image request");
  await tick(); f.closes[0].gate.resolve({protocolVersion:1,closed:true}); await tick();
}
for(const tamper of [
  (r:BridgeRemoteControllerSessionImageResponse)=>{r.view.sessionPath="/other";},
  (r:BridgeRemoteControllerSessionImageResponse)=>{r.view.workspace="/other";},
  (r:BridgeRemoteControllerSessionImageResponse)=>{r.controller.id="other";},
  (r:BridgeRemoteControllerSessionImageResponse)=>{r.view.image.url="https://private.invalid/image.png";},
  (r:BridgeRemoteControllerSessionImageResponse)=>{r.view.image.size=16777217;},
  (r:BridgeRemoteControllerSessionImageResponse)=>{Object.assign(r.view.image,{errorCode:"PRIVATE ERROR"});},
]) {
  const f=fixture(); const lease=f.pool.acquire("owned","/owned"); await tick(); f.attaches[0].gate.resolve(response("image")); await lease.ready;
  const read=lease.sessionImage(selected,"photo.png"); await tick(); const invalid=imageResponse(); tamper(invalid); f.images[0].gate.resolve(invalid); await assert.rejects(read,/connection changed/);
  lease.release(); await tick(); f.closes[0].gate.resolve({protocolVersion:1,closed:true}); await tick();
}
console.log("Remote controller pool: shared ownership, scoped images, private-field projection, stale responses, history IDs and SSH reset passed");
