import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom = new JSDOM("<div id='root'></div>",{url:"http://localhost/"});
Object.assign(globalThis,{window:dom.window,document:dom.window.document,HTMLElement:dom.window.HTMLElement,Event:dom.window.Event,IS_REACT_ACT_ENVIRONMENT:true,isTauri:true});
Object.defineProperty(globalThis,"navigator",{value:dom.window.navigator,configurable:true});
let mode="ask", fail=false, catalogFail=false;
const calls:Array<{command:string;args:Record<string,unknown>}>=[];
Object.assign(dom.window,{__TAURI_INTERNALS__:{async invoke(command:string,args:Record<string,unknown>={}) {
 calls.push({command,args});
 if(command==="bridge_session_approval_mode") {if(args.mode){if(fail)throw new Error("mode write refused");mode=String(args.mode)}return mode}
 if(command==="provider_summary") { if(catalogFail) throw new Error("catalog unavailable"); return providers; }
 if(command==="desktop_preferences")return {defaultToolApprovalMode:"auto"};
 if(command==="set_desktop_approval")return {defaultToolApprovalMode:args.mode};
 throw new Error(command);
}}});
const React=await import("react");const {act}=React;const {createRoot}=await import("react-dom/client");
const {TauriComposerControls}=await import("../tauri/TauriComposerControls");
const root=createRoot(document.getElementById('root')!);
const errors:string[]=[],models:string[]=[];let disabled=false;
let providers={providers:[{name:"deepseek",displayName:"DeepSeek",configured:true,models:["chat","reasoner"]},{name:"missing",configured:false,models:["private"]}]} as never;
const render=async(sessionId:string|undefined="active")=>act(async()=>root.render(<TauriComposerControls sessionId={sessionId} model="deepseek/chat" providers={providers} disabled={disabled} refreshToken={false} onBusyChange={()=>{}} onError={error=>errors.push(error)} onModelChange={async model=>{models.push(model);return true}} onOpenModels={()=>{}}/>));
const click=async(text:string)=>act(async()=>{const b=[...document.querySelectorAll<HTMLButtonElement>('button')].find(b=>b.textContent?.startsWith(text));assert.ok(b,text);assert.equal(b.disabled,false);b.click()});
await render();await click('需要审批');fail=true;await click('自动审批');assert.equal(mode,'ask');assert.deepEqual(errors,['mode write refused']);assert.ok(document.querySelector('button[data-picker="permission"]')?.textContent?.includes('需要审批'));
fail=false;await click('自动审批');assert.equal(mode,'auto');assert.equal(calls.filter(c=>c.args.mode).every(c=>c.args.sessionId==='active'),true,'writes target exact active session');
await click('chat');assert.match(document.body.textContent!,/private/);assert.match(document.body.textContent!,/请填写 API Key/);assert.equal([...document.querySelectorAll<HTMLButtonElement>("button")].find(b=>b.textContent?.startsWith("private"))?.disabled,true);await click('reasoner');assert.deepEqual(models,['deepseek/reasoner']);
await click('chat');await act(async()=>document.dispatchEvent(new dom.window.KeyboardEvent('keydown',{key:'Escape',bubbles:true})));assert.equal(document.querySelector('[aria-label="模型列表"]'),null);
providers={providers:[{name:"mimo",displayName:"MiMo",configured:true,models:["mimo-v2.6-flash"]}]} as never;
await click('chat');assert.match(document.body.textContent!,/mimo-v2.6-flash/);assert.doesNotMatch(document.body.textContent!,/private/);
await click('mimo-v2.6-flash');assert.deepEqual(models,['deepseek/reasoner','mimo/mimo-v2.6-flash'],'newly saved catalog refreshes when picker reopens');
catalogFail=true;await click('chat');assert.match(document.body.textContent!,/模型刷新失败/);assert.equal([...document.querySelectorAll<HTMLButtonElement>('button')].find(b=>b.textContent?.startsWith('mimo-v2.6-flash'))?.disabled,true,'failed refresh cannot select a stale model');
await act(async()=>document.dispatchEvent(new dom.window.KeyboardEvent('keydown',{key:'Escape',bubbles:true})));
catalogFail=false;
disabled=true;await render();assert.equal(document.querySelector<HTMLButtonElement>('[data-picker="permission"]')!.disabled,true);
disabled=false;await render('');await click('自动审批');await click('需要审批');assert.ok(calls.some(c=>c.command==='set_desktop_approval'&&c.args.mode==='ask'));
await act(async()=>root.unmount());console.log('PASS active session approval, refusal, default scope, model route, disabled state and Escape');
