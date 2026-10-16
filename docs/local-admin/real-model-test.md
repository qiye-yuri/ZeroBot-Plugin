# 真实模型测试

R6版脚本需配套包含R5/R6的构建，才能把独立参数保存到隔离实例；请先构建，再用 `-binary-name` 选择该程序。尚未部署R6的日常旧程序不作为本批脚本的配套构建。预检不启动程序，也不改变日常配置。

R1已随R1～R4正式部署。本轮在部署后的原模型模式与原配置上，用日常程序 `zerobot.exe` 再测GPT/聊天/Agent/识图4组，6次HTTP全部完整完成，见[部署验证](self-test.md#r1-r4-合并与日常部署验证)。测试仍在独立实例使用合成OneBot，不向真实QQ发消息。

R1已补充真实提供商复验：旧/命名副本各7组，以及日常后台实际保存临时命名配置后的GPT/聊天/Agent/识图4组，合计18组、38次HTTP完整通过；副本中的遗留地址不可用，仍按命名引用成功调用。脚本始终使用隔离OneBot，不修改日常配置或发送真实QQ；后台临时配置另在页面测试结束恢复。详见[本轮结果](self-test.md#r1-r4-实机测试进度)，不能据此声明QQ客户端验收。

用于验证已配置提供商的实际返回，以及完整程序中的聊天、Agent 动作、检索和识图链路。它使用 `.local/bin` 中的程序、独立配置和合成 OneBot；模型是真实接口，QQ 事件及回执是模拟的，不向真实 QQ 发消息。

测试入口同时支持旧字段与 N2 命名模型。命名模式按聊天、GPT、群总结、Agent、识图的已保存引用选择配置，和机器人运行时共用纯解析规则，不回退到旧字段。读取配置只做一次，作为本次测试快照；预检及脚本初始化不加载机器人聊天/素材模块，不创建控制库或备份。

预检显示 `mode`（legacy/named）、各入口模型/类型、`enabled`、`ready` 与所选用例的 `casesReady`；停用入口明确为不可用，凭据不输出。预检退出成功表示完成只读检查，是否可运行看 `casesReady`；缺项会附 `selectionError`。显式 `-run` 发现缺项、停用、未知/空/重复用例时非零退出，在启动实例或请求提供商前拒绝。失效命名引用直接读取失败，不能据此测试旧模型。

## 运行

需要 Windows、Go、已初始化的项目资源、可运行的程序及本机已保存的模型配置。先在仓库根目录执行只读预检：

```powershell
go run scripts/test-real-model.go
```

预检显示模型名、接口类型、配置是否齐全及每个入口有效的 `maxN`、`topP`、`reasoningEffort`，不输出密钥或发出模型请求。需要真实测试时执行；`-run` 会使用已保存的接口和密钥，消耗提供商额度：

```powershell
# 默认测试日常 zerobot.exe，共七组用例
go run scripts/test-real-model.go -run

# 指定已构建程序，只测试历史和记忆检索
go run scripts/test-real-model.go -run -binary-name zerobot-agent-complete.exe -cases history,memory
```

支持 `gpt,chat,agent,memory-save,history,memory,vision,summary`，逗号分隔。原七项默认列表不变，`summary`仅显式选择；例如 `-cases gpt,summary`。测试不自动切换模型或重试失败场次；缺失配置、无返回、任务失败或断言不符时记录失败。单项等待回复最多 100 秒，整个进程最多 12 分钟，退出时停止自己的机器人实例。

`chat`需要聊天入口，`gpt`需要GPT入口，`summary`需要群总结入口（后两者缺省继承聊天），`agent/memory-save/history/memory` 需要 Agent 入口，`vision` 同时需要 Agent 与识图入口。可用 `-cases gpt,chat` 只验证文字聊天，不能将未选用例或停用入口记为通过。

报告的 `configurationMode` 与 `configurationSnapshot` 说明测试时固定选择。R5 起，解析后的各入口及其有效参数分别保存为隔离实例的命名配置，避免映射到一组全局值时丢失差异；旧模式也先解析全局默认值并保留。该脚本验证实际提供商和业务用例，日常配置不迁移，运行途中再保存不会替换本轮快照。旧报告中的“投影到旧字段”只描述旧版脚本。

离线回归为 `go test scripts/test-real-model.go scripts/test-real-model_test.go -count=1 -timeout=120s`。统一复验另在最终构建后显式运行带 `-validation-binary` 的隔离测试，覆盖 GPT、普通聊天、Agent、识图及群总结五类流程，核对五入口地址/模型/凭据及独立参数，全部使用模拟模型与合成 OneBot，不消耗真实服务额度。

## 判定范围

| 用例 | 实际核对 |
| --- | --- |
| gpt | `/gpt` 使用真实接口，返回本次随机验收字符串 |
| chat | 关闭 Agent 的普通聊天返回本次随机字符串 |
| summary | 隔离OneBot提供合成群信息与历史，群总结入口返回历史中的随机代号；不读取真实群聊 |
| agent | 通过 Agent 的发送动作回复，任务完成且没有普通聊天兜底 |
| memory-save | 真正执行本地保存，回复及数据库记忆均包含随机字符串 |
| history | 模型执行 `search_history` 和 `read_history`，根据合成档案返回正确代号 |
| memory | 在目标之后新增 101 条记忆，使目标不在默认 100 条装载范围；模型执行 `search_memory` 和带准确修订的 `read_memory` 后返回正文末尾代号 |
| vision | 实际识图接口读取程序生成的纯色图片，由 Agent 回复主要颜色，不能使用普通聊天兜底 |

Agent 动作用例使用独立配置中的模拟机器人主人身份及私聊，不测试真实群管理角色。沿用本机接口类型、地址、模型名和密钥；测试实例另设合成人物、系统提示词及温度；R5 起输出长度、TopP 和推理设置保留本轮已解析的各入口参数。历史用例核对工具执行与结果，不证明历史目标在近期上下文之外；记忆用例另核对默认装载之外的目标。真实提供商的输出可能变化，一次通过不代表任意自然语言都能稳定触发工具。

## 证据与日常服务

每次运行打印 `ARTIFACTS=...`，在 `.local/real-model/run-*` 保存 `progress.json`、`result.json`、合成回复和独立进程日志。逐项保存耗时、任务摘要、取用统计及模型 HTTP 统计。当前入口要求所选功能断言和 HTTP 完整读取都通过才汇总 `passed=true`，同时输出 `httpResponsesCompleted=true`：started=completed，active/incomplete/failed 均为 0。无法取得最终统计或存在未完成响应也判失败。统计本身仍保留原口径，不把提前关闭改写成完成；旧版入口生成的历史报告仅按其当时的判定说明解释。

只读读取日常控制库的模型配置，不读取真实聊天、旧记忆或日常后台密码；不修改日常配置、角色、插件开关或服务。真实密钥会写入测试实例的控制库及其配置备份，测试目录整体忽略，不应上传或分享。公开记录只保留不含密钥的摘要。测试证据不是 QQ 客户端收发、日常人物配置或长期负载验收；结果见[自测摘要](self-test.md#真实模型链路验证)。
