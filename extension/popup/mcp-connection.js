'use strict';
(function(root){
 const PATTERN=/^tunnel_(?:[a-z0-9]{32}|[a-z0-9]{4}_[a-z0-9]{32})$/;
 const EXAMPLE='tunnel_0123456789abcdef0123456789abcdef';
 const $=id=>document.getElementById(id);
 const state={status:{},busy:false,pending:null,editedID:false,editedKey:false,autoStart:false,autoKnown:false,initialized:false,stopPoll:null};
 const validID=id=>PATTERN.test(id)&&id!==EXAMPLE;
 const running=s=>Boolean(s.running||s.ready);
 const controls=(s,{id,key,busy=false,pending=false}={})=>{
  const changed=Boolean((id||'')!==(s.tunnel_id||'')||key);
  const stored=Boolean(s.configured&&s.runtime_key_saved&&validID(s.tunnel_id));
  const active=running(s);
  return {
   canSave:!busy&&!pending,
   canStart:!active&&stored&&!changed&&!busy&&!pending,
   canStop:active&&!busy&&!pending,
   canRestart:active&&stored&&!changed&&!busy&&!pending,
   canAutoStart:stored&&!changed&&!busy&&!pending,
   changed,active,stored
  };
 };
 function message(text,error=false) {
  $('mcpConnectionMessage').textContent=text||'';
  $('mcpConnectionMessage').className=error?'message error':'message';
 }
 function form() {
  const input=$('mcpTunnelID'), key=$('mcpRuntimeKey');
  return {id:input?.value?.trim()||'',key:key?.value?.trim()||'',busy:state.busy,pending:!!state.pending};
 }
 function show() {
  const s=state.status,c=controls(s,form()),run=running(s);
  $('mcpSave').disabled=!c.canSave;
  $('mcpConnect').disabled=run?!c.canStop:!c.canStart;
  $('mcpConnect').textContent=run?'停止服务':'启动服务';
  $('mcpRestart').disabled=!c.canRestart;
  if($('mcpAutoStart')) {
   $('mcpAutoStart').disabled=!c.canAutoStart||!state.autoKnown;
   $('mcpAutoStart').checked=state.autoStart;
  }
  let label='未配置',kind='disabled';
  if(state.pending){label='正在保存';kind='checking';}
  else if(s.restart_required&&run){label='等待重启';kind='checking';}
  else if(s.ready){label='Ready';kind='ok';}
  else if(run){label='正在启动 / 未就绪';kind='checking';}
  else if(s.configured){label='已停止';kind='disabled';}
  $('mcpConnectionStatus').textContent=label;
  $('mcpConnectionStatus').className='status '+kind;
  $('mcpTunnelHint').textContent=s.configured?'已保存；可修改后再次保存':'尚未保存';
  $('mcpRuntimeHint').textContent=s.runtime_key_saved?'已保存；留空复用':'尚未保存';
 }
 function intendedInput(el,value) {
  if(!el)return;
  el.value=value;
  el.readOnly=true;
  el.autocomplete='off';
  el.setAttribute('data-lpignore','true');
  el.setAttribute('data-1p-ignore','true');
  el.setAttribute('data-bwignore','true');
  const unlock=()=>{el.readOnly=false;};
  for(const evt of ['keydown','pointerdown','paste'])el.addEventListener(evt,unlock);
  el.addEventListener('focus',()=>{
    const intended=el.id==='mcpTunnelID'
      ? (state.editedID?el.value:(state.status.tunnel_id||''))
      : (state.editedKey?el.value:'');
    if(el.value!==intended)el.value=intended;
  });
  el.addEventListener('input',()=>{
   if(document.activeElement!==el||el.matches?.(':-webkit-autofill')) {
    el.value=value;return;
   }
   if(el.id==='mcpTunnelID')state.editedID=true;
   else state.editedKey=true;
   message('');
   show();
  });
 }
 async function refresh(){
  if(state.busy)return;
  try{
   const result=await root.BlogCTLPopup.send('blogctl.mcp.connection.get');
   const s=result.connection||{};
   state.status=s;
   if(!state.editedID&&document.activeElement!==$('mcpTunnelID'))$('mcpTunnelID').value=s.tunnel_id||'';
   $('mcpRuntimeKey').placeholder=s.runtime_key_saved?'****************':'';
   $('mcpLocalConfig').textContent=JSON.stringify(s.local_config||{},null,2);
   const pending=state.pending;
   if(pending){
    if(s.setup_state==='failed'){state.pending=null;message('保存失败，之前的配置已保留。',true);}
    else if(s.setup_state==='succeeded'&&s.revision&&s.revision!==pending.revision&&s.tunnel_id===pending.id){
     state.pending=null;
     state.editedID=false;state.editedKey=false;
     $('mcpTunnelID').value=s.tunnel_id;
     $('mcpRuntimeKey').value='';
     message('配置已保存。');
    }else if(Date.now()>pending.deadline){state.pending=null;message('保存超时；请核对 Tunnel ID 和 Runtime API Key。',true);}
   }
   show();
  }catch(e){message(root.BlogCTLPopup.errorMessage(e),true);}
 }
 async function action(action){
  if(state.busy||state.pending)return;
  const s=state.status,c=controls(s,form());
  if(action==='start'&&!c.canStart||action==='stop'&&!c.canStop||
     action==='restart'&&!c.canRestart)return;
  const id=$('mcpTunnelID').value.trim();
  // Ignore silent password-manager injection: only user-initiated input may rotate the key.
  const key=state.editedKey ? $('mcpRuntimeKey').value.trim() : '';
  if(action==='save'&&!state.editedID&&id!==(s.tunnel_id||'')){
   $('mcpTunnelID').value=s.tunnel_id||'';
   message('检测到非手动填入的 Tunnel ID，已恢复原值。',true);
   show();return;
  }
  if(action==='save'){
   if(!validID(id)){message('请输入有效的非测试 Tunnel ID。',true);return;}
   if(!key&&!s.runtime_key_saved){message('请输入 Runtime API Key。',true);return;}
   if(id===s.tunnel_id&&!key&&s.configured&&s.runtime_key_saved){
    state.editedID=false;state.editedKey=false;$('mcpRuntimeKey').value='';
    message('原有配置已确认，无需再次写入。');
    show();return;
   }
  }
  state.busy=true;show();
  try{
   if(action==='save'&&!s.client_installed){
    await root.BlogCTLPopup.send('blogctl.mcp.connection.action',{action:'install-client'});
   }
   await root.BlogCTLPopup.send('blogctl.mcp.connection.action',{
    action,tunnelId:id,runtimeApiKey:action==='save'?key:'',
    reuseKey:action==='save'&&!key,
    replace:action==='save'&&!!s.configured,previousTunnelId:s.tunnel_id||''
   });
   if(action==='save'){
    state.pending={id,revision:s.revision||'',deadline:Date.now()+120000};
    $('mcpRuntimeKey').value='';
    message('已提交后台校验；保存成功前不会启动服务。');
   }else message('操作已提交，正在刷新服务状态。');
  }catch(e){message(root.BlogCTLPopup.errorMessage(e),true);}
  finally{state.busy=false;await refresh();}
 }
 async function autoStatus(){
  try{
   const r=await root.BlogCTLPopup.send('blogctl.mcp.connection.action',{action:'auto-status'});
   state.autoStart=!!r.auto_start;state.autoKnown=true;
  }catch(e){state.autoKnown=false;}
  show();
 }
 async function autoChanged(){
  if(state.busy||state.pending)return;
  const c=controls(state.status,form());
  if(!c.canAutoStart){show();return;}
  const desired=$('mcpAutoStart').checked;
  state.busy=true;show();
  try{
   const r=await root.BlogCTLPopup.send('blogctl.mcp.connection.action',{action:desired?'auto-enable':'auto-disable'});
   state.autoStart=!!r.auto_start;
   message('开机自启设置已更新，不会影响当前服务。');
  }catch(e){message(root.BlogCTLPopup.errorMessage(e),true);}
  finally{state.busy=false;show();}
 }
 function init(){
  if(state.initialized)return;state.initialized=true;
  intendedInput($('mcpTunnelID'),'');
  intendedInput($('mcpRuntimeKey'),'');
  $('mcpSave').addEventListener('click',()=>action('save'));
  $('mcpConnect').addEventListener('click',()=>action(running(state.status)?'stop':'start'));
  $('mcpRestart').addEventListener('click',()=>action('restart'));
  $('mcpAutoStart')?.addEventListener('change',autoChanged);
  $('mcpCopyConfig').addEventListener('click',async()=>{
   try{await navigator.clipboard.writeText(JSON.stringify(state.status.local_config||{},null,2));message('本地配置已复制。');}
   catch(e){message(root.BlogCTLPopup.errorMessage(e),true);}
  });
  const poll=root.setInterval(()=>{
   const panel=$('mcpConnectionSettings').closest('.settings-category-pane');
   if(!panel||!panel.hidden)refresh();
  },4000);
  state.stopPoll=()=>root.clearInterval(poll);
  refresh();autoStatus();
 }
 root.BlogCTLMCPConnection={init,refresh,controls,validID};
})(globalThis);
