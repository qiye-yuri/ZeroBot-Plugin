let resourcesGeneration=0, resourcesBusy=false, resourcesServerBusy=false;
let resourcesSnapshot=null;
function renderResourceControls(){
  $("resources-check").disabled=!authenticated || resourcesBusy || resourcesServerBusy;
  $("resources-refresh").disabled=!authenticated || resourcesBusy;
  $("resources-export").disabled=!authenticated || resourcesBusy || !resourcesSnapshot?.report;
}
function leaveResources(){resourcesGeneration++;resourcesBusy=false;renderResourceControls();}
function resetResources(){
  resourcesGeneration++;resourcesBusy=false;resourcesServerBusy=false;resourcesSnapshot=null;$("resources-only-issues").checked=true;
  $("resources-result").replaceChildren();$("resources-status").textContent="";
  $("resources-error").hidden=true;renderResourceControls();
}
function renderResourceReport(value){
  resourcesSnapshot=value;
  const root=$("resources-result"),report=value.report;root.replaceChildren();
  resourcesServerBusy=!!value.busy;
  $("resources-status").textContent=value.busy ? "检查正在进行，可稍后读取结果。" : report ? "显示上次已完成检查。" : "尚未检查；点击检查本地资源开始。";
  renderResourceControls();
  if(!report)return;
  const date=document.createElement("p");date.textContent=`检查时间：${report.startedAt} 至 ${report.completedAt}`;root.append(date);
  for(const group of report.groups || []){
    const box=document.createElement("section"),title=document.createElement("h3"),summary=document.createElement("p");
    title.textContent=group.name;box.append(title);
    summary.textContent=group.error ? `${group.error}；预期资源数量未确定。` : `预期 ${group.expected} · 正常 ${group.valid} · 缺失 ${group.missing} · 损坏/不可解码 ${group.damaged} · 无法读取 ${group.unreadable} · 已统计文件大小 ${(group.bytes/1048576).toFixed(2)} MiB`;
    box.append(summary);
    const details=document.createElement("details"),heading=document.createElement("summary"),list=document.createElement("ul");
    heading.textContent="查看资源明细（路径相对于data目录）";details.append(heading);
    const files=(group.files || []).filter(file=>!$("resources-only-issues").checked || file.status!=="正常");
    if(!files.length){const empty=document.createElement("p");empty.textContent=group.error?"清单不可用，未检查图片明细。":"当前筛选下无资源明细。";details.append(empty);}
    for(const file of files){const item=document.createElement("li");item.textContent=`${file.status === "损坏" ? "损坏/不可解码" : file.status} · ${file.path} · ${file.bytes} 字节${file.reason ? " · "+file.reason : ""}`;list.append(item);}
    details.append(list);box.append(details);root.append(box);
  }
}
async function loadResources(check=false){
  if(!authenticated || resourcesBusy || (check && resourcesServerBusy))return;
  const generation=resourcesGeneration;resourcesBusy=true;renderResourceControls();
  $("resources-error").hidden=true;$("resources-status").textContent=check?"正在检查本地资源…":"正在读取上次结果…";
  try{
    const value=await api(check?"/api/resources/check":"/api/resources",check?{}:undefined);
    if(!authenticated || generation!==resourcesGeneration)return;
    renderResourceReport(value);
  }catch(e){
    if(authenticated && generation===resourcesGeneration){showError("resources-error",e);$("resources-status").textContent="本次操作未完成，下方旧结果不代表本次检查。";}
  }finally{if(generation===resourcesGeneration){resourcesBusy=false;renderResourceControls();}}
}
$("resources-check").addEventListener("click",()=>loadResources(true));
$("resources-refresh").addEventListener("click",()=>loadResources(false));

$("resources-only-issues").addEventListener("change",()=>{
  if(authenticated && resourcesSnapshot){
    const status=$("resources-status").textContent;
    renderResourceReport(resourcesSnapshot);
    $("resources-status").textContent=status;
  }
});
function exportResources(){
  if(!authenticated || resourcesBusy || !resourcesSnapshot?.report)return;
  let url=null,link=null;
  try{
    const report={format:"zerobot-resource-check",version:1,exportedAt:new Date().toISOString(),scope:"tarot-and-built-in-wife",report:resourcesSnapshot.report};
    url=URL.createObjectURL(new Blob([JSON.stringify(report,null,2)+"\n"],{type:"application/json;charset=utf-8"}));
    link=document.createElement("a");link.href=url;link.download="zerobot-resource-check-"+new Date().toISOString().replace(/[-:.]/g,"")+".json";document.body.append(link);link.click();
  }catch(e){showError("resources-error",e);}
  finally{link?.remove();if(url)setTimeout(()=>URL.revokeObjectURL(url),1000);}
}
$("resources-export").addEventListener("click",exportResources);
