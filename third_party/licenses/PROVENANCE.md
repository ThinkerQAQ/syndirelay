# Pinned third-party license evidence — 2026-11-05

This directory contains only upstream license evidence and a pinned manifest. No business code or root README is modified.

## Verification

- 61 npm tarballs were obtained from official npm registry and all bytes verified against package-lock.json integrity hashes.
- 9 pinned Go modules were obtained from Go module cache and contain original LICENSE texts.
- 10 additional transitive Go modules were inspected from the complete go list -m all graph and included under go-transitive/, with checksums in GO_TRANSITIVE_MANIFEST.json. The existing Go gate itself only checks direct requirements.
- 70 tracked packages; 56 license records individually verified; 14 LGPL binary redistribution records are blocked.

## LGPL binary distribution is not yet approved

- Sharp embeds prebuilt libvips and further native libraries. Copying their text is not proof of full LGPL binary distribution compliance.
- Official unmodified LGPL-3.0 text: https://www.gnu.org/licenses/lgpl-3.0.txt SHA256 e3a994d82e644b03a792a930f574002658412f62407f5fee083f2555c5f23118
- Official unmodified GPL-3.0 text: https://www.gnu.org/licenses/gpl-3.0.txt SHA256 3972dc9744f6499f0f9b2dbf76696f2ae7ad8af9b23dde66d6af86c9dfb36986
- Exact pinned Sharp/libvips source recipes: https://github.com/lovell/sharp-libvips/tree/v1.3.3
- Exact upstream release: https://github.com/lovell/sharp-libvips/releases/tag/v1.3.3
- Source recipe versions.properties SHA256 02253a2d39a6b1cfd2c1deacb0947f7d2a3398a522236f7efc4d57e82cbfe980 identifies libvips 8.18.6, glib 2.89.4, libheif 1.23.2 and additional dependencies.
- Original npm README files and licensing matrices for 10 LGPL libvips variants are retained beside the full GNU license texts.
- 14 LGPL manifest entries stay reviewed=false until matching source/patches/build conditions, notices, replacement/relink requirements, and delivery are validated for the shipped binaries.

## Existing project license gate (no code changes needed)

The production scripts require a generated license bundle in the ignored .build/out/publish-licenses directory.
To check this repository-only material: create that directory, copy these license files into it, and run go run ./cmd/maint licenses.
The release gate is expected to BLOCK the 14 unapproved LGPL entries. Do not override or bypass this check.

## Records awaiting binary redistribution approval
- node_modules/@img/sharp-libvips-darwin-arm64@1.3.3
- node_modules/@img/sharp-libvips-darwin-x64@1.3.3
- node_modules/@img/sharp-libvips-linux-arm@1.3.3
- node_modules/@img/sharp-libvips-linux-arm64@1.3.3
- node_modules/@img/sharp-libvips-linux-ppc64@1.3.3
- node_modules/@img/sharp-libvips-linux-riscv64@1.3.3
- node_modules/@img/sharp-libvips-linux-s390x@1.3.3
- node_modules/@img/sharp-libvips-linux-x64@1.3.3
- node_modules/@img/sharp-libvips-linuxmusl-arm64@1.3.3
- node_modules/@img/sharp-libvips-linuxmusl-x64@1.3.3
- node_modules/@img/sharp-wasm32@0.35.4
- node_modules/@img/sharp-win32-arm64@0.35.4
- node_modules/@img/sharp-win32-ia32@0.35.4
- node_modules/@img/sharp-win32-x64@0.35.4
