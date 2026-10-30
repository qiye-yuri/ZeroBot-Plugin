"use strict";
function resetAIImage(){aiBaselines.delete("image");$("ai-image-form").replaceChildren();$("ai-image-form").hidden=true;}
function renderAIImage(value){
 const form=$("ai-image-form");form.replaceChildren();form.hidden=false;
 const fields=document.createElement("fieldset"),heading=document.createElement("h2"),note=document.createElement("p");
 heading.textContent="AI画图 · 独立配置";note.className="field-help";note.textContent="本实例所有画图会话共用。地址须为完整图片生成端点；本地无鉴权服务可清除密钥。保存前备份，不改变插件开关。";fields.append(heading,note);
 const label=document.createElement("label"),mode=document.createElement("select");mode.id="ai-image-mode";label.htmlFor=mode.id;label.textContent="接口类型";
 for(const [v,t] of [["openai","OpenAI / 本地兼容接口"],["siliconflow","硅基流动兼容接口"]]){const option=document.createElement("option");option.value=v;option.textContent=t;mode.append(option);}mode.value=value.mode;fields.append(label,mode);
 const url=aiInput(fields,"image-url","图片生成接口地址","url",value.url);url.required=true;url.maxLength=2048;url.placeholder="https://api.openai.com/v1/images/generations";
 const model=aiInput(fields,"image-model","画图模型名","text",value.model);model.required=true;model.placeholder="填写服务提供的模型ID";
 const selectField=(id,title,options,current)=>{const l=document.createElement("label"),select=document.createElement("select");select.id=id;l.htmlFor=id;l.textContent=title;for(const [v,t] of options){const o=document.createElement("option");o.value=v;o.textContent=t;select.append(o);}select.value=current;fields.append(l,select);return select;};
 const quality=selectField("ai-image-quality","画图质量（OpenAI模式）",[["low","低"],["medium","中等"],["high","高"],["xhigh","超高 · 2.5"],["max","最高 · 2.5"],["auto","自动 · 按最高档预留"]],value.quality||"medium");
 const sizeChoices=[["1024x1024","1024×1024 · 正方形"],["1536x1024","1536×1024 · 横图"],["1024x1536","1024×1536 · 竖图"],["2048x2048","2K · 2048×2048"],["2048x1152","2K横图 · 2048×1152"],["1152x2048","2K竖图 · 1152×2048"],["2560x1440","2560×1440 · 横图"],["3840x2160","4K横图 · 3840×2160（实验性）"],["2160x3840","4K竖图 · 2160×3840（实验性）"],["2880x2880","最大方图 · 2880×2880（实验性）"],["auto","自动 · 按最大像素预留"],["custom","自定义尺寸"]];
 const savedSize=value.size||"1024x1024";const size=selectField("ai-image-size","默认分辨率",sizeChoices,sizeChoices.some(([v])=>v===savedSize)?savedSize:"custom");
 const customSize=aiInput(fields,"image-size-custom","自定义尺寸（宽x高，如1920x1088）","text",savedSize);customSize.maxLength=11;
 const defaults={referenceReserveUSD:"0.25",background:"auto",format:"png",compression:100,count:1,moderation:"auto",stream:false,partialImages:0,safetyIdentifier:"",proxyURL:"",timeoutSeconds:300};const optionValue={...defaults,...value.options};
 const background=selectField("ai-image-background","背景",[["auto","自动"],["opaque","不透明"],["transparent","透明（PNG/WebP）"]],optionValue.background);
 const format=selectField("ai-image-format","输出格式",[["png","PNG"],["jpeg","JPEG"],["webp","WebP"]],optionValue.format);
 const compression=aiInput(fields,"image-compression","压缩质量（0–100，仅JPEG/WebP）","number",optionValue.compression,0,100);
 const referenceReserve=aiInput(fields,"image-reference-reserve","每张参考图额外预算预留（美元，非固定收费）","number",optionValue.referenceReserveUSD,0.01,100);referenceReserve.step="0.01";
 const referenceHelp=document.createElement("p");referenceHelp.className="field-help";referenceHelp.textContent="同一条QQ消息附图并发送AI画图指令时，使用图片编辑接口。官方图片输入另收费，默认每张参考图额外预留$0.25×输出张数；这是可调整预算额度，不是官方每张报价。没附图不增加此项。";fields.append(referenceHelp);
 const count=aiInput(fields,"image-count","每次生成张数（1–10，按张预留预算）","number",optionValue.count,1,10);
 const moderation=selectField("ai-image-moderation","内容审核级别",[["auto","标准自动审核"],["low","较宽松（仍有内容限制）"]],optionValue.moderation);
 const stream=aiInput(fields,"image-stream","流式生成","checkbox",optionValue.stream);
 const partial=aiInput(fields,"image-partial","流式预览图数量（0–3，会额外占用预算）","number",optionValue.partialImages,0,3);
 const identifier=aiInput(fields,"image-safety-id","安全标识（可选匿名标识，勿填密钥或个人信息）","text",optionValue.safetyIdentifier);identifier.maxLength=128;
 const proxy=aiInput(fields,"image-proxy","画图专用代理（留空沿用服务进程环境）","text",optionValue.proxyURL);proxy.maxLength=2048;proxy.placeholder="http://127.0.0.1:7897";
 const timeout=aiInput(fields,"image-timeout","等待上限（秒，30–1200）","number",optionValue.timeoutSeconds,30,1200);
 const optionsHelp=document.createElement("p");optionsHelp.className="field-help";optionsHelp.textContent="新模型尺寸：两边为16的倍数、最大边3840、总像素655360–8294400、长宽比不超过3:1。超过2560×1440总像素为实验性；4K不是4096×4096。旧模型仅支持标准尺寸。auto预留最高质量/最大像素，实际等待和画面由模型决定。PNG忽略压缩率；透明背景不能配JPEG。";fields.append(optionsHelp);
 const budget=aiInput(fields,"image-budget","每月画图预算（美元，0表示停止生成）","number",value.monthlyBudgetUSD??"100",0,100000);budget.step="0.01";
 const custom=aiInput(fields,"image-custom-cost","兼容/本地服务每张最高总价（美元；未知留空，免费填0）","number",value.customCostUSD??"",0,10000);custom.step="0.000001";custom.required=false;
 const usage=document.createElement("p");usage.id="ai-image-budget-status";usage.className="field-help";
 const ledger=value.budget;usage.textContent=ledger?`${ledger.month}（北京时间）· 预算占用 $${ledger.chargedUSD} · 剩余 $${ledger.remainingUSD} · 已登记 ${ledger.requests} 次 · 未确认/处理中 $${ledger.uncertainUSD}。${ledger.stopped?"已停止生成。":"余额不足单次预留时也会停止。"}`:"本月预算将在读取后显示。";fields.append(usage);
 const priceNote=document.createElement("p");priceNote.id="ai-image-pricing-status";priceNote.className="field-help";priceNote.textContent=value.pricingNote||"OpenAI官方价格自动估算；其他接口需明确单次总价。";fields.append(priceNote);
 const help=document.createElement("p");help.className="field-help";help.textContent="先登记预算再请求，重启不清零，北京时间每月1日重置。官方预算按公开图片单价加20%余量及提示词输入预留计数，不是服务商实际账单；超时或失败不自动退还预留。未知价格停止调用。非官方单次总价由你核实，低估可能超出真实账单预算。Flare和Sunburst已按官方2.5专用计算器接入；实际消耗可能不同，预算使用估价和余量。用 --size=4k、--quality=max、--n=2 等覆盖本次参数；自然语言描述不会自动改API参数。";fields.append(help);
 const prices=document.createElement("details"),summary=document.createElement("summary"),table=document.createElement("table");summary.textContent="模型价格对比（1024×1024，每张输出美元，2026-10-29）";
 for(const row of [["模型","低","中","高"],["gpt-image-1-mini（Deprecated）","0.005","0.011","0.036"],["gpt-image-1.5（Deprecated）","0.009","0.034","0.133"],["gpt-image-2","0.006","0.053","0.211"],["2.5 Flare（估价）","0.00588","0.01317","0.05268"],["2.5 Sunburst（估价）","0.00588","0.01317","0.05268"]]){const tr=document.createElement("tr");for(const text of row){const td=document.createElement(table.children.length?"td":"th");td.textContent=text;tr.append(td);}table.append(tr);}
 const priceHelp=document.createElement("p");priceHelp.className="field-help";priceHelp.textContent="均另计提示词输入。2.5专用计算器中等正方形为439输出token，横/竖图343；Standard文本输入$5/百万token、图片输出$30/百万token。Flare与Sunburst共用官方估算组，不保证实际每张费用相同，不使用Batch或缓存折扣。";
 const priceLink=document.createElement("a");priceLink.href="https://developers.openai.com/api/docs/guides/image-generation#calculating-costs";priceLink.textContent="OpenAI官方价格与计算说明";priceLink.target="_blank";priceLink.rel="noopener noreferrer";const priceScroll=document.createElement("div");priceScroll.className="table-scroll";priceScroll.style.overflowX="auto";priceScroll.append(table);prices.append(summary,priceScroll,priceHelp,priceLink);fields.append(prices);
 const updateCustom=()=>{customSize.disabled=size.value!=="custom";compression.disabled=format.value==="png";partial.disabled=!stream.checked;if(!stream.checked)partial.value="0";const official=url.value.trim()==="https://api.openai.com/v1/images/generations"&&mode.value==="openai";custom.disabled=official;quality.disabled=mode.value!=="openai";};url.addEventListener("input",updateCustom);mode.addEventListener("change",updateCustom);size.addEventListener("change",updateCustom);format.addEventListener("change",updateCustom);stream.addEventListener("change",updateCustom);updateCustom();
 const keyLabel=document.createElement("label"),action=document.createElement("select"),key=document.createElement("input"),clear=document.createElement("label"),confirm=document.createElement("input"),status=document.createElement("p");
 action.id="ai-image-key-action";keyLabel.htmlFor=action.id;keyLabel.textContent="画图密钥";
 for(const [v,t] of [["keep","保留当前密钥"],["replace","替换为新密钥"],["clear","清除密钥"]]){const option=document.createElement("option");option.value=v;option.textContent=t;action.append(option);}
 key.id="ai-image-key";key.type="password";key.autocomplete="new-password";key.maxLength=4096;key.disabled=true;key.setAttribute("aria-label","画图新密钥");
 confirm.id="ai-image-key-clear";confirm.type="checkbox";clear.className="check-label";clear.hidden=true;clear.append(confirm,document.createTextNode("确认清除画图密钥"));
 status.className="field-help";status.textContent=value.keyConfigured?"画图密钥已设置，当前值不显示。":"画图密钥未设置，不借用聊天密钥。";
 action.addEventListener("change",()=>{key.disabled=action.value!=="replace";key.required=action.value==="replace";if(action.value!=="replace")key.value="";clear.hidden=action.value!=="clear";confirm.checked=false;});
 fields.append(keyLabel,action,status,key,clear);
 const save=document.createElement("button"),discard=document.createElement("button");save.type="submit";save.className="primary";save.textContent="备份并保存画图配置";discard.type="button";discard.className="secondary";discard.dataset.aiDiscard="image";discard.textContent="放弃此表单更改";discard.onclick=()=>discardAIForm("image");fields.append(save,discard);form.append(fields);
 rememberAIForm("image",form);aiBaselines.get("image").image=structuredClone(value);
 form.onsubmit=async event=>{
  event.preventDefault();if(!authenticated||aiBusy)return;
  const body={revision:value.revision,mode:mode.value,url:url.value.trim(),model:model.value.trim(),quality:quality.value,size:size.value==="custom"?customSize.value.trim():size.value,options:{referenceReserveUSD:referenceReserve.value,background:background.value,format:format.value,compression:Number(compression.value),count:Number(count.value),moderation:moderation.value,stream:stream.checked,partialImages:stream.checked?Number(partial.value):0,safetyIdentifier:identifier.value,proxyURL:proxy.value.trim(),timeoutSeconds:Number(timeout.value)},monthlyBudgetUSD:budget.value,customCostUSD:custom.disabled?"":custom.value,key:{action:action.value}};
  if(action.value==="replace")body.key.value=key.value;
  if(action.value==="clear"){if(!confirm.checked){showError("ai-error",new Error("请确认清除画图密钥。"));return;}body.key.confirm=true;}
  const generation=aiGeneration;aiBusyState(true);$("ai-error").hidden=true;
  try{const saved=await api("/api/ai-image",body);if(generation!==aiGeneration||!authenticated)return;renderAIImage(saved);$("ai-status").textContent="画图配置已备份并保存，下一次画图生效；本次未调用模型。";}
  catch(error){if(generation===aiGeneration&&authenticated)showError("ai-error",error);}
  finally{if(generation===aiGeneration)aiBusyState(false);}
 };
}
async function loadAIImage(){
 if(!authenticated||aiBusy)return;
 if(aiDrafts().includes("image")&&!window.confirm("重新读取将放弃画图表单未保存的更改，是否继续？"))return;
 const generation=aiGeneration;aiBusyState(true);$("ai-error").hidden=true;
 try{const value=await api("/api/ai-image");if(generation!==aiGeneration||!authenticated)return;renderAIImage(value);}
 catch(error){if(generation===aiGeneration&&authenticated)showError("ai-error",error);}
 finally{if(generation===aiGeneration)aiBusyState(false);}
}
$("ai-image-load").addEventListener("click",loadAIImage);
