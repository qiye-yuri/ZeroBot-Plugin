// Independent arithmetic over exported reports; never import the page's calculations.
import {open} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {isDeepStrictEqual} from 'node:util';
import path from 'node:path';
import {pathToFileURL} from 'node:url';
import {stamp,writeEvidence} from './check-observation-evidence.mjs';

const fields=['input','hit','miss','output'];
const integer=x=>Number.isSafeInteger(x)&&x>=0;
const object=x=>x!==null&&typeof x==='object'&&!Array.isArray(x);
const text=x=>typeof x==='string'&&x.length<=4096;
const nullable=(x,check)=>x===null||check(x);
class Invalid extends Error {constructor(location){super('invalid-report');this.location=location;}}
function requireAt(ok,location){if(!ok)throw new Invalid(location);}
function keys(value,allowed,location){
  requireAt(object(value)&&Object.keys(value).every(k=>allowed.includes(k)),location);
}
function time(value,location){try{return stamp(value,true).ns;}catch{throw new Invalid(location);}}
function cache(rows){
  let valid=0,unknown=0,inconsistent=0,hit=0n,miss=0n;
  for(const {usage:u} of rows){
    if(!['input','hit','miss'].every(k=>integer(u[k])))unknown++;
    else if(BigInt(u.input)!==BigInt(u.hit)+BigInt(u.miss))inconsistent++;
    else {valid++;hit+=BigInt(u.hit);miss+=BigInt(u.miss);}
  }
  const total=hit+miss,rounded=total?(hit*10000n+total/2n)/total:null;
  const hitRate=rounded===null?null:`${rounded/100n}.${String(rounded%100n).padStart(2,'0')}%`;
  return {valid,unknown,inconsistent,hit:valid?String(hit):null,miss:valid?String(miss):null,hitRate};
}
function totals(rows){return Object.fromEntries(fields.map(k=>{
  const values=rows.map(r=>r.usage[k]).filter(integer);
  return [k,{value:values.length?String(values.reduce((n,v)=>n+BigInt(v),0n)):null,known:values.length}];
}));}
function models(rows){
  const groups=new Map();
  for(const r of rows){
    const provider=r.provider||null,model=r.model||null,key=JSON.stringify([provider,model]);
    if(!groups.has(key))groups.set(key,{provider,model,rows:[]});
    groups.get(key).rows.push(r);
  }
  return Array.from(groups.values()).map(g=>{
    const c=cache(g.rows);
    return {provider:g.provider,model:g.model,count:g.rows.length,totals:totals(g.rows),cacheCount:c.valid,inconsistent:c.inconsistent,hitRate:c.hitRate};
  }).sort((a,b)=>{
    const x=BigInt(a.totals.input.value??0),y=BigInt(b.totals.input.value??0);
    return x===y?0:x>y?-1:1;
  });
}
function tasks(rows,startedAt){
  const groups=new Map();
  rows.forEach((r,index)=>{
    const grouped=r.session!==null&&r.task!==null;
    const key=grouped?JSON.stringify([r.session,r.task]):JSON.stringify(['single',index]);
    if(!groups.has(key))groups.set(key,{session:grouped?r.session:null,task:grouped?r.task:null,grouped,requests:[]});
    groups.get(key).requests.push(r);
  });
  return {startedAt,count:rows.length,complete:false,groups:Array.from(groups.values()).map(g=>({...g,count:g.requests.length,totals:totals(g.requests),cacheSummary:cache(g.requests)}))};
}

export function validateUsageReport(value){
  const issues=[];
  const result={format:'zerobot-model-usage-check',version:1,passed:false,issues,taskBreakdownChecked:false,businessAcceptancePassed:false,billingVerified:false};
  try{
    keys(value,['format','version','exportedAt','scope','snapshot','thresholds','requests','cacheSummary','breakdown','taskBreakdown','hints'],'report');
    requireAt(value.format==='zerobot-model-usage'&&value.version===1,'report.format/version');
    requireAt(isDeepStrictEqual(value.scope,{kind:'recent-business-model-requests',limit:256,includesAllCalls:false,amount:null}),'scope');
    keys(value.snapshot,['startedAt','checkedAt'],'snapshot');
    const start=time(value.snapshot.startedAt,'snapshot.startedAt'),end=time(value.snapshot.checkedAt,'snapshot.checkedAt');
    requireAt(start<=end,'snapshot.time-order');
    // Browser clocks can differ from the server; exportedAt is not a server bound.
    time(value.exportedAt,'exportedAt');
    keys(value.thresholds,['large','minimum','missPercent'],'thresholds');
    for(const k of ['large','minimum','missPercent'])requireAt(integer(value.thresholds[k])&&value.thresholds[k]>=1&&value.thresholds[k]<=(k==='missPercent'?100:1e9),`thresholds.${k}`);
    requireAt(text(value.hints),'hints');
    requireAt(Array.isArray(value.requests)&&value.requests.length<=256,'requests');
    const rows=value.requests,ids=new Set();
    rows.forEach((r,index)=>{
      const loc=`requests[${index}]`;
      keys(r,['id','sequence','kind','provider','model','session','task','startedAt','milliseconds','outcome','usage'],loc);
      requireAt(integer(r.id)&&r.id>0&&!ids.has(r.id),loc+'.id');ids.add(r.id);
      requireAt(nullable(r.sequence,integer),loc+'.sequence');
      for(const k of ['kind','provider','model','outcome'])requireAt(nullable(r[k],text),loc+'.'+k);
      for(const k of ['session','task'])requireAt(nullable(r[k],x=>typeof x==='string'&&/^[a-f0-9]{24}$/i.test(x)),loc+'.'+k);
      requireAt(nullable(r.milliseconds,x=>typeof x==='number'&&Number.isFinite(x)&&x>=0),loc+'.milliseconds');
      if(r.startedAt!==null){const t=time(r.startedAt,loc+'.startedAt');requireAt(t>=start&&t<=end,loc+'.time-range');}
      keys(r.usage,fields,loc+'.usage');
      for(const k of fields)requireAt(nullable(r.usage[k],integer),loc+'.usage.'+k);
    });
    const compare=(actual,expected,location)=>{if(!isDeepStrictEqual(actual,expected))issues.push(location);};
    const computed=cache(rows);
    compare(value.cacheSummary,computed,'cacheSummary');
    keys(value.breakdown,['count','groups','top'],'breakdown');
    compare(value.breakdown.count,rows.length,'breakdown.count');
    compare(value.breakdown.groups,models(rows),'breakdown.groups');
    const top=rows.map((r,index)=>({r,index})).filter(x=>integer(x.r.usage.input)).sort((a,b)=>{
      const x=BigInt(a.r.usage.input),y=BigInt(b.r.usage.input);
      return x===y?b.index-a.index:x>y?-1:1;
    }).slice(0,5).map(x=>x.r);
    compare(value.breakdown.top,top,'breakdown.top');
    if(Object.hasOwn(value,'taskBreakdown')){
      compare(value.taskBreakdown,tasks(rows,value.snapshot.startedAt),'taskBreakdown');
      result.taskBreakdownChecked=true;
    }
    // Recompute the numeric hint summary, without depending on translated prose.
    const risk={large:0,highMiss:0,incomplete:0,inconsistent:computed.inconsistent};
    for(const r of rows){
      const u=r.usage;if(integer(u.input)&&u.input>=value.thresholds.large)risk.large++;
      if(!fields.every(k=>integer(u[k]))){risk.incomplete++;continue;}
      if(BigInt(u.input)===BigInt(u.hit)+BigInt(u.miss)&&u.input>=value.thresholds.minimum&&BigInt(u.miss)*100n>=BigInt(u.input)*BigInt(value.thresholds.missPercent))risk.highMiss++;
    }
    Object.assign(result,{requestCount:rows.length,totals:totals(rows),cacheSummary:computed,risks: risk,hintProseChecked:false});
    result.passed=issues.length===0;
  }catch(error){issues.push(error instanceof Invalid?error.location:'report.invalid');}
  return result;
}

export async function checkUsageFile(filename){
  const limit=4*1024*1024;
  let handle;
  try{
    handle=await open(filename,'r');
    const stat=await handle.stat();requireAt(stat.isFile()&&stat.size<=limit,'file.size/type');
    const chunks=[];let size=0;
    for await(const chunk of handle.createReadStream({autoClose:false})){size+=chunk.length;requireAt(size<=limit,'file.size');chunks.push(chunk);}
    const bytes=Buffer.concat(chunks);let value,decoded;
    // Do not silently replace damaged bytes in evidence before validating it.
    try{decoded=new TextDecoder('utf-8',{fatal:true}).decode(bytes);}catch{throw new Invalid('file.encoding');}
    try{value=JSON.parse(decoded);}catch{throw new Invalid('file.json');}
    return {...validateUsageReport(value),sourceSHA256:createHash('sha256').update(bytes).digest('hex')};
  }catch(error){return {format:'zerobot-model-usage-check',version:1,passed:false,issues:[error instanceof Invalid?error.location:'file.read'],businessAcceptancePassed:false,billingVerified:false};}
  finally{await handle?.close();}
}
export async function main(args=process.argv.slice(2)){
  try{
    const options={};
    for(let i=0;i<args.length;i+=2){
      requireAt(['--report','--output'].includes(args[i])&&args[i+1]&&!args[i+1].startsWith('--')&&!Object.hasOwn(options,args[i]),'arguments');
      options[args[i]]=args[i+1];
    }
    requireAt(options['--report'],'arguments');
    const result=await checkUsageFile(options['--report']);
    if(options['--output'])await writeEvidence(options['--output'],result);
    process.stdout.write(JSON.stringify(result,null,2)+'\n');return result.passed?0:1;
  }catch(error){process.stderr.write(error instanceof Invalid?'用法：node scripts/check-model-usage.mjs --report 报告.json [--output 新结果.json]\n':'核对结果无法写入；输出文件须尚不存在且目录可写。\n');return error instanceof Invalid?2:1;}
}
if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href)process.exitCode=await main();
