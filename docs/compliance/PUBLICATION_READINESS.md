# SyndiRelay publication readiness and repository split

Status checked: **2026-11-01**. Source publication and the separately gated binary/Extension releases are distinct decisions. The user has approved publishing platform adapter source while keeping remote operations explicitly authorized and unverified adapters documented as experimental.

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

**Current source decision (2026-11-01):** Publish source including experimental adapters after credential-history and bundled-license audit. This does not verify third-party API permission or authorize binary/Extension redistribution; keep those gates open. Switching a repository back to private cannot retract source already cloned.

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


## 2026-11-01 source visibility audit evidence

- All currently reachable 36 Git commits and 633 Git blobs were checked for high-confidence credential patterns (GitHub/OpenAI/AWS keys, private key blocks, credential-bearing URLs). One matched URL was a clearly synthetic compiler test case, not a live credential.
- All 35 available Actions run ZIP archives were obtained and scanned (3,829,606 decoded text bytes); none matched those credential patterns. GitHub reported zero retained Actions artifacts at review time. This is not a proof that every possible secret pattern is absent.
- The log editor vendor bundle is generated from the checked-in CodeMirror source build, with seven runtime MIT notices copied from exact pinned npm packages under extension/licenses/log-editor; see extension/THIRD_PARTY_NOTICES.md. The checked-in i18next notice remains.
- No local Data directory or private TOML, Cookie, token, or database file was staged for the public source commit. Authenticated adapters still require account permission, platform-specific review and explicit operator consent; not all compiler targets have verified write paths.
- Binary/Extension Release, complete third-party distribution manifest, genuine clean-machine operating-system tests, and live publishing acceptance remain separate, unresolved gates.
