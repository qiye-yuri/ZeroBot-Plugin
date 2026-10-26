// Offline integration of the actual app navigation, overview and resource controllers.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
import test,{after} from 'node:test';
const require=createRequire(import.meta.url);
const {chromium}=require(path.join(process.env.LOCALADMIN_NODE_MODULES||path.join(process.env.USERPROFILE,'.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules'),'playwright'));
const browser=await chromium.launch({channel:'chrome',headless:true});after(()=>browser.close());
const report={startedAt:'2026-10-26T00:00:00Z',completedAt:'2026-10-26T00:00:01Z',groups:[]};
async function fixture(t){
 const c=await browser.newContext();t.after(()=>c.close());await c.route('**/*',r=>r.abort());const p=await c.newPage();const root='internal/localadmin/assets/';
 const html=(await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,'');await p.setContent(html);
 for(const f of ['app.js','overview.js','resources.js'])await p.addScriptTag({content:await fs.readFile(root+f,'utf8')});
 await p.evaluate(()=>{authenticated=true;csrf='synthetic';$('auth').hidden=true;$('workspace').hidden=false;window.pending=[];window.fetch=(url,options)=>new Promise(resolve=>pending.push({url,options,resolve}));});return p;
}
async function respond(p,index,value,status=200){await p.evaluate(({index,value,status})=>pending[index].resolve({ok:status===200,status,json:async()=>value}),{index,value,status});}
test('actual navigation clears cost hints and ignores departed scan response',async t=>{
 const p=await fixture(t);await p.evaluate(()=>renderModelUsage([{id:1,model:'fixture',usage:{input:100000,hit:0,miss:100000,output:1}}]));
 await p.evaluate(()=>{$('model-task-filter').value='large';$('model-task-sort').value='miss'});
 assert.match(await p.locator('#model-usage-breakdown').textContent(),/fixture/);
 await p.locator('[data-view="resources"]').click();await p.waitForFunction(()=>pending.length===1);assert.equal(await p.locator('#model-task-filter').inputValue(),'all');assert.equal(await p.locator('#model-task-sort').inputValue(),'original');assert.equal(await p.evaluate(()=>pending[0].url),'/api/resources');assert.equal(await p.locator('#model-usage-breakdown').textContent(),'分组用量待更新。');
 await respond(p,0,{busy:false,report});await p.waitForFunction(()=>!resourcesBusy);
 await p.locator('#resources-check').click();await p.waitForFunction(()=>pending.length===2);
 await p.locator('[data-view="overview"]').click();await p.waitForFunction(()=>pending.length===3);assert.equal(await p.evaluate(()=>pending[2].url),'/api/overview');
 await respond(p,1,{busy:false,report:{...report,completedAt:'late-response'}});await p.waitForTimeout(30);assert.equal(await p.evaluate(()=>resourcesSnapshot.report.completedAt),report.completedAt);
 await p.locator('[data-view="resources"]').click();await p.waitForFunction(()=>pending.length===4);assert.equal(await p.evaluate(()=>pending[3].url),'/api/resources');
 assert.equal(await p.evaluate(()=>pending.filter(r=>r.options.method==='POST').length),1);
});
test('actual session expiry clears both controllers and prevents export',async t=>{
 const p=await fixture(t);await p.locator('[data-view="resources"]').click();await p.waitForFunction(()=>pending.length===1);await respond(p,0,{busy:false,report});await p.waitForFunction(()=>!resourcesBusy);
 await p.evaluate(()=>renderModelUsage([{id:1,model:'fixture',usage:{input:100000,hit:0,miss:100000,output:1}}]));
 await p.locator('#resources-refresh').click();await p.waitForFunction(()=>pending.length===2);await respond(p,1,{error:'session expired'},401);
 await p.waitForFunction(()=>!authenticated);assert.equal(await p.locator('#auth').isVisible(),true);assert.equal(await p.locator('#model-task-filter').inputValue(),'all');assert.equal(await p.locator('#model-task-sort').inputValue(),'original');assert.equal(await p.evaluate(()=>resourcesSnapshot),null);assert.equal(await p.locator('#resources-export').isDisabled(),true);assert.equal(await p.locator('#model-usage-breakdown').textContent(),'分组用量待更新。');
});


test('overview failure clears filtered tasks and recovery uses current data, logout resets choices',async t=>{
 const p=await fixture(t);await p.locator('[data-view="overview"]').click();await p.waitForFunction(()=>pending.length===1);
 const meta={startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z'},rows=[{id:1,model:'old-fixture',usage:{input:100000,hit:0,miss:100000,output:1}}];
 await p.evaluate(({rows,meta})=>{renderModelUsage(rows,meta);$('model-task-filter').value='large';$('model-task-sort').value='miss';renderModelTaskView()},{rows,meta});
 assert.match(await p.locator('#model-task-usage').textContent(),/old-fixture/);
 await respond(p,0,{error:'synthetic read failure'},500);await p.waitForFunction(()=>!overviewBusy);
 assert.equal(await p.locator('#model-task-filter').isDisabled(),true);assert.equal(await p.locator('#model-usage-export').isDisabled(),true);assert.doesNotMatch(await p.locator('#model-task-usage').textContent(),/old-fixture/);
 assert.equal(await p.locator('#model-task-filter').inputValue(),'large');
 await p.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows:[{...rows[0],model:'new-fixture'}],meta});
 assert.equal(await p.locator('#model-task-filter').isEnabled(),true);assert.match(await p.locator('#model-task-usage').textContent(),/new-fixture/);assert.doesNotMatch(await p.locator('#model-task-usage').textContent(),/old-fixture/);
 await p.evaluate(()=>{authenticated=false;resetOverview()});assert.equal(await p.locator('#model-task-filter').inputValue(),'all');assert.equal(await p.locator('#model-task-sort').inputValue(),'original');assert.equal(await p.locator('#model-usage-export').isDisabled(),true);
 await p.evaluate(()=>{authenticated=true;renderModelUsage([],{startedAt:'2026-10-27T00:00:00Z',checkedAt:'2026-10-27T01:00:00Z'})});
 assert.equal(await p.locator('#model-task-filter').inputValue(),'all');assert.match(await p.locator('#model-task-usage').textContent(),/暂无近期请求/);
});
