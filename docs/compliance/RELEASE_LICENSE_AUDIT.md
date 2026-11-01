# SyndiRelay binary-release license audit (reviewed 2026-11-01)

**Do not make this repository public or upload binary Releases solely because the core code is MIT.** License compatibility, bundled notices, platform API authorization and clean-machine operation are separate release conditions.

## Dependency evidence

- The six `CGO_ENABLED=0` target graphs (`windows`, `darwin`, `linux` x `amd64`, `arm64`) currently compile against **seven distinct non-stdlib Go modules**. The pinned `go.mod` has **nine direct/indirect requirements**, all of which must have reviewed license notices before a release (including noncompiled requirements, conservatively).
- The pinned npm `package-lock.json` has **61 third-party package entries**, including optional architecture-specific packages. On the inspected Linux machine, 35 were present, 26 optional/platform-dependent packages were not installed, and the installed `@img/sharp-libvips-linux-x64` declared `LGPL-3.0-or-later` but had no separate complete license-text file available in the package root. Do **not** infer the absent optional packages' licenses merely from Linux results. Official upstream licensing evidence and proper LGPL attribution/redistribution obligations require review.
- Sharp itself declares Apache-2.0; the precompiled libvips package has a separate LGPL licensing situation. The app currently distributes npm lock/runtime manifests rather than bundling `node_modules`; users install runtime packages with `npm ci`. This does not eliminate notice obligations or third-party license review.
- The old `tls-client`/`fhttp` fingerprinting transport is absent from the current Go module. The independently operating legacy BlogCTL binary still uses it; previous releases are not retroactively remediated.

## Release packaging enforcement

The `go run ./cmd/maint package` command now checks `.build/out/publish-licenses/MANIFEST.json` **before creating or deleting release artifacts**. Its schema is:

```json
{
  "schema": 1,
  "dependencies": [
    {
      "ecosystem": "go",
      "name": "github.com/example/library",
      "version": "v1.2.3",
      "license": "MIT",
      "file": "go/github.com/example/library/LICENSE",
      "sha256": "<64 lowercase hexadecimal characters>",
      "reviewed": true
    }
  ]
}
```

This is only a format example: **a release manifest must list *all* actual pinned Go and npm packages**, including optional npm platform packages, and each entry must point to its own complete, local, nonempty, reviewed license text with a matching SHA-256 digest. Duplicate entries, stale versions, unsafe paths, missing files, missing reviews and hash mismatches cause packaging to fail. The example does not authorize reuse of a generic MIT text across dependencies.

`reviewed=true` is a human sign-off indicator, not proof that rights have been granted. Independent legal/terms verification and runtime OS testing remain necessary. Dependency license files and local reports under `.build/` remain outside source control until intentionally included with a reviewed release.

## Outstanding before public Release

1. Collect and review current upstream license texts for all nine Go requirements and 61 npm lock entries, including optional macOS/Windows/Linux Sharp+libvips binaries. Resolve the missing libvips license-text evidence and any LGPL obligations.
2. Produce the pinned manifest and independently verify every SHA-256 checksum against original license files; do not invent or substitute license text.
3. Validate complete installation and rendering on actual macOS/Windows/Linux environments. GitHub Actions currently has green 3-OS build/test runs; those do **not** verify real installation/Extension/remote publishing or complete license redistribution.
4. Conduct platform-by-platform authorization review for authenticated publishing, especially 51CTO and Medium; read-only API checks do not prove remote writes.
5. Once all conditions pass, create Release packages, verify their contents and checksums and preserve existing user Data/ and extension identities.
