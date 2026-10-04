"""Failure evidence survives later fixture reuse; fake files, no native UI."""
import contextlib
import importlib.util
import io
from pathlib import Path
import plistlib
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('fullscreen', Path(__file__).with_name('smoke-native-fullscreen.py'))
fullscreen = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fullscreen)


class FullscreenReceiptTests(unittest.TestCase):
    def test_raw_failure_survives_reuse_without_copying_private_profile(self):
        with tempfile.TemporaryDirectory(dir='/private/tmp') as base:
            base = Path(base)
            app = base / 'Preview.app'
            (app / 'Contents').mkdir(parents=True)
            (app / 'Contents/Info.plist').write_bytes(plistlib.dumps({'CFBundleIdentifier': 'fixture.preview'}))
            root = base / 'fixture'
            root.mkdir(mode=0o700)
            payloads = {'reasonix-native-window-result.json': b'{"ok":false}',
                        'reasonix-native-window-trace.jsonl': b'{"stage":"failed"}\n',
                        'reasonix-native-window-normal.json': b'{"width":2000}',
                        'launch-services-exercise-exit.json': b'{"exitCode":2}'}

            def fail(*args, **kwargs):
                for name, data in payloads.items():
                    (root / 'tmp' / name).write_bytes(data)
                (root / 'home/private-profile').write_bytes(b'private data must stay private')
                raise RuntimeError('original minimize failure')

            with patch.object(fullscreen.windows.package, 'matching_package_is_running', return_value=False), \
                 patch.object(fullscreen.tempfile, 'mkdtemp', return_value=str(root)), \
                 patch.object(fullscreen.windows, 'launch', side_effect=fail) as launch, \
                 contextlib.redirect_stderr(io.StringIO()):
                with self.assertRaisesRegex(RuntimeError, 'original minimize failure'):
                    fullscreen.smoke(app)
            self.assertEqual(launch.call_count, 1)  # Never retry a failed action.
            receipt = root / 'failed-exercise'
            self.assertEqual(receipt.stat().st_mode & 0o777, 0o700)
            self.assertEqual({p.name for p in receipt.iterdir()}, set(payloads))
            for name, data in payloads.items():
                (root / 'tmp' / name).write_bytes(b'later phase overwrote the live file')
                self.assertEqual((receipt / name).read_bytes(), data)
            self.assertFalse((receipt / 'private-profile').exists())


if __name__ == '__main__':
    unittest.main()
