#!/usr/bin/env python3
"""Apply Win7 build overlays to prepared sources, leaving upstream intact."""
import hashlib
import json
from pathlib import Path
import shutil

ROOT = Path(__file__).resolve().parents[2]
COMPAT = ROOT / 'compat/win7'
SOURCE = ROOT / '.cache/win7/csgclaw'


def configure_emoji_fonts(web):
    lock = json.loads((COMPAT / 'sources.lock.json').read_text())
    fonts = COMPAT / 'web/fonts'
    for name, expected in lock['emoji_font']['files'].items():
        if hashlib.sha256((fonts / name).read_bytes()).hexdigest() != expected:
            raise ValueError('Win7 emoji font checksum mismatch: ' + name)
    styles = web / 'src/shared/styles'
    (styles / 'fonts').mkdir(parents=True, exist_ok=True)
    for name in lock['emoji_font']['files']:
        shutil.copy2(fonts / name, styles / 'fonts' / name)
    shutil.copy2(COMPAT / 'web/emoji.css', styles / 'emoji.css')
    replace(styles / 'globals.css', '@import "./tokens.css";\n',
            '@import "./tokens.css";\n@import "./emoji.css";\n')
    replace(styles / 'tokens.css', 'Helvetica, Arial, sans-serif;',
            'Helvetica, Arial,\n    "CSGClaw Emoji", sans-serif;')


def replace(path, before, after):
    text = path.read_text()
    if text.count(before) != 1:
        raise ValueError(f'Compatibility context changed in {path.relative_to(SOURCE)}: {before[:60]}')
    path.write_text(text.replace(before, after))


def main():
    for original in (COMPAT / 'go').glob('*.go'):
        target = SOURCE / 'internal/runtime/codex' / original.name
        if target.exists():
            raise ValueError('Win7 Go overlay conflicts with upstream: ' + str(target.relative_to(SOURCE)))
        shutil.copy2(original, target)
    for original in (COMPAT / 'processgroup').glob('*.go'):
        target = SOURCE / 'internal/processgroup' / original.name
        if target.exists():
            raise ValueError('Win7 process group overlay conflicts with upstream: ' + str(target.relative_to(SOURCE)))
        shutil.copy2(original, target)
    desktop = SOURCE / 'desktop'
    package = json.loads((desktop / 'package.json').read_text())
    package['devDependencies']['electron'] = '22.3.27'
    package['devDependencies']['@types/node'] = '16.18.126'
    package['devDependencies']['@types/node-host'] = 'npm:@types/node@24.9.0'
    package['scripts']['test'] = package['scripts']['test'].replace('pnpm run build &&', 'pnpm run build && tsc -p tsconfig.test.json &&') + ' dist/main/updater.test.js dist/main/legacyWebContentsView.test.js dist/main/legacyViewportFit.test.js'
    (desktop / 'package.json').write_text(json.dumps(package, indent=2) + '\n')
    replace(desktop / 'pnpm-workspace.yaml', 'allowBuilds:\n', 'allowBuilds:\n  electron: false\n')
    config = json.loads((desktop / 'tsconfig.json').read_text())
    config['compilerOptions'].update(target='ES2021', lib=['ES2021', 'DOM'])
    config['exclude'] = ['src/**/*.test.ts']
    (desktop / 'tsconfig.json').write_text(json.dumps(config, indent=2) + '\n')
    for name, target in [('updater.ts','src/main/updater.ts'),('updater.test.ts','src/main/updater.test.ts'),('tsconfig.test.json','tsconfig.test.json')]:
        shutil.copy2(COMPAT / 'desktop' / name, desktop / target)
    replace(desktop / 'src/main/sidecar/childEnvironment.ts', 'path.join(homeDirectory, ".docker", "bin")', 'path.posix.join(homeDirectory, ".docker", "bin")')
    replace(desktop / 'src/main/sidecar/childEnvironment.ts', 'path.join(homeDirectory, ".local", "bin")', 'path.posix.join(homeDirectory, ".local", "bin")')
    environment = desktop / 'src/main/sidecar/childEnvironment.ts'
    replace(environment, '[Text.UTF8Encoding]::new($false)', '(New-Object Text.UTF8Encoding)')
    replace(environment, '$parts = @($machine, $user) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }', '$parts = @(@($machine, $user) | Where-Object { $_ -and $_.Trim() })')
    replace(environment, '-not [string]::IsNullOrWhiteSpace($user)', '$user -and $user.Trim()')
    replace(environment, '-not [string]::IsNullOrWhiteSpace($machine)', '$machine -and $machine.Trim()')
    replace(environment, '[Console]::Out.Write(($result | ConvertTo-Json -Compress))', 'Add-Type -AssemblyName System.Web.Extensions\n$serializer = New-Object System.Web.Script.Serialization.JavaScriptSerializer\n[Console]::Out.Write($serializer.Serialize($result))')
    for name in ('legacyWebContentsView.ts', 'legacyWebContentsView.test.ts', 'legacyViewportFit.ts', 'legacyViewportFit.test.ts'):
        shutil.copy2(COMPAT / 'desktop' / name, desktop / 'src/main' / name)
    replace(desktop / 'src/main/windowManager.ts', 'import { NativeBrowserManager } from "./nativeBrowser";', 'import { NativeBrowserManager } from "./nativeBrowser";\nimport { installLegacyViewportFit } from "./legacyViewportFit";')
    replace(desktop / 'src/main/windowManager.ts', '    this.mainWindow = window;', '    this.mainWindow = window;\n    installLegacyViewportFit(window);')
    browser = desktop / 'src/main/nativeBrowser.ts'
    replace(browser, '  WebContentsView,\n', '')
    replace(browser, 'type Page = {', 'import { LegacyWebContentsView as WebContentsView } from "./legacyWebContentsView";\n\ntype Page = {')
    replace(browser, 'this.window.contentView.addChildView(view);', 'view.attach(this.window);')
    replace(browser, 'this.window.contentView.removeChildView(page.view);', 'page.view.detach();')
    replace(browser, 'contents.on("will-frame-navigate", (event) => {\n      if (!event.isMainFrame && !event.url.startsWith("file:")) return;', 'contents.on("will-navigate", (event, url) => {')
    replace(browser, 'browserURL(event.url, this.appOrigin());', 'browserURL(url, this.appOrigin());')
    replace(browser, 'request.action === \"back\" &&\n        contents.navigationHistory.getActiveIndex() > 0', 'request.action === \"back\" && contents.canGoBack()')
    replace(browser, 'contents.navigationHistory.goToIndex(\n          contents.navigationHistory.getActiveIndex() - 1,\n        )', 'contents.goBack()')
    replace(browser, 'contents.navigationHistory.getActiveIndex() + 1 <\n          contents.navigationHistory.length()', 'contents.canGoForward()')
    replace(browser, 'contents.navigationHistory.goToIndex(\n          contents.navigationHistory.getActiveIndex() + 1,\n        )', 'contents.goForward()')
    replace(browser, 'contents.navigationHistory.getActiveIndex() > 0', 'contents.canGoBack()')
    replace(browser, 'contents.navigationHistory.getActiveIndex() + 1 <\n        contents.navigationHistory.length()', 'contents.canGoForward()')
    web = SOURCE / 'web/app'
    configure_emoji_fonts(web)
    package = json.loads((web / 'package.json').read_text())
    package['devDependencies'].pop('@tailwindcss/vite')
    package['devDependencies'].update(tailwindcss='3.4.17', postcss='8.5.6', autoprefixer='10.4.21')
    (web / 'package.json').write_text(json.dumps(package, indent=2) + '\n')
    for name in ['postcss.config.js','tailwind.config.js']:
        shutil.copy2(COMPAT / 'web' / name, web / name)
    replace(web / 'vite.config.ts', 'import tailwindcss from "@tailwindcss/vite";\n', '')
    replace(web / 'vite.config.ts', '    tailwindcss(),\n', '')
    replace(web / 'vite.config.ts', '  build: {\n', '  build: {\n    target: "chrome108",\n    cssTarget: "chrome108",\n')
    replace(web / 'src/shared/styles/globals.css', '@layer theme, utilities;\n\n@import "tailwindcss/theme.css" layer(theme);\n@import "tailwindcss/utilities.css" layer(utilities);\n', '')
    replace(web / 'src/shared/styles/globals.css', '@import "./inputs.css";\n', '@import "./inputs.css";\n\n@tailwind utilities;\n')
    for directory in ('desktop', 'web/app'):
        overlay = 'desktop' if directory == 'desktop' else 'web'
        shutil.copy2(COMPAT / overlay / 'pnpm-lock.yaml', SOURCE / directory / 'pnpm-lock.yaml')
    print('Configured isolated Electron 22 / Chromium 108 build')


if __name__ == '__main__':
    main()
