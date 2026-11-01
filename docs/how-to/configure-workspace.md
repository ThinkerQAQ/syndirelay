# Configure a SyndiRelay workspace

SyndiRelay can compile a single Markdown file **without any blog workspace**. Repository-aware article selection and the optional browser Console need only an explicitly selected content root; Inkerio is not a required runtime dependency.

## Local paths

| Setting | Meaning |
| --- | --- |
| `SYNDIRELAY_HOME` | Absolute root of the local executable, Node renderer dependencies, Extension and optional diagram scripts |
| `SYNDIRELAY_DATA_DIR` | Absolute, local-only private directory for jobs, logs, configuration and publication bindings |
| Content Root | Directory containing `src/content/articles/` (for repository-aware publishing) |
| Site origin / canonical URL | Your own HTTPS domain; must be explicitly configured for content-repository publishing |
| Engine Root (older UI) | **Legacy compatibility only**; optional if you also maintain an Inkerio website |

In the Extension/Console open **Settings → Content and paths**, choose the real content repository folder, save, reopen Settings, and confirm the paths. Use **Create/Update** for local content selection, and **Detect** for an authorized remote account inventory. Detection is not required for standalone `compile --file`.

Do not select `src/content/` itself when the UI expects a repository root. On Windows, run a Windows Native Host to access a Windows Edge/Chrome profile. WSL and native Windows are different bridge processes and do not share browser cookies automatically.

For existing BlogCTL installations, copy the old `Data/` to a separate private backup and follow [Windows side-by-side](windows-side-by-side.md). Do not point two independent running products at a writable shared database. The local Bridge uses port **32146**; the former BlogCTL port was 32145.

Configure your own copyright policy and canonical origin as explained in [identity and copyright](../reference/copyright-and-identity.md), and inspect [configuration](../reference/configuration.md) for persisted TOML fields.
