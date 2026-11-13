"""Native Windows builds must select Git Bash rather than the WSL launcher."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location('win7_build_app', ROOT / 'compat/win7/build-app.py')
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)


class BuildAppTest(unittest.TestCase):
    def test_finds_git_bash_from_both_git_installation_paths(self):
        cache = ROOT / '.cache/win7/prepare-tests'
        cache.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=cache) as directory:
            root = Path(directory)
            (root / 'bin').mkdir()
            (root / 'bin/bash.exe').write_bytes(b'Git Bash fixture')
            for relative in ['cmd/git.exe', 'mingw64/bin/git.exe']:
                git = root / relative
                git.parent.mkdir(parents=True, exist_ok=True)
                git.touch()
                self.assertEqual(build.git_bash(git), root / 'bin/bash.exe')

    def test_missing_git_bash_fails_explicitly(self):
        with self.assertRaisesRegex(ValueError, 'Git for Windows Bash'):
            build.git_bash(ROOT / '.cache/no-git-installation/git.exe')
