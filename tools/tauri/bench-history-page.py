#!/usr/bin/env python3
"""Serial offline HTTP history comparison against a fixed source commit."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import platform
import re
import statistics
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[2]
BASE = "cbf06ed4efc4ffea36c5fa1f7ff0846feda0fbc5"
HARNESS = "cmd/reasonix-desktop-bridge/history_page_benchmark_test.go"
SOURCES = [
    "cmd/reasonix-desktop-bridge/core_runtime.go",
    "cmd/reasonix-desktop-bridge/history_page.go",
    "cmd/reasonix-desktop-bridge/history_reader.go",
    "internal/agent/conversation_prefix.go",
    "internal/agent/conversation_prefix_accumulator.go",
    "internal/agent/session.go",
    "internal/agent/preview.go",
    "internal/agent/session_history_delta.go",
    "internal/agent/save_dag.go",
    "internal/agent/save_dag_plan.go",
    "internal/agent/tool_result_checkpoint.go",
    "internal/control/controller.go",
    "internal/control/history_delta.go",
    "internal/desktopbridge/runtime.go",
    "internal/desktopbridge/history_page.go",
    HARNESS,
    "cmd/reasonix-desktop-bridge/history_page_integration_test.go",
]


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT, text=True).strip()


def run(args, cwd, log, env):
    print("Running " + " ".join(map(str, args)), flush=True)
    with log.open("w") as output:
        subprocess.run(list(map(str, args)), cwd=cwd, env=env, stdout=output,
                       stderr=subprocess.STDOUT, check=True)


def rows(log):
    pattern = r"^(BenchmarkHistoryHTTP/\S+?)(?:-\d+)?\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op"
    result = []
    for line in log.read_text().splitlines():
        if match := re.match(pattern, line):
            name, iterations, ns, size, allocations = match.groups()
            result.append(dict(name=name, iterations=int(iterations), ns_per_op=float(ns),
                               go_bytes_per_op=int(size), go_allocs_per_op=int(allocations)))
    expected = {f"BenchmarkHistoryHTTP/{fixture}/{mode}"
                for fixture in ["1K", "10K", "50K", "10KTool4KiB"]
                for mode in ["Warm", "Append", "Rewrite", "Cold"]}
    if len(result) != 16 or {row["name"] for row in result} != expected:
        raise RuntimeError(f"Expected 16 workloads in {log}, got {len(result)}")
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--baseline", default=BASE)
    parser.add_argument("--rounds", type=int, default=5)
    parser.add_argument("--iterations", type=int, default=20)
    args = parser.parse_args()
    if args.rounds < 1 or args.iterations < 1:
        parser.error("rounds and iterations must be positive")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    baseline = git("rev-parse", "--verify", args.baseline + "^{commit}")
    candidate_commit = git("rev-parse", "HEAD")
    candidate_status = git("status", "--short")
    source_hashes = {name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest() for name in SOURCES}
    source = output / "baseline-source"
    # An isolated archive requires no git-index mutation and cannot affect the
    # user's working tree or the protected previous release artifacts.
    source.mkdir(exist_ok=False)
    archive = subprocess.check_output(["git", "archive", baseline], cwd=ROOT)
    with tarfile.open(fileobj=io.BytesIO(archive)) as files:
        files.extractall(source, filter="data")
    (source / HARNESS).write_bytes((ROOT / HARNESS).read_bytes())
    # Newer baselines already contain the bounded runtime constructor and must
    # retain it: replacing it with the legacy adapter would exaggerate gains.
    bounded_baseline = (source / "cmd/reasonix-desktop-bridge/history_page_integration_test.go").is_file()
    if not bounded_baseline:
        shim = source / "cmd/reasonix-desktop-bridge/history_page_baseline_test.go"
        shim.write_text('''package main
import "reasonix/internal/desktopbridge"
func newHistoryBenchmarkRuntime(core *controllerRuntime) desktopbridge.Runtime {
    return &historyHTTPFullRuntime{bridgeTestRuntime: &bridgeTestRuntime{path: "/synthetic/history-bench", state: "idle"}, core: core}
}
''')
    env = os.environ.copy()
    env["GOTOOLCHAIN"] = "local"
    env["GOPROXY"] = "off"
    env["GOSUMDB"] = "off"
    env["REASONIX_HOME"] = str(output / "isolated-profile")
    binaries = {}
    for route, cwd in [("baseline", source), ("candidate", ROOT)]:
        binary = output / (route + ".test")
        run(["go", "test", "-mod=readonly", "-c", "-o", binary,
             "./cmd/reasonix-desktop-bridge"], cwd, output / (route + "-build.log"), env)
        binaries[route] = binary
    samples = {"baseline": [], "candidate": []}
    for round_index in range(args.rounds):
        order = ["baseline", "candidate"] if round_index % 2 == 0 else ["candidate", "baseline"]
        for route in order:
            log = output / f"{route}-{round_index}.log"
            run([binaries[route], "-test.run=^$", "-test.bench=^BenchmarkHistoryHTTP$",
                 "-test.benchmem", f"-test.benchtime={args.iterations}x", "-test.count=1"],
                ROOT, log, env)
            samples[route].append(rows(log))
    medians = {}
    for route, rounds in samples.items():
        medians[route] = []
        for index, row in enumerate(rounds[0]):
            medians[route].append(dict(name=row["name"], **{
                key: statistics.median(batch[index][key] for batch in rounds)
                for key in ["ns_per_op", "go_bytes_per_op", "go_allocs_per_op"]
            }))
    for name, expected in source_hashes.items():
        if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != expected:
            raise RuntimeError(f"Source changed after build started: {name}; discard measurements")
    report = dict(baseline_commit=baseline,
                  baseline_runtime="archived bounded constructor" if bounded_baseline else "injected legacy constructor",
                  candidate_commit=candidate_commit,
                  candidate_status=candidate_status,
                  platform=platform.platform(), machine=platform.machine(),
                  go=subprocess.check_output(["go", "version"], text=True).strip(),
                  rounds=args.rounds, iterations=args.iterations,
                  scope="In-process authenticated production HTTP handler, manager, controller, session, projection and JSON encoding; no socket, Rust decode, renderer, model or native capture. Setup and prewarming are untimed. Rewrite includes copying replacement messages. Go allocations are cumulative per operation, not peak RSS.",
                  source_sha256=source_hashes,
                  binary_sha256={route: hashlib.sha256(binary.read_bytes()).hexdigest() for route, binary in binaries.items()},
                  samples=samples, medians=medians)
    (output / "benchmark.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(medians, indent=2), flush=True)


if __name__ == "__main__":
    main()
