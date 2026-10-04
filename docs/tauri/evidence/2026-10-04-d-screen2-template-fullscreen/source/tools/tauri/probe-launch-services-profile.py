#!/usr/bin/env python3
"""Launch the real installed .app via LaunchServices in a fresh private profile.

Default: verify native package-smoke exit for managed and explicit profiles.
--interactive: leave one explicit private host running for CUA acceptance;
print its confirmed PID/sidecar/root. This is not UI or window acceptance.
--interactive --observe: publish bounded read-only native window snapshots
and retain the exact host kernel exit receipt while waiting for real UI Quit.
Keep CUA observations bound to that live PID and private origin. After Quit,
check the saved PID/processes; do not query a dead app binding, which may
resolve a newly launched/default-profile instance.
"""
import argparse
import json
import os
from pathlib import Path
import plistlib
import select
import shutil
import signal
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True
import importlib.util
spec = importlib.util.spec_from_file_location("package", Path(__file__).with_name("smoke-packaged-app.py"))
package = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package)


def confirmed_host(pid, binary):
    return any(found == pid and command == str(binary) for found, _, command in package.processes())


NATIVE_PHASES = ("menu-settings-hidden", "menu-settings-minimized", "menu-settings-native-minimized")


def read_window_state_template(path):
    """Read only the supported saved geometry, never copy arbitrary profile data."""
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 4096:
        raise ValueError("window template must be a bounded regular JSON file")
    state = json.loads(path.read_text())
    if not isinstance(state, dict) or set(state) != {"width", "height", "x", "y", "scale_factor", "maximized"}:
        raise ValueError("window template must contain only saved geometry")
    if (any(type(state[key]) is not int for key in ("width", "height", "x", "y"))
            or not 900 <= state["width"] <= 16384 or not 620 <= state["height"] <= 16384
            or any(not -(2**31) <= state[key] < 2**31 for key in ("x", "y"))
            or type(state["scale_factor"]) not in (int, float)
            or not .5 <= state["scale_factor"] <= 8
            or state["maximized"] is not False):
        raise ValueError("window template must describe valid normal physical bounds")
    return state


def seed_window_state(app_data, state):
    # Used only in a newly created, owned private fixture before its host starts.
    for directory in reversed([app_data, *app_data.parents]):
        if not directory.exists():
            directory.mkdir(mode=0o700)
    target = app_data / "window-state.json"
    with target.open("x") as output:
        target.chmod(0o600)
        output.write(json.dumps(state) + "\n")


def launch(app, identifier, managed, interactive=False, observe=False, failure_exit=False, wait_seconds=300, existing_root=None, native_phase=None, window_state=None):
    if window_state is not None and (not interactive or not observe or existing_root is not None or native_phase is not None or failure_exit):
        raise ValueError("window template requires a fresh interactive observation")
    if native_phase is not None and (native_phase not in NATIVE_PHASES or interactive or observe or failure_exit or existing_root is not None):
        raise ValueError("native phase requires a new isolated noninteractive profile")
    previous_identity = None
    if existing_root is None:
        root = Path(tempfile.mkdtemp(prefix="reasonix-launch-services-", dir="/private/tmp"))
        root.chmod(0o700)
        home, temporary = root / "home", root / "tmp"
        home.mkdir(mode=0o700)
        temporary.mkdir(mode=0o700)
    else:
        if managed or not interactive or not observe or failure_exit:
            raise ValueError("profile reuse requires explicit interactive observation")
        root = Path(existing_root)
        if root != root.resolve() or root.parent != Path("/private/tmp") or not root.name.startswith("reasonix-launch-services-"):
            raise ValueError("only an exact private LaunchServices fixture may be reused")
        home, temporary = root / "home", root / "tmp"
        for directory in (root, home, temporary):
            if directory.is_symlink() or not directory.is_dir() or directory.stat().st_uid != os.getuid() or directory.stat().st_mode & 0o777 != 0o700:
                raise ValueError("reused fixture directories must be owned, private and regular")
        previous = json.loads((root / "launch.json").read_text())
        exited = json.loads((root / "result.json").read_text())
        if (previous.get("app") != str(app) or previous.get("root") != str(root)
                or previous.get("identifier") != identifier or previous.get("profile") != "explicit"
                or exited.get("exitCode") != 0 or exited.get("sidecarRemaining") is not False
                or exited.get("readyRemaining") is not False):
            raise ValueError("fixture lacks a matching completed normal launch")
        if (package.matching_package_is_running(identifier)
                or package.own_sidecars(temporary, app / "Contents/MacOS/reasonix-desktop-bridge")
                or list(temporary.glob("reasonix-tauri-bridge-*/ready.json"))):
            raise RuntimeError("reused fixture still has a live process or readiness")
        previous_identity = package.check_credential_profile(root / "core")
        history = Path(tempfile.mkdtemp(prefix="prior-launch-", dir=root))
        history.chmod(0o700)
        for name in ("launch.json", "launch-control.json", "result.json", "exit-receipt.json", "host.log"):
            source = root / name
            if source.is_file() and not source.is_symlink():
                shutil.copy2(source, history / name)
        # A prior successful receipt must not describe the new live/rejected run.
        for name in ("result.json", "exit-receipt.json"):
            (root / name).unlink(missing_ok=True)
    app_data = home / "Library/Application Support" / identifier
    if window_state is not None:
        seed_window_state(app_data, window_state)
    core = app_data / "reasonix-core" if managed else root / "core"
    binary = app / "Contents/MacOS/reasonix-tauri"
    sidecar_binary = app / "Contents/MacOS/reasonix-desktop-bridge"
    # Pass only intended, nonsecret profile values. The opener itself has a
    # minimal environment; do not manufacture empty state/event overrides in
    # an explicit profile that intentionally preserves caller overrides.
    env = {"HOME": str(home), "TMPDIR": str(temporary),
           "REASONIX_HOME": "" if managed else str(core),
           "REASONIX_TAURI_PACKAGE_SMOKE": "" if interactive or failure_exit or native_phase else "1"}
    if not managed:
        env["REASONIX_CACHE_HOME"] = str(root / "cache")
    if observe:
        env["REASONIX_TAURI_NATIVE_WINDOW_SMOKE"] = "interactive-observe"
    if failure_exit:
        env["REASONIX_TAURI_NATIVE_WINDOW_SMOKE"] = "invalid-exit-status-probe"
    if native_phase:
        env["REASONIX_TAURI_NATIVE_WINDOW_SMOKE"] = native_phase
    command = ["/usr/bin/open", "-n", "-W", str(app), "--stdout", str(root / "host.log"),
               "--stderr", str(root / "host.log")]
    for name, value in env.items():
        command += ["--env", name + "=" + value]
    opener = subprocess.Popen(command, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                              stderr=subprocess.DEVNULL, start_new_session=True,
                              env={key: os.environ[key] for key in ("PATH", "LANG", "USER", "LOGNAME") if key in os.environ})
    host = None
    sidecar = None
    retained = False
    monitor = None
    (root / "launch-control.json").write_text(json.dumps({"app": str(app), "root": str(root),
        "openerPid": opener.pid, "profile": "managed" if managed else "explicit"}, indent=2) + "\n")
    try:
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline:
            rows = list(package.processes())
            children = [(pid, parent) for pid, parent, cmd in rows
                        if cmd.startswith(str(sidecar_binary) + " ") and "--ready-file " + str(temporary) + "/" in cmd]
            ready = list(temporary.glob("reasonix-tauri-bridge-*/ready.json"))
            if len(children) == len(ready) == 1:
                sidecar, host = children[0]
                if not confirmed_host(host, binary):
                    raise RuntimeError("LaunchServices sidecar parent is not the installed host")
                monitor = select.kqueue()
                monitor.control([select.kevent(host, filter=select.KQ_FILTER_PROC,
                    flags=select.KQ_EV_ADD | select.KQ_EV_ONESHOT,
                    fflags=select.KQ_NOTE_EXIT | 0x04000000)], 0, 0)
                package.check_unauthenticated_health(package.check_ready(ready[0]))
                package.check_sidecar_profile(sidecar, core, managed, root / "cache")
                identity = package.check_credential_profile(core)
                if previous_identity is not None and identity != previous_identity:
                    raise RuntimeError("restarted fixture changed credential identity")
                if not app_data.is_dir():
                    raise RuntimeError("LaunchServices app data escaped private HOME")
                break
            if opener.poll() is not None:
                raise RuntimeError("LaunchServices exited before private sidecar readiness")
            time.sleep(.05)
        else:
            raise RuntimeError("LaunchServices did not establish one private host/sidecar")
        info = {"app": str(app), "root": str(root), "identifier": identifier,
                "pid": host, "sidecarPid": sidecar, "openerPid": opener.pid,
                "profile": "managed" if managed else "explicit", "interactive": interactive}
        if window_state is not None:
            # Require actual restored native geometry, not just a written file.
            expected = {key: window_state[key] for key in ("x", "y", "width", "height")}
            expected["scale"] = window_state["scale_factor"]
            observation = temporary / "reasonix-native-window-observation.json"
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                try:
                    actual = json.loads(observation.read_text())["state"]["geometry"]
                except (OSError, ValueError, KeyError):
                    actual = None
                if actual == expected:
                    info["initialWindowGeometry"] = actual
                    break
                if opener.poll() is not None:
                    raise RuntimeError("host exited before requested window placement")
                time.sleep(.05)
            else:
                raise RuntimeError("requested window placement not restored; no interactive acceptance started")
        if native_phase:
            info["nativePhase"] = native_phase
            (root / "launch.json").write_text(json.dumps(info, indent=2) + "\n")
            print(json.dumps(info), flush=True)
        if interactive:
            workspace = root / "workspace"
            workspace.mkdir(mode=0o700, exist_ok=existing_root is not None)
            if existing_root is None:
                (workspace / "canary.txt").write_text("Private native dialog acceptance canary\n")
            retained = not observe
            (root / "launch.json").write_text(json.dumps(info, indent=2) + "\n")
            print(json.dumps(info), flush=True)
            if not observe:
                return identity
        # Keep the kernel receipt attached to this exact PID during real UI
        # actions. Never inspect/reopen the dead CUA binding after user Quit.
        exits = monitor.control([], 1, wait_seconds if observe else (45 if native_phase else 25))
        if len(exits) != 1 or exits[0].ident != host or not exits[0].fflags & select.KQ_NOTE_EXIT:
            raise RuntimeError("LaunchServices host did not publish an exit receipt")
        status = exits[0].data
        if not os.WIFEXITED(status):
            (root / "exit-receipt.json").write_text(json.dumps({**info,
                "rawKernelStatus": status, "signaled": os.WIFSIGNALED(status),
                "signal": os.WTERMSIG(status) if os.WIFSIGNALED(status) else None}, indent=2) + "\n")
            raise RuntimeError("LaunchServices host was terminated by a signal")
        actual_code = os.WEXITSTATUS(status)
        expected_code = 2 if failure_exit else 0
        open_code = opener.wait(timeout=5)
        info.update(exitCode=actual_code, expectedExitCode=expected_code, openExitCode=open_code)
        (root / "exit-receipt.json").write_text(json.dumps(info, indent=2) + "\n")
        if actual_code != expected_code or open_code != 0:
            print(json.dumps(info), flush=True)
            raise RuntimeError("LaunchServices host exit code differed from expected status")
        deadline = time.monotonic() + 5
        while package.is_alive(sidecar) and time.monotonic() < deadline:
            time.sleep(.05)
        if confirmed_host(host, binary) or package.is_alive(sidecar) or list(temporary.glob("reasonix-tauri-bridge-*/ready.json")):
            raise RuntimeError("LaunchServices normal exit left host/sidecar/readiness")
        if failure_exit:
            failed = json.loads((temporary / "reasonix-native-window-result.json").read_text())
            if failed.get("phase") != "invalid-exit-status-probe" or failed.get("ok") is not False:
                raise RuntimeError("LaunchServices failure exit lacked the deliberate failure marker")
        elif native_phase:
            receipt = json.loads((temporary / "reasonix-native-window-result.json").read_text())
            if receipt.get("phase") != native_phase or receipt.get("ok") is not True:
                raise RuntimeError("LaunchServices native phase lacked its exact successful receipt")
        elif not interactive:
            notification = json.loads((temporary / "reasonix-native-notification-smoke.json").read_text())
            workspace = json.loads((temporary / "reasonix-global-workspace-smoke.json").read_text())
            if notification.get("permission") not in {"not_determined", "denied", "granted", "provisional"} or notification.get("clickSupported") is not True:
                raise RuntimeError("LaunchServices native permission receipt failed")
            if workspace.get("workspaceRoot") != str((core / "global-workspace").resolve()) or workspace.get("sessionId") != "tauri-package-workspace-smoke":
                raise RuntimeError("LaunchServices Global workspace escaped private profile")
        info.update(sidecarRemaining=False, readyRemaining=False)
        (root / "result.json").write_text(json.dumps(info, indent=2) + "\n")
        print(json.dumps(info), flush=True)
        return identity
    finally:
        if monitor is not None:
            monitor.close()
        if not retained:
            if host is not None and confirmed_host(host, binary):
                os.kill(host, signal.SIGTERM)
                deadline = time.monotonic() + 5
                while confirmed_host(host, binary) and time.monotonic() < deadline:
                    time.sleep(.05)
                if confirmed_host(host, binary):
                    os.kill(host, signal.SIGKILL)
            if opener.poll() is None:
                opener.terminate()
                opener.wait(timeout=5)
            for pid in package.own_sidecars(temporary, sidecar_binary):
                os.kill(pid, signal.SIGKILL)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app", type=Path)
    parser.add_argument("--interactive", action="store_true")
    parser.add_argument("--reuse-root", type=Path, help="restart the exact completed explicit private fixture with unchanged credential identity")
    parser.add_argument("--window-state-template", type=Path, help="seed saved normal geometry in a fresh private interactive fixture; require actual native restoration before acceptance")
    parser.add_argument("--failure-exit", action="store_true", help="verify deliberate native probe failure exits 2 after normal cleanup")
    parser.add_argument("--native-phase", choices=NATIVE_PHASES, help="run the unchanged installed Settings acceptance via LaunchServices, keeping kernel exit and profile isolation checks")
    parser.add_argument("--observe", action="store_true", help="with --interactive, observe native state and wait for real Quit (default 300s; native state sampling remains bounded)")
    parser.add_argument("--wait-seconds", type=int, default=300, help="explicit interactive observation wait, 1..900 seconds")
    options = parser.parse_args()
    if options.reuse_root is not None and not (options.interactive and options.observe):
        parser.error("--reuse-root requires --interactive --observe")
    if options.window_state_template is not None and (not options.interactive or not options.observe or options.reuse_root is not None or options.native_phase or options.failure_exit):
        parser.error("--window-state-template requires a fresh --interactive --observe fixture")
    if options.observe and not options.interactive:
        parser.error("--observe requires --interactive")
    if options.failure_exit and options.interactive:
        parser.error("--failure-exit cannot be combined with --interactive")
    if options.native_phase and (options.interactive or options.observe or options.failure_exit or options.reuse_root is not None):
        parser.error("--native-phase requires a fresh noninteractive probe")
    if not 1 <= options.wait_seconds <= 900 or (options.wait_seconds != 300 and not (options.interactive and options.observe)):
        parser.error("a custom --wait-seconds requires --interactive --observe and must be 1..900")
    app = options.app.resolve()
    window_state = read_window_state_template(options.window_state_template) if options.window_state_template else None
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    with (app / "Contents/Info.plist").open("rb") as source:
        identifier = plistlib.load(source)["CFBundleIdentifier"]
    if package.matching_package_is_running(identifier):
        raise SystemExit("another Preview is running; no LaunchServices probe started")
    identities = [launch(app, identifier, managed, options.interactive, options.observe, options.failure_exit, options.wait_seconds, options.reuse_root, options.native_phase, window_state)
                  for managed in ((False,) if options.interactive else (True, False))]
    if len(identities) == 2 and identities[0] == identities[1]:
        raise SystemExit("LaunchServices profiles shared credential identity")
