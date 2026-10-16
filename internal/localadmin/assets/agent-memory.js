"use strict";
let memoryScope=null,memoryItems=[],memoryNext="",memoryMode="",memoryPreview=null,memorySelected=null,memoryWriting=false;
let memoryFilter="all";
let memorySearchContinue=null;
let memoryCurrent=null;
function clearCurrentMemory(clearID=false){memoryCurrent=null;$("memory-current-detail").replaceChildren();$("memory-current-state").textContent="按编号读取当前版本，可定位列表页之外或已停用的记忆。";if(clearID)$("memory-current-id").value="";}
function clearMemorySearchContinue(){memorySearchContinue=null;$("memory-search-continue").hidden=true;$("memory-search-continue").disabled=true;}
function memoryTarget(scope=agentScope){return scope?{botId:scope.botId,type:scope.type,id:scope.id}:null;}
function memoryControls(){
  const ready=authenticated&&agentVisible&&!!agentScope&&!agentBusy;
  $("memory-load").disabled=!ready;
  $("memory-filter").disabled=!ready;
  $("memory-search").disabled=!ready||!memoryScope||memoryMode!=="managed";
  $("memory-search-continue").hidden=!memorySearchContinue;
  $("memory-search-continue").disabled=!ready||!memorySearchContinue||!agentMemoryCanLeave();
  $("memory-create").disabled=!ready||!memoryScope||memoryMode!=="managed";
  $("memory-keyword").disabled=agentBusy||!memoryScope;
  $("memory-search-before").disabled=agentBusy||!memoryScope;
  $("memory-search-source").disabled=agentBusy||!memoryScope;
  $("memory-search-state-filter").disabled=agentBusy||!memoryScope;
  $("memory-search-from").disabled=agentBusy||!memoryScope;
  $("memory-search-until").disabled=agentBusy||!memoryScope;
  $("memory-current-id").disabled=agentBusy||!memoryScope||memoryMode!=="managed";
  $("memory-current-load").disabled=!ready||!memoryScope||memoryMode!=="managed"||!agentMemoryCanLeave();
  for(const button of $("memory-current-detail").querySelectorAll("button"))button.disabled=agentBusy;
  for(const button of $("memory-search-results").querySelectorAll("button"))button.disabled=agentBusy;
  for(const control of $("memory-versions").querySelectorAll("button,input"))control.disabled=agentBusy;
  $("memory-next").disabled=!ready||!memoryNext;
  $("memory-preview").disabled=!ready||!["legacy","pending"].includes(memoryMode);
  $("memory-migrate").disabled=!ready||!memoryPreview;
  $("memory-cancel-preview").disabled=agentBusy||!memoryPreview;
  $("memory-save").disabled=agentBusy||!memorySelected;
  $("memory-cancel").disabled=memoryWriting;
  $("memory-text").disabled=memoryWriting||$("memory-action").value!=="edit";
  $("memory-text").readOnly=!!memorySelected?.targetRevision;
  $("memory-action").disabled=memoryWriting||!!memorySelected?.targetRevision||!!memorySelected?.create;
}
function resetAgentMemory(){
  clearMemorySearch(true);
  clearCurrentMemory(true);
  memoryFilter="all";$("memory-filter").value="all";
  memoryScope=null;memoryItems=[];memoryNext="";memoryMode="";memoryPreview=null;memorySelected=null;memoryWriting=false;
  $("memory-list").replaceChildren();$("memory-versions").replaceChildren();$("memory-preview-text").textContent="";
  $("memory-source").replaceChildren();
  $("memory-state").textContent="读取档案后，可查看同一会话的长期记忆。";$("memory-dialog").close();$("memory-text").value="";$("memory-dialog-error").textContent="";
  memoryControls();
}
function clearMemorySearch(clearKeyword=false){
  clearMemorySearchContinue();
  $("memory-search-results").replaceChildren();$("memory-search-state").textContent="搜索已加载会话的当前独立记忆，默认仅启用，可选择停用或全部；结果为只读片段。";
  $("memory-search-before").value="";
  $("memory-search-source").value="";
  $("memory-search-state-filter").value="active";
  $("memory-search-from").value="";$("memory-search-until").value="";
  if(clearKeyword)$("memory-keyword").value="";
}
async function searchAgentMemory(query=null){
  if(!memoryScope||memoryMode!=="managed"||!agentMemoryCanLeave())return;
  const scope={...(query?.scope||memoryScope)},keyword=query?query.keyword:$("memory-keyword").value.trim(),before=query?query.before:$("memory-search-before").value.trim();
  const source=query?(query.source||""):$("memory-search-source").value;
  const state=query?(query.state||"active"):$("memory-search-state-filter").value;
  const bounds=query?[query.from||"",query.until||""]:[$("memory-search-from").value,$("memory-search-until").value];
  const dates=bounds.map(value=>value?new Date(value):null);
  if(dates.some(date=>date&&!Number.isFinite(date.getTime()))||(dates[0]&&dates[1]&&dates[0]>=dates[1])){showError("agent-error",new Error("请填写有效修订时间，开始时间须早于结束时间。"));return;}
  const [from,until]=dates.map(date=>date?date.toISOString():"");
  if(["botId","type","id"].some(key=>scope[key]!==memoryScope[key]))return;
  if((!keyword&&!source&&!from&&!until)||Array.from(keyword).length>128){showError("agent-error",new Error("须填写关键词或选择记忆来源，也可指定修订时间；关键词最多 128 个字符。"));return;}
  if(before&&(!/^[1-9]\d{0,18}$/.test(before)||BigInt(before)>9223372036854775807n)){showError("agent-error",new Error("记忆编号边界须为正整数字符串，不能填写来源序号或修订号。"));return;}
  return agentRequest(async(signal,current)=>{
    clearMemorySearch();$("memory-source").replaceChildren();$("memory-search-state").textContent="正在搜索记忆…";
    $("memory-search-before").value=before;if(query)$("memory-keyword").value=keyword;
    $("memory-search-source").value=source;
    $("memory-search-state-filter").value=state;
    $("memory-search-from").value=agentLocalTime(from);$("memory-search-until").value=agentLocalTime(until);
    try{
      const body={...scope,keyword};if(before)body.before=before;if(source)body.source=source;
      if(from)body.from=from;if(until)body.until=until;
      if(state!=="active")body.state=state;
      const value=await api("/api/agent/memories/search",body,signal);if(!current())return;
      $("memory-search-state").textContent="机器人 "+scope.botId+" / "+(scope.type==="group"?"群 ":"私聊 ")+scope.id+" · 已加载关键词："+(keyword||(from||until?"不限（按修订时间及其他已应用条件）":"不限（仅按来源）"))+" · "+(before?"仅查记忆编号小于 #"+before:"记忆编号不限")+"："+(value.available?"检查 "+value.scanned+" 条记忆，返回 "+value.memories.length+" 个只读片段。"+(value.partial?"仅覆盖部分内容，请核对限制。":"")+(value.memories.length?"":"本次检查未命中，不代表全部历史不存在。"):"当前独立记忆不可检索，请核对归属或迁移状态。");
      $("memory-search-state").textContent+=" 已加载搜索状态："+({active:"仅启用",disabled:"仅停用",all:"全部"}[state])+"。停用条目不会因此装入 Agent。";
      $("memory-search-state").textContent+=" 来源条件："+({"":"全部来源",agent:"Agent 保存",admin:"后台录入或修订",legacy:"旧文件导入"}[source])+"（按当前版本）。";
      $("memory-search-state").textContent+=" 修订时间："+(from?"从 "+from+"（含）":"开始不限")+"，"+(until?"至 "+until+"（不含）":"结束不限")+"；包括编辑、启停及恢复，仍按记忆编号排序。";
      const next=value.nextBefore,valid=typeof next==="string"&&/^[1-9]\d{0,18}$/.test(next)&&BigInt(next)<=9223372036854775807n&&(!before||BigInt(next)<BigInt(before));
      if(value.available===true&&value.mode==="managed"&&value.searchMore===true&&valid){
        memorySearchContinue={scope:{...scope},keyword,before:next,source,state,from,until};
        $("memory-search-state").textContent+=" 可继续搜索记忆编号 #"+next+" 之前的条目。";
      }else if(value.searchMore===true)$("memory-search-state").textContent+=" 继续位置不可用，请重新搜索核对。";
      else if(value.partial)$("memory-search-state").textContent+=" 本次没有可用继续位置；截断或跳过记录不代表还有下一批。";
      for(const item of value.memories){
        const row=document.createElement("section");row.className="panel";const title=document.createElement("h3");title.textContent="只读记忆 #"+item.id+" · v"+item.revision+" · "+(item.active?"启用":"停用");
        const text=document.createElement("pre");text.className="agent-record";text.textContent=item.text;row.append(title,text);
        if(item.truncated){const note=document.createElement("p");note.textContent="这是截断片段，不能直接作为完整正文修订。";row.append(note);}
        const read=document.createElement("button"),detail=document.createElement("div");read.type="button";read.className="secondary";read.textContent="读取较长记忆正文";detail.setAttribute("aria-live","polite");
        read.onclick=()=>{if(!agentMemoryCanLeave())return;return agentRequest(async(signal,current)=>{
          detail.textContent="正在核对当前版本…";
          try{
            const value=await api("/api/agent/memories/read?"+new URLSearchParams({...scope,memoryId:item.id,revision:item.revision}),undefined,signal);if(!current())return;
            detail.replaceChildren();const entry=value.memories?.find(entry=>entry.id===item.id&&entry.revision===item.revision);
            const note=document.createElement("p");
            if(!entry)note.textContent="此记忆已变更、停用或当前范围不可读取，请重新搜索核对。";
            else{const full=document.createElement("pre");full.className="agent-record";full.textContent=entry.text;detail.append(full);note.textContent=entry.truncated?"正文仍有截断或超限占位，原记忆未修改。":"已核对该条当前启用版本的脱敏正文。";}
            detail.append(note);
          }catch(error){if(current())detail.textContent="记忆正文读取失败，请重试；先前片段不代表当前版本已核对。";throw error;}
        });};
        if(item.active!==false)row.append(read,detail);
        else{const note=document.createElement("p");note.textContent="此条已停用，可定位当前记忆核对完整正文并按需恢复；不会自动启用。";row.append(note);}
        if(typeof item.id==="string"&&/^[1-9]\d{0,18}$/.test(item.id)&&BigInt(item.id)<=9223372036854775807n){
          const locate=document.createElement("button");locate.type="button";locate.className="secondary";locate.textContent="定位当前记忆";locate.onclick=()=>loadCurrentMemory(item.id);row.append(locate);
          const older=document.createElement("button");older.type="button";older.className="secondary";older.textContent="查此条之前";
          older.onclick=()=>searchAgentMemory({scope,keyword,before:item.id,source,state,from,until});row.append(older);
        }
        if(typeof item.sourceSeq==="string"&&/^[1-9]\d{0,18}$/.test(item.sourceSeq)){const source=document.createElement("button");source.type="button";source.className="secondary";source.textContent="查看来源";source.onclick=()=>loadMemorySource(item);row.append(source);}
        $("memory-search-results").append(row);
      }
    }catch(error){if(current()){clearMemorySearchContinue();$("memory-search-state").textContent="记忆搜索失败，请重试；不能据此判断没有记忆。";}throw error;}
  });
}
function agentMemoryCanLeave(){return !memoryWriting&&!$("memory-dialog").open&&(typeof tasksWriting==="undefined"||!tasksWriting);}
function renderMemoryComparison(target,previous,current,historical=false){
  target.replaceChildren();
  const title=document.createElement("h4");title.textContent=historical?"历史 v"+previous.revision+" 与历史 v"+current.revision+" 对比":"v"+previous.revision+" 与已加载当前 v"+current.revision+" 对比";target.append(title);
  const note=document.createElement("p");note.textContent=historical?"仅比较所选两次修订，不改变当前记忆。删除线表示基准正文差异，下划线表示目标正文差异；多个修改点可能合并标示。":"比较读取时的版本；期间可能有新修订，保存仍检查版本冲突。删除线表示旧正文差异，下划线表示当前正文差异；多个修改点可能合并标示。";target.append(note);
  const status=document.createElement("p");status.textContent="启停状态："+(historical?"基准修订":"旧版本")+(previous.active?"启用":"停用")+" → "+(historical?"目标修订":"已加载当前版本")+(current.active?"启用":"停用")+"。";target.append(status);
  const oldChars=Array.from(previous.text),newChars=Array.from(current.text);let start=0,end=0;
  while(start<oldChars.length&&start<newChars.length&&oldChars[start]===newChars[start])start++;
  while(end<oldChars.length-start&&end<newChars.length-start&&oldChars[oldChars.length-1-end]===newChars[newChars.length-1-end])end++;
  if(previous.text===current.text){const same=document.createElement("p");same.textContent="脱敏后的正文相同。";target.append(same);}
  for(const [chars,tag,label] of [[oldChars,"del",historical?"基准历史正文":"旧版本正文"],[newChars,"ins",historical?"目标历史正文":"已加载当前正文"]]){
    const heading=document.createElement("p");heading.textContent=label;const text=document.createElement("pre");text.className="agent-record";
    text.append(document.createTextNode(chars.slice(0,start).join("")));
    const changed=chars.slice(start,chars.length-end).join("");if(changed){const mark=document.createElement(tag);mark.textContent=changed;text.append(mark);}
    text.append(document.createTextNode(end?chars.slice(chars.length-end).join(""):""));target.append(heading,text);
  }
}
function memoryHistoryComparison(id,scope,initialRevision){
  const section=document.createElement("section"),form=document.createElement("form"),button=document.createElement("button"),result=document.createElement("div"),inputs=[];
  section.className="memory-history-compare";form.className="plugin-scope";result.className="memory-history-comparison";result.setAttribute("aria-live","polite");
  for(const [name,labelText,value] of [["from","基准历史修订",""],["to","目标历史修订",initialRevision]]){
    const label=document.createElement("label"),input=document.createElement("input");label.textContent=labelText;input.className="memory-compare-"+name;input.inputMode="numeric";input.maxLength=19;input.required=true;input.value=value;input.placeholder="填写修订号";label.append(input);form.append(label);inputs.push(input);
  }
  button.type="submit";button.className="secondary";button.textContent="对比两次修订";form.append(button);section.append(form,result);
  form.onsubmit=event=>{
    event.preventDefault();if(agentBusy||!memoryScope||!agentMemoryCanLeave())return;
    const revisions=inputs.map(input=>input.value.trim());
    if(revisions.some(revision=>!/^[1-9]\d{0,18}$/.test(revision)||BigInt(revision)>9223372036854775807n)){showError("agent-error",new Error("对比修订号须为有效正整数字符串。"));return;}
    return agentRequest(async(signal,current)=>{
      result.textContent="正在读取两次历史修订…";
      try{
        const items=[];
        for(const revision of revisions){
          const value=await api("/api/agent/memories/revision?"+new URLSearchParams({...scope,memoryId:id,revision}),undefined,signal);if(!current())return;
          if(value.mode!=="managed"||value.items.length===0){result.textContent="当前会话内未找到历史修订 v"+revision+"，无法完成对比；当前记忆未修改。";return;}
          if(value.items.length!==1||value.items[0].id!==id||value.items[0].revision!==revision)throw new Error("历史修订回执与请求不符，请重新读取。");
          items.push(value.items[0]);
        }
        renderMemoryComparison(result,items[0],items[1],true);
      }catch(error){if(current()){result.textContent="修订对比未完成，请重新核对；当前记忆未修改。";showError("agent-error",error);}}
    });
  };
  return section;
}
function openMemoryEdit(item,previous=null){
  if(agentBusy||!memoryScope||!agentMemoryCanLeave())return;
  memorySelected={...item};if(previous)memorySelected.targetRevision=previous.revision;
  $("memory-text").value=(previous||item).text;$("memory-action").value="edit";$("memory-dialog-error").textContent="";
  $("memory-dialog-title").textContent="记忆 #"+item.id+" · 当前 v"+item.revision+(previous?" · 恢复 v"+previous.revision+" 正文":"");
  $("memory-revert-note").hidden=!previous;
  $("memory-revert-note").textContent=previous?"确认后从服务端历史版本恢复正文，保存为新修订，保留全部历史与来源序号。当前"+(item.active?"启用":"停用")+"状态不变；下方为脱敏预览。":"";
  $("memory-dialog").showModal();memoryControls();
}
function openMemoryCreate(){
  if(agentBusy||!memoryScope||memoryMode!=="managed"||!agentMemoryCanLeave())return;
  memorySelected={create:true};$("memory-text").value="";$("memory-action").value="edit";$("memory-dialog-error").textContent="";
  $("memory-dialog-title").textContent="新增记忆 · 机器人 "+memoryScope.botId+" / "+(memoryScope.type==="group"?"群 ":"私聊 ")+memoryScope.id;
  $("memory-revert-note").hidden=false;$("memory-revert-note").textContent="确认后保存为启用的后台记忆，来源档案未知。相同启用正文会复用已有条目并保留原来源，不新增版本。";
  $("memory-dialog").showModal();memoryControls();
}
function memoryRows(items,versions=false,targetID=""){
  const target=$(targetID||(versions?"memory-versions":"memory-list"));target.replaceChildren();
  for(const item of items){
    const row=document.createElement("section");row.className="panel";
    const h=document.createElement("h3");h.textContent="#"+item.id+" · v"+item.revision+" · "+(item.active?"启用":"停用");
    const note=document.createElement("p");note.textContent=({agent:"Agent 保存",admin:"后台修订",legacy:"旧文件导入，原始来源未知"}[item.source]||"来源未知")+(item.sourceSeq!=="0"?" · 档案 #"+item.sourceSeq:"")+" · "+item.updatedAt;
    const text=document.createElement("pre");text.className="agent-record";text.textContent=item.text;row.append(h,note,text);
    if(!versions){
      const edit=document.createElement("button");edit.className="secondary";edit.textContent="修订或启停";edit.type="button";
      edit.onclick=()=>openMemoryEdit(item);
      const history=document.createElement("button");history.className="secondary";history.textContent="查看版本";history.type="button";history.onclick=()=>loadMemoryVersions(item.id);row.append(edit,history);
    }
    if(typeof item.sourceSeq==="string"&&/^[1-9]\d{0,18}$/.test(item.sourceSeq)){
      const source=document.createElement("button");source.type="button";source.className="secondary";source.textContent="查看来源";source.onclick=()=>loadMemorySource(item);row.append(source);
    }
    const latest=versions?(memoryCurrent?.id===item.id?memoryCurrent:memoryItems.find(current=>current.id===item.id)):null;
    if(latest&&/^[1-9]\d{0,18}$/.test(item.revision)&&/^[1-9]\d{0,18}$/.test(latest.revision)&&BigInt(item.revision)<BigInt(latest.revision)){
      const revert=document.createElement("button");revert.type="button";revert.className="secondary";revert.textContent="恢复此版本正文";revert.onclick=()=>openMemoryEdit(latest,item);row.append(revert);
      const compare=document.createElement("button"),comparison=document.createElement("section");compare.type="button";compare.className="secondary";compare.textContent="对比当前版本";compare.setAttribute("aria-expanded","false");comparison.className="memory-comparison";comparison.setAttribute("aria-live","polite");
      compare.onclick=()=>{if(agentBusy||!agentMemoryCanLeave())return;const open=compare.getAttribute("aria-expanded")==="true";comparison.replaceChildren();if(!open)renderMemoryComparison(comparison,item,latest);compare.setAttribute("aria-expanded",String(!open));compare.textContent=open?"对比当前版本":"收起对比";};row.append(compare,comparison);
    }
    target.append(row);
  }
}
async function loadMemorySource(item){
  if(!memoryScope||!agentMemoryCanLeave())return;const scope={...memoryScope},seq=item.sourceSeq;
  return agentRequest(async(signal,current)=>{
    const target=$("memory-source");target.replaceChildren();target.textContent="正在读取来源…";
    try{
      const value=await api("/api/agent/history?"+new URLSearchParams({...scope,seq,archived:"true"}),undefined,signal);if(!current())return;
      target.replaceChildren();const title=document.createElement("h3");title.textContent="记忆 #"+item.id+" · v"+item.revision+" 的来源档案 #"+seq;target.append(title);
      const note=document.createElement("p");note.textContent="机器人 "+scope.botId+" / "+(scope.type==="group"?"群 ":"私聊 ")+scope.id+"。来源查看包含重置前记录，只用于核对，不会重新加入模型上下文。";target.append(note);
      const entry=value.entries?.find(entry=>entry.seq===seq);
      if(!entry){const missing=document.createElement("p");missing.textContent=value.available?"此会话内未找到来源记录，不能据此认定记忆内容已被证实。":"此机器人的档案文件不可用，无法核对来源。";target.append(missing);return;}
      const stamp=document.createElement("p");stamp.textContent=entry.time+" · "+entry.kind;
      const text=document.createElement("pre");text.className="agent-record";text.textContent=entry.text;target.append(stamp,text);
      if(entry.truncated){const clipped=document.createElement("p");clipped.textContent="来源正文仅显示部分内容或超限占位，原始记录未修改。";target.append(clipped);}
    }catch(error){if(current())target.textContent="来源读取失败，请重试；当前无法核对记忆依据。";throw error;}
  });
}
function renderMemory(value,scope,filter="all"){
  memoryFilter=filter;
  memoryScope={...scope};memoryItems=value.items;memoryNext=value.more?value.next:"";memoryMode=value.mode;memoryPreview=null;
  $("memory-preview-text").textContent="";$("memory-versions").replaceChildren();memoryRows(value.items);
  $("memory-state").textContent="机器人 "+scope.botId+" / "+(scope.type==="group"?"群 ":"私聊 ")+scope.id+"："+({managed:"独立记忆存储，本页 "+value.items.length+" 条。",legacy:"旧文件尚未分配归属。后台不显示旧正文；可预览迁移到此机器人。",pending:"迁移归属已保留，尚未完成。请为此机器人重新预览并重试。",claimed_elsewhere:"同号旧文件已归属其他机器人，此机器人不会读取它。"}[value.mode]||"状态未知");
  if(value.mode==="managed")$("memory-state").textContent+=" 已加载筛选："+({all:"全部",active:"仅启用",disabled:"仅停用"}[filter])+"。";
}
async function loadAgentMemory(after=""){
  if(!agentScope||!agentMemoryCanLeave())return;const scope=after?memoryScope:memoryTarget(),filter=after?memoryFilter:$("memory-filter").value;
  if(!scope)return;
  return agentRequest(async(signal,current)=>{
    clearMemorySearch();
    clearCurrentMemory(true);
    $("memory-source").replaceChildren();
    memoryPreview=null;memoryNext="";memoryMode="";memoryItems=[];$("memory-list").replaceChildren();$("memory-versions").replaceChildren();$("memory-preview-text").textContent="";
    const value=await api("/api/agent/memories?"+new URLSearchParams({...scope,after,state:filter}),undefined,signal);
    if(current())renderMemory(value,scope,filter);
  });
}
async function loadCurrentMemory(id=""){
  if(!memoryScope||!agentMemoryCanLeave()||agentBusy)return;
  id=id||$("memory-current-id").value.trim();
  if(!/^[1-9]\d{0,18}$/.test(id)||BigInt(id)>9223372036854775807n){showError("agent-error",new Error("请填写有效的正整数记忆编号。"));return;}
  const scope={...memoryScope};
  return agentRequest(async(signal,current)=>{
    clearCurrentMemory();$("memory-current-id").value=id;$("memory-current-state").textContent="正在读取当前版本…";
    $("memory-versions").replaceChildren();$("memory-source").replaceChildren();
    try{
      const value=await api("/api/agent/memories/current?"+new URLSearchParams({...scope,memoryId:id}),undefined,signal);if(!current())return;
      if(value.mode!=="managed"){$("memory-current-state").textContent="此范围尚无可定位的独立记忆，请核对旧文件归属或迁移状态。";return;}
      memoryCurrent=value.items.find(item=>item.id===id)||null;
      if(!memoryCurrent){$("memory-current-state").textContent="当前机器人和会话内未找到该记忆，不能据此判断其他范围或旧文件的内容。";return;}
      $("memory-current-state").textContent="已定位机器人 "+scope.botId+" / "+(scope.type==="group"?"群 ":"私聊 ")+scope.id+" 的当前记忆 #"+id+"；列表和搜索结果保持原读取状态。";
      memoryRows([memoryCurrent],false,"memory-current-detail");
      $("memory-current-state").scrollIntoView({block:"nearest"});
    }catch(error){if(current()){$("memory-current-state").textContent="当前记忆读取失败，请重新定位；旧结果已清除。";}throw error;}
  });
}
async function loadMemoryVersions(id,after="",revision=""){
  if(!memoryScope||!agentMemoryCanLeave())return;const scope={...memoryScope};
  if(revision&&(!/^[1-9]\d{0,18}$/.test(revision)||BigInt(revision)>9223372036854775807n)){showError("agent-error",new Error("历史修订号须为有效正整数字符串。"));return;}
  return agentRequest(async(signal,current)=>{
    $("memory-source").replaceChildren();
    $("memory-versions").textContent="正在读取记忆版本…";
    try{
      const value=await api((revision?"/api/agent/memories/revision?":"/api/agent/memories?")+new URLSearchParams(revision?{...scope,memoryId:id,revision}:{...scope,memoryId:id,after,order:"desc"}),undefined,signal);
      if(!current())return;
      if(revision&&(value.items.length>1||value.items.some(item=>item.id!==id||item.revision!==revision)))throw new Error("历史修订回执与请求不符，请重新读取。");
      memoryRows(value.items,true);
      const heading=document.createElement("p");heading.textContent=revision?"记忆 #"+id+" · 定位历史修订 v"+revision+(value.mode!=="managed"?"：此范围尚无可定位的独立记忆版本，请核对迁移状态。":value.items.length?"：已读取脱敏历史正文，当前记忆未修改。":"：当前会话内未找到该修订，请核对编号。") : "记忆 #"+id+" 的版本记录（从新到旧）"+(after?"，仅查看 v"+after+" 之前的修订。":"，从最新修订开始。");$("memory-versions").prepend(heading);
      const form=document.createElement("form"),label=document.createElement("label"),input=document.createElement("input"),locate=document.createElement("button");form.className="plugin-scope";label.textContent="历史修订号";input.className="memory-revision-input";input.inputMode="numeric";input.maxLength=19;input.required=true;input.value=revision;input.placeholder="填写要核对的修订号";label.append(input);locate.type="submit";locate.className="secondary";locate.textContent="定位历史修订";form.append(label,locate);form.onsubmit=event=>{event.preventDefault();loadMemoryVersions(id,"",input.value.trim());};heading.after(form);
      if(value.mode==="managed")form.after(memoryHistoryComparison(id,scope,value.items[0]?.revision||""));
      if(revision){const latest=document.createElement("button");latest.type="button";latest.className="secondary";latest.textContent="返回最新版本";latest.onclick=()=>loadMemoryVersions(id);$("memory-versions").append(latest);}
      else if(value.more){const next=document.createElement("button");next.className="secondary";next.textContent="下一页版本";next.onclick=()=>loadMemoryVersions(id,value.next);$("memory-versions").append(next);}
    }catch(error){if(current())$("memory-versions").textContent="版本读取失败，请重新查看版本；旧分页已清除。";throw error;}
  });
}
$("memory-load").onclick=()=>loadAgentMemory();
$("memory-current-form").onsubmit=event=>{event.preventDefault();loadCurrentMemory();};
$("memory-create").onclick=openMemoryCreate;
$("memory-search-form").onsubmit=event=>{event.preventDefault();searchAgentMemory();};
$("memory-search-continue").onclick=()=>{if(!agentBusy&&memorySearchContinue)searchAgentMemory(memorySearchContinue);};
$("memory-action").onchange=memoryControls;
$("memory-next").onclick=()=>{if(memoryNext)loadAgentMemory(memoryNext);};
$("memory-cancel").onclick=()=>{if(!memoryWriting){$("memory-dialog").close();memorySelected=null;$("memory-text").value="";memoryControls();}};
$("memory-dialog").addEventListener("cancel",event=>{if(memoryWriting)event.preventDefault();});
$("memory-dialog").addEventListener("close",()=>{if(!memoryWriting){memorySelected=null;$("memory-text").value="";memoryControls();}});
$("memory-edit-form").onsubmit=async event=>{
  event.preventDefault();if(agentBusy||!memorySelected||!memoryScope)return;
  const selected={...memorySelected},scope={...memoryScope},action=selected.create?"create":selected.targetRevision?"revert":$("memory-action").value,text=["edit","create"].includes(action)?$("memory-text").value.trim():"";
  if(["edit","create"].includes(action)&&(!text||/[\r\n\x00]/.test(text)||new TextEncoder().encode(text).length>4000)){$("memory-dialog-error").textContent="请填写不超过 4,000 UTF-8 字节的单行记忆。";return;}
  clearMemorySearchContinue();
  clearCurrentMemory();
  const epoch=agentEpoch+1;memoryWriting=true;
  await agentRequest(async(signal,current)=>{
    try{
      const body=selected.create?{...scope,text}:{...scope,memoryId:selected.id,revision:selected.revision,action,text};
      if(selected.targetRevision)body.targetRevision=selected.targetRevision;
      const value=await api(selected.create?"/api/agent/memories/create":"/api/agent/memories/update",body,signal);
      if(!current())return;if(value.saved!==true)throw new Error("保存结果无法确认，请重新读取。");
      if(selected.create&&(typeof value.created!=="boolean"||typeof value.item?.id!=="string"||!/^[1-9]\d{0,18}$/.test(value.item.id)||BigInt(value.item.id)>9223372036854775807n))throw new Error("新增结果无法确认，请重新读取。");
      memoryWriting=false;$("memory-dialog").close();memorySelected=null;$("memory-text").value="";
      clearMemorySearch();
      $("memory-source").replaceChildren();
      memoryNext="";memoryItems=[];$("memory-list").replaceChildren();$("memory-versions").replaceChildren();$("memory-state").textContent=selected.create?(value.created?"已新增记忆 #":"已复用相同启用记忆 #")+value.item.id+"，点击读取记忆核对。":"已保存，点击读取记忆核对最新版本。";
    }catch(error){if(current())$("memory-dialog-error").textContent="保存未完成或结果未知。"+error.message+" 请关闭编辑窗口并重新读取后核对；不自动重试。";}
  });
  if(agentEpoch===epoch){memoryWriting=false;memoryControls();}
};
$("memory-preview").onclick=()=>{
  if(!memoryScope)return;const scope={...memoryScope};
  return agentRequest(async(signal,current)=>{
    memoryPreview=null;$("memory-preview-text").textContent="";
    const value=await api("/api/agent/memories/migration-preview",scope,signal);if(!current())return;
    memoryPreview={...value,scope};$("memory-preview-text").textContent="将旧文件的 "+value.count+" 条记忆（"+value.bytes+" 字节）归属机器人 "+scope.botId+" / "+scope.type+" "+scope.id+"。原文件保留并备份；之后此会话只写新库。回退旧程序不会自动读取迁移后的新增记忆。预览 5 分钟有效。";
  });
};
$("memory-cancel-preview").onclick=()=>{memoryPreview=null;$("memory-preview-text").textContent="";memoryControls();};
$("memory-migrate").onclick=async()=>{
  if(!memoryPreview||agentBusy)return;const preview=memoryPreview;
  if(Date.now()>=Date.parse(preview.expiresAt)){memoryPreview=null;$("memory-preview-text").textContent="预览已过期，请重新预览。";memoryControls();return;}
  const epoch=agentEpoch+1;memoryWriting=true;
  await agentRequest(async(signal,current)=>{
    memoryPreview=null;
    try {
      const value=await api("/api/agent/memories/migrate",{...preview.scope,ticket:preview.ticket},signal);if(!current())return;
      if(value.migrated!==true)throw new Error("迁移结果无法确认，请重新读取。");
      memoryMode="managed";$("memory-preview-text").textContent="迁移完成，原文件和备份保留。点击读取记忆核对。";
    } catch(error) {if(current()){$("memory-preview-text").textContent="迁移未完成或结果未知，请重新读取记忆后核对，不会自动重试。";showError("agent-error",error);}}
  });
  if(agentEpoch===epoch){memoryWriting=false;memoryControls();}
};
window.addEventListener("beforeunload",event=>{if(memoryWriting||$("memory-dialog").open){event.preventDefault();event.returnValue="";}});
memoryControls();
