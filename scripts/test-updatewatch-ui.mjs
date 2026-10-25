// Synthetic browser fixtures only: no model requests or QQ messages.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
import test,{after} from 'node:test';
const require=createRequire(import.meta.url);
const {chromium}=require(path.join(process.env.LOCALADMIN_NODE_MODULES||path.join(process.env.USERPROFILE,'.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules'),'playwright'));
const root='internal/localadmin/assets/';
const html=(await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,'');
const browser=await chromium.launch({channel:'chrome',headless:true});after(()=>browser.close());
const state={config:{enabled:true,intervalHours:24,ownerId:12345,botId:54321,modelRole:'Summary',curseForge:true,ftb:true,github:true,repositories:['FTBTeam/FTB-Evo-Companion'],announcements:[]},revision:'a'.repeat(64),owners:[12345,67890],bots:[54321],models:[{role:'Summary',name:'test-summary',ready:true}],lastCheck:'2026-10-18T12:00:00Z',pending:0,baseline:3,sourceErrors:{curseforge:'HTTP 403'}};
async function fixture(t,timezoneId="Asia/Shanghai"){
 const context=await browser.newContext({timezoneId});t.after(()=>context.close());await context.route('**/*',r=>r.abort());const page=await context.newPage();await page.setContent(html);
 for(const file of ['app.js','updatewatch.js'])await page.addScriptTag({content:await fs.readFile(root+file,'utf8')});
 await page.evaluate(value=>{authenticated=true;csrf='synthetic';window.showOverview=()=>{};window.requests=[];window.replies=[];window.pending=[];document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.getElementById('overview-view').hidden=true;document.getElementById('updatewatch-view').hidden=false;window.fetch=async(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const v=replies.shift();if(v)return {ok:!v.error,status:v.status||200,json:async()=>v};return new Promise(resolve=>pending.push(resolve));};renderUpdateWatch(value);renderUpdateWatchControls();showWorkspaceView("updatewatch");document.getElementById('uw-fields').disabled=false;},state);
 return page;
}
test('save all fields, preserve failed drafts, discard explicitly',async t=>{
 const p=await fixture(t);await p.locator('#uw-interval').fill('6');await p.locator('#uw-daily').fill('09:00');await p.selectOption('#uw-owner','67890');await p.selectOption('#uw-model','GPT');await p.locator('#uw-cf').uncheck();await p.locator('#uw-repos').fill('FTBTeam/FTB-Evo-Companion\nFTBTeam/example');await p.locator('#uw-announcements').fill('https://www.feed-the-beast.com/blog/example');
 await p.evaluate(()=>replies.push({error:'配置已被修改，请重新加载',status:409}));await p.locator('#uw-save').click();await p.waitForFunction(()=>!updateWatchBusy);
 const req=await p.evaluate(()=>requests[0]);assert.equal(req.body.config.intervalHours,6);assert.equal(req.body.config.dailyTime,"09:00");assert.equal(req.body.config.ownerId,67890);assert.equal(req.body.config.modelRole,'GPT');assert.equal(req.body.config.curseForge,false);assert.equal(req.body.config.repositories.length,2);assert.equal(req.body.config.announcements.length,1);assert.equal(await p.locator('#uw-interval').inputValue(),'6');assert.match(await p.locator('#uw-error').textContent(),/重新加载/);
 await p.evaluate(value=>replies.push(value),state);await p.locator('#uw-reload').click();await p.waitForFunction(()=>!updateWatchBusy);assert.equal(await p.locator('#uw-interval').inputValue(),'24');
 await p.evaluate(value=>replies.push(value),state);await p.locator('#uw-save').click();await p.waitForFunction(()=>!updateWatchBusy);assert.match(await p.locator('#uw-status').textContent(),/已保存/);
});
test('busy requests gated and logout invalidates delayed response',async t=>{
 const p=await fixture(t);await p.locator('#uw-save').click();await p.waitForFunction(()=>pending.length===1);assert.equal(await p.locator('#uw-reload').isDisabled(),true);await p.evaluate(()=>loadUpdateWatch(true));assert.equal(await p.evaluate(()=>requests.length),1);
 await p.evaluate(()=>{authenticated=false;resetUpdateWatch();authenticated=true;csrf='new-session';});await p.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),state);await p.waitForTimeout(25);assert.equal(await p.evaluate(()=>updateWatchState),null);assert.equal(await p.locator('#uw-interval').isDisabled(),true);assert.equal(await p.locator('#uw-reload').isEnabled(),true);
});
test('responsive light/dark layout',async t=>{
 const p=await fixture(t);await p.evaluate(value=>renderUpdateWatchRuntime({...value,lastError:'读取失败\nhttps://example.test/'+ 'x'.repeat(180),sourceErrors:{github:'第一行\n第二行',ftb:'https://example.test/'+ 'y'.repeat(180),['unknown-'+ 'z'.repeat(80)]:'未知来源错误'}}),state);await p.evaluate(prefix=>{for(const e of document.querySelectorAll('#'+prefix+'-form input, #'+prefix+'-form select, #'+prefix+'-form textarea')){if(e.type==='checkbox')e.checked=!e.checked;else if(e.tagName==='SELECT')e.selectedIndex=(e.selectedIndex+1)%e.options.length;else e.value=e.type==='time'?'03:21':'99';e.dispatchEvent(new Event('input',{bubbles:true}));}},'uw');await p.addStyleTag({content:await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8')});await fs.mkdir('.local/updatewatch-admin-ui',{recursive:true});
 for(const width of [1280,320]){await p.setViewportSize({width,height:900});for(const theme of ['light','dark']){await p.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await p.screenshot({path:`.local/updatewatch-admin-ui/${width}-${theme}.png`,fullPage:true});await p.locator("#uw-runtime").screenshot({path:`.local/updatewatch-admin-ui/runtime-${width}-${theme}.png`});}}
 assert.equal(await p.locator('#uw-save').isEnabled(),true);
});

async function draft(p){return p.locator('#uw-form').evaluate(f=>Array.from(f.elements).filter(x=>x.matches('input,select,textarea,datalist')).map(x=>[x.id,x.value,x.checked,x.matches('select,datalist')?x.innerHTML:null]));}
async function settle(p,value,index=0){await p.evaluate(({value,index})=>pending[index]({ok:!value.error,status:value.status||200,json:async()=>value}),{value,index});}
test('runtime refresh preserves all drafts, options, revision and config errors',async t=>{
 const p=await fixture(t);await p.locator('#uw-enabled').uncheck();await p.locator('#uw-interval').fill('6');await p.locator('#uw-daily').fill('08:30');await p.locator('#uw-model').selectOption('GPT');await p.locator('#uw-owner').selectOption('67890');await p.locator('#uw-bot').fill('98765');
 for(const id of ['uw-cf','uw-ftb','uw-github'])await p.locator('#'+id).uncheck();await p.locator('#uw-repos').fill(' a/b \n c/d ');await p.locator('#uw-announcements').fill(' https://example.test/a \n');
 const before=await draft(p),notice=await p.locator('#uw-status').textContent();
 await p.evaluate(v=>{showError('uw-error',new Error('配置冲突'));replies.push(v)}, {...state,revision:'b'.repeat(64),config:{},models:[],owners:[],bots:[],pending:2,baseline:4});
 await p.locator('#uw-runtime-refresh').click();await p.waitForFunction(()=>!updateWatchBusy);
 assert.deepEqual(await draft(p),before);assert.equal(await p.locator('#uw-status').textContent(),notice);assert.match(await p.locator('#uw-error').textContent(),/配置冲突/);assert.equal(await p.evaluate(()=>updateWatchState.revision),state.revision);
 assert.deepEqual(await p.evaluate(()=>requests[0]),{url:'/api/updatewatch',body:null});assert.equal(await p.locator('#uw-baseline-count').textContent(),'4个');assert.equal(await p.locator('#uw-pending-count').textContent(),'2条');
 await p.evaluate(()=>replies.push({error:'旧修订冲突',status:409}));await p.locator('#uw-save').click();await p.waitForFunction(()=>!updateWatchBusy);
 assert.equal(await p.evaluate(()=>requests[1].body.revision),state.revision);assert.deepEqual(await draft(p),before);
});
test('invalid form can refresh, requests serialize while inputs remain editable',async t=>{
 const p=await fixture(t);await p.locator('#uw-bot').fill('');await p.locator('#uw-interval').fill('999');
 await p.locator('#uw-runtime-refresh').click();await p.waitForFunction(()=>pending.length===1);await p.evaluate(()=>{refreshUpdateWatchRuntime();loadUpdateWatch();loadUpdateWatch(true)});
 assert.equal(await p.evaluate(()=>requests.length),1);assert.equal(await p.locator('#uw-save').isDisabled(),true);assert.equal(await p.locator('#uw-reload').isDisabled(),true);assert.equal(await p.locator('#uw-interval').isEnabled(),true);
 await p.locator('#uw-interval').fill('7');await settle(p,state);await p.waitForFunction(()=>!updateWatchBusy);assert.equal(await p.locator('#uw-interval').inputValue(),'7');assert.equal(await p.locator('#uw-bot').inputValue(),'');
 await p.locator('#uw-reload').click();await p.waitForFunction(()=>pending.length===2);await p.evaluate(()=>refreshUpdateWatchRuntime());assert.equal(await p.evaluate(()=>requests.length),2);
 await settle(p,state,1);await p.waitForFunction(()=>!updateWatchBusy);assert.equal(await p.locator('#uw-interval').inputValue(),'24');
});
test('busy and read failures retain old state, retry clears source errors and renders text safely',async t=>{
 const p=await fixture(t);const old=await p.locator('#uw-runtime').textContent();await p.locator('#uw-interval').fill('8');
 for(const error of ['正在检查来源或模型，请稍后重试','监控状态读取失败']){
  await p.evaluate(error=>replies.push({error,status:409}),error);await p.locator('#uw-runtime-refresh').click();await p.waitForFunction(()=>!updateWatchBusy);
  assert.equal(await p.locator('#uw-runtime').textContent(),old);assert.match(await p.locator('#uw-runtime-status').textContent(),/未完成.*上次/);assert.equal(await p.locator('#uw-interval').inputValue(),'8');
 }
 await p.evaluate(v=>replies.push(v),{...state,lastCheck:'0001-01-01T00:00:00Z',sourceErrors:{},lastError:'',pending:0});await p.locator('#uw-runtime-refresh').focus();await p.keyboard.press('Enter');await p.waitForFunction(()=>!updateWatchBusy);
 assert.match(await p.locator('#uw-runtime').textContent(),/尚未检查/);assert.doesNotMatch(await p.locator('#uw-runtime').textContent(),/HTTP 403/);
 await p.evaluate(v=>replies.push(v),{...state,lastError:'<img src=x onerror=alert(1)>',sourceErrors:{ftb:'<b>error</b>'}});await p.locator('#uw-runtime-refresh').click();await p.waitForFunction(()=>!updateWatchBusy);assert.equal(await p.locator('#uw-runtime img, #uw-runtime b').count(),0);
});
test('network rejection leaves state and allows manual retry',async t=>{
 const p=await fixture(t);const old=await p.locator('#uw-runtime').textContent();await p.evaluate(()=>{window.savedFetch=fetch;window.fetch=async()=>{throw new Error('network unavailable')}});
 await p.locator('#uw-runtime-refresh').click();await p.waitForFunction(()=>!updateWatchBusy);assert.equal(await p.locator('#uw-runtime').textContent(),old);assert.match(await p.locator('#uw-runtime-status').textContent(),/未完成.*上次/);
 await p.evaluate(v=>{window.fetch=savedFetch;replies.push(v)},state);await p.locator('#uw-runtime-refresh').click();await p.waitForFunction(()=>!updateWatchBusy);assert.match(await p.locator('#uw-runtime-status').textContent(),/已刷新/);
});
test('leaving invalidates old response and finally while preserving new request and draft',async t=>{
 const p=await fixture(t);await p.locator('#uw-interval').fill('8');
 await p.evaluate(()=>{window.oldRefresh=refreshUpdateWatchRuntime()});await p.waitForFunction(()=>pending.length===1);
 await p.evaluate(()=>{showWorkspaceView('overview');showWorkspaceView('updatewatch')});assert.equal(await p.evaluate(()=>requests.length),1);assert.equal(await p.locator('#uw-interval').inputValue(),'8');
 await p.locator('#uw-runtime-refresh').click();await p.waitForFunction(()=>pending.length===2);await settle(p,{...state,lastError:'旧响应'});await p.evaluate(()=>oldRefresh);
 assert.equal(await p.evaluate(()=>updateWatchBusy),true);assert.equal(await p.locator('#uw-runtime-refresh').isDisabled(),true);assert.doesNotMatch(await p.locator('#uw-runtime').textContent(),/旧响应/);
 await settle(p,{...state,lastError:'新响应'},1);await p.waitForFunction(()=>!updateWatchBusy);assert.match(await p.locator('#uw-runtime').textContent(),/新响应/);
});
test('logout resets runtime and late response cannot restore previous session',async t=>{
 const p=await fixture(t);await p.evaluate(()=>{window.oldRefresh=refreshUpdateWatchRuntime()});await p.waitForFunction(()=>pending.length===1);
 await p.evaluate(()=>{authenticated=false;resetUpdateWatch();refreshUpdateWatchRuntime();authenticated=true});await settle(p,{...state,lastError:'旧会话'});await p.evaluate(()=>oldRefresh);
 assert.equal(await p.evaluate(()=>requests.length),1);assert.equal(await p.evaluate(()=>updateWatchState),null);assert.equal(await p.locator('#uw-runtime').textContent(),'尚未读取。');assert.equal(await p.locator('#uw-runtime-refresh').isDisabled(),true);
});

// R18: observable draft behavior with synthetic APIs only.
test('R18 every field changes and reverts, labels only, fixed order',async t=>{
 const p=await fixture(t);assert.match(await p.locator('#uw-draft').textContent(),/无未保存/);
 assert.equal(await p.locator('#uw-undo').isDisabled(),true);
 const fields=["uw-enabled","uw-daily","uw-interval","uw-model","uw-owner","uw-bot","uw-cf","uw-ftb","uw-github","uw-repos","uw-announcements"];
 const original=await p.evaluate(ids=>ids.map(id=>{const e=document.getElementById(id);return e.type==='checkbox'?e.checked:e.value}),fields);
 for(let i=0;i<fields.length;i++){
  await p.evaluate(id=>{const e=document.getElementById(id);if(e.type==='checkbox')e.checked=!e.checked;else if(e.tagName==='SELECT')e.selectedIndex=(e.selectedIndex+1)%e.options.length;else e.value=e.type==='number'?'99':e.type==='time'?'03:21':' private-value <b>secret</b> ';e.dispatchEvent(new Event('input',{bubbles:true}));},fields[i]);
  assert.match(await p.locator('#uw-draft').textContent(),new RegExp('：'+(i+1)+'项'));
 }
 const summary=await p.locator('#uw-draft').textContent();assert.doesNotMatch(summary,/private-value|secret|99/);assert.equal(await p.locator('#uw-draft b').count(),0);
 for(let i=fields.length-1;i>=0;i--){
  await p.evaluate(({id,value})=>{const e=document.getElementById(id);if(e.type==='checkbox')e.checked=value;else e.value=value;e.dispatchEvent(new Event('change',{bubbles:true}));},{id:fields[i],value:original[i]});
  assert.match(await p.locator('#uw-draft').textContent(),i?new RegExp('：'+i+'项'):/无未保存/);
 }
 assert.equal(await p.locator('#uw-undo').isDisabled(),true);
});
test('R18 local undo after refreshed revision keeps runtime and conflict with zero requests',async t=>{
 const p=await fixture(t);const original=await draft(p);
 await p.locator('#uw-interval').fill('7');
 await p.evaluate(value=>replies.push({...value,revision:'b'.repeat(64),last:'新结果',pending:7}),state);
 await p.evaluate(()=>refreshUpdateWatchRuntime());
 const runtime=await p.locator('#uw-runtime').textContent();
 await p.evaluate(()=>showError('uw-error',new Error('旧修订冲突')));
 const count=await p.evaluate(()=>requests.length);
 await p.locator('#uw-undo').focus();await p.keyboard.press('Enter');
 assert.deepEqual(await draft(p),original);assert.equal(await p.evaluate(()=>requests.length),count);
 assert.equal(await p.locator('#uw-runtime').textContent(),runtime);assert.match(await p.locator('#uw-error').textContent(),/旧修订/);
 await p.evaluate(()=>replies.push({error:'仍需重新读取',status:409}));await p.locator('#uw-save').click();await p.waitForFunction(()=>!updateWatchBusy);
 assert.equal(await p.evaluate(()=>requests.at(-1).body.revision),state.revision);assert.match(await p.locator('#uw-error').textContent(),/重新读取/);
});
test('R18 baseline lifecycle preserves failed reads and saves, accepts server values',async t=>{
 const p=await fixture(t);const original=await draft(p);await p.locator('#uw-interval').fill('7');
 for(const action of ['reload','save']){await p.evaluate(()=>replies.push({error:'服务忙碌',status:409}));await p.locator('#uw-'+action).click();await p.waitForFunction(()=>!updateWatchBusy);assert.match(await p.locator('#uw-draft').textContent(),/1项/);}
 await p.evaluate(async()=>{const saved=fetch;window.fetch=async()=>{throw new Error('offline')};try{await loadUpdateWatch()}finally{window.fetch=saved}});assert.match(await p.locator('#uw-draft').textContent(),/1项/);
 await p.locator('#uw-undo').click();assert.deepEqual(await draft(p),original);
 const updated=structuredClone(state);updated.config.intervalHours=9;
 await p.locator('#uw-interval').fill('8');await p.evaluate(value=>replies.push(value),updated);await p.locator('#uw-save').click();await p.waitForFunction(()=>!updateWatchBusy);
 assert.equal(await p.locator('#uw-interval').inputValue(),'9');assert.match(await p.locator('#uw-draft').textContent(),/无未保存/);
 await p.locator('#uw-interval').fill('');await p.locator('#uw-undo').click();assert.equal(await p.locator('#uw-interval').inputValue(),'9');
 await p.evaluate(value=>replies.push(value),state);await p.locator('#uw-reload').click();await p.waitForFunction(()=>!updateWatchBusy);assert.deepEqual(await draft(p),original);
});
test('R18 in-flight edits, direct undo guard and reset reject late config',async t=>{
 const p=await fixture(t);await p.locator('#uw-interval').fill('7');await p.evaluate(()=>{window.refreshPromise=refreshUpdateWatchRuntime()});await p.waitForFunction(()=>pending.length===1);
 await p.locator('#uw-interval').fill('8');await p.evaluate(()=>undoUpdateWatchDraft());assert.equal(await p.locator('#uw-interval').inputValue(),'8');assert.equal(await p.locator('#uw-undo').isDisabled(),true);assert.match(await p.locator('#uw-draft').textContent(),/1项/);
 await p.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),state);await p.evaluate(()=>refreshPromise);assert.equal(await p.locator('#uw-undo').isEnabled(),true);
 await p.evaluate(()=>{window.readPromise=loadUpdateWatch()});await p.waitForFunction(()=>pending.length===2);await p.evaluate(()=>undoUpdateWatchDraft());assert.equal(await p.locator('#uw-interval').inputValue(),'8');
 await p.evaluate(()=>{authenticated=false;resetUpdateWatch();undoUpdateWatchDraft();authenticated=true});await p.evaluate(value=>pending[1]({ok:true,status:200,json:async()=>value}),state);await p.evaluate(()=>readPromise);
 assert.equal(await p.evaluate(()=>updateWatchBaseline),null);assert.match(await p.locator('#uw-draft').textContent(),/读取配置后/);assert.equal(await p.locator('#uw-undo').isDisabled(),true);
 await p.evaluate(()=>replies.push({error:'读取失败',status:500}));await p.evaluate(()=>loadUpdateWatch());assert.equal(await p.evaluate(()=>updateWatchBaseline),null);
});
test('R18 defaults and raw whitespace use rendered baseline; pages remain independent',async t=>{
 const p=await fixture(t);
 await p.evaluate(value=>renderUpdateWatch({...value,config:{...value.config,botId:0}}),state);
 assert.equal(await p.locator('#uw-bot').inputValue(),'54321');assert.match(await p.locator('#uw-draft').textContent(),/无未保存/);
 await p.locator('#uw-repos').fill(' FTBTeam/FTB-Evo-Companion \n');await p.locator('#uw-announcements').fill(' \n');
 assert.match(await p.locator('#uw-draft').textContent(),/2项 · 关联仓库、公告链接/);
 const uwDraft=await draft(p),summary=await p.locator('#uw-draft').textContent();
 await p.addScriptTag({content:await fs.readFile(root+'ctb.js','utf8')});
 await p.evaluate(()=>{replies.push({revision:'c'.repeat(64),config:{enabled:true,executable:'tool.exe',outputDir:'output',timeoutMinutes:30,threads:4,noProxy:false,upload:false,groupDailyLimit:0},busy:false,last:'idle'});showWorkspaceView('ctb')});await p.waitForFunction(()=>ctbState&&!ctbBusy);
 assert.equal(await p.locator('#ctb-group-limit').inputValue(),'5');assert.match(await p.locator('#ctb-draft').textContent(),/无未保存/);
 await p.locator('#ctb-exe').fill(' tool.exe ');assert.match(await p.locator('#ctb-draft').textContent(),/1项 · 程序路径/);
 const count=await p.evaluate(()=>requests.length);await p.locator('#ctb-undo').click();assert.equal(await p.evaluate(()=>requests.length),count);
 assert.deepEqual(await draft(p),uwDraft);assert.equal(await p.locator('#uw-draft').textContent(),summary);
 await p.locator('#ctb-threads').fill('7');await p.evaluate(()=>showWorkspaceView('updatewatch'));await p.locator('#uw-undo').click();assert.equal(await p.locator('#ctb-threads').inputValue(),'7');assert.match(await p.locator('#ctb-draft').textContent(),/1项/);
 assert.equal(await p.locator('#uw-repos').inputValue(),state.config.repositories[0]);assert.equal(await p.locator('#uw-announcements').inputValue(),'');assert.equal(await p.evaluate(()=>requests.length),count);
});

test('R19 unread, zero and reset states never retain old runtime details',async t=>{
 const p=await fixture(t);
 await p.evaluate(()=>resetUpdateWatch());
 assert.equal(await p.locator('#uw-runtime').textContent(),'尚未读取。');
 assert.equal(await p.locator('#uw-runtime dl, #uw-runtime li').count(),0);
 await p.evaluate(()=>replies.push({error:'读取失败',status:500}));await p.evaluate(()=>loadUpdateWatch());
 assert.equal(await p.locator('#uw-runtime').textContent(),'尚未读取。');
 for(const sourceErrors of [null,{}]){
  await p.evaluate(v=>replies.push(v),{...state,lastCheck:'0001-01-01T00:00:00Z',baseline:0,pending:0,lastError:'',sourceErrors});await p.evaluate(()=>loadUpdateWatch());
  assert.equal(await p.locator('#uw-last-check').textContent(),'尚未检查');assert.equal(await p.locator('#uw-baseline-count').textContent(),'0个');assert.equal(await p.locator('#uw-pending-count').textContent(),'0条');assert.equal(await p.locator('#uw-last-error').textContent(),'未记录最近错误');
  assert.match(await p.locator('#uw-runtime').textContent(),/未记录来源错误/);assert.doesNotMatch(await p.locator('#uw-runtime').textContent(),/全部正常|全部来源成功/);
 }
 await p.evaluate(()=>{authenticated=false;resetUpdateWatch()});assert.equal(await p.locator('#uw-runtime').textContent(),'尚未读取。');
 await p.evaluate(v=>{authenticated=true;replies.push(v)},state);await p.evaluate(()=>loadUpdateWatch());assert.equal(await p.locator('#uw-source-errors li').count(),1);
});

test('R19 labels, browser timezone and snapshot counts have explicit semantics',async t=>{
 for(const [zone,time] of [['UTC','2026/10/18 12:00:00'],['Asia/Shanghai','2026/10/18 20:00:00']]){
  const p=await fixture(t,zone);
  assert.equal(await p.locator('#uw-last-check').textContent(),time);
  assert.deepEqual(await p.locator('#uw-runtime dt').allTextContents(),['最近检查（浏览器本地时间）','已初始化来源数','待发通知']);
  assert.equal(await p.locator('#uw-runtime dd').count(),3);
  await p.evaluate(v=>renderUpdateWatchRuntime({...v,baseline:9,pending:2}),state);
  assert.equal(await p.locator('#uw-baseline-count').textContent(),'9个');assert.equal(await p.locator('#uw-pending-count').textContent(),'2条');
  assert.equal(await p.locator('#uw-runtime [role="status"], #uw-runtime [aria-live]').count(),0);assert.equal(await p.locator('#uw-runtime-status').getAttribute('role'),'status');
  assert.equal(await p.evaluate(()=>requests.length),0);
 }
});

test('R19 source errors sort and replace without mutating data or interpreting markup',async t=>{
 const p=await fixture(t);const errors={zeta:'末项',github:'第一行\n第二行',alpha:'https://example.test/'+ 'x'.repeat(180),'<img src=x onerror=alert(1)>':'<script>alert(1)</script>'};
 const expected=['<img src=x onerror=alert(1)>','alpha','github','zeta'];
 for(const sourceErrors of [errors,Object.fromEntries(Object.entries(errors).reverse())]){
  const before=JSON.stringify(sourceErrors);
  const after=await p.evaluate(v=>{renderUpdateWatchRuntime({...v,lastError:'<b>最近错误</b>\nnext'});return JSON.stringify(v.sourceErrors)}, {...state,sourceErrors});
  assert.equal(after,before);assert.deepEqual(await p.locator('#uw-source-errors li strong').allTextContents(),expected);
  assert.deepEqual(await p.locator('#uw-source-errors li p').allTextContents(),expected.map(k=>errors[k]));
  assert.equal(await p.locator('#uw-last-error').textContent(),'<b>最近错误</b>\nnext');assert.equal(await p.locator('#uw-runtime img, #uw-runtime script, #uw-runtime b, #uw-runtime a').count(),0);
 }
 await p.evaluate(v=>replies.push({...v,sourceErrors:{ftb:'仅余此项'}}),state);await p.locator('#uw-runtime-refresh').focus();await p.keyboard.press('Enter');await p.waitForFunction(()=>!updateWatchBusy);
 assert.equal(await p.locator('#uw-source-errors li').count(),1);assert.equal(await p.locator('#uw-source-errors strong').textContent(),'ftb');
 await p.evaluate(v=>replies.push({...v,lastError:'',sourceErrors:{}}),state);await p.evaluate(()=>refreshUpdateWatchRuntime());assert.equal(await p.locator('#uw-source-errors li').count(),0);assert.doesNotMatch(await p.locator('#uw-runtime').textContent(),/仅余此项|末项/);
});

test('R19 full configuration success replaces details; failures preserve whole subtree',async t=>{
 const p=await fixture(t);let count=0;
 for(const save of [false,true]){
  const value={...state,baseline:++count,pending:count+2,lastError:'本轮'+count,sourceErrors:{['source'+count]:'错误'+count}};
  await p.evaluate(v=>replies.push(v),value);await p.evaluate(save=>loadUpdateWatch(save),save);
  assert.equal(await p.locator('#uw-baseline-count').textContent(),count+'个');assert.equal(await p.locator('#uw-pending-count').textContent(),(count+2)+'条');assert.equal(await p.locator('#uw-last-error').textContent(),'本轮'+count);assert.equal(await p.locator('#uw-source-errors li').count(),1);
  const before=await p.locator('#uw-runtime').innerHTML();
  await p.evaluate(()=>replies.push({error:'冲突',status:409}));await p.evaluate(save=>loadUpdateWatch(save),save);assert.equal(await p.locator('#uw-runtime').innerHTML(),before);
  await p.evaluate(async()=>{const saved=fetch;window.fetch=async()=>{throw new Error('offline')};try{await loadUpdateWatch()}finally{window.fetch=saved}});assert.equal(await p.locator('#uw-runtime').innerHTML(),before);
 }
 assert.deepEqual(await p.locator('#uw-source-errors strong').allTextContents(),['source2']);
});
