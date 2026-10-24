#!/usr/bin/env python3
"""Build the source-pinned macOS adapter; optionally bundle the Cua edition."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import tomllib

ROOT = Path(__file__).resolve().parents[3]
RUNTIME = ROOT / 'desktop/cua-runtime'
CACHE = ROOT / 'desktop/cua-source'
PIN = json.loads((RUNTIME / 'upstream.json').read_text())


def run(args, **kwargs):
    return subprocess.run([str(arg) for arg in args], check=True, **kwargs)


def output(args, **kwargs):
    return run(args, capture_output=True, text=True, **kwargs).stdout.strip()


def prepare(source):
    marker = CACHE / '.reasonix-owned'
    if CACHE.exists() and not marker.exists():
        raise RuntimeError(f'Refusing to overwrite unowned source cache: {CACHE}')
    CACHE.mkdir(parents=True, exist_ok=True)
    marker.write_text('Source cache managed by desktop/cua-runtime/scripts/build.py\n')
    checkout = source or CACHE / '.checkout'
    if not checkout.exists():
        run(['git', 'clone', '--filter=blob:none', '--no-checkout', PIN['repository'], checkout])
        run(['git', '-C', checkout, 'sparse-checkout', 'set', PIN['scope']])
        run(['git', '-C', checkout, 'checkout', '--detach', PIN['revision']])
    if output(['git', '-C', checkout, 'rev-parse', 'HEAD']) != PIN['revision']:
        raise RuntimeError('Upstream source revision differs from upstream.json')
    if output(['git', '-C', checkout, 'status', '--porcelain', '--untracked-files=no']):
        raise RuntimeError('Upstream checkout contains tracked modifications')
    with tempfile.TemporaryDirectory(prefix='reasonix-cua-source-') as temp:
        archive = Path(temp) / 'source.tar'
        run(['git', '-C', checkout, 'archive', '--format=tar', f'--output={archive}', PIN['revision'], PIN['scope'], 'LICENSE.md'])
        extracted = Path(temp) / 'extracted'
        with tarfile.open(archive) as contents:
            contents.extractall(extracted, filter='data')
        destination = CACHE / PIN['scope']
        if destination.exists():
            shutil.rmtree(destination)
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copytree(extracted / PIN['scope'], destination)
        shutil.copy2(extracted / 'LICENSE.md', CACHE / 'LICENSE.md')
    # Explicit workspace membership prevents Cargo adopting sibling path crates
    # into our small wrapper workspace. Source behavior is unchanged.
    for manifest in destination.glob('crates/*/Cargo.toml'):
        content = manifest.read_text()
        manifest.write_text(content.replace('[package]', '[package]\nworkspace = "../.."', 1))
    (CACHE / 'reasonix-source.json').write_text(json.dumps(PIN, indent=2) + '\n')
    if (CACHE / 'LICENSE.md').read_bytes() != (RUNTIME / 'licenses/Cua-MIT.txt').read_bytes():
        raise RuntimeError('Pinned Cua MIT notice differs from shipped notice')


def notices(target):
    """Ship exact notices and corresponding unmodified MPL sources."""
    cargo_args = ['--manifest-path', RUNTIME / 'Cargo.toml', '--locked', '--offline']
    metadata = json.loads(output(['cargo', 'metadata', *cargo_args, '--format-version=1', '--filter-platform', target]))
    tree = output(['cargo', 'tree', *cargo_args, '--target', target, '--edges', 'normal,build', '--prefix', 'none', '--format', '{p}|{l}'])
    selected = set()
    for line in tree.splitlines():
        match = re.match(r'([^ ]+) v([^ |]+)', line)
        if match:
            selected.add(match.groups())
    folder = RUNTIME / 'generated-notices'
    if folder.exists():
        shutil.rmtree(folder)
    shutil.copytree(RUNTIME / 'licenses', folder)
    lock = tomllib.loads((RUNTIME / 'Cargo.lock').read_text())
    checksums = {(p['name'], p['version']): p.get('checksum') for p in lock['package']}
    index = []
    for package in sorted(metadata['packages'], key=lambda p: (p['name'], p['version'])):
        key = (package['name'], package['version'])
        if key not in selected:
            continue
        entry = {k: package.get(k) for k in ['name', 'version', 'license', 'repository']}
        if not entry['license'] or re.search(r'AGPL|GPL|FSL|BUSL', entry['license']):
            raise RuntimeError(f'Dependency needs a separate license review: {key}')
        entry['files'] = []
        if (package.get('source') or '').startswith('registry+'):
            source = Path(package['manifest_path']).parent
            files = [p for p in source.iterdir() if p.is_file() and p.name.lower().startswith(('license', 'licence', 'copying', 'notice', 'copyright'))]
            if package.get('license_file'):
                files.append(source / package['license_file'])
            for file in sorted(set(files)):
                if not file.is_file():
                    continue
                name = f"crates/{key[0]}-{key[1]}/{file.name}"
                dest = folder / name
                dest.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(file, dest)
                entry['files'].append(name)
            # MPL source is supplied alongside the executable, with no source
            # modifications. Crates without root notices include full source too.
            if 'MPL-2.0' in (package.get('license') or '') or not entry['files']:
                registry = source.parents[2]
                archive = registry / 'cache' / source.parent.name / f'{key[0]}-{key[1]}.crate'
                digest = hashlib.sha256(archive.read_bytes()).hexdigest()
                if digest != checksums[key]:
                    raise RuntimeError(f'Crate source checksum mismatch: {key}')
                dest = folder / 'corresponding-source' / archive.name
                dest.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(archive, dest)
                entry['source_archive'] = str(dest.relative_to(folder))
                entry['sha256'] = digest
        else:
            entry['files'] = ['Cua-MIT.txt'] if key[0] != 'reasonix-cua-runtime' else []
        index.append(entry)
    if {(p['name'], p['version']) for p in index} != selected:
        raise RuntimeError('Dependency notice inventory is incomplete')
    (folder / 'dependency-index.json').write_text(json.dumps({'upstream': PIN, 'target': target, 'dependencies': index}, indent=2) + '\n')
    (folder / 'README.txt').write_text('Reasonix Cua edition third-party notices\n\nCua Driver/SDK source: ' + PIN['repository'] + '\nCommit: ' + PIN['revision'] + '\n\nCua MIT and upstream notices are retained. Inter is SIL OFL.\nUniFFI components are MPL-2.0: corresponding unmodified source archives\nare supplied in corresponding-source/ with Cargo.lock checksums in the index.\nOther compiled dependencies and their notices are listed in dependency-index.json.\nReasonix wrapper changes do not modify these MPL source files.\n')
    return folder


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, help='existing clean checkout at the pinned revision')
    parser.add_argument('--bundle', action='store_true', help='also build the Cua macOS app and DMG')
    args = parser.parse_args()
    target = output(['rustc', '--print', 'host-tuple'])
    if not target.endswith('-apple-darwin'):
        raise RuntimeError('This first adapter is macOS-only')
    if f'const cuaSourceRevision = "{PIN["revision"]}"' not in (ROOT / 'internal/computeruse/cua_runtime.go').read_text():
        raise RuntimeError('Go host and Rust runtime source pins differ')
    prepare(args.source.resolve() if args.source else None)
    # --locked retains apple-metal 0.6.0, matching the pinned upstream lock;
    # newer versions need APIs missing from the supported Command Line Tools SDK.
    run(['cargo', 'build', '--manifest-path', RUNTIME / 'Cargo.toml', '--target-dir', RUNTIME / 'target', '--release', '--locked'])
    notices(target)
    binary = RUNTIME / 'target/release/reasonix-cua-runtime'
    destination = ROOT / f'desktop/tauri/binaries/reasonix-cua-runtime-{target}'
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(binary, destination)
    print(f'Cua {PIN["revision"]}: {destination}', flush=True)
    if args.bundle:
        run(['node', ROOT / 'desktop/frontend/scripts/tauri-build.mjs', '--config', 'tauri.cua.conf.json'], cwd=ROOT / 'desktop/frontend')


if __name__ == '__main__':
    main()
