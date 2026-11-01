# SyndiRelay architecture

The former BlogCTL tool has been extracted into an **independent publisher**, separate from Inkerio's Astro engine and from any user's private writing.

```text
Markdown file or content adapter
  -> compiler/ and renderers/ (local platform-specific preview)
  -> publisher/ + publishing/ + platform/ (explicit draft/update/publish operations)
  -> bridge/ (authenticated loopback task service)
     -> extension/ (authorized browser sessions, optional)
     -> cmd/mcp.go (STDIO MCP, optional)
  -> optional Reverfolio durable outbox (metadata-only impact tracking)
```

The Bridge listens only on local `127.0.0.1:32146`; never expose its full endpoint over a public tunnel. `Data/` remains local, with separate extension identity and native host from legacy BlogCTL. Search/AI Search code still supports the site deployment CI, but **SyndiRelay does not assemble or deploy Inkerio websites in the normal authoring flow**.

See [content and publishing](../concepts/content-and-publishing.md), [Bridge and state](../concepts/bridge-and-state.md), [operation safety](../concepts/operations.md), and [public-source review](../compliance/PUBLICATION_READINESS.md).
