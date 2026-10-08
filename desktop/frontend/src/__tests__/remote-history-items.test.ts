import assert from "node:assert/strict";
import { remoteHistoryItems } from "../lib/remoteHistoryItems";
import type { BridgeRemoteControllerSessionView } from "../lib/bridgeProtocol.generated";

const labels = {protocol:"protocol notice",readiness:"readiness notice"};
const view:BridgeRemoteControllerSessionView = {protocolVersion:1,sessionPath:"/remote/a.jsonl",readOnly:true,ownership:"saved",current:false,label:"",modelRef:"",history:[
  {id:"u1",role:"user",content:"question"},
  {id:"a1",role:"assistant",content:"answer",reasoning:"actual thought",toolCalls:[{id:"same-call",name:"write_file",arguments:"{}"}],serverSearch:[{id:"search",sources_status:"not_provided",results:[{title:"source",url:"https://example.invalid"}]}]},
  {id:"t1",role:"tool",content:"first result",toolCallId:"same-call",toolName:"write_file"},
  {id:"u2",role:"user",content:"second question"},
  {id:"a2",role:"assistant",content:"second answer",toolCalls:[{id:"same-call",name:"read_file",arguments:"{}"}]},
  {id:"t2",role:"tool",content:"second result",toolCallId:"same-call",toolName:"read_file"},
  {id:"recovery",role:"protocol_recovery",content:"",protocolRecovery:{id:"do-not-grant-an-action"}},
]};
const items=remoteHistoryItems(view,"owner",labels);
assert.equal(new Set(items.map(item=>item.id)).size,items.length);
assert.ok(items.some(item=>item.kind==="assistant" && item.reasoning==="actual thought"));
const tools=items.filter(item=>item.kind==="tool");
assert.equal(tools.find(item=>item.name==="write_file")?.output,"first result");
assert.equal(tools.find(item=>item.name==="read_file")?.output,"second result","reused tool call IDs do not merge different turns");
assert.equal(tools.find(item=>item.name==="web_search")?.searchSourcesStatus,"not_provided");
assert.ok(items.every(item=>item.kind!=="notice" || !item.action));
assert.ok(tools.every(item=>!item.dataArchived && !item.capabilityId));
const appended=remoteHistoryItems({...view,history:[...view.history,{id:"u3",role:"user",content:"tail"}]} ,"owner",labels);
assert.deepEqual(appended.slice(0,items.length),items,"appending preserves all existing presentation identity and content");
const prefixed=remoteHistoryItems({...view,history:[{id:"old",role:"user",content:"older"},...view.history]},"owner",labels);
assert.deepEqual(prefixed.slice(1),items,"prepend does not rename existing turns");
const filtered=remoteHistoryItems({...view,history:view.history.filter(row=>row.id!=="recovery")},"owner",labels);
assert.deepEqual(filtered,items.filter(item=>item.kind!=="notice"));
const other=remoteHistoryItems(view,"different-owner",labels);
assert.ok(other.every(item=>!items.some(old=>old.id===item.id)),"same backend IDs in another remote scope cannot alias local or prior rows");
assert.throws(()=>remoteHistoryItems({...view,history:[{...view.history[1],toolCalls:[{id:"duplicate",name:"read",arguments:"{}"},{id:"duplicate",name:"read",arguments:"{}"}]}]},"owner",labels),/duplicate/);
console.log("Remote history projection: stable backend keys, scope isolation, tools/search/reasoning and no actions passed");
