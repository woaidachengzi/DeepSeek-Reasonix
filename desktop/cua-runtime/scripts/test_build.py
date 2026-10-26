"""Hermetic source-export checks; never fetches or changes a developer checkout."""
import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('cua_build', Path(__file__).with_name('build.py'))
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)


class SourceExportTests(unittest.TestCase):
    def test_helper_is_exported_from_the_same_clean_pinned_commit(self):
        with tempfile.TemporaryDirectory(prefix='reasonix-cua-export-') as temp:
            root = Path(temp)
            source = root/'upstream'
            source.mkdir()
            env = {**os.environ, 'GIT_CONFIG_NOSYSTEM':'1', 'GIT_CONFIG_GLOBAL':os.devnull}
            def git(*args):
                return subprocess.check_output(['git', '-C', str(source), *args], env=env, stderr=subprocess.DEVNULL).decode().strip()
            git('init')
            rust = source/'libs/cua-driver/rust/crates/fixture'
            rust.mkdir(parents=True)
            (rust/'Cargo.toml').write_text('[package]\nname = "fixture"\n')
            imports = source/'libs/cua-driver/rust/crates/platform-linux/src/input/targeted.rs'
            imports.parent.mkdir(parents=True)
            imports.write_text('use super as input;\nuse super as input;\n')
            native = source/'libs/cua-driver/rust/crates/platform-linux/src/atspi/native.rs'
            native.parent.mkdir(parents=True)
            native.write_text('let len = text.chars().count() as i32;\nlet len = value.chars().count() as i32;\n')
            helper = source/'libs/cua-driver/wayland-helper/winrects@cua'
            helper.mkdir(parents=True)
            (helper/'extension.js').write_text('// pinned helper\n')
            (source/'LICENSE.md').write_text('fixture MIT notice\n')
            git('add', '.')
            git('-c','user.name=QA','-c','user.email=qa@example.invalid','-c','commit.gpgsign=false','commit','-m','fixture')
            pin = dict(repository='unused', revision=git('rev-parse','HEAD'), scope='libs/cua-driver/rust', additional_scopes=['libs/cua-driver/wayland-helper'])
            runtime = root/'runtime'
            (runtime/'licenses').mkdir(parents=True)
            (runtime/'licenses/Cua-MIT.txt').write_text('fixture MIT notice\n')
            cache = root/'cache'
            with patch.multiple(build, PIN=pin, CACHE=cache, RUNTIME=runtime):
                build.prepare(source)
                self.assertEqual((cache/'libs/cua-driver/wayland-helper/winrects@cua/extension.js').read_text(), '// pinned helper\n')
                self.assertIn('workspace = "../.."', (cache/'libs/cua-driver/rust/crates/fixture/Cargo.toml').read_text())
                self.assertNotIn('workspace =', (rust/'Cargo.toml').read_text())
                self.assertEqual((cache/'libs/cua-driver/rust/crates/platform-linux/src/input/targeted.rs').read_text(), 'use crate::input;\nuse crate::input;\n')
                self.assertEqual(imports.read_text(), 'use super as input;\nuse super as input;\n')
                self.assertEqual((cache/'libs/cua-driver/rust/crates/platform-linux/src/atspi/native.rs').read_text(), 'let len = text.len() as i32;\nlet len = value.len() as i32;\n')
                self.assertIn('text.chars().count()', native.read_text())
                (helper/'extension.js').write_text('// modified\n')
                with self.assertRaisesRegex(RuntimeError, 'tracked modifications'):
                    build.prepare(source)
                self.assertEqual((cache/'libs/cua-driver/wayland-helper/winrects@cua/extension.js').read_text(), '// pinned helper\n')

    def test_unowned_cache_is_preserved(self):
        with tempfile.TemporaryDirectory(prefix='reasonix-cua-unowned-') as temp:
            cache = Path(temp)
            sentinel = cache/'user-file'
            sentinel.write_text('keep')
            with patch.object(build, 'CACHE', cache):
                with self.assertRaisesRegex(RuntimeError, 'unowned'):
                    build.prepare(None)
            self.assertEqual(sentinel.read_text(), 'keep')


if __name__ == '__main__':
    unittest.main()
