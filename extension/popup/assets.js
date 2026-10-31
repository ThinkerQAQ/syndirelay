"use strict";

(function (root) {
  const state = { initialized: false, active: false, editing: false, compiler: {}, assets: {}, assetStatus: {} };
  let mermaidWidth, mermaidScale, r2Bucket, r2PublicBaseUrl, r2AccessKeyId, r2SecretAccessKey, r2AccountId, r2Endpoint;
  let assetStatus, assetStatusDetail, editButton, cancelButton, saveButton, editActions, message;

  function writeForm() {
    const mermaid = state.compiler?.mermaid ?? {};
    const r2 = state.assets?.r2 ?? {};
    mermaidWidth.value = Number(mermaid.width || 1200);
    mermaidScale.value = Number(mermaid.scale || 2);
    r2Bucket.value = r2.bucket || "";
    r2PublicBaseUrl.value = r2.publicBaseUrl || "";
    r2AccessKeyId.value = r2.accessKeyId || "";
    r2AccountId.value = r2.accountId || "";
    r2Endpoint.value = r2.endpoint || "";
    r2SecretAccessKey.value = "";
    r2SecretAccessKey.placeholder = r2.secretAccessKeyConfigured ? "已配置；留空保持不变" : "未配置";

    const ready = Boolean(state.assetStatus?.ready);
    BlogCTLPopup.setStatus(assetStatus, ready ? "ok" : "unknown", ready ? "R2 兜底可用" : "R2 兜底未就绪");
    const missing = state.assetStatus?.missing ?? [];
    assetStatusDetail.textContent = ready
      ? "共享 R2 已就绪；所有发布平台在平台原生图片上传失败时统一使用该兜底。"
      : `平台原生图片上传仍可使用；共享 R2 缺少：${missing.join("、") || "未知配置"}。平台原生上传失败时将无法使用 R2 兜底。`;
  }

  function setEditing(editing) {
    state.editing = editing;
    for (const element of [mermaidWidth, mermaidScale, r2Bucket, r2PublicBaseUrl, r2AccessKeyId, r2SecretAccessKey, r2AccountId, r2Endpoint]) {
      element.disabled = !editing;
    }
    editButton.hidden = editing;
    editActions.hidden = !editing;
  }


  function readForm() {
    return {
      compiler: {
        mermaid: {
          format: "png",
          width: Number(mermaidWidth.value || 1200),
          scale: Number(mermaidScale.value || 2),
        },
      },
      assets: {
        store: "r2",
        r2: {
          bucket: r2Bucket.value.trim(),
          publicBaseUrl: r2PublicBaseUrl.value.trim(),
          accessKeyId: r2AccessKeyId.value.trim(),
          secretAccessKey: r2SecretAccessKey.value.trim(),
          accountId: r2AccountId.value.trim(),
          endpoint: r2Endpoint.value.trim(),
        },
      },
    };
  }

  async function save() {
    const runtime = readForm();
    saveButton.disabled = true;
    BlogCTLPopup.setMessage(message, "正在保存共享资产配置…");
    try {
      const response = await BlogCTLPopup.send("blogctl.publishing.save", {
        compiler: runtime.compiler,
        assets: runtime.assets,
      });
      state.compiler = response.compiler ?? state.compiler;
      state.assets = response.assets ?? state.assets;
      state.assetStatus = response.assetStatus ?? state.assetStatus;
      writeForm();
      setEditing(false);
      BlogCTLPopup.setMessage(message, "共享资产配置已保存，所有平台将使用同一套 R2 配置。", "ok");
    } catch (error) {
      BlogCTLPopup.setMessage(message, BlogCTLPopup.errorMessage(error), "error");
    } finally {
      saveButton.disabled = false;
    }
  }

  async function refresh() {
    if (!state.active) return;
    BlogCTLPopup.setMessage(message);
    try {
      const response = await BlogCTLPopup.send("blogctl.publishing");
      state.compiler = response.compiler ?? {};
      state.assets = response.assets ?? {};
      state.assetStatus = response.assetStatus ?? {};
      writeForm();
    } catch (error) {
      BlogCTLPopup.setMessage(message, BlogCTLPopup.errorMessage(error), "error");
    }
  }

  function init() {
    if (state.initialized) return;
    mermaidWidth = document.getElementById("mermaidWidth");
    mermaidScale = document.getElementById("mermaidScale");
    r2Bucket = document.getElementById("r2Bucket");
    r2PublicBaseUrl = document.getElementById("r2PublicBaseUrl");
    r2AccessKeyId = document.getElementById("r2AccessKeyId");
    r2SecretAccessKey = document.getElementById("r2SecretAccessKey");
    r2AccountId = document.getElementById("r2AccountId");
    r2Endpoint = document.getElementById("r2Endpoint");
    assetStatus = document.getElementById("assetStatus");
    assetStatusDetail = document.getElementById("assetStatusDetail");
    editButton = document.getElementById("editAssets");
    cancelButton = document.getElementById("cancelAssets");
    saveButton = document.getElementById("saveAssets");
    editActions = document.getElementById("assetEditActions");
    message = document.getElementById("assetsMessage");

    for (const element of [mermaidWidth, mermaidScale, r2Bucket, r2PublicBaseUrl, r2AccessKeyId, r2SecretAccessKey, r2AccountId, r2Endpoint]) {
      element.addEventListener("input", () => BlogCTLPopup.setMessage(message));
    }
    editButton.addEventListener("click", () => setEditing(true));
    cancelButton.addEventListener("click", () => {
      writeForm();
      setEditing(false);
      BlogCTLPopup.setMessage(message);
    });
    saveButton.addEventListener("click", save);
    setEditing(false);
    state.initialized = true;
  }

  function activate() {
    state.active = true;
    refresh();
  }

  function deactivate() {
    state.active = false;
  }

  root.BlogCTLAssets = { init, activate, deactivate, refresh };
})(globalThis);
