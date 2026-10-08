#!/usr/bin/env python3
"""Actual packaged WKWebView remote-image command rejection boundaries.

Fresh private profiles only; no clipboard, remote account, SSH or model call.
This deliberately does not count as successful remote PNG/SSH/Serve acceptance.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("remote_image_windows", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


def validate_receipt(receipt):
    fields = ("ok", "registeredWebViewIPC", "unknownFieldRejected", "invalidSourceRejected", "unknownHandleRejected")
    if not isinstance(receipt, dict) or any(receipt.get(key) is not True for key in fields):
        raise RuntimeError("remote image boundary receipt incomplete")
    if any(receipt.get(key) is not False for key in ("positivePixels", "clipboardTouched", "sharedTranscriptUI")):
        raise RuntimeError("remote image boundary receipt overclaims its scope")


def smoke(app_path, template_path, profile):
    app = Path(app_path).resolve()
    identifier = plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleIdentifier"]
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("Preview already running; boundary probe refused")
    placement = windows.package.placement_helper()
    geometry = placement.read_window_state_template(Path(template_path))
    if geometry["x"] >= 0:
        raise ValueError("boundary probe requires a left-display fixture")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True, timeout=20)
    evidence = Path(tempfile.mkdtemp(prefix="reasonix-native-remote-image-boundary-", dir="/private/tmp"))
    evidence.chmod(0o700)
    environment = {key: os.environ[key] for key in ("PATH", "LANG", "LC_ALL", "USER", "LOGNAME") if key in os.environ}
    for mode in ("managed", "explicit") if profile == "both" else (profile,):
        root = evidence / mode
        root.mkdir(mode=0o700)
        for name in ("home", "tmp"):
            (root / name).mkdir(mode=0o700)
        managed = mode == "managed"
        app_data = root / "home/Library/Application Support" / identifier
        placement.seed_window_state(app_data, geometry)
        core = app_data / "reasonix-core" if managed else root / "core"
        core.mkdir(parents=True, mode=0o700)
        config = core / "config.toml"
        config.touch(mode=0o600, exist_ok=False)
        config.write_text('default_model = "local/alpha"\n[desktop]\nprovider_access = ["local"]\n'
                          '[[providers]]\nname = "local"\nkind = "openai"\n'
                          'base_url = "http://127.0.0.1:1/v1"\nmodels = ["alpha"]\ndefault = "alpha"\n')
        if windows.package.matching_package_is_running(identifier):
            raise RuntimeError("Preview started before probe; user app untouched")
        try:
            windows.launch(app / "Contents/MacOS/reasonix-tauri", app / "Contents/MacOS/reasonix-desktop-bridge",
                           root, identifier, managed, "ui-remote-image-boundary", verify_window_state=False,
                           environment=environment, launch_services=True)
            validate_receipt(json.loads((root / "tmp/reasonix-native-remote-image-result.json").read_text()))
            if json.loads((app_data / "window-state.json").read_text()) != geometry:
                raise RuntimeError("probe escaped its left-display fixture")
            print(f"native remote image {mode}: registered IPC/rejections/shutdown OK; positive pixels NOT tested", flush=True)
        finally:
            print(f"Private native boundary evidence: {root}", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    parser.add_argument("--window-state-template", required=True)
    parser.add_argument("--profile", choices=("managed", "explicit", "both"), default="both")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("native boundary probe requires macOS")
    try:
        smoke(args.app, args.window_state_template, args.profile)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"native remote image boundary probe failed: {error}") from error
