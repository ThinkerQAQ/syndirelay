import { findMediumEditorTab } from "./medium-upload.js";
export function readMediumEditorDocument(id,expected=null) {
 if(location.origin!=="https://medium.com"||location.pathname!==`/p/${id}/edit`)return {error:"Unexpected Medium editor URL"};
 const root=document.querySelector('.postArticle-content[contenteditable="true"]');
 if(!root)return {error:"Medium editor has not loaded"};
 if(/Saving failed because someone is also editing|Reload to see their changes/i.test(document.body.innerText))return {error:"Concurrent Medium editor changes"};
 const saved=document.querySelector('.js-metabarMessage')?.textContent?.trim();
 if(!/^(Saved|DraftSaved)$/.test(saved||""))return {error:"Medium editor has not confirmed autosave"};
 const allNodes=[...root.querySelectorAll('.graf')];
 const nodes=allNodes.filter(n=>n.tagName!=="P"||n.textContent.replace(/[\u200a\u200b\ufeff\u2060]/g,"").trim());
 if(!nodes.length)return {error:"Medium editor document is empty"};
 if(expected){
  if(nodes.length!==expected.length)return {error:`Medium editor has ${nodes.length} paragraphs; expected ${expected.length}`};
  const normalize=(s,pre)=>{s=(s||"").normalize('NFKC').replace(/[\u200a\u200b\ufeff\u2060]/g,"").replace(/\r\n?/g,"\n");return pre?s.trim():s.replace(/\s+/g," ").trim()};
  for(let i=0;i<expected.length;i++){
   const p=expected[i],n=nodes[i],marker=/^\[BLOGCTL_IMAGE_\d{3}\]$/.test(p.text||"");
   if(marker&&n.tagName==="FIGURE")continue;
   if(normalize(n.innerText,p.type===8)!==normalize(p.text,p.type===8))return {error:`Medium editor paragraph ${i+1} differs from compiled source`};
   if(p.type===8&&n.tagName!=="PRE")return {error:`Medium code block ${i+1} lost PRE formatting`};
  }
 }
 return {inserted:true,imageCount:expected?nodes.length:allNodes.length};
}
const valid=task=>/^[0-9a-f]{32}$/.test(task.id||"")&&/^[0-9a-f]{8,24}$/.test(task.draftId||"");
async function readSaved(tabId,id,expected=null){let last;for(let n=0;n<100;n++){last=(await chrome.scripting.executeScript({target:{tabId},world:"MAIN",func:readMediumEditorDocument,args:[id,expected]}))[0]?.result;if(last?.inserted)return last;if(last?.error&&!/not loaded|not confirmed/.test(last.error))break;await new Promise(r=>setTimeout(r,400));}throw Error(last?.error||"Medium document readback failed");}
export async function prepareMediumReplacement(task){
 if(!valid(task))throw Error("Invalid Medium replacement task");
 const tabId=await findMediumEditorTab(task.draftId);const tab=await chrome.tabs.get(tabId);const win=await chrome.windows.get(tab.windowId);
 if(win.focused&&tab.active)throw Error("Medium editor is in the foreground; refusing to interrupt it");
 try{const result=await readSaved(tabId,task.draftId);await chrome.tabs.remove(tabId);return result;}catch(e){throw e;}
}
export async function verifyMediumDocument(task){
 if(!valid(task)||!Array.isArray(task.expectedDocument)||task.expectedDocument.length<1||task.expectedDocument.length>20000)throw Error("Invalid Medium document verification task");
 const tabId=await findMediumEditorTab(task.draftId);
 return readSaved(tabId,task.draftId,task.expectedDocument);
}
