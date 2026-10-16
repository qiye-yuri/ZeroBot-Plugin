# 第一版验证摘要与复验入口

## N2 命名模型配置验收

2026-10-16 15:05 +08:00：`pwsh -File scripts/test-agent.ps1 -OutputName zerobot-n2-final.exe` 的 11 步全部退出 0。SDK、chat、后台/AI/统计与脚本 Go 回归、四项 JavaScript 语法、132 项 Chrome、最终构建及完整隔离整机通过。整机 147 项 `*Passed` 检查及总判定成功，`aiNamedModelsSelectionAndRequestsPassed=true`，N1 人物覆盖检查仍通过。构建 SHA256：`8FD903DF4DEF2A340BFB233DA09B75803628D0D5F5B981533D7353D72631EDC1`。

后台保存三个独立地址、模型名与密钥的配置后，GPT、普通聊天、识图及 Agent 共四次新增 HTTP 均准确匹配选中的配置，识图结果实际进入 Agent 请求，统计全部结算。原 53 次回归请求与新请求分开计数，原故障及零重放断言保持。OpenAI、OLLaMA、GenAI 的实际协议请求由 Go 模拟服务验证；迁移预览只读、完整组合去重、写前备份失败不写库、恢复原控制数据库、修订冲突、密钥操作/脱敏、引用删除与数据库重开通过。页面新增、编辑、删除、草稿整体撤销、旧字段停用、迟到响应及 320px 深浅主题通过。

首次整机汇总失败源于新测试复用了取消测试会话，保留档案中的图片被后续正常新任务处理，额外产生图片下载；补全诊断后改用独立会话，保留原计数和停止断言，完成以上最终复验。失败记录、截图与完整日志留本机 `.local/`；9 份使用/开发文档的 396 个本地引用及差异检查通过。未部署日常服务，未调用真实模型或发送真实 QQ 消息。使用与回退见[命名模型配置](named-models.md)及[维护说明](operations.md)。

## N1 会话人物覆盖验收

2026-10-16 14:32 +08:00：`pwsh -File scripts/test-agent.ps1 -OutputName zerobot-n1-final.exe` 的 11 步全部退出 0，包括 SDK、chat、后台/AI/统计、脚本判定、四项 JavaScript 语法、128 项 Chrome、最终构建及完整隔离整机。整机 146 项 `*Passed` 检查及总判定通过，新增 `aiConversationPersonaModelAndHTTPPassed=true`；实际后台保存的自定义人物进入五次角色模型请求，同号私聊保持继承，随后恢复群继承，原动作权限测试保持。

新增 Go 覆盖机器人/群私聊隔离、全局继承、恢复、写前备份、修订冲突、数据库重开与多轮人物快照；页面覆盖保存目标绑定、草稿撤销、冲突保留和退出后的迟到响应。人物保存不增加 Agent 代次，任务使用固定快照。构建 SHA256：`AE3A162481602EA0C7FC6A4C179A31B87365B6D1E01167F714BF26349FBCE801`。

首次沙箱 ACL、Windows 嵌入文本换行及目录联接导致的环境失败保留本机；统一入口现复用已有实际路径解析，嵌入说明固定 LF 后完成上述复验。日志与详细证据留 `.local/`。本轮不替换日常服务，不调用真实模型、不发送真实 QQ 消息；人物表现仍由实际使用体验判断。使用见[会话人物覆盖](conversation-persona.md)。

## PR #2 合并复核

2026-10-16 11:34 +08:00：复核 PR #2 的提交 0b9a05fc，重点检查后台认证及当前机器人/会话隔离、记忆归属迁移和修订事务、任务停止及重启行为、动作权限、输出和图片容量、HTTP 响应完整读取及页面纯文本呈现，未发现阻断合并的问题。141 个文件与已验证清单一致，103 篇文档的 813 个本地链接及暂存内容核对通过；代码未改变，沿用同一源码的下节完整验证，不重复发真实 QQ 消息或请求模型。

GitHub 状态为可合并且无待处理审查意见，未提供 CI 检查项，不将空检查列表记作 CI 通过。按使用者要求锁定已检查的提交合并，PR 状态已为 MERGED，合并提交 [31716852](https://github.com/qiye-yuri/ZeroBot-Plugin/commit/31716852fab99978218367f957aa7bb2ede3e261)。交付文档另以文档提交同步至 master；日常程序、服务和配置不变，本机 data 子模块工作区改动保留、不上传。结果及链接见[交付记录](github-delivery.md)。

## AI 与 Agent 上传前独立源码核验

2026-10-16 10:52 +08:00：按显式文件清单生成独立源码副本，未复制日常配置、凭据或聊天数据库，仅补齐公开资源夹具。SDK、chat、后台/AI/统计及整机脚本 Go 回归、三个脚本语法、119 项 Chrome 回归、最终构建和完整隔离整机的统一 10 步均退出 0；145 项整机分项检查和总判定通过，isolated=true、realQQMessagesSent=false。相关 Go vet、105 个变更 Go 文件的格式检查通过。

首次尝试的前八步通过，但新副本缺少被忽略的生成注册文件，构建失败；保留原始失败报告。随后按标准构建要求执行 go generate main.go，再完整运行统一入口通过，复验文档已补齐首次检出的准备步骤，不上传本机生成文件。排除 VCS 元数据后，独立副本与工作区对照构建 SHA-256 均为 D51FD5C09E63E96A91880E950B8F17BD189CE1FCF7DC0F2D859287DE7677A26E；日常程序不替换、不重启。

本机证据 `.local/ai-agent-publication/`：`source/.local/agent-validation/run-20261016-104544-bd5d6c32/result.json` passed=true，整机 `source/.local/smoke-admin/run-3476652092/result.json` passed=true；首次失败为 run-20261016-104141-b1ac531b。拟上传文件、暂存内容、公开文档链接、真实身份/实际密钥及五个本地依赖许可证均核对。data 子模块的本机改动、运行资料与对话原始记录保留本机，不随源码上传。

## 真实 QQ 收发与 Agent 复验

2026-10-16 10:28 +08:00：本轮主人私聊真实 QQ 基本流程验收通过，包括严格 GPT 回显、历史搜索/精确读取、旧文件显式迁移及重启核对，以及迁移后真实 Agent 保存、搜索、精确读取和发送回复。日常运行的 EOF 修复程序保持，未改动日常权限或配置；不扩展为其他角色或全部功能的真人验收。

机器人已实际向唯一配置的主人私聊发送一条专用验收提示，QQ 返回消息编号；`get_msg` 核对消息类型、机器人发送者及正文一致，`get_friend_msg_history` 找到同一消息。出站及平台回读通过，不能据此推定用户客户端已看到。

已记录新一轮私聊档案/任务起点并使用只读核对脚本，仅保存本轮唯一标记对应的入站、动作、记忆、任务和 QQ 回执。本轮合计五条真实入站及五条对应回复，包含首轮三条、严格 GPT 重发和迁移后记忆重发，QQ 私聊历史及本机档案均能核对。三项 Agent 任务均 completed、fallbackPreparations=0；历史任务 searchHistory=1、readHistory=1，平台发送成功且实际回复包含档案中的测试代号。

首次记忆任务实际执行 save_memory 和 search_memory，旧文件保存成功，但私聊当时仍是 legacy 模式：两条有效记忆、140 字节。搜索未找到可独立寻址的托管条目，因而没有执行 read_memory；模型如实说明没有可读条目。此为尚未迁移的运行条件，不作为新版完整记忆检索通过。

使用者明确授权协助迁移后，10:18 停止 ZeroBot，备份并逐文件核对 175 份运行文件，调用程序现有 Preview/Migrate 逻辑导入两条记忆。原文件保留、迁移备份摘要一致、归属匹配，实际搜索命中测试条目，按编号和修订号精确读取通过。仅重启 ZeroBot，OneBot 原进程和登录保持；程序、启动配置、后台密码、插件控制及其他核对文件摘要保持。重启后只读核对托管模式、两条 legacy 来源/零来源序号记录及两份初始修订，数据库 quick_check=ok。未将这些存储调用记为真实 QQ Agent 执行。

10:28 主人真实重发记忆指令后，Agent 实际依次执行 save_memory、search_memory、read_memory、send_private_msg 并结束任务。保存回执 mode=managed、created=false，沿用导入条目的编号、修订及 legacy/零序号来源；搜索命中后，精确读取请求使用同一编号和修订，返回完整测试代号。两次查询成功且无空结果、拒绝或错误；三次本地动作和一次平台动作均成功，无普通聊天兜底。QQ 成功发送回执与 get_msg 回读匹配，实际回复包含“蓝色海星”。复验后仍为两条记忆、两份修订，原件和备份未变，去重及来源保留通过。

首条 GPT 消息附加了测试代号说明，回复不能作为严格回显依据；主人单独重发精确指令后，机器人实际仅回复要求的唯一标记，QQ 历史与 get_msg 均已核对。未修改模型提示词或伪造固定答案。真人消息由主人发送，没有注入模拟入站事件。

本机证据 `.local/qq-live/run-20261016-095602/`：`session.json` 为起点，`outbound.json` 出站 passed=true，`observation.json` 为任务/动作记录，`qq-readback.json` 为真实平台回读，`memory-receipts.json` 为最新记忆操作回执，`legacy-status.json` 为旧文件保存证据，迁移及重启的三份报告均 passed=true。10:31 最终 `result.json` completed=true、passed=true，当前进程身份、单实例、程序与配置摘要、QQ 在线及后台就绪均通过。真人输入来自主人，未注入假入站。本轮未发现产品代码故障，未修改或替换日常程序；登录后台页面、其他角色及持续负载不计入本轮验收。步骤及判定见[真实 QQ 验证](real-agent-acceptance.md)。

## 聊天响应完整读取修复

2026-10-16 09:44 +08:00：修复 GPT 与普通聊天在成功解码非流式 JSON 后未读到 EOF、将未知长度 HTTP 响应提前关闭的问题。新增共享 `internal/modelrequest.Request`，仅在协议解析成功后将剩余正文读至 EOF，再让原 API 返回结果；GPT/群总结、普通聊天及 Agent 普通聊天兜底接入。读取失败或超时直接报错，解析失败不继续读取。没有改动通用统计口径、自动重试、响应缓存或权限逻辑。

- Go 回归通过：modelrequest 0.257 秒、modelstats 0.136 秒、llm 1.405 秒、aichat 1.490 秒。新增用例覆盖 GPT/Chat 的真实 chunked JSON（小/大正文）、成功解析后仍未结束传输的超时、正文尾部读取失败及解析失败立即结束，核对回复、错误和最终计数；见[回归测试](../../internal/modelrequest/request_test.go)。
- 新构建 `.local/bin/zerobot-chat-completion.exe` 为 44,448,256 字节，SHA-256 为 7BCAC3E9D7B0E4FE3F0395D002BE133F6A2B504D808EBF567BC3E6F2E028F945。
- 七组真实模型用例全部通过，09:44:26 完成；本轮 17 次实际请求全部 completed，incomplete=0、failed=0、active=0。GPT=1、Chat=1、Agent=14、Vision=1；GPT 与普通聊天均从旧构建的 incomplete 变为 completed。五项 Agent 用例仍无普通聊天兜底。证据 `.local/real-model/run-3860402463/result.json`，passed=true、httpResponsesCompleted=true。
- 真实测试入口同步要求功能断言及 HTTP 完整读取均通过；未完成响应不能再仅因正确回复而汇总通过。只提交合成资料到已配置真实接口，OneBot 为独立模拟实例，未发送真实 QQ 消息。

统一入口 `scripts/test-agent.ps1` 的后台步骤已加入 llm/modelrequest 回归，脚本语法通过；组合复验全部通过：localadmin 17.219 秒、aichat 1.805 秒、llm 1.475 秒、modelrequest 0.283 秒、modelstats 0.207 秒、replystats 0.305 秒。相关 Go vet、格式及文档链接检查通过。

完整隔离整机于 09:48:33 通过，146 项 Passed 检查均为 true，证据 `.local/smoke-admin/run-2279690901/result.json`；使用同一构建、合成模型及 OneBot，不发送真实 QQ 消息。

09:48:53 已部署到日常 `.local/bin/zerobot.exe`：停机备份原程序及配置/聊天/记忆等 184 个文件，保留 OneBot 原进程与登录，仅重启 ZeroBot。七份启动配置、后台凭据/选项、插件/语音数据库及 OneBot 配置哈希保持；程序哈希与上述真实验收构建一致，后台 HTTP 200、仅回环监听。09:49 收尾核对进程身份、单实例、实际 OneBot 已建立连接、预期账号握手、所有备份哈希通过，无启动错误。证据 `.local/chat-completion-deployment/run-20261016-094844/result.json` 及同目录 `final-check.json`，均 passed=true。

此前真实模型记录中的两次 incomplete 保留原值，不回填历史报告。使用与生效范围见[统计说明](model-request-statistics.md#统计口径)，复验见[真实模型入口](real-model-test.md)。本次只验证真实模型接口及合成平台动作，不代替真实 QQ 客户端验收。

## 真实模型链路验证

2026-10-16 09:32 +08:00：使用日常已部署程序及本机配置的真实 `deepseek-flash` 接口，七组功能用例全部通过。聊天、Agent、识图分别使用各自已保存的配置；测试实例使用合成主人私聊、测试提示词和独立数据，只向真实模型提交合成内容，不发送真实 QQ 消息。入口及范围见[真实模型测试](real-model-test.md)。

| 用例 | 结果与依据 |
| --- | --- |
| GPT | 1.946 秒，真实回复包含本次随机字符串 |
| 普通聊天 | 1.201 秒，关闭 Agent 后真实回复正确 |
| Agent 动作 | 2.743 秒，发送动作及任务完成，兜底次数为 0 |
| 保存记忆 | 3.728 秒，实际保存及回复均核对，任务完成 |
| 历史搜索/精确读取 | 3.802 秒，searchHistory=1、readHistory=1，返回正确档案代号 |
| 记忆搜索/精确读取 | 4.210 秒，默认仅装载 100 条、record_limit；更早目标经 searchMemory=1、readMemory=1 读取，返回完整正文末尾代号 |
| 识图 | 4.689 秒，真实识图请求后回复合成图片主要颜色为红色，任务完成且无普通聊天兜底 |

主验收共 16 次真实请求：Agent 13 次、识图 1 次、GPT 及普通聊天各 1 次。HTTP 统计 completed=14、incomplete=2、failed=0、active=0；两次聊天请求虽返回正确功能结果，正文完整读取统计为 incomplete，不记为 16 次 HTTP 全部完成。五项 Agent 用例全部完成且 fallbackPreparations=0。

首轮测试曾使用普通用户私聊身份，不具备私聊发送动作权限；另一个合成记忆超过既有 4000 字节限制。首轮 passed=false 保留，不作为通过结果；修正测试身份/样本，并排除聊天兜底后复验通过，没有修改日常角色或放宽程序权限。测试宿主增加按上下文等待与合成图片消息入口，原三秒无回复行为保持；宿主 Go 回归通过。未修改或重新部署产品程序。

主验收本机证据 `.local/real-model/run-2795029520/result.json`，程序 SHA-256 为 FB5F7C1FF2537203EAF1526FF7BCBCFDFED02FB3D969C6B42C344D7154695DE4；测试实例已停止。首轮证据 `.local/real-model/run-190975467/result.json`。

09:34 补测独立选择 history 用例通过（3.522 秒、三次真实请求），证据 `.local/real-model/run-3892541323/result.json`；验证无需先运行其他用例即可初始化合成存储。修订后预检不发请求，宿主 Go 回归 0.148 秒、既有隔离重启四次启动回归及 diff 格式检查通过，重启证据 `.local/admin-system/restart-2222516788/result.json`。包括首轮探索在内，三次实际模型测试记录合计 32 次请求（13+16+3）。

09:37 日常收尾核对通过：ZeroBot 和 OneBot 原进程保持、实际程序哈希及七份配置/数据库哈希保持、账号在线、仅日常机器人实例存活；证据 `.local/real-model/daily-after.json`。日常服务的实际 QQ 客户端体验、登录后后台流程及完整持续负载仍未验收；本次真实接口验证不替代这些范围。下方部署记录中的“未测试真实模型”描述 09:18 时的状态。

## AI 与 Agent 整机部署验证

2026-10-16 09:18 +08:00：先核对 M1～M3 的统一验收报告、10 步退出码及最终程序 SHA-256，确认功能与自动化验收完成后，09:11 将该程序部署为日常 `.local/bin/zerobot.exe`。最终部署及测试汇总 passed=true，源码未另作修改。

- 部署前仅停止 ZeroBot，备份旧程序、配置、后台凭据、OneBot 配置、插件/语音配置与完整聊天/记忆目录，共 184 个文件。保留原停机哈希清单；SQLite 只读检查更新了一份 SHM 的读锁协调区，单独记录复核哈希，数据库、WAL 及其余备份文件保持原哈希。原旧记忆及归属文件未改动、未迁移。
- 日常程序 SHA-256 与最终验收构建一致：FB5F7C1FF2537203EAF1526FF7BCBCFDFED02FB3D969C6B42C344D7154695DE4。计划任务宿主启动的新进程身份、单实例及预期账号握手通过；新进程到本机 OneBot 的 WebSocket 为 Established。OneBot 原进程及登录保持，三个只读 API 通过。
- 启动配置、后台密码/选项、插件开关数据库、语音配置及 OneBot 配置哈希保持。后台只监听 127.0.0.1:3010，HTTP 200；日常和备份中共六个 SQLite 数据库以只读方式执行 quick_check，均为 ok，未读取聊天正文。
- 实际日常后台的 17 个资源与当前源码逐字节核对一致；21 个未登录管理读写接口返回 401，跨来源请求返回 403。Chrome 桌面/手机登录页、刷新、窄屏布局通过，无脚本异常或失败资源。后台加载原凭据且无需首次设置；本次没有后台密码，未自动验证日常登录后的页面流程，未重设密码。
- 使用已部署的同一程序重新运行完整隔离整机，146 项 Passed 检查全部为 true，passed=true、isolated=true、realQQMessagesSent=false，09:16:49 完成。覆盖角色、上下文、历史/记忆查询、版本与来源、任务控制及现有后台功能；独立运行目录只连接回环假模型和假 OneBot。
- 独立重启测试启动程序四次，09:12:40 完成，passed=true；覆盖配置保存/恢复、后台开关/地址恢复、插件响应/禁用/屏蔽恢复、凭据保留及旧会话拒绝，未改变日常配置。
- 日常只读观察 180.03 秒、37 个样本，进程与程序稳定、日志覆盖完整、无新增错误；平均 CPU 0.0013%，私有内存增长 2.15 MiB，采样及资源阈值通过。观察脚本要求至少 900 秒才置 fullTiming=true，因此原报告 fullTiming=false、passed=false；这里只记录短时观察合格，不记为完整持续性能验收。

本机汇总：`.local/agent-deployment/run-20261016-091107/final-result.json`；部署、备份与页面证据位于同目录。整机 `.local/smoke-admin/run-1070324066/result.json`，重启 `.local/admin-system/restart-2609925445/result.json`，短时观察 `.local/live-observation/run-20261016-091212-d398de81/result.json`。这些含运行信息的证据不公开上传。

本次完成日常部署、实际连接/页面核验及隔离功能整机复验；真实模型请求、真实 QQ 消息收发、本阶段真人体验和真实旧记忆迁移未执行，不能据模拟通过推定其效果。使用和回退见[运行维护](operations.md#已验收程序部署与回退)。

## AI 与 Agent 本阶段统一终验

2026-10-16：M1 会话档案、M2 检索与记忆管理、M3 任务控制（AGA-01～07）开发与自动化验收完成。统一入口 `scripts/test-agent.ps1` 的 10 步全部退出 0，最终汇总 passed=true。

- 本轮统一运行：SDK 0.630s、chat 7.646s、后台 17.129s、AI 插件 1.600s、模型统计 0.134s、回复统计 0.265s、整机脚本回归 0.107s 通过；Chrome 119 项全部通过（79.762s）。统一脚本从仓库之外的工作目录启动，仍使用自身仓库路径。
- 新补充 `TestRetrievedHistoryCannotGrantActionPermissions` 定向 5.480s、追加 chat 全量 11.208s 通过：旧恶意事件不在近期上下文，真实检索后进入第二次模拟模型请求；普通用户角色仍拒绝禁言动作，同批保存前缀也不派发，实际仅执行一次本地查询、无平台动作。不新增运行时保护层。
- 使用退出码 37 的本地假 Go 命令验证统一入口失败路径：只执行首步，汇总 passed=false，不进入构建/整机，恢复 CGO_ENABLED 和工作目录。失败探针本机证据 `.local/agent-validation/run-20261016-022500-f5c3b199/result.json`，不作为成功终验结果。
- 最终构建 `.local/bin/zerobot-agent-complete.exe` 为 44,442,624 字节，SHA-256 为 FB5F7C1FF2537203EAF1526FF7BCBCFDFED02FB3D969C6B42C344D7154695DE4，已与统一汇总核对一致。完整隔离整机于 2026-10-16 02:25:41 +08:00 退出 0，passed=true、isolated=true、realQQMessagesSent=false，全部原验证项通过。统一汇总于 02:25:42 +08:00 完成，证据 `.local/agent-validation/run-20261016-021914-7b15a917/result.json`，关联整机 `.local/smoke-admin/run-318572831/result.json`。脚本和 Go 格式、diff 空白及文档链接检查通过。

| 验收项 | 自动验证覆盖及依据 |
| --- | --- |
| AGT-01 范围隔离 | [档案](../../third_party/zbputils/chat/archive_test.go)、[记忆](../../third_party/zbputils/chat/memory_store_test.go)、[任务](../../third_party/zbputils/chat/agent_tasks_test.go)和实际整机会话隔离 |
| AGT-02 快照与新增记录 | 档案固定快照分页、期间新增记录及刷新边界，见档案测试与页面回归 |
| AGT-03 有界搜索与正文 | 十万条合成档案、扫描/返回预算、Unicode、空窗口和实际继续位置，见[档案测试](../../third_party/zbputils/chat/archive_test.go)及[继续检索](../../third_party/zbputils/chat/archive_continuation_test.go) |
| AGT-04 重置与历史权限 | 最近重置边界、固定查询范围、[恶意历史不能授权](../../third_party/zbputils/chat/agent_history_permission_test.go)及 SDK 角色权限表 |
| AGT-05 迁移与归属 | 归属冲突、重复迁移、备份、事务失败及恢复，见[记忆存储测试](../../third_party/zbputils/chat/memory_store_test.go) |
| AGT-06 修订与停用 | 并发版本冲突、完整历史、停用/旧版本不进入模型读取、恢复正文及当前版本检查，见记忆模块和页面回归 |
| AGT-07 在途停止 | 模型/平台/兜底和准备阶段停止、无后续派发、保留已返回回执，不自动重发，见任务模块及隔离整机 |
| AGT-08 旧响应与旧任务 | 页面退出/切换/迟到响应和过期版本取消，见[页面](../../scripts/test-agent-tasks-ui.mjs)及任务模块 |
| AGT-09 故障与重启 | 存储故障不伪装空结果、未落盘内存终态覆盖、重启中断及未知结果不重放，见任务和记忆模块 |
| AGT-10 文本与隐私 | 纯文本、密钥及设置命令脱敏、资源不自动访问，见档案/记忆/任务页面及后端回归 |

本阶段完成指功能与合成数据自动化验证收敛。02:25 终验时尚未部署日常服务，后续部署见[整机部署验证](#ai-与-agent-整机部署验证)。真实模型效果、QQ 客户端体验和真实旧记忆迁移未执行；此前取消的真人与持续负载验收保持关闭。使用入口见[指南](user-guide.md)，复验命令与证据结构见[统一复验](agent-integration-test.md#统一复验)。

## 任务最近动作结果筛选验证（2026-10-16）

- 存储定向 Go 1.793s、接口定向 2.115s、新增 Chrome 两项 6.436s 通过。存储测试覆盖 240 条候选的结果/时间组合跨页（60 个匹配）与仅结果分页（80 个匹配）、大整数、范围隔离、缺失/空值及其他旧值、累计结果不作为最近结果、写失败后内存覆盖旧在途及缺失行兜底、读取不改版本；接口认证、参数/混用、HEAD、no-store、组合及故障通过。页面覆盖已应用条件、草稿/分页、独立单条、停止重置、全部选项及失败/迟到清理，320px 明暗截图已查看。
- chat 全量 15.422s、后台全量 31.417s、AI 插件 1.529s、整机脚本回归 0.159s 通过；完整 Chrome 119 项通过（87.683s），最终 CGO_ENABLED=0 构建通过（44,442,624 字节），Go 格式、JS 语法、diff 空白及 101 份文档链接检查通过。最终构建完整隔离整机脚本于 2026-10-16 02:14:45 +08:00 退出 0，passed=true、isolated=true、agentTaskOutcomeFilterHTTPPassed=true、realQQMessagesSent=false，原有全部验证项通过。实际动作在途 in_flight、模型/兜底未记录结果组合查询均核对同一任务及版本；16 个故障与恢复任务的八类筛选集合、空组合和私聊隔离通过，无额外模型调用。本机证据 .local/smoke-admin/run-370105864/result.json，构建 .local/bin/zerobot-task-outcome-filter.exe。未部署日常服务，无真实 QQ 验收。见[使用说明](agent-tasks.md#按最近动作结果筛选任务)。

## 两次历史记忆修订对比验证（2026-10-16）

- 定向 Chrome 5 项通过（17.017s），含新增三项：两次精确大整数修订读取、固定已加载范围、非当前历史的 Unicode/纯文本正文及启停差异、Enter、当前记忆与列表/历史分页保持、翻页重建；非法修订、缺失、回执不符、第二次读取失败、相同正文与同一修订；离开时在第一次读取后不再发第二次请求、第二次迟到不回填。原当前版本对比两项仍通过，320px 明暗截图已查看。
- 后台全量 Go 33.124s、整机脚本回归 0.262s 通过；完整 Chrome 117 项通过（136.335s），最终 CGO_ENABLED=0 构建通过（44,432,896 字节），Go 格式、JS 语法、diff 空白及 101 份文档链接检查通过。最终构建完整隔离整机脚本于 2026-10-16 02:03:08 +08:00 退出 0，passed=true、isolated=true、agentMemoryHistoryComparisonAssetsPassed=true、realQQMessagesSent=false，原有全部验证项通过。新增标记核对实际服务提供的比较脚本资源，交互由上述 Chrome 覆盖；实际 v1、停用 v4、当前 v5 精确修订读取与各字段、会话隔离及当前版本不变仍通过，无额外模型调用。本机证据 .local/smoke-admin/run-1030643724/result.json，构建 .local/bin/zerobot-memory-history-compare.exe。未部署日常服务，无真实 QQ 验收。见[使用说明](agent-memory.md#任意两次历史修订对比)。

## 档案来源序号定位验证（2026-10-16）

- 定向 Chrome 新增三项通过（8.613s）：精确大整数及 Enter 提交，已加载范围不采用表单草稿，搜索继续及时间线分页保留；纯文本/截断、非法编号、未找到/不可用/故障区分；归档模式、读取在途禁用和离开后的迟到清理。完整页面回归另核对定位失败仍保留搜索继续位置。320px 明暗截图已查看。
- 后台全量 Go 33.303s、整机脚本回归 0.231s 通过；完整 Chrome 114 项通过（123.186s），最终 CGO_ENABLED=0 构建通过（44,429,824 字节），Go 格式、JS 语法、diff 空白及 101 份文档链接检查通过。最终构建完整隔离整机脚本于 2026-10-16 01:53:34 +08:00 退出 0，passed=true、isolated=true、agentArchiveExactLookupHTTPPassed=true、realQQMessagesSent=false，原有全部验证项通过。实际角色搜索来源在两种 archived 模式下精确读取，序号、类型、时间及正文核对一致，同号私聊及不存在序号均为空；未增加模型调用。本机证据 .local/smoke-admin/run-3670158473/result.json，构建 .local/bin/zerobot-archive-exact-lookup.exe。未部署日常服务，无真实 QQ 验收。见[使用说明](agent-archive.md#按来源序号定位档案)。

## 任务开始时间范围验证（2026-10-16）

- 存储定向 Go 1.365s、接口定向 2.140s、Chrome 新增两项 5.669s 通过：240 条候选跨页取得全部 90 个匹配，大整数继续、时区及 1ns 包含/排除边界、单端范围、状态/类别/阶段组合、群/私聊范围、旧字段、取消、非法参数及内存写失败覆盖均核对。页面验证草稿时间不改变翻页、单条独立、停止重置、错误及迟到清理，320px 双主题截图已查看。
- chat 全量 15.548s、后台全量 33.493s、AI 插件 2.893s、整机脚本回归 0.205s 通过；完整 Chrome 111 项通过（131.361s），最终 CGO_ENABLED=0 构建通过（44,426,240 字节），Go 格式、JS 语法、diff 空白及 101 份文档链接检查通过。最终构建完整隔离整机脚本于 2026-10-16 01:26:55 +08:00 退出 0，passed=true、isolated=true、agentTaskStartRangeHTTPPassed=true、realQQMessagesSent=false，原有全部验证项通过。实际 16 个输出限制/格式/模型故障及恢复任务分别核对 1ns 三组边界、组合筛选、+08:00 输入的 UTC 回显和编号/版本不变，无额外模型调用。本机证据 .local/smoke-admin/run-1699478378/result.json，构建 .local/bin/zerobot-task-start-range.exe。未部署日常服务，无真实 QQ 验收。见[使用说明](agent-tasks.md#按任务开始时间筛选)。

## 任务取用与执行统计展开验证（2026-10-16）

- 定向 Chrome 4 项通过（5.632s），含新增两项：列表两条独立默认收起，键盘 Enter 展开且无网络请求，保留筛选与分页；快捷单条核对默认展开且不改变列表折叠状态，收起不发请求，翻页恢复列表默认。存储失败、结果未知及核对入口直接可见，离开后旧统计销毁；原查询与结果统计窄屏用例改为先展开再检查。
- 后台全量 Go 17.374s、整机脚本回归 0.107s 通过；完整 Chrome 109 项通过（97.290s），320px 明暗主题展开/收起截图已查看。最终 CGO_ENABLED=0 构建通过（44,416,000 字节），Go 格式、JS 语法、diff 空白及 101 份文档链接检查通过。最终构建完整隔离整机脚本于 2026-10-16 01:12:51 +08:00 退出 0，passed=true、isolated=true、agentTaskStatisticsDisclosureAssetsPassed=true、realQQMessagesSent=false，原有全部验证项通过。新增整机标记核对真实服务提供的 JS/CSS 资源，交互行为由上述 Chrome 模拟接口测试覆盖。本机证据 .local/smoke-admin/run-2138742266/result.json，构建 .local/bin/zerobot-task-statistics-disclosure.exe。未部署日常服务，无真实 QQ 验收。见[使用说明](agent-tasks.md#展开任务取用与执行统计)。

## 记忆历史修订定位验证（2026-10-16）

- 定向 chat 2.243s、后台接口 2.127s 及 Chrome 4 项（9.042s）通过：覆盖正文编辑/停用 v1～v3 精确读取、机器人/群/私聊范围、不存在修订、>2^53 精确值、脱敏、读取前后当前内容与历史计数不变、旧版/停用模型读取仍为空、旧文件模式、取消和存储故障；接口权限、HEAD、no-store、非法编号/修订及分页/筛选混用通过。页面定位与返回最新、对比、当前版本绑定、未找到/回执错误、读取失败及迟到清理通过。
- chat 全量 18.814s、后台 42.041s、AI 插件 2.909s、模型统计 0.242s 及整机脚本 0.202s Go 回归通过。完整 Chrome 107 项通过（123.115s），含定位后恢复提交；320px 明暗截图已查看。最终 CGO_ENABLED=0 构建通过（44,414,976 字节），Go 格式、JS 语法、diff 空白及 101 份文档链接检查通过。最终构建完整隔离整机脚本于 2026-10-16 01:04:13 +08:00 退出 0，passed=true、isolated=true、agentMemoryExactRevisionHTTPPassed=true、realQQMessagesSent=false，原有全部验证项通过。实际后台精确读取 v1、停用 v4、当前 v5，对照历史页各字段一致；不存在 v6 与同号私聊为空，读后当前 v5 不变，模拟模型旧版读取仍被拒绝。本机证据 .local/smoke-admin/run-738591542/result.json，构建 .local/bin/zerobot-memory-exact-revision.exe。未部署日常服务，无真实 QQ 验收。见[使用说明](agent-memory.md#按修订号定位历史版本)。

## 后台前后事件范围验证（2026-10-16）

- 定向 Chrome 7 项通过（12.830s），包含新增两项：默认每侧 1、选择 3 读取最多 7 条、改选 2 清除旧展开且不发请求、重读 5 条及重新搜索恢复 1；草稿范围不改目标，精确大整数序号、纯文本和实际条数提示。选定 2 的回执半径、单侧数量及总条数不符被拒绝；读取在途禁用选择，离开后迟到内容不回填。原前后核对用例保持通过，320px 双主题截图已生成。
- 后台范围参数定向 Go 1.610s、整机脚本 Go 回归 0.126s 通过；后台全量 Go 25.429s、完整 Chrome 105 项（85.172s）通过，320px 明暗截图已查看。最终 CGO_ENABLED=0 构建通过（44,410,368 字节），JS 语法、Go 格式、diff 空白及 101 份文档链接检查通过。最终构建完整隔离整机脚本于 2026-10-16 00:52:37 +08:00 退出 0，passed=true、isolated=true、agentArchiveContextRadiusHTTPPassed=true、realQQMessagesSent=false，原有全部验证项通过。实际服务对同一搜索中心的 1/2/3 范围、中心及每侧数量、精确序号倒序、纯事件、单条与总正文容量均已核对。本机证据 .local/smoke-admin/run-3614422665/result.json，构建 .local/bin/zerobot-archive-context-radius.exe。未部署日常服务，无真实 QQ 验收。见[使用说明](agent-archive.md#选择前后事件范围)。

## 任务条目快捷核对验证（2026-10-16）

- 定向 Chrome 10 项通过（26.982s），含新增两项快捷入口用例：点击第二条任务并忽略编号/范围/筛选草稿，保留已应用列表及分页；核对最新大整数版本和当前阶段，结果只读且可读触发来源；失败、返回编号或版本无效、未可靠保存任务、请求禁用和离开后的迟到响应清理。原手动读取、模型错误及停止用例兼容，320px 双主题截图已生成。
- 后台 Go 全量 44.147s、整机脚本 Go 回归 7.641s 通过；完整 Chrome 103 项通过（115.369s），320px 明暗主题截图已查看。最终 CGO_ENABLED=0 构建通过（44,409,856 字节）；JS 语法、Go 格式、diff 空白及 101 份文档链接检查通过。最终构建完整隔离整机脚本于 2026-10-16 00:43:04 +08:00 退出 0，passed=true、isolated=true、agentTaskLatestEntryAssetsPassed=true、agentTaskExactReadScopeAndTerminalPassed=true、realQQMessagesSent=false，原有全部验证项通过。快捷入口资源由实际服务返回，原单条任务范围/终态/观察时间接口继续通过。本机证据 .local/smoke-admin/run-4153658597/result.json，构建 .local/bin/zerobot-task-latest-entry.exe。未部署日常服务；无真实 QQ 验收。使用见[任务快捷核对](agent-tasks.md#从任务条目核对最新状态)。

## 记忆修订时间检索验证（2026-10-16）

- 定向 chat 4.394s、后台 3.307s 通过；SDK 全量 3.525s、chat 全量 20.518s、后台 35.711s、AI 插件 9.637s、模型统计 0.304s 通过；整机脚本 Go 回归 3.544s 通过。覆盖纳秒含/不含边界、不同时区同一时刻、仅单端时间、状态/来源/范围组合、旧时间 partial、修改后移出条件、5,000 条空窗口继续、版本读取独立以及实际任务存储的三次额度和错误/空结果累计。
- 两项新增 Chrome 用例通过，覆盖时间独立查询、组合条件、已应用范围跨页、草稿不影响继续、时间清空、失败/迟到清理及 320px 双主题。完整 Chrome 回归 101 项通过（119.236s），320px 明暗主题截图已查看。最终 CGO_ENABLED=0 构建通过（44,409,344 字节）；Go 格式、JS 语法、diff 空白及 101 份文档链接检查通过。最终构建的完整隔离整机脚本于 2026-10-16 00:33:00 +08:00 退出 0，passed=true、isolated=true、agentMemoryRevisionTimeModelAndHTTPPassed=true、realQQMessagesSent=false，原有全部验证标志通过。模拟模型实际发出的两次查询保留 UTC 纳秒时间边界，后台无关键词/来源的 1ns 窗口核对开始包含、结束排除。本机证据 .local/smoke-admin/run-655870618/result.json，构建 .local/bin/zerobot-memory-revision-time-final.exe；仍为 53 次模拟模型 HTTP。
- 未部署日常服务；无真实 QQ 验收，真实模型日期理解仍待验收。见[使用说明](agent-memory.md#按当前修订时间检索记忆)。

## 任务阶段筛选验证（2026-10-15）

- Go chat 全量 25.071s、后台 49.376s、AI 插件 4.935s、模型统计 0.217s 通过；阶段分页用例覆盖 180 条混合阶段中的 120 条精确结果、>2^53 分页、群/私聊范围、状态与错误类别组合、旧摘要缺失阶段、持久写入失败的实际内存阶段以及未落盘首页。接口验证权限、合法阶段、组合条件、HEAD、no-store、非法/重复参数、单条/停止混用及故障返回。
- 新增两项 Chrome 用例通过，覆盖草稿与已应用条件、继续页、单条读取独立、停止重置、失败与迟到清理及 320px 双主题。完整 Chrome 回归 99 项通过（134.012s），含原档案、记忆、任务流程；320px 明暗主题截图已人工查看。最终 CGO_ENABLED=0 构建通过（44,397,568 字节），Go 格式、JS 语法、diff 空白和 101 份文档链接检查通过。完整隔离整机脚本于 2026-10-15 23:24:51 +08:00 退出 0，passed=true、isolated=true、agentTaskPhaseFilterHTTPPassed=true、realQQMessagesSent=false；实际下载/识图准备中的模型等待任务与结束故障集合均已核对，原有验证标志全部通过。本机证据 .local/smoke-admin/run-857079808/result.json，构建 .local/bin/zerobot-task-phase.exe。隔离自测脚本 Go 回归 3.385s 通过。
- 未部署日常服务；无真实 QQ 验收。步骤见[按阶段筛选任务](agent-tasks.md#按阶段筛选任务)。

## 最新修订优先的记忆历史验证（2026-10-15）

- 存储定向 3.941s 通过：构造 121 个修订，首批读取后新增停用 v122，再向前翻页仍完整覆盖 v121→v1，无重复遗漏；重新查看从停用 v122 开始，原接口仍从 v1 升序。私聊/其他群/机器人/不存在条目、排他下界、>2^53 修订号、脱敏及非法参数通过；历史总数不因读取改变，模型仍不能精确读取停用版本。
- 后台定向 5.218s 通过，核对排序仅用于指定条目历史、空串/asc/省略兼容、私聊范围及大整数边界、认证、no-store、非法混用和存储故障。两项 Chrome 定向全部通过（4.591s）：最新优先分页和已加载范围保持，原列表/下一页保留，按当前版本比较及恢复请求准确；失败清除旧分页，离开后的迟到结果不回填，320px 深浅主题无横向溢出。
- chat 全量 22.151s、localadmin 33.069s、aichat 5.828s、modelstats 0.222s、整机脚本回归 2.907s 通过。最终构建 `.local/bin/zerobot-memory-newest-versions-final.exe` 成功，44,391,936 字节，23:05:15 +08:00 生成。倒序旧页直接使用修订号上界条件，保留按主键范围定位；最终 SQL 定向复验 6.987s 通过。SDK 及模型协议未修改。
- 档案/记忆/任务页面全量 97 项全部通过（97.696s），最新修订页浅色截图已查看。最终构建于 23:10:57 +08:00 完整隔离整机验证通过，产物 `.local/smoke-admin/run-235544640/result.json`：passed=true、isolated=true、agentMemoryNewestVersionsHTTPPassed=true、realQQMessagesSent=false。实际旧接口仍从 v1→v5，order=desc 从 v5→v1，after=3 只返回 v2/v1，同号私聊返回空；阅读后当前条目仍为启用 v5。仍为 53 次模拟模型 HTTP，50 完成、3 次预设故障、零进行中和未完整记录，原全部整机流程通过。格式、JavaScript 语法、差异及 101 份公开文档相对链接检查通过。未部署日常服务，未使用真实 QQ 或模型提供商验收。

## 历史事件类型筛选验证（2026-10-15）

- 聊天定向 6.704s 通过：混合消息、通知、事件请求与工具请求，严格匹配事件顶层 post_type；正文提及、嵌套/缺失/非字符串及其他类型不误匹配，未知元数据 partial，脱敏、私聊/重置/快照及实际继续核对。5,001 条较新通知不绕过消息搜索的 5,000 条预算，空窗口可沿真实位置继续。模型三类仅类型参数、空串取消、回执、省略关键词、非法类型及其他动作误用和共享额度通过。
- 后台定向 8.141s 通过，认证/CSRF、仅类型查询、时间/编号组合、私聊范围、回执和 no-store、非法类型/GET 混用及空查询均核对。新增三项 Chrome 定向全部通过（10.181s）：类型继续与查之前保留已应用条件、原文独立读取、组合和清空、返回时间线、迟到和离开清理及 320px 深浅主题；浅色截图已查看。
- SDK 全量 3.284s、chat 全量 21.254s、localadmin 42.010s、aichat 4.977s、modelstats 0.222s、整机脚本回归 2.762s 通过。构建 `.local/bin/zerobot-history-event-type.exe` 成功，44,386,816 字节，21:52:18 +08:00 生成。
- 档案/记忆/任务页面全量 95 项全部通过（113.683s）。最终构建于 21:58:11 +08:00 完整隔离整机验证通过，产物 `.local/smoke-admin/run-1370280288/result.json`：passed=true、isolated=true、agentHistoryEventTypeModelRoundTripPassed=true、agentArchiveEventTypeSearchHTTPPassed=true、realQQMessagesSent=false。实际知识模型两次搜索保留 message 类型，核对已应用回执与继续条件，再独立读取来源上下文；后台只按类型查询，核对实际消息和另外两类空结果，未把工具记录当作事件请求。仍为 53 次模拟模型 HTTP，50 完成、3 次预设故障、零进行中和未完整记录，原全部整机流程通过。格式、JavaScript 语法、差异与 101 份公开文档相对链接检查通过。未部署日常服务，未使用真实 QQ 或模型提供商验收。

## 任务耗时快照验证（2026-10-15）

- 后台定向 3.502s 通过：固定带时区与纳秒的服务端时钟，列表、筛选分页、单条读取与停止回执均返回本次 UTC observedAt；重复读取更新时间，原编号、版本、范围、no-store、列表项结构和存储调用次数保持，失败不返回观察快照。
- 新增三项 Chrome 流程，定向全部通过（7.735s）：运行总时长/阶段/停止等待按服务端快照，改变浏览器 Date.now 不影响；单条核对使用新观察时间且不改列表；终态总耗时与停止耗时固定，按整秒估算；旧摘要独立缺失、倒序、无效日期/正文、中断和未保存明确提示。停止回执、离开清理及 320px 深浅主题通过，浅色截图已查看。
- localadmin 全量 39.235s、aichat 4.049s、modelstats 0.214s、整机脚本回归 3.035s 通过。构建 `.local/bin/zerobot-task-duration.exe` 成功，44,381,696 字节，21:19:40 +08:00 生成。仅后台与脚本有代码变化，Agent SDK 和聊天存储未修改。
- 档案/记忆/任务页面全量 92 项全部通过（118.396s）。最终构建于 21:25:13 +08:00 完整隔离整机验证通过，产物 `.local/smoke-admin/run-3324837723/result.json`：passed=true、isolated=true、agentTaskObservationSnapshotsPassed=true、realQQMessagesSent=false。实际角色终态列表、模型/动作/普通聊天兜底在途列表及单条读取、首次/重复停止回执、停止后的终态列表与单条响应均返回有效观察时间且不早于任务登记时间；原阶段和首次停止时间保持。仍为 53 次模拟模型 HTTP，50 完成、3 次预设故障、零进行中和未完整记录，原全部整机流程通过。格式、JavaScript 语法、差异与 101 份公开文档相对链接检查通过。未部署日常服务，未使用真实 QQ 或模型提供商验收。

## 后台按启停状态搜索记忆验证（2026-10-15）

- 存储定向测试覆盖 active/disabled/all、来源组合及空关键词、当前修订不混入旧正文、脱敏、编号继续、机器人/群/私聊隔离及非法状态。另插入 5,001 条较新启用记忆，仍可直接查到窗口之外的停用条目，确认状态在有界扫描前过滤；全状态查询保留结果上限及实际继续位置。Agent 原启用包装和精确读取排除停用条目；后台新增 state 不进入模型回执，模型 state 参数拒绝且计查询额度。chat 定向 2.314s 通过。
- 后台定向 2.625s 通过，核对状态默认值与回执、来源组合、>2^53 编号、私聊范围、认证/CSRF、no-store、非法状态及混用操作、存储失败。新增三个 Chrome 流程：停用搜索定位最新正文并按最新版本恢复；继续/查之前保留已应用状态而非草稿，搜索与列表状态独立；重读、离开及迟到清理和 320px 深浅主题布局。迟到用例通过页面实际离开入口核对，定向三项全部通过（4.495s）。
- 最终 chat 全量 16.053s、localadmin 24.767s、aichat 4.937s、modelstats 0.161s、最终整机脚本回归 1.239s 通过。最终构建 `.local/bin/zerobot-memory-search-state-final.exe` 成功，44,377,600 字节，19:52:35 +08:00 生成。
- 档案/记忆/任务页面全量 89 项全部通过（65.680s），320px 深浅主题无横向溢出，浅色截图已查看。最终构建于 19:58:24 +08:00 完整隔离整机验证通过，产物 `.local/smoke-admin/run-3349205724/result.json`：passed=true、isolated=true、agentMemoryDisabledSearchAndRestoreHTTPPassed=true、realQQMessagesSent=false。实际停用 v4 在 disabled/all 可查、active 不可查，admin 来源与编号组合可用，恢复到启用 v5 后移出停用结果；原模型精确读取继续排除停用或旧版本，实际模型搜索回执未出现管理状态字段。仍为 53 次模拟模型 HTTP，50 完成、3 次预设故障、零进行中及未完整记录，原全部整机流程通过。格式、JavaScript 语法、差异及 101 份公开文档相对链接检查通过。未部署日常服务，未使用真实 QQ 或提供商验收。

## 仅按具体来源浏览记忆验证（2026-10-15）

- 存储与 Agent 参数定向回归通过：具体来源下省略、空串及空白关键词可用，三类来源保持当前修订、启用状态及账号/会话隔离；编号继续、后台修订后来源变化和原片段/字节预算通过。无关键词和无具体来源、非法来源或关键词类型仍按原参数错误处理，尝试计入共享额度。
- 后台 API 定向回归通过，核对省略/空白关键词、私聊绑定、>2^53 编号、认证/CSRF、no-store 及非法条件不访问存储。Chrome 新增来源浏览流程，继续及“查此条之前”沿用已应用空关键词与来源，忽略草稿且保留原列表；切回全部来源后空关键词不提交。完整档案/记忆/任务页面 86 项全部通过（60.446s）。
- SDK 全量 1.833s、chat 全量 11.487s、localadmin 22.462s、aichat 3.567s、modelstats 0.152s、整机脚本回归 3.770s 通过。最终构建 `.local/bin/zerobot-memory-source-only.exe` 成功，44,371,968 字节，19:37:24 +08:00 生成。
- 最终构建于 19:42:26 +08:00 完整隔离整机验证通过，产物 `.local/smoke-admin/run-2860943570/result.json`：passed=true、isolated=true、agentMemorySourceOnlyModelAndHTTPPassed=true、realQQMessagesSent=false。实际首轮省略 keyword 得到当前后台来源条目，沿实际边界添加关键词找到更早目标并读取精确正文；后台无关键词 admin/legacy 和已迁移 legacy 查询通过。原全部整机流程通过，仍为 53 次模拟模型 HTTP，50 完成、3 次预设故障、零进行中和未完整记录。格式、JavaScript 语法、差异和 101 份公开文档相对链接检查通过，320px 来源控件浅色截图已查看。未部署日常服务，未使用真实 QQ 或模型提供商验收。

## 按编号定位当前记忆验证（2026-10-15）

- 最终构建于 19:22:38 +08:00 完整隔离整机验证通过，产物 `.local/smoke-admin/run-1084735492/result.json`：passed=true、isolated=true、agentMemoryCurrentReadAndLifecycleHTTPPassed=true、realQQMessagesSent=false。实际确认目标不在首页，current 接口仍返回创建时的完整条目；修订/停用/旧正文恢复后定位到停用 v4，再启用后定位到 v5，同号私聊和另一群返回空。模型精确读取继续排除停用及旧版本。仍为 53 次模拟模型 HTTP，50 完成、3 次预设故障、零进行中和未完整记录，原全部整机流程通过。

- 新增存储测试，先创建 70 条记忆，确认目标不在首页，再读取修订并停用后的当前 v3；重复定位不新增历史，群/私聊及机器人隔离、未找到、取消上下文与旧文件正文不展示均核对。后台验证登录、账号范围、正整数字符串及 >2^53 编号、混用参数、no-store 与存储故障。定向 chat 3.228s、localadmin 4.008s 通过。
- 新增 3 项 Chrome 流程：定位后列表状态和分页保持；页外停用条目以当前版本打开编辑窗口，版本比较采用定位结果；旧搜索片段跳转读取最新正文，冲突提交不重试并清理旧定位；缺失/未迁移/失败、迟到和离开清理均核对。定向 3 项 4.544s，全量档案/记忆/任务 85 项全部通过（70.179s）；320px 深浅主题无横向溢出，浅色截图已查看。
- chat 全量 10.288s、localadmin 33.681s、aichat 2.213s、modelstats 0.139s 通过；整机脚本回归 1.191s 通过，新增页外条目直接读取、停用 v4/恢复启用 v5 及同号私聊/另一群不匹配的核对。
- 构建 `.local/bin/zerobot-memory-current.exe` 成功，44,371,456 字节，19:17:01 +08:00 生成。Go 模块 stat 缓存权限警告未影响退出码 0 和产物；格式、JavaScript 语法、差异及 101 份公开文档相对链接检查通过。未部署日常服务，未使用真实 QQ 或模型提供商验收。

## 按时间直接回顾历史验证（2026-10-15）

- 最终构建于 18:21:21 +08:00 完整隔离整机验证通过，产物 `.local/smoke-admin/run-52414044/result.json`：passed=true、isolated=true、agentHistoryTimeOnlyQueryPassed=true、agentArchiveTimeOnlySearchHTTPPassed=true、realQQMessagesSent=false。角色任务只填 from 与 limit 的查询成功且非空，实际任务额度/结果可靠保存；后台省略关键词的时间范围与来源边界检查通过。仍为 53 次模拟模型 HTTP，50 完成、3 次预设故障、零进行中及未完整记录，原全部整机流程通过。

- 存储原时间用例分别用关键词、空串及纯空白运行，核对纳秒精度、时区偏移、起含/终不含、单端边界、重置和来源序号组合。5,000 条空窗口用例增加无关键词查询及按实际 nextBefore 继续命中旧记录。Agent 参数覆盖只填时间，实际两轮模型任务取得不含原关键词的来源后正常结束；定向 chat 3.219s、后台 2.607s 通过。
- SDK 全量 1.530s、chat 10.152s、localadmin 22.609s、aichat 2.788s、modelstats 0.161s 通过；完整权限表和系统提示同步。整机脚本回归 1.358s 通过，角色任务改用只带 from 与 limit 的历史查询，成功/非空及额度仍按实际任务摘要核对；后台时间边界查询省略关键词。
- 新增 1 项 Chrome 流程，档案/记忆/任务共 82 项全部通过（68.547s）。覆盖只填本机时间提交、UTC 转换、空关键词/发言者、继续和快捷查询保留已应用范围、单端时间、清空所有条件不请求；原 320px 深浅主题布局检查保持。
- 构建 `.local/bin/zerobot-agent-time-review.exe` 成功，44,359,168 字节，18:15:51 +08:00 生成。Go 模块 stat 缓存权限警告未影响退出码 0 和产物；Go 格式、JavaScript 语法、差异及 101 份公开文档相对链接检查通过。未部署日常服务，未使用真实 QQ 或模型提供商验收。

## 长期记忆来源检索验证（2026-10-15）

- 最终构建于 18:08:15 +08:00 完整隔离整机验证通过，产物 `.local/smoke-admin/run-180518161/result.json`：passed=true、isolated=true、agentMemorySourceSearchModelRoundTripPassed=true、agentMemorySourceSearchHTTPPassed=true、realQQMessagesSent=false。实际模型在搜索和继续时使用 admin 条件、核对响应和条目来源，再读精确正文；后台在混合来源中分别核对 admin/agent/legacy，实际已迁移条目也可命中。仍为 53 次模拟模型 HTTP，50 完成、3 次预设故障、零进行中及未完整记录，原全部整机流程保持通过。

- 新增存储/Agent 来源测试，覆盖三种来源与不限来源、筛选后的扫描计数和实际继续位置、停用排除、群/私聊及机器人隔离、当前正文被后台修订后的来源变化、已应用条件回执及三次合计查询额度。非法来源和其他动作不接受来源参数；定向 chat 8.877s、后台接口 1.968s 通过。
- SDK 全量 2.039s、chat 10.126s、localadmin 21.207s、aichat 2.531s、modelstats 0.115s 通过；权限表和系统提示完整文本已同步。整机脚本单测 1.258s 通过，新增 source=admin 的查询与继续请求核对、实际响应条件检查，以及后台混合来源和已迁移条目的来源匹配。
- 新增 2 项 Chrome 用例，档案/记忆/任务共 81 项全部通过（62.835s）。覆盖来源选择、已应用条件与草稿分离、实际继续和快捷查询、正文读取不继承来源、取消条件、重新读取/切换范围清理；320px 深浅主题无横向溢出，浅色截图已核对。
- 最终构建 `.local/bin/zerobot-agent-memory-source.exe` 成功，44,358,656 字节，18:02:34 +08:00 生成。Go 模块 stat 缓存权限警告未影响退出码 0 和产物；格式、JavaScript 语法、差异及 101 份公开文档相对链接检查通过。未部署日常服务，未使用真实 QQ 或模型提供商验收。

## 无关键词发言者查询验证（2026-10-15）

- 最终构建于 17:31:28 +08:00 完整隔离整机验证通过，产物 `.local/smoke-admin/run-2615240248/result.json`：passed=true、isolated=true、agentHistorySenderOnlyModelRoundTripPassed=true、agentArchiveSenderOnlySearchHTTPPassed=true、realQQMessagesSent=false。实际模型使用查询回执继续到不含关键词的发言者搜索，再按实际来源读取上下文，后台有/无关键词和不存在作者均核对。仍为 53 次模拟模型 HTTP，50 完成、3 次预设故障，进行中和未完整记录均为零；其余记忆、任务、权限、识图及连接流程继续通过。

- 存储现有作者/范围/继续查询用例分别使用关键词、空串和纯空白重跑，核对精确大整数、消息与工具区分、未知字段、重置、快照、时间和私聊/机器人隔离。Agent 新增省略/空白关键词与参数组合测试，原额度重建测试改用无关键词查询；后台验证省略、空串、空白以及两项都空时的行为。定向 chat 2.674s、localadmin 2.645s 通过。
- 权限表和系统提示完整文本预期同步后，SDK 全量 1.751s、chat 10.033s、localadmin 22.357s、aichat 2.528s、modelstats 0.172s 通过。整机脚本核对后续请求不含 keyword，允许正确作者的消息不含原搜索词；最终脚本回归 1.201s 通过。
- 新增 Chrome 无关键词提交、两项都空不请求、时间与大整数发言者、实际继续及快捷查询保留已应用条件流程。档案/记忆/任务共 79 项全部通过（59.135s）；320px 深浅主题布局检查保持，浅色截图已查看。
- 构建 `.local/bin/zerobot-agent-sender-only.exe` 成功，44,353,024 字节，17:26:09 +08:00 生成；Go 模块 stat 缓存权限警告未影响退出码 0 和产物。未部署日常服务，未发送真实 QQ 消息或调用真实提供商。
- Go 格式、JavaScript 语法、差异及 101 份公开文档相对链接检查通过。

## 动作 JSON 结构预算验证（2026-10-15）

- 修复后的最终构建于 17:19:23 +08:00 完整隔离整机验证通过，产物 `.local/smoke-admin/run-623259696/result.json`：passed=true、isolated=true、agentActionStructureLimitsAndRecoveryPassed=true、realQQMessagesSent=false。实际 65 层与跨动作累计值超限任务均可靠失败，零派发/查询/兜底且保存前缀未写入；后续准确 64 层任务和多段文本恢复正常。53 次模拟模型 HTTP 为 50 完成、3 次预设故障、零进行中和未完整记录，原有检索、记忆、任务取消、识图、权限及正反向连接等流程继续通过。

- SDK 新增边界与自动前缀/恢复测试：准确 64 层与 10,000 个值通过，下一层/值拒绝；跨动作共享预算、结束对象自身计数、有效结束后尾部忽略、对象成员、字符串中大量括号、>2^53 数字与旧 1,000 层输入归类均核对。自动保存模式下，合法记忆/发送前缀后出现坏结构时零写入，之后合法边界回复正常恢复。SDK 全量 1.492s 通过，嵌入提示及其完整文本测试同步。
- chat 实际任务回归增加深层、单对象大量值和跨动作累计三类，均归 model_output_limit，一次模型准备、零派发/查询/兜底，动作请求/响应/结束标记未入档，终态可靠保存。定向 3.939s、最终 chat 全量 9.660s 通过；localadmin 21.577s、aichat 2.102s、modelstats 0.133s 通过。
- 脚本新增独立结构用例有效性检查，确认 JSON 合法、字节和顶层动作数未触及其他上限，并加入合法 64 层恢复数据；最终脚本回归 1.238s 通过。档案/记忆/任务现有 78 项 Chrome 全部通过（54.861s），模型输出超限说明现包含结构限制，列表/单条、筛选、停止及范围清理保持。
- 首次完整整机在 HTTP 统计处失败：53 次请求为 49 完成、3 预期失败、1 提前关闭，未作为通过证据。已定位未知长度分块响应被单次 JSON Decode 提前结束，在有界客户端内读到 EOF 后提供实测长度；未放宽统计断言或强制模拟服务响应长度。新增真实分块 JSON 的 Agent/识图多尺寸解码、EOF 等待超时、读取/关闭错误和已知长度异常超限回归，aichat 5.328s、modelstats 0.127s 通过。
- 修复后重新构建 `.local/bin/zerobot-agent-action-structure.exe` 成功，44,353,024 字节，17:14:35 +08:00 生成。Go 模块 stat 缓存权限警告未影响退出码 0 和产物。未部署日常服务，未使用真实 QQ 或提供商验收。
- Go 格式、JavaScript 语法、差异及 101 份公开文档相对链接检查通过；嵌入提示末尾无多余换行。

## 按发言者检索历史验证（2026-10-15）

- 最终完整隔离整机验证于 16:52:43 +08:00 通过，产物 `.local/smoke-admin/run-2045816411/result.json`：passed=true、isolated=true、agentHistorySenderModelRoundTripPassed=true、agentArchiveSenderSearchHTTPPassed=true、realQQMessagesSent=false。实际模型以档案选出的发言者组合时间/序号检索，按真实继续回执再查并读来源上下文，实际消息 user_id、条件回执和继续请求均核对；后台用实际中心发言者及不存在账号分别查询。仍为 51 次模拟模型 HTTP（48 完成、3 次原有预期失败），三次共享查询额度及既有全部整机流程保持。
- 新增 3 项存储测试，覆盖顶层发送者精确匹配、JSON 数字/字符串及 >2^53 编号、正文提及/嵌套字段不算作者、工具/通知/其他事件排除，未知字段跳过且 partial。跨机器人/同号私聊、重置/归档/快照、时间组合、结果容量边界不丢匹配、空 5,000 条窗口及 4 MiB 原始字节预算均核对。
- 新增 2 项 Agent 参数/任务测试，覆盖错误类型、三种其他查询拒绝 sender、已应用回执、独立来源读取不继承条件，参数和存储错误占额度、助手重建后仍共用三次上限及拒绝统计持久化。后台接口验证认证/CSRF、精确范围与时间/序号组合、空值取消、null/数字等非法类型、GET 拒绝及 no-store。
- 新增 4 项 Chrome 流程，最终档案/记忆/任务共 78 项全部通过（55.180s），覆盖大整数与条件组合、继续/快捷查询保留已加载条件、无效草稿不替换旧结果、清空、上下文不继承发言者、失败/迟到/退出清理。320px 深浅主题无横向溢出，截图已核对。
- SDK 全量 1.620s、chat 11.204s、localadmin 22.899s、aichat 2.578s、modelstats 0.188s 通过；脚本单测 2.665s 通过，新增完整消息作者校验及继续请求/条件回执检查，防止正文提及被误判。构建 `.local/bin/zerobot-agent-history-sender.exe` 成功，44,352,000 字节，16:45:45 +08:00 生成；Go 模块 stat 缓存权限警告未影响退出码 0 和产物。
- Go 格式、JavaScript 语法、差异及 101 份公开文档相对链接检查通过。
- 未部署日常服务，未使用真实 QQ 或真实历史数据验收。

## 任务错误类别组合筛选验证（2026-10-15）

- 最终布局构建的完整隔离整机验证于 16:15:53 +08:00 通过，产物 `.local/smoke-admin/run-4158989373/result.json`：passed=true、isolated=true、agentTaskReasonFilterHTTPPassed=true、realQQMessagesSent=false。以实际故障/恢复任务编号集合核对三类模型错误及未记录类别，状态组合无匹配时为空，同号私聊不返回群任务；最终程序实际提供筛选控件与脚本。仍为 51 次模拟模型 HTTP（48 完成、3 次原有预期失败），既有全部整机流程通过。
- 新增 3 项存储测试及后台接口测试，覆盖状态与类别组合、>2^53 创建序号的完整跨页集合、已知类别、旧/未知/缺失类别、群与同号私聊隔离、取消上下文和损坏 JSON。真实任务的模型请求/输出限制/格式错误及执行/设置/重复动作终态均可按准确类别读取。状态写失败后先覆盖内存实际故障，未插入任务仅在首页显示，非匹配故障不占用正常匹配的分页位置。
- 新增 4 项 Chrome 流程；初次全部 74 项通过（52.683s）。截图发现类别标签与原状态控件同行拥挤，已将类别控件独立成行；最终档案/记忆/任务共 74 项再次全部通过（62.183s），320px 深浅主题截图已核对。覆盖选择后手动应用、已加载条件与草稿分离、按编号读取保持列表、错误/迟到/退出清理、停止成功恢复全部及未知类别纯文本提示。
- SDK 全量 0.916s、chat 10.391s、localadmin 22.738s、aichat 4.427s、modelstats 0.147s 通过；脚本单测 1.435s 通过。最终布局构建 `.local/bin/zerobot-agent-task-reasons-final.exe` 成功，44,346,368 字节，16:11:01 +08:00 生成；Go 模块 stat 缓存权限警告未影响退出码 0 和产物。
- Go 格式、JavaScript 语法、差异及 101 份公开文档相对链接检查通过。
- 未部署日常服务，未使用真实 QQ 或真实任务数据验收。

## 后台来源前后事件核对验证（2026-10-15）

- 最终完整隔离整机验证于 15:59:07 +08:00 通过，产物 `.local/smoke-admin/run-1128281135/result.json`：passed=true、isolated=true、agentArchiveSourceContextHTTPPassed=true、realQQMessagesSent=false。从实际搜索来源经最终程序 HTTP 读取中心与邻居，顺序/类型/容量符合限制；同号私聊和缺失中心均为空，归档/重复/越界参数拒绝。最终程序实际提供新按钮脚本，既有模型检索与来源上下文流程也通过。
- 新增 4 项 Chrome 流程，最终档案/记忆/任务页面共 70 项全部通过（46.798s）。覆盖已加载范围与输入草稿分离、>2^53 字符串来源、中心/半径/顺序/类型不符时拒绝展示、原片段保留、纯文本、收起重读、没有邻居、缺失/文件不可用/失败区分、请求互斥与迟到清理，以及工具/归档模式不提供按钮。320px 深浅主题无横向溢出，截图已核对。
- 后台接口测试覆盖认证、当前已连接账号、半径 0/1/3、非法/重复参数、列表/搜索/游标混用、同号私聊范围、超时、no-store 与错误信息隐藏；不带 context 的归档单条读取保持。现有存储上下文回归继续验证中心、重置、跨账号/会话、UTF-8 与容量限制。
- 最终 SDK 全量 0.547s、chat 20.394s、localadmin 20.677s、aichat 1.892s、modelstats 0.178s 通过；整机脚本最终单测 1.194s 通过。构建 `.local/bin/zerobot-admin-source-context.exe` 成功，44,332,544 字节，15:52:34 +08:00 生成。Go 模块 stat 缓存权限警告未影响退出码 0 和产物。
- Go 格式、JavaScript 语法、差异及 101 份公开文档相对链接检查通过。
- 未部署日常服务，未使用真实 QQ 或真实聊天数据验收。

## 历史来源上下文核对验证（2026-10-15）

- 最终完整隔离整机验证于 15:43:16 +08:00 通过，产物 `.local/smoke-admin/run-596813808/result.json`：passed=true、isolated=true、agentHistorySourceContextModelRoundTripPassed=true、realQQMessagesSent=false。实际模型从继续搜索结果选择中心，调用 read_history(context=1)，下一轮核对中心、相邻事件、顺序和容量后完成任务。查询仍为两次搜索加一次读取，整机仍为 51 次模拟模型 HTTP（48 完成、3 次原有预期失败），原全部流程通过。

- 新增 3 项存储/Agent 测试，初次定向通过（2.729s），之后补充 context=0 兼容和其他搜索动作拒绝参数并纳入最终回归。覆盖中心验证、相邻事件排除工具/同号私聊、跨机器人/重置/快照隔离、不存在中心不返回邻居、>2^53 来源、最大半径和 UTF-8/单条/合计预算；中心正文保留既有额度，超大正文占位。
- 任务回归核对一次上下文读取只计一次 read_history，回执中心编号/半径和最多事件数准确，查询助手重建仍沿用额度。后台新增 context 参数拒绝测试；脚本新增中心来源必须来自实际搜索及邻居/顺序/容量/请求对应检查，单测通过（2.181s）。
- 最终 SDK 全量 3.392s、chat 15.315s、localadmin 24.092s、aichat 5.589s、modelstats 0.346s 通过；构建 `.local/bin/zerobot-agent-source-context.exe` 成功，44,328,448 字节。Go 模块 stat 缓存权限警告未影响退出码 0 和产物。格式、差异及 101 份公开文档相对链接检查通过。
- 本轮无页面行为修改，未重复 Chrome，保留上一轮 66 项通过记录。未部署日常服务，未使用真实 QQ 或真实聊天数据验收。

## 长期记忆实际进度继续检索验证（2026-10-15）

- 最终完整隔离整机验证于 15:28:23 +08:00 通过，产物 `.local/smoke-admin/run-506229376/result.json`：passed=true、isolated=true、agentMemoryContinuationModelRoundTripPassed=true、realQQMessagesSent=false。实际模型使用记忆回执而非最后匹配编号继续，按准确版本读取正文，三次查询额度与任务摘要一致；最终程序提供后台继续按钮和脚本。整机 51 次模拟模型 HTTP 为 48 完成、3 次原有预期失败，原全部流程通过。

- 新增 4 项存储/任务测试，定向通过（2.488s）：记忆编号而非来源序号的继续位置、未返回匹配不丢失、空 5,000 条窗口与 >2^53 编号、4 MiB/16 KiB 预算，以及片段截断不伪造下一批。继续读取当前修订、排除停用/旧文件、聊天重置不删除记忆和同号私聊/机器人隔离均核对。
- 真实任务使用回执继续两次搜索后按准确版本读取；助手重建不重置三次共享额度，第四次查询拒绝，查询类型、成功及拒绝统计正确持久化。整机脚本单测通过（1.195s），模拟模型必须依据实际 nextBefore 继续，拒绝非法回执。
- 新增 4 项页面流程测试，定向通过（5.894s）；最终档案/记忆/任务 Chrome 共 66 项全部通过（59.110s），含空结果、范围/草稿分离、无效/不前进编号、旧文件、编辑窗口、未知保存结果清理、失败/迟到及退出。320px 深浅主题无横向溢出，截图已核对。
- 最终 SDK 全量 1.851s、chat 19.471s、localadmin 31.599s、aichat 2.670s、modelstats 0.392s 通过。构建 `.local/bin/zerobot-memory-search-continuation.exe` 成功，44,323,840 字节；Go 模块 stat 缓存权限警告未影响退出码 0 和产物。格式、JavaScript 语法、差异及 101 份公开文档相对链接检查通过。
- 未部署日常服务，未进行真实 QQ 或真实档案验收。

## 后台档案继续搜索入口验证（2026-10-15）

- 最终完整隔离整机验证于 15:16:27 +08:00 通过，产物 `.local/smoke-admin/run-817109473/result.json`：passed=true、isolated=true、agentArchiveSearchContinuationAssetsPassed=true、realQQMessagesSent=false。最终程序实际提供按钮和绑定已应用范围的脚本；实际模型继续搜索/来源读取及原全部流程通过，模型 HTTP 仍为 51 次（48 完成、3 次原有预期失败）。页面点击行为由下述隔离 Chrome 测试覆盖。

- 新增 4 项隔离 Chrome 流程测试，定向通过（5.972s）。覆盖空结果窗口继续、已应用关键词/时间/机器人与会话条件、大整数字符串、非法/不前进的位置、无效草稿保留原条件、读取失败不重试、迟到响应以及时间线/范围/退出清理。
- 最终档案/记忆/任务页面完整回归 62 项全部通过（55.265s），320px 深浅主题无横向溢出，继续入口截图均已检查。未使用外部网络、真实 QQ 或真实档案。
- localadmin 回归 29.054s、aichat 2.141s、modelstats 0.446s、整机脚本单测 1.589s 通过。JavaScript 语法、差异及 101 份公开文档相对链接检查通过。
- 构建 `.local/bin/zerobot-archive-search-continue-ui.exe` 成功，44,321,280 字节；Go 模块 stat 缓存权限警告未影响退出码 0 和产物。此轮未改 SDK/存储逻辑，未重复其独立全量测试；未部署日常服务，真实 QQ 体验未验收。

## 历史搜索实际进度继续检索验证（2026-10-15）

- 最终完整隔离整机验证于 15:06:59 +08:00 通过，产物 `.local/smoke-admin/run-2802748928/result.json`：passed=true、isolated=true、agentHistoryContinuationModelRoundTripPassed=true、realQQMessagesSent=false。知识历史任务四次实际模型请求完成两次搜索和一次来源读取，三次共享查询额度、成功结果及任务摘要一致；整机 51 次模拟 HTTP 为 48 完成、3 次原有预期失败，Agent 类别为 39 次。

- 新增 3 项存储测试及 1 项实际 HTTP 模型任务测试，定向回归通过（2.531s）。覆盖结果数量、4 MiB 扫描及 16 KiB 返回预算不漏未返回匹配、5,000 条空时间窗口继续、>2^53 字符串序号、重置和同号私聊隔离，以及片段截断/异常时间不伪造继续位置。
- 实际四轮模型流程依据 nextBefore 继续搜索并读取较早来源，查询助手重建后沿用三次共享额度，最终任务成功和查询类型准确持久化；脚本单测覆盖继续回执缺失、无效边界及实际来源判断（1.240s）。
- 最终 SDK 全量通过（3.377s），chat 14.558s、localadmin 21.976s、aichat 3.098s、modelstats 0.196s 通过。构建 `.local/bin/zerobot-agent-search-continuation.exe` 成功，44,319,232 字节；Go 模块 stat 缓存权限警告未影响退出码 0 和产物。
- 格式、差异与 101 份公开文档相对链接检查通过。本轮没有前端代码改动，未重复 Chrome，保留此前 58 项通过记录；未部署日常服务，未使用真实 QQ 或真实聊天数据验收。

## Agent 历史时间范围检索验证（2026-10-15）

- 完整隔离整机验证于 14:41:41 +08:00 通过，产物 `.local/smoke-admin/run-214786034/result.json`：passed=true、isolated=true、agentHistoryTimeRangeModelRoundTripPassed=true、realQQMessagesSent=false。既有知识历史任务实际发出带 from/until/before 的模型查询，核对时间范围、来源及 UTC 回执后继续读取原文，原两次查询额度和三次模型调用保持；整机仍为 50 次模拟模型 HTTP（47 完成、3 次原有预期失败）。

- 新增 3 项 chat 测试：时间参数/单端/清空/时区与大整数来源边界；实际 HTTP 模型两轮往返只取得范围内事件并核对 UTC 回执，排除同号私聊、工具记录和结束边界外记录；真实任务保存额度，非法/存储失败累计错误，助手重建后的第四次查询仍拒绝。
- 定向 chat 通过（3.790s）。最终 SDK 全量通过（1.809s），chat 全量 15.002s、localadmin 26.041s、aichat 2.652s、modelstats 0.396s、整机脚本 0.589s 通过。初轮测试发现额度夹具缺少真实动作登记、SDK 表格黄金快照未同步，修正后完整回归通过。
- 构建 `.local/bin/zerobot-agent-time-search.exe` 成功，44,316,160 字节；Go 模块 stat 缓存写权限警告未影响退出码 0 和产物生成。格式、差异及 101 份公开文档的相对链接检查通过。
- 本轮无前端行为改动，未重复 Chrome；上一轮 58 项页面通过记录保留。未部署日常服务，未使用真实 QQ 或真实聊天数据验收。

## 档案保存时间范围搜索验证（2026-10-15）

- 最终完整隔离整机验证于 14:28:50 +08:00 通过，结果 `.local/smoke-admin/run-3320030233/result.json`：passed=true、isolated=true、agentArchiveTimeRangeSearchPassed=true、realQQMessagesSent=false。实际后台、只读数据库查询验证纳秒时间窗、偏移时区、起点包含/终点排除和 before 组合；原有 50 次模拟模型请求计数及所有流程通过，时间查询不增加模型调用。

- 存储新增纳秒精度、偏移时区、起点包含/终点排除、单端、来源序号组合、重置与同号私聊隔离、未知时间及扫描预算测试；后台新增认证/CSRF、参数类型/格式/顺序、GET 拒绝及清空条件测试。定向测试通过（chat 2.406s、localadmin 2.706s）。
- 最终全量 chat 回归通过（19.034s），localadmin 34.888s、aichat 6.690s、modelstats 0.340s、整机脚本 4.767s 通过。
- 最终档案/记忆/任务 Chrome 回归 58 项全部通过（55.338s），含新增 3 项时间搜索流程，覆盖本机时区转 UTC、已应用条件与草稿分离、单端/清空/无效顺序、切换及迟到响应。320px 深浅主题无横向溢出，已检查截图。初轮 3 项新增测试因旧 Playwright 不接受带秒的 datetime-local fill 格式失败，调整测试输入为分钟格式后定向及完整回归均通过。
- 构建 `.local/bin/zerobot-archive-time-search.exe` 成功，44,313,600 字节；Go 模块 stat 缓存写权限警告未影响退出码 0 和产物生成。格式/差异检查及 101 份文档的相对链接检查通过。
- 未部署日常服务，未进行真实 QQ 或真实聊天数据验收。

## 识图多段文本与异常响应验证（2026-10-15）

- 完整隔离整机验证于 14:14:30 +08:00 通过，结果位于 `.local/smoke-admin/run-1635096052/result.json`：passed=true、isolated=true、agentVisionTextPartsPassed=true、realQQMessagesSent=false。50 次模拟模型请求为 47 次 HTTP 完成、3 次既有预期失败；中文多段描述的截断标记、上下文内容及缓存恢复均通过实际进程检查。

- SDK 全量回归通过（1.404s）。识图测试覆盖 OpenAI 多段纯文本及超长中文截断/缓存、缺失输出、未知和混合类型、GenAI 空内容转换异常及后续正常多段响应恢复；失败结果不附加、不缓存。
- chat 全量通过（7.900s）；localadmin（18.174s）、aichat（2.604s）、modelstats（0.122s）及整机脚本测试（1.197s）通过。既有停止检查、任务和容量回归一并运行。
- 构建 `.local/bin/zerobot-vision-text-parts.exe` 成功，44,307,968 字节。构建时出现 Go 模块 stat 缓存写入权限警告，但退出码为 0 且产物生成。
- 本轮无页面变更，未重复 Chrome 测试；未部署日常服务、未进行真实 QQ 验收。

## Agent 文本片段与异常输出验证（2026-10-15）

新增按调用的响应适配器，保留协议路由、请求体和 Header，在 Parse 后检查协议输出：仅接受非空片段集合中的纯 text，合计 1 MiB 容量检查通过后按顺序无标签/分隔符拼接，再进入原动作格式和权限检查。缺失、未知或混合类型返回固定模型请求故障；响应解析/提取边界内的转换异常不携带异常值。容量仍为 model_output_limit，明确识别为 text 的空字符串兼容。GetActionChecked 先复制协议，避免响应字段复用；历史、请求构造和平台调用不纳入该响应异常保护。使用入口见[任务说明](agent-tasks.md)。

SDK 新增 4 项测试：17 类响应场景涵盖 OpenAI/GenAI 多段文本、OLLaMA 单文本、缺失/空 choices、错误对象、缺失/null/数值/空数组内容、未知类型、有效保存前缀与图片混合、GenAI 转换异常及明确空文本；分段在大整数编号中间，拼回后仍保留精确 json.Number。另覆盖中文合计容量精确边界及超一字节、实际超限片段不保存前缀、连续复用协议的第二轮缺失输出不重放第一轮结束动作，源协议不被响应污染。实际持久任务回归追加缺失/未知输出两项，均零动作/查询/兜底/动作归档且持久化 model_request_failed。

SDK、chat、localadmin、aichat、modelstats 回归分别通过（7.645、13.881、25.311、2.537、0.158 秒）。显式脚本新增 1 项输出形状测试，确认坏样例是有效 JSON，正常恢复样例确实为两段模型 text 且拼回原 JSON，其他模型仍用原单字符串；脚本回归通过（5.648 秒）。本轮未改页面和 HTTP 参数，未重复离线 Chrome，最近 55 项页面验证保留原范围。

复验：在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；仓库根执行 `go test ./internal/localadmin ./plugin/aichat ./internal/modelstats -count=1 -timeout=180s` 及 `go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s`。

最终构建 `.local/bin/zerobot-agent-model-output.exe` 成功，CGO_ENABLED=0、trimpath，44,307,968 字节。模块缓存统计文件写入提示权限不足，但构建退出为 0 并产出本次程序。

执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-model-output.exe`，最终完整隔离整机验证通过，完成于 2026-10-15 12:15:07 +08:00。本机产物为 `.local/smoke-admin/run-2616066914/result.json`，新增 `agentModelTextPartsAndInvalidOutputPassed=true`，原模型故障、容量、格式及其他流程标记也通过，且 `passed=true`、`isolated=true`、`realQQMessagesSent=false`。

实际插件先收到有效 HTTP JSON 但缺失文本输出的错误对象，再收到带有效记忆保存前缀但包含未知内容类型的响应。两个新增任务均保存为 failed/model_request_failed，一次准备、零动作/查询/兜底，列表与单条的原因和版本一致，记忆列表为空。之后同会话正常新任务的模型响应确实由两段 text 组成，拼回 JSON 后成功发送原有两段 QQ 文本并完成。没有通过丢弃坏片段保留可执行前缀，日志、摘要和统计不含合成提供商错误正文；原有所有流程继续通过。

最终统计 50 次模拟请求：47 次 HTTP 完成、3 次原有预期失败、0 活跃/未完成；分类为 4 次 GPT/群总结、3 次普通聊天、38 次 Agent（36 完成、2 次预期失败）、5 次识图（4 完成、1 次预期失败）。两个新增输出形状错误均为 HTTP 完成但任务失败，不混为传输失败。Go 格式、差异空白和 101 份公开文档链接检查通过。测试使用合成数据与回环假模型/OneBot，未发送真实 QQ 消息，日常服务未部署，不替代真实提供商或 QQ 客户端验收。

## Agent 模型请求故障识别验证（2026-10-15）

SDK 将 Agent API.Request 返回的错误包装为 ErrModelRequestFailed，默认 Error 文字固定，不包含提供商正文、接口地址或密钥；保留底层 cause，标准 Is/As 可继续识别超时、网络错误和宿主 HTTP 容量限制。HTTP 返回后的任务检查先执行，停止仍优先；准备、历史和图片跳过语义保持。TaskStore.Finish 保存 model_request_failed，原取消、设置更新、输出限额及存储故障优先。页面列表与单条只读核对显示“Agent 模型请求或响应解析失败”，不输出原错误正文，不提供重跑，不自动重试或普通聊天兜底。使用入口见[任务说明](agent-tasks.md)。

SDK 新增 2 项测试覆盖 HTTP 401、503、HTTP 200 的非法/空协议、固定错误文本、底层 EOF/超时与 URL 错误类型保留、停止优先和零自动写入；每次实际失败仅调用一次假接口。chat 新增 2 项含 10 个子用例的测试，核对四类实际失败的零动作/查询/兜底、一次准备、准确持久终态、零请求/响应/结束归档，及模型、设置更新、容量、无关本地错误、取消和存储故障的分类优先级。脚本新增 1 项故障样例测试，确认 503、200 非法协议和空协议分别进入预期 HTTP 路径。

新增空响应定向回归先复现 EOF 被 NextAgentActionsChecked 当作正常结束：RunAgent 返回 nil 而没有报告模型故障，用例失败（2.814 秒）。结束判定排除 ErrModelRequestFailed 后，最终 SDK、chat、localadmin、aichat、modelstats 和显式脚本完整回归分别通过（1.693、12.254、24.794、3.237、0.231、2.990 秒），既有显式结束及多轮测试保持。初次失败不计为通过。离线 Chrome 新增故障列表及按编号核对测试，提示一致，无重跑按钮或额外请求；档案、记忆、任务页面合计 55 项通过、0 失败（44.600 秒）；后续 EOF 修正未再改页面，未重复 Chrome。

复验：在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；仓库根执行 `go test ./internal/localadmin ./plugin/aichat ./internal/modelstats -count=1 -timeout=180s`、`go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s` 及 `node --test scripts/test-agent-archive-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-tasks-ui.mjs`。

最终构建 `.local/bin/zerobot-agent-model-failure.exe` 成功，CGO_ENABLED=0、trimpath，44,297,728 字节；EOF 修正后已重新构建。模块缓存统计文件写入提示权限不足，但构建退出为 0 并产出本次程序。

EOF 修正后执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-model-failure.exe`，最终完整隔离整机验证通过，完成于 2026-10-15 12:02:02 +08:00。本机产物为 `.local/smoke-admin/run-2199487707/result.json`，`agentModelRequestFailureAndRecoveryPassed=true`，原输出限额、格式和其他流程标记也通过，且 `passed=true`、`isolated=true`、`realQQMessagesSent=false`。

实际插件分别收到 HTTP 503、HTTP 200 非法协议体、HTTP 200 空协议体。三项任务均持久化为 failed/model_request_failed，一次准备、零动作/查询/兜底；列表与单条的原因及版本一致，没有重试请求，任务摘要、日志与聚合统计不含合成提供商错误正文。空协议的 EOF 未变成 end_action，也未调用普通聊天。之后同会话新消息正常发送一次并完成，原显式结束、容量、格式、识图、停止、记忆、来源、权限及后台流程全部通过。

最终统计 48 次模拟请求：45 次 HTTP 完成、3 次预期失败、0 活跃/未完成；分类为 4 次 GPT/群总结、3 次普通聊天、36 次 Agent（34 完成，HTTP 容量超限和新增 503 各失败一次）、5 次识图（4 完成、1 次原有预期容量失败）。非法/空协议两个 HTTP 200 均计为交换完成，但任务失败单独核对。Go 格式、差异空白及 101 份公开文档链接检查通过。测试使用合成数据和回环假模型/OneBot，未发送真实 QQ 消息，日常服务未部署，不替代真实提供商故障或 QQ 客户端验收。

## Agent 重复 JSON 字段验证（2026-10-15）

动作预检在解码 APIRequest 及判断首个结束动作之前，逐对象检查解码后的字段名，重复字段整批返回 ErrActionOutputInvalid。覆盖转义同名、顶层 action/params/echo 的大小写别名、参数和嵌套消息及未知扩展字段，防止后值覆盖或 Params 合并改变动作含义；不同对象各自保存字段集合，普通 map 保持大小写敏感，消息正文不再次解析，数字保持 UseNumber。宿主沿用 model_output_invalid，前缀不保存/派发，不增加错误接口或自动重试。使用和兼容边界见[任务说明](agent-tasks.md)。

SDK 新增 1 项含 20 个子用例的测试，覆盖 exact/转义/大小写/Unicode 大小写别名、相同值重复、未知字段、嵌套消息、兄弟对象、独立动作、有效结束后的忽略内容、引号内 JSON 文字、大整数和 1,000 层有效嵌套。自动/手动模式零写入测试新增 4 类歧义后缀，两种模式均不返回可执行前缀。实际持久任务测试新增重复动作别名、重复目标参数、重复嵌套消息字段三项，均零派发/查询/请求响应归档，终态准确。脚本新增 1 项测试，确认新增坏回复是可被普通解码器接受的覆盖歧义，恢复样例确实包含两个独立文本消息段。

初次 SDK 回归失败于新增 empty-objects 样例：echo 被写为对象，不符合现有 uint64 协议；将样例修正为 echo=0、空扩展对象后，完整 SDK 回归通过（1.383 秒）。初次 chat 回归暴露原有两项阶段时间测试假定每次系统时间严格前进，Windows 上出现同刻样本而失败。测试在需要区分样本时先等待时钟实际前进，不改产品时间戳或唯一性承诺；两项测试各连续 20 次通过（4.252 秒），随后完整 chat 回归通过（5.541 秒）。首次失败不计为通过。

localadmin、aichat、modelstats 回归分别通过（20.761、3.538、0.127 秒），显式脚本测试通过（1.423 秒）。本轮未改页面或后台 HTTP 参数，未重复离线 Chrome；最近 54 项页面测试仍保留原验证范围。

复验：在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`，时间稳定性另执行 `go test ./chat -run 'TestAgentTask(PhaseTimesAndReadOnlyUpdates|FirstStopTimeConcurrentAndInFlight)$' -count=20 -timeout=120s`。仓库根执行 `go test ./internal/localadmin ./plugin/aichat ./internal/modelstats -count=1 -timeout=180s` 及 `go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s`。

最终构建 `.local/bin/zerobot-agent-json-keys.exe` 成功，CGO_ENABLED=0、trimpath，44,296,192 字节。模块缓存统计文件写入提示权限不足，但构建退出为 0 并产出本次程序。

执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-json-keys.exe`，完整隔离整机验证通过，完成于 2026-10-15 11:44:15 +08:00。本机产物为 `.local/smoke-admin/run-2331703487/result.json`，新增 `agentDuplicateJSONFieldsRejectedPassed=true`，原格式及容量标记也通过，且 `passed=true`、`isolated=true`、`realQQMessagesSent=false`。

实际插件在有效记忆保存/消息发送前缀后，分别收到 action/Action 别名冲突、group_id 与转义同名目标冲突。两项新增任务均保存为 failed/model_output_invalid，一次准备、零动作/查询/兜底；列表与单条的原因和版本一致，记忆列表为空。随后同会话新消息由两个独立文本消息段正常发送并完成，未误判兄弟对象各自的 type/data/text 字段。原格式、容量、识图、停止、记忆、来源、权限及后台流程全部通过。

模型统计合计 45 次模拟请求：43 次 HTTP 完成、2 次原有预期超限失败、0 活跃/未完成；分类为 4 次 GPT/群总结、3 次普通聊天、33 次 Agent（32 完成、1 次预期 HTTP 超限失败）、5 次识图（4 完成、1 次预期超限失败）。两项新增格式拒绝的 HTTP 均完整读取，任务失败另行核对。Go 格式、差异空白与 101 份公开文档链接检查通过。使用合成数据和回环假模型/OneBot，未发送真实 QQ 消息，日常服务未部署，不替代真实模型和 QQ 客户端验收。

## Agent 动作回复整批格式验证（2026-10-15）

动作预检改为逐项 Decode 直到 EOF，修正 Decoder.More 在多余闭括号处提前结束、可能接受有效前缀的问题。首个结束动作之前，所有项目必须是可解码为 APIRequest 的 JSON 对象；不完整 JSON、多余括号、说明文字、非对象值、字段类型错误及无效 UTF-8 返回固定 ErrActionOutputInvalid 和空请求，在自动请求/记忆/结束标记写入前拒绝。两次解码均保持精确数字语义，代码围栏、纯空白、空对象和首个结束动作兼容；权限规则保持。宿主保存 model_output_invalid，列表及单条读取显示相同原因，不派发前缀、不重试或兜底，不回滚此前轮次。使用入口见[任务说明](agent-tasks.md)。

SDK 新增 2 项测试，覆盖 16 类格式/兼容输入及 8 类坏后缀在自动/手动模式下的 16 个零写入用例；包含超过 2^53 的编号、极大数值字面量、首个结束动作后的忽略内容、UTF-8、空白和空对象。实际任务存储测试增加多余闭括号、不完整 JSON 和 null 三项，核对一次模型准备、零派发/查询、准确终态和零请求/响应/结束归档。脚本增加合成坏格式判定测试，容量与格式错误类别不能互相代替。

Go 回归通过：SDK 3.387 秒，chat 16.632 秒，localadmin 22.000 秒，aichat 3.976 秒，modelstats 3.639 秒，显式脚本 2.921 秒。离线 Chrome 新增列表及按编号核对的格式失败提示测试，无重跑按钮或额外请求；档案、记忆与任务合计 54 项通过、0 失败（43.137 秒）。Go 格式、差异空白及 101 份公开文档链接检查通过。

复验命令：在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；仓库根执行 `go test ./internal/localadmin ./plugin/aichat ./internal/modelstats -count=1 -timeout=180s`、`go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s` 和 `node --test scripts/test-agent-archive-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-tasks-ui.mjs`。

最终构建 `.local/bin/zerobot-agent-action-syntax.exe` 成功，CGO_ENABLED=0、trimpath，44,293,632 字节。模块缓存统计文件写入提示权限不足，但构建退出为 0 并产出本次程序。随后执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-action-syntax.exe`，完整隔离整机验证通过，完成于 2026-10-15 11:32:09 +08:00。本机产物 `.local/smoke-admin/run-1313682501/result.json` 中 `agentMalformedActionBatchRejectedPassed=true`，原输出限额标记也通过，且 `passed=true`、`isolated=true`、`realQQMessagesSent=false`。

实际插件分别收到有效保存/发送前缀后的多余闭括号、不完整 JSON 和 null。三项任务均持久化为 failed/model_output_invalid，一次准备、零动作/查询/兜底；列表与单条的原因和版本一致，记忆列表为空。随后同会话正常新消息成功发送一次并完成。原容量、识图、准备停止、记忆、来源、权限及后台流程全部通过。

模型统计合计 43 次模拟请求：41 次 HTTP 完成、2 次原有预期超限失败、0 活跃/未完成；分类为 4 次 GPT/群总结、3 次普通聊天、31 次 Agent（30 完成、1 次预期 HTTP 超限失败）、5 次识图（4 完成、1 次预期超限失败）。三项格式拒绝的 HTTP 均完整返回，任务失败单独核对，不能混为 HTTP 失败。测试使用合成数据与回环假模型/OneBot，未发送真实 QQ 消息，日常服务未部署，不替代真实模型与 QQ 客户端验收。

## Agent 单轮输出与动作批次限制验证（2026-10-15）

Agent 宿主响应向解析器最多提供 2 MiB，未知长度最多额外探测 1 字节；SDK 完整回复最多 1 MiB，解析到首个结束标记为止最多 32 个 JSON 对象，结束与空对象也计入。数量预检在自动请求/记忆/结束标记写入前完成，超限不返回可执行前缀，不截断后继续派发。HTTP 超限映射为相同 model_output_limit 任务原因，页面列表和单条均明确显示。原有无固定总迭代次数、权限/范围过滤、取消及故障优先级保留，见[任务说明](agent-tasks.md)。

SDK 新增 2 项测试覆盖正文精确上限及多一字节、32 对象边界、结束/空对象计数、代码围栏、首个 end_action 之后不执行，以及超限前已有自动记忆保存动作时仍零保存/归档。完整 SDK 回归通过（1.383 秒）。宿主插件新增 1 项测试含 4 个子场景，验证高于识图限额但低于 Agent 限额仍可读、精确上限、声明/流式超限、响应关闭及失败分类；既有识图的 gzip/分块/重定向测试在共享有界读取实现上同时通过。

chat 新增 1 项测试含对象和正文两种场景，接入实际任务、历史及 RunAgent，确认一次模型准备、零派发/查询、零请求/响应/结束归档，错误分类正确且不重试；原超过八轮运行测试同时通过。请求计数改为原子变量后，完整 chat 再次回归通过（11.548 秒）。localadmin、aichat、modelstats 完整 Go 回归通过（34.069、8.500、0.282 秒），显式脚本判定测试通过（1.192 秒），新增 1 项验证批次前缀、容量阈值及失败摘要反向场景。

档案、记忆、任务共 53 项离线 Chrome 回归通过，新增 1 项核对列表和按编号读取显示输出超限原因、没有重跑/停止控件且只发起读取请求。最终构建 `.local/bin/zerobot-agent-output-limits.exe` 成功，CGO_ENABLED=0、trimpath；模块缓存统计文件写入提示权限不足，但构建退出为 0 并产出本次程序。

复验：在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录执行 `go test ./internal/localadmin ./plugin/aichat ./internal/modelstats -count=1 -timeout=180s`、`go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s` 及 `node --test scripts/test-agent-archive-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-tasks-ui.mjs`。

随后执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-output-limits.exe`，完整隔离整机验证通过，完成于 2026-10-15 11:19:17 +08:00。本机产物为 `.local/smoke-admin/run-2703079924/result.json`，新增 `agentActionOutputBatchAndResponseLimitsPassed=true`，且 `passed=true`、`isolated=true`、`realQQMessagesSent=false`。

实际插件连续接收三类坏输出：33 个对象且首个为记忆保存、超过 1 MiB 的正文且带记忆保存前缀、超过 2 MiB 的 HTTP 协议体。三项任务均为已保存的 failed/model_output_limit，一次模型准备、零动作/查询/兜底，列表及单条一致；记忆列表为空，未保存前缀。随后同会话的新消息正常执行一次发送并完成，没有重跑失败任务。

模型统计合计 40 次模拟请求：38 次 HTTP 完成、2 次预期响应超限失败、0 活跃/未完成。分类为 4 次 GPT/群总结、3 次普通聊天、28 次 Agent（27 完成、1 次预期 HTTP 超限失败）、5 次识图（4 完成、1 次原有预期超限失败）。对象和回复正文超限的两个 HTTP 仍记为完成，但对应任务明确失败。原有识图、准备停止、记忆、来源、查询、权限及后台流程全部通过。Go 格式、差异空白及 101 份公开文档链接检查通过。

HTTP 完成与动作解析成功分别统计，单轮上限不等于任务总动作或迭代次数上限。测试使用合成数据及回环假模型/OneBot，未发送真实 QQ 消息，日常服务未部署，不替代真实模型及 QQ 客户端验收。

## Agent 识图响应及描述边界验证（2026-10-15）

识图宿主 HTTP 客户端为解析器提供最多 256 KiB 响应，未知长度最多多探测 1 字节，覆盖声明长度、流式、透明解压及错误响应，保留原客户端超时、重定向、cookie jar 和连接关闭能力；响应超限由外层模型统计记为失败。SDK 描述先去首尾空白，空白、非法 UTF-8 或 NUL 结果不附加/缓存，每张最多 4,000 UTF-8 字节，在完整字符处截断。缓存同时保留片段标记，已有描述重新检查，正常结果清理旧标记；提示说明省略部分不得当作事实，识图失败日志不输出接口错误正文。使用入口见[识图说明](agent-vision.md)。

SDK 新增 2 项测试覆盖空白/非法字节/NUL、首尾空白、ASCII/中文/emoji 字节边界、精确上限、截断标记清理与保留、超长缓存、空白后恢复及已有描述复核。初次 SDK 回归因模板编辑新增了末尾换行，提示快照差一行而失败；保留原模板末尾格式后完整回归通过（1.237 秒），首次失败不计为通过。插件新增 2 项 HTTP 测试含 5 个容量子用例，验证声明超限零正文读取、精确上限、流式超限、错误正文、空正文、超限后继续拒绝、响应体关闭、超限最多探测一字节和模型统计分类；回环 gzip、分块、重定向及客户端设置验证通过。

SDK、chat、localadmin、aichat、modelstats 完整 Go 回归通过（1.237、12.723、23.316、4.353、0.569 秒）。脚本新增 1 项恢复判定测试并调整已有识图测试，验证超长中文只有 3,999 字节完整字符片段且带截断标记；拒绝空白/超限图补造描述、旧恢复掩盖当前缺失、陈旧标记、额外识图调用及错误格式。显式脚本测试通过（1.150 秒）。

复验：在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；仓库根执行 `go test ./internal/localadmin ./plugin/aichat ./internal/modelstats -count=1 -timeout=180s` 及 `go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s`。本轮未改页面和后台 HTTP 参数，未重复离线 Chrome，最近 52 项记录保留原验证范围。

最终构建 `.local/bin/zerobot-agent-vision-description.exe` 成功，CGO_ENABLED=0、trimpath；模块缓存统计文件写入提示权限不足，但构建退出为 0 并产出本次程序。随后执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-vision-description.exe`，完整隔离整机验证通过，完成于 2026-10-15 11:01:00 +08:00。本机产物为 `.local/smoke-admin/run-2791122515/result.json`，新增 `agentVisionDescriptionAndResponseLimitsPassed=true`，且 `passed=true`、`isolated=true`、`realQQMessagesSent=false`。

原识图流程实际返回超长中文，Agent 首次及缓存复用时均仅取得 3,999 字节完整汉字及截断标记，未取得末尾内容。另一独立合成群发送三张相同内容、不同地址图片，识图接口依次返回空白、超过 256 KiB 的正文和正常描述；第一轮前两张不附加描述，第三张正常恢复，下一任务三张均复用正常缓存，不增加识图请求。九次图片下载、三次识图 HTTP 和两次 Agent 请求、两个完成任务的准备/动作/查询摘要全部核对。

整机合计 36 次模拟请求：35 次 HTTP 完成、1 次预期超限失败、0 活跃/未完成；分类为 4 次 GPT/群总结、3 次普通聊天、24 次 Agent、5 次识图，其中识图 4 次完成、1 次预期失败。HTTP 200 空白结果仍属于 HTTP 完成，不等于识图成功。原有准备期间停止、输入限制、记忆、来源、查询、权限及后台回归全部通过。Go 格式、差异空白与 101 份公开文档链接检查通过。

单图描述限额不代表完整上下文或 token 限额；测试使用合成图片和回环假模型/OneBot，未发送真实 QQ 消息，日常服务未部署，不替代真实识图效果和 QQ 客户端验收。

## Agent 准备期间停止验证（2026-10-15）

修正准备期间已请求停止仍继续后续图片及 Agent 请求的问题：SDK 新增 GetActionChecked，任务检查仅属于本次调用，在准备入口、记忆读取后、事件预处理、图片下载前后、识图调用前后和 Agent HTTP 前后检查，首次检查错误保留。识图返回后发现停止，不新增描述缓存；Agent 返回后发现停止，不解析动作或登记结束标记。原接口及空检查调用兼容，非空检查要求 HistoryStore；视觉协议改为每次局部 Clone，避免修改共享模板。宿主绑定当前 run.Check 与设置代次，读取和终态仍使用原存储及权限规则。使用步骤与实际限制见[任务控制](agent-tasks.md)。

SDK 新增 3 项测试，覆盖准备入口/记忆/历史三个边界、首次检查失败后不恢复、图片下载及识图 HTTP 两种在途阻塞、后续事件/图片/Agent 请求不执行、停止后不缓存描述、Agent 回应取消后无结束标记、12 路并发会话检查隔离与共用识图协议、旧调用及无持久历史的明确拒绝。完整 SDK 回归通过，补充首次错误和缓存断言后最终通过（1.342 秒）。

chat 新增 1 项测试含 3 个子场景，接入实际历史及任务存储，覆盖下载中停止、识图中停止和配置代次变化，确认只登记一次模型准备，零动作/查询，取消时间与 cancelled 终态或 settings_changed 失败分类正确。初次编译因宿主依赖的模型构造函数多一个参数而失败，修正测试构造调用后完整 chat 回归通过（6.891 秒），首次失败不计为通过。localadmin 与 aichat 完整 Go 回归通过（25.453、1.914 秒），整机脚本新增 1 项反向判定测试，拒绝未保存、非取消、缺失停止时间、错误兜底次数或动作结果，显式脚本测试通过（1.295 秒）。

复验命令：在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录执行 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s`、`go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s`。本轮未改页面及后台 HTTP 参数，未重复离线 Chrome，最近 52 项结果保持原范围。

最终构建 `.local/bin/zerobot-agent-preparation-stop.exe` 成功，CGO_ENABLED=0、trimpath；模块缓存统计文件写入提示权限不足，但构建退出为 0 并产出本次程序。随后执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-preparation-stop.exe`，完整隔离整机验证通过，完成于 2026-10-15 10:42:57 +08:00。本机产物为 `.local/smoke-admin/run-1964662746/result.json`，新增 `agentPreparationStopBeforeNextRequestPassed=true`，且 `passed=true`、`isolated=true`、`realQQMessagesSent=false`。

两个独立合成群分别确认图片下载和识图 HTTP 已在途后停止：前者当前下载返回后没有识图请求，后者保留一次已发出的识图请求；二者都不下载下一张图、不调用 Agent 模型、不发送回复、不执行查询或兜底。两个终态均为 cancelled、一次准备登记且零动作/结果/查询，首次停止时间保留，单条读取及重复停止核对通过。图片入口合计两次，后续图片入口零次，额外识图模型一次；31 次模型 HTTP 统计分类为 4 次 GPT/群总结、3 次普通聊天、22 次 Agent、2 次识图，原有识图缓存、记忆、来源、查询、平台在途停止、角色权限和后台回归全部通过。

Go 格式、差异空白及 101 份公开文档链接检查通过。当前在途 HTTP 不立即中断，检查点之间也不承诺原子派发。测试使用合成数据及回环假图片/模型/OneBot，未发送真实 QQ 消息，日常服务尚未部署，不替代真实模型或 QQ 客户端验收。

## Agent 识图输入边界验证（2026-10-14）

修正 Agent 图片下载无超时及无界读入的问题：每次 HTTP(S) 下载设 10 秒上下文期限，覆盖响应头、重定向和正文；状态非 200、空正文及读取错误跳过。声明长度和流式正文上限为 8 MiB，透明解压同样有界，base64 解码前后限制容量。完整图片解码前先读取尺寸，边长不超过 8192，像素总量不超过 16,777,216。异常输入不调用当前图的识图模型、不缓存描述，原有缩放和相似缓存保留。使用说明见[识图输入](agent-vision.md)。

新增 4 项 SDK 测试覆盖正常 PNG/JPEG、HTTP 重定向、空/无效/本机路径输入、base64 和 HTTP 恰好字节上限及超过一字节、超限边长/像素、破损图片、失败状态、声明超限、分块及 gzip 超限、慢响应头/正文期限、生产 10 秒期限、响应体关闭和最多读取上限加一字节。实际预处理接入假识图模型，混合无效图、尺寸超限图和正常图，仅正常图取得描述；另一事件复用相似缓存不重复请求，清除识图 API 后保留原行为。输入错误不包含地址、错误正文或内嵌图片数据。

完整 SDK、chat、localadmin、aichat Go 回归均通过（2.159、16.334、36.249、7.372 秒）。脚本新增 1 项判定测试，验证嵌入图片及格式、尺寸、有效描述对应当前实际图片，拒绝远程 URL 替代、坏数据、格式伪造、失效描述、给失败图补造描述及旧成功掩盖新失败。依据既有重新编码实现，判定同时接受 PNG 和实际 JPEG，脚本最终复验通过（2.225 秒）。

复验：在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录执行 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s` 及 `go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s`。本轮未修改页面或后台 HTTP 参数，未重复离线 Chrome，上一轮 52 项结果保持原验证范围。

最终构建 `.local/bin/zerobot-agent-vision-input.exe` 成功，CGO_ENABLED=0、trimpath；模块缓存统计文件写入提示权限不足，但退出码为 0 并产出本次程序。随后执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-vision-input.exe`，完整隔离整机验证通过，完成于 2026-10-14 22:56:42 +08:00。本机产物为 `.local/smoke-admin/run-92071825/result.json`，新增 `agentVisionBoundedInputAndCachePassed=true`，且 `passed=true`、`isolated=true`、`realQQMessagesSent=false`。

两个实际 Agent 任务分别发送同样的混合图片事件：正常 HTTP PNG 进入识图模型，超限 HTTP 图片、无效 base64 和本机路径不获得描述。识图模型取得实际重新编码的图片后返回描述，Agent 上下文必须含该描述且只对应正常图片，才回复；第二次触发命中缓存，不重复请求识图模型。保留的历史图片仍加载，正常及超限 HTTP 图片各下载 3 次，识图请求仅 1 次，Agent 请求 2 次。两个任务均已完成，各登记一次模型准备及一次平台发送，查询额度未使用，成功回执各一次。

模型统计核对 30 次模拟请求，分类为 4 次 GPT/群总结、3 次普通聊天、22 次 Agent 和 1 次识图；原有角色、档案、记忆、保存回执、查询、停止与后台回归全部通过。Go 格式、差异空白及 101 份公开文档链接检查通过。测试使用合成图片和回环假模型/OneBot，不发送真实 QQ 消息，未部署日常服务，不替代真实模型识图效果、QQ 客户端体验或持续业务负载验收。

## 任务阶段及首次停止请求时间验证（2026-10-14）

任务新增可选 UTC 纳秒字符串 phaseStartedAt/cancelRequestedAt，分别记录当前阶段登记及首次有效停止请求。进入新一轮同类模型准备更新时间，摘要更新、只读查询及重复停止保留原时间；终态与重启恢复阶段时间等于 endedAt，恢复时刻不代表实际退出时刻。初次状态写入失败的内存摘要也明确进入结束阶段，仍未保存并保留会话锁。旧终态时间缺失保持未知，页面不展示未可靠保存的这两项时间。

新增 4 项 chat 测试覆盖连续同类模型阶段、上下文/记忆/查询摘要不改变阶段时间、只读列表与单条一致、重复结束、16 路并发停止、过期版本拒绝、取消后的在途结果、重启保留首次停止时间、重复恢复幂等、旧运行/终态缺失字段、停止写失败时内存/磁盘区别及初次写失败的故障会话锁。完整 chat 回归首次通过，补充旧终态和初次写失败用例后再次通过（17.095 秒）。后台单条 HTTP 测试验证两项纳秒文字原样传递，localadmin 和 aichat 完整回归通过（39.752 秒、3.855 秒）。

档案、记忆、任务共 52 项离线 Chrome 回归通过，新增 2 项覆盖列表和按编号读取一致、无效/缺失/未保存时间、重启时刻说明、纯文本及 320px 深浅主题。窄屏样例调整为实际结束阶段后两项定向复验通过，已查看两种主题截图，文字换行正常。脚本新增 1 项时间判定反向测试，显式 Go 脚本测试通过（1.854 秒）。

复验命令：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；仓库根执行 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s`、`go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s` 和 `node --test scripts/test-agent-archive-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-tasks-ui.mjs`。定向页面命令为 `node --test --test-name-pattern='task phase and first stop|task timestamps wrap' scripts/test-agent-tasks-ui.mjs`。

最终构建 `.local/bin/zerobot-agent-task-timing.exe` 成功，CGO_ENABLED=0、trimpath；模块缓存统计文件写入出现权限提示，但构建退出为 0 并产出本次程序。随后执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-task-timing.exe`，完整隔离整机验证通过，完成于 2026-10-14 22:19:03 +08:00。本机产物为 `.local/smoke-admin/run-2025692512/result.json`，`passed=true`、`isolated=true`、`realQQMessagesSent=false`，新增 `agentTaskPhaseAndStopTimesPassed=true`。

实际模型、平台动作及普通聊天兜底请求在途时，列表和单条核对阶段开始时间一致；停止不重置阶段时间，重复停止保持首次请求时间，终态阶段时间等于 endedAt，列表与单条保留首次停止时间。27 次模拟模型请求、9 个完成/3 个取消/0 个运行任务及原有来源、保存回执、查询、记忆和后台回归均通过。Go 格式、差异空白及 100 份公开文档链接检查通过。使用说明见[任务控制](agent-tasks.md)，复验步骤见[隔离整机自测](agent-integration-test.md)。本轮未部署日常服务，未发送真实 QQ 消息，不替代真实模型效果、客户端体验或持续负载验收。

## Agent 记忆保存回执与精确读取验证（2026-10-14）

新增 SaveFromSequenceResult：事务提交后返回新建引用，同文启用记忆返回复用引用，包含模式及字符串编号、当前修订和原来源，不重复携带正文。消息入口把成功回执入档给后续模型，入档失败不会登记本地成功或自动重试。旧错误接口兼容，旧文本文件沿用追加和脱敏，只返回 legacy 模式，不补造编号。chat、go-onebot-agent、localadmin、aichat 完整 Go 回归及整机脚本判定测试全部通过。

新增 3 项存储测试覆盖大整数编号/来源、创建与复用、返回值不影响存储、版本/来源/时间保留、后台修订来源、后续旧版本读取失效、停用后新建、机器人及群/私聊隔离、无正文或密钥的回执、24 路并发只报一次新建、首个修订写失败回滚且不返回成功引用、损坏引用拒绝、旧文件追加/脱敏及配置失败不改文件。SDK 动作说明标为宿主可选回执，提示与角色表预期同步，通用 SDK 自动保存仍可返回 null。首次 chat 测试因自动权限审核超时未启动，按工具提示重试后通过，不算代码测试失败或首轮通过。

脚本新增 2 项判定测试，验证从实际新建/复用回执取得大整数字符串编号和版本，后续精确读取的身份、来源和正文必须一致；缺失新建、错误复用、错误编号、最新失败响应或其他条目的正文均拒绝。来源片段计数用前后增量核对，拒绝错误增量、未确认或裁剪后的上下文。既有 chat 来源计数测试补充保存元数据响应，确认不计为记忆正文片段，完整 chat 回归再次通过。

复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；根目录执行 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s` 与 `go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s`。补充来源断言后 chat 和脚本测试完整复跑通过。

最终构建 `.local/bin/zerobot-agent-memory-receipt.exe` 成功，退出码 0，模块缓存权限警告未影响产物。首轮整机在新增上下文来源断言处失败：该计数统计实际装入的全部历史响应，先前已有 3 个片段，本次读取新增 1 个，最终为 4，不能假定新任务仅有 1 个。隔离数据库的只读核对确认三轮保存/读取、三次本地动作和一次查询已完成。脚本改为比较保存前后的已加载上下文增量并拒绝裁剪/未统计情况；首轮不计为整机通过，未因该失败修改应用程序。

随后用同一最终构建执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-memory-receipt.exe`，完整隔离整机回归通过，完成时间为 2026-10-14 20:37:40 +08:00，本机结果位于 `.local/smoke-admin/run-807996006/result.json`。三轮流程先连续保存同文，回执先新建后复用同一引用；模型按实际返回编号/修订读取，身份、来源和正文一致后才回复。数据库只保留一条，来源与本次任务触发序号一致，累计 3 次模型准备、3 次本地动作、1 次平台动作，仅使用 1 次记忆读取额度；新增元数据不增加记忆正文片段，前后计数从 3 增至 4。

新增 `agentMemorySaveReferenceRoundTripPassed=true`，模拟模型合计 27 次，其中知识流程 12 次，模型统计和 9 个完成/3 个取消/0 个运行任务筛选均核对通过，原来源、查询、记忆及后台流程同时通过。入口见[记忆保存回执](agent-memory.md)，前提见[隔离自测](agent-integration-test.md)。结果 `passed=true`、`isolated=true`、`realQQMessagesSent=false`，Go 格式、差异空白和 100 份公开文档链接检查通过。本轮未改页面及后台 HTTP 参数，未重复离线 Chrome，最近 50 项结果保留在下方。验证使用合成数据和回环假模型/OneBot，未部署日常服务，不替代真实模型效果、客户端体验或持续负载验收。

## 任务查询额度与覆盖摘要验证（2026-10-14）

任务新增共享查询额度、四类查询次数、成功响应、空结果、覆盖不完整、参数/读取错误和超额拒绝摘要。查询前保存实际已用额度，成功响应入档后结算覆盖；查询器绑定活动任务账号及会话，重建沿用原额度，旧任务不推断统计。修正响应入档失败仍可能保留成功 Outcome 的问题。chat、localadmin、aichat 完整 Go 回归与整机脚本判定测试均通过。

新增 3 项存储/接入测试及 4 个故障子用例覆盖查询执行前的可见额度、三次共享上限及第四次拒绝、查询器重建不重置、机器人/群/私聊绑定、结果数量裁剪、空结果和 partial 重叠、参数错误、结果持久化与筛选读取、任务摘要不保留关键词/正文、新任务零值、取消后保存在途查询真实结果、旧 payload 缺失，以及非法计数。额度写失败不执行查询，响应写失败不登记成功，读取错误不伪装为空；摘要写失败标为未保存，重启只恢复最后可靠额度和未确认状态，不补造成功。

后台按编号读取增加查询摘要传递断言，脚本新增 1 项判定测试拒绝未保存、数字型错误、错误额度、缺失成功/空结果及意外错误/拒绝计数。档案、记忆和任务共 50 项离线 Chrome 回归全部通过，新增 2 项覆盖列表与单条展示、额度及各类统计、实测零、旧/非法/未保存摘要、未确认结果、范围清理和 320px 深浅主题布局；已查看新增摘要的两种主题截图，完整段落换行正常。

复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录执行 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s`、`go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s` 和 `node --test scripts/test-agent-tasks-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs`，全部退出 0。

最终构建 `.local/bin/zerobot-agent-task-queries.exe` 成功，退出码 0，模块缓存权限警告未影响产物。执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-task-queries.exe` 后完整隔离整机回归通过，完成时间为 2026-10-14 17:23:15 +08:00，本机结果位于 `.local/smoke-admin/run-3039015613/result.json`。角色实际历史查询计一次、其他角色及取消流程不虚构查询；四轮记忆任务计两次检索和一次读取，历史任务计检索及读取，二者覆盖不完整均有计数。旧修订精确读取计一次成功空响应。新增 `agentTaskQueryUsageAndCoveragePassed=true`，原任务结果、来源、记忆和后台流程同时通过。

使用步骤及统计限制见[任务说明](agent-tasks.md)，运行前提见[隔离自测](agent-integration-test.md)。结果 `passed=true`、`isolated=true`、`realQQMessagesSent=false`，Go 格式、JavaScript 语法、差异空白和 100 份公开文档链接检查通过。错误、额度拒绝和重启未确认统计由模块测试覆盖，整机核对正常响应、空结果、partial 和取消；成功响应不证明模型阅读全部资料。本轮没有修改模型动作协议，未重复 go-onebot-agent 模块测试。验证使用合成数据及回环假模型/OneBot，未部署日常服务，不替代真实模型效果、客户端体验或持续负载验收。

## 任务累计动作结果验证（2026-10-14）

任务新增已结算的成功回执、平台拒绝、结果未知和本地失败计数，独立于原模型准备及动作登记次数。列表及单条摘要共用显示，旧记录不推断、实测零与未知分开、存储故障隐藏计数。chat、localadmin、aichat 完整 Go 回归和整机脚本判定测试全部通过。

新增 3 项存储测试及故障子用例验证四类累计结果、非法/未登记/重复结果拒绝、未结算动作不能被下一模型或动作覆盖、版本不被拒绝调用改变、24 路并发结果只接受一次、取消后成功回执保留、重复取消/结束不增加、结束缺失回执计为未知、模型失败不计为动作、新任务与群/私聊隔离、单条及筛选持久读取、返回值不影响内部状态、重启只记一次未知、旧 payload 不补造结果，以及结果写失败后内存不宣称持久、重启按最后可靠记录恢复未知。后台单条接口新增计数传递断言。

档案、记忆与任务共 48 项离线 Chrome 回归全部通过。新增 2 项验证列表和单条读取、四类结果、实测零、在途提示、旧/非法/未保存摘要、范围清理及 320px 深浅主题布局。首次截图只展示部分新增摘要，调整截图滚动位置后窄屏用例定向复验通过，已查看两种主题的完整结果段落；这是测试截图改进，页面代码未再次修改。

复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录执行 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s`、`go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s` 与 `node --test scripts/test-agent-tasks-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs`。窄屏定向入口为 `node --test --test-name-pattern='settled result summaries' scripts/test-agent-tasks-ui.mjs`。新增脚本判定测试拒绝重复、未保存和非法/缺失计数。

最终构建 `.local/bin/zerobot-agent-task-results.exe` 成功，退出码 0，模块缓存权限警告未影响产物。执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-task-results.exe` 后完整隔离整机回归通过，完成时间为 2026-10-14 16:58:45 +08:00，本机结果位于 `.local/smoke-admin/run-3152851632/result.json`。五个角色任务及多轮记忆/历史查询任务的成功回执与实际动作登记一致；实际模型和兜底取消保持零动作结果，平台动作在途取消后保留一次成功回执，列表与按编号读取一致，重复取消不重计。新增 `agentTaskSettledResultSummaryPassed=true`，原来源、检索、记忆及后台流程同时通过。

入口和计数限制见[任务说明](agent-tasks.md)，整机前提见[隔离自测](agent-integration-test.md)。结果 `passed=true`、`isolated=true`、`realQQMessagesSent=false`，Go 格式、JavaScript 语法、差异空白和 100 份公开文档链接检查通过。本轮未修改模型动作协议，未重复 go-onebot-agent 模块测试。测试使用合成数据和回环假模型/OneBot；平台拒绝、本地失败和重启故障计数由模块测试覆盖，整机主要核对成功和取消路径。未部署日常服务，不替代真实模型效果、客户端体验或持续负载验收。

## Agent 记忆准确触发来源验证（2026-10-14）

消息入口在任务启动时捕获一次准确档案序号，任务与新建独立记忆使用同一来源。MemoryStore.SaveFromSequence 校验同会话事件，不再以重复平台消息编号或最新事件猜测；旧 Save/SaveFrom 调用仍可保存，来源明确未知。chat、localadmin、aichat 完整 Go 回归及整机脚本判定测试全部通过。

新增 2 项存储测试覆盖大于 JavaScript 安全整数的准确序号、重复消息编号和后来的事件、跨机器人/群/私聊及非事件来源拒绝、非法和缺失来源、去重不能掩盖来源错误、重置后保留来源、5,001 条后续事件之后仍可准确绑定、初始修订、关闭并重开数据库、24 路并发保存及首次元数据保留、兼容调用未知来源、修订写失败回滚和档案读失败。既有来源读取、检索和恢复夹具改为传入已知准确序号，原预期保持；未对历史持久记忆自动改绑。

脚本新增 1 项判定测试，接受准确大整数字符串，并拒绝其他任务来源、数字型/非法编号、非事件、错误正文和非 Agent 来源。复验命令：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录执行 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s` 和 `go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s`，均退出 0。

最终构建 `.local/bin/zerobot-agent-memory-source.exe` 成功，退出码 0，模块缓存权限警告未影响产物。执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-memory-source.exe` 后完整隔离整机回归通过，完成时间为 2026-10-14 16:45:21 +08:00，本机结果位于 `.local/smoke-admin/run-2474744122/result.json`。五个角色事件刻意使用相同平台消息编号，三个相同保存仍只有一条记忆，该记忆准确来源与首次保存任务一致；按序号读取到实际触发事件，后续重复保存不改来源。新增 `agentMemoryExactTriggerSourcePassed=true`，任务触发来源与原检索、记忆、取消及后台流程同时通过。

使用入口见[记忆保存与来源核对](agent-memory.md)，整机前提见[隔离自测](agent-integration-test.md)。结果 `passed=true`、`isolated=true`、`realQQMessagesSent=false`，Go 格式、差异空白和 100 份公开文档的链接检查通过。本轮没有修改页面或模型动作协议，未重复离线 Chrome 和 go-onebot-agent 模块测试，其最近结果保留在下方。验证使用合成数据及回环假模型/OneBot，未部署日常服务，不替代真实模型效果、客户端体验或持续负载验收。

## 任务触发消息来源核对验证（2026-10-14）

任务接入当前事件入档返回的准确序号，随摘要持久化；新增后台列表和单条只读摘要中的“查看触发消息”，按原机器人及会话读取对应事件，包含重置前记录。旧任务没有来源时明确显示未知，不根据消息编号或最新档案补造。chat、localadmin、aichat 完整 Go 回归及脚本判定测试均通过。

新增 2 项存储与事件绑定测试覆盖重复平台消息编号、跨群/私聊及非事件来源拒绝、首次绑定后出现新事件、重置与重启恢复、未知及旧 payload、存储失败、24 路并发准确入档序号，以及超过 JavaScript 安全整数范围的序号。事件绑定同时核对原事件指针、机器人和会话，防止其他事件沿用来源。

档案、记忆与任务共 46 项离线 Chrome 回归全部通过。新增 3 项覆盖已加载范围和大整数字符串、保留任务筛选与分页、单条结果仅允许来源读取、旧任务未知来源、档案缺失/不可用/非事件/读取失败、纯文本展示、退出或切换后的迟到响应，以及刷新清理。已查看 320px 深浅主题截图，长任务编号换行正常，HTML 内容按纯文本展示。

复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录执行 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s`、`go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s` 和 `node --test scripts/test-agent-tasks-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs`。JavaScript 语法、Go 格式、差异空白及文档链接检查通过。

最终构建 `.local/bin/zerobot-agent-task-source.exe` 成功，退出码 0，模块缓存权限警告未影响产物。执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-task-source.exe` 后完整隔离整机回归通过，完成时间为 2026-10-14 16:01:24 +08:00，本机结果位于 `.local/smoke-admin/run-397433009/result.json`。五个角色任务均按实际来源序号读取角色触发事件，四轮记忆查询任务的来源正文含准确触发消息；新增 `agentTaskTriggerSourceArchivePassed=true`，原有查询、记忆、任务取消和后台流程同时通过。

使用步骤见[任务触发来源说明](agent-tasks.md)，整机前提见[隔离自测](agent-integration-test.md)。结果 `passed=true`、`isolated=true`、`realQQMessagesSent=false`。本轮未改变模型动作协议，未重复 go-onebot-agent 模块测试。测试使用合成数据和回环假模型/OneBot，未部署日常服务，不替代真实模型效果、客户端体验或持续负载验收。

## 单条任务最新状态核对验证（2026-10-14）

新增 TaskStore.Read、后台 GET /api/agent/tasks/read 与页面按编号只读核对。chat、localadmin、aichat 完整 Go 回归及脚本判定测试通过，补充 HEAD 接口测试后任务接口定向复验通过。新增 2 项存储测试（含首次/终态写失败子用例）覆盖超过 50 个首页条目的旧任务、机器人及群/私聊隔离、当前内存状态、返回可选计数指针不共享内部状态、版本不被读取改变、取消上下文、非法编号、损坏身份与数据库故障，以及重启中断和在途结果未知。

新增接口测试验证认证、连接账号、完整范围、小写十六进制任务编号、分页/筛选/版本混用拒绝、字符串版本精度、no-store、不存在 404 与读取故障 503；HEAD 保持只读，不进入取消方法。首次写入失败的存储用例起初错误使用了不存在的可运行句柄，已改为通过公开列表取得可见故障编号后读取，完整 chat 回归复验通过。

档案、记忆与任务共 43 项离线 Chrome 用例分次通过：首轮 42 项通过，1 项因为模拟接口把任务自身 error 字段当作 HTTP 错误而失败；修正模拟响应后 14 项任务页面用例全部复跑通过。新增 3 项覆盖固定已加载范围、查询不改变列表/筛选/游标、摘要只读且无停止按钮、大整数版本、非法输入不发请求、不存在/读取失败/错误身份与版本不显示成功、旧登录迟到响应清理、未落盘故障摘要和 320px 深浅主题布局；已查看窄屏截图。模拟器可区分成功返回的失败任务与接口读取失败，首轮失败项不计为通过。

复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录执行 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s`、`go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s` 与 `node --test scripts/test-agent-tasks-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs`。补充复验为 `go test ./internal/localadmin -run TestAgentTask -count=1 -timeout=120s` 和 `node --test scripts/test-agent-tasks-ui.mjs`。

最终构建 `.local/bin/zerobot-agent-task-read.exe` 成功，退出码 0；模块缓存权限警告未影响产物。执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-task-read.exe` 后完整隔离整机回归通过，完成时间为 2026-10-14 15:32:44 +08:00，本机结果位于 `.local/smoke-admin/run-3721886076/result.json`。实际模型、平台动作及兜底请求在途时均可按编号读取运行任务；取消后读取版本与终态一致，读取较早已结束任务保留历史来源摘要，同号私聊与其他群查询该编号返回 404。新增 `agentTaskExactReadScopeAndTerminalPassed=true`，原查询、记忆、任务控制和后台流程同时通过。

使用步骤见[任务核对说明](agent-tasks.md)，整机前提见[隔离自测](agent-integration-test.md)。结果 `passed=true`、`isolated=true`、`realQQMessagesSent=false`，差异空白、Go 格式、JavaScript 语法及文档链接检查通过。本轮未改变模型动作协议，未重复 go-onebot-agent 模块测试。使用合成数据与回环假模型/OneBot，未部署日常服务或发送真实 QQ 消息，不替代真实模型、客户端体验及负载验收。

## 后台记忆编号边界搜索验证（2026-10-14）

后台 POST 记忆搜索和页面接入可选 before，结果支持“查此条之前”。localadmin 与 aichat 全部 Go 回归、脚本判定的 2 项 Go 测试通过。新增接口用例验证登录/CSRF、未知机器人、严格正整数字符串和最大 int64 精度、私聊负内部会话、无边界兼容、no-store、搜索限流、读取故障及 GET/新增/修订/迁移拒绝边界混用；非法输入未进入存储。

档案、记忆和任务共 40 项离线 Chrome 回归通过。新增 3 项页面用例涵盖大整数边界、排他范围显示、实际关键词与条件回填、结果按钮忽略输入草稿但保留已加载会话、原列表/筛选/分页保留、非法值不发请求、重新读取/切换/退出清理、迟到旧登录响应不回填和 320px 深浅主题布局。补充 128 个连续英文字符的窄屏断言后定向复验通过，已查看深浅主题截图；状态文字完整保留关键词并可换行。

复验命令：根目录 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s`、`go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s` 与 `node --test scripts/test-agent-memory-ui.mjs scripts/test-agent-tasks-ui.mjs scripts/test-agent-archive-ui.mjs`。窄屏补充复验为 `node --test --test-name-pattern='320px bounded memory search' scripts/test-agent-memory-ui.mjs`。本轮未改存储查询与 Agent 动作协议，未重复依赖模块测试，其最近结果保留在下方记录中。

首轮整机在模型统计固定次数处失败：旧取消用例仅等待 model_wait，可能在模型准备阶段、HTTP 请求尚未发出时取消。合成任务的持久摘要确认该次上下文与记忆尚未准备完成。脚本已改为同时等待任务阶段和假服务确认请求在途，模型、平台动作、普通聊天兜底均采用此条件，并补充计数诊断；脚本测试复跑通过，首轮不计为整机通过。

包含长词布局修复的最终构建 `.local/bin/zerobot-agent-memory-before-ui-final.exe` 成功，退出码 0，模块缓存权限警告未影响产物。执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-memory-before-ui-final.exe` 后完整隔离整机回归通过，完成时间为 2026-10-14 14:56:59 +08:00，本机结果位于 `.local/smoke-admin/run-602938426/result.json`。POST 使用实际较新记忆编号只返回更早目标；目标停用后边界内为空，无边界仍可找到其他较新条目。新增 `agentMemoryBeforeAdminAPIPassed=true`，Agent 四轮查询/读取、任务摘要、在途取消及原认证、配置、备份、日志、统计、语音和反向连接回归同时通过。

结果 `passed=true`、`isolated=true`、`realQQMessagesSent=false`，差异空白、Go 格式、JavaScript 语法与文档链接检查通过。使用步骤见[记忆管理](agent-memory.md)，整机前提见[隔离自测](agent-integration-test.md)。测试使用合成数据和回环假模型/OneBot，未部署日常服务，未发送真实 QQ 消息，不替代真实模型效果、客户端体验或负载验收。

## Agent 向更早记忆检索验证（2026-10-14）

新增 `search_memory.before` 按记忆编号限定排他边界。chat、go-onebot-agent、aichat 和 localadmin 全部 Go 回归通过；新增 2 项存储用例覆盖当前修订、启停过滤、同号群/私聊与机器人隔离、脱敏、不存在/极大/极小编号边界、原无边界调用兼容、查询不改版本、聊天重置后记忆仍可检索、旧文件拒绝、取消及读取故障。5,002 条合成记忆验证加边界后仍受 5,000 条扫描上限，继续向前限定窗口可命中较早条目；4 MiB、每条 2,000 字节和总 16 KiB 返回预算均通过。

动作用例验证非法/数字型/超 int64/带前导零边界拒绝存储调用，最大 int64 字符串准确传递到固定任务范围；存储错误不泄漏路径，与历史检索和读取继续共用三次额度。提示与角色白名单回归通过。脚本判定的 2 项 Go 测试通过，补充真实请求边界缺失或相等时拒绝结果，强化“先前有效响应不能掩盖最新失败”的负向断言。

复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；根目录执行 `go test ./plugin/aichat ./internal/localadmin -count=1 -timeout=180s` 及 `go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s`。最终构建 `.local/bin/zerobot-agent-memory-before.exe` 成功，退出码 0，模块缓存权限警告未影响产物。

随后执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-memory-before.exe`，完整整机回归通过，完成时间为 2026-10-14 12:02:42 +08:00，本机结果位于 `.local/smoke-admin/run-908600701/result.json`。四轮流程先命中较新条目，再从真实响应取得编号作为 before，返回更早目标后按真实编号和版本读取，第四轮收到片段之外的末尾内容才回复；不使用固定目标编号冒充查询结果。任务记录 4 次模型准备、3 次本地动作、1 次平台动作、3 个记忆响应片段，默认记忆仍为 100 条及 record_limit。107 条启用记忆跨页、版本启停恢复、旧修订拒绝、本地不转发 OneBot、历史流程和原后台回归同时通过，模型统计计入新增轮次。

`passed=true`、`isolated=true`、`realQQMessagesSent=false`，新增 `agentMemoryBeforeModelRoundTripPassed=true`。使用说明与复验前提分别见[记忆检索](agent-memory.md)和[整机自测](agent-integration-test.md)；差异空白、Go 格式及文档链接检查通过。本轮无页面改动，未重跑离线 Chrome；测试使用合成数据及回环假模型/OneBot，未部署日常服务或发送真实 QQ 消息，不替代真实模型效果、客户端体验和负载验收。

## AI 与 Agent 完整隔离整机回归（2026-10-14）

扩展整机脚本并修正记忆去重后的旧断言：三个相同 Agent 保存只保留一条。新增 2 项显式 Go 测试通过，验证模拟模型判定逻辑会拒绝初始上下文泄漏目标、缺失正文、最新响应失败、数字型编号、越界序号、非事件来源和意外额外轮次，保留 int64 大整数字符串。复验命令为 `go test scripts/smoke-local-admin.go scripts/smoke-local-admin_test.go -count=1 -timeout=120s`；因构建标记为 ignore，不由普通包测试自动执行。

最终构建 `.local/bin/zerobot-agent-integration.exe` 成功，退出码 0；模块缓存权限警告未影响产物。随后执行 `go run scripts/smoke-local-admin.go -binary-name zerobot-agent-integration.exe`，完整隔离整机流程通过，完成时间为 2026-10-14 11:36:28 +08:00。本机结果位于 `.local/smoke-admin/run-2912866993/result.json`，`passed=true`、`isolated=true`、`realQQMessagesSent=false`，全部新增流程标记通过。

验证包括后台新增与同文复用不改元数据、106 条启用记忆跨三页读取、群/私聊与其他群精确读取隔离；较早目标不在默认 100 条装载中，检索片段不含末尾细节，模型从实际响应取得编号/版本读取后第三轮才获得细节。历史流程从事件取得 before，检索更早事件，再从实际响应取序号读取原文；另一个两轮流程确认恢复启用后的旧修订仍不可读。新增模型请求合计 8 次，任务分别记录 3/3/2 次模型准备、2/2/1 次本地动作和各 1 次平台动作，记忆响应片段计数为 2，历史来源计数至少 2，默认记忆 100 条及 record_limit 摘要正确，不含正文。

编辑、旧版本冲突、停用、恢复历史正文保留停用、显式启用及五个修订均核对通过，停用条目不可搜索；任务状态筛选核对 8 个完成、3 个取消和 0 个运行任务。本次任务总数未达到分页边界，任务跨页仍由独立存储测试覆盖。所有本地查询与记忆保存未转发 OneBot，新增轮次纳入模型统计。原有认证/CSRF、配置、备份、插件、角色权限、模型/动作/兜底取消、日志脱敏、统计、语音及反向连接整机回归同时通过。

复验前提和结果标记见[整机自测说明](agent-integration-test.md)。本轮只扩展验证脚本与文档，没有修改应用或页面代码，因此未重复模块与离线 Chrome 全量测试；其最近结果保留在以下历史记录中。测试使用合成数据与回环假模型/OneBot，未部署日常服务，未发送真实 QQ 消息，不替代真实模型兼容性、召回质量、客户端体验、负载或重启恢复验收。

## 后台新增长期记忆验证（2026-10-14）

chat、localadmin 和 aichat 全部 Go 回归通过。新增 2 项存储用例覆盖后台来源/修订 1/未知档案来源、脱敏、新条目参与后续装载、同文复用不改元数据或增加版本、跨机器人及群/私聊隔离、原 Agent 来源保留、仅停用条目时新建、24 个并发创建只新增一次、非法文本、旧文件归属拒绝及首个版本写入失败回滚。接口用例验证登录/CSRF、连接账号、禁止混用修订参数、私聊范围、字符串编号、no-store 和归属冲突。

档案、记忆及任务共 37 项离线 Chrome 回归全部通过；新增 2 项测试涵盖确认/取消、单行校验、固定已加载范围、新增与复用提示、旧文件入口禁用、保存期间导航保护及未知结果/冲突不自动重试。复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s` 与 `node --test scripts/test-agent-memory-ui.mjs scripts/test-agent-tasks-ui.mjs scripts/test-agent-archive-ui.mjs`。

构建 `.local/bin/zerobot-agent-manual-memory.exe` 成功，退出码 0，模块缓存权限警告未影响产物；差异空白与相关文档链接检查通过。本轮未重跑完整整机流程；使用合成数据及模拟接口，未部署日常服务，未发送真实 QQ 消息。

## 任务状态筛选验证（2026-10-14）

chat、localadmin 和 aichat 全部 Go 回归通过。新增 2 项存储用例以 125 个合成任务验证全部、63 个失败、62 个完成的筛选分页无重复或遗漏，覆盖群/私聊、非法筛选、停止状态转换与取消查询。故障场景验证数据库仍为 running 时内存 failed 被纳入失败筛选，并保留正确后续页；首次插入失败无数据库行的状态仅在失败筛选首页展示，相关用例补充后定向复跑通过。后台接口新增状态传递、所有合法状态、非法值拒绝及默认全部断言。

档案、记忆与任务共 35 项离线 Chrome 回归全部通过。新增页面用例验证翻页沿用已加载条件，重新读取应用筛选草稿，成功停止响应恢复全部筛选，切换范围清空状态；既有 320px 深浅主题布局继续通过。复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s` 与 `node --test scripts/test-agent-tasks-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs`。补充定向命令为 `go test ./chat -run TestAgentTaskFiltered -count=1 -timeout=120s`。

构建 `.local/bin/zerobot-agent-task-filter.exe` 成功，退出码 0，模块缓存权限警告未影响产物；差异空白和相关文档链接检查通过。本轮未重跑完整整机流程，使用合成数据和模拟接口，未部署日常服务，未发送真实 QQ 消息。

## 后台记忆搜索与正文核对验证（2026-10-14）

localadmin 全部 Go 回归通过，新增接口用例验证搜索登录/CSRF、连接账号范围、关键词上限、操作混用拒绝、私聊绑定、共享限流、精确版本大整数、分页/筛选参数拒绝、no-store 和存储故障。存储算法复用已经验证的 SearchEnabled/ReadEnabled，本轮没有修改其行为。

档案、记忆与任务共 34 项页面用例分次验证通过。首次整组 33 项通过，1 项断言误将“先前片段不代表当前版本已核对”中的文字当作成功标识；断言改为检查完整成功提示后，新增 2 项搜索用例定向复跑通过，包括只读片段、范围/版本绑定、纯文本、320px 深浅主题、版本失效、读取/搜索失败及退出后的迟到响应清理。该修正仅修改测试断言，产品代码未改动。

复验：根目录 `go test ./internal/localadmin -count=1 -timeout=180s`、`node --test scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs scripts/test-agent-tasks-ui.mjs`；定向入口 `node --test --test-name-pattern='memory search' scripts/test-agent-memory-ui.mjs`。构建 `.local/bin/zerobot-agent-memory-search-ui.exe` 成功，退出码 0，模块缓存权限警告未影响产物；差异空白与文档链接检查通过。本轮未重跑存储/模型模块或完整整机流程；使用合成数据和模拟接口，未部署日常服务，未发送真实 QQ 消息。

## 任务记忆查询片段计数验证（2026-10-14）

chat、localadmin 后台和 aichat 插件全部 Go 回归通过。新增 2 项任务用例覆盖历史来源/记忆片段分开计数、重复条目按出现次数、事件内容不计为响应、任务只保存计数、数据库重读保留计数、旧 payload 缺失不补造、重置后的实测零值、兜底清空及历史容量截断后无法解析的响应不计。后台接口断言确认可选 memorySources 数值随任务返回。

档案、记忆与任务共 32 项离线 Chrome 回归通过，新增页面用例验证正数、零、旧摘要及无上下文摘要分别提示，覆盖 320px 深浅主题和范围清理。复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s` 与 `node --test scripts/test-agent-tasks-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs`。测试夹具序列化类型修正后完整 chat 回归通过。

构建 `.local/bin/zerobot-agent-retrieval-counts.exe` 成功，退出码 0，模块缓存权限警告未影响产物；差异空白与相关文档链接检查通过。本轮未重跑完整整机流程，使用合成数据及模拟接口，未部署日常服务，未发送真实 QQ 消息。

## Agent 精确版本记忆读取验证（2026-10-14）

chat、go-onebot-agent 依赖、aichat 插件和 localadmin 后台全部 Go 回归通过。新增 4 项用例覆盖当前编号/版本/来源读取与脱敏，群/私聊及不同机器人隔离，旧修订、停用、不存在编号返回空，恢复启用后须用新修订号读取，聊天重置保持独立记忆，旧文件不可读，以及 8,000 字节 UTF-8 边界、超 64 KiB 占位、取消查询和存储故障。动作校验涵盖大整数、非法格式、模型范围覆盖拒绝、错误不泄漏本机信息和四种查询共享三次额度；角色权限与本地动作分类已纳入 read_memory。

三轮回环模拟模型通过：106 条启用记忆中的旧目标未进入初始装载，检索片段不含末尾细节，按精确编号/版本读取后第三轮才获得该细节，未转发 OneBot。复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；根目录 `go test ./plugin/aichat ./internal/localadmin -count=1 -timeout=180s`。统一额度提示后参数与共享额度用例定向复跑通过，最终程序重新构建。

构建 `.local/bin/zerobot-agent-read-memory.exe` 成功，退出码 0，模块缓存权限警告未影响产物；差异空白与文档链接检查通过。本轮无页面改动，未重跑浏览器或完整整机流程；使用合成数据和回环模拟模型，未部署日常服务，未发送真实 QQ 消息。

## Agent 长期记忆检索验证（2026-10-14）

chat、go-onebot-agent 依赖、aichat 插件和 localadmin 后台全部 Go 回归通过。新增 4 项记忆检索测试（含扫描边界子用例）覆盖启用过滤、编号/版本/来源、脱敏、同号群与私聊及不同机器人隔离、旧文件不展示、参数校验、取消查询及读取失败；5,001 条、4 MiB、20 条/16 KiB 片段及异常超大正文上限和 UTF-8 边界均通过。动作适配测试验证禁止模型指定范围或历史边界、错误不泄漏本机信息，以及三类查询共用三次额度。任务计数和角色权限测试已纳入 search_memory。

两轮回环模拟模型验证：106 条启用记忆中最早的目标不在初始默认装载范围，检索后结果正文与编号、版本进入第二轮模型输入，未转发 OneBot；强化结果正文检查后定向复跑通过。复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；根目录 `go test ./plugin/aichat ./internal/localadmin -count=1 -timeout=180s`。

最终构建 `.local/bin/zerobot-agent-search-memory.exe` 成功，退出码 0，模块缓存权限警告未影响产物；差异空白和相关文档链接检查通过。本轮未修改页面，未重跑浏览器或完整整机流程；测试使用合成数据及回环模拟模型，未部署日常服务，未发送真实 QQ 消息。

## 历史记忆版本对比验证（2026-10-14）

档案、记忆及任务共 31 项离线 Chrome 回归通过。新增 2 项用例覆盖大整数版本显示、中文及表情字符边界、纯文本差异标示、脱敏正文相同与启停变化分开提示、插入片段、展开/收起无网络请求、重新读取/切换会话清空对比及 320px 深浅主题横向布局。

复验：根目录 `node --test scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs scripts/test-agent-tasks-ui.mjs`。构建 `.local/bin/zerobot-agent-memory-compare.exe` 成功，退出码 0，模块缓存权限警告未影响产物。相关文档链接和差异空白检查通过。本轮仅修改页面展示，没有修改接口、存储和模型动作，未重跑 Go 包测试或完整整机流程；使用合成数据和模拟接口，未部署日常服务，未发送真实 QQ 消息。

## 长期记忆状态筛选验证（2026-10-14）

记忆存储和后台接口定向 Go 回归通过。新增存储用例以 105 条合成记忆验证全部、52 条启用、53 条停用跨页读取无重复或遗漏，涵盖机器人及群/私聊隔离、非法状态、历史版本拒绝启停筛选与原 View 兼容。新增接口用例验证状态传递、默认全部及非法组合拒绝。档案、记忆、任务共 29 项离线 Chrome 回归通过；新增页面测试确认翻页沿用已加载筛选，历史版本无状态过滤，重新读取应用草稿，切换范围清空筛选和分页。

复验：在 `third_party/zbputils` 执行 `go test ./chat -run TestMemory -count=1 -timeout=180s`；根目录 `go test ./internal/localadmin -run TestMemory -count=1 -timeout=120s` 与 `node --test scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs scripts/test-agent-tasks-ui.mjs`。构建 `.local/bin/zerobot-agent-memory-filter.exe` 成功，退出码 0，模块缓存权限警告未影响产物；相关文档链接和差异空白检查通过。本轮未重跑全部 Go 模块或完整整机流程，使用合成数据及模拟接口，未部署日常服务，未发送真实 QQ 消息。

## 后台搜索序号边界验证（2026-10-14）

档案接口定向 Go 回归通过，新增用例覆盖登录与 CSRF、正整数字符串及大整数、非法值和游标组合、GET 拒绝参数、私聊与归档范围绑定、清除边界及 no-store。档案/记忆/任务共 28 项离线 Chrome 回归通过；新增用例验证“查此条之前”沿用结果关键词和已加载范围，不使用输入草稿，支持手填与留空，非法值不请求接口，返回时间线及切换会话清空边界。既有 320px 深浅主题测试继续通过。

复验：根目录 `go test ./internal/localadmin -run TestArchive -count=1 -timeout=120s` 与 `node --test scripts/test-agent-archive-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-tasks-ui.mjs`。构建 `.local/bin/zerobot-agent-search-before.exe` 成功，退出码 0；模块缓存权限警告未影响产物。相关文档链接和差异空白检查通过。本轮未修改档案存储和模型动作，未重跑其全量测试或完整整机流程；使用合成数据及模拟接口，未部署日常服务，未发送真实 QQ 消息。

## 搜索结果展开原文验证（2026-10-14）

档案接口定向 Go 回归通过；档案、记忆及任务共 27 项离线 Chrome 测试通过。新增 2 项页面测试覆盖固定已加载范围与重置选项、大整数序号、保持搜索片段、纯文本渲染、截断提示、收起不请求网络、记录缺失与读取失败区分、导航后迟到响应丢弃及 320px 深浅主题横向布局。

复验：根目录 `go test ./internal/localadmin -run TestArchive -count=1 -timeout=120s`、`node --test scripts/test-agent-archive-ui.mjs scripts/test-agent-memory-ui.mjs scripts/test-agent-tasks-ui.mjs`。构建 `.local/bin/zerobot-agent-search-detail.exe` 成功，退出码 0；模块缓存权限警告未影响产物。相关文档链接与差异空白检查通过。本轮没有修改存储层和模型动作，未重跑其全量 Go 测试或完整整机流程；使用合成数据和模拟接口，未部署日常服务，未发送真实 QQ 消息。

## 任务累计执行摘要验证（2026-10-14）

chat、aichat 和 localadmin 全部 Go 回归通过。新增 2 项任务测试覆盖各类阶段登记次数、回执不重复累计、同号群与私聊独立、新任务从零计数、取消拒绝不增加计数、重启中断保留累计值、旧 payload 无统计及写入失败的未可靠保存标记。档案、记忆与任务共 25 项离线 Chrome 回归通过；新增页面用例验证非成功数口径、旧任务/零次/存储故障区分和 320px 深浅主题横向布局。

复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录执行 `go test ./plugin/aichat ./internal/localadmin -count=1 -timeout=180s` 与 `node --test scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs scripts/test-agent-tasks-ui.mjs`。构建 `.local/bin/zerobot-agent-task-activity.exe` 成功，退出码 0；模块缓存权限警告未影响可执行文件。差异空白和相关文档链接检查通过。本轮使用合成数据，未重跑完整整机流程，未部署日常服务，未发送真实 QQ 消息。

## Agent 向更早来源检索验证（2026-10-14）

chat、go-onebot-agent 依赖和 aichat 插件全部 Go 回归通过。新增 2 项用例覆盖无效边界、非字符串、前导零及溢出拒绝，大整数序号保持精度，当前机器人/会话绑定和共享三次额度；三轮回环模型确认首轮检索命中最新记录，第二次带 `before` 检索才获得更早匹配，未转发 OneBot。另验证边界本身不包含、同号私聊隔离和重置前记录不能被重新检索。

复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；根目录执行 `go test ./plugin/aichat -count=1 -timeout=180s`。最终构建 `.local/bin/zerobot-agent-history-before.exe` 成功（退出码 0，模块缓存权限警告未影响产物）。本轮无后台页面或接口改动，未重跑浏览器、后台包或完整整机流程；使用合成数据及回环模拟服务，未部署日常服务，未发送真实 QQ 消息。

## Agent 记忆保存去重验证（2026-10-14）

chat、aichat 插件和 localadmin 后台全部 Go 回归通过。新增 2 项存储测试覆盖 24 个并发请求只留一条记忆、归一化及脱敏后比较、重复保存不改来源/时间/修订、后台修订来源保留、不同机器人和群/私聊互不去重、大小写区别、仅停用条目时新建及查询失败上报。既有容量测试改用不同正文，100 条和字节上限仍通过；重复内容只占一条模型取用记录。

复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；根目录执行 `go test ./plugin/aichat ./internal/localadmin -count=1 -timeout=180s`。最终构建 `.local/bin/zerobot-agent-memory-dedup.exe` 成功，退出码 0；模块缓存写入权限警告未影响产物。本轮无页面改动，未重跑浏览器或完整整机流程；测试使用合成数据，未部署日常服务，未发送真实 QQ 消息。

## Agent 按来源读取历史验证（2026-10-14）

chat 模块、go-onebot-agent 依赖、aichat 插件和 localadmin 后台全部 Go 回归通过。新增存储适配测试覆盖正整数字符串及大整数、非法参数和范围覆盖拒绝、检索与读取共用三次额度、截断传播、读取错误脱敏；新增三轮回环模型测试确认先检索再读取才能获得片段之外的原文细节，两个动作均未转发 OneBot。另验证重置前记录和工具请求不能作为读取来源，以及主人/管理员/普通用户角色具备该本地动作、未知角色不获授权。

复验：在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`；在 `third_party/go-onebot-agent` 执行 `go test ./... -count=1 -timeout=120s`；根目录执行 `go test ./plugin/aichat ./internal/localadmin -count=1 -timeout=180s`。补充工具记录与角色断言后，相关定向测试再次通过。构建 `.local/bin/zerobot-agent-read-history.exe` 成功（退出码 0，模块缓存写入权限警告未影响产物），差异空白检查通过。本轮没有页面改动，未重跑浏览器或完整整机流程；测试均为合成数据和回环服务，未部署日常程序、未发送真实 QQ 消息。

## 长期记忆取用摘要验证（2026-10-14）

最终构建 `.local/bin/zerobot-agent-memory-usage.exe` 成功，退出码 0；Go 模块缓存写入权限警告未影响可执行文件生成。文档链接和差异空白检查通过。

chat 模块、后台 localadmin 与 aichat 插件全部 Go 回归通过；新增 4 项记忆摘要测试（含容量边界子用例）覆盖零条、恰好 100 条、101 条、字节预算、停用过滤、机器人及群/私聊隔离、旧文件与已归属模式、轮次及兜底重置、持久化重读、读取失败与摘要写入失败。回环模拟模型确认实际请求包含准备的记忆，任务同时保存历史及记忆摘要且不保存记忆正文。接口测试确认摘要随已鉴权任务响应返回。

档案、记忆与任务页面共 24 项离线 Chrome 回归通过。新增页面测试区分条数/字节限制、空记忆、旧文件、其他机器人归属、读取失败及旧任务未统计，覆盖 320px 深浅主题横向布局。复验：根目录 `go test ./internal/localadmin ./plugin/aichat -count=1 -timeout=180s` 与 `node --test scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs scripts/test-agent-tasks-ui.mjs`；在 `third_party/zbputils` 执行 `go test ./chat -count=1 -timeout=180s`。本轮使用合成数据及回环模型，未重跑完整整机流程、未部署日常服务、未发送真实 QQ 消息。

## 历史记忆正文恢复验证（2026-10-14）

记忆存储与后台接口定向 Go 回归通过，新增用例覆盖恢复后的版本递增、来源保留、当前脱敏规则、停用状态不变、历史保留、机器人及会话隔离、旧版本和重复请求拒绝、修订插入失败的事务回滚，以及鉴权、CSRF、大整数和非法参数组合。档案、记忆、任务共 23 项离线 Chrome 测试通过，其中新增 2 项覆盖恢复确认与取消、只提交范围和版本标识、冲突与未知结果不自动重试、写入期间导航保护和 320px 深浅主题横向布局。

复验：根目录 `go test ./internal/localadmin -run TestMemory -count=1 -timeout=120s`、`node --test scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs scripts/test-agent-tasks-ui.mjs`；在 `third_party/zbputils` 执行 `go test ./chat -run TestMemory -count=1 -timeout=180s`。构建 `.local/bin/zerobot-agent-memory-revert.exe` 成功，模块缓存写入权限警告未影响构建（退出码 0）。本轮使用合成数据，未重跑整机流程、未部署日常服务、未发送真实 QQ 消息。

## 记忆来源核对补充验证（2026-10-14）

档案精确序号读取和后台接口定向 Go 测试通过，覆盖范围隔离、重置前来源、脱敏、UTF-8 截断与超大正文、鉴权、非法组合和大整数序号。档案、记忆与任务共 21 项离线 Chrome 测试通过；新增 3 项验证来源范围固定、历史版本、缺失与失败区分、纯文本及迟到响应隔离。程序构建通过，产物为 `.local/bin/zerobot-agent-memory-source.exe`。Go 模块缓存写入出现权限警告，但构建退出码为 0；不影响产物生成。本次未重跑完整整机流程，未部署日常服务，未发送真实 QQ 消息。

复验：根目录执行 `go test ./internal/localadmin -run TestArchive -count=1 -timeout=120s` 与 `node --test scripts/test-agent-memory-ui.mjs scripts/test-agent-archive-ui.mjs scripts/test-agent-tasks-ui.mjs`；在 `third_party/zbputils` 执行 `go test ./chat -run 'TestArchive|TestAgentHistorySearch|TestAgentRetrieves' -count=1 -timeout=180s`。测试数据均为合成数据。

## 任务历史分页补充验证（2026-10-14）

任务存储与后台分页接口测试通过：125 条合成任务翻页期间新增任务，无重复或遗漏；大整数序号、私聊隔离、取消查询、非法分页参数和未保存状态首页覆盖均通过。档案、记忆和任务页面共 18 项回归通过，其中新增 2 项分页交互测试。程序编译成功，产物为 `.local/bin/zerobot-agent-tasks-paging.exe`。本轮为分页相关回归，未重跑下面记录的 M3 全部整机流程；日常服务未部署，未发送真实 QQ 消息。

## 后续 M3 任务控制验证

后台与 aichat、chat、模型统计和回复统计回归通过。任务测试覆盖并发互斥、会话隔离、模型等待和动作在途取消、旧任务/旧版本、重复停止、数据库关闭重开后的中断及未知结果、首次/中途/终态写失败和上下文计数。新增 5 项任务页面测试及档案/记忆回归共 16 项通过，另通过 4 项 AI 配置检查与 5 项安全页面测试，共 25 项；320px 深浅主题已检查。

Windows 隔离整机通过，新增 `agentTaskModelAndActionCancellationPassed` 覆盖等待 Agent 模型、动作在途及普通聊天兜底三种停止场景：没有后续回复，在途回执仍被记录，重复停止幂等。任务列表和上下文摘要可读取，15 次模拟模型请求与分类统计一致；原有 AI、档案、记忆、备份、连接及权限流程全部通过。取消任务的在途模型 HTTP 请求正常返回，因此 HTTP 完成数不等于任务成功数。最终程序为 `.local/bin/zerobot-agent-tasks-final.exe`。使用步骤见[任务控制](agent-tasks.md)。日常服务未部署，未发送真实 QQ 消息。Windows 受限沙箱拒绝回环端口或私有 ACL 时，须在具备对应本机测试权限的环境复跑，不将环境拒绝记为通过。

## 后续 M2 检索与记忆验证

Windows 隔离整机通过，新增 `agentLocalSearchMemoryManagementMigrationPassed` 验证实际程序的本地动作、来源、版本、迁移与幂等；既有 AI/备份/连接流程保持通过。未发送真实 QQ 消息。最终提交不确定时保留归属的调整补充存储测试，界面提交提示调整补充浏览器测试，随后重建最终程序。

2026-10-13：后台、aichat、chat 与 Agent 依赖模块回归通过；两轮回环模型测试确认旧内容不在首次近期上下文中、检索后第二轮读到来源。记忆测试覆盖来源、版本冲突、启停、机器人和群/私聊隔离、迁移备份、幂等、事务回滚及崩溃归属保留后的恢复。23 项页面回归通过，含新增 6 项记忆界面测试；320px 深浅主题截图已检查。使用说明见[检索与记忆](agent-memory.md)，最终构建为 `.local/bin/zerobot-agent-memory-final.exe`，日常服务未部署。

## 后续 M1 会话档案验证

2026-10-13 新增功能见[会话档案](agent-archive.md)。后台 `localadmin` 与依赖 `chat` 模块回归通过；新增 5 项离线 Chrome 用例、既有 AI/安全/概览 22 项页面回归通过。十万条合成档案验证了 5,000 条搜索上限与会话分页；当前构建完成。日常服务尚未部署，不计为真实 QQ 验收。Windows 私有 ACL 用例需在能够访问当前用户专用目录的环境执行；受限沙箱的拒绝访问不记作测试通过。

M1 Windows 隔离整机流程通过，包含新增档案接口与既有 AI/配置/备份/反向连接回归。超大记录占位文本搜索修正另有定向存储测试通过；最终构建保留为 `.local/bin/zerobot-agent-archive-final.exe`。测试通过不意味着已部署。

以下保留第一版的原验证范围。

更新日期：2026-10-13。公开文档记录可重复执行的方法与验证范围；真实QQ明细、私有日志、截图、凭据和原始结果不随源码上传。

| 类别 | 已执行范围 | 边界 |
| --- | --- | --- |
| 后端与配置 | 认证/来源/CSRF、共享持久化、冲突/写失败保护、备份/恢复、统计及脱敏 | Go包测试与隔离程序验证；不改日常配置演练恢复 |
| 页面与布局 | 现有242项界面回归、35项相关概览回归、64项离线布局；实际Chrome备份/AI/日志/连接/主题等流程 | 实际浏览器资料保留原执行范围，离线检查不替代所有浏览器行为 |
| 观察工具 | 210项模型/回复/资源联合工具回归，45次模拟GPT与15分钟模拟资源观察 | 模型HTTP、任务和发送回执各有口径，不强求计数相等 |
| 当前日常资源 | 20分钟、240样本、CPU平均约0.002018%、私有内存最大增长72KiB | 资源窗口，不是配套业务负载，也不能证明长期无泄漏 |
| 真实QQ | 指定群/主人私聊文字、实际引用、识图、热词出图、群单条语音与私聊限制 | 仅记录已验证业务，不扩大到所有消息类型 |
| 真实角色 | 普通成员/非主人私聊/管理员/真人群主的本轮会话参数允许与拒绝 | 临时身份已恢复；初次管理员无响应，刷新消息成员缓存后正向测试成功 |
| 主人真实Agent | 查询本群、保存记忆、4条群回执、4个完成任务、6次成功动作响应，单次重启后代号延续 | 最初查询收到两条事件但只有一个完成任务；事件不能当作任务数 |
| 取消项目 | 非主人Agent真人测试、客户端体验、真实持续业务负载 | 使用者取消，不记作实测通过，后续出现问题再优化 |

最终交付已从显式文件清单生成独立源码副本并完成构建、相关Go检查、界面及工具回归和Windows隔离整机检查，文档链接和私有资料排除核对通过。结果汇总见[第一版说明](release-v0.1.md)。

## 开发者复验

在仓库根目录执行，先初始化data子模块并按[运行维护](operations.md)准备本地环境。测试使用合成账号与独立目录；不要把日常凭据传入夹具。

```powershell
.\scripts\build-windows.ps1 -OutputName zerobot.exe
go test ./internal/... ./kanban ./plugin/llm ./plugin/aichat ./plugin/aichatcfg ./plugin/airecord ./plugin/wordcount ./plugin/tarot -count=1 -timeout=120s
go run scripts/smoke-local-admin.go -binary-name zerobot.exe
go run scripts/test-admin-system.go -mode restart -binary-name zerobot.exe
.\scripts\test-service-paths.ps1
```

前端测试按实际文件列表执行scripts/test-*-ui.mjs；离线布局使用scripts/test-appearance-layout.mjs。模型/回复/资源工具回归入口为scripts/test-observation-evidence.mjs、scripts/test-reply-evidence.mjs与scripts/test-combined-observation.mjs，其他用例按scripts/test-*.mjs的用途运行。保留原始结果，失败不能改写成通过。

本地依赖补丁可在各自目录单独开发：控制模块和工具模块的go.mod已显式引用相邻补丁，主程序根go.mod也保留同一组替换。隔离/观察启动器默认读取.local/bin/zerobot.exe，可通过binary-name或BinaryName参数指定其他已构建副本；不依赖开发者的内部构建文件名。

依赖模块原有TestMaru只是无条件失败的演示，没有业务断言，现保留名称并明确跳过，不计功能测试通过。最终整机夹具使用data子模块资源及经典诗歌资源缓存，不复制日常控制库或私有模型配置。
