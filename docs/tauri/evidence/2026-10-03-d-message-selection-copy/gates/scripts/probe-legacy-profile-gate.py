#!/usr/bin/env python3
"""Negative compatibility probe for the official macOS 1.38.3 CLI.

Uses only generated private data. Exit 1 means an old config writer ignored
the current directory lifetime gates; it is not an acceptance pass. This does
not launch Wails GUI or prove its entire lifecycle, sessions or attachment IO.
"""
import argparse
import fcntl
import hashlib
import os
from pathlib import Path
import subprocess
import sys
import tempfile

OFFICIAL_DIGESTS = {
    # Authenticated standalone arm64 CLI and CLI bundled in official Desktop.
    "49be12faf150879a1da58fb539871c26b6077fe2d45ffe3ad105bfb541d5a091",
    "5b1ab31424d45c8bf3cfe6a60b11da527df2252aee963d1c0c56352f57945962",
}
LOCK_NAME = ".reasonix-session-profile.lock"


def probe(cli):
    if cli.is_symlink() or not cli.is_file():
        raise RuntimeError("official CLI must be an ordinary file")
    if hashlib.sha256(cli.read_bytes()).hexdigest() not in OFFICIAL_DIGESTS:
        raise RuntimeError("CLI differs from authenticated official 1.38.3 artifacts")
    with tempfile.TemporaryDirectory(prefix="reasonix-legacy-gate-probe-", dir="/private/tmp") as folder:
        root = Path(folder)
        root.chmod(0o700)
        home, config, state, cache = [root / name for name in ("home", "config", "state", "cache")]
        for directory in (home, config, state, cache):
            directory.mkdir(mode=0o700)
        source = config / "config.toml"
        original = b'currency = "CNY"\n'
        source.write_bytes(original)
        source.chmod(0o600)
        environment = {key: os.environ[key] for key in ("PATH", "LANG", "LC_ALL") if key in os.environ}
        environment.update(HOME=str(home), REASONIX_HOME=str(config), REASONIX_STATE_HOME=str(state), REASONIX_CACHE_HOME=str(cache))
        gates = []
        try:
            for directory in (config, state):
                path = directory / LOCK_NAME
                gate = os.open(path, os.O_CREAT | os.O_RDWR, 0o600)
                gates.append(gate)
                fcntl.flock(gate, fcntl.LOCK_EX | fcntl.LOCK_NB)
                # This is the same macOS flock operation used by filelock.
                # Independently prove a second lock-aware writer is excluded.
                contender = os.open(path, os.O_RDWR)
                try:
                    try:
                        fcntl.flock(contender, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    except BlockingIOError:
                        pass
                    else:
                        raise RuntimeError("probe did not hold an exclusive directory gate")
                finally:
                    os.close(contender)
            result = subprocess.run([str(cli), "config", "currency", "USD"], cwd=root,
                                    env=environment, capture_output=True, timeout=20)
            changed = source.read_bytes() != original
            if result.returncode == 0 and changed and b'currency = "USD"' in source.read_bytes():
                print("UNSAFE: official 1.38.3 CLI changed config while both current directory gates were held")
                print("Current gate exclusion: config and state verified; legacy config write exclusion: failed")
                return 1
            raise RuntimeError("legacy writer outcome is inconclusive; do not count as accepted")
        finally:
            for gate in reversed(gates):
                os.close(gate)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("legacy_cli", type=Path)
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("macOS only")
    raise SystemExit(probe(args.legacy_cli.absolute()))
