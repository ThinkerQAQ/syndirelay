# Read-only dry-run guarantees

`syndirelay sync --article <slug> --platforms <csv> --dry-run` must not issue HTTP platform requests, mutate source content or publishing state, write local configuration, or invoke npm installation/upload/publish commands. It compiles locally and returns per-platform completion events.

- The CLI leaves `SYNDIRELAY_DATA_DIR/blogctl.toml` untouched on `--dry-run`.
- `SyncService.Run` requires its Node renderer dependencies already installed for `--dry-run`. If missing, the command fails closed with an explicit `npm ci` setup suggestion **without running** `npm ci`.
- `SyncService.Run` may use the Node Markdown-to-HTML renderer only, which reads stdin and emits HTML on stdout. The `NativePublisher` and `assets.Prepare` are inaccessible in the `DryRun` branch.
- No remote credentials or browser sessions are necessary for a preview.

## Private regression verification (2026-10-31)

Used the owner's content repository article `what-kind-of-life-do-i-want` in the local Windows installation. All 11 configured platforms compiled with `--dry-run` (exit 0). The actual Windows runtime output reported 11 successful `sync-compile` completions. SHA-256 hashes of the three files below were identical immediately before and after the command:

- `Data/blogctl.toml`
- `Data/jobs.json`
- `Data/publications.json`

No article text, configuration secrets, cookies or publication history are committed to GitHub. The private test output stays under `C:\software\Coding\SyndiRelay\Data\reports\what-kind-of-life-do-i-want-20261031\`.

This does **not** constitute verification of real remote create, update or publish operations. Those require separate authorization and acceptance. Dry-run does not invoke them.
