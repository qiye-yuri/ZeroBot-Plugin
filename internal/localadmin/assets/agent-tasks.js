"use strict";
let tasksScope=null,tasksItems=[],tasksWriting=false,tasksNext="";
let tasksFilter="all";
let tasksReason="all";
let tasksFrom="",tasksUntil="";
let tasksPhase="all";
let tasksOutcome="all";
const taskOutcomeNames={all:"全部结果",in_flight:"等待回执",ok:"成功回执",rejected:"平台拒绝",unknown:"结果未知",local_error:"本地失败",none:"未记录结果",other:"其他或旧结果"};
const taskPhaseNames={all:"全部阶段",starting:"准备中",model_wait:"等待模型（含上下文准备）",fallback_model_wait:"等待普通聊天兜底模型",action_wait:"动作在途",between_actions:"动作已返回",finished:"结束"};
const taskReasonNames={all:"全部类别",none:"未记录错误类别",other:"其他或旧类别",storage_error:"状态存储失败",settings_changed:"设置已更新",repeated_action:"重复动作已停止",model_request_failed:"Agent 模型请求或响应解析失败",model_output_limit:"模型单轮输出超过容量、动作数量或结构限制",model_output_invalid:"模型动作回复格式无效，当前批次未执行",process_interrupted:"进程退出或重启",execution_error:"执行失败"};
function taskControls(){
  $("tasks-load").disabled=!authenticated||!agentVisible||!agentScope||agentBusy;
  $("tasks-filter").disabled=!authenticated||!agentVisible||!agentScope||agentBusy;
  $("tasks-reason").disabled=!authenticated||!agentVisible||!agentScope||agentBusy;
  $("tasks-phase").disabled=!authenticated||!agentVisible||!agentScope||agentBusy;
  $("tasks-outcome").disabled=!authenticated||!agentVisible||!agentScope||agentBusy;
  for(const id of ["tasks-from","tasks-until"])$(id).disabled=!authenticated||!agentVisible||!agentScope||agentBusy;
  $("tasks-next").disabled=!authenticated||!agentVisible||!tasksScope||!tasksNext||agentBusy;
  $("task-read-id").disabled=!authenticated||!agentVisible||!tasksScope||agentBusy||tasksWriting;
  $("task-read").disabled=!authenticated||!agentVisible||!tasksScope||agentBusy||tasksWriting;
  for(const button of $("tasks-list").querySelectorAll("button"))button.disabled=agentBusy||tasksWriting;
  for(const button of $("task-read-detail").querySelectorAll("button"))button.disabled=agentBusy||tasksWriting;
}
function clearTaskSource(){$("task-source").replaceChildren();}
function clearTaskRead(clearID=false){$("task-read-detail").replaceChildren();$("task-read-status").textContent="读取任务列表后，可按编号核对单条最新摘要；列表保持原读取结果。";if(clearID)$("task-read-id").value="";}
function resetAgentTasks(){tasksOutcome="all";$("tasks-outcome").value="all";tasksFrom="";tasksUntil="";$("tasks-from").value="";$("tasks-until").value="";clearTaskSource();clearTaskRead(true);tasksFilter="all";tasksReason="all";tasksPhase="all";$("tasks-filter").value="all";$("tasks-reason").value="all";$("tasks-phase").value="all";tasksScope=null;tasksItems=[];tasksWriting=false;tasksNext="";$("tasks-list").replaceChildren();$("tasks-status").textContent="读取档案后，可分页查看同一会话的任务，每页最多 50 个。";taskControls();}
function renderAgentTasks(items,scope,next="",older=false,filter="all",reason="all",observedAt=undefined,phase="all",from="",until="",outcome="all"){
  tasksFilter=filter;
  tasksReason=reason;
  tasksPhase=phase;
  tasksOutcome=outcome;
  tasksFrom=from;tasksUntil=until;
  tasksScope={...scope};tasksItems=items;tasksNext=next;$("tasks-list").replaceChildren();
  $("tasks-status").textContent="机器人 "+scope.botId+" / "+(scope.type==="group"?"群 ":"私聊 ")+scope.id+"："+(older?"较早任务，":"最新任务，")+"本页 "+items.length+" 个。"+(next?"可继续查看更早任务。":"")+"状态不会自动刷新；读取任务状态返回最新一页。";
  $("tasks-status").textContent+=" 已加载筛选："+({all:"全部",running:"运行中",cancel_requested:"已请求停止",cancelled:"已停止",completed:"已完成",failed:"失败",interrupted:"进程中断"}[filter])+"。";
  $("tasks-status").textContent+=" 错误类别："+taskReasonNames[reason]+"。";
  $("tasks-status").textContent+=" 阶段："+taskPhaseNames[phase]+"。";
  $("tasks-status").textContent+=" 最近动作结果："+taskOutcomeNames[outcome]+"（非累计结果）。";
  $("tasks-status").textContent+=" 任务开始时间："+(from||"不限")+"（含）至 "+(until||"不限")+"（不含）。";
  for(const item of items)$("tasks-list").append(taskSummaryRow(item,scope,true,observedAt));
}
function taskSummaryRow(item,scope,listEntry=true,observedAt=item.observedAt){
    const row=document.createElement("section");row.className="panel";
    const title=document.createElement("h3");title.textContent=({running:"运行中",cancel_requested:"已请求停止，等待在途请求返回",cancelled:"已停止",completed:"已完成",failed:"失败",interrupted:"进程中断，未重放"}[item.status]||"状态未知")+" · v"+item.version;
    const identity=document.createElement("p");identity.className="agent-record";identity.textContent="任务 "+item.taskId+" · 实例 "+item.instance;
    const detail=document.createElement("p");detail.textContent="阶段："+(taskPhaseNames[item.phase]||"未知")+(item.action?" · 最近动作 "+item.action:"")+(item.outcome?" · "+({in_flight:"等待回执",ok:"成功",rejected:"平台拒绝",unknown:"结果未知，不自动重发",local_error:"本地动作失败"}[item.outcome]||"结果未知"):"");
    const time=document.createElement("p");time.textContent="开始 "+item.startedAt+(item.endedAt?" · 结束 "+item.endedAt:"");
    const timing=document.createElement("p");timing.textContent=taskTimingText(item);
    const duration=document.createElement("p");duration.className="task-duration";duration.textContent=taskDurationText(item,observedAt);
    const note=document.createElement("p");note.textContent=item.persisted?"状态已保存。":"状态未可靠保存，此会话暂停新任务，请检查存储后重启服务。";
    if(item.error)note.textContent+=" "+(Object.hasOwn(taskReasonNames,item.error)&&!["all","none","other"].includes(item.error)?taskReasonNames[item.error]:"其他或旧错误类别");
    const usage=document.createElement("p");const c=item.context;
    usage.textContent=c?.available?"最近一次上下文准备："+c.records+" 条历史记录，含 "+c.searchSources+" 个检索来源片段；历史正文 "+c.historyBytes+" 字节，预算 "+c.budgetBytes+" 字节。"+(c.clipped?"达到历史容量限制，更早记录未装入。":"在最近重置边界和容量内取用。")+"此数量不代表模型已理解全部档案，也不等于接口 token 用量。":"尚无上下文取用摘要。";
    if(c?.available)usage.textContent+=typeof c.memorySources==="number"?" 另含 "+c.memorySources+" 个记忆检索/读取响应片段（可能重复，已截断且无法解析的响应不计）。":" 旧摘要未统计记忆查询片段。";
    const memory=document.createElement("p"),m=item.memory;
    memory.textContent="尚无长期记忆取用摘要（旧任务、尚未准备或普通聊天兜底）。";
    if(m?.reason==="read_error")memory.textContent="长期记忆读取失败，本次不能按无记忆处理。";
    else if(m?.available&&m.mode==="managed")memory.textContent="最近一次长期记忆准备："+m.records+" 条启用记忆，"+m.bytes+" 字节（含编号和版本标签）；预算 "+m.budgetBytes+" 字节、最多 "+m.limit+" 条。"+(m.clipped?({record_limit:"达到条数限制，更早启用记忆未装入。",byte_limit:"达到字节限制，更早启用记忆未装入。"}[m.reason]||"部分记忆未装入。" ):"本次范围内的启用记忆均已准备。")+"不含条目间换行、人物设定和工具协议，不是 token 用量或模型阅读证明。";
    else if(m?.available&&m.mode==="legacy")memory.textContent="长期记忆仍使用未迁移的旧文件；最终输入条数和字节数尚未统计，仍受模型提示容量限制。";
    else if(m?.available&&m.mode==="claimed_elsewhere")memory.textContent="旧记忆已归属其他机器人，本次未装入该旧文件。";
    const activity=document.createElement("p"),a=item.activity;
    activity.textContent=!item.persisted?"累计执行摘要未可靠保存，请结合存储故障核对。":a?.available?"本任务累计登记：Agent 模型准备 "+a.modelPreparations+" 次，普通聊天兜底准备 "+a.fallbackPreparations+" 次，本地动作 "+a.localActions+" 次，平台动作 "+a.platformActions+" 次。准备可能在 HTTP 请求前失败；动作登记不代表执行成功或消息已送达。":"此任务没有累计执行摘要（旧版本记录）。";
    const results=document.createElement("p"),r=item.results;
    results.textContent=!item.persisted?"累计动作结果未可靠保存，请结合存储故障核对。":r?.available&&[r.ok,r.rejected,r.unknown,r.localErrors].every(n=>Number.isSafeInteger(n)&&n>=0)?"本任务累计动作结果：成功回执 "+r.ok+" 次，平台拒绝 "+r.rejected+" 次，结果未知 "+r.unknown+" 次，本地失败 "+r.localErrors+" 次。"+(item.outcome==="in_flight"?"当前动作仍在途，尚未计入结果。":"")+"成功回执不证明 QQ 客户端收到或已读，停止不会逆转已执行动作。":"此任务没有可核对的累计动作结果（旧记录或摘要无效）。";
    const queries=document.createElement("p"),q=item.queries,validQueries=q?.available===true&&q.limit===3&&[q.used,q.searchHistory,q.readHistory,q.searchMemory,q.readMemory,q.successful,q.empty,q.partial,q.errors,q.denied].every(n=>Number.isSafeInteger(n)&&n>=0)&&q.used<=q.limit&&q.searchHistory+q.readHistory+q.searchMemory+q.readMemory===q.used&&q.successful+q.errors<=q.used&&q.empty<=q.successful&&q.partial<=q.successful;
    queries.textContent=!item.persisted?"任务查询摘要未可靠保存，请结合存储故障核对。":validQueries?"本任务历史与记忆查询：额度已用 "+q.used+"/"+q.limit+" 次（历史检索 "+q.searchHistory+"、历史读取 "+q.readHistory+"、记忆检索 "+q.searchMemory+"、记忆读取 "+q.readMemory+"）。成功响应 "+q.successful+" 次，其中空结果 "+q.empty+" 次、覆盖不完整 "+q.partial+" 次；参数或读取错误 "+q.errors+" 次，超额拒绝 "+q.denied+" 次。"+(q.used>q.successful+q.errors?"另有 "+(q.used-q.successful-q.errors)+" 次查询结果尚未确认。":"")+"空结果不证明全部资料不存在，覆盖不完整也不表示模型已阅读全部来源。":"此任务没有可核对的查询摘要（旧记录或摘要无效）。";
    const source=document.createElement("p"),hasSource=typeof item.sourceSeq==="string"&&/^[1-9]\d{0,18}$/.test(item.sourceSeq)&&BigInt(item.sourceSeq)<=9223372036854775807n;
    source.textContent=hasSource?"触发消息档案 #"+item.sourceSeq+"。只保存来源序号，正文需另行核对。":"触发来源未知（旧任务或未捕获档案序号），不补造消息。";
    const statistics=document.createElement("details"),statisticsTitle=document.createElement("summary");statistics.className="task-statistics";statistics.open=!listEntry;statisticsTitle.textContent="上下文、记忆与执行统计";statistics.append(statisticsTitle,usage,memory,activity,results,queries);
    row.append(title,identity,detail,time,timing,duration,note,statistics,source);
    if(listEntry){const latest=document.createElement("button");latest.type="button";latest.className="secondary";latest.textContent="核对最新状态";latest.onclick=()=>readAgentTask(item.taskId,{...scope});row.append(latest);}
    if(hasSource){const readSource=document.createElement("button");readSource.type="button";readSource.className="secondary";readSource.textContent="查看触发消息";readSource.onclick=()=>readTaskSource(item,{...scope});row.append(readSource);}
    if(listEntry&&item.status==="running"&&item.persisted){const stop=document.createElement("button");stop.className="secondary";stop.type="button";stop.textContent="请求停止此任务";stop.onclick=()=>cancelAgentTask(item,{...scope});row.append(stop);}
    return row;
}
function validTaskTime(stamp){return typeof stamp==="string"&&/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(stamp)&&Number.isFinite(Date.parse(stamp))&&new Date(stamp).toISOString().slice(0,19)===stamp.slice(0,19);}
function taskDurationText(item,observedAt){
  if(item.persisted!==true)return "任务状态未可靠保存，暂不计算耗时。";
  if(item.status==="interrupted")return "进程中断，实际退出时刻未知，无法确定任务耗时。";
  const running=["running","cancel_requested"].includes(item.status),end=running?observedAt:item.endedAt;
  const elapsed=start=>{
    if(!validTaskTime(start)||!validTaskTime(end)||Date.parse(end)<Date.parse(start))return "未知（时间缺失、无效或先后异常）";
    const seconds=Math.floor((Date.parse(end)-Date.parse(start))/1000),hours=Math.floor(seconds/3600),minutes=Math.floor(seconds%3600/60),rest=seconds%60;
    return "约 "+(hours?hours+" 小时 ":"")+(hours||minutes?minutes+" 分 ":"")+rest+" 秒";
  };
  let text=(running?"截至读取时已运行：":"任务总耗时：")+elapsed(item.startedAt)+"。";
  if(running)text+=" 当前阶段已持续："+elapsed(item.phaseStartedAt)+"。";
  if(item.cancelRequestedAt||["cancel_requested","cancelled"].includes(item.status))text+=(running?" 停止请求已等待：":" 停止请求至任务结束：")+elapsed(item.cancelRequestedAt)+"。";
  if(running&&validTaskTime(observedAt))text+=" 读取时刻："+observedAt+"。";
  return text+" 按服务端时间估算到秒；需重新读取更新，不代表单次模型或平台耗时。";
}
function taskTimingText(item){
  if(item.persisted!==true)return "阶段与停止请求时间未可靠保存，请结合存储故障核对。";
  let text=validTaskTime(item.phaseStartedAt)?"当前阶段开始："+item.phaseStartedAt+"。":"当前阶段开始时间未记录或无效（旧摘要可能缺失）。";
  if(validTaskTime(item.cancelRequestedAt))text+=" 首次停止请求："+item.cancelRequestedAt+"。";
  else if(item.cancelRequestedAt||["cancel_requested","cancelled"].includes(item.status))text+=" 首次停止请求时间未记录或无效，不能据此计算停止耗时。";
  if(item.status==="interrupted")text+=" 结束阶段时间为启动后发现遗留任务的时间，实际退出时刻未知。";
  return text+" 时间来自服务端时钟；页面不会自动刷新，不表示模型或平台请求的实际开始时间。";
}
async function readTaskSource(item,scope){
  if(!tasksScope||agentBusy||!agentMemoryCanLeave()||["botId","type","id"].some(key=>tasksScope[key]!==scope[key]))return;
  return agentRequest(async(signal,current)=>{
    clearTaskSource();$("task-source").textContent="正在读取任务触发消息…";
    try{
      const value=await api("/api/agent/history?"+new URLSearchParams({...scope,seq:item.sourceSeq,archived:"true"}),undefined,signal);if(!current())return;
      clearTaskSource();const target=$("task-source"),title=document.createElement("h3"),note=document.createElement("p");
      title.textContent="任务 "+item.taskId+" · 触发消息档案 #"+item.sourceSeq;
      note.textContent="机器人 "+scope.botId+" / "+(scope.type==="group"?"群 ":"私聊 ")+scope.id+"。包含重置前记录，仅只读核对，不会重新加入模型上下文；触发消息不代表每项动作已获授权或成功。";target.append(title,note);
      const entry=value.entries?.find(entry=>entry.seq===item.sourceSeq&&entry.kind==="event");
      if(!entry){const missing=document.createElement("p");missing.textContent=value.available?"当前会话未找到对应触发事件，来源不能据此确认。":"档案文件不可用，无法核对触发消息。";target.append(missing);return;}
      const stamp=document.createElement("p"),text=document.createElement("pre");stamp.textContent=entry.time+" · 事件";text.className="agent-record";text.textContent=entry.text;target.append(stamp,text);
      if(entry.truncated){const clipped=document.createElement("p");clipped.textContent="来源正文有截断或超限占位，原档案未修改。";target.append(clipped);}
    }catch(error){if(current())$("task-source").textContent="任务触发消息读取失败，请重试；不能据此确认来源。";throw error;}
  });
}
async function readAgentTask(taskId=$("task-read-id").value.trim(),target=tasksScope){
  if(!target||agentBusy||!agentMemoryCanLeave())return;
  const scope={...target};
  if(!/^[a-f0-9]{32}$/.test(taskId)){showError("agent-error",new Error("任务编号须为 32 位小写十六进制字符串。"));return;}
  return agentRequest(async(signal,current)=>{
    $("task-read-id").value=taskId;
    clearTaskSource();clearTaskRead();$("task-read-status").textContent="正在核对该任务…";
    try{
      const value=await api("/api/agent/tasks/read?"+new URLSearchParams({...scope,taskId}),undefined,signal);if(!current())return;
      if(value.taskId!==taskId||typeof value.version!=="string"||!/^[1-9]\d{0,18}$/.test(value.version)||BigInt(value.version)>9223372036854775807n)throw new Error("返回的任务编号或版本无法核对。");
      $("task-read-status").textContent="机器人 "+scope.botId+" / "+(scope.type==="group"?"群 ":"私聊 ")+scope.id+" · 已核对任务 "+taskId+" 的当前摘要。此处只读，列表筛选和分页保留；停止前请重新读取列表。";
      $("task-read-detail").append(taskSummaryRow(value,scope,false));$("task-read-status").scrollIntoView({block:"start"});
    }catch(error){if(current())$("task-read-status").textContent="单条任务核对失败，请核对编号后重试；不能据此确认任务已结束。";throw error;}
  });
}
async function loadAgentTasks(before=""){
  if(!agentScope||!agentMemoryCanLeave()||(before&&!tasksScope))return;const target=before?tasksScope:agentScope,scope={botId:target.botId,type:target.type,id:target.id};
  const filter=before?tasksFilter:$("tasks-filter").value;
  const reason=before?tasksReason:$("tasks-reason").value;
  const phase=before?tasksPhase:$("tasks-phase").value;
  const outcome=before?tasksOutcome:$("tasks-outcome").value;
  const dates=(before?[tasksFrom,tasksUntil]:[$("tasks-from").value,$("tasks-until").value]).map(value=>value?new Date(value):null);
  if(dates.some(date=>date&&!Number.isFinite(date.getTime()))||(dates[0]&&dates[1]&&dates[0]>=dates[1])){showError("agent-error",new Error("请填写有效任务开始时间，下限须早于上限。"));return;}
  const [from,until]=dates.map(date=>date?date.toISOString():"");
  return agentRequest(async(signal,current)=>{
    clearTaskSource();
    clearTaskRead();
    tasksItems=[];tasksScope=null;tasksNext="";$("tasks-list").replaceChildren();$("tasks-status").textContent="正在读取任务…";
    try{const value=await api("/api/agent/tasks?"+new URLSearchParams({...scope,state:filter,...(reason!=="all"?{reason}:{}),...(phase!=="all"?{phase}:{}),...(outcome!=="all"?{outcome}:{}),...(from?{from}:{}),...(until?{until}:{}),...(before?{before}:{})}),undefined,signal);if(current())renderAgentTasks(value.items,scope,value.more&&typeof value.next==="string"&&/^[1-9]\d{0,18}$/.test(value.next)?value.next:"",!!before,filter,reason,value.observedAt,phase,from,until,outcome);}
    catch(error){if(current())$("tasks-status").textContent="任务读取失败，请重试。";throw error;}
  });
}
async function cancelAgentTask(item,scope){
  if(agentBusy||tasksWriting||!tasksScope)return;
  const epoch=agentEpoch+1;tasksWriting=true;
  await agentRequest(async(signal,current)=>{
    clearTaskSource();
    clearTaskRead();
    // Consume the stale version locally. Never automatically retry an uncertain write.
    $("tasks-list").replaceChildren();tasksItems=[];tasksNext="";
    try{const value=await api("/api/agent/tasks/cancel",{...scope,taskId:item.taskId,version:item.version},signal);if(!current())return;
      if(!["cancel_requested","cancelled","completed","failed","interrupted"].includes(value.status))throw new Error("停止结果无法确认");
      $("tasks-filter").value="all";$("tasks-reason").value="all";$("tasks-phase").value="all";$("tasks-outcome").value="all";$("tasks-from").value="";$("tasks-until").value="";renderAgentTasks([value],scope);
    }catch(error){if(current()){$("tasks-status").textContent="停止未完成或结果未知，请重新读取任务核对；不会自动重试。";showError("agent-error",error);}}
  });
  if(agentEpoch===epoch){tasksWriting=false;taskControls();}
}
$("tasks-load").onclick=()=>loadAgentTasks();
$("tasks-next").onclick=()=>{if(tasksNext)loadAgentTasks(tasksNext);};
$("task-read-form").onsubmit=event=>{event.preventDefault();readAgentTask();};
window.addEventListener("beforeunload",event=>{if(tasksWriting){event.preventDefault();event.returnValue="";}});
taskControls();
