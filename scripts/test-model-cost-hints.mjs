// Offline synthetic usage only: no provider, QQ or pricing API calls.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
function fixture(){
 const nodes=new Map();const $=id=>{if(!nodes.has(id))nodes.set(id,{textContent:'',replaceChildren(){}});return nodes.get(id)};
 const c=vm.createContext({$,document:{addEventListener(){}},Number,Date,setTimeout,clearTimeout});
 vm.runInContext(fs.readFileSync('internal/localadmin/assets/overview.js','utf8'),c);return {c,$};
}
const row=(input,hit,miss,output=10)=>({id:1,model:'fixture',usage:{input,hit,miss,output}});
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
