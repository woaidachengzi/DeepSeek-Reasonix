#!/usr/bin/env python3
"""Owned macOS WKWebView/xterm InputEvent -> packaged IPC -> actual PTY.

No clipboard, physical keyboard, real profile/model service or system Terminal.
LaunchServices runs only a fresh private Preview when its bundle ID is idle.
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
spec = importlib.util.spec_from_file_location("terminal_windows", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


def validate_receipt(receipt):
    fields = ("ok", "realWebViewIPC", "xtermInputEvent", "realTTY", "utf8AnsiPainted", "explicitCreateOnly",
              "collapseSamePTY", "explicitContextOnly", "closePIDGone", "switchPIDGone", "noModelHistory")
    if not isinstance(receipt, dict) or any(receipt.get(key) is not True for key in fields):
        raise RuntimeError("native integrated terminal receipt incomplete")
    pid = receipt.get("shutdownPID")
    if type(pid) is not int or not 1 < pid < 2**31:
        raise RuntimeError("native terminal shutdown PID invalid")
    return pid


def require_placement(geometry, allow_primary_display):
    if type(allow_primary_display) is not bool:
        raise ValueError("primary display permission must be explicit boolean")
    if geometry["x"] >= 0 and not allow_primary_display:
        raise ValueError("terminal smoke requires a left-display window template; use --allow-primary-display for an owned primary-screen run")


def smoke(app_path, template_path, profile, allow_primary_display=False):
    app = Path(app_path).resolve()
    identifier = plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleIdentifier"]
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("Preview already running; isolated terminal acceptance refused")
    placement = windows.package.placement_helper()
    geometry = placement.read_window_state_template(Path(template_path))
    require_placement(geometry, allow_primary_display)
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True, timeout=20)
    evidence = Path(tempfile.mkdtemp(prefix="reasonix-native-terminal-ui-", dir="/private/tmp"))
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
        # Prevent reading any user shell startup file even with the backend's
        # existing login-shell contract. These env values are test-owned only.
        scoped = {**environment, "SHELL": "/bin/sh", "ENV": "/dev/null", "BASH_ENV": "/dev/null", "ZDOTDIR": str(root / "home")}
        if windows.package.matching_package_is_running(identifier):
            raise RuntimeError("Preview started before terminal launch; user app untouched")
        try:
            windows.launch(app / "Contents/MacOS/reasonix-tauri", app / "Contents/MacOS/reasonix-desktop-bridge",
                           root, identifier, managed, "ui-integrated-terminal", verify_window_state=False,
                           environment=scoped, launch_services=True)
            receipt = json.loads((root / "tmp/reasonix-native-integrated-terminal-result.json").read_text())
            pid = validate_receipt(receipt)
            if windows.package.is_alive(pid):
                raise RuntimeError("owned terminal PID survived normal application shutdown")
            if json.loads((app_data / "window-state.json").read_text()) != geometry:
                raise RuntimeError("terminal acceptance escaped requested display geometry")
            print(f"native integrated terminal {mode}: IPC/input/TTY/UTF8/explicit context/close/switch/shutdown OK", flush=True)
        finally:
            # Preserve these owned profiles and fixed receipts for independent
            # inspection, including failed runs. Never remove user data.
            print(f"Private terminal evidence: {root}", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    parser.add_argument("--window-state-template", required=True)
    parser.add_argument("--profile", choices=("managed", "explicit", "both"), default="both")
    parser.add_argument("--allow-primary-display", action="store_true", help="explicitly allow the fresh owned test window on the primary display; exact geometry checks remain required")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("native integrated terminal acceptance requires macOS")
    try:
        smoke(args.app, args.window_state_template, args.profile, args.allow_primary_display)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"native integrated terminal smoke failed: {error}") from error
