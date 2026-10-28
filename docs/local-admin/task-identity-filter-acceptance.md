# 按会话或任务标识筛选验收清单

日期：2026-10-28（Asia/Shanghai）。实际基线PR #55头b6002329c99846a2d892ae405a264943c8352f27，分支codex/task-identity-filter，base codex/task-usage-table。设计见[计划](post-pr55-development-plan.md)，操作见[使用说明](model-request-statistics.md#按会话或任务标识筛选)。

新增原生搜索框，复用grouped/session/task完整精确匹配，trim首尾空白，保留大小写；纳入既有禁用、刷新、清空和重置流程。无新增状态、接口、存储、正则验证或历史查询。

| 编号 | 状态 | 证据范围 |
| --- | --- | --- |
| C01～C04 | 自动化通过 | 固定样例、完整大小写/空白、OR不重复、独立项部分标识排除、整组交集、五种排序及源数据保持 |
| C05～C07 | 自动化通过 | input即时、Escape原生清除、Enter无提交、输入/summary焦点、刷新/淘汰/跨启动、阈值/重置、失败恢复和导航清理 |
| C08 | 自动化通过 | 44px与Tab顺序、长查询、四宽度明暗8份截图、完整下载独立校验，无新增网络 |
| C09 | 通过 | 最终150项隔离、1324份备份、资源保护及实机空态/输入/刷新/清除/重置/报告 |
| C10 | 待集中验收（2/5） | 最终累计候选真实非空查询、表格、报告与QQ收件 |
| C11 | 待验收后合入 | 依赖#55，验收后先#55再本批合入 |

93项相关回归通过、0失败、0跳过，JavaScript语法通过。旧布局测试只查询select，已扩展为检查select/input并完整复测；产品未为测试添加兜底。四宽度明暗8份截图复核通过，仅扩展输入框局部样式。当前1/5，本批完成部署独立检查后按实际QQ缺口登记；旧样本或合成数据不冒充真实非空。data及两份无关测试修改留本机，程序、配置、密钥和私有证据不上传。

## 最终部署与独立验收

2026-10-28 21:36（北京时间）一次部署成功。源码45d030f03f8bb3a0dbe0b84c108173b8f210758f，最终程序SHA256为8869E038BCE863D5427EA48CE5C84BE6F8BBF39FE7D02E6146E05C2CBFD07263；页面版本45d030f0及既有本地修改。最终完整隔离passed=true、isolated=true、realQQMessagesSent=false，150个Passed全部为true。

停机备份1324份文件并核对来源哈希，仅替换ZeroBot；原配置/凭据、OneBot进程与登录、监控状态保持，单实例、账号握手与后台200通过。8份实际静态资源与源码一致，未登录401、跨来源403通过。备份位于.local/task-identity-filter/deployment-20261028-213542/，无迁移，回退只恢复backup/zerobot.exe并保留新增数据。

实际Chrome用已有填充登录，核对版本与空窗口；带首尾空格标识输入、Enter不提交、手动刷新保留原文及linked关联通过。Escape原生清除后关联仍为linked；重置查询为空、六个选项all/all/all/all/all/original，最终截图已保存。实际完整下载独立校验passed=true、issues=[]、requestCount=0，未知保持null；核对副本哈希后准确清理下载，最小证据留本机。

93项回归、JavaScript语法、四宽度明暗8份截图及195份文档链接/锚点和差异检查通过。C09通过，实际窗口尚无请求；C10真实非空、表格与报告及QQ收件登记第2项，当前2/5，继续满5项统一配合。C11待验收后先#55再本批合入。合成关联样例不冒充真实有效关联命中，本轮未发送QQ或调用付费模型，历史#46～#54不重复计数。

上传结果：[草稿PR #56](https://github.com/qiye-yuri/ZeroBot-Plugin/pull/56)，head codex/task-identity-filter，base codex/task-usage-table。源码与公开说明/交付记录已上传；C10/C11继续待满5项集中验收和依赖顺序合入，当前2/5。
