"use strict";

(function (root) {
  const PLATFORMS_KEY = "blogctl.sync.selectedPlatforms";
  const MATCH_KEY = "blogctl.sync.lastMatches";
  const MAX_AGE_MS = 24 * 60 * 60 * 1000;

  function loadPlatforms(storage) {
    try {
      const value = JSON.parse(storage.getItem(PLATFORMS_KEY));
      return Array.isArray(value) ? value.filter((id) => typeof id === "string") : [];
    } catch { return []; }
  }

  function hasPlatformPreference(storage) {
    try { return storage.getItem(PLATFORMS_KEY) !== null; }
    catch { return false; }
  }

  function savePlatforms(storage, ids) {
    try { storage.setItem(PLATFORMS_KEY, JSON.stringify([...new Set(ids)])); }
    catch { /* Publishing remains usable when browser storage is full. */ }
  }

  function loadMatches(storage, slug, now = Date.now()) {
    try {
      const value = JSON.parse(storage.getItem(MATCH_KEY));
      if (value?.slug !== slug || !Number.isFinite(value.savedAt) || value.savedAt > now || now - value.savedAt > MAX_AGE_MS || !value.matches || typeof value.matches !== "object") return null;
      return { matches: value.matches, savedAt: value.savedAt };
    } catch { return null; }
  }

  function saveMatches(storage, slug, matches, now = Date.now()) {
    const safe = {};
    for (const [platform, match] of Object.entries(matches)) {
      if (!match || typeof match.text !== "string" || match.text.includes("失败")) continue;
      safe[platform] = {
        text: match.text,
        items: (match.items ?? []).map((item) => ({
          title: item.title, id: item.id, published: item.published, url: item.url,
          bound: item.bound, bindingState: item.bindingState, unverified: item.unverified, localOnly: item.localOnly,
        })),
      };
    }
    try { storage.setItem(MATCH_KEY, JSON.stringify({ slug, matches: safe, savedAt: now })); }
    catch { /* A refreshed result stays visible in the active panel. */ }
  }

  function clearMatches(storage) {
    try { storage.removeItem(MATCH_KEY); }
    catch { /* Ignore unavailable storage. */ }
  }

  root.BlogCTLSyncState = { loadPlatforms, hasPlatformPreference, savePlatforms, loadMatches, saveMatches, clearMatches };
})(globalThis);
