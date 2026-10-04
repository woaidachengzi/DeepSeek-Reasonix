"""Placement templates cannot import profile content or overwrite saved state."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("launch_profile", Path(__file__).with_name("probe-launch-services-profile.py"))
profile = importlib.util.module_from_spec(spec)
spec.loader.exec_module(profile)


class WindowTemplateTests(unittest.TestCase):
    def test_negative_monitor_coordinates_and_private_non_overwriting_seed(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            template = root / "template.json"
            state = dict(width=2560, height=1640, x=-3200, y=142, scale_factor=2.0, maximized=False)
            template.write_text(json.dumps(state))
            original = template.read_bytes()
            parsed = profile.read_window_state_template(template)
            destination = root / "home/Library/Application Support/preview"
            profile.seed_window_state(destination, parsed)
            saved = destination / "window-state.json"
            self.assertEqual(json.loads(saved.read_text()), state)
            self.assertEqual(saved.stat().st_mode & 0o777, 0o600)
            self.assertEqual(destination.stat().st_mode & 0o777, 0o700)
            self.assertEqual(template.read_bytes(), original)
            with self.assertRaises(FileExistsError):
                profile.seed_window_state(destination, dict(state, x=0))
            self.assertEqual(json.loads(saved.read_text()), state)

    def test_rejects_profile_content_invalid_geometry_and_symlink(self):
        with tempfile.TemporaryDirectory() as folder:
            template = Path(folder) / "template.json"
            valid = dict(width=2560, height=1640, x=-3200, y=142, scale_factor=2.0, maximized=False)
            for patch in ({"token": "do-not-import"}, {"width": True}, {"x": 2**31},
                          {"scale_factor": float("nan")}, {"scale_factor": float("inf")},
                          {"maximized": True}, {"height": 0}):
                with self.subTest(patch=patch):
                    template.write_text(json.dumps(dict(valid, **patch)))
                    with self.assertRaises(ValueError):
                        profile.read_window_state_template(template)
            template.write_text(json.dumps(valid))
            link = Path(folder) / "link.json"
            link.symlink_to(template)
            with self.assertRaises(ValueError):
                profile.read_window_state_template(link)


if __name__ == "__main__":
    unittest.main()
