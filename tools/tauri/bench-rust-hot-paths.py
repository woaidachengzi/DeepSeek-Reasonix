#!/usr/bin/env python3
"""Offline comparisons of production hot paths; no models, capture or UI input."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import sys
import tomllib

ROOT = Path(__file__).resolve().parents[2]
BASE = 'c8c68f599209be3dc2e4259285ee1c0356574df6'


def run(args, output, env=None):
    print('Running ' + ' '.join(map(str, args)), flush=True)
    with output.open('w') as log:
        subprocess.run(list(map(str, args)), cwd=ROOT, env=env, stdout=log,
                       stderr=subprocess.STDOUT, check=True)


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT, text=True).strip()


def benchmark_rows(path):
    rows = []
    pattern = r'^(Benchmark\S+)-\d+\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op'
    for line in path.read_text().splitlines():
        if match := re.match(pattern, line):
            name, count, ns, size, allocs = match.groups()
            rows.append(dict(name=name, iterations=int(count), ns_per_op=float(ns),
                             go_bytes_per_op=int(size), go_allocs_per_op=int(allocs)))
    if not rows:
        raise RuntimeError(f'No benchmark rows in {path}')
    return rows


def prepare_terminal(folder, baseline):
    source = git('show', baseline + ':desktop/tauri/src/terminal.rs')
    source = '\n'.join('//' + line[3:] if line.startswith('//!') else line
                       for line in source.splitlines()) + '\n'
    archived = folder / 'baseline-terminal.rs'
    archived.write_text(source)
    lock = tomllib.loads((ROOT / 'desktop/tauri/Cargo.lock').read_text())
    host = next(p for p in lock['package'] if p['name'] == 'reasonix-tauri')
    versions = {}
    for name in ['base64', 'serde', 'serde_json']:
        reference = next(dep.split() for dep in host['dependencies'] if dep.split()[0] == name)
        candidates = {p['version'] for p in lock['package'] if p['name'] == name}
        if len(reference) > 1:
            versions[name] = reference[1]
        elif len(candidates) == 1:
            versions[name] = candidates.pop()
        else:
            raise RuntimeError(f'Ambiguous host dependency: {name}')
    # Start with the shipped resolution; only the scratch root package changes.
    (folder / 'Cargo.lock').write_bytes((ROOT / 'desktop/tauri/Cargo.lock').read_bytes())
    code = ROOT / 'desktop/tauri/src/terminal_bench.rs'
    manifest = ('[package]\nname="terminal-route-bench"\nversion="0.1.0"\nedition="2021"\n'
                '[[bin]]\nname="terminal-route-bench"\npath=' + json.dumps(str(code)) + '\n'
                '[dependencies]\nbase64="=' + versions['base64'] + '"\n'
                'serde={version="=' + versions['serde'] + '",features=["derive"]}\n'
                'serde_json="=' + versions['serde_json'] + '"\n[workspace]\n')
    (folder / 'Cargo.toml').write_text(manifest)
    env = os.environ.copy()
    env['REASONIX_TERMINAL_BASELINE_RS'] = str(archived)
    run(['cargo', 'build', '--release', '--offline', '--manifest-path', folder / 'Cargo.toml'],
        folder / 'build.log', env)
    return folder / 'target/release/terminal-route-bench'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--baseline', default=BASE)
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    baseline = git('rev-parse', '--verify', args.baseline + '^{commit}')
    terminal_folder = output / 'terminal'
    terminal_folder.mkdir(exist_ok=True)
    run(['cargo', 'build', '--release', '--locked', '--offline', '--manifest-path',
         ROOT / 'desktop/tauri/Cargo.toml', '--example', 'history_decode_bench'],
        output / 'history-build.log')
    run(['cargo', 'build', '--release', '--locked', '--offline', '--manifest-path',
         ROOT / 'desktop/cua-runtime/Cargo.toml', '--example', 'preview_pipeline'],
        output / 'preview-build.log')
    terminal = prepare_terminal(terminal_folder, baseline)
    overlay = {}
    for name in ['ledger.go', 'live_state.go']:
        relative = 'internal/desktopbridge/' + name
        archived = output / ('baseline-' + name)
        archived.write_text(git('show', baseline + ':' + relative) + '\n')
        overlay[str(ROOT / relative)] = str(archived)
    overlay_path = output / 'baseline-overlay.json'
    overlay_path.write_text(json.dumps({'Replace': overlay}))
    # Finish compilation before any timed comparison. Timing is intentionally
    # serial; each Rust harness alternates old/new order across seven rounds.
    run(['go', 'test', '-mod=readonly', './internal/desktopbridge', '-run', '^$',
         '-bench', '^$'], output / 'go-warmup.log')
    run(['go', 'test', '-mod=readonly', '-overlay=' + str(overlay_path),
         './internal/desktopbridge', '-run', '^$', '-bench', '^$'],
        output / 'go-baseline-warmup.log')
    run([ROOT / 'desktop/tauri/target/release/examples/history_decode_bench'],
        output / 'history.json')
    run([terminal], output / 'terminal.json')
    bench = ['-run', '^$', '-bench', '^BenchmarkEvent(StreamTextBurst|LedgerAppend)$',
             '-benchmem', '-benchtime=200ms', '-count=1']
    go_samples = {'baseline': [], 'current': []}
    go_order = [('baseline', 'current'), ('current', 'baseline'), ('baseline', 'current')]
    for round_index, routes in enumerate(go_order):
        for route in routes:
            command = ['go', 'test', '-mod=readonly']
            if route == 'baseline':
                command.append('-overlay=' + str(overlay_path))
            log = output / f'go-{route}-{round_index}.log'
            run([*command, './internal/desktopbridge', *bench], log)
            go_samples[route].extend(benchmark_rows(log))
    env = os.environ.copy()
    env['REASONIX_PREVIEW_BENCH_BIN'] = str(ROOT / 'desktop/cua-runtime/target/release/examples/preview_pipeline')
    run(['go', 'test', '-mod=readonly', './internal/computeruse', '-run', '^$',
         '-bench', '^BenchmarkCuaPreviewPipeline$', '-benchmem', '-benchtime=30x', '-count=3'],
        output / 'preview-pipeline.log', env)
    inputs = [
        'internal/desktopbridge/ledger.go', 'internal/desktopbridge/live_state.go',
        'desktop/tauri/src/bridge/http_response.rs', 'desktop/tauri/src/terminal.rs',
        'desktop/cua-runtime/src/preview.rs', 'internal/computeruse/cua_preview.go',
        'internal/computeruse/cua_preview_bench_test.go',
    ]
    report = dict(baseline_commit=baseline, head=git('rev-parse', 'HEAD'),
                  worktree_dirty=bool(git('status', '--porcelain')),
                  platform=platform.platform(), machine=platform.machine(),
                  go=subprocess.check_output(['go', 'version'], text=True).strip(),
                  rust=subprocess.check_output(['rustc', '--version'], text=True).strip(),
                  source_sha256={p: hashlib.sha256((ROOT / p).read_bytes()).hexdigest() for p in inputs},
                  history=json.loads((output / 'history.json').read_text()),
                  terminal=json.loads((output / 'terminal.json').read_text()),
                  go_sample_order=go_order,
                  go_baseline=go_samples['baseline'],
                  go_current=go_samples['current'],
                  preview_pipeline=benchmark_rows(output / 'preview-pipeline.log'),
                  limits=[
                      'Synthetic public fixtures only; no models, network, user screenshots or input.',
                      'History/terminal timing excludes IPC, Go projection and renderer.',
                      'Preview includes a warm worker, full image JSON/pipe/Go decode and validation; excludes SDK capture and startup.',
                      'Allocation counters are cumulative bytes allocated, not retained heap or whole-application RSS.',
                      'Go allocation counters exclude the Rust worker; Rust counters belong to isolated single-thread benchmark binaries.',
                      'Wall-clock ratios describe these workloads on this machine, not universal language performance.',
                  ])
    (output / 'benchmark.json').write_text(json.dumps(report, indent=2) + '\n')
    print('Wrote ' + str(output / 'benchmark.json'), flush=True)


if __name__ == '__main__':
    main()
