import assert from "node:assert/strict";
import test from "node:test";
import { findMediumEditorTab } from "../medium-upload.js";

const id = "65eb8f90268b";
const editor = "https://medium.com/p/" + id + "/edit";
const fast = async () => {};

test("wait for pendingUrl after chrome.tabs.create reports about:blank", async () => {
  const history = [
    { id: 12, url: "about:blank", pendingUrl: editor, status: "loading" },
    { id: 12, url: editor, status: "loading" },
    { id: 12, url: editor, status: "complete" },
  ];
  const mock = {
    async query() { return []; },
    async create(options) {
      assert.equal(options.url, editor);
      assert.equal(options.active, false);
      return history[0];
    },
    async get() { return history.shift() ?? { id: 12, url: editor, status: "complete" }; },
  };
  assert.equal(await findMediumEditorTab(id, mock, fast), 12);
});

test("completed about:blank is not mistaken for navigation away when pendingUrl points to editor", async () => {
  const history = [
    { id: 10, status: "complete", url: "about:blank", pendingUrl: editor },
    { id: 10, status: "complete", url: editor },
  ];
  const mock = {
    async query() { return []; },
    async create() { return history[0]; },
    async get() { return history.shift() ?? { id: 10, status: "complete", url: editor }; },
  };
  assert.equal(await findMediumEditorTab(id, mock, fast), 10);
});

test("an already complete existing editor can be reused by navigation", async () => {
  const mock = {
    async query() { return [{ id: 88, url: editor, status: "complete" }]; },
    async create() { throw Error("should not create another tab"); },
    async get() { return { id: 88, url: editor, status: "complete" }; },
  };
  assert.equal(await findMediumEditorTab(id, mock, fast), 88);
});

test("reject a confirmed redirect, but only after the navigation resolves", async () => {
  const mock = {
    async query() { return []; },
    async create() { return { id: 3, url: "about:blank", pendingUrl: editor }; },
    async get() { return { id: 3, url: "https://medium.com/", status: "complete" }; },
  };
  await assert.rejects(findMediumEditorTab(id, mock, fast), /redirected/);
});

test("reject a navigation that changes pendingUrl to an unrelated document", async () => {
  const mock = {
    async query() { return []; },
    async create() { return { id: 4, url: "about:blank", pendingUrl: editor }; },
    async get() { return { id: 4, url: "about:blank", pendingUrl: "https://other.example/", status: "loading" }; },
  };
  await assert.rejects(findMediumEditorTab(id, mock, fast), /redirected/);
});

test("invalid draft ID is rejected before tab access", async () => {
  await assert.rejects(findMediumEditorTab("../../other", {}, fast), /Invalid Medium draft/);
});
