#!/usr/bin/env python3
"""Read old UI allowlist via real main IPC twice without changing old storage.

Exercises reader destruction with closeBehavior=quit. No old values are logged
or imported. Private profile identities and ordinary cleanup are also checked.
"""
import argparse
import importlib.util
import os
from pathlib import Path
import sys
import subprocess
import tempfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("native_window", Path(__file__).with_name("smoke-native-window.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)
spec = importlib.util.spec_from_file_location("migration", Path(__file__).with_name("smoke-native-ui-migration.py"))
migration = importlib.util.module_from_spec(spec)
spec.loader.exec_module(migration)


def smoke(app_path, private_source=False):
    app = Path(app_path).resolve()
    host = app / "Contents/MacOS/reasonix-tauri"
    sidecar = app / "Contents/MacOS/reasonix-desktop-bridge"
    identifier = "io.reasonix.desktop.preview"
    if not host.is_file() or not sidecar.is_file() or runner.package.matching_package_is_running(identifier):
        raise RuntimeError("packaged Preview missing or already running")
    subprocess.run(['/usr/bin/codesign', '--verify', '--deep', '--strict', str(app)], check=True, capture_output=True)
    environment = {key: value for key, value in os.environ.items()
                   if key in ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL", "__CF_USER_TEXT_ENCODING")}
    for managed in (True, False):
        prefix = 'reasonix-ui-migration-read-' if private_source else 'reasonix-native-ui-legacy-read-'
        with tempfile.TemporaryDirectory(prefix=prefix, dir='/private/tmp') as directory:
            root = Path(directory).resolve()
            root.chmod(0o700)
            for name in ("home", "tmp"):
                (root / name).mkdir(mode=0o700)
            env = environment.copy()
            originals = {}
            if private_source:
                env['REASONIX_TAURI_LEGACY_UI_SMOKE'] = 'private-source'
                source = migration.prepare_source(root)
                core = root / 'home/Library/Application Support' / identifier / 'reasonix-core' if managed else root / 'core'
                core.mkdir(parents=True, mode=0o700)
                canary = core / 'ui-migration-original.canary'
                canary.touch(mode=0o600)
                canary.write_bytes(b'private original\n')
                originals = {path: migration.original(path) for path in (source, canary)}
            identities = []
            for _ in range(2 if private_source else 1):
                receipt = root / 'tmp/reasonix-native-legacy-ui-receipt.json'
                receipt.unlink(missing_ok=True)
                identities.append(runner.launch(host, sidecar, root, identifier, managed,
                                                "ui-legacy-read", verify_window_state=False, environment=env))
                if private_source:
                    migration.check_source_receipt(receipt, expected_reads=2)
                    if any(migration.original(path) != value for path, value in originals.items()):
                        raise RuntimeError('Private source read changed its source or protected original')
                elif receipt.exists():
                    raise RuntimeError('Production legacy read unexpectedly used a seeded fixture')
            if len(set(identities)) != 1:
                raise RuntimeError('Private source restart changed profile identity')
    if private_source:
        print('Private nonpersistent source: four packaged launches, eight exact three-value reads, stable identity, originals and cleanup OK; UI import/undo remain separate.')
    else:
        print("Actual main IPC legacy UI read, repeated inert reader cleanup and close-quit policy: OK")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app')
    parser.add_argument('--private-source', action='store_true', help='read only the controlled ephemeral three-preference source, with a restart per profile')
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("This acceptance requires macOS")
    smoke(args.app, private_source=args.private_source)
