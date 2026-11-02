// These functions execute in the logged-in Medium editor's MAIN world.
// Neither manipulates private Medium deltas or exposes browser credentials.

// Medium can render a native uploaded image from either its older CDN or
// miro.medium.com. Only exact Medium-owned HTTPS hosts count as persisted;
// external or lookalike hosts must not satisfy draft verification.
export function loadedNativeMediumImage(img) {
  if (!img?.complete || !(img.naturalWidth > 0)) return false;
  const url = img.currentSrc || img.src;
  if (typeof url !== "string") return false;
  try {
    const parsed = new URL(url);
    return parsed.protocol === "https:" && (
      /^cdn-images(?:-\d+)?\.medium\.com$/.test(parsed.hostname) ||
      parsed.hostname === "miro.medium.com" ||
      parsed.hostname === "images.medium.com");
  } catch { return false; }
}

function mediumEditorImages(root) {
  if (!root) return [];
  return [...new Set([...root.querySelectorAll("figure img,article img,img")])];
}

// An older image that is still loading must not make the next image fail.
// The final full-draft verification checks every expected image separately.
export function hasNewLoadedMediumImage(before, after) {
  const seen = new Set(before);
  return after.length > before.length && after.some(img => !seen.has(img) && loadedNativeMediumImage(img));
}
export async function insertMediumImageInPage(base64, mimeType, filename, expectedDraftID, marker) {
  const fail = (message) => ({ error: message });
  if (location.origin !== "https://medium.com" ||
      location.pathname !== "/p/" + expectedDraftID + "/edit" ||
      !/^[0-9a-f]{8,24}$/.test(expectedDraftID) ||
      !/^\[BLOGCTL_IMAGE_\d{3}\]$/.test(marker) ||
      !/^image\/(?:png|jpeg|gif|webp)$/.test(mimeType)) {
    return fail("Medium editor image task failed validation");
  }
  // executeScript serializes ONLY this function, not imported module helpers.
  // Keep every dependency used in MAIN world inside the injected function.
  const nativeMediumImage = (img) => {
    if (!img?.complete || !(img.naturalWidth > 0)) return false;
    const url = img.currentSrc || img.src;
    if (typeof url !== "string") return false;
    try {
      const parsed = new URL(url);
      return parsed.protocol === "https:" &&
        (/^cdn-images(?:-\d+)?\.medium\.com$/.test(parsed.hostname) ||
         parsed.hostname === "miro.medium.com" || parsed.hostname === "images.medium.com");
    } catch { return false; }
  };
  const imagesFrom = (element) => element
    ? [...new Set([...element.querySelectorAll("figure img,article img,img")])] : [];
  const hasNewImage = (previous, images) => {
    const known = new Set(previous);
    return images.length > previous.length &&
      images.some(img => !known.has(img) && nativeMediumImage(img));
  };
  const editor = () => document.querySelector('.postArticle-content[contenteditable="true"]');
  const imagesInEditor = () => imagesFrom(editor());
  const imageCount = () => imagesInEditor().length;
  const matches = () => {
    const all = [...document.querySelectorAll("p,div,span,h1,h2,h3,h4")].filter((el) =>
      el.isContentEditable && el.textContent?.trim() === marker &&
      ![...el.querySelectorAll("p,div,span,h1,h2,h3,h4")]
        .some((child) => child.isContentEditable && child.textContent?.trim() === marker));
    return all;
  };
  const locate = () => {
    const found = matches();
    if (found.length !== 1) throw new Error("Medium editor marker " + marker + " occurs " + found.length + " times");
    const node = found[0];
    let editable = node;
    while (editable && editable.getAttribute?.("contenteditable") !== "true") editable = editable.parentElement;
    if (!editable) throw new Error("Medium image marker is not editable");
    return { node, editable };
  };
  const select = (node, editable, collapse) => {
    node.scrollIntoView({ block: "center" });
    editable.focus({ preventScroll: true });
    const r = document.createRange();
    r.selectNodeContents(node);
    if (collapse) r.collapse(true);
    const selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(r);
  };
  const markerNumber = Number(marker.slice(-4, -1));
  const conflictingEdit = () => /Saving failed because someone is also editing|Reload to see their changes/i
    .test(document.body?.innerText || "");
  const savedStatus = () => {
    const status = document.querySelector(".js-metabarMessage");
    return Boolean(status && /Saved\s*$/i.test(status.textContent || "") &&
      !/Saving|Unsaved/i.test(status.textContent || ""));
  };
  // The original Tampermonkey script detects an exact, standalone marker.
  // A missing marker is NOT sufficient for an idempotent retry: only skip
  // after a matching, already-rendered native picture occupies its position.
  const alreadySaved = () => {
    const paragraphs = [...(editor()?.querySelectorAll("p") || [])];
    const remaining = paragraphs.map(p => (p.textContent || "").trim())
      .filter(t => /^\[BLOGCTL_IMAGE_\d{3}\]$/.test(t));
    if (remaining.includes(marker)) return false;
    if (remaining.some(t => Number(t.slice(-4, -1)) < markerNumber)) return false;
    const next = paragraphs.find(p => /^\[BLOGCTL_IMAGE_\d{3}\]$/.test(p.textContent?.trim() || "") &&
      Number(p.textContent.trim().slice(-4, -1)) > markerNumber);
    const pictures = imagesInEditor();
    const beforeNext = next
      ? pictures.filter(img => Boolean(img.compareDocumentPosition(next) & Node.DOCUMENT_POSITION_FOLLOWING))
      : pictures;
    return beforeNext.length >= markerNumber && beforeNext.every(nativeMediumImage);
  };
  try {
    if (conflictingEdit()) return fail("Medium rejected autosave due to concurrent edits; inspect the draft before retrying");
    if (alreadySaved()) {
      return { inserted: true, alreadyPresent: true, imageCount: imageCount() };
    }
    // Medium may show a Cloudflare challenge at the SAME /p/{id}/edit URL.
    // Wait for the real editor and marker, not merely tab.status=complete.
    let ready = false;
    const readyStart = Date.now();
    while (Date.now() - readyStart < 70000) {
      // A prior attempt can finish saving after its browser task times out.
      // Re-check the same saved draft rather than inserting a duplicate.
      if (alreadySaved() && savedStatus()) {
        return { inserted: true, alreadyPresent: true, imageCount: imageCount() };
      }
      if (editor() && matches().length === 1) {
        ready = true; break;
      }
      await new Promise((r) => setTimeout(r, 500));
    }
    if (!ready) {
      const body = document.body?.innerText || "";
      if (/验证您是真人|请验证您是真人|verify you are human|Checking your browser|正在进行安全验证/i.test(body)) {
        return fail("Medium requires human verification in the browser; finish the challenge in this existing draft before resuming");
      }
      return fail("Medium editor did not render the image marker after 70 seconds; draft may be blocked or text save did not persist");
    }
    const { node, editable } = locate();
    const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
    const file = new File([bytes], filename, { type: mimeType });
    const beforeImages = imagesInEditor();
    const before = beforeImages.length;
    select(node, editable, true);
    const data = new DataTransfer();
    data.items.add(file);
    const event = new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true, composed: true });
    editable.dispatchEvent(event);
    const start = Date.now();
    while (Date.now() - start < 24000 && imageCount() <= before) {
      await new Promise((r) => setTimeout(r, 250));
    }
    if (imageCount() <= before) return fail("Medium editor did not insert the PNG at " + marker);
    // Preserve the exact marker until a fully loaded native Medium CDN image
    // is visible. Deleting it earlier turns a transient upload failure into an
    // unresumable draft with neither placeholder nor verified image.
    // A native image is committed asynchronously. Do not time out an
    // individual insertion merely because Medium has not changed its
    // DraftSaving indicator to DraftSaved: our real 2-series test showed
    // that this incorrectly failed on image #11 even though the image was
    // eventually saved. Require a real CDN image and no marker, then let
    // the final whole-draft verification wait for DraftSaved.
    const settledAt = Date.now();
    let uploaded = false;
    while (Date.now() - settledAt < 30000) {
      await new Promise(resolve => setTimeout(resolve, 350));
      if (conflictingEdit()) {
        return fail("Medium rejected autosave due to concurrent edits");
      }
      const images = imagesInEditor();
      uploaded = hasNewImage(beforeImages, images);
      if (uploaded && Date.now()-settledAt >= 1100) break;
    }
    if (!uploaded) {
      const images = imagesInEditor();
      const native = images.filter(nativeMediumImage).length;
      return fail("Medium did not confirm a loaded native CDN image at " + marker +
        " (editor images " + images.length + ", loaded Medium images " + native +
        ", marker preserved for retry " + (matches().length > 0) + ")");
    }
    // Only now is it safe to remove the placeholder. A saved image may be
    // present before Medium finishes transforming its initial blob: preview.
    const remaining = matches();
    if (remaining.length === 1) {
      const current = locate();
      select(current.node, current.editable, false);
      if (!document.execCommand("delete", false, null) || matches().length !== 0) {
        return fail("Native image loaded but marker deletion failed; inspect before retry");
      }
    } else if (remaining.length !== 0) {
      return fail("Native image loaded but marker count changed; inspect draft before retry");
    }
    return { inserted: true, imageCount: imageCount() };
  } catch (error) {
    return fail(String(error?.message || error).slice(0, 260));
  }
}

// Check the editor's current auto-saved state without navigating or reloading.
// The original userscript's final scan is retained, with stronger image checks.
export function verifyMediumDraftInPage(expectedDraftID, expectedImages) {
  if (location.origin !== "https://medium.com" ||
      location.pathname !== "/p/" + expectedDraftID + "/edit") {
    return { error: "Browser verification is not on the expected Medium draft" };
  }
  // Chrome MAIN-world injection has no access to module-scope helpers.
  const nativeMediumImage = img => {
    if (!img?.complete || !(img.naturalWidth > 0)) return false;
    try {
      const parsed = new URL(img.currentSrc || img.src);
      return parsed.protocol === "https:" &&
        (/^cdn-images(?:-\d+)?\.medium\.com$/.test(parsed.hostname) ||
         parsed.hostname === "miro.medium.com" || parsed.hostname === "images.medium.com");
    } catch { return false; }
  };
  const root = document.querySelector('.postArticle-content[contenteditable="true"]');
  if (!root) return { error: "Medium draft editor has not loaded" };
  if (/Saving failed because someone is also editing|Reload to see their changes/i
    .test(document.body?.innerText || "")) {
    return { error: "Medium rejected autosave due to concurrent edits" };
  }
  const markers = [...root.querySelectorAll("p")].filter(el =>
    /^\[BLOGCTL_IMAGE_\d{3}\]$/.test(el.textContent?.trim() || ""));
  const pictures = [...new Set([...root.querySelectorAll("figure img,article img,img")])];
  if (markers.length) {
    return { error: markers.length + " image placeholders remain in Medium draft" };
  }
  if (pictures.length !== expectedImages) {
    return { error: "Medium editor has " + pictures.length +
      " images; expected " + expectedImages };
  }
  if (pictures.some(img => !nativeMediumImage(img))) {
    return { error: "Medium image uploads have not all resolved to native CDN images" };
  }
  const status = document.querySelector(".js-metabarMessage");
  if (!status || !/Saved\s*$/i.test(status.textContent || "") ||
      /Saving|Unsaved/i.test(status.textContent || "")) {
    return { error: "Medium editor has not confirmed autosave" };
  }
  return { inserted: true, imageCount: pictures.length };
}

export function validMediumEditorTask(task) {
  if (!task || !/^[0-9a-f]{32}$/.test(task.id || "") ||
      !/^[0-9a-f]{8,24}$/.test(task.draftId || "")) return false;
  if (task.mode === "verify") return Number.isInteger(task.expectedImages) && task.expectedImages >= 0 && task.expectedImages <= 500;
  return task.mode === "insert" && /^\[BLOGCTL_IMAGE_\d{3}\]$/.test(task.marker || "") &&
    /^[\w.-]{1,160}$/.test(task.filename || "") &&
    /^image\/(?:png|jpeg|gif|webp)$/.test(task.mimeType || "") &&
    typeof task.base64 === "string" && task.base64.length > 0 && task.base64.length < 28 * 1024 * 1024;
}
