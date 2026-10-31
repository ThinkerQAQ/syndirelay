"use strict";

(function (root) {
  const state = { initialized: false, active: false, editing: false, platforms: [], tools: [] };
  let platformSelect, languageSelect, changedOnly, footerEnabled, footerTemplate, canonicalMode;
  let trackingEnabled, trackingSource, trackingMedium, trackingCampaign;
  let platformAccessConfig, preview, editButton, cancelButton, saveButton, resetButton, editActions, message;

  function currentPlatform() {
    return state.platforms.find((platform) => platform.id === platformSelect.value);
  }

  function healthKind(health) {
    if (health?.status === "disabled") return "disabled";
    if (health?.ok) return "ok";
    if (health?.status === "missing" || health?.status === "error") return "error";
    return "unknown";
  }

  function accessToolFor(platformID) {
    if (platformID === "devto") return state.tools.find((tool) => tool.name === "devto-api") ?? null;
    return null;
  }

  function makeAccessInput(tool, field) {
    const label = document.createElement("label");
    label.className = "field";
    const title = document.createElement("span");
    title.textContent = field.label || field.key;
    const input = document.createElement("input");
    input.dataset.configKey = field.key;
    input.type = field.type === "secret" ? "password" : field.type === "integer" ? "number" : "text";
    if (field.type === "secret") input.autocomplete = "off";
    if (field.placeholder) input.placeholder = field.placeholder;
    if (field.min) input.min = String(field.min);
    if (field.max) input.max = String(field.max);
    input.value = tool.config?.values?.[field.key] ?? "";
    input.disabled = !state.editing;
    label.append(title, input);
    if (field.description) {
      const hint = document.createElement("small");
      hint.className = "field-hint";
      hint.textContent = field.description;
      label.append(hint);
    }
    return label;
  }

  async function saveAccessTool(tool, container) {
    const values = {};
    container.querySelectorAll("[data-config-key]").forEach((input) => {
      values[input.dataset.configKey] = input.type === "number" ? Number(input.value || 0) : input.value.trim();
    });
    const response = await BlogCTLPopup.send("blogctl.tool.save", { name: tool.name, config: values });
    state.tools = response.tools ?? state.tools;
  }

  function renderPlatformAccess() {
    platformAccessConfig.replaceChildren();
    const platform = currentPlatform();
    if (!platform) {
      platformAccessConfig.innerHTML = '<div class="platform-loading">请选择平台</div>';
      return;
    }

    const tool = accessToolFor(platform.id);
    if (!tool) {
      const note = document.createElement("p");
      note.className = "card-hint";
      note.textContent = platform.capabilities?.browserSession
        ? "此平台使用浏览器登录会话，不需要额外的平台凭据配置。登录状态在“绑定”页查看。"
        : "当前平台没有额外的接入配置。";
      platformAccessConfig.append(note);
      return;
    }

    const card = document.createElement("div");
    card.className = "platform-access-card";
    const head = document.createElement("div");
    head.className = "status-row";
    const name = document.createElement("strong");
    name.textContent = tool.displayName || tool.name;
    const status = document.createElement("span");
    BlogCTLPopup.setStatus(status, healthKind(tool.health), tool.health?.summary || tool.health?.status || "未知");
    head.append(name, status);
    card.append(head);

    if (tool.description) {
      const description = document.createElement("p");
      description.className = "card-hint";
      description.textContent = tool.description;
      card.append(description);
    }

    for (const field of tool.config?.schema ?? []) card.append(makeAccessInput(tool, field));

    platformAccessConfig.append(card);
  }

  function setEditing(editing) {
    state.editing = editing;
    const platform = currentPlatform();
    const canUpdateDraft = platform?.capabilities?.draftUpdate !== false;
    languageSelect.disabled = !editing;
    changedOnly.disabled = !editing || !canUpdateDraft;
    footerEnabled.disabled = !editing;
    footerTemplate.disabled = !editing;
    canonicalMode.disabled = !editing;
    trackingEnabled.disabled = !editing;
    trackingSource.disabled = !editing;
    trackingMedium.disabled = !editing;
    trackingCampaign.disabled = !editing;
    editButton.hidden = editing;
    editActions.hidden = !editing;
    renderPlatformAccess();
  }

  function defaultFooterTemplate(language) {
    return language === "en"
      ? "> This article was first published on [{site}]({url}) and syndicated here by the author. The original article may be revised over time; please refer to the personal blog for the latest version."
      : "> 本文首发于 [{site}]({url})，由作者本人同步发布。原文可能持续修订，最新版本请以个人博客为准。";
  }

  function defaultsFor(platform) {
    const english = platform === "devto" || platform === "medium";
    return {
      language: english ? "en" : "zh-CN",
      changedOnly: false,
      footer: {
        enabled: true,
        template: defaultFooterTemplate(english ? "en" : "zh-CN"),
      },
      canonical: { mode: english ? "native" : "footer" },
      tracking: {
        enabled: true,
        source: platform,
        medium: "referral",
        campaign: "article_syndication",
      },
    };
  }

  function trackedUrl() {
    const prefix = languageSelect.value === "en" ? "/en/articles/" : "/articles/";
    const url = new URL(`https://thinkerqaq.com${prefix}example/`);
    if (!trackingEnabled.checked) return url.toString();
    const values = {
      utm_source: trackingSource.value.trim(),
      utm_medium: trackingMedium.value.trim(),
      utm_campaign: trackingCampaign.value.trim(),
    };
    for (const [key, value] of Object.entries(values)) {
      if (value) url.searchParams.set(key, value);
    }
    return url.toString();
  }

  function updatePreview() {
    const canonicalLabels = {
      native: "Native canonical",
      footer: "Footer backlink",
      none: "Disabled",
    };
    const languageLabel = languageSelect.value === "en" ? "English" : "中文";
    const lines = [`内容语言: ${languageLabel}`, `Canonical: ${canonicalLabels[canonicalMode.value] || canonicalMode.value}`];
    if (!footerEnabled.checked) {
      lines.push("Footer: 已关闭");
      preview.textContent = lines.join("\n\n");
      return;
    }
    const english = languageSelect.value === "en";
    const rendered = footerTemplate.value
      .replaceAll("{site}", english ? "ThinkerQAQ's personal blog" : "ThinkerQAQ 的个人博客")
      .replaceAll("{title}", english ? "Example article" : "示例文章")
      .replaceAll("{url}", trackedUrl())
      .trim();
    lines.push(rendered || "Footer 模板为空。");
    preview.textContent = lines.join("\n\n");
  }

  function writeForm(platform) {
    const profile = platform || {};
    const fallback = defaultsFor(profile.id || "cnblogs");
    languageSelect.value = profile.language || fallback.language;
    const canUpdateDraft = profile.capabilities?.draftUpdate !== false;
    changedOnly.checked = canUpdateDraft && Boolean(profile.changedOnly);
    changedOnly.disabled = !state.editing || !canUpdateDraft;
    changedOnly.title = !canUpdateDraft ? "当前平台尚未验证安全更新已有草稿。" : "";
    const footer = profile.footer || fallback.footer;
    const canonical = profile.canonical || fallback.canonical;
    const tracking = profile.tracking || fallback.tracking;
    footerEnabled.checked = Boolean(footer.enabled);
    footerTemplate.value = footer.template || "";
    canonicalMode.value = canonical.mode || fallback.canonical.mode;
    trackingEnabled.checked = Boolean(tracking.enabled);
    trackingSource.value = tracking.source || "";
    trackingMedium.value = tracking.medium || "";
    trackingCampaign.value = tracking.campaign || "";
    updatePreview();
  }

  function readForm() {
    const platform = currentPlatform();
    if (!platform) return null;
    return {
      id: platform.id,
      label: platform.label,
      language: languageSelect.value,
      changedOnly: changedOnly.checked,
      footer: {
        enabled: footerEnabled.checked,
        template: footerTemplate.value.trim(),
      },
      canonical: { mode: canonicalMode.value },
      tracking: {
        enabled: trackingEnabled.checked,
        source: trackingSource.value.trim(),
        medium: trackingMedium.value.trim(),
        campaign: trackingCampaign.value.trim(),
      },
    };
  }

  function renderPlatformSelect() {
    const previous = platformSelect.value;
    platformSelect.replaceChildren();
    for (const platform of state.platforms) {
      const option = document.createElement("option");
      option.value = platform.id;
      option.textContent = platform.label || platform.id;
      platformSelect.append(option);
    }
    if (state.platforms.some((platform) => platform.id === previous)) platformSelect.value = previous;
    if (!platformSelect.value && state.platforms.length) platformSelect.value = state.platforms[0].id;
    writeForm(currentPlatform());
    renderPlatformAccess();
  }

  async function save() {
    const current = readForm();
    if (!current) return;
    if (current.footer.enabled && !current.footer.template) {
      BlogCTLPopup.setMessage(message, "启用 Footer 时模板不能为空。", "error");
      return;
    }
    if (current.tracking.enabled && !current.tracking.source) {
      BlogCTLPopup.setMessage(message, "启用 Tracking 时 Source 不能为空。", "error");
      return;
    }
    saveButton.disabled = true;
    BlogCTLPopup.setMessage(message, "正在保存平台配置…");
    try {
      const accessTool = accessToolFor(current.id);
      if (accessTool) await saveAccessTool(accessTool, platformAccessConfig);
      const response = await BlogCTLPopup.send("blogctl.publishing.save", {
        platforms: [current],
      });
      state.platforms = response.platforms ?? [];
      renderPlatformSelect();
      platformSelect.value = current.id;
      writeForm(currentPlatform());
      setEditing(false);
      BlogCTLPopup.setMessage(message, `${current.label || current.id} 平台配置已保存。`, "ok");
    } catch (error) {
      BlogCTLPopup.setMessage(message, BlogCTLPopup.errorMessage(error), "error");
    } finally {
      saveButton.disabled = false;
    }
  }

  function reset() {
    const platform = currentPlatform();
    if (!platform) return;
    writeForm({ id: platform.id, label: platform.label, ...defaultsFor(platform.id) });
    BlogCTLPopup.setMessage(message, "已恢复默认值；点击“保存”后写入本机配置。");
  }

  async function refresh() {
    if (!state.active) return;
    BlogCTLPopup.setMessage(message);
    try {
      const [response, toolsResponse] = await Promise.all([
        BlogCTLPopup.send("blogctl.publishing"),
        BlogCTLPopup.send("blogctl.tools"),
      ]);
      state.platforms = response.platforms ?? [];
      state.tools = toolsResponse.tools ?? [];
      state.editing = false;
      renderPlatformSelect();
      setEditing(false);
      await BlogCTLPopup.refreshBridgeIndicator();
    } catch (error) {
      BlogCTLPopup.setMessage(message, BlogCTLPopup.errorMessage(error), "error");
    }
  }

  function init() {
    if (state.initialized) return;
    platformSelect = document.getElementById("publishingPlatform");
    platformAccessConfig = document.getElementById("platformAccessConfig");
    languageSelect = document.getElementById("publishingLanguage");
    changedOnly = document.getElementById("publishingChangedOnly");
    footerEnabled = document.getElementById("footerEnabled");
    footerTemplate = document.getElementById("footerTemplate");
    canonicalMode = document.getElementById("canonicalMode");
    trackingEnabled = document.getElementById("trackingEnabled");
    trackingSource = document.getElementById("trackingSource");
    trackingMedium = document.getElementById("trackingMedium");
    trackingCampaign = document.getElementById("trackingCampaign");
    preview = document.getElementById("publishingPreview");
    editButton = document.getElementById("editPublishing");
    cancelButton = document.getElementById("cancelPublishing");
    saveButton = document.getElementById("savePublishing");
    resetButton = document.getElementById("resetPublishing");
    editActions = document.getElementById("publishingEditActions");
    message = document.getElementById("publishingMessage");

    platformSelect.addEventListener("change", () => {
      state.editing = false;
      writeForm(currentPlatform());
      setEditing(false);
    });
    languageSelect.addEventListener("change", () => {
      const currentTemplate = footerTemplate.value.trim();
      const defaultTemplates = new Set([defaultFooterTemplate("zh-CN"), defaultFooterTemplate("en")]);
      if (defaultTemplates.has(currentTemplate)) {
        footerTemplate.value = defaultFooterTemplate(languageSelect.value);
      }
      updatePreview();
    });
    for (const element of [footerEnabled, footerTemplate, canonicalMode, trackingEnabled, trackingSource, trackingMedium, trackingCampaign]) {
      element.addEventListener(element.tagName === "SELECT" || element.type === "checkbox" ? "change" : "input", updatePreview);
    }
    editButton.addEventListener("click", () => setEditing(true));
    cancelButton.addEventListener("click", () => {
      writeForm(currentPlatform());
      setEditing(false);
      BlogCTLPopup.setMessage(message);
    });
    saveButton.addEventListener("click", save);
    resetButton.addEventListener("click", reset);
    setEditing(false);
    state.initialized = true;
  }

  function activate() { state.active = true; refresh(); }
  function deactivate() { state.active = false; }
  root.BlogCTLPublishing = { init, activate, deactivate, refresh };
})(globalThis);
