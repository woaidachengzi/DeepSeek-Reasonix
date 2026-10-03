#!/usr/bin/env python3
"""Compare native edit focus using a desktop launch and an explicit private profile.

Diagnostic only: open -W does not return the actual host exit status. This
does not replace the strict direct-launch window gate or physical UI testing.
Uses the existing opt-in native edit probe and clipboard preservation fixture.
"""
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("native_windows", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)
package = windows.package


def matching_host(pid, binary):
    return any(process == pid and (command == str(binary) or command.startswith(str(binary) + " "))
               for process, _, command in package.processes())


def check_private_profile(pid, core, cache):
    # This diagnostic intentionally supplies a private explicit state override.
    # Inspect only fixed invariants; never print the process environment.
    result = subprocess.run(["ps", "eww", "-p", str(pid), "-o", "command="],
                            capture_output=True, text=True, check=True)
    for key, expected in {"REASONIX_HOME": core, "REASONIX_STATE_HOME": core / "state",
                          "REASONIX_CACHE_HOME": cache, "REASONIX_DESKTOP_BRIDGE_TOKEN": ""}.items():
        if not re.search(rf"(?:^|\s){key}={re.escape(str(expected))}(?:\s|$)", result.stdout):
            raise RuntimeError("desktop sidecar private profile/token invariant failed")
    if re.search(r"(?:^|\s)REASONIX_PREVIEW_SQLITE_EVENTS=", result.stdout):
        raise RuntimeError("desktop explicit profile inherited managed event storage")


def probe(app_path):
    app = Path(app_path).resolve()
    host = app / "Contents/MacOS/reasonix-tauri"
    sidecar = app / "Contents/MacOS/reasonix-desktop-bridge"
    if not host.is_file() or not sidecar.is_file():
        raise RuntimeError("packaged host or sidecar missing")
    with (app / "Contents/Info.plist").open("rb") as stream:
        identifier = plistlib.load(stream)["CFBundleIdentifier"]
    if package.matching_package_is_running(identifier):
        raise RuntimeError("a Preview is already running; no desktop launch performed")
    root = Path(tempfile.mkdtemp(prefix="reasonix-edit-launch-services-", dir="/private/tmp"))
    root.chmod(0o700)
    home, temporary, core = root / "home", root / "tmp", root / "core"
    for path in (home, temporary):
        path.mkdir(mode=0o700)
    pid, launcher, success = None, None, False
    def stop_owned():
        if pid is not None and matching_host(pid, host):
            os.kill(pid, signal.SIGTERM)
            deadline = time.monotonic() + 5
            while matching_host(pid, host) and time.monotonic() < deadline:
                time.sleep(0.05)
            if matching_host(pid, host):
                os.kill(pid, signal.SIGKILL)
        if launcher is not None and launcher.poll() is None:
            launcher.terminate()
            launcher.wait(timeout=5)
        deadline = time.monotonic() + 5
        while package.own_sidecars(temporary, sidecar) and time.monotonic() < deadline:
            time.sleep(0.05)

    try:
        with windows.clipboard_fixture.NativeClipboardFixture(temporary) as clipboard:
            try:
                command = ["/usr/bin/open", "-n", "-W", str(app), "--stdout", "/dev/null", "--stderr", "/dev/null"]
                # All profile directories are explicit, so LaunchServices cannot
                # select ordinary user data through inherited directory settings.
                for key, value in {
                    "HOME": home, "TMPDIR": temporary, "REASONIX_HOME": core,
                    "REASONIX_STATE_HOME": core / "state", "REASONIX_CACHE_HOME": root / "cache",
                    "REASONIX_TAURI_NATIVE_WINDOW_SMOKE": "menu-editing",
                    "REASONIX_TAURI_PACKAGE_SMOKE": "", "REASONIX_TAURI_PROFILE_SMOKE": "",
                    "REASONIX_DESKTOP_BRIDGE_TOKEN": "must-be-cleared",
                }.items():
                    command.extend(["--env", f"{key}={value}"])
                launcher = subprocess.Popen(command, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                deadline = time.monotonic() + 25
                while time.monotonic() < deadline:
                    leases = list(temporary.glob("reasonix-tauri-bridge-*/launch-owner.json"))
                    if len(leases) == 1:
                        owner = json.loads(leases[0].read_text())
                        candidate = owner.get("hostPid")
                        if type(candidate) is int and candidate > 1 and matching_host(candidate, host):
                            pid = candidate
                    ready = list(temporary.glob("reasonix-tauri-bridge-*/ready.json"))
                    children = package.own_sidecars(temporary, sidecar)
                    if pid is not None and len(ready) == len(children) == 1:
                        if not any(child == children[0] and parent == pid for child, parent, _ in package.processes()):
                            raise RuntimeError("private sidecar does not belong to the launch-owner host")
                        address = package.check_ready(ready[0])
                        package.check_unauthenticated_health(address)
                        check_private_profile(children[0], core, root / "cache")
                        package.check_credential_profile(core)
                        break
                    if launcher.poll() is not None:
                        raise RuntimeError("desktop launcher ended before private host inspection")
                    time.sleep(0.05)
                else:
                    raise RuntimeError("desktop launch did not publish one private host/sidecar")
                deadline = time.monotonic() + 45
                while matching_host(pid, host) and time.monotonic() < deadline:
                    time.sleep(0.05)
                if matching_host(pid, host):
                    raise RuntimeError("desktop edit probe host did not exit")
                if launcher.wait(timeout=5) != 0:
                    raise RuntimeError("desktop launch waiter failed; actual host exit status unavailable")
                windows.print_window_trace(temporary)
                result = json.loads((temporary / "reasonix-native-window-result.json").read_text())
                if result.get("phase") != "menu-editing" or result.get("ok") is not True:
                    raise RuntimeError("desktop native edit focus/operation receipt failed")
                deadline = time.monotonic() + 5
                while package.own_sidecars(temporary, sidecar) and time.monotonic() < deadline:
                    time.sleep(0.05)
                if package.own_sidecars(temporary, sidecar) or list(temporary.glob("reasonix-tauri-bridge-*/ready.json")):
                    raise RuntimeError("desktop edit probe left sidecar/readiness")
                clipboard.verify()
                print("LaunchServices explicit private native edit receipt: OK; host exit status and full window acceptance remain unproven")
            finally:
                stop_owned()
        success = True
    finally:
        stop_owned()
        if success:
            shutil.rmtree(root)
        else:
            print(f"Private desktop edit failure fixture retained: {root}", file=sys.stderr)


if __name__ == "__main__":
    if sys.platform != "darwin" or len(sys.argv) != 2:
        raise SystemExit("usage: probe-edit-launch-services.py MACOS_PREVIEW_APP")
    try:
        probe(sys.argv[1])
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"desktop edit diagnostic failed: {error}") from error
