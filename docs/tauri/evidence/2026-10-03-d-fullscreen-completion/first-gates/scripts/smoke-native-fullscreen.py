#!/usr/bin/env python3
"""Installed AppKit fullscreen role, completion notifications and exact restore.

Programmatic menu actions only; no physical keys or clipboard access.
"""
import argparse
import importlib.util
import json
from pathlib import Path
import plistlib
import shutil
import sys
import tempfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('windows', Path(__file__).with_name('smoke-native-window.py'))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


def smoke(app_path):
    app = Path(app_path).resolve()
    identifier = plistlib.loads((app / 'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError('Preview already running')
    for managed in (True, False):
        root = Path(tempfile.mkdtemp(prefix='reasonix-native-fullscreen-', dir='/private/tmp'))
        root.chmod(0o700)
        for name in ('home', 'tmp'):
            (root / name).mkdir(mode=0o700)
        passed = False
        identities = []
        try:
            for phase in ('exercise', 'restore-maximized', 'restore-normal', 'menu-fullscreen', 'restore-normal'):
                identities.append(windows.launch(app / 'Contents/MacOS/reasonix-tauri',
                    app / 'Contents/MacOS/reasonix-desktop-bridge', root, identifier, managed, phase,
                    launch_services=True))
                if phase == 'menu-fullscreen':
                    for line in (root / 'tmp/reasonix-native-window-trace.jsonl').read_text().splitlines():
                        receipt = json.loads(line)
                        if receipt['stage'].startswith('fullscreen-'):
                            print('native fullscreen receipt: ' + json.dumps(receipt), flush=True)
            if len(set(identities)) != 1:
                raise RuntimeError('fullscreen restarts changed credential identity')
            passed = True
            print(f"Native {'managed' if managed else 'explicit'} fullscreen entry/exit/completions/exact persistence/restart: OK", flush=True)
        finally:
            if passed:
                shutil.rmtree(root)
            else:
                print(f'Failed private fullscreen fixture retained: {root}', file=sys.stderr, flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app')
    args = parser.parse_args()
    if sys.platform != 'darwin':
        raise SystemExit('macOS required')
    smoke(args.app)
