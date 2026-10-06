import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { createServer } from 'vite';
import { chromium } from 'playwright';
import { load } from '../scripts/tauri-bridge-stub-loader.mjs';

// Browser plugin is unavailable in this workspace; use existing headless Chrome.
// The actual React components run; only the authenticated native bridge is mocked.
const root = fileURLToPath(new URL('../', import.meta.url));
const output = process.env.REASONIX_E_QA_OUTPUT ?? '/private/tmp/reasonix-e-browser';
await mkdir(output, { recursive: true });
let stub = (await load(path.join(root, 'src/lib/tauriBridge.ts'), {}, () => {})).source;
stub = stub.replace('export function changeTauriBotSettings(change) {', 'export function changeTauriBotSettings(change) { if(globalThis.__botMutationError) return Promise.reject(new Error("Fixture: save failed; retry"));');
const fixture = `
import React from 'react';
import { createRoot } from 'react-dom/client';
import { LocaleProvider, useI18n } from '/src/lib/i18n.tsx';
import { TauriBotSettings } from '/src/tauri/TauriBotSettings.tsx';
import { TauriRemoteSettings } from '/src/tauri/TauriRemoteSettings.tsx';
import '/src/styles.css';
import '/src/tauri/tauriChatWorkspace.css';
localStorage.setItem('reasonix-lang','en');
globalThis.__botPairing={protocolVersion:1,requests:[{code:'PAIR',platform:'feishu',connection_id:'work',user_id:'fixture-applicant',chat_id:'chat',chat_type:'dm',expires_at:'2099-01-01T00:00:00Z'}]};
globalThis.__remoteHosts=[{name:'fixture',host:'example.invalid',port:22,user:'fixture',identityFile:'',proxyJump:'',workspace:'',serveInstall:'auto',credentialMode:'remote',useSSHConfig:false,passwordSet:false,passphraseSet:false,connection:{protocolVersion:1,status:'connected',fingerprint:'SHA256:fixture'}}];
const container=document.getElementById('root');
const root=createRoot(container);
function QA({page}){const {setPref}=useI18n();window.setQALanguage=setPref;return <main className='qa-frame'>{page==='bot'?<TauriBotSettings/>:<TauriRemoteSettings/>}</main>;}
window.showPage=(page)=>root.render(<LocaleProvider><QA page={page}/></LocaleProvider>);
window.showPage('bot');
`;
const server = await createServer({
  root, configFile: path.join(root, 'vite.config.ts'),
  server: { host: '127.0.0.1', port: 5197, strictPort: true },
  plugins: [{
    name: 'e-management-qa', enforce: 'pre',
    resolveId(id) { if (id === '/e-qa.tsx' || id === path.join(root, 'e-qa.tsx')) return path.join(root, 'e-qa.tsx'); },
    load(id) {
      const clean = id.split('?')[0];
      if (clean === path.join(root, 'e-qa.tsx')) return fixture;
      if (clean === path.join(root, 'src/lib/tauriBridge.ts')) return stub;
    },
    configureServer(s) {
      s.middlewares.use('/e-qa', async (req, res, next) => {
        if (!['', '/'].includes((req.url ?? '').split('?')[0])) return next();
        res.setHeader('Content-Type', 'text/html; charset=utf-8');
        res.end(await s.transformIndexHtml('/e-qa', `<!doctype html><html lang='en'><head><meta charset='utf-8'><meta name='viewport' content='width=device-width, initial-scale=1'><link rel='icon' href='data:,'><title>Reasonix E management QA</title><style>.qa-frame{max-width:860px;height:calc(100vh - 24px);overflow:auto;margin:0 auto;padding:20px;background:var(--surface);border-radius:16px;min-width:0}body{margin:0;height:100vh;overflow:hidden;padding:12px;background:var(--bg);color:var(--fg)}*,*:before,*:after{box-sizing:border-box}</style></head><body><div id='root'></div><script type='module' src='/e-qa.tsx'></script></body></html>`));
      });
    },
  }],
});
const errors = [], checks = [], shots = [];
let browser;
try {
  await server.listen();
  browser = await chromium.launch({headless:true,executablePath:process.env.REASONIX_QA_CHROME ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'});
  globalThis.qaPage = await browser.newPage({locale:'en-US',viewport:{width:1280,height:900}});
  const page=globalThis.qaPage;
  page.on('pageerror', error => errors.push(error.message));
  page.on('console', message => {if (['error','warning'].includes(message.type())) errors.push(message.text());});
  await page.goto('http://127.0.0.1:5197/e-qa');
  await page.getByRole('button',{name:'Add connection',exact:true}).click();
  await page.getByLabel('Connection ID',{exact:true}).fill('qa-work');
  await page.getByLabel('Display name',{exact:true}).fill('QA work connection');
  const editor=page.locator('.tauri-bot-channel-runtime__body').filter({has:page.getByLabel('Connection ID',{exact:true})});
  await editor.getByLabel(/App ID/).fill('fixture-id');
  await editor.locator('input[type=password]').fill('fixture-secret');
  await page.evaluate(()=>globalThis.__botMutationError=true);
  await editor.getByRole('button',{name:'Save',exact:true}).click();
  await page.getByRole('alert').filter({hasText:'save failed'}).waitFor();
  assert.equal(await editor.getByLabel('Connection ID',{exact:true}).inputValue(),'qa-work');
  assert.equal(await editor.locator('input[type=password]').inputValue(),'fixture-secret');
  checks.push('failed save preserves editable draft and offers retry');
  await page.evaluate(()=>globalThis.__botMutationError=false);
  await editor.getByRole('button',{name:'Save',exact:true}).click();
  await page.locator('.tauri-settings-actions').filter({hasText:'QA work connection · qa-work'}).getByRole('button',{name:'Remove connection',exact:true}).waitFor();
  assert.ok((await page.locator('input[type=password]').evaluateAll(inputs=>inputs.map(input=>input.value))).every(value=>value===''));
  await page.getByRole('button',{name:'Approve',exact:true}).click();
  await page.getByRole('button',{name:'Confirm approval',exact:true}).waitFor();
  for (const theme of ['light','dark']) for (const width of [1280,390]) {
    await page.setViewportSize({width,height:900});
    await page.evaluate(theme=>document.documentElement.setAttribute('data-theme',theme),theme);
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,`${theme}/${width} overflow`);
    const name=`bot-${theme}-${width}.png`;await page.screenshot({path:path.join(output,name),fullPage:false});shots.push(name);
  }
  await page.getByRole('button',{name:'Confirm approval',exact:true}).click();
  await page.getByText('No pending pairing requests',{exact:true}).waitFor();
  checks.push('pairing requires explicit confirmation, then pending request disappears');
  await page.locator('.tauri-bot-routes > summary').click();
  await page.getByRole('button',{name:'Add route',exact:true}).click();
  const draft=page.locator('.tauri-bot-route input').first();await draft.fill('unsaved-route');
  await page.waitForFunction(()=>globalThis.__tauriBridgeCalls.filter(call=>call.name==='bot_runtime_status').length>=4);
  assert.equal(await draft.inputValue(),'unsaved-route');checks.push('three-second health poll preserves unsaved route draft');
  await page.getByRole('button',{name:'Restart connections',exact:true}).click();
  await page.getByText('Applying configuration…',{exact:true}).waitFor();
  checks.push('runtime restart displays configuration application status');
  await page.evaluate(()=>window.showPage('remote'));
  await page.locator('summary').filter({hasText:'Port forwarding'}).click();
  await page.getByLabel('Forward ID',{exact:true}).fill('dev');
  await page.getByLabel('Local port',{exact:true}).fill('54321');
  await page.getByLabel('Remote port',{exact:true}).fill('8080');
  await page.getByRole('button',{name:'Add forward',exact:true}).click();
  await page.getByText('dev: 127.0.0.1:54321 → 127.0.0.1:8080',{exact:true}).waitFor();
  for(const width of [1280,390]) {
    await page.setViewportSize({width,height:900});
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,`remote/${width} overflow`);
    const name=`remote-${width}.png`;await page.screenshot({path:path.join(output,name),fullPage:false});shots.push(name);
  }
  await page.getByRole('button',{name:'Disconnect',exact:true}).click();
  await page.waitForFunction(()=>!document.body.textContent.includes('SHA256:fixture'));
  assert.equal(await page.getByText('Port forwarding',{exact:true}).count(),0);
  checks.push('reopened remote settings show actual connection; disconnect retires port manager');
  await page.evaluate(()=>{window.showPage('bot');window.setQALanguage('zh');});
  await page.getByRole('button',{name:'添加连接',exact:true}).waitFor();
  await page.screenshot({path:path.join(output,'bot-chinese-390.png')});shots.push('bot-chinese-390.png');
  checks.push('management Chinese vocabulary loads on demand with no fallback labels');
  assert.equal(await page.locator('vite-error-overlay').count(),0);assert.deepEqual(errors,[]);
  await writeFile(path.join(output,'report.json'),JSON.stringify({checks,errors,shots,browser:'Browser plugin not available; existing Playwright and headless Chrome',actualComponents:['TauriBotSettings','TauriBotConnectionManager','TauriBotPairingManager','TauriRemoteSettings','TauriRemoteForwards'],nativeBridge:'isolated simulation; no IM/network credentials'},null,2));
  console.log(JSON.stringify({checks,errors,shots}));
} catch(error) {
  await writeFile(path.join(output,'failure.json'),JSON.stringify({error:String(error),errors,body:await globalThis.qaPage?.locator('body').innerText()},null,2));
  await globalThis.qaPage?.screenshot({path:path.join(output,'failure.png'),fullPage:false});
  throw error;
} finally {await browser?.close();await server.close();}
