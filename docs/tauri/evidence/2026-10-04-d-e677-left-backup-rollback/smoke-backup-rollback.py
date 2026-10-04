#!/usr/bin/env python3
"""Apply a real Preview config backup to a private official Wails CLI profile.

Does not restore the user's profile or claim complete data/GUI rollback.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tomllib

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('profiles', Path(__file__).with_name('smoke-profile-import.py'))
profiles = importlib.util.module_from_spec(spec)
spec.loader.exec_module(profiles)
OFFICIAL_EMBEDDED_CLI = '5b1ab31424d45c8bf3cfe6a60b11da527df2252aee963d1c0c56352f57945962'


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def smoke(app, cli, output, window_state=None):
    if not cli.is_file() or cli.is_symlink() or digest(cli) != OFFICIAL_EMBEDDED_CLI:
        raise RuntimeError('official Wails 1.38.3 embedded CLI digest differs')
    binaries = [app / 'Contents/MacOS' / name
                for name in ('reasonix-tauri', 'reasonix-desktop-bridge')]
    identity = {path.name: digest(path) for path in binaries}
    subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)
    output.mkdir(mode=0o700, parents=False, exist_ok=False)
    (output / 'artifact.json').write_text(json.dumps({
        'app': str(app), 'artifactSha256': identity, 'legacyCliSha256': digest(cli),
        'scope': 'Actual single config backup application and read-only official CLI recovery',
    }, indent=2) + '\n')
    sources = output / 'scripts'
    sources.mkdir(mode=0o700)
    for path in sorted(Path(__file__).parent.iterdir()):
        if path.is_file() and not path.is_symlink():
            shutil.copy2(path, sources / path.name)
    original_launch = profiles.launch
    receipts = []

    def launch(app_path, identifier, root, phase, configuration):
        if phase == 'explicit':
            if profiles.package.matching_package_is_running(identifier):
                raise RuntimeError('Preview must be stopped before backup application')
            core = root / 'home/Library/Application Support' / identifier / 'reasonix-core'
            if profiles.package.own_sidecars(root / 'tmp', binaries[1]):
                raise RuntimeError('Preview sidecar still owns the source profile')
            backups = list((core / 'backups').glob('*/config.toml'))
            if len(backups) != 1 or backups[0].is_symlink() or backups[0].read_bytes() != configuration:
                raise RuntimeError('imported original backup differs')
            backup = backups[0]
            original = profiles.tree(core)
            home = root / 'rollback-home'
            home.mkdir(mode=0o700)
            restored = home / '.reasonix'
            restored.mkdir(mode=0o700)
            target = restored / 'config.toml'
            # Create private from the start; do not temporarily expose copied bytes.
            with target.open('xb') as stream:
                os.fchmod(stream.fileno(), 0o600)
                stream.write(backup.read_bytes())
            if target.read_bytes() != configuration or tomllib.loads(target.read_text())['default_model'] != 'native-import/alpha':
                raise RuntimeError('restored backup content/model differs')
            restored_before = profiles.tree(restored)
            env = {key: os.environ[key] for key in ('PATH', 'USER', 'LOGNAME', 'LANG') if key in os.environ}
            env.update(HOME=str(home), REASONIX_HOME=str(restored), REASONIX_STATE_HOME=str(restored),
                       REASONIX_CACHE_HOME=str(root / 'rollback-cache'), HTTP_PROXY='http://127.0.0.1:9',
                       HTTPS_PROXY='http://127.0.0.1:9', ALL_PROXY='http://127.0.0.1:9')
            result = subprocess.run([str(cli), 'config', 'currency'], env=env, cwd=root,
                                    capture_output=True, text=True, timeout=15)
            receipt = {
                'backupSha256': digest(backup), 'restoredSha256': digest(target),
                'restoredDefaultModel': tomllib.loads(target.read_text())['default_model'],
                'legacyExitCode': result.returncode, 'legacyStdout': result.stdout,
                'legacyStderr': result.stderr, 'previewStoppedBeforeRestore': True,
                'restoredTreeUnchanged': profiles.tree(restored) == restored_before,
                'previewAndBackupTreeUnchanged': profiles.tree(core) == original,
            }
            (output / 'rollback-receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
            if result.returncode != 0 or 'currency = "CNY"' not in result.stdout or not receipt['restoredTreeUnchanged'] or not receipt['previewAndBackupTreeUnchanged']:
                raise RuntimeError('official CLI recovery or original preservation failed')
            receipts.append(receipt)
        identity = original_launch(app_path, identifier, root, phase, configuration, window_state)
        if window_state is not None:
            shutil.copyfile(root / "tmp" / ("profile-placement-" + phase + ".json"),
                            output / ("profile-placement-" + phase + ".json"))
        return identity

    profiles.launch = launch
    try:
        profiles.smoke(str(app), str(cli), retain_failed=True)
        if len(receipts) != 1 or any(digest(path) != identity[path.name] for path in binaries):
            raise RuntimeError('backup receipt count or Preview binaries changed')
        if digest(cli) != OFFICIAL_EMBEDDED_CLI:
            raise RuntimeError('official CLI changed during acceptance')
        subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)
        (output / 'result.json').write_text(json.dumps({'status': 'passed', 'receipts': len(receipts)}) + '\n')
    except BaseException as error:
        (output / 'result.json').write_text(json.dumps({'status': 'failed', 'error': str(error)}) + '\n')
        raise
    finally:
        profiles.launch = original_launch
    print('Actual config backup applied, official CLI readback and original preservation: OK', flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app', type=Path)
    parser.add_argument('--legacy-cli', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--window-state-template', type=Path, help='place each private Preview host on the requested display and require native geometry before import actions')
    args = parser.parse_args()
    if sys.platform != 'darwin':
        parser.error('macOS required')
    state = None
    if args.window_state_template is not None:
        spec = importlib.util.spec_from_file_location('placement', Path(__file__).with_name('probe-launch-services-profile.py'))
        placement = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(placement)
        state = placement.read_window_state_template(args.window_state_template)
    smoke(args.app.absolute(), args.legacy_cli.absolute(), args.output.absolute(), state)
