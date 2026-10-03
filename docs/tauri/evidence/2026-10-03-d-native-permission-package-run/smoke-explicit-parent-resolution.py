#!/usr/bin/env python3
"""Verify Rust metadata and actual Go workspace agree for alias/../home."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('boundary', Path(__file__).with_name('smoke-managed-profile-boundary.py'))
boundary = importlib.util.module_from_spec(spec)
spec.loader.exec_module(boundary)
package = boundary.package


def smoke(app):
    app = Path(app).resolve()
    host = app / 'Contents/MacOS/reasonix-tauri'
    sidecar = app / 'Contents/MacOS/reasonix-desktop-bridge'
    if package.matching_package_is_running('io.reasonix.desktop.preview'):
        raise RuntimeError('Preview already running')
    subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)
    root = Path(tempfile.mkdtemp(prefix='reasonix-explicit-parent-', dir='/private/tmp'))
    root.chmod(0o700)
    home, temporary = root / 'home', root / 'tmp'
    home.mkdir(mode=0o700)
    temporary.mkdir(mode=0o700)
    stable = home / '.reasonix'
    (stable / 'nested').mkdir(parents=True, mode=0o700)
    original = stable / 'config.toml'
    original.write_bytes(b'# private original\n')
    original.chmod(0o600)
    alias = root / 'alias'
    alias.symlink_to(stable / 'nested', target_is_directory=True)
    before = boundary.snapshot(stable)
    supplied, selected = alias / '../preview', root / 'preview'
    env = {k: v for k, v in os.environ.items() if k in ('PATH', 'USER', 'LOGNAME', 'LANG', 'LC_ALL', '__CF_USER_TEXT_ENCODING')}
    env.update(HOME=str(home), TMPDIR=str(temporary), REASONIX_HOME=str(supplied), REASONIX_CACHE_HOME=str(root / 'cache'), REASONIX_TAURI_PACKAGE_SMOKE='1')
    process = None
    success = False
    try:
        process = subprocess.Popen([str(host)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, start_new_session=True)
        _, error = process.communicate(timeout=30)
        if process.returncode != 0:
            raise RuntimeError(f'parent-resolution host exit={process.returncode}, originalUnchanged={boundary.snapshot(stable) == before}')
        identity = package.check_credential_profile(selected)
        workspace = json.loads((temporary / 'reasonix-global-workspace-smoke.json').read_text())
        if workspace.get('workspaceRoot') != str((selected / 'global-workspace').resolve()):
            raise RuntimeError('Go workspace and Rust selected profile differ')
        if boundary.snapshot(stable) != before or (stable / 'preview').exists():
            raise RuntimeError('Rust metadata escaped into stable tree')
        if package.own_sidecars(temporary, sidecar) or list(temporary.glob('reasonix-tauri-bridge-*')):
            raise RuntimeError('parent-resolution quit left sidecar/readiness')
        print(json.dumps({'exitCode': process.returncode, 'credentialIdentity': identity, 'rustAndGoProfileAgree': True, 'originalUnchanged': True, 'sidecarRemaining': False, 'readyRemaining': False}), flush=True)
        success = True
    finally:
        if process is not None and process.poll() is None:
            process.kill()
            process.wait(timeout=5)
        for pid in package.own_sidecars(temporary, sidecar):
            import signal
            try:
                os.kill(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        if success:
            shutil.rmtree(root)
        else:
            print(f'Private parent-resolution failure fixture retained: {root}', file=sys.stderr)


if __name__ == '__main__':
    if sys.platform != 'darwin' or len(sys.argv) != 2:
        raise SystemExit('usage: smoke-explicit-parent-resolution.py MACOS_PREVIEW_APP')
    smoke(sys.argv[1])
