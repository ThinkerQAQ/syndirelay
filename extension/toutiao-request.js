// The creator page owns request signing; no signature code or credentials are persisted.
export async function performToutiaoBrowserRequest(request) {
 if (typeof request?.body!=="string" || request.body.length>8*1024*1024) throw new Error("Invalid article request");
 const tab=await chrome.tabs.create({url:"https://mp.toutiao.com/profile_v4/graphic/publish",active:false});
 for(let i=0;i<40;i++) { if ((await chrome.tabs.get(tab.id)).status==="complete") break; await new Promise(r=>setTimeout(r,500)); }
 try {
 const results=await chrome.scripting.executeScript({target:{tabId:tab.id},world:"MAIN",args:[request.body],func:async body=>{
  if(location.origin!=="https://mp.toutiao.com"||!location.pathname.startsWith("/profile_v4/graphic/publish")) throw new Error("Creator editor required");
  for (let i=0;i<75 && (!window.byted_acrawler || !document.querySelector(".ProseMirror"));i++) await new Promise(resolve=>setTimeout(resolve,200));
  if(!window.byted_acrawler || !document.querySelector(".ProseMirror")) throw new Error("Creator editor is not ready");
  const preflight=await fetch("/spice/image",{method:"HEAD",credentials:"include",headers:{"x-secsdk-csrf-request":"1","x-secsdk-csrf-version":"1.2.22"}});
  const csrf=(preflight.headers.get("x-ware-csrf-token")||"").split(",")[1];
  if(!preflight.ok||!csrf) throw new Error("Creator CSRF unavailable");
  const response=await fetch("/mp/agw/article/publish?source=mp&type=article&aid=1231&mp_publish_ab_val=0",{method:"POST",credentials:"include",headers:{"content-type":"application/x-www-form-urlencoded;charset=UTF-8","x-secsdk-csrf-token":csrf},body});
  const text=await response.text();if(text.length>900000)throw new Error("Creator response too large");return {status:response.status,body:text};
 }});
 if(!results[0]?.result)throw new Error("Creator request returned no result");
 return results[0].result;
 } finally { await chrome.tabs.remove(tab.id).catch(()=>{}); }
}
