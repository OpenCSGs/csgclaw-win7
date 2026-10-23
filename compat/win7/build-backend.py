#!/usr/bin/env python3
"""Build the native Win7 Go service and CLI without downgrading dependencies."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[2]
COMPAT = ROOT / 'compat/win7'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go', type=Path, required=True, help='XTLS/go-win7 patched-1.27.2 go executable')
    parser.add_argument('--codex-bundle', type=Path, required=True, help='Directory containing the verified Codex/WinPTY set')
    parser.add_argument('--skip-web', action='store_true', help='Use already built assets in prepared sources')
    args = parser.parse_args()
    source = ROOT / '.cache/win7/csgclaw'
    if not (source / 'go.mod').is_file():
        parser.error('Run compat/win7/prepare.py first')
    env = dict(os.environ, GOTOOLCHAIN='local', GOWORK='off', CGO_ENABLED='0')
    if not args.skip_web:
        subprocess.run(['make', 'build-web'], cwd=source, env=env, check=True)
    output = ROOT / 'dist/win7/backend/csgclaw/bin'
    output.mkdir(parents=True, exist_ok=True)
    env.update(GOOS='windows', GOARCH='amd64')
    lock = json.loads((COMPAT / 'sources.lock.json').read_text())
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    built = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    # This custom desktop bundle retains the upstream local-install guard;
    # it must not opt into the ordinary Windows release updater.
    flags = (f'-s -w -X csgclaw/internal/version.Version=v{lock["app_version"]}+local '
             f'-X csgclaw/internal/version.Commit={commit} '
             f'-X csgclaw/internal/version.BuildTime={built}')
    for name in ('csgclaw', 'csgclaw-cli'):
        subprocess.run([str(args.go.resolve()), 'build', '-ldflags', flags, '-o', str(output / (name + '.exe')), './cmd/' + name], cwd=source, env=env, check=True)
    for name in ('codex.exe', 'winpty.dll', 'winpty-agent.exe', 'LICENSE.winpty'):
        original = args.codex_bundle / name
        digest = hashlib.sha256(original.read_bytes()).hexdigest()
        if digest != lock['runtime'][name]['sha256']:
            raise ValueError(f'Runtime hash mismatch: {name}; native validation is required before replacing it')
        shutil.copy2(original, output / name)
    for source, name in [(ROOT / 'LICENSE', 'LICENSE.csgclaw'), (args.go.resolve().parent.parent / 'LICENSE', 'LICENSE.go'), (ROOT / '.cache/win7/codex/LICENSE', 'LICENSE.codex'), (ROOT / '.cache/win7/codex/NOTICE', 'NOTICE.codex')]:
        shutil.copy2(source, output.parent / name)
    print(output.parent)


if __name__ == '__main__':
    main()
