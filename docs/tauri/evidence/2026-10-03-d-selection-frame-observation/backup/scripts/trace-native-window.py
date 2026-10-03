#!/usr/bin/env python3
"""Run unchanged window acceptance with a read-only activation observer per phase."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import sys
import time

sys.dont_write_bytecode = True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise RuntimeError("macOS required")
    output = Path(args.output).resolve()
    output.mkdir(mode=0o700, parents=True, exist_ok=False)
    source = Path(__file__).with_name("activation-timeline.swift")
    observer = output / "activation-timeline"
    subprocess.run(["swiftc", str(source), "-o", str(observer)], check=True)
    runner = Path(__file__).with_name("smoke-native-window.py")
    spec = importlib.util.spec_from_file_location("window_acceptance", runner)
    windows = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(windows)
    original = windows.launch
    app = Path(args.app).resolve()
    files = [source, runner, Path(__file__), app / "Contents/MacOS/reasonix-tauri",
             app / "Contents/MacOS/reasonix-desktop-bridge"]
    (output / "identity.json").write_text(json.dumps({str(p): hashlib.sha256(p.read_bytes()).hexdigest()
                                                    for p in files}, indent=2) + "\n")
    shutil.copy2(source, output / source.name)
    shutil.copy2(runner, output / runner.name)

    def traced(host, sidecar, root, identifier, managed, phase, *positional, **keywords):
        receipt = output / ("managed" if managed else "explicit") / phase
        receipt.mkdir(parents=True)
        events = {"phase": phase, "root": str(root), "startedUnixMs": int(time.time() * 1000)}
        with (receipt / "activation.jsonl").open("wb") as stream, (receipt / "observer.log").open("wb") as errors:
            process = subprocess.Popen([str(observer)], stdout=stream, stderr=errors)
            try:
                deadline = time.monotonic() + 5
                while (receipt / "activation.jsonl").stat().st_size == 0:
                    if process.poll() is not None or time.monotonic() >= deadline:
                        raise RuntimeError("read-only observer not ready")
                    time.sleep(0.01)
                result = original(host, sidecar, root, identifier, managed, phase, *positional, **keywords)
                events["passed"] = True
                return result
            except BaseException as error:
                events.update(passed=False, error=str(error))
                raise
            finally:
                if process.poll() is None:
                    process.terminate()
                process.wait(timeout=5)
                events.update(finishedUnixMs=int(time.time() * 1000), observerExit=process.returncode)
                for p in (root / "tmp").glob("*.json*"):
                    if p.name.startswith(("launch-services-", "reasonix-native-window-")):
                        shutil.copy2(p, receipt / p.name)
                (receipt / "receipt.json").write_text(json.dumps(events, indent=2) + "\n")

    windows.launch = traced
    # Keep the original complete matrix, fixtures, exact assertions and cleanup.
    windows.smoke(app, include_focus=True, include_edit=True, include_dialogs=True,
                  launch_services=True)


if __name__ == "__main__":
    main()
