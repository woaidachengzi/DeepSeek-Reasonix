#!/usr/bin/env python3
"""macOS official Wails 1.38.3 history rollback across real Preview import/restarts.

Requires authenticated official CLI and Desktop binaries; fixed digests reject
local modified builds. Uses a fake loopback provider and private profiles.
Checks original history, live session writer refusal and native quit. Optional
workspace-data creates real attachment references and edit checkpoints through
the official CLI's local serve entry. It does not test renderer images,
rewind UI, physical keys, full historical data or directory lifetime locks.
"""
import argparse
import base64
import hashlib
import http.client
import http.cookies
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

def smoke(preview_path, legacy_app_path, legacy_cli_path, workspace_data=False):
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
    RESUME = 'private-legacy-resume-' + uuid.uuid4().hex
    RESUME_ANSWER = 'private-legacy-resumed-answer-' + uuid.uuid4().hex
    FILE_BEFORE = 'private-global-before-' + uuid.uuid4().hex + '\n'
    FILE_AFTER = 'private-global-after-' + uuid.uuid4().hex + '\n'
    ATTACHMENT_TEXT = 'private-attachment-' + uuid.uuid4().hex
    text_ref = '.reasonix/attachments/clipboard-20261001-010203.000000.yml'
    image_ref = '.reasonix/attachments/clipboard-20261001-010203.000000.png'
    refs = ' @global-note.txt @' + text_ref + ' @' + image_ref if workspace_data else ''
    prompts = [QUESTION + refs, RESUME + refs]

    class Provider(http.server.BaseHTTPRequestHandler):
        calls = 0
        failure = None

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
            delta, finish = {'role': 'assistant', 'content': ANSWER}, 'stop'
            if workspace_data:
                # Fixed read/edit sequence, never accept arbitrary tool arguments
                # from the HTTP request and never execute a model-supplied shell.
                turn, step = divmod(type(self).calls, 3)
                if turn >= 2:
                    type(self).failure = 'Unexpected extra provider request'
                    self.send_error(400)
                    return
                delta = {'role': 'assistant', 'content': ANSWER if turn == 0 else RESUME_ANSWER}
                before, after = (FILE_BEFORE, FILE_AFTER) if turn == 0 else (FILE_AFTER, FILE_BEFORE)
                user_content = [message.get('content') for message in payload.get('messages', []) if message.get('role') == 'user']
                resolved = next((content for content in reversed(user_content) if isinstance(content, str) and prompts[turn] in content), '')
                if not all(value in resolved for value in [before.strip(), ATTACHMENT_TEXT,
                                                           '<file path="' + text_ref + '">',
                                                           '<image path="' + image_ref + '">']):
                    type(self).failure = 'Official CLI did not resolve current Global file/attachment references'
                    self.send_error(400)
                    return
                if step < 2:
                    name = 'read_file' if step == 0 else 'edit_file'
                    available = [item.get('function', {}).get('name') for item in payload.get('tools', [])]
                    if name not in available:
                        type(self).failure = 'Expected native file tool unavailable'
                        self.send_error(400)
                        return
                    arguments = {'path': 'global-note.txt', 'limit': 10} if step == 0 else {
                        'path': 'global-note.txt', 'old_string': before, 'new_string': after}
                    if step == 1:
                        tools = [message for message in payload['messages'] if message.get('role') == 'tool']
                        if not tools or before.strip() not in str(tools[-1].get('content')):
                            type(self).failure = 'Official read_file did not return the owned preimage'
                            self.send_error(400)
                            return
                    delta = {'role': 'assistant', 'tool_calls': [{'index': 0,
                        'id': f'private-file-{turn}-{step}', 'type': 'function',
                        'function': {'name': name, 'arguments': json.dumps(arguments)}}]}
                    finish = 'tool_calls'
                elif (workspace / 'global-note.txt').read_text() != after:
                    type(self).failure = 'Official edit_file did not write the expected owned file'
                    self.send_error(400)
                    return
            type(self).calls += 1
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.send_header('Connection', 'close')
            self.end_headers()
            for part, reason in [(delta, None), ({}, finish)]:
                chunk = {'id': 'private-legacy-reply', 'object': 'chat.completion.chunk', 'model': 'alpha', 'choices': [{'index': 0, 'delta': part, 'finish_reason': reason}]}
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
        if not any((record.get('role') == 'user' and (
                prompts[0] in str(record.get('content')) if workspace_data else record.get('content') == QUESTION)) for record in records):
            raise RuntimeError('Original user history missing')
        if not any((record.get('role') == 'assistant' and record.get('content') == ANSWER for record in records)):
            raise RuntimeError('Original assistant history missing')
        return raw

    def check_checkpoint(session, prompt, before, after):
        directory = session.with_suffix('.ckpt')
        matches = []
        for path in directory.glob('turns/*/meta.json'):
            data = json.loads(path.read_text())
            if data.get('prompt') == prompt:
                matches.append((path, data))
        if len(matches) != 1:
            raise RuntimeError('Official edit did not produce one persisted checkpoint')
        path, data = matches[0]
        files = data.get('files', [])
        if data.get('schemaVersion') != 3 or len(files) != 1 or Path(files[0]['path']).resolve() != workspace / 'global-note.txt':
            raise RuntimeError('Official checkpoint schema/file identity differs')
        snap = files[0]
        if snap.get('sha256') != hashlib.sha256(before.encode()).hexdigest() or snap.get('afterSha256') != hashlib.sha256(after.encode()).hexdigest():
            raise RuntimeError('Official checkpoint fingerprints differ')
        payload = path.parent / 'files/0000.before'
        if payload.is_symlink() or payload.read_bytes() != before.encode():
            raise RuntimeError('Official checkpoint preimage missing or changed')
        marker = directory / f"turn-{data['turn']}.json"
        if not marker.is_file() or json.loads(marker.read_text()).get('turn') != data['turn']:
            raise RuntimeError('Official checkpoint compatibility marker missing')
        return data['turn']

    def old_serve(root, env, phase, prompt, session=None, rewinds=(), conflict_turn=None):
        token = uuid.uuid4().hex + uuid.uuid4().hex
        token_file, port_file = root / (phase + '.token'), root / (phase + '.port')
        token_file.touch(mode=384)
        token_file.write_text(token)
        log_path = root / ('official-serve-' + phase + '.log')
        log_path.touch(mode=384)
        command = [str(cli), 'serve', '--addr', '127.0.0.1:0', '--no-open', '--max-steps', '3',
                   '--auth', 'token', '--token-file', str(token_file), '--port-file', str(port_file)]
        if session:
            command += ['--resume', str(session)]
        with log_path.open('ab') as log:
            host = subprocess.Popen(command, cwd=workspace, env=env, stdout=log, stderr=log, start_new_session=True)
        connection = None
        try:
            profiles.wait_until(port_file.is_file, host)
            address, port = port_file.read_text().strip().rsplit(':', 1)
            if address != '127.0.0.1' or not 1 <= int(port) <= 65535:
                raise RuntimeError('Official serve escaped the private loopback boundary')
            connection = http.client.HTTPConnection(address, int(port), timeout=5)
            cookie, current = '', ''

            def request(path, body=None, expected=200):
                headers = {'Content-Type': 'application/json'}
                if cookie:
                    headers['Cookie'] = cookie
                if current:
                    headers['X-Reasonix-Expected-Session-Path'] = current
                connection.request('POST' if body is not None else 'GET', path,
                                   json.dumps(body) if body is not None else None, headers)
                response = connection.getresponse()
                raw = response.read(2 * 1024 * 1024 + 1)
                if response.status != expected or len(raw) > 2 * 1024 * 1024:
                    raise RuntimeError(f'Official private serve {path} unexpected status/size: {response.status}')
                return response, raw

            response, _ = request('/auth/token', {'token': token}, 204)
            cookies = http.cookies.SimpleCookie(response.getheader('Set-Cookie'))
            if len(cookies) != 1 or next(iter(cookies.values())).value != token:
                raise RuntimeError('Official serve did not authenticate the owned cookie')
            cookie = next(iter(cookies.values())).OutputString(attrs=[])
            _, raw = request('/status')
            status = json.loads(raw)
            current = status.get('sessionPath', '')
            # A fresh legacy serve assigns the transcript path on first turn.
            # An explicit resume must already expose the exact original path.
            # Legacy status.cwd is Controller.SessionDir (history storage),
            # not the controller's workspace. Check the owned process's actual
            # kernel cwd independently rather than interpreting that label.
            cwd = subprocess.run(['/usr/sbin/lsof', '-a', '-p', str(host.pid), '-d', 'cwd', '-Fn'],
                                 check=True, capture_output=True, text=True, timeout=5)
            if [line[1:] for line in cwd.stdout.splitlines() if line.startswith('n')] != [str(workspace)]:
                raise RuntimeError('Official serve kernel cwd differs from owned Global workspace')
            if (current and not Path(current).resolve().is_relative_to(core)) or not Path(status['cwd']).resolve().is_relative_to(core):
                raise RuntimeError('Official serve session/workspace ownership differs')
            if session and Path(current).resolve() != session:
                raise RuntimeError('Official serve changed the resumed history identity')
            if session:
                _, raw = request('/checkpoints')
                checkpoints = json.loads(raw)
                if len(checkpoints) != (2 if rewinds else 1) or any(
                        item.get('canUndoFiles') is not True or item.get('canCode') is not True for item in checkpoints):
                    raise RuntimeError('Official resumed controller could not load the original file checkpoint')
            calls = Provider.calls
            if rewinds:
                original_history = check_history(session)
                for turn, expected_content in rewinds:
                    if (workspace / 'global-note.txt').read_text() == expected_content:
                        raise RuntimeError('Legacy rewind fixture would be a no-op')
                    request('/rewind', {'turn': turn, 'scope': 'code'}, 204)
                    if (workspace / 'global-note.txt').read_text() != expected_content:
                        raise RuntimeError('Official checkpoint rewind did not restore actual file content')
                    if check_history(session) != original_history or Provider.calls != calls:
                        raise RuntimeError('Code-only legacy rewind changed history or reached provider')
                    _, raw = request('/status')
                    if Path(json.loads(raw).get('sessionPath', '')).resolve() != session:
                        raise RuntimeError('Code-only legacy rewind switched history identity')
                if conflict_turn is not None:
                    # The authenticated 1.38.3 wrapper retains both checkpoints.
                    # A prior code rewind changes the file's after-fingerprint;
                    # the earlier cumulative rewind must refuse that conflict.
                    # This is separate from the successful restore assertion.
                    files = profiles.tree(workspace)
                    _, raw = request('/rewind', {'turn': conflict_turn, 'scope': 'code'}, 500)
                    if raw.strip() != b'file conflicts detected':
                        raise RuntimeError('Legacy conflicting rewind returned an unrelated failure')
                    if profiles.tree(workspace) != files or check_history(session) != original_history or Provider.calls != calls:
                        raise RuntimeError('Refused legacy conflicting rewind changed files/history or reached provider')
            else:
                request('/tool-approval-mode', {'mode': 'auto'}, 204)
                request('/submit', {'input': prompt}, 202)

                def complete():
                    nonlocal current
                    if Provider.failure:
                        raise RuntimeError(Provider.failure)
                    _, raw = request('/status')
                    status = json.loads(raw)
                    if Provider.calls == calls + 3 and status.get('running') is False:
                        actual = status.get('sessionPath', '')
                        if not actual or not Path(actual).resolve().is_relative_to(core) or (current and Path(actual).resolve() != Path(current).resolve()):
                            raise RuntimeError('Official completed turn escaped its history identity')
                        current = actual
                        return True
                    return False

                profiles.wait_until(complete, host)
                _, raw = request('/history')
                history = json.loads(raw)
                expected_answer = RESUME_ANSWER if session else ANSWER
                if not any(message.get('role') == 'assistant' and message.get('content') == expected_answer for message in history):
                    raise RuntimeError('Official serve did not complete the real turn')
                _, raw = request('/checkpoints')
                if not any(item.get('prompt') == prompt and item.get('canUndoFiles') is True for item in json.loads(raw)):
                    raise RuntimeError('Official controller did not expose the new usable checkpoint')
            host.terminate()
            if host.wait(timeout=10) != 0:
                raise RuntimeError('Official serve did not stop normally')
            if any(parent == host.pid for pid, parent, command in profiles.package.processes()):
                raise RuntimeError('Official serve left a child process')
            return Path(current).resolve()
        finally:
            if connection:
                connection.close()
            if host.poll() is None:
                os.killpg(host.pid, signal.SIGKILL)
                host.wait(timeout=5)
            token_file.unlink()

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
        if workspace_data:
            (workspace / 'global-note.txt').write_text(FILE_BEFORE)
            attachment_dir = workspace / '.reasonix/attachments'
            attachment_dir.mkdir(mode=448, parents=True)
            (workspace / text_ref).write_text('owned_fixture: ' + ATTACHMENT_TEXT + '\n')
            # Fixed valid 1x1 PNG fixture; no user image or image conversion.
            (workspace / image_ref).write_bytes(base64.b64decode(
                'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII='))
            for path in [workspace / 'global-note.txt', workspace / text_ref, workspace / image_ref]:
                path.chmod(384)
        configuration = profiles.CONFIG.replace(b'[desktop]\n', b'[desktop]\nmetrics = false\ntelemetry = false\n').replace(b'kind = "openai"\n', b'kind = "openai"\napi_key_env = "REASONIX_OFFICIAL_SMOKE_KEY"\n').replace(b'127.0.0.1:9', f'127.0.0.1:{server.server_port}'.encode())
        (core / 'config.toml').write_bytes(configuration)
        (core / 'config.toml').chmod(384)
        (core / '.env').write_text('REASONIX_OFFICIAL_SMOKE_KEY=private-fake-loopback-key\n')
        (core / '.env').chmod(384)
        env = {'PATH': '/usr/bin:/bin:/usr/sbin:/sbin', 'HOME': str(root / 'home'), 'TMPDIR': str(root / 'tmp'), 'REASONIX_HOME': str(core), 'REASONIX_STATE_HOME': str(core), 'REASONIX_CACHE_HOME': str(root / 'legacy-cache'), 'HTTP_PROXY': 'http://127.0.0.1:9', 'HTTPS_PROXY': 'http://127.0.0.1:9', 'NO_PROXY': '127.0.0.1,localhost'}
        if workspace_data:
            served_session = old_serve(root, env, 'create', prompts[0])
        else:
            result = subprocess.run([str(cli), 'run', '--print', '--max-steps', '1', QUESTION], cwd=workspace, env=env, capture_output=True, text=True, timeout=30)
            if result.returncode != 0 or ANSWER not in result.stdout or Provider.calls != 1:
                raise RuntimeError('Official CLI could not create real bounded history')
        # Match store.IsSessionTranscriptName, including turn/conflict/guardian
        # sidecars created by real edits rather than counting them as sessions.
        sidecars = ('.events.jsonl', '.turns.jsonl', '.conflicts.jsonl', '.guardian.jsonl')
        sessions = [p for p in (core / 'projects').rglob('*.jsonl')
                    if p.parent.name == 'sessions' and not p.name.endswith(sidecars)]
        if len(sessions) != 1:
            raise RuntimeError('Official CLI generated unexpected history set')
        session = sessions[0].resolve()
        if workspace_data and session != served_session:
            raise RuntimeError('Official serve saved a different history identity')
        check_history(session)
        if workspace_data:
            first_turn = check_checkpoint(session, prompts[0], FILE_BEFORE, FILE_AFTER)
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
        if workspace_data:
            # Positive old-version resume after Preview, in addition to the live
            # writer refusal. Re-resolve current references and create a second
            # real checkpoint while preserving the first checkpoint/attachments.
            protected = {name: value for name, value in original.items()
                         if name.startswith('global-workspace/.reasonix/attachments/') or
                         name.startswith(str(session.with_suffix('.ckpt').relative_to(core)) + '/')}
            if any(after.get(name) != value for name, value in protected.items()):
                raise RuntimeError('Old GUI rollback changed attachments/checkpoint files')
            old_serve(root, env, 'resume', prompts[1], session)
            if Provider.calls != 6:
                raise RuntimeError(Provider.failure or 'Official old CLI could not resume Global history after Preview')
            final_history = check_history(session)
            records = [json.loads(line) for line in final_history.splitlines()]
            if not any(record.get('role') == 'user' and prompts[1] in str(record.get('content')) for record in records):
                raise RuntimeError('Positive legacy resume did not persist the second prompt')
            if not any(record.get('role') == 'assistant' and record.get('content') == RESUME_ANSWER for record in records):
                raise RuntimeError('Positive legacy resume did not persist its own distinct reply')
            second_turn = check_checkpoint(session, prompts[1], FILE_AFTER, FILE_BEFORE)
            if check_checkpoint(session, prompts[0], FILE_BEFORE, FILE_AFTER) != first_turn or second_turn <= first_turn:
                raise RuntimeError('Resumed checkpoint did not preserve monotonic turn identity')
            resumed = profiles.tree(core)
            if any(resumed.get(name) != value for name, value in protected.items()):
                raise RuntimeError('Positive legacy resume changed original attachment/checkpoint files')
            if (workspace / 'global-note.txt').read_text() != FILE_BEFORE:
                raise RuntimeError('Resumed legacy edit did not restore owned fixture content')
            print('Official Global file/text+image references and edit checkpoints survived Preview; real legacy resume/second checkpoint: OK', flush=True)
            old_serve(root, env, 'rewind', None, session,
                      [(second_turn, FILE_AFTER)], conflict_turn=first_turn)
            final = profiles.tree(core)
            if any(final.get(name) != value for name, value in protected.items()
                   if name.startswith('global-workspace/.reasonix/attachments/')):
                raise RuntimeError('Legacy code rewind changed original attachments')
            print('Official legacy latest code rewind restored the actual preimage; earlier conflict refused, transcript/attachments preserved, no provider request: OK', flush=True)
        final = profiles.tree(core)
        for name in ['config.toml', 'sessions/legacy.txt', 'cache/legacy.txt', 'plugins/legacy.txt', '.env']:
            if final.get(name) != original[name]:
                raise RuntimeError('Positive legacy operations changed protected configuration/canaries')
        result = subprocess.run([str(cli), 'config', 'currency'], cwd=root, env=env, capture_output=True, text=True, timeout=15)
        if result.returncode != 0 or 'currency = "CNY"' not in result.stdout or Provider.calls != (6 if workspace_data else 1) or profiles.tree(core) != final:
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
    parser.add_argument('--workspace-data', action='store_true', help='Create/resume real Global refs, file edits and checkpoints')
    args = parser.parse_args()
    if sys.platform != 'darwin' or platform.machine() != 'arm64':
        raise SystemExit('This acceptance requires native arm64 macOS')
    try:
        smoke(args.app, args.legacy_app, args.legacy_cli, args.workspace_data)
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f'Official macOS rollback smoke failed: {error}') from error
