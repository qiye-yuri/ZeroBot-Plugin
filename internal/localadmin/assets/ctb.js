"use strict";
let ctbState=null,ctbBusy=false,ctbGeneration=0,ctbRefreshing=false;
function resetCTB(){ctbGeneration++;ctbState=null;ctbBusy=false;ctbRefreshing=false;$("ctb-form").reset();$("ctb-fields").disabled=true;$("ctb-reload").disabled=false;$("ctb-error").hidden=true;$("ctb-last").textContent="";$("ctb-status").textContent="";$("ctb-task-status").textContent="";renderCTBControls();}
async function loadCTB(save=false){if(!authenticated||ctbBusy||(save&&!ctbState))return;let body;if(save)body={revision:ctbState.revision,config:{enabled:$("ctb-enabled").checked,executable:$("ctb-exe").value.trim(),outputDir:$("ctb-dir").value.trim(),timeoutMinutes:Number($("ctb-timeout").value),threads:Number($("ctb-threads").value),noProxy:$("ctb-no-proxy").checked,upload:$("ctb-upload").checked,groupAccess:$("ctb-group-access").value,groupDelivery:$("ctb-group-delivery").value,groupDailyLimit:Number($("ctb-group-limit").value)}};const gen=ctbGeneration;ctbBusy=true;renderCTBControls();$("ctb-error").hidden=true;try{const v=await api('/api/ctb',body);if(!authenticated||gen!==ctbGeneration)return;ctbState=v;const c=v.config;$("ctb-enabled").checked=c.enabled;$("ctb-exe").value=c.executable;$("ctb-dir").value=c.outputDir;$("ctb-timeout").value=c.timeoutMinutes;$("ctb-threads").value=c.threads;$("ctb-no-proxy").checked=c.noProxy;$("ctb-upload").checked=c.upload;$("ctb-group-access").value=c.groupAccess||"disabled";$("ctb-group-delivery").value=c.groupDelivery||"group";$("ctb-group-limit").value=c.groupDailyLimit||5;renderCTBTask(v);$("ctb-status").textContent=save?'已保存，无需重启。':'已读取。切换页面保留草稿。';}catch(e){if(authenticated&&gen===ctbGeneration){showError('ctb-error',e);$("ctb-status").textContent='操作失败，草稿已保留。';}}finally{if(gen===ctbGeneration){ctbBusy=false;renderCTBControls();}}}
$("ctb-form").addEventListener('submit',e=>{e.preventDefault();loadCTB(true)});$("ctb-form").addEventListener('input',()=>{$("ctb-status").textContent='有未保存更改。'});$("ctb-reload").addEventListener('click',()=>loadCTB());

function renderCTBControls(){
  $("ctb-fields").disabled=!authenticated||!ctbState||(ctbBusy&&!ctbRefreshing);
  $("ctb-save").disabled=ctbBusy;
  $("ctb-reload").disabled=ctbBusy;
  $("ctb-task-refresh").disabled=!authenticated||!ctbState||ctbBusy;
}
function renderCTBTask(v){
  $("ctb-last").textContent=(v.busy?'执行中；':'空闲；')+(v.last||'本次启动尚未执行任务');
  $("ctb-task-status").textContent='已刷新任务状态。';
}
function leaveCTBStatus(){
  if(!ctbRefreshing)return;
  ctbGeneration++;ctbRefreshing=false;ctbBusy=false;
  $("ctb-task-status").textContent='已离开页面，本次刷新不再应用；显示上次读取结果。';
  renderCTBControls();
}
async function refreshCTBStatus(){
  if(!authenticated||!ctbState||ctbBusy||$("ctb-view").hidden)return;
  const gen=ctbGeneration;
  ctbBusy=true;ctbRefreshing=true;renderCTBControls();
  $("ctb-task-status").textContent='正在刷新任务状态…';
  try{
    const v=await api('/api/ctb');
    if(!authenticated||gen!==ctbGeneration)return;
    renderCTBTask(v);
  }catch(e){
    if(authenticated&&gen===ctbGeneration)$("ctb-task-status").textContent='刷新失败，显示上次读取结果：'+e.message;
  }finally{
    if(gen===ctbGeneration){ctbBusy=false;ctbRefreshing=false;renderCTBControls();}
  }
}
$("ctb-task-refresh").addEventListener('click',refreshCTBStatus);
