2026-10-31合入更新：按用户明确要求，PR #60～#65已审查并依赖顺序合入master，各合并树与审查头一致，产品源码与已部署候选一致；本次未重启服务。剩余QQ验收仍为用户取消，不记实机通过、不再要求配合。审查、复验与提交详情见[六项PR合入记录](pr60-65-review-merge.md)。下方草稿/未合入表述保留对应历史阶段。

2026-10-30用户更新：用户明确“剩下的不用验收”，本轮五项剩余QQ配合验收已取消，不再请求测试或反馈；验收队列关闭为0/5。此状态是用户取消，不是实机通过，已有自动化/隔离/部署证据及公共接口未成功的限制保留。当前PR #60～#65仍为草稿、未合入。下方旧队列与待配合文字为历史记录，以本条为准。

# 画图、引用图片与撤回候选收敛（2026-10-30）

## 范围和当前状态

基于master d2bbbd5d，在开发分支 codex/aiimage-reference-recall 收敛既有未提交实现。包含OpenAI兼容/SiliconFlow画图、独立配置和预算、参数/帮助、参考图及引用图、QQ缓存UUID兼容、前N张参考图选择、撤回状态和读取过滤。后台仅纳入已存在且为画图配置所需的接口与表单，没有继续扩展非必要页面。

本轮不把源码Git头当作旧日常程序：收敛前实际程序SHA256为 E61367415E78BD6DD086AEF49E301C238C4877CBBB9950215CB998F32902F9D9，与12:30部署记录一致。工作区已有前N张选择实现，未见对应闭环部署记录，本轮补说明、隔离验证并纳入新候选。

初轮候选SHA256：17C6DD5B60F2548D22240F0027FF8FBE0F26B56E6AEFB685C7201E35DA357B91。构建使用Go 1.26.10、CGO_ENABLED=0、go build -trimpath。构建时Git基线为d2bbbd5d并带工作区修改，源码与最终提交的对应通过本轮文件清单核对；上传提交另见[交付记录](github-delivery.md)。不把不同构建元信息导致的二进制差异当作源代码一致证据。

开发、相关回归、最终完整隔离和备份部署已完成；上传结果见交付记录。真实QQ仍待统一配合、未合入master。

## 本轮收敛内容

- 保留现有实现，未新增通用重试、兼容框架或后台页面。
- 补齐 --refs=N 使用说明和QQ内置帮助：1～16，先当前附图再引用图，省略则全选；仅下载/校验/计额选中的图片，不改变 --n 输出张数；原引用归属、测试隔离与撤回检查照常生效。
- 全部画图专项和撤回整机脚本增加 -binary 参数，可对同一个绝对或相对候选路径复验，无需复制多份程序或覆盖旧候选。
- 增加撤回后的列表/搜索/指定上下文读取检查及在途旧摘要拒绝检查。测试先证明原消息可读，再撤回并检查过滤，保留原始档案。

操作入口见[AI画图](aiimage.md)、[可选参数](aiimage-options.md)、[参考图](aiimage-reference.md)、[QQ帮助](aiimage-qq-help.md)与[撤回行为](message-recall.md)。普通用户按插件响应权限画图/查帮助；配置仍为主人私聊或既有受保护本机后台。单条参数即时生效，不改默认设置；新帮助和 --refs 能力以本次候选部署为准。

## 验证命令与证据边界

以下命令从仓库根目录执行，使用仓库现有Go 1.26.10工具链。第三方chat包需进入 third_party/zbputils。测试均为合成身份和独立目录；包初始化可能检查/下载字体等公开资源，不涉及真实模型或QQ。

~~~powershell
go test ./plugin/aiimage ./internal/imagegen ./internal/localadmin ./internal/adminqa ./plugin/aichat -count=1 -timeout=180s
# third_party/zbputils 内
go test ./chat -count=1 -timeout=180s
go test ./chat -run TestRecall -count=1 -timeout=60s
# 回到仓库根目录
node --test scripts/test-ai-image-ui.mjs
go build -trimpath -o .local/bin/zerobot-image-convergence.exe .
go run scripts/smoke-recall.go -binary .local/bin/zerobot-image-convergence.exe
go run scripts/smoke-aiimage-select.go -binary .local/bin/zerobot-image-convergence.exe
go run scripts/smoke-aiimage-options.go -binary .local/bin/zerobot-image-convergence.exe
go run scripts/smoke-aiimage-help.go -binary .local/bin/zerobot-image-convergence.exe
go run scripts/smoke-aiimage-budget.go -binary .local/bin/zerobot-image-convergence.exe
go run scripts/smoke-aiimage-admin.go -binary .local/bin/zerobot-image-convergence.exe
go run scripts/smoke-aiimage25.go -binary .local/bin/zerobot-image-convergence.exe
go run scripts/smoke-test-isolation.go -binary-name zerobot-image-convergence.exe
go run scripts/smoke-local-admin.go -binary-name zerobot-image-convergence.exe
~~~

已通过：画图、必要配置后台、adminqa、aichat和chat包回归；internal/imagegen没有独立测试文件，仅编译通过。画图内置帮助更新后复跑通过，新增撤回专项4项通过，11项离线Chrome回归通过；未把模拟页面验证写成日常浏览器实测。

首轮受限环境回归因临时文件/回环网络权限失败，正常权限完整复跑通过。新增上下文回归首轮未按原API设置EventsOnly，夹具被正常拒绝；修正夹具后通过，未为通过测试放宽业务规则。失败轮均不算通过。

## QQ队列与最终出口

当前仍为 **1/5**。画图、引用、选择图片与撤回为同一已登记批次的收敛，不新增次数，不要求本轮发消息。其余四项只在后续独立必要批次完成开发、隔离、部署及独立检查后按真实缺口登记。

集中验收逐项记录：实际QQ文生图收件、引用/附图进入编辑及输出、参考图选择顺序与数量、帮助可读性、撤回后的结果抑制。撤回需要额外操作，应与单条消息收件明确区分；不能把一次收件自动当作覆盖全部格式、多图、预览、撤回和账单。未覆盖项继续保留，不为了凑数增加付费请求。

真实模型测试须在北京时间空闲时段，真实QQ测试先启用宿主测试标记，结束后确认日常无回流。本轮真实模型调用0、真实QQ测试消息0。已提交请求可能计费，已发送图片不能保证撤销；机器人离线未收到的撤回通知无法保证识别。

QQ验收前仅上传开发分支/草稿PR，不合入master。备份与回退按[运行维护](operations.md)执行；回退不覆盖新配置、预算账本与聊天数据，且旧程序不应当作仍具备新引用/撤回规则。


## 专项最终结果

十个画图/引用/撤回整机脚本全部通过，均显式指定上述同一候选。测试开始/结束、独立测试记忆、日常无回流与第二批次隔离专项通过，4次本机假模型请求，真实调用0。

额外修正旧验收脚本：基础画图改为每轮新建独立目录，避免历史夹具已存在导致失败；基础与配置脚本显式为免费假模型填价格0，不放宽业务门控。旧配置脚本价格未知、旧基础脚本目录冲突的失败轮保留。完整回归首次从Windows目录联接路径启动，被原备份路径规则拒绝；改为仓库真实路径复跑，不修改生产路径保护。完整回归应从解析后的仓库物理目录执行。

文档检查器18项回归通过；当前213份文档、2725处本地链接、646处锚点零错误，外链未联网验证。公开清单与敏感内容模式检查完成，暂存范围不含data及两个既有无关测试文件。


完整隔离最终通过：153项成功标记（不含passed/isolated元数据），合成Agent滚动摘要请求1次，实际完成时间2026-10-30 17:26:28（Asia/Shanghai）。证据保留在本机忽略目录；同一候选可进入备份部署阶段。


最终构建来源：`64eb3a824179a112ca342600ecb9ea8624cdbf76`；SHA256 `C6FA3BAACA58810787AFFF9D8018ED835A274D559FCEFD53D32A73AA5542694E`。相较初轮仅清理7份文件末尾空行（其中包含内置帮助/页面资源），未改变业务逻辑；36份本批产品源码/资源与提交逐项一致。初轮十个专项与完整隔离结果保留，最终程序另跑完整隔离、帮助/选择/撤回、测试生命周期及11项页面回归后再部署。

最终构建专项复验通过：撤回/引用/预算链路、前N张选择、完整帮助（含4K多张/流式/预算重启）、测试结束后的日常无回流与第二批次隔离，以及11项离线页面回归。相关生产Go代码与初轮一致，无重复付费调用。

最终构建完整隔离在2026-10-30 17:34:09（Asia/Shanghai）通过，同样153项成功检查；证据与候选哈希已核对。初轮末尾空行检查未通过，清理后仓库差异检查通过，最终验证使用重建后的程序。


## 最终部署与回退

2026-10-30 17:35（Asia/Shanghai）部署成功。停机备份1,369份文件并逐一核对；使用最终候选SHA256 `C6FA3BAACA58810787AFFF9D8018ED835A274D559FCEFD53D32A73AA5542694E`，单实例、原账号握手、OneBot只读健康和后台HTTP 200通过。9份配置哈希、OneBot进程身份与更新监控状态保持。未发送真实QQ测试消息，未调用真实模型。

备份在本机忽略目录 `.local/image-convergence/deployment-20261030-173454/`。需回退时停止ZeroBot，恢复该目录backup/zerobot.exe后启动并复查健康；保留当前配置、预算账本与聊天数据，不用旧备份覆盖新数据。回退版本不包含本次前N张选择及新帮助，不得继续声称这些行为已生效。

本轮部署首次被自动审核要求补充授权依据；核对用户已采纳计划C及既有固定部署流程后复审通过，再执行实际备份部署。未绕过审核，也未修改后台非必要功能。

上传与审查入口：[草稿PR #60](https://github.com/qiye-yuri/ZeroBot-Plugin/pull/60)。开发分支已上传并核对远端，源码与公开说明可审查；master未合入，QQ仍1/5。最终文档检查213份、2736处本地引用、646处锚点零错误，外链未联网验证；差异检查通过。
