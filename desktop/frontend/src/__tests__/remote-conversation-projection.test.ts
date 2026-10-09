import assert from "node:assert/strict";
import { createRemoteConversationProjection } from "../lib/remoteConversationProjection";
import { remoteHistoryItems,remoteHistoryKey } from "../lib/remoteHistoryItems";
import { initialState,reducer } from "../lib/useController";
import type { RemoteDisplayCut } from "../lib/remoteSessionSnapshot";
const labels={protocol:"protocol notice",readiness:"readiness notice"};
const path="/owned.jsonl",turn="owned-turn",surface="owned-surface";
const events:Record<string,unknown>[]=[
  {kind:"turn_started"},
  {kind:"reasoning",text:"thought"},
  {kind:"stream_attempt",streamAttempt:{id:"sample",action:"begin"}},
  {kind:"text",text:"discard-me"},
  {kind:"stream_attempt",streamAttempt:{id:"sample",action:"discard",reason:"connection_reset"}},
  {kind:"text",text:"answer"},
  {kind:"tool_dispatch",tool:{id:"call-id",name:"read_file",args:"{}",readOnly:true}},
  {kind:"tool_result",tool:{id:"call-id",name:"read_file",output:"full remote result"}},
  {kind:"steer",text:"question",messageId:"guide-id",itemId:"inbox-id"},
].map((frame,index)=>({...frame,seq:index+1,turnId:turn,sessionPath:path,status:"in_progress"}));
const cut:RemoteDisplayCut={capturedThrough:events.length,events,projection:{protocolVersion:1,sessionPath:path,readOnly:true,initial:true,history:[
  {id:"old-user",role:"user",content:"old question"},
  {id:"old-assistant",role:"assistant",content:"old answer"},
  {id:"old-guidance",role:"notice",content:"↪ older guidance"},
],userSuffix:[{id:"question-id",role:"user",content:"question"}],activeTurnId:turn,turnStatus:"in_progress",replayAfterSeq:0,replay:{events:[],floorSeq:1,latestSeq:events.length,nextAfterSeq:events.length,hasMore:false,runtimeEpoch:"epoch"}}};
const projection=createRemoteConversationProjection(cut,surface,labels);
const view=projection.view();
assert.deepEqual(view.items.slice(0,4),remoteHistoryItems({history:[...cut.projection.history,...cut.projection.userSuffix]},surface,labels));
assert.equal(view.items.filter(item=>item.kind==="user").length,2,"guidance with equal question text is not another user question");
const sampled=view.items.find(item=>item.kind==="assistant"&&item.text==="answer");
assert.ok(sampled?.kind==="assistant","shared stream-attempt rollback discards speculative text");
assert.equal(sampled.reasoning,"thought");
assert.ok(view.items.some(item=>item.kind==="tool"&&item.output==="full remote result"&&!item.dataArchived));
assert.ok(view.items.some(item=>item.kind==="notice"&&item.id===remoteHistoryKey(surface,"guide-id")&&item.text==="↪ question"&&!item.inboxItemId));
assert.equal("approval" in view,false);assert.equal("commands" in view,false);
const activeIds=view.items.slice(4).map(item=>item.id);
projection.event({kind:"text",seq:10,turnId:turn,sessionPath:path,status:"in_progress",text:" tail"});
assert.deepEqual(projection.view().items.slice(4,4+activeIds.length).map(item=>item.id),activeIds,"later sampling preserves mounted display identities");
assert.equal(projection.view().live?.text," tail","shared tool boundary starts a new sampling segment");
const prefixed=createRemoteConversationProjection({...cut,projection:{...cut.projection,history:[{id:"older",role:"user",content:"older"},...cut.projection.history]}},surface,labels).view();
assert.deepEqual(prefixed.items.slice(5).map(item=>item.id),activeIds,"older prefix does not rename active sampling/tool/notice identities");
const other=createRemoteConversationProjection(cut,"other-surface",labels).view();
assert.ok(other.items.every(item=>!view.items.some(old=>old.id===item.id)),"scopes do not alias");
const saved=remoteHistoryItems({history:[{id:"guide-id",role:"notice",content:"↪ question"}]},surface,labels);
assert.equal(saved[0].id,view.items.find(item=>item.kind==="notice"&&item.text==="↪ question")?.id,"saved messageId matches the active guidance identity");
const terminal=createRemoteConversationProjection(cut,surface,labels);
terminal.event({kind:"turn_done",seq:10,turnId:turn,sessionPath:path,status:"completed",protocolRecovery:{id:"not-a-local-grant"}});
assert.equal(terminal.view().running,false);
assert.ok(terminal.view().items.every(item=>item.kind!=="notice"||!item.action&&!item.recoveryId));
assert.equal(terminal.view().live,undefined);
assert.throws(()=>createRemoteConversationProjection({...cut,projection:{...cut.projection,userSuffix:[]}},surface,labels),/question/);
assert.throws(()=>createRemoteConversationProjection({...cut,capturedThrough:8},surface,labels),/incomplete/);
const legacyEvents=events.map(frame=>frame.kind==="steer"?{...frame,messageId:undefined}:frame);
assert.throws(()=>createRemoteConversationProjection({...cut,events:legacyEvents},surface,labels),/guidance identity/);
assert.throws(()=>projection.event({kind:"text",seq:11,turnId:"other-turn",sessionPath:path,text:"foreign"}),/scope/);
assert.throws(()=>projection.event({kind:"text",seq:12,turnId:turn,sessionPath:path,text:"gap"}),/scope/);
assert.throws(()=>projection.event({kind:"compaction_done",seq:11,turnId:turn,sessionPath:path}),/base/);
assert.throws(()=>projection.event({kind:"steer",seq:11,turnId:turn,sessionPath:path,text:"legacy"}),/guidance identity/);
projection.event({kind:"text",seq:11,turnId:turn,sessionPath:path,text:" accepted"});
assert.equal(projection.view().live?.text," tail accepted","rejected base/legacy events do not consume the cursor");
const legacy=reducer(initialState,{type:"event",e:{kind:"steer",text:"legacy",itemId:"inbox"}});
assert.equal(legacy.items[0].id,"s0","local legacy guidance retains existing fallback behavior");
const identified=reducer(initialState,{type:"event",e:{kind:"steer",text:"new",messageId:"saved-id",itemId:"different-inbox"}});
assert.equal(identified.items[0].id,"steer:saved-id");
const admittedCut:RemoteDisplayCut={...cut,capturedThrough:1,events:[{kind:"user_message_admitted",seq:1,turnId:turn,sessionPath:path,messageId:"question-id"}],projection:{...cut.projection,replay:{...cut.projection.replay,latestSeq:1,nextAfterSeq:1}}};
const admitted=createRemoteConversationProjection(admittedCut,surface,labels);
assert.equal(admitted.view().items.filter(item=>item.kind==="user").length,2,"admission confirms the suffix identity without duplicating the question");
assert.throws(()=>createRemoteConversationProjection({...admittedCut,events:[{...admittedCut.events[0],messageId:"foreign-question"}]},surface,labels),/question identity/);
const compactedEvents=[...cut.events,
  {kind:"compaction_started",seq:10,turnId:turn,sessionPath:path},
  {kind:"compaction_done",seq:11,turnId:turn,sessionPath:path,compaction:{summary:"verified digest",messages:4,archive:"private archive"}},
];
const compacted=createRemoteConversationProjection({...cut,events:compactedEvents,capturedThrough:11,projection:{...cut.projection,replay:{...cut.projection.replay,latestSeq:11,nextAfterSeq:11}}},surface,labels);
assert.deepEqual(compacted.view().items.slice(4,4+activeIds.length).map(item=>item.id),activeIds,"verified resampling retains existing active display IDs");
assert.ok(compacted.view().items.some(item=>item.kind==="compaction"&&!item.pending&&item.summary==="verified digest"&&item.archive===""));
compacted.event({kind:"text",seq:12,turnId:turn,sessionPath:path,text:"continued after compaction"});
assert.equal(compacted.view().live?.text,"continued after compaction");
console.log("Remote conversation projection: shared reducer, stable prefix/turn/tool/guidance identity, stream rollback and spectator-only output passed");
