"""Do not rebuild the unsupported ordinary Windows CLI by accident."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / 'compat/win7/build-connectors.py'
spec = importlib.util.spec_from_file_location('win7_connectors', SCRIPT)
connectors = importlib.util.module_from_spec(spec)
spec.loader.exec_module(connectors)


class ConnectorBuildTest(unittest.TestCase):
    def setUp(self):
        cache = ROOT / '.cache/win7/connector-build-tests'
        cache.mkdir(parents=True, exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(dir=cache)
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)

    def test_changed_cached_source_is_rejected(self):
        archive = self.directory / 'source.tar.gz'
        archive.write_bytes(b'changed source')
        with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
            connectors.download('https://example.invalid/source.tar.gz', '0' * 64, archive)
        self.assertEqual(archive.read_bytes(), b'changed source')

    def test_stock_go_cannot_build_the_win7_connector(self):
        go = shutil.which('go')
        if not go:
            self.skipTest('Go SDK is unavailable')
        env = dict(os.environ, GOTOOLCHAIN='local', GOWORK='off')
        goroot = Path(subprocess.check_output([go, 'env', 'GOROOT'], env=env, text=True).strip())
        lock = json.loads((ROOT / 'compat/win7/sources.lock.json').read_text())
        fingerprints = lock['go']['source_fingerprints']
        if all((goroot / name).is_file() and hashlib.sha256((goroot / name).read_bytes()).hexdigest() == sha
               for name, sha in fingerprints.items()):
            self.skipTest('This is already the pinned Win7 SDK')
        output = self.directory / 'packages'
        result = subprocess.run([sys.executable, SCRIPT, '--go', go, '--output', output],
                                env=env, text=True, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('does not match the pinned Win7 source', result.stderr)
        self.assertFalse(output.exists())


if __name__ == '__main__':
    unittest.main()
