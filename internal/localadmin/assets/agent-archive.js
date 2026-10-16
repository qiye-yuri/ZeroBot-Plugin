"use strict";
let agentEpoch=0,agentAbort=null,agentVisible=false,agentBusy=false,agentScope=null,agentNext="",agentListNext="";
let agentSearchContinue=null;
function clearAgentSearchContinue() {
  agentSearchContinue=null;$("agent-search-continue").hidden=true;$("agent-search-continue").disabled=true;
}
function agentControls() {
  $("agent-search-continue").hidden=!agentSearchContinue;
  $("agent-search-continue").disabled=agentBusy||!agentSearchContinue;
  for(const id of ["agent-refresh","agent-bot","agent-type","agent-id","agent-archived"] ) $(id).disabled=agentBusy;
  $("agent-list-next").disabled=agentBusy||!agentListNext;
  $("agent-next").disabled=agentBusy||!agentNext;
  $("agent-search").disabled=agentBusy||!agentScope;
  $("agent-search-before").disabled=agentBusy||!agentScope;
  $("agent-search-sender").disabled=agentBusy||!agentScope;
  $("agent-search-event-type").disabled=agentBusy||!agentScope;
  for(const id of ["agent-search-from","agent-search-until"])$(id).disabled=agentBusy||!agentScope;
  $("agent-history-refresh").disabled=agentBusy||!agentScope;
  for(const id of ["agent-record-seq","agent-record-read"])$(id).disabled=!authenticated||!agentVisible||agentBusy||!agentScope;
  for(const control of $("agent-entries").querySelectorAll(".agent-detail-button,.agent-context-radius"))control.disabled=agentBusy;
  if(typeof memoryControls==="function")memoryControls();
  if(typeof taskControls==="function")taskControls();
}
function clearAgentContent() {
  clearAgentRecord();
  clearAgentSearchContinue();
  agentScope=null;agentNext="";$("agent-entries").replaceChildren();$("agent-keyword").value="";
  $("agent-search-before").value="";
  $("agent-search-sender").value="";
  $("agent-search-event-type").value="";
  $("agent-search-from").value="";$("agent-search-until").value="";
  if(typeof resetAgentMemory==="function")resetAgentMemory();
  if(typeof resetAgentTasks==="function")resetAgentTasks();
  $("agent-scope").textContent="尚未读取会话。";$("agent-status").textContent="";$("agent-error").hidden=true;
}
function resetAgentArchive() {
  agentEpoch++;agentAbort?.abort();agentAbort=null;agentBusy=false;agentVisible=false;agentListNext="";
  clearAgentContent();$("agent-bot").replaceChildren();$("agent-conversations").replaceChildren();
  $("agent-id").value="";$("agent-type").value="group";$("agent-archived").checked=false;agentControls();
}
function showAgentArchive(visible) {
  if(!visible) {resetAgentArchive();return;}
  agentVisible=true;loadAgentAccounts();
}
async function agentRequest(work) {
  if(!authenticated||!agentVisible||agentBusy)return;
  const epoch=++agentEpoch,token=csrf;agentAbort=new AbortController();const signal=agentAbort.signal;
  const current=()=>authenticated&&agentVisible&&epoch===agentEpoch&&csrf===token&&!signal.aborted;
  agentBusy=true;$("agent-error").hidden=true;agentControls();
  try {await work(signal,current);} catch(error) {if(current()) {clearAgentSearchContinue();agentNext="";agentListNext="";$("agent-status").textContent="读取未完成，请重新读取。";showError("agent-error",error);}}
  finally {if(epoch===agentEpoch){agentBusy=false;agentAbort=null;agentControls();}}
}
function renderAgentConversations(value) {
  agentListNext=value.nextCursor||"";const target=$("agent-conversations");target.replaceChildren();
  if(!value.conversations.length) {target.textContent=value.available?"没有已保存的会话。":"此机器人尚无档案文件。";return;}
  for(const item of value.conversations) {
    const button=document.createElement("button");button.type="button";button.className="secondary";
    button.textContent=(item.type==="group"?"群 ":"私聊 ")+item.id;
    button.addEventListener("click",()=>{if(agentBusy)return;$("agent-type").value=item.type;$("agent-id").value=item.id;clearAgentContent();loadAgentHistory();});target.append(button);
  }
}
async function loadAgentAccounts() {
  return agentRequest(async(signal,current)=>{
    clearAgentContent();agentListNext="";$("agent-conversations").replaceChildren();
    const previous=$("agent-bot").value;$("agent-bot").replaceChildren();
    const overview=await api("/api/overview",undefined,signal);if(!current())return;
    for(const id of overview.botAccounts||[]) {const option=document.createElement("option");option.value=id;option.textContent=id;$("agent-bot").append(option);}
    if((overview.botAccounts||[]).includes(previous))$("agent-bot").value=previous;
    if(!$("agent-bot").value){$("agent-status").textContent="暂无已连接机器人。连接后可查询已有档案。";return;}
    const page=await api("/api/agent/conversations?"+new URLSearchParams({botId:$("agent-bot").value}),undefined,signal);
    if(current())renderAgentConversations(page);
  });
}
async function loadAgentConversations(cursor="") {
  return agentRequest(async(signal,current)=>{
    clearAgentContent();agentListNext="";$("agent-conversations").replaceChildren();
    const page=await api("/api/agent/conversations?"+new URLSearchParams({botId:$("agent-bot").value,cursor}),undefined,signal);
    if(current())renderAgentConversations(page);
  });
}
function renderAgentEntries(value,search,query={}) {
  clearAgentSearchContinue();
  const target=$("agent-entries");target.replaceChildren();
  for(const item of value.entries) {
    const panel=document.createElement("section");panel.className="panel";
    const heading=document.createElement("h2");heading.textContent="#"+item.seq+" · "+({event:"事件",request:"动作尝试",response:"动作响应",end:"任务结束",reset:"上下文重置"}[item.kind]||"记录")+" · "+item.time;
    const body=document.createElement("pre");body.className="agent-record";body.textContent=item.text;
    panel.append(heading,body);if(item.truncated){const note=document.createElement("p");note.textContent="此条仅显示部分正文，磁盘原记录未更改。";panel.append(note);}
    if(search&&agentScope&&typeof item.seq==="string"&&/^[1-9]\d{0,18}$/.test(item.seq)){
      const scope={...agentScope},button=document.createElement("button"),detail=document.createElement("div");let expanded=false;
      button.type="button";button.className="secondary agent-detail-button";button.textContent="查看较长原文";button.setAttribute("aria-expanded","false");detail.setAttribute("aria-live","polite");
      button.onclick=()=>{
        if(agentBusy)return;
        if(expanded){expanded=false;detail.replaceChildren();button.textContent="查看较长原文";button.setAttribute("aria-expanded","false");return;}
        return agentRequest(async(signal,current)=>{
          detail.textContent="正在读取原文…";
          try{
            const value=await api("/api/agent/history?"+new URLSearchParams({botId:scope.botId,type:scope.type,id:scope.id,archived:String(scope.archived),seq:item.seq}),undefined,signal);if(!current())return;
            detail.replaceChildren();const entry=value.entries?.find(row=>row.seq===item.seq);
            const note=document.createElement("p");
            if(!entry)note.textContent=value.available?"当前会话与重置范围内未找到该记录，请重新搜索核对。":"档案文件不可用，无法核对原文。";
            else{
              const text=document.createElement("pre");text.className="agent-record";text.textContent=entry.text;detail.append(text);
              note.textContent=entry.truncated?"原文仍有截断或超限占位，最多显示 8,000 UTF-8 字节；磁盘记录未修改。":"已读取该条脱敏原文。";
            }
            detail.append(note);expanded=true;button.textContent="收起原文";button.setAttribute("aria-expanded","true");
          }catch(error){if(current())detail.textContent="原文读取失败，请重试；上方保留的是先前搜索片段。";throw error;}
        });
      };
      const older=document.createElement("button");older.type="button";older.className="secondary agent-detail-button";older.textContent="查此条之前";
      older.onclick=()=>loadAgentHistory({search:true,searchQuery:{...query,before:item.seq}});
      panel.append(button,older,detail);
      if(item.kind==="event"&&!scope.archived){
        const contextLabel=document.createElement("label"),contextRadius=document.createElement("select");
        contextLabel.textContent="相邻事件范围（每侧最多）";contextRadius.className="agent-context-radius";
        for(const radius of [1,2,3]){const option=document.createElement("option");option.value=String(radius);option.textContent=radius+" 条";contextRadius.append(option);}contextLabel.append(contextRadius);
        const contextButton=document.createElement("button"),contextDetail=document.createElement("div");let contextExpanded=false;
        contextButton.type="button";contextButton.className="secondary agent-detail-button";contextButton.textContent="查看前后事件";contextButton.setAttribute("aria-expanded","false");contextDetail.setAttribute("aria-live","polite");
        const collapseContext=()=>{contextExpanded=false;contextDetail.replaceChildren();contextButton.textContent="查看前后事件";contextButton.setAttribute("aria-expanded","false");};
        contextRadius.onchange=collapseContext;
        contextButton.onclick=()=>{
          if(agentBusy)return;
          if(contextExpanded){collapseContext();return;}
          const radius=Number(contextRadius.value);
          return agentRequest(async(signal,current)=>{
            contextDetail.textContent="正在读取前后事件…";
            try{
              const value=await api("/api/agent/history?"+new URLSearchParams({botId:scope.botId,type:scope.type,id:scope.id,archived:"false",seq:item.seq,context:String(radius)}),undefined,signal);if(!current())return;
              contextDetail.replaceChildren();const note=document.createElement("p"),rows=value.entries;
              if(value.available!==true)note.textContent="档案文件不可用，无法核对前后事件。";
              else if(Array.isArray(rows)&&rows.length===0)note.textContent="当前会话与重置范围内未找到中心事件，请重新搜索核对。";
              else if(value.anchorSeq!==item.seq||value.context!==radius||!Array.isArray(rows)||rows.length>2*radius+1||!rows.some(row=>row.seq===item.seq)||rows.some((row,index)=>row.kind!=="event"||typeof row.seq!=="string"||!(/^[1-9]\d{0,18}$/.test(row.seq))||BigInt(row.seq)>9223372036854775807n||typeof row.text!=="string"||(index>0&&BigInt(rows[index-1].seq)<=BigInt(row.seq)))||rows.filter(row=>BigInt(row.seq)>BigInt(item.seq)).length>radius||rows.filter(row=>BigInt(row.seq)<BigInt(item.seq)).length>radius){
                note.textContent="前后事件回执与请求不符，请重新读取核对。";
              }else{
                for(const row of rows){
                  const heading=document.createElement("h3"),text=document.createElement("pre");heading.textContent=(row.seq===item.seq?"中心事件":"相邻事件")+" #"+row.seq+" · "+row.time;text.className="agent-record";text.textContent=row.text;contextDetail.append(heading,text);
                }
                note.textContent="已核对中心，按来源序号倒序显示前后各最多 "+radius+" 条事件；不沿用搜索关键词、发言者或时间条件。中心最多 8,000、相邻各 2,000、合计最多 16 KiB UTF-8 字节。"+(value.partial||rows.some(row=>row.truncated)?"存在截断或超限占位，内容不完整。":"")+(rows.length===1?"当前范围内没有可展示的相邻事件。":"");
              }
              contextDetail.append(note);contextExpanded=true;contextButton.textContent="收起前后事件";contextButton.setAttribute("aria-expanded","true");
            }catch(error){if(current())contextDetail.textContent="前后事件读取失败，请重试；上方保留的是先前搜索片段。";throw error;}
          });
        };
        panel.append(contextLabel,contextButton,contextDetail);
      }
    }
    target.append(panel);
  }
  agentNext=search?"":value.nextCursor||"";
  $("agent-status").textContent=!value.available?"此机器人尚无档案文件。":(search?"搜索完成，检查 "+value.scanned+" 条，返回 ":"本页显示 ")+value.entries.length+" 条。"+(value.partial?"已达到读取或返回限制，仅搜索部分内容。":"")+(!value.entries.length?"当前范围内未找到记录。":"");
  if(search)$("agent-status").textContent+=" 查询范围："+(query.before?"序号 #"+query.before+" 之前（不含该条）。":"未限定来源序号。 ");
  if(search&&query.sender)$("agent-status").textContent+=" 发言者 QQ："+query.sender+"（仅消息事件，不按昵称或正文推断）。";
  if(search&&(query.from||query.until))$("agent-status").textContent+=" 档案保存时间（UTC）："+(query.from||"不限")+"（含）至 "+(query.until||"不限")+"（不含）。";
  if(search&&value.available===true){
    const next=value.nextBefore,valid=typeof next==="string"&&/^[1-9]\d{0,18}$/.test(next)&&BigInt(next)<=9223372036854775807n&&(!query.before||BigInt(next)<BigInt(query.before));
    if(value.searchMore===true&&valid){
      agentSearchContinue={scope:{...agentScope},query:{...query,before:next}};
      $("agent-status").textContent+=" 可继续搜索更早记录，下一次查序号 #"+next+" 之前。";
    }else if(value.searchMore===true)$("agent-status").textContent+=" 继续位置无效，请重新搜索核对。";
    else if(value.partial)$("agent-status").textContent+=" 本次没有可用继续位置；片段截断或跳过记录仍可能导致覆盖不完整。";
  }
}
function agentLocalTime(value) {
  if(!value)return "";
  const date=new Date(value),pad=value=>String(value).padStart(2,"0");
  return date.getFullYear()+"-"+pad(date.getMonth()+1)+"-"+pad(date.getDate())+"T"+pad(date.getHours())+":"+pad(date.getMinutes())+":"+pad(date.getSeconds());
}
function clearAgentRecord(){
  $("agent-record-seq").value="";$("agent-record-detail").replaceChildren();
  $("agent-record-state").textContent="读取会话档案后，可按来源序号核对单条原文，当前时间线和搜索结果保留。";
}
async function readAgentRecord(){
  if(!agentScope||agentBusy||(typeof agentMemoryCanLeave==="function"&&!agentMemoryCanLeave()))return;
  const scope={...agentScope},seq=$("agent-record-seq").value.trim();
  if(!/^[1-9]\d{0,18}$/.test(seq)||BigInt(seq)>9223372036854775807n){showError("agent-error",new Error("档案来源序号须为有效正整数字符串，不能填写任务编号或记忆修订号。"));return;}
  return agentRequest(async(signal,current)=>{
    $("agent-record-seq").value=seq;$("agent-record-detail").replaceChildren();$("agent-record-state").textContent="正在定位档案 #"+seq+"…";
    try{
      const value=await api("/api/agent/history?"+new URLSearchParams({botId:scope.botId,type:scope.type,id:scope.id,archived:String(scope.archived),seq}),undefined,signal);if(!current())return;
      $("agent-record-state").textContent="机器人 "+scope.botId+" / "+(scope.type==="group"?"群 ":"私聊 ")+scope.id+(scope.archived?" / 包含重置前记录":" / 最近重置之后")+" · 定位来源 #"+seq+"。不应用搜索关键词、发言者、时间或事件类型条件；时间线和搜索位置保留。";
      const target=$("agent-record-detail"),entry=value.entries?.find(row=>row.seq===seq),note=document.createElement("p");
      if(value.available!==true)note.textContent="档案文件不可用，无法核对该来源。";
      else if(!entry)note.textContent="当前会话与重置范围内未找到该序号，不能据此确认引用内容。";
      else{
        const title=document.createElement("h3"),text=document.createElement("pre");title.textContent="#"+entry.seq+" · "+({event:"事件",request:"动作尝试",response:"动作响应",end:"任务结束",reset:"上下文重置"}[entry.kind]||"记录")+" · "+entry.time;text.className="agent-record";text.textContent=entry.text;target.append(title,text);
        note.textContent=entry.truncated?"正文有截断或超限占位，最多显示 8,000 UTF-8 字节，原档案未修改。":"已读取该条脱敏原文，仅供只读核对；不证明引用结论或动作执行结果。";
      }
      target.append(note);
    }catch(error){if(current()){$("agent-record-detail").replaceChildren();$("agent-record-state").textContent="档案定位失败，请重试；不能据此判断来源不存在。";showError("agent-error",error);}}
  });
}
async function loadAgentHistory({cursor="",search=false,reload=false,searchQuery=null,searchScope=null}={}) {
  if(agentBusy)return;
  const scope=search&&searchScope?searchScope:(search||cursor||reload)?agentScope:{botId:$("agent-bot").value,type:$("agent-type").value,id:$("agent-id").value.trim(),archived:$("agent-archived").checked};
  if(!scope||!scope.botId||!/^\d{1,19}$/.test(scope.id)){showError("agent-error",new Error("请选择机器人并填写有效会话编号。"));return;}
  const keyword=searchQuery?searchQuery.keyword:$("agent-keyword").value.trim(),before=searchQuery?searchQuery.before:$("agent-search-before").value.trim();
  const sender=searchQuery?(searchQuery.sender||""):$("agent-search-sender").value.trim();
  const eventType=searchQuery?(searchQuery.eventType||""):$("agent-search-event-type").value;
  if(search&&sender&&(!/^[1-9]\d{0,18}$/.test(sender)||BigInt(sender)>9223372036854775807n)){showError("agent-error",new Error("发言者 QQ 须为有效正整数字符串，或留空不限。"));return;}
  if(search&&before&&(!/^[1-9]\d{0,18}$/.test(before)||BigInt(before)>9223372036854775807n)){showError("agent-error",new Error("请填写有效的正整数来源序号，或留空。"));return;}
  let from="",until="";
  if(search){
    const bounds=searchQuery?[searchQuery.from||"",searchQuery.until||""]:[$("agent-search-from").value,$("agent-search-until").value];
    const dates=bounds.map(value=>value?new Date(value):null);
    if(dates.some(date=>date&&!Number.isFinite(date.getTime()))||(dates[0]&&dates[1]&&dates[0]>=dates[1])){showError("agent-error",new Error("请填写有效时间，开始时间须早于结束时间。"));return;}
    [from,until]=dates.map(date=>date?date.toISOString():"");
  }
  if(search&&!keyword&&!sender&&!from&&!until&&!eventType){showError("agent-error",new Error("请填写关键词或指定发言者 QQ，也可填写时间范围或选择事件类型。"));return;}
  return agentRequest(async(signal,current)=>{
    clearAgentRecord();
    clearAgentSearchContinue();
    agentNext="";$("agent-entries").replaceChildren();$("agent-status").textContent="正在读取…";
    // Keep the last successfully loaded scope; never retarget a search to draft inputs.
    const value=search?await api("/api/agent/history/search",{...scope,keyword,...(before?{before}:{}),...(from?{from}:{}),...(until?{until}:{}),...(sender?{sender}:{}),...(eventType?{eventType}:{})},signal):await api("/api/agent/history?"+new URLSearchParams({...scope,archived:String(scope.archived),cursor}),undefined,signal);
    if(!current())return;
    if(typeof resetAgentMemory==="function"&&(!agentScope||["botId","type","id"].some(key=>agentScope[key]!==scope[key])))resetAgentMemory();
    if(typeof resetAgentTasks==="function"&&(!agentScope||["botId","type","id"].some(key=>agentScope[key]!==scope[key])))resetAgentTasks();
    agentScope={...scope};$("agent-scope").textContent="已加载：机器人 "+scope.botId+" / "+(scope.type==="group"?"群 ":"私聊 ")+scope.id+(scope.archived?" / 包含重置前记录":" / 最近重置之后");
    $("agent-search-before").value=search?before:"";
    $("agent-search-sender").value=search?sender:"";
    $("agent-search-event-type").value=search?eventType:"";
    $("agent-search-from").value=agentLocalTime(from);$("agent-search-until").value=agentLocalTime(until);
    if(search)$("agent-keyword").value=keyword;
    renderAgentEntries(value,search,{keyword,before,from,until,sender:search?sender:"",eventType:search?eventType:""});
    if(search)$("agent-status").textContent+=" 事件类型："+({"":"不限",message:"聊天消息",notice:"通知",request:"请求"}[eventType])+"（按保存事件的 post_type）。";
  });
}
$("agent-refresh").addEventListener("click",loadAgentAccounts);
$("agent-bot").addEventListener("change",()=>loadAgentConversations());
for(const id of ["agent-type","agent-id","agent-archived"])$(id).addEventListener("change",()=>{if(!agentBusy){clearAgentContent();agentControls();}});
$("agent-scope-form").addEventListener("submit",event=>{event.preventDefault();loadAgentHistory();});
$("agent-search-form").addEventListener("submit",event=>{event.preventDefault();loadAgentHistory({search:true});});
$("agent-record-form").addEventListener("submit",event=>{event.preventDefault();readAgentRecord();});
$("agent-search-continue").addEventListener("click",()=>{
  if(agentBusy||!agentSearchContinue)return;
  const saved=agentSearchContinue;
  loadAgentHistory({search:true,searchQuery:saved.query,searchScope:saved.scope});
});
$("agent-history-refresh").addEventListener("click",()=>loadAgentHistory({reload:true}));
$("agent-next").addEventListener("click",()=>{if(agentNext)loadAgentHistory({cursor:agentNext});});
$("agent-list-next").addEventListener("click",()=>{if(agentListNext)loadAgentConversations(agentListNext);});
agentControls();
