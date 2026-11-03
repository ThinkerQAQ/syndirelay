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
 const saved={configured:true,runtime_key_saved:true,client_installed:true,tunnel_id:id,running:false,ready:false,revision:'before',setup_state:'succeeded'};
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
