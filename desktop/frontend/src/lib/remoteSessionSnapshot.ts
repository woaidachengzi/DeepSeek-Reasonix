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
export interface RemoteCutInput {kind:"user"|"host";turn:string;messageId:string}
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

// Readiness is canonical backend metadata, not a synthetic question or a
// guessed parent turn. A host input never enters the visible history rows.
export function remoteCutInput(cut:RemoteDisplayCut):RemoteCutInput|undefined {
  const p=cut.projection,turn=p.activeTurnId??"";
  if(!turn)return undefined;
  const hosts=cut.events.filter(event=>event.kind==="host_input_admitted");
  if(p.userSuffix.length===1 && hosts.length===0)return {kind:"user",turn,messageId:p.userSuffix[0].id};
  if(p.userSuffix.length!==0 || hosts.length!==1)throw new Error("remote question/readiness requires reconciliation");
  const host=hosts[0];
  if(host.turnId!==turn||host.sessionPath!==p.sessionPath||!sequence(host.seq)||host.seq<=p.replayAfterSeq||host.seq>cut.capturedThrough||!clean(host.messageId,4096)||!host.messageId||p.history.some(row=>row.id===host.messageId))throw new Error("remote host readiness identity changed");
  return {kind:"host",turn,messageId:host.messageId};
}

// One listener/ready lifecycle owns both snapshot and live delivery. No timers,
// retry, resume, provider reconstruction or positional display identity guesses.
export function openRemoteSessionSnapshot(transport:RemoteSnapshotTransport, selected:BridgeRemoteControllerView, input:RemoteSubscriptionRequest, sink:RemoteSnapshotSink) {
  const request={controllerId:input.controllerId,sessionPath:input.sessionPath,surfaceId:input.surfaceId,generation:input.generation};
  const controller={...selected};
  let stopped=false, initialized=false, last=0, turn="", pendingBytes=0;
  let pending:Record<string,unknown>[]=[];
  let identity:RemoteSubscriptionIdentity|null=null, revision=0, candidate="", observed=0;
  let admission:(RemoteCutInput & {seq:number})|undefined;
  let needsCapture=true, reading:Promise<void>|undefined;
  let epoch:string|undefined, readyInput:RemoteCutInput|undefined;
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
  const enqueue=(event:Record<string,unknown>)=>{
    pendingBytes+=encoder.encode(JSON.stringify(event)).length;
    if(pending.length>=256||pendingBytes>9*1024*1024)throw new Error("remote queue budget");
    pending.push(event);
  };
  const requestCapture=():Promise<void>=>{
    if(stopped||!identity)return Promise.resolve();
    if(reading)return reading;
    reading=(async()=>{
      while(needsCapture&&!stopped){
        needsCapture=false;
        await capture(identity!,revision,admission);
      }
    })().catch(()=>fail()).finally(()=>{reading=undefined;if(needsCapture&&!stopped)void requestCapture();});
    return reading;
  };
  const receive=(value:unknown)=>{if(stopped)return;try{
      const event=checkedEvent(value);
      if(initialized && event.turnId===turn){
        if((event.seq as number)<=last)return;
        if(event.kind!=="compaction_done"){deliver(event);return;}
        if(event.seq!==last+1||!readyInput)throw new Error("remote compaction scope changed");
        // Re-read the complete cut through the backend's identity/content
        // fence. Never apply an unverified base barrier to the current view.
        candidate=turn;admission={...readyInput,seq:event.seq as number};revision++;initialized=false;
        pending=[];pendingBytes=0;needsCapture=true;enqueue(event);sink.state("syncing");void requestCapture();return;
      }
      if(event.turnId!==turn && (event.kind==="turn_started" || event.kind==="turn_status"&&event.status==="queued")){
        if(candidate!==event.turnId){
          if((event.seq as number)<=Math.max(last,observed))throw new Error("stale remote turn admission");
          candidate=event.turnId as string;admission=undefined;revision++;initialized=false;pending=[];pendingBytes=0;needsCapture=false;sink.state("syncing");
        }
      }
      observed=Math.max(observed,event.seq as number);
      if(candidate){
        if(event.turnId!==candidate)throw new Error("remote admission scope changed");
        if(event.kind==="user_message_admitted"||event.kind==="host_input_admitted"){
          const kind=event.kind==="user_message_admitted"?"user":"host";
          if(!clean(event.messageId,4096)||!event.messageId)throw new Error("missing remote question identity");
          if(admission){if(admission.kind!==kind||admission.messageId!==event.messageId||admission.seq!==event.seq)throw new Error("conflicting remote admission");return;}
          admission={kind,turn:candidate,messageId:event.messageId as string,seq:event.seq as number};needsCapture=true;
        }else if(!admission && !["turn_started","turn_status","turn_phase"].includes(event.kind as string))throw new Error("remote question not admitted");
        enqueue(event);
        if(admission)void requestCapture();
      }else if(initialized)throw new Error("remote turn changed");
      else enqueue(event);
    }catch{fail();}};
  subscription=openRemoteSessionSubscription(transport,controller,request,{
    state:state=>{if(stopped)return;if(state==="opening")sink.state("opening");else if(state==="failed"||state==="ended")fail();},
    event:frame=>receive(frame.event),
  });
  async function capture(identity:RemoteSubscriptionIdentity, capturedRevision:number, expected:typeof admission) {
    if(stopped||capturedRevision!==revision)return;
    sink.state("syncing");
    let first:BridgeRemoteControllerSessionProjection | undefined, next:string|undefined, bytes=0;
    let cutLast=0,cutTurn="";
    const events:Record<string,unknown>[]=[];
    for(let page=0;page<64;page++){
      let raw:unknown;
      try {raw=await transport.snapshot(identity.subscriptionId,next);}
      catch(error){if(stopped||capturedRevision!==revision)return;throw error;}
      if(stopped||capturedRevision!==revision)return;
      bytes+=encoder.encode(JSON.stringify(raw)).length;
      if(bytes>30*1024*1024||!record(raw)||raw.protocolVersion!==1||!sameIdentity(raw.subscription,identity)||!record(raw.snapshot))throw new Error("remote snapshot scope");
      const response=raw.snapshot as unknown as BridgeRemoteControllerSessionProjectionResponse;
      const p=response.projection,r=p?.replay;
      if(response.protocolVersion!==1||response.controller?.id!==controller.id||response.controller.name!==controller.name||response.controller.workspace!==controller.workspace||response.controller.readOnly!==true||p?.protocolVersion!==1||p.sessionPath!==request.sessionPath||p.readOnly!==true||p.pageToken!==undefined||p.initial!==(page===0)||!Array.isArray(p.history)||!Array.isArray(p.userSuffix)||p.history.length+p.userSuffix.length>100000||!r||!Array.isArray(r.events)||r.events.length>512||!sequence(r.latestSeq)||!sequence(r.floorSeq)||r.floorSeq<1||r.floorSeq>r.latestSeq+1||!sequence(p.replayAfterSeq)||p.replayAfterSeq<r.floorSeq-1||p.replayAfterSeq>r.latestSeq||!sequence(r.nextAfterSeq)||r.nextAfterSeq>r.latestSeq||typeof r.hasMore!=="boolean"||r.hasMore!==(r.nextAfterSeq<r.latestSeq)||r.hasMore!==(response.nextPage!==undefined)||(response.nextPage!==undefined&&!handle(response.nextPage)))throw new Error("invalid remote snapshot");
      if(page===0){
        const status=p.turnStatus??"",active=p.activeTurnId??"";
        if(!clean(active,4096)||!clean(r.runtimeEpoch??"",4096))throw new Error("invalid remote cut identity");
        if(epoch!==undefined&&epoch!==(r.runtimeEpoch??""))throw new Error("remote runtime changed");
        if(["","completed","interrupted","failed","protocol_failed"].includes(status)){
          if(active||p.userSuffix.length||r.events.length||r.hasMore||p.replayAfterSeq!==r.latestSeq)throw new Error("invalid settled cut");
        }else if(!["queued","in_progress","waiting_user","cancelling"].includes(status)||!active)throw new Error("invalid active cut");
        const ids=new Set<string>();
        for(const row of [...p.history,...p.userSuffix]){if(!row.id||!clean(row.id,4096)||ids.has(row.id)||!["user","assistant","tool","notice","protocol_recovery","final_readiness"].includes(row.role))throw new Error("invalid remote history");ids.add(row.id);}
        if(p.userSuffix.some(row=>row.role!=="user"))throw new Error("invalid remote question");
        if(expected && (r.latestSeq<expected.seq || active&&active!==expected.turn || expected.kind==="user"&&!(active?p.userSuffix:p.history).some(row=>row.id===expected.messageId&&row.role==="user")))throw new Error("remote admission cut changed");
        first=p;cutLast=p.replayAfterSeq;cutTurn=p.activeTurnId??"";
      }else if(!first||p.history.length||p.userSuffix.length||p.activeTurnId!==first.activeTurnId||p.turnStatus!==first.turnStatus||p.replayAfterSeq!==first.replayAfterSeq||r.latestSeq!==first.replay.latestSeq||r.runtimeEpoch!==first.replay.runtimeEpoch)throw new Error("remote cut changed");
      for(const value of r.events){const event=checkedEvent(value);if(event.turnId!==cutTurn||event.seq!==cutLast+1)throw new Error("remote replay gap");cutLast=event.seq as number;events.push(event);}
      if(cutLast!==r.nextAfterSeq||(r.hasMore&&!r.events.length))throw new Error("remote replay stalled");
      next=response.nextPage;
      if(!r.hasMore)break;
      if(page===63)throw new Error("remote page budget");
    }
    if(stopped||capturedRevision!==revision||!first)return;
    const cut:RemoteDisplayCut={projection:first,events,capturedThrough:cutLast};
    if(!expected && cutTurn && first.userSuffix.length===0 && !events.some(event=>event.kind==="host_input_admitted")){
      if(events.some(event=>!["turn_started","turn_status","turn_phase"].includes(event.kind as string)))throw new Error("remote input readiness missing");
      // Admission can precede canonical append. Wait on the existing
      // subscription for its identity barrier, never poll an empty cut.
      candidate=cutTurn;revision++;initialized=false;needsCapture=false;observed=Math.max(observed,first.replay.latestSeq);
      sink.state("syncing");
      // The SSE subscription can start after turn_started and buffer its
      // canonical admission while this pre-append body is in flight. Feed
      // those frames through the same scoped admission gate before waiting
      // for new events; otherwise its one identity barrier is lost.
      const waiting=pending;pending=[];pendingBytes=0;
      for(const event of waiting){if(stopped)return;receive(event);}
      return;
    }
    const inputReady=remoteCutInput(cut);
    // A fast-settled host input is intentionally absent from visible history.
    // Its scoped SSE admission was already confirmed; the same-owner/epoch
    // terminal cut must cover its sequence. Never synthesize a user row to
    // match that hidden ID. Active cuts still require exact replay identity.
    if(expected && cutTurn && (inputReady?.kind!==expected.kind||inputReady.messageId!==expected.messageId))throw new Error("remote input readiness changed");
    // Publish one complete bounded cut. A superseding admission fences the
    // previous read; overlap is not applied twice. No partial cut is painted.
    const live=pending.filter(event=>(event.seq as number)>cutLast);
    let cursor=cutLast;
    for(const event of live){if(event.turnId!==cutTurn||event.seq!==cursor+1)throw new Error("remote live gap");cursor=event.seq as number;}
    sink.snapshot(cut);
    if(stopped||capturedRevision!==revision)return;
    last=cutLast;turn=cutTurn||expected?.turn||"";epoch=first.replay.runtimeEpoch??"";readyInput=inputReady;candidate="";admission=undefined;pending=[];pendingBytes=0;initialized=true;
    for(const event of live){if(stopped)return;receive(event);}
    if(!stopped&&capturedRevision===revision)sink.state("live");
  }
  const settled=(async()=>{try{
    identity=await subscription.ready;
    if(stopped||!identity)return;
    await requestCapture();
  }catch{fail();}})();
  return {settled,dispose};
}
