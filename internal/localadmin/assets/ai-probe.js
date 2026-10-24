"use strict";
let aiProbeBusy=false,aiProbeEpoch=0,aiProbeResult=null,aiSearchProbeResult=null;
const aiProbeNames={chat:"聊天",gpt:"GPT",summary:"群总结",agent:"Agent"};
function resetAIProbe(){
  aiProbeEpoch++;aiProbeBusy=false;aiProbeResult=null;
  aiSearchProbeResult=null;
  if($("ai-search-probe-result"))$("ai-search-probe-result").replaceChildren();
  if($("ai-search-probe-state"))$("ai-search-probe-state").textContent="仅测试已保存的全局搜索密钥，不代表已启用会话搜索。";
  if($("ai-search-probe-error"))$("ai-search-probe-error").hidden=true;
  $("ai-probe-result").replaceChildren();$("ai-probe-error").hidden=true;
  $("ai-probe-state").textContent="先读取已保存配置，再手动测试。";
  renderAIProbeControls();
}
function showAIProbe(visible){if(!visible)resetAIProbe();else renderAIProbeControls();}
function renderAIProbeControls(){
  if($("ai-search-probe"))$("ai-search-probe").disabled=!authenticated||aiBusy||aiProbeBusy||!aiBaselines.get("search")?.search?.keyConfigured;
  if(aiSearchProbeResult&&$("ai-search-probe-version"))$("ai-search-probe-version").textContent=aiSearchProbeResult.revision===aiBaselines.get("search")?.search?.revision?"基于当前已读取的保存版本；未保存草稿不参与测试。":"历史结果：已读取配置版本发生变化，请按需重新测试。";
  for(const entry of Object.keys(aiProbeNames))$("ai-probe-"+entry).disabled=!authenticated||!aiState?.available||aiBusy||aiProbeBusy;
  if(aiProbeResult){
    $("ai-probe-version-state").textContent=aiProbeResult.revision===aiState?.global?.revision?"基于当前已读取的保存版本；未保存草稿不参与测试。":"历史结果：已读取配置版本发生变化，请按需重新测试。";
  }
}
function renderAIProbeResult(value){
  aiProbeResult=value;
  const heading=document.createElement("h3"),detail=document.createElement("p"),version=document.createElement("p"),status=document.createElement("p"),scope=document.createElement("p");
  heading.textContent=aiProbeNames[value.entry]+"模型 · "+(value.success?"基础调用通过":"本次测试失败");
  detail.textContent="模型："+value.model+"；测试时间："+value.checkedAt+"；耗时："+value.milliseconds+" ms"+(value.httpStatus?"；HTTP "+value.httpStatus:"");
  version.textContent="配置版本："+value.revision;version.className="field-help";
  status.textContent=value.message;scope.id="ai-probe-version-state";scope.className="field-help";
  $("ai-probe-result").replaceChildren(heading,detail,version,status,scope);renderAIProbeControls();
}
async function testSavedAIModel(entry){
  if(!authenticated||!aiState?.available||aiBusy||aiProbeBusy)return;
  const epoch=beginAIProbe(),token=csrf,revision=aiState.global.revision;
  aiProbeResult=null;$("ai-probe-result").replaceChildren();$("ai-probe-error").hidden=true;
  $("ai-probe-state").textContent="测试已保存的"+aiProbeNames[entry]+"模型中，最多等待30秒；不会自动重试。";renderAIProbeControls();
  const current=()=>authenticated&&csrf===token&&epoch===aiProbeEpoch&&!$("ai-view").hidden;
  try{
    const value=await api("/api/ai-test",{entry,revision});
    if(!current())return;
    renderAIProbeResult(value);$("ai-probe-state").textContent="本次测试已结束。仅验证基础文字调用，未运行 Agent 动作或 QQ 收发。";
  }catch(error){if(current()){$("ai-probe-state").textContent="测试未取得结果；不自动重试。";showError("ai-probe-error",error);}}
  finally{finishAIProbe(epoch);}
}
function beginAIProbe(){aiProbeBusy=true;return ++aiProbeEpoch;}
function finishAIProbe(epoch){if(epoch===aiProbeEpoch){aiProbeBusy=false;renderAIProbeControls();}}
function renderAISearchProbe(form){
  const panel=document.createElement("section"),note=document.createElement("p"),button=document.createElement("button"),state=document.createElement("p"),error=document.createElement("p"),result=document.createElement("div");
  note.className="field-help";note.textContent="发送一次固定公开查询（Go 编程语言 官方文档），可能消耗提供商额度。只测试已保存的全局搜索密钥，未保存草稿不参与，不会开启会话搜索。";
  button.id="ai-search-probe";button.type="button";button.className="secondary";button.textContent="测试已保存搜索密钥";button.onclick=testSavedSearchKey;
  state.id="ai-search-probe-state";state.setAttribute("role","status");state.textContent="仅测试已保存的全局搜索密钥，不代表已启用会话搜索。";
  error.id="ai-search-probe-error";error.setAttribute("role","alert");error.hidden=true;result.id="ai-search-probe-result";
  panel.append(note,button,state,error,result);form.append(panel);
  if(aiSearchProbeResult)renderAISearchProbeResult(aiSearchProbeResult);
  renderAIProbeControls();
}
function renderAISearchProbeResult(value){
  aiSearchProbeResult=value;if(!$("ai-search-probe-result"))return;
  const heading=document.createElement("h3"),detail=document.createElement("p"),message=document.createElement("p"),revision=document.createElement("p"),version=document.createElement("p");
  heading.textContent="搜索密钥 · "+({success:"基础调用通过",empty:"调用完成，无匹配结果"}[value.category]||"本次测试失败");
  detail.textContent="测试时间："+value.checkedAt+"；耗时："+value.milliseconds+" ms；合法结果："+value.resultCount+" 条"+(value.httpStatus?"；HTTP "+value.httpStatus:"");
  message.textContent=value.message;revision.textContent="配置版本："+value.revision;revision.className="field-help";version.id="ai-search-probe-version";version.className="field-help";
  $("ai-search-probe-result").replaceChildren(heading,detail,message,revision,version);renderAIProbeControls();
}
async function testSavedSearchKey(){
  const saved=aiBaselines.get("search")?.search;
  if(!authenticated||aiBusy||aiProbeBusy||!saved?.keyConfigured)return;
  const epoch=beginAIProbe(),token=csrf,revision=saved.revision;
  aiSearchProbeResult=null;$("ai-search-probe-result").replaceChildren();$("ai-search-probe-error").hidden=true;
  $("ai-search-probe-state").textContent="正在测试已保存搜索密钥，最多等待10秒；不会自动重试。";renderAIProbeControls();
  const current=()=>authenticated&&csrf===token&&epoch===aiProbeEpoch&&!$("ai-view").hidden;
  try{
    const value=await api("/api/ai-search-test",{revision});
    if(!current())return;
    renderAISearchProbeResult(value);
    if($("ai-search-probe-state"))$("ai-search-probe-state").textContent="本次测试已结束；不会开启会话搜索、运行 Agent 或发送 QQ 消息。";
  }catch(error){if(current()&&$("ai-search-probe-error")){$("ai-search-probe-state").textContent="测试未取得结果；不自动重试。";showError("ai-search-probe-error",error);}}
  finally{finishAIProbe(epoch);}
}
$("ai-probe-chat").addEventListener("click",()=>testSavedAIModel("chat"));
$("ai-probe-agent").addEventListener("click",()=>testSavedAIModel("agent"));
$("ai-probe-gpt").addEventListener("click",()=>testSavedAIModel("gpt"));
$("ai-probe-summary").addEventListener("click",()=>testSavedAIModel("summary"));
renderAIProbeControls();
