import fs from 'node:fs/promises';
import path from 'node:path';
import crypto from 'node:crypto';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const source = path.join(root, '.cache/win7/csgclaw');
const desktop = path.join(source, 'desktop');
const require = createRequire(path.join(desktop, 'package.json'));
const packager = require('@electron/packager').default ?? require('@electron/packager');
const { flipFuses, FuseV1Options, FuseVersion } = require('@electron/fuses');
const lock = JSON.parse(await fs.readFile(path.join(root, 'compat/win7/sources.lock.json')));
const appVersion = lock.app_version;
const archive = path.join(root, '.cache/win7/electron', lock.electron.archive);
const hash = crypto.createHash('sha256').update(await fs.readFile(archive)).digest('hex');
if (hash !== lock.electron.sha256) throw new Error('Electron archive checksum mismatch');
const application = path.join(root, '.cache/win7/application');
await fs.rm(application, { force: true, recursive: true });
await fs.mkdir(application, { recursive: true });
await fs.cp(path.join(desktop, 'dist'), path.join(application, 'dist'), {
  recursive: true,
  filter: (file) => !file.endsWith('.test.js') && !file.endsWith('.map'),
});
for (const name of ['electron-squirrel-startup', 'debug', 'ms']) {
  await fs.cp(await fs.realpath(path.join(desktop, 'node_modules', name)), path.join(application, 'node_modules', name), { recursive: true });
}
await fs.writeFile(path.join(application, 'package.json'), JSON.stringify({
  name: 'csgclaw-win7', productName: 'CSGClaw', version: appVersion,
  description: 'CSGClaw Windows 7 compatibility build', author: 'OpenCSG',
  main: 'dist/main/main.js', license: 'Apache-2.0',
}));
const update = path.join(root, '.cache/win7/desktop-update.json');
await fs.writeFile(update, JSON.stringify({ disabled: true, base_url: '', channels_base_url: '' }));
const icons = path.join(desktop, 'resources/icons');
const extraResource = [path.join(root, 'dist/win7/backend'), update];
for (const name of ['csgclaw-taskbar-light.ico', 'csgclaw-taskbar-dark.ico', 'csgclaw.ico', 'csgclaw-mark-light.svg', 'csgclaw-mark-dark.svg']) extraResource.push(path.join(icons, name));
const outputs = await packager({
  dir: application, out: path.join(root, 'dist/win7/app'), name: 'CSGClaw',
  executableName: 'CSGClaw', platform: 'win32', arch: 'x64',
  electronVersion: lock.electron.version, electronZipDir: path.dirname(archive),
  appVersion: appVersion.split('-')[0], buildVersion: appVersion.split('-')[0],
  icon: path.join(icons, 'csgclaw.ico'), asar: true, overwrite: true, prune: false,
  extraResource, win32metadata: { CompanyName: 'OpenCSG', ProductName: 'CSGClaw' },
});
for (const output of outputs) {
  await flipFuses(path.join(output, 'CSGClaw.exe'), {
    version: FuseVersion.V1,
    [FuseV1Options.RunAsNode]: false,
    [FuseV1Options.EnableCookieEncryption]: true,
    [FuseV1Options.EnableNodeOptionsEnvironmentVariable]: false,
    [FuseV1Options.EnableNodeCliInspectArguments]: false,
    [FuseV1Options.EnableEmbeddedAsarIntegrityValidation]: true,
    [FuseV1Options.OnlyLoadAppFromAsar]: true,
  });
  await fs.copyFile(path.join(root, 'LICENSE'), path.join(output, 'LICENSE.csgclaw'));
  console.log(output);
}
