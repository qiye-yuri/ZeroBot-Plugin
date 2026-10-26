import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
import test,{after} from 'node:test';
import {checkUsageFile} from './check-model-usage.mjs';
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
 const checked=await checkUsageFile(await download.path());assert.equal(checked.passed,true,JSON.stringify(checked.issues));assert.equal(checked.taskBreakdownChecked,true);assert.equal(checked.requestCount,1);
 const changed=(await download.path())+'.modified.json';t.after(()=>fs.rm(changed,{force:true}));
 const modified=structuredClone(result);modified.cacheSummary.hitRate='100.00%';await fs.writeFile(changed,JSON.stringify(modified),{flag:'wx'});assert.equal((await checkUsageFile(changed)).passed,false);
 assert.equal(result.taskBreakdown.groups.length,1);assert.equal(result.taskBreakdown.groups[0].grouped,false);assert.equal(result.taskBreakdown.groups[0].requests[0].prompt,undefined);
 await page.getByText('按任务查看用量',{exact:true}).click();assert.equal(await page.locator('#model-task-usage').isVisible(),true);assert.match(await page.locator('#model-task-usage').textContent(),/独立请求/);assert.match(await page.locator('#model-task-usage').textContent(),/输入 200000/);
 assert.equal(result.cacheSummary.valid,1);assert.equal(result.cacheSummary.hitRate,'0.00%');assert.equal(result.format,'zerobot-model-usage');assert.equal(result.thresholds.large,300000);assert.equal(result.requests.length,1);assert.equal(result.requests[0].prompt,undefined);assert.deepEqual(result.snapshot,meta);assert.equal(requests,0);
 await page.evaluate(()=>{overviewBusy=true;renderModelUsageExport()});assert.equal(await page.locator('#model-usage-export').isDisabled(),true);
 await page.evaluate(()=>{overviewBusy=false;clearModelRequests()});assert.equal(await page.locator('#model-usage-export').isDisabled(),true);assert.equal(await page.locator('#model-usage-export-status').textContent(),'');
});

test('multi-round downloads isolate sessions and replace old task snapshots',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());let requests=0;await context.route('**/*',r=>{requests++;return r.abort()});const page=await context.newPage(),root='internal/localadmin/assets/';
 await page.setContent((await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,''));
 await page.evaluate(()=>{window.$=id=>document.getElementById(id);window.authenticated=true;$('auth').hidden=true;$('workspace').hidden=false;$('overview-view').hidden=false;});await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 const session='a'.repeat(24),task='b'.repeat(24),meta={startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z'};
 const rows=[
  {id:1,session,task,sequence:1,provider:'fixture',model:'one',usage:{input:100,hit:60,miss:40,output:3}},
  {id:2,session,task,sequence:2,provider:'fixture',model:'two',usage:{input:200,hit:160,miss:40,output:4}},
  {id:3,session:'c'.repeat(24),task,sequence:1,provider:'fixture',model:'one',usage:{input:50,hit:0,miss:50,output:2}},
  {id:4,provider:'fixture',model:'one',usage:{input:null,hit:null,miss:null,output:null}}
 ];
 await page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows,meta});
 await page.getByText('AI费用风险提示',{exact:true}).click();await page.getByText('按提供商与模型查看用量',{exact:true}).click();await page.getByText('按任务查看用量',{exact:true}).click();
 const downloadReport=async()=>{const pending=page.waitForEvent('download');await page.locator('#model-usage-export').click();const filename=await (await pending).path();const checked=await checkUsageFile(filename);assert.equal(checked.passed,true,JSON.stringify(checked.issues));assert.equal(checked.taskBreakdownChecked,true);return JSON.parse(await fs.readFile(filename,'utf8'));};
 await page.locator('#cost-large').fill('150');await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();
 await page.locator('#model-task-filter').selectOption('large');await page.locator('#model-task-sort').selectOption('miss');
 assert.match(await page.locator('#model-task-usage').textContent(),/匹配任务 1\/3/);assert.match(await page.locator('#model-task-usage').textContent(),/#1 /);assert.match(await page.locator('#model-task-usage').textContent(),/#2 /);assert.doesNotMatch(await page.locator('#model-task-usage').textContent(),/#3 /);
 await page.locator('#cost-large').fill('300');assert.match(await page.locator('#model-task-usage').textContent(),/匹配任务 1\/3/);
 await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();assert.match(await page.locator('#model-task-usage').textContent(),/当前条件无匹配任务/);
 await page.getByRole('button',{name:'恢复默认提示',exact:true}).click();assert.match(await page.locator('#model-task-usage').textContent(),/当前条件无匹配任务/);
 await page.locator('#cost-large').fill('150');await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();
 const first=await downloadReport();assert.equal(first.taskBreakdown.groups.length,3);assert.equal(first.breakdown.groups.length,2);
 assert.deepEqual(first.taskBreakdown.groups.map(g=>g.count),[2,1,1]);assert.deepEqual(first.taskBreakdown.groups[0].totals.input,{value:'300',known:2});assert.equal(first.taskBreakdown.groups[0].cacheSummary.hitRate,'73.33%');assert.equal(first.taskBreakdown.groups[2].totals.input.value,null);
 const text=await page.locator('#model-task-usage').textContent();assert.match(text,/当前窗口4次请求，3组/);assert.ok(text.includes('会话'+session));assert.match(text,/输入 300（已知2\/2次）/);
 await page.setViewportSize({width:320,height:900});await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')});await page.addStyleTag({content:await fs.readFile(root+'appearance.css','utf8')});
 for(const id of ['model-task-filter','model-task-sort','model-task-usage'])assert.equal(await page.locator('#'+id).evaluate(e=>{const r=e.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&e.scrollWidth<=e.clientWidth+1}),true,id);
 assert.ok(text.indexOf('匹配任务')<text.indexOf('会话'+session));
 const nextMeta={startedAt:'2026-10-26T02:00:00Z',checkedAt:'2026-10-26T03:00:00Z'};
 await page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows:[{...rows[2],id:1}],meta:nextMeta});
 assert.equal(await page.locator('#model-task-filter').inputValue(),'large');assert.match(await page.locator('#model-task-usage').textContent(),/当前条件无匹配任务/);
 const next=await downloadReport();assert.deepEqual(next.snapshot,nextMeta);assert.equal(next.taskBreakdown.groups.length,1);assert.equal(next.taskBreakdown.groups[0].totals.input.value,'50');assert.ok(!(await page.locator('#model-task-usage').textContent()).includes(session));
 await page.evaluate(meta=>renderModelUsage([],meta),nextMeta);const empty=await downloadReport();assert.equal(empty.taskBreakdown.groups.length,0);assert.match(await page.locator('#model-task-usage').textContent(),/暂无近期请求/);
 await page.evaluate(()=>renderModelUsage(null));assert.equal(await page.locator('#model-usage-export').isDisabled(),true);assert.equal(await page.locator('#model-task-usage').textContent(),'任务用量未知。');assert.equal(await page.locator('#model-task-filter').isDisabled(),true);assert.equal(requests,0);
});
