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
async function fixture(t){
 const context=await browser.newContext();t.after(()=>context.close());await context.route('**/*',r=>r.abort());const page=await context.newPage();await page.setContent(html);
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
 const p=await fixture(t);await p.addStyleTag({content:await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8')});await fs.mkdir('.local/updatewatch-admin-ui',{recursive:true});
 for(const width of [1280,320]){await p.setViewportSize({width,height:900});for(const theme of ['light','dark']){await p.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await p.screenshot({path:`.local/updatewatch-admin-ui/${width}-${theme}.png`,fullPage:true});}}
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
 assert.deepEqual(await p.evaluate(()=>requests[0]),{url:'/api/updatewatch',body:null});assert.match(await p.locator('#uw-runtime').textContent(),/基线来源：4；待通知：2/);
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
