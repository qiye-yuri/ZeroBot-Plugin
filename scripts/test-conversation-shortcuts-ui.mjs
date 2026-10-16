// Offline Chrome: real page navigation, synthetic API responses, no provider/QQ calls.
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
const scripts=await Promise.all(['app.js','ai.js','agent-archive.js'].map(name=>fs.readFile(root+name,'utf8')));
const browser=await chromium.launch({channel:'chrome',headless:true});after(()=>browser.close());
const bot='9007199254740993',id='9007199254740995';
const global={fields:{Type:0,ModelName:'fixture',API:'https://example.invalid',MaxN:4096,TopP:.9,SystemP:'prompt',NoSystemP:false,ReasoningEffort:'',Separator:'',ImageType:0,ImageModelName:'',ImageAPI:'',AgentType:0,AgentModelName:'fixture',AgentAPI:'https://example.invalid',AgentChar:'global persona',AgentSex:'女'},keys:{Key:true,ImageKey:false,AgentKey:true},addressRedacted:{},revision:'a'.repeat(64),ready:true};
function aiFixture(type='group',target=id){return {available:true,global,groups:['aichatcfg','llm'].map(service=>({service,groupId:type==='group'?target:'',privateId:type==='private'?target:'',revision:'b'.repeat(64),temperature:70,rate:0,useAgent:true,voiceOutput:false,respondAt:true}))};}
function persona(type='group',target=id,account=bot){return {botId:account,type,id:target,revision:'c'.repeat(64),mode:'custom',character:'scoped persona',sex:'女'};}
async function harness(t,type='group'){
 const context=await browser.newContext();t.after(()=>context.close());await context.route('**/*',r=>r.abort());const page=await context.newPage();page.setDefaultTimeout(8000);page.on("pageerror",error=>console.error(error));await page.setContent(html);
 for(const content of scripts)await page.addScriptTag({content});
 await page.evaluate(({bot,id,type})=>{
  authenticated=true;csrf='fixture';window.showOverview=()=>{};window.requests=[];window.replies=[];window.pending=[];
  window.fetch=async(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const value=replies.shift();if(value)return {ok:!value.error,status:value.status||200,json:async()=>value};return new Promise(resolve=>pending.push(resolve));};
  document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.getElementById('overview-view').hidden=true;document.getElementById('agent-view').hidden=false;agentVisible=true;agentScope={botId:bot,id,type,archived:false};agentControls();
 },{bot,id,type});return page;
}
const idle=page=>page.waitForFunction(()=>!aiBusy);

test('persona shortcut follows loaded large-ID scope and saves via existing API',async t=>{
 const page=await harness(t);
 await page.evaluate(values=>replies.push(...values),[aiFixture(),persona()]);
 // Scope input drafts must not retarget a successfully loaded conversation.
 await page.evaluate(()=>{document.getElementById('agent-id').value='42';document.getElementById('agent-type').value='private';});
 await page.evaluate(({bot,id})=>{agentScope={botId:bot,type:'group',id,archived:false};agentControls();},{bot,id});
 await page.locator('#agent-open-persona').click();await idle(page);await page.waitForFunction(()=>!document.getElementById('ai-persona-form').hidden);
 const reads=await page.evaluate(()=>requests);assert.equal(reads.length,2);assert.ok(reads.every(x=>x.body===null));
 assert.deepEqual(Object.fromEntries(new URL(reads[1].url,'http://fixture').searchParams),{botId:bot,type:'group',id});
 assert.equal(await page.locator('#ai-view').isVisible(),true);assert.match(await page.locator('#ai-persona-form h2').textContent(),new RegExp(bot+'.*'+id));
 await page.locator('#ai-field-persona-character').fill('updated persona');await page.evaluate(value=>replies.push(value),{...persona(),character:'updated persona'});await page.locator('#ai-persona-form button[type=submit]').click();await idle(page);
 assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{botId:bot,type:'group',id,revision:'c'.repeat(64),mode:'custom',character:'updated persona',sex:'女'});
});

test('private parameter shortcut reads exact private scope and explains shared settings',async t=>{
 const page=await harness(t,'private');await page.evaluate(value=>replies.push(value),aiFixture('private'));
 await page.locator('#agent-open-ai').click();await idle(page);
 assert.equal(await page.evaluate(()=>requests.length),1);assert.equal(await page.evaluate(()=>requests[0].url),'/api/ai?private='+id);
 assert.match(await page.locator('#ai-status').textContent(),/各机器人共享/);assert.match(await page.locator('#ai-group-forms h2').first().textContent(),new RegExp('私聊 QQ '+id));
 await page.evaluate(value=>replies.push(value),aiFixture('private'));await page.locator('#ai-group-forms form[data-service=aichatcfg] button[type=submit]').click();await idle(page);
 const body=await page.evaluate(()=>requests.at(-1).body);assert.equal(body.privateId,id);assert.equal(body.groupId,undefined);assert.equal(body.botId,undefined);
});

test('cancelled shortcut preserves drafts and accepted persona shortcut preserves unrelated forms',async t=>{
 const page=await harness(t);await page.evaluate(({value,person})=>{aiState=value;renderAIGlobal(value);renderAIGroups(value.groups);renderAIPersona(person);aiBusyState(false);}, {value:aiFixture(),person:persona('private','123')});
 await page.locator('#ai-view').evaluate(view=>view.hidden=false);await page.locator('#ai-field-persona-character').fill('persona draft');await page.locator('#ai-field-AgentChar').fill('global draft');await page.locator('#ai-view').evaluate(view=>view.hidden=true);
 let dialogs=0;const cancel=dialog=>{dialogs++;dialog.dismiss();};page.on('dialog',cancel);await page.locator('#agent-open-persona').click();page.off('dialog',cancel);
 assert.equal(dialogs,1);assert.equal(await page.evaluate(()=>requests.length),0);assert.equal(await page.locator('#agent-view').isVisible(),true);assert.equal(await page.locator('#ai-field-persona-character').inputValue(),'persona draft');
 page.once('dialog',dialog=>{dialogs++;dialog.accept();});await page.evaluate(value=>replies.push(value),persona('group',id,'87654321'));
 await page.evaluate(()=>{agentScope.botId='87654321';});await page.locator('#agent-open-persona').click();await idle(page);
 assert.equal(dialogs,2);assert.equal(await page.evaluate(()=>requests.length),1);assert.equal(await page.locator('#ai-field-AgentChar').inputValue(),'global draft');assert.deepEqual(await page.evaluate(()=>aiDrafts()),['global']);assert.match(await page.locator('#ai-persona-form h2').textContent(),/87654321/);
});

test('parameter shortcut confirms once and clears old editable scope on read failure',async t=>{
 const page=await harness(t);await page.evaluate(value=>{aiState=value;renderAIGlobal(value);renderAIGroups(value.groups);aiBusyState(false);},aiFixture('private','123'));
 await page.locator('#ai-view').evaluate(view=>view.hidden=false);await page.locator('#ai-field-AgentChar').fill('draft');await page.locator('#ai-view').evaluate(view=>view.hidden=true);
 let dialogs=0;page.on('dialog',dialog=>{dialogs++;dialog.accept();});await page.evaluate(()=>replies.push({error:'read failed',status:503}));await page.locator('#agent-open-ai').click();await idle(page);
 assert.equal(dialogs,1);assert.equal(await page.locator('#ai-group-forms form').count(),0);assert.equal(await page.locator('#ai-global-inputs input').count(),0);assert.match(await page.locator('#ai-error').textContent(),/read failed/);
});

test('failed persona shortcut removes previous target and logout drops late response',async t=>{
 const page=await harness(t);await page.evaluate(({value,person})=>{aiState=value;renderAIGlobal(value);renderAIGroups(value.groups);renderAIPersona(person);}, {value:aiFixture(),person:persona('private','123')});
 await page.evaluate(()=>replies.push({error:'persona failed',status:503}));await page.locator('#agent-open-persona').click();await idle(page);
 assert.equal(await page.locator('#ai-persona-form').isHidden(),true);assert.match(await page.locator('#ai-error').textContent(),/persona failed/);
 await page.evaluate(()=>{document.getElementById('ai-view').hidden=true;document.getElementById('agent-view').hidden=false;agentVisible=true;agentScope={botId:'9007199254740993',type:'group',id:'9007199254740995'};agentControls();});
 await page.locator('#agent-open-persona').click();await page.waitForFunction(()=>pending.length===1);
 await page.evaluate(()=>{authenticated=false;showAuth();});await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),persona());await idle(page);
 assert.equal(await page.locator('#ai-persona-form').isHidden(),true);assert.deepEqual(await page.evaluate(()=>aiDrafts()),[]);
});

test('unloaded busy and changed scope disable shortcuts; layout fits 320px',async t=>{
 const page=await harness(t);for(const state of [{scope:null,busy:false},{scope:{botId:bot,type:'group',id},busy:true}]){await page.evaluate(state=>{agentScope=state.scope;agentBusy=state.busy;agentControls();},state);assert.equal(await page.locator('#agent-open-ai').isDisabled(),true);assert.equal(await page.locator('#agent-open-persona').isDisabled(),true);}
 await page.evaluate(({bot,id})=>{agentBusy=false;agentScope={botId:bot,type:'group',id};agentControls();},{bot,id});await page.locator('#agent-id').fill('123');await page.locator('#agent-id').dispatchEvent('change');assert.equal(await page.locator('#agent-open-ai').isDisabled(),true);
 await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8')});await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/conversation-shortcuts-ui',{recursive:true});
 for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.locator('#agent-open-ai').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/conversation-shortcuts-ui/'+theme+'.png'});}
});
