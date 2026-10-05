import assert from 'node:assert/strict';
import {mkdir,writeFile,readFile} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {createServer} from '/Users/jerry/temp/app/reasonix-tauri/desktop/frontend/node_modules/vite/dist/node/index.js';
import {chromium} from '/Users/jerry/temp/app/reasonix-tauri/desktop/frontend/node_modules/playwright/index.mjs';
import {load} from '/Users/jerry/temp/app/reasonix-tauri/desktop/frontend/scripts/tauri-bridge-stub-loader.mjs';
const mode=process.argv[2]??'after',repo='/Users/jerry/temp/app/reasonix-tauri',root=repo+'/desktop/frontend',output='/private/tmp/reasonix-harness-chat-browser';await mkdir(output,{recursive:true});
const stub=(await load(root+'/src/lib/tauriBridge.ts',{},()=>{})).source;
const date=new Date(),y=date.getFullYear(),m=date.getMonth(),d=date.getDate();
const yesterday=new Date(y,m,d-1,22,21).getTime(),old=new Date(y-1,m,d-1,22,21).getTime();
const fixture=`import React from 'react';import {createRoot} from 'react-dom/client';import {TauriSessionApp} from '/src/tauri/TauriChatWorkspace.tsx';import '/src/styles.css';
window.__TAURI_EVENT_PLUGIN_INTERNALS__={unregisterListener:()=>{}};window.__TAURI__={core:{invoke:async()=>[]}};
window.__TAURI_INTERNALS__={invoke:async(command,args)=>{if(command==='plugin:clipboard-manager|write_text')globalThis.__qaCopied=args.text;return [];},transformCallback:()=>0,unregisterCallback:()=>{},metadata:{currentWindow:{label:'main'},currentWebview:{label:'main'}}};
globalThis.isTauri=true;globalThis.__archivedSessions=[];globalThis.__workbenchSessions=[{sessionId:'harness-chat',title:'对话展示验收'}];
globalThis.__tauriHistoryMessages=[{role:'user',content:'所以是不是说 sock5 代理更好一些呢',createdAtMs:${yesterday}},{role:'assistant',content:'先核对相同端口、相同节点的两组结果。',workDurationMs:13000},{role:'assistant',content:'不是。实测否定了这个猜想。\\n\\n## 实验：同一个端口、同一个节点，只换协议\\n\\n| 协议 | 第一次 | 第二次 | 平均 |\\n| --- | --- | --- | --- |\\n| SOCKS5 | 0.234 MB/s | 0.215 MB/s | 0.225 |\\n| HTTP | 0.229 MB/s | 0.224 MB/s | 0.227 |',workDurationMs:47000},{role:'user',content:'再看一个没有中间过程的回答',createdAtMs:${old}},{role:'assistant',content:'这里保留完整答案，没有可展开的中间过程。',workDurationMs:47000}];
createRoot(document.getElementById('root')).render(<TauriSessionApp/>);`;
const baseline={};if(mode==='before')for(const rel of ['src/tauri/TauriChatWorkspace.tsx','src/tauri/historyPresentation.ts','src/tauri/tauriChatWorkspace.css'])baseline[root+'/'+rel]=execFileSync('git',['show','57a1b88b3:desktop/frontend/'+rel],{cwd:repo,encoding:'utf8'});
const server=await createServer({root,configFile:root+'/vite.config.ts',server:{host:'127.0.0.1',port:5197,strictPort:true},plugins:[{name:'harness-chat-qa',enforce:'pre',resolveId(id){if(id==='/chat-qa.tsx')return root+'/chat-qa.tsx';},load(id){const path=id.split('?')[0];if(path===root+'/chat-qa.tsx')return fixture;if(path===root+'/src/lib/tauriBridge.ts')return stub;if(baseline[path])return baseline[path];},configureServer(s){s.middlewares.use('/chat-qa',async(req,res,next)=>{if(!['','/'].includes((req.url??'').split('?')[0]))return next();res.setHeader('Content-Type','text/html; charset=utf-8');res.end(await s.transformIndexHtml('/chat-qa','<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><link rel="icon" href="data:,"><title>Reasonix Harness 对话展示验收</title></head><body><div id="root"></div><script type="module" src="/chat-qa.tsx"></script></body></html>'));});}}]});
let browser;const errors=[],checks=[];
try{
 await server.listen();browser=await chromium.launch({headless:true,executablePath:'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'});
 const page=await browser.newPage({viewport:{width:1280,height:900}});page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(['error','warning'].includes(m.type()))errors.push(m.text());});
 await page.goto('http://127.0.0.1:5197/chat-qa');assert.equal(await page.title(),'Reasonix Harness 对话展示验收');assert.equal(await page.locator('vite-error-overlay').count(),0);
 await page.locator('.tauri-sidebar__session').filter({hasText:'对话展示验收'}).click();await page.getByText('不是。实测否定了这个猜想。',{exact:true}).waitFor();
 await page.locator('.tauri-sidebar-toggle').click();
 for(const theme of ['dark','light'])for(const viewport of [{width:1280,height:900},{width:768,height:900},{width:390,height:900}]){
  await page.evaluate(t=>document.documentElement.setAttribute('data-theme',t),theme);await page.setViewportSize(viewport);
  const geometry=await page.locator('.tauri-transcript').evaluate(el=>{const root=el.getBoundingClientRect();return {overflow:el.scrollWidth>el.clientWidth,children:[...el.querySelectorAll('.tauri-message__meta, .tauri-progress > summary,.tauri-progress__summary')].map(child=>{const r=child.getBoundingClientRect();return {text:child.textContent,inside:r.left>=root.left-1&&r.right<=root.right+1};})};});
  assert.ok(!geometry.overflow&&geometry.children.every(c=>c.inside),JSON.stringify(geometry));checks.push({theme,viewport,...geometry});await page.screenshot({path:`${output}/${mode}-${theme}-${viewport.width}.png`});
 }
 if(mode==='after'){
  const process=page.locator('details.tauri-progress').first();assert.equal(await process.getAttribute('open'),null);
  assert.equal((await process.locator('summary').innerText()).trim(),'已完成，用时 47秒');
  await process.locator('summary').click();await page.getByText('先核对相同端口、相同节点的两组结果。',{exact:true}).waitFor({state:'visible'});
  await process.locator('summary').click();assert.ok(await page.getByText('先核对相同端口、相同节点的两组结果。',{exact:true}).isHidden());
  assert.ok(await page.getByText('不是。实测否定了这个猜想。',{exact:true}).isVisible());
  assert.equal(await page.locator('.tauri-progress--summary').count(),1);assert.equal(await page.locator('.tauri-progress--summary svg').count(),0);
  const time=await page.locator('.tauri-message.is-user time').first().innerText();assert.match(time,/\d+月\d+日 22:21/);
  assert.match(await page.locator('.tauri-message.is-user time').nth(1).innerText(),/\d{4}年\d+月\d+日 22:21/);
  const copy=page.locator('.tauri-message.is-user .tauri-message__meta button').first();await copy.click();await page.waitForFunction(()=>globalThis.__qaCopied==='所以是不是说 sock5 代理更好一些呢');
  assert.match(await copy.getAttribute('aria-label'),/已复制|Copied/);
  await page.setViewportSize({width:1280,height:900});await page.evaluate(()=>document.documentElement.setAttribute('data-theme','dark'));await page.screenshot({path:`${output}/after-interaction.png`});
 }
 assert.deepEqual(errors,[]);await writeFile(`${output}/${mode}.json`,JSON.stringify({url:page.url(),title:await page.title(),mode,browser:'Browser plugin not available; existing Playwright/headless installed Chrome',actualComponents:'TauriSessionApp + history renderer + shared CopyButton; simulated bridge, native clipboard write captured locally',checks,errors,interaction:mode==='after'?['expand/collapse process','final remains visible','single timed answer has non-interactive completion row','date-aware message time','copy original input + success icon']:[]},null,2)+'\n');
 console.log(JSON.stringify({mode,checks:checks.length,errors}));
}finally{await browser?.close();await server.close();}
