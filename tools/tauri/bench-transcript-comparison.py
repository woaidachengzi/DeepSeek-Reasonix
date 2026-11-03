#!/usr/bin/env python3
"""Serial alternating native-renderer comparison of two existing production builds."""
import argparse
import hashlib
import json
from pathlib import Path
import statistics
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
SOURCES = [
    'desktop/frontend/src/components/TranscriptBlockView.tsx',
    'desktop/frontend/src/components/TranscriptWindow.tsx',
    'desktop/frontend/src/lib/transcriptMeasurementLedger.ts',
    'tools/tauri/bench-transcript-native.py', 'tools/tauri/bench-transcript-native.swift',
    'tools/tauri/bench-transcript-native.js', 'tools/tauri/bench-transcript-comparison.py',
]


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--baseline-dist', required=True, type=Path)
    parser.add_argument('--baseline-commit', required=True)
    parser.add_argument('--candidate-dist', type=Path, default=ROOT / 'desktop/frontend/dist')
    parser.add_argument('--rounds', type=int, default=5)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    if args.rounds < 1:
        parser.error('rounds must be positive')
    builds = dict(baseline=args.baseline_dist.resolve(), candidate=args.candidate_dist.resolve())
    if builds['baseline'] == builds['candidate']:
        parser.error('baseline and candidate must be independent build directories')
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    sources = {name: digest(ROOT / name) for name in SOURCES}
    assets = {route: {str(p.relative_to(directory)): digest(p) for p in sorted(directory.rglob('*')) if p.is_file()}
              for route, directory in builds.items()}
    samples = dict(baseline=[], candidate=[])
    warmup = {}
    for round_index in range(-1, args.rounds):
        order = ['baseline', 'candidate'] if round_index % 2 == 0 else ['candidate', 'baseline']
        for route in order:
            directory = output / f'{route}-{round_index}'
            with (output / f'{route}-{round_index}.log').open('w') as log:
                subprocess.run([sys.executable, str(ROOT / 'tools/tauri/bench-transcript-native.py'),
                    '--output', str(directory), '--rounds', '1', '--frontend-dist', str(builds[route])],
                    cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=True)
            report = json.loads((directory / 'measurement.json').read_text())
            if report['asset_sha256'] != assets[route]:
                raise RuntimeError(f'{route} assets changed during measurement')
            sample = report['samples'][0]
            if round_index < 0:
                warmup[route] = sample
            else:
                samples[route].append(sample)
    medians = {}
    for route, batches in samples.items():
        landings = [t['landingMs'] for sample in batches for t in sample['transitions']]
        medians[route] = dict(
            navigation_landing_ms=statistics.median(landings), max_navigation_landing_ms=max(landings),
            median_run_raf_p95_ms=statistics.median(s['rafIntervalMs']['p95'] for s in batches),
            max_raf_interval_ms=max(s['rafIntervalMs']['max'] for s in batches),
            raf_intervals_over_50_ms=sum(s['rafIntervalMs']['over50'] for s in batches),
            frames=sum(s['frameCount'] for s in batches), empty_frames=sum(s['blankFrames'] for s in batches),
            max_mounted=max(s['maxMounted'] for s in batches))
    for name, expected in sources.items():
        if digest(ROOT / name) != expected:
            raise RuntimeError(f'source changed during measurement: {name}')
    for route, directory in builds.items():
        if {str(p.relative_to(directory)): digest(p) for p in directory.rglob('*') if p.is_file()} != assets[route]:
            raise RuntimeError(f'{route} build changed during measurement')
    result = dict(baseline_commit=args.baseline_commit,
        candidate_commit=subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
        candidate_status=subprocess.check_output(['git', 'status', '--short'], cwd=ROOT, text=True).strip(),
        source_sha256=sources, asset_sha256=assets, warmup=warmup, samples=samples, medians=medians,
        scope='Fresh nonpersistent native WKWebViews, fixed synthetic 1K-turn fixture, nine product DOM question-navigation transactions per run. Serial alternating builds; warm-up excluded. Geometry probe included. rAF interval is scheduling cadence, not CPU/paint/FPS. No physical input, real profile, model, capture or Tauri IPC timing.')
    (output / 'comparison.json').write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(medians, indent=2))


if __name__ == '__main__':
    main()
