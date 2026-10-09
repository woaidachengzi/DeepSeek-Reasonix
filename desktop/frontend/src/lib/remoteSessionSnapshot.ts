import type { BridgeRemoteControllerSessionProjection, BridgeRemoteControllerSessionProjectionResponse, BridgeRemoteControllerView } from "./bridgeProtocol.generated";
import { openRemoteSessionSubscription, type RemoteSubscriptionIdentity, type RemoteSubscriptionRequest, type RemoteSubscriptionTransport } from "./remoteSessionSubscription";

export interface RemoteSnapshotTransport extends RemoteSubscriptionTransport {
  snapshot(subscriptionId:string, continuation?:string):Promise<unknown>;
}
export interface RemoteDisplayCut {
  // The initial page is not rewritten into an oversized synthetic wire page.
  // events plus capturedThrough describe the complete fixed display cut.
  projection:BridgeRemoteControllerSessionProjection;
  events:Record<string,unknown>[];
  capturedThrough:number;
}
export interface RemoteSnapshotSink {
  state(state:"opening"|"syncing"|"live"|"reconcile"):void;
  snapshot(cut:RemoteDisplayCut):void;
  event(event:Record<string,unknown>):void;
}
const encoder=new TextEncoder();
const record=(value:unknown):value is Record<string,unknown> => !!value && typeof value==="object" && !Array.isArray(value);
const handle=(value:unknown):value is string => typeof value==="string" && /^[A-Za-z0-9_-]{21}[AQgw]$/.test(value);
const clean=(value:unknown,limit:number):value is string => typeof value==="string" && encoder.encode(value).length<=limit && !/\p{Cc}/u.test(value);
const sequence=(value:unknown):value is number => Number.isSafeInteger(value) && (value as number)>=0;
const sameIdentity=(a:unknown,b:RemoteSubscriptionIdentity):boolean => record(a) && ["protocolVersion","subscriptionId","controllerId","sessionPath","surfaceId","generation","sidecarInstanceId"].every(key=>a[key]===b[key as keyof RemoteSubscriptionIdentity]);

// One listener/ready lifecycle owns both snapshot and live delivery. No timers,
// retry, resume, provider reconstruction or positional display identity guesses.
export function openRemoteSessionSnapshot(transport:RemoteSnapshotTransport, selected:BridgeRemoteControllerView, input:RemoteSubscriptionRequest, sink:RemoteSnapshotSink) {
  const request={controllerId:input.controllerId,sessionPath:input.sessionPath,surfaceId:input.surfaceId,generation:input.generation};
  const controller={...selected};
  let stopped=false, initialized=false, last=0, turn="", pendingBytes=0;
  let pending:Record<string,unknown>[]=[];
  let subscription:ReturnType<typeof openRemoteSessionSubscription> | undefined;
  const dispose=()=>{if(stopped)return;stopped=true;pending=[];pendingBytes=0;subscription?.dispose();};
  const fail=()=>{if(stopped)return;dispose();sink.state("reconcile");};
  const checkedEvent=(value:unknown):Record<string,unknown> => {
    if(!record(value) || value.sessionPath!==request.sessionPath || !sequence(value.seq) || value.seq===0 || !clean(value.turnId,4096) || !value.turnId || !clean(value.kind,128) || !value.kind) throw new Error("invalid remote event");
    if(encoder.encode(JSON.stringify(value)).length>8*1024*1024)throw new Error("remote frame budget");
    return value;
  };
  const deliver=(event:Record<string,unknown>)=>{
    if(event.turnId!==turn || event.seq!==last+1) throw new Error("remote projection changed");
    last=event.seq as number;sink.event(event);
  };
  subscription=openRemoteSessionSubscription(transport,controller,request,{
    state:state=>{if(stopped)return;if(state==="opening")sink.state("opening");else if(state==="failed"||state==="ended")fail();},
    event:frame=>{if(stopped)return;try{
      const event=checkedEvent(frame.event);
      if(initialized){if(event.turnId!==turn)throw new Error("remote turn changed");if((event.seq as number)<=last)return;deliver(event);}
      else {pendingBytes+=encoder.encode(JSON.stringify(event)).length;if(pending.length>=256||pendingBytes>9*1024*1024)throw new Error("remote queue budget");pending.push(event);}
    }catch{fail();}},
  });
  const settled=(async()=>{try{
    const identity=await subscription.ready;
    if(stopped||!identity)return;
    sink.state("syncing");
    let first:BridgeRemoteControllerSessionProjection | undefined, next:string|undefined, bytes=0;
    const events:Record<string,unknown>[]=[];
    for(let page=0;page<64;page++){
      const raw=await transport.snapshot(identity.subscriptionId,next);
      if(stopped)return;
      bytes+=encoder.encode(JSON.stringify(raw)).length;
      if(bytes>30*1024*1024||!record(raw)||raw.protocolVersion!==1||!sameIdentity(raw.subscription,identity)||!record(raw.snapshot))throw new Error("remote snapshot scope");
      const response=raw.snapshot as unknown as BridgeRemoteControllerSessionProjectionResponse;
      const p=response.projection,r=p?.replay;
      if(response.protocolVersion!==1||response.controller?.id!==controller.id||response.controller.name!==controller.name||response.controller.workspace!==controller.workspace||response.controller.readOnly!==true||p?.protocolVersion!==1||p.sessionPath!==request.sessionPath||p.readOnly!==true||p.pageToken!==undefined||p.initial!==(page===0)||!Array.isArray(p.history)||!Array.isArray(p.userSuffix)||p.history.length+p.userSuffix.length>100000||!r||!Array.isArray(r.events)||r.events.length>512||!sequence(r.latestSeq)||!sequence(r.floorSeq)||r.floorSeq<1||r.floorSeq>r.latestSeq+1||!sequence(p.replayAfterSeq)||p.replayAfterSeq<r.floorSeq-1||p.replayAfterSeq>r.latestSeq||!sequence(r.nextAfterSeq)||r.nextAfterSeq>r.latestSeq||typeof r.hasMore!=="boolean"||r.hasMore!==(r.nextAfterSeq<r.latestSeq)||r.hasMore!==(response.nextPage!==undefined)||(response.nextPage!==undefined&&!handle(response.nextPage)))throw new Error("invalid remote snapshot");
      if(page===0){
        const status=p.turnStatus??"",active=p.activeTurnId??"";
        if(!clean(active,4096)||!clean(r.runtimeEpoch??"",4096))throw new Error("invalid remote cut identity");
        if(["","completed","interrupted","failed","protocol_failed"].includes(status)){
          if(active||p.userSuffix.length||r.events.length||r.hasMore||p.replayAfterSeq!==r.latestSeq)throw new Error("invalid settled cut");
        }else if(!["queued","in_progress","waiting_user","cancelling"].includes(status)||!active)throw new Error("invalid active cut");
        const ids=new Set<string>();
        for(const row of [...p.history,...p.userSuffix]){if(!row.id||!clean(row.id,4096)||ids.has(row.id)||!["user","assistant","tool","notice","protocol_recovery","final_readiness"].includes(row.role))throw new Error("invalid remote history");ids.add(row.id);}
        if(p.userSuffix.some(row=>row.role!=="user"))throw new Error("invalid remote question");
        first=p;last=p.replayAfterSeq;turn=p.activeTurnId??"";
      }else if(!first||p.history.length||p.userSuffix.length||p.activeTurnId!==first.activeTurnId||p.turnStatus!==first.turnStatus||p.replayAfterSeq!==first.replayAfterSeq||r.latestSeq!==first.replay.latestSeq||r.runtimeEpoch!==first.replay.runtimeEpoch)throw new Error("remote cut changed");
      for(const value of r.events){const event=checkedEvent(value);if(event.turnId!==turn||event.seq!==last+1)throw new Error("remote replay gap");last=event.seq as number;events.push(event);}
      if(last!==r.nextAfterSeq||(r.hasMore&&!r.events.length))throw new Error("remote replay stalled");
      next=response.nextPage;
      if(!r.hasMore)break;
      if(page===63)throw new Error("remote page budget");
    }
    if(stopped||!first)return;
    // First publish is one complete bounded cut. Buffered overlap is not
    // replayed a second time; a new turn or gap asks for explicit reconcile.
    const live=pending.filter(event=>(event.seq as number)>last);
    let expected=last;
    for(const event of live){if(event.turnId!==turn||event.seq!==expected+1)throw new Error("remote live gap");expected=event.seq as number;}
    sink.snapshot({projection:first,events,capturedThrough:last});
    if(stopped)return;
    pending=[];pendingBytes=0;initialized=true;
    for(const event of live){if(stopped)return;deliver(event);}
    if(!stopped)sink.state("live");
  }catch{fail();}})();
  return {settled,dispose};
}
