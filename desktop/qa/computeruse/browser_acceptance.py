#!/usr/bin/env python3
"""Explicit local DOM acceptance. Never connects to an existing browser/profile.

Run from the signed QA app after temporary Accessibility consent. Opens a fresh
Chrome user-data-dir and a local fixture only. The flag acknowledges that input
will be sent to this disposable page. No accounts, credentials or models used.
"""
import argparse
import functools
import http.server
import json
import os
from pathlib import Path
import selectors
import shutil
import signal
import subprocess
import tempfile
import threading
import time

HTML = '''<!doctype html><meta charset="utf-8"><title>Reasonix Cua DOM QA</title>
<h1>Reasonix 本地 DOM 验收</h1>
<label>姓名<input aria-label="姓名" value="initial"></label>
<label>内容<textarea aria-label="内容"></textarea></label>
<button onclick="this.textContent='本地提交成功 1';document.querySelector('output').textContent='仅本地测试，已完成'">提交本地测试</button>
<output aria-live="polite">等待测试</output>
<label>选项<select aria-label="选项" onchange="document.getElementById('selected').textContent='selected='+this.value">
<option value="">空选项</option><option value="zh">中文选项</option></select></label>
<output id="selected">selected=initial</output>
<button aria-label="指针目标" onmouseenter="document.getElementById('hover').textContent='hover=ok'"
oncontextmenu="event.preventDefault();document.getElementById('context').textContent='context=ok'"
ondblclick="document.getElementById('double').textContent='double=ok'">指针目标</button>
<output id="hover">hover=pending</output><output id="context">context=pending</output><output id="double">double=pending</output>
<div role="region" aria-label="滚动区域" style="height:90px;width:300px;overflow:auto"
onscroll="document.getElementById('scroll').textContent=this.scrollTop>0?'scroll=ok':'scroll=pending'">
<div style="height:800px">本地滚动内容</div></div><output id="scroll">scroll=pending</output>
<button aria-label="拖拽起点" onpointerdown="window.qaDragging=true">拖拽起点</button>
<button aria-label="拖拽终点" onpointerup="if(window.qaDragging)document.getElementById('drag').textContent='drag=ok';window.qaDragging=false">拖拽终点</button>
<output id="drag">drag=pending</output>'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runtime', type=Path, required=True)
    parser.add_argument('--state-helper', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--allow-local-input', action='store_true')
    parser.add_argument('--background-only', action='store_true', help='observe a separate fixture window for focus/pointer/input interference')
    parser.add_argument('--sentry-control', type=Path)
    args = parser.parse_args()
    if not args.allow_local_input:
        parser.error('explicit --allow-local-input and temporary host consent required')
    if args.background_only and (args.sentry_control is None or not args.sentry_control.is_dir()):
        parser.error('background acceptance requires the owned QA host sentry')
    chrome = Path('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome')
    if not chrome.is_file():
        parser.error('a system Chrome installation is required')
    args.output.mkdir(parents=True, exist_ok=True)
    root = Path(tempfile.mkdtemp(prefix='dom-',dir=args.output))
    html = root / 'fixture.html'
    html.write_text(HTML)
    (root / 'next.html').write_text('<!doctype html><meta charset="utf-8"><title>Reasonix Navigation QA</title><h1>本地导航完成</h1><button>新页面按钮</button>')
    class Handler(http.server.SimpleHTTPRequestHandler):
        def log_message(self, *_): pass
    server = http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(root)))
    server_thread = threading.Thread(target=server.serve_forever,daemon=True)
    server_thread.start()
    next_url = f'http://127.0.0.1:{server.server_port}/next.html'
    log = (root / 'evidence.jsonl').open('w')
    browser = worker = None
    sequence = 0
    target = None
    sentry = None
    parallel_started = None
    def sentry_state():
        try: return json.loads((args.sentry_control/'state.json').read_text())
        except (FileNotFoundError, json.JSONDecodeError): return {}
    def sentry_command(command):
        temporary = args.sentry_control/'command.tmp'
        temporary.write_text(command)
        temporary.replace(args.sentry_control/'command')
    def record(value):
        safe = dict(value)
        safe['images'] = [{'mime_type':i['mime_type'],'base64_bytes':len(i['data_base64'])} for i in value.get('images',[])]
        log.write(json.dumps(safe,ensure_ascii=False)+'\n'); log.flush()
    try:
        # LaunchServices returns the registered app PID, not Chrome's CLI launcher.
        launched = subprocess.run([str(args.state_helper),'--launch',str(root/'profile'),html.as_uri()],
                                  capture_output=True,text=True,check=True,timeout=20)
        browser = int(json.loads(launched.stdout)['pid'])
        deadline = time.monotonic()+20
        while time.monotonic()<deadline:
            info = subprocess.run([str(args.state_helper),str(browser)],capture_output=True,text=True)
            if info.returncode == 0:
                state = json.loads(info.stdout)
                windows = state['windows']
                if windows:
                    target = state['target']; window = windows[0]['kCGWindowNumber']; break
            os.kill(browser,0)
            time.sleep(.1)
        if target is None:
            raise RuntimeError('disposable browser window unavailable')
        worker = subprocess.Popen([str(args.runtime)],stdin=subprocess.PIPE,stdout=subprocess.PIPE,
                                  stderr=subprocess.DEVNULL,text=True,bufsize=1)
        def call(request, approved=False, window_id=window):
            nonlocal sequence
            sequence += 1
            request = dict(request,pid=target['pid'],window_id=window_id)
            envelope = dict(protocol=1,id=sequence,request=request,approved_target=target,browser_profile_approved=approved,foreground_approved=request.get('delivery_mode')=='foreground')
            worker.stdin.write(json.dumps(envelope,ensure_ascii=False)+'\n'); worker.stdin.flush()
            with selectors.DefaultSelector() as ready:
                ready.register(worker.stdout,selectors.EVENT_READ)
                if not ready.select(50): raise RuntimeError('Cua fixture response timed out')
            line = worker.stdout.readline()
            if not line: raise RuntimeError('Cua fixture worker ended')
            reply = json.loads(line); record(reply)
            assert reply['id']==sequence and reply['driver']=='cua'
            return reply
        # No profile authorization: must refuse without attaching or setup.
        denied = call({'action':'browser_prepare'})
        assert denied['is_error'] and denied['completion']=='not_started'
        prepared = call({'action':'browser_prepare','delivery_mode':'foreground'},True)
        if prepared.get('is_error'):
            raise RuntimeError('inspect preparation refusal in local evidence')
        bound = call({'action':'browser_observe'})
        assert bound['result']['status']=='ok' and bound['result']['mode']=='bind', bound
        binding = bound['result']
        tabs = [t for t in binding['tabs'] if t['url']==html.as_uri()]
        assert len(tabs)==1, 'fixture tab binding was not exact'
        anchor = {'target_id':binding['target_id'],'tab_id':tabs[0]['tab_id']}
        mode = 'background' if args.background_only else 'foreground'
        if args.background_only:
            sentry_command('begin')
            deadline = time.monotonic()+15
            while not sentry_state().get('monitoring'):
                if time.monotonic() >= deadline: raise RuntimeError('sentry did not begin monitoring')
                time.sleep(.1)
            assert sentry_state()['own_frontmost'], 'sentry was not foreground at the start'
            parallel_started = time.monotonic()
            print(json.dumps({'parallel_ready':True,'sentry':str(args.sentry_control)},ensure_ascii=False),flush=True)
        def observe(screenshot=False):
            r = call(dict(anchor,action='browser_observe',include_screenshot=screenshot))
            assert not r['is_error'] and r['result']['mode']=='snapshot', r
            return r['result']
        def ref(state,name):
            found = [r['ref'] for r in state['refs'] if r['name']==name]
            assert len(found)==1, 'fixture DOM ref not unique: '+name
            return found[0]
        def wait_for(text):
            deadline = time.monotonic()+5
            while True:
                state = observe()
                if text in json.dumps(state,ensure_ascii=False): return state
                if time.monotonic() >= deadline: raise AssertionError('fixture postcondition missing: '+text)
                time.sleep(.1)
        observed = observe()
        text = 'Reasonix DOM 中文验收 123'
        typed = call(dict(anchor,action='browser_type',ref=ref(observed,'姓名'),text=text,replace=True))
        assert not typed['is_error'], typed
        observed = observe()
        assert any(r['name']=='姓名' and r.get('value')==text for r in observed['refs']), 'DOM value readback mismatch'
        typed = call(dict(anchor,action='browser_type',ref=ref(observed,'姓名'),text=text,replace=True))
        assert not typed['is_error']
        observed = observe()
        assert any(r['name']=='姓名' and r.get('value')==text for r in observed['refs']), 'replacement duplicated input'
        cleared = call(dict(anchor,action='browser_type',ref=ref(observed,'姓名'),replace=True))
        assert not cleared['is_error']
        observed = observe()
        assert any(r['name']=='姓名' and r.get('value') in ('',None) for r in observed['refs']), 'clear failed'
        selected = call(dict(anchor,action='browser_select',ref=ref(observed,'选项'),label='中文选项'))
        assert not selected['is_error'] and selected['result']['changed'] and selected['result']['value']=='zh', selected
        observed = wait_for('selected=zh')
        selected = call(dict(anchor,action='browser_select',ref=ref(observed,'选项'),value=''))
        assert not selected['is_error'] and selected['result']['value']=='', selected
        observed = observe()
        selected = call(dict(anchor,action='browser_select',ref=ref(observed,'选项'),value=''))
        assert not selected['is_error'] and selected['result']['changed'] is False, selected
        for gesture,postcondition in [('hover','hover=ok'),('right_click','context=ok'),('double_click','double=ok')]:
            observed = observe()
            pointed = call(dict(anchor,action='browser_pointer',ref=ref(observed,'指针目标'),gesture=gesture,delivery_mode=mode))
            assert not pointed['is_error'], pointed
            wait_for(postcondition)
        observed = observe()
        scrolled = call(dict(anchor,action='browser_pointer',ref=ref(observed,'滚动区域'),gesture='scroll',delta_y=240,delivery_mode=mode))
        assert not scrolled['is_error'], scrolled
        wait_for('scroll=ok')
        observed = observe()
        dragged = call(dict(anchor,action='browser_pointer',ref=ref(observed,'拖拽起点'),destination_ref=ref(observed,'拖拽终点'),gesture='drag',delivery_mode=mode))
        assert not dragged['is_error'], dragged
        observed = wait_for('drag=ok')
        clicked = call(dict(anchor,action='browser_click',ref=ref(observed,'提交本地测试'),delivery_mode=mode))
        assert not clicked['is_error']
        final = observe(True)
        assert any(r['name']=='本地提交成功 1' for r in final['refs']), 'click did not update fixture'
        assert any('本地提交成功 1' in json.dumps(v,ensure_ascii=False) for v in [final['outline'],final['refs']])
        if args.background_only:
            # Continue real browser work while keys arrive in the independent
            # foreground fixture. Never raise either app or restore focus here.
            deadline = time.monotonic()+120
            expected = 'reasonixparallel2026'
            while True:
                sentry = sentry_state()
                assert sentry.get('monitoring') and sentry.get('own_frontmost'), sentry
                assert sentry['focus_changes']==0 and sentry['pointer_changes']==0, sentry
                if sentry['text']==expected and sentry['key_events']>=len(expected) and time.monotonic()-parallel_started>=5:
                    break
                if time.monotonic()>=deadline: raise AssertionError('parallel keyboard input incomplete: '+json.dumps(sentry))
                observed = observe()
                assert not any(expected in str(r.get('value','')) for r in observed['refs']), 'foreground text leaked into browser'
                pointed = call(dict(anchor,action='browser_pointer',ref=ref(observed,'指针目标'),gesture='hover',delivery_mode='background'))
                assert not pointed['is_error'], pointed
                time.sleep(.2)
        # The adapter must reject a known tab paired with a different native window.
        refused = call(dict(anchor,action='browser_click',ref='p1:e1'),window_id=window+1)
        assert refused['is_error'] and refused['completion']=='not_started'
        for action,fields in [('browser_navigate',{'url':next_url}),('browser_select',{'ref':'p1:e1','value':''}),('browser_pointer',{'ref':'p1:e1','gesture':'hover'})]:
            refused = call(dict(anchor,action=action,**fields),window_id=window+1)
            assert refused['is_error'] and refused['completion']=='not_started', refused
        old_ref = ref(final,'本地提交成功 1')
        navigated = call(dict(anchor,action='browser_navigate',url=next_url))
        assert not navigated['is_error'] and navigated['result']['refs_invalidated'], navigated
        stale = call(dict(anchor,action='browser_click',ref=old_ref,delivery_mode=mode))
        assert stale['is_error'] and 'browser_ref_stale' in json.dumps(stale), stale
        wait_for('本地导航完成')
        if args.background_only:
            sentry = sentry_state()
            assert sentry['focus_changes']==0 and sentry['pointer_changes']==0 and sentry['own_frontmost'] and sentry['text']=='reasonixparallel2026', sentry
        summary = {'ok':True,'delivery_mode':mode,'parallel':sentry,'checks':['profile-consent','exact-tab','Chinese-replace-readback','no-duplicate','clear','DOM-select-label','DOM-select-empty-value','DOM-select-no-op','trusted-hover','trusted-right-click','trusted-double-click','trusted-scroll','trusted-drag','DOM-click-readback','CDP-screenshot','cross-window-refusal','navigate-readback','navigation-invalidates-refs']}
        if args.background_only: summary['checks'] += ['foreground-retained','real-pointer-unchanged','parallel-keyboard-readback']
        record(summary)
        (root/'acceptance.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n')
        print(json.dumps({'ok':True,'evidence':str(root/'evidence.jsonl')},ensure_ascii=False),flush=True)
    finally:
        if args.background_only: sentry_command('end')
        server.shutdown()
        server.server_close()
        server_thread.join(timeout=5)
        if worker:
            worker.stdin.close()
            try: worker.wait(timeout=5)
            except subprocess.TimeoutExpired: worker.kill();worker.wait()
        if browser:
            try:
                os.kill(browser,signal.SIGTERM)
                deadline=time.monotonic()+5
                while time.monotonic()<deadline:
                    os.kill(browser,0);time.sleep(.1)
                os.kill(browser,signal.SIGKILL)
            except ProcessLookupError:
                pass
        shutil.rmtree(root / 'profile',ignore_errors=True)
        log.close()

if __name__=='__main__':
    main()
