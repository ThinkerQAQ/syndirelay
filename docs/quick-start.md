# SyndiRelay quick start

SyndiRelay is an independent Markdown compiler and content syndication toolkit. No Inkerio or Astro engine is required. Its open-source MIT license does not grant rights over users' content.

**Migration status:** The standalone source is in the private [SyndiRelay repository](https://github.com/ThinkerQAQ/syndirelay). Public release packages are blocked until third-party license and publishing-platform authorization review passes. Do not download the older BlogCTL release expecting a SyndiRelay installer.

## Install from source

Prerequisites: Go 1.24+ and Node.js 22+.

```sh
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
go build -o syndirelay ./cmd
printf '# My article\n\nMy original text.\n' > hello.md
./syndirelay compile --file ./hello.md --platform devto
```

A private repository checkout requires authorized access. The command only compiles an article to JSON locally; it does **not** create a draft, upload an image, or publish a post. For generated raster assets or diagrams, configure an HTTPS `--asset-base-url` explicitly.

## Connect a blog-content folder

Set `[publishing.site].origin` and the content workspace path under `content_root`, then configure your own copyright. See [site identity, copyright and precedence](reference/copyright-and-identity.md) and [configuration reference](reference/configuration.md). Without a source-site origin, content-repository publishing fails closed instead of inventing the software author's address.

## Browser extension and MCP

Source-based installs may register the browser Native Messaging Host with `./syndirelay install` after building a binary on the target operating system, then load `extension/` as an unpacked Chrome/Edge extension. Local Bridge and Extension use the same authenticated session; a browser extension is unnecessary for `compile --file`. MCP stdio is `./syndirelay mcp-stdio` and requires the Bridge for authenticated publishing tools. For Windows side-by-side migration from BlogCTL, follow [these instructions](how-to/windows-side-by-side.md) and preserve the original `Data/` directory.

Windows/macOS/Linux installation and all external platform write operations must be validated independently. Running `--dry-run` compiles locally without remote posting or making source/config changes.
