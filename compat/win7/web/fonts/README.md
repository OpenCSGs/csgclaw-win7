# Win7 Emoji Font

The Win7 build bundles the Noto Color Emoji COLRv1 font as a local WOFF2 asset.
It supplies missing Emoji glyphs without installing fonts in Windows or requesting a CDN at runtime.
The original text font order remains intact, with `CSGClaw Emoji` immediately before the generic fallback.

The pinned source revision, original TTF checksum, conversion version, bundled WOFF2 checksum and license checksum are recorded in `../../sources.lock.json` under `emoji_font`.
The upstream source is [googlefonts/noto-emoji](https://github.com/googlefonts/noto-emoji).
`LICENSE.noto-emoji` contains the upstream SIL Open Font License 1.1.

To reproduce the conversion, download `source_path` from the pinned revision, verify `source_sha256`, load it with FontTools 4.67.0, set `font.flavor = "woff2"`, and save the font.
Revalidate new fonts in the real Electron 22 / Chromium 108 Win7 application before updating the pinned files.
