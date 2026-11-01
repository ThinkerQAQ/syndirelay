# SyndiRelay publication readiness and repository split

Status checked: **2026-11-01**. SyndiRelay remains **private**; this document defines evidence needed before changing visibility and separately before distributing executable files or an extension. This is an engineering/compliance checklist, not an official legal determination.

## Current state

| Area | Evidence | Status |
| --- | --- | --- |
| Repository ownership | Original source uses project MIT license; private Git history has 13 commits | Confirmed |
| History secrets | 568 Git blobs examined for common high-confidence API-key/secret patterns; one credentials-in-URL-looking string in a synthetic compiler test | Preliminary; manual review and platform secret scanning still needed |
| Dependencies | Current Go module removes historical `tls-client` / `fhttp` | Confirmed for **new source**, not legacy downloadable binaries |
| Go/Node/extension CI | Current 3-OS CI workflows have successful runs; native build/test coverage exists | Confirmed; clean-machine installer + browser acceptance incomplete |
| Authenticated platform adapters | 11 formatter targets; external posting uses platform-dependent API/session/Bridge paths | Authorization and explicit target behavior need per-platform assessment |
| Third-party bundled scripts | Extension bundles `log-editor-vendor.js` and `i18next-vendor.js` | Complete provenance and embedded-license notices not yet verified |
| npm dependencies | `package-lock.json` includes Sharp and optional platform libvips packages | Full licenses/redistribution conditions not yet documented |
| Historical BlogCTL Releases | At the review snapshot the former website had 23 BlogCTL Releases and 1 Inkerio Release | Historical download packages included unresolved `fhttp` licensing evidence; the website Release attachments have now been removed after archiving the original GitHub metrics (Git tags retained) |

## Before making this *existing* private repository public

1. Inspect **all Git history**, current source, archived migration fixtures, committed extension vendor bundles, GitHub Actions runs and downloadable artifacts for personal secrets/private content. Automated pattern search alone does not prove the absence of secrets.
2. Document authorship and complete license notices for bundled third-party JavaScript and any embedded binary assets. Review third-party source reuse and `NOTICE`/attribution requirements. An MIT root license cannot relicense bundled upstream work.
3. Review each publishing adapter's API and browser-session/undocumented-endpoint behavior. Ensure opt-in, clear user intent, scoped permissions, non-bypass defaults and reasonable user-facing disclaimers. Avoid implying that all 11 platform integrations are officially supported.
4. Review extension permissions (`cookies`, `webRequest`, host permissions), Native Host pairing, authenticated loopback transport and installed-user state. Verify the published code cannot expose other users' sessions by default.
5. Update documentation links, repo descriptions and branch protection; GitHub makes **Git history and Actions logs publicly visible** when visibility changes. Decide whether a fresh sanitized public repository is necessary if the history cannot be responsibly published.

**Current outcome:** not ready to flip visibility on this existing repository. Source publication may be considered independently of downloadable binaries, but it still requires the first five checks.

## Before a binary or Extension Release

- Collect reviewed license texts for all pinned direct/indirect Go packages, npm dependencies and optional OS/architecture packages. Check the actual vendored/bundled JS and libvips obligations; see [dependency audit](RELEASE_LICENSE_AUDIT.md).
- Build *and install* real Windows, macOS, and Linux distributions on clean machines, including Node renderer setup, code signing expectations, default data paths and Native Messaging registration.
- Test the Extension's ID, native messaging host and local Bridge (`127.0.0.1:32146`) without altering existing BlogCTL installation state.
- Verify direct compilation has no network publishing; authenticated create/update/publish behavior must be explicitly exercised only for authorized test accounts. Use safe readback and recovery tests.
- Produce a full release manifest, integrity hashes, license notices, upgrade and rollback instructions. The package command intentionally fails closed when the reviewed license bundle is missing.

## Current repository ownership

- [Inkerio](https://github.com/ThinkerQAQ/inkerio): public Astro engine, static site build and source releases.
- [inkerio-content-template](https://github.com/ThinkerQAQ/inkerio-content-template): public content examples.
- [ThinkerQAQ.github.io](https://github.com/ThinkerQAQ/ThinkerQAQ.github.io): public deployment wrapper, public site config and site-specific Workers, **not an app download repo**.
- SyndiRelay: distribution compilers, adapters, Bridge, local UI, MCP, optional search/AI Search CLI used in the site workflow. Its optional CI integration **does not need a binary release**.
- Actual authors' content repositories and local `Data/`: owned separately; never redistribute with software.

A change in repository visibility should be treated as a separate, security-relevant release and requires a documented review outcome. Do not change visibility merely to enable GitHub branch-protection features.
