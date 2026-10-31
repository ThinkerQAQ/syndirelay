# Compile one article without publishing

The public template contains `sample-article` and its English translation. From a content checkout beside the engine:

```bash
blogctl sync --article sample-article --platforms devto,medium --dry-run
```

Use a built/installed `blogctl` on PATH. In a different directory layout, set `BLOG_CONTENT_ROOT` and `BLOGCTL_ENGINE_ROOT` to the absolute checkout roots first.

The command validates selected articles and platform payloads locally. It may install Node dependencies and update local workspace configuration; it does not call remote article Create/Update/Publish or upload publishing assets. It does not require a browser session or running Bridge. A successful exit is a compilation check, not a saved HTML preview or evidence of platform acceptance.

Language comes from `publishing.platforms.PLATFORM.language`. Selecting English requires a corresponding English source. Choose the root slug rather than guessing a translated file path. For remote writes, use the [explicit UI workflow](../tutorial/first-publish.md).
