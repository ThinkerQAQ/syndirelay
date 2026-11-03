// Read the real signed-in 51CTO draft manager DOM. The legacy
// /creative-center-ajax/list endpoint may return an HTML page instead of JSON.
// This function must be self-contained for chrome.scripting.executeScript.
export async function read51CTOInventoryInPage() {
  if (location.origin !== "https://blog.51cto.com" ||
      location.pathname !== "/creative-center/draft") {
    throw new Error("51CTO inventory requires the authenticated draft manager");
  }

  const extract = () => {
    const seen = new Set();
    const items = [];
    for (const link of document.querySelectorAll('a[href*="/blogger/draft/"]')) {
      let parsed;
      try {
        parsed = new URL(link.getAttribute("href") || "", location.origin);
      } catch {
        continue;
      }
      if (parsed.origin !== location.origin || parsed.search || parsed.hash ||
          parsed.username || parsed.password) continue;
      const match = /^\/blogger\/draft\/([0-9]{1,20})$/.exec(parsed.pathname);
      if (!match || seen.has(match[1])) continue;
      const title = String(link.textContent || "").replace(/\s+/g, " ").trim();
      if (!title || title === "编辑") continue;
      seen.add(match[1]);
      items.push({
        id: match[1],
        title,
        published: false,
        url: `https://blog.51cto.com/blogger/draft/${match[1]}`,
      });
    }
    return items;
  };

  // The 51CTO page may hydrate after the browser reports tab.status=complete.
  let items = [];
  for (let attempt = 0; attempt < 30; attempt++) {
    items = extract();
    if (items.length) break;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  const body = String(document.body?.innerText || "");
  if (!body.includes("草稿箱") || !body.includes("草稿管理")) {
    throw new Error("51CTO draft manager is not authenticated or its layout changed");
  }
  if (items.length === 0 && !/草稿箱\s*\(?\s*0\s*\)?/.test(body)) {
    throw new Error("51CTO draft manager did not expose any draft links");
  }
  return {
    platform: "51cto",
    account: "",
    items,
    partial: "实时读取了 51CTO 草稿；已发布文章列表暂未核验。",
  };
}

// Match draft and published titles only to verified links from the manager.
// Do not accept arbitrary remote IDs or fuzzy/substring matches.
export function match51CTOArticleTitles(items, localTitle) {
  const normalized = (value) => String(value || "").trim().replace(/\s+/g, " ");
  const title = normalized(localTitle);
  if (!title) return [];
  return (Array.isArray(items) ? items : []).filter((item) => {
    if (!/^[0-9]{1,20}$/.test(String(item?.id || "")) || typeof item?.published !== "boolean") return false;
    const remote = normalized(item?.title);
    if (remote === title) return true;
    return [" · ", " - ", " — "].some((separator) => remote.startsWith(title + separator));
  });
}


// Fetch only article identity fields from the signed-in article manager.
// The manager's pagination is dynamic (javascript: links), so advance it
// through its own UI instead of calling the obsolete JSON endpoint.
// This function must remain self-contained for chrome.scripting.executeScript.
export async function read51CTOPublishedInPage() {
  if (location.origin !== "https://blog.51cto.com" ||
      location.pathname !== "/creative-center/manager") {
    throw new Error("51CTO published inventory requires the authenticated article manager");
  }
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const collect = () => {
    const rows = new Map();
    for (const link of document.querySelectorAll('a[href*="/u_"]')) {
      let url;
      try { url = new URL(link.getAttribute("href") || "", location.origin); } catch { continue; }
      if (url.origin !== location.origin || url.search || url.hash ||
          url.username || url.password) continue;
      const match = /^\/u_([0-9]{1,20})\/([0-9]{1,20})\/?$/.exec(url.pathname);
      if (!match) continue;
      const title = String(link.textContent || "").replace(/\s+/g, " ").trim();
      if (!title) continue;
      rows.set(match[2], {
        id: match[2], title, published: true,
        url: `https://blog.51cto.com/u_${match[1]}/${match[2]}`,
      });
    }
    return rows;
  };
  let count = null;
  for (let attempt = 0; attempt < 40; attempt++) {
    const body = String(document.body?.innerText || "");
    if (body.includes("文章管理") && body.includes("草稿管理")) {
      const match = /公开\s*[（(]\s*(\d+)\s*[）)]/.exec(body);
      if (match) { count = Number(match[1]); break; }
    }
    await sleep(250);
  }
  if (!Number.isInteger(count) || count < 0 || count > 1000) {
    throw new Error("51CTO published article count is unavailable; refusing partial discovery");
  }

  const items = new Map();
  for (let page = 1; page <= 100; page++) {
    for (const [id, item] of collect()) items.set(id, item);
    if (items.size >= count) break;
    const nextPage = String(page + 1);
    const next = [...document.querySelectorAll("a")].find((link) =>
      link.getAttribute("href") === "javascript:;" &&
      String(link.textContent || "").trim() === nextPage);
    if (!next) throw new Error("51CTO published article pagination is incomplete");
    const current = [...collect().keys()].join(",");
    next.click();
    let advanced = false;
    for (let attempt = 0; attempt < 40; attempt++) {
      await sleep(250);
      if ([...collect().keys()].join(",") !== current) { advanced = true; break; }
    }
    if (!advanced) throw new Error("51CTO published article pagination did not advance");
  }
  if (items.size !== count) {
    throw new Error(`51CTO published article inventory incomplete (${items.size}/${count})`);
  }
  return { items: [...items.values()] };
}
