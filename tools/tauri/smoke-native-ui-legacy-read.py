#!/usr/bin/env python3
"""Read old UI allowlist via real main IPC twice without changing old storage.

Exercises reader destruction with closeBehavior=quit. No old values are logged
or imported. Private profile identities and ordinary cleanup are also checked.
"""
import importlib.util
import os
from pathlib import Path
import sys
import tempfile

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
    with tempfile.TemporaryDirectory(prefix="reasonix-native-ui-legacy-read-") as directory:
        base = Path(directory)
        base.chmod(0o700)
        for managed in (True, False):
            root = base / ("managed" if managed else "explicit")
            root.mkdir(mode=0o700)
            for name in ("home", "tmp"):
                (root / name).mkdir(mode=0o700)
            runner.launch(host, sidecar, root, identifier, managed,
                          "ui-legacy-read", verify_window_state=False, environment=environment)
    print("Actual main IPC legacy UI read, repeated inert reader cleanup and close-quit policy: OK")


if __name__ == "__main__":
    if sys.platform != "darwin" or len(sys.argv) != 2:
        raise SystemExit("usage: smoke-native-ui-legacy-read.py MACOS_PREVIEW_APP")
    smoke(sys.argv[1])
