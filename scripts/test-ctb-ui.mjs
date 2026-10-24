import assert from 'node:assert/strict';import fs from 'node:fs/promises';import path from 'node:path';import {createRequire} from 'node:module';import test,{after} from 'node:test';
const require=createRequire(import.meta.url);const {chromium}=require(path.join(process.env.USERPROFILE,'.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules','playwright'));const root='internal/localadmin/assets/';const html=(await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,'');const browser=await chromium.launch({channel:'chrome',headless:true});after(()=>browser.close());const state={revision:'a'.repeat(64),config:{enabled:true,executable:'D:\\tools\\CurseTheBeast.exe',outputDir:'D:\\downloads',timeoutMinutes:30,threads:4,noProxy:false,upload:false,groupAccess:'all',groupDelivery:'group',groupDailyLimit:5},busy:false,last:'已完成测试'};
async function fixture(t){const c=await browser.newContext();t.after(()=>c.close());await c.route('**/*',r=>r.abort());const p=await c.newPage();await p.setContent(html);for(const f of ['app.js','ctb.js'])await p.addScriptTag({content:await fs.readFile(root+f,'utf8')});await p.evaluate(v=>{authenticated=true;csrf='fixture';window.showOverview=()=>{};window.requests=[];window.replies=[v];window.pending=[];document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;window.fetch=async(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const v=replies.shift();return v?{ok:!v.error,status:v.status||200,json:async()=>v}:new Promise(resolve=>pending.push(resolve));};showWorkspaceView('ctb');},state);await p.waitForFunction(()=>!ctbBusy&&ctbState);return p;}
test('save fields, conflict keeps draft, explicit reload and session isolation',async t=>{const p=await fixture(t);await p.locator('#ctb-threads').fill('8');await p.locator('#ctb-upload').check();await p.locator('#ctb-group-limit').fill('7');await p.locator('#ctb-group-delivery').selectOption('private');await p.evaluate(()=>replies.push({error:'配置已变更',status:409}));await p.locator('#ctb-form button').click();await p.waitForFunction(()=>!ctbBusy);assert.equal(await p.locator('#ctb-threads').inputValue(),'8');const body=await p.evaluate(()=>requests[1].body);assert.equal(body.config.threads,8);assert.equal(body.config.upload,true);assert.equal(body.config.groupDailyLimit,7);assert.equal(body.config.groupDelivery,'private');assert.equal(body.config.groupAccess,'all');await p.evaluate(v=>replies.push(v),state);await p.locator('#ctb-reload').click();await p.waitForFunction(()=>!ctbBusy);assert.equal(await p.locator('#ctb-threads').inputValue(),'4');await p.locator('#ctb-form button').click();await p.waitForFunction(()=>pending.length===1);assert.equal(await p.locator('#ctb-reload').isDisabled(),true);await p.evaluate(()=>{authenticated=false;resetCTB();authenticated=true});await p.evaluate(v=>pending[0]({ok:true,status:200,json:async()=>v}),state);await p.waitForTimeout(30);assert.equal(await p.evaluate(()=>ctbState),null);assert.equal(await p.locator('#ctb-exe').isDisabled(),true)});
test('desktop/mobile themes wrap',async t=>{const p=await fixture(t);await p.addStyleTag({content:await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8')});await fs.mkdir('.local/ctb-ui',{recursive:true});for(const width of [1280,320]){await p.setViewportSize({width,height:900});for(const theme of ['light','dark']){await p.evaluate(v=>document.documentElement.dataset.theme=v,theme);assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await p.screenshot({path:`.local/ctb-ui/${width}-${theme}.png`,fullPage:true})}}});

async function draft(p){return p.locator('#ctb-form').evaluate(form=>Array.from(form.elements).filter(x=>x.matches('input,select')).map(x=>[x.id,x.value,x.checked]));}
async function resolveStatus(p,value,index=0){await p.evaluate(({value,index})=>pending[index]({ok:!value.error,status:value.status||200,json:async()=>value}),{value,index});}
test('status refresh preserves every draft field, revision and configuration error',async t=>{
 const p=await fixture(t);
 await p.locator('#ctb-exe').fill('D:\\draft.exe');await p.locator('#ctb-dir').fill('D:\\draft');
 await p.locator('#ctb-timeout').fill('40');await p.locator('#ctb-threads').fill('8');await p.locator('#ctb-group-limit').fill('9');
 await p.locator('#ctb-enabled').uncheck();await p.locator('#ctb-no-proxy').check();await p.locator('#ctb-upload').check();
 await p.locator('#ctb-group-access').selectOption('owner');await p.locator('#ctb-group-delivery').selectOption('private');
 const before=await draft(p),notice=await p.locator('#ctb-status').textContent();
 await p.evaluate(v=>{showError('ctb-error',new Error('保存冲突'));replies.push({...v,revision:'b'.repeat(64),busy:true,last:'正在执行 download'});},state);
 await p.locator('#ctb-task-refresh').click();await p.waitForFunction(()=>!ctbBusy);
 assert.deepEqual(await draft(p),before);assert.equal(await p.locator('#ctb-status').textContent(),notice);
 assert.equal(await p.evaluate(()=>ctbState.revision),state.revision);assert.match(await p.locator('#ctb-error').textContent(),/保存冲突/);
 assert.match(await p.locator('#ctb-last').textContent(),/执行中.*download/);
 assert.deepEqual(await p.evaluate(()=>requests[1]),{url:'/api/ctb',body:null});
 await p.evaluate(()=>replies.push({error:'配置已变更',status:409}));await p.locator('#ctb-save').click();await p.waitForFunction(()=>!ctbBusy);
 assert.equal(await p.evaluate(()=>requests[2].body.revision),state.revision);assert.deepEqual(await draft(p),before);
});
test('invalid draft does not block refresh; refresh and config requests are mutually exclusive',async t=>{
 const p=await fixture(t);await p.locator('#ctb-exe').fill('');await p.locator('#ctb-threads').fill('99');
 await p.locator('#ctb-task-refresh').click();await p.waitForFunction(()=>pending.length===1);
 await p.evaluate(()=>{refreshCTBStatus();loadCTB();loadCTB(true)});
 assert.equal(await p.evaluate(()=>requests.length),2);assert.equal(await p.locator('#ctb-save').isDisabled(),true);
 assert.equal(await p.locator('#ctb-reload').isDisabled(),true);assert.equal(await p.locator('#ctb-threads').isDisabled(),false);
 await p.locator('#ctb-threads').fill('7');await resolveStatus(p,{...state,last:'完成'});await p.waitForFunction(()=>!ctbBusy);
 assert.equal(await p.locator('#ctb-threads').inputValue(),'7');assert.equal(await p.locator('#ctb-exe').inputValue(),'');
 await p.locator('#ctb-reload').click();await p.waitForFunction(()=>pending.length===2);await p.evaluate(()=>refreshCTBStatus());
 assert.equal(await p.evaluate(()=>requests.length),3);await resolveStatus(p,state,1);await p.waitForFunction(()=>!ctbBusy);
 assert.equal(await p.locator('#ctb-exe').inputValue(),state.config.executable);
});
test('failed refresh retains old result and draft, retry renders empty and plain text states',async t=>{
 const p=await fixture(t);await p.locator('#ctb-threads').fill('6');const old=await p.locator('#ctb-last').textContent();
 await p.evaluate(()=>replies.push({error:'读取失败',status:500}));await p.locator('#ctb-task-refresh').click();await p.waitForFunction(()=>!ctbBusy);
 assert.equal(await p.locator('#ctb-last').textContent(),old);assert.match(await p.locator('#ctb-task-status').textContent(),/刷新失败.*上次/);
 for(const last of ['', '失败：<img src=x onerror=alert(1)>', '完成：download 135']){
  await p.evaluate(v=>replies.push(v),{...state,last});await p.locator('#ctb-task-refresh').focus();await p.keyboard.press('Enter');await p.waitForFunction(()=>!ctbBusy);
  assert.equal(await p.locator('#ctb-last img').count(),0);assert.match(await p.locator('#ctb-last').textContent(),/空闲/);
  assert.equal(await p.locator('#ctb-threads').inputValue(),'6');
 }
});
test('page departure invalidates old response and finally without overwriting new request',async t=>{
 const p=await fixture(t);await p.locator('#ctb-threads').fill('6');await p.locator('#ctb-task-refresh').click();await p.waitForFunction(()=>pending.length===1);
 await p.evaluate(()=>{showWorkspaceView('overview');showWorkspaceView('ctb')});
 assert.equal(await p.locator('#ctb-threads').inputValue(),'6');assert.equal(await p.evaluate(()=>requests.length),2);
 await p.locator('#ctb-task-refresh').click();await p.waitForFunction(()=>pending.length===2);
 await resolveStatus(p,{...state,last:'旧响应'});await p.evaluate(()=>new Promise(resolve=>setTimeout(resolve,0)));
 assert.equal(await p.evaluate(()=>ctbBusy),true);assert.equal(await p.locator('#ctb-task-refresh').isDisabled(),true);
 assert.doesNotMatch(await p.locator('#ctb-last').textContent(),/旧响应/);
 await resolveStatus(p,{...state,last:'新响应'},1);await p.waitForFunction(()=>!ctbBusy);assert.match(await p.locator('#ctb-last').textContent(),/新响应/);
});
test('logout discards pending status and prevents unauthenticated refresh',async t=>{
 const p=await fixture(t);await p.locator('#ctb-task-refresh').click();await p.waitForFunction(()=>pending.length===1);
 await p.evaluate(()=>{authenticated=false;resetCTB();refreshCTBStatus()});await resolveStatus(p,{...state,last:'旧会话'});
 await p.evaluate(()=>new Promise(resolve=>setTimeout(resolve,0)));
 assert.equal(await p.evaluate(()=>requests.length),2);assert.equal(await p.locator('#ctb-last').textContent(),'');assert.equal(await p.evaluate(()=>ctbState),null);
 assert.equal(await p.locator('#ctb-task-refresh').isDisabled(),true);
});

test('network rejection retains last result and enables retry',async t=>{
 const p=await fixture(t);const before=await p.locator('#ctb-last').textContent();
 await p.evaluate(()=>{window.originalFetch=fetch;window.fetch=async()=>{throw new Error('network unavailable')}});
 await p.locator('#ctb-task-refresh').click();await p.waitForFunction(()=>!ctbBusy);
 assert.equal(await p.locator('#ctb-last').textContent(),before);assert.match(await p.locator('#ctb-task-status').textContent(),/刷新失败.*上次/);
 assert.equal(await p.locator('#ctb-task-refresh').isDisabled(),false);
 await p.evaluate(v=>{window.fetch=originalFetch;replies.push(v)},state);await p.locator('#ctb-task-refresh').click();await p.waitForFunction(()=>!ctbBusy);
 assert.match(await p.locator('#ctb-task-status').textContent(),/已刷新/);
});
