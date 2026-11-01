# Example — build an Inkerio site (separate project)

SyndiRelay **does not own** the static-site build. If you want a working Astro website, check out the public Inkerio engine and public example content separately:

```sh
git clone https://github.com/ThinkerQAQ/inkerio.git
git clone https://github.com/ThinkerQAQ/inkerio-content-template.git
cd inkerio
npm ci
node scripts/assemble-content.mjs --content-root ../inkerio-content-template
npm run build
```

Deploy the generated `dist/` folder using your own site config and static host. See the [Inkerio first-site guide](https://github.com/ThinkerQAQ/inkerio/blob/main/docs/tutorial/first-site.md) and [site behavior configuration](https://github.com/ThinkerQAQ/inkerio/blob/main/docs/reference/site-features.md).

To publish a compiled article to an external platform, use SyndiRelay separately, with your own canonical URL, explicit account permission and platform-specific publishing operation. There is no `tools/blogctl` directory in Inkerio.
