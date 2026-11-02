"""Hermetic source-export checks; never fetches or changes a developer checkout."""
import importlib.util
import hashlib
from contextlib import chdir
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


class BackportTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory(prefix='reasonix-cua-patch-')
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        self.cache = root/'cache'
        self.runtime = root/'runtime'
        self.relative = 'libs/cua-driver/rust/crates/fixture/src/lib.rs'
        self.source = self.cache/self.relative
        self.source.parent.mkdir(parents=True)
        self.source.write_bytes(b'old\n')
        self.patch = self.runtime/'patches/fixture.patch'
        self.patch.parent.mkdir(parents=True)
        self.patch.write_text(f'diff --git a/{self.relative} b/{self.relative}\n'
                              f'--- a/{self.relative}\n+++ b/{self.relative}\n'
                              '@@ -1 +1 @@\n-old\n+new\n')
        self.entry = dict(file=self.relative,
                          original_sha256=hashlib.sha256(b'old\n').hexdigest(),
                          patched_sha256=hashlib.sha256(b'new\n').hexdigest())
        self.backport = dict(revision='reviewed-commit', reason='fixture',
                             patch='patches/fixture.patch',
                             sha256=hashlib.sha256(self.patch.read_bytes()).hexdigest(),
                             files=[self.entry])
        pin = dict(scope='libs/cua-driver/rust', backports=[self.backport])
        globals_patch = patch.multiple(build, PIN=pin, CACHE=self.cache, RUNTIME=self.runtime)
        globals_patch.start()
        self.addCleanup(globals_patch.stop)

    def test_verified_patch_is_applied_and_audited(self):
        records = build.apply_backports()
        self.assertEqual(self.source.read_bytes(), b'new\n')
        self.assertEqual(records, [{**self.entry, 'reason': 'fixture', 'upstream_revision': 'reviewed-commit'}])

    def test_local_fix_keeps_its_actual_provenance(self):
        local = {key: value for key, value in self.backport.items() if key != 'revision'}
        local.update(id='local-window-query', source_url='https://github.com/trycua/cua/issues/6270')
        with patch.object(build, 'PIN', dict(scope='libs/cua-driver/rust', local_fixes=[local])):
            records = build.apply_backports()
        self.assertEqual(self.source.read_bytes(), b'new\n')
        self.assertEqual(records[0]['local_fix'], 'local-window-query')
        self.assertNotIn('upstream_revision', records[0])

    def new_file_patch(self):
        self.patch.write_text(f'diff --git a/{self.relative} b/{self.relative}\n'
                              'new file mode 100644\n'
                              f'--- /dev/null\n+++ b/{self.relative}\n'
                              '@@ -0,0 +1 @@\n+new\n')
        self.backport['sha256'] = hashlib.sha256(self.patch.read_bytes()).hexdigest()
        self.entry['original_sha256'] = None

    def test_verified_new_file_is_created(self):
        self.new_file_patch()
        self.source.unlink()
        records = build.apply_backports()
        self.assertEqual(self.source.read_bytes(), b'new\n')
        self.assertIsNone(records[0]['original_sha256'])

    def test_new_file_cannot_overwrite_existing_source(self):
        self.new_file_patch()
        with self.assertRaisesRegex(RuntimeError, 'already exists'):
            build.apply_backports()
        self.assertEqual(self.source.read_bytes(), b'old\n')

    def test_patch_paths_do_not_depend_on_callers_directory(self):
        subprocess.run(['git', 'init', '-q', str(self.runtime)], check=True)
        nested = self.runtime/'nested'
        nested.mkdir()
        with chdir(nested):
            build.apply_backports()
        self.assertEqual(self.source.read_bytes(), b'new\n')

    def test_changed_patch_is_rejected(self):
        self.patch.write_text(self.patch.read_text() + '\n')
        with self.assertRaisesRegex(RuntimeError, 'patch checksum'):
            build.apply_backports()
        self.assertEqual(self.source.read_bytes(), b'old\n')

    def test_changed_source_is_preserved(self):
        self.source.write_bytes(b'local edit\n')
        with self.assertRaisesRegex(RuntimeError, 'reviewed input'):
            build.apply_backports()
        self.assertEqual(self.source.read_bytes(), b'local edit\n')

    def test_unreviewed_output_is_not_copied(self):
        self.entry['patched_sha256'] = hashlib.sha256(b'unexpected\n').hexdigest()
        with self.assertRaisesRegex(RuntimeError, 'reviewed result'):
            build.apply_backports()
        self.assertEqual(self.source.read_bytes(), b'old\n')

    def test_unlisted_patch_file_is_rejected(self):
        self.backport['files'] = []
        with self.assertRaisesRegex(RuntimeError, 'reviewed manifest'):
            build.apply_backports()
        self.assertEqual(self.source.read_bytes(), b'old\n')


if __name__ == '__main__':
    unittest.main()
