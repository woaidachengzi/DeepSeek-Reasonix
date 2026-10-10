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
    def test_launch_discards_prior_capture_receipt_before_starting_owned_host(self):
        with tempfile.TemporaryDirectory(prefix="reasonix-capture-receipt-unit-", dir="/private/tmp") as directory:
            root = Path(directory)
            (root / "home").mkdir()
            temporary = root / "tmp"
            temporary.mkdir()
            stale = temporary / "reasonix-native-window-queued-capture.json"
            stale.write_text('{"preserved":true}')
            normal = temporary / "reasonix-native-window-normal.json"
            normal.write_text("owned normal geometry")
            with patch.object(terminal.windows.launch_services_host, "LaunchServicesHost",
                              side_effect=RuntimeError("owned launch intercepted")):
                with self.assertRaisesRegex(RuntimeError, "owned launch intercepted"):
                    terminal.windows.launch(root / "host", root / "sidecar", root,
                                            "io.reasonix.desktop.preview", True, "restore-normal",
                                            environment={}, launch_services=True)
            self.assertFalse(stale.exists())
            self.assertEqual(normal.read_text(), "owned normal geometry")

    def test_saved_geometry_rejects_observed_primary_and_secondary_drift(self):
        requested = {"width": 2400, "height": 1600, "x": 200, "y": 120,
                     "scale_factor": 2, "maximized": False}
        terminal.require_saved_geometry(dict(requested), requested)
        terminal.require_saved_geometry({**requested, "scale_factor": 2.0}, requested)
        with self.assertRaisesRegex(RuntimeError, "escaped requested"):
            terminal.require_saved_geometry({**requested, "x": 468}, requested)
        secondary = {**requested, "x": -3400}
        with self.assertRaisesRegex(RuntimeError, "escaped requested"):
            terminal.require_saved_geometry({**secondary, "x": -3372}, secondary)
        for actual in [None, {}, {**requested, "unexpected": True}]:
            with self.subTest(actual=actual), self.assertRaises(RuntimeError):
                terminal.require_saved_geometry(actual, requested)

    def test_primary_display_requires_explicit_opt_in(self):
        terminal.require_placement({"x": -1200}, False)
        terminal.require_placement({"x": 100}, True)
        for x in (0, 100):
            with self.subTest(x=x), self.assertRaisesRegex(ValueError, "left-display"):
                terminal.require_placement({"x": x}, False)
        for value in (None, 1, "true"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                terminal.require_placement({"x": 100}, value)

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
                for primary in (False, True):
                    with self.subTest(primary=primary), self.assertRaisesRegex(RuntimeError, "already running"):
                        terminal.smoke(app, "not-read.json", "managed", primary)
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
