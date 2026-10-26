import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import vm from 'node:vm';
import {spawnSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {validateUsageReport,checkUsageFile,usageDiagnostics} from './check-model-usage.mjs';

const source=await fs.readFile('internal/localadmin/assets/overview.js','utf8');
const meta={startedAt:'2026-10-26T00:00:00Z',checkedAt:'2026-10-26T01:00:00Z'};
function report(rows){
  const c=vm.createContext({document:{addEventListener(){}},setTimeout,clearTimeout});vm.runInContext(source,c);
  return JSON.parse(JSON.stringify(c.buildModelUsageReport(rows,meta,{large:100000,minimum:10000,missPercent:80},'2026-10-26T01:00:01Z')));
}
function row(id,input=100,hit=80,miss=20){return {id,sequence:id,kind:'agent',provider:'example.invalid',model:'fixture',session:'a'.repeat(24),task:'b'.repeat(24),startedAt:'2026-10-26T00:30:00.1234567Z',milliseconds:2,outcome:'completed',usage:{input,hit,miss,output:1}};}

test('current and legacy exports reconcile, without claiming billing or business acceptance',()=>{
  const d=report([row(1),row(2)]),before=JSON.stringify(d),v=validateUsageReport(d);
  assert.equal(v.passed,true);assert.equal(v.taskBreakdownChecked,true);assert.equal(v.totals.input.value,'200');assert.equal(v.cacheSummary.hitRate,'80.00%');assert.equal(v.billingVerified,false);assert.equal(v.businessAcceptancePassed,false);assert.equal(JSON.stringify(d),before);
  delete d.taskBreakdown;const old=validateUsageReport(d);assert.equal(old.passed,true);assert.equal(old.taskBreakdownChecked,false);
});
test('empty, unknown, inconsistent and huge fields preserve report semantics',()=>{
  assert.equal(validateUsageReport(report([])).passed,true);
  const n=Number.MAX_SAFE_INTEGER;
  for(const rows of [[row(1,0,0,0)],[row(1,null,null,null)],[row(1,100,90,90)],[row(1,n,n,0),row(2,n,n,0)]])assert.equal(validateUsageReport(report(rows)).passed,true);
  const v=validateUsageReport(report([row(1,n,n,0),row(2,n,n,0)]));assert.equal(v.totals.input.value,'18014398509481982');
  assert.equal(validateUsageReport(report([row(1,100,90,90)])).cacheSummary.inconsistent,1);
});
test('independent recomputation finds tampered summaries groups ranks and task detail',()=>{
  const original=report([row(1),row(2,200,190,10)]);
  const cases=[['cacheSummary',d=>d.cacheSummary.hitRate='0.00%'],['breakdown.count',d=>d.breakdown.count=8],['breakdown.groups',d=>d.breakdown.groups[0].totals.input.value='1'],['breakdown.top',d=>d.breakdown.top.reverse()],['taskBreakdown',d=>d.taskBreakdown.groups[0].requests[0].sequence=999],['taskBreakdown',d=>d.taskBreakdown.complete=true]];
  for(const [location,mutate] of cases){const d=structuredClone(original);mutate(d);const v=validateUsageReport(d);assert.equal(v.passed,false,location);assert.ok(v.issues.includes(location));}
});
test('multiple models, task isolation, singleton identities and stable ties agree',()=>{
  const rows=[row(1),{...row(2),provider:'different',model:null},{...row(3),session:'c'.repeat(24)},{...row(4),task:null},{...row(5),task:null}];
  const d=report(rows);assert.equal(d.taskBreakdown.groups.length,4);assert.equal(validateUsageReport(d).passed,true);assert.deepEqual(d.breakdown.top.map(r=>r.id),[5,4,3,2,1]);
});
test('reject unsupported version, bad time, malformed rows, duplicate IDs and excess rows',()=>{
  const original=report([row(1)]);
  const changes=[d=>d.version=2,d=>d.snapshot.checkedAt='2026-02-30T00:00:00Z',d=>d.snapshot.checkedAt='2025-10-26T00:00:00Z',d=>d.requests[0].startedAt='2026-10-27T00:00:00Z',d=>d.requests[0].usage.input=-1,d=>d.requests[0].usage.input=Number.MAX_SAFE_INTEGER+1,d=>d.requests.push(structuredClone(d.requests[0])),d=>d.requests=Array(257).fill(row(1)),d=>d.taskBreakdown=null,d=>d.requests[0]=null];
  for(const mutate of changes){const d=structuredClone(original);mutate(d);assert.equal(validateUsageReport(d).passed,false);}
  const d=report([row(1)]);d.exportedAt='2026-10-25T00:00:00Z';assert.equal(validateUsageReport(d).passed,true,'browser clock is not server clock');
});
test('report additions and invalid identifiers cannot leak through diagnostic output',()=>{
  for(const mutate of [d=>d.requests[0].prompt='PRIVATE-CONTENT',d=>d.requests[0].session='PRIVATE-CONTENT',d=>d['PRIVATE-CONTENT']='PRIVATE-CONTENT',d=>d.breakdown.groups[0].extra='PRIVATE-CONTENT']){
    const d=report([row(1)]);mutate(d);const v=validateUsageReport(d);assert.equal(v.passed,false);assert.doesNotMatch(JSON.stringify(v),/PRIVATE-CONTENT|example.invalid|aaaaaaaa/);
  }
});
test('read boundaries, JSON failures, BOM and source hash',async t=>{
  const dir=await fs.mkdtemp(path.join(os.tmpdir(),'usage-check-'));t.after(()=>fs.rm(dir,{recursive:true,force:true}));
  const p=path.join(dir,'report.json'),bytes=Buffer.from('\ufeff'+JSON.stringify(report([row(1)])));await fs.writeFile(p,bytes);
  const v=await checkUsageFile(p);assert.equal(v.passed,true);assert.equal(v.sourceSHA256,createHash('sha256').update(bytes).digest('hex'));assert.deepEqual(await fs.readFile(p),bytes);
  await fs.writeFile(p,'{"broken":');assert.deepEqual((await checkUsageFile(p)).issues,['file.json']);
  await fs.writeFile(p,Buffer.alloc(4*1024*1024+1));assert.equal((await checkUsageFile(p)).passed,false);
  assert.equal((await checkUsageFile(dir)).passed,false);assert.deepEqual((await checkUsageFile(path.join(dir,'missing'))).issues,['file.read']);
});
test('damaged UTF-8 evidence is rejected without replacement or content disclosure',async t=>{
  const dir=await fs.mkdtemp(path.join(os.tmpdir(),'usage-encoding-'));t.after(()=>fs.rm(dir,{recursive:true,force:true}));
  const p=path.join(dir,'report.json'),d=report([row(1)]);d.hints='PRIVATE-MARKER';
  const good=Buffer.from(JSON.stringify(d)),offset=good.indexOf('PRIVATE-MARKER');assert.ok(offset>0);
  for(const invalid of [Buffer.from([0xff]),Buffer.from([0xc0,0xaf]),Buffer.from([0xed,0xa0,0x80]),Buffer.from([0xe4,0xb8])]){
    const bytes=Buffer.concat([good.subarray(0,offset),invalid,good.subarray(offset+'PRIVATE-MARKER'.length)]);await fs.writeFile(p,bytes);
    const checked=await checkUsageFile(p);assert.equal(checked.passed,false);assert.deepEqual(checked.issues,['file.encoding']);assert.doesNotMatch(JSON.stringify(checked),/PRIVATE|example.invalid/);assert.deepEqual(await fs.readFile(p),bytes);
  }
  d.hints='中文提示与合法替换字符\ufffd';await fs.writeFile(p,'\ufeff'+JSON.stringify(d));assert.equal((await checkUsageFile(p)).passed,true);
});

test('CLI exit codes and exclusive output never overwrite source or prior result',async t=>{
  const dir=await fs.mkdtemp(path.join(os.tmpdir(),'usage-cli-'));t.after(()=>fs.rm(dir,{recursive:true,force:true}));
  const p=path.join(dir,'in.json'),out=path.join(dir,'out.json'),bytes=JSON.stringify(report([row(1)]));await fs.writeFile(p,bytes);
  const run=args=>spawnSync(process.execPath,['scripts/check-model-usage.mjs',...args],{encoding:'utf8'});
  assert.equal(run([]).status,2);assert.equal(run(['--report',p,'--report',p]).status,2);
  const v=run(['--report',p,'--output',out]);assert.equal(v.status,0);assert.equal(JSON.parse(v.stdout).passed,true);
  const saved=await fs.readFile(out,'utf8');assert.equal(run(['--report',p,'--output',out]).status,1);assert.equal(await fs.readFile(out,'utf8'),saved);
  assert.equal(run(['--report',p,'--output',p]).status,1);assert.equal(await fs.readFile(p,'utf8'),bytes);
  await fs.writeFile(p,'null');assert.equal(run(['--report',p]).status,1);
});


test('fixed diagnostics explain all failure classes without copying input values',()=>{
 const cases={'file.read':'read','file.size/type':'size','file.encoding':'encoding','file.json':'json','report.format/version':'format','snapshot.time-order':'time','requests[2].time-range':'time','requests[1].usage.hit':'request','cacheSummary':'cache','breakdown.groups':'model','breakdown.top':'top','taskBreakdown':'task','PRIVATE-CONTENT':'invalid'};
 for(const [issue,code] of Object.entries(cases)){
  const [d]=usageDiagnostics([issue]);assert.equal(d.code,code);assert.deepEqual(Object.keys(d),['code','message','suggestion']);assert.ok(d.message.length&&d.suggestion.length);assert.doesNotMatch(JSON.stringify(d),/PRIVATE-CONTENT|requests\[/);
 }
 const valid=validateUsageReport(report([row(1)]));assert.deepEqual(valid.diagnostics,[]);assert.equal(valid.passed,true);
 const bad=report([row(1)]);bad.cacheSummary.hitRate='0.00%';const checked=validateUsageReport(bad);assert.deepEqual(checked.issues,['cacheSummary']);assert.equal(checked.diagnostics[0].code,'cache');assert.equal(checked.passed,false);
});
