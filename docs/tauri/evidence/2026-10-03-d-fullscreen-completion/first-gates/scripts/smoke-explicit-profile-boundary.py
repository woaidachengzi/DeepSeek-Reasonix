#!/usr/bin/env python3
"""Actual package refusals for known stable storage roots, using private HOME.

This protects default Wails roots and aliases; it does not certify arbitrary
custom historical roots or prevent a legacy process from later selecting Preview.
"""
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
    cases = ['home', 'alias', 'missing-descendant', 'ancestor', 'state', 'cache', 'legacy-support', 'legacy-cache', 'tilde', 'variable']
    for case in cases:
        root = Path(tempfile.mkdtemp(prefix='reasonix-explicit-boundary-', dir='/private/tmp'))
        root.chmod(0o700)
        home, temporary = root / 'home', root / 'tmp'
        home.mkdir(mode=0o700)
        temporary.mkdir(mode=0o700)
        stable = home / '.reasonix'
        support = home / 'Library/Application Support/reasonix'
        cache = home / 'Library/Caches/reasonix'
        for directory in (stable, support, cache):
            directory.mkdir(parents=True, mode=0o700)
            original = directory / 'config.toml'
            original.write_bytes(b'# private legacy original\n')
            original.chmod(0o600)
        originals = {str(path): boundary.snapshot(path) for path in (stable, support, cache)}
        alias = root / 'alias'
        alias.symlink_to(stable, target_is_directory=True)
        alias_before = alias.lstat()
        preview = root / 'separate-preview'
        env = {k: v for k, v in os.environ.items() if k in ('PATH', 'USER', 'LOGNAME', 'LANG', 'LC_ALL', '__CF_USER_TEXT_ENCODING')}
        env.update(HOME=str(home), TMPDIR=str(temporary), REASONIX_HOME=str(preview))
        if case in ('home', 'alias', 'missing-descendant', 'ancestor', 'legacy-support', 'legacy-cache'):
            env['REASONIX_HOME'] = str({'home': stable, 'alias': alias, 'missing-descendant': alias / 'missing/state', 'ancestor': home, 'legacy-support': support, 'legacy-cache': cache}[case])
        elif case in ('state', 'cache'):
            env['REASONIX_STATE_HOME' if case == 'state' else 'REASONIX_CACHE_HOME'] = str(stable / 'missing')
        else:
            env['REASONIX_HOME'] = '~/.reasonix' if case == 'tilde' else '${HOME}/.reasonix'
        process = None
        success = False
        try:
            process = subprocess.Popen([str(host)], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
            _, error = process.communicate(timeout=20)
            expected = b'resolved paths' if case in ('tilde', 'variable') else b'Preview storage overlaps the stable Reasonix data directory'
            if process.returncode != 1 or expected not in error:
                raise RuntimeError(f'{case}: expected refusal missing, exit={process.returncode}')
            if package.own_sidecars(temporary, sidecar) or list(temporary.glob('reasonix-tauri-bridge-*')):
                raise RuntimeError('refusal started sidecar or readiness')
            if any(boundary.snapshot(Path(p)) != value for p, value in originals.items()) or preview.exists():
                raise RuntimeError('refusal changed original or created Preview root')
            after = alias.lstat()
            if (after.st_mode, after.st_ino, after.st_mtime_ns) != (alias_before.st_mode, alias_before.st_ino, alias_before.st_mtime_ns) or alias.readlink() != stable:
                raise RuntimeError('refusal changed alias')
            print(json.dumps({'case': case, 'exitCode': process.returncode, 'expectedMessage': True, 'originalsUnchanged': True, 'sidecarRemaining': False, 'readyRemaining': False}), flush=True)
            success = True
        finally:
            if process is not None and process.poll() is None:
                process.kill()
                process.wait(timeout=5)
            for pid in package.own_sidecars(temporary, sidecar):
                import signal
                os.kill(pid, signal.SIGKILL)
            if success:
                shutil.rmtree(root)
            else:
                print(f'Private failure fixture retained: {root}', file=sys.stderr)


if __name__ == '__main__':
    if sys.platform != 'darwin' or len(sys.argv) != 2:
        raise SystemExit('usage: smoke-explicit-profile-boundary.py MACOS_PREVIEW_APP')
    smoke(sys.argv[1])
