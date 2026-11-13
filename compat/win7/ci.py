#!/usr/bin/env python3
"""Build and validate the Win7 and Kylin V10 tag-release artifacts."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import platform
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from compat.win7.version import normalize

COMPAT = ROOT / 'compat/win7'
CACHE = ROOT / '.cache/compat-ci'
OUTPUT = ROOT / 'dist/compat-release'
TARGETS = {'win7-amd64': 'windows-2025', 'kylin-v10-amd64': 'ubuntu-24.04', 'kylin-v10-arm64': 'ubuntu-24.04-arm'}


def run(args, cwd=ROOT, env=None, **kwargs):
    return subprocess.run([str(a) for a in args], cwd=cwd, env=env, check=True, **kwargs)


def digest(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def download(url, checksum, filename):
    CACHE.joinpath('downloads').mkdir(parents=True, exist_ok=True)
    path = CACHE / 'downloads' / filename
    if not path.exists():
        pending = path.with_suffix(path.suffix + '.part')
        with urllib.request.urlopen(url, timeout=180) as response, pending.open('wb') as output:
            shutil.copyfileobj(response, output)
        pending.replace(path)
    if digest(path) != checksum:
        raise ValueError('Download checksum mismatch: ' + filename)
    return path


def extract_zip(archive, destination):
    destination = Path(destination)
    destination.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(archive) as bundle:
        for entry in bundle.infolist():
            path = destination / entry.filename
            mode = entry.external_attr >> 16
            if not path.resolve().is_relative_to(destination.resolve()) or stat.S_ISLNK(mode):
                raise ValueError('Unsafe toolchain archive path')
            bundle.extract(entry, destination)
            if mode and path.exists():
                path.chmod(stat.S_IMODE(mode))


def win7_tools():
    lock = json.loads((COMPAT / 'sources.lock.json').read_text())
    system = {'Windows': 'windows', 'Darwin': 'darwin', 'Linux': 'linux'}[platform.system()]
    arch = 'arm64' if platform.machine().lower() in ('arm64', 'aarch64') else 'amd64'
    name = f'go-for-win7-{system}-{arch}.zip'
    source = lock['go']['assets'][name]
    sdk = CACHE / 'go'
    if sdk.exists():
        shutil.rmtree(sdk)
    extract_zip(download(source['url'], source['sha256'], name), sdk)
    executables = list(sdk.rglob('bin/go.exe' if system == 'windows' else 'bin/go'))
    if len(executables) != 1:
        raise ValueError('Unexpected Go SDK archive layout')
    go = executables[0]
    for path, checksum in lock['go']['source_fingerprints'].items():
        if digest(go.parent.parent / path) != checksum:
            raise ValueError('Win7 Go SDK source fingerprint mismatch: ' + path)
    runtime = CACHE / 'runtime'
    if runtime.exists():
        shutil.rmtree(runtime)
    runtime.mkdir(parents=True)
    record = json.loads((COMPAT / 'ci-lock.json').read_text())['win7_runtime']
    archive = download(record['url'], record['sha256'], 'codex-win7-runtime.tar.gz')
    allowed = set(lock['runtime']) | {'LICENSE.codex', 'NOTICE.codex'}
    with tarfile.open(archive) as bundle:
        entries = bundle.getmembers()
        if {p.name for p in entries} != allowed or len(entries) != len(allowed) or any(not p.isfile() for p in entries):
            raise ValueError('Unexpected Win7 runtime archive contents')
        bundle.extractall(runtime, filter='data')
    for name, expected in lock['runtime'].items():
        if (runtime / name).stat().st_size != expected['bytes'] or digest(runtime / name) != expected['sha256']:
            raise ValueError('Unverified Win7 runtime file: ' + name)
    return go, runtime


def metadata(version, output=None, target='all'):
    version = normalize(version)
    if target != 'all' and target not in TARGETS:
        raise ValueError('Unsupported target: ' + target)
    values = {'version': 'v' + version, 'app_version': version, 'prerelease': str('-' in version).lower(),
              'matrix': json.dumps({'include': [{'target': name, 'runner': runner} for name, runner in TARGETS.items() if target == 'all' or name == target]})}
    text = ''.join(f'{key}={value}\n' for key, value in values.items())
    if output:
        with Path(output).open('a') as stream:
            stream.write(text)
    print(text, end='')
    return values


def export_source(destination):
    if destination.exists():
        shutil.rmtree(destination)
    destination.mkdir(parents=True)
    data = subprocess.check_output(['git', '-c', 'core.autocrlf=false', 'archive', 'HEAD'], cwd=ROOT)
    with tarfile.open(fileobj=io.BytesIO(data)) as archive:
        archive.extractall(destination, members=[m for m in archive.getmembers() if not m.name.startswith('compat/')], filter='data')


def audit_elf(directory, arch):
    records = []
    machine = {'amd64': 62, 'arm64': 183}[arch]
    for path in sorted(Path(directory).rglob('*')):
        if not path.is_file() or path.is_symlink():
            continue
        with path.open('rb') as stream:
            header = stream.read(20)
        if header[:4] != b'\x7fELF':
            continue
        if len(header) != 20 or header[5] != 1 or int.from_bytes(header[18:20], 'little') != machine:
            raise ValueError('Wrong ELF architecture: ' + str(path))
        text = subprocess.check_output(['readelf', '--version-info', str(path)], text=True, env=dict(os.environ, LC_ALL='C'))
        record = {'path': path.relative_to(directory).as_posix(), 'sha256': digest(path)}
        for symbol, maximum in [('GLIBC', (2, 31)), ('GLIBCXX', (3, 4, 28))]:
            versions = [tuple(map(int, v.split('.'))) for v in re.findall(r'Name: ' + symbol + r'_([0-9.]+)', text)]
            if versions and max(versions) > maximum:
                raise ValueError(f'{path} requires {symbol}_{max(versions)}, exceeds Kylin V10 baseline {maximum}')
            record[symbol] = '.'.join(map(str, max(versions))) if versions else None
        records.append(record)
    if not records:
        raise ValueError('No ELF files found in Kylin package')
    return records


def stage_win7(version, env):
    go, runtime = win7_tools()
    run([sys.executable, COMPAT / 'build-app.py', '--go', go, '--codex-bundle', runtime], env=env)
    candidates = list((ROOT / 'dist/win7').glob(f'CSGClaw-Win7-x64-{version}-*-Setup.exe'))
    if len(candidates) != 1:
        raise ValueError('Expected one clean Win7 installer')
    installer = candidates[0]
    name = f'csgclaw-desktop_v{version}_win7_amd64.exe'
    shutil.copy2(installer, OUTPUT / name)
    shutil.copy2(installer.with_suffix('.exe.build.json'), OUTPUT / (name + '.build.json'))


def stage_kylin(version, arch, env):
    source = CACHE / ('kylin-' + arch)
    export_source(source)
    patch_env = dict(os.environ, GIT_CEILING_DIRECTORIES=str(source.parent))
    for arguments in (['--check'], []):
        run(['git', 'apply', *arguments, COMPAT / 'patches/kylin-packaging.patch'], cwd=source, env=patch_env)
    env = dict(env, CGO_ENABLED='0', VERSION='v' + version + '+local', CSGCLAW_DESKTOP_VERSION='v' + version,
               CSGCLAW_DESKTOP_UPDATE_BASE_URL='', CSGCLAW_DESKTOP_UPDATE_CHANNELS_URL='')
    run(['make', 'desktop-package', 'TARGET_OS=linux', 'TARGET_ARCH=' + arch, 'VERSION=v' + version + '+local'], cwd=source, env=env)
    packages = list((source / 'desktop/out/make').rglob('*.deb'))
    if len(packages) != 1:
        raise ValueError('Expected one Kylin DEB')
    stage = CACHE / ('deb-' + arch)
    if stage.exists():
        shutil.rmtree(stage)
    run(['dpkg-deb', '--raw-extract', packages[0], stage])
    update_config = json.loads((stage / 'usr/lib/opencsg-csgclaw-desktop/resources/desktop-update.json').read_text())
    if update_config != {'disabled': True, 'base_url': '', 'channels_base_url': ''}:
        raise ValueError('Kylin package must not use the ordinary desktop update feed')
    audit = audit_elf(stage, arch)
    name = f'csgclaw-desktop_v{version}_kylin-v10_{arch}.deb'
    # Kylin's dpkg 1.19 cannot read newer zstd-compressed control/data archives.
    run(['dpkg-deb', '-Zxz', '-z3', '--root-owner-group', '--build', stage, OUTPUT / name])
    (OUTPUT / (name + '.elf.json')).write_text(json.dumps(audit, indent=2) + '\n')
    smoke_kylin(OUTPUT / name)


def smoke_kylin(package):
    image = json.loads((COMPAT / 'ci-lock.json').read_text())['kylin_smoke_image']
    # The container is disposable. --no-sandbox is confined to the smoke command,
    # never written into the package launcher or production desktop settings.
    script = r'''set -eu
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y --no-install-recommends xvfb dbus-x11 ca-certificates /artifacts/''' + package.name + r'''
getconf GNU_LIBC_VERSION | grep '2.31'
app=/usr/lib/opencsg-csgclaw-desktop
"$app/resources/backend/csgclaw/bin/csgclaw" --version
"$app/resources/backend/csgclaw/bin/codex" --version
test -n "$(find "$app/resources/backend/csgclaw/codex-resources" -type f -name bwrap -perm /111 -print -quit)"
timeout 30s xvfb-run -a "$app/CSGClaw" --no-sandbox --disable-gpu --user-data-dir=/tmp/desktop-smoke > /tmp/desktop.log 2>&1 &
pid=$!
sleep 12
kill -0 "$pid" || { cat /tmp/desktop.log; exit 1; }
kill "$pid"
'''
    run(['docker', 'run', '--rm', '--init', '-v', str(OUTPUT) + ':/artifacts:ro', image, 'bash', '-c', script])


def build(target, version):
    version = normalize(version)
    OUTPUT.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ, CSGCLAW_WIN7_VERSION=version, GOTOOLCHAIN='local', GOWORK='off', CGO_ENABLED='0', ELECTRON_SKIP_BINARY_DOWNLOAD='1')
    if target == 'win7-amd64':
        stage_win7(version, env)
    elif target in TARGETS:
        stage_kylin(version, target.rsplit('-', 1)[1], env)
    else:
        raise ValueError('Unsupported target: ' + target)
    files = sorted(p for p in OUTPUT.iterdir() if p.is_file() and not p.name.startswith('SHA256SUMS'))
    (OUTPUT / ('SHA256SUMS-' + target + '.txt')).write_text(''.join(f'{digest(p)}  {p.name}\n' for p in files))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    p = sub.add_parser('metadata'); p.add_argument('--version', required=True); p.add_argument('--output'); p.add_argument('--target', choices=['all', *TARGETS], default='all')
    p = sub.add_parser('build'); p.add_argument('--version', required=True); p.add_argument('--target', choices=TARGETS, required=True)
    args = parser.parse_args()
    if args.command == 'metadata':
        metadata(args.version, args.output, args.target)
    else:
        build(args.target, args.version)


if __name__ == '__main__':
    main()
