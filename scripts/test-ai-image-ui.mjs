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
const scripts=await Promise.all(['app.js','ai.js','ai-image.js','agent-tasks.js'].map(name=>fs.readFile(root+name,'utf8')));
const browser=await chromium.launch({channel:'chrome',headless:true});after(()=>browser.close());
const settings={revision:'a'.repeat(64),mode:'openai',url:'https://example.invalid/v1/images/generations',model:'synthetic-model',keyConfigured:true,quality:'medium',size:'1024x1024',monthlyBudgetUSD:'100',customCostUSD:'',budget:{month:'2026-10',chargedUSD:'0.000000',remainingUSD:'100.000000',uncertainUSD:'0.000000',requests:0,stopped:false}};
const cfg={available:true,global:{revision:'a'.repeat(64),ready:true,fields:{Type:0,ModelName:'saved',API:'https://example.invalid',MaxN:4096,TopP:.9,SystemP:'saved prompt',NoSystemP:false,ReasoningEffort:'',Separator:'',ImageType:0,ImageModelName:'',ImageAPI:'',AgentType:0,AgentModelName:'saved',AgentAPI:'https://example.invalid',AgentChar:'persona',AgentSex:'女'},keys:{Key:true,ImageKey:false,AgentKey:true},addressRedacted:{}},groups:[]};
async function fixture(t){
 const context=await browser.newContext();t.after(()=>context.close());await context.route('**/*',r=>r.abort());const page=await context.newPage();await page.setContent(html);for(const content of scripts)await page.addScriptTag({content});
 await page.evaluate(({cfg,settings})=>{authenticated=true;csrf='search-fixture';aiState=cfg;window.showOverview=()=>{};window.requests=[];window.replies=[];window.pending=[];document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.getElementById('overview-view').hidden=true;document.getElementById('ai-view').hidden=false;
 window.fetch=async(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const value=replies.shift();if(value)return {ok:!value.error,status:value.status||200,json:async()=>value};return new Promise(resolve=>pending.push(resolve));};renderAIGlobal(cfg);renderAIImage(settings);aiBusyState(false);},{cfg,settings});return page;
}
const idle=page=>page.waitForFunction(()=>!aiBusy);
async function reply(page,value){await page.evaluate(value=>replies.push(value),value);}


test('save sends independent config and clears new key',async t=>{
 const page=await fixture(t);await page.locator('#ai-field-image-model').fill('new-model');await page.selectOption('#ai-image-key-action','replace');await page.locator('#ai-image-key').fill('SYNTHETIC_IMAGE_KEY');
 assert.doesNotMatch(await page.locator('#ai-draft-field-list').textContent(),/SYNTHETIC_IMAGE_KEY/);
 await reply(page,{...settings,model:'new-model',revision:'b'.repeat(64)});await page.locator('#ai-image-form button[type=submit]').click();await idle(page);
 assert.deepEqual(await page.evaluate(()=>requests),[{url:'/api/ai-image',body:{revision:settings.revision,mode:'openai',url:settings.url,model:'new-model',quality:'medium',size:'1024x1024',options:{referenceReserveUSD:'0.25',background:'auto',format:'png',compression:100,count:1,moderation:'auto',stream:false,partialImages:0,safetyIdentifier:'',proxyURL:'',timeoutSeconds:300},monthlyBudgetUSD:'100',customCostUSD:'',key:{action:'replace',value:'SYNTHETIC_IMAGE_KEY'}}}]);assert.equal(await page.locator('#ai-image-key').inputValue(),'');assert.deepEqual(await page.evaluate(()=>aiDrafts()),[]);
});
test('discard only resets image draft and key',async t=>{
 const page=await fixture(t);await page.locator('#ai-field-AgentChar').fill('other draft');await page.selectOption('#ai-image-key-action','replace');await page.locator('#ai-image-key').fill('UNSAVED');page.once('dialog',d=>d.accept());await page.locator('[data-ai-discard=image]').click();assert.equal(await page.locator('#ai-image-key').inputValue(),'');assert.equal(await page.locator('#ai-field-AgentChar').inputValue(),'other draft');assert.deepEqual(await page.evaluate(()=>aiDrafts()),['global']);
});
test('clear requires confirmation and conflict retains draft',async t=>{
 const page=await fixture(t);await page.selectOption('#ai-image-key-action','clear');await page.locator('#ai-image-form button[type=submit]').click();assert.equal(await page.evaluate(()=>requests.length),0);await page.locator('#ai-image-key-clear').check();await page.locator('#ai-image-form button[type=submit]').click();await page.waitForFunction(()=>pending.length===1);assert.equal(await page.locator('#ai-image-load').isDisabled(),true);
 await page.evaluate(()=>pending[0]({ok:false,status:409,json:async()=>({error:'配置已变化'})}));await idle(page);assert.match(await page.locator('#ai-error').textContent(),/配置已变化/);assert.equal(await page.locator('#ai-image-key-clear').isChecked(),true);
});
test('reset removes image secret and stale response cannot restore it',async t=>{
 const page=await fixture(t);await page.locator('#ai-image-load').click();await page.waitForFunction(()=>pending.length===1);await page.evaluate(()=>{resetAI();authenticated=false;pending[0]({ok:true,status:200,json:async()=>({revision:'b'.repeat(64),mode:'openai',model:'late',url:'http://localhost',keyConfigured:true})});});await page.waitForTimeout(50);assert.equal(await page.locator('#ai-image-form').isHidden(),true);assert.equal(await page.locator('#ai-image-form input').count(),0);
});
test('image form fits mobile and desktop themes',async t=>{
 const page=await fixture(t);await page.addStyleTag({content:await fs.readFile(root+'style.css','utf8')+'\n'+await fs.readFile(root+'appearance.css','utf8')});await fs.mkdir('.local/aiimage-admin-ui',{recursive:true});
 await page.locator('#ai-image-form details').evaluate(el=>el.open=true);for(const width of [320,1280]){await page.setViewportSize({width,height:900});await page.locator('#ai-image-form').scrollIntoViewIfNeeded();for(const theme of ['light','dark']){await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:'.local/aiimage-admin-ui/'+width+'-'+theme+'.png'});}}
});

test('quality, dimensions and monthly budget are explicit and saved',async t=>{
 const page=await fixture(t);assert.equal(await page.locator('#ai-image-quality').inputValue(),'medium');assert.equal(await page.locator('#ai-field-image-budget').inputValue(),'100');
 await page.selectOption('#ai-image-quality','high');await page.selectOption('#ai-image-size','1536x1024');await page.locator('#ai-field-image-budget').fill('80');await reply(page,{...settings,quality:'high',size:'1536x1024',monthlyBudgetUSD:'80'});await page.locator('#ai-image-form button[type=submit]').click();await idle(page);
 const body=await page.evaluate(()=>requests[0].body);assert.equal(body.quality,'high');assert.equal(body.size,'1536x1024');assert.equal(body.monthlyBudgetUSD,'80');
});
test('exhausted budget, unknown price and zero limit are clearly presented',async t=>{
 const page=await fixture(t);await page.evaluate(v=>renderAIImage({...v,monthlyBudgetUSD:'0',pricingReady:false,pricingNote:'价格未知，已停止调用',budget:{...v.budget,chargedUSD:'100.000000',remainingUSD:'0.000000',stopped:true}}),settings);
 assert.match(await page.locator('#ai-image-budget-status').textContent(),/已停止生成/);assert.match(await page.locator('#ai-image-pricing-status').textContent(),/价格未知/);assert.equal(await page.locator('#ai-field-image-budget').inputValue(),'0');
});


test('2.5 models show verified estimates and preserve medium and 100 dollars',async t=>{
 const page=await fixture(t);for(const model of ['gpt-image-2.5-flare','gpt-image-2.5-sunburst']){
  await page.evaluate(v=>renderAIImage({...v,pricingReady:true,pricingNote:'GPT Image 2.5官方计算器估价 + 20%输出余量',url:'https://api.openai.com/v1/images/generations'}),{...settings,model});
  assert.equal(await page.locator('#ai-image-quality').inputValue(),'medium');assert.equal(await page.locator('#ai-field-image-budget').inputValue(),'100');assert.equal(await page.locator('#ai-field-image-custom-cost').isDisabled(),true);
  await page.locator('#ai-image-form details').evaluate(el=>el.open=true);const text=await page.locator('#ai-image-form details').textContent();assert.match(text,/0\.01317/);assert.match(text,/439/);assert.doesNotMatch(await page.locator('#ai-image-form').textContent(),/暂不放行|缺少已核实单张估价/);
  await reply(page,{...settings,model,pricingReady:true});await page.locator('#ai-image-form button[type=submit]').click();await idle(page);
 }
 const requests=await page.evaluate(()=>window.requests);assert.equal(requests.length,2);for(const req of requests){assert.equal(req.body.quality,'medium');assert.equal(req.body.monthlyBudgetUSD,'100');assert.equal(req.body.customCostUSD,'');}
});


test('2K 4K advanced options serialize without changing keys or budget',async t=>{
 const page=await fixture(t);await page.selectOption('#ai-image-size','3840x2160');await page.selectOption('#ai-image-quality','max');await page.selectOption('#ai-image-background','transparent');await page.selectOption('#ai-image-format','webp');await page.locator('#ai-field-image-compression').fill('80');await page.locator('#ai-field-image-count').fill('2');await page.locator('#ai-field-image-stream').check();await page.locator('#ai-field-image-partial').fill('3');await page.locator('#ai-field-image-proxy').fill('http://127.0.0.1:7897');await page.locator('#ai-field-image-timeout').fill('900');
 await reply(page,{...settings,size:'3840x2160',quality:'max'});await page.locator('#ai-image-form button[type=submit]').click();await idle(page);const body=await page.evaluate(()=>requests[0].body);
 assert.equal(body.size,'3840x2160');assert.equal(body.quality,'max');assert.equal(body.monthlyBudgetUSD,'100');assert.deepEqual(body.key,{action:'keep'});assert.equal(body.options.count,2);assert.equal(body.options.partialImages,3);assert.equal(body.options.proxyURL,'http://127.0.0.1:7897');assert.equal(body.options.timeoutSeconds,900);assert.equal(body.options.background,'transparent');assert.equal(body.options.format,'webp');
});
test('custom dimensions auto quality and disabled compression controls',async t=>{
 const page=await fixture(t);assert.equal(await page.locator('#ai-field-image-compression').isDisabled(),true);assert.equal(await page.locator('#ai-field-image-partial').isDisabled(),true);await page.selectOption('#ai-image-size','custom');await page.locator('#ai-field-image-size-custom').fill('1920x1088');await page.selectOption('#ai-image-quality','auto');await reply(page,{...settings,size:'1920x1088',quality:'auto'});await page.locator('#ai-image-form button[type=submit]').click();await idle(page);assert.equal(await page.evaluate(()=>requests[0].body.size),'1920x1088');assert.equal(await page.locator('#ai-image-size').inputValue(),'custom');assert.equal(await page.locator('#ai-field-image-size-custom').inputValue(),'1920x1088');
});


test('reference input reserve is visible and saved independently of monthly budget',async t=>{
 const page=await fixture(t);assert.equal(await page.locator('#ai-field-image-reference-reserve').inputValue(),'0.25');await page.locator('#ai-field-image-reference-reserve').fill('0.5');await reply(page,{...settings,options:{referenceReserveUSD:'0.5'}});await page.locator('#ai-image-form button[type=submit]').click();await idle(page);const body=await page.evaluate(()=>requests[0].body);assert.equal(body.options.referenceReserveUSD,'0.5');assert.equal(body.monthlyBudgetUSD,'100');assert.deepEqual(body.key,{action:'keep'});
});
