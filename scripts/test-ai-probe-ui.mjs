// Offline Chrome: diagnostics, saved revisions and task help use synthetic data.
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
const scripts=await Promise.all(['app.js','ai.js','ai-probe.js'].map(name=>fs.readFile(root+name,'utf8')));
const browser=await chromium.launch({channel:'chrome',headless:true});after(()=>browser.close());
const cfg={available:true,global:{revision:'a'.repeat(64),ready:true,fields:{Type:0,ModelName:'saved',API:'https://example.invalid',MaxN:4096,TopP:.9,SystemP:'saved prompt',NoSystemP:false,ReasoningEffort:'',Separator:'',ImageType:0,ImageModelName:'',ImageAPI:'',AgentType:0,AgentModelName:'saved',AgentAPI:'https://example.invalid',AgentChar:'persona',AgentSex:'女'},keys:{Key:true,ImageKey:false,AgentKey:true},addressRedacted:{}},groups:[]};
const result={entry:'chat',model:'saved model',revision:'a'.repeat(64),checkedAt:'2026-10-16T10:00:00Z',milliseconds:21,success:true,category:'ok',message:'基础调用通过',httpStatus:200};
async function fixture(t){
 const context=await browser.newContext();t.after(()=>context.close());await context.route('**/*',r=>r.abort());const page=await context.newPage();await page.setContent(html);
 for(const content of scripts)await page.addScriptTag({content});
 await page.evaluate(value=>{
  authenticated=true;csrf='probe-fixture';aiState=value;window.showOverview=()=>{};window.requests=[];window.replies=[];window.pending=[];
  document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.getElementById('overview-view').hidden=true;document.getElementById('ai-view').hidden=false;
  window.fetch=async(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const value=replies.shift();if(value)return {ok:!value.error,status:value.status||200,json:async()=>value};return new Promise(resolve=>pending.push(resolve));};
  renderAIGlobal(value);aiBusyState(false);
 },cfg);return page;
}
const idle=page=>page.waitForFunction(()=>!aiProbeBusy);

test('GPT and summary diagnostics use their saved entries and the shared in-flight gate',async t=>{
 const page=await fixture(t);
 for(const entry of ['gpt','summary']){
  await page.evaluate(value=>replies.push(value),{...result,entry});await page.locator('#ai-probe-'+entry).click();await idle(page);
  assert.deepEqual(await page.evaluate(()=>requests.at(-1).body),{entry,revision:'a'.repeat(64)});
  assert.match(await page.locator('#ai-probe-result h3').textContent(),entry==='gpt'?/GPT模型/:/群总结模型/);
 }
 await page.locator('#ai-probe-summary').click();await page.waitForFunction(()=>pending.length===1);
 for(const entry of ['chat','gpt','summary','agent'])assert.equal(await page.locator('#ai-probe-'+entry).isDisabled(),true);
 await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),{...result,entry:'summary'});await idle(page);assert.equal(await page.evaluate(()=>requests.length),3);
});

test('manual test sends only the saved entry/revision and preserves all drafts',async t=>{
 const page=await fixture(t);assert.equal(await page.evaluate(()=>requests.length),0);
 await page.locator('#ai-field-ModelName').fill('UNSAVED_MODEL');await page.selectOption('#ai-action-Key','replace');await page.locator('#ai-key-Key').fill('UNSAVED_SECRET');
 await page.evaluate(value=>replies.push(value),{...result,model:'<img src=x onerror=alert(1)>'});await page.locator('#ai-probe-chat').click();await idle(page);
 assert.deepEqual(await page.evaluate(()=>requests),[{url:'/api/ai-test',body:{entry:'chat',revision:'a'.repeat(64)}}]);
 assert.equal(await page.locator('#ai-probe-result img').count(),0);assert.match(await page.locator('#ai-probe-result').textContent(),/基础调用通过/);assert.doesNotMatch(await page.locator('#ai-probe-result').textContent(),/UNSAVED/);
 assert.equal(await page.locator('#ai-key-Key').inputValue(),'UNSAVED_SECRET');assert.deepEqual(await page.evaluate(()=>aiDrafts()),['global']);
});

test('one in-flight test disables both buttons and failures never retry',async t=>{
 const page=await fixture(t);await page.locator('#ai-probe-agent').click();await page.waitForFunction(()=>pending.length===1);
 assert.equal(await page.locator('#ai-probe-chat').isDisabled(),true);assert.equal(await page.locator('#ai-probe-agent').isDisabled(),true);await page.evaluate(()=>testSavedAIModel('chat'));assert.equal(await page.evaluate(()=>requests.length),1);
 await page.evaluate(()=>pending[0]({ok:false,status:409,json:async()=>({error:'配置已变化，请重新读取'})}));await idle(page);
 assert.match(await page.locator('#ai-probe-error').textContent(),/重新读取/);assert.equal(await page.locator('#ai-probe-result').textContent(),'');assert.equal(await page.locator('#ai-probe-agent').isEnabled(),true);assert.equal(await page.evaluate(()=>requests.length),1);
 await page.evaluate(value=>replies.push(value),{...result,entry:'agent',success:false,category:'http_error',message:'提供商拒绝请求',httpStatus:401});await page.locator('#ai-probe-agent').click();await idle(page);assert.match(await page.locator('#ai-probe-result h3').textContent(),/失败/);
});

test('saved configuration changes mark both completed and pending results as historical',async t=>{
 const page=await fixture(t);await page.evaluate(value=>replies.push(value),result);await page.locator('#ai-probe-chat').click();await idle(page);
 await page.evaluate(()=>{aiState.global.revision='b'.repeat(64);renderAIGlobal(aiState);});assert.match(await page.locator('#ai-probe-version-state').textContent(),/历史结果/);
 await page.locator('#ai-probe-agent').click();await page.waitForFunction(()=>pending.length===1);
 await page.evaluate(()=>{aiState.global.revision='c'.repeat(64);renderAIGlobal(aiState);});await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),{...result,entry:'agent',revision:'b'.repeat(64)});await idle(page);
 assert.match(await page.locator('#ai-probe-version-state').textContent(),/历史结果/);assert.equal(await page.evaluate(()=>requests.at(-1).body.revision),'b'.repeat(64));
});

test('leaving AI clears results and discards late replies without clearing drafts',async t=>{
 const page=await fixture(t);await page.locator('#ai-field-AgentChar').fill('retained draft');await page.locator('#ai-probe-chat').click();await page.waitForFunction(()=>pending.length===1);
 await page.locator('[data-view=logs]').click();assert.equal(await page.locator('#ai-probe-result').textContent(),'');await page.evaluate(value=>pending[0]({ok:true,status:200,json:async()=>value}),result);await page.waitForTimeout(20);
 await page.locator('[data-view=ai]').click();assert.equal(await page.locator('#ai-probe-result').textContent(),'');assert.equal(await page.locator('#ai-field-AgentChar').inputValue(),'retained draft');assert.equal(await page.evaluate(()=>requests.length),1);
});

test('logout clears diagnostic state and old-session 401 cannot expire a new session',async t=>{
 const page=await fixture(t);await page.locator('#ai-probe-chat').click();await page.waitForFunction(()=>pending.length===1);
 await page.evaluate(()=>{authenticated=false;showAuth();authenticated=true;csrf='new-session';});await page.evaluate(()=>pending[0]({ok:false,status:401,json:async()=>({error:'old-session'})}));await page.waitForTimeout(20);
 assert.equal(await page.evaluate(()=>authenticated),true);assert.equal(await page.locator('#ai-probe-result').textContent(),'');assert.equal(await page.locator('#ai-probe-error').isHidden(),true);
});

test('task help works by keyboard and anchors without model calls; narrow themes wrap results',async t=>{
 const page=await fixture(t);await page.locator('#ai-help summary').focus();await page.keyboard.press('Enter');assert.equal(await page.locator('#ai-help').getAttribute('open'),'');
 await page.locator('#ai-help a[href="#ai-probe-panel"]').click();assert.match(await page.evaluate(()=>location.hash),/ai-probe-panel/);assert.equal(await page.evaluate(()=>requests.length),0);
 await page.evaluate(()=>{document.getElementById('ai-view').hidden=true;document.getElementById('agent-view').hidden=false;});await page.locator('#agent-help summary').focus();await page.keyboard.press('Enter');await page.locator('#agent-help a[href="#tasks-title"]').click();assert.equal(await page.evaluate(()=>requests.length),0);
 await page.evaluate(value=>{document.getElementById('agent-view').hidden=true;document.getElementById('ai-view').hidden=false;renderAIProbeResult(value);}, {...result,model:'很长的模型标识'.repeat(30)});
 await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8')});await page.setViewportSize({width:320,height:800});await page.locator('#ai-probe-panel').scrollIntoViewIfNeeded();await fs.mkdir('.local/model-probe-ui',{recursive:true});
 for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/model-probe-ui/'+theme+'.png'});}
});
