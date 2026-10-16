// Offline Chrome; all identities, credentials and HTTP responses are synthetic.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
import test,{after} from 'node:test';
const require=createRequire(import.meta.url);
const modules=process.env.LOCALADMIN_NODE_MODULES||path.join(process.env.USERPROFILE,'.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules');
const {chromium}=require(path.join(modules,'playwright'));
const root='internal/localadmin/assets/';
const html=(await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,'');
const scripts=await Promise.all(['app.js','ai.js','ai-search.js','agent-tasks.js'].map(name=>fs.readFile(root+name,'utf8')));
const browser=await chromium.launch({channel:'chrome',headless:true});after(()=>browser.close());
const settings={botId:'12345678',type:'group',id:'10',revision:'a'.repeat(64),enabled:false,keyConfigured:false,policy:{enabled:false,roles:['owner']}};
const cfg={available:true,global:{revision:'a'.repeat(64),ready:true,fields:{Type:0,ModelName:'saved',API:'https://example.invalid',MaxN:4096,TopP:.9,SystemP:'saved prompt',NoSystemP:false,ReasoningEffort:'',Separator:'',ImageType:0,ImageModelName:'',ImageAPI:'',AgentType:0,AgentModelName:'saved',AgentAPI:'https://example.invalid',AgentChar:'persona',AgentSex:'女'},keys:{Key:true,ImageKey:false,AgentKey:true},addressRedacted:{}},groups:[]};
async function fixture(t){
 const context=await browser.newContext();t.after(()=>context.close());await context.route('**/*',r=>r.abort());const page=await context.newPage();await page.setContent(html);for(const content of scripts)await page.addScriptTag({content});
 await page.evaluate(({cfg,settings})=>{authenticated=true;csrf='search-fixture';aiState=cfg;window.showOverview=()=>{};window.requests=[];window.replies=[];window.pending=[];document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.getElementById('overview-view').hidden=true;document.getElementById('ai-view').hidden=false;
 window.fetch=async(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const value=replies.shift();if(value)return {ok:!value.error,status:value.status||200,json:async()=>value};return new Promise(resolve=>pending.push(resolve));};renderAIGlobal(cfg);renderAISearch(settings);aiBusyState(false);},{cfg,settings});return page;
}
const idle=page=>page.waitForFunction(()=>!aiBusy);
async function reply(page,value){await page.evaluate(value=>replies.push(value),value);}

test('search defaults off and saving uses the frozen bot conversation with only selected roles',async t=>{
 const page=await fixture(t);assert.equal(await page.evaluate(()=>requests.length),0);assert.equal(await page.locator('#ai-field-search-enabled').isChecked(),false);assert.equal(await page.locator('#ai-field-search-role-owner').isChecked(),true);assert.equal(await page.locator('#ai-field-search-role-user').isChecked(),false);
 await page.locator('#ai-search-bot').fill('87654321');await page.locator('#ai-search-id').fill('20');await page.selectOption('#ai-search-type','private');
 await page.locator('#ai-field-search-enabled').check();await page.locator('#ai-field-search-scope-enabled').check();await page.selectOption('#ai-search-key-action','replace');await page.locator('#ai-search-key').fill('SYNTHETIC_SEARCH_KEY');
 await reply(page,{...settings,enabled:true,keyConfigured:true,policy:{enabled:true,roles:['owner']},revision:'b'.repeat(64)});await page.locator('#ai-search-form button[type=submit]').click();await idle(page);
 assert.deepEqual(await page.evaluate(()=>requests),[{url:'/api/ai-search',body:{botId:'12345678',type:'group',id:'10',revision:'a'.repeat(64),enabled:true,key:{action:'replace',value:'SYNTHETIC_SEARCH_KEY'},policy:{enabled:true,roles:['owner']}}}]);assert.equal(await page.locator('#ai-search-key').inputValue(),'');assert.deepEqual(await page.evaluate(()=>aiDrafts()),[]);
});

test('discard clears key drafts and restores switches without affecting another form',async t=>{
 const page=await fixture(t);await page.locator('#ai-field-AgentChar').fill('retained persona draft');await page.selectOption('#ai-search-key-action','replace');await page.locator('#ai-search-key').fill('UNSAVED_SECRET');await page.locator('#ai-field-search-role-admin').check();
 assert.match(await page.locator('#ai-draft-field-list').textContent(),/联网搜索设置.*机器人 12345678.*群 10/s);assert.doesNotMatch(await page.locator('#ai-draft-field-list').textContent(),/UNSAVED_SECRET/);
 page.once('dialog',d=>d.accept());await page.locator('[data-ai-discard=search]').click();assert.equal(await page.locator('#ai-search-key').inputValue(),'');assert.equal(await page.locator('#ai-field-search-role-admin').isChecked(),false);assert.equal(await page.locator('#ai-field-AgentChar').inputValue(),'retained persona draft');assert.deepEqual(await page.evaluate(()=>aiDrafts()),['global']);assert.equal(await page.evaluate(()=>requests.length),0);
});

test('key clearing requires confirmation and sends no replacement value',async t=>{
 const page=await fixture(t);await page.evaluate(value=>renderAISearch({...value,keyConfigured:true}),settings);await page.selectOption('#ai-search-key-action','clear');await page.locator('#ai-search-form button[type=submit]').click();assert.equal(await page.evaluate(()=>requests.length),0);assert.match(await page.locator('#ai-error').textContent(),/确认清除/);
 await page.locator('#ai-search-key-clear').check();await reply(page,settings);await page.locator('#ai-search-form button[type=submit]').click();await idle(page);assert.deepEqual(await page.evaluate(()=>requests[0].body.key),{action:'clear',confirm:true});
});

test('conflict retains draft and shared busy gate blocks duplicate saves and scope reads',async t=>{
 const page=await fixture(t);await page.locator('#ai-field-search-role-user').check();await page.locator('#ai-search-form button[type=submit]').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#ai-search-scope button').isDisabled(),true);assert.equal(await page.locator('#reload-ai').isDisabled(),true);await page.evaluate(()=>loadAISearch({botId:'12345678',type:'private',id:'20'}));assert.equal(await page.evaluate(()=>requests.length),1);
 await page.evaluate(()=>pending[0]({ok:false,status:409,json:async()=>({error:'配置已变化，请重新读取'})}));await idle(page);assert.match(await page.locator('#ai-error').textContent(),/重新读取/);assert.equal(await page.locator('#ai-field-search-role-user').isChecked(),true);assert.deepEqual(await page.evaluate(()=>aiDrafts()),['search']);
});

test('switching scope confirms drafts and a failed read removes the previous editable scope',async t=>{
 const page=await fixture(t);await page.locator('#ai-field-search-scope-enabled').check();await page.locator('#ai-search-bot').fill('12345678');await page.locator('#ai-search-id').fill('20');await page.selectOption('#ai-search-type','private');
 page.once('dialog',d=>d.dismiss());await page.locator('#ai-search-scope button').click();assert.equal(await page.evaluate(()=>requests.length),0);
 page.once('dialog',d=>d.accept());await reply(page,{error:'机器人未连接',status:404});await page.locator('#ai-search-scope button').click();await idle(page);assert.equal(await page.locator('#ai-search-form').isHidden(),true);assert.deepEqual(await page.evaluate(()=>aiDrafts()),[]);assert.match(await page.locator('#ai-error').textContent(),/未连接/);
 await reply(page,{...settings,type:'private',id:'20'});await page.locator('#ai-search-scope button').click();await idle(page);assert.match(await page.locator('#ai-search-form h2').textContent(),/私聊 QQ 20/);
});

test('logout clears key and forms and ignores old-session responses',async t=>{
 const page=await fixture(t);await page.selectOption('#ai-search-key-action','replace');await page.locator('#ai-search-key').fill('UNSAVED_SECRET');await page.locator('#ai-search-form button[type=submit]').click();await page.waitForFunction(()=>pending.length===1);await page.evaluate(()=>{authenticated=false;showAuth();authenticated=true;csrf='new-session';});await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),settings);await page.waitForTimeout(20);assert.equal(await page.locator('#ai-search-form').isHidden(),true);assert.equal(await page.locator('#ai-search-key').count(),0);assert.deepEqual(await page.evaluate(()=>aiDrafts()),[]);assert.equal(await page.evaluate(()=>authenticated),true);
});

test('task summary shows separate search quota and keeps old task compatibility',async t=>{
 const page=await fixture(t);const text=await page.evaluate(()=>{const scope={botId:'12345678',type:'group',id:'10'},item={persisted:true,status:'completed',taskId:'a'.repeat(32),version:1,phase:'finished',webSearch:{available:true,used:2,limit:2,successful:1,empty:1,errors:1,denied:1}};const row=taskSummaryRow(item,scope);const old=taskSummaryRow({...item,webSearch:undefined},scope);return {current:row.textContent,old:old.textContent};});assert.match(text.current,/联网搜索：已尝试 2\/2 次，成功 1 次（其中空结果 1），失败 1 次/);assert.doesNotMatch(text.old,/本任务联网搜索/);
});

test('320 pixel light and dark forms wrap and remain operable',async t=>{
 const page=await fixture(t);await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8')});await page.setViewportSize({width:320,height:800});await page.locator('#ai-search-form').scrollIntoViewIfNeeded();await fs.mkdir('.local/n3/ui',{recursive:true});for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/n3/ui/'+theme+'.png'});}assert.equal(await page.locator('#ai-search-form button[type=submit]').isEnabled(),true);assert.equal(await page.evaluate(()=>requests.length),0);
});
