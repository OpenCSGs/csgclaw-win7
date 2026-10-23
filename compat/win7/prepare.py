#!/usr/bin/env python3
"""Prepare committed CSGClaw and Codex sources for the Win7 build."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[2]
COMPAT = ROOT / 'compat/win7'
CACHE = ROOT / '.cache/win7'


def materialize(repository, ref, destination, patches, exclude_prefixes=()):
    destination = Path(destination)
    if destination.parent != CACHE:
        raise ValueError('Prepared sources must be direct children of the owned cache')
    raw = subprocess.check_output(['git', 'archive', ref], cwd=repository)
    temporary = destination.with_name(destination.name + '.preparing')
    if temporary.exists():
        shutil.rmtree(temporary)
    temporary.mkdir(parents=True)
    try:
        with tarfile.open(fileobj=io.BytesIO(raw)) as archive:
            members = [m for m in archive.getmembers() if not m.name.startswith(exclude_prefixes)]
            archive.extractall(temporary, members=members, filter='data')
        # The cache is inside the owning repository. Prevent Git from treating
        # these paths as a subdirectory prefix and silently skipping the patch.
        env = dict(os.environ, GIT_CEILING_DIRECTORIES=str(temporary.parent))
        for patch in patches:
            for arguments in (['--check'], []):
                subprocess.run(['git', 'apply', '--whitespace=nowarn', *arguments, str(patch)], cwd=temporary, env=env, check=True)
        if destination.exists():
            shutil.rmtree(destination)
        temporary.rename(destination)
    finally:
        if temporary.exists():
            shutil.rmtree(temporary)
    print(destination)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--ref', default='HEAD', help='CSGClaw tree to prepare, normally HEAD')
    parser.add_argument('--codex', action='store_true', help='Also prepare the pinned Codex fork')
    args = parser.parse_args()
    lock = json.loads((COMPAT / 'sources.lock.json').read_text())
    for name, expected in lock['patches'].items():
        actual = hashlib.sha256((COMPAT / 'patches' / name).read_bytes()).hexdigest()
        if actual != expected:
            raise ValueError(f'Patch hash mismatch: {name}; review it and update sources.lock.json')
    CACHE.mkdir(parents=True, exist_ok=True)
    patches = ['csgclaw.patch', *lock.get('csgclaw_extra_patches', [])]
    materialize(ROOT, args.ref, CACHE / 'csgclaw', [COMPAT / 'patches' / name for name in patches], exclude_prefixes=('compat/win7/',))
    if args.codex:
        # The checked-in fork already contains the adaptation. Archive the same
        # revision as CSGClaw; do not fetch or reapply the historical port patch.
        source_ref = f"{args.ref}:{lock['codex']['source_path']}"
        materialize(ROOT, source_ref, CACHE / 'codex', [])


if __name__ == '__main__':
    main()
