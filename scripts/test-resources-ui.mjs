import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
import test,{after} from 'node:test';
const require=createRequire(import.meta.url);
const {chromium}=require(path.join(process.env.LOCALADMIN_NODE_MODULES||path.join(process.env.USERPROFILE,'.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules'),'playwright'));
const root='internal/localadmin/assets/';
const html=(await fs.readFile(root+'index.html','utf8')).replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,'').replace(/<link\b[^>]*>/g,'');
await fs.mkdir('.local/resource-readonly/downloads',{recursive:true});
const browser=await chromium.launch({channel:'chrome',headless:true,downloadsPath:path.resolve('.local/resource-readonly/downloads')});after(()=>browser.close());
const report={startedAt:'2026-10-25T12:00:00Z',completedAt:'2026-10-25T12:00:01Z',groups:[{name:'塔罗',expected:2,valid:1,missing:1,damaged:0,unreadable:0,bytes:100,files:[{path:'<img src=x onerror=alert(1)>',status:'缺失',bytes:0}]}]};
async function fixture(t){
 const c=await browser.newContext();t.after(()=>c.close());await c.route('**/*',r=>r.abort());const p=await c.newPage();await p.setContent(html);
 for(const f of ['app.js','resources.js'])await p.addScriptTag({content:await fs.readFile(root+f,'utf8')});
 await p.evaluate(()=>{authenticated=true;csrf='synthetic';window.showOverview=()=>{};document.getElementById('auth').hidden=true;document.getElementById('workspace').hidden=false;window.requests=[];window.replies=[{busy:false,report:null}];window.pending=[];window.fetch=async(url,options)=>{requests.push({url,body:options.body?JSON.parse(options.body):null});const v=replies.shift();if(v)return {ok:!v.error,status:v.error?409:200,json:async()=>v};return new Promise(resolve=>pending.push(resolve));};showWorkspaceView('resources');});
 await p.waitForFunction(()=>!resourcesBusy);return p;
}
test('entry reads only; explicit check; errors preserve previous report and escape paths',async t=>{
 const p=await fixture(t);assert.deepEqual(await p.evaluate(()=>requests),[{url:'/api/resources',body:null}]);
 await p.evaluate(report=>replies.push({busy:false,report}),report);await p.locator('#resources-check').click();await p.waitForFunction(()=>!resourcesBusy);
 assert.deepEqual(await p.evaluate(()=>requests[1]),{url:'/api/resources/check',body:{}});assert.match(await p.locator('#resources-result').textContent(),/缺失 1/);assert.equal(await p.locator('#resources-result img').count(),0);
 const before=await p.locator('#resources-result').textContent();await p.evaluate(()=>replies.push({error:'正在检查'}));await p.locator('#resources-check').click();await p.waitForFunction(()=>!resourcesBusy);assert.equal(await p.locator('#resources-result').textContent(),before);assert.match(await p.locator('#resources-status').textContent(),/未完成/);
});
test('busy gates duplicate requests; logout ignores late response',async t=>{
 const p=await fixture(t);await p.locator('#resources-check').click();await p.waitForFunction(()=>pending.length===1);assert.equal(await p.locator('#resources-check').isDisabled(),true);await p.evaluate(()=>loadResources(true));assert.equal(await p.evaluate(()=>requests.length),2);
 await p.evaluate(()=>{authenticated=false;resetResources();authenticated=true;});await p.evaluate(report=>pending[0]({ok:true,json:async()=>({busy:false,report})}),report);await p.waitForTimeout(30);assert.equal(await p.locator('#resources-result').textContent(),'');
});
test('leaving ignores old response; server busy allows read refresh',async t=>{
 const p=await fixture(t);await p.locator('#resources-check').click();await p.waitForFunction(()=>pending.length===1);await p.evaluate(()=>{showWorkspaceView('overview');replies.push({busy:true,report:null});showWorkspaceView('resources');});await p.waitForFunction(()=>!resourcesBusy);
 await p.evaluate(report=>pending[0]({ok:true,json:async()=>({busy:false,report})}),report);await p.waitForTimeout(30);assert.equal(await p.locator('#resources-result').textContent(),'');assert.equal(await p.locator('#resources-check').isDisabled(),true);assert.equal(await p.locator('#resources-refresh').isEnabled(),true);
});

test('anomaly filter and complete export reuse snapshot without scanning',async t=>{
 const p=await fixture(t);assert.equal(await p.locator('#resources-export').isDisabled(),true);
 const value={...report,groups:[{...report.groups[0],files:[{path:'normal.png',status:'正常',bytes:1},{path:'large.png',status:'无法读取',bytes:2,reason:'像素数超过4000万检查上限'}]}]};
 await p.evaluate(value=>renderResourceReport({busy:false,report:value}),value);
 assert.equal(await p.locator('#resources-result li').count(),1);assert.match(await p.locator('#resources-result').textContent(),/4000万/);
 await p.locator('#resources-only-issues').uncheck();assert.equal(await p.locator('#resources-result li').count(),2);
 await p.locator('#resources-only-issues').check();
 const download= p.waitForEvent('download');await p.locator('#resources-export').click();const d=await download;const output=JSON.parse(await fs.readFile(await d.path(),'utf8'));
 assert.equal(output.format,'zerobot-resource-check');assert.deepEqual(output.report,value);assert.equal(output.report.groups[0].files.length,2);assert.equal(await p.evaluate(()=>requests.length),1);
 await p.evaluate(()=>{authenticated=false;resetResources()});assert.equal(await p.locator('#resources-export').isDisabled(),true);assert.equal(await p.evaluate(()=>resourcesSnapshot),null);
});
test('failed refresh keeps snapshot warning during filtering and export',async t=>{
 const p=await fixture(t);await p.evaluate(value=>renderResourceReport({busy:false,report:value}),report);
 await p.evaluate(()=>replies.push({error:'读取失败'}));await p.locator('#resources-refresh').click();await p.waitForFunction(()=>!resourcesBusy);
 await p.locator('#resources-only-issues').uncheck();assert.match(await p.locator('#resources-status').textContent(),/旧结果不代表本次/);
 assert.equal(await p.locator('#resources-export').isEnabled(),true);
 await p.locator('#resources-check').click();await p.waitForFunction(()=>pending.length===1);assert.equal(await p.locator('#resources-export').isDisabled(),true);
});

test('guidance distinguishes every existing reason without changing report or issuing requests',async t=>{
 const p=await fixture(t);
 const cases=[
  ['缺失','文件或上级目录不存在','上级目录是否存在'],
  ['损坏','图像解码失败或格式不受支持，需人工核对','实际编码'],
  ['无法读取','没有读取权限','运行账户'],
  ['无法读取','路径包含链接，未跟随读取','不绕过链接检查'],
  ['无法读取','目标不是普通文件','不直接删除'],
  ['无法读取','路径不在允许范围','相对路径'],
  ['无法读取','文件超过读取大小上限（清单4 MiB，图片8 MiB）','不自动压缩或替换'],
  ['无法读取','像素数超过4000万检查上限，未完整解码，不代表文件损坏','不代表损坏'],
  ['无法读取','新的未知原因','原因尚未明确'],
  ['无法读取',null,'原因尚未明确']
 ];
 const value={...report,groups:[{...report.groups[0],id:'tarot',files:cases.map(([status,reason],i)=>({path:`case${i}.png`,status,reason,bytes:1})).concat([{path:'ok.png',status:'正常',bytes:1}])}]};
 await p.evaluate(value=>renderResourceReport({busy:false,report:value}),value);
 const items=await p.locator('.resource-file-advice').allTextContents();assert.equal(items.length,cases.length);
 cases.forEach((c,i)=>assert.ok(items[i].includes(c[2]),c[1]));
 assert.match(await p.locator('.resource-maintenance-advice').textContent(),/python scripts\/cache-tarot-images.py/);
 assert.match(await p.locator('.resource-maintenance-advice').textContent(),/可能联网下载并替换/);
 await p.locator('#resources-only-issues').uncheck();assert.equal(await p.locator('.resource-file-advice').count(),cases.length);
 assert.deepEqual(await p.evaluate(()=>resourcesSnapshot.report),value);assert.equal(await p.evaluate(()=>requests.length),1);
 const download=p.waitForEvent('download');await p.locator('#resources-export').click();const d=await download;
 assert.deepEqual(JSON.parse(await fs.readFile(await d.path(),'utf8')).report,value);
 assert.equal(await p.locator('#resources-result button, #resources-result a').count(),0);
 await p.evaluate(()=>{authenticated=false;resetResources()});assert.equal(await p.locator('.resource-file-advice').count(),0);
});

test('manifest, wife and oversized-only groups never suggest tarot repair',async t=>{
 const p=await fixture(t);
 const large={path:'large.png',status:'无法读取',reason:'像素数超过4000万检查上限，未完整解码，不代表文件损坏',bytes:1};
 const values=[
  [{...report.groups[0],id:'wife'},'没有已确认可复用的一键修复入口'],
  [{...report.groups[0],id:'tarot',error:'资源清单无效'},'不要依据本次结果批量补图'],
  [{...report.groups[0],id:'unknown'},'没有已确认的修复入口'],
 ];
 for(const [group,text] of values){
  await p.evaluate(group=>renderResourceReport({busy:false,report:{groups:[group]}}),group);
  const rendered=await p.locator('#resources-result').textContent();assert.ok(rendered.includes(text));assert.ok(!rendered.includes('python scripts/'));
 }
 await p.evaluate(group=>renderResourceReport({busy:false,report:{groups:[group]}}),{...report.groups[0],id:'tarot',files:[large]});
 assert.equal(await p.locator('.resource-maintenance-advice').count(),0);assert.match(await p.locator('.resource-file-advice').textContent(),/保留原件/);
 assert.equal(await p.evaluate(()=>requests.length),1);
});
