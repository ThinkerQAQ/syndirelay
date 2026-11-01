# Windows side-by-side installation (private migration)

Install SyndiRelay in `C:\Apps\SyndiRelay`, alongside the existing BlogCTL directory. **Never overwrite** the original installation or data. Use an independent browser extension ID, Native Messaging Host (`com.thinkerqaq.syndirelay`), local port `32146`, and data directory.

The Windows runtime directory contains `syndirelay-windows-amd64.exe`, `extension/`, `renderers/`, `scripts/plantuml/`, `package.json`, `package-lock.json`, `node_modules/`, and `Data/`. `src/` is the optional local clone of the private repository for updates. Use `npm ci` within the runtime directory. The CLI and MCP do not require Inkerio.

Configuration migration: preserve a **private local copy** of old `Data/` (jobs, publication bindings, social records, article artifacts and user settings) while excluding stale `bridge.json`, `bridge.log`, and `com.thinkerqaq.blogctl.json`. Update only the copied TOML's `engine_root`, `distribution_root`, `publication_bindings_path`, and `log_directory` to paths under SyndiRelay. Leave the old BlogCTL files unchanged. This is a snapshot, not shared live state: future writes are independent.

The source extension has its own new public manifest key and ID in `extension/extension-id.txt`; a separate signing private key must remain **local** in the installation `Data/` and must never be committed. Register the new Native Host using `syndirelay-windows-amd64.exe install`; verify that the old `com.thinkerqaq.blogctl` registry keys remain unchanged. For an unpacked extension, open Edge's extension manager and load `C:\Apps\SyndiRelay\extension` as a separate extension. Do not disable or overwrite the old extension.

MCP: `syndirelay-windows-amd64.exe mcp-stdio` uses the new Bridge and private state; do not duplicate the existing BlogCTL connection until the new runtime is verified. Existing Cookie-based private-platform publishers require separate authorization review before wider distribution.

GitHub Actions now tests source builds on Windows, Linux and macOS. An actual installed Extension, Native Host, browser-authenticated publishing and distribution license manifest must still be verified independently before a binary Release.
