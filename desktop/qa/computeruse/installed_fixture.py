#!/usr/bin/env python3
"""Disposable local page for acceptance through the installed Reasonix chat UI.

Does not call Cua or drive page input. The operator submits the task in Reasonix;
page events independently report their postconditions to this loopback server.
"""
import argparse
import json
import os
from pathlib import Path
import signal
import subprocess
import threading
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--state-helper', type=Path, required=True)
    args = parser.parse_args()
    root = args.output.resolve()
    root.mkdir(parents=True, exist_ok=False)
    state_path = root / 'page-state.json'
    lock = threading.Lock()
    state = dict(text='', choice='', clicks=0, trusted_click=False, navigated=False)

    def save():
        state_path.write_text(json.dumps(state, ensure_ascii=False, indent=2)+'\n')
    save()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_): pass
        def send_headers(self, status, content_type):
            self.send_response(status)
            self.send_header('Content-Type', content_type)
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()
        def do_GET(self):
            if self.path == '/next':
                with lock:
                    state['navigated'] = True
                    save()
                body = '<!doctype html><meta charset="utf-8"><title>Reasonix Cua 导航验收完成</title><h1>本地导航验收完成</h1>'
                self.send_headers(200, 'text/html; charset=utf-8')
                self.wfile.write(body.encode())
            elif self.path == '/state':
                with lock:
                    body = json.dumps(state, ensure_ascii=False).encode()
                self.send_headers(200, 'application/json')
                self.wfile.write(body)
            else:
                self.send_headers(404, 'text/plain')
        def do_POST(self):
            if self.path != '/state':
                self.send_headers(404, 'text/plain')
                return
            size = int(self.headers.get('Content-Length', '0'))
            if not 0 < size < 4096:
                self.send_headers(400, 'text/plain')
                return
            payload = json.loads(self.rfile.read(size))
            if set(payload) != {'text','choice','clicks','trusted_click'}:
                self.send_headers(400, 'text/plain')
                return
            with lock:
                state.update(payload)
                save()
            self.send_headers(200, 'text/plain')
            self.wfile.write(b'ok')

    server = ThreadingHTTPServer(('127.0.0.1',0), Handler)
    port = server.server_port
    page = root / 'fixture.html'
    page.write_text('''<!doctype html><meta charset="utf-8"><title>Reasonix Cua Installed Acceptance</title>
<style>body{font:20px system-ui;max-width:900px;margin:70px auto}input,select,button{font:inherit;margin:16px;padding:12px}output{display:block;margin:16px}</style>
<h1>Reasonix Cua 安装版端到端验收</h1><p>仅本地测试，没有账号、邮件或外部提交。</p>
<label>验收文本<input aria-label="验收文本" oninput="report()"></label>
<label>验收选项<select aria-label="验收选项" onchange="report()"><option value="">请选择</option><option value="zh">中文选项</option></select></label>
<button onclick="clicks++;trusted=event.isTrusted;document.querySelector('output').textContent='本地验收完成 '+clicks;report()">完成本地验收</button>
<output>等待本地验收</output>
<script>let clicks=0,trusted=false;function report(){fetch('http://127.0.0.1:PORT/state',{method:'POST',body:JSON.stringify({text:document.querySelector('input').value,choice:document.querySelector('select').value,clicks:clicks,trusted_click:trusted})})}</script>
'''.replace('PORT',str(port)))
    threading.Thread(target=server.serve_forever,daemon=True).start()
    # Check a real HTTP response before asking the model to use the fixture.
    with urllib.request.urlopen(f'http://127.0.0.1:{port}/state',timeout=5) as response:
        assert response.status == 200 and json.load(response) == state
    pid = None
    stop = threading.Event()
    signal.signal(signal.SIGTERM,lambda *_:stop.set())
    signal.signal(signal.SIGINT,lambda *_:stop.set())
    try:
        launch = subprocess.run([str(args.state_helper),'--launch',str(root/'profile'),page.as_uri()],capture_output=True,text=True,check=True,timeout=20)
        pid = int(json.loads(launch.stdout)['pid'])
        info = None
        deadline = time.monotonic()+20
        while time.monotonic() < deadline:
            result = subprocess.run([str(args.state_helper),str(pid)],capture_output=True,text=True)
            if result.returncode == 0:
                info = json.loads(result.stdout)
                if info['windows']: break
            time.sleep(.1)
        if not info or not info['windows']:
            raise RuntimeError('fixture window unavailable')
        details = dict(pid=pid, window_id=info['windows'][0]['kCGWindowNumber'],url=page.as_uri(),next_url=f'http://127.0.0.1:{port}/next',state_path=str(state_path))
        (root/'fixture-info.json').write_text(json.dumps(details,indent=2)+'\n')
        print(json.dumps(details),flush=True)
        while not stop.wait(.5):
            os.kill(pid,0)
    finally:
        server.shutdown()
        server.server_close()
        if pid:
            try: os.kill(pid,signal.SIGTERM)
            except ProcessLookupError: pass


if __name__ == '__main__':
    main()
