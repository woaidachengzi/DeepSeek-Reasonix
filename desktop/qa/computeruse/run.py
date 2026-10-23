#!/usr/bin/env python3
"""Run explicit, real-input acceptance against a disposable native window.

Requires a built Preview app and its macOS Accessibility/Screen Recording grants.
Uses no external model, credentials, or existing Reasonix core profile.
"""
import argparse
import json
import os
import pathlib
import plistlib
import subprocess
import tempfile
import threading
import time

from provider import serve


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--app', type=pathlib.Path, required=True)
    parser.add_argument('--output', type=pathlib.Path)
    args = parser.parse_args()
    app = args.app.resolve(strict=True)
    if plistlib.loads((app / 'Contents/Info.plist').read_bytes()).get('CFBundleIdentifier') != 'io.reasonix.desktop.preview':
        parser.error('use the Preview bundle, not an installed working-profile app')
    root = args.output or pathlib.Path(tempfile.mkdtemp(prefix='reasonix-computeruse-'))
    root.mkdir(parents=True, exist_ok=True)
    root = root.resolve()
    if (root / 'fixture-state.json').exists() or (root / 'qa-home').exists():
        parser.error('output directory must not contain previous acceptance state')
    print(f'Acceptance artifacts: {root}', flush=True)
    fixture = root / 'Reasonix Control Fixture.app'
    binary = fixture / 'Contents/MacOS/fixture'
    binary.parent.mkdir(parents=True)
    (fixture / 'Contents/Info.plist').write_bytes(plistlib.dumps({
        'CFBundleIdentifier': 'io.reasonix.control.fixture', 'CFBundleName': 'Reasonix Control Fixture',
        'CFBundleExecutable': 'fixture', 'CFBundlePackageType': 'APPL'}))
    subprocess.run(['swiftc', str(pathlib.Path(__file__).with_name('fixture.swift')), '-o', str(binary)], check=True)
    home, workspace = root / 'qa-home', root / 'qa-workspace'
    home.mkdir()
    workspace.mkdir()
    server = serve(root)
    port = server.server_address[1]
    (home / 'config.toml').write_text(f'''config_version = 9
default_model = "local/desktop-qa"
credentials_store = "env"
[desktop]
provider_access = ["local"]
[[providers]]
name = "local"
kind = "openai"
base_url = "http://127.0.0.1:{port}/v1"
model = "desktop-qa"
vision = true
context_window = 131072
[tools]
enabled = ["computer_use"]
[permissions]
mode = "deny"
allow = ["computer_use"]
[sandbox]
bash = "off"
workspace_root = {json.dumps(str(workspace))}
''')
    env = dict(os.environ, REASONIX_CONTROL_FIXTURE_STATE=str(root / 'fixture-state.json'))
    child = subprocess.Popen([str(binary)], env=env)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        deadline = time.monotonic() + 15
        while not (root / 'fixture-state.json').is_file():
            if child.poll() is not None or time.monotonic() >= deadline:
                raise RuntimeError('native fixture failed to start')
            time.sleep(0.05)
        command = ['open', '-n', '-W', '-a', str(app)]
        for variable, value in {
            'TMPDIR': str(root), 'REASONIX_TAURI_NATIVE_WINDOW_SMOKE': 'computer-use',
            'REASONIX_HOME': str(home), 'REASONIX_STATE_HOME': str(home),
            'REASONIX_CACHE_HOME': str(home / 'cache'),
        }.items():
            command.extend(['--env', f'{variable}={value}'])
        command.extend(['--stdout', str(root / 'app.log'), '--stderr', str(root / 'app-error.log')])
        subprocess.run(command, check=True, timeout=120)
        result_path = root / 'computer-use-acceptance.json'
        if not result_path.exists():
            raise RuntimeError('acceptance did not run; close any other Preview instance and inspect app logs')
        result = json.loads(result_path.read_text())
        print(json.dumps(result, ensure_ascii=False, indent=2))
        if not result.get('ok'):
            raise RuntimeError('fixture postconditions failed; inspect tool-results.jsonl and app permissions')
    finally:
        server.shutdown()
        server.server_close()
        child.terminate()
        child.wait(timeout=5)


if __name__ == '__main__':
    main()
