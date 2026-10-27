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

test('request reasons follow applied thresholds, preserve downloads and clear with the snapshot',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());let requests=0;
 await context.route('**/*',r=>{requests++;return r.abort()});const page=await context.newPage(),root='internal/localadmin/assets/';
 await page.setContent((await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,''));
 await page.evaluate(()=>{window.$=id=>document.getElementById(id);window.authenticated=true;$('auth').hidden=true;$('workspace').hidden=false;$('overview-view').hidden=false;});
 await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 const rows=[{id:1,session:'a'.repeat(24),task:'b'.repeat(24),model:'fixture',usage:{input:200000,hit:0,miss:200000,output:1}},
 {id:2,session:'a'.repeat(24),task:'b'.repeat(24),model:'fixture',usage:{input:1,hit:1,miss:0,output:0}},
 {id:3,model:'other',usage:{input:10,hit:10,miss:0,output:1}}];
 const meta={startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z'};
 await page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows,meta});
 await page.getByText('AI费用风险提示',{exact:true}).click();await page.getByText('按任务查看用量',{exact:true}).click();await page.getByText('按提供商与模型查看用量',{exact:true}).click();
 const text=()=>page.locator('#model-task-usage').textContent();
 const report=async()=>{const pending=page.waitForEvent('download');await page.locator('#model-usage-export').click();const file=await(await pending).path();assert.equal((await checkUsageFile(file)).passed,true);const data=JSON.parse(await fs.readFile(file,'utf8'));delete data.exportedAt;return data;};
 const before=await report();assert.match(await text(),/提示：大输入（单次输入达到已应用阈值100000）；高未命中/);
 await page.locator('#model-task-filter').focus();await page.keyboard.press('ArrowDown');assert.equal(await page.locator('#model-task-filter').inputValue(),'anyRisk');await page.keyboard.press('ArrowDown');assert.equal(await page.locator('#model-task-filter').inputValue(),'large');
 await page.locator('#model-task-filter').selectOption('large');await page.locator('#model-task-sort').selectOption('miss');
 assert.match(await text(),/#2 [\s\S]*提示：未触发当前用量提示/);assert.doesNotMatch(await text(),/#3 /);
 assert.deepEqual(await report(),before);
 await page.locator('#cost-large').fill('300000');await page.locator('#cost-minimum').fill('300000');assert.match(await text(),/已应用阈值100000/);
 await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();assert.match(await text(),/当前条件无匹配任务/);assert.doesNotMatch(await text(),/提示：/);
 await page.locator('#model-task-filter').selectOption('all');assert.equal((await text()).match(/提示：未触发当前用量提示/g).length,3);
 await page.getByRole('button',{name:'恢复默认提示',exact:true}).click();assert.match(await text(),/已应用阈值100000/);
 await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')});await page.addStyleTag({content:await fs.readFile(root+'appearance.css','utf8')});
 const many=Array.from({length:256},(_,i)=>({...rows[0],id:i+1,model:'<img src=x onerror=alert(1)>'+('long-model-'.repeat(20))}));
 await page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows:many,meta});
 assert.equal((await text()).match(/提示：大输入/g).length,256);assert.equal(await page.locator('#model-task-usage img').count(),0);
 await page.locator('#model-task-usage summary').click();
 for(const width of [320,1280])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:900});await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
  assert.equal(await page.locator('#model-task-usage').evaluate(e=>e.scrollWidth<=e.clientWidth+1),true,`${width} ${theme}`);
 }
 await page.locator('#model-task-usage').evaluate(e=>e.scrollIntoView({block:'start'}));await page.screenshot({path:'.local/resource-readonly/task-risk-reasons.png',fullPage:false});
 await page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows:[rows[2]],meta});assert.doesNotMatch(await text(),/提示：大输入/);
 await page.evaluate(meta=>renderModelUsage([],meta),meta);assert.match(await text(),/暂无近期请求/);assert.doesNotMatch(await text(),/提示：/);
 await page.evaluate(()=>renderModelUsage(null));assert.equal(await text(),'任务用量未知。');
 await page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows,meta});await page.evaluate(()=>{authenticated=false;resetOverview()});assert.equal(await text(),'任务用量待更新。');assert.equal(await page.locator('#model-task-filter').inputValue(),'all');
 assert.equal(requests,0);
});

test('task disclosure preserves visible identity and keyboard focus across refresh, without changing reports',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());let requests=0;
 await context.route('**/*',r=>{requests++;return r.abort()});const page=await context.newPage(),root='internal/localadmin/assets/';
 await page.setContent((await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,''));
 await page.evaluate(()=>{window.$=id=>document.getElementById(id);window.authenticated=true;$('auth').hidden=true;$('workspace').hidden=false;$('overview-view').hidden=false;});
 await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 const meta={startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z'};
 const rows=[{id:1,session:'a'.repeat(24),task:'b'.repeat(24),model:'risk',usage:{input:100000,hit:0,miss:100000,output:1}},
 {id:2,session:'a'.repeat(24),task:'b'.repeat(24),model:'ordinary',usage:{input:1,hit:1,miss:0,output:0}},
 {id:3,session:'c'.repeat(24),task:'b'.repeat(24),model:'separate-session',usage:{input:2,hit:2,miss:0,output:0}},
 {id:4,model:'singleton',usage:{input:null,hit:null,miss:null,output:0}}];
 const render=(rs=rows,m=meta)=>page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows:rs,meta:m});
 await render();await page.getByText('AI费用风险提示',{exact:true}).click();await page.getByText('按任务查看用量',{exact:true}).click();await page.getByText('按提供商与模型查看用量',{exact:true}).click();
 const tasks=page.locator('#model-task-usage details'),summaries=page.locator('#model-task-usage summary');
 assert.equal(await tasks.count(),3);assert.equal(await page.locator('#model-task-usage details[open]').count(),0);
 assert.match(await summaries.nth(0).innerText(),/输入 100001（已知2\/2次）/);assert.match(await summaries.nth(2).innerText(),/独立请求 #4[\s\S]*输入 未知[\s\S]*输出 0/);
 assert.equal(await tasks.nth(0).locator('p').isVisible(),false);
 const download=async()=>{const wait=page.waitForEvent('download');await page.locator('#model-usage-export').click();const file=await(await wait).path();assert.equal((await checkUsageFile(file)).passed,true);const report=JSON.parse(await fs.readFile(file,'utf8'));delete report.exportedAt;return report;};
 const before=await download();
 await page.locator('#model-task-sort').focus();await page.keyboard.press('Tab');assert.equal(await page.getByRole('button',{name:'重置任务视图',exact:true}).evaluate(e=>e===document.activeElement),true);await page.keyboard.press('Tab');assert.equal(await summaries.nth(0).evaluate(e=>e===document.activeElement),true);
 await page.keyboard.press('Enter');assert.equal(await tasks.nth(0).locator('p').isVisible(),true);assert.match(await tasks.nth(0).locator('p').innerText(),/#1 [\s\S]*提示：大输入[\s\S]*#2 [\s\S]*未触发/);
 await render([...rows,{...rows[1],id:5,model:'new-round'}]);assert.equal(await tasks.nth(0).evaluate(e=>e.open),true);assert.equal(await summaries.nth(0).evaluate(e=>e===document.activeElement),true);assert.match(await tasks.nth(0).innerText(),/new-round/);
 await page.keyboard.press('Space');assert.equal(await tasks.nth(0).evaluate(e=>e.open),false);await page.keyboard.press('Enter');
 await render();assert.doesNotMatch(await tasks.nth(0).textContent(),/new-round/);
 await page.locator('#model-task-sort').focus();await page.locator('#model-task-sort').selectOption('miss');assert.equal(await page.locator('#model-task-sort').evaluate(e=>e===document.activeElement),true);assert.equal(await tasks.nth(0).evaluate(e=>e.open),true);
 await page.locator('#cost-large').fill('200000');assert.equal(await tasks.nth(0).evaluate(e=>e.open),true);assert.match(await tasks.nth(0).textContent(),/已应用阈值100000/);
 await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();assert.equal(await tasks.nth(0).evaluate(e=>e.open),true);assert.doesNotMatch(await tasks.nth(0).textContent(),/提示：大输入/);
 await page.getByRole('button',{name:'恢复默认提示',exact:true}).click();assert.equal(await tasks.nth(0).evaluate(e=>e.open),true);
 await page.locator('#model-task-filter').selectOption('large');assert.equal(await tasks.count(),1);assert.equal(await tasks.nth(0).evaluate(e=>e.open),true);assert.deepEqual(await download(),before);
 await page.locator('#model-task-filter').selectOption('invalid');await page.locator('#model-task-filter').selectOption('all');assert.equal(await tasks.nth(0).evaluate(e=>e.open),false);
 await summaries.nth(2).click();await render([rows[3],rows[0],rows[1],rows[2]]);assert.equal(await page.locator('#model-task-usage details[open] summary').innerText().then(s=>s.includes('独立请求 #4')),true);
 await render(rows,{startedAt:'2026-10-26T02:00:00Z',checkedAt:'2026-10-26T03:00:00Z'});assert.equal(await page.locator('#model-task-usage details[open]').count(),0);
 for(const clear of ['empty','invalid','failure','leave','logout']){
  await render();await summaries.nth(0).click();
  await page.evaluate(({clear,meta})=>{if(clear==='empty')renderModelUsage([],meta);if(clear==='invalid')renderModelUsage(null);if(clear==='failure')clearModelRequests();if(clear==='leave')showOverview(false);if(clear==='logout'){authenticated=false;resetOverview();}},{clear,meta});
  assert.equal(await tasks.count(),0);await page.evaluate(()=>authenticated=true);await render();assert.equal(await page.locator('#model-task-usage details[open]').count(),0);
 }
 assert.equal(requests,0);
});

test('ambiguous singleton identities do not inherit expansion',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());const page=await context.newPage();
 await page.setContent('<div id="model-task-usage"></div>');await page.evaluate(()=>window.$=id=>document.getElementById(id));
 await page.addScriptTag({content:await fs.readFile('internal/localadmin/assets/overview.js','utf8')});
 for(const id of [null,0,1]){
  const render=()=>page.evaluate(id=>{modelCostRows=[{id},{id}];modelUsageSnapshotMeta={startedAt:'2026-10-26',checkedAt:'2026-10-27'};renderModelTaskView();},id);
  await render();await page.locator('summary').nth(0).click();await render();assert.equal(await page.locator('details[open]').count(),0);
 }
});

test('outcome control combines at task scope and preserves lifecycle and full downloads',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());let network=0;await context.route('**/*',r=>{network++;return r.abort()});const page=await context.newPage(),root='internal/localadmin/assets/';
 await page.setContent((await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,''));
 await page.evaluate(()=>{window.$=id=>document.getElementById(id);window.authenticated=true;$('auth').hidden=true;$('workspace').hidden=false;$('overview-view').hidden=false;});await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 const meta={startedAt:'2026-10-27T00:00:00Z',checkedAt:'2026-10-27T01:00:00Z'},rows=[{id:1,session:'a'.repeat(24),task:'b'.repeat(24),outcome:'completed',usage:{input:100000,hit:0,miss:100000,output:1}},{id:2,session:'a'.repeat(24),task:'b'.repeat(24),outcome:'failed',usage:{input:1,hit:1,miss:0,output:0}},{id:3,outcome:'incomplete',usage:{input:1,hit:1,miss:0,output:0}},{id:4}];
 const render=(rs=rows,m=meta)=>page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows:rs,meta:m});await render();
 await page.getByText('AI费用风险提示',{exact:true}).click();await page.getByText('按任务查看用量',{exact:true}).click();await page.getByText('按提供商与模型查看用量',{exact:true}).click();
 const outcome=page.getByLabel('任务包含的请求结果',{exact:true}),tasks=page.locator('#model-task-usage details');assert.equal(await outcome.inputValue(),'all');
 const download=async()=>{const pending=page.waitForEvent('download');await page.locator('#model-usage-export').click();const file=await(await pending).path();assert.equal((await checkUsageFile(file)).passed,true);const data=JSON.parse(await fs.readFile(file,'utf8'));delete data.exportedAt;return data;};const before=await download();
 await outcome.focus();await page.keyboard.press('ArrowDown');assert.equal(await outcome.inputValue(),'completed');await tasks.nth(0).locator('summary').click();
 await outcome.selectOption('failed');await page.locator('#model-task-filter').selectOption('large');assert.equal(await tasks.count(),1);assert.equal(await tasks.nth(0).evaluate(e=>e.open),true);assert.match(await tasks.nth(0).textContent(),/#1 [\s\S]*completed[\s\S]*#2 [\s\S]*failed/);assert.deepEqual(await download(),before);
 await render();assert.equal(await outcome.inputValue(),'failed');assert.equal(await tasks.nth(0).evaluate(e=>e.open),true);
 await page.locator('#cost-large').fill('200000');assert.equal(await tasks.count(),1);await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();assert.equal(await tasks.count(),0);assert.equal(await outcome.inputValue(),'failed');
 await page.getByRole('button',{name:'恢复默认提示',exact:true}).click();assert.equal(await tasks.count(),1);assert.equal(await tasks.nth(0).evaluate(e=>e.open),false);
 await page.locator('#model-task-filter').selectOption('all');for(const value of ['incomplete','unknown']){await outcome.selectOption(value);assert.equal(await tasks.count(),1);}
 await outcome.selectOption('failed');await tasks.nth(0).locator('summary').click();await render(rows,{startedAt:'2026-10-27T02:00:00Z',checkedAt:'2026-10-27T03:00:00Z'});assert.equal(await outcome.inputValue(),'failed');assert.equal(await tasks.nth(0).evaluate(e=>e.open),false);
 await page.evaluate(()=>clearModelRequests());assert.equal(await outcome.isDisabled(),true);assert.equal(await outcome.inputValue(),'failed');await render();assert.equal(await outcome.isEnabled(),true);assert.equal(await tasks.count(),1);
 await page.evaluate(()=>showOverview(false));assert.equal(await outcome.inputValue(),'all');await render();await outcome.selectOption('failed');await page.evaluate(()=>{authenticated=false;resetOverview()});assert.equal(await outcome.inputValue(),'all');
 await page.evaluate(()=>authenticated=true);await render([]);assert.match(await page.locator('#model-task-usage').textContent(),/暂无近期请求/);
 await render();await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')});await page.addStyleTag({content:await fs.readFile(root+'appearance.css','utf8')});
 for(const width of [320,1280])for(const theme of ['light','dark']){await page.setViewportSize({width,height:900});await page.evaluate(t=>document.documentElement.dataset.theme=t,theme);assert.equal(await outcome.evaluate(e=>{const r=e.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&e.scrollWidth<=e.clientWidth+1}),true);}
 assert.equal(network,0);
});

test('reset task view restores full order and collapses details without changing thresholds or reports',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());let network=0;await context.route('**/*',r=>{network++;return r.abort()});const page=await context.newPage(),root='internal/localadmin/assets/';
 await page.setContent((await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,''));
 await page.evaluate(()=>{window.$=id=>document.getElementById(id);window.authenticated=true;$('auth').hidden=true;$('workspace').hidden=false;$('overview-view').hidden=false;});await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 const meta={startedAt:'2026-10-27T00:00:00Z',checkedAt:'2026-10-27T01:00:00Z'},rows=[{id:1,outcome:'completed',usage:{input:1,hit:1,miss:0,output:0}},{id:2,outcome:'failed',usage:{input:200,hit:0,miss:200,output:1}}];
 const render=(rs=rows,m=meta)=>page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows:rs,meta:m});await render();
 await page.getByText('AI费用风险提示',{exact:true}).click();await page.getByText('按任务查看用量',{exact:true}).click();await page.getByText('按提供商与模型查看用量',{exact:true}).click();
 const reset=page.getByRole('button',{name:'重置任务视图',exact:true}),tasks=page.locator('#model-task-usage details'),summaries=page.locator('#model-task-usage summary');
 const defaults=async()=>{for(const [id,value] of [['filter','all'],['outcome','all'],['sort','original']])assert.equal(await page.locator('#model-task-'+id).inputValue(),value);assert.equal(await page.locator('#model-task-usage details[open]').count(),0);};
 const download=async()=>{const pending=page.waitForEvent('download');await page.locator('#model-usage-export').click();const file=await(await pending).path();assert.equal((await checkUsageFile(file)).passed,true);const report=JSON.parse(await fs.readFile(file,'utf8'));delete report.exportedAt;return report;};
 await page.locator('#cost-large').fill('100');await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();await page.locator('#cost-large').fill('300');const before=await download();
 await page.locator('#model-task-filter').selectOption('large');await page.locator('#model-task-outcome').selectOption('failed');await page.locator('#model-task-sort').selectOption('input');await summaries.nth(0).click();assert.equal(await tasks.count(),1);
 await reset.click();await defaults();assert.equal(await tasks.count(),2);assert.match(await summaries.nth(0).textContent(),/独立请求 #1/);assert.match(await summaries.nth(1).textContent(),/独立请求 #2/);assert.equal(await page.locator('#cost-large').inputValue(),'300');assert.match(await tasks.nth(1).textContent(),/已应用阈值100/);assert.equal(await reset.evaluate(e=>e===document.activeElement),true);assert.deepEqual(await download(),before);
 await render();await defaults();await summaries.nth(0).click();await render();assert.equal(await tasks.nth(0).evaluate(e=>e.open),true);await reset.focus();await page.keyboard.press('Enter');await defaults();await summaries.nth(0).click();await reset.focus();await page.keyboard.press('Space');await defaults();await reset.click();await defaults();
 await page.locator('#model-task-outcome').selectOption('unknown');assert.match(await page.locator('#model-task-usage').textContent(),/当前条件无匹配任务/);await reset.click();assert.equal(await tasks.count(),2);
 await render([]);assert.equal(await reset.isEnabled(),true);await reset.click();assert.match(await page.locator('#model-task-usage').textContent(),/暂无近期请求/);
 await page.evaluate(()=>clearModelRequests());assert.equal(await reset.isDisabled(),true);assert.equal(await tasks.count(),0);await render();assert.equal(await reset.isEnabled(),true);
 for(const clear of ['leave','logout']){await page.locator('#model-task-outcome').selectOption('failed');await page.evaluate(clear=>{if(clear==='leave')showOverview(false);else{authenticated=false;resetOverview();}},clear);assert.equal(await reset.isDisabled(),true);assert.equal(await tasks.count(),0);await page.evaluate(()=>authenticated=true);await render();await defaults();}
 await summaries.nth(0).click();await render(rows,{startedAt:'2026-10-27T02:00:00Z',checkedAt:'2026-10-27T03:00:00Z'});await defaults();
 await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')});await page.addStyleTag({content:await fs.readFile(root+'appearance.css','utf8')});for(const width of [320,1280])for(const theme of ['light','dark']){await page.setViewportSize({width,height:900});await page.evaluate(t=>document.documentElement.dataset.theme=t,theme);assert.equal(await reset.evaluate(e=>{const r=e.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth}),true);}
 assert.equal(network,0);
});

test('collapsed summaries show full outcome distribution through filtering and fresh snapshots',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());let network=0;await context.route('**/*',r=>{network++;return r.abort()});const page=await context.newPage(),root='internal/localadmin/assets/';
 await page.setContent((await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,''));await page.evaluate(()=>{window.$=id=>document.getElementById(id);window.authenticated=true;$('auth').hidden=true;$('workspace').hidden=false;$('overview-view').hidden=false;});await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 const meta={startedAt:'2026-10-27T00:00:00Z',checkedAt:'2026-10-27T01:00:00Z'},rows=['completed','failed','incomplete',undefined].map((outcome,i)=>({id:i+1,outcome,session:'a'.repeat(24),task:'b'.repeat(24),usage:{input:1,hit:1,miss:0,output:0}}));
 const render=(rs=rows,m=meta)=>page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows:rs,meta:m});await render();await page.getByText('AI费用风险提示',{exact:true}).click();await page.getByText('按任务查看用量',{exact:true}).click();await page.getByText('按提供商与模型查看用量',{exact:true}).click();const summary=page.locator('#model-task-usage summary');
 assert.match(await summary.innerText(),/接口完成 1 · 失败 1 · 提前关闭 1 · 未知 1/);assert.equal(await page.locator('#model-task-usage details[open]').count(),0);
 await page.getByLabel('任务包含的请求结果',{exact:true}).selectOption('failed');assert.match(await summary.innerText(),/接口完成 1 · 失败 1 · 提前关闭 1 · 未知 1/);await summary.focus();await page.keyboard.press('Enter');await render([rows[0],rows[1]]);assert.match(await summary.innerText(),/接口完成 1 · 失败 1 · 提前关闭 0 · 未知 0/);assert.equal(await summary.evaluate(e=>e===document.activeElement),true);assert.equal(await page.locator('#model-task-usage details[open]').count(),1);
 await page.getByRole('button',{name:'重置任务视图',exact:true}).click();assert.equal(await page.locator('#model-task-usage details[open]').count(),0);assert.match(await summary.innerText(),/接口完成 1 · 失败 1/);
 const pending=page.waitForEvent('download');await page.getByRole('button',{name:'导出当前用量报告',exact:true}).click();const file=await(await pending).path();assert.equal((await checkUsageFile(file)).passed,true);const report=JSON.parse(await fs.readFile(file,'utf8'));assert.equal(report.version,1);assert.deepEqual(Object.keys(report.taskBreakdown.groups[0]).sort(),['cacheSummary','count','grouped','requests','session','task','totals']);assert.equal(report.requests.length,2);
 await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')});await page.addStyleTag({content:await fs.readFile(root+'appearance.css','utf8')});for(const width of [320,1280])for(const theme of ['light','dark']){await page.setViewportSize({width,height:900});await page.evaluate(t=>document.documentElement.dataset.theme=t,theme);assert.equal(await page.locator('#model-task-usage').evaluate(e=>e.scrollWidth<=e.clientWidth+1),true);}
 await render([rows[2]],{startedAt:'2026-10-27T02:00:00Z',checkedAt:'2026-10-27T03:00:00Z'});assert.match(await summary.innerText(),/接口完成 0 · 失败 0 · 提前关闭 1 · 未知 0/);await render([]);assert.equal(await summary.count(),0);await page.evaluate(()=>clearModelRequests());assert.equal(await summary.count(),0);await render();await page.evaluate(()=>showOverview(false));assert.equal(await summary.count(),0);await render();await page.evaluate(()=>{authenticated=false;resetOverview()});assert.equal(await summary.count(),0);assert.equal(network,0);
});

test('task layout separates labels and summary sections across viewport widths and themes',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());let network=0;await context.route('**/*',r=>{network++;return r.abort()});const page=await context.newPage(),root='internal/localadmin/assets/';
 await page.setContent((await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,''));await page.evaluate(()=>{window.$=id=>document.getElementById(id);window.authenticated=true;$('auth').hidden=true;$('workspace').hidden=false;$('overview-view').hidden=false;});await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 const meta={startedAt:'2026-10-27T00:00:00Z',checkedAt:'2026-10-27T01:00:00Z'},rows=[{id:1,session:'a'.repeat(24),task:'b'.repeat(24),model:'long-model-'.repeat(30),outcome:'completed',usage:{input:Number.MAX_SAFE_INTEGER,hit:0,miss:Number.MAX_SAFE_INTEGER,output:1}},{id:2,session:'a'.repeat(24),task:'b'.repeat(24),outcome:'failed'},{id:3,outcome:'incomplete'}];await page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows,meta});
 await page.getByText('AI费用风险提示',{exact:true}).click();await page.getByText('按任务查看用量',{exact:true}).click();await page.getByText('按提供商与模型查看用量',{exact:true}).click();
 const download=async()=>{const pending=page.waitForEvent('download');await page.getByRole('button',{name:'导出当前用量报告',exact:true}).click();const file=await(await pending).path();assert.equal((await checkUsageFile(file)).passed,true);const r=JSON.parse(await fs.readFile(file,'utf8'));delete r.exportedAt;return r;};const before=await download();
 await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')});await page.addStyleTag({content:await fs.readFile(root+'appearance.css','utf8')});const summary=page.locator('#model-task-usage summary').first();await summary.click();
 for(const width of [320,640,768,1280])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:1000});await page.evaluate(t=>document.documentElement.dataset.theme=t,theme);
  const geometry=await page.evaluate(()=>{const fields=Array.from(document.querySelectorAll('.model-task-field'));const boxes=fields.map(e=>e.getBoundingClientRect());const s=document.querySelector('#model-task-usage summary'),node=s.firstChild,parts=node.textContent.split('\n');let offset=0;const tops=parts.map(p=>{const range=document.createRange();range.setStart(node,offset);range.setEnd(node,offset+1);offset+=p.length+1;return range.getBoundingClientRect().top;});return {labelsAbove:fields.every(e=>e.querySelector('label').getBoundingClientRect().bottom<=e.querySelector('select').getBoundingClientRect().top),controlsFit:fields.every(e=>{const r=e.querySelector('select').getBoundingClientRect();return r.height>=44&&r.left>=0&&r.right<=innerWidth}),ordered:boxes.every((r,i)=>!i||r.top>boxes[i-1].top||r.left>=boxes[i-1].right),stacked:boxes.every((r,i)=>!i||r.top>=boxes[i-1].bottom),tops,display:getComputedStyle(s).display,pageFits:document.documentElement.scrollWidth<=innerWidth+1,bodyFits:document.querySelector('#model-task-usage').scrollWidth<=document.querySelector('#model-task-usage').clientWidth+1};});
  assert.equal(geometry.labelsAbove,true);assert.equal(geometry.controlsFit,true);assert.equal(geometry.ordered,true);if(width<=640)assert.equal(geometry.stacked,true);assert.equal(geometry.tops.length,5);assert.equal(geometry.tops.every((v,i)=>!i||v>geometry.tops[i-1]),true);assert.equal(geometry.display,'list-item');assert.equal(geometry.pageFits,true);assert.equal(geometry.bodyFits,true);
  if(width===320||width===1280){await page.locator('.model-task-toolbar').scrollIntoViewIfNeeded();await page.screenshot({path:'.local/task-risk-counts/'+width+'-'+theme+'.png',fullPage:true});await page.locator('.model-task-toolbar').screenshot({path:'.local/task-risk-counts/'+width+'-'+theme+'-controls.png'});await summary.screenshot({path:'.local/task-risk-counts/'+width+'-'+theme+'-summary.png'});}
 }
 await page.setViewportSize({width:1280,height:1000});await page.getByLabel('任务风险筛选',{exact:true}).focus();for(const id of ['model-task-outcome','model-task-sort','model-task-reset']){await page.keyboard.press('Tab');assert.equal(await page.locator('#'+id).evaluate(e=>e===document.activeElement),true);}await page.keyboard.press('Tab');assert.equal(await summary.evaluate(e=>e===document.activeElement),true);assert.notEqual(await summary.evaluate(e=>getComputedStyle(e).outlineStyle),'none');await page.keyboard.press('Space');assert.equal(await page.locator('#model-task-usage details[open]').count(),0);await page.keyboard.press('Enter');
 await page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows,meta});assert.equal(await summary.evaluate(e=>e===document.activeElement),true);assert.equal(await page.locator('#model-task-usage details[open]').count(),1);assert.deepEqual(await download(),before);assert.equal(network,0);
});

test('summary risks follow applied thresholds and retain full downloads through view changes',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());let network=0;await context.route('**/*',r=>{network++;return r.abort()});const page=await context.newPage(),root='internal/localadmin/assets/';
 await page.setContent((await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,''));await page.evaluate(()=>{window.$=id=>document.getElementById(id);window.authenticated=true;$('auth').hidden=true;$('workspace').hidden=false;$('overview-view').hidden=false;});await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 const meta={startedAt:'2026-10-27T00:00:00Z',checkedAt:'2026-10-28T00:00:00Z'},rows=[{id:1,session:'a'.repeat(24),task:'b'.repeat(24),outcome:'completed',usage:{input:100,hit:0,miss:100,output:1}},{id:2,session:'a'.repeat(24),task:'b'.repeat(24),outcome:'failed',usage:{input:1,hit:1,miss:0,output:0}}];const render=(rs=rows,m=meta)=>page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows:rs,meta:m});await render();await page.getByText('AI费用风险提示',{exact:true}).click();await page.getByText('按任务查看用量',{exact:true}).click();await page.getByText('按提供商与模型查看用量',{exact:true}).click();const summary=page.locator('#model-task-usage summary'),text=()=>summary.innerText();
 const download=async()=>{const pending=page.waitForEvent('download');await page.getByRole('button',{name:'导出当前用量报告',exact:true}).click();const file=await(await pending).path();assert.equal((await checkUsageFile(file)).passed,true);const r=JSON.parse(await fs.readFile(file,'utf8'));delete r.exportedAt;return r;};const before=await download();
 assert.match(await text(),/大输入 0次/);await page.locator('#cost-large').fill('100');assert.match(await text(),/大输入 0次/);await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();assert.match(await text(),/大输入 1次/);const applied=await download();assert.deepEqual(applied.requests,before.requests);assert.deepEqual(applied.taskBreakdown,before.taskBreakdown);assert.equal(applied.thresholds.large,100);
 await page.locator('#cost-large').fill('0');await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();assert.match(await text(),/大输入 1次/);await page.locator('#cost-large').fill('200');
 await page.getByLabel('任务风险筛选',{exact:true}).selectOption('large');await page.getByLabel('任务包含的请求结果',{exact:true}).selectOption('failed');await summary.click();await render();assert.match(await text(),/接口完成 1 · 失败 1/);assert.match(await text(),/大输入 1次/);assert.equal(await page.locator('#model-task-usage details').evaluate(e=>e.open),true);assert.deepEqual(await download(),applied);
 await page.getByRole('button',{name:'重置任务视图',exact:true}).click();assert.match(await text(),/大输入 1次/);assert.equal(await page.locator('#cost-large').inputValue(),'200');assert.deepEqual(await download(),applied);
 await page.getByRole('button',{name:'恢复默认提示',exact:true}).click();assert.match(await text(),/大输入 0次/);assert.deepEqual(await download(),before);
 await render([rows[1]],{startedAt:'2026-10-29',checkedAt:'2026-10-30'});assert.match(await text(),/接口完成 0 · 失败 1/);assert.match(await text(),/大输入 0次/);await render([]);assert.equal(await summary.count(),0);await render();await page.evaluate(()=>clearModelRequests());assert.equal(await summary.count(),0);assert.equal(network,0);
});

test('any risk native selection follows applied settings, lifecycle and complete exports',async t=>{
 const context=await browser.newContext();t.after(()=>context.close());let network=0;await context.route('**/*',r=>{network++;return r.abort()});const page=await context.newPage(),root='internal/localadmin/assets/';
 await page.setContent((await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,''));await page.evaluate(()=>{window.$=id=>document.getElementById(id);window.authenticated=true;$('auth').hidden=true;$('workspace').hidden=false;$('overview-view').hidden=false;});await page.addScriptTag({content:await fs.readFile(root+'overview.js','utf8')});
 const meta={startedAt:'2026-10-27T00:00:00Z',checkedAt:'2026-10-28T00:00:00Z'},rows=[{id:1,session:'a'.repeat(24),task:'b'.repeat(24),outcome:'completed',usage:{input:100,hit:100,miss:0,output:1}},{id:2,session:'a'.repeat(24),task:'b'.repeat(24),outcome:'failed',usage:{input:1,hit:1,miss:0,output:0}}];const render=(rs=rows,m=meta)=>page.evaluate(({rows,meta})=>renderModelUsage(rows,meta),{rows:rs,meta:m});await render();await page.getByText('AI费用风险提示',{exact:true}).click();await page.getByText('按任务查看用量',{exact:true}).click();await page.getByText('按提供商与模型查看用量',{exact:true}).click();const risk=page.getByLabel('任务风险筛选',{exact:true}),tasks=page.locator('#model-task-usage details');
 const download=async()=>{const pending=page.waitForEvent('download');await page.getByRole('button',{name:'导出当前用量报告',exact:true}).click();const f=await(await pending).path();assert.equal((await checkUsageFile(f)).passed,true);const r=JSON.parse(await fs.readFile(f,'utf8'));delete r.exportedAt;return r;};const before=await download();
 await risk.focus();await page.keyboard.press('ArrowDown');assert.equal(await risk.inputValue(),'anyRisk');assert.equal(await tasks.count(),0);assert.deepEqual(await download(),before);await page.locator('#cost-large').fill('100');assert.equal(await tasks.count(),0);await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();assert.equal(await tasks.count(),1);await page.getByLabel('任务包含的请求结果',{exact:true}).selectOption('failed');assert.match(await tasks.innerText(),/窗口内2次请求/);await tasks.locator('summary').click();await render();assert.equal(await tasks.evaluate(e=>e.open),true);assert.equal(await risk.inputValue(),'anyRisk');const applied=await download();assert.deepEqual(applied.requests,before.requests);assert.deepEqual(applied.taskBreakdown,before.taskBreakdown);
 await page.locator('#cost-large').fill('0');await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();assert.equal(await tasks.count(),1);await page.getByRole('button',{name:'恢复默认提示',exact:true}).click();assert.equal(await tasks.count(),0);await page.locator('#cost-large').fill('1');await page.getByRole('button',{name:'应用到本页提示',exact:true}).click();assert.equal(await tasks.evaluate(e=>e.open),false);
 await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')});await page.addStyleTag({content:await fs.readFile(root+'appearance.css','utf8')});
 for(const width of [320,640,768,1280])for(const theme of ['light','dark']){await page.setViewportSize({width,height:900});await page.evaluate(t=>document.documentElement.dataset.theme=t,theme);assert.equal(await risk.evaluate(e=>{const r=e.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&document.documentElement.scrollWidth<=innerWidth+1}),true);if(width===320||width===1280)await page.locator('.model-task-toolbar').screenshot({path:'.local/task-any-risk/'+width+'-'+theme+'.png'});}
 await page.evaluate(()=>clearModelRequests());assert.equal(await risk.isDisabled(),true);await render();assert.equal(await risk.inputValue(),'anyRisk');await tasks.locator('summary').click();await render(rows,{startedAt:'2026-10-29T00:00:00Z',checkedAt:'2026-10-30T00:00:00Z'});assert.equal(await tasks.evaluate(e=>e.open),false);
 await page.getByRole('button',{name:'重置任务视图',exact:true}).click();assert.equal(await risk.inputValue(),'all');assert.equal(await page.locator('#cost-large').inputValue(),'1');await render([]);await risk.selectOption('anyRisk');assert.match(await page.locator('#model-task-usage').innerText(),/暂无近期请求/);
 for(const leave of [false,true]){await render();await risk.selectOption('anyRisk');await page.evaluate(leave=>{if(leave){authenticated=false;resetOverview()}else showOverview(false)},leave);assert.equal(await risk.inputValue(),'all');assert.equal(await tasks.count(),0);await page.evaluate(()=>authenticated=true);}assert.equal(network,0);
});
