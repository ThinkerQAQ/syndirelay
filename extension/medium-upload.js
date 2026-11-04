// Runs only in the actual Medium editor's MAIN world. No Cookie or XSRF
// value is ever returned to SyndiRelay; only native fileId and dimensions.
export async function uploadMediumInPage(base64, mimeType, filename, expectedDraftID) {
  const expectedPath = "/p/" + expectedDraftID + "/edit";
  if (location.origin !== "https://medium.com" || location.pathname !== expectedPath ||
      !/^[a-f0-9]{8,24}$/.test(expectedDraftID)) {
    return { error: "Medium editor tab URL does not match the selected draft" };
  }
  if (!/^[a-zA-Z0-9._-]{1,160}$/.test(filename) || !/^image\/(?:png|jpeg|webp|gif)$/.test(mimeType)) {
    return { error: "Unsupported Medium image filename or format" };
  }
  try {
    const bytes = Uint8Array.from(atob(base64), (char) => char.charCodeAt(0));
    const form = new FormData();
    form.append("uploadedFile", new Blob([bytes], { type: mimeType }), filename);
    const xsrfCookie = document.cookie.split(";").map((entry) => entry.trim())
      .find((entry) => entry.startsWith("xsrf="));
    if (!xsrfCookie) return { error: "Medium editor XSRF cookie is unavailable" };
    const xsrf = decodeURIComponent(xsrfCookie.slice("xsrf=".length));
    const response = await fetch("/_/upload?source=6", {
      method: "POST", credentials: "same-origin", body: form,
      headers: { accept: "*/*", "x-obvious-cid": "web", "x-xsrf-token": xsrf },
    });
    if (!response.ok) return { error: "Medium browser upload HTTP " + response.status };
    const raw = await response.text();
    const start = raw.indexOf("{");
    if (start < 0) return { error: "Medium image upload returned no JSON" };
    const payload = JSON.parse(raw.slice(start));
    const value = payload?.payload?.value;
    if (payload?.success !== true || !value?.fileId) {
      return { error: "Medium image upload returned no native file ID" };
    }
    return { fileId: String(value.fileId), width: Number(value.imgWidth), height: Number(value.imgHeight) };
  } catch {
    return { error: "Medium browser image upload request failed" };
  }
}

export function validMediumUploadTask(task) {
  return Boolean(task && /^[a-f0-9]{32}$/.test(task.id ?? "") &&
    /^[a-f0-9]{8,24}$/.test(task.draftId ?? "") &&
    /^[a-zA-Z0-9._-]{1,160}$/.test(task.filename ?? "") &&
    /^image\/(?:png|jpeg|gif|webp)$/.test(task.mimeType ?? "") &&
    typeof task.base64 === "string" && task.base64.length > 0 &&
    task.base64.length <= 28 * 1024 * 1024);
}

// Edge/Chrome sometimes reject tab operations while the user is dragging a tab.
 // Retry only the browser's explicit temporary "tabs cannot be edited" error,
 // never timeouts, redirects, permission failures or content/script errors.
export function transientMediumTabError(error) {
  return /Tabs cannot be edited right now|user may be dragging a tab/i.test(String(error?.message || error));
}
export async function retryMediumTabOperation(operation, sleep = ms => new Promise(resolve => setTimeout(resolve, ms))) {
  for (let attempt = 0; attempt < 7; attempt++) {
    try { return await operation(); } catch (error) {
      if (!transientMediumTabError(error) || attempt === 6) throw error;
      await sleep(Math.min(2000, 300 * (attempt + 1)));
    }
  }
}

// Extension context: after chrome.tabs.create(), Edge may report about:blank
// as tab.url and the requested editor address only in pendingUrl until the
// first navigation commits. Never treat this expected transition as redirect.
export async function findMediumEditorTab(draftId, tabs = chrome.tabs, sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))) {
  if (!/^[a-f0-9]{8,24}$/.test(draftId)) throw new Error("Invalid Medium draft identifier");
  const editorUrl = "https://medium.com/p/" + draftId + "/edit";
  const query = () => retryMediumTabOperation(() => tabs.query({ url: "https://medium.com/p/*/edit" }), sleep);
  const matching = async () => {
    const found = await query();
    const matches = found.filter(tab => tab.url === editorUrl || tab.pendingUrl === editorUrl);
    if (matches.length > 1) {
      throw new Error("Multiple Medium editor tabs exist for the same draft; close duplicates before using SyndiRelay");
    }
    return matches[0];
  };
  let tab = await matching();
  if (!tab) {
    // A rejected create may still leave a pending navigation behind.
    // Query again before each attempt; never open duplicate editor tabs.
    for (let attempt = 0; attempt < 7; attempt++) {
      try {
        tab = await tabs.create({ url: editorUrl, active: false });
        break;
      } catch (error) {
        if (!transientMediumTabError(error) || attempt === 6) throw error;
        await sleep(Math.min(2000, 300 * (attempt + 1)));
        tab = await matching();
        if (tab) break;
      }
    }
  }
  if (!Number.isInteger(tab?.id)) throw new Error("Could not open Medium editor tab");

  for (let attempt = 0; attempt < 120; attempt++) {
    const current = await retryMediumTabOperation(() => tabs.get(tab.id), sleep);
    // If already at the final editor document, this is safe to inject even
    // if the browser still exposes a stale pendingUrl property.
    if (current.status === "complete" && current.url === editorUrl) return current.id;
    // Edge sometimes reports the placeholder about:blank as "complete" while
    // the actual editor is still in pendingUrl. Do not abort that transition.
    if (current.pendingUrl === editorUrl) {
      await sleep(500);
      continue;
    }
    if (current.status === "complete") {
      throw new Error("Medium editor redirected or failed to load the expected draft");
    }
    if (current.pendingUrl && current.pendingUrl !== editorUrl && current.url !== editorUrl) {
      throw new Error("Medium editor navigation was redirected");
    }
    await sleep(500);
  }
  throw new Error("Medium editor did not finish loading within 60 seconds");
}
