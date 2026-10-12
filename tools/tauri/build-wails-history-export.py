#!/usr/bin/env python3
"""Build the read-only v5 exporter against the original Wails source codec."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--legacy-root", type=Path, required=True)
parser.add_argument("--output", type=Path, required=True)
args = parser.parse_args()
source = args.legacy_root.resolve(strict=True)
if not (source / "internal/session/v4_codec.go").is_file():
    raise SystemExit("The source does not contain the Wails versioned session codec")
destination = args.output.resolve()
destination.parent.mkdir(parents=True, exist_ok=True)
template = Path(__file__).with_name("wails_history_export.go.in").read_bytes()
# A temporary package inside this module may import its own internal codec.
# It is removed even on failure; no tracked Wails source or dependency is edited.
with tempfile.TemporaryDirectory(prefix=".takeover-export-", dir=source) as directory:
    package = Path(directory)
    (package / "main.go").write_bytes(template)
    subprocess.run(["go", "build", "-mod=readonly", "-trimpath", "-o", str(destination), "./" + package.name], cwd=source, env=os.environ.copy(), check=True)
os.chmod(destination, 0o700)
print(f"Built read-only Wails history exporter: {destination}")
