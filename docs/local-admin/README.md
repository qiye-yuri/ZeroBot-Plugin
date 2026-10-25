# 本地机器人管理后台：使用与开发文档

管理本机ZeroBot-Plugin及已有OneBot连接。首次使用从[使用指南](user-guide.md)开始；日常配置、排障和历史验收按下列入口查找。

## 当前版本与计划

状态核对日期：2026-10-25（Asia/Shanghai）。最新部署为R16 PR #28合并后的master `d36bee1d`，包含FTB独立状态刷新及此前群聊下载、路径修复；本批部署后页面与服务检查通过，FTB真实查询、下载和群文件接口/列表核对保持此前结论，见[FTB工具与验证范围](ctb.md)及[运行维护](operations.md)。R1～R13及N1～N3已按各批记录合并部署，R13会话插件范围入口见[使用方法](conversation-check.md#设置会话对应的插件范围r13)与[部署验收](self-test.md#r13-合并部署验收)。开发批次编号不是仓库发布版本。

R14文档导航整理已完成，上传及合并状态见[交付记录](github-delivery.md#r14-文档导航整理交付)；本批仅改文档，无程序部署。[R13与R14计划](post-r12-development-plan.md)保留已完成工作包；[R15文档校验工具](post-r14-development-plan.md)与专项测试已实现并合并，使用见[文档链接检查](document-check.md)，上传与合并状态见[交付记录](github-delivery.md#r15-文档校验工具交付)。

[R16 FTB任务状态独立刷新](post-r15-development-plan.md)，已合并部署，使用见[独立刷新任务状态](ctb.md#独立刷新任务状态r16)；上传与合并状态见[交付记录](github-delivery.md#r16-任务状态刷新交付)。

下一阶段为[R17更新监控状态独立刷新](post-r16-development-plan.md)，目前仅完成规划，尚未实现。

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
| 当前工作包与开发约定 | [R17开发计划](post-r16-development-plan.md)、[仓库约定](../../AGENTS.md)、[需求](requirements.md) |
| 自动化、隔离整机和真实验收范围 | [自测记录](self-test.md)、[统一验证方法](agent-integration-test.md)、[真实Agent验收](real-agent-acceptance.md)、[真实模型测试](real-model-test.md) |
| 插件测试条件与已有结果 | [测试方案](feature-test-plan.md)、[结果清单](feature-test-results.md)、[剩余验收条件](remaining-acceptance.md) |
| 文档链接、锚点与检查命令 | [文档链接检查](document-check.md) |
| 提交文件边界及上传流程 | [GitHub交付](github-delivery.md) |

每次功能更新同步使用说明。自动化通过、真实QQ验收、上传、合并和部署分别记录；缺少外部条件不等于无法修复。

## 历史记录

- [原首页归档与记录去向](index-history.md)：保留R14整理前的公开记录、旧入口和阶段数量。
- [开发进度](progress.md)、[交付沿革](github-delivery.md)、[自测记录](self-test.md)：查某批次当时的实现、上传及验收情况。
- [第一版说明](release-v0.1.md)、[AI与Agent交付](release-ai-agent.md)、[M1～M3设计](ai-agent-development.md)：已完成阶段的范围。
- [R16 FTB状态刷新计划](post-r15-development-plan.md)：已完成阶段的设计与部署记录。
- [R15文档校验计划](post-r14-development-plan.md)：已完成阶段的设计与合并记录。
- [R13与R14计划](post-r12-development-plan.md)：已完成阶段的工作包与验收契约。
- [早期开发计划](next-development-plan.md)、[R1～R4计划](next-stage-development.md)、[R7计划](post-search-development-plan.md)、[R8～R10计划](post-r7-development-plan.md)、[R11～R12计划](post-r10-development-plan.md)：原设计契约与后续完成记录。
- [插件删除沿革](removed-plugins.md)、[问题判定记录](unresolved-features.md)、[上游评估](upstream-review.md)：历史故障与去留依据，不替代当前可用功能清单。

历史记录中的“当前”“待开发”和“未部署”按对应批次理解，不覆盖后来的验收。公开文档只保留使用、开发和验证摘要；凭据、身份、运行数据及私有证据不上传。
