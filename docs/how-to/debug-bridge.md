# Diagnose a disconnected Console

The executable, browser Extension and Console form one connection. Opening the Console HTML alone does not prove that Native Messaging or the authenticated browser relay works.

## 1. Check the running version and host

Confirm the executable's location and version. Keep it at a stable path; moving it after installation leaves the Native Messaging registration pointing at the old path.

Run the intended executable's `install` command again after moving it. Match the installed Extension version to the executable, reload the Extension, close old Console tabs and open the Console from the Extension.

## 2. Check local health

The default endpoint is local to the machine running the Bridge:

```powershell
Invoke-RestMethod http://127.0.0.1:32145/v1/health
```

On Linux or macOS:

```bash
curl --fail http://127.0.0.1:32145/v1/health
```

If the connection is refused, open the Extension to trigger Native Messaging, then inspect registration, executable permissions/path and process startup errors. A reachable endpoint may belong to an older process; compare its version with the package you installed.

## 3. Check the Extension

In `edge://extensions` or `chrome://extensions`, confirm BlogCTL is enabled and its unpacked directory still exists. Inspect the Extension's errors or service-worker console for native-host launch failures. Reload after replacing its files.

Health responding while Console operations fail points to the Extension/relay or a stale browser tab. Reopen the Console from the active Extension and check the connection status before submitting a task.

## 4. Check workspace and account state

A connected Console can still have inaccessible paths or an expired platform login. Check Settings and the [workspace guide](configure-workspace.md), then inspect the failed task's platform error. Windows paths must be accessible to the Windows executable; a WSL checkout path is not automatically a Windows path.

Use an explicit read-only detection/list action to check account access. It is not a required step before every Create operation. If the platform presents a challenge, complete it in the real browser and retry the read.

## 5. Preserve diagnostic state

Use Logs and Tasks to identify the failing layer. Record the version, action, timestamp and redacted error. Never export cookies, tokens, Native Messaging authentication data or an unredacted configuration file.

Keep `Data/` while upgrading. Deleting it loses configuration and publication history and is not a connection repair step.

See [Bridge model](../concepts/bridge-and-state.md) and [configuration](../reference/configuration.md).
