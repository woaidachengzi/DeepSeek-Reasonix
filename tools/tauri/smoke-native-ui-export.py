#!/usr/bin/env python3
"""Own ordinary Preview launches for actual export Save/Cancel/Replace UI.

In each private profile, start/mark a frontend trace through Settings >
Diagnostics. Export and cancel the real panel; --record cancel checks no file.
Export the retained trace to outputFile; --record new-save checks the actual
report. Start/mark another trace, select the same name and cancel Replace;
--record overwrite-cancel checks the original bytes/metadata are untouched.
Export the retained second trace, accept Replace, --record overwrite, then
Cmd+Q. File/lifecycle assertions never replace actual panel/feedback evidence.

For --scenario theme, use Settings > Appearance > Browse Themes, create the
image-free graphite-based theme named THEME_NAMES[0], then export/cancel/save.
Edit the same theme name to THEME_NAMES[1] before checking Cancel/Replace.
--record derives its scenario from the owned live control, not a CLI override.
For --scenario document, send the published fixed prompt to the loopback-only
provider. Original A is used for cancel/new-save; Original B for Cancel/Replace
to the same output. Neither prepared source is changed by the runner or UI.
The document-errors slice requires actual same-source Replace rejection followed
by Missing source rejection without a Save panel. Neither may write any file.
"""
import argparse
import hashlib
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
import zipfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('package', Path(__file__).with_name('smoke-packaged-app.py'))
package = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package)
document_spec = importlib.util.spec_from_file_location('document_provider', Path(__file__).with_name('native-document-provider.py'))
documents = importlib.util.module_from_spec(document_spec)
document_spec.loader.exec_module(documents)
STEPS = ('cancel', 'new-save', 'overwrite-cancel', 'overwrite')
ERROR_STEPS = ('same-source', 'missing-source')
THEME_NAMES = ('Reasonix UI Theme', 'Reasonix UI Theme Revised')


def private_file(path, maximum):
    info = path.lstat()
    if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid()
            or stat.S_IMODE(info.st_mode) != 0o600 or info.st_size > maximum):
        raise RuntimeError('Export acceptance file is not an owned bounded private regular file')
    return info


def original(path):
    info = path.lstat()
    return path.read_bytes(), info.st_mode, info.st_mtime_ns, info.st_ino


def document_sources(mode):
    return [mode / '原件 文件' / f'原件 {letter}.md' for letter in ('A', 'B')]


def fingerprint(path):
    private_file(path, 4096)
    info = path.stat()
    return {'sha256': hashlib.sha256(path.read_bytes()).hexdigest(), 'mode': info.st_mode,
            'mtime': info.st_mtime_ns, 'inode': info.st_ino}


def output_name(scenario):
    return {'theme': '主题 副本.reasonix-theme', 'diagnostics': '报告 副本.json',
            'document': '文档 副本.md', 'document-errors': '文档 副本.md'}[scenario]


def report(path, scenario='diagnostics'):
    private_file(path, 8 << 20)
    if scenario == 'document':
        data = path.read_bytes()
        if data not in documents.CONTENTS:
            raise RuntimeError('Saved document differs from both prepared originals')
        return 'AB'[documents.CONTENTS.index(data)]
    if scenario == 'theme':
        with zipfile.ZipFile(path) as archive:
            if archive.namelist() != ['theme.json'] or archive.getinfo('theme.json').file_size > 1 << 20:
                raise RuntimeError('Actual image-free theme export archive is invalid')
            data = json.loads(archive.read('theme.json'))
        if (not isinstance(data, dict) or type(data.get('schemaVersion')) is not int
                or data['schemaVersion'] != 2 or data.get('id') != 'user-reasonix-ui-theme'
                or data.get('name') not in THEME_NAMES or data.get('baseStyle') != 'graphite'
                or not isinstance(data.get('tokens'), dict)
                or data.get('recipes') != {'density': 'comfortable', 'corners': 'soft'}
                or data.get('background') or data.get('taskBackground')):
            raise RuntimeError('Actual theme export manifest is invalid')
        for mode, expected in (('light', {'bg': '#fcf8ee', 'fg': '#2a241b', 'accent': '#7a5a16'}),
                               ('dark', {'bg': '#151515', 'fg': '#f4f4f4', 'accent': '#ff6a45'})):
            tokens = data['tokens'].get(mode)
            if not isinstance(tokens, dict) or any(tokens.get(key) != value for key, value in expected.items()):
                raise RuntimeError('Actual theme export lost its prepared tokens')
        return data['name']
    if scenario != 'diagnostics':
        raise ValueError('Unknown export acceptance scenario')
    data = json.loads(path.read_bytes())
    if not isinstance(data, dict) or not isinstance(data.get('manifest'), dict):
        raise RuntimeError('Actual exported report schema/identity/user marker is invalid')
    events = data.get('events')
    identifier = data.get('manifest', {}).get('reportId')
    if (type(data.get('schemaVersion')) is not int or data['schemaVersion'] != 2
            or not isinstance(identifier, str) or not 1 <= len(identifier) <= 128
            or not isinstance(events, list) or not events
            or not any(isinstance(event, dict) and event.get('type') == 'marker' for event in events)):
        raise RuntimeError('Actual exported report schema/identity/user marker is invalid')
    return identifier


def record(step, control_path):
    if step not in STEPS + ERROR_STEPS:
        raise ValueError('Unknown export acceptance step')
    control = Path(control_path)
    private_file(control, 8192)
    state = json.loads(control.read_text())
    root = Path(state['root'])
    if (root.parent != Path('/private/tmp') or root.resolve() != root
            or not re.fullmatch(r'reasonix-native-ui-export-[0-9a-f]{32}', root.name)
            or type(state['managed']) is not bool):
        raise RuntimeError('Export UI control does not identify a private fixture')
    mode = root / ('managed' if state['managed'] else 'explicit')
    scenario = state.get('scenario', 'diagnostics')
    if scenario not in ('diagnostics', 'theme', 'document', 'document-errors'):
        raise RuntimeError('Export UI scenario is invalid')
    output = mode / '报告 测试'
    for directory in (root, mode, output):
        info = directory.lstat()
        if (not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid()
                or stat.S_IMODE(info.st_mode) != 0o700):
            raise RuntimeError('Export UI directory is not an owned private ordinary directory')
    target = output / output_name(scenario)
    if state['outputFile'] != str(target) or state['phase'] != step:
        raise RuntimeError('Export UI step/path does not match the current phase')
    if any(type(state[key]) is not int or not package.is_alive(state[key])
           for key in ('hostPid', 'sidecarPid')):
        raise RuntimeError('Export UI fixture is no longer live')
    if scenario in ('document', 'document-errors'):
        info = (mode / '原件 文件').lstat()
        if (not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid()
                or stat.S_IMODE(info.st_mode) != 0o700):
            raise RuntimeError('Document source directory is not private')
        sources = document_sources(mode)
        if (state.get('sourceFiles') != [str(path) for path in sources]
                or [fingerprint(path) for path in sources] != state.get('sourceFingerprints')
                or [path.read_bytes() for path in sources] != list(documents.CONTENTS)):
            raise RuntimeError('Document source protection failed')
        if set((mode / '原件 文件').iterdir()) != set(sources):
            raise RuntimeError('Document source directory changed')
    receipt = mode / 'ui-export-receipt.json'
    prior = {'steps': []}
    has_receipt = receipt.exists() or receipt.is_symlink()
    if has_receipt:
        private_file(receipt, 8192)
        prior = json.loads(receipt.read_text())
    steps = ERROR_STEPS if scenario == 'document-errors' else STEPS
    if step not in steps:
        raise RuntimeError('Step does not belong to the controlled scenario')
    index = steps.index(step)
    if prior['steps'] != list(steps[:index]):
        raise RuntimeError('Export UI receipt order is invalid')
    backup = mode / 'first-export-original.json'
    if step == 'cancel' or scenario == 'document-errors':
        if list(output.iterdir()):
            raise RuntimeError('Cancelled save unexpectedly wrote an output')
    else:
        if set(output.iterdir()) != {target}:
            raise RuntimeError('Export left unexpected output files')
        identifier = report(target, scenario)
        if step == 'new-save':
            if scenario == 'document' and identifier != 'A':
                raise RuntimeError('First Save As did not copy Original A')
            if scenario == 'theme' and identifier != THEME_NAMES[0]:
                raise RuntimeError('First export did not publish the created theme')
            private_file(target, 8 << 20)
            backup.touch(mode=0o600, exist_ok=False)
            backup.write_bytes(target.read_bytes())
            info = target.stat()
            prior |= {'firstId': identifier, 'firstMode': info.st_mode,
                      'firstMtime': info.st_mtime_ns, 'firstInode': info.st_ino}
        else:
            private_file(backup, 8 << 20)
            if step == 'overwrite-cancel':
                info = target.stat()
                if (target.read_bytes() != backup.read_bytes()
                        or (info.st_mode, info.st_mtime_ns, info.st_ino)
                        != (prior['firstMode'], prior['firstMtime'], prior['firstInode'])):
                    raise RuntimeError('Cancelled Replace changed the original export')
            elif identifier == prior['firstId'] or target.read_bytes() == backup.read_bytes():
                raise RuntimeError('Accepted Replace did not publish the second report')
    prior['steps'].append(step)
    if not has_receipt:
        receipt.touch(mode=0o600, exist_ok=False)
    receipt.write_text(json.dumps(prior))
    state['phase'] = steps[index + 1] if index + 1 < len(steps) else 'complete'
    control.write_text(json.dumps(state))
    print(f'Actual output check {step} passed; panel/feedback evidence remains separate', flush=True)


def smoke(app_path, control_path, seconds, profile, scenario='diagnostics'):
    if profile not in ('managed', 'explicit', 'both') or not 60 <= seconds <= 900:
        raise ValueError('Invalid export UI profile/duration')
    if scenario not in ('diagnostics', 'theme', 'document', 'document-errors'):
        raise ValueError('Invalid export UI scenario')
    control = Path(control_path)
    if control.parent != Path('/private/tmp') or control.parent.resolve() != control.parent:
        raise ValueError('Export UI control must be directly inside /private/tmp')
    app = Path(app_path).resolve()
    identifier = plistlib.loads((app / 'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
    if identifier != 'io.reasonix.desktop.preview' or package.matching_package_is_running(identifier):
        raise RuntimeError('Preview missing or already running')
    subprocess.run(['/usr/bin/codesign', '--verify', '--deep', '--strict', str(app)], check=True, capture_output=True)
    host_binary = app / 'Contents/MacOS/reasonix-tauri'
    sidecar_binary = app / 'Contents/MacOS/reasonix-desktop-bridge'
    environment = {key: value for key, value in os.environ.items()
                   if key in ('PATH', 'USER', 'LOGNAME', 'LANG', 'LC_ALL', '__CF_USER_TEXT_ENCODING')}
    root = Path('/private/tmp') / ('reasonix-native-ui-export-' + uuid.uuid4().hex)
    root.mkdir(mode=0o700)
    success, passed = False, 0
    try:
        # Refuse existing controls, including symlinks, before launching any host.
        control.touch(mode=0o600, exist_ok=False)
        modes = (True, False) if profile == 'both' else (profile == 'managed',)
        for managed in modes:
            mode = root / ('managed' if managed else 'explicit')
            mode.mkdir(mode=0o700)
            for name in ('home', 'tmp', '报告 测试'):
                (mode / name).mkdir(mode=0o700)
            core = mode / 'home/Library/Application Support' / identifier / 'reasonix-core' if managed else mode / 'core'
            core.mkdir(parents=True, mode=0o700)
            canary = core / 'ui-export-original.canary'
            canary.touch(mode=0o600)
            canary.write_bytes(b'private export original\n')
            before = original(canary)
            provider = None
            sources, source_fingerprints = [], []
            if scenario in ('document', 'document-errors'):
                (mode / '原件 文件').mkdir(mode=0o700)
                sources = document_sources(mode)
                for path, data in zip(sources, documents.CONTENTS):
                    path.touch(mode=0o600, exist_ok=False)
                    path.write_bytes(data)
                source_fingerprints = [fingerprint(path) for path in sources]
            env = environment | {'HOME': str(mode / 'home'), 'TMPDIR': str(mode / 'tmp')}
            if not managed:
                env |= {'REASONIX_HOME': str(core), 'REASONIX_CACHE_HOME': str(mode / 'cache')}
            host = None
            sidecar = None
            try:
                if scenario in ('document', 'document-errors'):
                    provider = documents.DocumentProvider(core, sources, errors=scenario == 'document-errors')
                host = subprocess.Popen([str(host_binary)], env=env, stdout=subprocess.DEVNULL,
                                        stderr=subprocess.DEVNULL, start_new_session=True)
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
                    raise RuntimeError('Ordinary export UI host did not start')
                control.write_text(json.dumps({'root': str(root), 'managed': managed, 'hostPid': host.pid,
                                               'sidecarPid': sidecar, 'phase': 'same-source' if scenario == 'document-errors' else 'cancel',
                                               'scenario': scenario,
                                               'sourceFiles': [str(path) for path in sources],
                                               'sourceFingerprints': source_fingerprints,
                                               'prompt': documents.PROMPT if scenario in ('document', 'document-errors') else '',
                                               'themeNames': list(THEME_NAMES) if scenario == 'theme' else [],
                                               'outputFile': str(mode / '报告 测试' / output_name(scenario))}))
                control.chmod(0o600)
                print(f"Ordinary {'managed' if managed else 'explicit'} export UI ready", flush=True)
                if host.wait(timeout=seconds) != 0:
                    raise RuntimeError('Export UI host did not quit normally')
                deadline = time.monotonic() + 5
                while package.is_alive(sidecar) and time.monotonic() < deadline:
                    time.sleep(0.05)
                if (package.is_alive(sidecar) or package.own_sidecars(mode / 'tmp', sidecar_binary)
                        or list((mode / 'tmp').glob('reasonix-tauri-bridge-*'))):
                    raise RuntimeError('Export UI quit left sidecar/readiness')
                receipt = mode / 'ui-export-receipt.json'
                private_file(receipt, 8192)
                state = json.loads(receipt.read_text())
                target = mode / '报告 测试' / output_name(scenario)
                steps = ERROR_STEPS if scenario == 'document-errors' else STEPS
                if state['steps'] != list(steps):
                    raise RuntimeError('Export UI steps were incomplete')
                if scenario == 'document-errors':
                    if list(target.parent.iterdir()) or set((mode / '原件 文件').iterdir()) != set(sources):
                        raise RuntimeError('Rejected document action wrote unexpected files')
                elif report(target, scenario) == state['firstId']:
                    raise RuntimeError('Export UI replacement was incomplete')
                if original(canary) != before or package.check_credential_profile(core) != identity:
                    raise RuntimeError('Export UI changed its protected original/profile identity')
                if scenario in ('document', 'document-errors') and (provider.requests != 1 or provider.error
                                               or [fingerprint(path) for path in sources] != source_fingerprints):
                    raise RuntimeError('Document provider or source protection did not pass')
                passed += 1
                print(f'{len(steps)} actual output checks, original/identity protection and normal quit cleanup passed', flush=True)
            finally:
                if host is not None and host.poll() is None:
                    os.killpg(host.pid, signal.SIGKILL)
                    host.wait(timeout=5)
                deadline = time.monotonic() + 5
                while package.own_sidecars(mode / 'tmp', sidecar_binary) and time.monotonic() < deadline:
                    time.sleep(0.05)
                for pid in package.own_sidecars(mode / 'tmp', sidecar_binary):
                    try:
                        os.kill(pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                if provider is not None:
                    provider.close()
        success = True
        print(f'{passed} ordinary export UI lifecycles passed; native panel/UI evidence is separate', flush=True)
    finally:
        if success:
            control.unlink(missing_ok=True)
            shutil.rmtree(root)
        else:
            print('Private failed export fixture retained:', root, flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app', nargs='?')
    parser.add_argument('--control', default='/private/tmp/reasonix-ui-export-control.json')
    parser.add_argument('--seconds', type=int, default=600, choices=range(60, 901), metavar='SECONDS')
    parser.add_argument('--profile', choices=('managed', 'explicit', 'both'), default='both')
    parser.add_argument('--scenario', choices=('diagnostics', 'theme', 'document', 'document-errors'), default='diagnostics')
    parser.add_argument('--record', choices=STEPS + ERROR_STEPS)
    args = parser.parse_args()
    if sys.platform != 'darwin':
        raise SystemExit('This acceptance requires macOS')
    if args.record:
        record(args.record, args.control)
    elif args.app:
        smoke(args.app, args.control, args.seconds, args.profile, args.scenario)
    else:
        parser.error('app is required unless --record is provided')
