#!/usr/bin/env python3
"""Build (never grant permissions or run) the local Cua acceptance fixture."""
import plistlib
from pathlib import Path
import shutil
import subprocess

repo = Path(__file__).resolve().parents[3]
root = repo / 'desktop/tauri/target/cua-acceptance.noindex'
app = root / 'Reasonix Cua QA.app'
contents = app / 'Contents'
binary = contents / 'MacOS/Reasonix Cua QA'
binary.parent.mkdir(parents=True, exist_ok=True)
runtime = repo / 'desktop/cua-runtime/target/release/reasonix-cua-runtime'
if not runtime.is_file():
    raise SystemExit('Build desktop/cua-runtime/scripts/build.py first')
shutil.copy2(runtime, binary.parent / 'reasonix-cua-runtime')
(contents / 'Info.plist').write_bytes(plistlib.dumps({
    'CFBundleIdentifier': 'io.reasonix.cua.qa',
    'CFBundleExecutable': binary.name,
    'CFBundleName': 'Reasonix Cua QA',
    'CFBundlePackageType': 'APPL',
    'CFBundleVersion': '1',
    'NSHighResolutionCapable': True,
    'NSScreenCaptureUsageDescription': '仅验证本地验收窗口截图。',
}))
subprocess.run(['swiftc', '-module-cache-path', str(root / 'swift-cache'),
                str(Path(__file__).with_name('cua_acceptance.swift')), '-o', str(binary),
                '-framework', 'AppKit', '-framework', 'WebKit'], check=True)
resources = contents / 'Resources'
resources.mkdir(exist_ok=True)
shutil.copy2(Path(__file__).with_name('browser_acceptance.py'), resources / 'browser_acceptance.py')
subprocess.run(['swiftc', '-module-cache-path', str(root / 'swift-cache'),
                str(Path(__file__).with_name('browser_fixture_info.swift')),
                '-o', str(binary.parent / 'browser-fixture-info'), '-framework', 'AppKit'], check=True)
for path in [binary.parent / 'reasonix-cua-runtime', binary.parent / 'browser-fixture-info', app]:
    subprocess.run(['codesign', '--force', '--sign', '-', str(path)], check=True)
subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)
print(app)
print('After explicit temporary consent, launch this exact app. DOM testing needs Accessibility; native screenshots also need Screen Recording.')
print('Write test, web, coordinates, second-screen or browser to /private/tmp/reasonix-cua-qa-command.')
print('Read /private/tmp/reasonix-cua-qa-results.jsonl; remove both temporary grants after testing.')
