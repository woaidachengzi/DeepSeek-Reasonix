#!/usr/bin/env python3
"""Own ordinary private Preview launches for actual storage-path Copy UI.

For each mode: Settings > Storage and paths, click Copy Preview configuration
directory, run the published helper claim command, check Copied feedback, return
to the workspace and Cmd+V into its empty composer. Check the exact private
path, then Cmd+Q. No message/model request is needed. UI assertions remain
separate from this runner's system value/generation and lifecycle checks.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import shutil
import signal
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


def smoke(app_path, seconds, control_path):
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
    try:
        for managed in (True, False):
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
            with clipboard.NativeClipboardFixture(mode / 'tmp', expected_path=core, nonce=nonce) as fixture:
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
                                                   'expectedPath': str(core),
                                                   'claim': [str(fixture.binary), 'claim', str(fixture.snapshot), str(fixture.marker)]}))
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
                    for pid in package.own_sidecars(mode / 'tmp', sidecar_binary):
                        try:
                            os.kill(pid, signal.SIGKILL)
                        except ProcessLookupError:
                            pass
            print('Original clipboard item/type order and bytes restored and independently verified', flush=True)
        success = True
        print('Two path-copy UI lifecycles/system checks passed; actual button, feedback and composer paste require UI evidence.', flush=True)
    finally:
        if success:
            control.unlink(missing_ok=True)
            shutil.rmtree(root)
        else:
            print('Private failure fixture retained:', root, flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app')
    parser.add_argument('--seconds', type=int, default=300, choices=range(60, 901), metavar='SECONDS')
    parser.add_argument('--control', default='/private/tmp/reasonix-ui-clipboard-control.json')
    args = parser.parse_args()
    if sys.platform != 'darwin':
        raise SystemExit('This acceptance requires macOS')
    smoke(args.app, args.seconds, args.control)
