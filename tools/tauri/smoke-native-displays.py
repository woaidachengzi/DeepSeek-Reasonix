#!/usr/bin/env python3
"""Verify actual secondary-display placement and restart in private profiles.

Requires two connected, non-mirrored displays. Does not prove physical dragging,
display unplugging, or mixed scaling unless the observed displays have it.
"""
import importlib.util
import argparse
import json
from pathlib import Path
import plistlib
import shutil
import sys
import tempfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location(
    "native_windows", Path(__file__).with_name("smoke-native-window.py")
)
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


def seed_straddling_state(root, identifier):
    temporary = root / "tmp"
    normal_path = temporary / "reasonix-native-window-normal.json"
    normal = json.loads(normal_path.read_text())
    trace = json.loads((temporary / "reasonix-native-window-trace.jsonl").read_text())
    monitors = trace["state"]["monitors"]
    secondary = next(area for area in monitors
                     if area["x"] <= normal["x"] < area["x"] + area["width"]
                     and area["y"] <= normal["y"] < area["y"] + area["height"])
    for other in monitors:
        if other == secondary:
            continue
        if secondary["x"] + secondary["width"] == other["x"]:
            saved_x = other["x"] - normal["width"] * 7 // 10
            restored_x = secondary["x"] + secondary["width"] - normal["width"]
        elif other["x"] + other["width"] == secondary["x"]:
            saved_x = secondary["x"] - normal["width"] * 3 // 10
            restored_x = secondary["x"]
        else:
            continue
        y = max(secondary["y"], other["y"]) + 50
        if y + 32 > min(secondary["y"] + secondary["height"], other["y"] + other["height"]):
            continue
        state_path = root / "home/Library/Application Support" / identifier / "window-state.json"
        saved = json.loads(state_path.read_text())
        saved.update(x=saved_x, y=y, maximized=False)
        state_path.write_text(json.dumps(saved))
        normal.update(x=restored_x, y=min(y, secondary["y"] + secondary["height"] - normal["height"]))
        normal_path.write_text(json.dumps(normal))
        print("private straddling geometry: " + json.dumps({"savedX": saved_x, "restoredX": restored_x}), flush=True)
        return
    raise RuntimeError("straddling acceptance requires horizontally adjacent displays with a shared title-bar region")


def smoke(app_path, straddle=False):
    app = Path(app_path).resolve()
    host = app / "Contents/MacOS/reasonix-tauri"
    sidecar = app / "Contents/MacOS/reasonix-desktop-bridge"
    if not host.is_file() or not sidecar.is_file():
        raise RuntimeError("packaged host or sidecar missing")
    with (app / "Contents/Info.plist").open("rb") as stream:
        identifier = plistlib.load(stream)["CFBundleIdentifier"]
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("a Preview is running; no display acceptance performed")
    for managed in (True, False):
        root = Path(tempfile.mkdtemp(prefix="reasonix-native-displays-", dir="/private/tmp"))
        root.chmod(0o700)
        success = False
        try:
            (root / "home").mkdir(mode=0o700)
            (root / "tmp").mkdir(mode=0o700)
            identities = []
            for phase in ("display-secondary", "display-restore-secondary"):
                identities.append(windows.launch(host, sidecar, root, identifier, managed, phase))
                windows.print_window_trace(root / "tmp")
            if straddle:
                seed_straddling_state(root, identifier)
                identities.append(windows.launch(host, sidecar, root, identifier, managed, "display-restore-secondary"))
                windows.print_window_trace(root / "tmp")
                identities.append(windows.launch(host, sidecar, root, identifier, managed, "display-restore-secondary"))
                windows.print_window_trace(root / "tmp")
            if len(set(identities)) != 1:
                raise RuntimeError("display restart changed the private credential identity")
            if list((root / "tmp").glob("reasonix-tauri-bridge-*/launch-owner.json")):
                raise RuntimeError("display acceptance left launch ownership")
            count = len(identities)
            print(f"native macOS secondary display {'managed' if managed else 'explicit'}: {count}/{count} passed", flush=True)
            success = True
        finally:
            if success:
                shutil.rmtree(root)
            else:
                print(f"Private display failure fixture retained: {root}", file=sys.stderr)
    print("secondary display placement/restart passed; physical dragging, unplugging and mixed-scale acceptance remain separate")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    parser.add_argument("--straddle", action="store_true", help="also restore seeded cross-display geometry and restart again")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("this acceptance requires macOS")
    try:
        smoke(args.app, args.straddle)
    except (OSError, ValueError, RuntimeError) as error:
        raise SystemExit(f"native display acceptance failed: {error}") from error
