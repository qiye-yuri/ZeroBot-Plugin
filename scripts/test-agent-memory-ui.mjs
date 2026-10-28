import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
import test from 'node:test';
const require=createRequire(import.meta.url),modules=process.env.LOCALADMIN_NODE_MODULES||path.join(process.env.USERPROFILE,'.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules');
const {chromium}=require(path.join(modules,'playwright'));
const root='internal/localadmin/assets/';
const html=(await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,'');
const scripts=await Promise.all(['app.js','agent-archive.js','agent-memory.js'].map(p=>fs.readFile(root+p,'utf8')));
const css=await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8');
const item={id:'9007199254740993',text:'合成记忆 <img src=x>',source:'agent',sourceSeq:'23',active:true,revision:'2',updatedAt:'2026-10-13T00:00Z'};
const managed={mode:'managed',items:[item],more:false,next:item.id};
async function fixture(t){
 const browser=await chromium.launch({channel:'chrome',headless:true});t.after(()=>browser.close());const context=await browser.newContext();await context.route('**/*',r=>r.abort());const page=await context.newPage();await page.setContent(html);await page.addStyleTag({content:css});for(const content of scripts)await page.addScriptTag({content});
 await page.evaluate(()=>{
  authenticated=true;csrf='memory-fixture';agentVisible=true;agentScope={botId:'12345678',type:'group',id:'10',archived:false};window.showOverview=()=>{};
  document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.getElementById('overview-view').hidden=true;document.getElementById('agent-view').hidden=false;
  document.querySelector('[data-view=overview]').classList.remove('active');document.querySelector('[data-view=agent]').classList.add('active');
  window.requests=[];window.replies=[];window.pending=[];window.fetch=(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const value=replies.shift();if(value)return Promise.resolve({ok:!value.error,status:value.status||200,json:async()=>value});return new Promise(resolve=>pending.push(resolve));};agentControls();
 });return page;
}
async function load(page,value=managed){await page.evaluate(value=>replies.push(value),value);await page.locator('#memory-load').click();await page.waitForFunction(()=>!agentBusy);}

test('layered memory labels show scope and personal attributes without markup',async t=>{
 const page=await fixture(t);await page.setViewportSize({width:320,height:800});
 await load(page,{...managed,items:[{...item,kind:'personal',subject:'12345678',attribute:'preference.tea'},{...item,id:'2',kind:'permanent',attribute:'style'},{...item,id:'3'}]});
 const text=await page.locator('#memory-list').textContent();assert.match(text,/个人记忆（仅本会话）/);assert.match(text,/QQ 12345678/);assert.match(text,/preference.tea/);assert.match(text,/永久知识（本机器人跨会话共享）/);assert.match(text,/长期记忆（仅本会话）/);
 assert.equal(await page.locator('#memory-list img').count(),0);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);
 await fs.mkdir('.local/layered-memory-ui',{recursive:true});await page.locator('#memory-list').scrollIntoViewIfNeeded();await page.screenshot({path:'.local/layered-memory-ui/320.png'});
});

test('historical revision comparison reads two exact revisions without changing current memory or pagination',async t=>{
 const page=await fixture(t),current={...item,revision:'9007199254740995'},base={...item,revision:'9007199254740993',text:'共同🙂旧<script>x</script>结尾'},other={...item,revision:'9007199254740994',text:'共同🙂新<img src=x>结尾',active:false};await load(page,{...managed,items:[current],more:true,next:item.id});const original=await page.locator('#memory-list').textContent();
 await page.evaluate(value=>replies.push(value),{...managed,items:[other,base],more:true,next:base.revision});await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('.memory-compare-to').inputValue(),other.revision);await page.locator('.memory-compare-from').fill(base.revision);await page.evaluate(()=>{agentScope={botId:'87654321',type:'private',id:'20'}});const count=await page.evaluate(()=>requests.length);
 await page.evaluate(values=>replies.push(...values),[{...managed,items:[base]},{...managed,items:[other]}]);await page.locator('.memory-compare-to').press('Enter');await page.waitForFunction(()=>!agentBusy);const requests=await page.evaluate(()=>window.requests.slice(-2));for(let index=0;index<2;index++){const url=new URL(requests[index].url,'http://fixture');assert.equal(url.pathname,'/api/agent/memories/revision');assert.deepEqual(Object.fromEntries(url.searchParams),{botId:'12345678',type:'group',id:'10',memoryId:item.id,revision:[base.revision,other.revision][index]});assert.equal(requests[index].body,null);}assert.equal(await page.evaluate(()=>window.requests.length),count+2);
 const comparison=page.locator('.memory-history-comparison');assert.match(await comparison.textContent(),/历史 v9007199254740993 与历史 v9007199254740994/);assert.match(await comparison.textContent(),/基准修订启用.*目标修订停用/s);assert.equal(await comparison.locator('pre').nth(0).textContent(),base.text);assert.equal(await comparison.locator('pre').nth(1).textContent(),other.text);assert.equal(await comparison.locator('img,script,button').count(),0);assert.equal(await comparison.locator('del').textContent(),'旧<script>x</script');assert.equal(await comparison.locator('ins').textContent(),'新<img src=x');assert.equal(await page.locator('#memory-list').textContent(),original);assert.equal(await page.locator('#memory-next').isEnabled(),true);assert.equal(await page.getByRole('button',{name:'下一页版本',exact:true}).isEnabled(),true);assert.equal(await page.evaluate(()=>memoryItems[0].revision),current.revision);assert.equal(await page.evaluate(()=>memoryCurrent),null);
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/memory-history-compare-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.locator('.memory-history-compare').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/memory-history-compare-ui/'+theme+'.png'});}
 await page.evaluate(value=>replies.push(value),{...managed,items:[base],more:false});await page.getByRole('button',{name:'下一页版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(await comparison.textContent(),'');assert.equal(await page.locator('.memory-compare-from').inputValue(),'');assert.equal(await page.locator('.memory-compare-to').inputValue(),base.revision);
});

test('historical revision comparison validates inputs and separates missing mismatched failed and equal revisions',async t=>{
 const page=await fixture(t);await load(page);await page.evaluate(value=>replies.push(value),managed);await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);const compare=page.getByRole('button',{name:'对比两次修订',exact:true}),base={...item,revision:'1'},other={...item,revision:'2',active:false};const count=await page.evaluate(()=>requests.length);
 for(const revision of ['0','01','9223372036854775808']){await page.locator('.memory-compare-from').fill(revision);await compare.click();assert.equal(await page.evaluate(()=>requests.length),count);assert.match(await page.locator('#agent-error').textContent(),/对比修订号须为有效/);}await page.locator('.memory-compare-from').fill('1');await page.locator('.memory-compare-to').fill('2');
 for(const values of [[{...managed,items:[]}],[{...managed,items:[base]},{...managed,items:[]}],[{...managed,items:[base]},{...managed,items:[{...other,id:'1'}]}],[{...managed,items:[base]},{error:'读取失败',status:503}]]){const before=await page.evaluate(()=>requests.length);await page.evaluate(values=>replies.push(...values),values);await compare.click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('.memory-history-comparison pre').count(),0);assert.match(await page.locator('.memory-history-comparison').textContent(),/未找到历史修订|修订对比未完成/);assert.equal(await page.evaluate(()=>requests.length),before+values.length);assert.equal(await page.locator('.memory-compare-from').inputValue(),'1');assert.equal(await page.getByRole('button',{name:'定位历史修订',exact:true}).isEnabled(),true);}
 await page.evaluate(values=>replies.push(...values),[{...managed,items:[base]},{...managed,items:[other]}]);await compare.click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('.memory-history-comparison').textContent(),/脱敏后的正文相同/);assert.match(await page.locator('.memory-history-comparison').textContent(),/基准修订启用.*目标修订停用/s);assert.equal(await page.locator('.memory-history-comparison del,.memory-history-comparison ins').count(),0);
 await page.locator('.memory-compare-to').fill('1');await page.evaluate(values=>replies.push(...values),[{...managed,items:[base]},{...managed,items:[base]}]);await compare.click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('.memory-history-comparison').textContent(),/历史 v1 与历史 v1/);assert.equal(await page.locator('.memory-history-comparison del,.memory-history-comparison ins').count(),0);
});

test('historical revision comparison stops between reads when leaving and clears late second replies',async t=>{
 for(const delivered of [0,1]){const page=await fixture(t);await load(page);await page.evaluate(value=>replies.push(value),managed);await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);await page.locator('.memory-compare-from').fill('1');await page.locator('.memory-compare-to').fill('2');const count=await page.evaluate(()=>requests.length);if(delivered)await page.evaluate(value=>replies.push(value),{...managed,items:[{...item,revision:'1'}]});await page.getByRole('button',{name:'对比两次修订',exact:true}).click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('.memory-compare-from').isDisabled(),true);assert.equal(await page.locator('.memory-compare-to').isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),{...managed,items:[{...item,revision:String(delivered+1)}]});await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#memory-versions').textContent(),'');assert.equal(await page.evaluate(()=>requests.length),count+delivered+1);assert.equal(await page.evaluate(()=>memoryScope),null);}
});

test('exact memory revision lookup preserves scope and list and reuses versioned compare and restore',async t=>{
 const page=await fixture(t),latest={...item,revision:'9007199254740995',active:false,text:'当前完整正文'},old={...item,revision:'9007199254740993',text:'历史正文 <img src=x>',active:true};await load(page,{...managed,items:[latest],more:true});const original=await page.locator('#memory-list').textContent();
 await page.evaluate(value=>replies.push(value),{mode:'managed',items:[latest],more:true,next:'9007199254740994'});await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);await page.locator('.memory-revision-input').fill(old.revision);
 await page.evaluate(value=>{agentScope={botId:'87654321',type:'private',id:'99'};replies.push(value)},{mode:'managed',items:[old],more:false,next:old.revision});await page.getByRole('button',{name:'定位历史修订',exact:true}).click();await page.waitForFunction(()=>!agentBusy);const request=await page.evaluate(()=>requests.at(-1)),url=new URL(request.url,'http://fixture');assert.equal(url.pathname,'/api/agent/memories/revision');assert.deepEqual(Object.fromEntries(url.searchParams),{botId:'12345678',type:'group',id:'10',memoryId:item.id,revision:old.revision});assert.equal(request.body,null);assert.equal(await page.locator('#memory-list').textContent(),original);assert.equal(await page.locator('#memory-next').isEnabled(),true);assert.equal(await page.locator('#memory-versions img').count(),0);assert.equal(await page.getByRole('button',{name:'下一页版本',exact:true}).count(),0);assert.match(await page.locator('#memory-versions').textContent(),/定位历史修订 v9007199254740993.*当前记忆未修改/s);
 await page.getByRole('button',{name:'对比当前版本',exact:true}).click();assert.match(await page.locator('.memory-comparison').textContent(),/当前 v9007199254740995/);await page.getByRole('button',{name:'恢复此版本正文',exact:true}).click();assert.match(await page.locator('#memory-dialog-title').textContent(),/当前 v9007199254740995.*恢复 v9007199254740993/);await page.locator('#memory-cancel').click();
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/memory-revision-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.locator('.memory-revision-input').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/memory-revision-ui/'+theme+'.png'});}
 await page.evaluate(value=>replies.push(value),{mode:'managed',items:[latest],more:true,next:'9007199254740994'});await page.getByRole('button',{name:'返回最新版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').pathname,'/api/agent/memories');assert.equal(await page.locator('.memory-revision-input').inputValue(),'');assert.equal(await page.getByRole('button',{name:'下一页版本',exact:true}).count(),1);
 await page.locator('.memory-revision-input').fill(old.revision);await page.evaluate(value=>replies.push(value),{mode:'managed',items:[old],more:false});await page.getByRole('button',{name:'定位历史修订',exact:true}).click();await page.waitForFunction(()=>!agentBusy);await page.getByRole('button',{name:'恢复此版本正文',exact:true}).click();await page.evaluate(()=>replies.push({saved:true}));await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',memoryId:item.id,revision:latest.revision,action:'revert',text:'',targetRevision:old.revision});
});

test('exact memory revision lookup distinguishes invalid missing failed and late reads',async t=>{
 const page=await fixture(t);await load(page);await page.evaluate(value=>replies.push(value),{mode:'managed',items:[item],more:false});await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);const original=await page.locator('#memory-versions').textContent();const before=await page.evaluate(()=>requests.length);
 for(const value of ['0','01','-1','9223372036854775808']){await page.locator('.memory-revision-input').fill(value);await page.getByRole('button',{name:'定位历史修订',exact:true}).click();assert.equal(await page.evaluate(()=>requests.length),before);assert.equal(await page.locator('#memory-versions').textContent(),original);}
 await page.locator('.memory-revision-input').fill('1');await page.evaluate(()=>replies.push({mode:'managed',items:[],more:false}));await page.getByRole('button',{name:'定位历史修订',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-versions').textContent(),/未找到该修订/);assert.equal(await page.getByRole('button',{name:'恢复此版本正文',exact:true}).count(),0);
 await page.evaluate(value=>replies.push(value),{mode:'managed',items:[{...item,revision:'2'}],more:false});await page.getByRole('button',{name:'定位历史修订',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-versions').textContent(),/版本读取失败/);assert.match(await page.locator('#agent-error').textContent(),/回执与请求不符/);assert.equal(await page.locator('#memory-versions button').count(),0);
 await page.evaluate(value=>replies.push(value),{mode:'managed',items:[item],more:false});await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);await page.locator('.memory-revision-input').fill('1');await page.evaluate(()=>replies.push({error:'读取失败',status:503}));await page.getByRole('button',{name:'定位历史修订',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-versions').textContent(),/版本读取失败/);
 await page.evaluate(value=>replies.push(value),{mode:'managed',items:[item],more:false});await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);await page.locator('.memory-revision-input').fill('1');await page.getByRole('button',{name:'定位历史修订',exact:true}).click();await page.waitForFunction(()=>pending.length===1);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),{mode:'managed',items:[{...item,revision:'1'}],more:false});await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#memory-versions').textContent(),'');assert.equal(await page.locator('.memory-revision-input').count(),0);
});

test('memory revision time alone and combined filters retain applied bounds across pages',async t=>{
 const page=await fixture(t);await load(page);const list=await page.locator('#memory-list').textContent();const from='2026-10-16T08:00',until='2026-10-17T08:00';
 await page.locator('#memory-search-from').fill(from);await page.locator('#memory-search-until').fill(until);
 const bounds=await page.evaluate(([from,until])=>({from:new Date(from).toISOString(),until:new Date(until).toISOString()}),[from,until]);
 await page.evaluate(value=>replies.push(value),{available:true,mode:'managed',memories:[{...item,truncated:false}],scanned:1,searchMore:true,nextBefore:'9007199254740993',...bounds});await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',keyword:'',...bounds});assert.match(await page.locator('#memory-search-state').textContent(),/修订时间：从 .*（含）.*（不含）.*包括编辑、启停及恢复/s);assert.equal(await page.locator('#memory-list').textContent(),list);
 await page.locator('#memory-keyword').fill('新草稿');await page.locator('#memory-search-source').selectOption('legacy');await page.locator('#memory-search-state-filter').selectOption('disabled');await page.locator('#memory-search-from').fill('2027-01-01T00:00');
 await page.evaluate(value=>replies.push(value),{available:true,mode:'managed',memories:[{...item,truncated:false}],scanned:1});await page.locator('#memory-search-continue').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',keyword:'',before:'9007199254740993',...bounds});assert.equal(await page.locator('#memory-search-from').inputValue(),from);
 await page.locator('#memory-keyword').fill('另一草稿');await page.locator('#memory-search-until').fill('2028-01-01T00:00');await page.evaluate(()=>replies.push({available:true,mode:'managed',memories:[],scanned:1}));await page.getByRole('button',{name:'查此条之前',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',keyword:'',before:item.id,...bounds});
 await page.locator('#memory-keyword').fill('关键词');await page.locator('#memory-search-source').selectOption('admin');await page.locator('#memory-search-state-filter').selectOption('all');await page.evaluate(()=>replies.push({available:true,mode:'managed',memories:[],scanned:1}));await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',keyword:'关键词',source:'admin',state:'all',before:item.id,...bounds});
 await page.locator('#memory-search-from').fill('');await page.locator('#memory-search-until').fill('');await page.evaluate(()=>replies.push({available:true,mode:'managed',memories:[],scanned:1}));await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(Object.hasOwn(await page.evaluate(()=>requests.at(-1).body),'from'),false);assert.equal(Object.hasOwn(await page.evaluate(()=>requests.at(-1).body),'until'),false);
});

test('memory revision time validates order clears on scope changes and fits narrow themes',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#memory-search-until').fill('2026-10-17T08:00');await page.evaluate(()=>replies.push({available:true,mode:'managed',memories:[],scanned:5000,partial:true,searchMore:true,nextBefore:'9007199254740993'}));await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);const count=await page.evaluate(()=>requests.length);
 await page.locator('#memory-search-from').fill('2026-10-17T08:00');await page.locator('#memory-search').click();assert.equal(await page.evaluate(()=>requests.length),count);assert.match(await page.locator('#agent-error').textContent(),/开始时间须早于/);assert.equal(await page.locator('#memory-search-continue').isEnabled(),true);
 await page.evaluate(()=>replies.push({error:'读取失败',status:503}));await page.locator('#memory-search-continue').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#memory-search-continue').isHidden(),true);
 await page.locator('#memory-search-from').fill('2026-10-16T08:00');await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/memory-time-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.locator('#memory-search-from').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/memory-time-ui/'+theme+'.png'});}
 await page.locator('#memory-search').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#memory-search-from').isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),{available:true,mode:'managed',memories:[item],scanned:1,searchMore:true,nextBefore:'1'});await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#memory-search-from').inputValue(),'');assert.equal(await page.locator('#memory-search-until').inputValue(),'');assert.equal(await page.locator('#memory-search-results').textContent(),'');
});

test('newest revision pages preserve scope and list state and restore with the loaded current version',async t=>{
 const page=await fixture(t),latest={...item,revision:'9007199254740995',active:false,text:'最新完整正文'};await load(page,{...managed,items:[latest],more:true});const original=await page.locator('#memory-list').textContent();
 await page.evaluate(value=>replies.push(value),{mode:'managed',items:[latest,{...latest,revision:'9007199254740994',text:'前一版本'}],more:true,next:'9007199254740994'});await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);let params=Object.fromEntries(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams);assert.deepEqual(params,{botId:'12345678',type:'group',id:'10',memoryId:item.id,after:'',order:'desc'});assert.match(await page.locator('#memory-versions').textContent(),/从新到旧.*从最新修订开始/s);
 await page.evaluate(value=>{agentScope={botId:'87654321',type:'private',id:'99'};replies.push(value)},{mode:'managed',items:[{...latest,revision:'9007199254740993',text:'更早正文 <img src=x>'}],more:false,next:'9007199254740993'});await page.getByRole('button',{name:'下一页版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);params=Object.fromEntries(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams);assert.deepEqual(params,{botId:'12345678',type:'group',id:'10',memoryId:item.id,after:'9007199254740994',order:'desc'});assert.match(await page.locator('#memory-versions').textContent(),/仅查看 v9007199254740994 之前/);assert.equal(await page.locator('#memory-versions img').count(),0);assert.equal(await page.locator('#memory-list').textContent(),original);assert.equal(await page.locator('#memory-next').isEnabled(),true);
 await page.getByRole('button',{name:'对比当前版本',exact:true}).click();assert.match(await page.locator('.memory-comparison').textContent(),/当前 v9007199254740995/);await page.getByRole('button',{name:'恢复此版本正文',exact:true}).click();await page.evaluate(()=>replies.push({saved:true}));await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',memoryId:item.id,revision:latest.revision,targetRevision:'9007199254740993',action:'revert',text:''});
});

test('newest revision page failures clear old controls and late responses do not refill another scope',async t=>{
 const page=await fixture(t);await load(page);const revisions={mode:'managed',items:[item,{...item,revision:'1',text:'旧正文🙂'}],more:true,next:'1'};await page.evaluate(value=>replies.push(value),revisions);await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/memory-versions-newest-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.locator('#memory-versions').scrollIntoViewIfNeeded();await page.screenshot({path:'.local/memory-versions-newest-ui/'+theme+'.png'});}
 await page.evaluate(()=>replies.push({error:'读取故障',status:503}));await page.getByRole('button',{name:'下一页版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-versions').textContent(),/版本读取失败/);assert.equal(await page.locator('#memory-versions button').count(),0);
 await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>pending.length===1);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending.shift()({ok:true,json:async()=>value}),revisions);await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#memory-versions').textContent(),'');assert.equal(await page.evaluate(()=>memoryScope),null);
});

test('disabled search locates current text and restores through the original versioned save',async t=>{
 const page=await fixture(t);await load(page,{...managed,more:true});const original=await page.locator('#memory-list').textContent();
 await page.locator('#memory-keyword').fill('停用');await page.locator('#memory-search-state-filter').selectOption('disabled');const found={...item,active:false,revision:'4',text:'停用片段',truncated:true};
 await page.evaluate(value=>replies.push(value),{available:true,mode:'managed',state:'disabled',memories:[found],scanned:1,partial:true});await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',keyword:'停用',state:'disabled'});assert.match(await page.locator('#memory-search-state').textContent(),/已加载搜索状态：仅停用/);assert.match(await page.locator('#memory-search-results').textContent(),/v4 · 停用.*可定位当前记忆/s);assert.equal(await page.locator('#memory-search-results').getByRole('button',{name:'读取较长记忆正文'}).count(),0);
 const current={...found,revision:'5',text:'停用完整正文 <img src=x> 🙂'};await page.evaluate(value=>replies.push(value),{mode:'managed',items:[current]});await page.locator('#memory-search-results').getByRole('button',{name:'定位当前记忆',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#memory-list').textContent(),original);assert.equal(await page.locator('#memory-next').isEnabled(),true);
 await page.locator('#memory-current-detail').getByRole('button',{name:'修订或启停',exact:true}).click();assert.equal(await page.locator('#memory-text').inputValue(),current.text);await page.locator('#memory-action').selectOption('restore');await page.evaluate(()=>replies.push({saved:true}));await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',memoryId:item.id,revision:'5',action:'restore',text:''});assert.equal(await page.locator('#memory-search-state-filter').inputValue(),'active');
});

test('memory search continuation and older results retain state independently of list and drafts',async t=>{
 const page=await fixture(t);await page.locator('#memory-filter').selectOption('active');await load(page);await page.locator('#memory-search-state-filter').selectOption('disabled');await page.locator('#memory-search-source').selectOption('admin');
 const result={available:true,mode:'managed',state:'disabled',source:'admin',memories:[{...item,active:false,source:'admin'}],scanned:1,partial:true,searchMore:true,nextBefore:'9007199254740992'};
 await page.evaluate(value=>replies.push(value),result);await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);const expected={botId:'12345678',type:'group',id:'10',keyword:'',source:'admin',state:'disabled'};assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),expected);
 await page.locator('#memory-search-state-filter').selectOption('all');await page.locator('#memory-search-source').selectOption('agent');await page.locator('#memory-keyword').fill('draft');await page.evaluate(value=>replies.push(value),{...result,memories:[],searchMore:false});await page.locator('#memory-search-continue').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:result.nextBefore});assert.equal(await page.locator('#memory-search-state-filter').inputValue(),'disabled');assert.equal(await page.evaluate(()=>memoryFilter),'active');
 await page.locator('#memory-search-before').fill('');await page.evaluate(value=>replies.push(value),result);await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);await page.locator('#memory-search-state-filter').selectOption('active');await page.evaluate(value=>replies.push(value),{...result,searchMore:false});await page.locator('#memory-search-results').getByRole('button',{name:'查此条之前',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:item.id});
 await page.locator('#memory-search-state-filter').selectOption('all');await page.evaluate(value=>replies.push(value),{...result,state:'all',memories:[{...item,active:true}]});await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.evaluate(()=>requests.at(-1).body.state),'all');assert.equal(await page.locator('#memory-search-results').getByRole('button',{name:'读取较长记忆正文'}).count(),1);
});

test('memory search state clears on reload scope and late responses and fits narrow themes',async t=>{
 const page=await fixture(t);assert.equal(await page.locator('#memory-search-state-filter').isDisabled(),true);await load(page);await page.locator('#memory-search-state-filter').selectOption('disabled');const count=await page.evaluate(()=>requests.length);await page.locator('#memory-search').click();assert.equal(await page.evaluate(()=>requests.length),count);
 await page.locator('#memory-keyword').fill('停用');await page.evaluate(value=>replies.push(value),{available:true,mode:'managed',state:'disabled',memories:[{...item,active:false}],scanned:1});await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/memory-search-state-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/memory-search-state-ui/'+theme+'.png',fullPage:true});}
 await load(page);assert.equal(await page.locator('#memory-search-state-filter').inputValue(),'active');await page.locator('#memory-search-state-filter').selectOption('disabled');await page.locator('#memory-search').click();await page.waitForFunction(()=>pending.length===1);await page.evaluate(()=>{resetAgentArchive();pending.shift()({ok:true,json:async()=>({available:true,mode:'managed',memories:[{id:'1',text:'late disabled'}]})});});await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#memory-search-state-filter').inputValue(),'active');assert.equal(await page.locator('#memory-search-state-filter').isDisabled(),true);assert.equal(await page.locator('#memory-search-results').textContent(),'');
});

test('source-only memory search keeps applied empty keyword for continuation and earlier lookup',async t=>{
 const page=await fixture(t);await load(page);const list=await page.locator('#memory-list').textContent();
 const count=await page.evaluate(()=>requests.length);await page.locator('#memory-search-before').fill(item.id);await page.locator('#memory-search').click();assert.equal(await page.evaluate(()=>requests.length),count);assert.match(await page.locator('#agent-error').textContent(),/关键词或选择记忆来源/);
 await page.locator('#memory-search-source').selectOption('admin');await page.locator('#memory-search-before').fill('');await page.locator('#memory-keyword').fill('  ');
 const result={available:true,mode:'managed',source:'admin',memories:[{...item,source:'admin',truncated:false}],scanned:2,partial:true,searchMore:true,nextBefore:'9007199254740992'};
 await page.evaluate(value=>replies.push(value),result);await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 const expected={botId:'12345678',type:'group',id:'10',keyword:'',source:'admin'};assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),expected);assert.match(await page.locator('#memory-search-state').textContent(),/不限（仅按来源）.*后台录入或修订/s);
 await page.locator('#memory-search-source').selectOption('agent');await page.locator('#memory-keyword').fill('draft');await page.evaluate(value=>replies.push(value),{...result,memories:[],searchMore:false});await page.locator('#memory-search-continue').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:result.nextBefore});assert.equal(await page.locator('#memory-keyword').inputValue(),'');assert.equal(await page.locator('#memory-search-source').inputValue(),'admin');
 await page.locator('#memory-search-before').fill('');await page.evaluate(value=>replies.push(value),result);await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);await page.locator('#memory-search-source').selectOption('legacy');await page.locator('#memory-keyword').fill('new draft');await page.evaluate(value=>replies.push(value),{...result,searchMore:false});await page.locator('#memory-search-results').getByRole('button',{name:'查此条之前',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:item.id});assert.equal(await page.locator('#memory-list').textContent(),list);
 await page.locator('#memory-search-source').selectOption('');const after=await page.evaluate(()=>requests.length);await page.locator('#memory-search').click();assert.equal(await page.evaluate(()=>requests.length),after);
});

test('current memory lookup preserves list paging and edits the loaded current version',async t=>{
 const page=await fixture(t);await page.locator('#memory-filter').selectOption('active');await load(page,{...managed,more:true,next:item.id});const original=await page.locator('#memory-list').textContent();
 const current={...item,id:'9007199254740995',revision:'7',active:false,text:'当前完整正文 <img src=x> 🙂'};
 await page.locator('#memory-current-id').fill(current.id);await page.evaluate(value=>replies.push(value),{mode:'managed',items:[current]});await page.locator('#memory-current-load').click();await page.waitForFunction(()=>!agentBusy);
 const url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.deepEqual(Object.fromEntries(url.searchParams),{botId:'12345678',type:'group',id:'10',memoryId:current.id});assert.equal(await page.locator('#memory-list').textContent(),original);assert.equal(await page.locator('#memory-next').isEnabled(),true);assert.equal(await page.evaluate(()=>memoryFilter),'active');assert.equal(await page.locator('#memory-current-detail img').count(),0);assert.match(await page.locator('#memory-current-detail').textContent(),/v7.*停用.*当前完整正文/s);
 await page.evaluate(value=>replies.push(value),{mode:'managed',items:[{...current,revision:'2',text:'旧正文'}],more:false});await page.locator('#memory-current-detail').getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);await page.locator('#memory-versions').getByRole('button',{name:'对比当前版本',exact:true}).click();assert.match(await page.locator('.memory-comparison').textContent(),/当前 v7/);
 await page.locator('#memory-current-detail').getByRole('button',{name:'修订或启停',exact:true}).click();assert.equal(await page.locator('#memory-text').inputValue(),current.text);await page.locator('#memory-action').selectOption('restore');await page.evaluate(()=>replies.push({saved:true}));await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',memoryId:current.id,revision:'7',action:'restore',text:''});assert.equal(await page.locator('#memory-current-detail').textContent(),'');assert.equal(await page.evaluate(()=>memoryCurrent),null);
});

test('memory search locates current full text independently of a truncated result version',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#memory-keyword').fill('片段');const found={...item,id:'9007199254740995',revision:'2',text:'旧搜索片段',truncated:true};
 await page.evaluate(value=>replies.push(value),{available:true,mode:'managed',memories:[found],scanned:1,partial:true});await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);const original=await page.locator('#memory-search-results').textContent();
 const current={...found,revision:'9',text:'最新完整正文',active:true};await page.evaluate(value=>replies.push(value),{mode:'managed',items:[current]});await page.locator('#memory-search-results').getByRole('button',{name:'定位当前记忆',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#memory-current-id').inputValue(),found.id);assert.equal(await page.locator('#memory-search-results').textContent(),original);
 await page.locator('#memory-current-detail').getByRole('button',{name:'修订或启停',exact:true}).click();assert.equal(await page.locator('#memory-text').inputValue(),'最新完整正文');await page.locator('#memory-text').fill('修订内容');await page.evaluate(()=>replies.push({error:'版本冲突',status:409}));await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.evaluate(()=>requests.at(-1).body.revision),'9');assert.equal(await page.locator('#memory-current-detail').textContent(),'');assert.match(await page.locator('#memory-dialog-error').textContent(),/重新读取/);await page.locator('#memory-cancel').click();
});

test('current lookup distinguishes missing unavailable failed and late responses with narrow layouts',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#memory-current-id').fill('0');const count=await page.evaluate(()=>requests.length);await page.locator('#memory-current-load').click();assert.equal(await page.evaluate(()=>requests.length),count);
 await page.locator('#memory-current-id').fill(item.id);await page.evaluate(value=>replies.push(value),managed);await page.locator('#memory-current-load').click();await page.waitForFunction(()=>!agentBusy);await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/memory-current-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/memory-current-ui/'+theme+'.png',fullPage:true});}
 for(const [value,message] of [[{mode:'managed',items:[]},/未找到该记忆/],[{mode:'legacy',items:[]},/旧文件归属或迁移/],[{error:'读取失败',status:503},/读取失败/]]){await page.evaluate(value=>replies.push(value),value);await page.locator('#memory-current-load').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#memory-current-detail').textContent(),'');assert.match(await page.locator('#memory-current-state').textContent(),message);}
 await page.locator('#memory-current-load').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#memory-current-id').isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),managed);await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#memory-current-detail').textContent(),'');assert.equal(await page.locator('#memory-current-id').inputValue(),'');assert.equal(await page.evaluate(()=>memoryCurrent),null);
});

test('memory source search preserves the applied source through continuation and older reads',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#memory-keyword').fill('记忆');await page.locator('#memory-search-source').selectOption('admin');
 const result={available:true,mode:'managed',source:'admin',memories:[{...item,source:'admin',truncated:false}],scanned:2,partial:true,searchMore:true,nextBefore:'9007199254740992'};
 await page.evaluate(value=>replies.push(value),result);await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 const expected={botId:'12345678',type:'group',id:'10',keyword:'记忆',source:'admin'};assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),expected);assert.match(await page.locator('#memory-search-state').textContent(),/后台录入或修订（按当前版本）/);
 await page.locator('#memory-search-source').selectOption('agent');await page.locator('#memory-keyword').fill('draft');await page.evaluate(value=>replies.push(value),{...result,searchMore:false});await page.locator('#memory-search-continue').click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:'9007199254740992'});assert.equal(await page.locator('#memory-search-source').inputValue(),'admin');
 await page.evaluate(value=>replies.push(value),result);await page.getByRole('button',{name:'查此条之前',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:item.id});
 await page.evaluate(value=>replies.push(value),result);await page.getByRole('button',{name:'读取较长记忆正文',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.has('source'),false);
 await page.locator('#memory-search-source').selectOption('');await page.evaluate(value=>replies.push(value),{...result,source:undefined});await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.evaluate(()=>Object.hasOwn(requests.at(-1).body,'source')),false);assert.match(await page.locator('#memory-search-state').textContent(),/全部来源/);
});

test('memory source controls clear on reload and scope change and fit narrow themes',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#memory-search-source').selectOption('legacy');await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/memory-source-ui',{recursive:true});
 for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/memory-source-ui/'+theme+'.png',fullPage:true});}
 await load(page);assert.equal(await page.locator('#memory-search-source').inputValue(),'');await page.locator('#memory-search-source').selectOption('admin');await page.evaluate(()=>{agentScope={botId:'12345678',type:'private',id:'10'};resetAgentMemory();});assert.equal(await page.locator('#memory-search-source').inputValue(),'');assert.equal(await page.locator('#memory-search-source').isDisabled(),true);
});

async function memoryContinueSearch(page,value={available:true,mode:'managed',memories:[],scanned:5000,partial:true,searchMore:true,nextBefore:'9007199254740993'}){
 await load(page);await page.locator('#memory-keyword').fill('记忆');await page.evaluate(value=>replies.push(value),value);await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
}

test('memory continuation uses the scanned empty-window position and loaded scope instead of drafts',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#memory-keyword').fill('记忆');await page.locator('#memory-search-before').fill('9223372036854775807');await page.evaluate(()=>replies.push({available:true,mode:'managed',memories:[],scanned:5000,partial:true,searchMore:true,nextBefore:'9007199254740993'}));await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 assert.equal(await page.locator('#memory-search-results').textContent(),'');assert.equal(await page.locator('#memory-search-continue').isEnabled(),true);assert.match(await page.locator('#memory-search-state').textContent(),/9007199254740993 之前/);
 const count=await page.evaluate(()=>requests.length);await page.locator('#memory-keyword').fill('draft');await page.locator('#memory-search-before').fill('01');await page.evaluate(()=>{document.getElementById('agent-id').value='99';replies.push({available:true,mode:'managed',memories:[{id:'2',revision:'4',text:'older',truncated:false}],scanned:3,partial:false,searchMore:false});});
 assert.equal(await page.evaluate(()=>requests.length),count);await page.locator('#memory-search-continue').click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',keyword:'记忆',before:'9007199254740993'});assert.equal(await page.locator('#memory-keyword').inputValue(),'记忆');assert.equal(await page.locator('#memory-search-before').inputValue(),'9007199254740993');assert.equal(await page.locator('#memory-search-continue').isHidden(),true);assert.equal(await page.evaluate(()=>requests.length),count+1);assert.match(await page.locator('#memory-search-results').textContent(),/older/);
});

test('memory continuation requires managed available data and an advancing string memory ID',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#memory-keyword').fill('记忆');await page.locator('#memory-search-before').fill('9007199254740994');
 for(const metadata of [
  {partial:true},{searchMore:false,nextBefore:'2'},{searchMore:'true',nextBefore:'2'},
  {available:false,searchMore:true,nextBefore:'2'},{mode:'legacy',searchMore:true,nextBefore:'2'},
  ...['0','01','-1','9223372036854775808','9007199254740994','9007199254740995',9007199254740992,null,'<img src=x>'].map(nextBefore=>({searchMore:true,nextBefore}))
 ]){
  await page.evaluate(value=>replies.push(value),{available:true,mode:'managed',memories:[],scanned:1,...metadata});await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#memory-search-continue').isHidden(),true);
 }
 await page.evaluate(()=>replies.push({available:true,mode:'managed',memories:[],searchMore:true,nextBefore:'9007199254740993'}));await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 const count=await page.evaluate(()=>requests.length);await page.locator('#memory-search-before').fill('01');await page.locator('#memory-search').click();assert.equal(await page.evaluate(()=>requests.length),count);assert.equal(await page.locator('#memory-search-continue').isEnabled(),true);
 await page.locator('#memory-create').click();assert.equal(await page.locator('#memory-search-continue').isDisabled(),true);await page.locator('#memory-cancel').click();assert.equal(await page.locator('#memory-search-continue').isEnabled(),true);
 await page.locator('#memory-create').click();await page.locator('#memory-text').fill('new entry');await page.evaluate(()=>replies.push({error:'结果未知',status:503}));await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#memory-search-continue').isHidden(),true);assert.match(await page.locator('#memory-dialog-error').textContent(),/重新读取/);
});

test('memory continuation clears failed and late requests without retrying or refilling another session',async t=>{
 const page=await fixture(t);await memoryContinueSearch(page);await page.evaluate(()=>replies.push({error:'记忆读取失败',status:503}));await page.locator('#memory-search-continue').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-search-state').textContent(),/搜索失败/);assert.equal(await page.locator('#memory-search-continue').isHidden(),true);
 await page.evaluate(()=>replies.push({available:true,mode:'managed',memories:[],searchMore:true,nextBefore:'8000000000000001'}));await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);await page.locator('#memory-search-continue').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#memory-search-continue').isHidden(),true);
 await page.evaluate(()=>{authenticated=false;csrf='';showAuth();});const count=await page.evaluate(()=>requests.length);await page.evaluate(()=>pending[0]({ok:true,status:200,json:async()=>({available:true,mode:'managed',memories:[],searchMore:true,nextBefore:'7000000000000001'})}));await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#memory-search-continue').isHidden(),true);assert.equal(await page.locator('#memory-search-results').textContent(),'');assert.equal(await page.evaluate(()=>requests.length),count);
});

test('memory continuation clears on reload and scope changes and fits narrow themes',async t=>{
 const page=await fixture(t);await memoryContinueSearch(page);await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/agent-memory-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/agent-memory-ui/continuation-'+theme+'.png',fullPage:true});}
 await load(page);assert.equal(await page.locator('#memory-search-continue').isHidden(),true);await page.locator('#memory-keyword').fill('记忆');await page.evaluate(()=>replies.push({available:true,mode:'managed',memories:[],searchMore:true,nextBefore:'9007199254740993'}));await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);await page.evaluate(()=>clearAgentContent());assert.equal(await page.locator('#memory-search-continue').isHidden(),true);assert.equal(await page.evaluate(()=>memorySearchContinue),null);
});

test('manual memory creation confirms loaded scope and distinguishes reuse without extra revisions',async t=>{
 const page=await fixture(t);await load(page,{...managed,items:[]});await page.locator('#memory-create').click();assert.equal(await page.locator('#memory-action').isDisabled(),true);assert.equal(await page.locator('#memory-text').evaluate(e=>e.readOnly),false);
 await page.locator('#memory-text').fill('line1\nline2');await page.locator('#memory-save').click();assert.equal(await page.evaluate(()=>requests.length),1);await page.locator('#memory-cancel').click();assert.equal(await page.evaluate(()=>requests.length),1);
 await page.locator('#memory-create').click();await page.locator('#memory-text').fill('手动记忆');await page.evaluate(()=>{document.getElementById('agent-id').value='99';replies.push({saved:true,created:true,item:{id:'9007199254740993'}});});await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1)),{url:'/api/agent/memories/create',body:{botId:'12345678',type:'group',id:'10',text:'手动记忆'}});assert.match(await page.locator('#memory-state').textContent(),/已新增记忆 #9007199254740993/);
 await load(page);await page.locator('#memory-create').click();await page.locator('#memory-text').fill('same');await page.evaluate(()=>replies.push({saved:true,created:false,item:{id:'7'}}));await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-state').textContent(),/已复用相同启用记忆 #7/);
 await load(page,{mode:'legacy',items:[],more:false,next:''});assert.equal(await page.locator('#memory-create').isDisabled(),true);
});

test('manual creation blocks navigation while pending and never claims unknown writes succeeded',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#memory-create').click();await page.locator('#memory-text').fill('new value');await page.locator('#memory-save').click();await page.waitForFunction(()=>pending.length===1);
 assert.equal(await page.locator('#memory-cancel').isDisabled(),true);await page.evaluate(()=>document.querySelector('[data-view=overview]').dispatchEvent(new Event('click')));assert.equal(await page.locator('#agent-view').isVisible(),true);
 await page.evaluate(()=>pending[0]({ok:true,status:200,json:async()=>({saved:true})}));await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-dialog-error').textContent(),/新增结果无法确认/);assert.equal(await page.evaluate(()=>requests.length),2);
 await page.locator('#memory-cancel').click();await page.locator('#memory-create').click();await page.locator('#memory-text').fill('new value');await page.evaluate(()=>replies.push({error:'归属冲突',status:409}));await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-dialog-error').textContent(),/归属冲突.*重新读取/);assert.equal(await page.evaluate(()=>requests.length),3);
});

test('memory search before uses string IDs and older buttons preserve loaded scope and keyword',async t=>{
 const page=await fixture(t);await load(page,{...managed,more:true,next:'99'});
 const keyword='原关键词 <img src=x>';
 await page.locator('#memory-keyword').fill(keyword);await page.locator('#memory-search-before').fill('9223372036854775807');
 await page.evaluate(value=>{document.getElementById('agent-id').value='99';replies.push(value)},{available:true,mode:'managed',memories:[{...item,truncated:false}],scanned:2,partial:false});
 await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1)),{url:'/api/agent/memories/search',body:{botId:'12345678',type:'group',id:'10',keyword,before:'9223372036854775807'}});
 assert.match(await page.locator('#memory-search-state').textContent(),/原关键词.*记忆编号小于 #9223372036854775807/);assert.equal(await page.locator('#memory-search-state img').count(),0);
 assert.equal(await page.locator('#memory-next').isDisabled(),false);const original=await page.locator('#memory-list').textContent();
 await page.locator('#memory-keyword').fill('尚未应用的草稿');await page.locator('#memory-search-before').fill('7');
 await page.evaluate(()=>replies.push({available:true,mode:'managed',memories:[],scanned:3,partial:false}));await page.locator('#memory-search-results').getByRole('button',{name:'查此条之前',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',keyword,before:'9007199254740993'});
 assert.equal(await page.locator('#memory-keyword').inputValue(),keyword);assert.equal(await page.locator('#memory-search-before').inputValue(),'9007199254740993');assert.match(await page.locator('#memory-search-state').textContent(),/小于 #9007199254740993.*未命中/);
 assert.equal(await page.locator('#memory-list').textContent(),original);assert.equal(await page.locator('#memory-next').isDisabled(),false);
 await load(page);assert.equal(await page.locator('#memory-search-before').inputValue(),'');
 await page.evaluate(()=>replies.push({available:true,mode:'managed',memories:[],scanned:0,partial:false}));await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 assert.equal(Object.hasOwn(await page.evaluate(()=>requests.at(-1).body),'before'),false);assert.match(await page.locator('#memory-search-state').textContent(),/记忆编号不限/);
});

test('memory before rejects invalid input and clears scope and late session results',async t=>{
 const page=await fixture(t);assert.equal(await page.locator('#memory-search-before').isDisabled(),true);await load(page);await page.locator('#memory-keyword').fill('边界测试');
 for(const value of ['0','01','-1','1.5','1e3','9223372036854775808']){await page.locator('#memory-search-before').fill(value);await page.locator('#memory-search').click();assert.equal(await page.evaluate(()=>requests.length),1);assert.match(await page.locator('#agent-error').textContent(),/记忆编号边界/);}
 await page.locator('#memory-search-before').fill('42');await page.locator('#memory-search').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#memory-search-before').isDisabled(),true);
 await page.evaluate(()=>{authenticated=false;showAuth();});assert.equal(await page.locator('#memory-search-before').inputValue(),'');assert.equal(await page.locator('#memory-keyword').inputValue(),'');
 await page.evaluate(()=>{authenticated=true;csrf='new-memory-session';agentVisible=true;agentScope={botId:'87654321',type:'private',id:'20'};document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;agentControls();});await load(page,{...managed,items:[{...item,text:'NEW_SCOPE_MEMORY'}]});
 await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),{available:true,mode:'managed',memories:[{...item,id:'41',text:'OLD_BOUNDARY_RESPONSE'}],scanned:1,partial:false});await page.evaluate(()=>Promise.resolve());
 assert.equal(await page.locator('#memory-search-results').textContent(),'');assert.equal(await page.locator('#memory-search-before').inputValue(),'');assert.match(await page.locator('#memory-list').textContent(),/NEW_SCOPE_MEMORY/);
 await page.locator('#memory-search-before').fill('11');await page.evaluate(()=>clearAgentContent());assert.equal(await page.locator('#memory-search-before').inputValue(),'');assert.equal(await page.evaluate(()=>memoryScope),null);
});

test('320px bounded memory search layout stays readable in both themes',async t=>{
 const page=await fixture(t);await load(page);await page.setViewportSize({width:320,height:800});await page.locator('#memory-keyword').fill('只读');await page.locator('#memory-search-before').fill('9223372036854775807');
 await page.evaluate(value=>replies.push(value),{available:true,mode:'managed',memories:[{...item,truncated:false}],scanned:5000,partial:true});await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 await fs.mkdir('.local/agent-memory-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/agent-memory-ui/before-'+theme+'.png'});}
 await page.locator('#memory-keyword').fill('k'.repeat(128));await page.evaluate(()=>replies.push({available:true,mode:'managed',memories:[],scanned:1,partial:false}));await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);assert.match(await page.locator('#memory-search-state').textContent(),/k{128}/);await page.screenshot({path:'.local/agent-memory-ui/before-long-'+theme+'.png'});}
});

test('memory search stays read-only and detailed reads bind exact loaded scope and version',async t=>{
 const page=await fixture(t);await load(page);const match={...item,text:'只读片段 <img src=x>',revision:'9007199254740994',truncated:true};
 await page.locator('#memory-keyword').fill('片段');await page.evaluate(value=>{document.getElementById('agent-id').value='99';replies.push(value)},{available:true,mode:'managed',memories:[match],scanned:5000,partial:true});await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',keyword:'片段'});const results=page.locator('#memory-search-results');assert.equal(await results.locator('img').count(),0);assert.match(await page.locator('#memory-search-state').textContent(),/5000.*只读片段.*部分内容/);
 assert.equal(await results.getByRole('button',{name:'修订或启停'}).count(),0);
 await page.evaluate(value=>replies.push(value),{available:true,mode:'managed',memories:[{...match,text:'完整较长正文 <script>x</script>',truncated:false}]});await results.getByRole('button',{name:'读取较长记忆正文'}).click();await page.waitForFunction(()=>!agentBusy);
 const url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.equal(url.searchParams.get('id'),'10');assert.equal(url.searchParams.get('memoryId'),item.id);assert.equal(url.searchParams.get('revision'),'9007199254740994');assert.match(await results.textContent(),/完整较长正文.*已核对/s);assert.equal(await results.locator('script').count(),0);
 await page.setViewportSize({width:320,height:800});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);}
 await load(page);assert.equal(await results.textContent(),'');
});

test('memory search handles stale versions, errors and late session responses',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#memory-keyword').fill('记忆');await page.evaluate(value=>replies.push(value),{available:true,mode:'managed',memories:[{...item,truncated:true}],scanned:1,partial:true});await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);
 const read=page.locator('#memory-search-results').getByRole('button',{name:'读取较长记忆正文'});
 await page.evaluate(()=>replies.push({available:true,memories:[]}));await read.click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-search-results').textContent(),/已变更、停用/);
 await page.evaluate(()=>replies.push({error:'读取故障',status:503}));await read.click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-search-results').textContent(),/读取失败/);assert.doesNotMatch(await page.locator('#memory-search-results').textContent(),/已核对该条当前启用版本/);
 await page.evaluate(()=>replies.push({error:'搜索故障',status:503}));await page.locator('#memory-search').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-search-state').textContent(),/搜索失败/);assert.equal(await page.locator('#memory-search-results').textContent(),'');
 await page.locator('#memory-search').click();await page.waitForFunction(()=>pending.length===1);await page.evaluate(()=>{authenticated=false;showAuth();});await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),{available:true,memories:[{...item,text:'OLD_PRIVATE_SEARCH'}],scanned:1});await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#memory-search-results').textContent(),'');assert.equal(await page.locator('#memory-keyword').inputValue(),'');
});

test('memory comparison preserves Unicode, renders inert differences and never writes',async t=>{
 const page=await fixture(t);const current={...item,text:'共同🙂新<img src=x>结尾',revision:'9007199254740994',active:false};await load(page,{...managed,items:[current]});
 await page.evaluate(value=>replies.push(value),{...managed,items:[{...item,text:'共同🙂旧<script>x</script>结尾',revision:'9007199254740993'},current]});await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 assert.equal(await page.getByRole('button',{name:'对比当前版本',exact:true}).count(),1);const before=await page.evaluate(()=>requests.length);
 await page.getByRole('button',{name:'对比当前版本',exact:true}).click();const panel=page.locator('.memory-comparison');assert.match(await panel.textContent(),/9007199254740993.*9007199254740994/s);assert.match(await panel.textContent(),/旧版本启用.*当前版本停用/s);
 assert.equal(await panel.locator('pre').nth(0).textContent(),'共同🙂旧<script>x</script>结尾');assert.equal(await panel.locator('pre').nth(1).textContent(),current.text);assert.equal(await panel.locator('img,script').count(),0);assert.equal(await panel.locator('del').textContent(),'旧<script>x</script');assert.equal(await panel.locator('ins').textContent(),'新<img src=x');assert.equal(await page.evaluate(()=>requests.length),before);
 await page.setViewportSize({width:320,height:800});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);}
 await page.getByRole('button',{name:'收起对比',exact:true}).click();assert.equal(await panel.textContent(),'');assert.equal(await page.evaluate(()=>requests.length),before);
 await page.getByRole('button',{name:'对比当前版本',exact:true}).click();await load(page);assert.equal(await page.locator('.memory-comparison').count(),0);
});

test('memory comparison distinguishes equal text from state changes and handles insertion',async t=>{
 const page=await fixture(t);await load(page,{...managed,items:[{...item,text:'前🙂后',active:false}]});
 await page.evaluate(value=>replies.push(value),{...managed,items:[{...item,text:'前🙂后',revision:'1',active:true}]});await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);await page.getByRole('button',{name:'对比当前版本',exact:true}).click();
 assert.match(await page.locator('.memory-comparison').textContent(),/脱敏后的正文相同/);assert.equal(await page.locator('.memory-comparison del,.memory-comparison ins').count(),0);
 await page.evaluate(()=>renderMemoryComparison(document.querySelector('.memory-comparison'),{text:'前后',revision:'1',active:true},{text:'前🙂后',revision:'2',active:true}));assert.equal(await page.locator('.memory-comparison ins').textContent(),'🙂');assert.equal(await page.locator('.memory-comparison del').count(),0);
 await page.evaluate(()=>document.getElementById('agent-id').dispatchEvent(new Event('change')));assert.equal(await page.locator('.memory-comparison').count(),0);
});

test('memory pagination binds loaded state while history remains unfiltered',async t=>{
 const page=await fixture(t);await page.locator('#memory-filter').selectOption('disabled');await load(page,{...managed,items:[{...item,active:false}],more:true,next:'9007199254740993'});
 assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.get('state'),'disabled');assert.match(await page.locator('#memory-state').textContent(),/已加载筛选：仅停用/);
 await page.locator('#memory-filter').selectOption('active');await page.evaluate(value=>replies.push(value),{...managed,items:[{...item,active:false}]});await page.locator('#memory-next').click();await page.waitForFunction(()=>!agentBusy);
 const url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.equal(url.searchParams.get('state'),'disabled');assert.equal(url.searchParams.get('after'),'9007199254740993');assert.match(await page.locator('#memory-state').textContent(),/仅停用/);
 await page.evaluate(value=>replies.push(value),managed);await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.has('state'),false);
 await load(page);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.get('state'),'active');assert.match(await page.locator('#memory-state').textContent(),/仅启用/);
 await page.evaluate(()=>document.getElementById('agent-type').dispatchEvent(new Event('change')));assert.equal(await page.locator('#memory-filter').inputValue(),'all');assert.equal(await page.locator('#memory-next').isDisabled(),true);
});

test('historical content restore requires confirmation and sends only scoped revision identifiers',async t=>{
 const page=await fixture(t);const latest={...item,active:false,revision:'9007199254740994'};
 await load(page,{...managed,items:[latest]});
 await page.evaluate(value=>replies.push(value),{...managed,items:[{...item,revision:'9007199254740993',text:'旧正文 <img src=x>'},latest]});
 await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 assert.equal(await page.getByRole('button',{name:'恢复此版本正文',exact:true}).count(),1);
 await page.getByRole('button',{name:'恢复此版本正文',exact:true}).click();
 assert.equal(await page.locator('#memory-text').inputValue(),'旧正文 <img src=x>');assert.equal(await page.locator('#memory-text').evaluate(e=>e.readOnly),true);
 assert.equal(await page.locator('#memory-action').isDisabled(),true);assert.match(await page.locator('#memory-revert-note').textContent(),/停用状态不变/);
 assert.equal(await page.locator('#memory-dialog img').count(),0);assert.equal(await page.evaluate(()=>requests.length),2);
 await page.locator('#memory-cancel').click();assert.equal(await page.evaluate(()=>requests.length),2);
 await page.getByRole('button',{name:'恢复此版本正文',exact:true}).click();
 await page.setViewportSize({width:320,height:800});
 for(const theme of ['light','dark']) { await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true); }
 await page.evaluate(()=>{document.getElementById('agent-id').value='99';replies.push({saved:true});});
 await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',memoryId:item.id,revision:'9007199254740994',targetRevision:'9007199254740993',action:'revert',text:''});
 assert.match(await page.locator('#memory-state').textContent(),/已保存/);assert.equal(await page.locator('#memory-versions').textContent(),'');
 await load(page);await page.locator('#memory-list button').first().click();assert.equal(await page.locator('#memory-text').evaluate(e=>e.readOnly),false);assert.equal(await page.locator('#memory-action').isDisabled(),false);assert.equal(await page.locator('#memory-revert-note').isVisible(),false);
});

test('historical restore conflicts and uncertain responses never retry or claim success',async t=>{
 const page=await fixture(t);await load(page);
 await page.evaluate(value=>replies.push(value),{...managed,items:[{...item,revision:'1'}]});await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 await page.getByRole('button',{name:'恢复此版本正文',exact:true}).click();await page.evaluate(()=>replies.push({error:'版本冲突',status:409}));await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);
 assert.match(await page.locator('#memory-dialog-error').textContent(),/版本冲突.*重新读取/);assert.equal(await page.evaluate(()=>requests.length),3);
 await page.locator('#memory-cancel').click();await page.getByRole('button',{name:'恢复此版本正文',exact:true}).click();await page.locator('#memory-save').click();await page.waitForFunction(()=>pending.length===1);
 assert.equal(await page.locator('#memory-cancel').isDisabled(),true);await page.evaluate(()=>document.querySelector('[data-view=overview]').dispatchEvent(new Event('click')));assert.equal(await page.locator('#agent-view').isVisible(),true);
 await page.evaluate(()=>pending[0]({ok:true,status:200,json:async()=>({})}));await page.waitForFunction(()=>!agentBusy);
 assert.match(await page.locator('#memory-dialog-error').textContent(),/结果未知/);assert.doesNotMatch(await page.locator('#memory-state').textContent(),/已保存/);assert.equal(await page.evaluate(()=>requests.length),4);
});

test('memory source uses loaded scope and string sequence without changing archive navigation',async t=>{
 const page=await fixture(t);const sourceSeq='9007199254740993';await load(page,{...managed,items:[{...item,sourceSeq}]});
 await page.evaluate(value=>{document.getElementById('agent-id').value='99';agentNext='original-cursor';replies.push(value)},{available:true,entries:[{seq:sourceSeq,kind:'event',time:'2026-10-14T00:00:00Z',text:'来源 <img src=x> 中文',truncated:true}]});
 await page.getByRole('button',{name:'查看来源',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 const url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.equal(url.searchParams.get('id'),'10');assert.equal(url.searchParams.get('seq'),sourceSeq);assert.equal(url.searchParams.get('archived'),'true');assert.equal(url.searchParams.get('type'),'group');assert.equal(await page.evaluate(()=>agentNext),'original-cursor');assert.equal(await page.evaluate(()=>agentScope.archived),false);
 assert.equal(await page.locator('#memory-source img').count(),0);assert.match(await page.locator('#memory-source').textContent(),/重置前记录.*来源.*仅显示部分/s);
 await load(page,{...managed,items:[{...item,sourceSeq:'0',source:'legacy'}]});assert.equal(await page.getByRole('button',{name:'查看来源',exact:true}).count(),0);assert.equal(await page.locator('#memory-source').textContent(),'');
});

test('missing source, failed read and historical revision remain distinct',async t=>{
 const page=await fixture(t);await load(page);await page.evaluate(()=>replies.push({available:true,entries:[]}));await page.getByRole('button',{name:'查看来源',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-source').textContent(),/未找到来源记录/);
 await page.evaluate(()=>replies.push({error:'不可读取',status:503}));await page.getByRole('button',{name:'查看来源',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-source').textContent(),/读取失败/);assert.doesNotMatch(await page.locator('#memory-source').textContent(),/未找到来源记录/);
 await page.evaluate(value=>replies.push(value),{...managed,items:[{...item,revision:'1',sourceSeq:'11'}]});await page.getByRole('button',{name:'查看版本',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 await page.evaluate(()=>replies.push({available:true,entries:[{seq:'11',kind:'event',time:'time',text:'old-source',truncated:false}]}));await page.locator('#memory-versions').getByRole('button',{name:'查看来源',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-source').textContent(),/v1.*#11.*old-source/s);
});

test('late source read cannot repopulate a different login or scope',async t=>{
 const page=await fixture(t);await load(page);await page.getByRole('button',{name:'查看来源',exact:true}).click();await page.waitForFunction(()=>pending.length===1);
 await page.evaluate(()=>{authenticated=false;showAuth();authenticated=true;csrf='new-source-session';agentVisible=true;agentScope={botId:'87654321',type:'private',id:'20'};document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;agentControls();});await load(page);
 await page.evaluate(()=>pending[0]({ok:true,status:200,json:async()=>({available:true,entries:[{seq:'23',kind:'event',text:'OLD_PRIVATE_CONTENT'}]})}));await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#memory-source').textContent(),'');assert.equal(await page.evaluate(()=>memoryScope.id),'20');
});

test('memory editing binds loaded scope and version, preserves old versions and renders inert text',async t=>{
 const page=await fixture(t);await load(page);assert.equal(await page.locator('#memory-list img').count(),0);assert.match(await page.locator('#memory-list').textContent(),/9007199254740993.*v2/);
 await page.locator('#memory-list button').first().click();await page.locator('#memory-text').fill('新记忆');await page.evaluate(()=>replies.push({saved:true}));await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);
 assert.equal(await page.locator('#memory-dialog').isVisible(),false);const request=await page.evaluate(()=>requests.at(-1));assert.deepEqual(request.body,{botId:'12345678',type:'group',id:'10',memoryId:item.id,revision:'2',action:'edit',text:'新记忆'});assert.match(await page.locator('#memory-state').textContent(),/已保存/);
 await load(page);await page.evaluate(value=>replies.push(value),{...managed,more:true,next:'2'});await page.locator('#memory-list button').nth(1).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-versions').textContent(),/下一页版本/);
});

test('edit cancel, conflict and uncertain writes never claim saved or resend automatically',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#memory-list button').first().click();await page.locator('#memory-text').fill('line1\nline2');await page.locator('#memory-save').click();assert.equal(await page.evaluate(()=>requests.length),1);
 await page.locator('#memory-cancel').click();assert.equal(await page.evaluate(()=>requests.length),1);
 await page.locator('#memory-list button').first().click();await page.locator('#memory-action').selectOption('disable');await page.evaluate(()=>replies.push({error:'版本冲突',status:409}));await page.locator('#memory-save').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-dialog-error').textContent(),/重新读取/);assert.equal(await page.locator('#memory-dialog').isVisible(),true);
 assert.equal(await page.evaluate(()=>requests.at(-1).body.text),'');assert.equal(await page.evaluate(()=>requests.length),2);
});

test('migration requires preview, explicit confirmation, original scope and fresh expiry',async t=>{
 const page=await fixture(t);await load(page,{mode:'legacy',items:[],more:false,next:''});assert.equal(await page.locator('#memory-migrate').isDisabled(),true);
 const preview={ticket:'signed-preview',count:2,bytes:30,expiresAt:new Date(Date.now()+60000).toISOString()};await page.evaluate(value=>replies.push(value),preview);await page.locator('#memory-preview').click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#memory-preview-text').textContent(),/12345678/);assert.equal(await page.evaluate(()=>requests.length),2);
 await page.locator('#memory-cancel-preview').click();assert.equal(await page.locator('#memory-migrate').isDisabled(),true);
 await page.evaluate(value=>replies.push(value),preview);await page.locator('#memory-preview').click();await page.waitForFunction(()=>!agentBusy);await page.evaluate(()=>replies.push({migrated:true}));await page.locator('#memory-migrate').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',ticket:'signed-preview'});assert.match(await page.locator('#memory-preview-text').textContent(),/迁移完成/);
 await load(page,{mode:'legacy',items:[],more:false,next:''});await page.evaluate(value=>replies.push(value),{...preview,expiresAt:'2000-01-01T00:00:00Z'});await page.locator('#memory-preview').click();await page.waitForFunction(()=>!agentBusy);const before=await page.evaluate(()=>requests.length);await page.locator('#memory-migrate').click();assert.equal(await page.evaluate(()=>requests.length),before);assert.match(await page.locator('#memory-preview-text').textContent(),/过期/);
});

test('pending write blocks navigation and old-session response cannot overwrite new memory',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#memory-list button').first().click();await page.locator('#memory-save').click();await page.waitForFunction(()=>pending.length===1);
 await page.evaluate(()=>document.querySelector('[data-view=overview]').dispatchEvent(new Event('click')));assert.equal(await page.locator('#agent-view').isVisible(),true);assert.equal(await page.locator('#memory-cancel').isDisabled(),true);
 await page.evaluate(()=>{authenticated=false;showAuth();authenticated=true;csrf='new-session';agentVisible=true;agentScope={botId:'87654321',type:'private',id:'10'};document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;agentControls();});await load(page,{...managed,items:[{...item,text:'new-session-only'}]});
 await page.evaluate(()=>pending[0]({ok:true,status:200,json:async()=>({saved:true})}));await page.evaluate(()=>Promise.resolve());assert.match(await page.locator('#memory-list').textContent(),/new-session-only/);assert.equal(await page.evaluate(()=>authenticated),true);
});

test('320px memory edit and migration layout in both themes',async t=>{
 const page=await fixture(t);await page.setViewportSize({width:320,height:800});await load(page);await page.locator('#memory-list button').first().click();await fs.mkdir('.local/agent-memory-ui',{recursive:true});
 for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/agent-memory-ui/'+theme+'.png'});}
});

test('submitting a new scope without a change event clears old memory and migration target',async t=>{
 const page=await fixture(t);await load(page);
 await page.evaluate(()=>{
   const option=document.createElement('option');option.value='12345678';document.getElementById('agent-bot').append(option);
   document.getElementById('agent-id').value='20';
   replies.push({available:true,entries:[],conversations:[],nextCursor:'',scanned:0});
   document.getElementById('agent-scope-form').dispatchEvent(new Event('submit',{cancelable:true}));
 });await page.waitForFunction(()=>!agentBusy&&agentScope.id==='20');
 assert.equal(await page.locator('#memory-list').textContent(),'');assert.equal(await page.evaluate(()=>memoryScope),null);assert.equal(await page.locator('#memory-migrate').isDisabled(),true);
 await load(page);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.get('id'),'20');
});
