# Install BlogCTL and connect the browser

BlogCTL has a local Go executable, an Edge/Chrome Extension and a Web Console. You need the Extension for browser-backed account operations. Building a website also requires an Inkerio checkout and Node/npm.

## 1. Download matching files

From [Releases](https://github.com/ThinkerQAQ/ThinkerQAQ.github.io/releases/latest), download the binary matching your OS/CPU and the Extension ZIP from the **same BlogCTL release**. The six binaries cover Windows, Linux and macOS on amd64/arm64. Extract the Extension ZIP into a stable directory.

A release executable does not need Go installed. On Windows, keep the executable in its permanent location, for example `C:\software\Coding\blogctl\`. Its adjacent `Data/` directory stores configuration and tasks; preserve it across upgrades.

To build a local executable instead, run from the engine root:

```bash
go run ./tools/blogctl/cmd/maint build
.build/out/blogctl help
```

The example is for Bash. For a named Windows executable, build with `go build -o blogctl-windows-amd64.exe ./tools/blogctl/cmd`. Use `tools/blogctl/extension/` from that checkout for the matching Extension.

## 2. Register Native Messaging

Windows PowerShell:

```powershell
& 'C:\software\Coding\blogctl\blogctl-windows-amd64.exe' install
```

Linux example:

```bash
chmod +x ./blogctl-linux-amd64
./blogctl-linux-amd64 install
```

On macOS use the matching `blogctl-darwin-*` binary. `install` registers the executable as the browser's Native Messaging host; it does not itself load the Extension. Re-run it after moving the executable. `uninstall` removes host registration while preserving user data.

## 3. Load and open the Extension

1. Open `edge://extensions` or `chrome://extensions` in the intended browser profile.
2. Enable **Developer mode**, choose **Load unpacked**, and select the extracted folder containing `manifest.json`.
3. Open BlogCTL. The Extension starts/connects to the Bridge on demand.
4. Confirm its connected state and choose **Open workspace**.

The Console opens at `http://127.0.0.1:32145/console/`. Keep the Extension enabled. Merely opening the URL does not establish its browser relay.

## 4. Set the workspace

In **Settings**, set **Engine Root** to the Inkerio checkout and **Content Root** to your content repository, using paths visible to the executable's OS. A Windows executable needs Windows paths; a Linux executable in WSL needs Linux paths. Save and reopen Settings to confirm persistence.

Run `blogctl doctor` from the engine directory to inspect Node/npm/Git availability. Unlike `help` and `install`, `doctor` currently requires an engine working directory.

## 5. Verify and upgrade

PowerShell:

```powershell
Invoke-RestMethod http://127.0.0.1:32145/v1/health
```

Bash:

```bash
curl http://127.0.0.1:32145/v1/health
```

A successful health response confirms HTTP is running; also check the Extension connection indicator. No platform account is needed for this check.

For upgrades, preserve `Data/`, replace the binary and Extension together, reload the Extension in the browser and reopen the Console. If the binary moved, register it again. Avoid leaving an older Bridge running while diagnosing a version mismatch.

Continue with [first draft](tutorial/first-publish.md), [new-machine walkthrough](tutorial/first-run-on-new-machine.md) or [connection troubleshooting](how-to/debug-bridge.md).

## Optional: connect ChatGPT with MCP

After installing the **matching** BlogCTL binary and Edge/Chrome Extension, launch `blogctl -mcp-stdio` through a dedicated Secure MCP Tunnel. The MCP process calls the same local Bridge and uses the Extension's existing browser session; it does not require a second publishing backend. A tunnel alone cannot access Edge unless the MCP executable runs on the same Windows side as the Extension. See [MCP integration](MCP.md) for full instructions, tool names, and privacy notes.

Image uploads require `engine_root`, Node.js + Inkerio's installed `sharp` dependency, and a configured `image_upload_roots` whitelist. Start with `dry_run=true` before uploading. See [MCP image uploads](MCP.md#private-image-upload-to-r2).
