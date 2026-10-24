"use strict";
let aiState = null, aiBusy = false, aiGroup = "0", aiPrivate = "", aiGeneration = 0;
let aiModelShortcutActive = false;
const aiFields = [
  ["Type","聊天接口类型","type"], ["ModelName","聊天模型名","text"], ["API","聊天接口地址","text"],
  ["MaxN","最大输出长度（0 为默认 4096）","number",0,131072], ["TopP","TopP（0 为默认 0.9）","number",0,1],
  ["SystemP","系统提示词","textarea"], ["NoSystemP","不发送系统提示词","checkbox"],
  ["ReasoningEffort","推理努力度（DeepSeek 官方接口：none 关闭思考，low/high/max 开启；留空用模型默认）","text"], ["Separator","回复分隔符（可留空）","text"],
  ["ImageType","识图接口类型","type"], ["ImageModelName","识图模型名（可留空）","text"], ["ImageAPI","识图接口地址（可留空）","text"],
  ["AgentType","Agent 接口类型","type"], ["AgentModelName","Agent 模型名（可留空）","text"], ["AgentAPI","Agent 接口地址（可留空）","text"],
  ["AgentChar","Agent 人物设定（修改后保留对话记录）","textarea"], ["AgentSex","Agent 性别","text"]
];
const aiKeys = [["Key","聊天密钥"],["ImageKey","识图密钥"],["AgentKey","Agent 密钥"]];
const aiBaselines = new Map();
const aiDraftNames={search:"联网搜索设置",models:"命名模型配置",persona:"会话人物覆盖",global:"全局模型与人物设定",aichatcfg:"聊天会话参数",llm:"GPT / 群总结温度"};
function aiFormSnapshot(form) {
  return JSON.stringify([...form.querySelectorAll("input,select,textarea")].map(input=>[input.id,input.type==="checkbox" ? input.checked : input.value]));
}
function aiDrafts() {
  return [...aiBaselines.entries()].filter(([,entry])=>entry.form.isConnected && aiFormSnapshot(entry.form)!==entry.value).map(([name])=>name);
}
function renderAIDrafts() {
  const drafts=aiDrafts();
  renderAIDraftFields(drafts);
  $("ai-draft-status").textContent=drafts.length ? "未保存更改："+drafts.map(name=>aiDraftNames[name]).join("、")+"。请分别保存；切换后台页面会保留草稿。" : aiState ? "当前 AI 表单没有未保存的更改。" : "读取设置后可编辑；草稿仅保留在当前页面。";
  for (const button of document.querySelectorAll("[data-ai-discard]")) button.disabled = !authenticated || aiBusy || !drafts.includes(button.dataset.aiDiscard);
}
function discardAIForm(name) {
  if (!authenticated || aiBusy || !aiDrafts().includes(name)) return;
  const entry = aiBaselines.get(name), generation = aiGeneration;
  const title = entry.form.querySelector("h2")?.textContent || "此表单";
  if (!window.confirm("放弃“" + title + "”的未保存更改？其他表单的编辑和已保存配置保持不变。")) return;
  if (!authenticated || aiBusy || generation !== aiGeneration || aiBaselines.get(name) !== entry || !entry.form.isConnected) return;
  if (name === "search") { renderAISearch(entry.search); $("ai-status").textContent="已放弃搜索设置的未保存更改。"; return; }
  const values = new Map(JSON.parse(entry.value));
  if (name === "models") { renderAIModels(entry.models); $("ai-status").textContent="已放弃命名模型表单的未保存更改。"; return; }
  for (const input of entry.form.querySelectorAll("input,select,textarea")) {
    if (!values.has(input.id)) continue;
    if (input.type === "checkbox") input.checked = values.get(input.id); else input.value = values.get(input.id);
  }
  if (name === "global") {
    // Reapply the key action UI: empty replacement input, keep mode, no clear confirmation.
    for (const [key] of aiKeys) $("ai-action-" + key).dispatchEvent(new Event("change", {bubbles:true}));
  }
  $("ai-status").textContent = "已放弃“" + title + "”的未保存更改；已保存配置保持不变。";
  renderAIDrafts();
}
function aiChangedFieldNames(name,entry) {
  if (name==="search") return ["搜索开关、密钥与会话角色"];
  if (name==="models") return ["模型列表与入口选择"];
  const baseline=new Map(JSON.parse(entry.value)),current=new Map(JSON.parse(aiFormSnapshot(entry.form)));
  const changed=new Set([...new Set([...baseline.keys(),...current.keys()])].filter(id=>baseline.get(id)!==current.get(id)));
  const labels=[];
  const take=(ids,label)=>{if(ids.some(id=>changed.has(id)))labels.push(label);for(const id of ids)changed.delete(id);};
  if (name==="global") {
    for (const [field,label] of aiFields) take(["ai-field-"+field],label);
    for (const [key,label] of aiKeys) take(["ai-action-"+key,"ai-key-"+key,"ai-clear-"+key],label);
  } else if (name==="persona") {
    take(["ai-field-persona-mode"],"人物继承模式"); take(["ai-field-persona-character"],"人物设定"); take(["ai-field-persona-sex"],"性别");
  } else if (name==="aichatcfg" || name==="llm") {
    for (const [field,label] of [["temperature","温度百分比"],...(name==="aichatcfg" ? [["rate","非 @ 消息触发概率"],["agent","Agent 模式"],["voice","QQ AI 语音输出"],["at","响应 @ 消息"],["proactive","群主动接话"],["proactiveMinutes","主动接话间隔"],["followupMinutes","相关话题续聊窗口"]] : [])]) take(["ai-field-"+field+"-"+name],label);
  }
  // Only fixed labels reach the DOM: never display input IDs or old/new values.
  if (changed.size) labels.push("其他表单字段");
  return labels;
}
function aiDraftFieldScope(name,entry) {
  if (name==="global" || name==="models") return "全局";
  if (name==="persona" || name==="search") return "机器人 "+entry.scope.botId+" · "+(entry.scope.type==="private" ? "私聊 QQ " : "群 ")+entry.scope.id;
  const scope=entry.scope,valid=id=>typeof id==="string" && /^[1-9][0-9]{0,18}$/.test(id);
  if (valid(scope?.privateId) && (!scope.groupId || scope.groupId==="0")) return "私聊 QQ "+scope.privateId;
  if (!scope?.privateId && valid(scope?.groupId)) return "群 "+scope.groupId;
  return "范围暂不可核对";
}
function renderAIDraftFields(drafts) {
  const panel=$("ai-draft-fields"),list=$("ai-draft-field-list");list.replaceChildren();
  panel.hidden=!authenticated || !aiState?.available || !drafts.length;
  $("ai-draft-field-state").textContent="";
  if (panel.hidden) return;
  $("ai-draft-field-state").textContent=aiBusy ? "请求处理中，保留当前字段摘要，请等待结果；不会自动保存。" : "与该表单最近读取或保存成功的页面基线比较；只列字段名称，不表示已生效或服务端最新状态。";
  for (const name of drafts) {
    const entry=aiBaselines.get(name),labels=aiChangedFieldNames(name,entry);
    const item=document.createElement("article"),heading=document.createElement("h3"),count=document.createElement("p"),fields=document.createElement("ul");
    const scope=aiDraftFieldScope(name,entry),title=name==="llm" && scope.startsWith("私聊 QQ ") ? "GPT 温度" : ({search:"联网搜索设置",models:"命名模型配置",persona:"会话人物覆盖",global:"全局模型与人物设定",aichatcfg:"聊天会话参数",llm:"GPT / 群总结温度"}[name] || "其他表单");
    item.dataset.aiDraftForm=["global","aichatcfg","llm","persona","models","search"].includes(name) ? name : "other";heading.textContent=title+" · "+scope;
    count.className="field-help";count.textContent="未保存字段："+labels.length+" 项（密钥操作、输入与确认合并计数）。";
    for (const label of labels) {const field=document.createElement("li");field.textContent=label;fields.append(field);}
    item.append(heading,count,fields);list.append(item);
  }
}
function rememberAIForm(name,form,scope=null) {
  aiBaselines.set(name,{form,value:aiFormSnapshot(form),scope}); renderAIDrafts();
}
function confirmAIDiscard(action) {
  if (aiBusy) { window.alert("AI 设置请求尚未结束，请等待结果后再"+action+"。"); return false; }
  const drafts=aiDrafts();
  return !drafts.length || window.confirm("AI 设置有未保存的更改："+drafts.map(name=>aiDraftNames[name]).join("、")+"。"+action+"将放弃这些更改，是否继续？");
}

function resetAI() {
  aiModelShortcutActive=false;
  if(typeof resetAIProbe==="function")resetAIProbe();
  if(typeof resetAISearch==="function")resetAISearch();
  aiGeneration++; aiState = null; aiGroup = "0"; aiPrivate = "";
  aiBaselines.clear(); aiBusy=false; $("ai-models-form").replaceChildren(); $("ai-models-form").hidden=true; $("ai-persona-form").replaceChildren(); $("ai-persona-form").hidden=true; $("ai-persona-bot").value=""; $("ai-persona-id").value="";
  renderAIConfigChecks(null);
  $("ai-global-inputs").replaceChildren(); $("ai-group-forms").replaceChildren(); $("ai-group").value = "";
  $("ai-scope-kind").value = "group"; renderAIScopeLabel();
  aiBusyState(false); renderAIDrafts();
}
function aiBusyState(busy) {
  aiBusy = busy; $("ai-models-load").disabled=busy; $("reload-ai").disabled = busy; $("apply-ai-group").disabled = busy;
  $("ai-global-fields").disabled = busy || !aiState?.available;
  for (const fields of document.querySelectorAll("#ai-search-form fieldset,#ai-search-scope input,#ai-search-scope select,#ai-search-scope button,#ai-models-form fieldset,#ai-group-forms fieldset,#ai-persona-form fieldset,#ai-persona-scope input,#ai-persona-scope select,#ai-persona-scope button")) fields.disabled = busy;
  renderAIDrafts();
  if(typeof renderAIProbeControls==="function")renderAIProbeControls();
}
function aiInput(container, name, labelText, kind, value, min, max) {
  const label = document.createElement("label"), input = document.createElement(kind === "textarea" ? "textarea" : kind === "type" ? "select" : "input");
  input.id = "ai-field-" + name; label.htmlFor = input.id; label.textContent = labelText;
  if (kind === "type") {
    for (const [index, text] of ["OpenAI", "OLLaMA", "GenAI"].entries()) { const option = document.createElement("option"); option.value = String(index); option.textContent = text; input.append(option); }
    if (![0,1,2].includes(value)) { const option = document.createElement("option"); option.value = String(value); option.textContent = "原配置类型无效，请选择"; input.append(option); }
    input.value = String(value);
  } else if (kind === "checkbox") { input.type = "checkbox"; input.checked = value; label.className = "check-label"; label.prepend(input); container.append(label); return input; }
  else {
    if (kind !== "textarea") input.type = kind;
    input.value = value ?? ""; input.autocomplete = "off";
    if (kind === "number") { input.min = min; input.max = max; input.step = name === "TopP" ? "0.01" : "1"; input.required = true; }
    else { input.maxLength = kind === "textarea" ? 32768 : name.endsWith("API") || name === "API" ? 2048 : 256; if (kind === "textarea") input.rows = 6; }
  }
  container.append(label, input); return input;
}
function renderAIGlobal(value) {
  renderAIConfigChecks(value);
  if(typeof renderAIProbeControls==="function")renderAIProbeControls();
  if (!value.available) { $("ai-status").textContent = "当前程序没有编译 AI 配置插件。"; $("ai-global-inputs").replaceChildren(); aiBaselines.delete("global"); renderAIDrafts(); return; }
  const container = $("ai-global-inputs"); container.replaceChildren();
  for (const [name,label,kind,min,max] of aiFields) {
    aiInput(container,name,label,kind,value.global.fields[name],min,max);
    if (value.global.addressRedacted[name]) { const note = document.createElement("p"); note.className="field-help"; note.textContent="旧地址中的认证参数已隐藏；不修改时保留原地址，替换时请填写独立密钥。"; container.append(note); }
  }
  for (const [name,text] of aiKeys) {
    const label=document.createElement("label"), select=document.createElement("select"), key=document.createElement("input"), note=document.createElement("p"), confirm=document.createElement("label"), checkbox=document.createElement("input");
    select.id="ai-action-"+name; label.htmlFor=select.id; label.textContent=text;
    for (const [action,title] of [["keep","保留当前密钥"],["replace","替换为新密钥"],["clear","清除密钥"]]) { const option=document.createElement("option"); option.value=action; option.textContent=title; select.append(option); }
    key.id="ai-key-"+name; key.type="password"; key.autocomplete="off"; key.maxLength=4096; key.placeholder="输入新密钥"; key.setAttribute("aria-label",text+"新值"); key.disabled=true;
    note.className="field-help"; note.textContent=value.global.keys[name] ? "已配置；当前值不会显示。" : "未配置。";
    confirm.className="check-label"; confirm.hidden=true; checkbox.type="checkbox"; checkbox.id="ai-clear-"+name; confirm.append(checkbox,document.createTextNode("我确认清除"+text));
    select.addEventListener("change",()=>{key.disabled=select.value!=="replace"; key.required=select.value==="replace"; if (select.value!=="replace") key.value=""; confirm.hidden=select.value!=="clear"; checkbox.checked=false;});
    container.append(label,select,note,key,confirm);
  }
  $("ai-status").textContent=value.global.ready ? "配置已读取。保存后的设置供后续请求使用。" : "聊天接口尚未配置完整；OLLaMA 可使用无密钥接口。";
  lockLegacyModelFields(value.global.namedModels);
  rememberAIForm("global",$("ai-global-form"));
}
function renderAIConfigChecks(value) {
  const panel = $("ai-config-checks"), list = $("ai-config-check-list");
  list.replaceChildren(); panel.hidden = !authenticated || !value?.available;
  if (panel.hidden) return;
  for (const [name,title] of [["chat","聊天"],["gpt","GPT"],["summary","群总结"],["agent","Agent"],["image","识图（可选）"]]) {
    const item = document.createElement("article"), heading = document.createElement("h3"), status = document.createElement("p"), detail = document.createElement("p");
    item.dataset.aiCheck = name; heading.textContent = title; status.className = "ai-check-result"; detail.className = "field-help";
    const check = value.global?.checks?.[name];
    const fields = ["typeValid","modelConfigured","modelRequired","addressConfigured","addressValid","keyConfigured","keyRequired","complete"];
    const valid = check && fields.every(key => typeof check[key] === "boolean") && check.modelRequired === (name !== "image") &&
      (!check.addressValid || check.addressConfigured) && check.complete === (check.typeValid && check.addressConfigured && check.addressValid && (check.modelConfigured || !check.modelRequired) && (check.keyConfigured || !check.keyRequired));
    if (!valid) {
      status.textContent = "暂不可核对基础状态"; detail.textContent = "接口未提供可核对的检查结果，请重新加载或更新程序。";
    } else {
      const optionalEmpty = name === "image" && check.typeValid && !check.modelConfigured && !check.addressConfigured && !check.keyConfigured;
      status.textContent = optionalEmpty ? "未配置（可选，不阻止文字聊天）" : check.complete ? "基础字段检查通过；尚未验证实际调用" : "基础字段待补充或检查";
      detail.textContent = "接口类型：" + (check.typeValid ? "支持" : "需检查") + "；模型：" + (check.modelConfigured ? "已填写" : check.modelRequired ? "未填写（需要）" : "未填写（可选，按接口要求核对）") +
        "；地址：" + (!check.addressConfigured ? "未填写" : check.addressValid ? "格式检查通过" : "需检查格式或隐藏认证参数") + "；密钥：" + (check.keyConfigured ? "已配置（未验证）" : check.keyRequired ? "未配置（需要）" : "未配置（OLLaMA 可无密钥）") + "。";
    }
    item.append(heading, status, detail); list.append(item);
  }
}
function renderAIGroups(groups, onlyService="") {
  const container=$("ai-group-forms"); if (!onlyService) {container.replaceChildren(); aiBaselines.delete("aichatcfg"); aiBaselines.delete("llm");}
  for (const value of groups) {
    if (onlyService && value.service !== onlyService) continue;
    const form=document.createElement("form"), fields=document.createElement("fieldset"), heading=document.createElement("h2"), save=document.createElement("button"); form.className="panel ai-form";
    form.dataset.service=value.service;
    heading.textContent=(value.service==="llm" ? (value.privateId ? "GPT 温度" : "GPT / 群总结温度") : "聊天会话参数")+" · "+(value.privateId ? "私聊 QQ "+value.privateId : "群 "+value.groupId); fields.append(heading);
    const suffix=value.service;
    const temp=aiInput(fields,"temperature-"+suffix,"温度百分比（0 为默认 70）","number",value.temperature,0,100);
    let rate,agent,voice,at,proactive,proactiveMinutes,followupMinutes;
    if (value.service==="aichatcfg") {
      rate=aiInput(fields,"rate-"+suffix,"非 @ 消息触发概率（百分比）","number",value.rate,0,100);
      agent=aiInput(fields,"agent-"+suffix,"使用 Agent 模式（可调用机器人操作）","checkbox",value.useAgent);
      voice=aiInput(fields,"voice-"+suffix,"尝试使用 QQ AI 语音输出","checkbox",value.voiceOutput);
      at=aiInput(fields,"at-"+suffix,"响应 @ 消息","checkbox",value.respondAt);
      if (!value.privateId) {
        proactive=aiInput(fields,"proactive-"+suffix,"主动接话：有新消息时按间隔接话，相关话题可续聊","checkbox",value.proactive);
        proactiveMinutes=aiInput(fields,"proactiveMinutes-"+suffix,"主动接话最小间隔（分钟，默认10）","number",value.proactiveMinutes ?? 10,1,1440);
        followupMinutes=aiInput(fields,"followupMinutes-"+suffix,"相关话题续聊窗口（分钟，默认2）","number",value.followupMinutes ?? 2,1,30);
        const hint=document.createElement("p");hint.textContent="开启后替代非@随机概率：提及机器人立即响应；忽略机器人自己的消息；无新消息不发言。相关性判断会调用聊天模型。";fields.append(hint);
      }
    }
    save.type="submit"; save.className="primary"; save.textContent="备份并保存会话参数";
    const discard = document.createElement("button"); discard.type="button"; discard.className="secondary ai-discard"; discard.textContent="放弃此表单更改";
    discard.id="discard-ai-"+value.service; discard.dataset.aiDiscard=value.service; discard.addEventListener("click",()=>discardAIForm(value.service));
    fields.append(save,discard); form.append(fields);
    const existing=onlyService ? [...container.children].find(item=>item.dataset.service===value.service) : null;
    if (existing) existing.replaceWith(form); else container.append(form);
    rememberAIForm(value.service,form,{groupId:value.groupId,privateId:value.privateId});
    form.addEventListener("submit",async(event)=>{
      event.preventDefault(); if (aiBusy || !authenticated) return;
      const generation=aiGeneration, body={service:value.service,revision:value.revision,temperature:Number(temp.value)};
      if (value.privateId) body.privateId=value.privateId; else body.groupId=value.groupId;
      if (rate) Object.assign(body,{rate:Number(rate.value),useAgent:agent.checked,voiceOutput:voice.checked,respondAt:at.checked});
      if (proactive) Object.assign(body,{proactive:proactive.checked,proactiveMinutes:Number(proactiveMinutes.value),followupMinutes:Number(followupMinutes.value)});
      aiBusyState(true); $("ai-error").hidden=true;
      try { const saved=await api("/api/ai-group",body); if (generation!==aiGeneration || !authenticated) return; aiState.groups=saved.groups; renderAIGroups(saved.groups,value.service); $("ai-status").textContent="会话参数已保存；后续消息使用新参数。"; }
      catch(error) { if (generation===aiGeneration && authenticated) showError("ai-error",error); }
      finally { if (generation===aiGeneration) {aiBusyState(false); renderAIDrafts();} }
    });
  }
}
async function loadAI(group=aiGroup,privateId=aiPrivate,{confirmed=false}={}) {
  if (aiBusy || !authenticated) return;
  if (!confirmed && !confirmAIDiscard(group===aiGroup && privateId===aiPrivate ? "重新加载设置" : "切换会话参数")) return;
  const generation=++aiGeneration; aiBusyState(true); $("ai-error").hidden=true;
  try { const value=await api("/api/ai?"+(privateId ? "private="+encodeURIComponent(privateId) : "group="+encodeURIComponent(group))); if (generation!==aiGeneration || !authenticated) return; aiGroup=group; aiPrivate=privateId; aiState=value; if(typeof resetAISearch==="function")resetAISearch(); aiBaselines.delete("models"); $("ai-models-form").replaceChildren(); $("ai-models-form").hidden=true; aiBaselines.delete("persona"); $("ai-persona-form").replaceChildren(); $("ai-persona-form").hidden=true; $("ai-scope-kind").value=privateId ? "private" : "group"; if (privateId || group!=="0") $("ai-group").value=privateId || group; renderAIScopeLabel(); renderAIGlobal(value); renderAIGroups(value.groups); }
  catch(error) { if (generation===aiGeneration && authenticated) { $("ai-status").textContent="读取失败，请重新加载。"; showError("ai-error",error); } }
  finally { if (generation===aiGeneration) {aiBusyState(false); renderAIDrafts();} }
}
$("reload-ai").addEventListener("click",()=>loadAI());
$("discard-ai-global").addEventListener("click",()=>discardAIForm("global"));
function renderAIScopeLabel() {
  const privateScope=$("ai-scope-kind").value==="private";
  $("ai-conversation-label").textContent=privateScope ? "私聊用户 QQ" : "群号";
  $("ai-group").placeholder=privateScope ? "填写与机器人聊天的用户 QQ" : "填写要管理的群号";
}
$("ai-scope-kind").addEventListener("change",renderAIScopeLabel);
$("ai-scope-form").addEventListener("submit",(event)=>{ event.preventDefault(); const id=$("ai-group").value.trim(); if (!/^[1-9][0-9]{0,18}$/.test(id)) {showError("ai-error",new Error("群号或私聊 QQ 须为正整数。")); return;} loadAI($("ai-scope-kind").value==="private" ? "0" : id,$("ai-scope-kind").value==="private" ? id : ""); });
$("ai-global-form").addEventListener("submit",async(event)=>{
  event.preventDefault(); if (aiBusy || !aiState?.available || !authenticated) return;
  const generation=aiGeneration, body={revision:aiState.global.revision,fields:{},keys:{}};
  for (const [name,,kind] of aiFields) {
    const input=$("ai-field-"+name), value=kind==="checkbox" ? input.checked : kind==="number" || kind==="type" ? Number(input.value) : input.value;
    if (value !== aiState.global.fields[name]) body.fields[name]=value;
  }
  for (const [name] of aiKeys) {
    const action=$("ai-action-"+name).value; body.keys[name]={action};
    if (action==="replace") body.keys[name].value=$("ai-key-"+name).value;
    if (action==="clear") { if (!$("ai-clear-"+name).checked) {showError("ai-error",new Error("请先确认清除密钥。")); return;} body.keys[name].confirm=true; }
  }
  aiBusyState(true); $("ai-error").hidden=true;
  try { const saved=await api("/api/ai",body); if (generation!==aiGeneration || !authenticated) return; aiState.global=saved.global; renderAIGlobal(aiState); $("ai-status").textContent="全局配置已保存，后续请求使用新设置，无需重启。"; }
  catch(error) { if (generation===aiGeneration && authenticated) showError("ai-error",error); }
  finally { if (generation===aiGeneration) {aiBusyState(false); renderAIDrafts();} }
});
for (const id of ["ai-global-form","ai-group-forms","ai-persona-form","ai-models-form","ai-search-form"]) {
  $(id).addEventListener("input",renderAIDrafts); $(id).addEventListener("change",renderAIDrafts);
}
window.addEventListener("beforeunload",event=>{
  if (authenticated && (aiBusy || aiDrafts().length)) {event.preventDefault();event.returnValue="";}
});

function renderAIPersona(value) {
  const form=$("ai-persona-form"); form.replaceChildren(); form.hidden=false;
  const fields=document.createElement("fieldset"), heading=document.createElement("h2"), note=document.createElement("p");
  heading.textContent="Agent 会话人物 · 机器人 "+value.botId+" · "+(value.type==="private" ? "私聊 QQ " : "群 ")+value.id;
  note.className="field-help"; note.textContent="当前来源："+(value.mode==="custom" ? "会话自定义" : "继承全局")+"。仅影响此会话的新 Agent 任务；人物更改不清理档案或记忆，普通聊天与 GPT 提示词仍独立。";
  fields.append(heading,note);
  const label=document.createElement("label"), mode=document.createElement("select"); mode.id="ai-field-persona-mode";label.htmlFor=mode.id;label.textContent="人物模式";
  for (const [id,text] of [["inherit","继承全局"],["custom","自定义"]]) {const option=document.createElement("option");option.value=id;option.textContent=text;mode.append(option);} mode.value=value.mode;fields.append(label,mode);
  const character=aiInput(fields,"persona-character","人物设定（继承时显示已读取的全局值）","textarea",value.character),sex=aiInput(fields,"persona-sex","性别","text",value.sex);
  const update=()=>{character.readOnly=sex.readOnly=mode.value==="inherit";character.required=mode.value==="custom";};mode.addEventListener("change",update);update();
  const save=document.createElement("button"),discard=document.createElement("button");save.type="submit";save.className="primary";save.textContent="备份并保存会话人物";discard.type="button";discard.className="secondary";discard.dataset.aiDiscard="persona";discard.textContent="放弃此表单更改";discard.addEventListener("click",()=>{discardAIForm("persona");update();});fields.append(save,discard);form.append(fields);
  rememberAIForm("persona",form,{botId:value.botId,type:value.type,id:value.id});
  form.onsubmit=async event=>{
    event.preventDefault();if(aiBusy||!authenticated)return;
    const generation=aiGeneration,body={botId:value.botId,type:value.type,id:value.id,revision:value.revision,mode:mode.value};
    if(mode.value==="custom"){body.character=character.value;body.sex=sex.value;}
    aiBusyState(true);$("ai-error").hidden=true;
    try {const saved=await api("/api/ai-persona",body);if(generation!==aiGeneration||!authenticated)return;renderAIPersona(saved);$("ai-status").textContent="会话人物已保存；下一个任务使用新设定。全局表单若提示版本变化，请重新读取。";}
    catch(error){if(generation===aiGeneration&&authenticated)showError("ai-error",error);}
    finally{if(generation===aiGeneration)aiBusyState(false);}
  };
}
async function loadAIPersona(scope,{confirmed=false,clearPrevious=false}={}) {
  if(aiBusy||!authenticated)return;
  if(!confirmed&&aiDrafts().includes("persona")&&!window.confirm("切换会话人物将放弃该表单未保存的更改，是否继续？"))return;
  if(!/^[1-9][0-9]{0,18}$/.test(scope.botId)||!/^[1-9][0-9]{0,18}$/.test(scope.id)){showError("ai-error",new Error("机器人及会话 QQ 须为正整数。"));return;}
  const generation=aiGeneration;aiBusyState(true);$("ai-error").hidden=true;
  if(clearPrevious){aiBaselines.delete("persona");$("ai-persona-form").replaceChildren();$("ai-persona-form").hidden=true;renderAIDrafts();}
  try{const value=await api("/api/ai-persona?"+new URLSearchParams(scope));if(generation!==aiGeneration||!authenticated)return;renderAIPersona(value);}
  catch(error){if(generation===aiGeneration&&authenticated)showError("ai-error",error);}
  finally{if(generation===aiGeneration)aiBusyState(false);}
}
$("ai-persona-scope").addEventListener("submit",event=>{
  event.preventDefault();loadAIPersona({botId:$("ai-persona-bot").value.trim(),type:$("ai-persona-type").value,id:$("ai-persona-id").value.trim()});
});

async function openConversationAI(scope,kind) {
  if(!authenticated||aiBusy)return;
  const needsGlobal=kind==="parameters"||!aiState;
  if(needsGlobal){if(!confirmAIDiscard("进入此会话设置"))return;}
  else if(aiDrafts().includes("persona")&&!window.confirm("切换会话人物将放弃该表单未保存的更改，是否继续？"))return;
  if(!showWorkspaceView("ai",{autoLoadAI:false}))return;
  if(needsGlobal){
    // Clear the previous target before reading, so a failed shortcut cannot
    // leave an editable form for another conversation beneath the new scope.
    resetAI();
    await loadAI(scope.type==="group"?scope.id:"0",scope.type==="private"?scope.id:"",{confirmed:true});
    if(!authenticated||!aiState||$("ai-view").hidden)return;
  }
  if(kind==="persona"){
    $("ai-persona-bot").value=scope.botId;$("ai-persona-type").value=scope.type;$("ai-persona-id").value=scope.id;
    await loadAIPersona({botId:scope.botId,type:scope.type,id:scope.id},{confirmed:true,clearPrevious:true});
    if(!authenticated||$("ai-view").hidden)return;
    $("ai-persona-scope").scrollIntoView({block:"start"});
  }else{
    $("ai-status").textContent="已读取机器人 "+scope.botId+" 的"+(scope.type==="group"?"群 ":"私聊 QQ ")+scope.id+"；会话参数由本实例各机器人共享，不是机器人专属设置。";
    $("ai-scope-form").scrollIntoView({block:"start"});
  }
}

function lockLegacyModelFields(named) {
  if (!named || $("ai-global-inputs").querySelector("[data-model-notice]")) return;
  for (const field of ["Type","ModelName","API","AgentType","AgentModelName","AgentAPI","ImageType","ImageModelName","ImageAPI"]) $("ai-field-"+field).disabled=true;
  for (const [key] of aiKeys) { $("ai-action-"+key).disabled=true; $("ai-key-"+key).disabled=true; $("ai-clear-"+key).disabled=true; }
  const note=document.createElement("p");note.dataset.modelNotice="";note.className="field-help";note.textContent="已启用命名模型。类型、模型、地址和密钥请在命名模型表单修改；本表单继续管理提示词、人物和采样参数。";$("ai-global-inputs").prepend(note);
}

function modelParameterInputs(row, profile, defaults) {
  const details=document.createElement("details"),summary=document.createElement("summary"),controls={};
  summary.textContent="请求参数";details.append(summary);
  for(const [name,title,kind,min,max] of [["maxN","最大输出长度","number",1,131072],["topP","TopP","number",0,1],["reasoningEffort","推理设置（自定义留空使用模型默认）","text"]]) {
    const label=document.createElement("label"),mode=document.createElement("select"),status=document.createElement("p");
    mode.id="ai-model-"+profile.id+"-"+name+"-mode";label.htmlFor=mode.id;label.textContent=title+"来源";
    for(const [value,text] of [["inherit","继承全局"],["custom","自定义"]]){const option=document.createElement("option");option.value=value;option.textContent=text;mode.append(option);}
    const saved=profile.parameters?.[name];mode.value=saved==null?"inherit":"custom";details.append(label,mode);
    const input=aiInput(details,"model-"+profile.id+"-"+name,title,kind,saved??defaults[name],min,max);
    if(name==="topP"){input.min="0";input.step="any";}
    status.className="field-help";details.append(status);
    const update=()=>{input.disabled=mode.value!=="custom";const effective=mode.value==="custom"?input.value:defaults[name];status.textContent="当前"+(mode.value==="custom"?"自定义":"继承全局")+"："+(effective===""?"模型默认":effective);input.setCustomValidity(name==="topP"&&mode.value==="custom"&&Number(input.value)<=0?"TopP 须大于 0 且不超过 1":"");};
    mode.addEventListener("change",update);input.addEventListener("input",update);update();controls[name]={mode,input};
  }
  row.append(details);return controls;
}

function renderAIModels(value) {
  const form=$("ai-models-form");form.replaceChildren();form.hidden=false;
  const fields=document.createElement("fieldset"),heading=document.createElement("h2"),note=document.createElement("p"),list=document.createElement("div"),rows=new Map(),routes={},routeHelp={};
  heading.textContent="命名模型与入口选择";note.className="field-help";note.textContent=value.named ? "命名配置已启用。一个配置可被多个入口引用；更改会影响所有引用入口。" : "当前为旧配置迁移预览，尚未写库；首次保存同时迁移配置与入口。";fields.append(heading,note,list);
  for(const [key,title] of [["chat","聊天"],["gpt","GPT"],["summary","群总结"],["agent","Agent"],["image","识图"]]) {
    const label=document.createElement("label"),select=document.createElement("select"),help=document.createElement("p");select.id="ai-model-route-"+key;label.htmlFor=select.id;label.textContent=title;routes[key]=select;help.className="field-help";routeHelp[key]=help;fields.append(label,select,help);select.addEventListener("change",updateRouteHelp);
  }
  function updateRouteHelp(){for(const [entry,select] of Object.entries(routes)){const inherited=(entry==="gpt"||entry==="summary")&&!select.value;const id=inherited?routes.chat.value:select.value;routeHelp[entry].textContent=id&&!rows.has(id)?"引用配置不存在，请重新选择。":id?(inherited?"继承聊天模型：":"当前选择：")+(rows.get(id).name.value||id)+"；使用该配置的请求参数。":"当前未选择配置。";}}
  function updateRoutes(){
    for(const [key,select] of Object.entries(routes)){
      const selected=select.value;select.replaceChildren();const empty=document.createElement("option");empty.value="";empty.textContent=key==="chat" ? "请选择模型配置" : key==="gpt"||key==="summary"?"继承聊天模型":"不启用";select.append(empty);
      for(const [id,row] of rows){const option=document.createElement("option");option.value=id;option.textContent=row.name.value||"未命名配置";select.append(option);}if(selected&&!rows.has(selected)){const option=document.createElement("option");option.value=selected;option.textContent="引用配置不存在，请重新选择";select.append(option);}select.value=selected;
    }
    updateRouteHelp();
  }
  function addRow(p){
    const row=document.createElement("article"),title=document.createElement("h3");row.className="panel ai-model-row";row.dataset.modelId=p.id;title.textContent="模型配置";row.append(title);
    const name=aiInput(row,"model-"+p.id+"-name","名称","text",p.name),type=aiInput(row,"model-"+p.id+"-type","接口类型","type",p.type),model=aiInput(row,"model-"+p.id+"-model","模型名","text",p.model),address=aiInput(row,"model-"+p.id+"-api","接口地址","text",p.api);
    name.required=model.required=address.required=true;address.maxLength=2048;
    const label=document.createElement("label"),action=document.createElement("select"),key=document.createElement("input"),clear=document.createElement("label"),confirm=document.createElement("input"),status=document.createElement("p");
    action.id="ai-model-"+p.id+"-action";label.htmlFor=action.id;label.textContent="密钥操作";
    for(const [id,text] of [["keep","保留当前密钥"],["replace","替换为新密钥"],["clear","清除密钥"]]){const option=document.createElement("option");option.value=id;option.textContent=text;action.append(option);}
    key.type="password";key.id="ai-model-"+p.id+"-key";key.maxLength=4096;key.autocomplete="off";key.setAttribute("aria-label","新密钥");key.disabled=true;
    confirm.type="checkbox";confirm.id="ai-model-"+p.id+"-clear";clear.className="check-label";clear.hidden=true;clear.append(confirm,document.createTextNode("确认清除密钥"));
    status.className="field-help";status.textContent=(p.keyConfigured?"已配置密钥，当前值不显示。":"尚未配置密钥，OLLaMA 可无密钥。")+(p.addressRedacted?"旧地址的认证参数已隐藏，迁移前请改为独立密钥。":"");
    action.addEventListener("change",()=>{key.disabled=action.value!=="replace";key.required=action.value==="replace";if(action.value!=="replace")key.value="";clear.hidden=action.value!=="clear";confirm.checked=false;});
    const remove=document.createElement("button");remove.type="button";remove.className="secondary";remove.textContent="删除配置";
    remove.addEventListener("click",()=>{if(Object.values(routes).some(select=>select.value===p.id)){showError("ai-error",new Error("请先更改引用此配置的入口，再删除配置。"));return;}rows.delete(p.id);row.remove();updateRoutes();renderAIDrafts();});
    row.append(label,action,status,key,clear);
    const parameters=modelParameterInputs(row,p,value.parameterDefaults);
    row.append(remove);list.append(row);rows.set(p.id,{name,type,model,address,action,key,confirm,parameters});name.addEventListener("input",updateRoutes);updateRoutes();
  }
  for(const p of value.profiles)addRow(p);
  for(const [key,select] of Object.entries(routes)){const id=value.routes[key]??"";if(id&&!rows.has(id)){const option=document.createElement("option");option.value=id;option.textContent="引用配置不存在，请重新选择";select.append(option);}select.value=id;}
  updateRouteHelp();
  const add=document.createElement("button"),save=document.createElement("button"),discard=document.createElement("button");add.type=discard.type="button";save.type="submit";add.className=discard.className="secondary";save.className="primary";add.textContent="新增模型配置";save.textContent="备份并保存命名模型";discard.textContent="放弃此表单更改";discard.dataset.aiDiscard="models";
  add.addEventListener("click",()=>{addRow({id:"m-"+Array.from(crypto.getRandomValues(new Uint8Array(12)),b=>b.toString(16).padStart(2,"0")).join(""),name:"",type:0,model:"",api:"",keyConfigured:false});renderAIDrafts();});discard.addEventListener("click",()=>discardAIForm("models"));fields.append(add,save,discard);form.append(fields);
  rememberAIForm("models",form);aiBaselines.get("models").models=structuredClone(value);
  form.onsubmit=async event=>{
    event.preventDefault();if(aiBusy||!authenticated)return;
    const body={revision:value.revision,profiles:[],routes:Object.fromEntries(Object.entries(routes).map(([k,s])=>[k,s.value]))};
    for(const [id,row] of rows){const key={action:row.action.value};if(key.action==="replace")key.value=row.key.value;if(key.action==="clear"){if(!row.confirm.checked){showError("ai-error",new Error("请确认清除密钥。"));return;}key.confirm=true;}const parameters={};for(const [name,{mode,input}] of Object.entries(row.parameters)){if(mode.value==="custom")parameters[name]=name==="reasoningEffort"?input.value:Number(input.value);}body.profiles.push({id,name:row.name.value,type:Number(row.type.value),model:row.model.value,api:row.address.value,key,parameters});}
    const generation=aiGeneration;aiBusyState(true);$("ai-error").hidden=true;
    try{const saved=await api("/api/ai-models",body);if(generation!==aiGeneration||!authenticated)return;renderAIModels(saved);if(saved.global&&aiState){if(!aiDrafts().includes("global")){aiState.global=saved.global;renderAIGlobal(aiState);}else{lockLegacyModelFields(true);}}$("ai-status").textContent="命名模型已保存。重新加载全局设置可核对各入口的生效字段；其他表单草稿保留，保存时需核对共享修订。";}
    catch(error){if(generation===aiGeneration&&authenticated)showError("ai-error",error);}
    finally{if(generation===aiGeneration)aiBusyState(false);}
  };
}
async function loadAIModels({confirmed=false}={}){
  if(aiBusy||!authenticated)return false;if(!confirmed&&aiDrafts().includes("models")&&!window.confirm("重新读取将放弃命名模型表单的未保存更改，是否继续？"))return false;
  const generation=aiGeneration;aiBusyState(true);$("ai-error").hidden=true;
  try{const value=await api("/api/ai-models");if(generation!==aiGeneration||!authenticated)return false;renderAIModels(value);return true;}
  catch(error){if(generation===aiGeneration&&authenticated)showError("ai-error",error);}
  finally{if(generation===aiGeneration)aiBusyState(false);}
  return false;
}
$("ai-models-load").addEventListener("click",()=>loadAIModels());

function showAIModelShortcut(visible){
  if(!visible&&aiModelShortcutActive)resetAI();
}
async function openGlobalModels(scope){
  if(!authenticated||aiBusy||!confirmAIDiscard("读取全局模型配置"))return;
  if(!showWorkspaceView("ai",{autoLoadAI:false}))return;
  resetAI();
  aiModelShortcutActive=true;
  const generation=aiGeneration+1; // loadAI advances the existing request generation.
  try{
    await loadAI("0","",{confirmed:true});
    if(generation!==aiGeneration||!authenticated||!aiState?.available||$("ai-view").hidden)return;
    let target=$("ai-global-form");
    if(aiState.global.namedModels){
      if(!await loadAIModels({confirmed:true}))return;
      target=$("ai-models-form");
    }
    if(generation!==aiGeneration||!authenticated||$("ai-view").hidden)return;
    $("ai-status").textContent="来自机器人 "+scope.botId+" / "+(scope.type==="group"?"群 ":"私聊 ")+scope.id+" 的配置核对。模型配置由本实例共享，修改会影响使用该配置的所有会话；此入口不创建会话模型覆盖。请显式保存后返回会话手动核对。";
    target.scrollIntoView({block:"start"});
  }finally{if(generation===aiGeneration)aiModelShortcutActive=false;}
}
