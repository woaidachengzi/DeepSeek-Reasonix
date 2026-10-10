#!/usr/bin/env python3
"""No app launch: validate private migration environment admission only."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('launch_services', Path(__file__).with_name('launch-services-host.py'))
launch_services = importlib.util.module_from_spec(spec)
spec.loader.exec_module(launch_services)


class LegacyEnvironmentTests(unittest.TestCase):
    def test_only_fixed_phase_and_private_source_are_admitted(self):
        flag = 'REASONIX_TAURI_LEGACY_UI_SMOKE'
        for phase in launch_services.UI_MIGRATION_PHASES:
            self.assertEqual(launch_services.private_legacy_ui_environment(
                phase, {flag: 'private-source', 'PROVIDER_API_KEY': 'private-canary'}),
                {flag: 'private-source'})
        for phase, env in [(None, {flag: 'private-source'}), ('other', {flag: 'private-source'}),
                           ('before-import', {}), ('before-import', {flag: '/real/profile'})]:
            with self.assertRaises(ValueError):
                launch_services.private_legacy_ui_environment(phase, env)

    def test_existing_native_launches_do_not_inherit_legacy_source(self):
        self.assertEqual(launch_services.private_legacy_ui_environment(None, {'HOME': '/owned'}), {})

    def test_ordinary_bot_phase_has_no_application_smoke_flag(self):
        self.assertEqual(launch_services.launch_phase({'HOME': '/owned'},
                         ordinary_bot_diagnostics=True), 'ui-bot-diagnostics')
        for name in ('REASONIX_TAURI_NATIVE_WINDOW_SMOKE', 'REASONIX_TAURI_PACKAGE_SMOKE',
                     'REASONIX_TAURI_PROFILE_SMOKE', 'REASONIX_TAURI_LEGACY_UI_SMOKE'):
            with self.assertRaises(ValueError):
                launch_services.launch_phase({name: ''}, ordinary_bot_diagnostics=True)
        with self.assertRaises(ValueError):
            launch_services.launch_phase({}, 'before-import', True)
        with self.assertRaises(ValueError):
            launch_services.launch_phase({}, ordinary_bot_diagnostics='true')
        self.assertEqual(launch_services.launch_phase(
            {'REASONIX_TAURI_NATIVE_WINDOW_SMOKE': 'exercise'}), 'exercise')
        self.assertEqual(launch_services.launch_phase({}, 'after-undo'), 'ui-migration-after-undo')


if __name__ == '__main__':
    unittest.main()
