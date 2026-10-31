import assert from "node:assert/strict";
import test from "node:test";
import { validMediumUploadTask, uploadMediumInPage } from "../medium-upload.js";

test("Medium browser upload task only accepts exact draft image references", () => {
  const task = { id: "a".repeat(32), draftId: "2b52b8609f8e", filename: "cover.png", mimeType: "image/png", base64: "iVBORw0KGgo=" };
  assert.equal(validMediumUploadTask(task), true);
  for (const bad of [
    { ...task, draftId: "../evil" },
    { ...task, filename: "../../cookies" },
    { ...task, mimeType: "text/html" },
    { ...task, id: "wrong" },
    { ...task, base64: "" },
  ]) assert.equal(validMediumUploadTask(bad), false);
});

test("Medium upload function refuses any page that is not the exact editor", async () => {
  globalThis.location = { origin: "https://evil.example", pathname: "/p/2b52b8609f8e/edit" };
  try {
    assert.deepEqual(await uploadMediumInPage("AAAA", "image/png", "cover.png", "2b52b8609f8e"),
      { error: "Medium editor tab URL does not match the selected draft" });
  } finally {
    delete globalThis.location;
  }
});

test("Medium same-origin browser upload returns only native image metadata", async () => {
  globalThis.location = { origin: "https://medium.com", pathname: "/p/2b52b8609f8e/edit" };
  globalThis.document = { cookie: "other=none; xsrf=synthetic-xsrf" };
  let calls = 0;
  globalThis.fetch = async (url, options) => {
    calls++;
    assert.equal(url, "/_/upload?source=6");
    assert.equal(options.method, "POST");
    assert.equal(options.credentials, "same-origin");
    assert.equal(options.headers["x-xsrf-token"], "synthetic-xsrf");
    assert.equal(options.headers["x-obvious-cid"], "web");
    const part = options.body.get("uploadedFile");
    assert.equal(part.name, "cover.png");
    assert.equal(part.type, "image/png");
    return { ok: true, text: async () => '])}while(1);</x>{"success":true,"payload":{"value":{"fileId":"1*medium.png","imgWidth":800,"imgHeight":600}}}' };
  };
  try {
    const output = await uploadMediumInPage(btoa("image-bytes"), "image/png", "cover.png", "2b52b8609f8e");
    assert.deepEqual(output, { fileId: "1*medium.png", width: 800, height: 600 });
    assert.equal(calls, 1);
    assert.equal(JSON.stringify(output).includes("synthetic-xsrf"), false);
  } finally {
    delete globalThis.fetch;
    delete globalThis.location;
    delete globalThis.document;
  }
});

test("Medium browser upload rejects missing XSRF before any network request", async () => {
  globalThis.location = { origin: "https://medium.com", pathname: "/p/2b52b8609f8e/edit" };
  globalThis.document = { cookie: "" };
  globalThis.fetch = () => { throw new Error("must not fetch"); };
  try {
    assert.deepEqual(await uploadMediumInPage(btoa("image-bytes"), "image/png", "cover.png", "2b52b8609f8e"),
      { error: "Medium editor XSRF cookie is unavailable" });
  } finally {
    delete globalThis.fetch;
    delete globalThis.location;
    delete globalThis.document;
  }
});
