#!/usr/bin/env python3
"""Linux browser acceptance through the real builtin and a disposable profile."""
import argparse
import functools
import http.server
import json
import os
from pathlib import Path
import selectors
import signal
import subprocess
import tempfile
import threading
import time
from browser_acceptance import HTML


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runner', type=Path, required=True)
    parser.add_argument('--chromium', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--allow-local-input', action='store_true')
    args = parser.parse_args()
    if not args.allow_local_input:
        parser.error('explicit local fixture/profile consent is required')
    args.output.mkdir(parents=True, exist_ok=True)
    root = Path(tempfile.mkdtemp(prefix='browser-', dir=args.output))
    site = root/'site'; site.mkdir()
    (site/'fixture.html').write_text(HTML.replace("this.textContent='本地提交成功 1';", "fetch('/state',{method:'POST',body:JSON.stringify({trusted:event.isTrusted})});this.textContent='本地提交成功 1';"))
    (site/'next.html').write_text('<!doctype html><meta charset="utf-8"><title>Reasonix Linux Navigation QA</title><h1>本地导航完成</h1>')
    oracle = dict(clicks=0, trusted=False)
    class Handler(http.server.SimpleHTTPRequestHandler):
        def log_message(self, *_): pass
        def do_POST(self):
            if self.path != '/state': self.send_error(404); return
            data = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            oracle.update(clicks=oracle['clicks']+1, trusted=data['trusted'])
            (root/'oracle.json').write_text(json.dumps(oracle)+'\n')
            self.send_response(200);self.end_headers();self.wfile.write(b'ok')
    server = http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(site)))
    threading.Thread(target=server.serve_forever,daemon=True).start()
    url = f'http://127.0.0.1:{server.server_port}/fixture.html'
    next_url = f'http://127.0.0.1:{server.server_port}/next.html'
    log = (root/'evidence.jsonl').open('w')
    browser = runner = None
    def start_runner(approved):
        flags = ['--browser-profile-approved'] if approved else []
        return subprocess.Popen([str(args.runner),'--pid',str(browser.pid),*flags],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=(root/('approved.log' if approved else 'unapproved.log')).open('w'),text=True,start_new_session=True)
    def stop(process):
        if not process: return
        if process.stdin: process.stdin.close()
        if process.poll() is None:
            try: process.wait(timeout=8)
            except subprocess.TimeoutExpired: process.terminate()
        try: os.killpg(process.pid,signal.SIGTERM)
        except ProcessLookupError: pass
        process.wait(timeout=5)
    try:
        browser = subprocess.Popen([str(args.chromium),'--ozone-platform=x11','--no-first-run','--no-default-browser-check','--disable-dev-shm-usage','--force-renderer-accessibility','--remote-debugging-port=0',f'--user-data-dir={root/"profile"}',url],stdout=subprocess.DEVNULL,stderr=(root/'browser.log').open('w'),start_new_session=True)
        # The launcher may exec through a confined package wrapper. Wait for
        # this private profile's debugger file before pinning process identity.
        deadline = time.monotonic()+45
        while not (root/'profile/DevToolsActivePort').is_file():
            if browser.poll() is not None or time.monotonic() >= deadline:
                raise RuntimeError('sandboxed Chromium fixture did not start')
            time.sleep(.1)
        runner = start_runner(False)
        def call(action, **fields):
            request = dict(action=action,pid=browser.pid,**fields)
            runner.stdin.write(json.dumps(request,ensure_ascii=False)+'\n');runner.stdin.flush()
            selector=selectors.DefaultSelector();selector.register(runner.stdout,selectors.EVENT_READ)
            try:
                if not selector.select(45): raise TimeoutError(action)
                line=runner.stdout.readline()
            finally: selector.close()
            if not line: raise RuntimeError('acceptance host exited')
            reply=json.loads(line)
            log.write(json.dumps(dict(request=request,reply=reply),ensure_ascii=False)+'\n');log.flush()
            return reply
        def success(reply):
            assert not reply.get('error'), reply
            return reply['body']['state']
        deadline=time.monotonic()+20
        while True:
            windows=success(call('windows'))['windows']
            matches=[w for w in windows if 'Reasonix' in w.get('title','') and w.get('is_on_screen') is True]
            if matches: break
            if browser.poll() is not None or time.monotonic() >= deadline: raise RuntimeError('Chromium fixture window missing')
            time.sleep(.2)
        window=matches[0]['window_id']
        assert call('browser_prepare',window_id=window).get('error'), 'profile consent bypassed'
        stop(runner);runner=start_runner(True)
        success(call('browser_prepare',window_id=window))
        binding=success(call('browser_observe',window_id=window))
        tabs=[t for t in binding['tabs'] if t.get('url') == url]
        assert len(tabs)==1,binding
        anchor=dict(window_id=window,target_id=binding['target_id'],tab_id=tabs[0]['tab_id'])
        def observe(image=False):
            reply=call('browser_observe',**anchor,include_screenshot=image)
            state=success(reply)
            if image: assert reply['image_count']==1,'tab screenshot missing'
            return state
        def ref(state,name):
            matches=[r['ref'] for r in state['refs'] if r['name']==name]
            assert len(matches)==1,(name,state)
            return matches[0]
        def wait_for(text):
            deadline=time.monotonic()+5
            while True:
                state=observe()
                if text in json.dumps(state,ensure_ascii=False): return state
                if time.monotonic()>=deadline: raise AssertionError('postcondition missing: '+text)
                time.sleep(.1)
        state=observe(True)
        text='Reasonix Linux DOM 中文 2026'
        success(call('browser_type',**anchor,ref=ref(state,'姓名'),text=text,replace=True))
        state=wait_for(text)
        assert sum(r.get('value')==text for r in state['refs'])==1,state
        success(call('browser_select',**anchor,ref=ref(state,'选项'),label='中文选项'))
        state=wait_for('selected=zh')
        success(call('browser_select',**anchor,ref=ref(state,'选项'),value=''))
        state=observe()
        same=call('browser_select',**anchor,ref=ref(state,'选项'),value='')
        success(same);assert same['body'].get('no_change') is True and not same['body']['dispatched'],same
        # Linux refuses trusted background CDP pointer input before sending it.
        # Read the unchanged fixture before explicitly allowing foreground input
        # on this disposable X11 server; never retry a submit based on an error.
        state=observe()
        refused=call('browser_pointer',**anchor,ref=ref(state,'指针目标'),gesture='hover',delivery_mode='background')
        assert refused.get('error') and 'browser_input_trust_unavailable' in json.dumps(refused),refused
        state=observe()
        assert 'hover=pending' in json.dumps(state),state
        for gesture,postcondition in [('hover','hover=ok'),('right_click','context=ok'),('double_click','double=ok')]:
            state=observe()
            success(call('browser_pointer',**anchor,ref=ref(state,'指针目标'),gesture=gesture,delivery_mode='foreground'))
            wait_for(postcondition)
        state=observe()
        success(call('browser_pointer',**anchor,ref=ref(state,'滚动区域'),gesture='scroll',delta_y=240,delivery_mode='foreground'))
        wait_for('scroll=ok')
        state=observe()
        success(call('browser_pointer',**anchor,ref=ref(state,'拖拽起点'),destination_ref=ref(state,'拖拽终点'),gesture='drag',delivery_mode='foreground'))
        state=wait_for('drag=ok')
        success(call('browser_click',**anchor,ref=ref(state,'提交本地测试'),delivery_mode='foreground'))
        state=wait_for('本地提交成功 1')
        assert oracle==dict(clicks=1,trusted=True),oracle
        old=ref(state,'本地提交成功 1')
        wrong=dict(anchor,window_id=window+1)
        refused=call('browser_navigate',**wrong,url=next_url)
        assert refused.get('error') and not refused['body']['dispatched'],refused
        success(call('browser_navigate',**anchor,url=next_url))
        stale=call('browser_click',**anchor,ref=old,delivery_mode='foreground')
        assert stale.get('error') and 'browser_ref_stale' in json.dumps(stale),stale
        wait_for('本地导航完成')
        observe(True)
        summary=dict(passed=True,pid=browser.pid,window_id=window,oracle=oracle,checks=['actual-builtin-route','profile-consent-refusal','exact-tab','Chinese-replace-readback','select-label','select-empty','select-no-op','background-trust-refusal-readback','trusted-hover','trusted-right-click','trusted-double-click','trusted-scroll','trusted-drag','one-trusted-click-oracle','cross-window-refusal','navigate-readback','stale-ref-refusal','tab-screenshots'])
        (root/'acceptance.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n')
        print(json.dumps(dict(passed=True,evidence=str(root)),ensure_ascii=False),flush=True)
    finally:
        stop(runner)
        if browser:
            try: os.killpg(browser.pid,signal.SIGTERM)
            except ProcessLookupError: pass
            browser.wait(timeout=10)
        server.shutdown();server.server_close();log.close()


if __name__=='__main__':main()
