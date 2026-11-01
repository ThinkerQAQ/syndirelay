# Extension bundled third-party notices

The shipped `popup/log-editor-vendor.js` is built from
`ui/log-viewer/src.js` with `ui/log-viewer/build.mjs` (esbuild).
It bundles the pinned runtime dependencies from
`ui/log-viewer/package-lock.json`. All seven runtime packages below
declare MIT; complete upstream LICENSE files are preserved as downloaded
from those exact npm package versions.

| Runtime package | Version | License | Complete notice |
| --- | --- | --- | --- |
| `@codemirror/search` | `6.7.2` | MIT | [`codemirror-search.LICENSE.txt`](licenses/log-editor/codemirror-search.LICENSE.txt) |
| `@codemirror/state` | `6.7.6` | MIT | [`codemirror-state.LICENSE.txt`](licenses/log-editor/codemirror-state.LICENSE.txt) |
| `@codemirror/view` | `6.43.14` | MIT | [`codemirror-view.LICENSE.txt`](licenses/log-editor/codemirror-view.LICENSE.txt) |
| `@marijn/find-cluster-break` | `1.0.4` | MIT | [`marijn-find-cluster-break.LICENSE.txt`](licenses/log-editor/marijn-find-cluster-break.LICENSE.txt) |
| `crelt` | `1.0.7` | MIT | [`crelt.LICENSE.txt`](licenses/log-editor/crelt.LICENSE.txt) |
| `style-mod` | `4.1.4` | MIT | [`style-mod.LICENSE.txt`](licenses/log-editor/style-mod.LICENSE.txt) |
| `w3c-keyname` | `2.2.8` | MIT | [`w3c-keyname.LICENSE.txt`](licenses/log-editor/w3c-keyname.LICENSE.txt) |

Build-only packages (esbuild and its platform binary) are not embedded in
the shipped log editor. They must be audited separately when distributing
a build tool or binary package.

The shipped `popup/i18next-vendor.js` is i18next v25.8.3, MIT licensed.
See [the preserved upstream notice](i18next.LICENSE.txt).

The SyndiRelay root MIT license covers original project code and does not
replace any third-party licenses. Public source visibility is separate from
a reviewed, redistributable executable/Extension Release.
