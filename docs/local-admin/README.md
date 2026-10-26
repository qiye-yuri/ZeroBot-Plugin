# 本地机器人管理后台：使用与开发文档

上一批代码已审查并合并至master：[PR #37](https://github.com/qiye-yuri/ZeroBot-Plugin/pull/37)。使用方式与既有验收结果保持，合并记录见[GitHub交付](github-delivery.md)。

当前部署状态（2026-10-26）：资源只读检查、AI费用风险提示、分组用量、页面阈值和JSON报告导出已安装到日常实例。后台访问保护、静态资源一致性、单实例及QQ连接检查通过。真实Chrome后台扫描、异常筛选、资源报告下载、空用量显示、页面阈值应用/恢复及用量报告下载通过；QQ服务列表已由用户确认通过。非空用量已用用户空闲时段的真实请求核对通过，本轮未新增模型调用。下文标注“未部署”的日期化记录为当时状态。

管理本机ZeroBot-Plugin及已有OneBot连接。首次使用从[使用指南](user-guide.md)开始；日常配置、排障和历史验收按下列入口查找。

## 当前版本与计划

后续开发见[PR37后详细计划](post-pr37-development-plan.md)：逐请求风险原因定位，包含5个工作包、10项验收标准及简洁实现约束。当前仅规划，尚未实现。

状态核对日期：2026-10-26（Asia/Shanghai）。资源维护与AI用量排查已通过PR #35合并并完成此前日常部署验收；当前批次已部署，用户已确认本批验收通过，见[本批交付状态](#本批交付状态)；已通过[PR #36](https://github.com/qiye-yuri/ZeroBot-Plugin/pull/36)合并至master。此前R19 包含更新监控运行分项展示、两页草稿摘要与本地撤销、更新监控独立状态刷新、FTB独立状态刷新及此前群聊下载、路径修复；本批部署后页面与服务检查通过，FTB真实查询、下载和群文件接口/列表核对保持此前结论，见[FTB工具与验证范围](ctb.md)及[运行维护](operations.md)。R1～R13及N1～N3已按各批记录合并部署，R13会话插件范围入口见[使用方法](conversation-check.md#设置会话对应的插件范围r13)与[部署验收](self-test.md#r13-合并部署验收)。开发批次编号不是仓库发布版本。

R14文档导航整理已完成，上传及合并状态见[交付记录](github-delivery.md#r14-文档导航整理交付)；本批仅改文档，无程序部署。[R13与R14计划](post-r12-development-plan.md)保留已完成工作包；[R15文档校验工具](post-r14-development-plan.md)与专项测试已实现并合并，使用见[文档链接检查](document-check.md)，上传与合并状态见[交付记录](github-delivery.md#r15-文档校验工具交付)。

[R16 FTB任务状态独立刷新](post-r15-development-plan.md)，已合并部署，使用见[独立刷新任务状态](ctb.md#独立刷新任务状态r16)；上传与合并状态见[交付记录](github-delivery.md#r16-任务状态刷新交付)。

[R17更新监控状态独立刷新](post-r16-development-plan.md)，已合并部署，使用见[独立刷新运行状态](updatewatch.md#独立刷新运行状态r17)，交付见[R17记录](github-delivery.md#r17-运行状态刷新交付)。

[R18两页草稿提示与本地撤销](post-r17-development-plan.md)已合并部署；使用见[FTB草稿](ctb.md#草稿摘要与本地撤销r18)与[更新监控草稿](updatewatch.md#草稿摘要与本地撤销r18)。

[R19更新监控运行状态分项展示](post-r18-development-plan.md)已合并部署；使用见[运行明细](updatewatch.md#运行状态分项展示r19)。

待验收与暂缓统一查阅[当前剩余验收条件](remaining-acceptance.md)：已确认通过的项目不重新列为待验收，钓鱼、真实好友申请与入群邀请等仍按原条件保留；P010本地AI画图及定时整合继续暂缓。插件去留以[现行清单](feature-plugin-inventory.md)为准，历史数量不作为当前状态。

## 按操作查找

### 安装与运行维护

| 要做的事 | 使用说明 |
| --- | --- |
| 安装、登录和了解后台 | [使用指南](user-guide.md)、[Windows安装与构建](windows-setup.md)、[OneBot连接](onebot-windows.md) |
| 启停服务、升级或回退 | [运行维护](operations.md)、[运行时更新](runtime-security.md) |
| 备份和恢复配置 | [配置备份](backup-restore-2026-10-02.md) |
| 核对日志、模型用量和回复 | [联合观察](combined-observation.md)、[模型请求统计](model-request-statistics.md)、[用量与缓存](model-cache-plan.md)、[日志说明](log-summary.md) |
| 管理开发更新与设备提醒 | [更新监控](updatewatch.md)、[电量提醒](battery-hub.md) |
| 搜索、查询和下载FTB整合包 | [FTB工具](ctb.md) |

### AI配置与诊断

| 要做的事 | 使用说明 |
| --- | --- |
| 核对当前会话为什么响应或不响应 | [会话配置核对](conversation-check.md)、[响应与插件规则](response-status.md)、[AI角色权限](ai-role-permissions.md) |
| 配置模型和各入口参数 | [命名模型](named-models.md)、[模型独立参数](r5-model-parameters.md)、[GPT与群总结入口](r6-model-routes.md) |
| 检查字段和手动测试已保存配置 | [字段检查](ai-config-check.md)、[模型诊断](model-probe.md)、[搜索密钥诊断](search-probe.md) |
| 设置联网搜索和会话角色 | [联网搜索](web-search.md) |
| 设置会话人物、主动接话或引用回复 | [会话人物](conversation-persona.md)、[AI中心与群互动](ai-center.md)、[GPT引用](gpt-quoted-message.md) |
| 使用图片理解和语音 | [Agent识图](agent-vision.md)、[识图配置](deepseek-vision.md)、[群语音](qq-ai-voice.md) |

### 会话记忆与任务

| 要做的事 | 使用说明 |
| --- | --- |
| 查档案、搜索历史并核对来源 | [会话档案](agent-archive.md) |
| 查找、新增、修订或恢复记忆 | [检索与记忆管理](agent-memory.md) |
| 查看任务进度、结果或停止任务 | [任务控制](agent-tasks.md) |
| 了解对话留存和重置边界 | [档案与容量](agent-history-2026-10-03.md) |

### 插件操作

| 要做的事 | 使用说明 |
| --- | --- |
| 按会话进入插件设置 | [范围快捷入口](conversation-check.md#设置会话对应的插件范围r13)、[逐项保存草稿](plugin-save-all.md)、[导出用法](plugin-usage-export.md) |
| 管理群、违禁词和申请事件 | [群管](group-management.md)、[违禁词](antiabuse.md)、[好友申请与群邀请](request-events.md) |
| 使用B站和MC功能 | [B站](bilibili.md)、[MC服务器监控](minecraft-monitor.md) |
| 使用游戏、塔罗及角色库 | [钓鱼与配对](games-testing.md)、[猜词](word-games.md)、[塔罗与素材](media-plugins.md)、[角色库](role-library.md) |
| 签到、自检或核对消息显示 | [签到与钱包](signin-wallet.md)、[自检与打断复读](status-and-repeat.md)、[发送者显示](message-senders.md) |

## 开发与验收

| 查阅内容 | 入口 |
| --- | --- |
| 当前工作包与开发约定 | [下一批任务风险原因定位计划](post-pr37-development-plan.md)、[本批交付状态](#本批交付状态)、[仓库约定](../../AGENTS.md)、[需求](requirements.md) |
| 自动化、隔离整机和真实验收范围 | [自测记录](self-test.md)、[统一验证方法](agent-integration-test.md)、[真实Agent验收](real-agent-acceptance.md)、[真实模型测试](real-model-test.md) |
| 插件测试条件与已有结果 | [测试方案](feature-test-plan.md)、[结果清单](feature-test-results.md)、[剩余验收条件](remaining-acceptance.md) |
| 文档链接、锚点与检查命令 | [文档链接检查](document-check.md) |
| 提交文件边界及上传流程 | [GitHub交付](github-delivery.md) |

每次功能更新同步使用说明。自动化通过、真实QQ验收、上传、合并和部署分别记录；缺少外部条件不等于无法修复。

## 历史记录

- [原首页归档与记录去向](index-history.md)：保留R14整理前的公开记录、旧入口和阶段数量。
- [开发进度](progress.md)、[交付沿革](github-delivery.md)、[自测记录](self-test.md)：查某批次当时的实现、上传及验收情况。
- [第一版说明](release-v0.1.md)、[AI与Agent交付](release-ai-agent.md)、[M1～M3设计](ai-agent-development.md)：已完成阶段的范围。
- [R18草稿摘要与本地撤销计划](post-r17-development-plan.md)：已完成设计与部署记录。
- [R17更新监控状态刷新计划](post-r16-development-plan.md)：已完成设计与部署记录。
- [R16 FTB状态刷新计划](post-r15-development-plan.md)：已完成阶段的设计与部署记录。
- [R15文档校验计划](post-r14-development-plan.md)：已完成阶段的设计与合并记录。
- [R13与R14计划](post-r12-development-plan.md)：已完成阶段的工作包与验收契约。
- [早期开发计划](next-development-plan.md)、[R1～R4计划](next-stage-development.md)、[R7计划](post-search-development-plan.md)、[R8～R10计划](post-r7-development-plan.md)、[R11～R12计划](post-r10-development-plan.md)：原设计契约与后续完成记录。
- [插件删除沿革](removed-plugins.md)、[问题判定记录](unresolved-features.md)、[上游评估](upstream-review.md)：历史故障与去留依据，不替代当前可用功能清单。

历史记录中的“当前”“待开发”和“未部署”按对应批次理解，不覆盖后来的验收。公开文档只保留使用、开发和验证摘要；凭据、身份、运行数据及私有证据不上传。

资源维护新增本地只读检查，使用见[资源只读检查](resource-maintenance.md)，已部署验收通过。

[AI费用风险提示](model-request-statistics.md#ai费用风险提示2026-10-26)已完成本地实现与测试，复用用量统计，不改变历史预算；已部署验收通过。

资源维护与AI用量提示已完成[第一批本地交付整理](resource-maintenance.md#第一批本地交付范围2026-10-26)，统一自测支持[只读维护专项](agent-integration-test.md#只读维护专项入口2026-10-26)。专项通过不等同完整整机验收；本批已完成日常实机验收，详见对应记录。


任务用量分组已部署并通过用户验收；入口与边界见[按任务查看用量](model-request-statistics.md#按任务查看用量2026-10-26本地实现)。复用近期统计，缺少关联标识的请求独立显示，不改变历史预算。


已导出的用量JSON可用[独立校验工具](model-request-statistics.md#用量报告独立校验2026-10-26本地工具)离线复算，无需调用模型或启动机器人。工具已完成；后台任务分组及资源处理指引已部署并通过用户验收。

## 本批交付状态

2026-10-26：以下四项已完成并于17:26部署。资源指引和任务空窗口/报告已在日常Chrome核对通过；服务列表与非空任务用量已由用户确认验收通过。已通过[PR #36](https://github.com/qiye-yuri/ZeroBot-Plugin/pull/36)合并至master，详情见[部署记录](operations.md#资源指引与任务用量部署验收2026-10-26)。

| 工作项 | 现有功能复用与验证范围 |
| --- | --- |
| [资源异常处理指引](resource-maintenance.md#资源异常处理指引2026-10-26本地实现) | 基于已有只读扫描原因显示建议，复用素材维护脚本说明；页面不执行修复或下载。 |
| [按任务查看用量](model-request-statistics.md#按任务查看用量2026-10-26本地实现) | 使用已有最多256条请求快照与导出入口，按会话和任务隔离；不新增模型调用或持久化。 |
| [独立报告校验](model-request-statistics.md#用量报告独立校验2026-10-26本地工具) | 离线复算分组、缓存和输入排名，兼容旧报告；验证数字一致性，不认证完整账单或业务成功。 |
| 文档与集成检查 | 当前入口和历史记录分开；浏览器实际下载接入独立校验，覆盖多轮多模型、跨会话隔离、重启与空窗口、无效数据清理，并检查修改统计后的报告被拒绝。 |

历史预算保持900000；其他待验收和暂缓项仍按[剩余验收条件](remaining-acceptance.md)执行。本批验收已由用户确认通过；本次只更新记录，不新增付费调用。代码已通过[PR #36](https://github.com/qiye-yuri/ZeroBot-Plugin/pull/36)合并至master。

本批完成[用量问题定位](post-pr36-development-plan.md)：任务筛选/排序和现有校验结果处理说明。已部署并通过空窗口检查，本批用户验收通过；已通过[PR #37](https://github.com/qiye-yuri/ZeroBot-Plugin/pull/37)合并到master，不改变历史预算和其他待验收、暂缓状态。

新增功能的使用步骤见[任务风险筛选与排序](model-request-statistics.md#任务风险筛选与排序)及[校验结果中文说明](model-request-statistics.md#校验结果中文说明本地工具)，当前日常实例已安装本批功能。
