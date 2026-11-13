"""A damaged CA bundle must never enter either prepared application."""
import importlib.util
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location('win7_configure', ROOT / 'compat/win7/configure.py')
configure = importlib.util.module_from_spec(spec)
spec.loader.exec_module(configure)


class HTTPTrustTest(unittest.TestCase):
    def test_corrupt_bundle_is_rejected_before_preparation(self):
        cache = ROOT / '.cache/win7/prepare-tests'
        cache.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=cache) as directory:
            root = Path(directory)
            compat = root / 'compat'
            shutil.copytree(ROOT / 'compat/win7/httptrust', compat / 'httptrust')
            shutil.copy2(ROOT / 'compat/win7/sources.lock.json', compat / 'sources.lock.json')
            (compat / 'httptrust/cacert.pem').write_bytes(b'changed roots')
            with patch.object(configure, 'COMPAT', compat):
                with self.assertRaisesRegex(ValueError, 'trust checksum mismatch'):
                    configure.configure_http_trust(root / 'source')
            self.assertFalse((root / 'source/internal/win7httptrust').exists())

    def test_overlay_preserves_its_source_and_rejects_conflicts(self):
        cache = ROOT / '.cache/win7/prepare-tests'
        cache.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=cache) as directory:
            source = Path(directory)
            configure.configure_http_trust(source)
            target = source / 'internal/win7httptrust'
            self.assertEqual((target / 'cacert.pem').read_bytes(), (ROOT / 'compat/win7/httptrust/cacert.pem').read_bytes())
            with self.assertRaisesRegex(ValueError, 'conflicts with upstream'):
                configure.configure_http_trust(source)
