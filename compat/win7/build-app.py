#!/usr/bin/env python3
"""Build the complete Win7 desktop application and its installable Setup.exe."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import urllib.request
from distribution import write_distribution

ROOT = Path(__file__).resolve().parents[2]
COMPAT = ROOT / 'compat/win7'
CACHE = ROOT / '.cache/win7'


def run(arguments, cwd=ROOT, env=None):
    subprocess.run([str(a) for a in arguments], cwd=cwd, env=env, check=True)


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go', type=Path, required=True, help='XTLS/go-win7 patched-1.27.2 executable')
    parser.add_argument('--codex-bundle', type=Path, required=True, help='Previously natively verified Codex/WinPTY runtime set')
    parser.add_argument('--electron-archive', type=Path, help='Optional offline official Electron archive')
    parser.add_argument('--allow-dirty', action='store_true', help='Local acceptance only; mark the installer as dirty')
    args = parser.parse_args()
    if not args.allow_dirty and subprocess.check_output(
            ['git', 'status', '--porcelain', '--', 'compat/win7'], cwd=ROOT, text=True).strip():
        parser.error('Commit Win7 changes before distribution; --allow-dirty is for local acceptance only')
    if not shutil.which('makensis'):
        parser.error('Install the pinned NSIS compiler first (macOS: brew install makensis)')
    lock = json.loads((COMPAT / 'sources.lock.json').read_text())
    env = dict(os.environ, GOTOOLCHAIN='local', GOWORK='off', ELECTRON_SKIP_BINARY_DOWNLOAD='1')
    node = shutil.which('node')
    if not node or subprocess.check_output([node, '--version'], text=True).split('.')[0] != 'v24':
        parser.error('Use Node 24 on the build host; it is separate from the packaged Electron runtime')
    go = args.go.resolve()
    goroot = Path(subprocess.check_output([go, 'env', 'GOROOT'], env=env, text=True).strip())
    for name, expected in lock['go']['source_fingerprints'].items():
        if digest(goroot / name) != expected:
            parser.error('Go SDK does not match the pinned Win7 source: ' + name)
    run([sys.executable, COMPAT / 'prepare.py', '--codex'])
    run([sys.executable, COMPAT / 'configure.py'])
    source = CACHE / 'csgclaw'
    electron = CACHE / 'electron' / lock['electron']['archive']
    electron.parent.mkdir(parents=True, exist_ok=True)
    if args.electron_archive:
        original = args.electron_archive.resolve()
        if original != electron.resolve():
            shutil.copy2(original, electron)
    if not electron.exists():
        temporary = electron.with_suffix('.downloading')
        with urllib.request.urlopen(lock['electron']['url'], timeout=60) as response, temporary.open('wb') as output:
            shutil.copyfileobj(response, output)
        temporary.replace(electron)
    if digest(electron) != lock['electron']['sha256']:
        raise ValueError('Electron archive checksum mismatch')
    # The packaged binary is supplied explicitly; no host Electron download is needed.
    env['ELECTRON_OVERRIDE_DIST_PATH'] = str(electron.parent)
    for script in ('desktop-pnpm.sh', 'web-pnpm.sh'):
        run(['bash', source / 'scripts' / script, 'install', '--frozen-lockfile'], cwd=source, env=env)
    run(['bash', source / 'scripts/web-pnpm.sh', 'check'], cwd=source, env=env)
    run(['bash', source / 'scripts/desktop-pnpm.sh', 'test'], cwd=source, env=env)
    # Server tests exercise embedded UI responses; a pristine archive contains
    # only the generated directory placeholder until the web build completes.
    run(['make', 'build-web'], cwd=source, env=env)
    run([go, 'test', './...'], cwd=source, env=env)
    run([sys.executable, COMPAT / 'build-backend.py', '--go', go, '--codex-bundle', args.codex_bundle.resolve(), '--skip-web'], env=env)
    run([node, COMPAT / 'package-desktop.mjs'], env=env)
    write_distribution()
    run([sys.executable, COMPAT / 'build-installer.py', '--go', go,
         *(['--allow-dirty'] if args.allow_dirty else [])])
    print('Installer EXE and host checks completed. Native Win7 application acceptance must run separately.')


if __name__ == '__main__':
    main()
