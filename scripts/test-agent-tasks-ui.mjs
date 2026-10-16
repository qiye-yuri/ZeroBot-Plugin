import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
import test from 'node:test';
const require=createRequire(import.meta.url),modules=process.env.LOCALADMIN_NODE_MODULES||path.join(process.env.USERPROFILE,'.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules');
const {chromium}=require(path.join(modules,'playwright'));
const root='internal/localadmin/assets/';
const html=(await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,'');
const scripts=await Promise.all(['app.js','agent-archive.js','agent-memory.js','agent-tasks.js'].map(p=>fs.readFile(root+p,'utf8')));
const css=await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8');
const item={taskId:'a'.repeat(32),instance:'b'.repeat(32),version:'9007199254740993',status:'running',phase:'model_wait',action:'<img src=x>',outcome:'ok',startedAt:'2026-10-13T00:00:00Z',endedAt:'',persisted:true,context:{records:3,searchSources:2,historyBytes:1000,budgetBytes:2048,clipped:true,available:true}};
async function fixture(t){
 const browser=await chromium.launch({channel:'chrome',headless:true});t.after(()=>browser.close());const context=await browser.newContext();await context.route('**/*',r=>r.abort());const page=await context.newPage();await page.setContent(html);await page.addStyleTag({content:css});for(const content of scripts)await page.addScriptTag({content});
 await page.evaluate(()=>{
  authenticated=true;csrf='task-fixture';agentVisible=true;agentScope={botId:'12345678',type:'group',id:'10',archived:false};window.showOverview=()=>{};
  document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.getElementById('overview-view').hidden=true;document.getElementById('agent-view').hidden=false;
  document.querySelector('[data-view=overview]').classList.remove('active');document.querySelector('[data-view=agent]').classList.add('active');
  window.requests=[];window.replies=[];window.pending=[];window.fetch=(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const value=replies.shift();if(value)return Promise.resolve({ok:!value.error||typeof value.taskId==='string',status:value.httpStatus||200,json:async()=>value});return new Promise(resolve=>pending.push(resolve));};agentControls();
 });return page;
}
async function load(page,items=[item]){await page.evaluate(items=>replies.push({items}),items);await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);}

test('task outcome filters combine with applied conditions and preserve paging and independent exact reads',async t=>{
 const page=await fixture(t),task={...item,outcome:'in_flight',phase:'action_wait'},scope={botId:'12345678',type:'group',id:'10'};await page.locator('#tasks-filter').selectOption('running');await page.locator('#tasks-reason').selectOption('none');await page.locator('#tasks-phase').selectOption('action_wait');await page.locator('#tasks-outcome').selectOption('in_flight');await page.locator('#tasks-from').fill('2026-10-16T00:00');assert.equal(await page.evaluate(()=>requests.length),0);const from=await page.evaluate(()=>new Date(document.getElementById('tasks-from').value).toISOString());
 await page.evaluate(value=>replies.push(value),{items:[task],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);const expected={...scope,state:'running',reason:'none',phase:'action_wait',outcome:'in_flight',from};assert.deepEqual(Object.fromEntries(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams),expected);assert.match(await page.locator('#tasks-status').textContent(),/最近动作结果：等待回执（非累计结果）/);
 await page.locator('#tasks-outcome').selectOption('unknown');await page.locator('#tasks-from').fill('2027-01-01T00:00');await page.evaluate(()=>{agentScope={botId:'87654321',type:'private',id:'20'}});await page.evaluate(value=>replies.push(value),{items:[task]});await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(Object.fromEntries(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams),{...expected,before:'9007199254740993'});
 await page.evaluate(value=>replies.push(value),task);await page.locator('#tasks-list').getByRole('button',{name:'核对最新状态',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(Object.fromEntries(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams),{...scope,taskId:task.taskId});assert.equal(await page.evaluate(()=>tasksOutcome),'in_flight');
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/task-outcome-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.locator('#tasks-outcome').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/task-outcome-ui/'+theme+'.png'});}
 await page.evaluate(value=>replies.push(value),{...task,status:'cancel_requested'});await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-outcome').inputValue(),'all');assert.equal(await page.evaluate(()=>tasksOutcome),'all');assert.match(await page.locator('#tasks-status').textContent(),/最近动作结果：全部结果/);assert.equal(Object.hasOwn(await page.evaluate(()=>requests.at(-1).body),'outcome'),false);
});

test('task outcome options errors and late reads keep list state consistent',async t=>{
 const page=await fixture(t);for(const outcome of ['all','none','other','in_flight','ok','rejected','unknown','local_error']){await page.locator('#tasks-outcome').selectOption(outcome);await load(page,[]);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.get('outcome'),outcome==='all'?null:outcome);assert.equal(await page.evaluate(()=>tasksOutcome),outcome);}
 await page.locator('#tasks-outcome').selectOption('unknown');await page.evaluate(value=>replies.push(value),{items:[{...item,status:'failed',outcome:'unknown'}],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);await page.evaluate(()=>replies.push({error:'读取失败',httpStatus:503}));await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.locator('#tasks-next').isDisabled(),true);assert.match(await page.locator('#tasks-status').textContent(),/读取失败/);
 await page.locator('#tasks-load').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#tasks-outcome').isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),{items:[item]});await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-outcome').inputValue(),'all');assert.equal(await page.evaluate(()=>tasksOutcome),'all');assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.evaluate(()=>tasksScope),null);
});

test('task start ranges combine with filters and paging keeps applied times while exact reads stay independent',async t=>{
 const page=await fixture(t);await page.locator('#tasks-from').fill('2026-10-15T08:00');await page.locator('#tasks-until').fill('2026-10-16T08:00');await page.locator('#tasks-filter').selectOption('running');await page.locator('#tasks-phase').selectOption('model_wait');assert.equal(await page.evaluate(()=>requests.length),0);
 const [from,until]=await page.evaluate(()=>[document.getElementById('tasks-from').value,document.getElementById('tasks-until').value].map(value=>new Date(value).toISOString()));await page.evaluate(value=>replies.push(value),{items:[item],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);
 const scope={botId:'12345678',type:'group',id:'10'};assert.deepEqual(Object.fromEntries(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams),{...scope,state:'running',phase:'model_wait',from,until});assert.match(await page.locator('#tasks-status').textContent(),new RegExp(from.replaceAll('.','\\.')));
 await page.locator('#tasks-from').fill('2026-11-01T00:00');await page.locator('#tasks-until').fill('2026-10-01T00:00');await page.evaluate(()=>{agentScope={botId:'87654321',type:'private',id:'20'}});await page.evaluate(value=>replies.push(value),{items:[item]});await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(Object.fromEntries(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams),{...scope,state:'running',phase:'model_wait',from,until,before:'9007199254740993'});
 await page.evaluate(value=>replies.push(value),item);await page.locator('#tasks-list').getByRole('button',{name:'核对最新状态',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(Object.fromEntries(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams),{...scope,taskId:item.taskId});assert.equal(await page.evaluate(()=>tasksFrom),from);
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/task-range-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.locator('#tasks-from').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/task-range-ui/'+theme+'.png'});}
 await page.evaluate(value=>replies.push(value),{...item,status:'cancel_requested'});await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-from').inputValue(),'');assert.equal(await page.locator('#tasks-until').inputValue(),'');assert.deepEqual(await page.evaluate(()=>[tasksFrom,tasksUntil]),['','']);assert.match(await page.locator('#tasks-status').textContent(),/任务开始时间：不限/);
});

test('task start ranges reject invalid drafts without requests and clear failures and late replies on leaving',async t=>{
 const page=await fixture(t);await page.locator('#tasks-from').fill('2026-10-16T00:00');await page.locator('#tasks-until').fill('2026-10-16T00:00');await page.locator('#tasks-load').click();assert.equal(await page.evaluate(()=>requests.length),0);assert.match(await page.locator('#agent-error').textContent(),/下限须早于上限/);
 await page.locator('#tasks-until').fill('2026-10-15T00:00');await page.locator('#tasks-load').click();assert.equal(await page.evaluate(()=>requests.length),0);await page.locator('#tasks-until').fill('');await page.evaluate(value=>replies.push(value),{items:[item],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.has('until'),false);
 await page.evaluate(()=>replies.push({error:'读取失败',httpStatus:503}));await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.locator('#tasks-next').isDisabled(),true);assert.match(await page.locator('#tasks-status').textContent(),/读取失败/);
 await page.locator('#tasks-load').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#tasks-from').isDisabled(),true);assert.equal(await page.locator('#tasks-until').isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),{items:[item]});await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.locator('#tasks-from').inputValue(),'');assert.equal(await page.locator('#tasks-until').inputValue(),'');assert.deepEqual(await page.evaluate(()=>[tasksFrom,tasksUntil,tasksScope]),['','',null]);
});

test('task statistics fold independently without requests and single lookup opens them',async t=>{
 const page=await fixture(t),other={...item,taskId:'c'.repeat(32),status:'failed',error:'model_request_failed'};await page.evaluate(value=>replies.push(value),{items:[item,other],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);const details=page.locator('#tasks-list .task-statistics'),count=await page.evaluate(()=>requests.length),status=await page.locator('#tasks-status').textContent();assert.deepEqual(await details.evaluateAll(nodes=>nodes.map(node=>node.open)),[false,false]);
 assert.equal(await details.first().locator('p').first().isVisible(),false);assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).isVisible(),true);assert.equal(await page.getByText('状态已保存。 Agent 模型请求或响应解析失败',{exact:true}).isVisible(),true);
 await details.first().locator('summary').focus();await page.keyboard.press('Enter');assert.equal(await details.first().evaluate(node=>node.open),true);assert.equal(await details.nth(1).evaluate(node=>node.open),false);assert.equal(await details.first().locator('p').first().isVisible(),true);assert.equal(await page.evaluate(()=>requests.length),count);assert.equal(await page.locator('#tasks-status').textContent(),status);assert.equal(await page.locator('#tasks-next').isEnabled(),true);
 await page.evaluate(value=>replies.push(value),{...item,version:'9007199254740994'});await page.locator('#tasks-list').getByRole('button',{name:'核对最新状态',exact:true}).first().click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#task-read-detail .task-statistics').evaluate(node=>node.open),true);assert.equal(await details.first().evaluate(node=>node.open),true);const after=await page.evaluate(()=>requests.length);await page.locator('#task-read-detail summary').click();assert.equal(await page.locator('#task-read-detail .task-statistics').evaluate(node=>node.open),false);assert.equal(await page.evaluate(()=>requests.length),after);
 await page.evaluate(value=>replies.push(value),{items:[item]});await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-list .task-statistics').evaluate(node=>node.open),false);assert.equal(await page.locator('#task-read-detail').textContent(),'');
});

test('task statistics keep faults visible and fit narrow themes when collapsed and expanded',async t=>{
 const page=await fixture(t),failed={...item,status:'failed',persisted:false,error:'storage_error',outcome:'unknown'};await load(page,[failed]);const details=page.locator('#tasks-list .task-statistics');assert.equal(await details.evaluate(node=>node.open),false);assert.equal(await page.getByText(/状态未可靠保存，此会话暂停新任务/).isVisible(),true);assert.equal(await page.getByText(/阶段：.*结果未知，不自动重发/).isVisible(),true);assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'核对最新状态',exact:true}).isVisible(),true);
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/task-statistics-ui',{recursive:true});const count=await page.evaluate(()=>requests.length);for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);for(const open of [false,true]){if(await details.evaluate(node=>node.open)!==open)await details.locator('summary').click();await details.locator('summary').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/task-statistics-ui/'+theme+'-'+(open?'open':'closed')+'.png'});}}assert.equal(await page.evaluate(()=>requests.length),count);
 await page.locator('[data-view=overview]').click();assert.equal(await page.locator('.task-statistics').count(),0);assert.equal(await page.evaluate(()=>tasksScope),null);
});

test('task latest entry binds the selected row and preserves loaded list filters and paging',async t=>{
 const page=await fixture(t),selected={...item,taskId:'c'.repeat(32)},scope={botId:'12345678',type:'group',id:'10'};
 await page.locator('#tasks-filter').selectOption('running');await page.locator('#tasks-phase').selectOption('model_wait');await page.evaluate(value=>replies.push(value),{items:[item,selected],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);
 const original=await page.locator('#tasks-list').textContent(),status=await page.locator('#tasks-status').textContent();await page.locator('#task-read-id').fill(item.taskId);await page.locator('#tasks-filter').selectOption('cancelled');await page.locator('#tasks-reason').selectOption('none');await page.locator('#tasks-phase').selectOption('finished');
 const latest={...selected,status:'cancel_requested',phase:'action_wait',version:'9007199254740994',sourceSeq:'23',observedAt:'2026-10-16T00:00:00Z'};
 await page.evaluate(value=>{agentScope={botId:'87654321',type:'private',id:'20'};replies.push(value)},latest);await page.locator('#tasks-list').getByRole('button',{name:'核对最新状态',exact:true}).nth(1).click();await page.waitForFunction(()=>!agentBusy);
 const request=await page.evaluate(()=>requests.at(-1)),url=new URL(request.url,'http://fixture');assert.deepEqual(Object.fromEntries(url.searchParams),{...scope,taskId:selected.taskId});assert.equal(request.body,null);assert.equal(await page.locator('#task-read-id').inputValue(),selected.taskId);assert.match(await page.locator('#task-read-detail').textContent(),/已请求停止.*v9007199254740994.*动作在途/s);assert.equal(await page.locator('#task-read-detail').getByRole('button',{name:'核对最新状态',exact:true}).count(),0);assert.equal(await page.locator('#task-read-detail').getByRole('button',{name:'请求停止此任务',exact:true}).count(),0);assert.equal(await page.locator('#task-read-detail').getByRole('button',{name:'查看触发消息',exact:true}).count(),1);assert.equal(await page.locator('#tasks-list').textContent(),original);assert.equal(await page.locator('#tasks-status').textContent(),status);assert.equal(await page.locator('#tasks-next').isEnabled(),true);
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/task-latest-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.locator('#task-read-status').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/task-latest-ui/'+theme+'.png'});}
 await page.evaluate(value=>replies.push(value),{items:[selected]});await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(Object.fromEntries(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams),{...scope,state:'running',phase:'model_wait',before:'9007199254740993'});assert.equal(await page.locator('#task-read-detail').textContent(),'');
});

test('task latest entry handles failed or mismatched reads and ignores late responses after leaving',async t=>{
 const page=await fixture(t),failed={...item,status:'failed',error:'storage_error',persisted:false};await page.evaluate(value=>replies.push(value),{items:[failed],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);const original=await page.locator('#tasks-list').textContent(),button=page.locator('#tasks-list').getByRole('button',{name:'核对最新状态',exact:true});assert.equal(await button.count(),1);
 for(const value of [{error:'任务不存在',httpStatus:404},{error:'存储不可用',httpStatus:503},{...failed,taskId:'b'.repeat(32)},{...failed,version:'9223372036854775808'}]){
  const before=await page.evaluate(()=>requests.length);await page.evaluate(value=>replies.push(value),value);await button.click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#task-read-detail').textContent(),'');assert.match(await page.locator('#task-read-status').textContent(),/核对失败.*不能据此确认任务已结束/);assert.equal(await page.locator('#tasks-list').textContent(),original);assert.equal(await page.locator('#tasks-next').isEnabled(),true);assert.equal(await page.evaluate(()=>requests.length),before+1);
 }
 await button.click();await page.waitForFunction(()=>pending.length===1);assert.equal(await button.isDisabled(),true);assert.equal(await page.locator('#task-read-id').isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),{...failed,status:'completed',version:'2'});await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.locator('#task-read-detail').textContent(),'');assert.equal(await page.locator('#task-read-id').inputValue(),'');assert.equal(await page.evaluate(()=>tasksScope),null);
});

test('task phases combine with status and reason and remain applied across pagination',async t=>{
 const page=await fixture(t),task={...item,status:'cancel_requested'};
 await page.locator('#tasks-filter').selectOption('cancel_requested');await page.locator('#tasks-reason').selectOption('none');await page.locator('#tasks-phase').selectOption('model_wait');assert.equal(await page.evaluate(()=>requests.length),0);
 await page.evaluate(value=>replies.push(value),{items:[task],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);
 let url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.deepEqual(Object.fromEntries(url.searchParams),{botId:'12345678',type:'group',id:'10',state:'cancel_requested',reason:'none',phase:'model_wait'});assert.match(await page.locator('#tasks-status').textContent(),/阶段：等待模型/);
 await page.locator('#tasks-phase').selectOption('action_wait');await page.locator('#tasks-filter').selectOption('running');await page.locator('#tasks-reason').selectOption('other');await page.evaluate(value=>{agentScope={botId:'87654321',type:'private',id:'20'};replies.push(value)},{items:[task],more:true,next:'9007199254740991'});
 await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.deepEqual(Object.fromEntries(url.searchParams),{botId:'12345678',type:'group',id:'10',state:'cancel_requested',reason:'none',phase:'model_wait',before:'9007199254740993'});
 await page.locator('#task-read-id').fill(task.taskId);await page.evaluate(value=>replies.push(value),task);await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.doesNotMatch(await page.evaluate(()=>requests.at(-1).url),/phase=|state=|reason=/);assert.match(await page.locator('#tasks-status').textContent(),/阶段：等待模型/);
 await load(page,[]);url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.equal(url.searchParams.get('phase'),'action_wait');assert.equal(url.searchParams.get('id'),'20');assert.match(await page.locator('#tasks-status').textContent(),/阶段：动作在途/);
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/task-phase-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.locator('#tasks-phase').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/task-phase-ui/'+theme+'.png'});}
 await page.locator('[data-view=overview]').click();assert.equal(await page.locator('#tasks-phase').inputValue(),'all');assert.equal(await page.evaluate(()=>tasksPhase),'all');
});

test('task phase failures and late reads clear old results and stopping resets filters',async t=>{
 const page=await fixture(t);await page.locator('#tasks-phase').selectOption('model_wait');await page.evaluate(value=>replies.push(value),{items:[item],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);
 await page.evaluate(()=>replies.push({error:'存储不可用',httpStatus:503}));await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.locator('#tasks-next').isDisabled(),true);
 await load(page);await page.evaluate(value=>replies.push(value),{...item,status:'cancel_requested'});await page.getByRole('button',{name:'请求停止此任务'}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-phase').inputValue(),'all');assert.equal(await page.evaluate(()=>tasksPhase),'all');assert.match(await page.locator('#tasks-status').textContent(),/阶段：全部阶段/);assert.doesNotMatch(JSON.stringify(await page.evaluate(()=>requests.at(-1))),/"phase"/);
 await page.locator('#tasks-phase').selectOption('fallback_model_wait');await page.locator('#tasks-load').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#tasks-phase').isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),{items:[item],more:true,next:'1'});await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.evaluate(()=>tasksPhase),'all');
});

test('task durations use the server snapshot and update only after an explicit read',async t=>{
 const page=await fixture(t);const task={...item,status:'cancel_requested',startedAt:'2026-10-15T00:00:00.123456789Z',phaseStartedAt:'2026-10-15T00:01:00.123456789Z',cancelRequestedAt:'2026-10-15T00:01:10.123456789Z'};
 await page.evaluate(value=>replies.push(value),{items:[task],observedAt:'2026-10-15T00:01:15.123456789Z'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);const duration=page.locator('#tasks-list .task-duration');assert.match(await duration.textContent(),/已运行：约 1 分 15 秒.*当前阶段已持续：约 15 秒.*停止请求已等待：约 5 秒.*读取时刻：2026-10-15T00:01:15.123456789Z/s);
 const original=await duration.textContent(),count=await page.evaluate(()=>requests.length);await page.evaluate(()=>{Date.now=()=>Date.parse('2099-01-01T00:00:00Z');});assert.equal(await duration.textContent(),original);assert.equal(await page.evaluate(()=>requests.length),count);
 await page.locator('#task-read-id').fill(task.taskId);await page.evaluate(value=>replies.push(value),{...task,observedAt:'2026-10-15T00:01:25.123456789Z'});await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#task-read-detail .task-duration').textContent(),/已运行：约 1 分 25 秒.*阶段已持续：约 25 秒.*已等待：约 15 秒/s);assert.equal(await duration.textContent(),original);
 const ended={...task,status:'cancelled',phase:'finished',endedAt:'2026-10-15T00:01:30.123456789Z',phaseStartedAt:'2026-10-15T00:01:30.123456789Z',observedAt:'2026-10-15T12:00:00Z'};await page.evaluate(value=>replies.push(value),ended);await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#task-read-detail .task-duration').textContent(),/任务总耗时：约 1 分 30 秒.*停止请求至任务结束：约 20 秒/s);assert.doesNotMatch(await page.locator('#task-read-detail .task-duration').textContent(),/当前阶段已持续|读取时刻/);
});

test('task durations handle terminal old uncertain and reversed timestamps independently',async t=>{
 const page=await fixture(t);const done={...item,status:'completed',phase:'finished',startedAt:'2026-10-15T00:00:00Z',endedAt:'2026-10-15T01:02:03Z'};await load(page,[done]);assert.match(await page.locator('.task-duration').textContent(),/任务总耗时：约 1 小时 2 分 3 秒/);
 await load(page,[{...done,status:'failed',endedAt:'2026-10-15T00:00:00.999999999Z'}]);assert.match(await page.locator('.task-duration').textContent(),/总耗时：约 0 秒/);
 await load(page,[{...item,status:'running',observedAt:'2026-10-13T00:00:05Z'}]);assert.match(await page.locator('.task-duration').textContent(),/已运行：约 5 秒.*阶段已持续：未知/s);
 for(const task of [{...item},{...done,endedAt:'2026-10-14T23:59:59Z'},{...done,endedAt:'2026-02-30T00:00:00Z'},{...done,endedAt:'<img src=x>'}]){await load(page,[task]);assert.match(await page.locator('.task-duration').textContent(),/未知（时间缺失、无效或先后异常）/);assert.equal(await page.locator('.task-duration img').count(),0);}
 await load(page,[{...done,status:'cancelled'}]);assert.match(await page.locator('.task-duration').textContent(),/任务总耗时：约 1 小时 2 分 3 秒.*停止请求至任务结束：未知/s);
 await load(page,[{...done,status:'interrupted'}]);assert.match(await page.locator('.task-duration').textContent(),/实际退出时刻未知/);assert.doesNotMatch(await page.locator('.task-duration').textContent(),/任务总耗时：/);
 await load(page,[{...done,persisted:false}]);assert.match(await page.locator('.task-duration').textContent(),/状态未可靠保存，暂不计算耗时/);assert.doesNotMatch(await page.locator('.task-duration').textContent(),/任务总耗时：/);
});

test('stop response durations and narrow layouts share the same calculation and clear on leaving',async t=>{
 const page=await fixture(t);const task={...item,startedAt:'2026-10-15T00:00:00Z',phaseStartedAt:'2026-10-15T00:01:00Z'};await page.evaluate(value=>replies.push(value),{items:[task],observedAt:'2026-10-15T00:01:01Z'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);
 await page.evaluate(value=>replies.push(value),{...task,status:'cancel_requested',cancelRequestedAt:'2026-10-15T00:01:02Z',observedAt:'2026-10-15T00:01:03Z'});await page.getByRole('button',{name:'请求停止此任务'}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('.task-duration').textContent(),/已运行：约 1 分 3 秒.*阶段已持续：约 3 秒.*已等待：约 1 秒/s);
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/task-duration-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.locator('.task-duration').scrollIntoViewIfNeeded();await page.screenshot({path:'.local/task-duration-ui/'+theme+'.png'});}
 await page.locator('[data-view=overview]').click();assert.equal(await page.locator('.task-duration').count(),0);assert.equal(await page.locator('#task-read-detail').textContent(),'');
});

test('task reason combines with status and older pages retain the loaded conditions',async t=>{
 const page=await fixture(t),failed={...item,status:'failed',error:'model_request_failed'};
 await page.locator('#tasks-filter').selectOption('failed');await page.locator('#tasks-reason').selectOption('model_request_failed');assert.equal(await page.evaluate(()=>requests.length),0);
 await page.evaluate(value=>replies.push(value),{items:[failed],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);
 let url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.deepEqual(Object.fromEntries(url.searchParams),{botId:'12345678',type:'group',id:'10',state:'failed',reason:'model_request_failed'});assert.match(await page.locator('#tasks-status').textContent(),/已加载筛选：失败.*错误类别：Agent 模型请求/s);
 await page.locator('#tasks-filter').selectOption('completed');await page.locator('#tasks-reason').selectOption('none');await page.evaluate(value=>{agentScope={botId:'87654321',type:'private',id:'20'};replies.push(value)},{items:[failed],more:true,next:'9007199254740991'});
 await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.deepEqual(Object.fromEntries(url.searchParams),{botId:'12345678',type:'group',id:'10',state:'failed',reason:'model_request_failed',before:'9007199254740993'});
 await page.locator('#task-read-id').fill(item.taskId);await page.evaluate(value=>replies.push(value),failed);await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.doesNotMatch(await page.evaluate(()=>requests.at(-1).url),/reason=|state=/);assert.equal(await page.locator('#tasks-next').isEnabled(),true);
 await load(page,[]);url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.deepEqual(Object.fromEntries(url.searchParams),{botId:'87654321',type:'private',id:'20',state:'completed',reason:'none'});assert.match(await page.locator('#tasks-status').textContent(),/错误类别：未记录错误类别/);
});

test('task reason failures clear paging and late responses cannot restore a different scope',async t=>{
 const page=await fixture(t);await page.locator('#tasks-reason').selectOption('other');await page.evaluate(value=>replies.push(value),{items:[{...item,status:'failed',error:'legacy_failure'}],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);
 assert.match(await page.locator('#tasks-list').textContent(),/其他或旧错误类别/);await page.evaluate(()=>replies.push({error:'存储不可用',httpStatus:503}));await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-next').isDisabled(),true);assert.equal(await page.locator('#tasks-list').textContent(),'');assert.match(await page.locator('#tasks-status').textContent(),/读取失败/);
 await page.locator('#tasks-load').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#tasks-reason').isDisabled(),true);
 await page.evaluate(()=>document.getElementById('agent-id').dispatchEvent(new Event('change')));await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),{items:[item]});await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.locator('#tasks-reason').inputValue(),'all');assert.equal(await page.evaluate(()=>tasksReason),'all');
});

test('task reason resets after stopping and unrecorded categories do not imply success',async t=>{
 const page=await fixture(t);await page.locator('#tasks-filter').selectOption('running');await page.locator('#tasks-reason').selectOption('none');await load(page);const count=await page.evaluate(()=>requests.length);
 await page.evaluate(value=>replies.push(value),{...item,status:'cancel_requested'});await page.getByRole('button',{name:'请求停止此任务',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-reason').inputValue(),'all');assert.equal(await page.locator('#tasks-filter').inputValue(),'all');assert.equal(await page.evaluate(()=>tasksReason),'all');assert.equal(await page.evaluate(()=>requests.length),count+1);
 await page.locator('#tasks-reason').selectOption('none');await load(page,[{...item,status:'failed',error:''}]);assert.match(await page.locator('#tasks-list').textContent(),/失败/);assert.match(await page.locator('#tasks-status').textContent(),/未记录错误类别/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/已完成/);
 await page.locator('#tasks-reason').selectOption('other');await load(page,[{...item,status:'failed',error:'<img src=x>'}]);assert.equal(await page.locator('#tasks-list img').count(),0);assert.match(await page.locator('#tasks-list').textContent(),/其他或旧错误类别/);
 await page.evaluate(()=>{authenticated=false;csrf='';showAuth();});assert.equal(await page.locator('#tasks-reason').inputValue(),'all');assert.equal(await page.locator('#tasks-list').textContent(),'');
});

test('task reason controls and long labels fit 320px light and dark themes',async t=>{
 const page=await fixture(t);await page.setViewportSize({width:320,height:800});await page.locator('#tasks-reason').selectOption('model_request_failed');await load(page,[{...item,status:'failed',error:'model_request_failed'}]);await fs.mkdir('.local/agent-tasks-ui',{recursive:true});
 for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.locator('#tasks-title').scrollIntoViewIfNeeded();await page.screenshot({path:'.local/agent-tasks-ui/reason-'+theme+'.png'});}
 await page.evaluate(()=>document.getElementById('agent-id').dispatchEvent(new Event('change')));assert.equal(await page.locator('#tasks-reason').inputValue(),'all');assert.equal(await page.locator('#tasks-reason').isDisabled(),true);
});

test('model output limits show a saved failure without replay controls',async t=>{
 const page=await fixture(t),failed={...item,status:'failed',phase:'finished',error:'model_output_limit'};
 await load(page,[failed]);assert.match(await page.locator('#tasks-list').textContent(),/模型单轮输出超过容量、动作数量或结构限制/);assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).count(),0);
 await page.locator('#task-read-id').fill(item.taskId);await page.evaluate(value=>replies.push(value),failed);await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#task-read-detail').textContent(),/模型单轮输出超过容量、动作数量或结构限制/);assert.equal(await page.locator('#task-read-detail button').count(),0);assert.equal(await page.evaluate(()=>requests.length),2);
});

test('malformed action batches show the same failure in list and exact lookup',async t=>{
 const page=await fixture(t),failed={...item,status:'failed',phase:'finished',error:'model_output_invalid'};
 await load(page,[failed]);assert.match(await page.locator('#tasks-list').textContent(),/模型动作回复格式无效，当前批次未执行/);assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).count(),0);
 await page.locator('#task-read-id').fill(item.taskId);await page.evaluate(value=>replies.push(value),failed);await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#task-read-detail').textContent(),/模型动作回复格式无效，当前批次未执行/);assert.equal(await page.locator('#task-read-detail button').count(),0);assert.equal(await page.evaluate(()=>requests.length),2);
});

test('model request failures show a safe reason in list and exact lookup without retry',async t=>{
 const page=await fixture(t),failed={...item,status:'failed',phase:'finished',error:'model_request_failed'};
 await load(page,[failed]);assert.match(await page.locator('#tasks-list').textContent(),/Agent 模型请求或响应解析失败/);assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).count(),0);
 await page.locator('#task-read-id').fill(item.taskId);await page.evaluate(value=>replies.push(value),failed);await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#task-read-detail').textContent(),/Agent 模型请求或响应解析失败/);assert.equal(await page.locator('#task-read-detail button').count(),0);assert.equal(await page.evaluate(()=>requests.length),2);
});

test('task phase and first stop timestamps are exact, inert and shared by list and lookup',async t=>{
 const page=await fixture(t),timed={...item,status:'cancel_requested',phaseStartedAt:'2026-10-14T00:00:00.123456789Z',cancelRequestedAt:'2026-10-14T00:00:01.987654321Z'};
 await load(page,[timed]);assert.match(await page.locator('#tasks-list').textContent(),/当前阶段开始：2026-10-14T00:00:00\.123456789Z.*首次停止请求：2026-10-14T00:00:01\.987654321Z/s);
 await page.locator('#task-read-id').fill(item.taskId);await page.evaluate(value=>replies.push(value),timed);await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#task-read-detail').textContent(),/首次停止请求：2026-10-14T00:00:01\.987654321Z/);
 assert.equal(await page.evaluate(()=>requests.length),2);assert.equal(await page.locator('#task-read-detail button').count(),0);
 await load(page,[{...timed,status:'interrupted',phase:'finished'}]);assert.match(await page.locator('#tasks-list').textContent(),/发现遗留任务.*实际退出时刻未知/s);
 await load(page,[{...timed,persisted:false}]);assert.match(await page.locator('#tasks-list').textContent(),/阶段与停止请求时间未可靠保存/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/123456789Z|987654321Z/);
 for(const value of [undefined,'',123,'2026-02-30T00:00:00Z','2026-10-14T08:00:00+08:00','<img src=x onerror=alert(1)>']){await load(page,[{...timed,phaseStartedAt:value,cancelRequestedAt:value}]);assert.match(await page.locator('#tasks-list').textContent(),/当前阶段开始时间未记录或无效.*首次停止请求时间未记录或无效/s);assert.equal(await page.locator('#tasks-list img').count(),0);}
});

test('task timestamps wrap in narrow themes and clear on conversation changes',async t=>{
 const page=await fixture(t);await page.setViewportSize({width:320,height:800});await load(page,[{...item,status:'cancelled',phase:'finished',endedAt:'2026-10-14T00:00:02.123456789Z',phaseStartedAt:'2026-10-14T00:00:02.123456789Z',cancelRequestedAt:'2026-10-14T00:00:01.987654321Z'}]);
 await fs.mkdir('.local/agent-tasks-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.getByText(/^当前阶段开始：/).scrollIntoViewIfNeeded();await page.screenshot({path:'.local/agent-tasks-ui/timing-'+theme+'.png'});}
 await page.evaluate(()=>document.getElementById('agent-id').dispatchEvent(new Event('change')));assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.locator('#task-read-detail').textContent(),'');
});

test('task query summaries distinguish quota, coverage, pending, old and uncertain records',async t=>{
 const page=await fixture(t);const queries={available:true,used:3,limit:3,searchHistory:1,readHistory:0,searchMemory:1,readMemory:1,successful:2,empty:1,partial:2,errors:1,denied:1};
 await load(page,[{...item,queries}]);assert.match(await page.locator('#tasks-list').textContent(),/额度已用 3\/3.*历史检索 1.*记忆检索 1.*记忆读取 1.*成功响应 2 次.*空结果 1 次.*覆盖不完整 2 次.*读取错误 1 次.*超额拒绝 1 次/s);
 await page.locator('#task-read-id').fill(item.taskId);await page.evaluate(value=>replies.push(value),{...item,queries});await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#task-read-detail').textContent(),/额度已用 3\/3/);
 await load(page,[{...item,status:'interrupted',queries:{...queries,used:1,searchHistory:1,searchMemory:0,readMemory:0,successful:0,empty:0,partial:0,errors:0,denied:0}}]);assert.match(await page.locator('#tasks-list').textContent(),/1 次查询结果尚未确认/);
 await load(page,[{...item,queries:{...queries,used:0,searchHistory:0,searchMemory:0,readMemory:0,successful:0,empty:0,partial:0,errors:0,denied:0}}]);assert.match(await page.locator('#tasks-list').textContent(),/额度已用 0\/3/);
 for(const q of [undefined,{...queries,available:false},{...queries,used:'3'},{...queries,used:4},{...queries,empty:3},{...queries,errors:-1}]){await load(page,[{...item,queries:q}]);assert.match(await page.locator('#tasks-list').textContent(),/没有可核对的查询摘要/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/额度已用/);}
 await load(page,[{...item,queries,persisted:false}]);assert.match(await page.locator('#tasks-list').textContent(),/任务查询摘要未可靠保存/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/额度已用/);
});

test('query summaries fit 320px themes and clear with the selected conversation',async t=>{
 const page=await fixture(t);await page.setViewportSize({width:320,height:800});await load(page,[{...item,queries:{available:true,used:3,limit:3,searchHistory:1,readHistory:0,searchMemory:1,readMemory:1,successful:2,empty:1,partial:2,errors:1,denied:1}}]);
 await fs.mkdir('.local/agent-tasks-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);if(!await page.locator('#tasks-list details').evaluate(el=>el.open))await page.locator('#tasks-list summary').click();await page.getByText(/^本任务历史与记忆查询：/).scrollIntoViewIfNeeded();await page.screenshot({path:'.local/agent-tasks-ui/queries-'+theme+'.png'});}
 await page.evaluate(()=>document.getElementById('agent-id').dispatchEvent(new Event('change')));assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.locator('#task-read-detail').textContent(),'');
});

test('task settled results distinguish zero, pending, legacy and uncertain data in lists and exact reads',async t=>{
 const page=await fixture(t);const results={available:true,ok:2,rejected:1,unknown:3,localErrors:4};
 await load(page,[{...item,results,outcome:'in_flight',phase:'action_wait'}]);assert.match(await page.locator('#tasks-list').textContent(),/成功回执 2 次.*平台拒绝 1 次.*结果未知 3 次.*本地失败 4 次.*仍在途.*不证明 QQ/s);
 await page.locator('#task-read-id').fill(item.taskId);await page.evaluate(value=>replies.push(value),{...item,results,status:'completed'});await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#task-read-detail').textContent(),/成功回执 2 次.*结果未知 3 次/s);assert.doesNotMatch(await page.locator('#task-read-detail').textContent(),/仍在途/);
 await load(page,[{...item,results:{available:true,ok:0,rejected:0,unknown:0,localErrors:0}}]);assert.match(await page.locator('#tasks-list').textContent(),/成功回执 0 次.*本地失败 0 次/s);
 for(const value of [undefined,{...results,available:false},{...results,ok:-1},{...results,ok:'2'}]){await load(page,[{...item,results:value}]);assert.match(await page.locator('#tasks-list').textContent(),/没有可核对的累计动作结果/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/成功回执 \d 次/);}
 await load(page,[{...item,results,persisted:false}]);assert.match(await page.locator('#tasks-list').textContent(),/累计动作结果未可靠保存/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/成功回执 \d 次/);
});

test('settled result summaries clear on scope changes and fit both themes at 320px',async t=>{
 const page=await fixture(t);await page.setViewportSize({width:320,height:800});const value={...item,status:'interrupted',phase:'finished',outcome:'unknown',results:{available:true,ok:1,rejected:0,unknown:1,localErrors:0}};await load(page,[value]);
 await fs.mkdir('.local/agent-tasks-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);if(!await page.locator('#tasks-list details').evaluate(el=>el.open))await page.locator('#tasks-list summary').click();await page.getByText(/^本任务累计动作结果：/).scrollIntoViewIfNeeded();await page.screenshot({path:'.local/agent-tasks-ui/results-'+theme+'.png'});}
 await page.evaluate(()=>document.getElementById('agent-id').dispatchEvent(new Event('change')));assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.locator('#task-read-detail').textContent(),'');
});

test('task trigger source binds the loaded scope and large sequence and preserves navigation',async t=>{
 const page=await fixture(t);const source={...item,sourceSeq:'9007199254740993'};await page.evaluate(value=>replies.push(value),{items:[source],more:true,next:'50'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);
 await page.evaluate(value=>{document.getElementById('agent-id').value='999';replies.push(value)},{available:true,entries:[{seq:source.sourceSeq,kind:'event',time:'fixture',text:'触发原文 <img src=x> synthetic',truncated:true}]});
 await page.locator('#tasks-list').getByRole('button',{name:'查看触发消息',exact:true}).click();await page.waitForFunction(()=>!agentBusy);const request=await page.evaluate(()=>requests.at(-1));const url=new URL(request.url,'http://fixture');
 assert.equal(request.body,null);assert.equal(url.pathname,'/api/agent/history');assert.equal(url.searchParams.get('botId'),'12345678');assert.equal(url.searchParams.get('id'),'10');assert.equal(url.searchParams.get('type'),'group');assert.equal(url.searchParams.get('seq'),'9007199254740993');assert.equal(url.searchParams.get('archived'),'true');assert.equal(await page.evaluate(()=>tasksNext),'50');assert.equal(await page.locator('#tasks-next').isDisabled(),false);
 assert.match(await page.locator('#task-source').textContent(),/触发消息档案 #9007199254740993.*包含重置前.*触发原文.*截断/s);assert.equal(await page.locator('#task-source img').count(),0);
 await page.setViewportSize({width:320,height:800});await page.locator('#task-source').scrollIntoViewIfNeeded();await fs.mkdir('.local/agent-tasks-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/agent-tasks-ui/source-'+theme+'.png'});}
 await load(page,[source]);assert.equal(await page.locator('#task-source').textContent(),'');
});

test('task source distinguishes unknown unavailable non-events and failures in list and lookup',async t=>{
 const page=await fixture(t);await load(page);assert.match(await page.locator('#tasks-list').textContent(),/触发来源未知/);assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'查看触发消息',exact:true}).count(),0);
 const source={...item,sourceSeq:'23'};await load(page,[source]);
 for(const value of [{available:true,entries:[]},{available:false,entries:[]},{available:true,entries:[{seq:'23',kind:'request',text:'NOT_A_TRIGGER'}]},{error:'读取失败',httpStatus:503}]){
  await page.evaluate(value=>replies.push(value),value);await page.locator('#tasks-list').getByRole('button',{name:'查看触发消息',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#task-source').textContent(),/未找到|档案文件不可用|读取失败/);assert.doesNotMatch(await page.locator('#task-source').textContent(),/NOT_A_TRIGGER/);
 }
 await page.locator('#task-read-id').fill(item.taskId);await page.evaluate(value=>replies.push(value),{...source,status:'completed'});await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#task-source').textContent(),'');
 assert.equal(await page.locator('#task-read-detail').getByRole('button',{name:'请求停止此任务'}).count(),0);await page.evaluate(()=>replies.push({available:true,entries:[{seq:'23',kind:'event',time:'fixture',text:'LOOKUP_TRIGGER'}]}));await page.locator('#task-read-detail').getByRole('button',{name:'查看触发消息',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#task-source').textContent(),/LOOKUP_TRIGGER/);
});

test('late trigger sources cannot refill a different login or conversation',async t=>{
 const page=await fixture(t);await load(page,[{...item,sourceSeq:'23'}]);await page.locator('#tasks-list').getByRole('button',{name:'查看触发消息',exact:true}).click();await page.waitForFunction(()=>pending.length===1);
 await page.evaluate(()=>{authenticated=false;showAuth();authenticated=true;csrf='new-source-session';agentVisible=true;agentScope={botId:'87654321',type:'private',id:'20'};document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;agentControls();});await load(page,[{...item,taskId:'c'.repeat(32),status:'completed'}]);
 await page.evaluate(()=>pending[0]({ok:true,status:200,json:async()=>({available:true,entries:[{seq:'23',kind:'event',text:'OLD_TASK_TRIGGER'}]})}));await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#task-source').textContent(),'');assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/OLD_TASK_TRIGGER/);
 await page.evaluate(()=>document.getElementById('agent-id').dispatchEvent(new Event('change')));assert.equal(await page.locator('#task-source').textContent(),'');
});

test('single task lookup preserves list paging and scope and renders only read-only latest data',async t=>{
 const page=await fixture(t);assert.equal(await page.locator('#task-read').isDisabled(),true);
 await page.evaluate(value=>replies.push(value),{items:[item],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);
 const original=await page.locator('#tasks-list').textContent();const id='c'.repeat(32);await page.locator('#task-read-id').fill(id);
 await page.evaluate(value=>{document.getElementById('agent-id').value='999';document.getElementById('tasks-filter').value='failed';replies.push(value)},{...item,taskId:id,version:'9007199254740994',status:'completed',phase:'finished',activity:{available:true,modelPreparations:2,fallbackPreparations:0,localActions:1,platformActions:1}});
 await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);const request=await page.evaluate(()=>requests.at(-1));const url=new URL(request.url,'http://fixture');
 assert.equal(request.body,null);assert.equal(url.pathname,'/api/agent/tasks/read');assert.equal(url.searchParams.get('botId'),'12345678');assert.equal(url.searchParams.get('type'),'group');assert.equal(url.searchParams.get('id'),'10');assert.equal(url.searchParams.get('taskId'),id);assert.equal(url.searchParams.has('state'),false);assert.equal(url.searchParams.has('before'),false);
 assert.equal(await page.locator('#tasks-list').textContent(),original);assert.equal(await page.locator('#tasks-next').isDisabled(),false);assert.equal(await page.evaluate(()=>tasksNext),'9007199254740993');assert.match(await page.locator('#task-read-detail').textContent(),/已完成.*9007199254740994.*模型准备 2 次/s);assert.equal(await page.locator('#task-read-detail button,img').count(),0);
 await page.evaluate(value=>replies.push(value),{...item,taskId:id,status:'running'});await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#task-read-detail button').count(),0);
 await load(page);assert.equal(await page.locator('#task-read-detail').textContent(),'');assert.equal(await page.locator('#task-read-id').inputValue(),id);
});

test('single task lookup rejects invalid IDs and separates missing failures and unverified responses',async t=>{
 const page=await fixture(t);await load(page);
 for(const bad of ['not-a-task','A'.repeat(32),'g'.repeat(32),'a'.repeat(31)]){await page.locator('#task-read-id').fill(bad);await page.locator('#task-read').click();assert.equal(await page.evaluate(()=>requests.length),1);assert.match(await page.locator('#agent-error').textContent(),/任务编号/);}
 await page.locator('#task-read-id').fill(item.taskId);
 for(const value of [{error:'任务不存在',httpStatus:404},{error:'状态读取失败',httpStatus:503},{...item,taskId:'c'.repeat(32)},{...item,version:123}]){
  await page.evaluate(value=>replies.push(value),value);await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#task-read-detail').textContent(),'');assert.match(await page.locator('#task-read-status').textContent(),/核对失败.*不能据此确认任务已结束/);assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).count(),1);
 }
 assert.equal(await page.evaluate(()=>requests.length),5);
});

test('single task lookup clears late sessions and wraps 320px summaries in both themes',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#task-read-id').fill(item.taskId);await page.locator('#task-read').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#task-read-id').isDisabled(),true);
 await page.evaluate(()=>{authenticated=false;showAuth();});assert.equal(await page.locator('#task-read-id').inputValue(),'');
 await page.evaluate(()=>{authenticated=true;csrf='task-lookup-new';agentVisible=true;agentScope={botId:'87654321',type:'private',id:'20'};document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;agentControls();});await load(page,[{...item,taskId:'d'.repeat(32),status:'interrupted'}]);
 await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),item);await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#task-read-detail').textContent(),'');assert.equal(await page.locator('#task-read-id').inputValue(),'');
 await page.locator('#task-read-id').fill('d'.repeat(32));await page.evaluate(value=>replies.push(value),{...item,taskId:'d'.repeat(32),status:'failed',persisted:false,error:'storage_error',outcome:'unknown'});await page.locator('#task-read').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#task-read-detail').textContent(),/未可靠保存/);assert.equal(await page.locator('#task-read-detail button').count(),0);
 await page.setViewportSize({width:320,height:800});await page.locator('#task-read-form').scrollIntoViewIfNeeded();await fs.mkdir('.local/agent-tasks-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/agent-tasks-ui/read-'+theme+'.png'});}
 await page.evaluate(()=>clearAgentContent());assert.equal(await page.locator('#task-read-detail').textContent(),'');assert.equal(await page.locator('#task-read-id').inputValue(),'');
});

test('task paging binds loaded status, rereading applies draft and cancel returns all',async t=>{
 const page=await fixture(t);await page.locator('#tasks-filter').selectOption('failed');await page.evaluate(value=>replies.push(value),{items:[{...item,status:'failed'}],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#tasks-status').textContent(),/已加载筛选：失败/);
 await page.locator('#tasks-filter').selectOption('running');await page.evaluate(value=>replies.push(value),{items:[{...item,status:'failed'}],more:false,next:''});await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);const url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.equal(url.searchParams.get('state'),'failed');assert.equal(url.searchParams.get('before'),'9007199254740993');
 await load(page);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.get('state'),'running');assert.match(await page.locator('#tasks-status').textContent(),/筛选：运行中/);
 await page.evaluate(value=>replies.push(value),{...item,status:'cancel_requested'});await page.getByRole('button',{name:'请求停止此任务'}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-filter').inputValue(),'all');assert.match(await page.locator('#tasks-status').textContent(),/筛选：全部/);
 await page.locator('#tasks-filter').selectOption('interrupted');await page.evaluate(()=>document.getElementById('agent-id').dispatchEvent(new Event('change')));assert.equal(await page.locator('#tasks-filter').inputValue(),'all');assert.equal(await page.locator('#tasks-list').textContent(),'');
});

test('context memory response counts distinguish measured zero from legacy and reset',async t=>{
 const page=await fixture(t);await load(page,[{...item,context:{...item.context,memorySources:3}}]);assert.match(await page.locator('#tasks-list').textContent(),/含 2 个检索来源片段.*另含 3 个记忆检索\/读取响应片段.*可能重复/s);
 await page.setViewportSize({width:320,height:800});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);}
 await load(page,[{...item,context:{...item.context,memorySources:0}}]);assert.match(await page.locator('#tasks-list').textContent(),/另含 0 个记忆/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/旧摘要未统计/);
 await load(page);assert.match(await page.locator('#tasks-list').textContent(),/旧摘要未统计记忆查询片段/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/另含 0 个记忆/);
 await load(page,[{...item,context:{available:false}}]);assert.match(await page.locator('#tasks-list').textContent(),/尚无上下文取用摘要/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/旧摘要未统计/);
 await page.evaluate(()=>document.getElementById('agent-id').dispatchEvent(new Event('change')));assert.equal(await page.locator('#tasks-list').textContent(),'');
});

test('task activity distinguishes stage registrations, legacy and uncertain records',async t=>{
 const page=await fixture(t);const activity={available:true,modelPreparations:2,fallbackPreparations:1,localActions:3,platformActions:1};
 await load(page,[{...item,activity}]);assert.match(await page.locator('#tasks-list').textContent(),/模型准备 2 次.*兜底准备 1 次.*本地动作 3 次.*平台动作 1 次.*不代表执行成功/s);
 await page.setViewportSize({width:320,height:800});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);}
 await load(page);assert.match(await page.locator('#tasks-list').textContent(),/没有累计执行摘要/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/本任务累计登记/);
 await load(page,[{...item,activity,persisted:false}]);assert.match(await page.locator('#tasks-list').textContent(),/累计执行摘要未可靠保存/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/本任务累计登记/);
 await load(page,[{...item,activity:{...activity,modelPreparations:0,fallbackPreparations:0,localActions:0,platformActions:0}}]);assert.match(await page.locator('#tasks-list').textContent(),/模型准备 0 次/);
});

test('memory usage distinguishes limits, legacy files, errors and absent measurements',async t=>{
 const page=await fixture(t);
 const memory={available:true,mode:'managed',records:100,bytes:2500,budgetBytes:30000,limit:100,clipped:true,reason:'record_limit'};
 await load(page,[{...item,memory}]);assert.match(await page.locator('#tasks-list').textContent(),/100 条启用记忆，2500 字节.*达到条数限制.*不是 token/s);
 await page.setViewportSize({width:320,height:800});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);}
 await load(page,[{...item,memory:{...memory,records:7,reason:'byte_limit'}}]);assert.match(await page.locator('#tasks-list').textContent(),/达到字节限制/);
 await load(page,[{...item,memory:{...memory,records:0,bytes:0,clipped:false,reason:''}}]);assert.match(await page.locator('#tasks-list').textContent(),/0 条启用记忆，0 字节.*均已准备/s);
 await load(page,[{...item,memory:{available:true,mode:'legacy'}}]);assert.match(await page.locator('#tasks-list').textContent(),/旧文件.*尚未统计/s);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/0 条启用记忆/);
 await load(page,[{...item,memory:{available:true,mode:'claimed_elsewhere'}}]);assert.match(await page.locator('#tasks-list').textContent(),/已归属其他机器人/);
 await load(page,[{...item,memory:{available:false,reason:'read_error'}}]);assert.match(await page.locator('#tasks-list').textContent(),/读取失败.*不能按无记忆/s);
 await load(page);assert.match(await page.locator('#tasks-list').textContent(),/尚无长期记忆取用摘要/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),/读取失败/);
});
test('older tasks preserve loaded scope and string cursor, refresh resets the page',async t=>{
 const page=await fixture(t);await page.evaluate(value=>replies.push(value),{items:[item],more:true,next:'9007199254740993'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-next').isDisabled(),false);
 await page.evaluate(value=>{document.getElementById('agent-id').value='999';replies.push(value)},{items:[{...item,taskId:'c'.repeat(32),status:'completed'}],more:false,next:''});await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);
 const url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.equal(url.searchParams.get('before'),'9007199254740993');assert.equal(url.searchParams.get('id'),'10');assert.equal(await page.locator('#tasks-next').isDisabled(),true);assert.match(await page.locator('#tasks-status').textContent(),/较早任务/);
 await load(page);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.has('before'),false);
});
test('paging failure and scope reset remove the cursor and stale stop controls',async t=>{
 const page=await fixture(t);await page.evaluate(value=>replies.push(value),{items:[item],more:true,next:'50'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);
 await page.evaluate(()=>replies.push({error:'读取失败',httpStatus:503}));await page.locator('#tasks-next').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#tasks-next').isDisabled(),true);assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).count(),0);
 await page.evaluate(value=>replies.push(value),{items:[item],more:true,next:'50'});await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);await page.evaluate(()=>document.getElementById('agent-id').dispatchEvent(new Event('change')));assert.equal(await page.evaluate(()=>tasksNext),'');assert.equal(await page.locator('#tasks-next').isDisabled(),true);
});
test('task display is inert, bounded and tied to loaded scope and exact string version',async t=>{
 const page=await fixture(t);await load(page);assert.equal(await page.locator('#tasks-list img').count(),0);assert.match(await page.locator('#tasks-list').textContent(),/3 条历史记录，含 2 个检索/);
 await page.evaluate(value=>{document.getElementById('agent-id').value='99';replies.push(value)},{...item,status:'cancel_requested',version:'9007199254740994'});
 await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',taskId:item.taskId,version:item.version});assert.match(await page.locator('#tasks-list').textContent(),/等待在途请求返回/);assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).count(),0);
});
test('cancel conflict or uncertain response clears stale actions and never retries',async t=>{
 const page=await fixture(t);await load(page);await page.evaluate(()=>replies.push({error:'任务版本已变化',httpStatus:409}));await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).count(),0);assert.match(await page.locator('#tasks-status').textContent(),/结果未知.*重新读取/);assert.equal(await page.evaluate(()=>requests.length),2);
 await load(page,[{...item,status:'completed'}]);assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).count(),0);
 await page.evaluate(()=>replies.push({error:'存储不可用',httpStatus:503}));await page.locator('#tasks-load').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#tasks-status').textContent(),/读取失败/);assert.equal(await page.locator('#tasks-list').textContent(),'');
});
test('cancel pending blocks navigation; expired login late response cannot refill task state',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).click();await page.waitForFunction(()=>pending.length===1);
 await page.evaluate(()=>document.querySelector('[data-view=overview]').dispatchEvent(new Event('click')));assert.equal(await page.locator('#agent-view').isVisible(),true);
 await page.evaluate(()=>{authenticated=false;showAuth();authenticated=true;csrf='new-task-session';agentVisible=true;agentScope={botId:'87654321',type:'private',id:'10'};document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;agentControls();});
 await load(page,[{...item,taskId:'c'.repeat(32),status:'interrupted',outcome:'unknown'}]);
 await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),{...item,status:'cancelled'});await page.evaluate(()=>Promise.resolve());assert.match(await page.locator('#tasks-list').textContent(),/进程中断/);assert.doesNotMatch(await page.locator('#tasks-list').textContent(),new RegExp(item.taskId));
});
test('scope change and direct scope submit clear old task buttons',async t=>{
 const page=await fixture(t);await load(page);await page.evaluate(()=>{document.getElementById('agent-id').dispatchEvent(new Event('change'));});assert.equal(await page.locator('#tasks-list').textContent(),'');
 await page.evaluate(()=>{agentScope={botId:'12345678',type:'group',id:'10'};agentControls();});await load(page);
 await page.evaluate(()=>{const o=document.createElement('option');o.value='12345678';document.getElementById('agent-bot').append(o);document.getElementById('agent-id').value='20';replies.push({available:true,entries:[],nextCursor:''});document.getElementById('agent-scope-form').dispatchEvent(new Event('submit',{cancelable:true}));});await page.waitForFunction(()=>!agentBusy&&agentScope.id==='20');assert.equal(await page.locator('#tasks-list').textContent(),'');assert.equal(await page.evaluate(()=>tasksScope),null);
});
test('unsaved terminal state has no cancel button and 320px themes wrap identifiers',async t=>{
 const page=await fixture(t);await page.setViewportSize({width:320,height:800});await load(page,[{...item,status:'failed',persisted:false,error:'storage_error',outcome:'unknown'}]);assert.match(await page.locator('#tasks-list').textContent(),/未可靠保存/);assert.equal(await page.locator('#tasks-list').getByRole('button',{name:'请求停止此任务',exact:true}).count(),0);
 await page.locator('#tasks-title').scrollIntoViewIfNeeded();await fs.mkdir('.local/agent-tasks-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.locator('#tasks-list').screenshot({path:'.local/agent-tasks-ui/'+theme+'.png'});}
});
