"""Local Codex builds must not change the recorded distribution inputs."""
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location('win7_distribution', ROOT / 'compat/win7/distribution.py')
distribution = importlib.util.module_from_spec(spec)
spec.loader.exec_module(distribution)


class DistributionTest(unittest.TestCase):
    def test_records_installer_payload_without_creating_a_portable_archive(self):
        cache = ROOT / '.cache/win7/distribution-tests'
        cache.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=cache) as directory:
            repo = Path(directory)
            compat = repo / 'compat/win7'
            compat.mkdir(parents=True)
            (compat / 'USER.zh.md').write_text('Install Setup.exe\n')
            (compat / 'sources.lock.json').write_text(json.dumps({
                'app_version': 'test', 'csgclaw': {'commit': 'a'*40}, 'electron': {'version': '22.3.27'},
            }))
            subprocess.run(['git', 'init', '-q', str(repo)], check=True)
            subprocess.run(['git', 'add', '.'], cwd=repo, check=True)
            subprocess.run(['git', '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                            'commit', '-qm', 'test: installer payload'], cwd=repo, check=True)
            commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=repo, text=True).strip()
            application = repo / 'dist/win7/app/CSGClaw-win32-x64'
            application.mkdir(parents=True)
            (application / 'CSGClaw.exe').write_bytes(b'application fixture')
            with patch.object(distribution, 'ROOT', repo), patch.object(distribution, 'COMPAT', compat):
                manifest = distribution.write_distribution()
            recorded = json.loads(manifest.read_text())
            self.assertEqual(recorded['source_commit'], commit)
            self.assertIn(commit[:8], recorded['build_id'])
            self.assertEqual(recorded['files']['CSGClaw.exe'], distribution.digest(application / 'CSGClaw.exe'))
            self.assertEqual(recorded['files']['使用说明.zh.md'], distribution.digest(compat / 'USER.zh.md'))
            self.assertFalse(list((repo / 'dist').rglob('*.zip')))
            self.assertFalse(list((repo / 'dist').rglob('*.packing')))

    def test_ignores_local_codex_outputs_but_records_new_source_inputs(self):
        cache = ROOT / '.cache/win7/distribution-tests'
        cache.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=cache) as directory:
            repo = Path(directory)
            compat = repo / 'compat/win7'
            codex = compat / 'codex'
            codex.mkdir(parents=True)
            (repo / '.gitignore').write_text('compat/win7/codex/target/\n')
            (codex / 'LICENSE').write_text('license\n')
            subprocess.run(['git', 'init', '-q', str(repo)], check=True)
            subprocess.run(['git', 'add', '.'], cwd=repo, check=True)
            (codex / 'target').mkdir()
            (codex / 'target/temporary.exe').write_bytes(b'build output')
            (compat / 'new-build-input.json').write_text('{}\n')
            previous_root, previous_compat = distribution.ROOT, distribution.COMPAT
            try:
                distribution.ROOT, distribution.COMPAT = repo, compat
                inputs = {p.relative_to(compat).as_posix() for p in distribution.compatibility_inputs()}
            finally:
                distribution.ROOT, distribution.COMPAT = previous_root, previous_compat
            self.assertEqual(inputs, {'codex/LICENSE', 'new-build-input.json'})


if __name__ == '__main__':
    unittest.main()
