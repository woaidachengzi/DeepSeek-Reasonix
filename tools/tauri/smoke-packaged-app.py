#!/usr/bin/env python3
"""Exercise a packaged macOS Preview without touching the user's profile.

Usage: python3 tools/tauri/smoke-packaged-app.py PATH_TO_APP
The host's package-smoke flag exits through Tauri's normal RunEvent::Exit path.
"""

import ipaddress
import json
import os
from pathlib import Path
import plistlib
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


def check_ready(path):
    payload = json.loads(path.read_text())
    if payload.get("protocolVersion") != 1 or not payload.get("sidecarInstanceId"):
        raise RuntimeError("sidecar published invalid readiness")
    address = payload.get("address", "")
    host, separator, port = address.rpartition(":")
    if not separator or not port.isdigit() or not ipaddress.ip_address(host).is_loopback:
        raise RuntimeError("sidecar readiness did not use a loopback address")


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
    if any(command.startswith(str(host_binary)) for _, _, command in processes()):
        raise RuntimeError("this Preview package is already running; close it before the smoke")

    with tempfile.TemporaryDirectory(prefix="reasonix-tauri-package-smoke-") as root_string:
        root = Path(root_string)
        home = root / "home"
        temp = root / "tmp"
        home.mkdir()
        temp.mkdir()
        env = os.environ.copy()
        env.update({
            "HOME": str(home),
            "TMPDIR": str(temp),
            "REASONIX_HOME": str(root / "reasonix-home"),
            "REASONIX_CACHE_HOME": str(root / "reasonix-cache"),
            "REASONIX_TAURI_PACKAGE_SMOKE": "1",
            # Release packages must ignore the development sidecar override.
            "REASONIX_DESKTOP_BRIDGE_BIN": str(root / "nonexistent-sidecar"),
        })
        for key in (
            "REASONIX_STATE_HOME",
            "REASONIX_PREVIEW_SQLITE_EVENTS",
        ):
            env.pop(key, None)

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
                    check_ready(ready_files[0])
                    app_data = home / "Library/Application Support" / identifier
                    if not app_data.is_dir():
                        raise RuntimeError("Tauri app data escaped the temporary HOME")
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
            print("packaged Preview startup, sidecar readiness, and shutdown: OK")
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


if __name__ == "__main__":
    if sys.platform != "darwin" or len(sys.argv) != 2:
        raise SystemExit("usage on macOS: smoke-packaged-app.py PATH_TO_APP")
    try:
        smoke(sys.argv[1])
    except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"packaged Preview smoke failed: {error}") from error
