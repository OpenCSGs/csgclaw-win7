# Win7 compatibility layer

See [the desktop build and acceptance guide](README.zh.md) for the complete Win7 application pipeline.

The repository root retains the CSGClaw upstream directory layout and sources.
Win7 changes live here and are applied to an ignored build tree, rather than moving or duplicating application directories.
The imported CSGClaw commit, Codex commit, patch digests and previously verified runtime digests are recorded in `sources.lock.json`.
The current CSGClaw baseline is upstream main `6fee9a0ee9d13cc9b4054845c33e90b06ba3a300` (2026-11-14), tagged `v0.11.0-beta.6`.
HTTP OpenCSG login and its regression tests come directly from upstream PR #787; no duplicate compatibility patch is applied.
Unrelated local work in the original CSGClaw checkout is excluded.

## Source layout

- `patches/csgclaw.patch`: the verified Go lifecycle, terminal and browser fixes, including native regression tests.
- `codex/`: the complete adapted Codex source, including licenses, Win7 runtime changes and build instructions.
- `patches/codex.patch`: provenance of the initial Codex import; it is no longer applied during builds.
- `patches/previous-desktop-port.patch`: the existing Electron 22 port preserved from this repository before synchronization, for migration and provenance.
- `REPORT.zh.md` and `verification-20261017.json`: the earlier Go + Codex acceptance scope and results, not acceptance of the new desktop build.

## Prepare and build

Use Python 3.12 or newer and the existing supported Node 24/pnpm host tools.
Prepare a disposable source tree before building.

```sh
python3 compat/win7/prepare.py
python3 compat/win7/prepare.py --codex
python3 compat/win7/build-backend.py --go /path/to/go-win7/bin/go --codex-bundle /path/to/verified/runtime
```

Codex is exported from `compat/win7/codex` in the selected Git revision.
No separate Codex checkout or upstream Git fetch is required.
Edit and commit the checked-in Codex source; `.cache/win7/codex` is disposable build output.
Cargo, Rust SDK and WinPTY dependencies still follow the documented build prerequisites.
The full build produces a per-user Windows installer using the pinned NSIS 3.13 compiler (`brew install makensis` on macOS).
The deliverable is `Setup.exe` with its checksum and build record; no portable ZIP is produced.
The unpacked desktop directory is an intermediate installer input.
Run `python3.12 compat/win7/build-installer.py --go /path/to/pinned-win7-go/bin/go` to wrap an existing verified desktop folder without rebuilding its application binaries.
The installer includes every runtime dependency, creates desktop/Start Menu shortcuts and an uninstall entry, and preserves user profiles and additional user files when uninstalling.
Installers use `CSGClaw-Win7-x64-VERSION-YYYYMMDD-COMMIT-Setup.exe` names and automatically discover previous installations.
See [installer upgrades](INSTALLER-UPGRADE.zh.md) for repair, downgrade confirmation, rollback, and native acceptance.
It rejects incomplete or changed payloads and refuses to replace files while the installed application is running.
The prepared Codex tree contains `compat/win7/build-macos.sh` and the pinned WinPTY build script.
The Go toolchain is XTLS/go-win7 `patched-1.27.2`; current Go dependencies are retained.
The verified runtime set is Codex `rust-v0.149.1` plus our Win7 patch and matching WinPTY DLL/agent/license.
The backend records the application version, source commit and build time.
The installer also includes Feishu CLI `1.0.97`, rebuilt from its checksum-pinned official source with the same Win7 Go SDK, and its pinned Skills.
The connector imports this verified local package before trying an online download, so first connection does not depend on GitHub access or an incompatible ordinary Windows CLI.
Its `+local` suffix preserves the upstream custom-install guard; the desktop application reports version `0.11.0-beta.6`.

## Merge future CSGClaw updates

Rebase Win7 commits onto the selected upstream release, preserving upstream commit SHAs and messages.
Never create synchronization merge commits.
Every locally authored commit uses the `win7` scope.
Keep root files identical to the adopted upstream revision and maintain adaptations only under `compat/win7/`.
Do not edit generated files under `.cache/win7`.
Run `prepare.py` after each synchronization; a conflicting patch must fail rather than silently dropping a compatibility fix.
Resolve the compatibility patch against the new upstream source, review its digest change and repeat native Win7 acceptance.
Update the CSGClaw base in `sources.lock.json` when adopting a new upstream baseline.
Codex is pinned separately, so a CSGClaw update does not implicitly replace the Win7 runtime with an incompatible Windows binary.

## Review boundaries

The history consists of the complete adopted CSGClaw upstream history followed by the original Win7 commits in order and subsequent Win7 maintenance commits.
The adaptation commits contain the compatibility overlays, adapted Codex source, desktop installer build and historical acceptance records.
The current upstream baseline includes the shared fixes from PRs #721 and #722.
History cleanup is not a new native acceptance run; older results retain their original tested revisions.
DSH/Node remains a separately tracked dependency; a desktop shell using Electron's Node runtime does not validate the independent DSH engine.
Local DSH context and memory plugins use escaped `file:` URLs so Windows drive letters and filename characters remain valid Node ESM module references.
DSH initialization allows up to two minutes for cold profile loading, while still respecting caller cancellation and earlier deadlines.

See [native desktop acceptance](DESKTOP-ACCEPTANCE.zh.md) and [fixture replay instructions](tests/README.zh.md).

The Windows workflow fixes from upstream PR #814 are inherited directly, including DSH module URLs and startup deadlines, Feishu authorization URL validation, CLI startup deadlines, and native Windows process identity.
Duplicate overlays and patch hunks have been removed.
The Win7 layer retains the bundled compatible Feishu CLI, automatic local package discovery, and opaque delete-dialog colors for Chromium 108.

## 2026-11-13 synchronization and connector checks

The compatibility build now uses the upstream web workspace and CSS modules.
The Electron 22 HTML preview server tracks and closes its sockets without Node 18-only APIs.
Codex process startup retains Win7 child-job support after upstream moved stderr management into diagnostics.
`api/connector_live_test.go` exercises the real MCP marketplace and Feishu authorization, cancellation, and retry over HTTP.
Set `CSGCLAW_TEST_MCP_AUTH_FILE` to a private JSON fixture containing `hub_url` and `access_token`, and `CSGCLAW_TEST_CONNECTOR_PACKAGES` to the shipped connector package directory.
These opt-in checks do not grant Feishu application or personal account access.
Native results must be recorded separately from host checks.
