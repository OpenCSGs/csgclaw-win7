"""Record the exact desktop payload consumed by the Win7 installer."""
import hashlib
from datetime import datetime
import json
import os
from pathlib import Path
import shutil
import subprocess
from zoneinfo import ZoneInfo

ROOT = Path(__file__).resolve().parents[2]
COMPAT = ROOT / 'compat/win7'


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def compatibility_inputs():
    # Respect nested Codex ignore rules so local targets, SDK environments and
    # formatter caches never become distribution inputs.
    raw = subprocess.check_output(
        ['git', 'ls-files', '--cached', '--others', '--exclude-standard', '-z', '--', 'compat/win7'],
        cwd=ROOT,
    )
    paths = {ROOT / os.fsdecode(name) for name in raw.split(b'\0') if name}
    return [p for p in sorted(paths) if p.is_file() and '__pycache__' not in p.parts and 'tests' not in p.relative_to(COMPAT).parts]


def write_distribution():
    lock = json.loads((COMPAT / 'sources.lock.json').read_text())
    application = ROOT / 'dist/win7/app/CSGClaw-win32-x64'
    shutil.copy2(COMPAT / 'USER.zh.md', application / '使用说明.zh.md')
    inputs = compatibility_inputs()
    status = subprocess.check_output(['git', 'status', '--porcelain', '--', 'compat/win7'], cwd=ROOT, text=True)
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    build_id = datetime.now(ZoneInfo('Asia/Shanghai')).strftime('%Y%m%d') + '-' + commit[:8]
    if status.strip():
        build_id += '-dirty'
    manifest = {
        'kind': 'Win7 desktop development build',
        'version': lock['app_version'],
        'source_commit': commit,
        'source_order': int(subprocess.check_output(['git', 'show', '-s', '--format=%ct', commit], cwd=ROOT, text=True)),
        'build_id': build_id,
        'source_dirty': bool(status.strip()),
        'upstream_commit': lock['csgclaw']['commit'],
        'compatibility_inputs': {p.relative_to(ROOT).as_posix(): digest(p) for p in sorted(inputs)},
        'electron': lock['electron']['version'],
        'files': {p.relative_to(application).as_posix(): digest(p) for p in sorted(application.rglob('*')) if p.is_file() and p.name != 'win7-build.json'},
    }
    path = application / 'win7-build.json'
    path.write_text(json.dumps(manifest, indent=2) + '\n')
    return path


if __name__ == '__main__':
    write_distribution()
