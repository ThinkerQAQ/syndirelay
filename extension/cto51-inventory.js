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

// Match a local article only to verified draft titles from the browser page.
// Do not accept arbitrary remote IDs or fuzzy/substring matches.
export function match51CTODraftTitles(items, localTitle) {
  const normalized = (value) => String(value || "").trim().replace(/\s+/g, " ");
  const title = normalized(localTitle);
  if (!title) return [];
  return (Array.isArray(items) ? items : []).filter((item) => {
    if (item?.published || !/^[0-9]{1,20}$/.test(String(item?.id || ""))) return false;
    const remote = normalized(item?.title);
    if (remote === title) return true;
    return [" · ", " - ", " — "].some((separator) => remote.startsWith(title + separator));
  });
}
