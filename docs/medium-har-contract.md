# Medium editor integration

This is the current implementation contract for BlogCTL's Medium adapter. Browser observations from October 2026 explain the editor choices below; they are not a guarantee that Medium will keep its private interfaces unchanged.

For everyday operation, use [publishing workflows](guide/workflows.md). No captured article IDs, credentials or HAR payloads belong in this document.

## Create, update and publish

| Operation | Target and result |
| --- | --- |
| Create | Allocate a new draft, record its identity durably, then write and verify content |
| Update draft | Use the selected remote draft ID; replace its content and verify saved state |
| Update published article | Edit the selected article's pending editor revision; do not publish automatically |
| Publish | A separate explicit action; published edits require the user to complete the Medium editor's save/publish flow |

A creation failure may occur after Medium allocated a draft. Preserve the recorded link and inspect it before another creation. An update must never silently allocate a replacement story.

## Compiler and browser responsibilities

The compiler produces native Medium paragraphs for text, headings, lists, quotes, code and inline formatting. It also emits a `browserPlan`: the same text sequence with image paragraphs replaced by unique marker paragraphs, plus an ordered image list. The cover is the first image.

The Extension inserts actual PNG files through the logged-in editor's paste handler. It locates the exact marker, observes a Medium-hosted image, removes that marker and proceeds in order. Image insertion must not replace the document with a full HTML clipboard paste, which loses native code and paragraph formatting.

Confirmed paragraph types used by the compiler:

| Type | Meaning |
| --- | --- |
| 6 | Block quote |
| 9 | Unordered-list item |
| 10 | Ordered-list item |
| 13 | Fourth-level heading |
| 8 | PRE / code block |

Wide structural text diagrams are rasterized for Medium because its PRE line wrapping breaks their alignment. Ordinary text/code fences remain PRE; other platform outputs keep their own compilation behavior.

## Saved-state verification

A visible image or acknowledged Delta is not sufficient evidence that the editor saved the complete article.

The integration verifies paragraph order and types, exact text/code, image identity/dimensions and removal of insertion markers against the current compiled source. Medium can insert empty paragraphs after figures; these do not change the expected meaningful block sequence.

Wait for the final autosave to settle. Concurrent-edit conflicts fail the operation. A late `DraftSaved` indicator may use authoritative saved-state readback where the adapter supports it; do not classify every timeout as success.

For a published article, its public GraphQL/body response may still represent the old published revision. Verify the pending editor revision being edited. A successful update does not assert that the public article has changed.

## Partial failure and retry

Generic Create and Publish operations are not safe to replay automatically. The Bridge permits a narrower Medium resume when the failed job has a trustworthy, durably recorded draft-allocation event. The resume retains that remote draft and job identity; it does not call new-story or accept a client-supplied resume ID.

Eligible published-editor image failures also have a restricted resume path. The Bridge checks target state, operation, compiled source and expected image/marker state. Use the retry action only when the task exposes it. A partially completed sequence that fails these checks requires inspection rather than blind replay.

If Medium shows a human-verification page, complete it in the browser. The Extension waits for a real editor; challenge pages are not editable article content.

## What browser captures established

October 2026 captures showed native image uploads to `POST /_/upload?source=6` and editor saves to `/p/{draftId}/deltas?logLockId=...`. Image paragraphs use type 4 with Medium file identity and original dimensions. Save requests carry revision and lock state.

The HAR did not include the uploaded binary body or establish how locks were acquired. Replaying captured locks/revisions is invalid. Go upload timeouts contrasted with successful browser uploads, motivating browser-owned image insertion; the captures did not prove a particular TLS fingerprint or header caused the failure.

Earlier guessed quote/list types were incorrect and are replaced by the table above. Updating compiler constants does not repair existing remote drafts.

## Changing this integration

Read the [browser creation adapter](../bridge/medium_browser_create.go), [compiler](../compiler/medium_compile.go) and Extension implementation before changing the contract. Use offline fixtures and existing tests first. A live create/update requires a specifically authorized target and review of its remote state; a documentation check does not require remote writes.

Keep captures local and redact diagnostics. Cookies, authorization headers, signed URLs, complete request bodies and private article identities must not enter Git.
