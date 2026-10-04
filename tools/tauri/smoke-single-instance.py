#!/usr/bin/env python3
"""Run the existing real second-instance gate at requested private window bounds.

The seeded normal file is an expected reference, not evidence of an exercise.
The installed gate still verifies native geometry/focus, background close,
second LaunchServices exit, original sidecar identity and final cleanup.
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


windows = module("single_instance_windows", "smoke-native-window.py")
placement = module("single_instance_placement", "probe-launch-services-profile.py")


def smoke(app, state, output):
    identifier = plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleIdentifier"]
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("Preview already running")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    output.mkdir(mode=0o700, parents=False, exist_ok=False)
    binaries = [app / "Contents/MacOS" / name for name in ("reasonix-tauri", "reasonix-desktop-bridge")]
    (output / "artifact.json").write_text(json.dumps({"app": str(app), "sha256": {
        binary.name: hashlib.sha256(binary.read_bytes()).hexdigest() for binary in binaries},
        "expectedGeometry": placement.expected_window_geometry(state)}, indent=2) + "\n")
    identities = []
    for managed in (True, False):
        root = Path(tempfile.mkdtemp(prefix="reasonix-single-instance-", dir="/private/tmp"))
        root.chmod(0o700)
        for name in ("home", "tmp"):
            (root / name).mkdir(mode=0o700)
        app_data = root / "home/Library/Application Support" / identifier
        placement.seed_window_state(app_data, state)
        expected = root / "tmp/reasonix-native-window-normal.json"
        with expected.open("x") as stream:
            expected.chmod(0o600)
            stream.write(json.dumps(placement.expected_window_geometry(state)) + "\n")
        passed = False
        profile = "managed" if managed else "explicit"
        receipts = output / profile
        receipts.mkdir(mode=0o700)
        try:
            identities.append(windows.launch(*binaries, root, identifier, managed,
                                             "second-instance", launch_services=True))
            saved = placement.read_window_state_template(app_data / "window-state.json")
            if saved != state:
                raise RuntimeError("single-instance restore changed requested normal state")
            passed = True
        finally:
            # Fixed receipts only; no ready files, credential identity or HOME.
            for name in ("reasonix-native-window-result.json", "launch-services-second-instance-exit.json"):
                source = root / "tmp" / name
                if source.is_file() and not source.is_symlink():
                    shutil.copyfile(source, receipts / name)
            (receipts / "result.json").write_text(json.dumps({"profile": profile, "passed": passed,
                "expectedGeometry": placement.expected_window_geometry(state),
                "scope": "programmatic background close, real second LaunchServices start, native focus/geometry and cleanup"}, indent=2) + "\n")
            if passed:
                shutil.rmtree(root)
            else:
                print(f"Failed single-instance private fixture retained: {root}", file=sys.stderr, flush=True)
    if len(identities) != 2 or identities[0] == identities[1]:
        raise RuntimeError("managed and explicit profile identities collided")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    for binary in binaries:
        if hashlib.sha256(binary.read_bytes()).hexdigest() != json.loads((output / "artifact.json").read_text())["sha256"][binary.name]:
            raise RuntimeError("installed binary changed during single-instance acceptance")
    (output / "result.json").write_text(json.dumps({"passed": True, "cases": 2,
        "distinctCredentialProfiles": True}) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app", type=Path)
    parser.add_argument("--window-state-template", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if sys.platform != "darwin":
        parser.error("macOS required")
    smoke(args.app.resolve(), placement.read_window_state_template(args.window_state_template), args.output)
