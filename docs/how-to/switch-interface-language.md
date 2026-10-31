# Change the BlogCTL interface language

The browser Extension and Web Console share the same interface and translation resources.

Open **Settings → Interface & language** and select **Follow browser language / Simplified Chinese / English**. Auto follows the browser's language; Chinese is the fallback when it cannot be resolved. A successful choice is saved to `ui_locale` in BlogCTL's `blogctl.toml`, so the next launch and the other UI host use the same preference. The Bridge must be connected for the change to persist.

| Preference | Result |
| --- | --- |
| `auto` | Browser language (`en*` → English, otherwise Simplified Chinese) |
| `zh-CN` | Simplified Chinese |
| `en` | English |

The CLI accepts `BLOGCTL_LANG=zh-CN` or `BLOGCTL_LANG=en` to override the language for a single process. Commands, API error codes, TOML keys, platform IDs and machine-readable outputs never change with the display language.

**Important:** `ui_locale` only changes the interface. `publishing.platforms.<id>.language` independently selects the *article content language* used for distribution. Switching the UI does not translate an article or change its canonical URL.

The UI uses the locally bundled, MIT-licensed i18next v25.8.3 (under `extension/popup/`), and Chrome Manifest translations use the built-in `_locales` mechanism. No runtime network translation service is required. Technical logs and user-supplied article text are intentionally left in their original language.
