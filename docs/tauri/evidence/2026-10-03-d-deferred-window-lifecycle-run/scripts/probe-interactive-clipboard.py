#!/usr/bin/env python3
"""Private actual-keyboard clipboard acceptance with complete preservation.

Run with an installed app, follow the printed unique canary/controller using
CUA, and Quit the exact private app. Never inspect its UI binding after Quit.
This runner never types, copies, cuts or pastes on behalf of the UI operator.
"""
import argparse
import importlib.util
import json
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app", type=Path)
    options = parser.parse_args()
    app = options.app.resolve()
    launcher = module("launch_services", "probe-launch-services-profile.py")
    clipboard = module("clipboard_fixture", "native-clipboard-fixture.py")
    if launcher.package.matching_package_is_running("io.reasonix.desktop.preview"):
        raise SystemExit("another Preview is running; no clipboard fixture started")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    with (app / "Contents/Info.plist").open("rb") as stream:
        identifier = plistlib.load(stream)["CFBundleIdentifier"]
    if identifier != "io.reasonix.desktop.preview" or not all(
        (app / "Contents/MacOS" / name).is_file()
        for name in ("reasonix-tauri", "reasonix-desktop-bridge")
    ):
        raise SystemExit("the signed Preview host and sidecar are required")
    root = Path(tempfile.mkdtemp(prefix="reasonix-interactive-clipboard-", dir="/private/tmp"))
    root.chmod(0o700)
    with clipboard.NativeClipboardFixture(root) as fixture:
        controller = {"root": str(root), "binary": str(fixture.binary),
                      "snapshot": str(fixture.snapshot), "marker": str(fixture.marker),
                      "canary": "reasonix-native-clipboard-" + fixture.nonce}
        (root / "controller.json").write_text(json.dumps(controller, indent=2) + "\n")
        print(json.dumps(controller), flush=True)
        launcher.launch(app, identifier, False, interactive=True, observe=True, wait_seconds=900)
        fixture.verify()
    # The context verifies byte/order/type restoration before deleting the
    # original payload; evidence contains no original clipboard contents.
    result = {"root": str(root), "originalFormatsRestored": True,
              "originalSnapshotRemoved": not fixture.snapshot.exists()}
    (root / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result), flush=True)
