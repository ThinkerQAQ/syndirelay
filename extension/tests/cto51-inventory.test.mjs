import test from "node:test";
import assert from "node:assert/strict";
import { read51CTOInventoryInPage, match51CTODraftTitles } from "../cto51-inventory.js";

test("51CTO reads actual signed-in draft links and ignores unsafe or duplicate links", async (t) => {
  const prevLocation = globalThis.location;
  const prevDocument = globalThis.document;
  t.after(() => {
    globalThis.location = prevLocation;
    globalThis.document = prevDocument;
  });
  globalThis.location = {
    origin: "https://blog.51cto.com",
    pathname: "/creative-center/draft",
  };
  const entries = [
    ["/blogger/draft/2108123", "SyndiRelay 端到端草稿测试 2026-11-02（请勿发布）"],
    ["https://blog.51cto.com/blogger/draft/2108123", "编辑"],
    ["/blogger/draft/2108110", "杭州这一年"],
    ["https://other.example/blogger/draft/40", "攻击者"],
    ["/blogger/draft/111?delete=1", "不安全"],
    ["/blogger/publish/99", "非草稿"],
  ];
  globalThis.document = {
    body: { innerText: "草稿管理 草稿箱 (7)" },
    querySelectorAll: () => entries.map(([href, text]) => ({
      getAttribute: () => href,
      textContent: text,
    })),
  };
  const inventory = await read51CTOInventoryInPage();
  assert.deepEqual(inventory.items.map((item) => item.id), ["2108123", "2108110"]);
  assert.equal(inventory.items[0].published, false);
  assert.equal(inventory.items[0].url, "https://blog.51cto.com/blogger/draft/2108123");
  assert.equal(inventory.platform, "51cto");
  assert.match(inventory.partial, /已发布文章列表/);
});

test("51CTO title association uses exact or known author-suffix matches only", () => {
  const items = [
    { id: "2108123", title: "SyndiRelay 端到端草稿测试 2026-11-02（请勿发布）", published: false },
    { id: "2108124", title: "SyndiRelay 端到端草稿测试 2026-11-02（请勿发布） · ThinkerQAQ", published: false },
    { id: "2108125", title: "SyndiRelay 端到端草稿测试 2026-11-01（请勿发布）", published: false },
    { id: "2108126", title: "SyndiRelay 端到端草稿测试 2026-11-02（请勿发布）", published: true },
    { id: "../666", title: "SyndiRelay 端到端草稿测试 2026-11-02（请勿发布）", published: false },
  ];
  const matches = match51CTODraftTitles(items, " SyndiRelay 端到端草稿测试 2026-11-02（请勿发布） ");
  assert.deepEqual(matches.map((item) => item.id), ["2108123", "2108124"]);
  assert.deepEqual(match51CTODraftTitles(items, "unrelated"), []);
  assert.deepEqual(match51CTODraftTitles(items, ""), []);
});

test("51CTO DOM reader refuses unrelated pages", async (t) => {
  const oldLocation = globalThis.location;
  t.after(() => { globalThis.location = oldLocation; });
  globalThis.location = { origin: "https://blog.51cto.com", pathname: "/blogger/publish" };
  await assert.rejects(read51CTOInventoryInPage(), /authenticated draft manager/);
});
