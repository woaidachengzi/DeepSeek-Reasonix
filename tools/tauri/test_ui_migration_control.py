#!/usr/bin/env python3
"""Private lifecycle receipt tests; no native launch or user profile access."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    'migration', Path(__file__).with_name('smoke-native-ui-migration.py'))
migration = importlib.util.module_from_spec(spec)
spec.loader.exec_module(migration)


class ControlReceiptTests(unittest.TestCase):
    def test_lifecycle_replaces_stale_running_receipt_privately(self):
        with tempfile.TemporaryDirectory() as directory:
            control = Path(directory) / 'control.json'
            payload = {'hostPid': 123, 'phase': 'before-import', 'state': 'running',
                       'webviewOrigin': 'reasonix-preview://0123456789abcdef0123456789abcdef.localhost/'}
            migration.publish_control(control, payload)
            self.assertEqual(json.loads(control.read_text()), payload)
            for state in ('ending', 'ended'):
                expected = payload | {'state': state}
                migration.publish_control(control, expected)
                self.assertEqual(json.loads(control.read_text()), expected)
                self.assertEqual(control.stat().st_mode & 0o777, 0o600)
            self.assertEqual(list(Path(directory).iterdir()), [control])


if __name__ == '__main__':
    unittest.main()
