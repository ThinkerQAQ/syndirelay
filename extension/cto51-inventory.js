// Browser-owned 51CTO inventory reader. Executed in an existing authenticated
// first-party tab, without copying cookies or emulating a browser TLS profile.
// IMPORTANT: This function must be self-contained for chrome.scripting.executeScript.
export async function read51CTOInventoryInPage() {
  if (location.origin !== "https://blog.51cto.com") {
    throw new Error("51CTO inventory requires a signed-in blog.51cto.com tab");
  }
  const url = "https://blog.51cto.com/creative-center-ajax/list";
  const items = [];
  const seen = new Set();
  for (const [type, published] of [[4, false], [2, true]]) {
    for (let page = 1; page <= 50; page++) {
      const payload = { type, blog_type: 0, year: "", month: "", ccid: "", flag: false, level: "" };
      if (page > 1) payload.page = page;
      const body = new URLSearchParams({ data: JSON.stringify(payload) });
      const response = await fetch(url, {
        method: "POST",
        credentials: "same-origin",
        redirect: "follow",
        headers: {
          "content-type": "application/x-www-form-urlencoded; charset=UTF-8",
          "accept": "*/*",
          "x-requested-with": "XMLHttpRequest",
        },
        body,
        signal: AbortSignal.timeout(15000),
      });
      if (!response.ok) throw new Error(`51CTO ${published ? "published" : "draft"} list HTTP ${response.status}`);
      const contentType = response.headers.get("content-type") || "";
      if (!contentType.toLowerCase().includes("json")) {
        throw new Error("51CTO returned an unexpected page instead of the article list");
      }
      const decoded = await response.json();
      if (decoded?.status !== 1 || !Array.isArray(decoded?.data?.list)) {
        throw new Error("51CTO article list schema is no longer supported");
      }
      let added = 0;
      for (const item of decoded.data.list) {
        const id = String(item.blog_id ?? "").trim();
        const title = String(item.title || "").trim();
        if (!/^\d{1,24}$/.test(id) || !title) continue;
        const key = `${published ? "published" : "draft"}:${id}`;
        if (seen.has(key)) continue;
        seen.add(key);
        added++;
        const candidate = published ? String(item.blog_url || "") : String(item.edit_url || "");
        let targetURL = published ? "" : `https://blog.51cto.com/blogger/draft/${id}`;
        if (candidate) {
          try {
            const parsed = new URL(candidate, location.origin);
            if (parsed.protocol === "https:" && parsed.hostname === "blog.51cto.com" && !parsed.username && !parsed.password) {
              targetURL = parsed.href;
            }
          } catch { /* Ignore invalid remote link; no arbitrary navigation. */ }
        }
        items.push({ id, title, published, url: targetURL });
      }
      if (decoded.data.list.length === 0 || added === 0 || !String(decoded.data.pagination || "").trim()) break;
    }
  }
  return { platform: "51cto", account: "", items, partial: "" };
}
