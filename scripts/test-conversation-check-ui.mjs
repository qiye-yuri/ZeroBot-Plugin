// Offline Chrome with synthetic saved configuration; all external requests blocked.
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
const css=(await Promise.all(['style.css','appearance.css'].map(name=>fs.readFile(root+name,'utf8')))).join('\n');
const browser=await chromium.launch({channel:'chrome',headless:true});after(()=>browser.close());
const scope={botId:'9007199254740993',type:'private',id:'9007199254740995'};
const settings={scope,observedAt:'2026-10-18T04:00:00Z',revision:'a'.repeat(64),response:{globalEnabled:false,conversationEnabled:true,effectiveEnabled:true,source:'conversation'},plugins:[{service:'aichatcfg',registered:true,state:{effectiveEnabled:true,source:'default',globalMode:'inherit',groupMode:'inherit',revision:'b'.repeat(64)}},{service:'llm',registered:false}],conversation:[{service:'aichatcfg',rate:20,effectiveTemperature:.7,useAgent:true,respondAt:true,voiceOutput:false,revision:'c'.repeat(64)}],models:{named:true,error:'',entries:[{entry:'chat',applicable:true,enabled:true,inherited:false,profileId:'example',name:'示例',model:'<img src=x onerror=alert(1)>',type:0,check:{complete:true},parameters:{maxN:2048,topP:.9,reasoningEffort:''},parameterSources:{maxN:'model',topP:'global',reasoningEffort:'global'}},{entry:'summary',applicable:false,enabled:true,inherited:true,profileId:'example',name:'示例',model:'summary',type:0,check:{complete:true},parameters:{maxN:2048,topP:.9,reasoningEffort:''},parameterSources:{maxN:'model',topP:'global',reasoningEffort:'global'}}]},persona:{mode:'custom'},search:{enabled:true,keyConfigured:true,policy:{enabled:false,roles:[]}}};
async function harness(t){
 const context=await browser.newContext();t.after(()=>context.close());await context.route('**/*',r=>r.abort());
 const page=await context.newPage();page.setDefaultTimeout(8000);await page.setContent(html);await page.addStyleTag({content:css});
 for(const content of scripts)await page.addScriptTag({content});
 await page.evaluate(scope=>{
  authenticated=true;csrf='fixture';window.showOverview=()=>{};window.requests=[];window.replies=[];window.pending=[];
  window.fetch=(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const value=replies.shift();return value?Promise.resolve({ok:!value.error,status:value.status||200,json:async()=>value}):new Promise(resolve=>pending.push(resolve));};
  document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.getElementById('overview-view').hidden=true;document.getElementById('agent-view').hidden=false;
  agentVisible=true;agentScope={...scope,archived:true};agentNext='timeline-cursor';agentSearchContinue={query:{keyword:'saved'},scope:{...agentScope}};
  document.getElementById('agent-entries').textContent='original timeline';document.getElementById('agent-keyword').value='draft search';agentControls();
 },scope);
 return page;
}
const idle=page=>page.waitForFunction(()=>!agentBusy);
async function read(page,value=settings){await page.evaluate(value=>replies.push(value),value);await page.locator('#conversation-check-load').click();await idle(page);}
const completeCheck={typeValid:true,modelConfigured:true,modelRequired:true,addressConfigured:true,addressValid:true,keyConfigured:true,keyRequired:true,complete:true};

test('R10 renders protocol and switch names for legacy and named models without guessing unknown values',async t=>{
 const page=await harness(t);
 for(const [type,name,globalMode,groupMode,expected] of [[0,'OpenAI','inherit','enabled','全局 继承 / 会话 启用'],[1,'OLLaMA','disabled','inherit','全局 停用 / 会话 继承'],[2,'GenAI','enabled','disabled','全局 启用 / 会话 停用'],[9,'未知（9）','future','inherit','全局 未知（future） / 会话 继承']]){
  const value=structuredClone(settings);value.models.named=type!==1;value.models.entries=[{...value.models.entries[0],type,check:completeCheck}];value.plugins[0].state={...value.plugins[0].state,globalMode,groupMode};
  await read(page,value);const text=await page.locator('#conversation-check-result').innerText();
  assert.ok(text.includes('协议 '+name));assert.ok(text.includes(expected));assert.ok(text.includes(type===1?'旧字段模式':'命名模式'));assert.match(text,/来源 插件默认/);
 }
 assert.equal(await page.evaluate(()=>requests.length),4);
});

test('R10 lists existing local check failures and keeps missing address distinct from invalid address',async t=>{
 const page=await harness(t);
 for(const addressConfigured of [false,true]){
  const value=structuredClone(settings);value.models.entries=[{...value.models.entries[0],check:{...completeCheck,complete:false,typeValid:false,modelConfigured:false,keyConfigured:false,addressConfigured,addressValid:false}}];
  await read(page,value);const text=await page.locator('#conversation-check-result').innerText();
  for(const issue of ['本地字段不完整','接口类型无效','缺少必要模型','缺少必要密钥',addressConfigured?'接口地址无效':'缺少接口地址'])assert.ok(text.includes(issue),issue);
  assert.ok(!text.includes(addressConfigured?'缺少接口地址':'接口地址无效'));
 }
});

test('R10 respects OLLaMA key and optional image model rules and prioritizes disabled or inapplicable entries',async t=>{
 const page=await harness(t);
 for(const row of [
  {type:1,check:{...completeCheck,keyConfigured:false,keyRequired:false}},
  {entry:'image',check:{...completeCheck,modelConfigured:false,modelRequired:false}},
  {entry:'image',enabled:false,check:{...completeCheck,complete:false,keyConfigured:false}},
  {entry:'summary',applicable:false,check:{...completeCheck,complete:false,keyConfigured:false}}
 ]){
  const value=structuredClone(settings);value.models.entries=[{...value.models.entries[0],...row}];await read(page,value);
  const text=await page.locator('#conversation-check-result').innerText();assert.doesNotMatch(text,/需处理：|缺少必要/);
  assert.ok(text.includes(row.applicable===false?'此会话不适用':row.enabled===false?'入口停用':'本地字段完整'));
 }
 // Optional fields must not become required merely because another field fails.
 const value=structuredClone(settings);value.models.entries=[{...value.models.entries[0],entry:'image',type:1,check:{...completeCheck,complete:false,addressValid:false,modelRequired:false,modelConfigured:false,keyRequired:false,keyConfigured:false}}];
 await read(page,value);const text=await page.locator('#conversation-check-result').innerText();assert.match(text,/需处理：接口地址无效/);assert.doesNotMatch(text,/缺少必要/);
});

test('R10 complete revisions remain selectable in a keyboard disclosure with no extra requests',async t=>{
 const page=await harness(t);await read(page);
 const details=page.locator('#conversation-check-result details');assert.equal(await details.getAttribute('open'),null);
 const visible=await page.locator('#agent-view').innerText();assert.ok(visible.includes(scope.botId)&&visible.includes(scope.id)&&visible.includes(settings.observedAt));
 for(const revision of [settings.revision,settings.plugins[0].state.revision,settings.conversation[0].revision])assert.ok(!visible.includes(revision));
 await page.getByText('完整配置修订（展开核对或复制）',{exact:true}).focus();await page.keyboard.press('Enter');
 const expanded=await details.innerText();for(const revision of [settings.revision,settings.plugins[0].state.revision,settings.conversation[0].revision])assert.ok(expanded.includes(revision));
 const selected=await details.locator('p').evaluate(el=>{const range=document.createRange();range.selectNodeContents(el);getSelection().removeAllRanges();getSelection().addRange(range);return getSelection().toString();});assert.ok(selected.includes(settings.revision));
 await page.keyboard.press('Enter');assert.equal(await details.getAttribute('open'),null);assert.equal(await page.evaluate(()=>requests.length),1);
});

test('R10 expanded revisions and local issues wrap at 320px in both themes',async t=>{
 const page=await harness(t);await page.setViewportSize({width:320,height:800});
 const value=structuredClone(settings);value.models.entries[0].check={...completeCheck,complete:false,addressConfigured:false,addressValid:false,keyConfigured:false};await read(page,value);
 await page.getByText('完整配置修订（展开核对或复制）',{exact:true}).click();await fs.mkdir('.local/r10-ui',{recursive:true});
 for(const theme of ['light','dark']){
  await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);
  await page.locator('#conversation-check-result details').scrollIntoViewIfNeeded();await page.screenshot({path:'.local/r10-ui/'+theme+'-revisions.png'});
  await page.getByRole('heading',{name:'普通聊天',exact:true}).scrollIntoViewIfNeeded();await page.screenshot({path:'.local/r10-ui/'+theme+'-issues.png'});
 }
 assert.equal(await page.evaluate(()=>requests.length),1);
});

test('check uses loaded large private scope, is read-only and preserves timeline and drafts',async t=>{
 const page=await harness(t);
 await page.evaluate(()=>{document.getElementById('agent-type').value='group';document.getElementById('agent-id').value='42';});
 await read(page);
 const requests=await page.evaluate(()=>requests);assert.equal(requests.length,1);assert.equal(requests[0].body,null);
 assert.deepEqual(Object.fromEntries(new URL(requests[0].url,'http://fixture').searchParams),scope);
 assert.match(await page.locator('#conversation-check-status').textContent(),new RegExp(scope.botId+'.*私聊 '+scope.id));
 const text=await page.locator('#conversation-check-result').textContent();
 for(const pattern of [/各机器人共享/,/当前机器人会话自定义/,/此会话不适用/,/未注册/,/当前会话 关闭/,/允许角色：无/,/2048（模型覆盖）/])assert.match(text,pattern);
 assert.equal(await page.locator('#conversation-check-result img').count(),0);assert.match(text,/<img src=x/);
 assert.equal(await page.locator('#agent-entries').textContent(),'original timeline');assert.equal(await page.locator('#agent-keyword').inputValue(),'draft search');
 assert.equal(await page.evaluate(()=>agentNext),'timeline-cursor');assert.equal(await page.evaluate(()=>agentSearchContinue.query.keyword),'saved');
});

test('duplicate clicks issue one request, failure clears old result and manual reread shows model errors',async t=>{
 const page=await harness(t);await read(page);
 await page.locator('#conversation-check-load').click();await page.waitForFunction(()=>pending.length===1);
 assert.equal(await page.locator('#conversation-check-load').isDisabled(),true);assert.equal(await page.locator('#conversation-check-result').textContent(),'');
 await page.evaluate(()=>document.getElementById('conversation-check-load').click());assert.equal(await page.evaluate(()=>requests.length),2);
 await page.evaluate(()=>pending.shift()({ok:false,status:500,json:async()=>({error:'读取失败'})}));await idle(page);
 assert.match(await page.locator('#conversation-check-status').textContent(),/核对失败/);assert.equal(await page.locator('#conversation-check-links').isHidden(),true);
 await read(page,{...settings,models:{named:true,error:'入口引用无效，请重新选择',entries:[]}});
 assert.match(await page.locator('#conversation-check-result').textContent(),/入口引用无效/);assert.match(await page.locator('#conversation-check-result').textContent(),/当前机器人会话自定义/);
});

test('unloaded scope is disabled and scope changes or leaving discard check results and late replies',async t=>{
 const page=await harness(t);await read(page);
 await page.locator('#agent-id').fill('20');await page.locator('#agent-id').press('Tab');
 assert.equal(await page.locator('#conversation-check-result').textContent(),'');assert.equal(await page.locator('#conversation-check-load').isDisabled(),true);
 await page.evaluate(scope=>{agentScope={...scope};agentControls();},scope);
 await page.locator('#conversation-check-load').click();await page.waitForFunction(()=>pending.length===1);
 await page.locator('[data-view=overview]').click();await page.evaluate(value=>pending.shift()({ok:true,status:200,json:async()=>value}),settings);await idle(page);
 assert.equal(await page.locator('#conversation-check-result').textContent(),'');assert.equal(await page.locator('#conversation-check-links').isHidden(),true);
 assert.equal(await page.evaluate(()=>agentScope),null);
});

test('result parameter navigation reuses existing draft confirmation and exact private target',async t=>{
 const page=await harness(t);await read(page);
 const value={available:true,global:{fields:{Type:0,ModelName:'fixture',API:'https://example.invalid',MaxN:4096,TopP:.9,SystemP:'prompt',NoSystemP:false,ReasoningEffort:'',Separator:'',ImageType:0,ImageModelName:'',ImageAPI:'',AgentType:0,AgentModelName:'fixture',AgentAPI:'https://example.invalid',AgentChar:'global',AgentSex:'女'},keys:{Key:true},addressRedacted:{},revision:'d'.repeat(64),ready:true},groups:[]};
 await page.evaluate(value=>{aiState=value;renderAIGlobal(value);aiBusyState(false);},value);
 await page.locator('#ai-field-AgentChar').evaluate(input=>{input.value='unsaved persona';input.dispatchEvent(new Event('input',{bubbles:true}));});
 const cancel=dialog=>dialog.dismiss();page.on('dialog',cancel);await page.locator('#conversation-check-parameters').click();page.off('dialog',cancel);
 assert.equal(await page.locator('#agent-view').isVisible(),true);assert.equal(await page.evaluate(()=>requests.length),1);
 page.once('dialog',dialog=>dialog.accept());await page.evaluate(value=>replies.push(value),value);await page.locator('#conversation-check-parameters').click();await page.waitForFunction(()=>!aiBusy);
 assert.equal(await page.evaluate(()=>requests.at(-1).url),'/api/ai?private='+scope.id);assert.equal(await page.locator('#ai-view').isVisible(),true);
});

test('summary and navigation remain readable at 320px in both themes and reachable by keyboard',async t=>{
 const page=await harness(t);await page.setViewportSize({width:320,height:800});await read(page);await fs.mkdir('.local/r7-ui',{recursive:true});
 for(const theme of ['light','dark']){
  await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);
  await page.locator('#conversation-check-title').scrollIntoViewIfNeeded();await page.screenshot({path:'.local/r7-ui/'+theme+'.png'});
 }
 await page.locator('#conversation-check-persona').focus();await page.keyboard.press('Tab');assert.equal(await page.locator('#conversation-check-parameters').evaluate(el=>el===document.activeElement),true);
});
