// Uses product navigation transactions, never direct scrollTop/scrollTo writes.
(async () => {
  const check = (value, message) => { if (!value) throw Error(message); };
  const frame = () => new Promise(resolve => requestAnimationFrame(resolve));
  const wait = async predicate => {
    const until=performance.now()+30000;
    while(performance.now()<until){const value=predicate();if(value)return value;await frame();}
    throw Error('navigation fixture timeout');
  };
  const observations=[], frameTimes=[], transitions=[], writes=[], sampleCosts=[];
  window.__REASONIX_TRANSCRIPT_SCROLL_WRITE__=write=>writes.push(write);
  let active=false;
  const sample=time=>{
    if(!active)return;
    const started=performance.now();
    frameTimes.push(time);
    const e=document.querySelector('.transcript');
    const bounds=e.getBoundingClientRect();
    const blocks=[...e.querySelectorAll('[data-transcript-block-key]')];
    observations.push({top:e.scrollTop, mounted:blocks.length, covered:blocks.some(b=>{const r=b.getBoundingClientRect();return r.bottom>bounds.top&&r.top<bounds.bottom;})});
    sampleCosts.push(performance.now()-started);
    requestAnimationFrame(sample);
  };
  window.__navigationPhase="topic";
  const topic=await wait(()=>[...document.querySelectorAll('.project-tree__topic-main')].find(e=>e.textContent.includes('bench:windowed-1000t')));
  topic.click();window.__navigationPhase="topic-painted";
  await wait(()=>document.querySelector('.project-tree__topic--active')?.textContent.includes('bench:windowed-1000t')&&document.querySelector('.transcript')?.textContent.includes('Windowed turn 1000')&&!document.querySelector('.transcript-navigation-overlay'));
  window.__navigationPhase="windowed";
  await wait(()=>document.querySelector('.transcript__projection')?.dataset.transcriptRenderMode==='windowed');
  for(let i=0;i<15;i++)await frame();
  window.__navigationPhase="sampling";
  active=true;requestAnimationFrame(sample);
  for(const ratio of [0.9,0.7,0.5,0.3,0.1,0.3,0.5,0.7,0.9]){
    window.__navigationPhase='jump-'+ratio;
    const rail=document.querySelector('.jump-scroll');check(rail,'question rail missing');
    const e=document.querySelector('.transcript'), before=e.scrollTop, writeBoundary=writes.length;
    const beforeQuestion=Number(rail.getAttribute('aria-valuenow'));
    const started=performance.now(), r=rail.getBoundingClientRect();
    rail.dispatchEvent(new MouseEvent('mousedown',{button:0,clientX:r.left+r.width/2,clientY:r.top+r.height*ratio,bubbles:true,cancelable:true}));
    document.dispatchEvent(new MouseEvent('mouseup',{button:0,bubbles:true}));
    await wait(()=>writes.slice(writeBoundary).some(w=>w.owner==='question-jump'&&['accepted','native-clamp','no-op'].includes(w.outcome))&&!document.querySelector('.transcript-navigation-overlay'));
    const landingMs=performance.now()-started;
    for(let i=0;i<8;i++)await frame();
    const afterQuestion=Number(rail.getAttribute('aria-valuenow'));
    check(beforeQuestion!==afterQuestion,'question navigation identity did not change');
    transitions.push({ratio,landingMs,settledMs:performance.now()-started,from:before,to:e.scrollTop,beforeQuestion,afterQuestion});
  }
  active=false;
  const intervals=frameTimes.slice(1).map((t,i)=>t-frameTimes[i]).sort((a,b)=>a-b);
  const blanks=observations.filter(s=>!s.covered).length;
  check(transitions.length===9,'missing transitions');check(blanks===0,'uncovered frame');
  check(observations.every(s=>s.mounted<=40),'mount budget');
  check(document.visibilityState==='visible','background WebView');
  check(!document.querySelector('vite-error-overlay'),'framework overlay');
  window.webkit.messageHandlers.result.postMessage(JSON.stringify({passed:true,scope:'Native WKWebView shared production Transcript, synthetic history; product question navigator via DOM mouse events, no native wheel or model/IPC timing',url:location.href,title:document.title,viewport:[innerWidth,innerHeight],visibility:document.visibilityState,focused:document.hasFocus(),userAgent:navigator.userAgent,transitions,frameCount:observations.length,blankFrames:blanks,maxMounted:Math.max(...observations.map(s=>s.mounted)),rafIntervalMs:{p50:intervals[Math.floor(intervals.length*.5)],p95:intervals[Math.floor(intervals.length*.95)],max:intervals.at(-1),over50:intervals.filter(t=>t>50).length},probeCostMs:{max:Math.max(...sampleCosts),total:sampleCosts.reduce((a,b)=>a+b,0)},longTaskSupported:PerformanceObserver.supportedEntryTypes.includes('longtask'),console:window.__qaConsole??[],errors:window.__qaErrors??[]}));
})().catch(error=>window.webkit.messageHandlers.result.postMessage(JSON.stringify({passed:false,error:String(error),phase:window.__navigationPhase,stack:error.stack,state:{rows:document.querySelector('.transcript')?.dataset.transcriptRowCount,top:document.querySelector('.transcript')?.scrollTop,overlay:!!document.querySelector('.transcript-navigation-overlay')}})));
