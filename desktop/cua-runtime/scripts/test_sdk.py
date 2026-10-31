#!/usr/bin/env python3
"""Run pinned SDK unit regressions without the excluded upstream CLI workspace."""
import json
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import tomllib

import build


def path_dependencies(table, folder, base):
    for key, value in table.items():
        if isinstance(value, dict):
            yield from path_dependencies(value, folder, base)
        elif key == 'path' and isinstance(value, str):
            manifest = (folder / value / 'Cargo.toml').resolve()
            if not manifest.is_file():
                continue
            if manifest.parent.parent != base / 'crates':
                raise RuntimeError(f'Test dependency is outside the pinned crates: {manifest}')
            yield manifest.parent.name


def export_workspace(base, destination, seeds):
    pending, seen = list(seeds), set()
    while pending:
        name = pending.pop()
        if name in seen:
            continue
        seen.add(name)
        crate = base / 'crates' / name
        pending.extend(path_dependencies(tomllib.loads((crate / 'Cargo.toml').read_text()), crate, base))
        shutil.copytree(crate, destination / 'crates' / name)
    source = (base / 'Cargo.toml').read_text()
    members = json.dumps(['crates/' + name for name in sorted(seen)])
    manifest = '[workspace]\nresolver = "2"\nmembers = ' + members + '\n\n[workspace.package]'
    manifest += source.split('[workspace.package]', 1)[1]
    (destination / 'Cargo.toml').write_text(manifest)
    shutil.copytree(base / 'Skills', destination / 'Skills')
    shutil.copytree(base.parent / 'wayland-helper', destination.parent / 'wayland-helper')


def registry_packages(lock):
    return {(p['name'], p['version'], p['source'], p['checksum'])
            for p in tomllib.loads(lock.read_text())['package']
            if p.get('source', '').startswith('registry+')}


def main():
    host = build.output(['rustc', '--print', 'host-tuple'])
    platform = 'platform-macos' if host.endswith('-apple-darwin') else 'platform-linux'
    if platform == 'platform-linux' and not host.endswith('-unknown-linux-gnu'):
        raise RuntimeError('SDK platform regressions require native macOS or GNU/Linux')
    if json.loads((build.CACHE / 'reasonix-source.json').read_text()) != build.PIN:
        raise RuntimeError('Build a clean pinned export with build.py first')
    core = [('cua-driver-contract', ''), ('cua-driver-core', 'image_utils::tests'),
            ('cua-driver-core', 'post_action_wait'), ('cua-driver-core', 'before_reading_the_wait'),
            ('cua-driver-core', 'scroll')]
    patterns = ['capture::tests', 'tools::get_window_state::tests',
                'window_change_detector::tests', 'finder_is_located_by_name_outside_the_app_roots']
    if platform == 'platform-linux':
        patterns = ['wayland::tests', 'wayland::libei::tests']
    features = ['--features', 'platform-linux/portal-libei'] if platform == 'platform-linux' else []
    with tempfile.TemporaryDirectory(prefix='reasonix-sdk-tests-') as temp:
        workspace = Path(temp) / 'libs/cua-driver/rust'
        export_workspace(build.CACHE / build.PIN['scope'], workspace, [platform])
        lock = workspace / 'Cargo.lock'
        shipped_lock = build.RUNTIME / 'Cargo.lock'
        shutil.copy2(shipped_lock, lock)
        args = ['--manifest-path', workspace / 'Cargo.toml', '--offline']
        build.output(['cargo', 'metadata', *args, *features, '--format-version=1', '--filter-platform', host])
        if not registry_packages(lock).issubset(registry_packages(shipped_lock)):
            raise RuntimeError('Test workspace resolved registry dependencies outside the shipped lock')
        tests = core + [(platform, pattern) for pattern in patterns]
        for package, pattern in tests:
            selected_features = ['--features', 'portal-libei'] if package == 'platform-linux' else []
            result = subprocess.run(['cargo', 'test', *map(str, args), '--locked', '--target-dir',
                                     str(build.RUNTIME / 'target'), '-p', package, *selected_features, '--lib', pattern],
                                    capture_output=True, text=True)
            print(result.stdout + result.stderr, flush=True)
            result.check_returncode()
            if not any(int(n) > 0 for n in re.findall(r'test result: ok\. (\d+) passed', result.stdout)):
                raise RuntimeError(f'No regressions matched {package}: {pattern}')


if __name__ == '__main__':
    main()
