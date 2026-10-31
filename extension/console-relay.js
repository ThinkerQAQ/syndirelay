"use strict";

// Content script only installed on BlogCTL's fixed local console endpoint.
// This is the same authenticated Extension message transport as Side Panel.
// The Go-served page never receives a Bridge token, cookie or platform session.
const CONSOLE_ORIGIN = "http://127.0.0.1:32145";
if (location.origin === CONSOLE_ORIGIN &&
    location.pathname.startsWith("/console/") &&
    window.top === window) {
  window.addEventListener("message", (event) => {
    if (event.source !== window || event.origin !== CONSOLE_ORIGIN) return;
    const request = event.data;
    if (request?.channel !== "blogctl:console:request:v1" ||
        typeof request.id !== "string" || request.id.length > 96 ||
        typeof request.type !== "string" || !/^blogctl\.[a-z0-9._-]+$/.test(request.type) ||
        !request.payload || typeof request.payload !== "object" || Array.isArray(request.payload)) return;
    chrome.runtime.sendMessage({ ...request.payload, type: request.type }, (response) => {
      const runtimeError = chrome.runtime.lastError;
      window.postMessage({
        channel: "blogctl:console:response:v1",
        id: request.id,
        response: runtimeError ? { ok: false, error: runtimeError.message } :
          (response ?? { ok: false, error: "Bridge did not respond" }),
      }, CONSOLE_ORIGIN);
    });
  });
}
