#!/usr/bin/env python3
"""Actual macOS package import/restart with private stable and Preview profiles.

Host entry points and Go settings are real; no WebView clicks are asserted.
Optional --legacy-cli performs only the old CLI's read-only currency query.
This is not acceptance of the historical Wails GUI or its directory locks.
"""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import signal
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("package_smoke", Path(__file__).with_name("smoke-packaged-app.py"))
package = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package)

CONFIG = ('default_model = "native-import/alpha"\n\n[desktop]\ncurrency = "CNY"\n'
          'provider_access = ["native-import"]\n\n[[providers]]\nname = "native-import"\n'
          'kind = "openai"\nbase_url = "http://127.0.0.1:9/v1"\n'
          'models = ["alpha", "beta"]\ndefault = "alpha"\n').encode()


def tree(directory):
    return {str(path.relative_to(directory)): (hashlib.sha256(path.read_bytes()).hexdigest(),
            path.stat().st_mode & 0o777, path.stat().st_mtime_ns)
            for path in directory.rglob("*") if path.is_file()}


def wait_until(predicate, host):
    deadline = time.monotonic() + 25
    while time.monotonic() < deadline:
        value = predicate()
        if value:
            return value
        if host.poll() is not None:
            raise RuntimeError("profile acceptance host exited before its checkpoint")
        time.sleep(0.05)
    raise RuntimeError("profile acceptance checkpoint timeout")


def inspect(host, sidecar, temporary, core, managed, cache):
    ready = list(temporary.glob("reasonix-tauri-bridge-*/ready.json"))
    children = [pid for pid, parent, command in package.processes()
                if parent == host.pid and command.startswith(str(sidecar))]
    if len(ready) != 1 or len(children) != 1:
        return None
    address = package.check_ready(ready[0])
    package.check_unauthenticated_health(address)
    package.check_sidecar_profile(children[0], core, managed, cache)
    return children[0], json.loads(ready[0].read_text())["sidecarInstanceId"], package.check_credential_profile(core)


def launch(app, identifier, root, phase, configuration):
    home, temporary = root / "home", root / "tmp"
    managed = phase != "explicit"
    core = home / "Library/Application Support" / identifier / "reasonix-core" if managed else root / "explicit-core"
    if not managed:
        core.mkdir(mode=0o700)
        (core / "config.toml").write_bytes(configuration)
        (core / "config.toml").chmod(0o600)
    for path in temporary.glob("reasonix-native-profile-*.json"):
        path.unlink()
    env = os.environ.copy()
    for name in ("REASONIX_HOME", "REASONIX_STATE_HOME", "REASONIX_CACHE_HOME", "REASONIX_PREVIEW_SQLITE_EVENTS",
                 "REASONIX_TAURI_PACKAGE_SMOKE", "REASONIX_TAURI_NATIVE_WINDOW_SMOKE"):
        env.pop(name, None)
    env.update(HOME=str(home), TMPDIR=str(temporary), REASONIX_TAURI_PROFILE_SMOKE=phase,
               REASONIX_DESKTOP_BRIDGE_TOKEN="must-be-cleared")
    if not managed:
        env.update(REASONIX_HOME=str(core), REASONIX_CACHE_HOME=str(root / "cache"))
    host_binary, sidecar = app / "Contents/MacOS/reasonix-tauri", app / "Contents/MacOS/reasonix-desktop-bridge"
    host = subprocess.Popen([str(host_binary)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                            start_new_session=True)
    try:
        initial = wait_until(lambda: inspect(host, sidecar, temporary, core, managed, root / "cache"), host)
        wait_until(lambda: (temporary / "reasonix-native-profile-waiting.json").is_file(), host)
        (temporary / "reasonix-native-profile-begin.json").write_text("{}")
        result_path = temporary / "reasonix-native-profile-result.json"
        wait_until(result_path.is_file, host)
        result = json.loads(result_path.read_text())
        if result.get("phase") != phase or result.get("ok") is not True:
            raise RuntimeError(f"{phase}: {result.get('error', 'profile acceptance failed')}")
        final = wait_until(lambda: inspect(host, sidecar, temporary, core, managed, root / "cache"), host)
        if initial[1] != result.get("initialInstance") or final[1] != result.get("finalInstance") or initial[2] != final[2]:
            raise RuntimeError("profile restart/credential identity differs from actual sidecar")
        if phase == "import":
            if initial[0] == final[0] or package.is_alive(initial[0]) or initial[1] == final[1]:
                raise RuntimeError("profile restart did not clean up and replace the original sidecar")
            backup = Path(result["backup"])
            if not backup.is_relative_to(core / "backups") or backup.is_symlink() or backup.read_bytes() != configuration:
                raise RuntimeError("profile import backup escaped or changed")
            backups = list((core / "backups").glob("*/config.toml"))
            if backups != [backup]:
                raise RuntimeError("rejected re-import created another backup")
        if managed:
            backups = list((core / "backups").glob("*/config.toml"))
            if len(backups) != 1 or backups[0].read_bytes() != configuration:
                raise RuntimeError("Preview edits/restart changed or duplicated the stable config backup")
            for directory in (core / "backups", backups[0].parent):
                if directory.is_symlink() or not directory.is_dir() or directory.stat().st_mode & 0o777 != 0o700:
                    raise RuntimeError("config backup directory is not an ordinary private directory")
            expected = {"projects": [{"root": str(root / "project 中文 & spaces"), "title": "Private project"}]}
            if json.loads((core / "desktop-projects.json").read_text()) != expected:
                raise RuntimeError("folder import copied legacy session/order metadata")
            for name in ("sessions/legacy.txt", "cache/legacy.txt", "plugins/legacy.txt", ".env"):
                if (core / name).exists():
                    raise RuntimeError("config import copied excluded stable state")
            for path in [core / "config.toml", core / "desktop-projects.json", *backups]:
                if path.is_symlink() or path.stat().st_mode & 0o777 != 0o600:
                    raise RuntimeError("imported configuration/project backup is not private")
        elif (core / "config.toml").read_bytes() != configuration or (core / "desktop-projects.json").exists() or (core / "backups").exists():
            raise RuntimeError("explicit profile import changed operator-owned files")
        (temporary / "reasonix-native-profile-exit.json").write_text("{}")
        if host.wait(timeout=10) != 0:
            raise RuntimeError("profile acceptance host failed to exit normally")
        deadline = time.monotonic() + 5
        while package.is_alive(final[0]) and time.monotonic() < deadline:
            time.sleep(0.05)
        if package.is_alive(final[0]) or package.own_sidecars(temporary, sidecar) or list(temporary.glob("reasonix-tauri-bridge-*/ready.json")):
            raise RuntimeError("profile acceptance left sidecar/readiness after exit")
        print(f"packaged macOS profile {phase}, real bridge settings, private files, and cleanup: OK")
        return final[2]
    finally:
        if host.poll() is None:
            host.kill()
            host.wait(timeout=5)
        for pid in package.own_sidecars(temporary, sidecar):
            try:
                os.kill(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass


def smoke(app_path, legacy_cli=None):
    app = Path(app_path).resolve()
    with (app / "Contents/Info.plist").open("rb") as file:
        identifier = plistlib.load(file)["CFBundleIdentifier"]
    if package.matching_package_is_running(identifier):
        raise RuntimeError("a Preview with this identifier is running; close it before acceptance")
    with tempfile.TemporaryDirectory(prefix="reasonix-profile-smoke-") as directory:
        root = Path(directory)
        for name in ("home", "tmp", "project 中文 & spaces"):
            (root / name).mkdir()
        stable = root / "home/.reasonix"
        stable.mkdir(mode=0o700)
        (stable / "config.toml").write_bytes(CONFIG)
        projects = {"projects": [{"root": str(root / "project 中文 & spaces"), "title": "Private project", "topics": ["legacy"], "order": 99}], "sessions": ["legacy"]}
        (stable / "desktop-projects.json").write_text(json.dumps(projects))
        for name in ("sessions/legacy.txt", "cache/legacy.txt", "plugins/legacy.txt", ".env"):
            path = stable / name
            path.parent.mkdir(exist_ok=True)
            path.write_text("private legacy sentinel")
        for path in stable.rglob("*"):
            if path.is_file():
                path.chmod(0o600)
        legacy_env = os.environ.copy()
        legacy_env.update(HOME=str(root / "home"), REASONIX_HOME=str(stable), REASONIX_STATE_HOME=str(stable), REASONIX_CACHE_HOME=str(root / "legacy-cache"))
        legacy_command = [str(Path(legacy_cli).resolve()), "config", "currency"] if legacy_cli else None
        if legacy_command:
            # The old CLI's theme initialization calls config.Load(), which
            # upgrades sparse old configs. Construct the native old-version
            # fixture before capturing originals; do not excuse later writes.
            prepared = subprocess.run(legacy_command, cwd=root, env=legacy_env, capture_output=True, text=True, timeout=15)
            if prepared.returncode != 0 or 'currency = "CNY"' not in prepared.stdout:
                raise RuntimeError("legacy CLI could not construct its native configuration fixture")
        configuration = (stable / "config.toml").read_bytes()
        original = tree(stable)
        imported = launch(app, identifier, root, "import", configuration)
        if launch(app, identifier, root, "restore", configuration) != imported:
            raise RuntimeError("Preview restart changed credential profile identity")
        launch(app, identifier, root, "explicit", configuration)
        if tree(stable) != original:
            raise RuntimeError("Preview import/update/restart changed stable originals")
        if legacy_cli:
            result = subprocess.run(legacy_command, cwd=root, env=legacy_env, capture_output=True, text=True, timeout=15)
            if result.returncode != 0 or 'currency = "CNY"' not in result.stdout or tree(stable) != original:
                raise RuntimeError("legacy CLI did not read unchanged stable configuration after Preview exit")
            print("legacy CLI read-only stable config compatibility after Preview exit: OK")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    parser.add_argument("--legacy-cli")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("this acceptance requires macOS")
    try:
        smoke(args.app, args.legacy_cli)
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"packaged profile smoke failed: {error}") from error
