#!/usr/bin/env python3
"""Actual macOS Preview main IPC must reject a broken private Ghostty bundle.

Uses the production catalog, local document validation and LaunchServices handoff.
Never substitutes or launches an installed user's Ghostty. Does not accept normal
Finder/editor/terminal operations, working directories, or physical UI clicks.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import secrets
import shutil
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location(
    "native_windows", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


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
        root = Path(tempfile.mkdtemp(prefix="reasonix-native-app-failure-", dir="/private/tmp"))
        root.chmod(0o700)
        home, temporary = root / "home", root / "tmp"
        home.mkdir(mode=0o700)
        temporary.mkdir(mode=0o700)
        broken = home / "Applications/Ghostty.app/Contents"
        (broken / "MacOS").mkdir(parents=True, mode=0o700)
        # A valid declared APPL bundle with a deliberately absent executable:
        # catalog discovery succeeds, actual LaunchServices must refuse it.
        (broken / "Info.plist").write_bytes(plistlib.dumps({
            "CFBundleIdentifier": "io.reasonix.test.invalid-ghostty." + secrets.token_hex(8),
            "CFBundleName": "Reasonix private invalid application",
            "CFBundleExecutable": "must-not-exist",
            "CFBundlePackageType": "APPL",
            "CFBundleVersion": "1",
        }))
        document = temporary / 'document 中文\n"$.md'
        document.write_text("Reasonix private read-only document canary\n")
        document.chmod(0o600)
        before = document.stat()
        control = temporary / "reasonix-native-link-control.json"
        control.write_text(json.dumps({"nonce": secrets.token_hex(16)}))
        control.chmod(0o600)
        success = False
        try:
            windows.launch(host, sidecar, root, identifier, managed, "external-app-failure",
                           verify_window_state=False,
                           environment={key: os.environ[key] for key in
                                        ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL",
                                         "__CF_USER_TEXT_ENCODING") if key in os.environ})
            if document.read_text() != "Reasonix private read-only document canary\n" or (
                    document.stat().st_mode, document.stat().st_mtime_ns) != (
                        before.st_mode, before.st_mtime_ns):
                raise RuntimeError("failed native open changed the original document")
            success = True
        finally:
            if success:
                shutil.rmtree(root)
            else:
                print(f"Private failure fixture retained: {root}", file=sys.stderr)
    print("Main WKWebView missing/broken application errors, original protection and cleanup: OK")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("this acceptance requires macOS")
    try:
        smoke(args.app)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"native macOS application failure smoke failed: {error}") from error
