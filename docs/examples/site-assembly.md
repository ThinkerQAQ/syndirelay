# Example — assemble a separate content repository

**Goal:** build a working blog site from the public content template.

**Inputs:** sibling `ThinkerQAQ.github.io/` and `blog-content/`, cloned as in [engine tutorial](../../../../docs/tutorial/first-site.md).

From the **engine** directory:

```bash
go run ./tools/blogctl/cmd site assemble --content-root ../blog-content
npm run dev:site
```

**What this uses:** Go BlogCTL Site Assembly, canonical Markdown and Astro collections. The key code is [cmd/site.go](../../cmd/site.go). The engine can now render content without storing the authoritative sources in its repository.
