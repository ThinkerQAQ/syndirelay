import { performToutiaoBrowserRequest } from "./toutiao-request.js";
import { prepareMediumReplacement, verifyMediumDocument } from "./medium-document.js";
import { configureMediumCanonical } from "./medium-settings.js";
import { PLATFORM_AUTH, PLATFORM_SESSIONS } from "./platforms.js";
import { collectBrowserSessionCookieBatches, cookieHeaderFromRequest, cookieQueryDiagnostic, selectBrowserSessionCookies } from "./session.js";
import { toError } from "./errors.js";
import { uploadMediumInPage, validMediumUploadTask, findMediumEditorTab } from "./medium-upload.js";
import { insertMediumImageInPage, verifyMediumDraftInPage, validMediumEditorTask } from "./medium-insert.js";

const NATIVE_HOST = "com.thinkerqaq.blogctl";
const AUTH_TIMEOUT_MS = 7000;
const BRIDGE_CACHE_MS = 30000;
let googleSearchConsoleProperty = "";
let googleSearchConsoleURL = "";
let googleIndexOrigin = "";
let bridgeSession = null;
let googleSearchConsoleTabId = null;
let googleIndexPumpPromise = null;
let googleSearchConsoleStatus = {
  known: false,
  loggedIn: false,
  ready: false,
  tabId: null,
  error: "",
  code: "",
};
const pendingCNBlogsCookieCaptures = new Map();
const pendingPlatformCookieCaptures = new Map();
// Browser-observed Toutiao editor headers live only in extension memory.
const toutiaoEditorHeaders = {};
let toutiaoEditorHeadersCapturedAt = 0;
const extensionOrigin = chrome.runtime.getURL("").replace(/\/$/, "");
const openControlTab = () => chrome.tabs.create({ url: chrome.runtime.getURL("popup/popup.html") });

if (chrome.sidePanel?.setPanelBehavior) {
  chrome.sidePanel.setPanelBehavior({ openPanelOnActionClick: true }).catch((error) => {
    console.error("BlogCTL side panel setup failed:", errorMessage(error));
    chrome.action.onClicked.addListener(openControlTab);
  });
} else {
  console.warn("BlogCTL side panel API is unavailable in this browser");
  chrome.action.onClicked.addListener(openControlTab);
}

chrome.webRequest.onSendHeaders.addListener((details) => {
  const pending = pendingCNBlogsCookieCaptures.get(details.url);
  if (!pending) return;
  const header = cookieHeaderFromRequest(details, extensionOrigin, pending.url);
  if (header !== null) pending.resolve(header);
}, { urls: ["https://i.cnblogs.com/api/user*"] }, ["requestHeaders", "extraHeaders"]);

const platformSessionRequestPatterns = [...new Set(
  Object.values(PLATFORM_SESSIONS).flatMap((definition) =>
    (definition.cookieDomains ?? []).flatMap((domain) => [
      `https://${domain}/*`,
      `https://*.${domain}/*`,
    ]),
  ),
)];

chrome.webRequest.onSendHeaders.addListener((details) => {
  const pending = pendingPlatformCookieCaptures.get(details.url);
  if (!pending) return;
  const header = cookieHeaderFromRequest(details, extensionOrigin, pending.url);
  if (header !== null) pending.resolve(header);
}, { urls: platformSessionRequestPatterns }, ["requestHeaders", "extraHeaders"]);

chrome.webRequest.onSendHeaders.addListener((details) => {
  if (details.method !== "POST" || details.initiator !== "https://mp.toutiao.com") return;
  const url = new URL(details.url);
  if (!["/spice/image", "/mp/agw/article/publish"].includes(url.pathname)) return;
  for (const header of details.requestHeaders ?? []) {
    const name = String(header.name || "").toLowerCase();
    if ((name === "x-secsdk-csrf-token" || name === "tt-anti-token") &&
        typeof header.value === "string" && header.value.length > 0 && header.value.length <= 4096 &&
        !/[\r\n]/.test(header.value)) {
      toutiaoEditorHeaders[name] = header.value;
      toutiaoEditorHeadersCapturedAt = Date.now();
    }
  }
}, { urls: ["https://mp.toutiao.com/spice/image*", "https://mp.toutiao.com/mp/agw/article/publish*"] }, ["requestHeaders", "extraHeaders"]);

function errorMessage(error) {
  return error?.message || String(error);
}

async function writeBridgeLog(level, message, fields = {}) {
  try {
    await fetchJSON("/v1/logs", jsonOptions("POST", {
      level: String(level || "info"),
      message: String(message || ""),
      fields,
    }));
  } catch (error) {
    console.warn("[BlogCTL][log] failed to write Bridge log", errorMessage(error));
  }
}

function readPath(value, path) {
  return String(path || "").split(".").filter(Boolean).reduce((current, key) => current?.[key], value);
}

async function fetchWithTimeout(url, options = {}) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), AUTH_TIMEOUT_MS);
  try {
    return await fetch(url, { credentials: "include", redirect: "follow", ...options, signal: controller.signal });
  } finally {
    clearTimeout(timer);
  }
}

function requestNativeBridge() {
  return new Promise((resolve, reject) => {
    chrome.runtime.sendNativeMessage(NATIVE_HOST, { command: "ensure_bridge" }, (response) => {
      const error = chrome.runtime.lastError;
      if (error) {
        reject(new Error(`无法连接 BlogCTL Native Host：${error.message}`));
        return;
      }
      if (!response?.ok || !response.baseUrl || !response.token) {
        reject(new Error(response?.error || "BlogCTL Native Host 返回无效结果"));
        return;
      }
      resolve(response);
    });
  });
}

async function ensureBridge(force = false) {
  if (!force && bridgeSession && Date.now() - bridgeSession.checkedAt < BRIDGE_CACHE_MS) return bridgeSession;
  const response = await requestNativeBridge();
  bridgeSession = { ...response, checkedAt: Date.now() };
  return bridgeSession;
}

function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

async function restartBridge() {
  const current = await ensureBridge(false);
  const previousPID = Number(current?.pid || 0);
  await fetchJSON("/v1/restart", { method: "POST" }, false);
  bridgeSession = null;
  await delay(300);

  const deadline = Date.now() + 10000;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const response = await requestNativeBridge();
      if (!previousPID || Number(response?.pid || 0) !== previousPID) {
        bridgeSession = { ...response, checkedAt: Date.now() };
        return bridgeSession;
      }
    } catch (error) {
      lastError = error;
    }
    await delay(250);
  }
  throw new Error(lastError?.message || "Bridge 重启超时");
}

async function probeCookies(probe) {
  const cookies = await chrome.cookies.getAll({ url: probe.cookieUrl });
  const byName = new Map(cookies.map((cookie) => [cookie.name, cookie]));
  if (probe.requireAny) return cookies.some((cookie) => Boolean(cookie.name && cookie.value));
  return (probe.requiredCookieNames ?? []).every((name) => Boolean(byName.get(name)?.value));
}

async function probeJSON(probe) {
  const response = await fetchWithTimeout(probe.url, { headers: probe.headers ?? {} });
  if (response.status === 401 || response.status === 403) return false;
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  const payload = await response.json();
  const value = readPath(payload, probe.path);
  return Object.prototype.hasOwnProperty.call(probe, "equals") ? value === probe.equals : Boolean(value);
}

async function probeHTML(probe) {
  const response = await fetchWithTimeout(probe.url, { headers: probe.headers ?? {} });
  if (response.status === 401 || response.status === 403) return false;
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return new RegExp(probe.match, "i").test(await response.text());
}

async function probeFinalURL(probe) {
  const response = await fetchWithTimeout(probe.url, { headers: probe.headers ?? {} });
  if (response.status === 401 || response.status === 403) return false;
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  const pathname = new URL(response.url).pathname;
  return !(probe.loggedOutPathPatterns ?? []).some((pattern) => pathname.startsWith(pattern));
}

async function platformLoginStatus(definition) {
  const probe = definition.probe ?? {};
  try {
    let loggedIn;
    switch (probe.kind) {
      case "cookies": loggedIn = await probeCookies(probe); break;
      case "json": loggedIn = await probeJSON(probe); break;
      case "html": loggedIn = await probeHTML(probe); break;
      case "final-url": loggedIn = await probeFinalURL(probe); break;
      default: throw new Error(`Unsupported auth probe: ${probe.kind || "missing"}`);
    }
    if (loggedIn) {
      return { id: definition.id, label: definition.label, known: true, loggedIn: true, inferred: false };
    }
    if (definition.id !== "cnblogs" && await platformHasSessionCookies(definition.id)) {
      return {
        id: definition.id, label: definition.label, known: false, loggedIn: false, inferred: true,
        warning: "Browser cookies exist, but the login probe did not verify the session.",
      };
    }
    return { id: definition.id, label: definition.label, known: true, loggedIn: false, inferred: false };
  } catch (error) {
    if (definition.id !== "cnblogs" && await platformHasSessionCookies(definition.id)) {
      return {
        id: definition.id, label: definition.label, known: false, loggedIn: false, inferred: true,
        warning: `Login probe failed: ${errorMessage(error)}`,
      };
    }
    return { id: definition.id, label: definition.label, known: false, loggedIn: false, error: errorMessage(error) };
  }
}

async function allPlatformLoginStatuses() {
  return Promise.all(PLATFORM_AUTH.map((definition) => platformLoginStatus(definition)));
}

async function fetchJSON(pathname, options = {}, retry = true) {
  const bridge = await ensureBridge(false);
  try {
    const headers = new Headers(options.headers ?? {});
    headers.set("x-thinkerqaq-token", bridge.token);
    const response = await fetch(`${bridge.baseUrl}${pathname}`, { ...options, headers });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw toError(payload, response.status);
    return payload;
  } catch (error) {
    if (retry) {
      bridgeSession = null;
      await ensureBridge(true);
      return fetchJSON(pathname, options, false);
    }
    throw error;
  }
}

function jsonOptions(method, body) {
  return { method, headers: { "content-type": "application/json" }, body: JSON.stringify(body) };
}

async function bridgeStatus() {
  const extensionVersion = chrome.runtime.getManifest().version;
  try {
    const bridge = await ensureBridge(false);
    const health = await fetchJSON("/v1/health");
    if (!health?.ok) throw new Error("Bridge health check failed.");
    const nativeHostVersion = String(bridge.nativeHostVersion || "");
    const bridgeVersion = String(health.version || "");
    const compatible = nativeHostVersion === extensionVersion && bridgeVersion === extensionVersion;
    const runtime = {
      extensionVersion,
      nativeHostVersion,
      bridgeVersion,
      nativeHostExecutable: String(bridge.executablePath || ""),
      compatible,
    };
    try {
      const result = await fetchJSON("/v1/config");
      return {
        running: true, pid: bridge.pid || 0, configKnown: true, config: result?.config ?? {},
        networkMode: result?.networkMode || "", ...runtime,
      };
    } catch (error) {
      return {
        running: true, pid: bridge.pid || 0, configKnown: false, config: {},
        configError: errorMessage(error), ...runtime,
      };
    }
  } catch (error) {
    bridgeSession = null;
    return {
      running: false, configKnown: false, config: {}, error: errorMessage(error),
      extensionVersion, nativeHostVersion: "", bridgeVersion: "", nativeHostExecutable: "", compatible: false,
    };
  }
}

function runtimeVersionHealth(version, expectedVersion, healthySummary, detail = "", path = "") {
  const normalized = String(version || "").trim();
  if (!normalized) {
    return {
      ok: false, status: "error", summary: "版本未知", version: "",
      detail: ["当前组件未提供版本握手；需要更新本机 BlogCTL runtime。", detail].filter(Boolean).join(" · "),
      path,
    };
  }
  if (normalized !== expectedVersion) {
    return {
      ok: false, status: "error", summary: "版本不一致", version: normalized,
      detail: [`期望 v${expectedVersion}`, detail].filter(Boolean).join(" · "),
      path,
    };
  }
  return { ok: true, status: "ok", summary: healthySummary, version: normalized, detail, path };
}

async function environmentTools(serverTools = []) {
  const bridge = await bridgeStatus();
  const extensionVersion = bridge.extensionVersion || chrome.runtime.getManifest().version;
  const bridgeVersion = String(bridge.bridgeVersion || "").trim();
  const extensionBridgeCompatible = Boolean(bridge.running && bridgeVersion && bridgeVersion === extensionVersion);

  const extensionTool = {
    name: "extension",
    displayName: "BlogCTL Extension",
    kind: "runtime",
    description: "浏览器侧控制面。版本必须与 BlogCTL Bridge 一致。",
    required: true,
    health: !bridge.running
      ? {
          ok: false,
          status: "error",
          summary: "Bridge 未连接",
          version: extensionVersion,
          detail: bridge.error || "无法确认 Extension 与 Bridge 的版本兼容性",
        }
      : extensionBridgeCompatible
        ? {
            ok: true,
            status: "ok",
            summary: "已加载",
            version: extensionVersion,
            detail: `Extension v${extensionVersion} · Bridge v${bridgeVersion}`,
          }
        : {
            ok: false,
            status: "error",
            summary: "版本不一致",
            version: extensionVersion,
            detail: `Extension v${extensionVersion} · Bridge v${bridgeVersion || "未知"}`,
          },
    config: { scope: "extension", values: {}, defaultExpanded: false },
  };

  const tools = serverTools.map((tool) => {
    if (tool?.name !== "bridge") return tool;
    const current = tool.health ?? {};
    const versionHealth = !bridge.running
      ? {
          ok: false,
          status: "error",
          summary: "未运行",
          version: bridgeVersion || current.version || "",
          detail: bridge.error || current.detail || "Bridge unavailable",
        }
      : extensionBridgeCompatible
        ? {
            ok: true,
            status: "ok",
            summary: "运行中",
            version: bridgeVersion,
            detail: current.detail || `Extension v${extensionVersion} · Bridge v${bridgeVersion}`,
          }
        : {
            ok: false,
            status: "error",
            summary: "版本不一致",
            version: bridgeVersion || current.version || "",
            detail: `Extension v${extensionVersion} · Bridge v${bridgeVersion || "未知"}`,
          };
    return { ...tool, health: { ...current, ...versionHealth } };
  });

  return [extensionTool, ...tools];
}

async function platformSessionStatus(platform, bridge) {
  if (!bridge.running) return { known: false, synced: false, unavailable: true, expiresInSeconds: 0, error: bridge.error || "BlogCTL Bridge 未运行" };
  try {
    const session = await fetchJSON(`/v1/sessions/${encodeURIComponent(platform)}/status`);
    return { known: true, synced: Boolean(session?.authenticated), expiresInSeconds: Number(session?.expiresInSeconds || 0) };
  } catch (error) {
    return { known: false, synced: false, unavailable: false, expiresInSeconds: 0, error: errorMessage(error) };
  }
}

async function getStatus() {
  const [bridge, platforms] = await Promise.all([bridgeStatus(), allPlatformLoginStatuses()]);
  let publishingPlatforms = [];
  if (bridge.running) {
    try {
      const publishing = await fetchJSON("/v1/publishing");
      publishingPlatforms = publishing?.platforms ?? [];
    } catch (error) {
      console.warn("BlogCTL platform capabilities unavailable:", errorMessage(error));
    }
  }
  const publishingByID = new Map(publishingPlatforms.map((item) => [item.id, item]));
  const enrichedPlatforms = platforms.map((platform) => ({
    ...platform,
    capabilities: publishingByID.get(platform.id)?.capabilities ?? {},
  }));
  const sessionEntries = await Promise.all(
    Object.keys(PLATFORM_SESSIONS).map(async (platform) => [
      platform,
      await platformSessionStatus(platform, bridge),
    ]),
  );
  return { bridge, platforms: enrichedPlatforms, sessions: Object.fromEntries(sessionEntries) };
}

async function collectPlatformCookieBatches(definition, diagnostics) {
  return collectBrowserSessionCookieBatches(definition, (filter) => chrome.cookies.getAll(filter), (filter, cookies) => {
    if (!diagnostics) return;
    diagnostics.push(cookieQueryDiagnostic(filter, cookies));
  });
}

async function cnBlogsCookieStores() {
  try {
    const stores = await chrome.cookies.getAllCookieStores();
    const tabs = await chrome.tabs.query({ url: ["https://*.cnblogs.com/*", "https://cnblogs.com/*"] });
    const cnBlogsTabIds = new Set(tabs.map((tab) => tab.id));
    return stores.map((store) => ({
      storeId: store.id,
      cnBlogsTabCount: store.tabIds.filter((id) => cnBlogsTabIds.has(id)).length,
    }));
  } catch (error) {
    return [{ error: errorMessage(error) }];
  }
}

async function captureCNBlogsRequestCookieHeader() {
  const url = `https://i.cnblogs.com/api/user?blogctl_cookie_probe=${crypto.randomUUID()}`;
  let resolveCapture;
  const captured = new Promise((resolve) => { resolveCapture = resolve; });
  pendingCNBlogsCookieCaptures.set(url, { url, resolve: resolveCapture });
  try {
    const response = await fetchWithTimeout(url, { cache: "no-store" });
    const header = await Promise.race([captured, delay(1500).then(() => "")]);
    if (!response.ok) throw new Error(`CNBlogs browser auth HTTP ${response.status}`);
    const payload = await response.json();
    if (!payload?.loginName) throw new Error("CNBlogs browser auth has no loginName");
    if (!header) throw new Error("CNBlogs browser request Cookie header was not captured");
    return header;
  } finally {
    pendingCNBlogsCookieCaptures.delete(url);
  }
}

async function capturePlatformRequestCookieHeader(platform) {
  const definition = PLATFORM_SESSIONS[platform];
  const rawURL = String(definition?.sessionProbeUrl || "").trim();
  if (!rawURL) return "";

  const url = new URL(rawURL);
  url.searchParams.set("blogctl_cookie_probe", crypto.randomUUID());
  const expectedURL = url.toString();
  let resolveCapture;
  const captured = new Promise((resolve) => { resolveCapture = resolve; });
  pendingPlatformCookieCaptures.set(expectedURL, { url: expectedURL, resolve: resolveCapture });
  try {
    void fetchWithTimeout(expectedURL, { cache: "no-store" }).catch(() => null);
    const header = await Promise.race([
      captured,
      delay(1500).then(() => ""),
    ]);
    return String(header || "");
  } finally {
    pendingPlatformCookieCaptures.delete(expectedURL);
  }
}

async function captureRequestCookieHeaderForURL(rawURL) {
  const url = new URL(rawURL);
  url.searchParams.set("blogctl_cookie_probe", crypto.randomUUID());
  const expectedURL = url.toString();
  let resolveCapture;
  const captured = new Promise((resolve) => { resolveCapture = resolve; });
  pendingPlatformCookieCaptures.set(expectedURL, { url: expectedURL, resolve: resolveCapture });
  try {
    void fetchWithTimeout(expectedURL, { cache: "no-store" }).catch(() => null);
    const header = await Promise.race([
      captured,
      delay(1500).then(() => ""),
    ]);
    return String(header || "");
  } finally {
    pendingPlatformCookieCaptures.delete(expectedURL);
  }
}

async function selectedPlatformCookies(platform, diagnostics) {
  const definition = PLATFORM_SESSIONS[platform];
  if (!definition) throw new Error(`${platform}: browser session sync is not supported.`);
  const batches = await collectPlatformCookieBatches(definition, diagnostics);
  return selectBrowserSessionCookies(definition, batches);
}

function cookieQuerySummary(diagnostics = []) {
  return diagnostics
    .filter((item) => item && item.target)
    .map((item) => {
      const names = Array.isArray(item.names) && item.names.length ? item.names.join(",") : "none";
      return `${item.target}[${names}]`;
    })
    .join("; ");
}

async function platformHasSessionCookies(platform) {
  if (!PLATFORM_SESSIONS[platform]) return false;
  try {
    return (await selectedPlatformCookies(platform)).length > 0;
  } catch {
    return false;
  }
}

async function syncPlatformSession(platform) {
  const definition = PLATFORM_SESSIONS[platform];
  if (!definition) throw new Error(`${platform}: browser session sync is not supported.`);

  let selected;
  const cookieQueries = [];
  const cookieStores = platform === "cnblogs" ? await cnBlogsCookieStores() : [];
  const requestCookieHeaders = {};
  let requestCookieHeader = platform === "cnblogs"
    ? await captureCNBlogsRequestCookieHeader()
    : await capturePlatformRequestCookieHeader(platform);
  if (platform === "cnblogs") {
    const uploadCookieHeader = await captureRequestCookieHeaderForURL("https://upload.cnblogs.com/v2/images/cors-upload");
    if (uploadCookieHeader) requestCookieHeaders["upload.cnblogs.com"] = uploadCookieHeader;
  }
  try {
    selected = await selectedPlatformCookies(platform, cookieQueries);
  } catch (error) {
    if (requestCookieHeader) {
      selected = [];
    } else {
      const summary = cookieQuerySummary(cookieQueries);
      const detail = summary ? ` Cookie keys seen: ${summary}.` : "";
      throw new Error(`${platform}: ${errorMessage(error)}.${detail} Sign in first.`);
    }
  }

  return fetchJSON(
    `/v1/sessions/${encodeURIComponent(platform)}`,
    jsonOptions("POST", {
      cookies: selected,
      userAgent: navigator.userAgent,
      cookieQueries,
      cookieStores,
      requestCookieHeader,
      requestCookieHeaders,
      requestHeaders: platform === "toutiao" && Date.now() - toutiaoEditorHeadersCapturedAt < 10 * 60 * 1000
        ? { ...toutiaoEditorHeaders } : undefined,
    }),
  );
}

async function syncSessionsForPlatforms(platforms = []) {
  const unique = [...new Set(platforms.map((platform) => String(platform || "").trim()).filter(Boolean))];
  for (const platform of unique) {
    const definition = PLATFORM_SESSIONS[platform];
    if (!definition) continue;
    try {
      await syncPlatformSession(platform);
    } catch (error) {
      if (definition.optional === true) {
        console.warn(`${platform}: optional browser session unavailable; platform-native image upload may fall back.`, errorMessage(error));
        continue;
      }
      throw error;
    }
  }
}

async function waitForTabLoaded(tabId, timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const tab = await chrome.tabs.get(tabId);
    if (tab?.status === "complete") return tab;
    await delay(150);
  }
  throw new Error("browser publish page load timed out");
}

async function refreshGoogleSearchConfiguration() {
  const response = await fetchJSON("/v1/config");
  const site = response?.config?.search;
  if (!site?.origin || !site?.googleProperty) throw new Error("索引站点配置不可用，请检查设置中的索引站点与 URL。");
  googleSearchConsoleProperty = site.googleProperty;
  googleIndexOrigin = site.origin;
  googleSearchConsoleURL = `https://search.google.com/search-console?resource_id=${encodeURIComponent(site.googleProperty)}`;
}

function googleSearchConsoleTabCandidate(tab) {
  try {
    const parsed = new URL(tab?.url || "");
    if (parsed.hostname !== "search.google.com" || !/^(?:\/u\/\d+)?\/search-console(?:\/|$)/u.test(parsed.pathname)) {
      return { matches: false, property: false };
    }
    const resourceID = String(parsed.searchParams.get("resource_id") || "").trim();
    return {
      matches: true,
      property: resourceID === googleSearchConsoleProperty,
    };
  } catch {
    return { matches: false, property: false };
  }
}

async function googleSearchConsoleTab({ active = false, reset = false } = {}) {
  await refreshGoogleSearchConfiguration();
  let tab = null;
  if (googleSearchConsoleTabId !== null) {
    try {
      const current = await chrome.tabs.get(googleSearchConsoleTabId);
      if (googleSearchConsoleTabCandidate(current).property) tab = current;
      else googleSearchConsoleTabId = null;
    } catch {
      googleSearchConsoleTabId = null;
    }
  }
  if (!tab) {
    const tabs = await chrome.tabs.query({});
    tab = tabs.find((candidate) => googleSearchConsoleTabCandidate(candidate).property) || null;
  }
  if (!tab) {
    tab = await chrome.tabs.create({ url: googleSearchConsoleURL, active });
    googleSearchConsoleTabId = tab.id;
    await waitForTabLoaded(tab.id, 30000);
  } else {
    googleSearchConsoleTabId = tab.id;
    if (active && !tab.active) await chrome.tabs.update(tab.id, { active: true });
    if (reset) {
      tab = await chrome.tabs.update(tab.id, { url: googleSearchConsoleURL, active });
      await waitForTabLoaded(tab.id, 30000);
    } else if (tab.status !== "complete") {
      await waitForTabLoaded(tab.id, 30000);
    }
  }

  const current = await chrome.tabs.get(googleSearchConsoleTabId);
  let hostname = "";
  try { hostname = new URL(current.url || "").hostname; } catch {}
  if (hostname === "accounts.google.com") {
    const error = new Error("Google Search Console 尚未登录");
    error.code = "google_search_console_not_logged_in";
    throw error;
  }
  if (hostname !== "search.google.com") {
    const error = new Error("Google Search Console 页面未就绪");
    error.code = "google_search_console_not_ready";
    throw error;
  }
  await chrome.scripting.executeScript({
    target: { tabId: googleSearchConsoleTabId },
    files: ["google-indexing-content.js"],
  });
  return chrome.tabs.get(googleSearchConsoleTabId);
}

function updateGoogleSearchConsoleStatus(status = {}) {
  googleSearchConsoleStatus = {
    known: true,
    loggedIn: Boolean(status.loggedIn),
    ready: Boolean(status.ready ?? status.loggedIn),
    tabId: status.tabId ?? googleSearchConsoleTabId,
    error: String(status.error || ""),
    code: String(status.code || ""),
  };
  chrome.runtime.sendMessage({
    type: "blogctl.index.gsc",
    google: googleSearchConsoleStatus,
  }).catch(() => {});
  return googleSearchConsoleStatus;
}

async function googleProbeTab(tab) {
  const result = await chrome.tabs.sendMessage(tab.id, { type: "blogctl.google.index.probe" });
  const ready = Boolean(result?.ok && (result?.ready || result?.inspectionInput || result?.hasRequestButton || result?.inspectionState));
  await writeBridgeLog(
    ready ? "info" : "warn",
    "gsc probe result",
    {
      ready,
      inspectionInput: Boolean(result?.inspectionInput),
      inspectionState: String(result?.inspectionState || ""),
      hasInspectionTrigger: Boolean(result?.hasInspectionTrigger),
      hasRequestButton: Boolean(result?.hasRequestButton),
      pathname: result?.pathname || "",
      title: result?.title || "",
      controls: Number(result?.controls || 0),
    },
  );
  return { ...result, ready };
}

async function ensureGoogleSearchConsoleReady({ active = false, allowReset = true } = {}) {
  let tab;
  try {
    tab = await googleSearchConsoleTab({ active, reset: false });
    let probe = await googleProbeTab(tab);
    if (!probe?.ready && allowReset) {
      await writeBridgeLog("warn", "gsc surface not ready; resetting once", {
        pathname: probe?.pathname || "",
        title: probe?.title || "",
      });
      tab = await googleSearchConsoleTab({ active, reset: true });
      probe = await googleProbeTab(tab);
    }
    if (!probe?.ready) {
      const error = new Error("Google Search Console 页面已打开，但 URL Inspection 界面仍未就绪");
      error.code = "google_search_console_not_ready";
      throw error;
    }
    const status = updateGoogleSearchConsoleStatus({
      loggedIn: true,
      ready: true,
      tabId: tab.id,
    });
    return { tab, probe, status };
  } catch (error) {
    const status = updateGoogleSearchConsoleStatus({
      loggedIn: false,
      ready: false,
      tabId: googleSearchConsoleTabId,
      error: errorMessage(error),
      code: error?.code || "google_search_console_not_ready",
    });
    error.googleStatus = status;
    throw error;
  }
}

async function googleSearchConsoleProbe({ active = false } = {}) {
  try {
    const prepared = await ensureGoogleSearchConsoleReady({ active, allowReset: false });
    return prepared.status;
  } catch (error) {
    return error.googleStatus || googleSearchConsoleStatus;
  }
}

async function googleRequestIndexingURL(url) {
  try {
    await refreshGoogleSearchConfiguration();
    const target = new URL(url);
    if (target.origin !== googleIndexOrigin || target.username || target.password) throw new Error("URL does not belong to the configured Search Console site");
  } catch (error) {
    return { ok: false, action: "failed", stage: "url_validation", url, error: errorMessage(error) };
  }
  const maxPrepareAttempts = 3;
  for (let attempt = 1; attempt <= maxPrepareAttempts; attempt += 1) {
    let tab;
    try {
      // Reuse an existing inspection result when possible. The GSC result
      // page may not expose a textbox until its "Inspect URL" surface is
      // activated. After a transient UI miss, force one clean reset before
      // retrying the same URL; this is still before Request Indexing is
      // clicked, so it does not consume duplicate request quota.
      const prepared = await ensureGoogleSearchConsoleReady({
        active: true,
        allowReset: true,
      });
      tab = prepared.tab;
    } catch (error) {
      await writeBridgeLog(attempt < maxPrepareAttempts ? "warn" : "error", "gsc page preparation failed", {
        url,
        attempt,
        code: error?.code || "",
        error: errorMessage(error),
      });
      if (error?.code === "google_search_console_not_logged_in") {
        return { ok: false, action: "not_logged_in", stage: "page_prepare", url, error: errorMessage(error) };
      }
      if (attempt < maxPrepareAttempts) {
        try {
          await googleSearchConsoleTab({ active: true, reset: true });
        } catch {}
        await delay(500);
        continue;
      }
      return { ok: false, action: "ui_changed", stage: "page_prepare", url, error: errorMessage(error) };
    }

    try {
      await writeBridgeLog("info", "gsc request workflow started", { url, attempt });
      const result = await chrome.tabs.sendMessage(tab.id, {
        type: "blogctl.google.index.request",
        url,
      });
      if (!result || typeof result !== "object") {
        await writeBridgeLog("error", "gsc request workflow returned no result", { url, attempt });
        return { ok: false, action: "failed", stage: "workflow_result", url, error: "Google Search Console returned no result" };
      }

      const action = String(result.action || "");
      const stage = String(result.stage || "");
      const safeToRetry =
        (action === "ui_changed" && [
          "inspection_control",
          "request_button",
          "request_dialog",
          "request_processing_timeout",
        ].includes(stage)) ||
        (action === "failed" && stage === "inspection_result");
      await writeBridgeLog(
        result.ok ? "info" : safeToRetry && attempt < maxPrepareAttempts ? "warn" : "error",
        "gsc request workflow finished",
        {
          url,
          attempt,
          action,
          stage,
          error: String(result.error || ""),
        },
      );

      if (safeToRetry && attempt < maxPrepareAttempts) {
        await writeBridgeLog("warn", "gsc transient ui miss; retrying same url", {
          url,
          attempt,
          stage,
        });
        try {
          await googleSearchConsoleTab({ active: true, reset: true });
        } catch {}
        await delay(500);
        continue;
      }

      if (action === "requested_indexing") {
        const cleanup = result.cleanup || {};
        if (cleanup.dialogPresent || cleanup.closed === false) {
          await writeBridgeLog("warn", "gsc request succeeded; forcing clean page for next url", {
            url,
            attempt,
            dialogPresent: Boolean(cleanup.dialogPresent),
            closed: cleanup.closed !== false,
          });
          try {
            await googleSearchConsoleTab({ active: true, reset: true });
          } catch (cleanupError) {
            // The indexing request already succeeded. Never downgrade it to a
            // failed queue item just because page cleanup failed; the next URL
            // will run its own recovery path.
            await writeBridgeLog("warn", "gsc post-request cleanup reset failed", {
              url,
              error: errorMessage(cleanupError),
            });
          }
        }
      }
      return { ...result, url };
    } catch (error) {
      await writeBridgeLog("error", "gsc request workflow exception", {
        url,
        attempt,
        error: errorMessage(error),
      });
      return { ok: false, action: "failed", stage: "workflow_exception", url, error: errorMessage(error) };
    }
  }

  return { ok: false, action: "ui_changed", stage: "page_prepare", url, error: "Google Search Console did not become ready" };
}

function broadcastIndexProgress(index) {
  chrome.runtime.sendMessage({
    type: "blogctl.index.progress",
    index,
    google: googleSearchConsoleStatus,
  }).catch(() => {});
}

async function googleIndexQueueSnapshot() {
  const result = await fetchJSON("/v1/search/index");
  return result?.index ?? {};
}

async function pumpGoogleIndexQueue() {
  while (true) {
    const index = await googleIndexQueueSnapshot();
    const queue = index?.google?.requestQueue ?? {};
    if (queue.state !== "running") {
      broadcastIndexProgress(index);
      return index;
    }
    const items = Array.isArray(queue.items) ? queue.items : [];
    const currentIndex = Number(queue.currentIndex || 0);
    const item = items[currentIndex];
    if (!item) {
      broadcastIndexProgress(index);
      return index;
    }
    if (!["queued", "failed"].includes(String(item.status || "queued"))) {
      await fetchJSON("/v1/search/index/google/request-queue/result", jsonOptions("POST", {
        url: item.url,
        result: item.status === "requested" ? "requested_indexing" : item.status === "indexed" ? "already_indexed" : "failed",
        error: "",
      }));
      continue;
    }

    const startedAt = Date.now();
    const processing = await fetchJSON("/v1/search/index/google/request-queue/result", jsonOptions("POST", {
      url: item.url,
      result: "processing",
      error: "",
    }));
    broadcastIndexProgress(processing?.index ?? index);
    await writeBridgeLog("info", "gsc queue item started", {
      jobId: String(queue.jobId || ""),
      url: item.url,
      current: currentIndex + 1,
      total: items.length,
    });
    const result = await googleRequestIndexingURL(item.url);
    const action = String(result?.action || "failed");
    const stage = String(result?.stage || "").trim();
    const errorText = String(result?.error || "").trim();
    const updated = await fetchJSON("/v1/search/index/google/request-queue/result", jsonOptions("POST", {
      url: item.url,
      result: action,
      error: stage && errorText ? `[${stage}] ${errorText}` : errorText,
    }));
    broadcastIndexProgress(updated?.index ?? {});
    await writeBridgeLog(result?.ok ? "info" : "warn", "gsc queue item finished", {
      jobId: String(queue.jobId || ""),
      url: item.url,
      current: currentIndex + 1,
      total: items.length,
      action,
      stage,
      durationMs: Date.now() - startedAt,
      error: errorText,
    });

    if (["quota_blocked", "rate_limited", "not_logged_in", "ui_changed", "timeout"].includes(action)) {
      return updated?.index ?? {};
    }
    await delay(1200);
  }
}

function kickGoogleIndexQueuePump() {
  if (googleIndexPumpPromise) return googleIndexPumpPromise;
  googleIndexPumpPromise = pumpGoogleIndexQueue()
    .catch(async (error) => {
      console.warn("[BlogCTL][google-index] queue pump failed", errorMessage(error));
      await writeBridgeLog("error", "gsc queue pump failed", { error: errorMessage(error) });
      try {
        const paused = await fetchJSON("/v1/search/index/google/request-queue/pause", { method: "POST" });
        broadcastIndexProgress(paused?.index ?? {});
      } catch (pauseError) {
        await writeBridgeLog("error", "gsc queue pump could not pause queue", {
          error: errorMessage(pauseError),
        });
      }
      return null;
    })
    .finally(() => {
      googleIndexPumpPromise = null;
    });
  return googleIndexPumpPromise;
}

async function handleMessage(message) {
  switch (message.type) {
    case "blogctl.status": return { ok: true, status: await getStatus() };
    case "blogctl.social.channels":
      return fetchJSON("/v1/social/channels");
    case "blogctl.social.list":
      return fetchJSON("/v1/social/drafts");
    case "blogctl.social.get": {
      const id = encodeURIComponent(String(message.id || ""));
      return fetchJSON(`/v1/social/drafts/${id}`);
    }
    case "blogctl.social.save":
    case "blogctl.social.stage":
    case "blogctl.social.reconcile":
    case "blogctl.social.publish": {
      const id = encodeURIComponent(String(message.id || ""));
      const action = message.type.split(".")[2];
      const url = action === "save"
        ? `/v1/social/drafts/${id}`
        : `/v1/social/drafts/${id}/${action}`;
      return fetchJSON(url, jsonOptions(action === "save" ? "PUT" : "POST", {
        revision: message.revision, platform: message.platform, posts: message.posts,
      }));
    }
    case "blogctl.articles": {
      const result = await fetchJSON("/v1/articles");
      return { ok: true, articles: result?.articles ?? [] };
    }
    case "blogctl.publications": {
      const result = await fetchJSON("/v1/publications");
      return { ok: true, records: result?.records ?? [] };
    }
    case "blogctl.publication.pending.resolve": {
      const article = String(message.article || "").trim();
      const platform = String(message.platform || "").trim();
      if (!article || !platform) throw new Error("article and platform are required");
      const query = new URLSearchParams({ article, platform });
      const result = await fetchJSON(`/v1/publications/pending/resolve?${query.toString()}`, jsonOptions("POST", {
        fields: Array.isArray(message.fields) ? message.fields : [],
      }));
      return { ok: true, pendingFields: result?.pendingFields ?? [] };
    }
    case "blogctl.remote.inventory": {
      const platform = String(message.platform || "").trim();
      if (!["cnblogs", "juejin", "csdn", "segmentfault", "zhihu", "51cto", "oschina", "toutiao", "devto", "hashnode", "medium"].includes(platform)) {
        throw new Error("不支持的平台");
      }
      if (platform !== "devto") await syncPlatformSession(platform);
      const query = new URLSearchParams({ platform });
      const result = await fetchJSON(`/v1/remote/inventory?${query.toString()}`, { method: "POST" });
      // Runtime message replies must carry ok=true. The Bridge response is
      // an inventory payload, not the popup's message-envelope contract.
      return { ok: true, ...result };
    }
    case "blogctl.article.match": {
      const article = encodeURIComponent(String(message.article || ""));
      const platform = String(message.platform || "");
      if (!article || !platform) throw new Error("article and platform are required");
      if (platform === "hashnode") {
        await syncPlatformSession("hashnode");
        const result = await fetchJSON(`/v1/hashnode/articles/list?article=${article}`, { method: "POST" });
        const candidates = result.candidates ?? [];
        return { ok: true, match: {
          text: candidates.length
            ? `已从 Hashnode 草稿与已发布文章列表匹配到 ${candidates.length} 篇文章。`
            : "已检查 Hashnode 草稿及已发布文章，没有匹配的英文标题。",
          items: candidates.map((post) => ({
            title: post.title, id: post.id, published: post.published,
            url: post.url || "", bound: Boolean(post.bound),
            bindingState: post.bindingState || "",
          })),
        } };
      }
      if (platform === "cnblogs") {
        await syncPlatformSession("cnblogs");
        const result = await fetchJSON(`/v1/cnblogs/binding/search?article=${article}`, { method: "POST" });
        const candidates = result.candidates ?? [];
        return { ok: true, match: {
          text: `远端找到 ${candidates.length} 篇候选文章，请明确勾选更新目标。`,
          items: candidates.map((post) => ({
            title: String(post.title || "").replace(/<\/?strong>/gi, ""),
            id: post.id,
            published: post.published,
            url: post.published ? (post.url || "") :
              `https://i.cnblogs.com/posts/edit;postId=${encodeURIComponent(post.id)}`,
            updatedAt: String(post.updatedAt || ""),
          })),
        } };
      }
      if (platform === "segmentfault") {
        await syncPlatformSession("segmentfault");
        const result = await fetchJSON(`/v1/segmentfault/articles/list?article=${article}`, { method: "POST" });
        const candidates = result.candidates ?? [];
        return { ok: true, match: {
          text: candidates.length
            ? `从草稿列表和文章列表本地匹配到 ${candidates.length} 条候选。`
            : "已读取思否草稿列表和文章列表，本地未匹配到同名文章。",
          items: candidates.map((post) => ({
            title: post.title,
            id: post.id,
            published: post.published,
            url: post.url || "",
            bound: Boolean(post.bound),
            bindingState: post.bindingState || "",
          })),
          bindings: result.bindings ?? [],
        } };
      }
      if (platform === "zhihu") {
        await syncPlatformSession("zhihu");
        const result = await fetchJSON(`/v1/zhihu/articles/list?article=${article}`, { method: "POST" });
        const candidates = result.candidates ?? [];
        return { ok: true, match: {
          text: candidates.length
            ? `从知乎草稿列表和已发布文章列表本地匹配到 ${candidates.length} 条候选。`
            : "已读取知乎草稿列表和已发布文章列表，本地未匹配到同名文章。",
          items: candidates.map((post) => ({
            title: post.title,
            id: post.id,
            published: post.published,
            url: post.url || "",
            bound: Boolean(post.bound),
            bindingState: post.bindingState || "",
          })),
          bindings: result.bindings ?? [],
        } };
      }
      if (platform === "toutiao") {
        await syncPlatformSession("toutiao");
        const result = await fetchJSON(`/v1/toutiao/articles/list?article=${article}`, { method: "POST" });
        const candidates = result.candidates ?? [];
        return { ok: true, match: {
          text: candidates.length
            ? `从头条草稿列表和已发布文章列表本地匹配到 ${candidates.length} 条候选。`
            : "已读取头条草稿列表和已发布文章列表，本地未匹配到同名文章。",
          items: candidates.map((post) => ({
            title: post.title,
            id: post.id,
            published: post.published,
            updatedAt: post.updatedAt || "",
            url: post.url || "",
            bound: Boolean(post.bound),
            bindingState: post.bindingState || "",
          })),
          bindings: result.bindings ?? [],
        } };
      }
      if (platform === "oschina") {
        await syncPlatformSession("oschina");
        const result = await fetchJSON(`/v1/oschina/articles/list?article=${article}`, { method: "POST" });
        const candidates = result.candidates ?? [];
        return { ok: true, match: {
          text: candidates.length
            ? `从开源中国草稿列表和已发布文章列表本地匹配到 ${candidates.length} 条候选。`
            : "已读取开源中国草稿列表和已发布文章列表，本地未匹配到同名文章。",
          items: candidates.map((post) => ({
            title: post.title,
            id: post.id,
            published: post.published,
            url: post.url || "",
            bound: Boolean(post.bound),
            bindingState: post.bindingState || "",
          })),
          bindings: result.bindings ?? [],
        } };
      }
      if (platform === "juejin") {
        await syncPlatformSession("juejin");
        const result = await fetchJSON(`/v1/juejin/articles/list?article=${article}`, { method: "POST" });
        const candidates = result.candidates ?? [];
        return { ok: true, match: {
          text: candidates.length
            ? `从掘金草稿与已发布文章列表匹配到 ${candidates.length} 条候选。`
            : "已读取掘金草稿与已发布文章列表，未匹配到同名文章。",
          items: candidates.map((post) => ({
            title: post.title,
            id: post.id,
            published: post.published,
            url: post.url || "",
            bound: Boolean(post.bound),
            bindingState: post.bindingState || "",
          })),
          bindings: result.bindings ?? [],
        } };
      }
      if (platform === "51cto") {
        await syncPlatformSession("51cto");
        const result = await fetchJSON(`/v1/51cto/articles/list?article=${article}`, { method: "POST" });
        const candidates = result.candidates ?? [];
        return { ok: true, match: {
          text: candidates.length
            ? `从 51CTO 草稿列表和已发布文章列表本地匹配到 ${candidates.length} 条候选。`
            : "已读取 51CTO 草稿列表和已发布文章列表，本地未匹配到同名文章。",
          items: candidates.map((post) => ({
            title: post.title,
            id: post.id,
            published: post.published,
            url: post.url || "",
            bound: Boolean(post.bound),
            bindingState: post.bindingState || "",
          })),
          bindings: result.bindings ?? [],
        } };
      }
      if (platform === "medium") {
        await syncPlatformSession("medium");
        const result = await fetchJSON(`/v1/medium/articles/list?article=${article}`, { method: "POST" });
        const candidates = result.candidates ?? [];
        return { ok: true, match: {
          text: candidates.length
            ? `从 Medium 草稿和已发布文章列表本地匹配到 ${candidates.length} 条候选。`
            : "已通过 Bridge 读取 Medium 草稿和已发布文章列表，本地未匹配到同名文章。",
          items: candidates.map((post) => ({
            title: post.title,
            id: post.id,
            published: post.published,
            url: post.url || "",
            bound: Boolean(post.bound),
            bindingState: post.bindingState || "",
          })),
          bindings: result.bindings ?? [],
        } };
      }
      if (platform === "devto") {
        const result = await fetchJSON(`/v1/devto/articles/search?article=${article}`, { method: "POST" });
        const candidates = result.candidates ?? [];
        return { ok: true, match: {
          text: candidates.length
            ? `从 DEV.to 全部文章列表本地匹配到 ${candidates.length} 条候选${result.truncated ? "；仅检索了前 500 篇" : ""}。`
            : result.truncated ? "前 500 篇中未匹配到候选，结果尚不完整。" : "已读取 DEV.to 全部文章列表，本地未匹配到对应文章。",
          items: candidates.map((post) => ({
            title: post.title,
            id: post.id,
            published: post.published,
            url: post.url || "",
            bound: Boolean(post.bound),
            bindingState: post.bindingState || "",
          })),
          bindings: result.bindings ?? [],
        } };
      }
      if (platform === "csdn") {
        await syncPlatformSession("csdn");
        const result = await fetchJSON(`/v1/csdn/articles/list?article=${article}`, { method: "POST" });
        const candidates = result.candidates ?? [];
        return { ok: true, match: {
          text: candidates.length
            ? `从 CSDN 草稿列表和已发布文章列表本地匹配到 ${candidates.length} 条候选。`
            : "已读取 CSDN 草稿列表和已发布文章列表，本地未匹配到同名文章。",
          items: candidates.map((post) => ({
            title: post.title,
            id: post.id,
            published: post.published,
            url: post.url || "",
            bound: Boolean(post.bound),
            bindingState: post.bindingState || "",
          })),
          bindings: result.bindings ?? [],
        } };
      }
      const result = await fetchJSON(`/v1/article-links?article=${article}`);
      const link = result.links?.[platform];
      const reference = link?.remoteId || link?.publishedUrl || link?.draftUrl;
      return { ok: true, match: { text: reference
        ? `本地记录：${reference} · 尚未远端验证（当前平台不支持在线查找）`
        : "当前平台尚不支持在线查找；本地没有文章关联记录。",
        items: reference ? [{ title: "本地历史记录（未远端验证）", id: link.remoteId || "未知", published: Boolean(link.publishedUrl), url: link.publishedUrl || link.draftUrl || "", localOnly: true }] : [] } };
    }
    case "blogctl.cnblogs.binding": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/cnblogs/binding?article=${article}`)) };
    }
    case "blogctl.cnblogs.bind": {
      const article = encodeURIComponent(String(message.article || ""));
      await syncPlatformSession("cnblogs");
      return { ok: true, ...(await fetchJSON(`/v1/cnblogs/binding?article=${article}`, jsonOptions("POST", { reference: message.reference ?? "", state: message.state ?? "", replace: message.replace === true }))) };
    }
    case "blogctl.cnblogs.unbind": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/cnblogs/binding?article=${article}`, jsonOptions("DELETE", { state: message.state, postId: message.postId }))) };
    }
    case "blogctl.segmentfault.bind": {
      const article = encodeURIComponent(String(message.article || ""));
      await syncPlatformSession("segmentfault");
      return { ok: true, ...(await fetchJSON(`/v1/segmentfault/binding?article=${article}`, jsonOptions("POST", {
        postId: message.postId ?? "",
        state: message.state ?? "",
        replace: message.replace === true,
      }))) };
    }
    case "blogctl.segmentfault.unbind": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/segmentfault/binding?article=${article}`, jsonOptions("DELETE", {
        state: message.state,
        postId: message.postId,
      }))) };
    }
    case "blogctl.zhihu.bind": {
      const article = encodeURIComponent(String(message.article || ""));
      await syncPlatformSession("zhihu");
      return { ok: true, ...(await fetchJSON(`/v1/zhihu/binding?article=${article}`, jsonOptions("POST", {
        postId: message.postId ?? "",
        state: message.state ?? "",
        replace: message.replace === true,
        candidate: message.candidate ?? null,
      }))) };
    }
    case "blogctl.zhihu.unbind": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/zhihu/binding?article=${article}`, jsonOptions("DELETE", {
        state: message.state,
        postId: message.postId,
      }))) };
    }
    case "blogctl.toutiao.bind": {
      const article = encodeURIComponent(String(message.article || ""));
      await syncPlatformSession("toutiao");
      return { ok: true, ...(await fetchJSON(`/v1/toutiao/binding?article=${article}`, jsonOptions("POST", {
        postId: message.postId ?? "",
        state: message.state ?? "",
        replace: message.replace === true,
      }))) };
    }
    case "blogctl.toutiao.unbind": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/toutiao/binding?article=${article}`, jsonOptions("DELETE", {
        state: message.state,
        postId: message.postId,
      }))) };
    }
    case "blogctl.oschina.bind": {
      const article = encodeURIComponent(String(message.article || ""));
      await syncPlatformSession("oschina");
      return { ok: true, ...(await fetchJSON(`/v1/oschina/binding?article=${article}`, jsonOptions("POST", {
        postId: message.postId ?? "",
        state: message.state ?? "",
        replace: message.replace === true,
      }))) };
    }
    case "blogctl.oschina.unbind": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/oschina/binding?article=${article}`, jsonOptions("DELETE", {
        state: message.state,
        postId: message.postId,
      }))) };
    }
    case "blogctl.juejin.bind": {
      const article = encodeURIComponent(String(message.article || ""));
      await syncPlatformSession("juejin");
      return { ok: true, ...(await fetchJSON(`/v1/juejin/binding?article=${article}`, jsonOptions("POST", {
        postId: message.postId ?? "",
        state: message.state ?? "",
        replace: message.replace === true,
      }))) };
    }
    case "blogctl.juejin.unbind": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/juejin/binding?article=${article}`, jsonOptions("DELETE", {
        state: message.state,
        postId: message.postId,
      }))) };
    }
    case "blogctl.51cto.bind": {
      const article = encodeURIComponent(String(message.article || ""));
      await syncPlatformSession("51cto");
      return { ok: true, ...(await fetchJSON(`/v1/51cto/binding?article=${article}`, jsonOptions("POST", {
        postId: message.postId ?? "",
        state: message.state ?? "",
        replace: message.replace === true,
      }))) };
    }
    case "blogctl.51cto.unbind": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/51cto/binding?article=${article}`, jsonOptions("DELETE", {
        state: message.state,
        postId: message.postId,
      }))) };
    }
    case "blogctl.devto.bind": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/devto/binding?article=${article}`, jsonOptions("POST", {
        postId: message.postId ?? "",
        state: message.state ?? "",
        replace: message.replace === true,
      }))) };
    }
    case "blogctl.devto.unbind": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/devto/binding?article=${article}`, jsonOptions("DELETE", {
        state: message.state,
        postId: message.postId,
      }))) };
    }
    case "blogctl.csdn.bind": {
      const article = encodeURIComponent(String(message.article || ""));
      await syncPlatformSession("csdn");
      return { ok: true, ...(await fetchJSON(`/v1/csdn/binding?article=${article}`, jsonOptions("POST", {
        postId: message.postId ?? "",
        state: message.state ?? "",
        replace: message.replace === true,
      }))) };
    }
    case "blogctl.csdn.unbind": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/csdn/binding?article=${article}`, jsonOptions("DELETE", {
        state: message.state,
        postId: message.postId,
      }))) };
    }
    case "blogctl.medium.bind": {
      const article = encodeURIComponent(String(message.article || ""));
      await syncPlatformSession("medium");
      return { ok: true, ...(await fetchJSON(`/v1/medium/binding?article=${article}`, jsonOptions("POST", {
        postId: message.postId ?? "",
        state: message.state ?? "",
        replace: message.replace === true,
        candidate: message.candidate ?? null,
        manual: message.manual === true,
      }))) };
    }
    case "blogctl.medium.unbind": {
      const article = encodeURIComponent(String(message.article || ""));
      return { ok: true, ...(await fetchJSON(`/v1/medium/binding?article=${article}`, jsonOptions("DELETE", {
        state: message.state,
        postId: message.postId,
      }))) };
    }
    case "blogctl.tools": {
      const result = await fetchJSON("/v1/tools");
      return { ok: true, tools: await environmentTools(result?.tools ?? []) };
    }
    case "blogctl.locale.get": {
      const result = await fetchJSON("/v1/ui-locale");
      return { ok: true, uiLocale: result.uiLocale };
    }
    case "blogctl.locale.set": {
      const uiLocale = String(message.uiLocale || "");
      if (!["auto", "zh-CN", "en"].includes(uiLocale)) {
        throw new Error("invalid UI locale");
      }
      const result = await fetchJSON("/v1/ui-locale", jsonOptions("PUT", { uiLocale }));
      return { ok: true, uiLocale: result.uiLocale };
    }
    case "blogctl.config.edit": {
      const result = await fetchJSON("/v1/config/edit", { method: "POST" });
      return { ok: true, path: result?.path || "" };
    }
    case "blogctl.index.get": {
      const result = await fetchJSON("/v1/search/index");
      const index = result?.index ?? {};
      if (index?.google?.requestQueue?.state === "running" && !googleIndexPumpPromise) {
        kickGoogleIndexQueuePump();
      }
      return { ok: true, index, google: googleSearchConsoleStatus };
    }
    case "blogctl.index.inventory.refresh": {
      const result = await fetchJSON("/v1/search/index/inventory/refresh", { method: "POST" });
      return { ok: true, index: result?.index ?? {} };
    }
    case "blogctl.index.indexnow.submit": {
      const mode = String(message.mode || "incremental");
      const result = await fetchJSON(
        "/v1/search/index/jobs/indexnow",
        jsonOptions("POST", { mode }),
      );
      return { ok: true, index: result?.index ?? {}, job: result?.job };
    }
    case "blogctl.index.baidu.submit": {
      const mode = String(message.mode || "incremental");
      const result = await fetchJSON(
        "/v1/search/index/jobs/baidu",
        jsonOptions("POST", { mode }),
      );
      return { ok: true, index: result?.index ?? {}, job: result?.job };
    }
    case "blogctl.index.so360.submit":
    case "blogctl.index.sogou.submit": {
      const provider = message.type === "blogctl.index.so360.submit" ? "so360" : "sogou";
      const result = await fetchJSON(`/v1/search/index/jobs/${provider}`, jsonOptions("POST", { mode: String(message.mode || "incremental") }));
      return { ok: true, index: result?.index ?? {}, result: result?.result };
    }
    case "blogctl.index.google.sitemaps": {
      const result = await fetchJSON("/v1/search/index/jobs/google/sitemaps", { method: "POST" });
      return { ok: true, index: result?.index ?? {}, job: result?.job };
    }
    case "blogctl.index.google.inspect": {
      const offset = Number(message.offset ?? 0);
      const limit = Number(message.limit ?? 2000);
      const result = await fetchJSON("/v1/search/index/jobs/google/inspect", jsonOptions("POST", { offset, limit }));
      return { ok: true, index: result?.index ?? {}, job: result?.job };
    }
    case "blogctl.index.google.open": {
      const google = await googleSearchConsoleProbe({ active: true });
      return { ok: true, google };
    }
    case "blogctl.index.google.request.start": {
      await fetchJSON("/v1/search/index/google/request-queue", { method: "POST" });
      const prepared = await ensureGoogleSearchConsoleReady({ active: true, allowReset: true });
      const result = await fetchJSON("/v1/search/index/google/request-queue/start", { method: "POST" });
      kickGoogleIndexQueuePump();
      return { ok: true, index: result?.index ?? {}, job: result?.job, google: prepared.status };
    }
    case "blogctl.index.google.request.pause": {
      const result = await fetchJSON("/v1/search/index/google/request-queue/pause", { method: "POST" });
      return { ok: true, index: result?.index ?? {}, job: result?.job };
    }
    case "blogctl.index.google.request.resume": {
      const prepared = await ensureGoogleSearchConsoleReady({ active: true, allowReset: true });
      const result = await fetchJSON("/v1/search/index/google/request-queue/resume", { method: "POST" });
      kickGoogleIndexQueuePump();
      return { ok: true, index: result?.index ?? {}, job: result?.job, google: prepared.status };
    }
    case "blogctl.tool.save": {
      const name = String(message.name || "").trim();
      if (!name) throw new Error("tool name is required");
      const result = await fetchJSON(`/v1/tools/${encodeURIComponent(name)}`, jsonOptions("PUT", { config: message.config ?? {} }));
      return { ok: true, tools: await environmentTools(result?.tools ?? []) };
    }
    case "blogctl.tool.action": {
      const name = String(message.name || "").trim();
      const action = String(message.action || "").trim();
      if (name === "bridge" && action === "restart") {
        const bridge = await restartBridge();
        return { ok: true, bridge };
      }
      if (!name || !action) throw new Error("tool name and action are required");
      const result = await fetchJSON(
        `/v1/tools/${encodeURIComponent(name)}/actions/${encodeURIComponent(action)}`,
        { method: "POST" },
      );
      return {
        ok: true,
        message: result?.message || "操作已完成。",
        detail: result?.detail || {},
        tools: await environmentTools(result?.tools ?? []),
      };
    }
    case "blogctl.medium.engine": {
      return { ok: true, version: "native-cdn-batch-20261021" };
    }
    case "blogctl.publishing": {
      const result = await fetchJSON("/v1/publishing");
      return {
        ok: true,
        platforms: result?.platforms ?? [],
        compiler: result?.compiler ?? {},
        assets: result?.assets ?? {},
        assetStatus: result?.assetStatus ?? {},
      };
    }
    case "blogctl.publishing.save": {
      const result = await fetchJSON("/v1/publishing", jsonOptions("PUT", {
        platforms: message.platforms ?? [],
        compiler: message.compiler,
        assets: message.assets,
      }));
      return {
        ok: true,
        platforms: result?.platforms ?? [],
        compiler: result?.compiler ?? {},
        assets: result?.assets ?? {},
        assetStatus: result?.assetStatus ?? {},
      };
    }
    case "blogctl.google.index.event": {
      await writeBridgeLog(
        message.level || "info",
        message.message || "Google Search Console automation",
        message.fields || {},
      );
      return { ok: true };
    }
    case "blogctl.logs": {
      const limit = Math.max(1, Math.min(2000, Number(message.limit || 500)));
      const result = await fetchJSON(`/v1/logs?limit=${encodeURIComponent(limit)}`);
      return {
        ok: true,
        path: result?.path || "",
        level: result?.level || "info",
        entries: Array.isArray(result?.entries) ? result.entries : [],
      };
    }
    case "blogctl.logs.clear": {
      const result = await fetchJSON("/v1/logs", { method: "DELETE" });
      return { ok: true, path: result?.path || "" };
    }
    case "blogctl.jobs": {
      const result = await fetchJSON("/v1/jobs");
      const jobs = result?.jobs ?? [];
      if (jobs.some(job => ["running", "queued", "waiting"].includes(job.state) && (job.platforms ?? []).includes("toutiao"))) beginToutiaoRequestPump();
      // CLI/Bridge can enqueue authorized jobs too. The task list must wake
      // the browser driver, just as opening an individual active job does.
      if (jobs.some(job => ["running", "queued", "waiting"].includes(job.state) &&
          (job.platforms ?? []).includes("medium"))) beginMediumUploadPump();
      return { ok: true, jobs };
    }
    case "blogctl.job.start": {
      const request = message.request ?? {};
      await syncSessionsForPlatforms(request.platforms ?? []);
      const result = await fetchJSON("/v1/sync/jobs", jsonOptions("POST", request));
      if ((request.platforms ?? []).includes("toutiao")) beginToutiaoRequestPump();
      if ((request.platforms ?? []).includes("medium") && result?.job?.id) {
        beginMediumUploadPump();
      }
      return { ok: true, job: result?.job };
    }
    case "blogctl.job.get": {
      const id = String(message.id || "").trim();
      if (!id) throw new Error("job id is required");
      const result = await fetchJSON(`/v1/jobs/${encodeURIComponent(id)}`);
      if (result?.job && ["running", "queued", "waiting"].includes(result.job.state) &&
          (result.job.platforms ?? []).includes("medium")) beginMediumUploadPump();
      return { ok: true, job: result?.job };
    }
    case "blogctl.jobs.clear": {
      const result = await fetchJSON("/v1/jobs", { method: "DELETE" });
      return { ok: true, jobs: result?.jobs ?? [], removed: Number(result?.removed || 0) };
    }
    case "blogctl.job.retry": {
      const id = String(message.id || "").trim();
      if (!id) throw new Error("job id is required");
      const current = await fetchJSON(`/v1/jobs/${encodeURIComponent(id)}`);
      if (current?.job?.kind === "publishing") {
        await syncSessionsForPlatforms(current?.job?.platforms ?? []);
      }
      let google = null;
      if (current?.job?.type === "google-request-indexing") {
        google = (await ensureGoogleSearchConsoleReady({ active: true, allowReset: true })).status;
      }
      const result = await fetchJSON(`/v1/jobs/${encodeURIComponent(id)}/retry`, { method: "POST" });
      if (result?.job?.type === "google-request-indexing") kickGoogleIndexQueuePump();
      if ((result?.job?.platforms ?? []).includes("medium")) beginMediumUploadPump();
      return { ok: true, job: result?.job, ...(google ? { google } : {}) };
    }
    case "blogctl.job.pause": {
      const id = String(message.id || "").trim();
      if (!id) throw new Error("job id is required");
      const result = await fetchJSON(`/v1/jobs/${encodeURIComponent(id)}/pause`, { method: "POST" });
      return { ok: true, job: result?.job };
    }
    case "blogctl.job.resume": {
      const id = String(message.id || "").trim();
      if (!id) throw new Error("job id is required");
      const current = await fetchJSON(`/v1/jobs/${encodeURIComponent(id)}`);
      let google = null;
      if (current?.job?.type === "google-request-indexing") {
        google = (await ensureGoogleSearchConsoleReady({ active: true, allowReset: true })).status;
      }
      const result = await fetchJSON(`/v1/jobs/${encodeURIComponent(id)}/resume`, { method: "POST" });
      if (result?.job?.type === "google-request-indexing") kickGoogleIndexQueuePump();
      if ((result?.job?.platforms ?? []).includes("medium")) beginMediumUploadPump();
      return { ok: true, job: result?.job, ...(google ? { google } : {}) };
    }
    default: return null;
  }
}

const MEDIUM_UPLOAD_ALARM = "blogctl.medium.upload";
let mediumPumpRunning = false;

const mediumDelay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));


async function processMediumUpload(task) {
  if (task.mode === "prepare") return prepareMediumReplacement(task);
  if (task.mode === "document") return verifyMediumDocument(task);
  if (task.mode === "canonical") return configureMediumCanonical(task);
  if (task.mode === "insert" || task.mode === "verify") {
    if (!validMediumEditorTask(task)) throw new Error("Invalid Medium browser editor task");
    const tabId = await findMediumEditorTab(task.draftId);
    if (task.mode === "verify") {
      // Allow Medium's debounced autosave to finish after the last paste.
      // Do not reload the editor: refreshing during a pending save can lose
      // the last few inserted figures.
      await mediumDelay(7000);
      // Medium saves editor operations automatically. Port the Tampermonkey
      // "scan after insertion" logic; do not reload or recreate the draft.
      let lastError = "Medium editor is not ready";
      for (let attempt = 0; attempt < 180; attempt++) {
        const tab = await chrome.tabs.get(tabId);
        if (tab.url && tab.url !== "https://medium.com/p/" + task.draftId + "/edit" &&
            tab.pendingUrl !== "https://medium.com/p/" + task.draftId + "/edit") {
          throw new Error("Medium draft redirected during verification");
        }
        try {
          const rows = await chrome.scripting.executeScript({
            target: { tabId }, world: "MAIN", func: verifyMediumDraftInPage,
            args: [task.draftId, task.expectedImages],
          });
          const result = rows?.[0]?.result;
          if (result?.inserted) return result;
          if (result?.error) lastError = result.error;
        } catch (error) {
          lastError = String(error?.message || error).slice(0,220);
        }
        await mediumDelay(500);
      }
      throw new Error("Medium draft autosave verification failed: " + lastError);
    }
    const rows = await chrome.scripting.executeScript({
      target: { tabId }, world: "MAIN", func: insertMediumImageInPage,
      args: [task.base64, task.mimeType, task.filename, task.draftId, task.marker],
    });
    return rows?.[0]?.result ?? { error: "Medium editor returned no image insertion result" };
  }
  if (!validMediumUploadTask(task)) throw new Error("Invalid Medium browser upload task");
  const tabId = await findMediumEditorTab(task.draftId);
  const results = await chrome.scripting.executeScript({
    target: { tabId }, world: "MAIN",
    func: uploadMediumInPage,
    args: [task.base64, task.mimeType, task.filename, task.draftId],
  });
  const response = results?.[0]?.result;
  if (!response || typeof response !== "object") {
    throw new Error("Medium page did not return an upload result");
  }
  return response;
}

async function pollMediumUploads() {
  if (mediumPumpRunning) return;
  mediumPumpRunning = true;
  try {
    await writeBridgeLog("info", "medium browser upload pump started");
    for (let attempt = 0; attempt < 900; attempt++) {
      const jobs = (await fetchJSON("/v1/jobs")).jobs ?? [];
      const active = jobs.some((job) => ["running", "queued", "waiting"].includes(job.state) &&
        (job.platforms ?? []).includes("medium"));
      if (!active) {
        if (!jobs.some(job => ["running", "queued", "waiting"].includes(job.state) && (job.platforms ?? []).includes("toutiao"))) await chrome.alarms?.clear(MEDIUM_UPLOAD_ALARM);
        return;
      }
      const response = await fetchJSON("/v1/medium/browser-upload/next");
      const task = response?.upload;
      if (!task) {
        await mediumDelay(700);
        continue;
      }
      await writeBridgeLog("info", "medium browser upload task claimed", { draftId: task.draftId });
      let result;
      try {
        result = await processMediumUpload(task);
      } catch (error) {
        result = { error: errorMessage(error).slice(0, 220) };
      }
      await writeBridgeLog(result.error ? "warn" : "info", "medium browser upload finished", {
        draftId: task.draftId, success: !result.error, error: result.error ?? "",
      });
      await fetchJSON("/v1/medium/browser-upload/result", jsonOptions("POST", {
        id: task.id, fileId: result.fileId ?? "",
        width: result.width ?? 0, height: result.height ?? 0,
        inserted: result.inserted === true, imageCount: result.imageCount ?? 0,
        error: result.error ?? "",
      }));
    }
  } catch (error) {
    console.warn("BlogCTL Medium browser upload polling paused:", errorMessage(error));
    await writeBridgeLog("error", "medium browser upload pump failed", {
      error: errorMessage(error).slice(0, 220),
    });
  } finally {
    mediumPumpRunning = false;
  }
}

function beginMediumUploadPump() {
  if (mediumPumpRunning) return;
  // MV3 service workers may sleep. A repeating alarm resumes the task pump.
  chrome.alarms.create(MEDIUM_UPLOAD_ALARM, { periodInMinutes: 0.5 }).catch(() => {});
  void pollMediumUploads();
}

// MCP-driven publishing can start without opening the extension UI. MV3
// alarms wake the background worker to refresh its ordinary browser session.
const MCP_SESSION_ALARM = "blogctl-mcp-session-refresh";
let mcpRefreshRunning = false;
async function pollMCPSessionRefresh() {
  if (mcpRefreshRunning) return;
  mcpRefreshRunning = true;
  try {
    const response = await fetchJSON("/v1/mcp/session-refresh");
    for (const platform of response?.pending ?? []) {
      if (!PLATFORM_SESSIONS[platform]) continue;
      try { await syncPlatformSession(platform); }
      catch (error) { console.warn("BlogCTL MCP session refresh:", platform, errorMessage(error)); }
    }
  } catch (error) {
    // Bridge is optional when no MCP operation is pending.
    console.debug("BlogCTL MCP refresh poll:", errorMessage(error));
  } finally {
    mcpRefreshRunning = false;
  }
}
chrome.alarms?.create(MCP_SESSION_ALARM, { periodInMinutes: 0.5 }).catch(() => {});

chrome.alarms?.onAlarm.addListener((alarm) => {
  if (alarm.name === MEDIUM_UPLOAD_ALARM) { void pollMediumUploads(); void pollToutiaoRequests(); }
  if (alarm.name === MCP_SESSION_ALARM) void pollMCPSessionRefresh();
});

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (!message || typeof message !== "object") return false;
  const task = handleMessage(message);
  if (!task) return false;
  task.then(sendResponse).catch(async (error) => {
    console.error("BlogCTL extension request failed:", message.type, errorMessage(error));
    sendResponse({ ok: false, error: errorMessage(error), code: error?.code || "", status: error?.status || 0, details: error?.details || null });
  });
  return true;
});

let toutiaoPumpRunning = false;
function beginToutiaoRequestPump() {
 chrome.alarms.create(MEDIUM_UPLOAD_ALARM, { periodInMinutes: 0.5 }).catch(() => {});
 void pollToutiaoRequests();
}
async function pollToutiaoRequests() {
  if (toutiaoPumpRunning) return;
  toutiaoPumpRunning = true;
  try {
    for (let attempt = 0; attempt < 900; attempt++) {
      const jobs = (await fetchJSON("/v1/jobs")).jobs ?? [];
      const active = jobs.some(job => ["running", "queued", "waiting"].includes(job.state) && (job.platforms ?? []).includes("toutiao"));
      if (!active) return;
      const next = await fetchJSON("/v1/toutiao/browser/next");
      if (!next.request) { await delay(700); continue; }
      await writeBridgeLog("info", "toutiao browser request claimed");
      let result;
      try { result = await performToutiaoBrowserRequest(next.request); }
      catch { result = { error: "Browser request failed; inspect remote state before retrying" }; }
      await writeBridgeLog(result.error ? "warn" : "info", "toutiao browser request finished", { success: !result.error, status: result.status ?? 0 });
      await fetchJSON("/v1/toutiao/browser/result", jsonOptions("POST", { id: next.request.id, ...result }));
    }
  } catch {
    console.warn("Toutiao browser transport unavailable");
  } finally {
    toutiaoPumpRunning = false;
  }
}
