export async function collectBrowserSessionCookieBatches(definition, getAll, onQuery = () => {}) {
  const cookieUrls = definition.cookieUrls ?? (definition.cookieUrl ? [definition.cookieUrl] : []);
  const cookieDomains = definition.cookieDomains ?? [];
  const partitionKeys = definition.cookiePartitionKeys ?? [];
  const filters = [
    ...cookieUrls.map((url) => ({ url })),
    ...cookieDomains.map((domain) => ({ domain })),
    ...partitionKeys.flatMap((partitionKey) => [
      ...cookieUrls.map((url) => ({ url, partitionKey })),
      ...cookieDomains.map((domain) => ({ domain, partitionKey })),
    ]),
  ];
  return Promise.all(filters.map(async (filter) => {
    try {
      const cookies = await getAll(filter);
      onQuery(filter, cookies);
      return cookies;
    } catch (error) {
      const target = filter.url ? new URL(filter.url).hostname : filter.domain;
      const scope = filter.partitionKey ? "partitioned" : "unpartitioned";
      throw new Error(`cookie query failed for ${target} (${scope}): ${error?.message || String(error)}`);
    }
  }));
}

export function cookieQueryDiagnostic(filter, cookies) {
  const url = filter.url ? new URL(filter.url) : null;
  return {
    target: url ? `${url.hostname}${url.pathname}` : filter.domain,
    partitioned: Boolean(filter.partitionKey),
    count: cookies.length,
    names: cookies.map((cookie) => cookie.name),
  };
}

export function cookieHeaderFromRequest(details, extensionOrigin, expectedURL) {
  if (details?.initiator && details.initiator !== extensionOrigin) return null;
  if (details?.url !== expectedURL || details?.method !== "GET") return null;
  const headers = details.requestHeaders ?? [];
  return headers.filter((header) => header.name?.toLowerCase() === "cookie")
    .map((header) => header.value || "").filter(Boolean).join("; ");
}

function matchesCookieDomain(domain, allowedDomains) {
  const host = String(domain || "").replace(/^\./, "").toLowerCase();
  if (!host) return false;
  return allowedDomains.some((allowed) => {
    const base = String(allowed || "").replace(/^\./, "").toLowerCase();
    return base && (host === base || host.endsWith("." + base));
  });
}

export function selectBrowserSessionCookies(definition, cookieBatches = []) {
  const allowed = Array.isArray(definition?.cookieNames) ? new Set(definition.cookieNames) : null;
  const allowedDomains = (definition?.cookieDomains ?? [])
    .filter(Boolean)
    .map((domain) => String(domain).replace(/^\./, "").toLowerCase());
  const deduped = new Map();

  for (const cookie of cookieBatches.flat()) {
    if (!cookie || !cookie.name || !cookie.value) continue;
    if (allowed && !allowed.has(cookie.name)) continue;
    if (allowedDomains.length > 0 && !matchesCookieDomain(cookie.domain, allowedDomains)) continue;
    const key = [cookie.name, cookie.domain || "", cookie.path || "/", cookie.storeId || "", cookie.partitionKey?.topLevelSite || "", Boolean(cookie.partitionKey?.hasCrossSiteAncestor)].join("\u0000");
    deduped.set(key, {
      name: cookie.name,
      value: cookie.value,
      domain: cookie.domain || "",
      path: cookie.path || "/",
      secure: Boolean(cookie.secure),
      httpOnly: Boolean(cookie.httpOnly),
      hostOnly: Boolean(cookie.hostOnly),
      sameSite: cookie.sameSite || "unspecified",
      expirationDate: Number.isFinite(cookie.expirationDate) ? cookie.expirationDate : null,
      storeId: cookie.storeId || "",
      partitionKey: cookie.partitionKey ?? null,
    });
  }

  const selected = [...deduped.values()];
  for (const required of definition?.requiredCookieNames ?? []) {
    if (!selected.some((cookie) => cookie.name === required && cookie.value)) {
      throw new Error(`required cookie ${required} not found`);
    }
  }
  if (selected.length === 0) {
    throw new Error("no browser cookies were available");
  }
  return selected;
}
