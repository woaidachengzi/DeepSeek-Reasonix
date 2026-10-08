import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import type { MarkdownImageView } from "../lib/markdownImage";

const dom=new JSDOM("<body><div id='root'></div></body>",{url:"http://localhost/",pretendToBeVisual:true});
Object.assign(globalThis,{window:dom.window,document:dom.window.document,HTMLElement:dom.window.HTMLElement,MouseEvent:dom.window.MouseEvent,IS_REACT_ACT_ENVIRONMENT:true});
let localReads=0,localOpens=0,openerLists=0;
Object.assign(window,{go:{main:{App:{AttachmentDataURL:async()=>{localReads++;return "data:image/png;base64,legacy";},OpenLocalPath:async()=>{localOpens++;},ExternalOpeners:async()=>{openerLists++;return {openers:[]};}}}}});
const React=await import("react"); const {act}=React; const {createRoot}=await import("react-dom/client");
const {useAttachmentImageScope}=await import("../lib/useAttachmentImageScope");
const {RichMarkdownLink}=await import("../components/githubLink");
const {LocalPathActionContext}=await import("../components/SourceReferenceContext");
const root=createRoot(document.getElementById("root")!);
let scope!:ReturnType<typeof useAttachmentImageScope>; const intercepted:string[]=[];
const intercept=(path:string)=>{intercepted.push(path);};
function Probe({resolver,identity}:{resolver:((path:string)=>Promise<MarkdownImageView>)|null;identity:string}) {
  scope=useAttachmentImageScope(resolver,identity,".reasonix/attachments/shared.png");
  return <LocalPathActionContext.Provider value={intercept}><div data-preview={scope.urls[".reasonix/attachments/shared.png"] || ""}>
    <RichMarkdownLink href="file:///remote/private.txt">remote file</RichMarkdownLink>
    <RichMarkdownLink href="/remote/file.ts:12">remote code</RichMarkdownLink>
  </div></LocalPathActionContext.Provider>;
}
const flush=async()=>{for(let i=0;i<10;i++)await Promise.resolve();};
const render=async(resolver:((path:string)=>Promise<MarkdownImageView>)|null,identity:string)=>act(async()=>{root.render(<Probe resolver={resolver} identity={identity}/>);await flush();});
let finish!:(view:MarkdownImageView)=>void;
const old=()=>new Promise<MarkdownImageView>(resolve=>{finish=resolve;});
await render(old,"old-session"); const pending=scope.load(".reasonix/attachments/shared.png");
const refused=async()=>({url:"",errorCode:"not-authorized"}); await render(refused,"new-session");
await act(async()=>{finish({url:"data:image/png;base64,stale"});await flush();});
assert.equal(await pending,undefined); assert.equal(document.querySelector("[data-preview]")?.getAttribute("data-preview"),"");
assert.equal(localReads,0,"scoped refusal must not fall back to local attachment authority");
await render(async()=>{throw new Error("private error");},"failed-session"); await flush(); assert.equal(localReads,0);
await act(async()=>{
  for(const link of document.querySelectorAll("a")) {
    link.dispatchEvent(new dom.window.MouseEvent("click",{bubbles:true,cancelable:true}));
    link.dispatchEvent(new dom.window.MouseEvent("auxclick",{button:1,bubbles:true,cancelable:true}));
    const menu=new dom.window.MouseEvent("contextmenu",{bubbles:true,cancelable:true});link.dispatchEvent(menu);assert.ok(menu.defaultPrevented);
  }
});
assert.equal(intercepted.length,4); assert.equal(localOpens,0); assert.equal(openerLists,0);
await render(null,"legacy-local"); assert.equal(localReads,1,"unscoped legacy attachment path retains its existing API");
await act(async()=>root.unmount()); dom.window.close();
console.log("Remote media scope: late result fence, resolver refusal/no fallback, file click/middle/context-menu isolation and legacy compatibility passed");
