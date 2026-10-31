2026-10-31前批收尾：按现行开发交付顺序，PR #66已审查并合入master（72ac2863），合并树与锁定头6a8694a4完全一致。代理/TLS复验通过，原完整隔离及部署哈希对应已核对；没有重启服务或重新安排QQ验收。此后新目标以更新后的master为基线。

# B站查询环境代理：开发与验收

日期：2026-10-31（Asia/Shanghai）。基线为最新origin/master 4fc31b12，分支codex/bili-query-proxy。原有data及两份无关测试修改保留。

## 目标依据与范围

按相关待办摘要定位查询失败/超时记录；摘要已早于当前文件，随后只核对当前对应章节及客户端代码。现有多人/真实事件缺口、暂缓功能和已取消QQ清单不重新安排。

当前newDanmakuClient自建Transport但未设置Proxy，与项目其他遵循进程代理的客户端不同。在配置HTTP(S)代理的环境中，它仍直接访问目标。选定修复这一可复现的路径差异，不把历史所有超时归因于它。

生产改动仅设置Transport.Proxy为http.ProxyFromEnvironment。查弹幕、用户资料及更新vup共用该客户端；原12秒请求超时、调用方截止时间、TLS校验、禁用长连接和有限空JSON重试规则保持。不新建配置、网页、代理检测或重试框架，不修改用户环境。用法和生效限制见[说明](bili-query-proxy.md)。

## 验证

新增测试在独立子进程中运行，避免Go缓存的代理环境影响不同场景：配置代理、NO_PROXY绕过、无代理直连；检查HTTP和HTTPS代理选择，并实际通过本机代理读取一份合成弹幕JSON，代理命中仅1次。不访问真实B站或QQ。

修复前仅配置代理场景失败，代理实际命中0次，直连/绕过对照通过；修复后B站主包及query/push/userselect全包回归通过，包括原TLS不可信证书拒绝/指定测试证书可信后通过。最初测试将nil选择器也视为直连失败，已在生产修复前调整对照语义并重跑，最终红灯明确对应配置代理缺口。

候选SHA256：D58A0D67A6CA4E06BF4F22EC6643B1355128306F67C00B08850C76F2710438A4。Go 1.26.10、CGO_ENABLED=0、go build -trimpath；构建基线4fc31b12含本批工作区改动，源码与提交逐项核对。

~~~powershell
go test ./plugin/bilibiliparse/... -count=1 -timeout=180s
go build -trimpath -o .local/bin/zerobot-bili-query-proxy.exe .
go run scripts/test-bilibili-merge.go -binary-name zerobot-bili-query-proxy.exe
go run scripts/smoke-test-isolation.go -binary-name zerobot-bili-query-proxy.exe
go run scripts/smoke-local-admin.go -binary-name zerobot-bili-query-proxy.exe
~~~

完整隔离从仓库真实物理路径运行。候选部署按当前operations.md“已验收程序部署与回退”执行停机备份、程序哈希核对、OneBot和本机健康检查，不更改代理值或日常功能开关。

## 结果边界与交付

本批证明查询客户端遵循标准进程代理设置，不证明实际代理/第三方服务可达，也不把原公共弹幕请求失败记录改成成功。没有真实Cookie或QQ配合测试，没有付费模型调用；已取消验收保持取消，不加入新QQ配合队列。

本批完成独立检查后上传开发分支供审查；测试、部署、上传与合入分别记录，不以推送代替合入。公开源码/说明和合成测试可上传，私有摘要、配置、日志及原始数据不上传。最终隔离和部署结果按实际追加。

候选的B站25项权限/开关/选择/持久化检查通过；测试生命周期验证独立测试库、结束后日常无回流及第二批次隔离，4次本机假模型请求，真实调用0。文档226份、2971处本地引用、646处锚点零错误，外链未联网验证；18项检查器回归和差异检查通过。

最终完整隔离于2026-10-31 11:01:19（Asia/Shanghai）通过153项成功检查（不含passed/isolated元数据），同一候选可进入备份部署。已逐项核对当前operations.md部署/回退步骤及限制，非依据旧摘要直接操作。

## 最终部署与交付状态

2026-10-31 11:06（Asia/Shanghai）备份部署成功。实现53e383c9a91807df0f4d9962a4f59ca4ab678864与候选产品文件一致，程序SHA256保持上述值。1,374份停机备份逐项核对；OneBot进程、9份配置及更新监控状态保持，B站Cookie文件另做哈希对照未变。原账号握手、单实例、后台HTTP200及既有资源检查通过。未修改系统/进程代理配置或日常功能开关。

备份位于本机忽略目录 .local/bili-query-proxy/deployment-20261031-110621/。需回退时停止ZeroBot、恢复backup/zerobot.exe并重启检查健康；保留最新配置、Cookie、预算/下载记录和聊天数据。旧版本查询客户端重新忽略环境代理。

部署首次被自动审核要求补充授权依据，核对用户沿用的当前计划C及AGENTS既定流程后复审通过，再执行实际部署。GitHub代理路径连接中断，后使用单次命令直连及Windows既有信任根的公共副本供OpenSSL校验恢复访问；未关闭证书验证、未添加信任根，也未修改持久Git/系统/机器人代理设置。

本批只交付开发分支供审查。已取消的QQ验收保持取消，不再安排、不记实机通过；没有真实QQ消息或付费模型调用。外部服务实际可用性不因合成代理通过而变成已验证。

上传完成：[草稿PR #66](https://github.com/qiye-yuri/ZeroBot-Plugin/pull/66)，head codex/bili-query-proxy，base master，未合入。远端与本地提交已核对一致。GitHub上传的临时TLS/凭据参数仅用于本次开发工具，未写入持久Git配置或机器人环境，也未输出令牌。226份文档链接/锚点零错误，差异检查通过。
