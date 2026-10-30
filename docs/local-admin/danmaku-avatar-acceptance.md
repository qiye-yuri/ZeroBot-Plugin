# 查弹幕头像故障降级：计划与验收

日期：2026-10-30（Asia/Shanghai）。分支codex/danmaku-avatar-fallback，基于待验收PR #64头98f88872，未合入master。

## 待办核对与目标

当前QQ4/5，前四项待集中配合。多人角色、真实通知、本地AI及定时整合缺口保持，不新增后台页面。

查弹幕已有上游超时待办。源码发现另一个可独立修复的问题：记录取得后，用户或主播头像失败会让整个处理器返回；被跳过的WebP头像还可能沿用上一张back变量，将他人头像画到下一位主播旁。本批只解决头像对出图的影响，不宣称解决记录接口本身超时。

本项是查弹幕报告恢复，与第3项视频链接/总结返回属于不同命令和场景。完成部署独立检查后登记第5项，头像读取、占位和防串图不拆分计数。

## 实施与边界

- 整页头像共用3秒网络时间，受原查询上下文剩余时间限制；到期后后续位置直接占位。
- 按真实内容解码PNG/JPEG/WebP，单份最多2MiB、尺寸不超过2048×2048，内存缩放到500×500，不创建新头像缓存，不携带Cookie。
- 失败显示“头像暂不可用”，保留已有记录，不沿用前一头像。
- 原整页渲染提取为直接调用的纯函数，便于合成记录验证；没有服务端测试入口、接口切换开关或通用渲染框架。
- 字体、渲染、QQ发送错误及记录接口错误分别保留，不伪造成功。使用见[说明](danmaku-avatar.md)。

## 验证结果

B站全包Go回归通过。新增4项主测试覆盖头像独立位置、坏WebP链接和有效WebP、正常图片、请求不带Cookie/Authorization、共享截止时间后不继续请求、坏数据/超大响应/超大尺寸，以及全部头像失败时整页记录仍可渲染。

合成像素检查证明中间头像失败不串用前后头像；合成整页的用户及两位主播全部无头像，标题、时间和两行正文仍完整。中文字体预览已目视检查，占位文字与布局正常。常规测试使用模块内置字体，无额外字体下载依赖。首次变量拆分遗留引用导致编译失败，修正后全包通过，失败轮不计通过。

公开样本只尝试一次，使用独立OneBot、无真实Cookie。返回“弹幕记录接口：请求失败或超时”，未到头像阶段；该轮出图检查失败，不算通过，不反复重试。真实上游可用性及新部署非空报告仍待验收。

初轮构建664A6482E0CEE2CC580259B8AF54BB7F4E8E9551F1FEFFD700DC9C847E9FF776的完整隔离通过，随后提取原渲染代码并重建。最终候选SHA256：CEB43DDCFE649A2984237363D9C5823DF2674D1E4754D10DC8922C717F1D14B3。Go 1.26.10、CGO_ENABLED=0、go build -trimpath；基线98f88872含本批工作区改动，最终构建另行复验，不套用初轮结果。

~~~powershell
go test ./plugin/bilibiliparse/... -count=1 -timeout=180s
go test ./plugin/bilibiliparse/internal/query -run TestDanmaku -count=1 -timeout=120s
go build -trimpath -o .local/bin/zerobot-danmaku-avatar.exe .
go run scripts/test-bilibili-merge.go -binary-name zerobot-danmaku-avatar.exe
go run scripts/smoke-test-isolation.go -binary-name zerobot-danmaku-avatar.exe
go run scripts/smoke-local-admin.go -binary-name zerobot-danmaku-avatar.exe
~~~

完整隔离从仓库真实物理路径运行。公开样本脚本scripts/test-bili-danmaku.go依赖真实上游，本轮已失败，不能为取得绿灯反复调用。最终部署和上传按实际追加。

## 集中QQ验收

完成独立检查及部署后登记5/5，进入[集中清单](qq-batch-20261030.md)，不继续新增功能绕过验收。数字UID可等待10秒自动选择，逐项串行执行；上游仍不可用则保留待条件，不记通过、不合入。

本轮无真实QQ测试消息、无付费模型调用。预览和公共失败回执留本机，公开文档只保留合成验证摘要与限制。

最终构建的B站权限/开关25项及测试生命周期复验通过；完整隔离于2026-10-30 20:39:55（Asia/Shanghai）通过153项成功检查（不含passed/isolated元数据）。公开样本超时仍未转为通过。3份产品文件与候选/暂存内容一致；223份文档链接和锚点零错误，18项检查器回归及差异检查通过。
