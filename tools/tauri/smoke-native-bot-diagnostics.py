#!/usr/bin/env python3
"""Own ordinary macOS bot diagnostic UI launches; never activate an IM gateway.

Use Settings > Bots > diagnostics, inspect legacy:feishu configuration separately
from runtime observation, refresh, then Cmd+Q. No save, restart, model submit,
clipboard or credentials action. Native UI proof must be recorded separately.
Require a live control PID AND its exact private WebView origin before inputs.
Fixtures and terminal receipts are retained even after success; they contain only
synthetic config, not real accounts. No smoke JavaScript is injected into the app.
"""
import argparse
import importlib.util
import os
from pathlib import Path
import plistlib
import signal
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True


def sibling(name):
    spec = importlib.util.spec_from_file_location(name.replace('-', '_'), Path(__file__).with_name(name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


package = sibling('smoke-packaged-app')
launch_services = sibling('launch-services-host')
migration = sibling('smoke-native-ui-migration')


def prepare(root, managed):
    root.chmod(0o700)
    for name in ('home', 'tmp'):
        (root / name).mkdir(mode=0o700)
    core = (root / 'home/Library/Application Support/io.reasonix.desktop.preview/reasonix-core'
            if managed else root / 'core')
    core.mkdir(parents=True, mode=0o700)
    config = core / 'config.toml'
    config.touch(mode=0o600)
    config.write_text('credentials_store = "file"\n'
                      '[desktop]\nlanguage = "zh"\n'
                      '[bot]\nenabled = false\n'
                      '[bot.feishu]\nenabled = true\ndomain = "feishu"\n'
                      'app_id = "synthetic-native-diagnostic-account"\n'
                      'app_secret_env = "SYNTHETIC_DIAGNOSTIC_NO_SECRET"\n')
    # Minimal runner environment; ambient provider keys/proxies never forwarded.
    env = {key: value for key, value in os.environ.items()
           if key in ('PATH', 'LANG', 'LC_ALL', 'USER', 'LOGNAME')}
    env |= {'HOME': str(root / 'home'), 'TMPDIR': str(root / 'tmp')}
    if not managed:
        env |= {'REASONIX_HOME': str(core), 'REASONIX_CACHE_HOME': str(root / 'cache')}
    return core, config, env


def smoke(app, profile, seconds):
    app = Path(app).resolve()
    identifier = plistlib.loads((app / 'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
    if identifier != 'io.reasonix.desktop.preview' or package.matching_package_is_running(identifier):
        raise RuntimeError('Preview identity invalid or another instance is running')
    subprocess.run(['/usr/bin/codesign', '--verify', '--deep', '--strict', str(app)],
                   check=True, capture_output=True)
    binary, sidecar_binary = (app / 'Contents/MacOS' / name for name in
                              ('reasonix-tauri', 'reasonix-desktop-bridge'))
    modes = (True, False) if profile == 'both' else (profile == 'managed',)
    for managed in modes:
        root = Path(tempfile.mkdtemp(prefix='reasonix-native-bot-diagnostics-', dir='/private/tmp')).resolve()
        core, config, env = prepare(root, managed)
        original = migration.original(config)
        control = root / 'control.json'
        host = None
        sidecar = None
        passed = False
        payload = {'root': str(root), 'managed': managed, 'state': 'preparing'}
        migration.publish_control(control, payload)
        try:
            host = launch_services.LaunchServicesHost(binary, sidecar_binary, root, env, package,
                                                     ordinary_bot_diagnostics=True)
            deadline = time.monotonic() + 25
            while host.poll() is None and time.monotonic() < deadline:
                ready = list((root / 'tmp').glob('reasonix-tauri-bridge-*/ready.json'))
                children = package.own_sidecars(root / 'tmp', sidecar_binary)
                if len(ready) == len(children) == 1:
                    sidecar = children[0]
                    package.check_unauthenticated_health(package.check_ready(ready[0]))
                    package.check_sidecar_profile(sidecar, core, managed, root / 'cache')
                    identity = package.check_credential_profile(core)
                    payload |= {'hostPid': host.pid, 'sidecarPid': sidecar,
                                'webviewOrigin': f'reasonix-preview://{identity}.localhost/',
                                'state': 'running'}
                    migration.publish_control(control, payload)
                    print('Private ordinary bot UI ready: ' + str(control), flush=True)
                    break
                time.sleep(.05)
            else:
                raise RuntimeError('Private bot diagnostic instance did not become ready')
            if host.wait(timeout=seconds) != 0:
                raise RuntimeError('Private bot diagnostic instance did not quit normally')
            deadline = time.monotonic() + 5
            while package.is_alive(sidecar) and time.monotonic() < deadline:
                time.sleep(.05)
            if package.is_alive(sidecar) or package.own_sidecars(root / 'tmp', sidecar_binary) or list((root / 'tmp').glob('reasonix-tauri-bridge-*')):
                raise RuntimeError('Private bot diagnostic quit left sidecar/readiness')
            if migration.original(config) != original or package.check_credential_profile(core) != identity:
                raise RuntimeError('Read-only UI changed config bytes/permissions/mtime or profile identity')
            passed = True
            print('Private lifecycle, config immutability, identity and cleanup PASS; native UI evidence required separately.', flush=True)
        finally:
            migration.publish_control(control, payload | {'state': 'ending'})
            if host is not None:
                if host.poll() is None:
                    host.kill()
                    host.wait(timeout=5)
                host.close()
            for pid in package.own_sidecars(root / 'tmp', sidecar_binary):
                try:
                    os.kill(pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            migration.publish_control(control, payload | {'state': 'ended',
                                      'lifecyclePassed': passed,
                                      'exitCode': None if host is None else host.returncode})
            print('Synthetic fixture retained: ' + str(root), flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app')
    parser.add_argument('--profile', choices=('both', 'managed', 'explicit'), default='both')
    parser.add_argument('--seconds', type=int, choices=range(60, 901), default=300)
    args = parser.parse_args()
    if sys.platform != 'darwin':
        raise SystemExit('Requires macOS native UI')
    smoke(args.app, args.profile, args.seconds)
