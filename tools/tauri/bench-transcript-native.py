#!/usr/bin/env python3
"""Serial native WKWebView question-navigation baseline, synthetic data only.

Requires an existing production frontend build and macOS Swift/WebKit. Uses a
private nonpersistent WebView; no system input, permissions, or model calls.
"""
import argparse
import ctypes
import functools
import hashlib
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import plistlib
import signal
import subprocess
import sys
import threading
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
SOURCES = [Path(__file__), Path(__file__).with_suffix('.swift'), Path(__file__).with_suffix('.js')]


class StaticHandler(SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass


def stop_owned_probe(pid_file, executable):
    if not pid_file.exists():
        return
    pid = int(pid_file.read_text())
    library = ctypes.CDLL('/usr/lib/libproc.dylib')
    path = ctypes.create_string_buffer(4096)
    if library.proc_pidpath(pid, path, len(path)) > 0 and Path(os.fsdecode(path.value)).resolve() == executable.resolve():
        os.kill(pid, signal.SIGTERM)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--rounds', default=3, type=int)
    args = parser.parse_args()
    if sys.platform != 'darwin' or args.rounds < 1:
        parser.error('macOS and positive rounds required')
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False, mode=0o700)
    dist = ROOT / 'desktop/frontend/dist'
    if not (dist / 'index.html').is_file():
        parser.error('build desktop/frontend first; dependencies are not installed by this runner')
    hashes = {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in SOURCES}
    assets = {str(p.relative_to(dist)): hashlib.sha256(p.read_bytes()).hexdigest()
              for p in sorted(dist.rglob('*')) if p.is_file()}
    app = output / 'Reasonix Transcript Measurement.app'
    executable = app / 'Contents/MacOS/probe'
    executable.parent.mkdir(parents=True)
    (app / 'Contents/Info.plist').write_bytes(plistlib.dumps({
        'CFBundleIdentifier': 'io.reasonix.testing.transcript-measurement.' + uuid.uuid4().hex,
        'CFBundleName': 'Reasonix Transcript Measurement', 'CFBundleExecutable': 'probe',
        'CFBundlePackageType': 'APPL', 'CFBundleVersion': '1', 'NSHighResolutionCapable': True,
    }))
    env = {key: os.environ[key] for key in ('PATH', 'USER', 'LOGNAME', 'LANG', 'LC_ALL', 'TMPDIR') if key in os.environ}
    with (output / 'build.log').open('w') as log:
        subprocess.run(['swiftc', str(SOURCES[1]), '-o', str(executable)], check=True, env=env, stdout=log, stderr=log)
        subprocess.run(['codesign', '--force', '--sign', '-', str(app)], check=True, env=env, stdout=log, stderr=log)
    handler = functools.partial(StaticHandler, directory=str(dist))
    server = ThreadingHTTPServer(('127.0.0.1', 0), handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    url = f'http://127.0.0.1:{server.server_port}/?mock=bench&bench=1'
    samples = []
    try:
        for index in range(args.rounds):
            result = output / f'round-{index}.json'
            log = output / f'round-{index}.log'
            try:
                process = subprocess.Popen(['open', '-n', '-W', str(app), '--stdout', str(log), '--stderr', str(log),
                    '--args', str(SOURCES[2]), str(result), url], env=env)
                try:
                    deadline = time.monotonic() + 10
                    while not Path(str(result) + '.pid').exists() and process.poll() is None and time.monotonic() < deadline:
                        time.sleep(0.1)
                    # Request ordinary foreground presentation of our already
                    # started test app; sampling begins only after it paints.
                    subprocess.run(['open', str(app)], check=True, env=env, timeout=10)
                    process.wait(timeout=110)
                    if process.returncode:
                        raise RuntimeError(f'LaunchServices failed: {process.returncode}')
                finally:
                    if process.poll() is None:
                        process.terminate()
                        process.wait(timeout=10)
                sample = json.loads(result.read_text())
                if (not sample.get('passed') or sample.get('errors') or sample.get('console') or
                        sample.get('url') != url or sample.get('title') != 'Reasonix' or
                        not sample.get('focused') or not Path(str(result) + '.png').is_file()):
                    raise RuntimeError(f'native acceptance failed: {result}: {sample}')
                samples.append(sample)
            finally:
                stop_owned_probe(Path(str(result) + '.pid'), executable)
    finally:
        server.shutdown()
        server.server_close()
        thread.join()
    report = dict(samples=samples, source_sha256=hashes, asset_sha256=assets,
                  commit=subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
                  status=subprocess.check_output(['git', 'status', '--short'], cwd=ROOT, text=True).strip(),
                  scope='Native WKWebView shared production Transcript through browser mock adapter. Nine product question-navigation transactions per fresh nonpersistent WebView; DOM mouse events, not native input. rAF cadence includes probe/layout/scheduling, not paint/FPS. No Tauri shell IPC or model calls.',
                  swift=subprocess.check_output(['swiftc', '--version'], env=env, text=True).strip())
    (output / 'measurement.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps([dict(frames=s['frameCount'], blank=s['blankFrames'], maxMounted=s['maxMounted'],
        raf=s['rafIntervalMs'], navigationMs=[t['landingMs'] for t in s['transitions']]) for s in samples], indent=2))


if __name__ == '__main__':
    main()
