"use strict";
let updateWatchBaseline = null;
const updateWatchDraftFields = [["uw-enabled","启用"],["uw-daily","每日时间"],["uw-interval","间隔"],["uw-model","模型入口"],["uw-owner","主人"],["uw-bot","机器人"],["uw-cf","CurseForge来源"],["uw-ftb","FTB来源"],["uw-github","GitHub来源"],["uw-repos","关联仓库"],["uw-announcements","公告链接"]];
function captureUpdateWatchDraft() {
  updateWatchBaseline = updateWatchDraftFields.map(([id]) => $(id).type === 'checkbox' ? $(id).checked : $(id).value);
  $("uw-draft-feedback").textContent = '';
  renderUpdateWatchDraft();
}
function renderUpdateWatchDraft() {
  const changed = updateWatchBaseline ? updateWatchDraftFields.filter(([id], i) => ($(id).type === 'checkbox' ? $(id).checked : $(id).value) !== updateWatchBaseline[i]).map(([,label]) => label) : [];
  $("uw-draft").textContent = !updateWatchBaseline ? '读取配置后可编辑' : changed.length ? '未保存字段：'+changed.length+'项 · '+changed.join('、') : '无未保存更改';
  $("uw-undo").disabled = !authenticated || !updateWatchBaseline || updateWatchBusy || !changed.length;
}
function undoUpdateWatchDraft() {
  if (!authenticated || !updateWatchBaseline || updateWatchBusy) return;
  updateWatchDraftFields.forEach(([id], i) => {const el=$(id); if(el.type==='checkbox') el.checked=updateWatchBaseline[i]; else el.value=updateWatchBaseline[i];});
  renderUpdateWatchDraft();
  $("uw-draft-feedback").textContent = '已恢复本页最近读取或保存成功的值，未读取或保存服务器。';
}
for (const event of ['input','change']) $("uw-form").addEventListener(event, () => {$("uw-draft-feedback").textContent='';renderUpdateWatchDraft();});
$("uw-undo").addEventListener('click', undoUpdateWatchDraft);

let updateWatchState = null, updateWatchBusy = false, updateWatchGeneration = 0, updateWatchRefreshing = false;
function resetUpdateWatch() {
  updateWatchBaseline=null;$("uw-draft-feedback").textContent="";
  updateWatchGeneration++; updateWatchState = null; updateWatchBusy = false; updateWatchRefreshing = false;
  $("uw-reload").disabled = false; $("uw-form").reset(); $("uw-fields").disabled = true;
  $("uw-error").hidden = true; $("uw-status").textContent = "进入页面后读取配置。";
  renderUpdateWatchRuntime();
  $("uw-runtime-status").textContent = ""; renderUpdateWatchControls();
}
function renderUpdateWatch(value) {
  updateWatchState = value;
  const c = value.config;
  $("uw-enabled").checked = c.enabled;
  $("uw-interval").value = c.intervalHours;
  $("uw-daily").value = c.dailyTime || "";
  $("uw-model").value = c.modelRole;
  for (const option of $("uw-model").options) {
    const m = value.models?.find(x => x.role === option.value);
    option.textContent = (option.value || "Chat") + " · " + (m?.ready ? m.name : "未配置完整");
  }
  $("uw-owner").replaceChildren();
  for (const id of new Set([0,...value.owners,c.ownerId])) {
    const option = document.createElement("option"); option.value = id;
    option.textContent = id ? String(id) + (value.owners.includes(id) ? "" : "（不再是主人）") : "自动选择唯一主人";
    $("uw-owner").append(option);
  }
  $("uw-owner").value = c.ownerId;
  $("uw-bot").value = c.botId || (value.bots.length === 1 ? value.bots[0] : "");
  $("uw-bots").replaceChildren(...value.bots.map(id => {const o=document.createElement("option");o.value=id;return o;}));
  $("uw-cf").checked = c.curseForge; $("uw-ftb").checked = c.ftb; $("uw-github").checked = c.github;
  $("uw-repos").value = (c.repositories || []).join("\n");
  $("uw-announcements").value = (c.announcements || []).join("\n");
  captureUpdateWatchDraft();
  renderUpdateWatchRuntime(value);
}
function renderUpdateWatchRuntime(value) {
  const root = $("uw-runtime");
  if (!value) {root.textContent = "尚未读取。"; return;}
  const last = value.lastCheck.startsWith("0001-") ? "尚未检查" : new Date(value.lastCheck).toLocaleString("zh-CN",{hour12:false});
  const summary = document.createElement("dl");
  for (const [id,label,text] of [
    ["uw-last-check","最近检查（浏览器本地时间）",last],
    ["uw-baseline-count","已初始化来源数",value.baseline+"个"],
    ["uw-pending-count","待发通知",value.pending+"条"]
  ]) {
    const term=document.createElement("dt"), detail=document.createElement("dd");
    term.textContent=label; detail.id=id; detail.textContent=text;
    summary.append(term,detail);
  }
  const lastHeading=document.createElement("h3"), lastError=document.createElement("p");
  lastHeading.textContent="最近错误"; lastError.id="uw-last-error";
  lastError.textContent=value.lastError || "未记录最近错误";
  const sourceHeading=document.createElement("h3"), sources=document.createElement("ul");
  sourceHeading.textContent="来源错误"; sources.id="uw-source-errors";
  for (const source of Object.keys(value.sourceErrors || {}).sort()) {
    const item=document.createElement("li"), name=document.createElement("strong"), error=document.createElement("p");
    name.textContent=source; error.textContent=value.sourceErrors[source]; item.append(name,error); sources.append(item);
  }
  const empty=document.createElement("p"); empty.textContent="未记录来源错误";
  root.replaceChildren(summary,lastHeading,lastError,sourceHeading,sources.childElementCount ? sources : empty);
  $("uw-runtime-status").textContent = "已刷新运行状态。";
}

async function loadUpdateWatch(save = false) {
  if (!authenticated || updateWatchBusy || (save && !updateWatchState)) return;
  let body;
  if (save) {
    const lines = id => $(id).value.split(/\r?\n/).map(s=>s.trim()).filter(Boolean);
    const owner = Number($("uw-owner").value), bot = Number($("uw-bot").value);
    if (!Number.isSafeInteger(owner) || !Number.isSafeInteger(bot) || bot <= 0) {showError("uw-error",new Error("请填写有效的机器人 QQ"));return;}
    body = {revision:updateWatchState.revision,config:{enabled:$("uw-enabled").checked,intervalHours:Number($("uw-interval").value),dailyTime:$("uw-daily").value,modelRole:$("uw-model").value,ownerId:owner,botId:bot,curseForge:$("uw-cf").checked,ftb:$("uw-ftb").checked,github:$("uw-github").checked,repositories:lines("uw-repos"),announcements:lines("uw-announcements")}};
  }
  const generation = updateWatchGeneration;
  updateWatchBusy = true; renderUpdateWatchControls();
  $("uw-error").hidden = true; $("uw-status").textContent = save ? "正在保存…" : "正在读取…";
  try {
    const value = await api("/api/updatewatch",body);
    if (!authenticated || generation !== updateWatchGeneration) return;
    renderUpdateWatch(value); $("uw-status").textContent = save ? "已保存，最迟一分钟生效。" : "已读取当前配置。切换页面保留草稿。";
  } catch(error) {
    if (authenticated && generation === updateWatchGeneration) {showError("uw-error",error);$("uw-status").textContent = "操作未完成；已有草稿保留。";}
  } finally {
    if (generation === updateWatchGeneration) {updateWatchBusy=false;renderUpdateWatchControls();}
  }
}
$("uw-form").addEventListener("submit",event=>{event.preventDefault();loadUpdateWatch(true);});

$("uw-reload").addEventListener("click",()=>loadUpdateWatch());

function renderUpdateWatchControls() {
  renderUpdateWatchDraft();
  $("uw-fields").disabled = !authenticated || !updateWatchState || (updateWatchBusy && !updateWatchRefreshing);
  $("uw-save").disabled = updateWatchBusy;
  $("uw-reload").disabled = updateWatchBusy;
  $("uw-runtime-refresh").disabled = !authenticated || !updateWatchState || updateWatchBusy;
}
function leaveUpdateWatchRuntime() {
  if (!updateWatchRefreshing) return;
  updateWatchGeneration++; updateWatchRefreshing=false; updateWatchBusy=false;
  $("uw-runtime-status").textContent="已离开页面，本次刷新不再应用；显示上次读取结果。";
  renderUpdateWatchControls();
}
async function refreshUpdateWatchRuntime() {
  if (!authenticated || !updateWatchState || updateWatchBusy || $("updatewatch-view").hidden) return;
  const generation=updateWatchGeneration;
  updateWatchBusy=true; updateWatchRefreshing=true; renderUpdateWatchControls();
  $("uw-runtime-status").textContent="正在刷新运行状态…";
  try {
    const value=await api('/api/updatewatch');
    if (!authenticated || generation!==updateWatchGeneration) return;
    renderUpdateWatchRuntime(value);
  } catch(error) {
    if (authenticated && generation===updateWatchGeneration) $("uw-runtime-status").textContent="刷新未完成，显示上次读取结果："+error.message;
  } finally {
    if (generation===updateWatchGeneration) {updateWatchBusy=false;updateWatchRefreshing=false;renderUpdateWatchControls();}
  }
}
$("uw-runtime-refresh").addEventListener("click",refreshUpdateWatchRuntime);
