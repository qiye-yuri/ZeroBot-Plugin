"use strict";
const modelCostDefaults=Object.freeze({large:100000,minimum:10000,missPercent:80});
let modelCostSettings={...modelCostDefaults},modelCostRows=null,modelUsageSnapshotMeta=null;
let overviewActive=false, overviewBusy=false, overviewGeneration=0, overviewRequest=null, overviewTimer=null;

function clearModelRequests(message="请求统计待更新。") {
  modelCostRows=null;modelUsageSnapshotMeta=null;renderModelUsageExport();renderModelTaskView();
  if($("model-usage-export-status"))$("model-usage-export-status").textContent="";
  if (typeof clearModelObservationCurrent==="function") clearModelObservationCurrent();
  $("model-request-summary").textContent=message;
 if ($("model-cost-hints")) $("model-cost-hints").textContent="费用风险提示待更新。";
 if ($("model-usage-breakdown")) $("model-usage-breakdown").textContent="分组用量待更新。";
 if ($("model-task-usage")) $("model-task-usage").textContent="任务用量待更新。";
 if ($("model-usage-summary")) $("model-usage-summary").textContent="用量未知";
 if ($("model-usage-recent")) $("model-usage-recent").textContent="";
  $("model-request-categories").replaceChildren();
}
function renderModelRequests(value) {
  const names={gpt:"GPT / 群总结",chat:"普通聊天",agent:"Agent",vision:"识图"};
  const counts=["started","active","completed","failed","incomplete"];
  const valid=row=>row && counts.every(key=>Number.isSafeInteger(row[key]) && row[key]>=0) &&
    row.started===row.active+row.completed+row.failed+row.incomplete &&
    ["averageMilliseconds","maxMilliseconds"].every(key=>Number.isFinite(row[key]) && row[key]>=0) && row.maxMilliseconds>=row.averageMilliseconds;
  if (!valid(value) || !Array.isArray(value.categories) || value.categories.length!==4 ||
      !Object.keys(names).every(kind=>value.categories.filter(row=>row?.kind===kind && valid(row)).length===1) ||
      !counts.every(key=>value[key]===value.categories.reduce((sum,row)=>sum+row[key],0)) ||
      typeof value.startedAt!=="string" || !Number.isFinite(Date.parse(value.startedAt)) ||
      typeof value.checkedAt!=="string" || !Number.isFinite(Date.parse(value.checkedAt))) {
    clearModelRequests("请求统计暂不可读取。");return;
  }
  const elapsed=row=>row.completed+row.failed+row.incomplete ?
    "已结束请求平均 "+(row.averageMilliseconds/1000).toFixed(3)+" 秒，最长 "+(row.maxMilliseconds/1000).toFixed(3)+" 秒。" : "尚无已结束请求，耗时待统计。";
  const countsText=row=>"发起 "+row.started+" 次；进行中 "+row.active+" 次；接口完成 "+row.completed+" 次；失败 "+row.failed+" 次；提前关闭 "+row.incomplete+" 次。";
  $("model-request-summary").textContent="统计起点："+new Date(value.startedAt).toLocaleString("zh-CN",{hour12:false})+"。"+countsText(value)+" "+elapsed(value)+(value.started===0 ? " 本次启动尚无模型接口请求。" : "");
  $("model-request-categories").replaceChildren();
  for (const [kind,name] of Object.entries(names)) {
    const row=value.categories.find(item=>item.kind===kind),card=document.createElement("article"),title=document.createElement("h3"),count=document.createElement("p"),time=document.createElement("p");
    title.textContent=name;count.textContent=countsText(row);time.className="field-help";time.textContent=elapsed(row);card.append(title,count,time);$("model-request-categories").append(card);
  }
  renderModelUsage(value.recent,{startedAt:value.startedAt,checkedAt:value.checkedAt});
  if (typeof receiveModelObservation==="function") receiveModelObservation(value);
}

function renderOverviewAlerts(value) {
  $("overview-alerts").replaceChildren();
  if (!value) {
    $("overview-alert-summary").textContent="近期异常暂不可读取，请到运行日志检查配置是否可读。";
    return;
  }
  $("overview-alert-summary").textContent="本次启动仍保留的日志：警告 "+value.warningCount+" 条，错误 "+value.errorCount+" 条。"+(value.evicted ? " 更早记录已淘汰。" : "")+" 显示最新 5 条；历史记录不代表问题仍未解决。";
  if (!value.entries.length) {
    const empty=document.createElement("p");empty.className="empty";empty.textContent="保留日志中暂无警告或错误。";$("overview-alerts").append(empty);
  }
  for (const entry of value.entries) {
    const row=document.createElement("article"),meta=document.createElement("div"),time=document.createElement("time"),level=document.createElement("span"),text=document.createElement("pre");
    row.className="log-entry";meta.className="log-meta";
    time.textContent=new Date(entry.time).toLocaleString("zh-CN",{hour12:false});
    level.textContent=entry.level.toUpperCase();level.className="log-level "+(entry.level==="warning" ? "log-warning-level" : "log-error-level");
    text.textContent=entry.message;meta.append(time,level);row.append(meta,text);$("overview-alerts").append(row);
  }
}
function overviewReady(generation) {return authenticated && overviewActive && !document.hidden && generation===overviewGeneration;}
function staleOverview(message="状态待更新") {
  if (typeof clearReplyTasks==="function") clearReplyTasks();
  clearModelRequests();
  $("connection-badge").textContent=message;$("connection-badge").classList.add("waiting");
  $("connection-detail").textContent="请等待重新读取当前连接状态。";
  $("account-count").textContent="—";$("checked-at").textContent="—";$("accounts").replaceChildren();
  $("overview-alerts").replaceChildren();$("overview-alert-summary").textContent="近期异常待更新。";
}
function cancelOverview() {
  overviewGeneration++;clearTimeout(overviewTimer);overviewTimer=null;
  overviewRequest?.abort();overviewRequest=null;overviewBusy=false;$("refresh").disabled=false;renderModelUsageExport();
  if (typeof renderModelObservation==="function") renderModelObservation();
  if (typeof renderReplyDownload==="function") renderReplyDownload();
}
function resetOverview() {
  resetModelTaskView();
  resetModelCostSettings(false);
  overviewActive=false;cancelOverview();staleOverview();$("page-error").hidden=true;
  if (typeof resetModelObservation==="function") resetModelObservation();
}
function showOverview(active) {
  if(!active)resetModelTaskView();
  overviewActive=active;cancelOverview();staleOverview();
  $("page-error").hidden=true;
  if (active && authenticated && !document.hidden) refreshOverview();
}
async function refreshOverview() {
  if (!authenticated || !overviewActive || document.hidden || overviewBusy) return;
  clearTimeout(overviewTimer);overviewTimer=null;
  const generation=++overviewGeneration,controller=new AbortController();overviewRequest=controller;overviewBusy=true;$("refresh").disabled=true;renderModelUsageExport();$("page-error").hidden=true;
  if (typeof renderModelObservation==="function") renderModelObservation();
  if (typeof renderReplyDownload==="function") renderReplyDownload();
  try {
    const value=await api("/api/overview",undefined,controller.signal);
    if (overviewReady(generation)) renderOverview(value);
  } catch(error) {
    if (overviewReady(generation) && error.name!=="AbortError") {staleOverview("无法获取状态");$("connection-detail").textContent="状态更新失败，旧连接信息已清除。";showError("page-error",error);}
  } finally {
    if (overviewReady(generation)) {overviewRequest=null;overviewBusy=false;$("refresh").disabled=false;renderModelUsageExport();overviewTimer=setTimeout(refreshOverview,10000);}
    if (overviewReady(generation) && typeof renderModelObservation==="function") renderModelObservation();
    if (overviewReady(generation) && typeof renderReplyDownload==="function") renderReplyDownload();
  }
}
document.addEventListener("visibilitychange",()=>{
  cancelOverview();staleOverview();
  if (!document.hidden && authenticated && overviewActive) refreshOverview();
});

function renderModelUsage(recent,meta=null) {
  const summary=$("model-usage-summary"), detail=$("model-usage-recent");
  if (!summary || !detail) return;
  if(!validModelUsageRows(recent)){
    modelCostRows=null;modelUsageSnapshotMeta=null;renderModelUsageExport();renderModelTaskView();
    summary.textContent="近期用量字段不可读取，不代表零请求。";detail.textContent="";
    if($("model-cost-hints"))$("model-cost-hints").textContent="近期用量未知，无法生成费用风险提示。";
    if($("model-usage-breakdown"))$("model-usage-breakdown").textContent="分组用量未知。";
    if($("model-task-usage"))$("model-task-usage").textContent="任务用量未知。";
    if($("model-usage-export-status"))$("model-usage-export-status").textContent="";
    return;
  }
  const rows=recent.slice(-256);
  modelCostRows=rows;modelUsageSnapshotMeta=meta;renderModelUsageExport();renderModelTaskView();
  if ($("model-cost-hints")) $("model-cost-hints").textContent=modelCostHints(rows,modelCostSettings);
  if ($("model-usage-breakdown")) $("model-usage-breakdown").textContent=modelUsageBreakdownText(modelUsageBreakdown(rows));
  const known=n=>Number.isSafeInteger(n)&&n>=0;
  const cache=summarizeModelCache(rows);
  summary.textContent=`最近${rows.length}次已结束请求，有效缓存统计${cache.valid}次，字段缺失/无效${cache.unknown}次，分项不一致${cache.inconsistent}次，按Token加权命中率${cache.hitRate??"未知"}。缺失值为未知；重启清空，不等同账单。`;
  const number=n=>known(n)?String(n):"未知";
  detail.textContent=rows.slice(-20).reverse().map(r=>`#${r.id} ${r.kind} ${r.model||"模型未知"} ${r.outcome}
输入 ${number(r.usage?.input)} / 命中 ${number(r.usage?.hit)} / 未命中 ${number(r.usage?.miss)} / 输出 ${number(r.usage?.output)}
${r.provider||"提供商未知"} · ${r.startedAt} · ${Number(r.milliseconds).toFixed(0)}ms · 会话 ${r.session||"未知"} · 任务 ${r.task||"未知"} / 客户端请求序号 ${r.sequence||"未知"}`).join("\n\n");
}

// Read-only interpretation of the existing bounded usage window, never billing or request control.
function modelCostHints(rows,settings=modelCostDefaults) {
  const known=n=>Number.isSafeInteger(n)&&n>=0;
  if (!rows.length) return "暂无近期请求，费用未知。";
  let large=0,uncached=0,unknown=0,inconsistent=0,top=null;
  for (const row of rows) {
    const u=row.usage||{},risk=modelRequestRisk(row,settings);
    if (known(u.input)) {
      if (!top || u.input>top.usage.input) top=row;
      if (risk.large) large++;
    }
    if(risk.inconsistent)inconsistent++;
    if(risk.incomplete)unknown++;
    if(risk.highMiss)uncached++;
  }
  const lines=[`最近${rows.length}次已结束请求：大输入${large}次，高未命中${uncached}次；用量字段不完整${unknown}次，输入与缓存分项不一致${inconsistent}次。`];
  if(top) lines.push(`输入最多：#${top.id} ${top.model||"模型未知"}，输入${top.usage.input} Token，未命中${known(top.usage.miss)?top.usage.miss:"未知"} Token。可结合下方会话和任务标识排查。`);
  if(large || uncached) lines.push("建议核对是否夹带长工具结果或重复内容，并检查稳定提示前缀；冷启动与上下文变化也会造成未命中，不能据此认定缓存故障。");
  lines.push(`提示条件：单次输入≥${settings.large} Token；输入≥${settings.minimum}且未命中占比≥${settings.missPercent}%。缺失或不一致字段不参与高未命中判断。仅提示，不修改请求、历史预算或插件开关。金额未知：未配置可核对的单价，实际费用以服务商账单为准；此窗口不含全部调用。人工诊断不计入业务统计，重启清空，最多保留256次。`);
  return lines.join("\n");
}

function modelUsageBreakdown(recent) {
  const known=n=>Number.isSafeInteger(n)&&n>=0;
  const rows=Array.isArray(recent)?recent.slice(-256):[];
  const groups=new Map(), fields=["input","hit","miss","output"];
  const label=s=>typeof s==="string"&&s.length?s:null;
  for(const r of rows){
    const provider=label(r.provider),model=label(r.model),key=JSON.stringify([provider,model]);
    if(!groups.has(key))groups.set(key,{provider,model,count:0,totals:Object.fromEntries(fields.map(f=>[f,{sum:0n,known:0}])),cacheCount:0,cacheHit:0n,cacheMiss:0n,inconsistent:0});
    const g=groups.get(key),u=r.usage||{};g.count++;
    for(const f of fields)if(known(u[f])){g.totals[f].sum+=BigInt(u[f]);g.totals[f].known++;}
    const state=modelCacheState(u);
    if(state==="valid"){g.cacheCount++;g.cacheHit+=BigInt(u.hit);g.cacheMiss+=BigInt(u.miss);}
    else if(state==="inconsistent")g.inconsistent++;

  }
  const result=Array.from(groups.values()).sort((a,b)=>a.totals.input.sum>b.totals.input.sum?-1:a.totals.input.sum<b.totals.input.sum?1:0).map(g=>{
    return {provider:g.provider,model:g.model,count:g.count,totals:Object.fromEntries(fields.map(f=>[f,{value:g.totals[f].known?String(g.totals[f].sum):null,known:g.totals[f].known}])),cacheCount:g.cacheCount,inconsistent:g.inconsistent,hitRate:modelCacheRate(g.cacheHit,g.cacheMiss)};
  });
  // Copy before sorting: preserve the original collector order and data.
  const top=rows.map((r,index)=>({r,index})).filter(x=>known(x.r.usage?.input)).sort((a,b)=>b.r.usage.input-a.r.usage.input || b.index-a.index).slice(0,5).map(x=>x.r);
  return {count:rows.length,groups:result,top};
}
function modelUsageBreakdownText(value){
  if(!value.count)return "暂无近期请求，分组用量未知。";
  const names={input:"输入",hit:"命中",miss:"未命中",output:"输出"};
  const lines=[`最近${value.count}次请求，按提供商与模型分组；按已知输入小计降序排列。小计只包含各字段已知值，覆盖不全时不能代表全部用量；不同模型Token数量不等同费用。`];
  for(const g of value.groups){
    lines.push(`\n${g.provider??"提供商未知"} / ${g.model??"模型未知"} · ${g.count}次请求`);
    lines.push(Object.entries(names).map(([f,name])=>`${name} ${g.totals[f].value??"未知"}（已知${g.totals[f].known}/${g.count}次）`).join(" / "));
    lines.push(`有效缓存统计${g.cacheCount}/${g.count}次 · 加权命中率${g.hitRate??"未知"} · 分项不一致${g.inconsistent}次`);
  }
  lines.push("\n输入最多的5次请求（仅已知输入；同量时较新记录优先，包含失败请求）：");
  if(!value.top.length)lines.push("输入量全部未知，无法排序。");
  for(const r of value.top)lines.push(`#${r.id} · ${r.provider||"提供商未知"} / ${r.model||"模型未知"} · 输入${r.usage.input} · ${r.outcome||"状态未知"}\n${r.startedAt||"时间未知"} · 会话${r.session||"未知"} · 任务${r.task||"未知"} · 客户端请求序号${r.sequence||"未知"}`);
  lines.push("\n复用现有最多256条记录；不新增采集或模型调用。缓存命中率仅纳入输入=命中+未命中的完整分项，未按请求百分比平均。金额仍未知，不等同服务商账单。");
  return lines.join("\n");
}

function parseModelCostSettings(values){
  const integer=(s,max)=>typeof s==="string" && /^[0-9]+$/.test(s.trim()) && Number(s)>=1 && Number(s)<=max ? Number(s):null;
  const large=integer(values.large,1000000000),minimum=integer(values.minimum,1000000000),missPercent=integer(values.missPercent,100);
  return large===null || minimum===null || missPercent===null ? null : {large,minimum,missPercent};
}
function renderModelCostSettings(){
  for(const [key,id] of Object.entries({large:"cost-large",minimum:"cost-minimum",missPercent:"cost-miss-percent"}))if($(id))$(id).value=String(modelCostSettings[key]);
}
function applyModelCostSettings(){
  if(typeof authenticated==="undefined" || !authenticated)return;
  const next=parseModelCostSettings({large:$("cost-large").value,minimum:$("cost-minimum").value,missPercent:$("cost-miss-percent").value});
  if(!next){$("cost-settings-status").textContent="未应用：Token阈值须为1至1000000000的整数，百分比须为1至100的整数。原提示条件保持。";return;}
  modelCostSettings=next;renderModelTaskView();
  if(modelCostRows!==null)$("model-cost-hints").textContent=modelCostHints(modelCostRows,modelCostSettings);
  $("cost-settings-status").textContent="已应用到本页提示，未修改机器人配置或历史预算。"+(modelCostRows===null?"当前用量待更新。":"已用最近读取的用量重新计算，未发起请求。");
}
function resetModelCostSettings(notify=true){
  modelCostSettings={...modelCostDefaults};renderModelCostSettings();
  if(notify)renderModelTaskView();
  if(notify && modelCostRows!==null)$("model-cost-hints").textContent=modelCostHints(modelCostRows,modelCostSettings);
  if($("cost-settings-status"))$("cost-settings-status").textContent=notify?"已恢复默认提示条件，仅影响本页。":"";
}
document.addEventListener("submit",event=>{if(event.target.id==="model-cost-settings"){event.preventDefault();applyModelCostSettings();}});
document.addEventListener("click",event=>{if(event.target.id==="cost-settings-reset" && typeof authenticated!=="undefined" && authenticated)resetModelCostSettings();});
document.addEventListener("input",event=>{
  if(["cost-large","cost-minimum","cost-miss-percent"].includes(event.target.id))$("cost-settings-status").textContent="阈值草稿尚未应用，提示仍使用上次应用的条件。";
});

function validModelUsageMeta(meta){
  return meta && typeof meta.startedAt==="string" && typeof meta.checkedAt==="string" && Number.isFinite(Date.parse(meta.startedAt)) && Number.isFinite(Date.parse(meta.checkedAt)) && Date.parse(meta.checkedAt)>=Date.parse(meta.startedAt);
}
function renderModelUsageExport(){
  if($("model-usage-export"))$("model-usage-export").disabled=typeof authenticated==="undefined" || !authenticated || overviewBusy || document.hidden || modelCostRows===null || !validModelUsageMeta(modelUsageSnapshotMeta);
}
function buildModelUsageReport(rows,meta,settings,exportedAt){
  if(!validModelUsageRows(rows) || !validModelUsageMeta(meta))return null;
  const number=n=>Number.isSafeInteger(n)&&n>=0?n:null;
  const text=s=>typeof s==="string"?s:null;
  const anonymous=s=>typeof s==="string" && /^[a-f0-9]{24}$/i.test(s)?s:null;
  // Explicit fields only: never forward future API fields such as prompts or raw responses.
  const requests=rows.slice(-256).map(r=>({id:number(r.id),sequence:number(r.sequence),kind:text(r.kind),provider:text(r.provider),model:text(r.model),session:anonymous(r.session),task:anonymous(r.task),startedAt:text(r.startedAt),milliseconds:Number.isFinite(r.milliseconds)&&r.milliseconds>=0?r.milliseconds:null,outcome:text(r.outcome),usage:Object.fromEntries(["input","hit","miss","output"].map(f=>[f,number(r.usage?.[f])]))}));
  const thresholds={large:settings.large,minimum:settings.minimum,missPercent:settings.missPercent};
  return {format:"zerobot-model-usage",version:1,exportedAt,scope:{kind:"recent-business-model-requests",limit:256,includesAllCalls:false,amount:null},snapshot:{startedAt:meta.startedAt,checkedAt:meta.checkedAt},thresholds,requests,cacheSummary:summarizeModelCache(requests),breakdown:modelUsageBreakdown(requests),taskBreakdown:modelTaskUsage(requests,meta),hints:modelCostHints(requests,thresholds)};
}
function downloadModelUsage(){
  if(typeof authenticated==="undefined" || !authenticated || overviewBusy || document.hidden || modelCostRows===null)return;
  const report=buildModelUsageReport(modelCostRows,modelUsageSnapshotMeta,modelCostSettings,new Date().toISOString());if(!report)return;
  let url=null,link=null;
  try{
    url=URL.createObjectURL(new Blob([JSON.stringify(report,null,2)+"\n"],{type:"application/json;charset=utf-8"}));
    link=document.createElement("a");link.href=url;link.download="zerobot-model-usage-"+report.exportedAt.replace(/[-:.]/g,"")+".json";document.body.append(link);link.click();
    $("model-usage-export-status").textContent="已发起最近用量报告下载，包含已应用的提示条件与原统计时间；未重新读取或调用模型。";
  }catch(e){$("model-usage-export-status").textContent="导出未完成，请重试。";}
  finally{link?.remove();if(url)setTimeout(()=>URL.revokeObjectURL(url),1000);}
}
document.addEventListener("click",event=>{if(event.target.id==="model-usage-export")downloadModelUsage();});

function validModelUsageRows(rows){
  return Array.isArray(rows) && rows.every(r=>r!==null && typeof r==="object" && !Array.isArray(r));
}
function modelCacheState(u){
  const known=n=>Number.isSafeInteger(n)&&n>=0;
  if(![u?.input,u?.hit,u?.miss].every(known))return "unknown";
  return BigInt(u.hit)+BigInt(u.miss)===BigInt(u.input)?"valid":"inconsistent";
}
function modelCacheRate(hit,miss){
  const total=hit+miss;if(total===0n)return null;
  const rate=(hit*10000n+total/2n)/total;
  return `${rate/100n}.${String(rate%100n).padStart(2,"0")}%`;
}
function summarizeModelCache(rows){
  let valid=0,unknown=0,inconsistent=0,hit=0n,miss=0n;
  for(const r of rows){
    const state=modelCacheState(r.usage);
    if(state==="valid"){valid++;hit+=BigInt(r.usage.hit);miss+=BigInt(r.usage.miss);}
    else if(state==="unknown")unknown++;else inconsistent++;
  }
  return {valid,unknown,inconsistent,hit:valid?String(hit):null,miss:valid?String(miss):null,hitRate:modelCacheRate(hit,miss)};
}


// One snapshot belongs to one process lifetime. Never retain groups across snapshots.
function modelTaskUsage(recent,meta){
  if(!validModelUsageRows(recent) || !validModelUsageMeta(meta))return null;
  const rows=recent.slice(-256),groups=new Map();
  const anonymous=s=>typeof s==="string" && /^[a-f0-9]{24}$/i.test(s)?s:null;
  rows.forEach((r,index)=>{
    const session=anonymous(r.session),task=anonymous(r.task);
    const grouped=session!==null && task!==null;
    const key=grouped?JSON.stringify([meta.startedAt,session,task]):JSON.stringify(["single",index]);
    if(!groups.has(key))groups.set(key,{session:grouped?session:null,task:grouped?task:null,grouped,rows:[]});
    groups.get(key).rows.push(r);
  });
  return {startedAt:meta.startedAt,count:rows.length,complete:false,groups:Array.from(groups.values()).map(g=>{
    const byModel=modelUsageBreakdown(g.rows);
    const totals=Object.fromEntries(["input","hit","miss","output"].map(f=>{
      const known=byModel.groups.reduce((n,m)=>n+m.totals[f].known,0);
      const sum=byModel.groups.reduce((n,m)=>n+BigInt(m.totals[f].value??0),0n);
      return [f,{value:known?String(sum):null,known}];
    }));
    return {session:g.session,task:g.task,grouped:g.grouped,count:g.rows.length,totals,cacheSummary:summarizeModelCache(g.rows),requests:g.rows};
  })};
}
function modelTaskUsageText(value,groups=value?.groups){
  if(!value)return "任务用量未知，缺少有效统计窗口。";
  if(!value.count)return "暂无近期请求，任务用量未知。";
  const names={input:"输入",hit:"命中",miss:"未命中",output:"输出"};
  const lines=[`统计起点：${value.startedAt}。当前窗口${value.count}次请求，${value.groups.length}组。仅包含最近最多256条已结束请求，可能缺少早期轮次；不代表任务完整用量、任务完成状态或费用。缺少有效会话/任务标识的请求逐条显示。`];
  lines.push(`匹配任务 ${groups.length}/${value.groups.length}；每个匹配任务显示全部窗口内请求。`);
  if(!groups.length)lines.push("当前条件无匹配任务。");
  for(const g of groups){
    lines.push(g.grouped?`\n会话${g.session} · 任务${g.task} · 窗口内${g.count}次请求`:`\n独立请求（缺少有效关联标识） · ${g.count}次请求`);
    lines.push(Object.entries(names).map(([f,n])=>`${n} ${g.totals[f].value??"未知"}（已知${g.totals[f].known}/${g.count}次）`).join(" / "));
    lines.push(`有效缓存统计${g.cacheSummary.valid}次 · 加权命中率${g.cacheSummary.hitRate??"未知"} · 分项不一致${g.cacheSummary.inconsistent}次`);
    for(const r of g.requests)lines.push(`#${r.id??"未知"} · 客户端请求序号${r.sequence??"未知"} · ${r.kind??"类别未知"} · ${r.provider??"提供商未知"} / ${r.model??"模型未知"} · ${r.outcome??"状态未知"} · ${r.startedAt??"时间未知"}\n`+Object.entries(names).map(([f,n])=>`${n} ${Number.isSafeInteger(r.usage?.[f]) && r.usage[f]>=0?r.usage[f]:"未知"}`).join(" / "));
  }
  return lines.join("\n");
}

// Shared with the existing hint summary; the offline report checker stays independent.
function modelRequestRisk(row,settings=modelCostSettings){
  const u=row.usage||{},known=n=>Number.isSafeInteger(n)&&n>=0;
  const incomplete=![u.input,u.hit,u.miss,u.output].every(known),state=modelCacheState(u);
  return {large:known(u.input)&&u.input>=settings.large,incomplete,inconsistent:state==="inconsistent",
    highMiss:!incomplete&&state==="valid"&&u.input>=settings.minimum&&BigInt(u.miss)*100n>=BigInt(u.input)*BigInt(settings.missPercent)};
}
function selectModelTaskGroups(value,filter="all",sort="original",settings=modelCostSettings){
  if(!value)return [];
  const groups=value.groups.filter(g=>!["large","highMiss","invalid"].includes(filter)||g.requests.some(r=>{
    const risk=modelRequestRisk(r,settings);return filter==="invalid"?risk.incomplete||risk.inconsistent:risk[filter];
  }));
  if(sort==="input"||sort==="miss")groups.sort((a,b)=>{
    const x=a.totals[sort].value,y=b.totals[sort].value;
    if(x===null)return y===null?0:1;if(y===null)return -1;
    return BigInt(x)===BigInt(y)?0:BigInt(x)>BigInt(y)?-1:1;
  });
  return groups;
}
function resetModelTaskView(){
  if($("model-task-filter"))$("model-task-filter").value="all";
  if($("model-task-sort"))$("model-task-sort").value="original";
}
function renderModelTaskView(){
  const value=modelTaskUsage(modelCostRows,modelUsageSnapshotMeta);
  for(const id of ["model-task-filter","model-task-sort"])if($(id))$(id).disabled=!value;
  if(!$("model-task-usage"))return;
  const groups=selectModelTaskGroups(value,$("model-task-filter")?.value,$("model-task-sort")?.value);
  $("model-task-usage").textContent=modelTaskUsageText(value,groups);
}
document.addEventListener("change",event=>{
  if(["model-task-filter","model-task-sort"].includes(event.target.id)&&typeof authenticated!=="undefined"&&authenticated)renderModelTaskView();
});
