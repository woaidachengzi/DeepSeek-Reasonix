import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { createServer } from '/Users/jerry/temp/app/reasonix-tauri/desktop/frontend/node_modules/vite/dist/node/index.js';
import { chromium } from '/Users/jerry/temp/app/reasonix-tauri/desktop/frontend/node_modules/playwright/index.mjs';

const root = '/Users/jerry/temp/app/reasonix-tauri/desktop/frontend';
const phase = process.argv[2] ?? 'after';
const output = `/private/tmp/reasonix-approval-overflow-${phase}`;
await mkdir(output, { recursive: true });
const subject = `curl -s --max-time 15 "https://search-api-web.eastmoney.com/search/jsonp?cb=cb&param=${encodeURIComponent(JSON.stringify({ uid: '', keyword: '美联储 COMEX 黄金', type: ['cmsArticleWebOld'], client: 'web', pageSize: 20, pageIndex: 1 }))}"\n-H "Referer: https://so.eastmoney.com/" | python3 /tmp/em_search.py`;
const fixture = `import React, {useState} from 'react';
import {createRoot} from 'react-dom/client';
import {PromptCard} from '/src/tauri/TauriChatWorkspace.tsx';
import '/src/styles.css';
const subject = ${JSON.stringify(subject)};
const stress = new URLSearchParams(location.search).has('stress');
function Fixture() {
 const [result, setResult] = useState('');
 return <div className="tauri-shell" data-sidebar-hidden="true"><section className="tauri-main">
  <div className="tauri-conversation"><div style={{width:'min(820px, calc(100% - 48px))',margin:'auto auto 16px'}}><div className="tauri-message is-user"><div className="tauri-message__content">再分析下COMEX黄金</div></div><details className="tauri-progress"><summary>正在处理　展开查看当前输出</summary></details></div></div>
  {result ? <p role="status">{result}</p> : <PromptCard prompt={{kind:'approval',id:'qa',tool:stress?'bash-custom-tool-'.repeat(12):'bash',subject,reason:stress?'/workspace/'+'long-directory-'.repeat(24):''}} busy={false} selections={{}} onApproval={allow=>setResult(allow?'已允许':'已拒绝')} onAskSelection={()=>{}} onAskSubmit={()=>{}} onMCPAction={()=>{}} onOpenExternalURL={()=>{}}/>}
  <footer className="tauri-composer-area"><div className="tauri-composer" style={{minHeight:90}}>等待确认后继续…</div></footer>
 </section></div>;
}
createRoot(document.getElementById('root')).render(<Fixture/>);`;
const server = await createServer({ root, configFile: `${root}/vite.config.ts`, server: { host:'127.0.0.1', port:5198, strictPort:true }, plugins:[{
 name:'approval-qa-only', enforce:'pre',
 resolveId(id) { if(id==='/approval-qa.tsx' || id===`${root}/approval-qa.tsx`) return `${root}/approval-qa.tsx`; },
 load(id) { if(id===`${root}/approval-qa.tsx`) return fixture; },
 transform(code,id) { if(id.split('?')[0]===`${root}/src/tauri/TauriChatWorkspace.tsx`) return `${code}\nexport { PromptCard };`; },
 configureServer(s) { s.middlewares.use('/approval-qa', async (req,res,next)=>{ if((req.url??'').split('?')[0]!=='/' && (req.url??'').split('?')[0]!=='') return next(); res.setHeader('Content-Type','text/html; charset=utf-8'); res.end(await s.transformIndexHtml('/approval-qa', '<!doctype html><html lang="zh-CN" data-theme="light"><head><meta charset="utf-8"><link rel="icon" href="data:,"><title>Reasonix 审批卡片验证</title></head><body><div id="root"></div><script type="module" src="/approval-qa.tsx"></script></body></html>')); }); }
}] });
let browser;
const errors=[]; const checks=[];
try {
 await server.listen();
 browser=await chromium.launch({headless:true,executablePath:'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'});
 const page=await browser.newPage();
 page.on('pageerror', e=>errors.push(e.message));
 page.on('console', msg=>{if(['error','warning'].includes(msg.type())) errors.push(msg.text());});
 for(const viewport of [{width:1280,height:900},{width:768,height:900},{width:390,height:900}]) {
  await page.setViewportSize(viewport);
  await page.goto('http://127.0.0.1:5198/approval-qa');
  await page.getByRole('region',{name:'等待权限确认'}).waitFor();
  assert.equal(await page.title(),'Reasonix 审批卡片验证');
  assert.match(page.url(), /127\.0\.0\.1:5198\/approval-qa$/);
  assert.equal(await page.locator('vite-error-overlay').count(),0);
  assert.equal(await page.locator('.tauri-prompt-card__subject').textContent(),subject);
  const measure = () => page.evaluate(()=>{
   const card=document.querySelector('.tauri-prompt-card'),subject=card.querySelector('.tauri-prompt-card__subject');
   const rect=card.getBoundingClientRect();const range=document.createRange();range.selectNodeContents(subject);
   const lines=[...range.getClientRects()];
   return {cardWidth:rect.width,subjectWidth:subject.clientWidth,subjectScrollWidth:subject.scrollWidth,
    overflow:Math.max(0,...lines.map(r=>r.right-(rect.right-15)),...lines.map(r=>rect.left+15-r.left)),
    documentOverflow:document.documentElement.scrollWidth>innerWidth,
    buttonsInside:[...card.querySelectorAll('button')].every(b=>{const r=b.getBoundingClientRect();return r.left>=rect.left&&r.right<=rect.right&&r.top>=rect.top&&r.bottom<=rect.bottom&&r.bottom<=innerHeight;}),
    lineCount:lines.length};
  });
  const geometry=await measure();
  await page.screenshot({path:`${output}/${viewport.width}.png`});
  checks.push({viewport,...geometry});
  if(phase==='before') { assert.ok(geometry.overflow>1,'baseline must reproduce long URL overflow'); continue; }
  assert.ok(geometry.overflow<=1, JSON.stringify(geometry));
  assert.ok(!geometry.documentOverflow && geometry.buttonsInside);
  assert.ok(geometry.subjectScrollWidth<=geometry.subjectWidth+1);
  const selected=await page.locator('.tauri-prompt-card__subject').evaluate(el=>{const range=document.createRange();range.selectNodeContents(el);const selection=window.getSelection();selection.removeAllRanges();selection.addRange(range);return selection.toString();});
  assert.equal(selected,subject,'visual wrapping must not change copied command');
  await page.getByRole('button',{name:'允许一次',exact:true}).click();
  assert.equal(await page.getByRole('status').textContent(),'已允许');
  await page.reload();
  await page.getByRole('button',{name:'拒绝',exact:true}).click();
  assert.equal(await page.getByRole('status').textContent(),'已拒绝');
  await page.goto('http://127.0.0.1:5198/approval-qa?stress');
  await page.getByRole('region',{name:'等待权限确认'}).waitFor();
  const stressGeometry=await page.locator('.tauri-prompt-card').evaluate(card=>{const r=card.getBoundingClientRect();return [...card.querySelectorAll('strong,span,p,button')].every(el=>{const range=document.createRange();range.selectNodeContents(el);return [...range.getClientRects()].every(line=>line.left>=r.left&&line.right<=r.right);});});
  assert.ok(stressGeometry,'long tool name and reason stay inside the card');
  await page.evaluate(()=>document.documentElement.dataset.theme='dark');
  await page.screenshot({path:`${output}/${viewport.width}-stress-dark.png`});
 }
 assert.deepEqual(errors,[]);
 await writeFile(`${output}/results.json`,JSON.stringify({phase,browser:'Chromium / installed Google Chrome (headless)',browserPath:'Browser plugin not available; existing Playwright',url:'http://127.0.0.1:5198/approval-qa',component:'actual PromptCard; fixture exports via Vite transform only; host callbacks simulated',subject,checks,errors},null,2)+'\n');
 console.log(JSON.stringify({phase,checks,errors}));
} finally {await browser?.close();await server.close();}
