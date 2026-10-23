#!/usr/bin/env python3
"""Build tiny, isolated installers exercising the production NSIS upgrade path."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[4]
COMPAT = ROOT / 'compat/win7'
PRODUCT = 'OpenCSG.CSGClaw.Win7.UpgradeAcceptance'
NAME = 'CSGClaw Win7 Upgrade Test'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ, GOOS='windows', GOARCH='amd64', CGO_ENABLED='0', GOWORK='off', GOTOOLCHAIN='local')
    go = args.go.resolve()
    spec = importlib.util.spec_from_file_location('installer', COMPAT / 'build-installer.py')
    builder = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(builder)
    subprocess.run([go, 'build', '-o', output / 'upgrade-acceptance.exe', Path(__file__).with_name('main_windows.go')], env=env, check=True)
    subprocess.run([go, 'test', '-c', '-o', output / 'upgrade-native-tests.exe'], cwd=COMPAT / 'installer', env=env, check=True)
    helper = output / 'upgrade-helper.exe'
    subprocess.run([go, 'build', '-trimpath', '-ldflags=-s -w -X main.product=' + PRODUCT, '-o', helper, '.'], cwd=COMPAT / 'installer', env=env, check=True)
    for kind, order, commit in [('old', 100, 'a'*40), ('new', 200, 'b'*40), ('downgrade', 100, 'a'*40)]:
        directory = output / kind
        directory.mkdir(exist_ok=True)
        files = {'CSGClaw.exe': output / 'upgrade-acceptance.exe'}
        for name, body in {'resources/app.asar': kind, ('obsolete.dll' if kind == 'old' else 'replacement.dll'): kind}.items():
            path = directory / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(body)
            files[name] = path
        manifest = directory / 'win7-build.json'
        manifest.write_text(json.dumps(dict(source_commit=commit, build_id=kind, source_order=order,
            files={name: builder.digest(path) for name, path in files.items()})))
        files['win7-build.json'] = manifest
        builder.write_includes(files, directory)
        script = COMPAT / 'installer.nsi'
        if kind == 'old':
            # The committed pre-upgrade installer is the actual upgrade baseline.
            legacy = subprocess.check_output(['git', 'show', '070609b4:compat/win7/installer.nsi'], cwd=ROOT, text=True)
            legacy = legacy.replace('OpenCSG.CSGClaw.Win7', PRODUCT).replace('CSGClawWin7', PRODUCT).replace('CSGClaw Win7', NAME)
            script = directory / 'legacy.nsi'
            script.write_text(legacy)
            include = directory / 'install-files.nsh'
            include.write_text(include.read_text().replace('$PayloadDir', '$INSTDIR'))
        options = [
            'makensis', '-V2', '-WX', '-DGENERATED=' + str(directory),
            '-DAPP_VERSION=' + kind, '-DNUMERIC_VERSION=2026.10.20.' + str(order),
            '-DAPP_ICON=' + str(ROOT / 'dist/win7/app/CSGClaw-win32-x64/resources/csgclaw.ico'),
            '-DINSTALLED_KB=8192',
        ]
        if kind != 'old':
            options += ['-DUPGRADE_HELPER=' + str(helper), '-DSOURCE_COMMIT=' + commit,
                        '-DSOURCE_ORDER=' + str(order), '-DPRODUCT_ID=' + PRODUCT, '-DPRODUCT_NAME=' + NAME]
        builder.compile_installer(options, script, output / ('upgrade-fixture-' + kind + '.exe'))


if __name__ == '__main__':
    main()
