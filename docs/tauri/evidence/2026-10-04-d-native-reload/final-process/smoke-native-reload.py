#!/usr/bin/env python3
"""Installed Reload menu, live renderer/storage and hidden-window preservation.

Private profiles only. Does not prove physical menu input or recovery of a
renderer that never finished loading or stopped executing JavaScript.
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
w = importlib.util.module_from_spec(spec)
spec.loader.exec_module(w)


def smoke(app_path):
    app = Path(app_path).resolve()
    identifier = plistlib.loads((app / 'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
    if w.package.matching_package_is_running(identifier):
        raise RuntimeError('Preview already running; do not operate it')
    for managed in (True, False):
        root = Path(tempfile.mkdtemp(prefix='reasonix-native-reload-', dir='/private/tmp'))
        root.chmod(0o700)
        for name in ('home', 'tmp'):
            (root / name).mkdir(mode=0o700)
        success = False
        try:
            w.launch(app / 'Contents/MacOS/reasonix-tauri', app / 'Contents/MacOS/reasonix-desktop-bridge',
                     root, identifier, managed, 'ui-native-reload', verify_window_state=False, launch_services=True)
            receipt = json.loads((root / 'tmp/reasonix-native-reload-result.json').read_text())
            cycles = receipt.get('cycles', [])
            if receipt.get('ok') is not True or len(cycles) != 2 or [cycle.get('hidden') for cycle in cycles] != [False, True]:
                raise RuntimeError('native Reload cycle receipts incomplete')
            for cycle in cycles:
                if any(cycle.get(key) is not True for key in ('newRealm', 'renderedMain', 'storagePreserved', 'trustedOrigin', 'nativeWindowPreserved', 'sidecarPreserved')):
                    raise RuntimeError('native Reload receipt failed')
            print(f"Native {'managed' if managed else 'explicit'} Reload receipt: " + json.dumps(receipt), flush=True)
            for line in (root / 'tmp/reasonix-native-window-trace.jsonl').read_text().splitlines():
                print('Native Reload trace: ' + line, flush=True)
            success = True
        finally:
            if success:
                shutil.rmtree(root)
            else:
                print('Failed private Reload fixture retained: ' + str(root), file=sys.stderr, flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app')
    args = parser.parse_args()
    if sys.platform != 'darwin':
        raise SystemExit('macOS required')
    smoke(args.app)
