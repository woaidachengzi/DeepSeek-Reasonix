import type { BridgeRemoteControllerSessionPromptRequest } from "./bridgeProtocol.generated";
import type { RemoteControllerLease } from "./remoteControllerPool";

export type RemotePromptScope = Omit<BridgeRemoteControllerSessionPromptRequest,"answer">;
export type RemotePromptOutcome = "sent"|"changed"|"unknown"|"discarded"|"ignored";

// One captured displayed prompt owns one decision attempt. The card supplies
// an owner/generation + displayed-identity fence, never a foreground lookup.
// A fresh session read verifies liveness, not permission: the scoped backend
// still verifies the exact pending prompt and durable decision transition.
export function createRemotePromptDecision(
  lease:RemoteControllerLease,
  input:RemotePromptScope,
  isCurrent:()=>boolean,
  onOutcome:(outcome:"pending"|"sent"|"changed"|"unknown")=>void,
) {
  const scope:RemotePromptScope={sessionPath:input.sessionPath,runtimeEpoch:input.runtimeEpoch,
    turnId:input.turnId,promptId:input.promptId,promptRuntimeEpoch:input.promptRuntimeEpoch,kind:input.kind};
  let attempted=false,disposed=false;
  const alive=()=>!disposed&&isCurrent();
  const publish=(outcome:"pending"|"sent"|"changed"|"unknown")=>{if(alive())onOutcome(outcome);};
  const resolve=async(answer:Record<string,unknown>):Promise<RemotePromptOutcome>=>{
    if(!alive())return "discarded";
    if(attempted)return "ignored";
    attempted=true;
    let dispatched=false;
    publish("pending");
    try {
      // Copy the entire answer before the first await; mutable card drafts
      // cannot change what a user's click submitted while the read is pending.
      const message:BridgeRemoteControllerSessionPromptRequest={...scope,answer:structuredClone(answer)};
      if(!lease.sessionPrompt)throw new Error("unavailable remote decision");
      const fresh=await lease.sessionView(scope.sessionPath);
      if(!alive())return "discarded";
      const state=fresh.runtimeState;
      if(fresh.protocolVersion!==1||fresh.readOnly!==true||fresh.sessionPath!==scope.sessionPath||fresh.ownership!=="serve"
        ||!state||state.schemaVersion!==1||state.runtimeEpoch!==scope.runtimeEpoch||state.turnId!==scope.turnId
        ||state.phase!=="executing"||!state.running||!state.pendingPrompt||state.cancelRequested)throw new Error("changed remote prompt");
      dispatched=true;
      const receipt=await lease.sessionPrompt(message);
      if(!alive())return "discarded";
      if(receipt.protocolVersion!==1||receipt.resolved!==true
        ||receipt.sessionPath!==scope.sessionPath||receipt.runtimeEpoch!==scope.runtimeEpoch||receipt.turnId!==scope.turnId
        ||receipt.promptId!==scope.promptId||receipt.promptRuntimeEpoch!==scope.promptRuntimeEpoch||receipt.kind!==scope.kind)throw new Error("unconfirmed decision");
      // This is a decision receipt, not a turn/answer completion. Only the
      // existing prompt_answered/terminal stream removes the displayed card.
      publish("sent");return "sent";
    } catch {
      if(!alive())return "discarded";
      const outcome=dispatched?"unknown":"changed";
      publish(outcome);return outcome;
    }
  };
  return {resolve,dispose:()=>{disposed=true;}};
}
