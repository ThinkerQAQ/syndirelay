# Create, update and publish articles

The Extension side panel and Web Console use the same controls. Before writing to a platform, set the engine/content paths in **Settings**, select the intended article language, and sign in to that platform in the same browser profile. DEV.to also supports an API key.

## Detect: inspect the account

Open **Detect**, choose a platform and filter **Draft / Published / All**. Results show remote titles, IDs and editor/view links. This reads the account without selecting a local article or creating an article binding.

Listing and matching are different operations: the Detect page lists the account; **Detect associations** on Create/Update compares a selected local article with remote candidates. A title match is a candidate, so inspect the remote link before updating it.

Juejin lists at most the latest 500 drafts and reports truncation. A truncated or failed listing cannot establish that an article is absent. DEV.to editor links require the full URL returned by the platform, including a `temp-slug` suffix when present; a numeric ID alone is insufficient to construct an editor link.

## Create: make a new article

1. Open **Create**, choose a local article and select platforms.
2. Optionally click the top **Detect** button or a platform's **Detect associations** action.
3. Review any matches. For an existing article, use **Update** instead.
4. Choose **Create draft**, or **Create and publish** where that platform supports it.
5. Open **Tasks** and inspect each platform result and remote link.

**Detection is manual and optional.** Selecting/changing an article, refreshing the page, or completing a task does not automatically run detection. Undetected, running or failed detection does not disable creation. A platform with a detected existing article is excluded from batch creation. An active write task disables creation to prevent double submission.

Create always creates a new draft; it does not reuse a previous publication binding. **Create and publish** first creates the draft, then invokes the platform's publishing operation. If the second step fails, inspect the saved draft ID rather than creating another one.

## Update: choose the remote target

1. Open **Update**, choose the local article and platforms.
2. Click **Detect associations**.
3. Open candidate links and select the exact remote articles to replace. Multiple targets are allowed; a known draft ID can be entered where that platform supports it.
4. Choose **Update selected** or **Update and publish**, if available.
5. Inspect the task result and remote editor.

Update requires a remote ID and never falls back to Create. Targets, their remote state and available version metadata are stored with the task. A failed platform does not undo successful writes to other platforms.

| Target | Current UI behavior |
| --- | --- |
| Draft on a supported platform | Replace the selected draft |
| Published cnblogs or DEV.to article | Update the existing public article; cnblogs also checks the detected remote version |
| Published Toutiao article | Submit an update to the same article after checking its detected remote version |
| Published Medium article | Save a pending editor revision; the user clicks **Save and publish** on Medium to make it public |
| Other published articles | Not offered as a normal published-body update in the current UI |

Medium's public body can remain unchanged after a successful pending-edit save. Verify the editor revision, not just the public URL. See [Medium editor integration](../medium-har-contract.md).

## Publish an existing draft

Tasks with a saved remote draft ID may offer **Publish draft** when the adapter supports it. This invokes the draft publish operation for that ID; it does not create another draft. Some platforms require current local article content again as part of that request.

The platform catalog currently exposes explicit publication for cnblogs, Juejin, CSDN, SegmentFault, Zhihu, 51CTO, Toutiao, DEV.to and Medium. OSChina and Hashnode do not expose this action. Hashnode draft creation/update and account listing remain available. Toutiao is available for account detection, draft creation/update and explicit publication. Writes require the installed Extension and its signed-in Toutiao creator editor; BlogCTL opens a background editor for the browser-native signed request. A publish request submits the article for platform review; verify its remote state before retrying. Claimed browser writes are never automatically redelivered after a disconnect. Check the installed UI's enabled actions before choosing a workflow; implementation and account constraints can still reject a request.

## Read a partial failure

| Result | What to do |
| --- | --- |
| Draft ID returned, later image/save step failed | Open that draft; do not assume nothing was created |
| One platform succeeded, another failed | Keep the successful result and inspect only the failed platform |
| Write timed out | Read Tasks and remote state before submitting another write |
| Retry action is offered | Confirm its retained target; a single-target update or eligible Medium resume can reuse it |
| Retry is unavailable | Inspect the remote outcome and submit a new explicit action only when its effect is understood |

Create and publish actions have no generic blind retry. Medium has a specific resume path for an allocated draft or eligible saved edit; it uses the recorded ID and verifies content before continuing. Do not paste a different draft ID into an internal resume request. Old `publications.json` records are retained for history; current explicit Create/Update target selection does not use them to choose an article.

## Indexing and social posts

Indexing is separate from article publication. Use IndexNow, Baidu or Google Search Console with the site's saved search configuration. Acceptance by a provider does not guarantee indexing. 360/Sogou controls currently report `submission_not_implemented` and no remote submission; see [index troubleshooting](../how-to/diagnose-index-submission.md).

Short social posts use the separate [Buffer workflow](social-publishing.md): local draft, review, private Buffer staging, then explicit public publication.

## Console connection

Open the Console from the Extension at `http://127.0.0.1:32145/console/`. Keep the Extension enabled: the page relies on its relay for restricted requests. The page loading successfully does not prove that the browser session or Bridge is connected. Use [Bridge troubleshooting](../how-to/debug-bridge.md) if operations fail.

### Toutiao section references

The compiler assigns heading IDs from matching authored TOC/reference links. BlogCTL flattens the Toutiao TOC while retaining resolvable in-page links; only references without a target degrade to rich text labels. External URLs, including their fragments, are preserved. The creator preview may disable pointer interactions on the entire article.

A published Toutiao update requires a detected remote version. Changed versions and articles still under review are rejected; a successful update is a submission for platform processing, not a guarantee of immediate public visibility.
