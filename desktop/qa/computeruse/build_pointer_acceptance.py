#!/usr/bin/env python3
"""Build a disposable app for explicit local-only input acceptance.

No permission requests or input are sent by this builder. After granting the
app Accessibility, launch it through LaunchServices. It clicks only its own
native button and inline WKWebView page, on at most two displays, then exits.
With --fill it instead tests only its own text fields, refusals and readback.
"""
import pathlib
import argparse
import plistlib
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--fill', action='store_true', help='test local AX text fill, readback, refusals and idempotence')
    args = parser.parse_args()
    source = pathlib.Path(__file__).resolve().parent
    repo = source.parents[2]
    # LaunchServices/TCC may not resolve an application stored in macOS's
    # ephemeral temp directory. Keep the disposable build in ignored target/.
    target = repo / 'desktop/tauri/target'
    target.mkdir(parents=True, exist_ok=True)
    root = pathlib.Path(tempfile.mkdtemp(prefix='pointer-acceptance-', dir=target))
    app = root / 'Reasonix Pointer QA.app'
    binary = app / 'Contents/MacOS/pointer-qa'
    binary.parent.mkdir(parents=True)
    (app / 'Contents/Info.plist').write_bytes(plistlib.dumps({
        'CFBundleIdentifier': 'io.reasonix.pointer.qa.local',
        'CFBundleName': 'Reasonix Pointer QA', 'CFBundleExecutable': 'pointer-qa',
        'CFBundlePackageType': 'APPL', 'CFBundleVersion': '1',
        'NSHighResolutionCapable': True,
    }))
    subprocess.run([
        'xcrun', '--sdk', 'macosx', 'clang', '-fobjc-arc', '-Wno-deprecated-declarations',
        '-framework', 'AppKit', '-framework', 'ApplicationServices',
        '-framework', 'ImageIO', '-framework', 'WebKit',
        str(source / ('native_fill_acceptance.m' if args.fill else 'native_pointer_acceptance.m')),
        str(repo / 'internal/computeruse/native_pointer_darwin.m'), '-o', str(binary),
    ], check=True)
    subprocess.run(['codesign', '--force', '--sign', '-', str(app)], check=True)
    subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)
    subprocess.run([
        '/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister',
        '-f', str(app),
    ], check=True)
    print(f'App: {app}')
    print(f'Results: {root / "result.json"}')
    print('Read-only preflight: open -n -W -a "' + str(app) + '" --args --preflight')
    print('After explicitly granting Accessibility: open -n -W -a "' + str(app) + '"')
    print('Remove the temporary Accessibility entry after acceptance.')


if __name__ == '__main__':
    main()
