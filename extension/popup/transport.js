"use strict";

// The UI depends on a small, host-agnostic request contract. IDFlow uses the
// same Host -> Feature -> Transport separation. Never duplicate feature state
// or expose the native messaging token to a Web page.
(function (root) {
  function send(type, payload = {}) {
    if (!root.chrome?.runtime?.sendMessage) {
      return Promise.reject(new Error("当前宿主尚未连接 BlogCTL Transport"));
    }
    return new Promise((resolve, reject) => {
      root.chrome.runtime.sendMessage({ type, ...payload }, (response) => {
        const runtimeError = root.chrome.runtime.lastError;
        if (runtimeError) {
          reject(new Error(runtimeError.message));
          return;
        }
        if (!response?.ok) {
          const error = new Error(response?.error || "BlogCTL Extension request failed");
          error.code = response?.code || "";
          error.details = response?.details || null;
          error.status = response?.status || 0;
          error.response = response;
          reject(error);
          return;
        }
        resolve(response);
      });
    });
  }
  function webSend(type, payload = {}) {
    const id = String(Date.now()) + "-" + (++webSequence);
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => { pending.delete(id); reject(new Error("BlogCTL Web Console request timed out; check Extension connection")); }, 45000);
      pending.set(id, { resolve, reject, timeout });
      root.postMessage({ channel: "blogctl:console:request:v1", id, type, payload },
        "http://127.0.0.1:32145");
    });
  }
  let webSequence = 0;
  const pending = new Map();
  if (root.location?.origin === "http://127.0.0.1:32145" &&
      root.location?.pathname.startsWith("/console/")) {
    root.addEventListener("message", (event) => {
      if (event.origin !== "http://127.0.0.1:32145" || event.source !== root ||
          event.data?.channel !== "blogctl:console:response:v1") return;
      const ticket = pending.get(event.data.id);
      if (!ticket) return;
      clearTimeout(ticket.timeout);
      pending.delete(event.data.id);
      const response = event.data.response;
      if (response?.ok) ticket.resolve(response);
      else ticket.reject(Object.assign(new Error(response?.error || "Web Console request failed"), {
        code: response?.code || "", details: response?.details || null, status: response?.status || 0,
      }));
    });
    root.BlogCTLTransport = { send: webSend, host: "web" };
  } else {
    root.BlogCTLTransport = { send, host: "extension" };
  }
})(globalThis);
