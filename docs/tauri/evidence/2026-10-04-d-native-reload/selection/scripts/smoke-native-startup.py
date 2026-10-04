#!/usr/bin/env python3
"""Kill an owned macOS host at three real pre-readiness startup boundaries.

Twelve cases: managed/explicit x SIGTERM/SIGKILL x before parent validation,
before token consumption, and after token consumption but before core startup.
Uses the real packaged host/bridge, kernel exit receipts and private profiles.
No renderer command, provider request or normal-exit cleanup substitute.
"""
import importlib.util
import json
import os
from pathlib import Path
import select
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("lifetime", Path(__file__).with_name("smoke-host-lifetime.py"))
lifetime = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lifetime)
windows, package = lifetime.windows, lifetime.package


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
            for phase in ("startup-before-parent-check", "startup-before-token", "startup-before-ready"):
                root = Path(tempfile.mkdtemp(prefix="reasonix-native-startup-", dir="/private/tmp"))
                root.chmod(0o700)
                home, temporary = root / "home", root / "tmp"
                home.mkdir(mode=0o700)
                temporary.mkdir(mode=0o700)
                core = home / "Library/Application Support" / identifier / "reasonix-core" if managed else root / "core"
                core.mkdir(parents=True, mode=0o700)
                canary = core / "startup-original.canary"
                canary.write_bytes(b"startup original\n")
                canary.chmod(0o600)
                host, owned_sidecar, monitor, success = None, None, None, False
                try:
                    env = dict(environment, HOME=str(home), TMPDIR=str(temporary), REASONIX_TAURI_NATIVE_WINDOW_SMOKE=phase)
                    if not managed:
                        env.update(REASONIX_HOME=str(core), REASONIX_CACHE_HOME=str(root / "cache"))
                    host = subprocess.Popen([str(host_binary)], env=env, stdout=subprocess.DEVNULL,
                                            stderr=subprocess.DEVNULL, start_new_session=True)
                    marker = temporary / "reasonix-native-startup.json"
                    deadline = time.monotonic() + 10
                    while host.poll() is None and time.monotonic() < deadline:
                        children = package.own_sidecars(temporary, sidecar_binary)
                        if len(children) == 1 and marker.is_file():
                            owned_sidecar = children[0]
                            break
                        time.sleep(0.05)
                    else:
                        raise RuntimeError("native startup boundary not reached")
                    receipt = json.loads(marker.read_text())
                    if receipt != {"phase": phase, "hostPid": host.pid, "sidecarPid": owned_sidecar,
                                   "tokenConsumed": phase == "startup-before-ready"}:
                        raise RuntimeError("native startup boundary receipt mismatched")
                    identity = lifetime.process_identity(owned_sidecar)
                    fields = identity.split()
                    if int(fields[0]) != os.getuid() or int(fields[1]) != host.pid or str(sidecar_binary) not in identity:
                        raise RuntimeError("startup sidecar is not the confirmed native host child")
                    package.check_sidecar_profile(owned_sidecar, core, managed, root / "cache")
                    credential = package.check_credential_profile(core)
                    directories = list(temporary.glob("reasonix-tauri-bridge-*"))
                    if len(directories) != 1 or stat.S_IMODE(directories[0].stat().st_mode) != 0o700:
                        raise RuntimeError("startup directory is not uniquely private")
                    lease = directories[0] / "launch-owner.json"
                    record = json.loads(lease.read_text())
                    if stat.S_IMODE(lease.stat().st_mode) != 0o600 or record.get("hostPid") != host.pid or len(record.get("launchId", "")) < 32:
                        raise RuntimeError("startup lease is not bound to its native host")
                    if list(temporary.glob("reasonix-tauri-bridge-*/ready.json")):
                        raise RuntimeError("startup boundary already published readiness")
                    paths = [canary, core / "tauri-credential-profile.json", core / "tauri-credential-profile.backup.json"]
                    originals = lifetime.protected(paths)
                    if host.poll() is not None or lifetime.process_identity(owned_sidecar) != identity:
                        raise RuntimeError("startup process identity changed before termination")
                    (root / "owned-processes.json").write_text(json.dumps({"host": host.pid, "sidecar": owned_sidecar}))
                    monitor = select.kqueue()
                    monitor.control([select.kevent(owned_sidecar, filter=select.KQ_FILTER_PROC,
                        flags=select.KQ_EV_ADD | select.KQ_EV_ONESHOT,
                        fflags=select.KQ_NOTE_EXIT | 0x04000000)], 0, 0)
                    host.send_signal(termination)
                    if host.wait(timeout=5) != -termination:
                        raise RuntimeError("startup host did not exit from the requested signal")
                    exits = monitor.control([], 1, 12)
                    if len(exits) != 1 or exits[0].ident != owned_sidecar or not exits[0].fflags & select.KQ_NOTE_EXIT:
                        raise RuntimeError("startup sidecar has no kernel exit receipt")
                    status = exits[0].data
                    if not os.WIFEXITED(status) or os.WEXITSTATUS(status) != 0:
                        raise RuntimeError("startup sidecar did not finish its own normal cleanup")
                    deadline = time.monotonic() + 5
                    while (package.is_alive(owned_sidecar) or list(temporary.glob("reasonix-tauri-bridge-*"))) and time.monotonic() < deadline:
                        time.sleep(0.05)
                    if package.is_alive(owned_sidecar) or package.own_sidecars(temporary, sidecar_binary) or list(temporary.glob("reasonix-tauri-bridge-*")):
                        raise RuntimeError("startup host termination left sidecar/lease/readiness directory")
                    if lifetime.protected(paths) != originals:
                        raise RuntimeError("startup host termination changed protected originals")
                    restarted = windows.launch(host_binary, sidecar_binary, root, identifier, managed,
                                               "menu-shortcuts", verify_window_state=False, environment=environment)
                    if restarted != credential or lifetime.protected(paths) != originals:
                        raise RuntimeError("startup restart changed durable identity or protected originals")
                    success = True
                    print(f"native macOS startup {'managed' if managed else 'explicit'} {termination.name} {phase}: kernel exit 0, cleanup, originals and restart OK", flush=True)
                finally:
                    if monitor is not None:
                        monitor.close()
                    if host is not None and host.poll() is None:
                        host.kill()
                        host.wait(timeout=5)
                    for pid in package.own_sidecars(temporary, sidecar_binary):
                        package.check_sidecar_profile(pid, core, managed, root / "cache")
                        if pid == owned_sidecar:
                            try:
                                os.kill(pid, signal.SIGKILL)
                            except ProcessLookupError:
                                pass
                    if success:
                        shutil.rmtree(root)
                    else:
                        print(f"Private startup failure fixture retained: {root}", file=sys.stderr)


if __name__ == "__main__":
    if sys.platform != "darwin" or len(sys.argv) != 2:
        raise SystemExit("usage: smoke-native-startup.py MACOS_PREVIEW_APP")
    smoke(sys.argv[1])
