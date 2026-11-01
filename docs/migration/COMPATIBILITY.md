# SyndiRelay extraction and compatibility gate

- Source: `ThinkerQAQ/ThinkerQAQ.github.io` commit `d07c72f5d1a1221c5fbb46548bc41a73072519b2`.
- Initial snapshot: 356 tracked files copied from `tools/blogctl/`, all source SHA-256 hashes matched (`SOURCE_SHA256.txt`). The original snapshot is preserved as a separate Git commit before any refactoring.
- Current independent Go import path: `github.com/ThinkerQAQ/syndirelay`.
- Same original fixture compiled on all 11 platforms: `51cto`, `cnblogs`, `csdn`, `devto`, `hashnode`, `juejin`, `medium`, `oschina`, `segmentfault`, `toutiao`, `zhihu`.
- For each platform the complete JSON representation (Markdown, HTML, content hash, canonical URL, fallback Medium payload, asset IDs) matched **byte-for-byte** between old BlogCTL and the standalone migration.
- Original Go, extension and image sanitization tests pass. A new `compile --file` entrypoint accepts ordinary Markdown independently, without any Inkerio or blog-content installation. It uses the same platform-specific compiler logic while defaulting to no ThinkerQAQ canonical URL, copyright footer or CDN.
- Locally built Windows/macOS/Linux on AMD64 and ARM64; building a binary is not equivalent to testing Node dependencies, browser sessions and remote platforms on those operating systems.

## Compatibility boundary

The ThinkerQAQ production wrapper now pins the standalone Inkerio engine for static builds and separately pins SyndiRelay for optional search/AI Search CLI. The SyndiRelay `site` and compatibility commands are **not** part of the recommended standalone authoring flow; retire unused handlers only after explicit caller/dependency auditing. The site has been verified on GitHub Pages and EdgeOne. The normal new CLI help exposes the distribution commands.

## Public release blockers

1. Cross-platform CI and **real installations** (Go executable + Node renderer + extension) on Windows/macOS/Linux.
2. Copyright/license collector for all six target dependency graphs and npm runtime; attach full third-party license texts to each download.
3. Per-platform policy/authorization audit, particularly nonpublic APIs and browser-session workflows. No protocol spoofing or unauthorised automation.
4. Chrome Extension identity, Native Host version and existing local Data directory migration: no overwriting the old BlogCTL installation or user data.
5. Remove site-only legacy commands after the production wrapper has been decoupled; validate remote publishing in dry-run mode with explicit user permissions.
6. The **source repository is now Public** following the [source audit](../compliance/PUBLICATION_READINESS.md). Protected main, PR-only Squash Merge and the three-OS CI are enabled; binary/Extension Release and live platform acceptance remain separate gates.
