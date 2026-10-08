import type { BridgeRemoteControllerSessionView } from "./bridgeProtocol.generated";
import type { Item } from "./useController";
import { historySearchAndAnswer } from "./searchTranscript";

// Shared Transcript presentation, but never its local hydration/actions. Every
// item key is namespaced backend identity; tool-result matching is turn-local.
export function remoteHistoryItems(view: BridgeRemoteControllerSessionView, surface: string, labels: {protocol:string;readiness:string}): Item[] {
  const items: Item[] = [];
  const calls = new Map<string,number>();
  const identities = new Set<string>();
  const key = (entry:string,part="entry",child="") => `remote:${JSON.stringify([surface,entry,part,child])}`;
  const append = (item:Item) => {
    if (identities.has(item.id)) throw new Error("invalid remote display identity");
    identities.add(item.id); items.push(item);
  };
  for (const message of view.history) {
    const id = key(message.id);
    if (message.role === "user") {
      calls.clear(); append({kind:"user",id,text:message.content});
    } else if (message.role === "assistant") {
      calls.clear();
      const serverSearch = message.serverSearch?.map(search => ({...search,sources_status:search.sources_status === "available" ? "available" as const : search.sources_status === "not_provided" ? "not_provided" as const : undefined}));
      for (const item of historySearchAndAnswer(id,{...message,serverSearch})) {
        append(item.kind === "tool" ? {...item,id:key(message.id,"search",item.id)} : item);
      }
      for (const call of message.toolCalls ?? []) {
        if (calls.has(call.id)) throw new Error("duplicate remote tool identity");
        calls.set(call.id,items.length);
        append({kind:"tool",id:key(message.id,"tool",call.id),name:call.name,args:call.arguments,readOnly:false,status:"stopped"});
      }
    } else if (message.role === "tool") {
      const index = message.toolCallId ? calls.get(message.toolCallId) : undefined;
      const call = index === undefined ? undefined : items[index];
      if (call?.kind === "tool") {
        items[index!] = {...call,output:message.content,status:"done"};
        calls.delete(message.toolCallId!);
      } else {
        append({kind:"tool",id,name:message.toolName || "tool",args:"",readOnly:false,status:"done",output:message.content});
      }
    } else if (message.role === "notice" || message.role === "protocol_recovery" || message.role === "final_readiness") {
      append({kind:"notice",id,level:"info",text:message.content || (message.role === "protocol_recovery" ? labels.protocol : labels.readiness),detail:message.missing?.join("\n")});
    }
    // System prompts are not conversation turns. No recovery action, local
    // archive lookup, provider raw replay, MCP app grant or live status inferred.
  }
  return items;
}
