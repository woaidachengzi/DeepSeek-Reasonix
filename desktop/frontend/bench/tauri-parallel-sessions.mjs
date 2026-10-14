import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { createServer } from "vite";
import { chromium } from "playwright";
import { load } from "../scripts/tauri-bridge-stub-loader.mjs";

// Browser plugin not available. Render real components in isolated headless
// Chrome; only native bridge operations are simulated. No user profile is read.
const root = fileURLToPath(new URL("../", import.meta.url));
const output = process.env.REASONIX_PARALLEL_QA_OUTPUT ?? "/private/tmp/reasonix-parallel-browser";
await mkdir(output, { recursive: true });
const stub = (await load(path.join(root, "src/lib/tauriBridge.ts"), {}, () => {})).source;
const fixture = `
import React from 'react';
import { createRoot } from 'react-dom/client';
import { TauriSessionApp } from '/src/tauri/TauriChatWorkspace.tsx';
import '/src/styles.css';
import '/src/tauri/tauriChatWorkspace.css';
globalThis.isTauri=true;
globalThis.__TAURI_INTERNALS__={invoke:async command=>command==='workspace_external_openers'?[]:null,metadata:{currentWindow:{label:'main'},currentWebview:{label:'main'}},transformCallback:()=>1};
globalThis.__TAURI_EVENT_PLUGIN_INTERNALS__={unregisterListener:()=>{}};
globalThis.__workbenchSessions=['a','b'].map(id=>({sessionId:id,title:'Task '+id.toUpperCase(),workspaceRoot:'/tmp/parallel'}));
globalThis.__historyBySession={a:[{role:'user',content:'Question A'}],b:[{role:'user',content:'Question B'}]};
globalThis.__parallelSessionStates={a:'running',b:'idle'};
globalThis.__snapshotSequence=10;
globalThis.__parallelLiveStates={a:{active:true,answer:{segments:['A public running response'],streaming:true},progress:{phase:'answering',tools:[]},truncated:false}};
localStorage.setItem('reasonix-lang','zh');
createRoot(document.getElementById('root')).render(<TauriSessionApp/>);
`;
const server = await createServer({
  root, configFile: path.join(root, "vite.config.ts"), server: { host: "127.0.0.1", port: 5199, strictPort: true },
  plugins: [{ name: "parallel-session-qa", enforce: "pre",
    resolveId(id) { if (id === "/parallel-qa.tsx" || id === path.join(root, "parallel-qa.tsx")) return path.join(root, "parallel-qa.tsx"); },
    load(id) {
      const clean = id.split("?")[0];
      if (clean === path.join(root, "parallel-qa.tsx")) return fixture;
      if (clean === path.join(root, "src/lib/tauriBridge.ts")) return stub;
    },
    configureServer(s) { s.middlewares.use("/parallel-qa", async (req, res, next) => {
      if (!["", "/"].includes((req.url ?? "").split("?")[0])) return next();
      res.setHeader("Content-Type", "text/html; charset=utf-8");
      res.end(await s.transformIndexHtml("/parallel-qa", `<!doctype html><html lang='zh'><head><meta charset='utf-8'><meta name='viewport' content='width=device-width, initial-scale=1'><link rel='icon' href='data:,'><title>Reasonix parallel session QA</title></head><body><div id='root'></div><script type='module' src='/parallel-qa.tsx'></script></body></html>`));
    }); },
  }],
});
let browser;
let page;
const errors = [], checks = [], shots = [];
try {
  await server.listen();
  browser = await chromium.launch({ headless: true, executablePath: process.env.REASONIX_QA_CHROME ?? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" });
  page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, locale: "zh-CN" });
  page.on("pageerror", error => errors.push(error.message));
  page.on("console", message => { if (["error", "warning"].includes(message.type())) errors.push(message.text()); });
  await page.goto("http://127.0.0.1:5199/parallel-qa");
  assert.equal(await page.title(), "Reasonix parallel session QA");
  await page.locator(".tauri-shell").waitFor();
  const row = id => page.locator(".tauri-session-row").filter({ has: page.locator(".tauri-sidebar__session > span", { hasText: "Task " + id.toUpperCase() }) });
  await row("a").locator(".tauri-sidebar__session").click();
  await page.getByText("A public running response", { exact: true }).waitFor();
  assert.equal(await row("b").locator(".tauri-sidebar__session").isEnabled(), true);
  await page.locator(".tauri-composer textarea").fill("先分析，暂不修改文件");
  await page.getByRole("button", { name: "引导当前任务", exact: true }).click();
  await page.getByText("引导已提交，等待当前任务处理。", { exact: true }).waitFor();
  assert.equal(await page.locator(".tauri-composer textarea").inputValue(), "");
  assert(await page.evaluate(() => globalThis.__tauriBridgeCalls.some(call => call.name === "bridge_steer" && call.args.sessionId === "a")));
  assert(await page.getByText("A public running response", { exact: true }).isVisible());
  const steerBounds = await page.getByRole("button", { name: "引导当前任务", exact: true }).boundingBox();
  const stopBounds = await page.getByRole("button", { name: "停止生成", exact: true }).boundingBox();
  assert(steerBounds.x + steerBounds.width <= stopBounds.x, "steering and stop overlap");
  checks.push("running guidance preserves the answer and keeps a separate stop action");
  await page.locator(".tauri-composer textarea").fill("A unsent draft");
  await row("b").locator(".tauri-sidebar__session").click();
  await page.getByText("Question B", { exact: true }).waitFor();
  assert.equal(await page.locator(".tauri-composer textarea").inputValue(), "");
  await page.locator(".tauri-composer textarea").fill("B independent task");
  await page.getByRole("button", { name: "发送消息", exact: true }).click();
  await page.getByRole("button", { name: "停止生成", exact: true }).waitFor();
  assert(await page.evaluate(() => globalThis.__tauriBridgeCalls.some(call => call.name === "bridge_submit" && call.args.sessionId === "b")));
  checks.push("A remains selectable during B submission and drafts stay separate");
  await row("a").locator(".tauri-sidebar__session").click();
  await page.getByText("A public running response", { exact: true }).waitFor();
  assert.equal(await page.locator(".tauri-composer textarea").inputValue(), "A unsent draft");
  assert.equal(await page.evaluate(() => globalThis.__tauriBridgeCalls.filter(call => call.name === "bridge_cancel").length), 0);
  checks.push("returning to A restores public in-flight output without cancellation");
  await row("b").hover();
  const visibleBounds = await row("b").evaluate(element => {
    const badge = element.querySelector(".tauri-session-activity").getBoundingClientRect();
    const archive = element.querySelector(".tauri-session-row__archive").getBoundingClientRect();
    return { badgeRight: badge.right, archiveLeft: archive.left };
  });
  assert(visibleBounds.badgeRight <= visibleBounds.archiveLeft, "hover actions overlap activity badge");
  await page.mouse.move(900, 500);
  assert.equal(await row("b").locator(".tauri-session-row__delete").evaluate(element => getComputedStyle(element).opacity), "0");
  checks.push("background activity badge stays clear of disabled row actions");
  for (const theme of ["light", "dark"]) {
    await page.evaluate(theme => document.documentElement.setAttribute("data-theme", theme), theme);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
    const name = `parallel-${theme}-1440.png`;
    await page.screenshot({ path: path.join(output, name) }); shots.push(name);
  }
  await page.getByRole("button", { name: "停止生成", exact: true }).click();
  await page.waitForFunction(() => globalThis.__tauriBridgeCalls.some(call => call.name === "bridge_cancel" && call.args.sessionId === "a"));
  assert.equal(await page.evaluate(() => globalThis.__tauriBridgeCalls.filter(call => call.name === "bridge_cancel" && call.args.sessionId === "b").length), 0);
  checks.push("Stop A does not submit cancellation for B");
  assert.equal(await page.locator("vite-error-overlay").count(), 0);
  assert.deepEqual(errors, []);
  await writeFile(path.join(output, "report.json"), JSON.stringify({ checks, errors, shots, browser: "Browser plugin not available; existing Playwright and headless Chrome", nativeBridge: "isolated simulation; actual core parallelism is covered by Go controller tests" }, null, 2));
  console.log(JSON.stringify({ checks, errors, shots }));
} catch (error) {
  await writeFile(path.join(output, "failure.json"), JSON.stringify({ error: String(error), errors, body: await page?.locator("body").innerText() }, null, 2));
  await page?.screenshot({ path: path.join(output, "failure.png") });
  throw error;
} finally { await browser?.close(); await server.close(); }
