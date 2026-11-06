import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const dir=path.dirname(fileURLToPath(import.meta.url));
const source=fs.readFileSync(path.join(dir,'../popup/mcp-connection.js'),'utf8');
const id='tunnel_'+'a'.repeat(32);
function load() {
 const context={document:{},globalThis:null};
 context.globalThis=context;
 vm.createContext(context);
 vm.runInContext(source,context);
 return context.BlogCTLMCPConnection;
}
test('Tunnel Stop remains available when running but unhealthy, even with edited credentials',()=>{
 const {controls}=load();
 const status={configured:true,runtime_key_saved:true,tunnel_id:id,running:true,ready:false};
 const result=controls(status,{id:id.replace(/a$/,'b'),key:'changed_key'});
 assert.equal(result.canStop,true);
 assert.equal(result.canRestart,false);
 assert.equal(result.canStart,false);
});
test('health-ready can be stopped even when process detection is unavailable',()=>{
 const {controls}=load();
 const result=controls({configured:true,runtime_key_saved:true,tunnel_id:id,running:false,ready:true},{id});
 assert.equal(result.canStop,true);
});
test('dirty fields block start and auto-start while preserving Stop',()=>{
 const {controls}=load();
 const s={configured:true,runtime_key_saved:true,tunnel_id:id,running:false,ready:false};
 assert.equal(controls(s,{id}).canStart,true);
 assert.equal(controls(s,{id}).canAutoStart,true);
 assert.equal(controls(s,{id,key:'different'}).canAutoStart,false);
});
test('invalid and fixture Tunnel IDs are rejected',()=>{
 const {validID}=load();
 assert.equal(validID(id),true);
 assert.equal(validID('tunnel_0123456789abcdef0123456789abcdef'),false);
 assert.equal(validID('tunnel_invalid'),false);
});
function fakeUI() {
 const elements=new Map();
 const calls=[];
 const defaultPath='/private/SyndiRelay/bin/tunnel-client';
 const saved={configured:true,runtime_key_saved:true,client_installed:true,client_path:defaultPath,client_default_path:defaultPath,client_custom:false,tunnel_id:id,running:false,ready:false,revision:'before',setup_state:'succeeded'};
 const elem=(name)=>{
  if(!elements.has(name)){
   elements.set(name,{id:name,value:'',textContent:'',className:'',disabled:false,readOnly:false,
    placeholder:'',checked:false,handlers:new Map(),attrs:new Map(),
    addEventListener(e,f){this.handlers.set(e,f)},
    setAttribute(k,v){this.attrs.set(k,v)},
    matches(){return false},closest(){return {hidden:false}},
   });
  }
  return elements.get(name);
 };
 const ctx={document:{getElementById:elem,activeElement:null},globalThis:null,
   setInterval(){return 1},clearInterval(){},navigator:{clipboard:{writeText:async()=>{}}},
   BlogCTLPopup:{send:async(action,args)=>{
    calls.push({action,args});
    if(action==='blogctl.mcp.connection.get')return {connection:{...saved}};
    if(args?.action==='auto-status')return {auto_start:false};
    if(args?.action==='install-client'){
     saved.client_installed=true;saved.client_custom=false;saved.client_path=saved.client_default_path;
    }
    if(args?.action==='set-client-path'){
     saved.client_path=args.clientPath||saved.client_default_path;
     saved.client_custom=saved.client_path!==saved.client_default_path;
     saved.client_installed=true;
    }
    return {accepted:true};
   },errorMessage:e=>String(e?.message||e)}
 };
 ctx.globalThis=ctx;vm.createContext(ctx);vm.runInContext(source,ctx);
 return {api:ctx.BlogCTLMCPConnection,elem,ctx,calls,saved};
}
test('silent password manager autofill does not rotate the key',async()=>{
 const {api,elem,calls}=fakeUI();
 api.init();
 await new Promise(resolve=>setTimeout(resolve,10));
 elem('mcpRuntimeKey').value='passwordmanagerInjected123456';
 await elem('mcpSave').handlers.get('click')();
 const changes=calls.filter(c=>c.args?.action==='save');
 assert.equal(changes.length,0);
});
test('running unhealthy state displays Stop and not Stopped',async()=>{
 const {api,elem,saved}=fakeUI();
 saved.running=true;saved.ready=false;
 api.init();
 await new Promise(resolve=>setTimeout(resolve,10));
 assert.equal(elem('mcpConnect').textContent,'停止服务');
 assert.equal(elem('mcpConnect').disabled,false);
 assert.notEqual(elem('mcpConnectionStatus').textContent,'已停止');
});

test('standby supervisor remains running and displays Tunnel disconnected',async()=>{
 const {api,elem,saved}=fakeUI();
 saved.running=true;saved.ready=false;saved.tunnel_connecting=false;
 api.init();
 await new Promise(resolve=>setTimeout(resolve,10));
 assert.equal(elem('mcpConnectionStatusText').textContent,'本地服务运行中 · Tunnel 未连接');
 assert.equal(elem('mcpConnect').textContent,'停止服务');
 assert.equal(elem('mcpConnect').disabled,false);
 assert.equal(elem('mcpTunnelToggle').textContent,'连接 Tunnel');
});

test('environment detection requires explicit download and shows private executable',async()=>{
 const {api,elem,calls,saved}=fakeUI();
 saved.client_installed=false;
 api.init();
 await new Promise(resolve=>setTimeout(resolve,10));
 assert.equal(elem('mcpClientPath').value,saved.client_default_path);
 assert.equal(elem('mcpClientDownload').hidden,false);
 assert.equal(elem('mcpClientDownload').disabled,false);
 assert.equal(elem('mcpConnect').disabled,true);
 await elem('mcpClientDownload').handlers.get('click')();
 const installCalls=calls.filter(c=>c.args?.action==='install-client');
 assert.equal(installCalls.length,1);
 assert.equal(calls.filter(c=>c.args?.action==='save').length,0);
 assert.equal(elem('mcpClientDownload').hidden,true);
 assert.equal(elem('mcpClientStatus').attrs.get('data-kind'),'ready');
 assert.equal(elem('mcpClientPath').value,saved.client_default_path);
});

test('custom executable path is editable, saved, and reset independently',async()=>{
 const {api,elem,calls,saved}=fakeUI();
 api.init();
 await new Promise(resolve=>setTimeout(resolve,10));
 const custom='/custom/runtime/tunnel-client';
 elem('mcpClientPath').value=custom;
 elem('mcpClientPath').handlers.get('input')();
 assert.equal(elem('mcpClientPathSave').disabled,false);
 await elem('mcpClientPathSave').handlers.get('click')();
 assert.equal(saved.client_path,custom);
 assert.equal(saved.client_custom,true);
 assert.equal(calls.filter(c=>c.args?.action==='set-client-path').length,1);
 assert.equal(elem('mcpClientPathReset').disabled,false);
 await elem('mcpClientPathReset').handlers.get('click')();
 assert.equal(saved.client_custom,false);
 assert.equal(saved.client_path,saved.client_default_path);
 assert.equal(calls.filter(c=>c.args?.action==='set-client-path').length,2);
});

test('saving Tunnel credentials never downloads a missing client automatically',async()=>{
 const {api,elem,calls,saved}=fakeUI();
 saved.client_installed=false;
 api.init();
 await new Promise(resolve=>setTimeout(resolve,10));
 assert.equal(elem('mcpSave').disabled,true);
 await elem('mcpSave').handlers.get('click')();
 assert.equal(calls.filter(c=>['install-client','save'].includes(c.args?.action)).length,0);
});

test('a running Tunnel without a private client can be stopped but not restarted',()=>{
 const {controls}=load();
 const status={configured:true,runtime_key_saved:true,client_installed:false,tunnel_id:id,running:true,ready:false};
 const options=controls(status,{id});
 assert.equal(options.canStop,true);
 assert.equal(options.canRestart,false);
 assert.equal(options.canAutoStart,false);
});
