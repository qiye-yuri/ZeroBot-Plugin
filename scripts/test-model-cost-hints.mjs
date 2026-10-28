// Offline synthetic usage only: no provider, QQ or pricing API calls.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
function fixture(){
 const node=()=>({children:[],dataset:{},_text:'',get textContent(){return this._text+this.children.map(c=>c.textContent).join('');},set textContent(v){this._text=v;this.children=[];},append(...items){this.children.push(...items);},replaceChildren(...items){this._text='';this.children=items;},querySelectorAll(){return [];}});
 const nodes=new Map();const $=id=>{if(!nodes.has(id))nodes.set(id,node());return nodes.get(id)};
 const c=vm.createContext({$,document:{addEventListener(){},createElement:node,createDocumentFragment:node},Number,Date,setTimeout,clearTimeout});
 vm.runInContext(fs.readFileSync('internal/localadmin/assets/overview.js','utf8'),c);return {c,$};
}

function taskTableText(c,value,groups=value?.groups){
 const intro=c.modelTaskUsageText(value,groups);if(!value?.count)return intro;
 const table=c.modelTaskUsageTable(value,groups),rows=table.children[2].children;
 return intro+'\n'+rows.map(row=>row.children.slice(1).map((cell,i)=>['匹配任务','全窗口'][i]+'已知'+row.children[0].textContent+'：'+cell.children[0].textContent+'（'+cell.children[1].textContent+'）。').join('\n')).join('\n');
}

const row=(input,hit,miss,output=10)=>({id:1,model:'fixture',usage:{input,hit,miss,output}});

test('output sorting uses known partial totals, stable ties and exact large sums after group filters',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-28',checkedAt:'2026-10-29'},settings={large:100,minimum:50,missPercent:80};
 const r=(id,task,output,input=1,outcome='completed')=>({...row(input,input,0,output),id,session:'a'.repeat(24),task:task.repeat(24),outcome});
 const rows=[r(1,'a',10,100),r(2,'a',null,1,'failed'),r(3,'b',0),r(4,'c',null),r(5,'d',20),r(6,'e',10),r(7,'f',null)];
 const v=c.modelTaskUsage(rows,meta),before=JSON.stringify(v),ids=gs=>Array.from(gs,g=>g.requests[0].id);
 const sorted=c.selectModelTaskGroups(v,'all','output',settings);
 assert.deepEqual(ids(sorted),[5,1,6,3,4,7]);assert.equal(v.groups[0].totals.output.known,1);
 assert.match(c.modelTaskSummaryText(v.groups[0]),/输出 10（已知1\/2次）/);
 assert.deepEqual(ids(c.selectModelTaskGroups(v,'anyRisk','output',settings,'failed','multiple')),[1]);
 assert.match(c.modelTaskUsageText(v,c.selectModelTaskGroups(v,'anyRisk','output',settings,'failed','multiple')),/窗口请求 2\/7次/);
 assert.equal(JSON.stringify(v),before);assert.deepEqual(Array.from(sorted[1].requests,r=>r.id),[1,2]);
 const n=Number.MAX_SAFE_INTEGER,large=c.modelTaskUsage([r(1,'a',n),r(2,'a',1),r(3,'b',n),r(4,'b',2)],meta);
 assert.deepEqual(ids(c.selectModelTaskGroups(large,'all','output')),[3,1]);
 const singles=c.modelTaskUsage([{...row(1,1,0,2),id:1},{...row(1,1,0,3),id:1},r(3,'a',4),{...r(4,'a',5),session:'b'.repeat(24)}],meta);
 assert.deepEqual(Array.from(c.selectModelTaskGroups(singles,'all','output'),g=>g.totals.output.value),['5','4','3','2']);
 const window=c.modelTaskUsage(Array.from({length:257},(_,i)=>({...row(1,1,0,257-i),id:i})),meta);
 assert.equal(window.count,256);assert.equal(c.selectModelTaskGroups(window,'all','output')[0].requests[0].id,1);
});
test('threshold boundaries and unknown amount',()=>{
 const {c}=fixture();const s=c.modelCostHints([row(100000,20000,80000),row(99999,80000,19999),row(10000,2000,8000),row(9999,0,9999)]);
 assert.match(s,/大输入1次，高未命中2次/);assert.match(s,/输入最多：#1 fixture，输入100000/);assert.match(s,/金额未知/);
});
test('missing fields never become zero; inconsistent totals excluded',()=>{
 const {c}=fixture();const s=c.modelCostHints([row(200000,null,200000),row(200000,0,300000),row(200000,0,200000,null)]);
 assert.match(s,/高未命中0次/);assert.match(s,/字段不完整2次/);assert.match(s,/不一致1次/);
});
test('zero usage and empty window do not claim zero cost',()=>{
 const {c}=fixture();assert.match(c.modelCostHints([]),/费用未知/);assert.match(c.modelCostHints([row(0,0,0,0)]),/高未命中0次/);
});
test('unsafe counters and negative fields excluded',()=>{
 const {c}=fixture();assert.match(c.modelCostHints([row(100000,-1,100001),row(Number.MAX_SAFE_INTEGER+1,0,0)]),/字段不完整2次/);
});
test('render reuses bounded recent window, clears stale hints, preserves inputs',()=>{
 const {c,$}=fixture();const rows=Array.from({length:260},()=>row(100000,0,100000));const before=JSON.stringify(rows);c.renderModelUsage(rows);
 assert.match($('model-cost-hints').textContent,/最近256次/);assert.equal(JSON.stringify(rows),before);c.clearModelRequests();assert.equal($('model-cost-hints').textContent,'费用风险提示待更新。');
});
test('model names are text only and no bill claim without pricing',()=>{
 const {c,$}=fixture();c.renderModelUsage([{...row(100000,0,100000),model:'<img src=x onerror=alert(1)>'}]);assert.match($('model-cost-hints').textContent,/<img/);assert.match($('model-cost-hints').textContent,/未配置可核对的单价/);
});

test('group by exact provider and model with independent coverage',()=>{
 const {c}=fixture();const r=c.modelUsageBreakdown([
 {...row(100,90,10),provider:'one'}, {...row(900,0,900,null),provider:'one'},
 {...row(100,0,100),provider:'two'}, {...row(null,null,null,null),provider:'one',model:'other'}]);
 assert.equal(r.groups.length,3);const g=r.groups[0];assert.equal(g.count,2);assert.equal(g.totals.input.value,'1000');assert.equal(g.totals.output.value,'10');assert.equal(g.totals.output.known,1);assert.equal(g.hitRate,'9.00%');
 const unknown=r.groups.find(g=>g.model==='other');assert.equal(unknown.totals.input.value,null);assert.equal(unknown.hitRate,null);
});
test('inconsistent cache and zero total do not invent hit rates',()=>{
 const {c}=fixture();const r=c.modelUsageBreakdown([row(100,90,90),row(0,0,0)]);assert.equal(r.groups[0].inconsistent,1);assert.equal(r.groups[0].cacheCount,1);assert.equal(r.groups[0].hitRate,null);
});
test('large aggregate remains exact beyond safe numeric sum',()=>{
 const {c}=fixture();const n=Number.MAX_SAFE_INTEGER;const g=c.modelUsageBreakdown([row(n,n,0),row(n,n,0)]).groups[0];assert.equal(g.totals.input.value,'18014398509481982');assert.equal(g.hitRate,'100.00%');
});
test('top five includes older and failed requests; stable recent tie break',()=>{
 const {c}=fixture();const rows=Array.from({length:30},(_,i)=>({...row(i===0?1000:100,0,i===0?1000:100),id:i,outcome:i===0?'failed':'completed'}));const before=JSON.stringify(rows);const r=c.modelUsageBreakdown(rows);assert.deepEqual(Array.from(r.top,x=>x.id),[0,29,28,27,26]);assert.equal(r.top[0].outcome,'failed');assert.equal(JSON.stringify(rows),before);
});
test('bounded groups, unknown identities and clearing',()=>{
 const {c,$}=fixture();const rows=Array.from({length:260},()=>({...row(null,null,null,null),model:null}));const r=c.modelUsageBreakdown(rows);assert.equal(r.count,256);assert.equal(r.groups.length,1);assert.equal(r.top.length,0);
 c.renderModelUsage(rows);assert.match($('model-usage-breakdown').textContent,/已知0\/256/);c.clearModelRequests();assert.equal($('model-usage-breakdown').textContent,'分组用量待更新。');
});
test('group identities do not collide at separators or object keys',()=>{
 const {c}=fixture();const r=c.modelUsageBreakdown([{...row(1,1,0),provider:'a/b',model:'c'},{...row(1,1,0),provider:'a',model:'b/c'},{...row(1,1,0),provider:'__proto__',model:'constructor'}]);assert.equal(r.groups.length,3);
});

test('threshold parsing rejects empty, fractional, exponent and out-of-range drafts',()=>{
 const {c}=fixture();const good={large:'100000',minimum:'10000',missPercent:'80'};
 for(const field of Object.keys(good))for(const v of ['', '0','-1','1.2','1e2','Infinity','1000000001'])assert.equal(c.parseModelCostSettings({...good,[field]:v}),null);
 assert.equal(c.parseModelCostSettings({...good,missPercent:'101'}),null);
 assert.equal(c.parseModelCostSettings({...good,large:' 1 ',minimum:'1000000000',missPercent:'100'}).large,1);
});
test('custom thresholds recalculate precisely at percent boundary',()=>{
 const {c}=fixture();const s=c.modelCostHints([row(100,75,25),row(99,75,24)],{large:100,minimum:100,missPercent:25});assert.match(s,/大输入1次，高未命中1次/);assert.match(s,/输入≥100且未命中占比≥25%/);
});
test('settings only recalculate existing data; invalid drafts and logout preserve boundaries',()=>{
 const {c,$}=fixture();c.authenticated=true;c.renderModelUsage([row(200000,0,200000)]);
 $('cost-large').value='300000';$('cost-minimum').value='300000';$('cost-miss-percent').value='90';c.applyModelCostSettings();assert.match($('model-cost-hints').textContent,/大输入0次，高未命中0次/);
 $('cost-large').value='0';c.applyModelCostSettings();assert.match($('cost-settings-status').textContent,/未应用/);assert.match($('model-cost-hints').textContent,/输入≥300000/);
 c.clearModelRequests();$('cost-large').value='100';c.applyModelCostSettings();assert.equal($('model-cost-hints').textContent,'费用风险提示待更新。');assert.match($('cost-settings-status').textContent,/待更新/);
 c.authenticated=false;c.resetModelCostSettings(false);$('cost-large').value='1';c.applyModelCostSettings();assert.equal(vm.runInContext('modelCostSettings.large',c),100000);
});

test('usage export whitelists fields, preserves unknown and excludes future response content',()=>{
 const {c}=fixture();const meta={startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z',secret:'do-not-export'};
 const rows=[{...row(10,0,10,null),id:7,session:'a'.repeat(24),task:'raw-private-id',prompt:'secret-prompt',response:'secret-response',usage:{input:10,hit:0,miss:10,output:null,raw:'secret-usage'}}];const before=JSON.stringify(rows);
 const report=c.buildModelUsageReport(rows,meta,{large:100,minimum:10,missPercent:90},'2026-10-26T01:01:00Z');const serialized=JSON.stringify(report);
 assert.equal(report.requests[0].usage.hit,0);assert.equal(report.requests[0].usage.output,null);assert.equal(report.requests[0].session,'a'.repeat(24));assert.equal(report.requests[0].task,null);assert.equal(report.scope.amount,null);assert.equal(report.thresholds.large,100);assert.equal(report.snapshot.checkedAt,meta.checkedAt);assert.doesNotMatch(serialized,/secret-|do-not-export|raw-private/);assert.equal(JSON.stringify(rows),before);
});
test('usage export requires ordered snapshot time and bounds complete rows at 256',()=>{
 const {c}=fixture(),settings={large:100,minimum:10,missPercent:80};assert.equal(c.buildModelUsageReport([],null,settings,''),null);assert.equal(c.buildModelUsageReport([],{startedAt:'invalid',checkedAt:'invalid'},settings,''),null);assert.equal(c.buildModelUsageReport([],{startedAt:'2026-10-27',checkedAt:'2026-10-26'},settings,''),null);
 const rows=Array.from({length:260},(_,i)=>({...row(1,1,0),id:i}));const result=c.buildModelUsageReport(rows,{startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z'},settings,'');assert.equal(result.requests.length,256);assert.equal(result.requests[0].id,4);assert.equal(result.breakdown.count,256);
});

test('missing recent field disables export without treating it as empty window',()=>{
 const {c,$}=fixture();c.authenticated=true;const meta={startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z'};
 c.renderModelUsage([],meta);assert.equal($('model-usage-export').disabled,false);
 c.renderModelUsage(null,meta);assert.equal($('model-usage-export').disabled,true);assert.match($('model-usage-summary').textContent,/不代表零请求/);assert.equal(vm.runInContext('modelCostRows',c),null);
});

test('overview group and exported cache summary share valid-record rules',()=>{
 const {c,$}=fixture();const rows=[row(100,90,10,null),row(900,0,900),row(100,90,90,null),row(null,1,1)];
 const meta={startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z'};c.renderModelUsage(rows,meta);
 const cache=c.summarizeModelCache(rows),group=c.modelUsageBreakdown(rows).groups[0];const report=c.buildModelUsageReport(rows,meta,{large:100,minimum:10,missPercent:80},'');
 assert.equal(cache.valid,2);assert.equal(cache.inconsistent,1);assert.equal(cache.unknown,1);assert.equal(cache.hitRate,'9.00%');assert.equal(group.hitRate,cache.hitRate);assert.equal(report.cacheSummary.hitRate,cache.hitRate);assert.match($('model-usage-summary').textContent,/有效缓存统计2次.*分项不一致1次.*9.00%/);
 assert.match(c.modelCostHints(rows),/分项不一致1次/);
});
test('overview uses exact integer totals instead of overflowing accumulated numbers',()=>{
 const {c,$}=fixture();const n=Number.MAX_SAFE_INTEGER;c.renderModelUsage([row(n,n,0),row(n,n,0)]);assert.match($('model-usage-summary').textContent,/100.00%/);assert.equal(c.summarizeModelCache([row(n,n,0),row(n,n,0)]).hit,'18014398509481982');
});
test('no valid cache or zero denominators remain unknown',()=>{
 const {c}=fixture();assert.equal(c.summarizeModelCache([row(100,90,90)]).hit,null);assert.equal(c.summarizeModelCache([row(0,0,0)]).hit,'0');assert.equal(c.summarizeModelCache([row(0,0,0)]).hitRate,null);
});
test('malformed request entries clear only usage and prevent misleading export',()=>{
 const {c,$}=fixture();c.authenticated=true;const meta={startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z'};
 for(const bad of [null,[],17,'broken']){
  c.renderModelUsage([row(10,1,9)],meta);$('model-request-summary').textContent='valid cumulative counts';c.renderModelUsage([row(10,1,9),bad],meta);assert.equal($('model-request-summary').textContent,'valid cumulative counts');assert.equal($('model-usage-export').disabled,true);assert.equal($('model-usage-recent').textContent,'');assert.equal(c.buildModelUsageReport([bad],meta,{large:1,minimum:1,missPercent:1},''),null);
 }
});

test('task grouping isolates sessions and leaves missing identities independent',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z'};
 const a='a'.repeat(24),b='b'.repeat(24),task='c'.repeat(24);
 const rows=[{...row(10,8,2),id:1,session:a,task},{...row(20,10,10),id:2,session:a,task,model:'other'},{...row(30,0,30),id:3,session:b,task},{...row(1,0,1),id:4},{...row(2,0,2),id:5,session:a,task:'raw-id'}];
 const before=JSON.stringify(rows),v=c.modelTaskUsage(rows,meta);assert.equal(v.groups.length,4);assert.equal(v.groups[0].count,2);assert.equal(v.groups[0].totals.input.value,'30');assert.equal(v.groups[0].cacheSummary.hitRate,'60.00%');assert.equal(v.complete,false);assert.equal(v.groups[2].grouped,false);assert.equal(v.groups[3].task,null);assert.equal(JSON.stringify(rows),before);
 const next=c.modelTaskUsage([rows[0]],{...meta,startedAt:'2026-10-26T00:30:00Z'});assert.equal(next.count,1);assert.notEqual(next.startedAt,v.startedAt);assert.equal(c.modelTaskUsage(rows,null),null);
});
test('task window preserves partial coverage and exact sums, never claims completion',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-26',checkedAt:'2026-10-27'},n=Number.MAX_SAFE_INTEGER;
 const rows=Array.from({length:260},(_,i)=>({...row(n,n,0,null),id:i,session:'a'.repeat(24),task:'b'.repeat(24)}));
 const value=c.modelTaskUsage(rows,meta);assert.equal(value.count,256);assert.equal(value.groups[0].requests[0].id,4);assert.equal(value.groups[0].totals.input.value,String(BigInt(n)*256n));assert.equal(value.groups[0].totals.output.value,null);assert.equal(value.groups[0].cacheSummary.hitRate,'100.00%');assert.match(c.modelTaskUsageText(value),/可能缺少早期轮次/);
 const bad=c.modelTaskUsage([row(10,8,8,null),row(null,null,null,null)],meta);assert.equal(bad.groups[0].cacheSummary.inconsistent,1);assert.equal(bad.groups[1].cacheSummary.hitRate,null);
});
test('task rendering clears on invalid snapshots and export keeps only allowed fields',()=>{
 const {c,$}=fixture(),meta={startedAt:'2026-10-26',checkedAt:'2026-10-27'};
 const rows=[{...row(10,8,2),session:'a'.repeat(24),task:'b'.repeat(24),prompt:'secret-prompt'}];c.renderModelUsage(rows,meta);assert.match($('model-task-usage').textContent,/窗口内1次/);
 const report=c.buildModelUsageReport(rows,meta,{large:100,minimum:10,missPercent:80},'');assert.equal(report.taskBreakdown.groups[0].totals.input.value,'10');assert.doesNotMatch(JSON.stringify(report.taskBreakdown),/secret-prompt|prompt/);
 c.renderModelUsage(null,meta);assert.equal($('model-task-usage').textContent,'任务用量未知。');c.clearModelRequests();assert.equal($('model-task-usage').textContent,'任务用量待更新。');
});


test('task risk filters use single-request thresholds and keep whole matching groups',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-26',checkedAt:'2026-10-27'},settings={large:100,minimum:50,missPercent:80};
 const r=(id,input,hit,miss,output=1,task='a')=>({...row(input,hit,miss,output),id,session:'b'.repeat(24),task:task.repeat(24)});
 const rows=[r(1,60,60,0),r(2,60,60,0),r(3,100,20,80,1,'c'),r(4,1,1,0,1,'c'),r(5,100,0,100,null,'d'),r(6,100,90,90,1,'e')];
 const v=c.modelTaskUsage(rows,meta),before=JSON.stringify(v),ids=g=>Array.from(g,x=>Array.from(x.requests,r=>r.id));
 assert.deepEqual(ids(c.selectModelTaskGroups(v,'large','original',settings)),[[3,4],[5],[6]]);
 assert.deepEqual(ids(c.selectModelTaskGroups(v,'highMiss','original',settings)),[[3,4]]);
 assert.deepEqual(ids(c.selectModelTaskGroups(v,'invalid','original',settings)),[[5],[6]]);
 assert.match(c.modelCostHints(rows,settings),/大输入3次，高未命中1次/);
 assert.equal(JSON.stringify(v),before);
 assert.match(c.modelTaskUsageText(v,[]),/当前条件无匹配任务/);assert.match(c.modelTaskUsageText(c.modelTaskUsage([],meta),[]),/暂无近期请求/);
});
test('task sorting uses exact partial totals, stable ties, and unknown after known zero',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-26',checkedAt:'2026-10-27'},n=Number.MAX_SAFE_INTEGER;
 const rows=[row(null,null,null),row(0,0,0),{...row(n,n,0),session:'a'.repeat(24),task:'b'.repeat(24)},{...row(n,n,0),session:'a'.repeat(24),task:'b'.repeat(24)},row(n,0,n),row(n,0,n)];rows.forEach((r,i)=>r.id=i+1);
 const v=c.modelTaskUsage(rows,meta),before=JSON.stringify(v);
 assert.deepEqual(Array.from(c.selectModelTaskGroups(v,'all','input'),g=>g.requests[0].id),[3,5,6,2,1]);
 assert.deepEqual(Array.from(c.selectModelTaskGroups(v,'all','miss'),g=>g.requests[0].id),[5,6,2,3,1]);assert.equal(JSON.stringify(v),before);
});

test('per-request reasons cover boundaries and retain multiple independent risks',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-26',checkedAt:'2026-10-27'},settings={large:100,minimum:50,missPercent:80};
 const cases=[
  [row(100,20,80),'大输入（单次输入达到已应用阈值100）；高未命中（输入达到50且未命中占比达到80%）'],
  [row(99,20,79),'未触发当前用量提示'],
  [row(50,10,40),'高未命中（输入达到50且未命中占比达到80%）'],
  [row(49,0,49),'未触发当前用量提示'],
  [row(0,0,0,0),'未触发当前用量提示'],
  [row(100,90,90,null),'大输入（单次输入达到已应用阈值100）；用量字段不完整；缓存分项与输入不一致'],
  [row(10,-1,11),'用量字段不完整'],
  [row(null,null,null,null),'用量字段不完整'],
  [{...row(1,1,0),outcome:'failed'},'未触发当前用量提示']
 ];
 for(const [r,reason] of cases){
  const text=c.modelTaskRequestsText(c.modelTaskUsage([r],meta).groups[0],settings);
  assert.equal(text.split('\n提示：')[1],reason);
  if(r.outcome)assert.match(text,/failed/);
 }
});

test('risk text explains each request without mutating groups or complete exports',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-26',checkedAt:'2026-10-27'},settings={large:100,minimum:50,missPercent:80};
 const rows=[60,60,100,1].map((n,i)=>({...row(n,n,0),id:i+1,session:'a'.repeat(24),task:(i<2?'b':'c').repeat(24)}));
 rows.push({...row(100,0,100),id:5,session:'d'.repeat(24),task:'c'.repeat(24)},{...row(100,0,100),id:6});
 const original=JSON.stringify(rows),v=c.modelTaskUsage(rows,meta),report=JSON.stringify(c.buildModelUsageReport(rows,meta,settings,'2026-10-27'));
 const all=v.groups.map(g=>c.modelTaskRequestsText(g,settings)).join('\n');
 assert.equal((all.match(/提示：未触发当前用量提示/g)||[]).length,3);
 const selected=c.selectModelTaskGroups(v,'large','input',settings),text=selected.map(g=>c.modelTaskSummaryText(g)+'\n'+c.modelTaskRequestsText(g,settings)).join('\n');
 assert.match(text,/#3 [\s\S]*提示：大输入[\s\S]*#4 [\s\S]*提示：未触发当前用量提示/);
 assert.doesNotMatch(text,/#1 /);assert.match(text,/独立请求/);assert.equal(selected.length,3);
 assert.equal(JSON.stringify(rows),original);assert.equal(JSON.stringify(c.buildModelUsageReport(rows,meta,settings,'2026-10-27')),report);
});

test('outcome and risk intersect at task scope, retaining every request',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-27',checkedAt:'2026-10-28'},settings={large:100,minimum:100,missPercent:80};
 const rows=[{...row(100,0,100),id:1,outcome:'completed',session:'a'.repeat(24),task:'b'.repeat(24)},{...row(1,1,0),id:2,outcome:'failed',session:'a'.repeat(24),task:'b'.repeat(24)},{...row(100,0,100),id:3,outcome:'completed',session:'c'.repeat(24),task:'b'.repeat(24)},{...row(1,1,0),id:4,outcome:'failed'},{...row(1,1,0),id:5,outcome:'incomplete'}];
 const original=JSON.stringify(rows),v=c.modelTaskUsage(rows,meta),select=(risk,result)=>c.selectModelTaskGroups(v,risk,'input',settings,result);
 const both=select('large','failed');assert.equal(both.length,1);assert.deepEqual(Array.from(both[0].requests,r=>r.id),[1,2]);assert.equal(both[0].totals.input.value,'101');
 assert.equal(select('all','completed').length,2);assert.equal(select('all','failed').length,2);assert.equal(select('all','incomplete').length,1);assert.equal(select('large','incomplete').length,0);assert.equal(select('all','all').length,4);assert.equal(JSON.stringify(rows),original);
});

test('unknown results are distinct from failed or prematurely closed requests',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-27',checkedAt:'2026-10-28'};
 const rows=[undefined,null,'','other','completed','failed','incomplete'].map((outcome,i)=>({...row(0,0,0,0),id:i+1,outcome}));const v=c.modelTaskUsage(rows,meta);
 assert.deepEqual(Array.from(c.selectModelTaskGroups(v,'all','original',undefined,'unknown'),g=>g.requests[0].id),[1,2,3,4]);
 assert.equal(c.selectModelTaskGroups(v,'all','original',undefined,'failed').length,1);assert.equal(c.selectModelTaskGroups(null,'all','original',undefined,'failed').length,0);
});

test('task summary counts complete group outcomes without changing report objects',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-27',checkedAt:'2026-10-28'};
 const rows=['completed','completed','failed','incomplete',undefined,null,'','COMPLETED','<script>'].map(outcome=>({...row(1,1,0),id:1,outcome,session:'a'.repeat(24),task:'b'.repeat(24)}));
 const v=c.modelTaskUsage(rows,meta),before=JSON.stringify(v),report=JSON.stringify(c.buildModelUsageReport(rows,meta,{large:100000,minimum:10000,missPercent:80},'2026-10-28'));
 assert.match(c.modelTaskSummaryText(v.groups[0]),/接口完成 2 · 失败 1 · 提前关闭 1 · 未知 5/);assert.doesNotMatch(c.modelTaskSummaryText(v.groups[0]),/<script>/);assert.equal(JSON.stringify(v),before);
 assert.deepEqual(Object.keys(v.groups[0]).sort(),['cacheSummary','count','grouped','requests','session','task','totals']);
 assert.equal(JSON.stringify(c.buildModelUsageReport(rows,meta,{large:100000,minimum:10000,missPercent:80},'2026-10-28')),report);
 const selected=c.selectModelTaskGroups(v,'all','original',undefined,'failed');assert.match(c.modelTaskSummaryText(selected[0]),/接口完成 2 · 失败 1/);
 const isolated=c.modelTaskUsage([{...rows[0],session:'c'.repeat(24)},{...rows[2]},{...rows[3],session:null}],meta);assert.equal(isolated.groups.length,3);assert.match(c.modelTaskSummaryText(isolated.groups[0]),/接口完成 1 · 失败 0 · 提前关闭 0 · 未知 0/);assert.match(c.modelTaskSummaryText(isolated.groups[2]),/接口完成 0 · 失败 0 · 提前关闭 1 · 未知 0/);
 const window=c.modelTaskUsage(Array.from({length:257},(_,i)=>({...rows[0],outcome:i===0?'failed':'completed'})),meta);assert.match(c.modelTaskSummaryText(window.groups[0]),/接口完成 256 · 失败 0/);
});

test('summary risk counts overlap, use per-request thresholds and preserve reports',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-27',checkedAt:'2026-10-28'},settings={large:100,minimum:100,missPercent:80};
 const rows=[row(100,0,100),row(1,1,0),row(100,0,100,null),row(10,0,11,null)].map((r,i)=>({...r,id:1,outcome:i===1?'failed':'completed',session:'a'.repeat(24),task:'b'.repeat(24)}));
 const v=c.modelTaskUsage(rows,meta),before=JSON.stringify(v),report=JSON.stringify(c.buildModelUsageReport(rows,meta,settings,'2026-10-28'));
 const selected=c.selectModelTaskGroups(v,'large','input',settings,'failed');assert.equal(selected.length,1);
 assert.match(c.modelTaskSummaryText(selected[0],settings),/大输入 2次 · 高未命中 1次 · 字段不完整 2次 · 分项不一致 1次/);
 assert.match(c.modelTaskSummaryText(selected[0],{...settings,large:101,minimum:101}),/大输入 0次 · 高未命中 0次 · 字段不完整 2次 · 分项不一致 1次/);
 assert.equal(JSON.stringify(v),before);assert.equal(JSON.stringify(c.buildModelUsageReport(rows,meta,settings,'2026-10-28')),report);
 const group=rs=>c.modelTaskUsage(rs.map(r=>({...r,session:'a'.repeat(24),task:'b'.repeat(24)})),meta).groups[0];
 assert.match(c.modelTaskSummaryText(group([row(60,60,0),row(60,60,0)]),settings),/大输入 0次 · 高未命中 0次 · 字段不完整 0次 · 分项不一致 0次/);
 assert.match(c.modelTaskSummaryText(group([row(-1,0,0),row(Number.MAX_SAFE_INTEGER+1,0,0)]),settings),/大输入 0次 · 高未命中 0次 · 字段不完整 2次/);
 const window=group([row(100,0,100),...Array.from({length:256},()=>row(0,0,0,0))]);assert.match(c.modelTaskSummaryText(window,settings),/大输入 0次 · 高未命中 0次/);
 const isolated=c.modelTaskUsage([rows[0],{...rows[1],session:'c'.repeat(24)},{...rows[3],session:null}],meta);assert.equal(isolated.groups.length,3);assert.match(c.modelTaskSummaryText(isolated.groups[1],settings),/大输入 0次 · 高未命中 0次/);assert.match(c.modelTaskSummaryText(isolated.groups[2],settings),/字段不完整 1次 · 分项不一致 1次/);
});

test('any risk selects the union once and intersects results at task scope',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-27',checkedAt:'2026-10-28'},settings={large:100,minimum:10,missPercent:80};
 const rows=[row(1,1,0),row(100,100,0),row(10,0,10),row(1,1,0,null),row(1,0,2),row(100,0,100)].map((r,i)=>({...r,id:i+1,outcome:'completed'}));
 const original=JSON.stringify(rows),v=c.modelTaskUsage(rows,meta),select=value=>c.selectModelTaskGroups(value,'anyRisk','original',settings);
 assert.deepEqual(Array.from(select(v),g=>g.requests[0].id),[2,3,4,5,6]);assert.equal(JSON.stringify(rows),original);
 const grouped=[{...rows[5],session:'a'.repeat(24),task:'b'.repeat(24)},{...rows[0],id:6,outcome:'failed',session:'a'.repeat(24),task:'b'.repeat(24)},{...rows[0],session:'c'.repeat(24),task:'b'.repeat(24)}];
 const g=c.modelTaskUsage(grouped,meta),before=JSON.stringify(g),selected=c.selectModelTaskGroups(g,'anyRisk','input',settings,'failed');assert.equal(selected.length,1);assert.equal(selected[0].requests.length,2);assert.match(c.modelTaskSummaryText(selected[0],settings),/接口完成 1 · 失败 1/);assert.equal(JSON.stringify(g),before);
 const small=c.modelTaskUsage([row(60,60,0),row(60,60,0)].map(r=>({...r,session:'a'.repeat(24),task:'b'.repeat(24)})),meta);assert.equal(select(small).length,0);
 const window=c.modelTaskUsage([rows[5],...Array.from({length:256},()=>rows[0])],meta);assert.equal(select(window).length,0);assert.equal(c.selectModelTaskGroups(v,'anyRisk','original',settings,'failed').length,0);
});

test('count sorting is stable after filtering and never mutates original groups',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-27',checkedAt:'2026-10-28'},settings={large:100,minimum:100,missPercent:80};
 const rows=[1,3,2,3].flatMap((n,i)=>Array.from({length:n},(_,j)=>({...row(i===3?200:100,0,i===3?200:100),id:1,session:String(i+1).repeat(24),task:'a'.repeat(24),outcome:j===1?'failed':'completed'})));const original=JSON.stringify(rows),v=c.modelTaskUsage(rows,meta),before=JSON.stringify(v);
 const ids=gs=>Array.from(gs,g=>g.session[0]);assert.deepEqual(ids(c.selectModelTaskGroups(v,'all','count')),['2','4','3','1']);assert.deepEqual(ids(c.selectModelTaskGroups(v,'anyRisk','count',settings,'failed')),['2','4','3']);assert.deepEqual(ids(c.selectModelTaskGroups(v,'all','original')),['1','2','3','4']);assert.equal(JSON.stringify(v),before);assert.equal(JSON.stringify(rows),original);
 const independent=c.modelTaskUsage([{...rows[0],session:null},{...rows[0],session:null}],meta);assert.equal(c.selectModelTaskGroups(independent,'all','count').length,2);
 const window=c.modelTaskUsage([...Array.from({length:255},()=>rows[0]),...Array.from({length:2},()=>rows[1])],meta);assert.deepEqual(Array.from(c.selectModelTaskGroups(window,'all','count'),g=>g.count),[254,2]);
});

test('count filtering intersects whole groups, preserves ordering and follows retained window',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-28',checkedAt:'2026-10-29'},settings={large:100,minimum:100,missPercent:80};
 const rows=[1,2,3,2].flatMap((n,i)=>Array.from({length:n},(_,j)=>({...row(i===2?null:j===0?100:1,j===0?0:1,j===0?100:0),id:i*10+j+1,session:String(i+1).repeat(24),task:'a'.repeat(24),outcome:j===1?'failed':i===3?'other':'completed'})));
 const original=JSON.stringify(rows),v=c.modelTaskUsage(rows,meta),before=JSON.stringify(v),ids=gs=>Array.from(gs,g=>g.session[0]);
 const select=(count,sort='original',risk='all',outcome='all')=>c.selectModelTaskGroups(v,risk,sort,settings,outcome,count);
 assert.deepEqual(ids(select('all')),['1','2','3','4']);assert.deepEqual(ids(select('single')),['1']);assert.deepEqual(ids(select('multiple')),['2','3','4']);
 assert.deepEqual(ids(select('multiple','count')),['3','2','4']);assert.deepEqual(ids(select('multiple','input')),['2','4','3']);assert.deepEqual(ids(select('multiple','miss')),['2','3','4']);
 const matched=select('multiple','original','anyRisk','failed');assert.deepEqual(ids(matched),['2','3','4']);assert.equal(matched[0].requests.length,2);assert.equal(matched[0].requests[0].outcome,'completed');
 assert.deepEqual(ids(select('multiple','original','anyRisk','unknown')),['4']);assert.equal(select('single','original','anyRisk','failed').length,0);
 assert.equal(JSON.stringify(rows),original);assert.equal(JSON.stringify(v),before);
 const isolated=c.modelTaskUsage([{...rows[0],session:null},{...rows[0],session:null},{...rows[0],session:'f'.repeat(24)},rows[0]],meta);assert.equal(c.selectModelTaskGroups(isolated,'all','original',settings,'all','single').length,4);
 const window=c.modelTaskUsage([rows[1],...Array.from({length:255},()=>rows[0]),rows[2]],meta);assert.deepEqual(Array.from(c.selectModelTaskGroups(window,'all','original',settings,'all','single'),g=>g.count),[1]);assert.equal(c.selectModelTaskGroups(window,'all','original',settings,'all','single')[0].session,rows[1].session);
});

test('matching request coverage sums complete selected groups once within the retained window',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-28',checkedAt:'2026-10-29'},settings={large:100,minimum:100,missPercent:80};
 const rows=[1,2,3].flatMap((n,i)=>Array.from({length:n},(_,j)=>({...row(i===1&&j===0?100:1,i===1&&j===0?0:1,i===1&&j===0?100:0),id:i*10+j+1,session:String(i+1).repeat(24),task:'a'.repeat(24),outcome:j===1?'failed':'completed'})));
 const v=c.modelTaskUsage(rows,meta),original=JSON.stringify(v),text=(count='all',risk='all',outcome='all',sort='original')=>c.modelTaskUsageText(v,c.selectModelTaskGroups(v,risk,sort,settings,outcome,count));
 assert.match(text(),/匹配任务 3\/3；匹配任务内窗口请求 6\/6次/);assert.match(text('single'),/匹配任务 1\/3；匹配任务内窗口请求 1\/6次/);assert.match(text('multiple'),/匹配任务 2\/3；匹配任务内窗口请求 5\/6次/);
 for(const sort of ['original','count','input','miss'])assert.match(text('multiple','anyRisk','failed',sort),/匹配任务 1\/3；匹配任务内窗口请求 2\/6次/);
 assert.match(text('single','anyRisk'),/匹配任务 0\/3；匹配任务内窗口请求 0\/6次[\s\S]*当前条件无匹配任务/);assert.equal(JSON.stringify(v),original);
 const independent=c.modelTaskUsage([{...rows[0],session:null,usage:{}},{...rows[0],session:null,usage:{}},{...rows[0],session:'f'.repeat(24)},rows[0]],meta);assert.match(c.modelTaskUsageText(independent),/匹配任务 4\/4；匹配任务内窗口请求 4\/4次/);
 const window=c.modelTaskUsage([rows[1],...Array.from({length:255},()=>rows[0]),rows[2]],meta);assert.match(c.modelTaskUsageText(window,c.selectModelTaskGroups(window,'all','original',settings,'all','single')),/匹配任务 1\/2；匹配任务内窗口请求 1\/256次/);
 assert.doesNotMatch(c.modelTaskUsageText(null),/匹配任务内/);assert.doesNotMatch(c.modelTaskUsageText(c.modelTaskUsage([],meta)),/匹配任务内/);
});

test('association filtering distinguishes single linked tasks and retains whole group intersections',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-28',checkedAt:'2026-10-29'},settings={large:100,minimum:100,missPercent:80};
 const r=(id,task,input,output,outcome='completed')=>({...row(input,input,0,output),id,session:'a'.repeat(24),task:task.repeat(24),outcome});
 const rows=[r(1,'b',1,20),r(2,'c',100,10),r(3,'c',1,1,'failed'),{...r(4,'d',2,0),task:null},{...r(5,'e',3,null),session:null}];
 const v=c.modelTaskUsage(rows,meta),before=JSON.stringify(v),ids=gs=>Array.from(gs,g=>Array.from(g.requests,r=>r.id));
 const select=(association,count='all',risk='all',outcome='all',sort='original')=>c.selectModelTaskGroups(v,risk,sort,settings,outcome,count,association);
 assert.deepEqual(ids(select('all')),[[1],[2,3],[4],[5]]);assert.deepEqual(ids(select('linked')),[[1],[2,3]]);assert.deepEqual(ids(select('independent')),[[4],[5]]);
 assert.deepEqual(ids(select('linked','single')),[[1]]);assert.deepEqual(ids(select('linked','multiple')),[[2,3]]);assert.equal(select('independent','multiple').length,0);
 assert.match(c.modelTaskUsageText(v,select('linked')),/匹配任务 2\/4；匹配任务内窗口请求 3\/5次/);assert.match(c.modelTaskUsageText(v,select('independent')),/窗口请求 2\/5次/);assert.match(c.modelTaskUsageText(v,select('independent','multiple')),/匹配任务 0\/4；匹配任务内窗口请求 0\/5次/);
 for(const sort of ['original','count','input','miss','output'])assert.deepEqual(ids(select('linked','multiple','anyRisk','failed',sort)),[[2,3]]);
 assert.deepEqual(ids(select('linked','all','all','all','output')),[[1],[2,3]]);assert.deepEqual(ids(select('linked','all','all','all','input')),[[2,3],[1]]);
 assert.equal(JSON.stringify(v),before);assert.deepEqual(ids(c.selectModelTaskGroups(v)),ids(select('all')));
});

test('association classification reuses exact group identities and current bounded snapshot',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-28',checkedAt:'2026-10-29'},base={...row(1,1,0),id:1,session:'a'.repeat(24),task:'b'.repeat(24)};
 const rows=[base,{...base,id:2,session:'c'.repeat(24)},{...base,id:3,task:'B'.repeat(24)},{...base,session:null},{...base,task:null},{...base,task:'invalid'},{...base,session:'invalid'}];
 const select=v=>c.selectModelTaskGroups(v,'all','original',undefined,'all','all','independent');const v=c.modelTaskUsage(rows,meta);
 assert.equal(v.groups.length,7);assert.equal(select(v).length,4);assert.ok(select(v).every(g=>g.count===1));
 const window=c.modelTaskUsage([base,...Array.from({length:256},(_,i)=>({...base,id:i+2,task:null}))],meta);assert.equal(window.count,256);assert.equal(select(window).length,256);
 const next=c.modelTaskUsage([{...rows[3],session:'a'.repeat(24)}],meta);assert.equal(select(next).length,0);assert.equal(next.groups[0].grouped,true);
});

test('filtered output summary totals whole groups with exact known coverage',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-28',checkedAt:'2026-10-29'},settings={large:100,minimum:100,missPercent:80};
 const r=(id,task,input,output,outcome='completed')=>({...row(input,input,0,output),id,session:'a'.repeat(24),task:task?.repeat(24),outcome});
 const rows=[r(1,'b',100,10),r(2,'b',1,null,'failed'),r(3,'c',1,20),r(4,null,1,0),r(5,null,1,null)];
 const v=c.modelTaskUsage(rows,meta),before=JSON.stringify(v),select=(association='all',count='all',risk='all',outcome='all',sort='original')=>c.selectModelTaskGroups(v,risk,sort,settings,outcome,count,association);
 const check=(gs,expected)=>{const text=taskTableText(c,v,gs);assert.ok(text.includes('匹配任务已知输出：'+expected));assert.ok(text.includes('全窗口已知输出：30 Token（已知3/5次）。'));};
 check(select(),'30 Token（已知3/5次）。');check(select('linked'),'30 Token（已知2/3次）。');check(select('independent'),'0 Token（已知1/2次）。');check(select('linked','multiple'),'10 Token（已知1/2次）。');check(select('independent','multiple'),'未知（已知0/0次）。');
 for(const sort of ['original','count','input','miss','output']){check(select('linked','all','anyRisk','failed',sort),'10 Token（已知1/2次）。');check(select('all','all','all','all',sort),'30 Token（已知3/5次）。');}
 assert.equal(JSON.stringify(v),before);
 const bounded=c.modelTaskUsage([r(9,'d',1,999),...Array.from({length:256},(_,i)=>r(i+10,null,1,1))],meta);assert.match(taskTableText(c,bounded),/全窗口已知输出：256 Token（已知256\/256次）/);
});

test('output summary distinguishes zero unknown no match and large exact totals',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-28',checkedAt:'2026-10-29'},text=outputs=>taskTableText(c,c.modelTaskUsage(outputs.map(output=>row(1,0,0,output)),meta));
 assert.match(text([0]),/匹配任务已知输出：0 Token（已知1\/1次）/);
 assert.match(text([null,-1,1.5,Number.MAX_SAFE_INTEGER+1,'2']),/匹配任务已知输出：未知（已知0\/5次）/);
 assert.match(text([Number.MAX_SAFE_INTEGER,Number.MAX_SAFE_INTEGER,null]),/全窗口已知输出：18014398509481982 Token（已知2\/3次）/);
 assert.match(text([7]),/全窗口已知输出：7 Token（已知1\/1次）/); // cache inconsistency does not discard known output
 assert.doesNotMatch(text([]),/已知输出/);assert.doesNotMatch(taskTableText(c,null),/已知输出/);
});

test('output coverage filter keeps whole groups and exact matching subtotals',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-28',checkedAt:'2026-10-29'},settings={large:100,minimum:100,missPercent:80};
 const r=(id,task,input,output,outcome='completed')=>({...row(input,input,0,output),id,session:'a'.repeat(24),task:task?.repeat(24),outcome});
 const v=c.modelTaskUsage([r(1,'b',100,10),r(2,'b',1,null,'failed'),r(3,'c',1,0),r(4,null,1,20),r(5,null,1,null)],meta),before=JSON.stringify(v);
 const select=(coverage,association='all',count='all',risk='all',outcome='all',sort='original')=>c.selectModelTaskGroups(v,risk,sort,settings,outcome,count,association,coverage),ids=gs=>Array.from(gs,g=>Array.from(g.requests,r=>r.id));
 assert.deepEqual(ids(select('all')),[[1,2],[3],[4],[5]]);assert.deepEqual(ids(select('complete')),[[3],[4]]);assert.deepEqual(ids(select('partial')),[[1,2]]);assert.deepEqual(ids(select('unknown')),[[5]]);
 for(const [type,assoc,count,expected] of [['complete','all','all','20 Token（已知2/2次）'],['partial','all','all','10 Token（已知1/2次）'],['unknown','all','all','未知（已知0/1次）'],['complete','linked','all','0 Token（已知1/1次）'],['partial','independent','all','未知（已知0/0次）'],['partial','all','single','未知（已知0/0次）']]){const text=taskTableText(c,v,select(type,assoc,count));assert.ok(text.includes('匹配任务已知输出：'+expected));assert.match(text,/全窗口已知输出：30 Token（已知3\/5次）/);}
 for(const sort of ['original','count','input','miss','output'])assert.deepEqual(ids(select('partial','linked','multiple','anyRisk','failed',sort)),[[1,2]]);
 assert.deepEqual(ids(select('complete','all','all','all','all','output')),[[4],[3]]);assert.equal(JSON.stringify(v),before);assert.deepEqual(ids(c.selectModelTaskGroups(v)),ids(select('all')));
});

test('output coverage follows upstream known values and new bounded snapshots',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-28',checkedAt:'2026-10-29'},r=(output)=>({...row(1,0,0,output),id:1}),select=(v,type)=>c.selectModelTaskGroups(v,'all','original',undefined,'all','all','all',type);
 const v=c.modelTaskUsage([0,null,-1,1.5,'2',Number.MAX_SAFE_INTEGER+1].map(r),meta);assert.equal(v.groups.length,6);assert.equal(select(v,'complete').length,1);assert.equal(select(v,'unknown').length,5);assert.equal(select(v,'partial').length,0);
 const linked={...r(Number.MAX_SAFE_INTEGER),session:'a'.repeat(24),task:'b'.repeat(24)};const big=c.modelTaskUsage([linked,linked],meta);assert.equal(select(big,'complete').length,1);assert.equal(big.groups[0].totals.output.value,'18014398509481982');
 const old=c.modelTaskUsage([{...linked,usage:{output:null}},...Array.from({length:255},()=>linked)],meta);assert.equal(select(old,'partial').length,1);const next=c.modelTaskUsage([{...linked,usage:{output:null}},...Array.from({length:256},()=>linked)],meta);assert.equal(select(next,'partial').length,0);assert.equal(select(next,'complete').length,1);
});

test('token subtotals use independent field coverage and complete selected groups',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-28',checkedAt:'2026-10-29'};
 const r=(id,task,input,hit,miss,output,outcome='completed')=>({id,session:'a'.repeat(24),task:task?.repeat(24),outcome,usage:{input,hit,miss,output}});
 const rows=[r(1,'b',100,60,40,10),r(2,'b',null,0,null,null,'failed'),r(3,'c',0,null,0,0),r(4,null,30,10,20,5)],snapshot=JSON.stringify(rows),v=c.modelTaskUsage(rows,meta);
 const check=(groups,expected)=>{const text=taskTableText(c,v,groups);for(const [i,label] of ['输入','缓存命中','缓存未命中','输出'].entries()){assert.ok(text.includes('匹配任务已知'+label+'：'+expected[i]),text);assert.ok(text.includes('全窗口已知'+label+'：'+['130','70','60','15'][i]+' Token（已知3/4次）'),text);}};
 check(v.groups,['130 Token（已知3/4次）','70 Token（已知3/4次）','60 Token（已知3/4次）','15 Token（已知3/4次）']);
 const partial=['100 Token（已知1/2次）','60 Token（已知2/2次）','40 Token（已知1/2次）','10 Token（已知1/2次）'];
 check(c.selectModelTaskGroups(v,'all','original',undefined,'all','all','all','partial'),partial);
 check(c.selectModelTaskGroups(v,'all','original',undefined,'all','single','linked'),['0 Token（已知1/1次）','未知（已知0/1次）','0 Token（已知1/1次）','0 Token（已知1/1次）']);
 check(c.selectModelTaskGroups(v,'all','original',undefined,'all','all','independent','partial'),Array(4).fill('未知（已知0/0次）'));
 for(const sort of ['original','count','input','miss','output'])check(c.selectModelTaskGroups(v,'large',sort,{large:100,minimum:100,missPercent:50},'failed','multiple','linked','partial'),partial);
 assert.equal(JSON.stringify(rows),snapshot);
 const bounded=c.modelTaskUsage([r(1,null,999,999,999,999),...Array.from({length:256},(_,i)=>r(i+2,null,1,0,null,null))],meta);
 assert.match(taskTableText(c,bounded),/全窗口已知输入：256 Token（已知256\/256次）/);assert.match(taskTableText(c,bounded),/全窗口已知缓存未命中：未知（已知0\/256次）/);
});
test('token subtotals keep inconsistent fields and exact integers without inference',()=>{
 const {c}=fixture(),meta={startedAt:'2026-10-28',checkedAt:'2026-10-29'};
 const text=rs=>taskTableText(c,c.modelTaskUsage(rs,meta));
 const inconsistent=text([row(7,0,100,null)]);assert.match(inconsistent,/全窗口已知输入：7 Token（已知1\/1次）/);assert.match(inconsistent,/全窗口已知缓存命中：0 Token（已知1\/1次）/);assert.match(inconsistent,/全窗口已知缓存未命中：100 Token（已知1\/1次）/);
 const huge=text([row(Number.MAX_SAFE_INTEGER,Number.MAX_SAFE_INTEGER,Number.MAX_SAFE_INTEGER),row(Number.MAX_SAFE_INTEGER,Number.MAX_SAFE_INTEGER,Number.MAX_SAFE_INTEGER),row(Number.MAX_SAFE_INTEGER+1,-1,'2')]);
 for(const label of ['输入','缓存命中','缓存未命中'])assert.ok(huge.includes('全窗口已知'+label+'：18014398509481982 Token（已知2/3次）'));
 assert.doesNotMatch(text([]),/已知输入|已知缓存/);assert.doesNotMatch(taskTableText(c,null),/已知输入|已知缓存/);
});
