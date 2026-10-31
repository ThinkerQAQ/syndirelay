import { build } from "esbuild";

await build({
  entryPoints: ["src.js"],
  bundle: true,
  minify: true,
  format: "iife",
  platform: "browser",
  target: ["chrome130"],
  globalName: "BlogCTLLogEditor",
  outfile: "../../extension/popup/log-editor-vendor.js",
  legalComments: "inline",
});
