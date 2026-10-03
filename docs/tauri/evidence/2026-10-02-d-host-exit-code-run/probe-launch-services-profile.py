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


def launch(app, identifier, managed, interactive=False, observe=False, failure_exit=False):
    root = Path(tempfile.mkdtemp(prefix="reasonix-launch-services-", dir="/private/tmp"))
    root.chmod(0o700)
    home, temporary = root / "home", root / "tmp"
    home.mkdir(mode=0o700)
    temporary.mkdir(mode=0o700)
    app_data = home / "Library/Application Support" / identifier
    core = app_data / "reasonix-core" if managed else root / "core"
    binary = app / "Contents/MacOS/reasonix-tauri"
    sidecar_binary = app / "Contents/MacOS/reasonix-desktop-bridge"
    # Pass only intended, nonsecret profile values. The opener itself has a
    # minimal environment; do not manufacture empty state/event overrides in
    # an explicit profile that intentionally preserves caller overrides.
    env = {"HOME": str(home), "TMPDIR": str(temporary),
           "REASONIX_HOME": "" if managed else str(core),
           "REASONIX_TAURI_PACKAGE_SMOKE": "" if interactive or failure_exit else "1"}
    if not managed:
        env["REASONIX_CACHE_HOME"] = str(root / "cache")
    if observe:
        env["REASONIX_TAURI_NATIVE_WINDOW_SMOKE"] = "interactive-observe"
    if failure_exit:
        env["REASONIX_TAURI_NATIVE_WINDOW_SMOKE"] = "invalid-exit-status-probe"
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
        if interactive:
            workspace = root / "workspace"
            workspace.mkdir(mode=0o700)
            (workspace / "canary.txt").write_text("Private native dialog acceptance canary\n")
            retained = not observe
            (root / "launch.json").write_text(json.dumps(info, indent=2) + "\n")
            print(json.dumps(info), flush=True)
            if not observe:
                return identity
        # Keep the kernel receipt attached to this exact PID during real UI
        # actions. Never inspect/reopen the dead CUA binding after user Quit.
        exits = monitor.control([], 1, 300 if observe else 25)
        if len(exits) != 1 or exits[0].ident != host or not exits[0].fflags & select.KQ_NOTE_EXIT:
            raise RuntimeError("LaunchServices host did not publish an exit receipt")
        status = exits[0].data
        if not os.WIFEXITED(status):
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
    parser.add_argument("--failure-exit", action="store_true", help="verify deliberate native probe failure exits 2 after normal cleanup")
    parser.add_argument("--observe", action="store_true", help="with --interactive, observe native state and wait for real Quit (300s)")
    options = parser.parse_args()
    if options.observe and not options.interactive:
        parser.error("--observe requires --interactive")
    if options.failure_exit and options.interactive:
        parser.error("--failure-exit cannot be combined with --interactive")
    app = options.app.resolve()
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    with (app / "Contents/Info.plist").open("rb") as source:
        identifier = plistlib.load(source)["CFBundleIdentifier"]
    if package.matching_package_is_running(identifier):
        raise SystemExit("another Preview is running; no LaunchServices probe started")
    identities = [launch(app, identifier, managed, options.interactive, options.observe, options.failure_exit)
                  for managed in ((False,) if options.interactive else (True, False))]
    if len(identities) == 2 and identities[0] == identities[1]:
        raise SystemExit("LaunchServices profiles shared credential identity")
