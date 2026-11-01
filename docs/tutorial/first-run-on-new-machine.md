# Install SyndiRelay from source on a new machine

SyndiRelay is independent of Inkerio. **The source repository is public; binary and Extension installers remain separately gated.** Begin with a read-only local compile; connect a browser only if you intend to use an authorized publishing workflow.

## 1. Prerequisites and source

Install Git, Go 1.24+, Node.js 22+ and npm. On the operating system that will run the app:

```sh
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
go test ./...
go build -o syndirelay ./cmd
printf '# My original article\n\nHello from SyndiRelay.\n' > example.md
./syndirelay compile --file ./example.md --platform devto
```

On Windows PowerShell, build `go build -o syndirelay.exe ./cmd`, create `example.md` with an editor and invoke `.\syndirelay.exe compile --file .\example.md --platform devto`. Source compilation is local and never posts to a platform.

`go test ./...` tests Go code and is not an installation or publishing acceptance test. For frontend tests run `npm run test:renderers && npm run test:extension`.

## 2. Optional local browser integration

Use a persistent installation directory, **separate from any older BlogCTL installation**, with your binary, `extension/`, `renderers/`, `package.json`, `package-lock.json`, `node_modules/` and private `Data/`. Set `SYNDIRELAY_HOME` to the installation root and, if needed, absolute `SYNDIRELAY_DATA_DIR` to your private data root. Never copy historical user `Data/` into Git.

Run `./syndirelay install` (or `.\syndirelay.exe install`) from the operating system that owns Edge/Chrome, then load **this repo's `extension/`** as an unpacked extension. The Native Messaging Host is `com.thinkerqaq.syndirelay`; it is a stable product identifier, **not a user website domain**. The local Bridge is `127.0.0.1:32146`.

Do **not** use the old `tools/blogctl/extension/`, previous binary names, port `32145`, or the retired website repository installation instructions. Separate extension identities prevent writing to the old BlogCTL user profile.

## 3. Optional content and remote publishing

For repository-aware content, select the folder containing `src/content/articles/` and explicitly set your own `[publishing.site]` canonical origin/copyright and the intended distribution paths. See [configuration](../how-to/configure-workspace.md). The generic single-file `compile` command needs neither a content repository nor an Inkerio checkout.

Only after signing into a supported platform **and verifying authorization** should you use its remote draft/update/publish workflow. Check actual supported operations, remote IDs and task history before retrying a write. Different platforms support different workflows. See [first draft](first-publish.md).

## 4. Optional MCP

Run `./syndirelay mcp-stdio` locally and connect the limited STDIO tool interface. Never tunnel the entire Bridge. Keep the browser process/Extension on the same OS as the Bridge. See [MCP details](../MCP.md).

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Clone denied | Verify the public GitHub repository URL and Git/network connectivity |
| Compile fails to render a diagram | Install renderer dependencies; provide explicit asset URL when needed |
| Native Host missing | Correct OS binary, install path and `syndirelay install` |
| Extension fails to connect | Same native host/extension version, local port 32146, browser allowed |
| No articles in Console | Explicit content root and suitable article status/language |
| Task timed out after a remote create | Inspect local task and remote ID before any retry |

Do not send Cookies, API keys or local `Data/` as troubleshooting attachments.
