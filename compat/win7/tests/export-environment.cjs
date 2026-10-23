// Export the actual compiled sidecar command, including its current key list.
const fs = require('node:fs');
const path = require('node:path');
const { createRequire } = require('node:module');
const source = path.resolve(process.argv[2]);
const output = path.resolve(process.argv[3]);
const exportsObject = {};
const capture = new Function('require', 'exports', 'module', fs.readFileSync(source, 'utf8') + '\nreturn WINDOWS_ENVIRONMENT_SCRIPT;');
fs.writeFileSync(output, capture(createRequire(source), exportsObject, { exports: exportsObject }));
