import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import React, { act } from "react";
const dom = new JSDOM('<div id="root"></div>', {url:"http://localhost",pretendToBeVisual:true});
Object.assign(globalThis,{window:dom.window,document:dom.window.document,localStorage:dom.window.localStorage,Node:dom.window.Node,HTMLElement:dom.window.HTMLElement,requestAnimationFrame:dom.window.requestAnimationFrame.bind(dom.window),cancelAnimationFrame:dom.window.cancelAnimationFrame.bind(dom.window),IS_REACT_ACT_ENVIRONMENT:true});
window.HTMLDialogElement.prototype.showModal = function(){this.setAttribute("open","");};
const {createRoot} = await import("react-dom/client");
const {LocaleProvider} = await import("../lib/i18n");
const {TauriProviderEditor} = await import("../tauri/TauriProviderEditor");
await import("../components/ProviderModelDialog");
const {ProviderEditor, normalizeProviderView} = await import("../components/SettingsPanel");
const root = createRoot(document.getElementById("root")!);
const options = ["low","medium","high","xhigh","ultra"].map(id=>({id,name:id}));
const initial:any = {name:"custom",kind:"openai",baseUrl:"https://example.test/v1",models:["chat"],default:"chat",apiKeyEnv:"KEY",keySet:true,added:true,builtIn:false,visionModels:[],supportedEfforts:[],modelCapabilities:[{model:"chat",state:"unknown",source:"adapter",inputModalities:[],reasoning:{options}}],modelOverrides:[]};
assert.deepEqual(normalizeProviderView(initial).modelCapabilities?.[0].reasoning?.options,options,"normalization retains reasoning metadata");
let saved:any;
await act(async()=>root.render(<LocaleProvider><ProviderEditor initial={initial} kinds={["openai"]} busy={false} onSave={p=>{saved=p;}} onCancel={()=>{}}/></LocaleProvider>));
const click = async (selector:string) => act(async()=>{(document.querySelector(selector) as HTMLElement).click();});
const change = async (input:HTMLInputElement,value:string) => act(async()=>{Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype,"value")!.set!.call(input,value);input.dispatchEvent(new window.Event("input",{bubbles:true}));});
const open = async () => {await click('.provider-model-draft__option button');await act(async()=>{await new Promise(r=>setTimeout(r,30));});};
const submit = async()=>act(async()=>{document.querySelector('dialog form')!.dispatchEvent(new window.Event('submit',{bubbles:true,cancelable:true}));});
await open();
assert.equal(document.querySelectorAll('dialog .provider-model-dialog__effort-card input[type="checkbox"]').length,5);
await click('dialog input[aria-label="medium"]');
await act(async()=>{const select=document.querySelector('dialog .provider-model-dialog__effort-card select') as HTMLSelectElement;select.value="ultra";select.dispatchEvent(new window.Event('change',{bubbles:true}));});
await submit();assert.equal(saved,undefined,"dialog apply only changes draft");
await click('.provider-editor-footer .btn--primary');
assert.deepEqual(saved.modelOverrides[0].supportedEfforts,["low","high","xhigh","ultra"]);
assert.equal(saved.modelOverrides[0].defaultEffort,"ultra");
await open();await click('dialog button[aria-label="Reset reasoning effort"]');await submit();await click('.provider-editor-footer .btn--primary');
assert.deepEqual(saved.modelOverrides[0]?.supportedEfforts ?? [],[]);assert.equal(saved.modelOverrides[0]?.defaultEffort ?? "","");
await open();await click('dialog input[aria-label="medium"]');await act(async()=>document.querySelector('dialog')!.dispatchEvent(new window.Event('cancel',{cancelable:true})));assert.equal((document.querySelector('.provider-editor-footer .btn--primary') as HTMLButtonElement).disabled,true);
// Exercise the Tauri save payload, including manually declared adapter IDs.
const config:any = {name:"custom",displayName:"Custom",kind:"openai",models:["chat"],default:"chat",modelReasoning:[{model:"chat",reasoningProtocol:"openai",supportedEfforts:[],defaultEffort:"",options}],revision:"r1",removable:true};
let payload:any;
Object.assign(globalThis,{isTauri:true});
Object.assign(window,{__TAURI_INTERNALS__:{async invoke(command:string,args:any){if(command==="preview_provider_reasoning")return config.modelReasoning; if(command==="provider_configs")return {protocolVersion:1,providers:[config],presets:[]};if(command==="save_provider_config"){payload=args.input;return {protocolVersion:1,providers:[config],presets:[]};}if(command==="provider_summary")return {protocolVersion:1,providers:[]};throw new Error(command);}}});
await act(async()=>root.render(<LocaleProvider><TauriProviderEditor onSummaryChange={()=>{}}/></LocaleProvider>));
await click('.tauri-provider-editor-row button');
await act(async()=>{await new Promise(resolve=>setTimeout(resolve,200));});
const manual=document.querySelector('.tauri-provider-editor-form .provider-model-dialog__effort-card input.mem-input') as HTMLInputElement;
await change(manual,"max");await click('.tauri-provider-editor-form .provider-model-dialog__effort-card > button');
await act(async()=>{await new Promise(resolve=>setTimeout(resolve,200));});
await act(async()=>{const select=document.querySelector('.tauri-provider-editor-form .provider-model-dialog__effort-card select') as HTMLSelectElement;select.value="max";select.dispatchEvent(new window.Event('change',{bubbles:true}));});
await click('.tauri-provider-editor-form .tauri-settings-actions button');
assert.equal(payload.modelReasoning[0].defaultEffort,"max");assert(payload.modelReasoning[0].supportedEfforts.includes("max"));

// A response from the previous protocol must not replace the new vocabulary.
let completeOld!: (models: any[]) => void;
Object.assign(window,{__TAURI_INTERNALS__:{async invoke(command:string,args:any){
  if(command === "provider_configs") return {protocolVersion:1,providers:[config],presets:[]};
  if(command === "preview_provider_reasoning") {
    if(args.input.modelReasoning?.[0]?.reasoningProtocol === "glm") return [{...config.modelReasoning[0],options:["enabled","disabled"].map(id=>({id,name:id}))}];
    return new Promise<any[]>(resolve=>{completeOld=resolve;});
  }
  throw new Error(command);
}}});
await act(async()=>root.render(<LocaleProvider><TauriProviderEditor key="stale-protocol" onSummaryChange={()=>{}}/></LocaleProvider>));
await click('.tauri-provider-editor-row button');
await act(async()=>{await new Promise(resolve=>setTimeout(resolve,200));});
assert(completeOld,"old preview is in flight");
await act(async()=>{const select=document.querySelector('.tauri-provider-editor-form details details > label select') as HTMLSelectElement;select.value="glm";select.dispatchEvent(new window.Event('change',{bubbles:true}));});
await act(async()=>{await new Promise(resolve=>setTimeout(resolve,200));});
await act(async()=>completeOld(config.modelReasoning));
assert.equal(document.querySelector('input[aria-label="medium"]'),null,"old protocol response was discarded");
assert(document.querySelector('input[aria-label="enabled"]'));
await act(async()=>root.unmount());dom.window.close();console.log("PASS model reasoning: shared and Tauri drafts, levels, default, reset, cancel, save");
