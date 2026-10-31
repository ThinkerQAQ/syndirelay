"use strict";

// One Feature, two intents. Creation is never inferred from missing bindings;
// Update always carries the exact checked remote ID snapshot.
(function (root) {
  const state = {
    initialized:false, active:false, mode:"update", articles:[], selectedSlug:"",
    selectedPlatformIDs:new Set(BlogCTLSyncModel.visiblePlatformIDs(BlogCTLSyncState.loadPlatforms(localStorage))),
    status:null, publishing:[], tools:[], currentJob:null, pollTimer:null,
    createMatches:new Map(), createScanSerial:0, createScanning:new Set(),
    platformSelectionInitialized:BlogCTLSyncState.hasPlatformPreference(localStorage),
  };
  let articlePicker, articleOptions, articleMeta, platformList, actionButton,
      publishButton, nextActions, viewTaskButton, message, allButton, invertButton;

  const article = () => state.articles.find((item) => item.slug === state.selectedSlug);
  const profile = (id) => state.publishing.find((item) => item.id === id) || {};
  const running = () => ["queued","running"].includes(state.currentJob?.state);
  const selectedPlatformIDs = () => [...state.selectedPlatformIDs].filter((id) =>
    BlogCTLSyncModel.isVisiblePlatform(id));

  function availability(platform) {
    const source = BlogCTLSyncModel.platformAvailability(article(),platform,profile(platform.id));
    if (!source.available) return source;
    const dependency = BlogCTLSyncModel.deliveryToolAvailability(platform,state.tools);
    if (!dependency.available) return dependency;
    if (state.mode==="create" && !platform.capabilities?.draftCreate) {
      return {available:false,reason:"不支持创建草稿"};
    }
    if (state.mode==="update" && !platform.capabilities?.remoteList) {
      return {available:false,reason:"目前没有远端关联检测能力"};
    }
    return {available:true,reason:""};
  }

  function clearTask() {
    if (running()) return;
    if (state.pollTimer) clearTimeout(state.pollTimer);
    state.pollTimer=null;
    state.currentJob=null;
    BlogCTLPopup.setMessage(message);
  }

  function candidateTargets() {
    return (root.BlogCTLSync?.selectedTargets?.() || [])
      .filter((target) => state.selectedPlatformIDs.has(target.platform));
  }

  // A Create page is a view over current remote articles, not a second
  // binding database. Match by the selected local article using the same
  // read-only discovery contract as Update.
  function resetCreateMatches() {
    ++state.createScanSerial;
    state.createScanning.clear();
    state.createMatches.clear();
  }

  function createdTaskEntries(platformID) {
    if (!state.currentJob || state.currentJob.operation !== "create" ||
        state.currentJob.article !== state.selectedSlug || state.currentJob.dryRun) return [];
    const links=BlogCTLSyncModel.taskArtifactLinks(state.currentJob,platformID);
    const entries=links.map((link)=>({
      title:article()?.title || "远端文章",
      id:link.id,
      published:link.state==="published",
      url:link.url,
      link,
    }));
    // Successful writes may be returned without a usable editor URL. Keep
    // the existence signal, so that a missing link never enables duplicate
    // creation of the same article.
    for (const event of state.currentJob.events || []) {
      if (event.platform!==platformID ||
          !["draft-created","published","draft-updated","published-updated","published-edit-saved"].includes(event.result)) continue;
      const id=String(event.targetId || "");
      if (entries.some((item)=>id && item.id===id)) continue;
      entries.push({
        title:article()?.title || "远端文章", id,
        published:event.result.startsWith("published"), url:event.url || "",
        link:null,
      });
    }
    return entries;
  }

  function existingCreateArticles(platformID) {
    const result=state.createMatches.get(platformID);
    const byId=new Map();
    for (const item of result?.items || []) {
      const id=String(item.id || "");
      if (!id) continue;
      byId.set(id+":"+Boolean(item.published),item);
    }
    for (const item of createdTaskEntries(platformID)) {
      const key=String(item.id)+":"+Boolean(item.published);
      if (!byId.has(key)) byId.set(key,item);
    }
    return [...byId.values()];
  }

  function resumableHashnodeDraft(id) {
    const existing=existingCreateArticles(id);
    return id==="hashnode" && existing.length===1 && existing[0].id &&
      existing[0].bound===true && !existing[0].published;
  }

  function eligibleCreatePlatforms() {
    return selectedPlatformIDs().filter((id)=>{
      const platform=state.status?.platforms?.find((p)=>p.id===id);
      return platform && availability(platform).available &&
        (!existingCreateArticles(id).length || resumableHashnodeDraft(id));
    });
  }

  async function scanCreateMatches(platformIDs) {
    if (state.mode!=="create" || !state.selectedSlug) return;
    const articleSlug=state.selectedSlug;
    const serial=state.createScanSerial;
    const platforms=platformIDs.filter((id)=>!state.createScanning.has(id) &&
      (!state.createMatches.has(id) || state.createMatches.get(id).error));
    if (!platforms.length) return;
    for (const id of platforms) state.createScanning.add(id);
    renderPlatforms();
    // Bounded concurrency avoids overwhelming browser sessions or Bridge.
    const workers=Array.from({length:Math.min(3,platforms.length)},async(_,index)=>{
      for (let i=index;i<platforms.length;i+=Math.min(3,platforms.length)) {
        const id=platforms[i];
        let result;
        try {
          const response=await BlogCTLPopup.send("blogctl.article.match",{article:articleSlug,platform:id});
          result={items:(response.match?.items || []).filter((item)=>item?.id)};
        } catch(error) {
          result={items:[],error:BlogCTLPopup.errorMessage(error)};
        }
        if (state.createScanSerial!==serial || state.selectedSlug!==articleSlug) return;
        state.createMatches.set(id,result);
        state.createScanning.delete(id);
        if (state.active && state.mode==="create") renderPlatforms();
      }
    });
    await Promise.all(workers);
  }

  function createDetectablePlatforms() {
    return BlogCTLSyncModel.visiblePlatforms(state.status?.platforms || [])
      .filter((platform)=>state.selectedPlatformIDs.has(platform.id) &&
        availability(platform).available)
      .map((platform)=>platform.id);
  }

  function refreshCreateMatches(platformIDs = createDetectablePlatforms()) {
    if(state.mode!=="create" || !state.active || !state.selectedSlug ||
        !state.status || running()) return;
    const permitted=new Set(createDetectablePlatforms());
    const candidates=[...new Set(platformIDs)].filter((id)=>
      permitted.has(id) && !state.createScanning.has(id));
    for(const id of candidates) state.createMatches.delete(id);
    void scanCreateMatches(candidates);
    renderPlatforms();
  }

  function ensureUpdateScan() {
    if(state.mode!=="update" || !state.active || !state.selectedSlug || !state.status)return;
    const candidates=BlogCTLSyncModel.visiblePlatforms(state.status.platforms)
      .filter((platform)=>state.selectedPlatformIDs.has(platform.id) && availability(platform).available)
      .map((platform)=>platform.id);
    void root.BlogCTLSync?.ensureMatches?.(candidates);
  }

  function renderExistingCreateArticles(platform,card,items) {
    const body=document.createElement("div");
    body.className="created-remote-items";
    for(const item of items){
      const row=document.createElement("div");
      row.className="article-match-row created-remote-row";
      const identity=document.createElement("div");
      identity.className="inventory-article-content";
      const title=document.createElement("strong");
      title.className="inventory-article-title";
      if (item.title || article()?.title) title.setAttribute("data-i18n-ignore", "");
      title.textContent=item.title || article()?.title || "(无标题)";
      const meta=document.createElement("div");
      meta.className="inventory-article-meta";
      const stateLabel=document.createElement("span");
      stateLabel.className=item.published?"inventory-article-state is-published":
        "inventory-article-state is-draft";
      stateLabel.textContent=item.published?"已发布":"草稿";
      const id=document.createElement("span");
      id.className="inventory-article-id";
      id.textContent=item.id?"ID "+item.id:"远端已创建";
      meta.append(stateLabel,id);
      identity.append(title,meta);
      row.append(identity);
      const link=item.link || BlogCTLSyncModel.articleMatchLink(platform.id,item);
      if(link){
        const anchor=document.createElement("a");
        anchor.className="inventory-article-action";
        anchor.href=link.url;
        anchor.textContent=link.label;
        anchor.target="_blank";
        anchor.rel="noopener noreferrer";
        row.append(anchor);
      } else {
        const unavailable=document.createElement("span");
        unavailable.className="card-hint";
        unavailable.textContent="远端编辑链接暂不可用";
        row.append(unavailable);
      }
      body.append(row);
    }
    card.append(body);
  }

  function canPublishAfter() {
    const platforms = state.mode === "create" ? eligibleCreatePlatforms() :
      [...new Set(candidateTargets().map((target) => target.platform))];
    if (!platforms.length || platforms.some((id) =>
      !state.status?.platforms?.find((p) => p.id===id)?.capabilities?.explicitPublish)) return false;
    return state.mode==="create" || candidateTargets().every((target) => target.state==="draft");
  }

  function updateAction() {
    const platforms = selectedPlatformIDs().filter((id) => {
      const platform=state.status?.platforms?.find((p)=>p.id===id);
      return platform && availability(platform).available;
    });
    const targets = candidateTargets();
    const canSubmit = Boolean(state.selectedSlug && state.status?.bridge?.running) &&
      !running() && (state.mode==="create" || !(root.BlogCTLSync?.isBindingBusy?.() ?? false)) &&
      (state.mode==="create" ? eligibleCreatePlatforms().length>0 : targets.length>0);
    actionButton.disabled=!canSubmit;
    publishButton.disabled=!canSubmit || !canPublishAfter();
    if(state.mode==="create"){
      const detect=document.getElementById("refreshArticleMatches");
      detect.disabled=!state.selectedSlug || !state.status?.bridge?.running ||
        running() || state.createScanning.size>0 || !createDetectablePlatforms().length;
      detect.title="检测勾选平台是否已有对应文章";
    }
    actionButton.textContent=state.mode==="create"?"创建草稿":"更新所选";
    publishButton.textContent=state.mode==="create"?"创建并发布":"更新并发布";
    const terminal=["completed","failed"].includes(state.currentJob?.state);
    nextActions.hidden=!terminal;
    // Completion is recorded in Tasks; allow the next explicit operation.
    // Re-running CREATE still requires a fresh explicit confirmation.
  }

  // Native <datalist> supplies keyboard selection, popup positioning and
  // accessibility instead of the former custom, partially ARIA-compliant listbox.
  function populateArticleOptions() {
    articleOptions.replaceChildren();
    for (const item of state.articles) {
      const option = document.createElement("option");
      option.value = `${item.title} · ${item.slug}`;
      option.label = item.slug;
      articleOptions.append(option);
    }
  }

  function selectArticle(item) {
    if(running())return;
    clearTask();
    if (state.selectedSlug!==item.slug) resetCreateMatches();
    state.selectedSlug=item.slug;
    articlePicker.value=`${item.title} · ${item.slug}`;
    localStorage.setItem("blogctl.selectedArticle",item.slug);
    articleMeta.setAttribute("data-i18n-ignore", "");
    articleMeta.textContent=`${item.title} · ${item.slug}`;
    document.dispatchEvent(new CustomEvent("blogctl:article-selected",{
      detail:{article:item.slug},
    }));
    renderPlatforms();
    ensureUpdateScan();
  }

  function renderPlatforms() {
    platformList.replaceChildren();
    for(const platform of BlogCTLSyncModel.visiblePlatforms(state.status?.platforms || [])) {
      const card=document.createElement("div");
      card.className="platform-choice-card";
      card.dataset.platformCard=platform.id;
      const header=document.createElement("div");
      header.className="platform-choice";
      const check=document.createElement("input");
      check.type="checkbox";
      check.dataset.platform=platform.id;
      check.checked=state.selectedPlatformIDs.has(platform.id);
      const permission=availability(platform);
      check.disabled=!state.selectedSlug || running() || !permission.available ||
        (state.mode==="update" && (root.BlogCTLSync?.isBindingBusy?.() ?? false));
      check.addEventListener("change",()=>{
        if(check.checked)state.selectedPlatformIDs.add(platform.id);
        else state.selectedPlatformIDs.delete(platform.id);
        BlogCTLSyncState.savePlatforms(localStorage,state.selectedPlatformIDs);
        root.BlogCTLSync?.refresh?.();
        renderPlatforms();
        document.dispatchEvent(new CustomEvent("blogctl:update-platform-selection"));
        ensureUpdateScan();
      });
      const description=document.createElement("span");
      description.className="platform-choice-text";
      const name=document.createElement("strong");
      name.textContent=platform.label || platform.id;
      const hint=document.createElement("small");
      hint.textContent=state.mode==="create"
        ? (existingCreateArticles(platform.id).length ? "已找到对应远端文章" : "创建新的远端草稿")
        : "选中检测到的远端文章作为更新目标";
      description.append(name,hint);
      const status=document.createElement("strong");
      const alreadyExists=state.mode==="create" && existingCreateArticles(platform.id).length>0;
      BlogCTLPopup.setStatus(status,permission.available?"ok":"disabled",
        permission.available?(alreadyExists?"已有文章":"可操作"):permission.reason);
      const actions=document.createElement("div");
      actions.className="platform-card-actions";
      const existing=state.mode==="create" ? existingCreateArticles(platform.id) : [];
      const scanning=state.mode==="create" && state.createScanning.has(platform.id);
      if(state.mode==="create"){
        const detect=document.createElement("button");
        detect.type="button";
        detect.className="secondary compact";
        detect.textContent="检测关联";
        detect.title="检测此平台是否已有对应文章";
        detect.disabled=!permission.available || !state.selectedSlug || running() ||
          state.createScanning.has(platform.id);
        detect.addEventListener("click",()=>{
          state.selectedPlatformIDs.add(platform.id);
          BlogCTLSyncState.savePlatforms(localStorage,state.selectedPlatformIDs);
          refreshCreateMatches([platform.id]);
        });
        actions.append(detect);
        if(!existing.length || resumableHashnodeDraft(platform.id)){
          const create=document.createElement("button");
        create.type="button";
        create.className="secondary compact";
        create.textContent=existing.length ? "继续同步草稿" : "创建草稿";
        create.disabled=!permission.available || !state.selectedSlug || running();
        create.addEventListener("click",()=>startExplicit([platform.id],[],false));
          actions.append(create);
        }
      } else {
        const detect=document.createElement("button");
        detect.type="button";
        detect.className="secondary compact";
        detect.textContent="检测关联";
        detect.disabled=!permission.available || !state.selectedSlug || running() ||
          (root.BlogCTLSync?.isBindingBusy?.() ?? false);
        detect.addEventListener("click", () => {
          state.selectedPlatformIDs.add(platform.id);
          BlogCTLSyncState.savePlatforms(localStorage,state.selectedPlatformIDs);
          renderPlatforms();
          root.BlogCTLSync?.refreshArticleMatches?.([platform.id]);
        });
        actions.append(detect);
      }
      header.append(check, description, actions, status);
      card.append(header);
      if(state.mode==="create"){
        if(existing.length) renderExistingCreateArticles(platform,card,existing);
        else if(scanning){
          const hint=document.createElement("p");
          hint.className="create-detection-hint";
          hint.textContent="正在检查远端是否已有对应文章…";
          card.append(hint);
        } else if(state.createMatches.get(platform.id)?.error){
          const hint=document.createElement("p");
          hint.className="create-detection-hint error-text";
          hint.textContent="远端检查失败："+state.createMatches.get(platform.id).error;
          card.append(hint);
          const retry=document.createElement("button");
          retry.type="button";
          retry.className="secondary compact create-detection-retry";
          retry.textContent="检测";
          retry.addEventListener("click",()=>void scanCreateMatches([platform.id]));
          card.append(retry);
        }
      }
      if(state.mode==="update")root.BlogCTLSync?.appendPlatformMatches?.(platform,card);
      const result=state.currentJob?.results?.[platform.id];
      // Successful target-level task events are represented by the article
      // row above. Do not also show the old machine-readable "explicit-create"
      // result footer (or a second Create button).
      const hasCreatedArticle=state.mode==="create" && existing.length>0;
      if(result && (result.state==="failed" || running() || !hasCreatedArticle)){
        const footer=document.createElement("div");
        footer.className="platform-task-status";
        const statusName=document.createElement("strong");
        statusName.textContent=result.result || result.state;
        const details=document.createElement("small");
        details.textContent=result.error || result.message || "";
        footer.append(statusName,details);
        card.append(footer);
      }
      platformList.append(card);
    }
    updateAction();
  }

  function selectAll(invert) {
    if(!state.selectedSlug || running())return;
    for(const platform of BlogCTLSyncModel.visiblePlatforms(state.status?.platforms || [])) {
      if(!availability(platform).available)continue;
      if(invert && state.selectedPlatformIDs.has(platform.id))state.selectedPlatformIDs.delete(platform.id);
      else state.selectedPlatformIDs.add(platform.id);
    }
    BlogCTLSyncState.savePlatforms(localStorage,state.selectedPlatformIDs);
    renderPlatforms();
    document.dispatchEvent(new CustomEvent("blogctl:update-platform-selection"));
    ensureUpdateScan();
  }

  function validateTargets(targets) {
    if(!targets.length)throw new Error("请先检测并勾选至少一篇远端文章。更新不会自动创建草稿。");
    if(targets.some((t)=>t.state==="published"&&
      !["cnblogs","devto","toutiao"].includes(t.platform) &&
      !(t.platform==="medium" && state.status?.platforms?.find((p)=>p.id===t.platform)?.capabilities?.publishedDraftEdit===true)))
      throw new Error("所选平台的已发布文章暂不支持安全更新。");
    if(targets.some((t)=>!state.selectedPlatformIDs.has(t.platform)))
      throw new Error("目标所属平台未勾选。");
  }

  async function startExplicit(platforms,targets,publishAfter=false) {
    if(!state.selectedSlug||running()||!platforms.length)return;
    try {
      if(state.mode==="update")validateTargets(targets);
      if(state.mode==="create" && platforms.some((id)=>
          existingCreateArticles(id).length && !resumableHashnodeDraft(id))){
        throw new Error("远端已有文章；请先确认，不要重复创建。");
      }
      if(publishAfter && (state.mode==="update"&&targets.some((t)=>t.state==="published")))
        throw new Error("已发布文章无需再次发布，请使用「更新所选」。");
      if(publishAfter && !canPublishAfter())throw new Error("有平台不支持直接发布草稿。");
      if(!window.confirm(state.mode==="create"
        ? `将在 ${platforms.length} 个平台创建或继续同步草稿${publishAfter?"并发布":""}，确认？`
        : `将覆盖 ${targets.length} 篇所选远端文章${publishAfter?"并发布草稿":""}，确认？`))return;
      const request={
        article:state.selectedSlug,platforms,operation:state.mode,
        targets:state.mode==="create"?[]:targets.map((item)=>({...item})),
        publishAfter,dryRun:false,
      };
      actionButton.disabled=true;
      publishButton.disabled=true;
      BlogCTLPopup.setMessage(message,"正在启动任务…");
      const response=await BlogCTLPopup.send("blogctl.job.start",{request});
      state.currentJob=response.job;
      renderPlatforms();
      BlogCTLPopup.setMessage(message,"已启动任务，具体结果请查看任务页。","ok");
      if(state.currentJob?.id)pollJob(state.currentJob.id);
    } catch(error) {
      BlogCTLPopup.setMessage(message,BlogCTLPopup.errorMessage(error),"error");
      updateAction();
    }
  }

  async function pollJob(id) {
    if(state.pollTimer)clearTimeout(state.pollTimer);
    if(!state.active)return;
    try {
      const response=await BlogCTLPopup.send("blogctl.job.get",{id});
      state.currentJob=response.job;
      renderPlatforms();
      if(["completed","failed"].includes(state.currentJob?.state)){
        // Real remote objects are recorded in job events. Show their direct
        // editor links immediately without starting remote detection.
        BlogCTLPopup.setMessage(message,state.currentJob.state==="completed"?
          "任务完成，结果已记录。":"部分或全部平台失败，请在任务页查看具体远端 ID 和错误。",
          state.currentJob.state==="completed"?"ok":"error");
        return;
      }
    }catch(error){
      BlogCTLPopup.setMessage(message,BlogCTLPopup.errorMessage(error),"error");
    }
    state.pollTimer=setTimeout(()=>pollJob(id),1400);
  }

  async function refresh() {
    if(!state.active)return;
    try {
      const [articles,status,publishing,tools]=await Promise.all([
        BlogCTLPopup.send("blogctl.articles"),
        BlogCTLPopup.send("blogctl.status"),
        BlogCTLPopup.send("blogctl.publishing"),
        BlogCTLPopup.send("blogctl.tools"),
      ]);
      state.articles=articles.articles||[];
      populateArticleOptions();
      state.status=status.status;
      if(!state.platformSelectionInitialized) {
        // All available platforms on the first visit, but preserve subsequent
        // user choices (including explicitly selecting none).
        state.selectedPlatformIDs=new Set(
          BlogCTLSyncModel.visiblePlatforms(state.status?.platforms || []).map((p)=>p.id)
        );
        state.platformSelectionInitialized=true;
        BlogCTLSyncState.savePlatforms(localStorage,state.selectedPlatformIDs);
      }
      state.publishing=publishing.platforms||[];
      state.tools=tools.tools||[];
      const previous=state.selectedSlug || localStorage.getItem("blogctl.selectedArticle") || "";
      const found=state.articles.find((item)=>item.slug===previous);
      if(found){
        if(state.selectedSlug && state.selectedSlug!==found.slug)resetCreateMatches();
        state.selectedSlug=found.slug;
        articlePicker.value=`${found.title} · ${found.slug}`;
        articleMeta.textContent=`${found.title} · ${found.slug}`;
      } else {
        state.selectedSlug="";
        articleMeta.textContent="";
      }
      document.dispatchEvent(new CustomEvent("blogctl:article-selected",{
        detail:{article:state.selectedSlug},
      }));
      renderPlatforms();
      ensureUpdateScan();
      if(running()&&state.currentJob?.id)pollJob(state.currentJob.id);
      BlogCTLPopup.refreshBridgeIndicator(state.status).catch(()=>{});
    } catch(error) {
      BlogCTLPopup.setMessage(message,BlogCTLPopup.errorMessage(error),"error");
    }
  }

  function setMode(mode) {
    if(mode!=="create"&&mode!=="update")return;
    if (state.mode!==mode) clearTask();
    state.mode=mode;
    document.getElementById("draftModeTitle").textContent=mode==="create"?"创建":"更新";
    document.getElementById("draftModeHint").textContent=mode==="create"?
      "选择本地文章和平台，明确创建一篇新草稿，不读取历史绑定。":
      "选择本地文章，检测并勾选远端目标，然后直接更新。";
    document.getElementById("draftPlatformHint").textContent=mode==="create"?
      "可手动检测勾选平台的文章关联；检测不影响创建操作。":
      "自动检测已选平台的远端文章；勾选目标后直接更新。可手动重新检测。";
    const detect=document.getElementById("refreshArticleMatches");
    detect.hidden=false;
    detect.textContent=mode==="create"?"检测":"重新检测";
    root.BlogCTLSync?.clearMatches?.();
    if(state.initialized)renderPlatforms();
    ensureUpdateScan();
  }

  function init() {
    if(state.initialized)return;
    articlePicker=document.getElementById("draftArticlePicker");
    articleOptions=document.getElementById("draftArticleOptions");
    articleMeta=document.getElementById("draftArticleMeta");
    platformList=document.getElementById("draftPlatforms");
    actionButton=document.getElementById("saveDrafts");
    publishButton=document.getElementById("publishAfterSave");
    nextActions=document.getElementById("draftNextActions");
    viewTaskButton=document.getElementById("draftViewTask");
    message=document.getElementById("draftsMessage");
    allButton=document.getElementById("selectAllDraftPlatforms");
    invertButton=document.getElementById("invertDraftPlatforms");
    articlePicker.addEventListener("input", () => {
      if (running()) return;
      const value = articlePicker.value.trim();
      const chosen = state.articles.find((item) =>
        value === `${item.title} · ${item.slug}` || value === item.slug);
      if (chosen) {
        selectArticle(chosen);
        return;
      }
      clearTask();
      resetCreateMatches();
      state.selectedSlug = "";
      articleMeta.textContent = "";
      document.dispatchEvent(new CustomEvent("blogctl:article-selected", {
        detail: { article: "" },
      }));
      renderPlatforms();
    });
    allButton.addEventListener("click",()=>selectAll(false));
    invertButton.addEventListener("click",()=>selectAll(true));
    actionButton.addEventListener("click",()=>{
      const targets=candidateTargets();
      const platforms=state.mode==="create"?eligibleCreatePlatforms():
        [...new Set(targets.map((t)=>t.platform))];
      startExplicit(platforms,targets,false);
    });
    publishButton.addEventListener("click",()=>{
      const targets=candidateTargets();
      const platforms=state.mode==="create"?eligibleCreatePlatforms():
        [...new Set(targets.map((t)=>t.platform))];
      startExplicit(platforms,targets,true);
    });
    viewTaskButton.addEventListener("click",()=>{
      if(state.currentJob?.id)document.dispatchEvent(new CustomEvent("blogctl:navigate-task",{
        detail:{jobId:state.currentJob.id},
      }));
    });
    document.addEventListener("blogctl:association-results-changed",()=>{
      if(state.active&&state.mode==="update")renderPlatforms();
    });
    state.initialized=true;
  }
  function activate(){state.active=true;refresh();}
  function deactivate(){
    state.active=false;
    if(state.pollTimer)clearTimeout(state.pollTimer);
    state.pollTimer=null;
  }

  root.BlogCTLDrafts={
    init,activate,deactivate,refresh,setMode,
    selectedPlatformIDs,
    currentMode:()=>state.mode,
    refreshCreateMatches,
    isJobRunning:running,
    selectedArticleTitle:()=>article()?.title || "",
    selectionChanged:updateAction,
    updateTargets:(targets,publishAfter=false)=>{
      for (const target of targets) state.selectedPlatformIDs.add(target.platform);
      BlogCTLSyncState.savePlatforms(localStorage,state.selectedPlatformIDs);
      return startExplicit([...new Set(targets.map((item)=>item.platform))],targets,publishAfter);
    },
  };
})(globalThis);
