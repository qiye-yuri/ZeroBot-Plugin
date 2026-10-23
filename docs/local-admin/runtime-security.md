# 运行时安全更新与回退

本机 bot 的 Go 工具链与已编译程序使用 1.26.10；NapCat v4.18.28 的内置 Node 已更新至 22.23.4，保持 Node 22 主版本。系统及开发工具的 Node 24 更新至 24.22.1。安装包来自官方发布并核对 SHA-256。NapCat 更新后可使用原缓存账户快捷登录，QQ 账号与 OneBot token 无需更改。

升级 NapCat 时先执行 `scripts/stop-local-services.ps1 -Service OneBot`，备份内置 node.exe 与 `.local/onebot/runtime`，再替换同主版本的官方 Node，执行 `scripts/start-local-services.ps1 -Service OneBot`。日常 ZeroBot 会自动重新连接；通过 `scripts/test-onebot.ps1` 检查登录与只读接口。若等待扫码但存在原账户缓存，可在本机 NapCat WebUI 选择原账户快捷登录。不要重置设备标识或覆盖登录配置。

安全更新备份保存在本机忽略目录 `.local/security-upgrade-20261022/`，包括 Node、Clash 内核及配置、Java 拒绝列表、Python 证书包和完整 Windows 防火墙策略。具体文件映射与本机验收见该目录报告，不上传这些备份。

防火墙三种网络配置均已启用，默认入站阻止、出站允许。原有规则保留，另为 IIS、MySQL 与 Windows 文件共享补充 LocalSubnet 入站允许规则。现有远程范围为 Any 的允许规则没有收紧；这不等于只允许局域网访问。恢复完整旧策略需要管理员权限运行备份目录中的 `restore-firewall.ps1`，但旧策略会恢复防火墙关闭状态，仅供故障恢复。

当前使用 Clash 稳定内核 Mihomo 1.19.33（Go 1.26.10），原代理和订阅配置保留。未启用的旧 alpha 内核没有更新，勿切换至旧内核。Python certifi 更新至 2026.10.19，pip 更新至 26.2.2。五套独立 Java 安装保留原 cacerts，仅将与 Mozilla 事件报告核对的 16 张中间证书指纹追加至 SHA-256 拒绝列表，并逐套核验。此处理只针对本次证书事件，不代表旧 JDK 的其他漏洞已修复。

已启动的程序可能缓存旧运行时或证书拒绝列表；完成工作后重启对应应用。Codex 中已运行的 Node 进程需完全退出并重新打开 Codex 才能全部使用新版本。应用自动更新或重新安装可能覆盖手工修复的运行时，更新后应再次检查版本。

回退 NapCat 只恢复备份 node.exe 并重启 OneBot，不覆盖之后新增的聊天数据。Java 回退按照 `java-manifest.json` 的 path/backup 映射恢复原列表，需要管理员权限。Clash 回退须先退出界面并停止服务，恢复旧稳定内核后重启；旧版本含已知风险，只用于紧急兼容性恢复。Node 与证书包同理，优先保留修复版并定位具体不兼容原因。

验收分开记录：隔离完整冒烟和重启恢复检查验证 bot 程序；真实 OneBot 只读检查验证登录与接口；TLS 检查验证正常站点握手及受影响链拒绝。未发送真实 QQ 测试消息或计费模型请求，未在第二台局域网设备逐项测试，也不声称所有第三方软件均已验收。

2026-10-22重启复查更正：CUA内置Node被恢复为24.21.0，手工替换未持久生效，此项仍未修复。全局与primary运行时仍为24.22.1，bot及防火墙正常。需应用受支持的运行时更新，不能把重新打开应用视为该项修复完成。
