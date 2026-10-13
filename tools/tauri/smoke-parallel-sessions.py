#!/usr/bin/env python3
"""Verify packaged Preview parallel tasks with an owned private loopback fixture.
Never targets the actual login profile or an already running Preview instance.
Uses the normal native setup, supervisor API, and application Exit cleanup.
"""
import argparse
import importlib.util
import json
import os
import plistlib
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True
def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    loaded = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(loaded)
    return loaded

package = module("parallel_package", "smoke-packaged-app.py")
provider = module("parallel_provider", "native-task-provider.py")

def wait_file(path, process):
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        if path.is_file():
            return
        if process.poll() is not None:
            raise RuntimeError("private parallel host exited before acceptance marker")
        time.sleep(0.05)
    raise RuntimeError("private parallel host timed out")

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--app", type=Path, required=True)
    args = parser.parse_args()
    app = args.app.resolve(strict=True)
    with (app / "Contents/Info.plist").open("rb") as file:
        if plistlib.load(file).get("CFBundleIdentifier") != "io.reasonix.desktop.preview":
            raise RuntimeError("parallel acceptance requires the isolated Preview package")
    if package.matching_package_is_running("io.reasonix.desktop.preview"):
        raise RuntimeError("Preview is already running; private acceptance must not attach to it")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    with tempfile.TemporaryDirectory(prefix="reasonix-parallel-native-", dir="/private/tmp") as tmp:
        root = Path(tmp)
        root.chmod(0o700)
        home, core, temporary = root / "home", root / "core", root / "tmp"
        for directory in (home, core, temporary):
            directory.mkdir(mode=0o700)
        env = {name: value for name, value in os.environ.items() if name in ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL", "__CF_USER_TEXT_ENCODING")}
        env.update(HOME=str(home), TMPDIR=str(temporary), REASONIX_HOME=str(core),
                   REASONIX_CACHE_HOME=str(root / "cache"), REASONIX_CREDENTIALS_STORE="file",
                   REASONIX_TAURI_PROFILE_SMOKE="parallel")
        process = None
        with provider.NativeTaskProvider(core, temporary, host_death=True) as fixture:
            try:
                declaration = core / "parallel-fixture.json"
                declaration.write_text(json.dumps({"port": fixture.server.server_port}))
                declaration.chmod(0o600)
                binary = app / "Contents/MacOS/reasonix-tauri"
                process = subprocess.Popen([str(binary)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
                wait_file(temporary / "reasonix-native-profile-waiting.json", process)
                (temporary / "reasonix-native-profile-begin.json").write_text("{}")
                wait_file(temporary / "reasonix-native-profile-result.json", process)
                result = json.loads((temporary / "reasonix-native-profile-result.json").read_text())
                if not result.get("ok"):
                    raise RuntimeError(result.get("error", "native parallel acceptance failed"))
                deadline = time.monotonic() + 5
                independently_closed = False
                while time.monotonic() < deadline:
                    with fixture.lock:
                        independently_closed = fixture.closed_requests == {1}
                    if independently_closed:
                        break
                    time.sleep(0.05)
                if not independently_closed:
                    raise RuntimeError("A cancellation did not leave B provider connected")
                (temporary / "reasonix-native-profile-exit.json").write_text("{}")
                if process.wait(timeout=20) != 0:
                    raise RuntimeError("parallel host did not exit cleanly")
                fixture.verify_parallel_quit()
                deadline = time.monotonic() + 5
                while package.own_sidecars(temporary, app / "Contents/MacOS/reasonix-desktop-bridge") and time.monotonic() < deadline:
                    time.sleep(0.05)
                if package.own_sidecars(temporary, app / "Contents/MacOS/reasonix-desktop-bridge"):
                    raise RuntimeError("parallel host left an owned sidecar running")
                result["providerRequests"] = fixture.requests
                result["bothProvidersDisconnectedOnExit"] = True
                result["sidecarCleanedOnExit"] = True
                print(json.dumps(result))
            finally:
                if process is not None and process.poll() is None:
                    os.killpg(process.pid, signal.SIGTERM)
                    process.wait(timeout=10)

if __name__ == "__main__":
    main()
