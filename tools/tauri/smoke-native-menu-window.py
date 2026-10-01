#!/usr/bin/env python3
"""Installed macOS Minimize responder role and Settings restoration.

Requires the real active main key window, validates the installed selector,
and checks actual AppKit Will/Did Miniaturize and Did Deminiaturize events.
Uses fresh private profiles. Does not replace Tauri window API/geometry gates,
or prove physical menu clicks/keys, tray/Dock interactions or display unplugging.
"""
import argparse
import importlib.util
import os
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("native_windows", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


def smoke(app_path):
    app = Path(app_path).resolve()
    host, sidecar = (app / "Contents/MacOS" / name for name in ("reasonix-tauri", "reasonix-desktop-bridge"))
    if not host.is_file() or not sidecar.is_file():
        raise RuntimeError("packaged Preview host/sidecar missing")
    with (app / "Contents/Info.plist").open("rb") as file:
        identifier = plistlib.load(file).get("CFBundleIdentifier")
    if not isinstance(identifier, str) or not identifier:
        raise RuntimeError("bundle identifier missing")
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("a Preview is already running; no menu acceptance started")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    environment = {key: value for key, value in os.environ.items()
                   if key in ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL", "__CF_USER_TEXT_ENCODING")}
    for managed in (True, False):
        with tempfile.TemporaryDirectory(prefix="reasonix-native-menu-window-", dir="/private/tmp") as directory:
            root = Path(directory)
            root.chmod(0o700)
            for name in ("home", "tmp"):
                (root / name).mkdir(mode=0o700)
            identity = windows.launch(host, sidecar, root, identifier, managed,
                                      "menu-settings-native-minimized", verify_window_state=False,
                                      environment=environment)
            windows.print_window_trace(root / "tmp")
            restarted = windows.launch(host, sidecar, root, identifier, managed,
                                       "menu-shortcuts", verify_window_state=False, environment=environment)
            if restarted != identity or list((root / "tmp").glob("reasonix-tauri-bridge-*")):
                raise RuntimeError("native menu gate left readiness or changed the profile identity")
    print("Installed Minimize role, actual AppKit lifecycle events, Settings restoration and restart: OK")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("this gate requires macOS")
    try:
        smoke(args.app)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"native macOS menu window gate failed: {error}") from error
