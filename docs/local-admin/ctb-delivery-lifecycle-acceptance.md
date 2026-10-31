# FTB首次发送生命周期：开发与验收

日期：2026-10-31（Asia/Shanghai）。基线为最新master 6746c4c1，分支codex/ctb-delivery-lifecycle。

## 基线和目标核对

相关历史摘要已有更新差异，核对当前待办及FTB代码后选择本项。前批PR #66已部署但未合入，先按固定流程完成源码/部署哈希及代理/TLS复验，合入72ac2863，合并树与审查头一致；同步收尾说明后master为6746c4c1。本批不回退该代理修复。

原FTB首次自动发送在RunFor返回之后执行，此时busy已清除；文件还在发送时会显示空闲，允许新的重发/下载或配置保存。原重发流程已有busy保护，因此这属于首次发送与既有规则不一致的明确缺口，不是新增任务管理功能。

## 实施范围

复用原Service任务状态，将启用自动发送的下载与首次发送放在同一个忙碌周期。进入发送前，在同一把锁下核对下载取消并切换阶段；之后不再接受下载取消，发送用原独立10分钟等待上限。成功/失败回执返回后才释放状态。

调用方使用实际任务配置快照中的发送位置；下载结果、重发编号、文件与计次规则保持。不改变未启用自动发送的行为，不增加队列、重试、持久化表或后台页面。入口和边界见[使用说明](ctb-delivery-lifecycle.md)。

## 验证

最初用合成下载器和发送回调复现“首次上传回执未返回就显示idle”，红灯失败后修复。内部CTB、localadmin和adminqa回归通过，plugin/ctb无独立测试文件，仅编译通过；命令由独立OneBot覆盖。

新增2项核心主测试，覆盖发送期busy、拒绝并发下载/重发/有效修订配置保存、正确拒绝取消上传、拒绝操作不改额度，以及成功/失败/关闭自动上传3个结束场景。下载上下文在发送阶段到期不会覆盖发送结果，发送后下一任务可执行。

隔离验收工具仅增加可延迟合成文件回执的能力，其他OneBot请求继续处理，无生产接口或测试开关。独立整机专项通过：真实命令入口在首次上传中显示busy、拒绝重复下载/重发、保存返回409、取消提示正确；释放回执后恢复，模拟上传失败保留文件和重发命令，后续查询成功。下载器只执行两次预定任务，拒绝操作没有额外下载、上传或扣次。

候选SHA256：B74420B2B29EE18E4EEC4FE865F3A9794C99A3B182B3951B4CBD826A0212EB35。Go 1.26.10、CGO_ENABLED=0、go build -trimpath，构建基线6746c4c1含本批工作区修改，产品文件与交付提交核对。

~~~powershell
go test ./internal/ctb ./plugin/ctb ./internal/localadmin ./internal/adminqa -count=1 -timeout=180s
go build -trimpath -o .local/bin/zerobot-ctb-delivery-lifecycle.exe .
go run scripts/smoke-ctb-delivery.go -binary .local/bin/zerobot-ctb-delivery-lifecycle.exe
go run scripts/smoke-ctb-resend.go -binary .local/bin/zerobot-ctb-delivery-lifecycle.exe
go run scripts/smoke-ctb-cancel.go -binary .local/bin/zerobot-ctb-delivery-lifecycle.exe
go run scripts/smoke-test-isolation.go -binary-name zerobot-ctb-delivery-lifecycle.exe
go run scripts/smoke-local-admin.go -binary-name zerobot-ctb-delivery-lifecycle.exe
~~~

完整隔离从仓库真实物理路径运行。部署前核对当前operations.md的备份、单实例和回退步骤。完整回归、部署及上传状态按实际追加，不提前记通过。

已取消的QQ验收不重排，本轮真实QQ消息、真实下载和付费模型测试均0。接口回执不代表收件，回执不明后手动重发仍可能重复；不承诺平台严格一次投递。本批上传独立分支供审查，非必要后台保持暂停，私有摘要/配置/证据不上传。

重发、取消与测试生命周期在同一候选复验通过，测试结束后日常无回流、另一批次隔离；4次本机假模型请求，真实调用0。最终完整隔离于2026-10-31 11:58:36（Asia/Shanghai）通过153项成功检查（不含passed/isolated元数据）。228份文档、3009处本地引用、646处锚点零错误，外链未联网验证；18项检查器回归及差异检查通过。部署前已核对当前运行维护步骤，不依据旧摘要直接操作。

## 最终部署

2026-10-31 12:02（Asia/Shanghai）备份部署完成，实现7bf5fdd924707189cd279f2abf41dbd14d040f4d与候选业务文件一致，SHA256保持上述值。1,374份停机备份逐项核对；OneBot进程、9份配置（含FTB配置及下载次数）、更新监控状态保持，原账号握手、单实例、后台HTTP200及既有资源检查通过。

备份位于本机忽略目录 .local/ctb-delivery-lifecycle/deployment-20261031-120144/。需回退时停止ZeroBot、恢复backup/zerobot.exe后启动复查，保留当前配置、文件/重发记录、下载次数和聊天数据。旧版会恢复首次发送期间提前释放busy的行为。

本批没有发起真实下载、真实QQ验收或付费模型测试，不把模拟接口成功记作实际收件。已取消QQ验收继续保持取消；上传及合入状态另行登记。
