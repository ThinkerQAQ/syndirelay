# Publish your first platform draft

This walkthrough uses DEV.to as an example. Start with one article and one platform so the result is easy to inspect.

## Prepare the article and account

Complete [BlogCTL installation](../quick-start.md) and the [Inkerio content tutorial](../../../../docs/tutorial/first-site.md). In **Settings**, set the engine and content roots. For the public template, the root slug `sample-article` has both Chinese and English versions.

In the platform's settings, choose `en` for DEV.to. Sign in to DEV.to in the browser profile running the Extension, or configure its API key. Other browser platforms need their own signed-in sessions. An interface-language change does not change article language.

## Check compilation locally

From the content repository, with `blogctl` on PATH:

```bash
blogctl sync --article sample-article --platforms devto --dry-run
```

This runs the local compiler; it does not require a running authenticated Bridge. It can install missing Node dependencies and save workspace paths locally. A zero exit code confirms compilation, not remote login, image upload or publication. No HTML preview file is promised by this CLI command.

If the engine is not beside the content checkout under its usual name, set `BLOGCTL_ENGINE_ROOT` to the engine's absolute path. See [CLI workspace selection](../reference/cli.md).

## Create the draft

1. Open **Create**, select `sample-article` and DEV.to.
2. If the account may already contain that article, click **Detect**. Detection is optional and manual; there is no automatic scan to wait for.
3. For an existing article, switch to **Update** and select it. Otherwise choose **Create draft**.
4. Open **Tasks**, inspect the per-platform outcome, then open the returned editor link.

Creating the sample writes it to your real platform account. Only perform this step when you want that draft. The script's successful dry run does not authorize or execute this write.

## Verify the result

Check the remote title, language, body, cover, image order and original-source link. The task should contain a remote ID and a draft result. If image insertion or another later step fails, the article may already exist; use the recorded ID before making another create request.

Updating a draft and publishing it are separate actions. Use the task's **Publish draft** action only when you intend to make it public and the platform supports it. For Medium published edits, save the pending version and finish **Save and publish** manually on Medium. See [platform differences](../guide/workflows.md).
