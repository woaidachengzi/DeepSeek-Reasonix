import { useEffect, useRef } from "react";
import { ApprovalModal } from "../components/ApprovalModal";
import { AskCard } from "../components/AskCard";
import { MCPInteractionCard } from "../components/MCPInteractionCard";
import { createRemotePromptDecision, type RemotePromptScope } from "../lib/remotePromptDecision";
import type { RemotePendingPrompt } from "../lib/remoteConversationProjection";
import type { RemoteControllerLease } from "../lib/remoteControllerPool";
import { openTauriExternalURL, tauriSafeMCPURL } from "../lib/tauriBridge";

// Parent keys this host by surface + full captured prompt identity. Refresh
// retires the generation's command without remounting the same prompt's draft.
// Keep the request/schema stable across sampling commits so MCP form fields and
// Ask/plan drafts are not reset by display projection's defensive deep copies.
export function TauriRemotePrompt({lease,scope,prompt,disabled,generation,isCurrent,onOutcome,onStop,onLinkFailed}: {
  lease:RemoteControllerLease;scope:RemotePromptScope;prompt:RemotePendingPrompt;disabled:boolean;
  generation:number;
  isCurrent:()=>boolean;onOutcome:(outcome:"pending"|"sent"|"changed"|"unknown")=>void;
  onStop:()=>void;onLinkFailed:()=>void;
}) {
  const captured=useRef({scope:{...scope},prompt});
  const latest=useRef({isCurrent,onOutcome,disabled,onLinkFailed,generation});latest.current={isCurrent,onOutcome,disabled,onLinkFailed,generation};
  const mounted=useRef(true);
  const command=useRef<ReturnType<typeof createRemotePromptDecision>|null>(null);
  useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;command.current?.dispose();command.current=null;};},[generation]);
  const alive=()=>mounted.current&&latest.current.isCurrent();
  const decide=async(answer:Record<string,unknown>)=>{
    if(!alive()||latest.current.disabled)throw new Error("remote prompt unavailable");
    const sourcePredicate=latest.current.isCurrent,sourceGeneration=generation;
    command.current??=createRemotePromptDecision(lease,captured.current.scope,()=>mounted.current&&latest.current.generation===sourceGeneration&&sourcePredicate(),outcome=>latest.current.onOutcome(outcome));
    const result=await command.current.resolve(answer);
    if(result!=="sent")throw new Error("remote decision not confirmed");
  };
  const send=(answer:Record<string,unknown>)=>{void decide(answer).catch(()=>{});};
  const ownedStop=()=>{if(alive()&&!latest.current.disabled)onStop();};
  const request=captured.current.prompt;
  if(request.kind==="ask")return <AskCard ask={request.request} draftScope={JSON.stringify(captured.current.scope)} busy={disabled} preserveDraftOnStop controlledSubmission onStop={ownedStop} onAnswer={(_id,questions)=>decide({questions})}/>;
  if(request.kind==="mcp"){
    const safeURL=tauriSafeMCPURL(request.request.url);
    return <MCPInteractionCard interaction={request.request} busy={disabled} onAnswer={(_id,action,content)=>send(content===undefined?{action}:{action,content})} onOpenLink={safeURL?async()=>{
      if(!alive()||latest.current.disabled)throw new Error("remote link unavailable");
      const sourcePredicate=latest.current.isCurrent,sourceGeneration=generation;
      const owns=()=>mounted.current&&latest.current.generation===sourceGeneration&&sourcePredicate();
      try {await openTauriExternalURL(safeURL);if(!owns())throw new Error("remote link owner changed");}catch {if(owns())latest.current.onLinkFailed();throw new Error("remote link unavailable");}
    }:undefined}/>;
  }
  return <ApprovalModal approval={request.request} decisionPending={disabled} waitForConfirmation fileReferencesEnabled={false} onStop={ownedStop}
    onAnswer={(allow,session,persist)=>send(request.kind==="plan"?{action:allow?"start_execution":"exit_plan"}:{allow,session:session&&!persist,persist})}
    onRevisePlan={feedback=>send({action:"revise_plan",feedback})} onExitPlan={()=>send({action:"exit_plan"})}
    onResolveRecovery={(action,feedback)=>send(feedback===undefined?{action}:{action,feedback})}/>;
}
