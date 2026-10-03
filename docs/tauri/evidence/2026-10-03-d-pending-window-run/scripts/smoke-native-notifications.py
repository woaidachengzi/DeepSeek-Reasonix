#!/usr/bin/env python3
"""Real macOS notification submission, delivered captions and exact cleanup.

Uses existing OS authorization, fresh private profiles and the production host
command. Checks three fixed captions; the last two are sent while the actual
application is hidden. Removes only this invocation's newly generated tokens.
Does not prove visible banners, actual notification clicks or denial settings.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import shutil
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
        raise RuntimeError("package identifier missing")
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("a Preview is already running; no notification test started")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    environment = {key: value for key, value in os.environ.items()
                   if key in ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL", "__CF_USER_TEXT_ENCODING")}
    for managed in (True, False):
        root = Path(tempfile.mkdtemp(prefix="reasonix-native-notifications-", dir="/private/tmp"))
        root.chmod(0o700)
        for name in ("home", "tmp"):
            (root / name).mkdir(mode=0o700)
        core = root / "home/Library/Application Support" / identifier / "reasonix-core" if managed else root / "core"
        core.mkdir(parents=True, mode=0o700)
        original = core / "notification-original.canary"
        original.write_bytes(b"private original\n")
        original.chmod(0o600)
        preserved = (original.read_bytes(), original.stat().st_mode, original.stat().st_mtime_ns)
        success = False
        try:
            identity = windows.launch(host, sidecar, root, identifier, managed,
                                      "notification-delivery", verify_window_state=False,
                                      environment=environment)
            path = root / "tmp/reasonix-native-notification-delivery.json"
            if path.stat().st_size > 4096 or path.stat().st_mode & 0o777 != 0o600:
                raise RuntimeError("native notification receipts exceed private bounds")
            record = json.loads(path.read_text())
            receipts = record.get("receipts")
            kinds = ["turn_done", "approval_request", "ask_request"]
            if record.get("permission") not in ("granted", "provisional") or not isinstance(receipts, list) or len(receipts) != 3:
                raise RuntimeError("native notification receipts incomplete")
            for index, item in enumerate(receipts):
                if not isinstance(item, dict) or item.get("kind") != kinds[index] or item.get("delivered") is not True or item.get("removed") is not True:
                    raise RuntimeError("native notification delivery/cleanup not confirmed")
                if type(item.get("applicationActive")) is not bool or type(item.get("applicationHidden")) is not bool:
                    raise RuntimeError("native notification application state missing")
                if index > 0 and (item["applicationHidden"] is not True or item["applicationActive"] is not False):
                    raise RuntimeError("native notification background precondition not confirmed")
            restarted = windows.launch(host, sidecar, root, identifier, managed,
                                       "menu-shortcuts", verify_window_state=False, environment=environment)
            if restarted != identity or preserved != (original.read_bytes(), original.stat().st_mode, original.stat().st_mtime_ns):
                raise RuntimeError("native notification gate changed originals or profile identity")
            if list((root / "tmp").glob("reasonix-tauri-bridge-*")):
                raise RuntimeError("native notification restart left readiness directories")
            # Preserve only validated native receipts before the successful
            # private fixture is removed. Never emit notification tokens,
            # session routes, profile credentials or arbitrary extra fields.
            print(json.dumps({
                "profile": "managed" if managed else "explicit",
                "permission": record["permission"],
                "receipts": [{key: item[key] for key in
                              ("kind", "applicationActive", "applicationHidden", "delivered", "removed")}
                             for item in receipts],
                "originalPreserved": True,
                "sameProfileIdentityAfterRestart": True,
                "readinessRemaining": False,
            }, sort_keys=True), flush=True)
            print(f"native notifications {'managed' if managed else 'explicit'}: three OS-delivered captions, two hidden sends, exact cleanup, originals and same-profile restart OK; initial application active={receipts[0]['applicationActive']}", flush=True)
            success = True
        finally:
            if success:
                shutil.rmtree(root)
            else:
                print(f"Private notification failure fixture retained: {root}", file=sys.stderr)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("this gate requires macOS")
    try:
        smoke(args.app)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"native macOS notification gate failed: {error}") from error
