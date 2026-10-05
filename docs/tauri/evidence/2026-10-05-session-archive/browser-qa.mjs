import assert from 'node:assert/strict';
import {mkdir,writeFile} from 'node:fs/promises';
import {createServer} from '/Users/jerry/temp/app/reasonix-tauri/desktop/frontend/node_modules/vite/dist/node/index.js';
import {chromium} from '/Users/jerry/temp/app/reasonix-tauri/desktop/frontend/node_modules/playwright/index.mjs';
import {load} from '/Users/jerry/temp/app/reasonix-tauri/desktop/frontend/scripts/tauri-bridge-stub-loader.mjs';
const root='/Users/jerry/temp/app/reasonix-tauri/desktop/frontend';
const output='/private/tmp/reasonix-session-archive-browser';await mkdir(output,{recursive:true});
const stub=(await load(root+'/src/lib/tauriBridge.ts',{},()=>{})).source.replace('globalThis.__archivedSessions=items;','globalThis.__archivedSessions=items; localStorage.setItem("archive-qa",JSON.stringify(items));');
const fixture=`import React from 'react';import {createRoot} from 'react-dom/client';import {TauriSessionApp} from '/src/tauri/TauriChatWorkspace.tsx';import '/src/styles.css';
globalThis.isTauri=true;
window.__TAURI_EVENT_PLUGIN_INTERNALS__={unregisterListener:()=>{}};
window.__TAURI__={core:{invoke:async()=>[]}};
window.__TAURI_INTERNALS__={invoke:async()=>[],transformCallback:()=>0,unregisterCallback:()=>{},metadata:{currentWindow:{label:'main'},currentWebview:{label:'main'}}};
globalThis.__workbenchSessions=[{sessionId:'archive-browser-first',title:'保留历史的对话'},{sessionId:'archive-browser-second',title:'另一个对话'}];
globalThis.__archivedSessions=JSON.parse(localStorage.getItem('archive-qa')??'[]');
globalThis.__tauriHistoryMessages=[{role:'user',content:'这条历史在归档后仍然保留'},{role:'assistant',content:'恢复后可以继续对话。'}];
createRoot(document.getElementById('root')).render(<TauriSessionApp/>);`;
const server=await createServer({root,configFile:root+'/vite.config.ts',server:{host:'127.0.0.1',port:5199,strictPort:true},plugins:[{
 name:'archive-browser-qa',enforce:'pre',
 resolveId(id){if(id==='/archive-qa.tsx'||id===root+'/archive-qa.tsx')return root+'/archive-qa.tsx';},
 load(id){if(id===root+'/archive-qa.tsx')return fixture;if(id.split('?')[0]===root+'/src/lib/tauriBridge.ts')return stub;},
 configureServer(s){s.middlewares.use('/archive-qa',async(req,res,next)=>{if(!['','/'].includes((req.url??'').split('?')[0]))return next();res.setHeader('Content-Type','text/html; charset=utf-8');res.end(await s.transformIndexHtml('/archive-qa','<!doctype html><html lang="zh-CN" data-theme="light"><head><meta charset="utf-8"><link rel="icon" href="data:,"><title>Reasonix 对话归档验收</title></head><body><div id="root"></div><script type="module" src="/archive-qa.tsx"></script></body></html>'));});}
}]});
let browser;const errors=[];const checks=[];
try{
 await server.listen();browser=await chromium.launch({headless:true,executablePath:'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'});
 const page=await browser.newPage({viewport:{width:1280,height:900}});
 page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(['error','warning'].includes(m.type()))errors.push(m.text());});
 await page.goto('http://127.0.0.1:5199/archive-qa');
 assert.equal(await page.title(),'Reasonix 对话归档验收');assert.equal(await page.locator('vite-error-overlay').count(),0);
 await page.getByRole('button',{name:'归档对话 保留历史的对话',exact:true}).waitFor();
 await page.locator('.tauri-sidebar__session').filter({hasText:'保留历史的对话'}).click();
 await page.getByText('这条历史在归档后仍然保留',{exact:true}).waitFor();
 await page.getByRole('button',{name:'归档对话 保留历史的对话',exact:true}).click();
 await page.getByRole('group',{name:'确认归档对话'}).getByRole('button',{name:'归档',exact:true}).click();
 await page.waitForFunction(()=>!document.querySelector('.tauri-sidebar__session')?.textContent?.includes('保留历史的对话'));
 assert.equal(await page.getByRole('button',{name:'归档对话 保留历史的对话',exact:true}).count(),0);
 assert.ok(!(await page.evaluate(()=>globalThis.__tauriBridgeCalls)).some(call=>call.name==='bridge_delete_session'));
 await page.reload();
 await page.getByRole('button',{name:'已归档对话 (1)',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'已归档对话'});await dialog.getByText('保留历史的对话',{exact:true}).waitFor();
 for(const viewport of [{width:1280,height:900},{width:900,height:760},{width:390,height:900}]){
  await page.setViewportSize(viewport);
  const geometry=await dialog.evaluate(el=>{const r=el.getBoundingClientRect();return {width:r.width,inside:r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight,overflow:el.scrollWidth>el.clientWidth};});
  assert.ok(geometry.inside&&!geometry.overflow,JSON.stringify(geometry));
  checks.push({viewport,...geometry});await page.screenshot({path:`${output}/archive-${viewport.width}.png`});
 }
 await page.setViewportSize({width:1280,height:900});
 await dialog.getByRole('button',{name:'恢复并打开',exact:true}).click();
 await page.getByText('这条历史在归档后仍然保留',{exact:true}).waitFor();
 assert.equal(await dialog.count(),0);assert.equal(await page.getByRole('button',{name:'归档对话 保留历史的对话',exact:true}).count(),1);
 await page.screenshot({path:`${output}/restored.png`});
 assert.deepEqual(errors,[]);
 const result={url:page.url(),title:await page.title(),browser:'Browser plugin not available; existing Playwright / headless installed Chrome',actualComponents:'TauriSessionApp / SessionRow; bridge responses and history simulated',checks,errors,cases:['archive current idle conversation','ordinary list hides committed archive','no permanent delete invoked','reload retains archive fixture state','responsive archived dialog','restore reopens original ID with history']};
 await writeFile(`${output}/results.json`,JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify(result));
}finally{await browser?.close();await server.close();}
