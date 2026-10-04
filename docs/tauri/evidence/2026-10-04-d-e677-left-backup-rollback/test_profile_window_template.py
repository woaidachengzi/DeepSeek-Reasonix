"""Reject unexpected placement state before spawning an import host."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("profile_import", Path(__file__).with_name("smoke-profile-import.py"))
profiles = importlib.util.module_from_spec(spec)
spec.loader.exec_module(profiles)


class ProfileWindowTemplateTests(unittest.TestCase):
    def test_existing_different_or_extended_state_is_not_overwritten_or_launched(self):
        state = dict(width=2560, height=1640, x=-3200, y=142, scale_factor=2.0, maximized=False)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            app_data = root / "home/Library/Application Support/io.reasonix.desktop.preview"
            app_data.mkdir(parents=True)
            saved = app_data / "window-state.json"
            for different in [dict(state, x=0), dict(state, token="not-a-placement-field")]:
                saved.write_text(json.dumps(different))
                original = saved.read_bytes()
                with patch.object(profiles.subprocess, "Popen") as spawn:
                    with self.assertRaises((RuntimeError, ValueError)):
                        profiles.launch(Path("/nonexistent/app"), "io.reasonix.desktop.preview", root,
                                        "restore", b"config sentinel", state)
                    spawn.assert_not_called()
                self.assertEqual(saved.read_bytes(), original)
                self.assertFalse((app_data / "reasonix-core").exists())

    def test_existing_symlink_is_rejected_before_launch(self):
        state = dict(width=2560, height=1640, x=-3200, y=142, scale_factor=2.0, maximized=False)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            app_data = root / "home/Library/Application Support/io.reasonix.desktop.preview"
            app_data.mkdir(parents=True)
            target = root / "original.json"
            target.write_text(json.dumps(state))
            (app_data / "window-state.json").symlink_to(target)
            with patch.object(profiles.subprocess, "Popen") as spawn:
                with self.assertRaises(RuntimeError):
                    profiles.launch(Path("/nonexistent/app"), "io.reasonix.desktop.preview", root,
                                    "import", b"config sentinel", state)
                spawn.assert_not_called()
            self.assertEqual(json.loads(target.read_text()), state)


if __name__ == "__main__":
    unittest.main()
