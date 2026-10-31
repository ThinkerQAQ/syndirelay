// Native Story settings controls, observed on Medium. Never click publish.
export async function setMediumCanonicalInPage(postID, canonical, readOnly = false) {
 const fail = error => ({error});
 if(location.origin!=="https://medium.com" || location.pathname!==`/p/${postID}/settings`) return fail("Unexpected Medium Story settings URL");
 const parsed=new URL(canonical);
 if(parsed.protocol!=="https:"||parsed.username||parsed.password) return fail("Invalid canonical URL");
 const wait=ms=>new Promise(r=>setTimeout(r,ms));
 const button=text=>[...document.querySelectorAll("button")].find(x=>x.innerText.trim()===text);
 for(let n=0;n<60&&!button("Advanced Settings");n++) await wait(300);
 const advanced=button("Advanced Settings");
 if(!advanced)return fail("Medium Advanced Settings not found");
 if(!document.querySelector('input[placeholder="Type the canonical URL..."]')) {advanced.click();await wait(250);}
 const label=[...document.querySelectorAll("label")].find(x=>x.textContent.includes("This story was originally published elsewhere"));
 const check=label?.querySelector('input[type="checkbox"]');
 if(!check)return fail("Medium canonical checkbox not found");
 if(!check.checked){if(readOnly)return fail("Canonical is not enabled");check.click();await wait(250);}
 let input=document.querySelector('input[placeholder="Type the canonical URL..."]');
 if(!input)return fail("Medium canonical field not found");
 if(input.value===canonical && input.disabled)return {inserted:true,imageCount:0};
 if(readOnly)return fail("Medium canonical did not persist");
 const edit=button("Edit canonical link");if(input.disabled){if(!edit)return fail("Canonical edit control not found");edit.click();await wait(150);}
 input=document.querySelector('input[placeholder="Type the canonical URL..."]');
 Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,"value").set.call(input,canonical);
 input.dispatchEvent(new Event("input",{bubbles:true}));input.dispatchEvent(new Event("change",{bubbles:true}));
 await wait(200);
 const save=button("Save canonical link");
 if(!save||save.disabled)return fail("Canonical save control is unavailable");
 save.click();
 for(let n=0;n<80;n++){
  await wait(250);
  input=document.querySelector('input[placeholder="Type the canonical URL..."]');
  if(input?.disabled&&input.value===canonical&&button("Edit canonical link"))return {inserted:true,imageCount:0};
 }
 return fail("Medium did not confirm canonical settings save");
}

export async function configureMediumCanonical(task, tabs=chrome.tabs, scripting=chrome.scripting) {
 if(!/^[0-9a-f]{32}$/.test(task.id||"")||!/^[0-9a-f]{8,24}$/.test(task.draftId||""))throw Error("Invalid Medium canonical task");
 const canonical=new URL(task.canonicalUrl);
 if(canonical.protocol!=="https:"||canonical.username||canonical.password)throw Error("Invalid canonical destination");
 const url=`https://medium.com/p/${task.draftId}/settings`;
 if((await tabs.query({url})).length)throw Error("Medium Story settings already open; close it before updating canonical");
 const tab=await tabs.create({url,active:false});
 const ready=async()=>{for(let n=0;n<120;n++){const t=await tabs.get(tab.id);if(t.status==="complete"&&t.url===url)return;if(t.status==="complete"&&t.url!==url&&t.pendingUrl!==url)throw Error("Medium Story settings redirected");await new Promise(r=>setTimeout(r,400));}throw Error("Medium Story settings did not load");};
 try{
  await ready();
  let result=(await scripting.executeScript({target:{tabId:tab.id},world:"MAIN",func:setMediumCanonicalInPage,args:[task.draftId,task.canonicalUrl,false]}))[0]?.result;
  if(!result?.inserted)throw Error(result?.error||"Canonical save returned no result");
  await tabs.reload(tab.id);await ready();
  result=(await scripting.executeScript({target:{tabId:tab.id},world:"MAIN",func:setMediumCanonicalInPage,args:[task.draftId,task.canonicalUrl,true]}))[0]?.result;
  if(!result?.inserted)throw Error(result?.error||"Canonical persistence verification failed");
  return result;
 }finally{await tabs.remove(tab.id).catch(()=>{});}
}
