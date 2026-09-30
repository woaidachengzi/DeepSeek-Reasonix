#!/usr/bin/env python3
"""macOS official Wails 1.38.3 history rollback across real Preview import/restarts.

Requires authenticated official CLI and Desktop binaries; fixed digests reject
local modified builds. Uses a fake loopback provider and private profiles.
Checks original history, live session writer refusal and native quit, not UI
clicks, physical keys, full historical data or host directory lifetime locks.
"""
import argparse
import hashlib
import http.server
import importlib.util
import json
import os
from pathlib import Path
import platform
import plistlib
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time
import uuid
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('profiles', Path(__file__).with_name('smoke-profile-import.py'))
profiles = importlib.util.module_from_spec(spec)
spec.loader.exec_module(profiles)

def verify_official_legacy(app, cli):
    expected = {
        cli: '49be12faf150879a1da58fb539871c26b6077fe2d45ffe3ad105bfb541d5a091',
        app / 'Contents/MacOS/reasonix-desktop':
            '869b02d8f8a92f5847c1728923fde7153d931e105f26fd2926bfa8feeb863650',
        app / 'Contents/MacOS/reasonix':
            '5b1ab31424d45c8bf3cfe6a60b11da527df2252aee963d1c0c56352f57945962',
    }
    for path, digest in expected.items():
        if not path.is_file() or path.is_symlink():
            raise RuntimeError('Legacy artifact is not a regular file')
        hasher = hashlib.sha256()
        with path.open('rb') as binary:
            for chunk in iter(lambda: binary.read(1024 * 1024), b''):
                hasher.update(chunk)
        if hasher.hexdigest() != digest:
            raise RuntimeError('Legacy binary differs from authenticated official 1.38.3 artifact')
    # This authenticated legacy plist starts with DOCTYPE, without an XML declaration.
    info = plistlib.loads((app / 'Contents/Info.plist').read_bytes(), fmt=plistlib.FMT_XML)
    if info.get('CFBundleIdentifier') != 'com.wails.reasonix-desktop' or info.get('CFBundleShortVersionString') != '1.38.3':
        raise RuntimeError('Legacy bundle identity/version differs')

def compile_quit_helper(root):
    source = root / 'quit.swift'
    source.write_text('''import AppKit
import Foundation
let pid = pid_t(CommandLine.arguments[1])!
let expected = URL(fileURLWithPath: CommandLine.arguments[2]).standardizedFileURL
guard let app = NSRunningApplication(processIdentifier: pid),
      app.bundleURL?.standardizedFileURL == expected,
      app.bundleIdentifier == "com.wails.reasonix-desktop" else { exit(2) }
if !app.terminate() { exit(3) }
''')
    output = root / 'native-quit'
    subprocess.run(['/usr/bin/swiftc', '-module-cache-path', str(root / 'swift-cache'), str(source), '-o', str(output)], check=True, capture_output=True, timeout=90)
    return output

def smoke(preview_path, legacy_app_path, legacy_cli_path):
    legacy = Path(legacy_app_path).resolve()
    cli = Path(legacy_cli_path).resolve()
    preview = Path(preview_path).resolve()
    verify_official_legacy(legacy, cli)
    identifier = plistlib.loads((preview / 'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
    if profiles.package.matching_package_is_running(identifier):
        raise RuntimeError('Preview already running')
    for pid, parent, command in profiles.package.processes():
        if '/Contents/MacOS/reasonix-desktop' in command and 'reasonix-desktop-bridge' not in command:
            raise RuntimeError('Wails already running')
    for app in [legacy, preview]:
        subprocess.run(['/usr/bin/codesign', '--verify', '--deep', '--strict', str(app)], check=True, capture_output=True)
    QUESTION = 'private-legacy-history-' + uuid.uuid4().hex
    ANSWER = 'private-legacy-answer-' + uuid.uuid4().hex

    class Provider(http.server.BaseHTTPRequestHandler):
        calls = 0

        def log_message(self, *args):
            pass

        def do_POST(self):
            self.connection.settimeout(5)
            length = int(self.headers.get('Content-Length', '0'))
            if self.path != '/v1/chat/completions' or not 0 < length <= 2 * 1024 * 1024 or self.headers.get('Authorization') != 'Bearer private-fake-loopback-key':
                self.send_error(400)
                return
            payload = json.loads(self.rfile.read(length))
            if payload.get('model') != 'alpha':
                self.send_error(400)
                return
            type(self).calls += 1
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.send_header('Connection', 'close')
            self.end_headers()
            for delta, finish in [({'role': 'assistant', 'content': ANSWER}, None), ({}, 'stop')]:
                chunk = {'id': 'private-legacy-reply', 'object': 'chat.completion.chunk', 'model': 'alpha', 'choices': [{'index': 0, 'delta': delta, 'finish_reason': finish}]}
                self.wfile.write(('data: ' + json.dumps(chunk) + '\n\n').encode())
                self.wfile.flush()
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()
            self.close_connection = True

    def check_history(path):
        if path.stat().st_size > 2 * 1024 * 1024:
            raise RuntimeError('History exceeds private fixture limit')
        raw = path.read_bytes()
        records = [json.loads(line) for line in raw.splitlines()]
        if not any((record.get('role') == 'user' and record.get('content') == QUESTION for record in records)):
            raise RuntimeError('Original user history missing')
        if not any((record.get('role') == 'assistant' and record.get('content') == ANSWER for record in records)):
            raise RuntimeError('Original assistant history missing')
        return raw

    def old_gui(root, env, phase, session):
        core = root / 'home/.reasonix'
        log_path = root / ('official-wails-' + phase + '.log')
        log_path.touch(mode=384)
        with log_path.open('ab') as log:
            host = subprocess.Popen([str(legacy / 'Contents/MacOS/reasonix-desktop')], cwd=root, env=env, stdout=log, stderr=log, start_new_session=True)
        try:
            deadline = time.monotonic() + 25
            while time.monotonic() < deadline:
                if host.poll() is not None:
                    raise RuntimeError('Wails exited before session lease')
                data = json.loads((core / 'desktop-tabs.json').read_text())
                tabs = data.get('tabs', [])
                if len(tabs) != 1 or tabs[0].get('model') != 'native-import/alpha' or tabs[0].get('scope') != 'global' or (data.get('activeTab') != tabs[0]['id']):
                    raise RuntimeError('Wails changed restored tab identity/model')
                actual = Path(tabs[0]['sessionPath']).resolve()
                if actual != session or not Path(tabs[0]['workspaceRoot']).resolve().is_relative_to(core):
                    raise RuntimeError('Wails changed session/profile boundary')
                lease = Path(str(session) + '.lease.lock')
                if lease.is_file():
                    try:
                        metadata = json.loads(lease.read_text().strip())
                        if metadata.get('pid') == host.pid and Path(metadata['session_path']).resolve() == session:
                            break
                    except json.JSONDecodeError:
                        pass
                time.sleep(0.1)
            else:
                raise RuntimeError('Wails session lease timeout')
            check_history(session)
            calls = Provider.calls
            rejected = subprocess.run([str(cli), 'run', '--resume', str(session), '--print', 'must-not-run'], cwd=core / 'global-workspace', env=env, capture_output=True, text=True, timeout=10)
            if rejected.returncode == 0 or not any((word in (rejected.stdout + rejected.stderr).lower() for word in ['session lease', 'session is in use', 'session is already in use', 'session in use'])):
                raise RuntimeError('Old CLI did not reject the live Wails session lease')
            if Provider.calls != calls:
                raise RuntimeError('Rejected session writer reached provider')
            subprocess.run([str(quit_helper), str(host.pid), str(legacy)], check=True, capture_output=True, timeout=5)
            if host.wait(timeout=10) != 0:
                raise RuntimeError('Wails native quit was not normal')
            if any((parent == host.pid for pid, parent, command in profiles.package.processes())):
                raise RuntimeError('Wails child survived quit')
            check_history(session)
            print('official Wails ' + phase + ': original history, live session writer refusal, native quit and cleanup: OK', flush=True)
        finally:
            if host.poll() is None:
                os.killpg(host.pid, signal.SIGKILL)
                host.wait(timeout=5)
    root = Path(tempfile.mkdtemp(prefix='reasonix-official-wails-history-', dir='/private/tmp')).resolve()
    print('Private history test root:', root, flush=True)
    quit_helper = compile_quit_helper(root)
    success = False
    saved_environment = os.environ.copy()
    os.environ.clear()
    os.environ.update(PATH='/usr/bin:/bin:/usr/sbin:/sbin')
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Provider)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        for name in ['home', 'tmp', 'project 中文 & spaces', 'legacy-cache']:
            (root / name).mkdir(mode=448)
        core = root / 'home/.reasonix'
        core.mkdir(mode=448)
        workspace = core / 'global-workspace'
        workspace.mkdir(mode=448)
        configuration = profiles.CONFIG.replace(b'[desktop]\n', b'[desktop]\nmetrics = false\ntelemetry = false\n').replace(b'kind = "openai"\n', b'kind = "openai"\napi_key_env = "REASONIX_OFFICIAL_SMOKE_KEY"\n').replace(b'127.0.0.1:9', f'127.0.0.1:{server.server_port}'.encode())
        (core / 'config.toml').write_bytes(configuration)
        (core / 'config.toml').chmod(384)
        (core / '.env').write_text('REASONIX_OFFICIAL_SMOKE_KEY=private-fake-loopback-key\n')
        (core / '.env').chmod(384)
        env = {'PATH': '/usr/bin:/bin:/usr/sbin:/sbin', 'HOME': str(root / 'home'), 'TMPDIR': str(root / 'tmp'), 'REASONIX_HOME': str(core), 'REASONIX_STATE_HOME': str(core), 'REASONIX_CACHE_HOME': str(root / 'legacy-cache'), 'HTTP_PROXY': 'http://127.0.0.1:9', 'HTTPS_PROXY': 'http://127.0.0.1:9', 'NO_PROXY': '127.0.0.1,localhost'}
        result = subprocess.run([str(cli), 'run', '--print', '--max-steps', '1', QUESTION], cwd=workspace, env=env, capture_output=True, text=True, timeout=30)
        if result.returncode != 0 or ANSWER not in result.stdout or Provider.calls != 1:
            raise RuntimeError('Official CLI could not create real bounded history')
        sessions = [p for p in (core / 'projects').rglob('*.jsonl') if p.parent.name == 'sessions' and (not p.name.endswith('.events.jsonl'))]
        if len(sessions) != 1:
            raise RuntimeError('Official CLI generated unexpected history set')
        session = sessions[0].resolve()
        check_history(session)
        tab_id = 'tab_' + uuid.uuid4().hex
        (core / 'desktop-tabs.json').write_text(json.dumps({'tabs': [{'id': tab_id, 'scope': 'global', 'workspaceRoot': str(workspace), 'topicId': '', 'sessionPath': str(session), 'model': 'native-import/alpha'}], 'activeTab': tab_id}))
        (core / 'desktop-tabs.json').chmod(384)
        projects = {'projects': [{'root': str(root / 'project 中文 & spaces'), 'title': 'Private project', 'topics': ['legacy'], 'order': 99}], 'sessions': ['legacy']}
        (core / 'desktop-projects.json').write_text(json.dumps(projects))
        (core / 'desktop-projects.json').chmod(384)
        for name in ['sessions/legacy.txt', 'cache/legacy.txt', 'plugins/legacy.txt']:
            path = core / name
            path.parent.mkdir(exist_ok=True)
            path.write_text('private legacy sentinel')
            path.chmod(384)
        old_gui(root, env, 'before-Preview', session)
        configuration = (core / 'config.toml').read_bytes()
        original = profiles.tree(core)
        history = check_history(session)
        imported = profiles.launch(preview, identifier, root, 'import', configuration)
        if profiles.launch(preview, identifier, root, 'restore', configuration) != imported:
            raise RuntimeError('Preview credential identity changed')
        profiles.launch(preview, identifier, root, 'explicit', configuration)
        if profiles.tree(core) != original:
            raise RuntimeError('Preview changed original Wails tree')
        old_gui(root, env, 'after-Preview', session)
        if check_history(session) != history:
            raise RuntimeError('Old GUI rollback changed original history bytes')
        after = profiles.tree(core)
        for name in ['config.toml', 'sessions/legacy.txt', 'cache/legacy.txt', 'plugins/legacy.txt', '.env']:
            if after[name] != original[name]:
                raise RuntimeError('Old GUI rollback changed protected originals')
        result = subprocess.run([str(cli), 'config', 'currency'], cwd=root, env=env, capture_output=True, text=True, timeout=15)
        if result.returncode != 0 or 'currency = "CNY"' not in result.stdout or Provider.calls != 1:
            raise RuntimeError('Final old readback changed currency/provider requests')
        print('Official old GUI history rollback, config readback and Preview full original protection: OK', flush=True)
        success = True
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)
        os.environ.clear()
        os.environ.update(saved_environment)
        if success:
            shutil.rmtree(root)
        else:
            print('Failed private fixture retained:', root, flush=True)
if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app', help='Actual macOS Preview .app')
    parser.add_argument('--legacy-app', required=True, help='Extracted official Desktop 1.38.3 .app')
    parser.add_argument('--legacy-cli', required=True, help='Extracted official arm64 CLI 1.38.3')
    args = parser.parse_args()
    if sys.platform != 'darwin' or platform.machine() != 'arm64':
        raise SystemExit('This acceptance requires native arm64 macOS')
    try:
        smoke(args.app, args.legacy_app, args.legacy_cli)
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f'Official macOS rollback smoke failed: {error}') from error
