#!/usr/bin/env python3
"""Exercise the packaged takeover host against an already prepared private HOME.
Never accepts the actual login HOME: this probe opens every imported session.
Only counts and sanitized errors are emitted; no provider requests are submitted.
"""
import argparse
import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import time


def wait_file(path, process, timeout=120):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if path.is_file():
            return
        if process.poll() is not None:
            raise RuntimeError(f"packaged host exited before marker ({process.returncode})")
        time.sleep(0.1)
    raise RuntimeError("packaged takeover acceptance timed out")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--app', type=Path, required=True)
    parser.add_argument('--private-home', type=Path, required=True)
    args = parser.parse_args()
    home = args.private_home.resolve(strict=True)
    if home == Path.home().resolve() or not str(home).startswith('/private/tmp/'):
        raise RuntimeError('acceptance requires a private temporary HOME')
    receipt = json.loads((home / '.reasonix/tauri-takeover-v1.json').read_text())
    if Path(receipt['profileRoot']).resolve() != home / '.reasonix':
        raise RuntimeError('private receipt points outside its HOME')
    binary = args.app.resolve(strict=True) / 'Contents/MacOS/reasonix-tauri'
    with tempfile.TemporaryDirectory(prefix='reasonix-takeover-native-') as tmp:
        directory = Path(tmp)
        env = os.environ.copy()
        for name in ('REASONIX_HOME', 'REASONIX_STATE_HOME', 'REASONIX_CACHE_HOME',
                     'REASONIX_TAURI_PACKAGE_SMOKE', 'REASONIX_DESKTOP_BRIDGE_TOKEN'):
            env.pop(name, None)
        env.update(HOME=str(home), TMPDIR=str(directory),
                   REASONIX_TAURI_PROFILE_SMOKE='takeover')
        process = subprocess.Popen([str(binary)], env=env, stdout=subprocess.DEVNULL,
                                   stderr=subprocess.DEVNULL, start_new_session=True)
        try:
            wait_file(directory / 'reasonix-native-profile-waiting.json', process)
            (directory / 'reasonix-native-profile-begin.json').write_text('{}')
            wait_file(directory / 'reasonix-native-profile-result.json', process)
            result = json.loads((directory / 'reasonix-native-profile-result.json').read_text())
            if not result.get('ok'):
                raise RuntimeError(result.get('error', 'takeover acceptance failed'))
            (directory / 'reasonix-native-profile-exit.json').write_text('{}')
            if process.wait(timeout=20) != 0:
                raise RuntimeError('packaged host did not exit cleanly')
            print(json.dumps(result))
        finally:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGTERM)
                process.wait(timeout=10)


if __name__ == '__main__':
    main()
