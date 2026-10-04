#!/usr/bin/env python3
"""Conditional native regression: retain saved geometry with zero active CG displays.

Requires that actual OS condition; never simulates unplugging or changes displays.
The existing menu-contract phase does not move/resize windows. Both profiles use
owned private state. This does not verify restoration after displays reactivate.
"""
import argparse
import ctypes
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("windows", Path(__file__).with_name("smoke-native-window.py"))
w = importlib.util.module_from_spec(spec)
spec.loader.exec_module(w)


def require_inactive_displays():
    framework = ctypes.CDLL("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
    query = framework.CGGetActiveDisplayList
    query.argtypes = [ctypes.c_uint32, ctypes.POINTER(ctypes.c_uint32), ctypes.POINTER(ctypes.c_uint32)]
    query.restype = ctypes.c_int32
    ids = (ctypes.c_uint32 * 8)()
    count = ctypes.c_uint32()
    status = query(8, ids, ctypes.byref(count))
    if status != 0 or count.value != 0:
        raise RuntimeError("zero active CoreGraphics displays required; conditional gate is not applicable")


def smoke(app):
    require_inactive_displays()
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    identifier = plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleIdentifier"]
    if w.package.matching_package_is_running(identifier):
        raise RuntimeError("another Preview is running; do not operate it")
    host = app / "Contents/MacOS/reasonix-tauri"
    sidecar = app / "Contents/MacOS/reasonix-desktop-bridge"
    environment = {key: os.environ[key] for key in
                   ("PATH", "LANG", "LC_ALL", "USER", "LOGNAME", "SHELL", "__CF_USER_TEXT_ENCODING")
                   if key in os.environ}
    seed = {"width": 2000, "height": 1400, "maximized": False,
            "x": -3400, "y": 100, "scale_factor": 2.0}
    for managed in (True, False):
        root = Path(tempfile.mkdtemp(prefix="reasonix-inactive-display-state-", dir="/private/tmp"))
        root.chmod(0o700)
        (root / "home").mkdir(mode=0o700)
        (root / "tmp").mkdir(mode=0o700)
        state = root / "home/Library/Application Support" / identifier / "window-state.json"
        state.parent.mkdir(parents=True, mode=0o700)
        state.write_text(json.dumps(seed) + "\n")
        result = {"profile": "managed" if managed else "explicit", "root": str(root), "seed": seed}
        print("Private inactive-display fixture: " + str(root), flush=True)
        try:
            require_inactive_displays()
            w.launch(host, sidecar, root, identifier, managed, "menu-shortcuts",
                     verify_window_state=False, environment=environment)
            require_inactive_displays()
            result["saved"] = json.loads(state.read_text())
            result["remainingSidecars"] = w.package.own_sidecars(root / "tmp", sidecar)
            result["remainingReadiness"] = len(list((root / "tmp").glob("reasonix-tauri-bridge-*/ready.json")))
            if result["saved"] != seed or result["remainingSidecars"] or result["remainingReadiness"]:
                raise RuntimeError("saved geometry changed or native fixture left children/readiness")
            result["passed"] = True
            print(result["profile"] + ": original saved geometry retained; native exit/cleanup OK", flush=True)
        finally:
            (root / "result.json").write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app", type=Path)
    args = parser.parse_args()
    smoke(args.app.resolve())
