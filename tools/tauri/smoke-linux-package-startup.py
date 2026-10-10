#!/usr/bin/env python3
"""Linux package startup/lifecycle; run under owned Xvfb and dbus-run-session.

No UI actions, account requests, clipboard access, or installed profile access.
--normal-exit uses the existing host smoke hook's ordinary RunEvent::Exit path.
Without it, cleanup terminates exact owned processes, not a normal quit.
"""
import argparse
import ctypes
import contextlib
import hashlib
import http.client
import ipaddress
import json
import os
from pathlib import Path
import select
import signal
import subprocess
import sys
import tempfile
import time


def probe(host, sidecar, managed, normal_exit=False):
    # Adopt our own orphaned sidecar so cleanup can reap it, never kill by name.
    if ctypes.CDLL(None, use_errno=True).prctl(36, 1, 0, 0, 0) != 0:
        raise RuntimeError("could not enable owned-child reaping")
    # Retain owned fixtures and diagnostics on failure as well as success.
    with contextlib.nullcontext(tempfile.mkdtemp(prefix="reasonix-linux-package-")) as directory:
        root = Path(directory)
        print("private fixture", root, flush=True)
        paths = {name: root / name for name in ("home", "data", "config", "cache", "tmp", "runtime", "core")}
        for path in paths.values():
            path.mkdir(mode=0o700)
        env = {key: os.environ[key] for key in ("DISPLAY", "XAUTHORITY", "DBUS_SESSION_BUS_ADDRESS") if key in os.environ}
        env.update(PATH="/usr/bin:/bin", LANG="C.UTF-8", HOME=str(paths["home"]),
                   TMPDIR=str(paths["tmp"]), XDG_DATA_HOME=str(paths["data"]),
                   XDG_CONFIG_HOME=str(paths["config"]), XDG_CACHE_HOME=str(paths["cache"]),
                   XDG_RUNTIME_DIR=str(paths["runtime"]), REASONIX_CREDENTIALS_STORE="file")
        # Release packages must select their packaged sidecar and stdin token,
        # not inherited development overrides or an environment credential.
        env["REASONIX_DESKTOP_BRIDGE_BIN"] = str(root / "nonexistent-sidecar")
        env["REASONIX_DESKTOP_BRIDGE_TOKEN"] = "inherited-token-must-be-cleared"
        if normal_exit:
            env["REASONIX_TAURI_PACKAGE_SMOKE"] = "1"
        if not managed:
            env.update(REASONIX_HOME=str(paths["core"]), REASONIX_STATE_HOME=str(paths["core"]),
                       REASONIX_CACHE_HOME=str(paths["cache"] / "core"))
        child_pidfd = None
        child_pid = None
        with (root / "host.log").open("wb") as log:
            process = subprocess.Popen([str(host)], cwd=root, env=env, stdout=log, stderr=log)
            try:
                deadline = time.monotonic() + 30
                ready = None
                while time.monotonic() < deadline:
                    if process.poll() is not None:
                        raise RuntimeError("private packaged host exited before readiness")
                    # Only inspect kernel children of this exact live host.
                    children = Path(f"/proc/{process.pid}/task/{process.pid}/children").read_text().split()
                    for candidate in children if child_pidfd is None else []:
                        proc = Path("/proc") / candidate
                        try:
                            if (proc / "exe").resolve(strict=True) == sidecar:
                                child_pid = int(candidate)
                                child_pidfd = os.pidfd_open(child_pid)
                                break
                        except FileNotFoundError:
                            continue
                    matches = list(paths["tmp"].glob("reasonix-tauri-bridge-*/ready.json"))
                    if child_pidfd is not None and len(matches) == 1:
                        ready = json.loads(matches[0].read_text())
                        break
                    time.sleep(0.05)
                if ready is None or ready.get("protocolVersion") != 1 or not ready.get("sidecarInstanceId"):
                    raise RuntimeError("private packaged sidecar did not publish valid readiness")
                address, port = ready["address"].rsplit(":", 1)
                address = address.strip("[]")
                if not ipaddress.ip_address(address).is_loopback or not 0 < int(port) < 65536:
                    raise RuntimeError("packaged sidecar readiness escaped loopback")
                connection = http.client.HTTPConnection(address, int(port), timeout=2)
                try:
                    connection.request("GET", "/v1/health")
                    if connection.getresponse().status != 401:
                        raise RuntimeError("packaged sidecar accepted unauthenticated health")
                finally:
                    connection.close()
                # Never print process environment: it may contain secrets.
                raw_env = Path(f"/proc/{child_pid}/environ").read_bytes()
                inherited = dict(item.split(b"=", 1) for item in raw_env.split(b"\0") if b"=" in item)
                expected = paths["data"] / "io.reasonix.desktop.preview" / "reasonix-core" if managed else paths["core"]
                if Path(os.fsdecode(inherited.get(b"REASONIX_HOME", b""))).resolve() != expected.resolve():
                    raise RuntimeError("packaged sidecar profile escaped its expected private root")
                if inherited.get(b"REASONIX_CREDENTIALS_STORE") != b"file":
                    raise RuntimeError("private probe unexpectedly selected a real credential service")
                if inherited.get(b"REASONIX_DESKTOP_BRIDGE_TOKEN", b""):
                    raise RuntimeError("packaged launcher leaked an inherited bridge token")
                identity = json.loads((expected / "tauri-credential-profile.json").read_text())
                if identity.get("version") != 1 or len(identity.get("id", "")) != 32:
                    raise RuntimeError("private profile credential identity missing")
                if process.poll() is not None or select.select([child_pidfd], [], [], 0)[0]:
                    raise RuntimeError("owned host or sidecar stopped during startup checks")
                if normal_exit:
                    if process.wait(timeout=20) != 0:
                        raise RuntimeError("packaged host normal Exit path failed")
                    if not select.select([child_pidfd], [], [], 5)[0]:
                        raise RuntimeError("normal Exit did not stop the owned sidecar")
                    if matches[0].exists():
                        raise RuntimeError("normal Exit left sidecar readiness behind")
                    target = json.loads((paths["tmp"] / "reasonix-global-workspace-smoke.json").read_text())
                    workspace = expected / "global-workspace"
                    if target.get("protocolVersion") != 1 or target.get("sessionId") != "tauri-package-workspace-smoke" or target.get("workspaceRoot") != str(workspace.resolve()):
                        raise RuntimeError("Global workspace escaped the private profile")
                    if not workspace.is_dir() or workspace.is_symlink() or workspace.stat().st_mode & 0o777 != 0o700:
                        raise RuntimeError("Global workspace is not a private regular directory")
                    # Record XDG's truthful readonly status; no notifications sent.
                    notification = json.loads((paths["tmp"] / "reasonix-native-notification-smoke.json").read_text())
                    if notification.get("permission") not in {"not_determined", "denied", "granted", "provisional", "unknown", "unavailable"}:
                        raise RuntimeError("invalid native notification status")
                    if type(notification.get("clickSupported")) is not bool:
                        raise RuntimeError("invalid native notification click status")
                print(json.dumps({"profile": "managed" if managed else "explicit", "startup": "passed",
                                  "unauthenticatedHealth": 401, "privateProfile": True,
                                  "cleanup": "ordinary RunEvent::Exit; sidecar/readiness gone" if normal_exit else "owned-process termination, not normal quit"}))
            finally:
                if process.poll() is None:
                    process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
                # Early host exit can occur between fork and the first poll.
                # As subreaper we own the adopted child; match only our kernel
                # children and this exact packaged executable, never a name.
                if child_pidfd is None:
                    children = Path(f"/proc/{os.getpid()}/task/{os.getpid()}/children").read_text().split()
                    for candidate in children:
                        try:
                            if (Path("/proc") / candidate / "exe").resolve(strict=True) == sidecar:
                                child_pid = int(candidate)
                                child_pidfd = os.pidfd_open(child_pid)
                                break
                        except FileNotFoundError:
                            continue
                if child_pidfd is not None:
                    try:
                        if not select.select([child_pidfd], [], [], 0)[0]:
                            signal.pidfd_send_signal(child_pidfd, signal.SIGTERM)
                        if not select.select([child_pidfd], [], [], 5)[0]:
                            signal.pidfd_send_signal(child_pidfd, signal.SIGKILL)
                        if not select.select([child_pidfd], [], [], 5)[0]:
                            raise RuntimeError("owned sidecar survived cleanup")
                        # A normal host Exit reaps its child; abrupt host cleanup
                        # instead adopts it into this probe's subreaper.
                        try:
                            os.waitpid(child_pid, 0)
                        except ChildProcessError:
                            pass
                    finally:
                        os.close(child_pidfd)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("host", type=Path)
    parser.add_argument("sidecar", type=Path)
    parser.add_argument("--normal-exit", action="store_true")
    parser.add_argument("--profile", choices=("managed", "explicit", "both"), default="both")
    args = parser.parse_args()
    if sys.platform != "linux" or not os.environ.get("DISPLAY") or not os.environ.get("DBUS_SESSION_BUS_ADDRESS"):
        raise SystemExit("Linux probe requires an owned Xvfb display and dbus-run-session")
    host, sidecar = (path.resolve(strict=True) for path in (args.host, args.sidecar))
    for binary in (host, sidecar):
        print(binary.name, "SHA256", hashlib.sha256(binary.read_bytes()).hexdigest())
    profiles = (True, False) if args.profile == "both" else (args.profile == "managed",)
    for managed in profiles:
        probe(host, sidecar, managed, args.normal_exit)
