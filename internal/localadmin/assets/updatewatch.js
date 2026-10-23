"use strict";
let updateWatchState = null, updateWatchBusy = false, updateWatchGeneration = 0;
function resetUpdateWatch() {
  updateWatchGeneration++; updateWatchState = null; updateWatchBusy = false;
  $("uw-reload").disabled = false; $("uw-form").reset(); $("uw-fields").disabled = true;
  $("uw-error").hidden = true; $("uw-status").textContent = "进入页面后读取配置。";
  $("uw-runtime").textContent = "尚未读取。";
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
  const last = value.lastCheck.startsWith("0001-") ? "尚未检查" : new Date(value.lastCheck).toLocaleString("zh-CN",{hour12:false});
  const errors = Object.entries(value.sourceErrors || {}).map(([source,error])=>source+"："+error);
  $("uw-runtime").textContent = `最近检查：${last}；基线来源：${value.baseline}；待通知：${value.pending}。` + (value.lastError || "") + (errors.length ? " 来源失败："+errors.join("；") : "");
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
  updateWatchBusy = true; $("uw-fields").disabled = true; $("uw-reload").disabled = true;
  $("uw-error").hidden = true; $("uw-status").textContent = save ? "正在保存…" : "正在读取…";
  try {
    const value = await api("/api/updatewatch",body);
    if (!authenticated || generation !== updateWatchGeneration) return;
    renderUpdateWatch(value); $("uw-status").textContent = save ? "已保存，最迟一分钟生效。" : "已读取当前配置。切换页面保留草稿。";
  } catch(error) {
    if (authenticated && generation === updateWatchGeneration) {showError("uw-error",error);$("uw-status").textContent = "操作未完成；已有草稿保留。";}
  } finally {
    if (generation === updateWatchGeneration) {updateWatchBusy=false;$("uw-fields").disabled=!authenticated || !updateWatchState;$("uw-reload").disabled=false;}
  }
}
$("uw-form").addEventListener("submit",event=>{event.preventDefault();loadUpdateWatch(true);});
$("uw-form").addEventListener("input",()=>{$("uw-status").textContent="有未保存更改；切换页面保留草稿。";});
$("uw-reload").addEventListener("click",()=>loadUpdateWatch());
