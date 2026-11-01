# Optional impact tracking with Reverfolio

SyndiRelay publishes content; [Reverfolio](https://github.com/ThinkerQAQ/reverfolio) measures registered works. They remain independent applications with separate data stores. This integration is disabled by default.

In Settings → Impact tracking, configure:

| Field | Purpose |
| --- | --- |
| Enabled | Automatically register confirmed public publishes/updates |
| Endpoint | Local Reverfolio HTTP origin, for example http://127.0.0.1:19452 |
| Token file | Absolute path to Reverfolio's existing local API token file; enter only the path |
| Source ID | Stable namespace for this publisher/catalog, for example syndirelay |
| Primary language | Explicitly choose zh-CN or en for the work's canonical URL |
| Monitor new | Initial monitoring choice for newly registered works; existing preferences are preserved |

The canonical origin and article paths come from your existing publishing site configuration. Article identity is the stable slug under the configured source ID. Language variants and public platform URLs become channels of that work. Changing identity or canonical mapping can produce a visible receiver conflict; it is never silently reassigned.

After a confirmed public publication/update, the local outbox stores only title, slug, canonical URL, language and published URL. It excludes article bodies, local source paths, drafts, cookies and credentials. The application uses the configured local token file internally; the UI returns no token value.

Delivery runs independently of publishing. A slow/offline receiver does not block publication's network flow. Events survive application restarts and retry with exponential delay capped at 256 minutes. You can inspect pending/error/delivered states and manually retry from Settings. A stable request ID allows the receiver to replay an acknowledged import safely. Once queued, a timeout remains pending until an acknowledgement with a real work ID is received; no second publication is required.

State lives in reverfolio-outbox under the SyndiRelay data directory. Preserve it during upgrades. Disabling the integration pauses delivery without deleting pending records; removing either application does not remove the other's data. Existing publications are not backfilled automatically: export a selected publication catalog for historical registration.

The protected local interfaces are GET/PUT /v1/impact and POST /v1/impact/retry. Receiver redirects and non-loopback destinations are rejected. Browser writes use the same extension transport and permission checks as other settings.
