#!/usr/bin/env python3
"""Exercise a packaged macOS Preview without touching the user's profile.

Usage: python3 tools/tauri/smoke-packaged-app.py PATH_TO_APP
The host's package-smoke flag exits through Tauri's normal RunEvent::Exit path.
"""

import http.client
import ipaddress
import json
import os
from pathlib import Path
import plistlib
import re
import signal
import subprocess
import sys
import tempfile
import time


def processes():
    result = subprocess.run(
        ["ps", "-wwaxo", "pid=,ppid=,command="],
        capture_output=True,
        text=True,
        check=True,
    )
    for line in result.stdout.splitlines():
        fields = line.strip().split(None, 2)
        if len(fields) == 3 and fields[0].isdigit() and fields[1].isdigit():
            yield int(fields[0]), int(fields[1]), fields[2]


def own_sidecars(root, binary):
    return [
        pid
        for pid, _, command in processes()
        if command.startswith(str(binary)) and f"--ready-file {root}/" in command
    ]


def is_alive(pid):
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False


def matching_package_is_running(identifier):
    # The macOS single-instance channel is shared by copies of the same app,
    # including release-candidate worktrees. Do not launch a smoke that would
    # focus an unrelated running copy instead of starting an isolated host.
    marker = ".app/Contents/MacOS/reasonix-tauri"
    for _, _, command in processes():
        prefix, separator, suffix = command.partition(marker)
        if not separator or (suffix and not suffix.startswith(" ")):
            continue
        plist = Path(prefix + ".app") / "Contents/Info.plist"
        try:
            with plist.open("rb") as file:
                if plistlib.load(file).get("CFBundleIdentifier") == identifier:
                    return True
        except (OSError, ValueError, plistlib.InvalidFileException):
            continue
    return False


def check_ready(path):
    payload = json.loads(path.read_text())
    if payload.get("protocolVersion") != 1 or not payload.get("sidecarInstanceId"):
        raise RuntimeError("sidecar published invalid readiness")
    address = payload.get("address", "")
    host, separator, port = address.rpartition(":")
    host = host.removeprefix("[").removesuffix("]")
    if not separator or not port.isdigit() or not 0 < int(port) <= 65535 or not ipaddress.ip_address(host).is_loopback:
        raise RuntimeError("sidecar readiness did not use a loopback address")
    return host, int(port)


def check_unauthenticated_health(address):
    connection = http.client.HTTPConnection(*address, timeout=2)
    try:
        connection.request("GET", "/v1/health")
        response = connection.getresponse()
        if response.status != 401:
            raise RuntimeError("packaged sidecar accepted an unauthenticated health request")
    finally:
        connection.close()


def check_sidecar_profile(pid, expected_home, managed, cache_home):
    # Inspect the child's inherited environment, not the parent's intended
    # values. Never include ps output in an error: it contains the bridge token.
    result = subprocess.run(
        ["ps", "eww", "-p", str(pid), "-o", "command="],
        capture_output=True, text=True, check=True,
    )

    def has(name):
        return re.search(rf"(?:^|\s){re.escape(name)}=", result.stdout) is not None

    def equals(name, value):
        return re.search(
            rf"(?:^|\s){re.escape(name)}={re.escape(str(value))}(?:\s|$)",
            result.stdout,
        ) is not None

    if not equals("REASONIX_HOME", expected_home):
        raise RuntimeError("sidecar did not inherit the selected Preview home")
    if has("REASONIX_STATE_HOME"):
        raise RuntimeError("sidecar retained an inherited state override")
    if not equals("REASONIX_DESKTOP_BRIDGE_TOKEN", ""):
        raise RuntimeError("sidecar launch environment exposed a bridge token")
    if managed:
        if has("REASONIX_CACHE_HOME"):
            raise RuntimeError("managed sidecar retained an inherited cache override")
        if not equals("REASONIX_PREVIEW_SQLITE_EVENTS", "1"):
            raise RuntimeError("managed sidecar did not enable Preview event storage")
    elif not equals("REASONIX_CACHE_HOME", cache_home) or has("REASONIX_PREVIEW_SQLITE_EVENTS"):
        raise RuntimeError("explicit sidecar profile environment is incorrect")


def check_credential_profile(home):
    identities = []
    for name in ("tauri-credential-profile.json", "tauri-credential-profile.backup.json"):
        path = home / name
        metadata = path.lstat()
        if path.is_symlink() or not path.is_file() or metadata.st_size > 512:
            raise RuntimeError("credential profile metadata must be a bounded regular file")
        if metadata.st_mode & 0o777 != 0o600:
            raise RuntimeError("credential profile metadata is not private")
        identity = json.loads(path.read_text())
        if set(identity) != {"version", "id"} or identity["version"] != 1:
            raise RuntimeError("credential profile metadata contains unexpected attributes")
        profile_id = identity["id"]
        if not isinstance(profile_id, str) or len(profile_id) != 32 or any(c not in "0123456789abcdef" for c in profile_id):
            raise RuntimeError("credential profile metadata has an invalid identity")
        identities.append(profile_id)
    if identities[0] != identities[1]:
        raise RuntimeError("credential profile metadata backup does not match")
    return identities[0]


def smoke_once(host_binary, sidecar_binary, identifier, managed):
    with tempfile.TemporaryDirectory(prefix="reasonix-tauri-package-smoke-") as root_string:
        root = Path(root_string)
        home = root / "home"
        temp = root / "tmp"
        home.mkdir()
        temp.mkdir()
        app_data = home / "Library/Application Support" / identifier
        expected_home = app_data / "reasonix-core" if managed else root / "reasonix-home"
        cache_home = root / "reasonix-cache"
        env = os.environ.copy()
        env.pop("REASONIX_TAURI_PROFILE_SMOKE", None)
        env.pop("REASONIX_TAURI_NATIVE_WINDOW_SMOKE", None)
        env.update({
            "HOME": str(home),
            "TMPDIR": str(temp),
            "REASONIX_TAURI_PACKAGE_SMOKE": "1",
            "REASONIX_DESKTOP_BRIDGE_TOKEN": "inherited-token-must-be-cleared",
            # Release packages must ignore the development sidecar override.
            "REASONIX_DESKTOP_BRIDGE_BIN": str(root / "nonexistent-sidecar"),
        })
        env.pop("REASONIX_PREVIEW_SQLITE_EVENTS", None)
        if managed:
            env.pop("REASONIX_HOME", None)
            env["REASONIX_STATE_HOME"] = str(root / "inherited-state")
            env["REASONIX_CACHE_HOME"] = str(root / "inherited-cache")
        else:
            env["REASONIX_HOME"] = str(expected_home)
            env["REASONIX_CACHE_HOME"] = str(cache_home)
            env.pop("REASONIX_STATE_HOME", None)

        host = subprocess.Popen(
            [str(host_binary)], env=env, stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL, start_new_session=True,
        )
        sidecar_pid = None
        try:
            deadline = time.monotonic() + 25
            while time.monotonic() < deadline:
                if host.poll() is not None:
                    raise RuntimeError(f"packaged host exited before readiness ({host.returncode})")
                children = [
                    pid for pid, parent, command in processes()
                    if parent == host.pid and command.startswith(str(sidecar_binary))
                ]
                ready_files = list(temp.glob("reasonix-tauri-bridge-*/ready.json"))
                if len(children) == 1 and len(ready_files) == 1:
                    sidecar_pid = children[0]
                    address = check_ready(ready_files[0])
                    if not app_data.is_dir():
                        raise RuntimeError("Tauri app data escaped the temporary HOME")
                    if managed and not expected_home.is_dir():
                        raise RuntimeError("managed Preview home was not created inside app data")
                    credential_identity = check_credential_profile(expected_home)
                    check_sidecar_profile(sidecar_pid, expected_home, managed, cache_home)
                    check_unauthenticated_health(address)
                    break
                time.sleep(0.1)
            else:
                raise RuntimeError("packaged host did not start one ready sidecar")

            try:
                exit_code = host.wait(timeout=20)
            except subprocess.TimeoutExpired as error:
                raise RuntimeError("packaged host did not exit through the smoke path") from error
            if exit_code != 0:
                raise RuntimeError(f"packaged host exited with status {exit_code}")
            cleanup_deadline = time.monotonic() + 5
            while is_alive(sidecar_pid) and time.monotonic() < cleanup_deadline:
                time.sleep(0.1)
            if is_alive(sidecar_pid) or sidecar_pid in own_sidecars(temp, sidecar_binary):
                raise RuntimeError("packaged sidecar survived host exit")
            if list(temp.glob("reasonix-tauri-bridge-*/ready.json")):
                raise RuntimeError("sidecar readiness directory survived host exit")
            notification_status = json.loads((temp / "reasonix-native-notification-smoke.json").read_text())
            if notification_status.get("permission") not in {"not_determined", "denied", "granted", "provisional"} or notification_status.get("clickSupported") is not True:
                raise RuntimeError("packaged native notification permission query failed")
            workspace_target = json.loads((temp / "reasonix-global-workspace-smoke.json").read_text())
            global_workspace = expected_home / "global-workspace"
            if workspace_target.get("protocolVersion") != 1 or workspace_target.get("sessionId") != "tauri-package-workspace-smoke" or workspace_target.get("workspaceRoot") != str(global_workspace.resolve()):
                raise RuntimeError("packaged Global workspace escaped the current profile")
            if not global_workspace.is_dir() or global_workspace.is_symlink() or global_workspace.stat().st_mode & 0o777 != 0o700:
                raise RuntimeError("packaged Global workspace is not a private directory")
            profile = "managed" if managed else "explicit"
            print(f"packaged Preview {profile} profile, private credential identity, native notification authorization ({notification_status['permission']}), Global workspace, sidecar readiness, and shutdown: OK")
            return credential_identity
        finally:
            if host.poll() is None:
                host.kill()
                host.wait(timeout=5)
            remaining = set(own_sidecars(temp, sidecar_binary))
            if sidecar_pid is not None and is_alive(sidecar_pid):
                remaining.update(
                    pid for pid, _, command in processes()
                    if pid == sidecar_pid and command.startswith(str(sidecar_binary))
                )
            for pid in remaining:
                try:
                    os.kill(pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass


def smoke(app_path):
    app = Path(app_path).resolve()
    host_binary = app / "Contents/MacOS/reasonix-tauri"
    sidecar_binary = app / "Contents/MacOS/reasonix-desktop-bridge"
    if not host_binary.is_file() or not sidecar_binary.is_file():
        raise RuntimeError("packaged host or sidecar is missing")
    with (app / "Contents/Info.plist").open("rb") as file:
        identifier = plistlib.load(file).get("CFBundleIdentifier")
    if not isinstance(identifier, str) or not identifier:
        raise RuntimeError("package has no bundle identifier")
    if matching_package_is_running(identifier):
        raise RuntimeError("a Preview with this bundle identifier is already running; close it before the smoke")
    identities = [smoke_once(host_binary, sidecar_binary, identifier, managed) for managed in (True, False)]
    if identities[0] == identities[1]:
        raise RuntimeError("independently created profiles share a credential identity")


if __name__ == "__main__":
    if sys.platform != "darwin" or len(sys.argv) != 2:
        raise SystemExit("usage on macOS: smoke-packaged-app.py PATH_TO_APP")
    try:
        smoke(sys.argv[1])
    except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"packaged Preview smoke failed: {error}") from error
