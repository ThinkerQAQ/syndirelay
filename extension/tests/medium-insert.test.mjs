import assert from "node:assert/strict";
import test from "node:test";
import { validMediumEditorTask, insertMediumImageInPage, verifyMediumDraftInPage } from "../medium-insert.js";

test("BlogCTL image insertion queue accepts only authorized draft and raster markers", () => {
  const source = {
    id: "a".repeat(32), draftId: "abcdef123456", mode: "insert",
    marker: "[BLOGCTL_IMAGE_001]", filename: "diagram-01.png",
    mimeType: "image/png", base64: "iVBORw0KGgo=",
  };
  assert.equal(validMediumEditorTask(source), true);
  for (const invalid of [
    { ...source, marker: "[IMAGE 01]" },
    { ...source, draftId: "../secret" },
    { ...source, filename: "../../secret.png" },
    { ...source, mimeType: "image/svg+xml" },
    { ...source, base64: "" },
  ]) assert.equal(validMediumEditorTask(invalid), false);
  assert.equal(validMediumEditorTask({
    id: "a".repeat(32), mode: "verify", draftId: "abcdef123456", expectedImages: 42,
  }), true);
});

test("Browser-editor image insertion never attempts changes on another site", async () => {
  globalThis.location = { origin: "https://not-medium.example", pathname: "/p/abcdef123456/edit" };
  try {
    const result = await insertMediumImageInPage("AAAA", "image/png", "diagram.png",
      "abcdef123456", "[BLOGCTL_IMAGE_001]");
    assert.match(result.error, /validation/);
  } finally { delete globalThis.location; }
});

test("Browser image verification never trusts an unrelated draft", () => {
  globalThis.location = { origin: "https://medium.com", pathname: "/p/otherstory/edit" };
  try {
    const result = verifyMediumDraftInPage("abcdef123456", 42);
    assert.match(result.error, /expected Medium draft/);
  } finally { delete globalThis.location; }
});

test("BlogCTL only completes after native editor reports autosaved pictures and no placeholders", () => {
  const originalLocation = globalThis.location;
  const originalDocument = globalThis.document;
  const pngs = [
    { complete:true, naturalWidth:1200, src:"https://cdn-images-1.medium.com/max/1000/1*diagram-a.png" },
    { complete:true, naturalWidth:1200, src:"https://cdn-images-1.medium.com/max/1000/1*diagram-b.png" },
  ];
  const editor = {
    querySelectorAll(selector) {
      if (selector === "p") return [];
      if (selector === "figure img,article img,img") return pngs;
      throw new Error("Unexpected selector: "+selector);
    }
  };
  let saved = true;
  globalThis.location={origin:"https://medium.com",pathname:"/p/abcdef123456/edit"};
  globalThis.document={
    querySelector(selector) {
      if(selector==='.postArticle-content[contenteditable="true"]')return editor;
      if(selector===".js-metabarMessage")return {textContent:saved?"DraftSaved":"DraftSaving..."};
      return null;
    }
  };
  try {
    assert.deepEqual(verifyMediumDraftInPage("abcdef123456",2),{inserted:true,imageCount:2});
    saved=false;
    assert.match(verifyMediumDraftInPage("abcdef123456",2).error,/autosave/);
    saved=true;
    editor.querySelectorAll=selector=>selector==="p" ? [{textContent:"[BLOGCTL_IMAGE_002]"}] : pngs;
    assert.match(verifyMediumDraftInPage("abcdef123456",2).error,/placeholders/);
    editor.querySelectorAll=selector=>selector==="p" ? [] : pngs.slice(0,1);
    assert.match(verifyMediumDraftInPage("abcdef123456",2).error,/expected 2/);
  } finally {
    if(originalLocation===undefined)delete globalThis.location;else globalThis.location=originalLocation;
    if(originalDocument===undefined)delete globalThis.document;else globalThis.document=originalDocument;
  }
});

test("Bridge Medium Extension verifies via editor DOM instead of reloading draft", async () => {
  const {readFile} = await import("node:fs/promises");
  const source=await readFile(new URL("../background.js",import.meta.url),"utf8");
  const begin=source.indexOf('if (task.mode === "verify")');
  const end=source.indexOf('if (!validMediumUploadTask(task))',begin);
  assert.ok(begin>=0&&end>begin);
  const block=source.slice(begin,end);
  assert.ok(block.includes("verifyMediumDraftInPage"));
  assert.ok(!block.includes("chrome.tabs.reload"),"Medium autosave check must never force reload");
});
