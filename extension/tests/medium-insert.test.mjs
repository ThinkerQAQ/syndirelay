import assert from "node:assert/strict";
import test from "node:test";
import { validMediumEditorTask, insertMediumImageInPage, verifyMediumDraftInPage, loadedNativeMediumImage, hasNewLoadedMediumImage } from "../medium-insert.js";

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


test("native Medium image detection supports current CDNs without trusting lookalikes", () => {
  const img = url => ({ complete: true, naturalWidth: 640, src: url });
  for (const url of [
    "https://cdn-images-1.medium.com/max/1000/1*picture.png",
    "https://cdn-images-2.medium.com/max/1000/1*picture.png",
    "https://miro.medium.com/v2/resize:fit:720/format:webp/1*picture.png",
    "https://images.medium.com/0*legacy.png",
  ]) assert.equal(loadedNativeMediumImage(img(url)), true, url);
  for (const url of [
    "http://miro.medium.com/v2/image.png",
    "https://miro.medium.com.evil.invalid/a.png",
    "https://cdn-images-1.medium.com.evil.invalid/a.png",
    "https://untrusted.example.com/1.png",
    "blob:https://medium.com/image",
    "data:image/png;base64,AAAA",
  ]) assert.equal(loadedNativeMediumImage(img(url)), false, url);
  assert.equal(loadedNativeMediumImage({ complete: false, naturalWidth: 640, src: "https://miro.medium.com/a.png" }), false);
  assert.equal(loadedNativeMediumImage({ complete: true, naturalWidth: 0, src: "https://miro.medium.com/a.png" }), false);
});

test("previously saved miro image is idempotent and never pasted again", async () => {
  const previousLocation = globalThis.location;
  const previousDocument = globalThis.document;
  const previousNode = globalThis.Node;
  const nextMarker = { textContent: "[BLOGCTL_IMAGE_002]" };
  const img = { complete: true, naturalWidth: 800, currentSrc: "https://miro.medium.com/v2/1*saved.png",
    compareDocumentPosition(other) { return other === nextMarker ? 4 : 0; } };
  const editor = { querySelectorAll(selector) {
    if (selector === "p") return [nextMarker];
    if (selector === "figure img,article img,img") return [img];
    throw new Error(`Unexpected selector: ${selector}`);
  } };
  const document = {
    querySelector(selector) {
      if (selector === '.postArticle-content[contenteditable="true"]') return editor;
      if (selector === ".js-metabarMessage") return {textContent: "DraftSaved"};
      return null;
    },
    querySelectorAll(selector) {
      if (selector === "p,div,span,h1,h2,h3,h4") return [];
      throw new Error(`Unexpected selector: ${selector}`);
    }
  };
  globalThis.location = { origin: "https://medium.com", pathname: "/p/abcdef123456/edit" };
  globalThis.document = document;
  globalThis.Node = { DOCUMENT_POSITION_FOLLOWING: 4 };
  try {
    const result = await insertMediumImageInPage("iVBORw0KGgo=", "image/png", "diagram.png",
      "abcdef123456", "[BLOGCTL_IMAGE_001]");
    assert.deepEqual(result, { inserted: true, alreadyPresent: true, imageCount: 1 });
  } finally {
    if (previousLocation === undefined) delete globalThis.location; else globalThis.location = previousLocation;
    if (previousDocument === undefined) delete globalThis.document; else globalThis.document = previousDocument;
    if (previousNode === undefined) delete globalThis.Node; else globalThis.Node = previousNode;
  }
});

test("saved-draft verification accepts Medium miro images and rejects foreign sources", () => {
  const originalLocation = globalThis.location;
  const originalDocument = globalThis.document;
  let src = "https://miro.medium.com/v2/resize:fit:640/1*example.png";
  const image = { complete:true, naturalWidth: 600, get src() { return src; } };
  globalThis.location = { origin: "https://medium.com", pathname: "/p/abcdef123456/edit" };
  globalThis.document = {querySelector(selector) {
    if (selector === '.postArticle-content[contenteditable="true"]') return { querySelectorAll(lookFor) {
      return lookFor === "p" ? [] : [image];
    }};
    if (selector === ".js-metabarMessage") return {textContent: "DraftSaved"};
    return null;
  }};
  try {
    assert.deepEqual(verifyMediumDraftInPage("abcdef123456", 1), {inserted:true, imageCount:1});
    src = "https://example.com/a.png";
    assert.match(verifyMediumDraftInPage("abcdef123456", 1).error, /native CDN/);
  } finally {
    if (originalLocation === undefined) delete globalThis.location; else globalThis.location = originalLocation;
    if (originalDocument === undefined) delete globalThis.document; else globalThis.document = originalDocument;
  }
});


test("insertion verifies only a newly loaded Medium image; final draft checks all", () => {
  const older = {complete:false,naturalWidth:0,src:"blob:https://medium.com/older"};
  const inserted = {complete:true,naturalWidth:640,src:"https://miro.medium.com/1*new.png"};
  assert.equal(hasNewLoadedMediumImage([older],[older,inserted]),true);
  assert.equal(hasNewLoadedMediumImage([older],[older,{complete:true,naturalWidth:300,src:"https://evil.invalid/img.png"}]),false);
  assert.equal(hasNewLoadedMediumImage([inserted],[inserted]),false);
  assert.equal(hasNewLoadedMediumImage([inserted],[{...inserted}]),false);
});

test("Medium does not delete the marker before the new native image is confirmed", async () => {
  const {readFile} = await import("node:fs/promises");
  const source = await readFile(new URL("../medium-insert.js",import.meta.url),"utf8");
  const start = source.indexOf("export async function insertMediumImageInPage");
  const end = source.indexOf("export function verifyMediumDraftInPage",start);
  const insertion = source.slice(start,end);
  const upload = insertion.indexOf("if (!uploaded) {");
  const deletion = insertion.indexOf('document.execCommand("delete"');
  assert.ok(upload>=0 && deletion>upload,"preserve placeholder until native upload is confirmed");
});
