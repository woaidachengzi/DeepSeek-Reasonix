// Browser plugin unavailable. Existing Playwright/installed Chrome only.
import { preview } from 'vite';
import { chromium } from 'playwright';
import { readFileSync, writeFileSync, mkdirSync, readdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const args = Object.fromEntries(process.argv.slice(2).reduce((pairs, value, i, all) => i % 2 ? pairs : [...pairs, [value.replace(/^--/, ''), all[i + 1]]], []));
if (!args.baseline || !args.output) throw Error('--baseline DIST --output DIRECTORY [--rounds N] required');
const output = path.resolve(args.output); mkdirSync(path.dirname(output), { recursive: true }); mkdirSync(output);
const rounds = Number(args.rounds || 3);
if (!Number.isInteger(rounds) || rounds < 1) throw Error('positive rounds required');
const scriptPath = path.resolve(root, '../../tools/tauri/bench-transcript-stream.js');
const script = readFileSync(scriptPath, 'utf8');
const directories = { baseline: path.resolve(args.baseline), candidate: path.resolve(args.candidate || path.join(root, 'dist')) };
const digest = file => createHash('sha256').update(readFileSync(file)).digest('hex');
const assetHashes = directory => Object.fromEntries(readdirSync(directory, { recursive: true, withFileTypes: true })
  .filter(entry => entry.isFile()).map(entry => { const file = path.join(entry.parentPath, entry.name); return [path.relative(directory, file), digest(file)]; }).sort(([a], [b]) => a.localeCompare(b)));
const assetSha256 = Object.fromEntries(Object.entries(directories).map(([route, directory]) => [route, assetHashes(directory)]));
const sourceNames = ['../src/lib/useTranscriptLiveFlags.ts', '../src/components/Transcript.tsx', './live-stream.mjs', '../../../tools/tauri/bench-transcript-stream.js'];
const sourceFiles = sourceNames.map(name => path.resolve(path.dirname(fileURLToPath(import.meta.url)), name));
const sourceSha256 = Object.fromEntries(sourceFiles.map(file => [path.relative(path.resolve(root, '../..'), file), digest(file)]));
if (directories.baseline === directories.candidate) throw Error('independent baseline/candidate builds required');
const servers = {}, urls = {}, samples = { baseline: [], candidate: [] }, warmup = [];
let browser;
try {
  for (const [route, dist] of Object.entries(directories)) {
    servers[route] = await preview({ root, build: { outDir: dist }, logLevel: 'silent', preview: { host: '127.0.0.1', port: route === 'baseline' ? 4187 : 4188, strictPort: true } });
    urls[route] = `http://127.0.0.1:${route === 'baseline' ? 4187 : 4188}/?mock=bench&bench=1`;
  }
  browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' });
  for (let round = -1; round < rounds; round++) for (const route of (round % 2 ? ['candidate', 'baseline'] : ['baseline', 'candidate'])) for (const fixture of ['windowed', 'markdown']) {
    const context = await browser.newContext({ viewport: { width: 1200, height: 800 } });
    const page = await context.newPage();
    const errors = [], blocked = [];
    page.on('pageerror', e => errors.push(String(e)));
    page.on('console', e => { if (['warning', 'error'].includes(e.type())) errors.push(e.text()); });
    await page.route('**/*', request => {
      const url = new URL(request.request().url());
      if (['http:', 'https:'].includes(url.protocol) && url.hostname !== '127.0.0.1') { blocked.push(url.origin); return request.abort(); }
      return request.continue();
    });
    try {
      await page.goto(urls[route] + '&streamFixture=' + fixture);
      await page.waitForSelector('.project-tree__topic-main');
      await page.evaluate(() => { window.__streamProbeControlled = true; });
      await page.evaluate(source => { void (0, eval)(source); }, script);
      await page.waitForFunction(() => window.__streamProbeReady || window.__streamResult?.passed === false, undefined, { timeout: 60000 });
      const failedSetup = await page.evaluate(() => window.__streamResult?.passed === false ? window.__streamResult : null);
      if (failedSetup) throw Error(JSON.stringify(failedSetup));
      const cdp = await context.newCDPSession(page); await cdp.send('Performance.enable');
      const before = await cdp.send('Performance.getMetrics');
      await page.evaluate(() => { window.__streamProbeGo = true; });
      await page.waitForFunction(() => window.__streamResult, undefined, { timeout: 60000 });
      const result = await page.evaluate(() => window.__streamResult);
      const after = await cdp.send('Performance.getMetrics');
      const old = Object.fromEntries(before.metrics.map(v => [v.name, v.value]));
      const metrics = Object.fromEntries(after.metrics.filter(v => /Duration$/.test(v.name)).map(v => [v.name, (v.value - (old[v.name] || 0)) * 1000]));
      const sample = { round, route, fixture, metricsMs: metrics, result, errors, blocked };
      writeFileSync(path.join(output, `${route}-${fixture}-${round}.json`), JSON.stringify(sample, null, 2));
      await page.screenshot({ path: path.join(output, `${route}-${fixture}-${round}.png`) });
      if (!result.passed || errors.length || blocked.length || await page.title() !== 'Reasonix') throw Error(JSON.stringify(sample));
      if (round < 0) warmup.push(sample); else samples[route].push(sample);
      console.log(`${route}/${fixture}/${round} script=${metrics.ScriptDuration.toFixed(1)}ms task=${metrics.TaskDuration.toFixed(1)}ms changes=${result.visibleTextChanges}`);
    } finally { await context.close(); }
  }
  const median = values => [...values].sort((a, b) => a - b)[Math.floor(values.length / 2)];
  const medians = Object.fromEntries(Object.entries(samples).map(([route, values]) => [route, Object.fromEntries(['windowed', 'markdown'].map(fixture => {
    const selected = values.filter(v => v.fixture === fixture);
    return [fixture, { scriptMs: median(selected.map(v => v.metricsMs.ScriptDuration)), taskMs: median(selected.map(v => v.metricsMs.TaskDuration)), rafP95Ms: median(selected.map(v => v.result.rafIntervalMs.p95)), emptyFrames: selected.reduce((n, v) => n + v.result.emptyFrames, 0), minVisibleTextChanges: Math.min(...selected.map(v => v.result.visibleTextChanges)) }];
  }))]));
  for (const file of sourceFiles) if (digest(file) !== sourceSha256[path.relative(path.resolve(root, '../..'), file)]) throw Error('source changed during measurement');
  for (const [route, directory] of Object.entries(directories)) if (JSON.stringify(assetHashes(directory)) !== JSON.stringify(assetSha256[route])) throw Error('build changed during measurement');
  const report = { baselineCommit: '6fbbeb2c87434d2e1b33cc29ca6e7959faf87e11', candidateCommit: execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim(), sourceSha256, assetSha256, scope: 'Serial alternating fresh Chrome contexts; production browser mock emits fixed-delay synthetic reply. CDP main-thread durations include renderer work after setup, not worker CPU, real model/network or native WKWebView. Warm-up discarded. Wall time is not model speed.', rounds, chrome: await browser.version(), probeSha256: createHash('sha256').update(script).digest('hex'), samples, warmup, medians };
  writeFileSync(path.join(output, 'comparison.json'), JSON.stringify(report, null, 2)); console.log(JSON.stringify(medians, null, 2));
} finally { await browser?.close(); for (const server of Object.values(servers)) await server.close(); }
