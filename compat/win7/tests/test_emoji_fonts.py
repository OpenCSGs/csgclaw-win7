"""Win7's bundled emoji fallback must remain self-contained and pinned."""
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


class EmojiFontsTest(unittest.TestCase):
    def setUp(self):
        cache = ROOT / '.cache/win7/prepare-tests'
        cache.mkdir(parents=True, exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(dir=cache)
        self.addCleanup(self.temporary.cleanup)
        self.source = Path(self.temporary.name)
        self.web = self.source / 'web/app'
        styles = self.web / 'src/shared/styles'
        shutil.copytree(ROOT / 'web/app/src/shared/styles', styles)

    def test_fallback_keeps_text_fonts_and_copies_local_assets(self):
        with patch.object(configure, 'SOURCE', self.source):
            configure.configure_emoji_fonts(self.web)
        styles = self.web / 'src/shared/styles'
        css = (styles / 'emoji.css').read_text()
        self.assertIn('url("./fonts/noto-color-emoji.woff2")', css)
        self.assertTrue((styles / 'fonts/noto-color-emoji.woff2').is_file())
        self.assertTrue((styles / 'fonts/LICENSE.noto-emoji').is_file())
        tokens = (styles / 'tokens.css').read_text()
        self.assertIn('"Microsoft YaHei"', tokens)
        self.assertIn('Arial,\n    "CSGClaw Emoji", sans-serif', tokens)
        self.assertIn('@import "./emoji.css";', (styles / 'globals.css').read_text())

    def test_corrupt_asset_fails_before_installing_fonts(self):
        compat = self.source / 'compat'
        shutil.copytree(ROOT / 'compat/win7/web', compat / 'web')
        shutil.copy2(ROOT / 'compat/win7/sources.lock.json', compat / 'sources.lock.json')
        (compat / 'web/fonts/noto-color-emoji.woff2').write_bytes(b'corrupted')
        with patch.object(configure, 'COMPAT', compat):
            with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                configure.configure_emoji_fonts(self.web)
        self.assertFalse((self.web / 'src/shared/styles/fonts').exists())


if __name__ == '__main__':
    unittest.main()
