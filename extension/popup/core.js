"use strict";

(function (root) {
  function errorMessage(error) {
    return error?.message || String(error);
  }

  // Feature modules use a stable request surface independent of the host.
  const send = (type, payload = {}) => root.BlogCTLTransport.send(type, payload);

  function setStatus(element, kind, text, detail = "") {
    if (!element) return;
    const className = `status ${kind}`;
    if (element.className !== className) element.className = className;
    if (element.textContent !== text) element.textContent = text;
    if (element.title !== detail) element.title = detail;
  }

  function setMessage(element, text = "", type = "") {
    if (!element) return;
    const className = `message${type ? ` ${type}` : ""}`;
    if (element.textContent !== text) element.textContent = text;
    if (element.className !== className) element.className = className;
  }

  function formatTime(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "";
    return date.toLocaleTimeString(root.BlogCTLI18n?.locale || undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  }

  function versionLabel(value) {
    return value ? `v${value}` : "版本未知";
  }

  async function refreshBridgeIndicator(status) {
    const indicator = document.getElementById("bridgeIndicator");
    const extensionVersion = document.getElementById("aboutExtensionVersion");
    const bridgeVersion = document.getElementById("aboutBridgeVersion");
    const aboutStatus = document.getElementById("aboutBridgeStatus");
    const localVersion = root.chrome?.runtime?.getManifest?.()?.version || "";
    if (extensionVersion && localVersion) extensionVersion.textContent = versionLabel(localVersion);
    try {
      const current = status || (await send("blogctl.status")).status;
      if (document.activeElement?.id === "indexEngineFilter") return current;
      const bridge = current?.bridge ?? {};
      if (extensionVersion) extensionVersion.textContent = versionLabel(bridge.extensionVersion || localVersion);
      if (bridgeVersion) bridgeVersion.textContent = bridge.running ? versionLabel(bridge.bridgeVersion) : "未连接";
      if (bridge.running && bridge.compatible === false) {
        const versions = [
          bridge.extensionVersion ? `Extension v${bridge.extensionVersion}` : "",
          bridge.nativeHostVersion ? `Native Host v${bridge.nativeHostVersion}` : "Native Host 版本未知",
          bridge.bridgeVersion ? `Bridge v${bridge.bridgeVersion}` : "Bridge 版本未知",
        ].filter(Boolean).join(" · ");
        setStatus(indicator, "error", "运行时版本不一致", versions);
      } else if (bridge.running) {
        setStatus(indicator, "ok", "Bridge 已连接", bridge.pid ? `PID ${bridge.pid}` : "");
      } else {
        setStatus(indicator, "error", "Bridge 未连接", bridge.error || "");
      }
      setStatus(aboutStatus, bridge.running ? "ok" : "error", bridge.running ? "Bridge 已连接" : "Bridge 未连接", bridge.error || "");
      return current;
    } catch (error) {
      setStatus(indicator, "unknown", "Bridge 检测失败", errorMessage(error));
      if (extensionVersion) extensionVersion.textContent = versionLabel(localVersion);
      if (bridgeVersion) bridgeVersion.textContent = "版本未知";
      setStatus(aboutStatus, "unknown", "Bridge 检测失败", errorMessage(error));
      throw error;
    }
  }

  root.BlogCTLPopup = {
    errorMessage,
    send,
    setStatus,
    setMessage,
    formatTime,
    refreshBridgeIndicator,
  };
})(globalThis);
