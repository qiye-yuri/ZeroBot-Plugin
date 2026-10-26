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
function resourceAdvice(file){
  if(file.status==="正常")return "";
  const reason=file.reason || "";
  if(reason.startsWith("像素数超过4000万"))return "超过检查上限不代表损坏。可用可信本地图片查看器人工核对；保留原件，不要仅凭此结果删除或重新下载。";
  if(reason.startsWith("文件超过读取大小上限"))return "文件超过检查大小上限。先人工核对文件类型和大小，保留原件，不自动压缩或替换。";
  if(reason==="没有读取权限")return "核对机器人运行账户对文件及上级目录的读取权限；不要为排查而开放所有人权限。";
  if(reason==="路径包含链接，未跟随读取")return "检查资源目录或文件是否为链接。确认可信来源后再人工整理为普通本地文件，不绕过链接检查。";
  if(reason==="目标不是普通文件")return "核对该位置是否被目录等非普通文件占用，先备份再人工整理，不直接删除。";
  if(reason==="路径不在允许范围")return "核对清单中的相对路径和清单来源；不要通过修改检查范围读取外部文件。";
  if(file.status==="缺失")return "核对清单对应的文件及上级目录是否存在；补齐方式见本组维护说明。";
  if(file.status==="损坏")return "可能是文件损坏或格式不受支持。先用可信本地工具核对实际编码，不只按扩展名判断；确认需要替换后参考本组维护说明。";
  return "原因尚未明确。核对文件占用、运行账户读取权限及原检查时间，人工处理后再显式检查；不要直接删除或替换。";
}
function resourceMaintenanceAdvice(group){
  if(group.error)return "清单不可用，尚不能判断图片是否缺失。先核对清单是否存在、格式和来源、读取权限及大小限制；不要依据本次结果批量补图。";
  if(!(group.files || []).some(f=>f.status==="缺失" || f.status==="损坏"))return "";
  if(group.id==="tarot")return "塔罗维护：确认清单可信且文件确需补齐后，可由维护者在项目根目录手动运行 python scripts/cache-tarot-images.py（需要Python与Pillow）。已有脚本会检查整套塔罗清单，可能联网下载并替换文件；旧损坏文件备份到.local。脚本要求78张牌、156个路径；清单不符时应先核对，不强行修改。这里只显示指引，不执行命令。";
  if(group.id==="wife")return "内置角色维护：目前没有已确认可复用的一键修复入口。请从可信备份或来源人工核对并补齐清单对应图片，替换前保留原件；不要运行塔罗脚本处理角色库。";
  return "本组没有已确认的修复入口，请先人工核对清单和可信备份。";
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
    const advice=resourceMaintenanceAdvice(group);
    if(advice){const help=document.createElement("p");help.className="resource-maintenance-advice";help.textContent=advice;box.append(help);}
    const details=document.createElement("details"),heading=document.createElement("summary"),list=document.createElement("ul");
    heading.textContent="查看资源明细（路径相对于data目录）";details.append(heading);
    const files=(group.files || []).filter(file=>!$("resources-only-issues").checked || file.status!=="正常");
    if(!files.length){const empty=document.createElement("p");empty.textContent=group.error?"清单不可用，未检查图片明细。":"当前筛选下无资源明细。";details.append(empty);}
    for(const file of files){const item=document.createElement("li");item.textContent=`${file.status === "损坏" ? "损坏/不可解码" : file.status} · ${file.path} · ${file.bytes} 字节${file.reason ? " · "+file.reason : ""}`;const advice=resourceAdvice(file);if(advice){const help=document.createElement("p");help.className="resource-file-advice";help.textContent="处理建议："+advice;item.append(help);}list.append(item);}
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
