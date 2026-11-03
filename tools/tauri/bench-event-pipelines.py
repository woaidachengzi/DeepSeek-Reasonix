#!/usr/bin/env python3
"""Serial synthetic event/SSE/Cua reply comparisons; no models or native input."""
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
import tomllib

ROOT = Path(__file__).resolve().parents[2]
BASE = "bab3297ba129a0a8db5393e765a5638501713adb"
GO_BENCH = "internal/desktopbridge/live_typed_benchmark_test.go"
SOURCES = [
    "internal/desktopbridge/events.go", "internal/desktopbridge/live_state.go",
    "internal/desktopbridge/live_update.go", "internal/desktopbridge/ledger.go", GO_BENCH,
    "desktop/tauri/src/bridge/event_stream.rs", "desktop/tauri/src/bridge/event_stream_bench.rs",
    "desktop/tauri/src/bridge/http_response.rs", "desktop/tauri/src/protocol_generated.rs",
    "desktop/tauri/Cargo.lock", "desktop/cua-runtime/src/main.rs",
    "desktop/cua-runtime/src/reply.rs", "desktop/cua-runtime/src/preview.rs",
    "desktop/cua-runtime/examples/reply_pipeline.rs", "desktop/cua-runtime/Cargo.lock",
    "internal/computeruse/cua_reply_bench_test.go",
    "tools/tauri/bench-event-pipelines.py",
]


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT, text=True).strip()


def run(args, log, env, cwd=ROOT):
    print("Running " + " ".join(map(str, args)), flush=True)
    with log.open("w") as output:
        subprocess.run(list(map(str, args)), cwd=cwd, env=env, stdout=output,
                       stderr=subprocess.STDOUT, check=True)


def go_rows(log):
    pattern = r"^(Benchmark\S+?)(?:-\d+)?\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op"
    result = []
    for line in log.read_text().splitlines():
        if match := re.match(pattern, line):
            name, iterations, ns, size, count = match.groups()
            result.append(dict(name=name, iterations=int(iterations), ns_per_op=float(ns),
                               go_bytes_per_op=int(size), go_allocs_per_op=int(count)))
    if not result:
        raise RuntimeError(f"No benchmark records in {log}")
    return result


def summarize(rounds, key, metrics):
    names = [row[key] for row in rounds[0]]
    if len(names) != len(set(names)):
        raise RuntimeError("Duplicate benchmark keys")
    if any({row[key] for row in batch} != set(names) for batch in rounds):
        raise RuntimeError("Benchmark workload set changed across rounds")
    indexed = [{row[key]: row for row in batch} for batch in rounds]
    return [{key: name, **{metric: statistics.median(batch[name][metric] for batch in indexed)
                          for metric in metrics}} for name in names]


def sse_binary(output, baseline, env):
    folder = output / "sse"
    folder.mkdir()
    archived = folder / "baseline.rs"
    old = git("show", baseline + ":desktop/tauri/src/bridge/event_stream.rs")
    archived.write_text("\n".join("//" + line[3:] if line.startswith("//!") else line
                                  for line in old.splitlines()) + "\n")
    lock_path = ROOT / "desktop/tauri/Cargo.lock"
    lock = tomllib.loads(lock_path.read_text())
    host = next(p for p in lock["package"] if p["name"] == "reasonix-tauri")
    versions = {}
    for name in ["serde", "serde_json"]:
        reference = next(dep.split() for dep in host["dependencies"] if dep.split()[0] == name)
        choices = {p["version"] for p in lock["package"] if p["name"] == name}
        if len(reference) > 1:
            versions[name] = reference[1]
        elif len(choices) == 1:
            versions[name] = choices.pop()
        else:
            raise RuntimeError(f"Ambiguous locked host dependency: {name}")
    (folder / "Cargo.lock").write_bytes(lock_path.read_bytes())
    code = ROOT / "desktop/tauri/src/bridge/event_stream_bench.rs"
    (folder / "Cargo.toml").write_text(
        '[package]\nname="sse-cursor-bench"\nversion="0.1.0"\nedition="2021"\n'
        '[[bin]]\nname="sse-cursor-bench"\npath=' + json.dumps(str(code)) + '\n'
        '[dependencies]\nserde={version="=' + versions["serde"] + '",features=["derive"]}\n'
        'serde_json="=' + versions["serde_json"] + '"\n[workspace]\n')
    local_env = dict(env, REASONIX_SSE_BASELINE_RS=str(archived))
    run(["cargo", "build", "--release", "--offline", "--manifest-path", folder / "Cargo.toml"],
        output / "sse-build.log", local_env)
    return folder / "target/release/sse-cursor-bench"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--rounds", type=int, default=5)
    args = parser.parse_args()
    if args.rounds < 1:
        parser.error("rounds must be positive")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    baseline = git("rev-parse", "--verify", BASE + "^{commit}")
    candidate_commit, status = git("rev-parse", "HEAD"), git("status", "--short")
    hashes = {name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest() for name in SOURCES}
    env = dict(os.environ, GOTOOLCHAIN="local", GOPROXY="off", GOSUMDB="off",
               REASONIX_HOME=str(output / "isolated-profile"))
    source = output / "baseline-source"
    source.mkdir(exist_ok=False)
    archive = subprocess.check_output(["git", "archive", baseline], cwd=ROOT)
    with tarfile.open(fileobj=io.BytesIO(archive)) as files:
        files.extractall(source, filter="data")
    (source / GO_BENCH).write_bytes((ROOT / GO_BENCH).read_bytes())
    binaries = {}
    for route, cwd in [("baseline", source), ("candidate", ROOT)]:
        binary = output / (route + "-events.test")
        run(["go", "test", "-mod=readonly", "-c", "-o", binary, "./internal/desktopbridge"],
            output / (route + "-events-build.log"), env, cwd)
        binaries[route] = binary
    pipe = output / "cua-pipe.test"
    run(["go", "test", "-mod=readonly", "-c", "-o", pipe, "./internal/computeruse"],
        output / "cua-pipe-build.log", env)
    sse = sse_binary(output, baseline, env)
    run(["cargo", "build", "--release", "--locked", "--offline", "--manifest-path",
         ROOT / "desktop/cua-runtime/Cargo.toml", "--example", "reply_pipeline"],
        output / "cua-reply-build.log", env)
    reply = ROOT / "desktop/cua-runtime/target/release/examples/reply_pipeline"
    # All compilers have finished. No tests or other benchmarks overlap timing.
    events, sse_samples, reply_samples, pipes = ({"baseline": [], "candidate": []},
        {"baseline": [], "candidate": []}, {"legacy": [], "borrowed": []}, {"legacy": [], "borrowed": []})
    for index in range(args.rounds):
        order = ["baseline", "candidate"] if index % 2 == 0 else ["candidate", "baseline"]
        for route in order:
            log = output / f"events-{route}-{index}.log"
            run([binaries[route], "-test.run=^$", "-test.bench=^BenchmarkEventTypedPublish$",
                 "-test.benchmem", "-test.benchtime=100ms", "-test.count=1"], log, env)
            records = go_rows(log)
            if len(records) != 12:
                raise RuntimeError(f"Expected 12 event workloads in {log}")
            events[route].append(records)
        for route in order:
            log = output / f"sse-{route}-{index}.json"
            run([sse, route, "3"], log, env)
            records = json.loads(log.read_text())["rows"]
            if len(records) != 4:
                raise RuntimeError("Expected four SSE workloads")
            for row in records:
                row["name"] = row["workload"] + "/reader" + str(row["reader_capacity"])
            sse_samples[route].append(records)
        reply_order = ["legacy", "borrowed"] if index % 2 == 0 else ["borrowed", "legacy"]
        for route in reply_order:
            batch = []
            for size in [64 << 10, 1 << 20, 8 << 20]:
                log = output / f"reply-{route}-{size}-{index}.json"
                run([reply, "--bench", route, size, "20"], log, env)
                batch.append(json.loads(log.read_text()))
            reply_samples[route].append(batch)
        pipe_env = dict(env, REASONIX_CUA_REPLY_BENCH_BIN=str(reply))
        for route in reply_order:
            log = output / f"pipe-{route}-{index}.log"
            run([pipe, "-test.run=^$", f"-test.bench=^BenchmarkCuaReplyPipeline/.*/{route}$",
                 "-test.benchmem", "-test.benchtime=20x", "-test.count=1"], log, pipe_env)
            records = go_rows(log)
            if len(records) != 3:
                raise RuntimeError(f"Expected three pipe workloads in {log}")
            pipes[route].append(records)
    # Diagnostic contention samples are separate from unprofiled timing above.
    for route, binary in binaries.items():
        profile = output / f"{route}-mutex.pprof"
        run([binary, "-test.run=^$", "-test.bench=^BenchmarkEventTypedConcurrent/Tool1MiB/Sessions16/Subscribers0$",
             "-test.benchtime=100x", "-test.count=1", "-test.mutexprofilefraction=1",
             "-test.mutexprofile=" + str(profile)], output / f"{route}-mutex.log", env)
        run(["go", "tool", "pprof", "-top", "-nodecount=10", binary, profile],
            output / f"{route}-mutex.txt", env)
    for name, expected in hashes.items():
        if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != expected:
            raise RuntimeError(f"Source changed after build: {name}; discard samples")
    report = dict(baseline_commit=baseline, candidate_commit=candidate_commit, candidate_status=status,
        platform=platform.platform(), machine=platform.machine(), rounds=args.rounds,
        go=subprocess.check_output(["go", "version"], text=True).strip(),
        rust=subprocess.check_output(["rustc", "--version"], text=True).strip(),
        source_sha256=hashes,
        binary_sha256={name: hashlib.sha256(path.read_bytes()).hexdigest()
                       for name, path in {**binaries, "sse": sse, "cua_reply": reply, "go_pipe": pipe}.items()},
        events=events, sse=sse_samples, reply=reply_samples, pipe=pipes,
        medians=dict(
            events={route: summarize(rows, "name", ["ns_per_op", "go_bytes_per_op", "go_allocs_per_op"]) for route, rows in events.items()},
            sse={route: summarize(rows, "name", ["ns_per_stream", "allocation_calls_per_stream", "allocated_bytes_per_stream"]) for route, rows in sse_samples.items()},
            reply={route: summarize(rows, "base64_bytes", ["ns_per_op", "rust_allocated_bytes_per_op", "rust_allocations_per_op", "encoded_bytes"]) for route, rows in reply_samples.items()},
            pipe={route: summarize(rows, "name", ["ns_per_op", "go_bytes_per_op", "go_allocs_per_op"]) for route, rows in pipes.items()}),
        limits=["Synthetic data only; no model, screenshot, native input or permissions.",
                "Events include typed Sink, JSON wire, ledger, checkpoint and optional channel; no socket or renderer.",
                "SSE includes headers, framing, actual DTO JSON decode and global cursor; no socket or renderer.",
                "Cua isolated replies measure envelope/JSON work; pipe samples include a warm worker and complete Go decode/image identity checks. SDK/capture/thumbnail/model excluded.",
                "Go B/op and Rust allocation requests are cumulative, not peak RSS. Rust realloc counts the requested new size.",
                "Mutex profiles are diagnostic and excluded from timing medians."])
    (output / "benchmark.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report["medians"], indent=2), flush=True)


if __name__ == "__main__":
    main()
