#!/usr/bin/env python3
"""Run macOS D gates sequentially against one installed Preview candidate.

Each gate owns private fixtures and enforces its existing native acceptance
conditions. This runner records package/source/script identity and keeps logs;
it never turns programmatic checks into physical UI or full D acceptance.
The output directory must be new, so previous evidence cannot be overwritten.
"""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import subprocess
import sys
from datetime import datetime, timezone

sys.dont_write_bytecode = True
GATES = {
    "boundary": ("smoke-managed-profile-boundary.py", []),
    "package": ("smoke-packaged-app.py", []),
    "profile": ("smoke-profile-import.py", []),
    "startup": ("smoke-native-startup.py", []),
    "window": ("smoke-native-window.py", ["--focus", "--edit", "--dialogs"]),
    "lifetime": ("smoke-host-lifetime.py", []),
    "displays": ("smoke-native-displays.py", ["--straddle"]),
    "links": ("smoke-native-links.py", []),
    "notifications": ("smoke-native-notifications.py", []),
}


def digest(path):
    with path.open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def verify(app, output, names):
    repo = Path(__file__).resolve().parents[2]
    scripts = repo / "tools/tauri"
    artifacts = [app / "Contents/MacOS" / name
                 for name in ("reasonix-tauri", "reasonix-desktop-bridge")]
    artifacts.append(app / "Contents/Info.plist")
    if platform.system() != "Darwin" or not app.is_dir() or any(not path.is_file() for path in artifacts):
        raise ValueError("a real installed macOS Preview app is required")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    # Keep the signed bundle identity, including resources, alongside hashes
    # of the executable pair; recheck the signature after every native gate.
    signature = subprocess.run(["codesign", "-d", "--verbose=4", str(app)],
                               check=True, capture_output=True, text=True).stderr
    output.mkdir(mode=0o700, parents=False, exist_ok=False)
    (output / "signature.txt").write_text(signature)
    # Gates import other runners and native providers. Record those sources as
    # well so a unchanged entry script cannot hide changed verification logic.
    sources = [path for path in sorted(scripts.iterdir()) if path.is_file()]
    snapshots = output / "scripts"
    snapshots.mkdir(mode=0o700)
    for source in sources:
        (snapshots / source.name).write_bytes(source.read_bytes())
    patch = subprocess.check_output(["git", "diff", "--binary", "HEAD", "--"], cwd=repo)
    (output / "source.patch").write_bytes(patch)
    summary = {
        "startedAt": datetime.now(timezone.utc).isoformat(),
        "app": str(app),
        "sourceHead": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=repo, text=True).strip(),
        "dirty": bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=repo)),
        "sourcePatchSha256": hashlib.sha256(patch).hexdigest(),
        "artifactSha256": {str(path.relative_to(app)): digest(path) for path in artifacts},
        "runnerSha256": digest(Path(__file__).resolve()),
        "scriptSha256": {source.name: digest(source) for source in sources},
        "scope": "Selected programmatic D gates only; physical UI, historical stability and complete D/E acceptance remain separate.",
        "gates": [{"name": name, "status": "not-run"} for name in names],
    }

    def save():
        (output / "result.json").write_text(json.dumps(summary, indent=2) + "\n")

    save()
    for entry in summary["gates"]:
        name = entry["name"]
        script, args = GATES[name]
        source = scripts / script
        entry.update(status="running", script=script, scriptSha256=digest(source),
                     args=args, startedAt=datetime.now(timezone.utc).isoformat(), log=name + ".log")
        (output / script).write_bytes(source.read_bytes())
        save()
        try:
            with (output / entry["log"]).open("w") as log:
                result = subprocess.run([sys.executable, "-B", str(source), str(app), *args],
                                        cwd=repo, stdout=log, stderr=subprocess.STDOUT)
            entry["exitCode"] = result.returncode
            if any(digest(path) != summary["artifactSha256"][str(path.relative_to(app))] for path in artifacts):
                raise RuntimeError("installed candidate identity changed during acceptance")
            if digest(source) != entry["scriptSha256"]:
                raise RuntimeError("gate script changed during acceptance")
            if any(digest(path) != summary["scriptSha256"][path.name] for path in sources):
                raise RuntimeError("verification dependency changed during acceptance")
            subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)],
                           check=True, capture_output=True)
            current_signature = subprocess.run(["codesign", "-d", "--verbose=4", str(app)],
                                               check=True, capture_output=True, text=True).stderr
            if current_signature != signature:
                raise RuntimeError("signed candidate identity changed during acceptance")
            entry["status"] = "passed" if result.returncode == 0 else "failed"
        except BaseException as error:
            entry.update(status="failed", error=str(error) or type(error).__name__)
            save()
            raise
        entry["finishedAt"] = datetime.now(timezone.utc).isoformat()
        save()
        print(f"{name}: {entry['status']} (exit {entry['exitCode']})", flush=True)
        if entry["status"] != "passed":
            return 1
    summary["finishedAt"] = datetime.now(timezone.utc).isoformat()
    save()
    return 0


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app", type=Path)
    parser.add_argument("--output", required=True, type=Path, help="new evidence directory under an existing parent")
    parser.add_argument("--gates", nargs="+", choices=GATES, default=list(GATES))
    options = parser.parse_args()
    if len(set(options.gates)) != len(options.gates):
        parser.error("each gate may be selected only once")
    try:
        sys.exit(verify(options.app.resolve(), options.output.resolve(), options.gates))
    except (ValueError, FileExistsError) as error:
        parser.error(str(error))
