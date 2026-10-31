# What each operation changes

| Operation | Remote effect | Target selection |
| --- | --- | --- |
| Detect / list | Reads account state | Platform and account session |
| Detect associations | Reads candidates for one local article | Local article and platform |
| Create draft | Allocates a new article | Local article; detection is optional |
| Update | Replaces selected content | Explicit remote ID |
| Publish draft | Makes the selected draft public when supported | Known draft ID |
| Index submission | Notifies a search provider | Configured site and provider |

Detection does not grant permission or create a publication binding. Clicking Create is a separate decision and creates a new draft even if an older task once wrote that article.

A timeout after a write is ambiguous: the server may have committed it. Tasks retain remote IDs and events so the result can be inspected. Generic Create/Publish does not blindly retry; an eligible Medium resume instead continues the task's recorded draft or saved editor revision. Single-target updates without publishing can offer a retained-target retry.

A Medium pending edit is not a new public version. The user finishes **Save and publish** in Medium. Unsupported updates fail rather than silently creating another article. See [workflows](../guide/workflows.md) for supported actions and recovery.
