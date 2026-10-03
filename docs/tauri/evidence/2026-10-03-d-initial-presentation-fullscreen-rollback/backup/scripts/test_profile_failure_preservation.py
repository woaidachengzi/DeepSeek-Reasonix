"""Evidence stays private and survives failure, while successful fixtures clean up."""
import contextlib
import importlib.util
import io
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('profiles', Path(__file__).with_name('smoke-profile-import.py'))
profiles = importlib.util.module_from_spec(spec)
spec.loader.exec_module(profiles)


class ProfileFailurePreservationTests(unittest.TestCase):
    def test_failed_fixture_retains_original_bytes_and_success_cleans_up(self):
        failed = None
        try:
            with contextlib.redirect_stderr(io.StringIO()) as errors:
                with self.assertRaisesRegex(RuntimeError, 'acceptance failed'):
                    with profiles.private_fixture(retain_failed=True) as failed:
                        (failed / 'receipt.json').write_bytes(b'{"ok":false}')
                        raise RuntimeError('acceptance failed')
            self.assertTrue(failed.is_dir())
            self.assertEqual(failed.stat().st_mode & 0o777, 0o700)
            self.assertEqual((failed / 'receipt.json').read_bytes(), b'{"ok":false}')
            self.assertIn(str(failed), errors.getvalue())
            with profiles.private_fixture(retain_failed=True) as success:
                (success / 'receipt.json').write_bytes(b'{"ok":true}')
            self.assertFalse(success.exists())
        finally:
            if failed and failed.exists():
                profiles.shutil.rmtree(failed)


if __name__ == '__main__':
    unittest.main()
