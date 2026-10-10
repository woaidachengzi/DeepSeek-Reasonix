"""Exact-PID LaunchServices host adapter for isolated installed-app acceptance.

The open process is never treated as the application. Native phases retain
their original assertions and must have a real kernel exit receipt.
"""
import json
import os
from pathlib import Path
import select
import signal
import subprocess
import time

UI_MIGRATION_PHASES = ("before-import", "after-import", "after-undo")


def private_legacy_ui_environment(ordinary_ui_phase, env):
    """Only the fixed private-source fixture may enter an ordinary UI launch."""
    name = "REASONIX_TAURI_LEGACY_UI_SMOKE"
    if ordinary_ui_phase is None:
        if name in env:
            raise ValueError("legacy UI fixture requires an explicit ordinary migration phase")
        return {}
    if ordinary_ui_phase not in UI_MIGRATION_PHASES or env.get(name) != "private-source":
        raise ValueError("ordinary migration requires a fixed phase and private legacy source")
    return {name: "private-source"}


def owned_terminal_shell_environment(phase, root, env):
    if phase != "ui-integrated-terminal":
        return {}
    expected = {"SHELL": "/bin/sh", "ENV": "/dev/null", "BASH_ENV": "/dev/null",
                "ZDOTDIR": str(root / "home")}
    if any(env.get(name) != value for name, value in expected.items()):
        raise ValueError("integrated terminal startup environment is not runner-owned")
    return expected


def launch_phase(env, ordinary_ui_phase=None, ordinary_bot_diagnostics=False):
    if type(ordinary_bot_diagnostics) is not bool:
        raise ValueError("ordinary bot diagnostics selector must be boolean")
    if ordinary_bot_diagnostics:
        if ordinary_ui_phase is not None or any(name in env for name in (
                "REASONIX_TAURI_NATIVE_WINDOW_SMOKE", "REASONIX_TAURI_PACKAGE_SMOKE",
                "REASONIX_TAURI_PROFILE_SMOKE", "REASONIX_TAURI_LEGACY_UI_SMOKE")):
            raise ValueError("ordinary bot diagnostics cannot mix automated smoke phases")
        return "ui-bot-diagnostics"
    return ("ui-migration-" + ordinary_ui_phase if ordinary_ui_phase is not None
            else env["REASONIX_TAURI_NATIVE_WINDOW_SMOKE"])


class LaunchServicesHost:
    def __init__(self, binary, sidecar, root, env, package, ordinary_ui_phase=None,
                 ordinary_bot_diagnostics=False):
        legacy_environment = private_legacy_ui_environment(ordinary_ui_phase, env)
        phase = launch_phase(env, ordinary_ui_phase, ordinary_bot_diagnostics)
        self.binary, self.root, self.package = binary, root, package
        self.pid = None
        self.returncode = None
        self.monitor = None
        self.temporary = root / "tmp"
        self.phase = phase
        # Never forward ambient provider credentials, proxies or caller secrets
        # into LaunchServices. These values are owned acceptance inputs only.
        names = ("HOME", "TMPDIR", "REASONIX_HOME", "REASONIX_STATE_HOME",
                 "REASONIX_CACHE_HOME", "REASONIX_PREVIEW_SQLITE_EVENTS",
                 "REASONIX_TAURI_PACKAGE_SMOKE", "REASONIX_TAURI_PROFILE_SMOKE",
                 "REASONIX_TAURI_NATIVE_WINDOW_SMOKE")
        command = ["/usr/bin/open", "-n", "-W", str(binary.parents[2]),
                   "--stdout", str(root / "launch-services-host.log"),
                   "--stderr", str(root / "launch-services-host.log")]
        for name in names:
            # An absent explicit-profile state override must stay absent:
            # empty strings still count as inherited environment overrides.
            # Managed HOME is deliberately empty to select Preview isolation.
            if name not in env and name != "REASONIX_HOME":
                continue
            command.extend(["--env", name + "=" + env.get(name, "")])
        command.extend(["--env", "REASONIX_DESKTOP_BRIDGE_TOKEN=must-be-cleared"])
        for name, value in legacy_environment.items():
            command.extend(["--env", name + "=" + value])
        for name, value in owned_terminal_shell_environment(self.phase, root, env).items():
            command.extend(["--env", name + "=" + value])
        self.opener = subprocess.Popen(command, stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            start_new_session=True, env={key: env[key] for key in
            ("PATH", "LANG", "LC_ALL", "USER", "LOGNAME") if key in env})
        try:
            deadline = time.monotonic() + 25
            while time.monotonic() < deadline:
                rows = list(package.processes())
                parents = [parent for _, parent, text in rows
                    if text.startswith(str(sidecar) + " ")
                    and "--ready-file " + str(self.temporary) + "/" in text]
                if len(parents) == 1 and any(pid == parents[0] and text == str(binary)
                                            for pid, _, text in rows):
                    self.pid = parents[0]
                    self.monitor = select.kqueue()
                    self.monitor.control([select.kevent(self.pid, filter=select.KQ_FILTER_PROC,
                        flags=select.KQ_EV_ADD | select.KQ_EV_ONESHOT,
                        fflags=select.KQ_NOTE_EXIT | 0x04000000)], 0, 0)
                    return
                if self.opener.poll() is not None:
                    raise RuntimeError("LaunchServices exited before exact native host identification")
                time.sleep(.05)
            raise RuntimeError("LaunchServices did not start one private native host")
        except BaseException:
            self.close()
            raise

    def poll(self):
        if self.returncode is None and self.monitor is not None:
            events = self.monitor.control([], 1, 0)
            if events:
                event = events[0]
                if event.ident != self.pid or not event.fflags & select.KQ_NOTE_EXIT:
                    raise RuntimeError("native host kernel receipt differs from exact PID")
                status = event.data
                if os.WIFEXITED(status):
                    self.returncode = os.WEXITSTATUS(status)
                elif os.WIFSIGNALED(status):
                    self.returncode = -os.WTERMSIG(status)
                else:
                    raise RuntimeError("native host kernel status is not terminal")
                receipt = {"phase": self.phase, "pid": self.pid, "rawKernelStatus": status,
                           "exitCode": self.returncode, "signaled": os.WIFSIGNALED(status)}
                (self.temporary / ("launch-services-" + self.phase + "-exit.json")).write_text(json.dumps(receipt) + "\n")
                print("LaunchServices native kernel exit: " + json.dumps(receipt), flush=True)
        return self.returncode

    def wait(self, timeout):
        deadline = time.monotonic() + timeout
        while self.poll() is None:
            if time.monotonic() >= deadline:
                raise subprocess.TimeoutExpired(str(self.binary), timeout)
            time.sleep(.05)
        if self.opener.wait(timeout=max(.1, deadline - time.monotonic())) != 0:
            raise RuntimeError("LaunchServices opener failed after native host exit")
        return self.returncode

    def kill(self):
        if self.pid is not None and any(pid == self.pid and text == str(self.binary)
                                       for pid, _, text in self.package.processes()):
            try:
                os.kill(self.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass

    def close(self):
        if self.monitor is not None:
            self.monitor.close()
            self.monitor = None
        if self.opener.poll() is None:
            self.opener.terminate()
            self.opener.wait(timeout=5)
