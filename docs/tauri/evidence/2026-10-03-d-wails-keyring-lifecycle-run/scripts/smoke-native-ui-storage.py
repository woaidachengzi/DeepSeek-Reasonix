#!/usr/bin/env python3
"""Verify actual WKWebView UI storage, restart persistence and profile separation.

Only private profiles and fixed canaries are used. Existing unscoped Preview UI
data is neither imported nor cleared. Does not prove legacy UI-pref migration.
"""
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import uuid

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("native_window", Path(__file__).with_name("smoke-native-window.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


def smoke(app_path):
    app = Path(app_path).resolve()
    host = app / "Contents/MacOS/reasonix-tauri"
    sidecar = app / "Contents/MacOS/reasonix-desktop-bridge"
    identifier = "io.reasonix.desktop.preview"
    if not host.is_file() or not sidecar.is_file() or runner.package.matching_package_is_running(identifier):
        raise RuntimeError("packaged Preview missing or already running")
    environment = {key: value for key, value in os.environ.items()
                   if key in ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL", "__CF_USER_TEXT_ENCODING")}
    with tempfile.TemporaryDirectory(prefix="reasonix-native-ui-storage-") as directory:
        base = Path(directory)
        base.chmod(0o700)
        for managed in (True, False):
            roots = [base / f"{'managed' if managed else 'explicit'}-{index}" for index in (1, 2)]
            for root in roots:
                root.mkdir(mode=0o700)
                for name in ("home", "tmp", "工作区 UI"):
                    (root / name).mkdir(mode=0o700)
                (root / "tmp/reasonix-native-ui-storage-control.json").write_text(json.dumps({
                    "nonce": uuid.uuid4().hex, "workspace": str(root / "工作区 UI"),
                }))
            identities = {}

            def launch(index, phase):
                identity = runner.launch(host, sidecar, roots[index], identifier, managed,
                                         phase, verify_window_state=False, environment=environment)
                if index in identities and identities[index] != identity:
                    raise RuntimeError("UI restart changed the credential/profile identity")
                identities[index] = identity

            for index in (0, 1):
                launch(index, "ui-store-empty")
                launch(index, "ui-store-seed")
                launch(index, "ui-store-restore")
            if identities[0] == identities[1]:
                raise RuntimeError("separate core profiles shared an identity")
            launch(0, "ui-store-restore")
            for index in (0, 1):
                launch(index, "ui-store-clear")
        print("packaged macOS UI storage: distinct profiles, restart rendering and canary cleanup OK")


if __name__ == "__main__":
    if sys.platform != "darwin" or len(sys.argv) != 2:
        raise SystemExit("usage: smoke-native-ui-storage.py MACOS_PREVIEW_APP")
    smoke(sys.argv[1])
