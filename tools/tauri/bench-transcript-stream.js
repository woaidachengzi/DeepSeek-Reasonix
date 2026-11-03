// Real production Transcript through its fixed browser mock. No native input.
(async () => {
  const check = (value, message) => { if (!value) throw Error(message); };
  check(new URLSearchParams(location.search).get('mock') === 'bench' && !globalThis.isTauri && !globalThis.go?.main?.App,
    'stream fixture requires the isolated browser mock');
  const fixture = new URLSearchParams(location.search).get('streamFixture') || 'windowed';
  const label = fixture === 'markdown' ? 'bench:markdown-46t' : 'bench:windowed-1000t';
  const frame = () => new Promise(resolve => requestAnimationFrame(resolve));
  const wait = async predicate => {
    const until = performance.now() + 45000;
    while (performance.now() < until) { const result = predicate(); if (result) return result; await frame(); }
    throw Error(`stream fixture timeout (${window.__navigationPhase})`);
  };
  window.__navigationPhase = 'topic';
  const topic = await wait(() => [...document.querySelectorAll('.project-tree__topic-main')].find(e => e.textContent.includes(label)));
  topic.click();
  await wait(() => document.querySelector('.project-tree__topic--active')?.textContent.includes(label)
    && document.querySelector('.transcript')?.dataset.transcriptHydrating === 'false'
    && !document.querySelector('.transcript-navigation-overlay'));
  const writes = [];
  window.__REASONIX_TRANSCRIPT_SCROLL_WRITE__ = write => writes.push(write);
  if (fixture === 'windowed') {
    for (const ratio of [0.6, 0.2, 0]) {
      const rail = await wait(() => document.querySelector('.jump-scroll'));
      const before = writes.length, rect = rail.getBoundingClientRect();
      rail.dispatchEvent(new MouseEvent('mousedown', { button: 0, clientX: rect.left + rect.width / 2,
        clientY: rect.top + Math.max(1, rect.height * ratio), bubbles: true, cancelable: true }));
      document.dispatchEvent(new MouseEvent('mouseup', { button: 0, bubbles: true }));
      await wait(() => writes.slice(before).some(w => w.owner === 'question-jump' && ['accepted', 'native-clamp', 'no-op'].includes(w.outcome))
        && !document.querySelector('.transcript-navigation-overlay'));
      for (let i = 0; i < 8; i++) await frame();
    }
    check(Number(document.querySelector('.transcript').dataset.transcriptBlockCount) >= 900, 'long fixture was not loaded');
  }
  document.querySelector('.transcript__jump-bottom:not([hidden])')?.click();
  await wait(() => { const e = document.querySelector('.transcript'); return e && e.scrollHeight - e.scrollTop - e.clientHeight <= 4; });
  await wait(() => !window.__reasonixPerf?.stats().markdownWorker?.pending);
  for (let i = 0; i < 20; i++) await frame();
  const initialBlocks = Number(document.querySelector('.transcript').dataset.transcriptBlockCount);
  const input = await wait(() => document.querySelector('textarea.composer__input:not(.composer__input--measure)'));
  Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(input, 'local render performance audit');
  input.dispatchEvent(new Event('input', { bubbles: true }));
  const send = await wait(() => { const e = document.querySelector('.composer__btn--send'); return e && !e.disabled ? e : null; });
  const frameTimes = [], samples = [], changes = new Set();
  let active = true;
  const sample = time => {
    if (!active) return;
    frameTimes.push(time);
    const e = document.querySelector('.transcript'), rect = e.getBoundingClientRect();
    const blocks = [...e.querySelectorAll('[data-transcript-block-key]')];
    const shown = document.querySelector('[data-transcript-block-phase="active"] .msg--assistant .msg__body')?.textContent;
    if (shown) changes.add(shown);
    samples.push({ covered: blocks.some(b => { const r = b.getBoundingClientRect(); return r.bottom > rect.top && r.top < rect.bottom; }),
      mounted: blocks.length, completed: blocks.filter(b => b.dataset.transcriptBlockPhase === 'completed').length, mode: e.querySelector('.transcript__projection')?.dataset.transcriptRenderMode });
    requestAnimationFrame(sample);
  };
  window.__navigationPhase = 'stream-ready';
  window.__streamProbeReady = true;
  await wait(() => !window.__streamProbeControlled || window.__streamProbeGo);
  requestAnimationFrame(sample);
  const started = performance.now();
  send.click();
  await wait(() => document.querySelector('.composer__btn--stop'));
  await wait(() => !document.querySelector('.composer__btn--stop'));
  await wait(() => document.querySelector('.transcript')?.textContent.includes('hello from the mock'));
  for (let i = 0; i < 8; i++) await frame();
  active = false;
  const scroller = document.querySelector('.transcript');
  const bottomDistance = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight;
  check(bottomDistance <= 4, 'stream did not settle at the physical tail');
  const intervals = frameTimes.slice(1).map((t, i) => t - frameTimes[i]).sort((a, b) => a - b);
  check(changes.size >= 5, 'stream did not visibly progress');
  check(samples.length >= 20 && samples.every(s => s.covered), 'empty sampled viewport');
  check(samples.every(s => fixture !== 'windowed' || (s.mode === 'windowed' && s.completed <= 40 && s.mounted <= 41)), 'window mode/completed-block mount budget changed');
  check(document.visibilityState === 'visible', 'background WebView');
  check(!document.querySelector('vite-error-overlay'), 'framework overlay');
  const result = { passed: true, fixture, initialBlocks, scope: 'Synthetic browser mock stream in production Transcript; fixed timers and no real model/network or native input',
    url: location.href, title: document.title, viewport: [innerWidth, innerHeight], visibility: document.visibilityState, focused: document.hasFocus(),
    userAgent: navigator.userAgent, wallMs: performance.now() - started, bottomDistance, visibleTextChanges: changes.size, frameCount: samples.length,
    emptyFrames: samples.filter(s => !s.covered).length, maxMounted: Math.max(...samples.map(s => s.mounted)),
    rafIntervalMs: { p50: intervals[Math.floor(intervals.length * .5)], p95: intervals[Math.floor(intervals.length * .95)], max: intervals.at(-1), over50: intervals.filter(t => t > 50).length },
    console: window.__qaConsole || [], errors: window.__qaErrors || [] };
  window.__streamResult = result;
  window.webkit?.messageHandlers?.result?.postMessage(JSON.stringify(result));
})().catch(error => {
  const result = { passed: false, error: String(error), phase: window.__navigationPhase };
  window.__streamResult = result;
  window.webkit?.messageHandlers?.result?.postMessage(JSON.stringify(result));
});
