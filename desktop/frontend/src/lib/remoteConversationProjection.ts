import { initialState, reducer, type Item, type LiveStream, type State } from "./useController";
import { remoteHistoryItems, remoteHistoryKey } from "./remoteHistoryItems";
import type { RemoteDisplayCut } from "./remoteSessionSnapshot";
import type { WireEvent } from "./types";

export interface RemoteConversationView {
  items:Item[];
  live?:LiveStream;
  running:boolean;
  pendingPrompt:boolean;
}

// The shared event reducer owns sampling/tool/search/retry semantics. This
// spectator adapter owns display IDs only; it exposes no State or command port.
export function createRemoteConversationProjection(cut:RemoteDisplayCut, surface:string, labels:{protocol:string;readiness:string}) {
  const p=cut.projection,turn=p.activeTurnId??"";
  if(!p.initial||!p.readOnly||cut.capturedThrough!==p.replay.latestSeq)throw new Error("remote cut incomplete");
  if(turn&&p.userSuffix.length!==1)throw new Error("remote question requires reconciliation");
  const prefix=remoteHistoryItems({history:[...p.history,...p.userSuffix]},surface,labels);
  const messages=new Set([...p.history,...p.userSuffix].map(row=>row.id));
  if(messages.size!==p.history.length+p.userSuffix.length)throw new Error("duplicate remote message identity");
  let state:State={...initialState,items:[],activeTurnId:turn||undefined,running:!!turn,turnActive:!!turn,pendingPrompt:p.turnStatus==="waiting_user"};
  let after=p.replayAfterSeq;
  const keys=new Map<string,string>();
  const apply=(raw:Record<string,unknown>,verifiedCut:boolean)=>{
    if(!Number.isSafeInteger(raw.seq)||raw.seq!==after+1||raw.turnId!==turn||raw.sessionPath!==p.sessionPath)throw new Error("remote event scope changed");
    const e=raw as unknown as WireEvent;
    if(e.kind==="user_message_admitted"&&e.messageId!==p.userSuffix[0]?.id)throw new Error("remote question identity requires reconciliation");
    if(e.kind==="steer"&&(typeof e.messageId!=="string"||!e.messageId||new TextEncoder().encode(e.messageId).length>4096||/\p{Cc}/u.test(e.messageId)||messages.has(e.messageId)))throw new Error("remote guidance identity requires reconciliation");
    if(e.kind==="session_changed"||e.kind==="workspace_changed"||e.kind==="compaction_done"&&!verifiedCut)throw new Error("remote base requires reconciliation");
    const next=reducer(state,{type:"event",e,remote:true});
    const staged:[string,string][]=[];
    let guidance:string|undefined;
    for(const item of next.items){
      if(keys.has(item.id))continue;
      if(keys.size+staged.length>=100000)throw new Error("remote display identity budget");
      let key:string;
      if(e.kind==="steer"&&item.kind==="notice"&&item.id===`steer:${e.messageId}`){key=remoteHistoryKey(surface,e.messageId!);guidance=e.messageId;}
      else if(item.kind==="assistant"||item.kind==="tool")key=remoteHistoryKey(surface,turn,item.kind,item.id);
      else key=remoteHistoryKey(surface,turn,"event",`${raw.seq}:${item.kind}:${item.id}`);
      staged.push([item.id,key]);
    }
    for(const [id,key] of staged)keys.set(id,key);
    if(guidance)messages.add(guidance);
    state=next;after=raw.seq as number;
  };
  for(const frame of cut.events)apply(frame,true);
  if(after!==cut.capturedThrough)throw new Error("remote replay incomplete");
  const view=():RemoteConversationView=>{
    const items=state.items.map((item):Item=>{
      const id=keys.get(item.id);
      if(!id)throw new Error("remote display identity missing");
      if(item.kind==="notice")return {...item,id,action:undefined,recoveryId:undefined,inboxItemId:undefined};
      if(item.kind==="tool")return {...item,id,parentId:item.parentId?keys.get(item.parentId):undefined,capabilityId:undefined,dataArchived:undefined,execution:undefined};
      if(item.kind==="compaction")return {...item,id,archive:""};
      if(item.kind==="extension")return {...item,id,card:{...item.card,actions:undefined}};
      if(item.kind==="user")return {...item,id,submissionId:undefined,submitText:undefined,tailFollowRequested:undefined,checkpointTurn:undefined};
      return {...item,id};
    });
    const combined=[...prefix,...items];
    if(new Set(combined.map(item=>item.id)).size!==combined.length)throw new Error("duplicate remote display identity");
    return {items:combined,live:state.live?{...state.live,id:keys.get(state.live.id)!}:undefined,running:state.running,pendingPrompt:state.pendingPrompt};
  };
  return {event:(raw:Record<string,unknown>)=>apply(raw,false),view};
}
