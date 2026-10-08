"""Read-only/unit gates; do not launch a GUI, shell or clipboard operation."""
import importlib.util
from pathlib import Path
import plistlib
import tempfile
import unittest
from unittest.mock import patch


def module(name, file):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(file))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


terminal = module("integrated_terminal", "smoke-native-integrated-terminal.py")
launcher = module("terminal_launcher", "launch-services-host.py")


def receipt():
    return {**{key: True for key in ("ok", "realWebViewIPC", "xtermInputEvent", "realTTY", "utf8AnsiPainted",
                                    "explicitCreateOnly", "collapseSamePTY", "explicitContextOnly", "closePIDGone", "switchPIDGone", "noModelHistory")},
            "shutdownPID": 12345}


class NativeTerminalGates(unittest.TestCase):
    def test_fixed_boolean_receipt_and_pid_bounds(self):
        self.assertEqual(terminal.validate_receipt(receipt()), 12345)
        for key in receipt():
            for value in (None, False, "true", 1 if key != "shutdownPID" else True):
                changed = receipt()
                changed[key] = value
                with self.subTest(key=key, value=value), self.assertRaises(RuntimeError):
                    terminal.validate_receipt(changed)
        for pid in (-1, 0, 1, 2**31):
            with self.subTest(pid=pid), self.assertRaises(RuntimeError):
                terminal.validate_receipt({**receipt(), "shutdownPID": pid})

    def test_busy_preview_refuses_before_placement_or_launch(self):
        with tempfile.TemporaryDirectory(prefix="reasonix-terminal-guard-", dir="/private/tmp") as directory:
            app = Path(directory) / "owned.app"
            (app / "Contents").mkdir(parents=True)
            (app / "Contents/Info.plist").write_bytes(plistlib.dumps({"CFBundleIdentifier": "io.reasonix.desktop.preview"}))
            with patch.object(terminal.windows.package, "matching_package_is_running", return_value=True), \
                    patch.object(terminal.windows.package, "placement_helper") as placement, \
                    patch.object(terminal.windows, "launch") as launch:
                with self.assertRaisesRegex(RuntimeError, "already running"):
                    terminal.smoke(app, "not-read.json", "managed")
                placement.assert_not_called()
                launch.assert_not_called()

    def test_shell_environment_is_fixed_owned_and_phase_limited(self):
        root = Path("/private/tmp/owned-terminal-fixture")
        expected = {"SHELL": "/bin/sh", "ENV": "/dev/null", "BASH_ENV": "/dev/null", "ZDOTDIR": str(root / "home")}
        self.assertEqual(launcher.owned_terminal_shell_environment("ui-integrated-terminal", root, {**expected, "PRIVATE_SENTINEL": "not-forwarded"}), expected)
        self.assertEqual(launcher.owned_terminal_shell_environment("ui-image-paste", root, expected), {})
        for key in expected:
            with self.subTest(key=key), self.assertRaises(ValueError):
                launcher.owned_terminal_shell_environment("ui-integrated-terminal", root, {**expected, key: "/foreign"})


if __name__ == "__main__":
    unittest.main()
