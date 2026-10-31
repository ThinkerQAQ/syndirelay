# Bridge, browser identity and durable state

**Definition:** the Go Bridge owns local runtime operations, while the browser Extension owns the authenticated browser context required by supported platforms.

```text
Extension / shared Web Console
      ↓ constrained transport
Native Messaging / browser relay
      ↓
Local Go Bridge (jobs, config, publishers)
      ↓
remote platforms, search tools, local data
```

The Web Console and Extension use the same UI implementation; the Console itself must not receive browser cookies or a privileged Bridge token. See [current workflow guide](../guide/workflows.md).

## Runtime data

On Windows, the default persistent location is `Data/` next to the executable. Linux/macOS use the user configuration directory. All platforms can select an absolute `BLOGCTL_DATA_DIR`. Configuration is `blogctl.toml`; jobs, logs and publication state are separate runtime files. Exact path rules come from [bridge/config.go](../../bridge/config.go), summarized in [Configuration Reference](../reference/configuration.md).

## Example and boundaries

A detection request reads authenticated remote metadata through the Bridge. A Create Draft task stores a durable outcome and remote ID; the session cookie stays ephemeral. The Web Console cannot replace the Extension's browser authentication path.

Never copy `Data/` into source control; it may contain credentials and private platform metadata.
