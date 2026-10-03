#!/usr/bin/env python3
"""Own private ordinary Preview launches for actual theme import and restart UI.

In Settings > Appearance > Browse Themes, cancel Import; import sourceFile;
import it again; import invalidFile and observe the error; then Cmd+Q. On the
second launch verify both themes and their home/task previews remain, record
restart, then Cmd+Q. --record checks disk evidence only, never operates UI.
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
spec = importlib.util.spec_from_file_location('export_ui', Path(__file__).with_name('smoke-native-ui-export.py'))
export_ui = importlib.util.module_from_spec(spec)
spec.loader.exec_module(export_ui)
package = export_ui.package
STEPS = ('cancel', 'import', 'duplicate', 'invalid', 'restart')
IDS = ('user-ui-import', 'user-ui-import-2')
NAME = 'Imported UI Theme'


def preference_file(path):
    info = path.lstat()
    if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid()
            or stat.S_IMODE(info.st_mode) not in (0o600, 0o644) or info.st_size > 1 << 20):
        raise RuntimeError('Theme preferences are not bounded/owned/ordinary')
    return info


def fingerprint(path, preferences=False):
    info = preference_file(path) if preferences else export_ui.private_file(path, 8 << 20)
    return [hashlib.sha256(path.read_bytes()).hexdigest(), info.st_mode, info.st_mtime_ns, info.st_ino]


def prepare_source(mode):
    source = mode / '导入 原件'
    source.mkdir(mode=0o700)
    repository = Path(__file__).resolve().parents[2]
    official = repository / 'desktop/themes/official'
    manifest = json.loads((official / 'official-crimson-horizon/theme.json').read_text())
    manifest |= {'schemaVersion': 2, 'id': 'ui-import', 'name': NAME,
                 'recipes': {'density': 'compact', 'corners': 'round'},
                 'background': {'image': 'home.webp', 'focusX': 0.5, 'focusY': 0.5, 'safeArea': 'center',
                                'homeOpacity': 1, 'taskOpacity': 0.28, 'overlayStrength': 0.1, 'paneOpacity': 0.5},
                 'taskBackground': {'image': 'task.webp', 'focusX': 0.5, 'focusY': 0.5, 'safeArea': 'center',
                                    'opacity': 1, 'overlayStrength': 0.1, 'paneOpacity': 0.5}}
    for destination, origin in (('home.webp', 'official-crimson-horizon'), ('task.webp', 'official-sage-breeze')):
        target = source / destination
        target.touch(mode=0o600, exist_ok=False)
        target.write_bytes((official / origin / 'background.webp').read_bytes())
    manifest_path = source / 'expected-manifest.json'
    manifest_path.touch(mode=0o600, exist_ok=False)
    manifest_path.write_text(json.dumps(manifest))
    for name, schema in (('主题 原件.reasonix-theme', 2), ('无效 主题.reasonix-theme', 99)):
        target = source / name
        target.touch(mode=0o600, exist_ok=False)
        with zipfile.ZipFile(target, 'w', compression=zipfile.ZIP_DEFLATED) as archive:
            archive.writestr('theme.json', json.dumps(manifest | {'schemaVersion': schema}))
            for image in ('home.webp', 'task.webp'):
                archive.writestr(image, (source / image).read_bytes())
    snapshot = source / 'originals.json'
    snapshot.touch(mode=0o600, exist_ok=False)
    snapshot.write_text(json.dumps({name: fingerprint(source / name) for name in
                                   ('home.webp', 'task.webp', 'expected-manifest.json', '主题 原件.reasonix-theme', '无效 主题.reasonix-theme')}))


def check_sources(mode):
    source = mode / '导入 原件'
    snapshot = source / 'originals.json'
    export_ui.private_file(snapshot, 8192)
    saved = json.loads(snapshot.read_text())
    names = ('home.webp', 'task.webp', 'expected-manifest.json', '主题 原件.reasonix-theme', '无效 主题.reasonix-theme')
    if set(saved) != set(names) or any(fingerprint(source / name) != saved[name] for name in names):
        raise RuntimeError('Theme import changed a source original')
    return json.loads((source / 'expected-manifest.json').read_text())


def preferences(mode):
    path = mode / 'home/Library/Application Support/io.reasonix.desktop.preview/host-preferences.json'
    if path.exists() or path.is_symlink():
        preference_file(path)
        data = json.loads(path.read_text())
        if not isinstance(data, dict) or not isinstance(data.get('userThemes'), list):
            raise RuntimeError('Theme preferences are invalid')
        return path, data['userThemes']
    return path, []


def check_themes(mode, expected_count):
    manifest = check_sources(mode)
    path, themes = preferences(mode)
    if [theme.get('id') for theme in themes] != list(IDS[:expected_count]):
        raise RuntimeError('Imported theme count/IDs are invalid')
    asset_root = path.parent / 'theme-assets'
    asset_entries = set(entry.name for entry in asset_root.iterdir()) if asset_root.exists() else set()
    if asset_entries != set(IDS[:expected_count]):
        raise RuntimeError('Theme import left unexpected/staging asset directories')
    for theme in themes:
        for key in ('name', 'author', 'description', 'license', 'baseStyle'):
            if theme.get(key) != manifest[key]:
                raise RuntimeError('Imported theme metadata/tokens changed')
        normalized_tokens = {scheme: {key: value.lower() for key, value in tokens.items()}
                             for scheme, tokens in manifest['tokens'].items()}
        if theme.get('tokens') != normalized_tokens:
            raise RuntimeError('Imported theme tokens changed beyond hex normalization')
        if (theme.get('density'), theme.get('corners')) != ('compact', 'round'):
            raise RuntimeError('Imported theme recipes changed')
        assets = asset_root / theme['id']
        info = assets.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid():
            raise RuntimeError('Theme asset directory is not owned and ordinary')
        if set(entry.name for entry in assets.iterdir()) != {'background.webp', 'background-task.webp'}:
            raise RuntimeError('Imported asset names are invalid')
        for field, destination, source in (('background', 'background.webp', 'home.webp'),
                                           ('taskBackground', 'background-task.webp', 'task.webp')):
            if theme.get(field) != manifest[field] | {'image': destination}:
                raise RuntimeError('Imported scene background metadata changed')
            asset = assets / destination
            info = asset.lstat()
            if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_size > 8 << 20
                    or asset.read_bytes() != (mode / '导入 原件' / source).read_bytes()):
                raise RuntimeError('Imported image bytes changed')
    return path, themes


def record(step, control_path):
    if step not in STEPS:
        raise ValueError('Unknown import UI step')
    control = Path(control_path)
    export_ui.private_file(control, 8192)
    state = json.loads(control.read_text())
    root = Path(state['root'])
    if (root.parent != Path('/private/tmp') or root.resolve() != root
            or not re.fullmatch(r'reasonix-native-ui-theme-import-[0-9a-f]{32}', root.name)
            or type(state['managed']) is not bool):
        raise RuntimeError('Import control does not identify a private fixture')
    mode = root / ('managed' if state['managed'] else 'explicit')
    for directory in (root, mode, mode / 'home', mode / 'tmp', mode / '导入 原件'):
        info = directory.lstat()
        if (not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid()
                or stat.S_IMODE(info.st_mode) != 0o700):
            raise RuntimeError('Import fixture directory is not private/owned/ordinary')
    if (state['phase'] != step or state['sourceFile'] != str(mode / '导入 原件/主题 原件.reasonix-theme')
            or state['invalidFile'] != str(mode / '导入 原件/无效 主题.reasonix-theme')
            or any(type(state[key]) is not int or not package.is_alive(state[key]) for key in ('hostPid', 'sidecarPid'))):
        raise RuntimeError('Import UI step/path/live processes do not match')
    receipt = mode / 'ui-import-receipt.json'
    has_receipt = receipt.exists() or receipt.is_symlink()
    prior = {'steps': []}
    if has_receipt:
        export_ui.private_file(receipt, 8192)
        prior = json.loads(receipt.read_text())
    index = STEPS.index(step)
    if prior['steps'] != list(STEPS[:index]):
        raise RuntimeError('Import UI receipt order is invalid')
    path, themes = check_themes(mode, 0 if step == 'cancel' else 1 if step == 'import' else 2)
    first = mode / 'first-imported-theme.json'
    after = mode / 'after-duplicate-preferences.json'
    if step == 'import':
        first.touch(mode=0o600, exist_ok=False)
        first.write_text(json.dumps(themes[0]))
    elif step in ('duplicate', 'invalid', 'restart'):
        export_ui.private_file(first, 1 << 20)
        if themes[0] != json.loads(first.read_text()):
            raise RuntimeError('Duplicate import changed the first saved theme')
        if step == 'duplicate':
            after.touch(mode=0o600, exist_ok=False)
            after.write_bytes(path.read_bytes())
            prior['preferences'] = fingerprint(path, preferences=True)
        else:
            export_ui.private_file(after, 1 << 20)
            if path.read_bytes() != after.read_bytes() or fingerprint(path, preferences=True) != prior['preferences']:
                raise RuntimeError('Invalid import/restart changed saved preferences')
    prior['steps'].append(step)
    if not has_receipt:
        receipt.touch(mode=0o600, exist_ok=False)
    receipt.write_text(json.dumps(prior))
    state['phase'] = STEPS[index + 1] if index + 1 < len(STEPS) else 'complete'
    control.write_text(json.dumps(state))
    print(f'Actual disk check {step} passed; panel/preview/error evidence remains separate', flush=True)


def smoke(app_path, control_path, seconds, profile):
    if profile not in ('managed', 'explicit', 'both') or not 60 <= seconds <= 900:
        raise ValueError('Invalid import UI profile/duration')
    control = Path(control_path)
    if control.parent != Path('/private/tmp') or control.parent.resolve() != control.parent:
        raise ValueError('Import control must be directly inside /private/tmp')
    app = Path(app_path).resolve()
    identifier = plistlib.loads((app / 'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
    if identifier != 'io.reasonix.desktop.preview' or package.matching_package_is_running(identifier):
        raise RuntimeError('Preview missing or already running')
    subprocess.run(['/usr/bin/codesign', '--verify', '--deep', '--strict', str(app)], check=True, capture_output=True)
    host_binary, sidecar_binary = (app / 'Contents/MacOS' / name for name in ('reasonix-tauri', 'reasonix-desktop-bridge'))
    environment = {key: value for key, value in os.environ.items()
                   if key in ('PATH', 'USER', 'LOGNAME', 'LANG', 'LC_ALL', '__CF_USER_TEXT_ENCODING')}
    root = Path('/private/tmp') / ('reasonix-native-ui-theme-import-' + uuid.uuid4().hex)
    root.mkdir(mode=0o700)
    success, passed = False, 0
    try:
        control.touch(mode=0o600, exist_ok=False)
        for managed in ((True, False) if profile == 'both' else (profile == 'managed',)):
            mode = root / ('managed' if managed else 'explicit')
            mode.mkdir(mode=0o700)
            for name in ('home', 'tmp'):
                (mode / name).mkdir(mode=0o700)
            core = mode / 'home/Library/Application Support' / identifier / 'reasonix-core' if managed else mode / 'core'
            core.mkdir(parents=True, mode=0o700)
            canary = core / 'ui-theme-import-original.canary'
            canary.touch(mode=0o600, exist_ok=False)
            canary.write_bytes(b'private theme import original\n')
            before = fingerprint(canary)
            prepare_source(mode)
            identity = None
            for phase in ('cancel', 'restart'):
                env = environment | {'HOME': str(mode / 'home'), 'TMPDIR': str(mode / 'tmp')}
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
                            current_identity = package.check_credential_profile(core)
                            if identity is not None and current_identity != identity:
                                raise RuntimeError('Theme restart changed profile identity')
                            identity = current_identity
                            break
                        time.sleep(0.05)
                    else:
                        raise RuntimeError('Ordinary import UI host did not start')
                    control.write_text(json.dumps({'root': str(root), 'managed': managed, 'phase': phase,
                                                   'hostPid': host.pid, 'sidecarPid': sidecar,
                                                   'sourceFile': str(mode / '导入 原件/主题 原件.reasonix-theme'),
                                                   'invalidFile': str(mode / '导入 原件/无效 主题.reasonix-theme')}))
                    print(f"Ordinary {'managed' if managed else 'explicit'} theme {phase} UI ready", flush=True)
                    if host.wait(timeout=seconds) != 0:
                        raise RuntimeError('Theme import host did not quit normally')
                    deadline = time.monotonic() + 5
                    while package.is_alive(sidecar) and time.monotonic() < deadline:
                        time.sleep(0.05)
                    if (package.is_alive(sidecar) or package.own_sidecars(mode / 'tmp', sidecar_binary)
                            or list((mode / 'tmp').glob('reasonix-tauri-bridge-*'))):
                        raise RuntimeError('Theme import quit left sidecar/readiness')
                    receipt = mode / 'ui-import-receipt.json'
                    export_ui.private_file(receipt, 8192)
                    expected = list(STEPS[:4] if phase == 'cancel' else STEPS)
                    if json.loads(receipt.read_text())['steps'] != expected:
                        raise RuntimeError('Actual import UI steps were incomplete')
                    check_themes(mode, 2)
                    if fingerprint(canary) != before or package.check_credential_profile(core) != identity:
                        raise RuntimeError('Theme import changed protected original/identity')
                    passed += 1
                    print('Actual disk steps, sources, identity/original and normal quit cleanup passed', flush=True)
                finally:
                    if host.poll() is None:
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
        success = True
        print(f'{passed} ordinary import/restart UI lifecycles passed; actual UI evidence is separate', flush=True)
    finally:
        if success:
            control.unlink(missing_ok=True)
            shutil.rmtree(root)
        else:
            print('Private failed theme import fixture retained:', root, flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app', nargs='?')
    parser.add_argument('--control', default='/private/tmp/reasonix-ui-theme-import-control.json')
    parser.add_argument('--seconds', type=int, default=600, choices=range(60, 901), metavar='SECONDS')
    parser.add_argument('--profile', choices=('managed', 'explicit', 'both'), default='both')
    parser.add_argument('--record', choices=STEPS)
    args = parser.parse_args()
    if sys.platform != 'darwin':
        raise SystemExit('This acceptance requires macOS')
    if args.record:
        record(args.record, args.control)
    elif args.app:
        smoke(args.app, args.control, args.seconds, args.profile)
    else:
        parser.error('app is required unless --record is provided')
