# Content and publishing

**Definition:** BlogCTL compiles canonical Markdown into platform-specific payloads while preserving the original content repository as the sole source of truth.

## Problem and model

Different article platforms accept different HTML/Markdown, asset uploads, draft IDs and canonical URLs. BlogCTL separates:

```text
blog-content Markdown
   ↓ compiler
CompiledArticle / content policy
   ↓ asset pipeline
prepared images / diagrams
   ↓ platform adapter
remote draft (explicit action)
   ↓ publish adapter (only if supported and requested)
public article
```

The compiler and asset pipeline are reusable; adapters own platform-specific request constraints. [Source implementation](../../compiler/compile.go) and [publishing contract](../../publishing/config.go) define the boundaries.

## Article frontmatter

Canonical Markdown frontmatter is YAML, parsed with `go.yaml.in/yaml/v3` into
the publishing fields. Tags are a YAML sequence: both `tags: [Java, Go, Message Passing]`
and a multiline list under `tags:` are supported, including quoted values and
YAML comments. Titles and descriptions also support YAML quoting and block scalars.
Site-only metadata such as dates, series and translation references remains in
the content repository and is not part of the publishing payload.

Malformed YAML, duplicate keys and invalid publishing field types stop compilation
before asset preparation or remote writes. Errors include the source file and YAML
diagnostics; structured logs identify the `parse-frontmatter` operation and duration.
Published articles still require a title, description, body, cover URL and cover alt text.

## Example

For one article and DEV.to, a `--dry-run` creates a local preview without third-party changes. A live Create Draft requires an account session and explicit authorization. A later Update must select an actual remote target rather than guessing from the title.

## Trade-off

This design adds explicit steps in exchange for preventing duplicate articles and accidental publication. For exact operations see [Workflows](../guide/workflows.md) and [CLI](../reference/cli.md).
