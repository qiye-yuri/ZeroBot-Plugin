// R9 offline Chrome; all identities, credentials and HTTP responses are synthetic.
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
const scripts=await Promise.all(['app.js','ai.js','ai-search.js','ai-probe.js'].map(name=>fs.readFile(root+name,'utf8')));
const browser=await chromium.launch({channel:'chrome',headless:true});after(()=>browser.close());
const settings={botId:'12345678',type:'group',id:'10',revision:'a'.repeat(64),enabled:false,keyConfigured:true,policy:{enabled:false,roles:['owner']}};
const cfg={available:true,global:{revision:'a'.repeat(64),ready:true,fields:{Type:0,ModelName:'saved',API:'https://example.invalid',MaxN:4096,TopP:.9,SystemP:'saved prompt',NoSystemP:false,ReasoningEffort:'',Separator:'',ImageType:0,ImageModelName:'',ImageAPI:'',AgentType:0,AgentModelName:'saved',AgentAPI:'https://example.invalid',AgentChar:'persona',AgentSex:'女'},keys:{Key:true,ImageKey:false,AgentKey:true},addressRedacted:{}},groups:[]};
async function fixture(t){
 const context=await browser.newContext();t.after(()=>context.close());await context.route('**/*',r=>r.abort());const page=await context.newPage();await page.setContent(html);for(const content of scripts)await page.addScriptTag({content});
 await page.evaluate(({cfg,settings})=>{authenticated=true;csrf='search-fixture';aiState=cfg;window.showOverview=()=>{};window.requests=[];window.replies=[];window.pending=[];document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.getElementById('overview-view').hidden=true;document.getElementById('ai-view').hidden=false;
 window.fetch=async(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const value=replies.shift();if(value)return {ok:!value.error,status:value.status||200,json:async()=>value};return new Promise(resolve=>pending.push(resolve));};renderAIGlobal(cfg);renderAISearch(settings);aiBusyState(false);},{cfg,settings});return page;
}
const idle=page=>page.waitForFunction(()=>!aiProbeBusy);
const result={revision:'a'.repeat(64),checkedAt:'2026-10-24T00:00:00Z',milliseconds:20,category:'success',httpStatus:200,resultCount:5,message:'本次搜索基础调用通过；不代表已启用会话搜索'};

test('R12 search outcomes and scope remain visible while full revisions start collapsed',async t=>{
 const page=await fixture(t);
 for(const [category,status,count,title] of [['success',200,5,'基础调用通过'],['empty',200,0,'调用完成，无匹配结果'],['http_error',401,0,'本次测试失败'],['network_error',0,0,'本次测试失败']]){
  const value={...result,category,httpStatus:status,resultCount:count,message:'服务端说明：'+category};await page.evaluate(v=>replies.push(v),value);await page.locator('#ai-search-probe').click();await idle(page);
  const text=await page.locator('#ai-search-probe-result').innerText();assert.ok(text.includes(title));assert.ok(text.includes(value.message));assert.ok(text.includes(value.checkedAt));assert.ok(text.includes('合法结果：'+count+' 条'));assert.equal(text.includes('HTTP'),status!==0);assert.doesNotMatch(text,/配置版本：/);assert.equal(await page.locator('#ai-search-probe-version').isVisible(),true);assert.equal(await page.locator('#ai-search-probe-result details').getAttribute('open'),null);
 }
 assert.equal(await page.evaluate(()=>requests.length),4);
});

test('R12 search disclosure keeps full selectable revision and independent model result',async t=>{
 const page=await fixture(t);await page.evaluate(v=>{renderAIProbeResult({entry:'agent',success:true,model:'fixture',message:'模型通过',revision:v.revision,checkedAt:v.checkedAt,milliseconds:1});replies.push(v);},result);await page.locator('#ai-search-probe').click();await idle(page);
 const summary=page.locator('#ai-search-probe-result summary');await summary.focus();await page.keyboard.press('Enter');
 assert.equal(await page.locator('#ai-search-probe-result details p').evaluate(e=>{const range=document.createRange();range.selectNodeContents(e);const selection=window.getSelection();selection.removeAllRanges();selection.addRange(range);return selection.toString();}),'配置版本：'+result.revision);
 await page.evaluate(()=>{aiBaselines.get('search').search.revision='b'.repeat(64);renderAIProbeControls();});assert.equal(await page.locator('#ai-search-probe-result details').getAttribute('open'),'');assert.match(await page.locator('#ai-search-probe-version').innerText(),/历史结果/);assert.match(await page.locator('#ai-probe-result').innerText(),/模型通过/);assert.equal(await page.locator('#ai-probe-result details').getAttribute('open'),null);
 await summary.press('Enter');assert.equal(await page.locator('#ai-search-probe-version').isVisible(),true);assert.equal(await page.evaluate(()=>requests.length),1);
 await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8')});await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/r12-ui',{recursive:true});
 for(const theme of ['light','dark'])for(const expanded of [false,true]){if((await page.locator('#ai-search-probe-result details').getAttribute('open')!==null)!==expanded)await summary.press('Enter');await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.locator('#ai-search-probe-result').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/r12-ui/search-'+theme+'-'+expanded+'.png'});}
 await page.evaluate(v=>replies.push(v),{...result,revision:'b'.repeat(64)});await page.locator('#ai-search-probe').click();await idle(page);assert.equal(await page.locator('#ai-search-probe-result details').getAttribute('open'),null);assert.equal(await page.evaluate(()=>requests.length),2);
});

test('saved search key test preserves replacement and enablement drafts and sends only revision',async t=>{
 const page=await fixture(t);assert.equal(await page.evaluate(()=>requests.length),0);
 await page.selectOption('#ai-search-key-action','replace');await page.locator('#ai-search-key').fill('UNSAVED_SECRET');await page.locator('#ai-field-search-enabled').check();
 await page.evaluate(value=>replies.push(value),result);await page.locator('#ai-search-probe').click();await idle(page);
 assert.deepEqual(await page.evaluate(()=>requests),[{url:'/api/ai-search-test',body:{revision:'a'.repeat(64)}}]);
 assert.equal(await page.locator('#ai-search-key').inputValue(),'UNSAVED_SECRET');assert.equal(await page.locator('#ai-field-search-enabled').isChecked(),true);assert.deepEqual(await page.evaluate(()=>aiDrafts()),['search']);
 assert.match(await page.locator('#ai-search-probe-result').textContent(),/基础调用通过/);assert.doesNotMatch(await page.locator('#ai-search-probe-result').textContent(),/UNSAVED_SECRET/);
});

test('missing saved key cannot test even when a replacement draft is present',async t=>{
 const page=await fixture(t);await page.evaluate(value=>renderAISearch({...value,keyConfigured:false}),settings);
 await page.selectOption('#ai-search-key-action','replace');await page.locator('#ai-search-key').fill('DRAFT_ONLY');assert.equal(await page.locator('#ai-search-probe').isDisabled(),true);
 await page.evaluate(()=>testSavedSearchKey());assert.equal(await page.evaluate(()=>requests.length),0);
});

test('search and model probes share one in-flight gate in both directions',async t=>{
 const page=await fixture(t);await page.locator('#ai-search-probe').click();await page.waitForFunction(()=>pending.length===1);
 for(const entry of ['chat','gpt','summary','agent'])assert.equal(await page.locator('#ai-probe-'+entry).isDisabled(),true);
 await page.evaluate(()=>{testSavedSearchKey();testSavedAIModel('chat');});assert.equal(await page.evaluate(()=>requests.length),1);
 await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),result);await idle(page);
 await page.locator('#ai-probe-chat').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#ai-search-probe').isDisabled(),true);
 await page.evaluate(()=>testSavedSearchKey());assert.equal(await page.evaluate(()=>requests.length),2);
 await page.evaluate(()=>pending.shift()({ok:false,status:429,json:async()=>({error:'已有测试进行中'})}));await idle(page);assert.equal(await page.locator('#ai-search-probe').isEnabled(),true);
});

test('completed and pending search results become historical after loading a new revision',async t=>{
 const page=await fixture(t);await page.evaluate(value=>replies.push(value),result);await page.locator('#ai-search-probe').click();await idle(page);
 await page.evaluate(value=>renderAISearch({...value,revision:'b'.repeat(64)}),settings);assert.match(await page.locator('#ai-search-probe-version').textContent(),/历史结果/);
 await page.locator('#ai-search-probe').click();await page.waitForFunction(()=>pending.length===1);
 await page.evaluate(value=>renderAISearch({...value,revision:'c'.repeat(64)}),settings);await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),{...result,revision:'b'.repeat(64)});await idle(page);
 assert.match(await page.locator('#ai-search-probe-version').textContent(),/历史结果/);assert.equal(await page.evaluate(()=>requests.at(-1).body.revision),'b'.repeat(64));
});

test('conflict preserves drafts and empty response is distinct from failures without retry',async t=>{
 const page=await fixture(t);await page.locator('#ai-field-search-role-user').check();await page.evaluate(()=>replies.push({error:'配置已变化，请重新读取',status:409}));await page.locator('#ai-search-probe').click();await idle(page);
 assert.match(await page.locator('#ai-search-probe-error').textContent(),/重新读取/);assert.equal(await page.locator('#ai-field-search-role-user').isChecked(),true);
 for(const category of ['empty','http_error','invalid_response','response_limit','network_error']){
  await page.evaluate(value=>replies.push(value),{...result,category,resultCount:0});await page.locator('#ai-search-probe').click();await idle(page);
  assert.match(await page.locator('#ai-search-probe-result h3').textContent(),category==='empty'?/无匹配结果/:/测试失败/);
 }
 assert.equal(await page.evaluate(()=>requests.length),6);
});

for(const mode of ['navigation','logout'])test(mode+' clears search result and ignores a late response',async t=>{
 const page=await fixture(t);await page.locator('#ai-search-probe').click();await page.waitForFunction(()=>pending.length===1);
 if(mode==='navigation')await page.locator('[data-view=logs]').click();else await page.evaluate(()=>{authenticated=false;showAuth();authenticated=true;csrf='new-session';});
 await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),result);await page.waitForTimeout(20);
 assert.equal(await page.evaluate(()=>aiSearchProbeResult),null);assert.equal(await page.evaluate(()=>requests.length),1);
});

test('keyboard and 320 pixel themes remain usable with full revision and escaped text',async t=>{
 const page=await fixture(t);await page.evaluate(value=>replies.push(value),{...result,message:'<img src=x onerror=alert(1)>'});await page.locator('#ai-search-probe').focus();await page.keyboard.press('Enter');await idle(page);
 assert.equal(await page.locator('#ai-search-probe-result img').count(),0);
 await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8')});await page.setViewportSize({width:320,height:800});await fs.mkdir('.local/r9-ui',{recursive:true});
 for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.locator('#ai-search-probe').scrollIntoViewIfNeeded();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/r9-ui/'+theme+'.png'});}
});
