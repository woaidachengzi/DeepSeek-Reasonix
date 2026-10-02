#!/usr/bin/env python3
"""Verify actual secondary-display placement and restart in private profiles.

Requires two connected, non-mirrored displays. Does not prove physical dragging,
display unplugging, or mixed scaling unless the observed displays have it.
"""
import importlib.util
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


def smoke(app_path):
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
            if len(set(identities)) != 1:
                raise RuntimeError("display restart changed the private credential identity")
            if list((root / "tmp").glob("reasonix-tauri-bridge-*/launch-owner.json")):
                raise RuntimeError("display acceptance left launch ownership")
            print(f"native macOS secondary display {'managed' if managed else 'explicit'}: 2/2 passed", flush=True)
            success = True
        finally:
            if success:
                shutil.rmtree(root)
            else:
                print(f"Private display failure fixture retained: {root}", file=sys.stderr)
    print("secondary display placement/restart passed; physical dragging, unplugging and mixed-scale acceptance remain separate")


if __name__ == "__main__":
    if sys.platform != "darwin" or len(sys.argv) != 2:
        raise SystemExit("usage: smoke-native-displays.py MACOS_PREVIEW_APP")
    try:
        smoke(sys.argv[1])
    except (OSError, ValueError, RuntimeError) as error:
        raise SystemExit(f"native display acceptance failed: {error}") from error
