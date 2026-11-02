'use strict';
(function (root) {
  const STATUS_INTERVAL_MS = 4000;
  const TUNNEL_ID = /^tunnel_(?:[a-z0-9]{32}|[a-z0-9]{4}_[a-z0-9]{32})$/;
  const ids = (id) => document.getElementById(id);
  let busy = false, installed = false, configured = false, ready = false;
  let localConfig = {}, timer = null, initialized = false, pending = false;
  function setMessage(message, error = false) {
    const el = ids('mcpConnectionMessage');
    el.textContent = message || '';
    el.className = error ? 'message error' : 'message';
  }
  function stateLabel(connection) {
    if (connection.ready) return '本机连接已就绪。在 ChatGPT 插件设置中启用 SyndiRelay。';
    if (connection.configured) return '连接已保存，点击「连接 ChatGPT」即可重新启动。';
    if (!connection.client_installed) return '首次连接将下载并校验官方 Tunnel 客户端。';
    return '在 ChatGPT 中创建自定义 MCP Tunnel，粘贴完整的 Tunnel ID。';
  }
  function render(connection) {
    installed = !!connection.client_installed;
    configured = !!connection.configured;
    ready = !!connection.ready;
    if (ready) pending = false;
    localConfig = connection.local_config || {};
    ids('mcpConnectionStatus').textContent = pending && !ready ? '等待本机设置完成，状态将自动刷新…' : stateLabel(connection);
    ids('mcpTunnelField').hidden = configured;
    if (configured && connection.tunnel_id) ids('mcpTunnelID').value = connection.tunnel_id;
    ids('mcpConnect').textContent = ready ? '打开 ChatGPT 插件设置' : configured ? '连接' : '连接 ChatGPT';
    ids('mcpConnect').disabled = busy;
    ids('mcpRestart').disabled = busy || !configured;
    ids('mcpStop').disabled = busy || !configured;
    ids('mcpLocalConfig').textContent = JSON.stringify(localConfig, null, 2);
  }
  async function refresh() {
    if (!initialized || busy) return;
    try {
      const response = await root.BlogCTLPopup.send('blogctl.mcp.connection.get');
      render(response.connection || {});
      if (!pending) setMessage();
    } catch (error) {
      setMessage(root.BlogCTLPopup.errorMessage(error), true);
    }
  }
  async function act(action) {
    if (busy) return;
    if (action === 'open') {
      root.open('https://chatgpt.com/settings/plugins-settings', '_blank', 'noopener,noreferrer');
      return;
    }
    const tunnelId = ids('mcpTunnelID').value.trim();
    if (action === 'connect' && !configured && !TUNNEL_ID.test(tunnelId)) {
      setMessage('请粘贴 ChatGPT 提供的完整 Tunnel ID。', true);
      return;
    }
    busy = true;
    ids('mcpConnect').disabled = true;
    setMessage(action === 'connect' ? '正在准备本机连接…' : '正在执行操作…');
    try {
      const run = async (action) => root.BlogCTLPopup.send('blogctl.mcp.connection.action', { action, tunnelId });
      if (action === 'connect') {
        if (!installed) await run('install-client');
        await run(configured ? 'start' : 'setup');
      } else {
        await run(action);
      }
      // Guided setup opens a local terminal. The key is entered there, never in the UI.
      pending = action === 'connect';
      setMessage(action === 'connect' && !configured
        ? '首次连接已打开本机配置窗口。请在该窗口完成凭据输入；此处将自动检查。'
        : '已发出操作请求。');
      await new Promise((resolve) => setTimeout(resolve, 800));
    } catch (error) {
      pending = false;
      setMessage(root.BlogCTLPopup.errorMessage(error), true);
    } finally {
      busy = false;
      await refresh();
    }
  }
  function init() {
    if (initialized) return;
    initialized = true;
    ids('mcpConnect').addEventListener('click', () => act(ready ? 'open' : 'connect'));
    ids('mcpRefresh').addEventListener('click', refresh);
    ids('mcpRestart').addEventListener('click', () => act('restart'));
    ids('mcpStop').addEventListener('click', () => act('stop'));
    ids('mcpCopyConfig').addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(JSON.stringify(localConfig, null, 2));
        setMessage('已复制本地客户端配置。');
      } catch (error) { setMessage(root.BlogCTLPopup.errorMessage(error), true); }
    });
    timer = root.setInterval(() => {
      const pane = ids('mcpConnectionSettings').closest('.settings-category-pane');
      if (pane && !pane.hidden) refresh();
    }, STATUS_INTERVAL_MS);
  }
  root.BlogCTLMCPConnection = { init, refresh };
})(globalThis);
