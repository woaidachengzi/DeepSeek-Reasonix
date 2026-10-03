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


def smoke(app_path, api_startups=0):
    if not 0 <= api_startups <= 5:
        raise ValueError("API startup sample count must be between zero and five")
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
            # Independent samples, not retries: any failed startup fails the
            # gate immediately. Keep the original window API predicate intact.
            phases = (["menu-settings-minimized"] * api_startups
                      + ["menu-settings-native-minimized", "menu-shortcuts"])
            identity = None
            for phase in phases:
                current = windows.launch(host, sidecar, root, identifier, managed,
                                         phase, verify_window_state=False, environment=environment)
                if identity is None:
                    identity = current
                elif current != identity:
                    raise RuntimeError("native menu/startup gate changed the profile identity")
                if phase != "menu-shortcuts":
                    windows.print_window_trace(root / "tmp")
                if list((root / "tmp").glob("reasonix-tauri-bridge-*")):
                    raise RuntimeError("native menu/startup gate left readiness directories")
    print("Installed Minimize role, actual AppKit lifecycle events, Settings restoration and restart: OK")
    if api_startups:
        print(f"Original Tauri API startup samples: {api_startups * 2} passed; no failed sample was retried")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    parser.add_argument("--api-startups", type=int, choices=range(6), default=0,
                        help="also run this many independent original-API startups per profile (no retries)")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("this gate requires macOS")
    try:
        smoke(args.app, api_startups=args.api_startups)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"native macOS menu window gate failed: {error}") from error
