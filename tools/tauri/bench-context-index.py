#!/usr/bin/env python3
"""Offline serial context fingerprint and history-index comparison; synthetic data only."""
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
BASE = "ab07af612736cab0fa4d730075115abf18a30f01"
HARNESSES = ["internal/agent/projection_fingerprint_test.go",
             "internal/historycatalog/documents_benchmark_test.go"]
SOURCES = HARNESSES + ["internal/agent/projection.go", "internal/agent/projection_fingerprint.go",
    "internal/agent/sampling_request.go", "internal/agent/computeruse_context.go",
    "internal/provider/projection.go", "internal/provider/provider.go",
    "internal/historycatalog/documents.go", "internal/historycatalog/benchmark_test.go",
    "internal/historycatalog/catalog.go", "internal/retrieval/bm25.go", "internal/retrieval/token_text.go"]


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT, text=True).strip()


def run(command, cwd, log, env):
    print(f"Running {' '.join(map(str, command))}", flush=True)
    with log.open("w") as output:
        subprocess.run(list(map(str, command)), cwd=cwd, env=env, check=True,
                       stdout=output, stderr=subprocess.STDOUT)


def parse(log, expected):
    pattern = r"^(Benchmark\S+?)(?:-\d+)?\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op"
    rows = []
    for line in log.read_text().splitlines():
        if match := re.match(pattern, line):
            name, iterations, ns, size, allocs = match.groups()
            rows.append(dict(name=name, iterations=int(iterations), ns_per_op=float(ns),
                             go_bytes_per_op=int(size), go_allocs_per_op=int(allocs)))
    if len(rows) != len(expected) or {row["name"] for row in rows} != set(expected):
        raise RuntimeError(f"Incomplete benchmark: {log}: {rows}")
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--rounds", type=int, default=5)
    parser.add_argument("--iterations", type=int, default=10)
    args = parser.parse_args()
    if args.rounds < 1 or args.iterations < 1:
        parser.error("rounds/iterations must be positive")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    baseline = git("rev-parse", "--verify", BASE + "^{commit}")
    hashes = {name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest() for name in SOURCES}
    source = output / "baseline-source"
    source.mkdir(exist_ok=False)
    with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(["git", "archive", baseline], cwd=ROOT))) as files:
        files.extractall(source, filter="data")
    for name in HARNESSES:
        (source / name).write_bytes((ROOT / name).read_bytes())
    env = os.environ.copy()
    env.update(GOTOOLCHAIN="local", GOPROXY="off", GOSUMDB="off",
               REASONIX_HOME=str(output / "isolated-profile"))
    packages = {"context": ("./internal/agent", "Benchmark(ProjectionFingerprint|CoveredProjectionHash)",
        ["BenchmarkProjectionFingerprint/200", "BenchmarkProjectionFingerprint/10000",
         "BenchmarkProjectionFingerprint/50000", "BenchmarkCoveredProjectionHash"]),
        "index": ("./internal/historycatalog", "Benchmark(HistoryDocuments|WarmSearchTenThousandSources)",
         ["BenchmarkHistoryDocuments/latin", "BenchmarkHistoryDocuments/mixed", "BenchmarkWarmSearchTenThousandSources"])}
    binaries = {}
    for route, cwd in [("baseline", source), ("candidate", ROOT)]:
        for key, (package, pattern, expected) in packages.items():
            binary = output / f"{route}-{key}.test"
            run(["go", "test", "-mod=readonly", "-c", "-o", binary, package], cwd,
                output / f"{route}-{key}-build.log", env)
            binaries[(route, key)] = binary
    samples = {route: [] for route in ["baseline", "candidate"]}
    for index in range(-1, args.rounds):
        for route in (["baseline", "candidate"] if index % 2 == 0 else ["candidate", "baseline"]):
            batch = []
            for key, (_, pattern, expected) in packages.items():
                log = output / f"{route}-{key}-{index}.log"
                run([binaries[(route, key)], "-test.run=^$", f"-test.bench=^{pattern}$",
                     "-test.benchmem", f"-test.benchtime={args.iterations}x", "-test.count=1"], ROOT, log, env)
                batch.extend(parse(log, expected))
            if index >= 0:
                samples[route].append(batch)
    medians = {}
    for route, rounds in samples.items():
        medians[route] = [dict(name=row["name"], **{
            key: statistics.median(batch[index][key] for batch in rounds)
            for key in ["ns_per_op", "go_bytes_per_op", "go_allocs_per_op"]
        }) for index, row in enumerate(rounds[0])]
    for name, expected in hashes.items():
        if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != expected:
            raise RuntimeError(f"Source changed during measurement: {name}")
    report = dict(baseline_commit=baseline, candidate_commit=git("rev-parse", "HEAD"),
        candidate_status=git("status", "--short"), source_sha256=hashes,
        harness_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        go=subprocess.check_output(["go", "version"], text=True).strip(), platform=platform.platform(),
        rounds=args.rounds, iterations=args.iterations, samples=samples, medians=medians,
        scope="Synthetic in-process fingerprint, full covered-prefix normalization/hash, 1000-document indexing text and unchanged 10K-source FTS query. Setup excluded; warm-up discarded; serial alternating routes. Go bytes/op are cumulative allocations, not peak RSS. No model, disk persist, IPC, renderer or capture timing.",
        binary_sha256={f"{route}-{key}": hashlib.sha256(binary.read_bytes()).hexdigest()
                       for (route, key), binary in binaries.items()})
    (output / "benchmark.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(medians, indent=2))


if __name__ == "__main__":
    main()
