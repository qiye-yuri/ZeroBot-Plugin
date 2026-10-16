"use strict";
let aiProbeBusy=false,aiProbeEpoch=0,aiProbeResult=null;
function resetAIProbe(){
  aiProbeEpoch++;aiProbeBusy=false;aiProbeResult=null;
  $("ai-probe-result").replaceChildren();$("ai-probe-error").hidden=true;
  $("ai-probe-state").textContent="先读取已保存配置，再手动测试。";
  renderAIProbeControls();
}
function showAIProbe(visible){if(!visible)resetAIProbe();else renderAIProbeControls();}
function renderAIProbeControls(){
  for(const id of ["ai-probe-chat","ai-probe-agent"])$(id).disabled=!authenticated||!aiState?.available||aiBusy||aiProbeBusy;
  if(aiProbeResult){
    $("ai-probe-version-state").textContent=aiProbeResult.revision===aiState?.global?.revision?"基于当前已读取的保存版本；未保存草稿不参与测试。":"历史结果：已读取配置版本发生变化，请按需重新测试。";
  }
}
function renderAIProbeResult(value){
  aiProbeResult=value;
  const heading=document.createElement("h3"),detail=document.createElement("p"),version=document.createElement("p"),status=document.createElement("p"),scope=document.createElement("p");
  heading.textContent=(value.entry==="agent"?"Agent":"聊天")+"模型 · "+(value.success?"基础调用通过":"本次测试失败");
  detail.textContent="模型："+value.model+"；测试时间："+value.checkedAt+"；耗时："+value.milliseconds+" ms"+(value.httpStatus?"；HTTP "+value.httpStatus:"");
  version.textContent="配置版本："+value.revision;version.className="field-help";
  status.textContent=value.message;scope.id="ai-probe-version-state";scope.className="field-help";
  $("ai-probe-result").replaceChildren(heading,detail,version,status,scope);renderAIProbeControls();
}
async function testSavedAIModel(entry){
  if(!authenticated||!aiState?.available||aiBusy||aiProbeBusy)return;
  const epoch=++aiProbeEpoch,token=csrf,revision=aiState.global.revision;
  aiProbeBusy=true;aiProbeResult=null;$("ai-probe-result").replaceChildren();$("ai-probe-error").hidden=true;
  $("ai-probe-state").textContent="测试已保存的"+(entry==="agent"?" Agent":"聊天")+"模型中，最多等待30秒；不会自动重试。";renderAIProbeControls();
  const current=()=>authenticated&&csrf===token&&epoch===aiProbeEpoch&&!$("ai-view").hidden;
  try{
    const value=await api("/api/ai-test",{entry,revision});
    if(!current())return;
    renderAIProbeResult(value);$("ai-probe-state").textContent="本次测试已结束。仅验证基础文字调用，未运行 Agent 动作或 QQ 收发。";
  }catch(error){if(current()){$("ai-probe-state").textContent="测试未取得结果；不自动重试。";showError("ai-probe-error",error);}}
  finally{if(epoch===aiProbeEpoch){aiProbeBusy=false;renderAIProbeControls();}}
}
$("ai-probe-chat").addEventListener("click",()=>testSavedAIModel("chat"));
$("ai-probe-agent").addEventListener("click",()=>testSavedAIModel("agent"));
renderAIProbeControls();
