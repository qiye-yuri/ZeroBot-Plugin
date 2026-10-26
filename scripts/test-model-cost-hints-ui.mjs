import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
import test,{after} from 'node:test';
const require=createRequire(import.meta.url);
const {chromium}=require(path.join(process.env.LOCALADMIN_NODE_MODULES||path.join(process.env.USERPROFILE,'.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules'),'playwright'));
await fs.mkdir('.local/resource-readonly/usage-downloads',{recursive:true});
const browser=await chromium.launch({channel:'chrome',headless:true,downloadsPath:path.resolve('.local/resource-readonly/usage-downloads')});after(()=>browser.close());
test('offline real overview disclosure and stale clearing',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());await context.route('**/*',r=>r.abort());const page=await context.newPage();
 const root='internal/localadmin/assets/';const html=(await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,'');await page.setContent(html);
 await page.evaluate(()=>{window.$=id=>document.getElementById(id);document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.getElementById('overview-view').hidden=false;});
 await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 await page.evaluate(()=>renderModelUsage([{id:1,kind:'agent',model:'<img src=x onerror=alert(1)>',usage:{input:200000,hit:1000,miss:199000,output:10},milliseconds:1}]));
 await page.getByText('AI费用风险提示',{exact:true}).click();assert.equal(await page.locator('#model-cost-hints').isVisible(),true);assert.match(await page.locator('#model-cost-hints').textContent(),/大输入1次，高未命中1次/);assert.equal(await page.locator('#model-cost-hints img').count(),0);
 await page.getByText('按提供商与模型查看用量',{exact:true}).click();
 assert.equal(await page.locator('#model-usage-breakdown').isVisible(),true);assert.equal(await page.locator('#model-usage-breakdown img').count(),0);assert.match(await page.locator('#model-usage-breakdown').textContent(),/已知1\/1次/);
 await page.setViewportSize({width:320,height:900});await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')});await page.addStyleTag({content:await fs.readFile(root+'appearance.css','utf8')});
 assert.equal(await page.locator('#model-usage-breakdown').evaluate(e=>e.scrollWidth<=e.clientWidth+1),true);
 await page.evaluate(()=>clearModelRequests());assert.equal(await page.locator('#model-usage-breakdown').textContent(),'分组用量待更新。');assert.equal(await page.locator('#model-cost-hints').textContent(),'费用风险提示待更新。');
});

test('threshold drafts apply without requests, survive refresh, reset on logout',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());let requests=0;await context.route('**/*',r=>{requests++;return r.abort()});const page=await context.newPage();const root='internal/localadmin/assets/';
 const html=(await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,'');await page.setContent(html);
 await page.evaluate(()=>{window.$=id=>document.getElementById(id);window.authenticated=true;$('auth').hidden=true;$('workspace').hidden=false;$('overview-view').hidden=false;});await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 const rows=[{id:1,usage:{input:200000,hit:1000,miss:199000,output:1}}];await page.evaluate(rows=>renderModelUsage(rows),rows);await page.getByText('AI费用风险提示',{exact:true}).click();
 await page.locator('#cost-large').fill('300000');await page.locator('#cost-minimum').fill('300000');assert.match(await page.locator('#cost-settings-status').textContent(),/尚未应用/);assert.match(await page.locator('#model-cost-hints').textContent(),/大输入1次/);
 await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();assert.match(await page.locator('#model-cost-hints').textContent(),/大输入0次，高未命中0次/);
 await page.locator('#cost-large').fill('0');await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();assert.match(await page.locator('#cost-settings-status').textContent(),/未应用/);
 await page.evaluate(rows=>renderModelUsage(rows),rows);assert.equal(await page.locator('#cost-large').inputValue(),'0');assert.match(await page.locator('#model-cost-hints').textContent(),/输入≥300000/);
 await page.getByRole('button',{name:'恢复默认提示',exact:true}).click();assert.equal(await page.locator('#cost-large').inputValue(),'100000');assert.match(await page.locator('#model-cost-hints').textContent(),/大输入1次/);
 await page.locator('#cost-large').fill('300000');await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();await page.evaluate(()=>{authenticated=false;resetOverview()});assert.equal(await page.locator('#cost-large').inputValue(),'100000');assert.equal(await page.locator('#model-cost-hints').textContent(),'费用风险提示待更新。');assert.equal(requests,0);
});

test('actual usage report download uses applied settings and no network request',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());let requests=0;await context.route('**/*',r=>{requests++;return r.abort()});const page=await context.newPage(),root='internal/localadmin/assets/';
 const html=(await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,'');await page.setContent(html);
 await page.evaluate(()=>{window.$=id=>document.getElementById(id);window.authenticated=true;$('auth').hidden=true;$('workspace').hidden=false;$('overview-view').hidden=false;});await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 await page.getByText('AI费用风险提示',{exact:true}).click();await page.getByText('按提供商与模型查看用量',{exact:true}).click();assert.equal(await page.locator('#model-usage-export').isDisabled(),true);
 const rows=[{id:2,model:'fixture',usage:{input:200000,hit:0,miss:200000,output:0},prompt:'private-synthetic'}];const meta={startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z'};
 await page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows,meta});assert.equal(await page.locator('#model-usage-export').isEnabled(),true);
 await page.locator('#cost-large').fill('300000');await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();await page.locator('#cost-large').fill('400000');
 const downloaded=page.waitForEvent('download');await page.locator('#model-usage-export').click();const download=await downloaded;const result=JSON.parse(await fs.readFile(await download.path(),'utf8'));
 assert.equal(result.cacheSummary.valid,1);assert.equal(result.cacheSummary.hitRate,'0.00%');assert.equal(result.format,'zerobot-model-usage');assert.equal(result.thresholds.large,300000);assert.equal(result.requests.length,1);assert.equal(result.requests[0].prompt,undefined);assert.deepEqual(result.snapshot,meta);assert.equal(requests,0);
 await page.evaluate(()=>{overviewBusy=true;renderModelUsageExport()});assert.equal(await page.locator('#model-usage-export').isDisabled(),true);
 await page.evaluate(()=>{overviewBusy=false;clearModelRequests()});assert.equal(await page.locator('#model-usage-export').isDisabled(),true);assert.equal(await page.locator('#model-usage-export-status').textContent(),'');
});
