'use strict';
(function(root){
 const PATTERN=/^tunnel_(?:[a-z0-9]{32}|[a-z0-9]{4}_[a-z0-9]{32})$/;
 const EXAMPLE='tunnel_0123456789abcdef0123456789abcdef';
 const $=id=>document.getElementById(id);
 const state={status:{},busy:false,pending:null,editedID:false,editedKey:false,editedClientPath:false,autoStart:false,autoKnown:false,initialized:false,stopPoll:null};
 const validID=id=>PATTERN.test(id)&&id!==EXAMPLE;
 const running=s=>Boolean(s.running||s.ready);
 const controls=(s,{id,key,busy=false,pending=false}={})=>{
  const changed=Boolean((id||'')!==(s.tunnel_id||'')||key);
  const stored=Boolean(s.configured&&s.runtime_key_saved&&validID(s.tunnel_id));
  const active=running(s);
  return {
   canSave:s.client_installed!==false&&!busy&&!pending,
   canStart:!active&&stored&&!changed&&s.client_installed!==false&&!busy&&!pending,
   canStop:active&&!busy&&!pending,
   canRestart:active&&stored&&!changed&&s.client_installed!==false&&!busy&&!pending,
   canAutoStart:stored&&!changed&&s.client_installed!==false&&!busy&&!pending,
   changed,active,stored
  };
 };
 function message(text,error=false) {
  $('mcpConnectionMessage').textContent=text||'';
  $('mcpConnectionMessage').className=error?'mcp-message error':'mcp-message';
 }
 function form() {
  const input=$('mcpTunnelID'), key=$('mcpRuntimeKey');
  return {id:input?.value?.trim()||'',key:key?.value?.trim()||'',busy:state.busy,pending:!!state.pending};
 }
 function show() {
  const s=state.status,c=controls(s,form()),run=running(s);
  const clientPresent=Boolean(s.client_installed);
  const pathInput=$('mcpClientPath');
  const pathChanged=(pathInput?.value?.trim()||'')!==(s.client_path||'');
  const clientStatus=$('mcpClientStatus');
  clientStatus.textContent=clientPresent?'检测通过：Tunnel Client 已就绪':'未检测到 Tunnel Client，请下载或填写已有可执行文件路径。';
  clientStatus.setAttribute('data-kind',clientPresent?'ready':'missing');
  $('mcpClientDownload').disabled=state.busy||!!state.pending||clientPresent;
  $('mcpClientDownload').hidden=clientPresent;
  $('mcpClientPathSave').disabled=state.busy||!!state.pending||!pathChanged;
  $('mcpClientPathReset').disabled=state.busy||!!state.pending||!s.client_custom;
  $('mcpClientPathHint').textContent=s.client_custom
    ? '自定义位置已保存；修改后点击「保存路径」，运行中的服务需重启生效。'
    : '默认位置：SyndiRelay 专属目录。路径可修改，运行中的服务需重启生效。';
  $('mcpSave').disabled=!c.canSave;
  $('mcpConnect').disabled=run?!c.canStop:!c.canStart;
  $('mcpConnect').textContent=run?'停止服务':'启动服务';
  $('mcpRestart').disabled=!c.canRestart;
  const connectPref=$('mcpAutoConnect');
  if(connectPref){
   connectPref.disabled=!c.canAutoStart||!s.configured;
   connectPref.checked=s.tunnel_auto_connect!==false;
  }
  const toggle=$('mcpTunnelToggle');
  if(toggle) {
   const connecting=Boolean(s.tunnel_connecting||s.ready);
   toggle.textContent=connecting?'断开 Tunnel':'连接 Tunnel';
   toggle.disabled=state.busy||!!state.pending||!run||(!connecting&&c.changed)||(!connecting&&s.restart_required);
  }
  if($('mcpAutoStart')) {
   $('mcpAutoStart').disabled=!c.canAutoStart||!state.autoKnown;
   $('mcpAutoStart').checked=state.autoStart;
  }
  let label='未配置',kind='disabled';
  if(state.pending){label='正在保存';kind='checking';}
  else if(s.restart_required&&run){label='等待重启';kind='checking';}
  else if(s.ready){label='Ready';kind='ok';}
  else if(run&&!s.tunnel_connecting){label='本地服务运行中 · Tunnel 未连接';kind='disabled';}
  else if(run){label='正在启动 / 未就绪';kind='checking';}
  else if(s.configured){label='已停止';kind='disabled';}
  $('mcpConnectionStatusText').textContent=label;
  const statusChip=$('mcpConnectionStatus');
  const chipKind=kind==='ok'?'ready':kind==='checking'?'pending':'idle';
  if(statusChip.dataset) statusChip.dataset.kind=chipKind;
  else if(statusChip.setAttribute) statusChip.setAttribute('data-kind',chipKind);
  $('mcpPluginDescription').textContent=s.restart_required&&run
    ? '已保存的修改等待重启生效。重启服务后再到 ChatGPT 中管理插件。'
    : s.ready
      ? '本地 Tunnel 已就绪。请在 ChatGPT 中创建或管理插件，授权结果以 ChatGPT 为准。'
      : run&&!s.tunnel_connecting
        ? '本地服务正在运行，但 Tunnel 未连接。点击「连接 Tunnel」，等待就绪后再到 ChatGPT 管理插件。'
        : '先启动本地 Tunnel 并等待就绪，再到 ChatGPT 中创建或管理插件。';
  $('mcpTunnelHint').textContent=s.configured?'已保存；可修改后再次保存':'尚未保存';
  $('mcpRuntimeHint').textContent=s.runtime_key_saved?'已保存；留空则复用':'尚未保存';
  const reveal=$('mcpKeyReveal');
  if(reveal) {
   reveal.disabled=!$('mcpRuntimeKey').value;
   if(reveal.disabled) {
    $('mcpRuntimeKey').type='password';
    reveal.setAttribute?.('aria-pressed','false');
    reveal.setAttribute?.('aria-label','显示输入的密钥');
   }
  }
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
   if(!state.editedClientPath&&document.activeElement!==$('mcpClientPath'))$('mcpClientPath').value=s.client_path||'';
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
   if(!s.client_installed){message('请先在「环境检测」中下载 Tunnel Client 或保存可用的客户端路径。',true);return;}
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
   await root.BlogCTLPopup.send('blogctl.mcp.connection.action',{
    action,tunnelId:id,runtimeApiKey:action==='save'?key:'',
    reuseKey:action==='save'&&!key,
    replace:action==='save'&&!!s.configured,previousTunnelId:s.tunnel_id||''
   });
   if(action==='save'){
    state.pending={id,revision:s.revision||'',deadline:Date.now()+120000};
    $('mcpRuntimeKey').value='';
    message('已提交配置校验；保存成功前不会启动服务。');
   }else message('操作已提交，正在刷新服务状态。');
  }catch(e){message(root.BlogCTLPopup.errorMessage(e),true);}
  finally{state.busy=false;await refresh();}
 }
 function clientMessage(text,error=false) {
  $('mcpClientMessage').textContent=text||'';
  $('mcpClientMessage').className=error?'mcp-message error':'mcp-message';
 }
 async function clientAction(action){
  if(state.busy||state.pending)return;
  const value=$('mcpClientPath').value.trim();
  if(action==='set-client-path'&&value===(state.status.client_path||''))return;
  state.busy=true;show();clientMessage(action==='install-client'?'正在下载并校验官方 Tunnel Client…':'');
  try{
   await root.BlogCTLPopup.send('blogctl.mcp.connection.action',{action,clientPath:action==='set-client-path'?value:''});
   state.editedClientPath=false;
   state.busy=false;
   await refresh();
   $('mcpClientPath').value=state.status.client_path||'';
   clientMessage(action==='install-client'
     ? '官方 Tunnel Client 已下载并通过校验，路径已保存。'
     : '可执行文件路径已保存。'+(running(state.status)?'重启本地服务后生效。':''));
  }catch(e){clientMessage(root.BlogCTLPopup.errorMessage(e),true);}
  finally{state.busy=false;show();}
 }
 async function autoStatus(){
  try{
   const r=await root.BlogCTLPopup.send('blogctl.mcp.connection.action',{action:'auto-status'});
   state.autoStart=!!r.auto_start;state.autoKnown=true;
    $('mcpStartupError').textContent='';
  }catch(e){
    state.autoKnown=false;
    $('mcpStartupError').textContent='无法读取系统自启动设置，请检查系统权限或登录会话。';
  }
  show();
 }
   async function autoConnectChanged(){
   if(state.busy||state.pending)return;
   const c=controls(state.status,form());
   if(!c.canAutoStart){show();return;}
   const wanted=!!$('mcpAutoConnect').checked;
   state.busy=true;show();
   try {
    await root.BlogCTLPopup.send('blogctl.mcp.connection.action',{action:wanted?'auto-connect-enable':'auto-connect-disable'});
    message(state.status.running?'已保存。重启服务后，新的自动连接设置生效。':'Tunnel 自动连接设置已保存。');
   }catch(e){message(root.BlogCTLPopup.errorMessage(e),true);}
   finally{state.busy=false;await refresh();}
  }
  async function tunnelChanged(){
   const s=state.status;
   if(state.busy||state.pending||!running(s))return;
   const connecting=Boolean(s.tunnel_connecting||s.ready);
   if(!connecting&&(!controls(s,form()).stored||controls(s,form()).changed||s.restart_required))return;
   state.busy=true;show();
   try {
    await root.BlogCTLPopup.send('blogctl.mcp.connection.action',{action:connecting?'disconnect':'connect'});
    message(connecting?'Tunnel 已断开，本地服务仍运行。':'Tunnel 连接请求已提交，正在检查状态。');
   }catch(e){message(root.BlogCTLPopup.errorMessage(e),true);}
   finally{state.busy=false;await refresh();}
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
  $('mcpClientPath').addEventListener('input',()=>{state.editedClientPath=true;clientMessage('');show();});
  $('mcpClientDownload').addEventListener('click',()=>clientAction('install-client'));
  $('mcpClientPathSave').addEventListener('click',()=>clientAction('set-client-path'));
  $('mcpClientPathReset').addEventListener('click',()=>{
   $('mcpClientPath').value=state.status.client_default_path||'';
   state.editedClientPath=true;
   clientAction('set-client-path');
  });
  intendedInput($('mcpTunnelID'),'');
  intendedInput($('mcpRuntimeKey'),'');
  $('mcpSave').addEventListener('click',()=>action('save'));
  $('mcpConnect').addEventListener('click',()=>action(running(state.status)?'stop':'start'));
  $('mcpRestart').addEventListener('click',()=>action('restart'));
  $('mcpAutoStart')?.addEventListener('change',autoChanged);
  $('mcpAutoConnect')?.addEventListener('change',autoConnectChanged);
  $('mcpTunnelToggle')?.addEventListener('click',tunnelChanged);
  $('mcpKeyReveal')?.addEventListener('click',()=>{
   const input=$('mcpRuntimeKey'),reveal=$('mcpKeyReveal');
   if(!input.value || reveal.disabled)return;
   const showing=input.type==='password';
   input.type=showing?'text':'password';
   reveal.setAttribute('aria-pressed',String(showing));
   reveal.setAttribute('aria-label',showing?'隐藏输入的密钥':'显示输入的密钥');
   reveal.title=showing?'隐藏输入的密钥':'显示输入的密钥';
  });
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
