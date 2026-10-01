#!/usr/bin/env python3
"""Own ordinary private Preview launches for path-copy or composer edit UI.

For each mode: Settings > Storage and paths, click Copy Preview configuration
directory, run the published helper claim command, check Copied feedback, return
to the workspace and Cmd+V into its empty composer. Check the exact private
path, then Cmd+Q. No message/model request is needed. UI assertions remain
separate from this runner's system value/generation and lifecycle checks.
With --scenario edit, seed expectedText through the real empty composer, exercise
Cmd+A/C/X/V and the actual selection context menu's Copy/Cut/Paste. Run checkpoint
immediately before each Copy/Cut and claim after it, inspect the actual
empty/restored values, then clear the draft and Cmd+Q. The displayed test value
requires an exact claimed generation. This is not an IME/input-method gate.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import signal
import stat
import subprocess
import sys
import time
import uuid

sys.dont_write_bytecode = True


def module(name, file):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(file))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


package = module('package', 'smoke-packaged-app.py')
clipboard = module('clipboard', 'native-clipboard-fixture.py')


def record(action, control_path):
    control = Path(control_path)
    attributes = control.lstat()
    if (not stat.S_ISREG(attributes.st_mode) or attributes.st_uid != os.getuid()
            or stat.S_IMODE(attributes.st_mode) != 0o600 or attributes.st_size > 8192):
        raise RuntimeError('Clipboard UI control is not a bounded private regular file')
    state = json.loads(control.read_text())
    root = Path(state['root'])
    if (root.parent != Path('/private/tmp') or root.resolve() != root
            or not re.fullmatch(r'reasonix-native-ui-clipboard-[0-9a-f]{32}', root.name)
            or type(state['managed']) is not bool or state['scenario'] not in ('edit', 'path')):
        raise RuntimeError('Clipboard UI control does not identify a private fixture')
    temporary = root / ('managed' if state['managed'] else 'explicit') / 'tmp'
    for directory in (root, temporary.parent, temporary):
        attributes = directory.lstat()
        if (not stat.S_ISDIR(attributes.st_mode) or attributes.st_uid != os.getuid()
                or stat.S_IMODE(attributes.st_mode) != 0o700):
            raise RuntimeError('Clipboard UI fixture directory is not private')
    binary, snapshot, marker = (temporary / name for name in
                                ('clipboard-snapshot', 'clipboard-original.plist', 'reasonix-native-clipboard-owned.json'))
    claim_action = 'claim-after' if state['scenario'] == 'edit' else 'claim'
    if (state['claim'] != [str(binary), claim_action, str(snapshot), str(marker)]
            or state['checkpoint'] != [str(binary), 'checkpoint', str(snapshot), str(marker)]):
        raise RuntimeError('Clipboard UI control contains an unexpected command')
    if any(type(state[key]) is not int or not package.is_alive(state[key])
           for key in ('hostPid', 'sidecarPid')):
        raise RuntimeError('Clipboard UI fixture is no longer live')
    if not binary.is_file() or binary.is_symlink() or binary.stat().st_uid != os.getuid():
        raise RuntimeError('Clipboard UI helper is not an owned regular executable')
    if action not in ('checkpoint', 'claim', 'verify'):
        raise ValueError('Unknown clipboard UI recording action')
    actual = claim_action if action == 'claim' else action
    result = subprocess.run([str(binary), actual, str(snapshot), str(marker)], capture_output=True, timeout=15)
    if result.returncode:
        raise RuntimeError('Clipboard UI value/generation recording rejected; clipboard was not written')
    print(f'Clipboard UI {action} accepted without writing system clipboard', flush=True)


def smoke(app_path, seconds, control_path, scenario='path', profile='both'):
    if scenario not in ('path', 'edit'):
        raise ValueError('Unknown clipboard UI scenario')
    if profile not in ('managed', 'explicit', 'both'):
        raise ValueError('Unknown clipboard UI profile')
    app = Path(app_path).resolve()
    identifier = plistlib.loads((app / 'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
    if identifier != 'io.reasonix.desktop.preview' or package.matching_package_is_running(identifier):
        raise RuntimeError('Preview missing or already running')
    subprocess.run(['/usr/bin/codesign', '--verify', '--deep', '--strict', str(app)], check=True, capture_output=True)
    host_binary = app / 'Contents/MacOS/reasonix-tauri'
    sidecar_binary = app / 'Contents/MacOS/reasonix-desktop-bridge'
    env_base = {key: value for key, value in os.environ.items()
                if key in ('PATH', 'USER', 'LOGNAME', 'LANG', 'LC_ALL', '__CF_USER_TEXT_ENCODING')}
    nonce = uuid.uuid4().hex
    root = Path('/private/tmp') / ('reasonix-native-ui-clipboard-' + nonce)
    root.mkdir(mode=0o700)
    control = Path(control_path)
    success = False
    passed = 0
    try:
        modes = (True, False) if profile == 'both' else (profile == 'managed',)
        for managed in modes:
            mode = root / ('managed' if managed else 'explicit')
            mode.mkdir(mode=0o700)
            for name in ('home', 'tmp'):
                (mode / name).mkdir(mode=0o700)
            core = mode / 'home/Library/Application Support' / identifier / 'reasonix-core' if managed else mode / 'core'
            core.mkdir(parents=True, mode=0o700)
            canary = core / 'ui-clipboard-original.canary'
            canary.touch(mode=0o600)
            canary.write_bytes(b'private clipboard UI original\n')
            before = (canary.read_bytes(), canary.stat().st_mode, canary.stat().st_mtime_ns)
            expected = core
            if scenario == 'edit':
                expected = mode / '输入 测试🧪'
                expected.mkdir(mode=0o700)
            with clipboard.NativeClipboardFixture(mode / 'tmp', expected_path=expected, nonce=nonce) as fixture:
                env = env_base | {'HOME': str(mode / 'home'), 'TMPDIR': str(mode / 'tmp')}
                if not managed:
                    env |= {'REASONIX_HOME': str(core), 'REASONIX_CACHE_HOME': str(mode / 'cache')}
                host = subprocess.Popen([str(host_binary)], env=env, stdout=subprocess.DEVNULL,
                                        stderr=subprocess.DEVNULL, start_new_session=True)
                sidecar = None
                try:
                    deadline = time.monotonic() + 25
                    while host.poll() is None and time.monotonic() < deadline:
                        ready = list((mode / 'tmp').glob('reasonix-tauri-bridge-*/ready.json'))
                        children = package.own_sidecars(mode / 'tmp', sidecar_binary)
                        if len(ready) == len(children) == 1:
                            sidecar = children[0]
                            package.check_unauthenticated_health(package.check_ready(ready[0]))
                            package.check_sidecar_profile(sidecar, core, managed, mode / 'cache')
                            identity = package.check_credential_profile(core)
                            break
                        time.sleep(0.05)
                    else:
                        raise RuntimeError('Ordinary clipboard UI host did not start')
                    control.touch(mode=0o600)
                    control.write_text(json.dumps({'root': str(root), 'managed': managed,
                                                   'hostPid': host.pid, 'sidecarPid': sidecar,
                                                   'scenario': scenario, 'expectedPath': str(expected),
                                                   'expectedText': str(expected),
                                                   'checkpoint': [str(fixture.binary), 'checkpoint', str(fixture.snapshot), str(fixture.marker)],
                                                   'claim': [str(fixture.binary), 'claim-after' if scenario == 'edit' else 'claim',
                                                             str(fixture.snapshot), str(fixture.marker)]}))
                    control.chmod(0o600)
                    print(f"Ordinary {'managed' if managed else 'explicit'} clipboard UI ready", flush=True)
                    if host.wait(timeout=seconds) != 0:
                        raise RuntimeError('Clipboard UI host did not quit normally')
                    deadline = time.monotonic() + 5
                    while package.is_alive(sidecar) and time.monotonic() < deadline:
                        time.sleep(0.05)
                    if (package.is_alive(sidecar) or package.own_sidecars(mode / 'tmp', sidecar_binary)
                            or list((mode / 'tmp').glob('reasonix-tauri-bridge-*'))):
                        raise RuntimeError('Clipboard UI quit left sidecar/readiness')
                    fixture.verify()
                    if package.check_credential_profile(core) != identity:
                        raise RuntimeError('Clipboard UI changed profile identity')
                    after = (canary.read_bytes(), canary.stat().st_mode, canary.stat().st_mtime_ns)
                    if after != before:
                        raise RuntimeError('Clipboard UI changed protected original')
                    print('Exact private path/generation, original, identity and normal quit cleanup OK', flush=True)
                finally:
                    if host.poll() is None:
                        os.killpg(host.pid, signal.SIGKILL)
                        host.wait(timeout=5)
                    # Let the existing parent-loss watcher release readiness
                    # before force-reaping only this fixture's remaining child.
                    deadline = time.monotonic() + 5
                    while package.own_sidecars(mode / 'tmp', sidecar_binary) and time.monotonic() < deadline:
                        time.sleep(0.05)
                    for pid in package.own_sidecars(mode / 'tmp', sidecar_binary):
                        try:
                            os.kill(pid, signal.SIGKILL)
                        except ProcessLookupError:
                            pass
            print('Original clipboard item/type order and bytes restored and independently verified', flush=True)
            passed += 1
        success = True
        print(f'{passed} {scenario} UI lifecycle/system checks passed; actual interaction assertions require UI evidence.', flush=True)
    finally:
        if success:
            control.unlink(missing_ok=True)
            shutil.rmtree(root)
        else:
            print('Private failure fixture retained:', root, flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app', nargs='?')
    parser.add_argument('--seconds', type=int, default=300, choices=range(60, 901), metavar='SECONDS')
    parser.add_argument('--control', default='/private/tmp/reasonix-ui-clipboard-control.json')
    parser.add_argument('--scenario', choices=('path', 'edit'), default='path')
    parser.add_argument('--profile', choices=('managed', 'explicit', 'both'), default='both')
    parser.add_argument('--record', choices=('checkpoint', 'claim', 'verify'),
                        help='record the current live fixture before/after an actual UI operation; does not launch an app')
    args = parser.parse_args()
    if sys.platform != 'darwin':
        raise SystemExit('This acceptance requires macOS')
    if args.record:
        record(args.record, args.control)
    elif args.app:
        smoke(args.app, args.seconds, args.control, args.scenario, args.profile)
    else:
        parser.error('app is required unless --record is provided')
