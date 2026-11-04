#!/usr/bin/env python3
"""Bundle the pinned Feishu CLI with the same Win7 Go SDK as the service."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import struct
import subprocess
import tarfile
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
COMPAT = ROOT / 'compat/win7'
CACHE = ROOT / '.cache/win7/connectors'


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def download(url, checksum, destination):
    if not destination.exists():
        temporary = destination.with_suffix('.downloading')
        with urllib.request.urlopen(url, timeout=120) as response, temporary.open('wb') as output:
            shutil.copyfileobj(response, output)
        temporary.replace(destination)
    if digest(destination) != checksum:
        raise ValueError('Connector archive checksum mismatch: ' + destination.name)


def directory_digest(root):
    result = hashlib.sha256()

    def visit(directory):
        # Match skill/state.DirectoryDigest's per-directory WalkDir order.
        for path in sorted(directory.iterdir()):
            if path.is_symlink():
                raise ValueError('Connector Skills must not contain symlinks')
            if path.is_dir():
                visit(path)
            elif path.is_file():
                data = path.read_bytes()
                result.update(path.relative_to(root).as_posix().encode() + b'\0')
                result.update(struct.pack('<q', len(data)))
                result.update(data)
            else:
                raise ValueError('Connector Skills contain a special file')

    visit(root)
    return result.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    lock = json.loads((COMPAT / 'sources.lock.json').read_text())
    source = lock['feishu_cli']
    spec = json.loads((ROOT / 'internal/apps/builtin/feishu/cli.json').read_text())['install']
    if source['version'] != spec['version']:
        raise ValueError('Feishu source version does not match the connector declaration')
    go = args.go.resolve()
    env = dict(os.environ, GOTOOLCHAIN='local', GOWORK='off', CGO_ENABLED='0', GOOS='windows', GOARCH='amd64')
    goroot = Path(subprocess.check_output([go, 'env', 'GOROOT'], env=env, text=True).strip())
    for name, expected in lock['go']['source_fingerprints'].items():
        if not (goroot / name).is_file() or digest(goroot / name) != expected:
            raise ValueError('Connector Go SDK does not match the pinned Win7 source: ' + name)
    CACHE.mkdir(parents=True, exist_ok=True)
    archive = CACHE / 'source.tar.gz'
    download(source['url'], source['sha256'], archive)
    tree = CACHE / 'source'
    if tree.exists():
        shutil.rmtree(tree)
    tree.mkdir()
    with tarfile.open(archive) as bundle:
        bundle.extractall(tree, filter='data')
    roots = list(tree.iterdir())
    if len(roots) != 1 or not (roots[0] / 'go.mod').is_file():
        raise ValueError('Unexpected Feishu source archive layout')
    target = args.output.resolve() / spec['executable'] / spec['version']
    if target.exists():
        shutil.rmtree(target)
    (target / 'bin').mkdir(parents=True)
    executable = target / 'bin/lark-cli.exe'
    subprocess.run([go, 'build', '-buildvcs=false', '-trimpath', '-ldflags',
                    '-s -w -X github.com/larksuite/cli/internal/build.Version=' + spec['version'],
                    '-o', executable, '.'], cwd=roots[0], env=env, check=True)
    skills_archive = CACHE / 'skills.tar.gz'
    download('https://codeload.github.com/larksuite/cli/tar.gz/' + spec['skills']['version'],
             spec['skills']['sha256'], skills_archive)
    skills_tree = CACHE / 'skills'
    if skills_tree.exists():
        shutil.rmtree(skills_tree)
    skills_tree.mkdir()
    with tarfile.open(skills_archive) as bundle:
        bundle.extractall(skills_tree, filter='data')
    skills = target / 'skills'
    skills.mkdir()
    for directory in sorted(skills_tree.glob('*/skills/' + spec['skills']['prefix'] + '*')):
        if directory.is_dir():
            shutil.copytree(directory, skills / directory.name, symlinks=True)
    if not any(skills.iterdir()):
        raise ValueError('Feishu Skills archive is empty')
    manifest = {'version': spec['version'], 'skills_commit': spec['skills']['version'],
                'executable_sha256': digest(executable), 'skills_sha256': directory_digest(skills)}
    (target / 'package.json').write_text(json.dumps(manifest, indent=2) + '\n')
    shutil.copy2(roots[0] / 'LICENSE', target / 'LICENSE')
    print('Bundled Win7 Feishu CLI and pinned Skills: ' + str(target))


if __name__ == '__main__':
    main()
