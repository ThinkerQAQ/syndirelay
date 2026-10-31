# Configure the workspace

Open **Settings → Content and paths** in the connected Extension/Console.

| Field | Select |
| --- | --- |
| Engine Root | Root containing `package.json`, `astro.config.mjs` and `tools/blogctl/` |
| Content Root | Root containing `src/content/articles/` |

Select repository roots, not `src/content/` or the executable directory. Save, leave Settings, then reopen and confirm the paths. Check local articles on Create/Update; Detect lists remote account content.

## Windows and WSL

Windows Bridge paths use Windows syntax, such as `C:\code\blog-content`. A Linux Bridge in WSL uses Linux paths. A Windows browser requires a Windows Native Messaging executable even if source files are edited in WSL.

CLI `sync` uses its current directory or `BLOG_CONTENT_ROOT` and `BLOGCTL_ENGINE_ROOT` to discover repositories. Saved UI paths do not override those discovery rules. See [CLI](../reference/cli.md).

## Move or share Data

Set absolute `BLOGCTL_DATA_DIR` before starting the process. Stop the previous Bridge, back up/copy existing Data, start at the new location and inspect Settings and Tasks. The override does not copy files or translate workspace paths.

For WSL social commands sharing a Windows installation, use its `/mnt/c/.../Data` directory and pass explicit Linux `--content-root` where saved paths are Windows paths. Avoid concurrent runtimes overwriting incompatible workspace paths in the same configuration.

See [Configuration](../reference/configuration.md) for directory defaults and TOML keys.
