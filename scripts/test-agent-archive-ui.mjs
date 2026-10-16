// Isolated Chrome, synthetic archives and mocked network only.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
import test from 'node:test';
const require=createRequire(import.meta.url);
const modules=process.env.LOCALADMIN_NODE_MODULES||path.join(process.env.USERPROFILE,'.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules');
const {chromium}=require(path.join(modules,'playwright'));
const root='internal/localadmin/assets/';
const html=(await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,'');
const app=await fs.readFile(root+'app.js','utf8'),archive=await fs.readFile(root+'agent-archive.js','utf8');
const css=await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8');
const empty={available:true,entries:[],conversations:[],nextCursor:'',scanned:0};
const record={...empty,entries:[{seq:'9007199254740993',kind:'event',time:'2026-10-13T00:00:00Z',text:'<img src="https://invalid.test/leak"> 中文🙂',truncated:false}],nextCursor:'signed-cursor'};
async function fixture(t){
 const browser=await chromium.launch({channel:'chrome',headless:true});t.after(()=>browser.close());
 const context=await browser.newContext({timezoneId:'Asia/Shanghai'});await context.route('**/*',r=>r.abort());const page=await context.newPage();
 await page.setContent(html);await page.addStyleTag({content:css});await page.addScriptTag({content:app});await page.addScriptTag({content:archive});
 await page.evaluate(empty=>{
  authenticated=true;csrf='archive-fixture';window.showOverview=()=>{};window.requests=[];window.pending=[];window.replies=[];
  window.fetch=(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null,signal:options.signal});let value;
   if(url==='/api/overview')value={botAccounts:['12345678','87654321']};else if(url.startsWith('/api/agent/conversations'))value={...empty,conversations:[{type:'group',id:'10'},{type:'private',id:'10'}]};else value=replies.shift();
   if(value)return Promise.resolve({ok:!value.error,status:value.status||200,json:async()=>value});
   return new Promise(resolve=>pending.push(resolve));
  };
  document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.querySelector('[data-view=agent]').click();
 },empty);
 await page.waitForFunction(()=>!agentBusy&&document.getElementById('agent-conversations').children.length===2);return page;
}
async function load(page,value=record){await page.evaluate(value=>replies.push(value),value);await page.locator('#agent-conversations button').first().click();await page.waitForFunction(()=>!agentBusy);}

test('exact archive lookup binds the loaded scope and preserves search continuation and drafts',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-keyword').fill('needle');await page.evaluate(value=>replies.push(value),{...record,searchMore:true,nextBefore:'9007199254740992'});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 const original=await page.locator('#agent-entries').textContent(),status=await page.locator('#agent-status').textContent(),continuation=await page.evaluate(()=>structuredClone(agentSearchContinue));
 await page.evaluate(()=>{document.getElementById('agent-bot').value='87654321';document.getElementById('agent-type').value='private';document.getElementById('agent-id').value='20';document.getElementById('agent-archived').checked=true});await page.locator('#agent-keyword').fill('draft');await page.locator('#agent-search-sender').fill('123');await page.locator('#agent-search-event-type').selectOption('request');
 const seq='9223372036854775806',entry={seq,kind:'response',time:'2026-10-16T00:00:00Z',text:'<img src=x onerror=alert(1)> 中文🙂 '+('原文'.repeat(90)),truncated:true};await page.evaluate(value=>replies.push(value),{...empty,entries:[entry]});await page.locator('#agent-record-seq').fill(seq);await page.locator('#agent-record-seq').press('Enter');await page.waitForFunction(()=>!agentBusy);
 const request=await page.evaluate(()=>requests.at(-1));assert.equal(request.body,null);assert.deepEqual(Object.fromEntries(new URL(request.url,'http://fixture').searchParams),{botId:'12345678',type:'group',id:'10',archived:'false',seq});assert.equal(await page.locator('#agent-record-detail pre').textContent(),entry.text);assert.equal(await page.locator('#agent-record-detail img').count(),0);assert.match(await page.locator('#agent-record-detail').textContent(),/动作响应.*正文有截断/s);assert.match(await page.locator('#agent-record-state').textContent(),/群 10.*最近重置之后/s);assert.equal(await page.locator('#agent-entries').textContent(),original);assert.equal(await page.locator('#agent-status').textContent(),status);assert.deepEqual(await page.evaluate(()=>agentSearchContinue),continuation);assert.equal(await page.locator('#agent-keyword').inputValue(),'draft');assert.equal(await page.locator('#agent-search-continue').isEnabled(),true);
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/archive-exact-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.locator('#agent-record-title').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/archive-exact-ui/'+theme+'.png'});}
 await page.evaluate(()=>replies.push({error:'读取失败',status:503}));await page.locator('#agent-record-read').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#agent-record-detail').textContent(),'');assert.deepEqual(await page.evaluate(()=>agentSearchContinue),continuation);assert.equal(await page.locator('#agent-search-continue').isEnabled(),true);assert.equal(await page.locator('#agent-entries').textContent(),original);
 await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-search-continue').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...continuation.scope,keyword:'needle',before:continuation.query.before});assert.equal(await page.locator('#agent-record-detail').textContent(),'');assert.equal(await page.locator('#agent-record-seq').inputValue(),'');
});

test('exact archive lookup distinguishes invalid missing unavailable and failed reads without losing timeline paging',async t=>{
 const page=await fixture(t);assert.equal(await page.locator('#agent-record-read').isDisabled(),true);await load(page);const original=await page.locator('#agent-entries').textContent(),status=await page.locator('#agent-status').textContent(),count=await page.evaluate(()=>requests.length);
 for(const seq of ['0','01','-1','1.5','9223372036854775808']){await page.locator('#agent-record-seq').fill(seq);await page.locator('#agent-record-read').click();assert.equal(await page.evaluate(()=>requests.length),count);assert.match(await page.locator('#agent-error').textContent(),/来源序号须为有效/);}
 const seq=record.entries[0].seq;await page.locator('#agent-record-seq').fill(seq);
 for(const [value,expected] of [[empty,/未找到该序号/],[{...empty,available:false},/档案文件不可用/],[{...record,entries:[{...record.entries[0],seq:'1'}]},/未找到该序号/],[{error:'读取失败',status:503},/档案定位失败/],[record,/已读取该条脱敏原文/]]){
  await page.evaluate(value=>replies.push(value),value);await page.locator('#agent-record-read').click();await page.waitForFunction(()=>!agentBusy);assert.match((await page.locator('#agent-record-state').textContent())+(await page.locator('#agent-record-detail').textContent()),expected);assert.equal(await page.locator('#agent-entries').textContent(),original);assert.equal(await page.locator('#agent-status').textContent(),status);assert.equal(await page.locator('#agent-next').isEnabled(),true);
 }
 await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-next').click();await page.waitForFunction(()=>!agentBusy);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.get('cursor'),'signed-cursor');assert.equal(await page.locator('#agent-record-detail').textContent(),'');
});

test('exact archive lookup honors loaded reset history mode and discards late reads after leaving',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-archived').check();assert.equal(await page.locator('#agent-record-read').isDisabled(),true);await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-scope-form').evaluate(form=>form.requestSubmit());await page.waitForFunction(()=>!agentBusy);
 await page.locator('#agent-record-seq').fill(record.entries[0].seq);await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-record-read').click();await page.waitForFunction(()=>!agentBusy);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.get('archived'),'true');assert.match(await page.locator('#agent-record-state').textContent(),/包含重置前记录/);
 await page.locator('#agent-record-read').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#agent-record-seq').isDisabled(),true);assert.equal(await page.locator('#agent-record-read').isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),record);await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#agent-record-detail').textContent(),'');assert.equal(await page.locator('#agent-record-seq').inputValue(),'');assert.equal(await page.evaluate(()=>agentScope),null);
});

test('event-only history search keeps its applied type through continuation and earlier results',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-search-event-type').selectOption('notice');const result={...record,eventType:'notice',searchMore:true,nextBefore:'9007199254740992',partial:true};await page.evaluate(value=>replies.push(value),result);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 const expected={botId:'12345678',type:'group',id:'10',archived:false,keyword:'',eventType:'notice'};assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),expected);assert.match(await page.locator('#agent-status').textContent(),/事件类型：通知/);
 await page.locator('#agent-search-event-type').selectOption('request');await page.locator('#agent-keyword').fill('draft');await page.evaluate(value=>replies.push(value),{...result,entries:[],searchMore:false});await page.locator('#agent-search-continue').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:result.nextBefore});assert.equal(await page.locator('#agent-search-event-type').inputValue(),'notice');assert.equal(await page.locator('#agent-keyword').inputValue(),'');
 await page.locator('#agent-search-before').fill('');await page.evaluate(value=>replies.push(value),result);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);await page.locator('#agent-search-event-type').selectOption('message');await page.evaluate(value=>replies.push(value),{...result,searchMore:false});await page.getByRole('button',{name:'查此条之前',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:record.entries[0].seq});
 await page.evaluate(value=>replies.push(value),record);await page.getByRole('button',{name:'查看较长原文',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.has('eventType'),false);
});

test('history event type can combine conditions clear to unrestricted and reset with the timeline',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-keyword').fill('needle');await page.locator('#agent-search-sender').fill('9007199254740993');await page.locator('#agent-search-from').fill('2026-10-15T08:00');
 for(const eventType of ['message','notice','request','']){await page.locator('#agent-search-event-type').selectOption(eventType);await page.evaluate(value=>replies.push(value),{...record,eventType});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);const body=await page.evaluate(()=>requests.at(-1).body);assert.equal(body.sender,'9007199254740993');assert.equal(body.from,'2026-10-15T00:00:00.000Z');assert.equal(body.eventType,eventType||undefined);assert.equal(body.keyword,'needle');}
 await page.locator('#agent-search-event-type').selectOption('notice');await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-history-refresh').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#agent-search-event-type').inputValue(),'');await page.locator('#agent-keyword').fill('');const count=await page.evaluate(()=>requests.length);await page.locator('#agent-search').click();assert.equal(await page.evaluate(()=>requests.length),count);
});

test('event type controls fit narrow themes and late searches cannot restore cleared scope',async t=>{
 const page=await fixture(t);assert.equal(await page.locator('#agent-search-event-type').isDisabled(),true);await load(page);await page.locator('#agent-search-event-type').selectOption('request');await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/archive-event-type-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.locator('#agent-search-event-type').scrollIntoViewIfNeeded();await page.screenshot({path:'.local/archive-event-type-ui/'+theme+'.png'});}
 await page.locator('#agent-search').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#agent-search-event-type').isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending.shift()({ok:true,json:async()=>value}),{...record,eventType:'request'});await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#agent-search-event-type').inputValue(),'');assert.equal(await page.locator('#agent-entries').textContent(),'');
});

async function search(page){await load(page);await page.locator('#agent-keyword').fill('中文');await page.evaluate(value=>replies.push(value),{...record,nextCursor:'',entries:[{...record.entries[0],text:'搜索片段',truncated:true}]});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);}

const sourceContext={...empty,anchorSeq:record.entries[0].seq,context:1,partial:true,entries:[
 {...record.entries[0],seq:'9007199254740995',text:'较新 <img src=x> 中文🙂',truncated:true},
 {...record.entries[0],text:'中心完整正文🙂'},
 {...record.entries[0],seq:'9007199254740991',text:'较早相邻事件'}
]};

test('source context radius selects one to three events per side and resets expanded content without fetching',async t=>{
 const page=await fixture(t);await search(page);const selector=page.locator('.agent-context-radius');assert.equal(await selector.inputValue(),'1');const count=await page.evaluate(()=>requests.length);await selector.selectOption('3');assert.equal(await page.evaluate(()=>requests.length),count);
 const center=record.entries[0],anchor=BigInt(center.seq),rows=[3n,2n,1n,0n,-1n,-2n,-3n].map(offset=>({...center,seq:String(anchor+offset),text:'事件 '+String(offset)+' <img src=x> 中文🙂'}));
 await page.locator('#agent-keyword').fill('draft');await page.locator('#agent-search-event-type').selectOption('notice');await page.evaluate(value=>{document.getElementById('agent-bot').value='87654321';document.getElementById('agent-id').value='88';document.getElementById('agent-type').value='private';document.getElementById('agent-archived').checked=true;replies.push(value)},{...sourceContext,context:3,entries:rows});
 await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(Object.fromEntries(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams),{botId:'12345678',type:'group',id:'10',archived:'false',seq:center.seq,context:'3'});assert.equal(await page.locator('#agent-entries h3').count(),7);assert.match(await page.locator('#agent-entries').textContent(),/前后各最多 3 条事件.*合计最多 16 KiB/s);assert.equal(await page.locator('#agent-entries img').count(),0);assert.equal(await page.locator('#agent-keyword').inputValue(),'draft');assert.equal(await page.locator('#agent-search-event-type').inputValue(),'notice');
 const loaded=await page.evaluate(()=>requests.length);await selector.selectOption('2');assert.equal(await page.evaluate(()=>requests.length),loaded);assert.equal(await page.locator('#agent-entries h3').count(),0);assert.equal(await page.getByRole('button',{name:'查看前后事件',exact:true}).getAttribute('aria-expanded'),'false');
 await page.evaluate(value=>replies.push(value),{...sourceContext,context:2,partial:false,entries:rows.slice(1,6)});await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.get('context'),'2');assert.equal(await page.locator('#agent-entries h3').count(),5);assert.match(await page.locator('#agent-entries').textContent(),/前后各最多 2 条事件/);
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/archive-context-radius-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await selector.scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/archive-context-radius-ui/'+theme+'.png'});}
 await page.getByRole('button',{name:'收起前后事件',exact:true}).click();assert.equal(await page.evaluate(()=>requests.length),loaded+1);await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('.agent-context-radius').inputValue(),'1');
});

test('source context radius checks the selected receipt and disables selection until a read settles',async t=>{
 const page=await fixture(t);await search(page);const selector=page.locator('.agent-context-radius');await selector.selectOption('2');const center=record.entries[0],anchor=BigInt(center.seq),rows=[3n,2n,1n,0n].map(offset=>({...center,seq:String(anchor+offset),text:'范围正文 '+String(offset)}));
 for(const value of [{...sourceContext,context:1},{...sourceContext,context:2,entries:rows},{...sourceContext,context:2,entries:[...rows,{...center,seq:String(anchor-1n)},{...center,seq:String(anchor-2n)}]}]){
  await page.evaluate(value=>replies.push(value),value);await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#agent-entries').textContent(),/回执与请求不符/);assert.equal(await page.locator('#agent-entries h3').count(),0);await page.getByRole('button',{name:'收起前后事件',exact:true}).click();
 }
 await selector.selectOption('3');await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>pending.length===1);assert.equal(await selector.isDisabled(),true);assert.equal(await page.getByRole('button',{name:'查看前后事件',exact:true}).isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),{...sourceContext,context:3,entries:[center]});await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('.agent-context-radius').count(),0);assert.equal(await page.locator('#agent-entries').textContent(),'');
});

test('time-only search submits local bounds and preserves them through continuation and older reads',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-search-from').fill('2026-10-15T08:00');await page.locator('#agent-search-until').fill('2026-10-15T16:00');
 await page.evaluate(value=>replies.push(value),{...record,nextCursor:'',searchMore:true,nextBefore:'9007199254740992'});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 const expected={botId:'12345678',type:'group',id:'10',archived:false,keyword:'',from:'2026-10-15T00:00:00.000Z',until:'2026-10-15T08:00:00.000Z'};
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),expected);assert.match(await page.locator('#agent-status').textContent(),/档案保存时间（UTC）/);
 await page.locator('#agent-keyword').fill('draft');await page.locator('#agent-search-sender').fill('123');await page.locator('#agent-search-from').fill('2026-10-16T08:00');await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-search-continue').click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:'9007199254740992'});assert.equal(await page.locator('#agent-search-sender').inputValue(),'');assert.equal(await page.locator('#agent-keyword').inputValue(),'');
 await page.evaluate(value=>replies.push(value),empty);await page.getByRole('button',{name:'查此条之前',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:record.entries[0].seq});
 await page.locator('#agent-search-before').fill('');await page.locator('#agent-search-from').fill('');await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',archived:false,keyword:'',until:expected.until});
 const count=await page.evaluate(()=>requests.length);await page.locator('#agent-search-until').fill('');await page.locator('#agent-search').click();assert.equal(await page.evaluate(()=>requests.length),count);assert.match(await page.locator('#agent-error').textContent(),/时间范围/);
});

test('sender-only search submits without a keyword and retains applied conditions when continuing',async t=>{
 const page=await fixture(t);await load(page);const count=await page.evaluate(()=>requests.length);
 await page.locator('#agent-keyword').fill('  ');await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 assert.equal(await page.evaluate(()=>requests.length),count);assert.match(await page.locator('#agent-error').textContent(),/关键词或指定发言者/);
 await page.locator('#agent-search-sender').fill('9007199254740993');await page.locator('#agent-search-from').fill('2026-10-15T08:00');
 await page.evaluate(value=>replies.push(value),{...record,nextCursor:'',searchMore:true,nextBefore:'9007199254740992'});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 const expected={botId:'12345678',type:'group',id:'10',archived:false,keyword:'',sender:'9007199254740993',from:'2026-10-15T00:00:00.000Z'};
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),expected);assert.equal(await page.locator('#agent-keyword').inputValue(),'');assert.match(await page.locator('#agent-status').textContent(),/发言者 QQ：9007199254740993/);
 await page.locator('#agent-keyword').fill('draft');await page.locator('#agent-search-sender').fill('123');await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-search-continue').click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:'9007199254740992'});
 await page.locator('#agent-keyword').fill('other draft');await page.evaluate(value=>replies.push(value),empty);await page.getByRole('button',{name:'查此条之前',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:'9007199254740993'});
});

test('sender search keeps exact applied author with time bounds through older and continued queries',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-keyword').fill('中文');await page.locator('#agent-search-sender').fill('9007199254740993');await page.locator('#agent-search-before').fill('9223372036854775807');await page.locator('#agent-search-from').fill('2026-10-15T08:00');
 await page.evaluate(value=>replies.push(value),{...record,nextCursor:'',searchMore:true,nextBefore:'9007199254740992'});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 const expected={botId:'12345678',type:'group',id:'10',archived:false,keyword:'中文',sender:'9007199254740993',before:'9223372036854775807',from:'2026-10-15T00:00:00.000Z'};assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),expected);assert.match(await page.locator('#agent-status').textContent(),/发言者 QQ：9007199254740993/);
 await page.locator('#agent-search-sender').fill('123');await page.locator('#agent-keyword').fill('draft');await page.evaluate(value=>{document.getElementById('agent-id').value='99';replies.push(value)},{...record,nextCursor:'',entries:[{...record.entries[0],seq:'9007199254740991'}],searchMore:true,nextBefore:'9007199254740990'});
 await page.getByRole('button',{name:'查此条之前',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:'9007199254740993'});assert.equal(await page.locator('#agent-search-sender').inputValue(),'9007199254740993');
 await page.locator('#agent-search-sender').fill('456');await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-search-continue').click();await page.waitForFunction(()=>!agentBusy);assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:'9007199254740990'});
});

test('sender input rejects invalid IDs without replacing valid search continuation and supports clearing',async t=>{
 const page=await fixture(t);assert.equal(await page.locator('#agent-search-sender').isDisabled(),true);await load(page);await page.locator('#agent-keyword').fill('中文');await page.locator('#agent-search-sender').fill('123');await page.evaluate(value=>replies.push(value),{...empty,partial:true,searchMore:true,nextBefore:'9007199254740993'});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 const count=await page.evaluate(()=>requests.length);for(const value of ['0','01','-1','1.5','9223372036854775808','<img src=x>']){await page.locator('#agent-search-sender').fill(value);await page.locator('#agent-search').click();assert.equal(await page.evaluate(()=>requests.length),count);assert.equal(await page.locator('#agent-search-continue').isEnabled(),true);assert.match(await page.locator('#agent-error').textContent(),/发言者 QQ/);}
 await page.locator('#agent-search-sender').fill('');await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.evaluate(()=>Object.hasOwn(requests.at(-1).body,'sender')),false);assert.doesNotMatch(await page.locator('#agent-status').textContent(),/发言者 QQ：/);
});

test('sender search does not constrain source context and clears failed or late requests',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-keyword').fill('中文');await page.locator('#agent-search-sender').fill('123');await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 await page.evaluate(value=>replies.push(value),sourceContext);await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.has('sender'),false);assert.match(await page.locator('#agent-entries').textContent(),/不沿用搜索关键词、发言者或时间/);
 await page.evaluate(()=>replies.push({error:'读取失败',status:503}));await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#agent-entries').textContent(),'');assert.match(await page.locator('#agent-status').textContent(),/未完成/);
 await page.locator('#agent-search').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#agent-search-sender').isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),record);await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#agent-search-sender').inputValue(),'');assert.equal(await page.locator('#agent-entries').textContent(),'');
});

test('sender controls clear with timeline scope and logout and fit narrow themes',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-keyword').fill('中文');await page.locator('#agent-search-sender').fill('9007199254740993');await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/agent-archive-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/agent-archive-ui/sender-'+theme+'.png',fullPage:true});}
 await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-history-refresh').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#agent-search-sender').inputValue(),'');await page.locator('#agent-search-sender').fill('123');await page.locator('#agent-type').selectOption('private');assert.equal(await page.locator('#agent-search-sender').inputValue(),'');assert.equal(await page.locator('#agent-search-sender').isDisabled(),true);
 await load(page);await page.locator('#agent-search-sender').fill('123');await page.evaluate(()=>{authenticated=false;csrf='';showAuth();});assert.equal(await page.locator('#agent-search-sender').inputValue(),'');
});

test('source context binds the loaded event and scope and preserves search conditions',async t=>{
 const page=await fixture(t);await search(page);
 await page.locator('#agent-keyword').fill('draft');await page.locator('#agent-search-before').fill('99');await page.locator('#agent-search-from').fill('2026-10-15T08:00');
 await page.evaluate(value=>{document.getElementById('agent-bot').value='87654321';document.getElementById('agent-id').value='88';document.getElementById('agent-type').value='private';document.getElementById('agent-archived').checked=true;replies.push(value)},sourceContext);
 await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 const url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.deepEqual(Object.fromEntries(url.searchParams),{botId:'12345678',type:'group',id:'10',archived:'false',seq:'9007199254740993',context:'1'});
 assert.match(await page.locator('#agent-entries').textContent(),/搜索片段.*相邻事件 #9007199254740995.*中心事件 #9007199254740993.*相邻事件 #9007199254740991.*不沿用搜索关键词、发言者或时间.*不完整/s);assert.equal(await page.locator('#agent-entries img').count(),0);
 assert.equal(await page.locator('#agent-keyword').inputValue(),'draft');assert.equal(await page.locator('#agent-search-before').inputValue(),'99');assert.match(await page.locator('#agent-status').textContent(),/搜索完成/);
 const count=await page.evaluate(()=>requests.length);await page.getByRole('button',{name:'收起前后事件',exact:true}).click();assert.equal(await page.evaluate(()=>requests.length),count);assert.doesNotMatch(await page.locator('#agent-entries').textContent(),/中心完整正文/);
 await page.evaluate(value=>replies.push(value),{...sourceContext,partial:false,entries:[sourceContext.entries[1]]});await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#agent-entries').textContent(),/没有可展示的相邻事件/);assert.equal(await page.evaluate(()=>requests.length),count+1);
});

test('source context rejects mismatched anchors ranges and event sequences',async t=>{
 const page=await fixture(t);await search(page);
 const center=sourceContext.entries[1];
 for(const value of [
  {...sourceContext,anchorSeq:'1'}, {...sourceContext,context:'1'}, {...sourceContext,context:2},
  {...sourceContext,entries:[sourceContext.entries[0]]}, {...sourceContext,entries:[center,center]},
  {...sourceContext,entries:[{...center,kind:'request'}]}, {...sourceContext,entries:[{...center,seq:9007199254740992}]},
  {...sourceContext,entries:[{...center,seq:'9223372036854775808'},center]},
  {...sourceContext,entries:[center,sourceContext.entries[0]]},
  {...sourceContext,entries:[sourceContext.entries[0],{...center,seq:'9007199254740994'},center]}
 ]){
  await page.evaluate(value=>replies.push(value),value);await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
  assert.match(await page.locator('#agent-entries').textContent(),/回执与请求不符/);assert.doesNotMatch(await page.locator('#agent-entries').textContent(),/中心完整正文|较新 <img/);await page.getByRole('button',{name:'收起前后事件',exact:true}).click();
 }
});

test('source context distinguishes missing unavailable failures and late responses',async t=>{
 const page=await fixture(t);await search(page);
 for(const [value,message] of [[{...empty,anchorSeq:record.entries[0].seq,context:1},/未找到中心事件/],[{...empty,available:false},/档案文件不可用/]]){
  await page.evaluate(value=>replies.push(value),value);await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#agent-entries').textContent(),message);await page.getByRole('button',{name:'收起前后事件',exact:true}).click();
 }
 const count=await page.evaluate(()=>requests.length);await page.evaluate(()=>replies.push({error:'档案不可读',status:503}));await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.evaluate(()=>requests.length),count+1);assert.match(await page.locator('#agent-entries').textContent(),/读取失败.*先前搜索片段/);assert.equal(await page.locator('#agent-error').isVisible(),true);
 await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.getByRole('button',{name:'查看较长原文',exact:true}).isDisabled(),true);
 await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),sourceContext);await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#agent-entries').textContent(),'');
});

test('source context is limited to current event results and clears on logout with narrow themes',async t=>{
 const page=await fixture(t);await search(page);await page.evaluate(value=>replies.push(value),sourceContext);await page.getByRole('button',{name:'查看前后事件',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/agent-archive-ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/agent-archive-ui/source-context-'+theme+'.png',fullPage:true});}
 await page.evaluate(value=>replies.push(value),{...record,entries:[{...record.entries[0],kind:'request'}]});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.getByRole('button',{name:'查看前后事件',exact:true}).count(),0);assert.doesNotMatch(await page.locator('#agent-entries').textContent(),/中心完整正文/);
 await page.locator('#agent-archived').check();await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-scope-form button').click();await page.waitForFunction(()=>!agentBusy);await page.locator('#agent-keyword').fill('中文');await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.getByRole('button',{name:'查看前后事件',exact:true}).count(),0);assert.equal(await page.getByRole('button',{name:'查看较长原文',exact:true}).count(),1);
 await page.evaluate(()=>{authenticated=false;csrf='';showAuth();});assert.equal(await page.locator('#agent-entries').textContent(),'');
});

async function continuationSearch(page,value={...empty,partial:true,scanned:5000,searchMore:true,nextBefore:'9007199254740993'}){
 await load(page);await page.locator('#agent-keyword').fill('中文');await page.evaluate(value=>replies.push(value),value);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
}

test('archive continuation uses actual empty-window position and preserves applied time and scope',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-keyword').fill('中文');await page.locator('#agent-search-before').fill('9223372036854775807');await page.locator('#agent-search-from').fill('2026-10-15T08:00');await page.locator('#agent-search-until').fill('2026-10-16T08:00');
 await page.evaluate(value=>replies.push(value),{...empty,scanned:5000,partial:true,searchMore:true,nextBefore:'9007199254740993'});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 assert.equal(await page.locator('#agent-entries').textContent(),'');assert.equal(await page.locator('#agent-search-continue').isVisible(),true);assert.equal(await page.locator('#agent-search-continue').isEnabled(),true);assert.match(await page.locator('#agent-status').textContent(),/9007199254740993 之前/);
 const count=await page.evaluate(()=>requests.length);await page.locator('#agent-keyword').fill('draft');await page.locator('#agent-search-before').fill('01');await page.locator('#agent-search-from').fill('2026-10-01T00:00');await page.locator('#agent-search-until').fill('2026-10-02T00:00');
 await page.evaluate(value=>{document.getElementById('agent-id').value='99';document.getElementById('agent-bot').value='87654321';document.getElementById('agent-archived').checked=true;replies.push(value)},{...record,nextCursor:'',searchMore:false});
 assert.equal(await page.evaluate(()=>requests.length),count);await page.locator('#agent-search-continue').click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:'12345678',type:'group',id:'10',archived:false,keyword:'中文',before:'9007199254740993',from:'2026-10-15T00:00:00.000Z',until:'2026-10-16T00:00:00.000Z'});
 assert.equal(await page.locator('#agent-keyword').inputValue(),'中文');assert.equal(await page.locator('#agent-search-before').inputValue(),'9007199254740993');assert.equal(await page.locator('#agent-search-from').inputValue(),'2026-10-15T08:00');assert.equal(await page.locator('#agent-search-continue').isHidden(),true);assert.equal(await page.evaluate(()=>requests.length),count+1);
});

test('archive continuation requires a safe advancing string boundary and preserves valid results after invalid drafts',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-keyword').fill('中文');await page.locator('#agent-search-before').fill('9007199254740994');
 for(const metadata of [
  {partial:true}, {partial:true,searchMore:false,nextBefore:'2'}, {searchMore:'true',nextBefore:'2'},
  ...['0','01','-1','9223372036854775808','9007199254740994','9007199254740995',9007199254740992,null,'<img src=x>'].map(nextBefore=>({searchMore:true,nextBefore}))
 ]){
  await page.evaluate(value=>replies.push(value),{...empty,...metadata});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#agent-search-continue').isHidden(),true);
 }
 await page.evaluate(value=>replies.push(value),{...empty,partial:true,searchMore:true,nextBefore:'9007199254740993'});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 const count=await page.evaluate(()=>requests.length);await page.locator('#agent-search-before').fill('01');await page.locator('#agent-search').click();assert.equal(await page.evaluate(()=>requests.length),count);assert.equal(await page.locator('#agent-search-continue').isEnabled(),true);
 await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-search-continue').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.evaluate(()=>requests.at(-1).body.before),'9007199254740993');assert.equal(await page.locator('#agent-entries img').count(),0);
});

test('archive continuation failures and late responses clear old actions without automatic retries',async t=>{
 const page=await fixture(t);await continuationSearch(page);await page.evaluate(()=>replies.push({error:'会话档案暂不可读取',status:503}));await page.locator('#agent-search-continue').click();await page.waitForFunction(()=>!agentBusy);
 assert.match(await page.locator('#agent-status').textContent(),/未完成/);assert.equal(await page.locator('#agent-search-continue').isHidden(),true);assert.equal(await page.locator('#agent-error').isVisible(),true);
 await page.evaluate(value=>replies.push(value),{...empty,searchMore:true,nextBefore:'8000000000000001'});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 await page.locator('#agent-search-continue').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#agent-search-continue').isHidden(),true);
 await page.locator('[data-view=overview]').click();const count=await page.evaluate(()=>requests.length);await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),{...record,searchMore:true,nextBefore:'7000000000000001'});await page.evaluate(()=>Promise.resolve());
 assert.equal(await page.locator('#agent-entries').textContent(),'');assert.equal(await page.locator('#agent-search-continue').isHidden(),true);assert.equal(await page.evaluate(()=>requests.length),count);
});

test('archive continuation clears on timeline scope and logout and fits narrow themes',async t=>{
 const page=await fixture(t);await continuationSearch(page);await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/agent-archive-ui',{recursive:true});
 for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/agent-archive-ui/continuation-'+theme+'.png',fullPage:true});}
 await page.evaluate(value=>replies.push(value),record);await page.locator('#agent-history-refresh').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#agent-search-continue').isHidden(),true);
 await page.evaluate(value=>replies.push(value),{...empty,searchMore:true,nextBefore:'9007199254740993'});await page.locator('#agent-keyword').fill('中文');await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);await page.locator('#agent-type').selectOption('private');assert.equal(await page.locator('#agent-search-continue').isHidden(),true);
 await continuationSearch(page);await page.evaluate(()=>{authenticated=false;csrf='';showAuth();});assert.equal(await page.locator('#agent-search-continue').isHidden(),true);assert.equal(await page.evaluate(()=>agentSearchContinue),null);
});

test('archive time search converts local time and keeps applied bounds for older results',async t=>{
 const page=await fixture(t);assert.equal(await page.locator('#agent-search-from').isDisabled(),true);await load(page);
 await page.locator('#agent-keyword').fill('中文');await page.locator('#agent-search-from').fill('2026-10-15T08:00');await page.locator('#agent-search-until').fill('2026-10-16T08:00');
 await page.evaluate(value=>replies.push(value),{...record,nextCursor:''});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 const expected={botId:'12345678',type:'group',id:'10',archived:false,keyword:'中文',from:'2026-10-15T00:00:00.000Z',until:'2026-10-16T00:00:00.000Z'};
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),expected);assert.match(await page.locator('#agent-status').textContent(),/UTC.*2026-10-15T00:00:00.000Z.*不含/);
 await page.locator('#agent-search-from').fill('2026-10-01T00:00');await page.locator('#agent-search-until').fill('2026-10-02T00:00');await page.locator('#agent-keyword').fill('draft');
 await page.evaluate(value=>{document.getElementById('agent-id').value='99';replies.push(value)},{...record,nextCursor:''});await page.getByRole('button',{name:'查此条之前',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{...expected,before:'9007199254740993'});assert.equal(await page.locator('#agent-search-from').inputValue(),'2026-10-15T08:00');assert.equal(await page.locator('#agent-search-until').inputValue(),'2026-10-16T08:00');
 await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-history-refresh').click();await page.waitForFunction(()=>!agentBusy);
 for(const id of ['agent-search-from','agent-search-until'])assert.equal(await page.locator('#'+id).inputValue(),'');assert.doesNotMatch(await page.evaluate(()=>requests.at(-1).url),/from=|until=/);
});

test('archive time search validates order and supports clearing either boundary',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-keyword').fill('中文');await page.locator('#agent-search-from').fill('2026-10-16T08:00');await page.locator('#agent-search-until').fill('2026-10-15T08:00');
 const count=await page.evaluate(()=>requests.length);await page.locator('#agent-search').click();assert.equal(await page.evaluate(()=>requests.length),count);assert.match(await page.locator('#agent-error').textContent(),/开始时间须早于/);
 await page.locator('#agent-search-from').fill('2026-10-15T08:00');await page.locator('#agent-search').click();assert.equal(await page.evaluate(()=>requests.length),count);
 for(const [from,until] of [['2026-10-15T08:00',''],['','2026-10-16T08:00'],['','']]){
  await page.locator('#agent-search-from').fill(from);await page.locator('#agent-search-until').fill(until);await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
  const body=await page.evaluate(()=>requests.at(-1).body);assert.equal(Object.hasOwn(body,'from'),!!from);assert.equal(Object.hasOwn(body,'until'),!!until);
 }
});

test('archive time controls clear on navigation and ignore late search responses',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-keyword').fill('中文');await page.locator('#agent-search-from').fill('2026-10-15T08:00');await page.locator('#agent-search').click();await page.waitForFunction(()=>pending.length===1);
 assert.equal(await page.locator('#agent-search-from').isDisabled(),true);await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),record);await page.evaluate(()=>Promise.resolve());
 assert.equal(await page.locator('#agent-search-from').inputValue(),'');assert.equal(await page.locator('#agent-search-until').inputValue(),'');assert.equal(await page.locator('#agent-entries').textContent(),'');
 await page.locator('[data-view=agent]').click();await page.waitForFunction(()=>!agentBusy);await load(page);await page.locator('#agent-search-from').fill('2026-10-15T08:00');await page.locator('#agent-type').selectOption('private');assert.equal(await page.locator('#agent-search-from').inputValue(),'');assert.equal(await page.locator('#agent-search-from').isDisabled(),true);
});

test('older search uses loaded keyword and scope, validates boundary and resets on navigation',async t=>{
 const page=await fixture(t);await search(page);
 await page.locator('#agent-keyword').fill('draft-keyword');await page.locator('#agent-search-before').fill('99');
 await page.evaluate(value=>{document.getElementById('agent-id').value='88';replies.push(value)},{...record,nextCursor:''});
 await page.getByRole('button',{name:'查此条之前',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 const request=await page.evaluate(()=>requests.at(-1));assert.deepEqual(request.body,{botId:'12345678',type:'group',id:'10',archived:false,keyword:'中文',before:'9007199254740993'});
 assert.match(await page.locator('#agent-status').textContent(),/9007199254740993 之前/);assert.equal(await page.locator('#agent-keyword').inputValue(),'中文');assert.equal(await page.locator('#agent-search-before').inputValue(),'9007199254740993');
 const count=await page.evaluate(()=>requests.length);await page.locator('#agent-search-before').fill('01');await page.locator('#agent-search').click();assert.equal(await page.evaluate(()=>requests.length),count);assert.match(await page.locator('#agent-error').textContent(),/有效的正整数/);
 await page.locator('#agent-search-before').fill('2');await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.evaluate(()=>requests.at(-1).body.before),'2');
 await page.locator('#agent-search-before').fill('');await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.evaluate(()=>Object.hasOwn(requests.at(-1).body,'before')),false);
 await page.locator('#agent-search-before').fill('9');await page.evaluate(value=>replies.push(value),empty);await page.locator('#agent-history-refresh').click();await page.waitForFunction(()=>!agentBusy);assert.equal(await page.locator('#agent-search-before').inputValue(),'');
 await page.locator('#agent-search-before').fill('8');await page.locator('#agent-type').selectOption('private');assert.equal(await page.locator('#agent-search-before').inputValue(),'');
});

test('search detail expands the scoped string sequence and keeps the search excerpt',async t=>{
 const page=await fixture(t);await search(page);
 await page.evaluate(value=>{document.getElementById('agent-id').value='99';document.getElementById('agent-archived').checked=true;replies.push(value)},{...record,entries:[{...record.entries[0],text:'较长原文 <img src=x> 中文🙂',truncated:true}]});
 await page.getByRole('button',{name:'查看较长原文',exact:true}).click();await page.waitForFunction(()=>!agentBusy);
 const url=new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture');assert.equal(url.searchParams.get('seq'),'9007199254740993');assert.equal(url.searchParams.get('id'),'10');assert.equal(url.searchParams.get('botId'),'12345678');assert.equal(url.searchParams.get('archived'),'false');assert.equal(url.searchParams.has('cursor'),false);
 assert.match(await page.locator('#agent-entries').textContent(),/搜索片段.*较长原文.*仍有截断/s);assert.equal(await page.locator('#agent-entries img').count(),0);assert.match(await page.locator('#agent-status').textContent(),/搜索完成/);
 await page.setViewportSize({width:320,height:800});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);}
 const count=await page.evaluate(()=>requests.length);await page.getByRole('button',{name:'收起原文',exact:true}).click();assert.equal(await page.evaluate(()=>requests.length),count);assert.doesNotMatch(await page.locator('#agent-entries').textContent(),/较长原文 <img/);assert.match(await page.locator('#agent-entries').textContent(),/搜索片段/);
});

test('search detail distinguishes missing and failed reads and discards late responses',async t=>{
 const page=await fixture(t);await search(page);
 await page.evaluate(value=>replies.push(value),empty);await page.getByRole('button',{name:'查看较长原文',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#agent-entries').textContent(),/未找到该记录/);
 await page.getByRole('button',{name:'收起原文',exact:true}).click();await page.evaluate(()=>replies.push({error:'不可读取',status:503}));await page.getByRole('button',{name:'查看较长原文',exact:true}).click();await page.waitForFunction(()=>!agentBusy);assert.match(await page.locator('#agent-entries').textContent(),/原文读取失败/);assert.doesNotMatch(await page.locator('#agent-entries').textContent(),/未找到该记录/);
 await page.getByRole('button',{name:'查看较长原文',exact:true}).click();await page.waitForFunction(()=>pending.length===1);await page.locator('[data-view=overview]').click();
 await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),record);await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#agent-entries').textContent(),'');
});

test('scope, inert Unicode content, snapshot cursor, POST search and fresh timeline',async t=>{
 const page=await fixture(t);await load(page);
 assert.match(await page.locator('#agent-scope').textContent(),/12345678.*群 10/);
 assert.match(await page.locator('#agent-entries').textContent(),/9007199254740993/);assert.equal(await page.locator('#agent-entries img').count(),0);
 await page.evaluate(empty=>replies.push(empty),empty);await page.locator('#agent-next').click();await page.waitForFunction(()=>!agentBusy);
 assert.match(await page.evaluate(()=>requests.at(-1).url),/cursor=signed-cursor/);
 await page.locator('#agent-keyword').fill('中文');await page.evaluate(value=>replies.push(value),{...record,partial:true,scanned:5000});await page.locator('#agent-search').click();await page.waitForFunction(()=>!agentBusy);
 const request=await page.evaluate(()=>requests.at(-1));assert.equal(request.url,'/api/agent/history/search');assert.equal(request.body.keyword,'中文');assert.equal(request.body.id,'10');assert.equal(request.body.type,'group');assert.match(await page.locator('#agent-status').textContent(),/部分内容/);assert.equal(await page.locator('#agent-next').isDisabled(),true);
 await page.evaluate(empty=>replies.push(empty),empty);await page.locator('#agent-history-refresh').click();await page.waitForFunction(()=>!agentBusy);assert.equal(new URL(await page.evaluate(()=>requests.at(-1).url),'http://fixture').searchParams.get('cursor'),'');
});

test('scope change clears private content and failures cannot display success',async t=>{
 const page=await fixture(t);await load(page);await page.locator('#agent-type').selectOption('private');
 assert.equal(await page.locator('#agent-entries').textContent(),'');assert.equal(await page.locator('#agent-search').isDisabled(),true);
 await page.evaluate(()=>replies.push({error:'会话档案暂不可读取',status:503}));await page.locator('#agent-scope-form button').click();await page.waitForFunction(()=>!agentBusy);
 assert.equal(await page.locator('#agent-error').isVisible(),true);assert.match(await page.locator('#agent-status').textContent(),/未完成/);assert.equal(await page.locator('#agent-next').isDisabled(),true);
 const request=await page.evaluate(()=>requests.at(-1));assert.match(request.url,/type=private/);
});

test('navigation, logout and late responses never repopulate old archive',async t=>{
 const page=await fixture(t);await page.locator('#agent-conversations button').first().click();await page.waitForFunction(()=>pending.length===1);
 await page.locator('[data-view=overview]').click();assert.equal(await page.locator('#agent-entries').textContent(),'');
 await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),record);await page.evaluate(()=>Promise.resolve());assert.equal(await page.locator('#agent-entries').textContent(),'');
 await page.locator('[data-view=agent]').click();await page.waitForFunction(()=>!agentBusy);await load(page);
 await page.evaluate(()=>{authenticated=false;csrf='';showAuth();});assert.equal(await page.locator('#agent-entries').textContent(),'');assert.equal(await page.locator('#agent-id').inputValue(),'');assert.equal(await page.locator('#agent-bot option').count(),0);
});

test('late old-session 401 does not terminate new session',async t=>{
 const page=await fixture(t);await page.locator('#agent-conversations button').first().click();await page.waitForFunction(()=>pending.length===1);
 await page.evaluate(()=>{authenticated=false;showAuth();authenticated=true;csrf='new-fixture';document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;showAgentArchive(true);});await page.waitForFunction(()=>!agentBusy);
 await load(page);await page.evaluate(()=>pending[0]({ok:false,status:401,json:async()=>({error:'expired'})}));await page.evaluate(()=>Promise.resolve());
 assert.equal(await page.evaluate(()=>authenticated),true);assert.match(await page.locator('#agent-entries').textContent(),/中文/);
});

test('320px light/dark layout wraps long payloads without active markup',async t=>{
 const page=await fixture(t);await page.setViewportSize({width:320,height:800});await load(page,{...record,entries:[{...record.entries[0],text:'长文本🙂'.repeat(400)}]});
 await fs.mkdir('.local/agent-archive-ui',{recursive:true});
 for(const theme of ['light','dark']) {await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1),true);await page.screenshot({path:'.local/agent-archive-ui/'+theme+'.png',fullPage:true});}
});
