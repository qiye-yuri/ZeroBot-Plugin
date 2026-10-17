"use strict";
function resetAISearch() {
  aiBaselines.delete("search");
  $("ai-search-form").replaceChildren(); $("ai-search-form").hidden=true;
  $("ai-search-bot").value=""; $("ai-search-id").value=""; $("ai-search-type").value="group";
}
function renderAISearch(value) {
  const form=$("ai-search-form");form.replaceChildren();form.hidden=false;
  const fields=document.createElement("fieldset"),heading=document.createElement("h2"),note=document.createElement("p");
  heading.textContent="搜索设置 · 机器人 "+value.botId+" / "+(value.type==="private"?"私聊 QQ ":"群 ")+value.id;
  note.className="field-help";note.textContent="全局开关与密钥供本实例共享；会话开关和角色仅作用于标题中的机器人会话。保存会停止旧任务继续执行，不会调用搜索。";fields.append(heading,note);
  const enabled=aiInput(fields,"search-enabled","全局启用搜索","checkbox",value.enabled);
  const label=document.createElement("label"),action=document.createElement("select"),key=document.createElement("input"),clear=document.createElement("label"),confirm=document.createElement("input"),status=document.createElement("p");
  action.id="ai-search-key-action";label.htmlFor=action.id;label.textContent="DeepSeek 官方搜索密钥";
  for(const [value,text] of [["keep","保留当前密钥"],["replace","替换为新密钥"],["clear","清除密钥"]]){const option=document.createElement("option");option.value=value;option.textContent=text;action.append(option);}
  key.type="password";key.id="ai-search-key";key.maxLength=4096;key.autocomplete="new-password";key.disabled=true;key.setAttribute("aria-label","搜索新密钥");
  confirm.type="checkbox";confirm.id="ai-search-key-clear";clear.className="check-label";clear.hidden=true;clear.append(confirm,document.createTextNode("确认清除搜索密钥（需关闭全局搜索）"));
  status.className="field-help";status.textContent=value.keyConfigured?"搜索密钥已配置，当前值不显示。":"搜索密钥尚未配置，不自动借用模型密钥。";
  action.addEventListener("change",()=>{key.disabled=action.value!=="replace";key.required=action.value==="replace";if(action.value!=="replace")key.value="";clear.hidden=action.value!=="clear";confirm.checked=false;});
  fields.append(label,action,status,key,clear);
  const scopeEnabled=aiInput(fields,"search-scope-enabled","允许本会话联网搜索","checkbox",value.policy.enabled),roles={};
  for(const [role,title] of [["owner","机器人主人"],["admin","群主 / 群管理员"],["user","普通成员 / 私聊用户"]])roles[role]=aiInput(fields,"search-role-"+role,"允许角色："+title,"checkbox",value.policy.roles.includes(role));
  const save=document.createElement("button"),discard=document.createElement("button");save.type="submit";save.className="primary";save.textContent="备份并保存搜索设置";discard.type="button";discard.className="secondary";discard.dataset.aiDiscard="search";discard.textContent="放弃此表单更改";discard.onclick=()=>discardAIForm("search");fields.append(save,discard);form.append(fields);
  rememberAIForm("search",form,{botId:value.botId,type:value.type,id:value.id});aiBaselines.get("search").search=structuredClone(value);
  form.onsubmit=async event=>{
    event.preventDefault();if(!authenticated||aiBusy)return;
    const body={botId:value.botId,type:value.type,id:value.id,revision:value.revision,enabled:enabled.checked,key:{action:action.value},policy:{enabled:scopeEnabled.checked,roles:Object.entries(roles).filter(([,input])=>input.checked).map(([role])=>role)}};
    if(action.value==="replace")body.key.value=key.value;
    if(action.value==="clear"){if(!confirm.checked){showError("ai-error",new Error("请确认清除搜索密钥。"));return;}body.key.confirm=true;}
    const generation=aiGeneration;aiBusyState(true);$("ai-error").hidden=true;
    try{const saved=await api("/api/ai-search",body);if(generation!==aiGeneration||!authenticated)return;renderAISearch(saved);$("ai-status").textContent="搜索设置已保存，后续任务使用新设置；其他 AI 表单保存时需核对共享修订。";}
    catch(error){if(generation===aiGeneration&&authenticated)showError("ai-error",error);}
    finally{if(generation===aiGeneration)aiBusyState(false);}
  };
}
async function loadAISearch(scope) {
  if(!authenticated||aiBusy)return;
  if(aiDrafts().includes("search")&&!window.confirm("重新读取将放弃搜索表单未保存的更改，是否继续？"))return;
  if(!/^[1-9][0-9]{0,18}$/.test(scope.botId)||!/^[1-9][0-9]{0,18}$/.test(scope.id)){showError("ai-error",new Error("机器人及会话 QQ 须为正整数。"));return;}
  const generation=aiGeneration;aiBusyState(true);$("ai-error").hidden=true;aiBaselines.delete("search");$("ai-search-form").replaceChildren();$("ai-search-form").hidden=true;
  try{const value=await api("/api/ai-search?"+new URLSearchParams(scope));if(generation!==aiGeneration||!authenticated)return;renderAISearch(value);}
  catch(error){if(generation===aiGeneration&&authenticated)showError("ai-error",error);}
  finally{if(generation===aiGeneration)aiBusyState(false);}
}
$("ai-search-scope").addEventListener("submit",event=>{event.preventDefault();loadAISearch({botId:$("ai-search-bot").value.trim(),type:$("ai-search-type").value,id:$("ai-search-id").value.trim()});});
