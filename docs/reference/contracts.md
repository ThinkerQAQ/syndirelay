# Runtime and publishing contracts

Code, not prose, defines the actual contract. This index identifies the owning modules.

| Contract | Code authority | Safety property |
| --- | --- | --- |
| CLI and sync option validation | [cmd/main.go](../../cmd/main.go), [cmd/sync.go](../../cmd/sync.go) | Explicit article/platform selection; `--all` and `--article` exclusive |
| Workspace/Bridge configuration | [bridge/config.go](../../bridge/config.go) | One TOML, explicit absolute data override |
| Publishing options | [publishing/config.go](../../publishing/config.go) | Platform language, footer, canonical, tracking and assets |
| Supported platforms and capability catalog | [platform/capabilities.go](../../platform/capabilities.go), [publisher/capabilities.go](../../publisher/capabilities.go) | Reject unsupported operations rather than guessing |
| Local Bridge & authenticated request processing | [bridge/server.go](../../bridge/server.go), [bridge/control.go](../../bridge/control.go) | Browser-owned identity, limited token access |
| Durable jobs, task events and retries | [bridge/jobs.go](../../bridge/jobs.go), [bridge/explicit_sync.go](../../bridge/explicit_sync.go) | Remote identity and side effect tracked across operation |
| Source compilation | [compiler/compile.go](../../compiler/compile.go) | Canonical content transformed into explicit platform payload |
| Site assembly | [cmd/site.go](../../cmd/site.go) | Builds from selected external content root |

See [operation concepts](../concepts/operations.md) and [workflow guide](../guide/workflows.md). Reference pages must change with new fields, flags, and supported capability declarations.
