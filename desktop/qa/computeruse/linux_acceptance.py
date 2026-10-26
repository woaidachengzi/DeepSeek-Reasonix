#!/usr/bin/env python3
"""Real builtin/Go/Rust acceptance against this run's disposable GTK window."""
import argparse
import json
import os
from pathlib import Path
import selectors
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runner', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--allow-local-input', action='store_true')
    args = parser.parse_args()
    if not args.allow_local_input:
        parser.error('explicit local input consent is required')
    args.output.mkdir(parents=True, exist_ok=True)
    root = Path(tempfile.mkdtemp(prefix='gtk-', dir=args.output))
    fixture = runner = None
    log = (root/'evidence.jsonl').open('w')
    def record(value):
        log.write(json.dumps(value, ensure_ascii=False)+'\n')
        log.flush()
    try:
        fixture = subprocess.Popen(['/usr/bin/python3', str(Path(__file__).with_name('linux_fixture.py')), '--oracle', str(root/'oracle.json')], env={**os.environ, 'GDK_BACKEND':'x11'}, stdout=subprocess.DEVNULL, stderr=(root/'fixture.log').open('w'))
        deadline = time.monotonic()+60
        while not (root/'oracle.json').exists():
            if fixture.poll() is not None or time.monotonic() >= deadline:
                raise RuntimeError('GTK fixture did not start')
            time.sleep(.1)
        runner = subprocess.Popen([str(args.runner), '--pid', str(fixture.pid)], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=(root/'runner.log').open('w'), text=True)
        ready = selectors.DefaultSelector()
        ready.register(runner.stdout, selectors.EVENT_READ)
        def call(action, **fields):
            request = dict(action=action, **({} if action == 'status' else {'pid':fixture.pid}), **fields)
            runner.stdin.write(json.dumps(request, ensure_ascii=False)+'\n'); runner.stdin.flush()
            if not ready.select(45): raise TimeoutError(action)
            line = runner.stdout.readline()
            if not line: raise RuntimeError('acceptance host exited')
            reply = json.loads(line)
            record(dict(request=request, reply=reply))
            return reply
        def success(reply):
            assert not reply.get('error'), reply
            return reply['body']['state']
        status = success(call('status'))
        assert status['x11'] and status['atspi'], status
        window = None
        deadline = time.monotonic()+30
        while window is None:
            windows = success(call('windows'))
            if windows.get('windows'):
                candidates = [w for w in windows['windows'] if w.get('title') == 'Reasonix Cua Linux QA' and w.get('is_on_screen') is True]
                if candidates: window = candidates[0]['window_id']
            if time.monotonic() >= deadline: raise AssertionError('fixture window missing')
            if window is None: time.sleep(.2)
        def observe():
            reply = call('observe', window_id=window)
            state = success(reply)
            assert reply['image_count'] == 1, 'exact fixture screenshot missing'
            return state
        def token(state, label):
            matches = [e['element_token'] for e in state['elements'] if e.get('label') == label]
            assert len(matches) == 1, (label, state)
            return matches[0]
        state = observe()
        success(call('type', window_id=window, element_token=token(state, '验收文本'), text='Reasonix Linux 中文 2026'))
        state = observe()
        assert any(e.get('value') == 'Reasonix Linux 中文 2026' for e in state['elements']), state
        success(call('click', window_id=window, element_token=token(state, '完成本地验收')))
        state = observe()
        oracle = json.loads((root/'oracle.json').read_text())
        assert oracle == dict(text='Reasonix Linux 中文 2026', clicks=1), oracle
        assert '本地验收完成 1' in json.dumps(state, ensure_ascii=False), state
        # A changed PID is rejected by the fixture host before any controller call.
        runner.stdin.write(json.dumps(dict(action='observe',pid=fixture.pid+1,window_id=window))+'\n');runner.stdin.flush()
        assert ready.select(5)
        assert json.loads(runner.stdout.readline())['error'] == 'fixture scope refused'
        summary = dict(passed=True, pid=fixture.pid, window_id=window, status=status, oracle=oracle, checks=['actual-builtin-route','same-user-process-identity','independent-rust-identity','exact-window-screenshot','Chinese-input-readback','one-click-oracle','wrong-pid-refusal'])
        (root/'acceptance.json').write_text(json.dumps(summary, ensure_ascii=False, indent=2)+'\n')
        print(json.dumps(dict(passed=True, evidence=str(root)), ensure_ascii=False), flush=True)
    finally:
        if runner:
            runner.stdin.close()
            try: runner.wait(timeout=10)
            except subprocess.TimeoutExpired: runner.terminate(); runner.wait(timeout=5)
        if fixture:
            fixture.terminate()
            fixture.wait(timeout=5)
        log.close()


if __name__ == '__main__':
    main()
