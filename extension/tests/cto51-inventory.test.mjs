import test from "node:test";
import assert from "node:assert/strict";
import {
  read51CTOInventoryInPage,
  read51CTOPublishedInPage,
  match51CTOArticleTitles,
} from "../cto51-inventory.js";

function onPage(t, pathname, body, links, pagination = () => []) {
  const priorLocation = globalThis.location;
  const priorDocument = globalThis.document;
  t.after(() => {
    globalThis.location = priorLocation;
    globalThis.document = priorDocument;
  });
  globalThis.location = { origin: "https://blog.51cto.com", pathname };
  globalThis.document = {
    body: { innerText: body },
    querySelectorAll: (selector) => selector === "a" ? pagination() : links(),
  };
}

const link = (href, text, onClick) => ({
  getAttribute: (name) => name === "href" ? href : null,
  textContent: text,
  click: onClick || (() => {}),
});

test("51CTO reads the authenticated draft manager and rejects unsafe links", async (t) => {
  const links = [
    link("/blogger/draft/2108123", "SyndiRelay 端到端草稿测试 2026-11-02（请勿发布）"),
    link("/blogger/draft/2108123", "编辑"),
    link("/blogger/draft/2108110", "杭州这一年"),
    link("https://other.example/blogger/draft/40", "invalid domain"),
    link("/blogger/draft/111?delete=1", "invalid query"),
  ];
  onPage(t, "/creative-center/draft", "草稿箱 (7) 草稿管理", () => links);
  const result = await read51CTOInventoryInPage();
  assert.deepEqual(result.items.map((p) => p.id), ["2108123", "2108110"]);
  assert.equal(result.items[0].published, false);
  assert.equal(result.items[0].url, "https://blog.51cto.com/blogger/draft/2108123");
});

test("51CTO reads all published pages using only first-party manager pagination", async (t) => {
  let page = 1;
  const pages = [
    [
      link("/u_17784995/14930586", "并发编程（零）：并发问题与讨论范围 · ThinkerQAQ"),
      link("/u_17784995/14933456", "并发编程（一）：先谈硬件"),
      link("https://evil.example/u_17784995/123", "offsite"),
    ],
    [link("https://blog.51cto.com/u_17784995/15026307", "并发编程（十一）：Shared Memory")],
  ];
  onPage(t, "/creative-center/manager", "文章管理 草稿管理 公开 (3)", () => pages[page-1],
    () => page === 1 ? [link("javascript:;", "2", () => { page = 2; })] : []);
  const result = await read51CTOPublishedInPage();
  assert.deepEqual(result.items.map((p) => p.id), ["14930586", "14933456", "15026307"]);
  assert.ok(result.items.every((p) => p.published));
  assert.equal(page, 2);
});

test("51CTO article matching considers both drafts and published without fuzzy aliases", () => {
  const title = "SyndiRelay 端到端草稿测试 2026-11-02（请勿发布）";
  const items = [
    { id: "2108123", title, published: false },
    { id: "14999999", title: title + " · ThinkerQAQ", published: true },
    { id: "2108125", title: title + "误", published: false },
    { id: "invalid", title, published: true },
    { id: "123", title, published: "true" },
  ];
  assert.deepEqual(match51CTOArticleTitles(items, title).map((item) => item.id),
    ["2108123", "14999999"]);
  assert.deepEqual(match51CTOArticleTitles(items, ""), []);
});

test("51CTO refuses incomplete published pagination and wrong origin", async (t) => {
  onPage(t, "/creative-center/manager", "文章管理 草稿管理 公开 (2)",
    () => [link("/u_17784995/14930586", "Published article")]);
  await assert.rejects(read51CTOPublishedInPage(), /pagination is incomplete/);
  globalThis.location.pathname = "/blogger/publish";
  await assert.rejects(read51CTOPublishedInPage(), /authenticated article manager/);
  await assert.rejects(read51CTOInventoryInPage(), /authenticated draft manager/);
});
