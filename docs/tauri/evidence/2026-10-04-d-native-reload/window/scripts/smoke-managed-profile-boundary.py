#!/usr/bin/env python3
"""Reject an aliased managed profile before opening the UI or starting a bridge.

Uses a real macOS package and three private HOME fixtures. Protects the legacy
directory's bytes, modes, timestamps and inode identities, including the link
itself. Does not certify arbitrary historical hosts or explicit shared roots.
"""
import importlib.util
import os
from pathlib import Path
import plistlib
import signal
import shutil
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("package", Path(__file__).with_name("smoke-packaged-app.py"))
package = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package)


def snapshot(directory):
    result = {}
    for path in [directory, *sorted(directory.rglob("*"))]:
        metadata = path.lstat()
        result[str(path.relative_to(directory))] = (
            metadata.st_mode, metadata.st_mtime_ns, metadata.st_ino,
            path.read_bytes() if path.is_file() and not path.is_symlink() else None,
        )
    return result


def smoke(app_path):
    app = Path(app_path).resolve()
    host_binary = app / "Contents/MacOS/reasonix-tauri"
    sidecar = app / "Contents/MacOS/reasonix-desktop-bridge"
    with (app / "Contents/Info.plist").open("rb") as file:
        identifier = plistlib.load(file)["CFBundleIdentifier"]
    if not host_binary.is_file() or not sidecar.is_file() or package.matching_package_is_running(identifier):
        raise RuntimeError("packaged Preview is missing or already running")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    for case in ("legacy-link", "dangling-link", "regular-file"):
        root = Path(tempfile.mkdtemp(prefix="reasonix-managed-boundary-", dir="/private/tmp"))
        root.chmod(0o700)
        home, temporary = root / "home", root / "tmp"
        home.mkdir(mode=0o700)
        temporary.mkdir(mode=0o700)
        legacy = home / ".reasonix"
        legacy.mkdir(mode=0o700)
        for name, content in (("config.toml", b"# legacy original\n"), (".env", b"PRIVATE_DUMMY=fixture\n")):
            path = legacy / name
            path.write_bytes(content)
            path.chmod(0o600)
        before = snapshot(legacy)
        app_data = home / "Library/Application Support" / identifier
        app_data.mkdir(parents=True, mode=0o700)
        profile = app_data / "reasonix-core"
        missing = home / "missing-profile"
        if case == "legacy-link":
            profile.symlink_to(legacy, target_is_directory=True)
        elif case == "dangling-link":
            profile.symlink_to(missing, target_is_directory=True)
        else:
            profile.write_bytes(b"profile file original\n")
            profile.chmod(0o600)
        original = profile.lstat()
        env = {key: value for key, value in os.environ.items()
               if key in ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL", "__CF_USER_TEXT_ENCODING")}
        env.update(HOME=str(home), TMPDIR=str(temporary), REASONIX_TAURI_PACKAGE_SMOKE="1")
        process = None
        success = False
        try:
            process = subprocess.Popen([str(host_binary)], env=env, stdout=subprocess.PIPE,
                                       stderr=subprocess.PIPE, start_new_session=True)
            _, error = process.communicate(timeout=20)
            expected_message = b"managed Preview data directory must be an ordinary directory" in error
            if process.returncode != 1 or not expected_message:
                raise RuntimeError(f"host did not refuse the managed profile with the expected recovery instruction: exit={process.returncode}, expectedMessage={expected_message}")
            if package.own_sidecars(temporary, sidecar) or list(temporary.glob("reasonix-tauri-bridge-*")):
                raise RuntimeError("profile refusal left a sidecar or readiness directory")
            if snapshot(legacy) != before or missing.exists():
                raise RuntimeError("profile refusal changed a legacy original or created the dangling target")
            after = profile.lstat()
            if (after.st_mode, after.st_mtime_ns, after.st_ino) != (original.st_mode, original.st_mtime_ns, original.st_ino):
                raise RuntimeError("profile refusal changed the conflicting profile entry")
            if case != "regular-file" and profile.readlink() != (legacy if case == "legacy-link" else missing):
                raise RuntimeError("profile refusal changed the original link")
            if case == "regular-file" and profile.read_bytes() != b"profile file original\n":
                raise RuntimeError("profile refusal changed the original file")
            success = True
            print(f"managed profile {case}: expected startup refusal, originals and no sidecar/readiness OK", flush=True)
        finally:
            if process is not None and process.poll() is None:
                process.kill()
                process.wait(timeout=5)
            # Any failure remains a failure even after cleanup. Signal only
            # the exact sidecar executable with this fixture's ready-file root.
            for pid in package.own_sidecars(temporary, sidecar):
                try:
                    os.kill(pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            if success:
                shutil.rmtree(root)
            else:
                print(f"Private boundary failure fixture retained: {root}", file=sys.stderr)


if __name__ == "__main__":
    if sys.platform != "darwin" or len(sys.argv) != 2:
        raise SystemExit("usage: smoke-managed-profile-boundary.py MACOS_PREVIEW_APP")
    smoke(sys.argv[1])
