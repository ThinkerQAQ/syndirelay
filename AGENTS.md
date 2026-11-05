# SyndiRelay migration rules

- Prefer small PRs and Squash Merge, keep main protected.
- Preserve every existing platform compiler, renderer, publisher and test before changing behavior.
- Compare deterministic compiled outputs with the original BlogCTL baseline before replacing the original runtime.
- Never upload users' article content, authentication cookies, local publishing databases, credentials, or tokens.
- Keep any unofficial/private platform integration private until policy and authorization review is complete.
- Cross-platform CI: Linux, macOS and Windows.

## AI development workflow (Web ChatGPT + Remote Desktop MCP)

**At the start of any coding task:** confirm the absolute WSL workspace path, Git branch, `git status --short --branch`, and `git worktree list`. Read `/home/zsk/code/AGENTS.md` when available, then read this file. The shared parent file is not automatically loaded by the web ChatGPT client; the agent must explicitly read it via the existing Remote Desktop MCP. These rules apply to the **selected worktree**, not to other worktrees of the same repository.

- **Development environment:** treat WSL `/home/zsk/code` as the source of truth, and use the existing Remote Desktop MCP to read/edit files and run commands. Use this repository's Devbox toolchain (`devbox.json` / `devbox.lock`), not a global Go or Node version.
- **Adaptive reading sequence:** `rg` / file search to locate candidates → CodeGraph for cross-file relationships and impact → language server (Go: `gopls`; JavaScript: corresponding LSP) when exact symbols, implementations, or diagnostics are needed → read relevant source and tests → edit → build/test. Do not invoke every layer for a trivial change.
- **CodeGraph:** use its existing WSL executable through Remote Desktop MCP, on demand. Pass `--workspace <absolute-worktree-path>` on every query, and verify returned locations belong to the selected worktree. Prefer one-shot `--run-tool` with `--graph-only` when semantic embeddings are unnecessary. Never assume global graph indexes isolate branches automatically.
- **LSP:** share the executable and compatible caches, not mutable workspace state. Use an explicit worktree and the correct Devbox toolchain; prefer CLI queries and on-demand `gopls -remote=auto` with an idle timeout rather than one permanent server per worktree. Check compatibility before sharing across Go versions. Fall back to `go list`, build, tests, and source reading if LSP is unavailable.
- **Workspace safety:** do not reuse CodeGraph/LSP conclusions across branches without checking file paths and the current revision. Preserve existing user changes and local data; do not switch, reset, clean, or overwrite the user's active worktree to make a documentation or code change.
- **Resource boundary:** do not modify Remote Desktop MCP or introduce a second public Tunnel, separate CodeGraph MCP, or persistent indexing service for this workflow.
- **Verification:** use `devbox run test`, `devbox run build`, and, when packaging is relevant, `devbox run verify-package`. Report test failures, skipped steps, and platform requirements instead of claiming success without execution.
- **Delivery:** isolate unrelated changes in small PRs, Squash Merge only with authorization, and preserve branch protection, release process, platform compatibility, and the privacy rules above.
