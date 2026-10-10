#!/usr/bin/env python3
"""Synthetic fixture contract only; never launches an app or claims native QA."""
import importlib.util
import os
from pathlib import Path
import plistlib
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('bot_ui', Path(__file__).with_name('smoke-native-bot-diagnostics.py'))
bot_ui = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bot_ui)


class FixtureTests(unittest.TestCase):
    def test_running_preview_refused_before_signing_or_launch_or_fixture_write(self):
        with tempfile.TemporaryDirectory() as name:
            app = Path(name) / 'Preview.app'
            (app / 'Contents').mkdir(parents=True)
            (app / 'Contents/Info.plist').write_bytes(plistlib.dumps(
                {'CFBundleIdentifier': 'io.reasonix.desktop.preview'}))
            with patch.object(bot_ui.package, 'matching_package_is_running', return_value=True), \
                    patch.object(bot_ui.subprocess, 'run') as command, \
                    patch.object(bot_ui.tempfile, 'mkdtemp') as fixture, \
                    patch.object(bot_ui.launch_services, 'LaunchServicesHost') as launch:
                with self.assertRaisesRegex(RuntimeError, 'another instance is running'):
                    bot_ui.smoke(app, 'both', 300)
                command.assert_not_called()
                fixture.assert_not_called()
                launch.assert_not_called()

    def test_gateway_off_and_no_ambient_secrets_or_smoke_flags(self):
        for managed in (True, False):
            with self.subTest(managed=managed), tempfile.TemporaryDirectory() as name, patch.dict(
                    os.environ, {'PROVIDER_API_KEY': 'canary', 'HTTP_PROXY': 'canary',
                                 'REASONIX_HOME': '/real-profile',
                                 'REASONIX_TAURI_NATIVE_WINDOW_SMOKE': 'exercise'}, clear=True):
                root = Path(name)
                core, config, env = bot_ui.prepare(root, managed)
                self.assertEqual(config, core / 'config.toml')
                self.assertEqual(config.stat().st_mode & 0o777, 0o600)
                self.assertEqual(root.stat().st_mode & 0o777, 0o700)
                self.assertIn('[bot]\nenabled = false\n', config.read_text())
                self.assertIn('[bot.feishu]\nenabled = true\n', config.read_text())
                self.assertNotIn('canary', config.read_text())
                self.assertEqual(set(env), {'HOME', 'TMPDIR'} if managed else
                                 {'HOME', 'TMPDIR', 'REASONIX_HOME', 'REASONIX_CACHE_HOME'})
                if not managed:
                    self.assertEqual(env['REASONIX_HOME'], str(core))
                    self.assertNotIn('REASONIX_STATE_HOME', env)
                self.assertEqual(bot_ui.launch_services.launch_phase(
                    env, ordinary_bot_diagnostics=True), 'ui-bot-diagnostics')


if __name__ == '__main__':
    unittest.main()
