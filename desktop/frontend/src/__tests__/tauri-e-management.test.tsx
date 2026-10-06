import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
const dom=new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>",{url:"http://localhost/",pretendToBeVisual:true});
Object.assign(globalThis,{window:dom.window,document:dom.window.document,HTMLElement:dom.window.HTMLElement,Event:dom.window.Event,MouseEvent:dom.window.MouseEvent,IS_REACT_ACT_ENVIRONMENT:true});
Object.defineProperty(globalThis,"navigator",{value:dom.window.navigator,configurable:true});
const scope=globalThis as typeof globalThis & {__botSettings:any;__botPairing:any;__tauriBridgeCalls:{name:string;args?:any}[];__remoteHosts:any[];__remoteConnectHandler?:(request:any)=>Promise<any>};
const settle=()=>new Promise<void>(resolve=>setTimeout(resolve,0));
const type=(input:HTMLInputElement|HTMLTextAreaElement,value:string)=>{Object.getOwnPropertyDescriptor(input instanceof dom.window.HTMLTextAreaElement?dom.window.HTMLTextAreaElement.prototype:dom.window.HTMLInputElement.prototype,"value")?.set?.call(input,value);input.dispatchEvent(new dom.window.Event("input",{bubbles:true}));};
function click(text:string){const button=[...document.querySelectorAll<HTMLButtonElement>("button")].find(button=>button.textContent?.trim()===text);assert.ok(button,`button ${text}`);assert.equal(button.disabled,false,`enabled ${text}`);button.click();}
function field(text:string){const label=[...document.querySelectorAll("label")].find(label=>label.textContent?.includes(text));const input=label?.querySelector<HTMLInputElement>("input");assert.ok(input,`field ${text}`);return input;}
async function main(){
 const React=await import("react");const {act}=React;const {createRoot}=await import("react-dom/client");const {LocaleProvider}=await import("../lib/i18n");const {TauriBotSettings}=await import("../tauri/TauriBotSettings");const {TauriRemoteSettings}=await import("../tauri/TauriRemoteSettings");
 let poll:()=>void=()=>{};window.setInterval=((fn:()=>void)=>{poll=fn;return 1;}) as typeof window.setInterval;window.clearInterval=()=>{};
 scope.__botSettings={protocolVersion:1,configPath:"/fixture/config.toml",enabled:false,accessControlConfigured:true,pairingEnabled:true,allowlistEnabled:true,allowAll:false,allowlist:Object.fromEntries(["qq","feishu","weixin","dingtalk"].map(p=>[p,{users:[],groups:[],approvers:[],admins:[]}])),channels:[],routes:[],selfUserIds:{qq:[],feishu:[],weixin:[],dingtalk:[]},queueMode:"steer",queueCap:20,queueDrop:"old",maxSteps:100,debounceMs:300,ignoreSelfMessages:true,pairingRequestTtlMinutes:60,pairingMaxPendingPerPlatform:3};
 scope.__botPairing={protocolVersion:1,requests:[{code:"PAIR",platform:"feishu",connection_id:"work",user_id:"applicant",chat_id:"chat",chat_type:"dm",expires_at:"2099-01-01T00:00:00Z",created_at:"2026-10-06T00:00:00Z"}]};
 const root=createRoot(document.getElementById("root")!);
 await act(async()=>{root.render(React.createElement(LocaleProvider,null,React.createElement(TauriBotSettings)));await settle();});
 await act(async()=>{click("Add connection");});
 await act(async()=>{type(field("Connection ID"),"work-a");type(field("Display name"),"Work account");type(field("App ID"),"fixture-id");});
 const secret=document.querySelector<HTMLInputElement>('input[type="password"]');assert.ok(secret);await act(async()=>{type(secret,"fixture-only-secret");});
 await act(async()=>{click("Save");await settle();});
 assert.ok(scope.__tauriBridgeCalls.some(call=>call.args?.change?.action==="create_connection"&&call.args.change.connection.id==="work-a"));assert.ok(!document.body.textContent?.includes("fixture-only-secret"));assert.ok(!document.querySelector('input[value="fixture-only-secret"]'));
 await act(async()=>{click("Remove connection");});assert.ok(document.body.textContent?.includes("unused app-owned credential"));await act(async()=>{click("Confirm removal");await settle();});assert.equal(scope.__botSettings.channels.length,0);
 await act(async()=>{click("Approve");});assert.ok(!scope.__tauriBridgeCalls.some(call=>call.name==="change_bot_pairing"));await act(async()=>{click("Confirm approval");await settle();});assert.ok(scope.__tauriBridgeCalls.some(call=>call.name==="change_bot_pairing"&&call.args.action==="approve"&&call.args.code==="PAIR"));
 // Runtime polling must never reload settings or erase route drafts.
 await act(async()=>{click("Add route");});
 const routeInput=document.querySelector<HTMLInputElement>('.tauri-bot-route input');assert.ok(routeInput);
 await act(async()=>{type(routeInput,"unsaved-chat-draft");});
 const before=scope.__tauriBridgeCalls.filter(call=>call.name==="bot_settings").length;
 await act(async()=>{poll();await settle();});assert.equal(scope.__tauriBridgeCalls.filter(call=>call.name==="bot_settings").length,before);assert.equal(routeInput.value,"unsaved-chat-draft");
 await act(async()=>{click("Restart connections");await settle();});assert.ok(document.body.textContent?.includes("Applying configuration"));assert.ok(scope.__tauriBridgeCalls.some(call=>call.name==="restart_bot_runtime"));
 await act(async()=>{root.unmount();});
 scope.__remoteHosts=[{name:"host",host:"fixture",port:22,user:"",identityFile:"",proxyJump:"",workspace:"",serveInstall:"auto",credentialMode:"remote",useSSHConfig:false,passwordSet:false,passphraseSet:false,connection:{protocolVersion:1,status:"connected",fingerprint:"SHA256:fixture"}}];
 const remoteRoot=createRoot(document.getElementById("root")!);await act(async()=>{remoteRoot.render(React.createElement(LocaleProvider,null,React.createElement(TauriRemoteSettings)));await settle();});assert.ok(document.body.textContent?.includes("SHA256:fixture"),"saved page reads current connection state");await act(async()=>{type(field("Forward ID"),"dev");type(field("Local port"),"12345");type(field("Remote host"),"127.0.0.1");type(field("Remote port"),"8080");});
 await act(async()=>{document.querySelector('.tauri-bot-channel-access form')?.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true}));await settle();});
 assert.ok(document.body.textContent?.includes("127.0.0.1:12345"));assert.ok(scope.__tauriBridgeCalls.some(call=>call.name==="remote_forwards"&&call.args.input.action==="add"));
 await act(async()=>{click("Disconnect");await settle();});assert.ok(!document.body.textContent?.includes("SHA256:fixture"));
 let finish:(value:any)=>void=()=>{};scope.__remoteConnectHandler=()=>new Promise(resolve=>{finish=resolve;});
 await act(async()=>{click("Connect");await settle();});await act(async()=>{click("Cancel");await settle();});await act(async()=>{finish({protocolVersion:1,status:"connected",fingerprint:"SHA256:stale"});await settle();});assert.ok(!document.body.textContent?.includes("SHA256:stale"),"late dial result ignored after cancellation");
 await act(async()=>{remoteRoot.unmount();});const {TauriPermissionsSettings}=await import("../tauri/TauriPermissionsSettings");
 const permissionScope=globalThis as any;
 const reads: {workspaceRoot?:string;resolve:(value:any)=>void}[]=[];
 permissionScope.__permissionReadHandler=(workspaceRoot?:string)=>new Promise(resolve=>reads.push({workspaceRoot,resolve}));
 const permissions=createRoot(document.getElementById("root")!);
 await act(async()=>{permissions.render(React.createElement(LocaleProvider,null,React.createElement(TauriPermissionsSettings,{workspaceRoot:"/fixture-project"})));await settle();});
 const globalRead=reads[0];assert.equal(globalRead.workspaceRoot,undefined);
 await act(async()=>{click("Current project");await settle();});
 assert.equal(reads[1].workspaceRoot,"/fixture-project");
 const view=(mode:string)=>({protocolVersion:1,mode,allow:[],ask:[],deny:[],scope:"project",projectOverrides:{mode:true,allow:false,ask:false,deny:false}});
 await act(async()=>{reads[1].resolve(view("deny"));await settle();});
 assert.equal(document.querySelector('[aria-label="Default writer decision"] [aria-checked="true"]')?.textContent,"deny (block writers)");
 await act(async()=>{globalRead.resolve(view("allow"));await settle();});
 assert.equal(document.querySelector('[aria-label="Default writer decision"] [aria-checked="true"]')?.textContent,"deny (block writers)","late global result cannot replace project rules");
 await act(async()=>{permissions.unmount();});
 process.stdout.write("PASS E connection CRUD, credential clearing, pairing confirmation, status-only polling, restart, reopened remote status and cancelled dial fencing\n");
}
main().catch(error=>{process.stderr.write(String(error)+"\n");process.exitCode=1;});
