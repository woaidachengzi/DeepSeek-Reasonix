#!/usr/bin/env python3
"""Own three ordinary Preview launches for real UI import/restart/undo testing.

Use Settings > Storage and paths > Old Preview UI preferences. In the first
launch preview/confirm/import, then Cmd+Q. In the second verify imported values
and the rollback button, undo, then Cmd+Q. In the third verify restored defaults
and absent rollback record, then Cmd+Q. Repeat for managed and explicit profiles.
The runner verifies native source isolation, identity, originals and lifecycle;
actual UI assertions must also be recorded via the native UI or by the operator.
No shared legacy store is seeded, cleared or imported.
Control receipts are running/ending/ended; require a live matching PID AND
the private WebView origin before UI actions. Never rebind a dead fixture by
bundle/path: a computer-use binding may launch a default-profile instance.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time
import uuid

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('package', Path(__file__).with_name('smoke-packaged-app.py'))
package = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package)


def original(path):
    info = path.stat()
    return path.read_bytes(), info.st_mode, info.st_mtime_ns


def publish_control(control, payload):
    """Publish a complete private lifecycle receipt, never a partial JSON file."""
    with tempfile.NamedTemporaryFile(mode='w', prefix='.ui-migration-control-',
                                     dir=control.parent, delete=False) as output:
        temporary = Path(output.name)
        os.fchmod(output.fileno(), 0o600)
        output.write(json.dumps(payload) + '\n')
    try:
        os.replace(temporary, control)
    finally:
        temporary.unlink(missing_ok=True)


def prepare_source(root):
    workspace = root / '旧界面 工作区'
    workspace.mkdir(mode=0o700)
    source = root / 'tmp/reasonix-native-legacy-ui-source.json'
    source.touch(mode=0o600)
    source.write_text(json.dumps({'nonce': uuid.uuid4().hex, 'workspace': str(workspace)}))
    return source


def check_source_receipt(receipt, expected_reads=None):
    info = receipt.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o777 != 0o600 or info.st_uid != os.geteuid() or info.st_size > 1024:
        raise RuntimeError('Private nonpersistent legacy source receipt is not an owned bounded ordinary file')
    with receipt.open('rb') as file:
        data = json.loads(file.read(1025))
    reads = data.get('readCount')
    if (data.get('nonPersistent') is not True or type(data.get('valueCount')) is not int
            or data['valueCount'] != 3 or type(reads) is not int or not 1 <= reads <= 100
            or (expected_reads is not None and reads != expected_reads)):
        raise RuntimeError('Private nonpersistent legacy source receipt missing/invalid')


def smoke(app_path, control_path, seconds, profile='both'):
    if profile not in ('both', 'managed', 'explicit'):
        raise ValueError('Unknown UI migration profile mode')
    app = Path(app_path).resolve()
    identifier = plistlib.loads((app / 'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
    if identifier != 'io.reasonix.desktop.preview' or package.matching_package_is_running(identifier):
        raise RuntimeError('Preview missing or already running')
    subprocess.run(['/usr/bin/codesign', '--verify', '--deep', '--strict', str(app)], check=True, capture_output=True)
    host_binary = app / 'Contents/MacOS/reasonix-tauri'
    sidecar_binary = app / 'Contents/MacOS/reasonix-desktop-bridge'
    environment = {key: value for key, value in os.environ.items()
                   if key in ('PATH', 'USER', 'LOGNAME', 'LANG', 'LC_ALL', '__CF_USER_TEXT_ENCODING')}
    control = Path(control_path).resolve()
    success = False
    roots = []
    passed = 0
    try:
        modes = (True, False) if profile == 'both' else (profile == 'managed',)
        for managed in modes:
            root = Path(tempfile.mkdtemp(prefix='reasonix-ui-migration-' + ('managed-' if managed else 'explicit-'), dir='/private/tmp')).resolve()
            root.chmod(0o700)
            roots.append(root)
            for name in ('home', 'tmp'):
                (root / name).mkdir(mode=0o700)
            core = root / 'home/Library/Application Support' / identifier / 'reasonix-core' if managed else root / 'core'
            core.mkdir(parents=True, mode=0o700)
            source = prepare_source(root)
            canary = core / 'ui-migration-original.canary'
            canary.touch(mode=0o600)
            canary.write_bytes(b'private original\n')
            originals = {path: original(path) for path in (source, canary)}
            identity = None
            for phase in ('before-import', 'after-import', 'after-undo'):
                receipt = root / 'tmp/reasonix-native-legacy-ui-receipt.json'
                # Every launch must perform its own real read. A previous
                # launch's receipt cannot satisfy the restart acceptance.
                receipt.unlink(missing_ok=True)
                env = environment | {'HOME': str(root / 'home'), 'TMPDIR': str(root / 'tmp'),
                                     'REASONIX_TAURI_LEGACY_UI_SMOKE': 'private-source'}
                if not managed:
                    # Match the explicit-profile contract: REASONIX_HOME
                    # selects its state; do not add a separate state override.
                    env |= {'REASONIX_HOME': str(core), 'REASONIX_CACHE_HOME': str(root / 'cache')}
                host = subprocess.Popen([str(host_binary)], env=env, stdout=subprocess.DEVNULL,
                                        stderr=subprocess.DEVNULL, start_new_session=True)
                sidecar = None
                control_payload = None
                try:
                    deadline = time.monotonic() + 25
                    while host.poll() is None and time.monotonic() < deadline:
                        ready = list((root / 'tmp').glob('reasonix-tauri-bridge-*/ready.json'))
                        children = package.own_sidecars(root / 'tmp', sidecar_binary)
                        if len(ready) == len(children) == 1:
                            sidecar = children[0]
                            package.check_unauthenticated_health(package.check_ready(ready[0]))
                            package.check_sidecar_profile(sidecar, core, managed, root / 'cache')
                            current = package.check_credential_profile(core)
                            if identity is not None and current != identity:
                                raise RuntimeError('UI migration restart changed profile identity')
                            identity = current
                            break
                        time.sleep(0.05)
                    else:
                        raise RuntimeError('Ordinary private Preview did not start')
                    control_payload = {'root': str(root), 'hostPid': host.pid,
                                       'sidecarPid': sidecar, 'phase': phase, 'managed': managed,
                                       'workspace': str(root / '旧界面 工作区'), 'state': 'running'}
                    publish_control(control, control_payload)
                    print(f"Ordinary {'managed' if managed else 'explicit'} UI phase {phase} ready", flush=True)
                    if host.wait(timeout=seconds) != 0:
                        raise RuntimeError('UI migration did not quit normally')
                    deadline = time.monotonic() + 5
                    while package.is_alive(sidecar) and time.monotonic() < deadline:
                        time.sleep(0.05)
                    if package.is_alive(sidecar) or package.own_sidecars(root / 'tmp', sidecar_binary) or list((root / 'tmp').glob('reasonix-tauri-bridge-*')):
                        raise RuntimeError('UI migration quit left sidecar/readiness')
                    if any(original(path) != value for path, value in originals.items()):
                        raise RuntimeError('UI migration changed its source or protected original')
                    check_source_receipt(receipt)
                    passed += 1
                    print(f"UI phase {phase}: private source, identity, originals and normal quit cleanup OK", flush=True)
                finally:
                    if control_payload is not None:
                        publish_control(control, control_payload | {'state': 'ending'})
                    if host.poll() is None:
                        os.killpg(host.pid, signal.SIGKILL)
                        host.wait(timeout=5)
                    for pid in package.own_sidecars(root / 'tmp', sidecar_binary):
                        try:
                            os.kill(pid, signal.SIGKILL)
                        except ProcessLookupError:
                            pass
                    if control_payload is not None:
                        publish_control(control, control_payload | {
                            'state': 'ended', 'exitCode': host.returncode,
                        })
        success = True
        print(f'{passed} ordinary UI lifecycles and source-isolation checks passed; native UI evidence must confirm import/restore behavior.', flush=True)
    finally:
        if success:
            control.unlink(missing_ok=True)
            for root in roots:
                shutil.rmtree(root)
        else:
            print('Private failure fixtures retained:', *(str(root) for root in roots), flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app')
    parser.add_argument('--control', default='/private/tmp/reasonix-ui-migration-control.json')
    parser.add_argument('--seconds', type=int, default=300, choices=range(60, 901))
    parser.add_argument('--profile', choices=('both', 'managed', 'explicit'), default='both',
                        help='run both profile modes, or only an independently unfinished mode')
    args = parser.parse_args()
    if sys.platform != 'darwin':
        raise SystemExit('This acceptance requires macOS')
    smoke(args.app, args.control, args.seconds, profile=args.profile)
