#!/usr/bin/env python3
"""Actual macOS Preview main IPC -> system Terminal -> real shell cwd.

Opens a private directory and a file in it. Requires two new Terminal shells
with the exact directory as kernel cwd. Stops only those identified test shells;
completed test windows may be closed manually. No Apple Events or screenshot.
Does not accept other terminal/editor apps or physical opener-menu clicks.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import secrets
import shutil
import signal
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location(
    "native_windows", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


def processes():
    result = subprocess.run(["ps", "-wwaxo", "pid=,ppid=,lstart=,comm="],
                            capture_output=True, text=True, check=True, timeout=3)
    rows = {}
    for line in result.stdout.splitlines():
        fields = line.split(None, 7)
        if len(fields) == 8 and fields[0].isdigit() and fields[1].isdigit():
            rows[int(fields[0])] = (int(fields[1]), tuple(fields[2:7]), fields[7])
    return rows


def shell_candidates(rows, baseline):
    terminals = {pid for pid, row in rows.items()
                 if row[2] == "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal"}
    selected = {}
    for pid, row in rows.items():
        if pid in baseline or Path(row[2]).name.lstrip("-") not in ("zsh", "bash", "sh", "fish"):
            continue
        parent, visited = row[0], {pid}
        while parent in rows and parent not in visited:
            if parent in terminals:
                selected[pid] = row
                break
            # Startup/configuration commands can temporarily spawn another
            # shell on the same tty/cwd. Count the Terminal session's root
            # shell, not every descendant shell in that session.
            if Path(rows[parent][2]).name.lstrip("-") in ("zsh", "bash", "sh", "fish"):
                break
            visited.add(parent)
            parent = rows[parent][0]
    if len(selected) > 64:
        raise RuntimeError("too many new Terminal shells to inspect safely")
    return selected


def build_cwd_helper(directory):
    helper = directory / "native-terminal-cwd"
    subprocess.run(["/usr/bin/clang", "-Wall", "-Wextra", "-Werror", "-O2",
                    str(Path(__file__).with_name("native-terminal-cwd.c")), "-o", str(helper)],
                   capture_output=True, check=True, timeout=30)
    helper.chmod(0o700)
    return helper


def working_directories(pids, helper):
    if not pids:
        return {}
    result = subprocess.run([str(helper), *map(str, pids)], capture_output=True, timeout=3)
    if result.returncode != 0 or not result.stdout.endswith(b"\0") and result.stdout:
        raise RuntimeError("cannot query Terminal shell kernel cwd")
    fields, paths = result.stdout.split(b"\0")[:-1], {}
    if len(fields) % 2 or len(fields) > 128:
        raise RuntimeError("invalid bounded kernel cwd fields")
    for index in range(0, len(fields), 2):
        if not fields[index].isdigit() or int(fields[index]) not in pids:
            raise RuntimeError("unexpected kernel cwd PID")
        paths[int(fields[index])] = os.fsdecode(fields[index + 1])
    return paths


class TerminalReceipts:
    def __init__(self, temporary):
        self.temporary = temporary
        self.workspace = temporary / 'workspace 中文\n"$'
        self.workspace.mkdir(mode=0o700)
        self.document = self.workspace / "document.md"
        self.document.write_text("Reasonix native Terminal cwd canary\n")
        self.document.chmod(0o600)
        self.before = self.document.stat()
        self.baseline = set(processes())
        self.helper = build_cwd_helper(temporary)
        self.owned = {}
        self.nonce = secrets.token_hex(16)
        control = temporary / "reasonix-native-link-control.json"
        control.write_text(json.dumps({"nonce": self.nonce, "baselinePids": sorted(self.baseline)}))
        control.chmod(0o600)

    def current(self):
        candidates = shell_candidates(processes(), self.baseline)
        paths = working_directories(candidates, self.helper)
        return {pid: row for pid, row in candidates.items()
                if paths.get(pid) == str(self.workspace)}

    def pump(self, _inspect):
        # Startup/normal exit checks belong to launch(); do not introduce a
        # task-style live check racing the native acceptance's normal exit.
        current = self.current()
        if len(current) > 2:
            raise RuntimeError("unexpected additional shell in the private test workspace")
        for pid, row in current.items():
            if pid in self.owned and self.owned[pid] != row:
                raise RuntimeError("observed test shell identity changed")
            self.owned.setdefault(pid, row)
        ownership = self.temporary / "reasonix-native-terminal-owned.json"
        ownership.write_text(json.dumps(self.owned))
        ownership.chmod(0o600)
        if len(current) == 2:
            receipt = self.temporary / "reasonix-native-terminal.receipt"
            receipt.write_text(self.nonce)
            receipt.chmod(0o600)

    def verify(self):
        if len(self.owned) != 2 or self.current() != self.owned:
            raise RuntimeError("two original Terminal test shells did not retain the exact kernel cwd")
        after = self.document.stat()
        if self.document.read_text() != "Reasonix native Terminal cwd canary\n" or (
                after.st_mode, after.st_mtime_ns) != (self.before.st_mode, self.before.st_mtime_ns):
            raise RuntimeError("opening Terminal changed the original document")

    def __enter__(self):
        return self

    def __exit__(self, *_):
        # Recheck parent chain, birth time, executable and private cwd before
        # signaling. A reused PID or user-moved shell must never be stopped.
        current = self.current()
        for pid, original in self.owned.items():
            if pid not in processes():
                continue
            if current.get(pid) != original:
                raise RuntimeError("test shell identity/cwd changed; private fixture retained")
            try:
                os.kill(pid, signal.SIGHUP)
            except ProcessLookupError:
                pass
        deadline = time.monotonic() + 5
        while any(pid in processes() for pid in self.owned):
            if time.monotonic() >= deadline:
                raise RuntimeError("owned Terminal test shell did not exit; fixture retained")
            time.sleep(0.05)


def smoke(app_path):
    app = Path(app_path).resolve()
    host = app / "Contents/MacOS/reasonix-tauri"
    sidecar = app / "Contents/MacOS/reasonix-desktop-bridge"
    if not host.is_file() or not sidecar.is_file():
        raise RuntimeError("packaged host/sidecar missing")
    with (app / "Contents/Info.plist").open("rb") as file:
        identifier = plistlib.load(file).get("CFBundleIdentifier")
    if not isinstance(identifier, str) or not identifier:
        raise RuntimeError("bundle identifier missing")
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("a Preview with this identifier is running; close it first")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    for managed in (True, False):
        root = Path(tempfile.mkdtemp(prefix="reasonix-native-terminal-", dir="/private/tmp"))
        root.chmod(0o700)
        (root / "home").mkdir(mode=0o700)
        (root / "tmp").mkdir(mode=0o700)
        success = False
        try:
            with TerminalReceipts(root / "tmp") as receipts:
                windows.launch(host, sidecar, root, identifier, managed, "external-terminal", receipts,
                               verify_window_state=False,
                               environment={key: os.environ[key] for key in
                                            ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL",
                                             "__CF_USER_TEXT_ENCODING") if key in os.environ})
            success = True
        finally:
            if success:
                shutil.rmtree(root)
            else:
                print(f"Private failure fixture retained: {root}", file=sys.stderr)
    print("Main WKWebView directory/file opens, real Terminal shell cwd, originals and cleanup: OK")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("this acceptance requires macOS")
    try:
        smoke(args.app)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"native macOS Terminal smoke failed: {error}") from error
