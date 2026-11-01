# Developer-branding removal audit (2026-11-01)

SyndiRelay is a reusable publishing tool, not an implicit claim over a user’s text or images.

## Removed from new-install runtime behavior

- No embedded ThinkerQAQ copyright/copyright.json: no author-specific legal attribution is ever inserted without user-owned policy configuration.
- No developer blog origin, localized site title, or default asset/CDN URL. Blog-content publishing now requires an explicitly configured canonical origin. `compile --file` remains standalone without a site or license footer.
- `auto` copyright precedence: configured locale text → content-owned `src/content/site/copyright.json` → no copyright statement. `config`, `content` and `none` enforce their explicit behavior. Existing malformed JSON fails closed.
- No hardcoded developer canonical host in the Medium Markdown compiler, image/cover URL resolver or DEV.to candidate matching. Old hosts may be allowed only through explicit `publishing.site.legacy_origins`.
- DEV.to outbound User-Agent now advertises the tool, not a personal blog name. The log viewer npm package is branded for SyndiRelay.
- Social source publishing requires explicit canonical origin. Legacy search/AI Search compatibility modules use the reserved non-routable placeholder `example.invalid` instead of the developer’s production site until configured.
- Extension Settings supports per-locale copyright text and site origin/name. Source-link Footer and legal notice remain independently controllable. English/Chinese labels are both provided.
- Default assets store is `none`. R2 requires explicitly configured bucket, public base URL and credentials before asset uploads.

## Intentionally retained identifiers (not user-facing defaults)

- `github.com/ThinkerQAQ/syndirelay`: the real Go module import path and repository owner. Changing it would break module consumers and checksums, not configure a user's publishing identity.
- `LICENSE Copyright (c) 2026 ThinkerQAQ`: required accurate historical authorship for the **software source code** under MIT, not a license claim on user content.
- `x-thinkerqaq-token`, `com.thinkerqaq.syndirelay` and other `blogctl.*` private bridge/extension message identifiers: existing authenticated wire contracts/registered Native Host IDs. Changing these requires coordinated client+server+installed extension migration. **They are not used as site domain or content copyright.**
- `docs/migration/legacy`, source provenance and fixtures mentioning the author: migration/audit evidence and historical compatibility samples, not software defaults.

## Regression evidence

1. Existing BlogCTL fixture on **all 11 platform compilers** yielded JSON **byte-for-byte identical** to the original BlogCTL baseline when *all original site/copyright/R2 values were supplied explicitly*.
2. A different author and site at `https://independent.example.org` compiled successfully for all 11 platforms with alternate localized names, copyright and paths; no original author branding appeared in the output.
3. A missing content copyright file produces **no legal notice**, rather than using an obsolete restrictive statement. Invalid existing policies fail closed.
4. The user’s real `what-kind-of-life-do-i-want` article compiled into all 11 platform previews with the *current* CC BY-NC-SA 4.0 content policy, without the old “full republication/adaptation requires prior permission” line. The private article output is stored **only on the authorized Windows device under `Data/reports/`**, not in Git.
5. Automated Go/Node/extension tests pass. GitHub Actions now has successful Windows, macOS and Linux validation runs; historical runner-billing failures are resolved. CI build success does not establish third-party API authorization, full package redistribution rights or real-browser acceptance. See [publication review](../compliance/PUBLICATION_READINESS.md).

## Safety when updating an existing installation

Back up local TOML and runtime executable, explicitly configure your own site/copyright/R2 policy, then restart SyndiRelay with matching Extension assets. Keep the existing extension ID, local authentication contract and private user Data. No remote drafts or published posts are altered by a configuration change.
