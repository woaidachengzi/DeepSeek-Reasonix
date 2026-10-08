"""Recovery regressions using fake payloads; never accesses system clipboard."""
import contextlib
import importlib.util
import io
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

source = Path(os.environ.get("REASONIX_CLIPBOARD_FIXTURE_SOURCE", Path(__file__).with_name("native-clipboard-fixture.py")))
spec = importlib.util.spec_from_file_location("clipboard_fixture", source)
fixture_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixture_module)


class ClipboardRecoveryTests(unittest.TestCase):
    def test_invalid_image_modes_are_rejected_before_access(self):
        with tempfile.TemporaryDirectory(dir="/private/tmp") as temporary:
            for options in ({"image_format": "svg"}, {"image_format": "png", "expected_path": "/fixture"}):
                with self.assertRaises(ValueError):
                    fixture_module.NativeClipboardFixture(temporary, **options)

    def test_image_seed_failure_restores_even_when_command_raises(self):
        for failure in (1, subprocess.TimeoutExpired("helper", 15), OSError("private failure")):
            with self.subTest(failure=type(failure).__name__), tempfile.TemporaryDirectory(dir="/private/tmp") as temporary:
                fixture = fixture_module.NativeClipboardFixture(temporary, image_format="png")
                fixture.binary.touch()
                calls = []

                def command(action, argument):
                    calls.append(action)
                    if action == "capture-image":
                        fixture.snapshot.write_bytes(b"fake original clipboard")
                    if action == "seed-image":
                        if isinstance(failure, Exception):
                            raise failure
                        return subprocess.CompletedProcess([], failure)
                    return subprocess.CompletedProcess([], 0)

                fixture.command = command
                with contextlib.redirect_stdout(io.StringIO()), self.assertRaisesRegex(RuntimeError, "restoration attempted"):
                    fixture.__enter__()
                self.assertEqual(calls, ["selftest", "capture-image", "seed-image", "restore"])
                self.assertFalse(fixture.snapshot.exists())

    def test_image_capture_refusal_never_seeds(self):
        with tempfile.TemporaryDirectory(dir="/private/tmp") as temporary:
            fixture = fixture_module.NativeClipboardFixture(temporary, image_format="tiff")
            fixture.binary.touch()
            calls = []

            def command(action, argument):
                calls.append(action)
                return subprocess.CompletedProcess([], 0 if action == "selftest" else 2)

            fixture.command = command
            with self.assertRaisesRegex(RuntimeError, "no clipboard write was performed"):
                fixture.__enter__()
            self.assertEqual(calls, ["selftest", "capture-image"])

    def test_snapshot_refusal_reports_fixed_category_without_content_or_write(self):
        for status, reason in ((1, "snapshot unavailable"), (2, "clipboard changed"),
                               (3, "cannot round-trip"), (9, "snapshot helper failed")):
            with self.subTest(status=status), tempfile.TemporaryDirectory(dir="/private/tmp") as temporary:
                fixture = fixture_module.NativeClipboardFixture(temporary)
                fixture.binary.touch()
                calls = []

                def command(action, argument):
                    calls.append(action)
                    return subprocess.CompletedProcess([], 0 if action == "selftest" else status,
                                                       stderr="private clipboard content must not escape")

                fixture.command = command
                with self.assertRaisesRegex(RuntimeError, reason) as error:
                    fixture.__enter__()
                self.assertNotIn("private clipboard", str(error.exception))
                self.assertIn(f"exit {status}", str(error.exception))
                self.assertIn("no clipboard write was performed", str(error.exception))
                self.assertEqual(calls, ["selftest", "capture"])
                self.assertFalse((Path(temporary) / "reasonix-native-clipboard-control.json").exists())

    def exercise(self, status, interrupted):
        with tempfile.TemporaryDirectory(dir="/private/tmp") as temporary:
            root = Path(temporary)
            caller = root / "caller"
            caller.mkdir(mode=0o700)
            recovery = root / "recovery"
            fixture = fixture_module.NativeClipboardFixture(caller)
            payload = b"fake original multi-format clipboard bytes"
            fixture.snapshot.write_bytes(payload)
            fixture.snapshot.chmod(0o600)
            fixture.marker.write_text('{"nonce":"fixture","count":7}')
            fixture.marker.chmod(0o600)
            fixture.command = lambda *_: subprocess.CompletedProcess([], status)

            def retain(**_):
                recovery.mkdir(mode=0o700)
                return str(recovery)

            with patch.object(fixture_module.tempfile, "mkdtemp", side_effect=retain), contextlib.redirect_stdout(io.StringIO()):
                if (status == 2 and not interrupted) or status == 3:
                    with self.assertRaisesRegex(RuntimeError, "recovery snapshot retained"):
                        fixture.__exit__(None)
                else:
                    self.assertFalse(fixture.__exit__(RuntimeError if interrupted else None))
            # Caller cleanup must not remove the only original backup when
            # restoration declined due to a newer pasteboard generation.
            shutil.rmtree(caller)
            if status in (2, 3):
                self.assertEqual((recovery / "original.plist").read_bytes(), payload)
                self.assertEqual((recovery / "original.plist").stat().st_mode & 0o777, 0o600)
                self.assertEqual(recovery.stat().st_mode & 0o777, 0o700)
                self.assertTrue((recovery / "owned.json").is_file())
            else:
                self.assertFalse(recovery.exists())

    def test_interrupted_changed_generation_keeps_recovery_after_caller_cleanup(self):
        self.exercise(2, True)

    def test_completed_changed_generation_reports_and_retains_recovery(self):
        self.exercise(2, False)

    def test_restored_generation_needs_no_recovery_copy(self):
        self.exercise(0, False)

    def test_unverified_restoration_retains_original_after_caller_cleanup(self):
        self.exercise(3, False)


if __name__ == "__main__":
    unittest.main()
