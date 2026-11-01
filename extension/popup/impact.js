"use strict";
(function(root){
 let form,status,events,initialized=false;
 const tr=(zh,en)=>root.BlogCTLI18n?.locale==="en"?en:zh;
 function note(text,error=false){status.textContent=text;status.className=error?"message error":"message";}
 async function refresh(){
  try{
   const result=await BlogCTLPopup.send("blogctl.impact.get");
   const c=result.config||{};
   for(const key of ["enabled","endpoint","tokenFile","sourceId","primaryLanguage","monitorNew"]){
    const field=form.elements[key];if(field.type==="checkbox")field.checked=Boolean(c[key]);else field.value=c[key]||"";
   }
   const rows=result.events||[];
   const pending=rows.filter(e=>!e.delivered_at).length;
   note((result.problem?result.problem+" · ":"")+tr("待交接 ","Pending ")+pending+" · "+tr("已交接 ","Delivered ")+(rows.length-pending),Boolean(result.problem));
   events.replaceChildren();
   for(const event of rows.slice().sort((a,b)=>b.created_at.localeCompare(a.created_at)).slice(0,50)){
    const row=document.createElement("p");
    row.textContent=(event.payload?.manifest?.works?.[0]?.title||event.id)+" · "+(event.delivered_at?tr("已交接","Delivered"):tr("等待重试","Pending retry"))+" · "+tr("尝试 ","Attempts ")+event.attempts+(event.problem?" · "+event.problem:"");
    events.append(row);
   }
  }catch(err){note(err.message,true);}
 }
 function init(){
  if(initialized)return;form=document.getElementById("impactConfigForm");status=document.getElementById("impactConfigStatus");events=document.getElementById("impactEvents");
  form.addEventListener("submit",async e=>{
   e.preventDefault();
   const config={};for(const key of ["enabled","endpoint","tokenFile","sourceId","primaryLanguage","monitorNew"]){const field=form.elements[key];config[key]=field.type==="checkbox"?field.checked:field.value.trim();}
   try{await BlogCTLPopup.send("blogctl.impact.save",{config});await refresh();}catch(err){note(err.message,true);}
  });
  document.getElementById("impactRefresh").addEventListener("click",refresh);
  document.getElementById("impactRetry").addEventListener("click",async()=>{try{await BlogCTLPopup.send("blogctl.impact.retry");note("已启动重试，稍后刷新查看结果。");}catch(err){note(err.message,true);}});
  initialized=true;
 }
 root.BlogCTLImpact={init,refresh};
})(globalThis);
