#!/usr/bin/env python3
"""Current installed clipboard IPC/caller isolation with private display seeds.

Never archive clipboard payloads. NativeClipboardFixture preserves all formats
and rejects a changed generation; the original installed assertions are intact.
"""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import plistlib
import shutil
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


windows = module("permission_windows", "smoke-native-window.py")
placement = module("permission_placement", "probe-launch-services-profile.py")


def smoke(app, state, output):
    identifier = plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleIdentifier"]
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("Preview already running")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    output.mkdir(mode=0o700, parents=False, exist_ok=False)
    binaries = [app / "Contents/MacOS" / name for name in ("reasonix-tauri", "reasonix-desktop-bridge")]
    hashes = {binary.name: hashlib.sha256(binary.read_bytes()).hexdigest() for binary in binaries}
    (output / "artifact.json").write_text(json.dumps({"app": str(app), "sha256": hashes,
        "seededGeometry": state}, indent=2) + "\n")
    for managed in (True, False):
        root = Path(tempfile.mkdtemp(prefix="reasonix-clipboard-permissions-", dir="/private/tmp"))
        root.chmod(0o700)
        for name in ("home", "tmp"):
            (root / name).mkdir(mode=0o700)
        placement.seed_window_state(root / "home/Library/Application Support" / identifier, state)
        passed = False
        profile = "managed" if managed else "explicit"
        receipts = output / profile
        receipts.mkdir(mode=0o700)
        try:
            fixture = windows.clipboard_fixture.NativeClipboardFixture(root / "tmp")
            with fixture as provider:
                windows.launch(*binaries, root, identifier, managed, "clipboard-native", provider,
                               verify_window_state=False, launch_services=True)
            # The fixture removes its payload only after a successful verified
            # restore. Do not copy it, including on a failure.
            if fixture.snapshot.exists():
                raise RuntimeError("clipboard recovery remains incomplete")
            passed = True
        finally:
            for name in ("reasonix-native-window-result.json", "launch-services-clipboard-native-exit.json"):
                source = root / "tmp" / name
                if source.is_file() and not source.is_symlink():
                    shutil.copyfile(source, receipts / name)
            (receipts / "result.json").write_text(json.dumps({"profile": profile, "passed": passed,
                "clipboardOriginalFormatsRestored": passed,
                "nativeWindowPositionObserved": False,
                "scope": "actual clipboard IPC and hidden caller denial, not physical Copy/Paste or selection editing"}, indent=2) + "\n")
            if passed:
                shutil.rmtree(root)
            else:
                print(f"Failed private clipboard fixture retained: {root}", file=sys.stderr, flush=True)
    for binary in binaries:
        if hashlib.sha256(binary.read_bytes()).hexdigest() != hashes[binary.name]:
            raise RuntimeError("installed binary changed")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    (output / "result.json").write_text(json.dumps({"passed": True, "cases": 2}) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app", type=Path)
    parser.add_argument("--window-state-template", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if sys.platform != "darwin":
        parser.error("macOS required")
    smoke(args.app.resolve(), placement.read_window_state_template(args.window_state_template), args.output)
