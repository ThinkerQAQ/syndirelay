"use strict";
(function(root) {
  const state={initialized:false,draft:null,platform:"linkedin"};
  let picker,platform,text,media,preview,message,save,stage,reconcile,publish,refresh;
  async function request(type,payload={}) {
    return BlogCTLPopup.send("blogctl.social."+type,payload);
  }
  function note(value,kind="") { BlogCTLPopup.setMessage(message,value,kind); }
  function draftSummary(d) {
    const remotes=Object.entries(d.remote||{}).map(([p,v])=>p+":"+v.state).join(", ");
    return d.id+" · "+(d.approvedRevision===d.revision?"已确认版本":"本地草稿")+(remotes?" · "+remotes:"");
  }
  function render() {
    const d=state.draft;
    const selected=state.platform;
    platform.replaceChildren();
    if(!d){
      preview.textContent="从 ChatGPT 生成草稿后，点击刷新即可在这里查看。";
      text.value="";media.replaceChildren();
      [save,stage,reconcile,publish].forEach(b=>b.disabled=true);
      return;
    }
    for(const key of Object.keys(d.posts||{})){
      const option=document.createElement("option");option.value=key;option.textContent=key;platform.append(option);
    }
    if(!d.posts[selected])state.platform=Object.keys(d.posts)[0];
    platform.value=state.platform;
    const post=d.posts[state.platform];
    text.value=post.text||"";
    media.replaceChildren();
    if(post.mediaUrl){
      const img=document.createElement("img");
      img.src=post.mediaUrl;img.alt=post.mediaAlt||"文章封面";img.loading="lazy";img.className="social-cover";
      media.append(img);
    }
    const suffix=new URL(d.source.url);
    suffix.searchParams.set("utm_campaign","article_syndication");
    suffix.searchParams.set("utm_medium","social");
    suffix.searchParams.set("utm_source",state.platform);
    preview.textContent=post.text+"\n\n"+suffix.toString();
    const remote=d.remote?.[state.platform];
    const blocked=remote&&Boolean(remote.state);
    const staged=d.approvedRevision===d.revision;
    save.disabled=Boolean(blocked);
    stage.disabled=Boolean(blocked);
    publish.disabled=!staged||!remote||remote.state!=="draft";
    reconcile.disabled=!remote?.id;
    note(draftSummary(d)+(remote?.error?" · "+remote.error:""),remote?.error?"error":"");
  }
  async function load() {
    const result=await request("list");
    const all=result.drafts||[];
    const selected=picker.value||state.draft?.id||"";
    picker.replaceChildren();
    const empty=document.createElement("option");empty.value="";empty.textContent="选择社交草稿";picker.append(empty);
    for(const d of all){
      const option=document.createElement("option");
      option.value=d.id;option.textContent=d.id;picker.append(option);
    }
    picker.value=all.some(d=>d.id===selected)?selected:(all[0]?.id||"");
    state.draft=all.find(d=>d.id===picker.value)||null;
    render();
  }
  async function selectDraft(){
    const id=picker.value;
    if(!id){state.draft=null;render();return;}
    const response=await request("get",{id});
    state.draft=response.draft;
    state.platform="linkedin";
    render();
  }
  async function act(action) {
    if(!state.draft)return;
    const draft=state.draft;
    const data={id:draft.id,revision:draft.revision,platform:state.platform};
    if(action==="publish"){
      if(!confirm("确认将已审核的 "+state.platform+" 动态公开发布？此操作不可撤销。"))return;
    }
    if(action==="stage"){
      if(!confirm("确认当前文案并存入 Buffer 私有草稿？这不会公开发布。"))return;
    }
    if(action==="reconcile"){
      // Remote read and local state repair only; never re-attempt publishing.
      note("正在检查 Buffer 实际发布状态…");
    }
    if(action==="save"){
      const posts=structuredClone(draft.posts);
      posts[state.platform].text=text.value;
      data.posts=posts;
    }
    [save,stage,reconcile,publish].forEach(b=>b.disabled=true);
    try{
      const response=await request(action,data);
      state.draft=response.draft;
      note(action+" 成功");
      render();
    }catch(err){
      note(BlogCTLPopup.errorMessage(err),"error");
      render();
    }
  }
  const api={
    init(){
      if(state.initialized)return;
      state.initialized=true;
      picker=document.getElementById("socialDraftPicker");
      platform=document.getElementById("socialPlatform");
      text=document.getElementById("socialText");
      media=document.getElementById("socialMedia");
      preview=document.getElementById("socialPreview");
      message=document.getElementById("socialMessage");
      refresh=document.getElementById("socialRefresh");
      save=document.getElementById("socialSave");
      stage=document.getElementById("socialStage");
      publish=document.getElementById("socialPublish");
      reconcile=document.getElementById("socialReconcile");
      picker.addEventListener("change",()=>selectDraft().catch(e=>note(e.message,"error")));
      platform.addEventListener("change",()=>{state.platform=platform.value;render();});
      refresh.addEventListener("click",()=>load().catch(e=>note(e.message,"error")));
      text.addEventListener("input",()=>{const d=state.draft;if(!d)return;
        const u=new URL(d.source.url);
        u.searchParams.set("utm_campaign","article_syndication");u.searchParams.set("utm_medium","social");u.searchParams.set("utm_source",state.platform);
        preview.textContent=text.value+"\n\n"+u.toString();
        const changed=text.value!==d.posts[state.platform].text;
        save.disabled=!changed;
        if(changed){stage.disabled=true;reconcile.disabled=true;publish.disabled=true;note("文字已修改，请先保存后再存入 Buffer 草稿。");}
      });
      for(const [button,action] of [[save,"save"],[stage,"stage"],[reconcile,"reconcile"],[publish,"publish"]]){
        button.addEventListener("click",()=>act(action).catch(e=>note(e.message,"error")));
      }
    },
    activate(){load().catch(e=>note(e.message,"error"));},
    deactivate(){},
    refresh(){return load();}
  };
  root.BlogCTLSocial=api;
})(globalThis);
