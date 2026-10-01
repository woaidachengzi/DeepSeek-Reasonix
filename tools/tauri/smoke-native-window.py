#!/usr/bin/env python3
"""Verify macOS windows, native menus and task lifecycle in a private profile.

Uses real Tauri window APIs, installed AppKit Settings menu actions, and the
shared tray/Dock/single-instance show path. Settings checks the host event,
not the rendered WebView settings overlay.
Tasks stream through the real Go core and a bounded loopback provider. Native
close must preserve background progress; the installed Quit menu must clean up
the active stream, host and sidecar without a runner-driven successful exit.
The --focus gate additionally requires a real second process to restore focus;
it remains a strict separate acceptance condition when activation is unavailable.
The --edit gate requires an active main key window and tests installed edit
roles against disposable WKWebView textareas, including native undo/redo.
The --dialogs gate cancels actual AppKit document/diagnostics/import/folder
panels and verifies production callbacks; it does not simulate mouse clicks.
Does not prove menu/tray clicks, physical keyboard editing, or display unplugging.
All state is confined to a temporary HOME; no existing Preview is operated.
"""

import argparse
import importlib.util
from contextlib import nullcontext
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
spec = importlib.util.spec_from_file_location(
    "package_smoke", Path(__file__).with_name("smoke-packaged-app.py")
)
package = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package)
task_spec = importlib.util.spec_from_file_location(
    "native_task_provider", Path(__file__).with_name("native-task-provider.py")
)
task_provider = importlib.util.module_from_spec(task_spec)
task_spec.loader.exec_module(task_provider)
clipboard_spec = importlib.util.spec_from_file_location(
    "native_clipboard_fixture", Path(__file__).with_name("native-clipboard-fixture.py")
)
clipboard_fixture = importlib.util.module_from_spec(clipboard_spec)
clipboard_spec.loader.exec_module(clipboard_fixture)


def verify_saved_state(app_data, temporary, maximized):
    saved = json.loads((app_data / "window-state.json").read_text())
    normal = json.loads((temporary / "reasonix-native-window-normal.json").read_text())
    if saved.get("maximized") is not maximized:
        raise RuntimeError("persisted native maximized state is incorrect")
    for key in ("width", "height", "x", "y"):
        if saved.get(key) != normal.get(key):
            raise RuntimeError(f"native transition overwrote saved normal {key}")
    if saved.get("scale_factor") != normal.get("scale"):
        raise RuntimeError("native transition overwrote saved normal display scale")


def print_window_trace(temporary):
    """Keep bounded, fixed-status evidence before TemporaryDirectory cleanup."""
    path = temporary / "reasonix-native-window-trace.jsonl"
    try:
        with path.open("rb") as file:
            data = file.read(16_385)
        lines = data.decode("utf-8").splitlines()
        if len(data) > 16_384 or len(lines) > 4:
            return
        fields = ("mainPageFinished", "focused", "minimized", "visible",
                  "applicationActive", "applicationHidden", "nativeKeyWindow",
                  "nativeMainWindow", "nativeCanBecomeKey", "nativeOnActiveSpace",
                  "nativeMiniaturizable", "nativeMiniaturized", "nativeVisible")
        stages = {"settings-ready", "settings-shown", "settings-minimize-requested",
                  "exercise-ready", "exercise-shown", "exercise-minimize-requested"}
        for line in lines:
            item = json.loads(line)
            if not isinstance(item, dict) or item.get("stage") not in stages:
                continue
            state = item.get("state")
            if not isinstance(state, dict):
                continue
            status = {key: state[key] for key in fields if type(state.get(key)) is bool}
            status.update({key: state[key] for key in ("restoreRequests", "restoreCompletions", "reopenEvents")
                           if type(state.get(key)) is int and 0 <= state[key] <= 1000})
            print("native window trace: " + json.dumps({"stage": item["stage"], "state": status}), flush=True)
    except (OSError, UnicodeError, ValueError, TypeError):
        # A missing/malformed diagnostic must not replace the original failure.
        pass


def launch(host_binary, sidecar_binary, root, identifier, managed, phase, provider=None,
           verify_window_state=True, environment=None):
    home, temporary = root / "home", root / "tmp"
    app_data = home / "Library/Application Support" / identifier
    core_home = app_data / "reasonix-core" if managed else root / "core"
    result_path = temporary / "reasonix-native-window-result.json"
    result_path.unlink(missing_ok=True)
    (temporary / "reasonix-native-window-trace.jsonl").unlink(missing_ok=True)
    instance_marker = temporary / "reasonix-native-window-awaiting-instance.json"
    instance_marker.unlink(missing_ok=True)
    env = dict(os.environ if environment is None else environment)
    for key in ("REASONIX_HOME", "REASONIX_STATE_HOME", "REASONIX_CACHE_HOME",
                "REASONIX_PREVIEW_SQLITE_EVENTS", "REASONIX_TAURI_PACKAGE_SMOKE",
                "REASONIX_TAURI_PROFILE_SMOKE"):
        env.pop(key, None)
    env.update({"HOME": str(home), "TMPDIR": str(temporary),
                "REASONIX_TAURI_NATIVE_WINDOW_SMOKE": phase,
                "REASONIX_DESKTOP_BRIDGE_TOKEN": "must-be-cleared"})
    if not managed:
        env["REASONIX_HOME"] = str(core_home)
        env["REASONIX_CACHE_HOME"] = str(root / "cache")
    host = subprocess.Popen([str(host_binary)], env=env, stdout=subprocess.DEVNULL,
                            stderr=subprocess.DEVNULL, start_new_session=True)
    sidecar_pid = None
    second = None
    try:
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline:
            ready = list(temporary.glob("reasonix-tauri-bridge-*/ready.json"))
            children = [pid for pid, parent, command in package.processes()
                        if parent == host.pid and command.startswith(str(sidecar_binary))]
            if len(ready) == len(children) == 1:
                sidecar_pid = children[0]
                address = package.check_ready(ready[0])
                package.check_unauthenticated_health(address)
                package.check_sidecar_profile(sidecar_pid, core_home, managed, root / "cache")
                identity = package.check_credential_profile(core_home)
                ready_identity = json.loads(ready[0].read_text())
                break
            if host.poll() is not None:
                raise RuntimeError("native window host exited before sidecar inspection")
            time.sleep(0.05)
        else:
            raise RuntimeError("native window host did not start one ready sidecar")
        if phase == "second-instance":
            deadline = time.monotonic() + 10
            while not instance_marker.is_file() and time.monotonic() < deadline:
                if host.poll() is not None or result_path.is_file():
                    result = json.loads(result_path.read_text()) if result_path.is_file() else {}
                    raise RuntimeError(f"{phase}: {result.get('error', 'host exited before background close')}")
                time.sleep(0.05)
            if not instance_marker.is_file():
                raise RuntimeError("host did not reach the second-instance acceptance")
            # LaunchServices supplies the application activation context that a
            # raw executable launch lacks on modern macOS. Explicit, nonsecret
            # environment values confine even an unexpected second host to the
            # same private profile and make it fail rather than run indefinitely.
            second_command = ["/usr/bin/open", "-n", "-W", str(host_binary.parents[2]),
                              "--stdout", "/dev/null", "--stderr", "/dev/null"]
            for key, value in {
                "HOME": home, "TMPDIR": temporary, "REASONIX_HOME": core_home,
                "REASONIX_STATE_HOME": core_home / "state", "REASONIX_CACHE_HOME": root / "cache",
                "REASONIX_TAURI_PACKAGE_SMOKE": "",
                "REASONIX_TAURI_NATIVE_WINDOW_SMOKE": "unexpected-second-primary",
            }.items():
                second_command.extend(["--env", f"{key}={value}"])
            second = subprocess.Popen(second_command,
                                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                      start_new_session=True)
            if second.wait(timeout=10) != 0:
                raise RuntimeError("second instance did not exit successfully")
            live_sidecars = package.own_sidecars(temporary, sidecar_binary)
            if live_sidecars != [sidecar_pid]:
                raise RuntimeError("second instance created or replaced the sidecar")
            package.check_unauthenticated_health(address)
        def inspect_live_task():
            if host.poll() is not None or package.own_sidecars(temporary, sidecar_binary) != [sidecar_pid]:
                raise RuntimeError("background native task replaced or exited its host/sidecar")
            if not ready[0].is_file():
                raise RuntimeError("background native task removed readiness")
            if json.loads(ready[0].read_text()) != ready_identity:
                raise RuntimeError("background native task replaced its sidecar instance")
            package.check_unauthenticated_health(address)

        deadline = time.monotonic() + 45
        while host.poll() is None and time.monotonic() < deadline:
            if provider:
                provider.pump(inspect_live_task)
            time.sleep(0.05)
        if host.poll() is None:
            raise RuntimeError("native window host did not exit")
        exit_code = host.wait(timeout=5)
        result = json.loads(result_path.read_text())
        if result.get("phase") != phase or result.get("ok") is not True or exit_code != 0:
            print_window_trace(temporary)
            raise RuntimeError(f"{phase}: {result.get('error', 'native acceptance failed')}")
        cleanup_deadline = time.monotonic() + 5
        while package.is_alive(sidecar_pid) and time.monotonic() < cleanup_deadline:
            time.sleep(0.05)
        if package.is_alive(sidecar_pid) or list(temporary.glob("reasonix-tauri-bridge-*/ready.json")):
            raise RuntimeError("native window host left sidecar/readiness after exit")
        if verify_window_state:
            verify_saved_state(app_data, temporary, phase == "exercise")
        if provider:
            provider.verify()
        print(f"native macOS window {('managed' if managed else 'explicit')} {phase}: OK")
        return identity
    finally:
        if second is not None and second.poll() is None:
            second.kill()
            second.wait(timeout=5)
        if host.poll() is None:
            host.kill()
            host.wait(timeout=5)
        # Inspect only package sidecars carrying this private readiness root.
        for pid in package.own_sidecars(temporary, sidecar_binary):
            try:
                os.kill(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass


def smoke(app_path, include_focus=False, include_edit=False, include_dialogs=False):
    app = Path(app_path).resolve()
    host_binary = app / "Contents/MacOS/reasonix-tauri"
    sidecar_binary = app / "Contents/MacOS/reasonix-desktop-bridge"
    if not host_binary.is_file() or not sidecar_binary.is_file():
        raise RuntimeError("packaged host or sidecar missing")
    with (app / "Contents/Info.plist").open("rb") as file:
        identifier = plistlib.load(file).get("CFBundleIdentifier")
    if not isinstance(identifier, str) or not identifier:
        raise RuntimeError("package bundle identifier missing")
    if package.matching_package_is_running(identifier):
        raise RuntimeError("a Preview with this identifier is running; close it before acceptance")
    for managed in (True, False):
        with tempfile.TemporaryDirectory(prefix="reasonix-native-window-smoke-") as directory:
            root = Path(directory)
            (root / "home").mkdir()
            (root / "tmp").mkdir()
            phases = ["exercise", "restore-maximized", "restore-normal", "appearance-rollback-unconfigured", "application-hide", "background-close"]
            phases.extend(["menu-shortcuts", "menu-settings-hidden", "menu-settings-minimized", "menu-settings-app-hidden"])
            phases.append("task-background-menu-quit")
            phases.extend(["appearance-dark", "restore-appearance-dark", "restore-appearance-light", "restore-appearance-auto"])
            phases.extend(["appearance-rollback", "restore-appearance-rollback"])
            phases.append("clipboard-native")
            if include_dialogs:
                phases.append("dialog-cancel")
            if include_edit:
                phases.append("menu-editing")
            if include_focus:
                phases.append("second-instance")
            phases.append("close-quit")
            phases.append("restore-close-quit")
            identities = []
            for phase in phases:
                core_home = root / "home/Library/Application Support" / identifier / "reasonix-core" if managed else root / "core"
                fixture = task_provider.NativeTaskProvider(core_home, root / "tmp") if phase == "task-background-menu-quit" else nullcontext()
                if phase in ("clipboard-native", "menu-editing"):
                    fixture = clipboard_fixture.NativeClipboardFixture(root / "tmp")
                with fixture as provider:
                    identities.append(launch(host_binary, sidecar_binary, root, identifier, managed, phase, provider))
            if len(set(identities)) != 1:
                raise RuntimeError("restarts changed the native profile credential identity")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    parser.add_argument("--focus", action="store_true")
    parser.add_argument("--edit", action="store_true")
    parser.add_argument("--dialogs", action="store_true")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("this acceptance requires macOS")
    try:
        smoke(args.app, include_focus=args.focus, include_edit=args.edit,
              include_dialogs=args.dialogs)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"native macOS window smoke failed: {error}") from error
