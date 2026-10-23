"""Keep the CSGClaw export separate from the checked-in Codex fork."""
import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location('win7_prepare', ROOT / 'compat/win7/prepare.py')
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


class PrepareTest(unittest.TestCase):
    def setUp(self):
        cache = ROOT / '.cache/win7/prepare-tests'
        cache.mkdir(parents=True, exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(dir=cache)
        self.addCleanup(self.temporary.cleanup)
        self.repo = Path(self.temporary.name)
        self.original_cache = prepare.CACHE
        prepare.CACHE = self.repo / 'exports'
        prepare.CACHE.mkdir()
        self.addCleanup(setattr, prepare, 'CACHE', self.original_cache)
        subprocess.run(['git', 'init', '-q', str(self.repo)], check=True)
        for name, value in [('user.name', 'Fixture'), ('user.email', 'fixture@example.invalid')]:
            subprocess.run(['git', 'config', name, value], cwd=self.repo, check=True)
        (self.repo / 'main.go').write_text('package main\n')
        source = self.repo / 'compat/win7/codex'
        (source / 'compat/win7').mkdir(parents=True)
        (source / 'LICENSE').write_text('Codex license\n')
        script = source / 'compat/win7/build-macos.sh'
        script.write_text('#!/bin/sh\nexit 0\n')
        script.chmod(0o755)
        (source / 'NOTICE').symlink_to('LICENSE')
        subprocess.run(['git', 'add', '.'], cwd=self.repo, check=True)
        subprocess.run(['git', 'commit', '-qm', 'fixture'], cwd=self.repo, check=True)

    def test_codex_export_preserves_its_own_win7_helpers(self):
        # An uncommitted source edit must not contaminate a revision-pinned build.
        (self.repo / 'compat/win7/codex/LICENSE').write_text('uncommitted edit\n')
        destination = prepare.CACHE / 'codex'
        prepare.materialize(self.repo, 'HEAD:compat/win7/codex', destination, [])
        self.assertEqual((destination / 'LICENSE').read_text(), 'Codex license\n')
        self.assertEqual((destination / 'NOTICE').readlink(), Path('LICENSE'))
        self.assertTrue((destination / 'compat/win7/build-macos.sh').stat().st_mode & 0o111)
        self.assertFalse((destination / 'main.go').exists())

    def test_csgclaw_export_excludes_the_compatibility_tree(self):
        destination = prepare.CACHE / 'csgclaw'
        prepare.materialize(self.repo, 'HEAD', destination, [], exclude_prefixes=('compat/win7/',))
        self.assertTrue((destination / 'main.go').is_file())
        self.assertFalse((destination / 'compat/win7/codex').exists())

    def test_invalid_source_keeps_the_previous_export(self):
        destination = prepare.CACHE / 'codex'
        destination.mkdir()
        (destination / 'previous').write_text('keep\n')
        with self.assertRaises(subprocess.CalledProcessError):
            prepare.materialize(self.repo, 'HEAD:missing-codex', destination, [])
        self.assertEqual((destination / 'previous').read_text(), 'keep\n')


if __name__ == '__main__':
    unittest.main()
