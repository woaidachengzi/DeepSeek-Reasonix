#!/usr/bin/env python3
"""Actual WKWebView -> packaged bridge -> owned SSH -> real Serve -> PNG.

Only the opt-in Go SSH fixture supplies this private manifest. No clipboard or
real account, model call or system SSH configuration is involved.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import re
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("positive_windows", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


def manifest(path):
    path = Path(path)
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 65536:
        raise ValueError("invalid owned manifest file")
    root = path.resolve().parent
    if not str(root).startswith("/private/tmp/reasonix-native-ssh-"):
        raise ValueError("manifest is outside the owned fixture root")
    value = json.loads(path.read_text())
    if not isinstance(value, dict) or set(value) != {"port", "fingerprint", "identityFile", "workspace", "sessionPath", "source", "width", "height"}:
        raise ValueError("invalid manifest fields")
    if type(value["port"]) is not int or not 1 <= value["port"] <= 65535 or not isinstance(value["fingerprint"], str) or not re.fullmatch(r"SHA256:[A-Za-z0-9+/]{43}", value["fingerprint"]):
        raise ValueError("invalid owned SSH target")
    for key in ("identityFile", "workspace", "sessionPath"):
        if not isinstance(value[key], str):
            raise ValueError("invalid manifest path")
        entry = Path(value[key])
        if entry.is_symlink() or not entry.resolve().is_relative_to(root):
            raise ValueError("manifest path escaped its fixture")
        if (key == "workspace" and not entry.is_dir()) or (key != "workspace" and not entry.is_file()):
            raise ValueError("owned fixture path missing")
    if value["source"] != "owned.png" or type(value["width"]) is not int or type(value["height"]) is not int or value["width"] != 16 or value["height"] != 10:
        raise ValueError("invalid owned image")
    return value


def validate_receipt(receipt):
    fields = ("ok", "registeredWebViewIPC", "unknownFieldRejected", "invalidSourceRejected", "unknownHandleRejected", "positivePixels")
    if not isinstance(receipt, dict) or any(receipt.get(key) is not True for key in fields):
        raise RuntimeError("native positive image receipt incomplete")
    if receipt.get("clipboardTouched") is not False or receipt.get("sharedTranscriptUI") is not False:
        raise RuntimeError("native positive image receipt overclaims scope")


def smoke(app_path, template_path, manifest_path, profile):
    value = manifest(manifest_path)
    app = Path(app_path).resolve()
    identifier = plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleIdentifier"]
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("Preview running; owned SSH image probe refused")
    placement = windows.package.placement_helper()
    geometry = placement.read_window_state_template(Path(template_path))
    if geometry["x"] >= 0:
        raise ValueError("probe requires owned left-display geometry")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True, timeout=20)
    evidence = Path(tempfile.mkdtemp(prefix="reasonix-native-remote-image-positive-", dir="/private/tmp"))
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
        config.touch(mode=0o600)
        q = json.dumps
        config.write_text('default_model="local/alpha"\n[desktop]\nprovider_access=["local"]\n'
                          '[[providers]]\nname="local"\nkind="openai"\nbase_url="http://127.0.0.1:1/v1"\nmodels=["alpha"]\ndefault="alpha"\n'
                          f'[[remote.hosts]]\nname="owned-image"\nhost="127.0.0.1"\nport={value["port"]}\nuser="owned-fixture"\n'
                          f'identity_file={q(value["identityFile"])}\nserve_install="never"\nuse_ssh_config=false\n')
        control = root / "tmp/reasonix-native-remote-image-control.json"
        control.touch(mode=0o600)
        control.write_text(json.dumps({"attach": {"name": "owned-image", "workspace": value["workspace"], "fingerprint": value["fingerprint"]},
                                       **{key: value[key] for key in ("sessionPath", "source", "width", "height")}}))
        if windows.package.matching_package_is_running(identifier):
            raise RuntimeError("Preview started before launch; user app untouched")
        try:
            windows.launch(app / "Contents/MacOS/reasonix-tauri", app / "Contents/MacOS/reasonix-desktop-bridge", root,
                           identifier, managed, "ui-remote-image-ipc", verify_window_state=False, environment=environment,
                           launch_services=True, acceptance_timeout=90)
            receipt = json.loads((root / "tmp/reasonix-native-remote-image-result.json").read_text())
            validate_receipt(receipt)
            if json.loads((app_data / "window-state.json").read_text()) != geometry:
                raise RuntimeError("owned image probe moved outside fixture geometry")
            print(f"native positive remote image {mode}: real IPC/SSH attach/PNG decode/close/disconnect/shutdown OK", flush=True)
        finally:
            print(f"Private positive image evidence: {root}", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    parser.add_argument("--window-state-template", required=True)
    parser.add_argument("--fixture-manifest", required=True)
    parser.add_argument("--profile", choices=("managed", "explicit", "both"), default="both")
    args = parser.parse_args()
    try:
        smoke(args.app, args.window_state_template, args.fixture_manifest, args.profile)
    except (OSError, ValueError, TypeError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"positive remote image probe failed: {error}") from error
