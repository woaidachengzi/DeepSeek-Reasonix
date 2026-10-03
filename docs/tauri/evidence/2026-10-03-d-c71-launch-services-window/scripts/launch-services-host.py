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


class LaunchServicesHost:
    def __init__(self, binary, sidecar, root, env, package):
        self.binary, self.root, self.package = binary, root, package
        self.pid = None
        self.returncode = None
        self.monitor = None
        self.temporary = root / "tmp"
        self.phase = env["REASONIX_TAURI_NATIVE_WINDOW_SMOKE"]
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
            command.extend(["--env", name + "=" + env.get(name, "")])
        command.extend(["--env", "REASONIX_DESKTOP_BRIDGE_TOKEN=must-be-cleared"])
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
