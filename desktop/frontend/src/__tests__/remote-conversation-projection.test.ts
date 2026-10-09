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

const hostCut:RemoteDisplayCut={...admittedCut,events:[{kind:"host_input_admitted",seq:1,turnId:turn,sessionPath:path,messageId:"host-input",text:"PRIVATE host instructions",itemId:"not-an-authority"}],projection:{...admittedCut.projection,userSuffix:[]}};
const host=createRemoteConversationProjection(hostCut,surface,labels);
assert.deepEqual(host.view().items,remoteHistoryItems({history:hostCut.projection.history},surface,labels),"host readiness invents no question or notice");
assert.equal(host.view().running,true);
host.event({kind:"text",seq:2,turnId:turn,sessionPath:path,text:"continued answer"});
assert.equal(host.view().live?.text,"continued answer");
assert.equal(host.view().items.filter(item=>item.kind==="user").length,1);
assert.ok(!JSON.stringify(host.view()).includes("PRIVATE"));
assert.throws(()=>host.event({kind:"host_input_admitted",seq:3,turnId:turn,sessionPath:path,messageId:"host-input"}),/readiness/);
host.event({kind:"text",seq:3,turnId:turn,sessionPath:path,text:" more"});
assert.equal(host.view().live?.text,"continued answer more","rejected live readiness cannot consume the cursor");
for(const bad of [
  {...hostCut,events:[{...hostCut.events[0],messageId:""}]},
  {...hostCut,events:[{...hostCut.events[0],messageId:"old-user"}]},
  {...hostCut,events:[{...hostCut.events[0],turnId:"foreign"}]},
  {...hostCut,projection:{...hostCut.projection,userSuffix:cut.projection.userSuffix}},
  {...hostCut,events:[...hostCut.events,{...hostCut.events[0],seq:2}],capturedThrough:2,projection:{...hostCut.projection,replay:{...hostCut.projection.replay,latestSeq:2}}},
])assert.throws(()=>createRemoteConversationProjection(bad,surface,labels),/readiness|question/);
console.log("Remote host display: no fabricated user, private input/actions absent, canonical identity/origin refusal and shared live output passed");

// A verified waiting cut must recover the same card as the live event stream.
// These are display snapshots, never a controller lease or an executable grant.
const promptFixtures=[
  {kind:"ask",event:"ask_request",field:"ask",request:{id:"ask-id",questions:[{id:"question",prompt:"Choose",options:[{label:"One",value:"one"}]}]}},
  {kind:"approval",event:"approval_request",field:"approval",request:{id:"approval-id",tool:"write_file",subject:"remote file"}},
  {kind:"plan",event:"approval_request",field:"approval",request:{id:"plan-id",tool:"exit_plan_mode",subject:"plan",kind:"plan"}},
  {kind:"recovery",event:"approval_request",field:"approval",request:{id:"recovery-id",tool:"read_file",subject:"retry",kind:"recovery"}},
  {kind:"mcp",event:"mcp_interaction",field:"mcpInteraction",request:{id:"mcp-id",server:"fixture",mode:"form",message:"Details",requestedSchema:{type:"object",properties:{name:{type:"string"}}}}},
] as const;
for(const fixture of promptFixtures){
  const frame={kind:fixture.event,seq:10,sessionPath:path,turnId:turn,runtimeEpoch:"prompt-routing",[fixture.field]:fixture.request};
  const waitingCut:RemoteDisplayCut={...cut,events:[...events,frame],capturedThrough:10,
    projection:{...cut.projection,turnStatus:"waiting_user",replay:{...cut.projection.replay,latestSeq:10,nextAfterSeq:10}}};
  const recovered=createRemoteConversationProjection(waitingCut,surface,labels);
  const live=createRemoteConversationProjection(cut,surface,labels);
  live.event(frame);
  assert.deepEqual(recovered.view().prompt,live.view().prompt);
  const pending=recovered.view().prompt!;
  assert.equal(pending.kind,fixture.kind);
  assert.equal(pending.request.id,fixture.request.id);
  assert.equal(pending.request.turnId,turn);
  assert.equal(pending.request.runtimeEpoch,"prompt-routing","routing stamp is not the replay/controller epoch");
  assert.equal("commands" in recovered.view(),false);
  assert.equal("approval" in recovered.view(),false);
  pending.request.id="consumer-mutated";
  assert.equal(recovered.view().prompt?.request.id,fixture.request.id);
  if(pending.kind==="ask"){
    pending.request.questions[0].prompt="consumer-mutated";
    const again=recovered.view().prompt!;
    assert.ok(again.kind==="ask"&&again.request.questions[0].prompt==="Choose");
  }
  live.event({kind:"prompt_answered",seq:11,sessionPath:path,turnId:turn,itemId:"another-prompt"});
  assert.equal(live.view().prompt?.request.id,fixture.request.id,"unrelated receipt cannot dismiss this card");
  live.event({kind:"prompt_answered",seq:12,sessionPath:path,turnId:turn});
  assert.equal(live.view().prompt?.request.id,fixture.request.id,"identity-free receipts cannot dismiss remote cards");
  live.event({kind:"prompt_answered",seq:13,sessionPath:path,turnId:turn,itemId:fixture.request.id});
  assert.equal(live.view().prompt,undefined);
  assert.equal(live.view().pendingPrompt,false);
  live.event({...frame,seq:14});
  assert.equal(live.view().prompt,undefined,"late re-delivery cannot resurrect an answered prompt");
  recovered.event({kind:"turn_done",seq:11,sessionPath:path,turnId:turn,status:"completed"});
  assert.equal(recovered.view().prompt,undefined,"completed turns expose no pending decision");
}
const isolated=createRemoteConversationProjection(cut,surface,labels);
const mutableAsk={id:"immutable-id",questions:[{id:"q",prompt:"original",options:[]}]};
isolated.event({kind:"ask_request",seq:10,sessionPath:path,turnId:turn,ask:mutableAsk});
mutableAsk.id="transport-mutated";mutableAsk.questions[0].prompt="transport-mutated";
assert.equal(isolated.view().prompt?.request.id,"immutable-id");
const isolatedPrompt=isolated.view().prompt!;
assert.ok(isolatedPrompt.kind==="ask"&&isolatedPrompt.request.questions[0].prompt==="original");
assert.equal(isolatedPrompt.request.runtimeEpoch,undefined,"legacy empty routing stamps must not acquire an instance epoch");
for(const request of [
  {id:"foreign",questions:[],turnId:"foreign-turn"},
  {id:"foreign",questions:[],runtimeEpoch:"other-routing"},
])assert.throws(()=>isolated.event({kind:"ask_request",seq:11,sessionPath:path,turnId:turn,runtimeEpoch:"routing",ask:request}),/prompt identity/);
isolated.event({kind:"text",seq:11,sessionPath:path,turnId:turn,text:"accepted cursor"});
assert.equal(isolated.view().prompt?.request.id,"immutable-id","rejected prompt identities do not consume the cursor");
for(const id of ["","bad\nidentity","x".repeat(4097)]){
  const invalid=createRemoteConversationProjection(cut,surface,labels);
  invalid.event({kind:"ask_request",seq:10,sessionPath:path,turnId:turn,ask:{id,questions:[]}});
  assert.equal(invalid.view().pendingPrompt,true);
  assert.equal(invalid.view().prompt,undefined,"unusable identity retains waiting state but exposes no actionable card");
}
isolated.event({kind:"approval_request",seq:12,sessionPath:path,turnId:turn,approval:{id:"new-approval",tool:"write_file",subject:"remote"}});
assert.equal(isolated.view().prompt?.kind,"approval","the latest arrival selects the current slot, not an older Ask");
isolated.event({kind:"prompt_answered",seq:13,sessionPath:path,turnId:turn,itemId:"immutable-id"});
assert.equal(isolated.view().prompt?.request.id,"new-approval","a delayed receipt for a retained older slot cannot dismiss the current card");
isolated.event({kind:"prompt_answered",seq:14,sessionPath:path,turnId:turn,itemId:"new-approval"});
assert.equal(isolated.view().prompt,undefined);
const ambiguous=createRemoteConversationProjection(cut,surface,labels);
ambiguous.event({kind:"ask_request",seq:10,sessionPath:path,turnId:turn,ask:{id:"same-id",questions:[]}});
ambiguous.event({kind:"approval_request",seq:11,sessionPath:path,turnId:turn,approval:{id:"same-id",tool:"write_file",subject:"remote"}});
assert.equal(ambiguous.view().prompt,undefined,"ambiguous prompt ownership exposes no actionable card");
const badRouting=createRemoteConversationProjection(cut,surface,labels);
badRouting.event({kind:"ask_request",seq:10,sessionPath:path,turnId:turn,runtimeEpoch:"bad\nrouting",ask:{id:"valid-id",questions:[]}});
assert.equal(badRouting.view().prompt,undefined);
console.log("Remote prompt display: five recovered kinds, routing/turn identity, isolated snapshots, answered dismissal and fail-closed cards passed");
