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
 await page.evaluate(value=>{authenticated=true;csrf='synthetic';window.showOverview=()=>{};window.requests=[];window.replies=[];window.pending=[];document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;document.getElementById('overview-view').hidden=true;document.getElementById('updatewatch-view').hidden=false;window.fetch=async(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const v=replies.shift();if(v)return {ok:!v.error,status:v.status||200,json:async()=>v};return new Promise(resolve=>pending.push(resolve));};renderUpdateWatch(value);showWorkspaceView("updatewatch");document.getElementById('uw-fields').disabled=false;},state);
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
