'use strict';
(function (root) {
  const RE = /^tunnel_(?:[a-z0-9]{32}|[a-z0-9]{4}_[a-z0-9]{32})$/;
  const $ = id => document.getElementById(id);
  let busy=false, pending=false, current={}, dirty=false, initialized=false;
  function message(s,error=false) { $('mcpConnectionMessage').textContent=s||''; $('mcpConnectionMessage').className=error?'message error':'message'; }
  function changed() { return $('mcpTunnelID').value.trim() !== (current.tunnel_id||'') || !!$('mcpRuntimeKey').value; }
  function update() {
    const valid=RE.test($('mcpTunnelID').value.trim());
    const canStart=valid && current.configured && current.runtime_key_saved && !changed();
    $('mcpSave').disabled=busy||pending;
    $('mcpConnect').disabled=busy||pending||( !current.ready && !canStart );
    $('mcpConnect').textContent=current.running?'停止服务':'启动服务';
    $('mcpRestart').disabled=busy||pending||!current.running||!canStart;
    $('mcpConnectionStatus').textContent=pending?'保存中':current.restart_required?'等待重启':current.ready?'Ready':current.configured?'已停止 / 未就绪':'未配置';
    $('mcpTunnelHint').textContent=current.tunnel_id?'已保存；修改后点击保存':'尚未保存';
    $('mcpRuntimeHint').textContent=current.runtime_key_saved?'已保存；留空则复用':'尚未保存';
  }
  async function refresh() {
    if(busy) return;
    try {
      const response=await root.BlogCTLPopup.send('blogctl.mcp.connection.get');
      current=response.connection||{};
      if(!dirty && document.activeElement!==$('mcpTunnelID')) $('mcpTunnelID').value=current.tunnel_id||'';
      $('mcpRuntimeKey').placeholder=current.runtime_key_saved?'****************':'';
      $('mcpLocalConfig').textContent=JSON.stringify(current.local_config||{},null,2);
      if(pending && current.setup_state!=='running') {
        pending=false;
        if(current.setup_state==='failed') message('保存失败，先前的配置已保留。',true);
        else if(current.setup_state==='succeeded') {
          message('配置已保存。');
          dirty=false; $('mcpTunnelID').value=current.tunnel_id||''; $('mcpRuntimeKey').value='';
        }
      }
      update();
    } catch(e) { message(root.BlogCTLPopup.errorMessage(e),true); }
  }
  async function act(action) {
    if(busy||pending) return;
    const tunnelId=$('mcpTunnelID').value.trim();
    if(action==='save') {
      if(!RE.test(tunnelId)) { message('请填写有效的 Tunnel ID。',true);return; }
      if(!$('mcpRuntimeKey').value.trim()&&!current.runtime_key_saved) { message('请填写 Runtime API Key。',true);return; }
      if(!changed()&&current.configured&&current.runtime_key_saved) { message('已保存的配置未改变。');return; }
    }
    busy=true;update();
    try {
      if(action==='save'&&!current.client_installed) {
        await root.BlogCTLPopup.send('blogctl.mcp.connection.action',{action:'install-client'});
      }
      await root.BlogCTLPopup.send('blogctl.mcp.connection.action',{
        action,tunnelId,
        runtimeApiKey:action==='save'?$('mcpRuntimeKey').value.trim():'',
        reuseKey:action==='save'&&!$('mcpRuntimeKey').value.trim(),
        replace:action==='save'&&!!current.configured,
        previousTunnelId:current.tunnel_id||''
      });
      if(action==='save') { pending=true;message('正在后台校验并保存配置…'); }
      else message('服务操作已提交，状态将自动刷新。');
    } catch(e) { message(root.BlogCTLPopup.errorMessage(e),true); }
    finally { busy=false;await refresh(); }
  }
  function init() {
    if(initialized) return; initialized=true;
    $('mcpSave').addEventListener('click',()=>act('save'));
    $('mcpConnect').addEventListener('click',()=>act(current.running?'stop':'start'));
    $('mcpRestart').addEventListener('click',()=>act('restart'));
    for(const id of ['mcpTunnelID','mcpRuntimeKey']) $(id).addEventListener('input',()=>{dirty=changed();update();});
    $('mcpCopyConfig').addEventListener('click',async()=>{
      try { await navigator.clipboard.writeText(JSON.stringify(current.local_config||{},null,2));message('本地客户端配置已复制。'); }
      catch(e) {message(root.BlogCTLPopup.errorMessage(e),true);}
    });
    root.setInterval(()=>{
      const pane=$('mcpConnectionSettings').closest('.settings-category-pane');
      if(pane&&!pane.hidden) refresh();
    },4000);
    refresh();
  }
  root.BlogCTLMCPConnection={init,refresh};
})(globalThis);