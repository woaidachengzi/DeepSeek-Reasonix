#!/usr/bin/env python3
"""Terminate only owned macOS hosts; verify sidecar/task/lock cleanup and restart.

Eight cases: managed/explicit, SIGTERM/SIGKILL, idle/actually streaming. Private
HOME/core/TMPDIR and a bounded loopback provider; no user profile or model API.
Successful fixtures are removed; failures retain evidence after owned cleanup.
"""
from contextlib import nullcontext
import importlib.util
import json
import os
from pathlib import Path
import signal
import stat
import subprocess
import sys
import tempfile
import time
import shutil

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("native_window", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)
package = windows.package


def protected(paths):
    return [(path.read_bytes(), path.stat().st_mode, path.stat().st_mtime_ns) for path in paths]


def process_identity(pid):
    return subprocess.check_output(["ps", "-p", str(pid), "-o", "uid=,ppid=,lstart=,comm="], text=True)


def smoke(app_path):
    app = Path(app_path).resolve()
    host_binary = app / "Contents/MacOS/reasonix-tauri"
    sidecar_binary = app / "Contents/MacOS/reasonix-desktop-bridge"
    identifier = "io.reasonix.desktop.preview"
    if not host_binary.is_file() or not sidecar_binary.is_file() or package.matching_package_is_running(identifier):
        raise RuntimeError("packaged Preview missing or already running")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    environment = {key: value for key, value in os.environ.items()
                   if key in ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL", "__CF_USER_TEXT_ENCODING")}
    for managed in (True, False):
        for termination in (signal.SIGTERM, signal.SIGKILL):
            for active in (False, True):
                root = Path(tempfile.mkdtemp(prefix="reasonix-host-lifetime-", dir="/private/tmp"))
                root.chmod(0o700)
                for name in ("home", "tmp"):
                    (root / name).mkdir(mode=0o700)
                home, temporary = root / "home", root / "tmp"
                core = home / "Library/Application Support" / identifier / "reasonix-core" if managed else root / "core"
                core.mkdir(parents=True, mode=0o700)
                canary = core / "host-lifetime-original.canary"
                canary.write_bytes(b"host lifetime original\n")
                canary.chmod(0o600)
                fixture = windows.task_provider.NativeTaskProvider(core, temporary, host_death=True) if active else nullcontext()
                success = False
                host = None
                owned_sidecar = None
                try:
                    with fixture as provider:
                        env = dict(environment, HOME=str(home), TMPDIR=str(temporary))
                        if not managed:
                            env.update(REASONIX_HOME=str(core), REASONIX_CACHE_HOME=str(root / "cache"))
                        if active:
                            env["REASONIX_TAURI_NATIVE_WINDOW_SMOKE"] = "task-host-death"
                        host = subprocess.Popen([str(host_binary)], env=env, stdout=subprocess.DEVNULL,
                                                stderr=subprocess.DEVNULL, start_new_session=True)
                        deadline = time.monotonic() + 20
                        while host.poll() is None and time.monotonic() < deadline:
                            ready = list(temporary.glob("reasonix-tauri-bridge-*/ready.json"))
                            children = package.own_sidecars(temporary, sidecar_binary)
                            if len(ready) == 1 and len(children) == 1:
                                owned_sidecar = children[0]
                                package.check_unauthenticated_health(package.check_ready(ready[0]))
                                package.check_sidecar_profile(owned_sidecar, core, managed, root / "cache")
                                credential = package.check_credential_profile(core)
                                if stat.S_IMODE(ready[0].parent.stat().st_mode) != 0o700:
                                    raise RuntimeError("native readiness directory is not private")
                                break
                            time.sleep(0.05)
                        else:
                            raise RuntimeError("owned host did not start one ready sidecar")
                        sidecar_identity = process_identity(owned_sidecar)
                        fields = sidecar_identity.split()
                        if int(fields[0]) != os.getuid() or int(fields[1]) != host.pid or str(sidecar_binary) not in sidecar_identity:
                            raise RuntimeError("sidecar is not the confirmed native host child")
                        marker = temporary / "reasonix-native-task-host-death.json"
                        if active:
                            while host.poll() is None and not marker.is_file() and time.monotonic() < deadline:
                                time.sleep(0.05)
                            receipt = json.loads(marker.read_text()) if marker.is_file() else {}
                            if receipt != {"phase": "task-host-death", "hostPid": host.pid, "taskRunning": True, "observedText": True} or provider.requests != 1:
                                raise RuntimeError("host did not observe the actual active provider stream")
                        paths = [canary, core / "tauri-credential-profile.json", core / "tauri-credential-profile.backup.json"]
                        if (core / "config.toml").is_file():
                            paths.append(core / "config.toml")
                        originals = protected(paths)
                        host_identity = process_identity(host.pid)
                        if int(host_identity.split()[0]) != os.getuid() or str(host_binary) not in host_identity:
                            raise RuntimeError("owned host executable identity changed")
                        if host.poll() is not None or package.own_sidecars(temporary, sidecar_binary) != [owned_sidecar] or process_identity(owned_sidecar) != sidecar_identity:
                            raise RuntimeError("process identity changed before host termination")
                        (root / "owned-processes.json").write_text(json.dumps({"host": host.pid, "sidecar": owned_sidecar}))
                        host.send_signal(termination)
                        if host.wait(timeout=5) != -termination:
                            raise RuntimeError("host did not terminate from the requested signal")
                        cleanup_deadline = time.monotonic() + 12
                        while (package.is_alive(owned_sidecar) or list(temporary.glob("reasonix-tauri-bridge-*"))) and time.monotonic() < cleanup_deadline:
                            time.sleep(0.05)
                        if package.is_alive(owned_sidecar) or package.own_sidecars(temporary, sidecar_binary) or list(temporary.glob("reasonix-tauri-bridge-*")):
                            raise RuntimeError(f"native host termination cleanup failed: sidecarAlive={package.is_alive(owned_sidecar)}, readinessDirectories={len(list(temporary.glob('reasonix-tauri-bridge-*')))}")
                        if active:
                            provider.verify_host_death() # Before the fixture closes its server.
                        if protected(paths) != originals:
                            raise RuntimeError("host termination changed protected profile originals")
                        # Reuse the same profile, requiring the same durable identity.
                        # A successful real bridge startup proves both directory locks
                        # were released rather than just readiness being removed.
                        restarted = windows.launch(host_binary, sidecar_binary, root, identifier, managed,
                                                   "menu-shortcuts", verify_window_state=False, environment=environment)
                        if restarted != credential:
                            raise RuntimeError("host termination/restart changed the profile identity")
                        if protected(paths) != originals:
                            raise RuntimeError("restart changed protected profile originals")
                        success = True
                        print(f"native macOS host lifetime {'managed' if managed else 'explicit'} {termination.name} {'streaming' if active else 'idle'}: cleanup, originals and same-profile restart OK", flush=True)
                finally:
                    if host is not None and host.poll() is None:
                        host.kill()
                        host.wait(timeout=5)
                    # Failure cleanup is never counted as acceptance. Limit it
                    # to package sidecars with this private readiness root.
                    for pid in package.own_sidecars(temporary, sidecar_binary):
                        package.check_sidecar_profile(pid, core, managed, root / "cache")
                        if pid == owned_sidecar:
                            try: os.kill(pid, signal.SIGKILL)
                            except ProcessLookupError: pass
                    if success:
                        shutil.rmtree(root)
                    else:
                        print(f"Private host-lifetime failure fixture retained: {root}", file=sys.stderr)


if __name__ == "__main__":
    if sys.platform != "darwin" or len(sys.argv) != 2:
        raise SystemExit("usage: smoke-host-lifetime.py MACOS_PREVIEW_APP")
    smoke(sys.argv[1])
