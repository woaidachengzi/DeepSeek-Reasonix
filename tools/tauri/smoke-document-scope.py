#!/usr/bin/env python3
"""Actual macOS Preview document caller scope and executable rejection IPC.

Main WKWebView rejects executable documents and symlink aliases, and resolves
the system application catalog. A same-origin hidden WKWebView must be denied
all eight document/workspace application commands even with a forged window
argument. No application launch, dialog selection or physical click is accepted.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import secrets
import shlex
import shutil
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location(
    "native_windows", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


def snapshot(paths):
    return [(os.readlink(path) if path.is_symlink() else path.read_bytes(),
             path.lstat().st_mode, path.lstat().st_mtime_ns) for path in paths]


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
        root = Path(tempfile.mkdtemp(prefix="reasonix-document-scope-", dir="/private/tmp"))
        root.chmod(0o700)
        (root / "home").mkdir(mode=0o700)
        temporary = root / "tmp"
        temporary.mkdir(mode=0o700)
        document = temporary / 'document 中文\n"$.md'
        document.write_text("Reasonix native caller scope canary\n")
        document.chmod(0o600)
        executable = temporary / "executable-canary.sh"
        execution = temporary / "must-not-be-executed.receipt"
        executable.write_text("#!/bin/sh\n: > " + shlex.quote(str(execution)) + "\n")
        executable.chmod(0o700)
        alias = temporary / "alias.md"
        alias.symlink_to(executable)
        originals = [document, executable, alias]
        before = snapshot(originals)
        control = temporary / "reasonix-native-link-control.json"
        control.write_text(json.dumps({"nonce": secrets.token_hex(16)}))
        control.chmod(0o600)
        success = False
        try:
            windows.launch(host, sidecar, root, identifier, managed, "document-scope",
                           verify_window_state=False,
                           environment={key: os.environ[key] for key in
                                        ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL",
                                         "__CF_USER_TEXT_ENCODING") if key in os.environ})
            if snapshot(originals) != before or execution.exists():
                raise RuntimeError("forbidden document IPC modified originals or ran the executable")
            success = True
        finally:
            if success:
                shutil.rmtree(root)
            else:
                print(f"Private failure fixture retained: {root}", file=sys.stderr)
    print("Main executable/alias rejection, eight document plus two bridge caller denials, originals and cleanup: OK")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("this acceptance requires macOS")
    try:
        smoke(args.app)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"native macOS document scope smoke failed: {error}") from error
