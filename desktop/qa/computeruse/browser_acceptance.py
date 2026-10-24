#!/usr/bin/env python3
"""Explicit local DOM acceptance. Never connects to an existing browser/profile.

Run from the signed QA app after temporary Accessibility consent. Opens a fresh
Chrome user-data-dir and a local fixture only. The flag acknowledges that input
will be sent to this disposable page. No accounts, credentials or models used.
"""
import argparse
import json
import os
from pathlib import Path
import selectors
import shutil
import signal
import subprocess
import tempfile
import time

HTML = '''<!doctype html><meta charset="utf-8"><title>Reasonix Cua DOM QA</title>
<h1>Reasonix 本地 DOM 验收</h1>
<label>姓名<input aria-label="姓名" value="initial"></label>
<label>内容<textarea aria-label="内容"></textarea></label>
<button onclick="this.textContent='本地提交成功 1';document.querySelector('output').textContent='仅本地测试，已完成'">提交本地测试</button>
<output aria-live="polite">等待测试</output>'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runtime', type=Path, required=True)
    parser.add_argument('--state-helper', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--allow-local-input', action='store_true')
    args = parser.parse_args()
    if not args.allow_local_input:
        parser.error('explicit --allow-local-input and temporary host consent required')
    chrome = Path('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome')
    if not chrome.is_file():
        parser.error('a system Chrome installation is required')
    args.output.mkdir(parents=True, exist_ok=True)
    root = Path(tempfile.mkdtemp(prefix='dom-',dir=args.output))
    html = root / 'fixture.html'
    html.write_text(HTML)
    log = (root / 'evidence.jsonl').open('w')
    browser = worker = None
    sequence = 0
    target = None
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
            envelope = dict(protocol=1,id=sequence,request=request,approved_target=target,browser_profile_approved=approved)
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
        def observe(screenshot=False):
            r = call(dict(anchor,action='browser_observe',include_screenshot=screenshot))
            assert not r['is_error'] and r['result']['mode']=='snapshot', r
            return r['result']
        def ref(state,name):
            found = [r['ref'] for r in state['refs'] if r['name']==name]
            assert len(found)==1, 'fixture DOM ref not unique: '+name
            return found[0]
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
        clicked = call(dict(anchor,action='browser_click',ref=ref(observed,'提交本地测试'),delivery_mode='foreground'))
        assert not clicked['is_error']
        final = observe(True)
        assert any(r['name']=='本地提交成功 1' for r in final['refs']), 'click did not update fixture'
        assert any('本地提交成功 1' in json.dumps(v,ensure_ascii=False) for v in [final['outline'],final['refs']])
        # The adapter must reject a known tab paired with a different native window.
        refused = call(dict(anchor,action='browser_click',ref='p1:e1'),window_id=window+1)
        assert refused['is_error'] and refused['completion']=='not_started'
        record({'ok':True,'checks':['profile-consent','exact-tab','Chinese-replace-readback','no-duplicate','clear','DOM-click-readback','CDP-screenshot','cross-window-refusal']})
        print(json.dumps({'ok':True,'evidence':str(root/'evidence.jsonl')},ensure_ascii=False),flush=True)
    finally:
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
