# SyndiRelay migration rules

- Prefer small PRs and Squash Merge, keep main protected.
- Preserve every existing platform compiler, renderer, publisher and test before changing behavior.
- Compare deterministic compiled outputs with the original BlogCTL baseline before replacing the original runtime.
- Never upload users' article content, authentication cookies, local publishing databases, credentials, or tokens.
- Keep any unofficial/private platform integration private until policy and authorization review is complete.
- Cross-platform CI: Linux, macOS and Windows.
