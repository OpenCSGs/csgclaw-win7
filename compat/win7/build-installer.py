#!/usr/bin/env python3
"""Wrap a verified Win7 desktop distribution in a per-user NSIS installer."""
import argparse
from datetime import datetime
import hashlib
import json
import os
from pathlib import Path, PureWindowsPath
import re
import shutil
import subprocess
import tempfile
from zoneinfo import ZoneInfo

ROOT = Path(__file__).resolve().parents[2]
COMPAT = ROOT / 'compat/win7'
REQUIRED = {
    'CSGClaw.exe', 'ffmpeg.dll', 'libEGL.dll', 'libGLESv2.dll', 'LICENSE.noto-emoji',
    'resources/app.asar', 'resources/csgclaw.ico',
    'resources/backend/csgclaw/bin/csgclaw.exe',
    'resources/backend/csgclaw/bin/csgclaw-cli.exe',
    'resources/backend/csgclaw/bin/codex.exe',
    'resources/backend/csgclaw/bin/winpty.dll',
    'resources/backend/csgclaw/bin/winpty-agent.exe',
    '使用说明.zh.md',
}


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def safe_path(name):
    path = PureWindowsPath(name)
    if (not name or any(c in name for c in '\\:\n\r$"<>|?*')
            or path.is_absolute() or path.drive or '..' in path.parts
            or any(part.endswith((' ', '.')) or PureWindowsPath(part).is_reserved() for part in path.parts)):
        raise ValueError('Unsafe installer path: ' + repr(name))
    return name


def verified_files(application):
    manifest = json.loads((application / 'win7-build.json').read_text())
    files = manifest['files']
    if REQUIRED - files.keys():
        raise ValueError('Incomplete desktop manifest: ' + ', '.join(sorted(REQUIRED - files.keys())))
    identities = set()
    for name, expected in files.items():
        safe_path(name)
        if name.casefold() in identities:
            raise ValueError('Duplicate Windows path: ' + name)
        identities.add(name.casefold())
        path = application / name
        if not path.resolve().is_relative_to(application.resolve()) or path.is_symlink():
            raise ValueError('Desktop input must be a regular distribution file: ' + name)
        if not path.is_file() or digest(path) != expected:
            raise ValueError('Desktop file missing or modified: ' + name)
    actual = {p.relative_to(application).as_posix() for p in application.rglob('*') if p.is_file()}
    if actual != set(files) | {'win7-build.json'}:
        raise ValueError('Unrecorded files in desktop distribution')
    return manifest, {name: application / name for name in files}


def quoted(value):
    return '"' + str(value).replace('$', '$$').replace('"', '$\\"') + '"'


def write_includes(files, directory):
    install, uninstall, checks = [], [], ['!macro CheckPayloadClosed PREFIX']
    folders = set()
    for name, source in sorted(files.items()):
        parts = PureWindowsPath(name)
        folder = str(parts.parent)
        target = '$PayloadDir' + ('\\' + folder if folder != '.' else '')
        install += [f'SetOutPath "{target}"', f'File {quoted("/oname=" + parts.name)} {quoted(source)}']
        uninstall.append('Delete "$INSTDIR\\' + name.replace('/', '\\') + '"')
        if parts.suffix.lower() in ('.exe', '.dll'):
            checks += ['Push "$INSTDIR\\' + name.replace('/', '\\') + '"', 'Call ${PREFIX}CheckFileClosed']
        for parent in parts.parents:
            if str(parent) != '.':
                folders.add(str(parent))
    # Empty-directory removal never erases files added by the user.
    uninstall += [f'RMDir "$INSTDIR\\{folder}"' for folder in sorted(folders, key=lambda p: (-p.count('\\'), p))]
    checks.append('!macroend')
    for name, lines in [('install-files.nsh', install), ('uninstall-files.nsh', uninstall), ('check-running.nsh', checks)]:
        (directory / name).write_text('\n'.join(lines) + '\n')


def compile_installer(arguments, script, output):
    """A failed compiler must never replace an already verified installer."""
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.win7-setup-', dir=output.parent) as directory:
        candidate = Path(directory) / output.name
        subprocess.run([*arguments, '-DSETUP_OUTPUT=' + str(candidate), str(script)], check=True)
        candidate.replace(output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--application', type=Path, default=ROOT / 'dist/win7/app/CSGClaw-win32-x64')
    parser.add_argument('--makensis', default=shutil.which('makensis'))
    parser.add_argument('--go', type=Path, required=True, help='Pinned Win7 Go SDK for the transaction helper')
    parser.add_argument('--output-dir', type=Path, default=ROOT / 'dist/win7')
    parser.add_argument('--allow-dirty', action='store_true', help='Local acceptance only; clearly mark uncommitted inputs')
    args = parser.parse_args()
    if not args.makensis:
        parser.error('Install NSIS 3 (macOS: brew install makensis) or provide --makensis')
    application = args.application.resolve()
    manifest, files = verified_files(application)
    lock = json.loads((COMPAT / 'sources.lock.json').read_text())
    if manifest['version'] != lock['app_version']:
        parser.error('Desktop version differs from sources.lock.json; rebuild the desktop first')
    for name, record in lock['runtime'].items():
        if digest(application / 'resources/backend/csgclaw/bin' / name) != record['sha256']:
            parser.error('Unverified Win7 runtime: ' + name)
    commit = manifest['source_commit']
    if not re.fullmatch(r'[0-9a-f]{40}', commit):
        parser.error('Desktop manifest must contain its real full source commit')
    source_order = int(subprocess.check_output(['git', 'show', '-s', '--format=%ct', commit], cwd=ROOT, text=True))
    now = datetime.now(ZoneInfo('Asia/Shanghai'))
    version = now.strftime('%Y%m%d') + '-' + commit[:8]
    dirty = bool(manifest.get('source_dirty')) or bool(subprocess.check_output(
        ['git', 'status', '--porcelain', '--', 'compat/win7'], cwd=ROOT, text=True).strip())
    if dirty and not args.allow_dirty:
        parser.error('Commit the Win7 changes and rebuild the desktop before distribution; --allow-dirty is for local acceptance only')
    if dirty:
        version += '-dirty'
    numeric = f'{now.year}.{now.month}.{now.day}.{now.hour * 60 + now.minute}'
    manifest.update(build_id=version, source_order=source_order)
    cache = ROOT / '.cache/win7/installer-builds'
    cache.mkdir(parents=True, exist_ok=True)
    work = Path(tempfile.mkdtemp(prefix='build-', dir=cache))
    go = args.go.resolve()
    env = dict(os.environ, GOOS='windows', GOARCH='amd64', CGO_ENABLED='0', GOTOOLCHAIN='local', GOWORK='off')
    goroot = Path(subprocess.check_output([go, 'env', 'GOROOT'], env=env, text=True).strip())
    for name, expected in lock['go']['source_fingerprints'].items():
        if digest(goroot / name) != expected:
            parser.error('Go SDK does not match the pinned Win7 source: ' + name)
    helper = work / 'upgrade.exe'
    subprocess.run([go, 'build', '-trimpath', '-ldflags=-s -w', '-o', helper, '.'], cwd=COMPAT / 'installer', env=env, check=True)
    compiler = subprocess.check_output([args.makensis, '-VERSION'], text=True).strip()
    if compiler != 'v' + lock['installer']['version']:
        parser.error('Use the pinned NSIS compiler: ' + lock['installer']['version'])
    sources = ['build-installer.py', 'installer.nsi', 'USER.zh.md', 'sources.lock.json', 'installer/main.go', 'installer/go.mod']
    installer = {
        'compiler': compiler,
        'source_commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
        'source_dirty': bool(subprocess.check_output(['git', 'status', '--porcelain', '--', 'compat/win7'], cwd=ROOT, text=True).strip()),
        'inputs': {name: digest(COMPAT / name) for name in sources},
        'build_id': version,
        'helper_sha256': digest(helper),
    }
    # Override only the usage guide; application binaries retain their verified hashes.
    files['使用说明.zh.md'] = COMPAT / 'USER.zh.md'
    manifest['files']['使用说明.zh.md'] = digest(COMPAT / 'USER.zh.md')
    manifest['installer'] = installer
    (work / 'win7-build.json').write_text(json.dumps(manifest, indent=2) + '\n')
    files['win7-build.json'] = work / 'win7-build.json'
    write_includes(files, work)
    output = args.output_dir.resolve() / f'CSGClaw-Win7-x64-{version}-Setup.exe'
    compile_installer([
        args.makensis, '-V3', '-WX', '-DGENERATED=' + str(work),
        '-DAPP_VERSION=' + version,
        '-DNUMERIC_VERSION=' + numeric, '-DAPP_ICON=' + str(application / 'resources/csgclaw.ico'),
        '-DUPGRADE_HELPER=' + str(helper), '-DSOURCE_COMMIT=' + commit, '-DSOURCE_ORDER=' + str(source_order),
        '-DINSTALLED_KB=' + str((sum(p.stat().st_size for p in files.values()) + 1023) // 1024),
    ], COMPAT / 'installer.nsi', output)
    output.with_suffix('.exe.sha256').write_text(f'{digest(output)}  {output.name}\n')
    record = dict(installer, payload_source_commit=manifest['source_commit'], payload_files=len(files), sha256=digest(output))
    output.with_suffix('.exe.build.json').write_text(json.dumps(record, indent=2) + '\n')
    print(output)


if __name__ == '__main__':
    main()
