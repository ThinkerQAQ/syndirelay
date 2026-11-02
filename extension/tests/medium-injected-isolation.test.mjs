import assert from "node:assert/strict";
import test from "node:test";
import vm from "node:vm";
import { insertMediumImageInPage, verifyMediumDraftInPage } from "../medium-insert.js";

const draft = "df48e08a9cfb";
const marker = "[BLOGCTL_IMAGE_001]";

// Mimic chrome.scripting.executeScript({world:"MAIN",func}): the imported JS
// module and its top-level helpers do not exist in the page's global scope.
function mainWorld(fn, globals) {
  return vm.runInNewContext("(" + fn.toString() + ")", { URL, ...globals });
}

function pageFixture() {
  const images = [];
  let markerPresent = true;
  const markerEl = {
    isContentEditable: true,
    get textContent() { return markerPresent ? marker : ""; },
    querySelectorAll: () => [],
    getAttribute: key => key === "contenteditable" ? "true" : null,
    scrollIntoView() {}, focus() {},
    dispatchEvent() {
      images.push({complete:true,naturalWidth:640,currentSrc:"https://miro.medium.com/v2/resize:fit:800/1*image.png"});
      return true;
    }
  };
  const editor = {
    querySelectorAll(selector) {
      if (selector === "p") return markerPresent ? [markerEl] : [];
      if (selector === "figure img,article img,img") return images;
      throw Error("Unexpected editor selector "+selector);
    }
  };
  const document = {
    body: {innerText:""},
    querySelector(selector) {
      if (selector === '.postArticle-content[contenteditable="true"]') return editor;
      if (selector === ".js-metabarMessage") return {textContent:"DraftSaved"};
      return null;
    },
    querySelectorAll(selector) {
      if (selector === "p,div,span,h1,h2,h3,h4") return markerPresent ? [markerEl] : [];
      throw Error("Unexpected document selector "+selector);
    },
    createRange() {return {selectNodeContents(){},collapse(){}};},
    execCommand(command) {
      assert.equal(command,"delete");
      markerPresent = false;
      return true;
    }
  };
  const window = {getSelection() {return {removeAllRanges(){},addRange(){}}}};
  class ClipboardEvent {constructor(){}}
  class File {constructor(){}}
  class DataTransfer {items={add(){}};}
  let clock = 0;
  const Date = class {static now(){return clock+=700;}};
  const ctx = {location:{origin:"https://medium.com",pathname:"/p/"+draft+"/edit"},
    document,window,Node:{DOCUMENT_POSITION_FOLLOWING:4},
    atob: value => Buffer.from(value,"base64").toString("binary"), Uint8Array,
    File, DataTransfer, ClipboardEvent, Date,
    setTimeout: fn => fn()};
  return {ctx, images};
}

test("Medium MAIN-world image insertion is self-contained and actually inserts a loaded image",async()=>{
  const {ctx,images}=pageFixture();
  const injected=mainWorld(insertMediumImageInPage,ctx);
  const result=await injected("aGVsbG8=","image/png","test.png",draft,marker);
  assert.equal(result.inserted,true, JSON.stringify(result));
  assert.equal(result.imageCount,1);
  assert.equal(images.length,1);
});

test("Medium MAIN-world saved-draft verification is self-contained",()=>{
  const {ctx,images}=pageFixture();
  images.push({complete:true,naturalWidth:640,src:"https://miro.medium.com/v2/1*test.png"});
  ctx.document.querySelector = selector => {
    if (selector === '.postArticle-content[contenteditable="true"]') {
      return {querySelectorAll: key => key === "p" ? [] : images};
    }
    if (selector === ".js-metabarMessage") return {textContent:"DraftSaved"};
    return null;
  };
  const injected=mainWorld(verifyMediumDraftInPage,ctx);
  const result=injected(draft,1);
  assert.equal(result.inserted,true,JSON.stringify(result));
  assert.equal(result.imageCount,1);
});
