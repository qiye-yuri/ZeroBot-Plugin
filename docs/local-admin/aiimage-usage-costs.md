# 绘图服务商费用查询

## 后台费用查询

本地后台 → AI设置 → “画图 · 服务商费用” → “读取费用查询配置”。只有登录后台的管理员可以配置或查询；没有QQ设置Admin Key命令。

保存遵循[后台密钥操作规则](named-models.md#后台密钥操作规则)：保留、替换、确认清除分别校验，无效组合不会修改原配置。费用查询密钥仍独立保存，替换时继续校验Admin Key格式。

1. 在OpenAI API Platform组织设置的Admin keys创建独立密钥。不要填入原“画图密钥”字段。
2. 填入要统计的项目ID（必填，proj_…）。可选填写绘图API Key ID，注意是平台密钥记录的ID而不是密钥正文；不填时统计该项目所有密钥的GPT Image费用。
3. 填写Admin API Key；留空保留当前值，勾选确认可清除。当前密钥不会回显。若需要代理，填写费用查询专用代理，例如http://127.0.0.1:7897；不会借用绘图密钥或自动更改绘图代理。
4. 点击“备份并保存费用配置”。首次保存时会创建独立私有数据库及配置备份。
5. 选择UTC开始/结束日期，结束不包含，范围1–31天，点击“查询服务商费用”。例如2026-10-29至2026-10-30表示UTC29日全天，即北京时间29日08:00至30日08:00。

仅手动查询，不定时请求。固定访问https://api.openai.com/v1/organization/costs，不跟随重定向、不使用自定义生成接口地址。页面显示GPT Image费用总额、各费用项目数量/单位/金额、项目/Key筛选范围和查询时间；其他模型费用不会混入。完整处理分页，查询失败不覆盖上次结果，旧结果仍显示原查询时间。修改查询配置会清空旧报告以免混淆范围。

## 账单与本地预算的区别

- “预算占用”：沿用现有本地生成预留与结算规则；按北京时间自然月限制。
- “服务商费用”：官方按UTC天汇总的GPT Image费用，可能包含该项目其他应用。它单独展示，不重复扣减本地预算，不能按request ID拆分到单次请求。

对账必须使用同一项目/Key、日期和模型范围。不要把一天汇总直接与一张图片比较，也不要把UTC月账单直接当成北京时间月预算。401/403检查Admin Key和组织权限；没有记录或尚未查询不表示费用为零。

本机凭据位于后台私有目录image-costs.sqlite，备份为image-costs-backup-*.sqlite，包含密钥，禁止上传。本批不增加绘图usage结算或实际额度账本。

官方依据：[图片计费](https://developers.openai.com/api/docs/guides/image-generation)、[Costs接口](https://developers.openai.com/api/reference/resources/admin/subresources/organization/subresources/usage/methods/costs)。
