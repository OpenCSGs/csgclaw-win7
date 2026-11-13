"""Check release validation and compatibility gates before running builders."""
import importlib.util
import json
from pathlib import Path
import struct
import tempfile
import unittest
from unittest.mock import patch
import zipfile

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location('compat_ci', ROOT / 'compat/win7/ci.py')
ci = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci)
from compat.win7.version import app_version, normalize


class ReleaseCITest(unittest.TestCase):
    def setUp(self):
        directory = ROOT / '.cache/compat-ci/tests'
        directory.mkdir(parents=True, exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(dir=directory)
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)

    def test_versions_and_all_three_targets(self):
        for value in ['v0.10.1', 'v0.10.1-beta.1', '0.10.1-alpha.win7.3']:
            self.assertEqual(normalize(value), value.removeprefix('v'))
        for value in ['v1', 'v01.2.3', 'v1.2.3-01', 'v1.2.3\nunsafe=1', 'v1.2.3+extra', '../v1.2.3']:
            with self.assertRaises(ValueError):
                normalize(value)
        output = self.root / 'output'
        values = ci.metadata('v0.10.1-beta.1', output)
        self.assertEqual(values['prerelease'], 'true')
        self.assertEqual({v['target'] for v in json.loads(values['matrix'])['include']},
                         {'win7-amd64', 'kylin-v10-amd64', 'kylin-v10-arm64'})
        selected = ci.metadata('v0.10.1', target='win7-amd64')
        self.assertEqual([v['target'] for v in json.loads(selected['matrix'])['include']], ['win7-amd64'])

    def test_tag_override_does_not_change_locked_version(self):
        lock = {'app_version': '0.10.0'}
        with patch.dict('os.environ', {'CSGCLAW_WIN7_VERSION': 'v0.10.1-beta.2'}):
            self.assertEqual(app_version(lock), '0.10.1-beta.2')
        self.assertEqual(lock['app_version'], '0.10.0')

    def test_download_rejects_poisoned_cache(self):
        cache = self.root / 'downloads'
        cache.mkdir()
        (cache / 'runtime.tar.gz').write_bytes(b'changed')
        with patch.object(ci, 'CACHE', self.root):
            with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                ci.download('https://unused.invalid/runtime', '0' * 64, 'runtime.tar.gz')

    def test_toolchain_extraction_rejects_escape_and_preserves_modes(self):
        archive = self.root / 'sdk.zip'
        with zipfile.ZipFile(archive, 'w') as bundle:
            entry = zipfile.ZipInfo('go/bin/go')
            entry.external_attr = 0o100755 << 16
            bundle.writestr(entry, b'fixture')
        destination = self.root / 'sdk'
        ci.extract_zip(archive, destination)
        self.assertTrue((destination / 'go/bin/go').stat().st_mode & 0o100)
        with zipfile.ZipFile(archive, 'w') as bundle:
            bundle.writestr('../escaped', b'bad')
        with self.assertRaisesRegex(ValueError, 'Unsafe'):
            ci.extract_zip(archive, destination)
        self.assertFalse((self.root / 'escaped').exists())

    def test_elf_audit_rejects_newer_glibc_and_wrong_architecture(self):
        binary = self.root / 'binary'
        binary.write_bytes(b'\x7fELF\x02\x01' + b'\0' * 12 + struct.pack('<H', 62))
        with patch('subprocess.check_output', return_value='Name: GLIBC_2.31\nName: GLIBCXX_3.4.28'):
            self.assertEqual(ci.audit_elf(self.root, 'amd64')[0]['GLIBC'], '2.31')
        with patch('subprocess.check_output', return_value='Name: GLIBC_2.34'):
            with self.assertRaisesRegex(ValueError, 'exceeds Kylin'):
                ci.audit_elf(self.root, 'amd64')
        with self.assertRaisesRegex(ValueError, 'Wrong ELF architecture'):
            ci.audit_elf(self.root, 'arm64')


if __name__ == '__main__':
    unittest.main()
