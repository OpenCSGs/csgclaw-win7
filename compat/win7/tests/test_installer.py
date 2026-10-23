"""Never package incomplete, changed or unrelated desktop files."""
import importlib.util
import json
from pathlib import Path
import tempfile
import subprocess
import sys
import unittest

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location('win7_installer', ROOT / 'compat/win7/build-installer.py')
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


class InstallerTest(unittest.TestCase):
    def setUp(self):
        cache = ROOT / '.cache/win7/installer-tests'
        cache.mkdir(parents=True, exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(dir=cache)
        self.addCleanup(self.temporary.cleanup)
        self.application = Path(self.temporary.name)
        files = {}
        for name in installer.REQUIRED:
            path = self.application / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(b'verified fixture')
            files[name] = installer.digest(path)
        (self.application / 'win7-build.json').write_text(json.dumps({'files': files}))

    def test_complete_desktop_is_accepted(self):
        _, files = installer.verified_files(self.application)
        self.assertEqual(set(files), installer.REQUIRED)

    def test_missing_ffmpeg_is_rejected(self):
        (self.application / 'ffmpeg.dll').unlink()
        with self.assertRaisesRegex(ValueError, 'ffmpeg.dll'):
            installer.verified_files(self.application)

    def test_changed_runtime_is_rejected(self):
        (self.application / 'ffmpeg.dll').write_bytes(b'changed')
        with self.assertRaisesRegex(ValueError, 'ffmpeg.dll'):
            installer.verified_files(self.application)

    def test_unrecorded_config_is_not_shipped(self):
        (self.application / 'config.toml').write_text('private fixture')
        with self.assertRaisesRegex(ValueError, 'Unrecorded'):
            installer.verified_files(self.application)

    def test_windows_escape_paths_are_rejected(self):
        for path in ['../profile', 'C:/profile', 'resources/../../profile', 'resources\\file', 'CON', 'file.']:
            with self.subTest(path=path), self.assertRaises(ValueError):
                installer.safe_path(path)

    def test_failed_compilation_retains_the_previous_installer(self):
        self.assert_compiler_publish(1)

    def test_successful_compilation_publishes_complete_installer(self):
        self.assert_compiler_publish(0)

    def assert_compiler_publish(self, exit_code):
        compiler = self.application / 'compiler.py'
        compiler.write_text(
            'import sys\nfrom pathlib import Path\n'
            'out = next(a.split("=", 1)[1] for a in sys.argv if a.startswith("-DSETUP_OUTPUT="))\n'
            'Path(out).write_bytes(b"new installer")\n'
            f'sys.exit({exit_code})\n')
        output = self.application / 'Setup.exe'
        output.write_bytes(b'previous verified installer')
        if exit_code:
            with self.assertRaises(subprocess.CalledProcessError):
                installer.compile_installer([sys.executable, str(compiler)], 'fixture.nsi', output)
            self.assertEqual(output.read_bytes(), b'previous verified installer')
        else:
            installer.compile_installer([sys.executable, str(compiler)], 'fixture.nsi', output)
            self.assertEqual(output.read_bytes(), b'new installer')
        self.assertFalse(list(self.application.glob('.win7-setup-*')))


if __name__ == '__main__':
    unittest.main()
