#!/usr/bin/env python3
"""Real package refusal and backup recovery for unusable profile identities.

Six private managed/explicit fixtures: damaged metadata, conflicting valid
identities and a linked primary. Refusal must exit 1 before UI/sidecar. Repair
only this fixture's metadata backup, then require normal package startup/quit.
No user credentials, permission settings or ordinary applications are changed.
"""
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import shutil
import signal
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("package", Path(__file__).with_name("smoke-packaged-app.py"))
package = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package)
RECOVERY = b"credential profile identity is unavailable; restore its metadata backup or check profile permissions"


def protected(paths):
    return [(path.lstat().st_mode, path.lstat().st_mtime_ns, path.lstat().st_ino,
             str(path.readlink()) if path.is_symlink() else path.read_bytes()) for path in paths]


def smoke(app_path):
    app = Path(app_path).resolve()
    host, sidecar = [app / "Contents/MacOS" / name for name in ("reasonix-tauri", "reasonix-desktop-bridge")]
    identifier = plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleIdentifier"]
    if not host.is_file() or not sidecar.is_file() or package.matching_package_is_running(identifier):
        raise RuntimeError("packaged Preview missing or already running")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    for managed in (True, False):
        for case in ("damaged", "conflicting", "linked"):
            root = Path(tempfile.mkdtemp(prefix="reasonix-identity-refusal-", dir="/private/tmp"))
            root.chmod(0o700)
            home, temporary = root / "home", root / "tmp"
            home.mkdir(mode=0o700)
            temporary.mkdir(mode=0o700)
            core = home / "Library/Application Support" / identifier / "reasonix-core" if managed else root / "core"
            core.mkdir(parents=True, mode=0o700)
            primary, backup = [core / name for name in ("tauri-credential-profile.json", "tauri-credential-profile.backup.json")]
            canary = core / "original.canary"
            canary.write_bytes(b"private profile original\n")
            canary.chmod(0o600)
            identity = json.dumps({"version": 1, "id": "1" * 32}).encode()
            primary.write_bytes(identity if case == "conflicting" else b"damaged primary\n")
            backup.write_bytes(json.dumps({"version": 1, "id": "2" * 32}).encode() if case == "conflicting" else b"damaged backup\n")
            primary.chmod(0o600)
            backup.chmod(0o600)
            external = root / "external-original.json"
            external.write_bytes(identity)
            external.chmod(0o600)
            if case == "linked":
                primary.unlink()
                primary.symlink_to(external)
            originals = [primary, backup, canary, external]
            before = protected(originals)
            env = {key: value for key, value in os.environ.items()
                   if key in ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL", "__CF_USER_TEXT_ENCODING")}
            env.update(HOME=str(home), TMPDIR=str(temporary), REASONIX_TAURI_PACKAGE_SMOKE="1")
            if not managed:
                env.update(REASONIX_HOME=str(core), REASONIX_CACHE_HOME=str(root / "cache"))
            process = None
            success = False
            try:
                process = subprocess.Popen([str(host)], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
                _, error = process.communicate(timeout=20)
                expected = RECOVERY in error
                if process.returncode != 1 or not expected:
                    raise RuntimeError(f"identity refusal must exit normally with recovery instructions: exit={process.returncode}, expectedMessage={expected}")
                if protected(originals) != before:
                    raise RuntimeError("identity refusal changed original metadata or canaries")
                if package.own_sidecars(temporary, sidecar) or list(temporary.glob("reasonix-tauri-bridge-*")):
                    raise RuntimeError("identity refusal left a sidecar or readiness directory")
                # Perform the stated recovery only in this disposable profile;
                # preserve the damaged/linked primary and its original target.
                backup.write_bytes(identity)
                recovered = protected(originals)
                process = subprocess.Popen([str(host)], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
                _, error = process.communicate(timeout=40)
                if process.returncode != 0:
                    raise RuntimeError(f"metadata backup recovery did not start/quit normally: exit={process.returncode}")
                deadline = time.monotonic() + 5
                while package.own_sidecars(temporary, sidecar) and time.monotonic() < deadline:
                    time.sleep(0.05)
                if package.own_sidecars(temporary, sidecar) or list(temporary.glob("reasonix-tauri-bridge-*")):
                    raise RuntimeError("backup recovery left a sidecar or readiness directory")
                if protected(originals) != recovered:
                    raise RuntimeError("backup recovery overwrote the preserved primary or originals")
                if not (temporary / "reasonix-native-notification-smoke.json").is_file() or not (temporary / "reasonix-global-workspace-smoke.json").is_file():
                    raise RuntimeError("backup recovery omitted normal package smoke receipts")
                success = True
                print(f"identity {'managed' if managed else 'explicit'} {case}: exit 1, originals, backup recovery and normal quit OK", flush=True)
            finally:
                if process is not None and process.poll() is None:
                    process.kill()
                    process.wait(timeout=5)
                for pid in package.own_sidecars(temporary, sidecar):
                    try:
                        os.kill(pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                if success:
                    shutil.rmtree(root)
                else:
                    print(f"Private identity failure fixture retained: {root}", file=sys.stderr)


if __name__ == "__main__":
    if sys.platform != "darwin" or len(sys.argv) != 2:
        raise SystemExit("usage: smoke-profile-identity-refusal.py MACOS_PREVIEW_APP")
    smoke(sys.argv[1])
