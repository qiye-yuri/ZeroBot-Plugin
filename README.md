2026-10-29后续规划：下一项为[按窗口内最慢请求耗时排序](docs/local-admin/post-pr58-development-plan.md)，4个工作包、11项验收标准，预计4～6小时有效工时。仅规划，尚未实现；复用已有耗时，摘要显示最慢已知值和覆盖，原值排序、未知末尾，不新增采集或通用统计框架。QQ队列保持4/5，继续满5项统一配合。

2026-10-28单次请求耗时已完成99项回归、最终150项隔离、23:29备份部署及实际空窗口/口径/刷新/重置/报告检查。当前QQ队列4/5，真实非空继续满5项统一验收，未合入；见[本批清单](docs/local-admin/task-request-duration-acceptance.md)。

2026-10-28任务明细单次请求耗时：已实现，99项相关回归通过；操作见[使用说明](docs/local-admin/model-request-statistics.md#任务明细显示单次请求耗时)，部署及验收状态见[本批清单](docs/local-admin/task-request-duration-acceptance.md)。复用已有字段，报告精度保持，QQ配合继续按实际缺口累计5项，未提前合入。

2026-10-28后续规划：下一项为[任务明细显示单次请求耗时](docs/local-admin/post-pr57-development-plan.md)，4个工作包、11项验收标准，预计3～5小时有效工时。仅规划，尚未实现；直接显示已有milliseconds，区分已知0和未知，不增加采集或通用格式框架。QQ队列保持3/5，继续满5项统一配合。

2026-10-28类别筛选已完成96项回归、最终150项隔离、22:53备份部署及实际空窗口/类别/刷新/重置/报告检查。当前QQ队列3/5，真实非空继续满5项统一验收，未合入；见[本批清单](docs/local-admin/task-kind-filter-acceptance.md)。

2026-10-28按请求类别筛选任务：已实现，96项相关回归通过；操作见[使用说明](docs/local-admin/model-request-statistics.md#按请求类别筛选任务)，部署与验收状态见[本批清单](docs/local-admin/task-kind-filter-acceptance.md)。保留完整组和报告，QQ配合继续按实际缺口累计5项，未提前合入。

2026-10-28后续规划：下一项为[按请求类别筛选任务](docs/local-admin/post-pr56-development-plan.md)，4个工作包、11项验收标准，预计4～6小时有效工时。仅规划，尚未实现；复用现有kind按组匹配，不增加后端或通用筛选框架。QQ队列保持2/5，继续满5项统一配合。

2026-10-28标识筛选已完成93项回归、最终150项隔离、21:36备份部署及实际空窗口/输入/刷新/清除/重置/报告检查。当前QQ队列2/5，真实非空继续满5项统一验收，未合入；见[本批清单](docs/local-admin/task-identity-filter-acceptance.md)。

2026-10-28按会话或任务标识筛选：已实现，93项相关回归通过；使用见[说明](docs/local-admin/model-request-statistics.md#按会话或任务标识筛选)，部署与验收状态见[本批清单](docs/local-admin/task-identity-filter-acceptance.md)。复用关联分组精确匹配，QQ配合继续按实际缺口累计5项，未提前合入。

2026-10-28后续规划：下一项为[按会话或任务标识筛选](docs/local-admin/post-pr55-development-plan.md)，4个工作包、11项验收标准，预计4～6小时有效工时。仅规划，尚未实现；复用有效关联分组做本地完整精确匹配，不加后端或持久化。QQ队列保持1/5，继续满5项统一配合。

2026-10-28任务用量汇总表已完成90项相关回归、最终150项隔离、21:00备份部署及实机空态/刷新/重置/报告检查。旧真实样本离线对照通过，新部署非空按实际缺口登记1/5，累计5项统一QQ配合，尚未合入。详见[本批清单](docs/local-admin/task-usage-table-acceptance.md)。

2026-10-28任务用量汇总表：已实现，90项相关回归及最终窄屏表格专项通过。入口和口径见[使用说明](docs/local-admin/model-request-statistics.md#任务用量汇总表展示)，部署及实际验收状态见[本批清单](docs/local-admin/task-usage-table-acceptance.md)。QQ配合按实际缺口累计到5项，不提前合入。

2026-10-28后续规划：下一项为[任务用量汇总表展示](docs/local-admin/post-pr54-development-plan.md)，含4个工作包、11项验收标准，预计4～6小时有效工时。仅设计，尚未实现；沿用现有四字段统计，改善匹配任务与全窗口对照。QQ队列保持0/5，后续按实际配合缺口累计5项统一验收。

2026-10-28晚：PR #50～#54五项已完成集中真实非空验收、用户QQ收件确认并依次合入master，前批重置回读和实机截图已补齐。详见[最终验收与合入记录](docs/local-admin/pr50-54-batch-acceptance.md)。QQ队列已清空为0/5，后续继续累计5项统一安排。

2026-10-28筛选结果已知输入与缓存用量小计：已实现，89项相关回归通过；操作见[使用说明](docs/local-admin/model-request-statistics.md#筛选结果已知输入与缓存用量小计)，最终部署与集中验收状态见[本批清单](docs/local-admin/task-token-summary-acceptance.md)。已完成备份部署及独立检查，当前5/5；空闲时段统一QQ配合，最终重置回读/实机截图集中补核对，未合入。

2026-10-28后续规划：下一项为[筛选结果已知输入与缓存用量小计](docs/local-admin/post-pr53-development-plan.md)，含4个工作包、11项验收标准，预计4～6小时有效工时。仅设计，尚未实现；现有功能与QQ队列4/5保持，完成第五项部署独立检查后再统一安排配合验收。

2026-10-28按输出用量已知情况筛选：已按[详细计划](docs/local-admin/post-pr52-development-plan.md)实现，86项相关回归、最终150项隔离及备份部署独立检查通过；操作见[使用说明](docs/local-admin/model-request-statistics.md#按输出用量已知情况筛选)，部署与集中验收状态见[本批清单](docs/local-admin/task-output-coverage-filter-acceptance.md)。当前队列4/5，真实非空与QQ配合继续满5项统一验收，尚未合入master。

上一批记录（2026-10-28筛选结果已知输出量小计）：已按[详细计划](docs/local-admin/post-pr51-development-plan.md)实现，83项相关回归、最终150项隔离及备份部署独立检查通过；操作见[使用说明](docs/local-admin/model-request-statistics.md#筛选结果已知输出量小计)，部署与集中验收状态见[本批清单](docs/local-admin/task-output-summary-acceptance.md)。当前队列3/5，真实非空及QQ配合继续满5项统一验收，尚未合入master。

上一批记录（2026-10-28请求关联类型筛选）：已按[详细计划](docs/local-admin/post-pr50-development-plan.md)实现，80项相关回归、最终150项隔离及备份部署独立检查通过，使用方式见[说明](docs/local-admin/model-request-statistics.md#按请求关联类型筛选)，部署及集中验收状态见[本批清单](docs/local-admin/task-association-filter-acceptance.md)。当前队列2/5，真实非空与QQ配合继续累计5项统一进行，尚未合入master。

上一批记录（2026-10-28输出量排序）：已实现[按窗口内已知输出量排序](docs/local-admin/model-request-statistics.md#按窗口内已知输出量排序)，77项相关离线回归通过，最终部署与上传进度见[验收清单](docs/local-admin/task-output-sort-acceptance.md)。本批已部署并通过独立检查，真实非空与QQ验收登记1/5，累计5项统一安排，尚未合入master。

此前规划基线（2026-10-28）：PR #46～#49已集中验收并合入master，QQ配合队列0/5。下一批规划为[按窗口内已知输出量排序](docs/local-admin/post-pr49-development-plan.md)，含4个工作包、10项验收标准，预计3～5小时有效工时；尚未实现，本轮文档不计入队列，继续累计5项统一验收。

## 本地机器人管理后台 v0.1

本分支增加本机管理后台，支持连接、插件、AI、日志与配置备份。仅监听本机回环地址，默认端口为3010。

[文档导航](docs/local-admin/README.md) · [使用指南](docs/local-admin/user-guide.md) · [第一版说明](docs/local-admin/release-v0.1.md) · [开发与验证](docs/local-admin/self-test.md)

---

<div align="center">
  <img src=".github/hua_nobg_512.gif" alt="椛" width = "256">
  <img src="https://github.com/FloatTech/ZeroBot-Plugin/assets/41315874/93fb795d-e519-45a6-a654-076fd6ac54ae" alt="zbp-uwu" width = "400">
  <br>

  <h1>ZeroBot-Plugin</h1>

  “椛椛是[真寻](https://github.com/HibiKier/zhenxun_bot)的好朋友！”

  ZeroBot-Plugin 是 ZeroBot 的 实用插件合集<br><br>


  <img src="https://counter.seku.su/cmoe?name=ZeroBot-Plugin&theme=r34" /><br>

  [![miraigo](https://img.shields.io/badge/OneBot-MiraiGo-green.svg?style=social&logo=appveyor)](https://github.com/Mrs4s/MiraiGo)
  [![oicq](https://img.shields.io/badge/OneBot-OICQ-green.svg?style=social&logo=appveyor)](https://github.com/takayama-lily/oicq)
  [![mirai](https://img.shields.io/badge/OneBot-Mirai-green.svg?style=social&logo=appveyor)](https://github.com/mamoe/mirai)



  [![go](https://goreportcard.com/badge/github.com/FloatTech/ZeroBot-Plugin?style=flat-square&logo=go)](https://goreportcard.com/badge/github.com/FloatTech/ZeroBot-Plugin)
  [![onebot](https://img.shields.io/badge/onebot-v11-black?style=flat-square&logo=data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAHAAAABwCAMAAADxPgR5AAAAGXRFWHRTb2Z0d2FyZQBBZG9iZSBJbWFnZVJlYWR5ccllPAAAAAxQTFRF////29vbr6+vAAAAk1hCcwAAAAR0Uk5T////AEAqqfQAAAKcSURBVHja7NrbctswDATQXfD//zlpO7FlmwAWIOnOtNaTM5JwDMa8E+PNFz7g3waJ24fviyDPgfhz8fHP39cBcBL9KoJbQUxjA2iYqHL3FAnvzhL4GtVNUcoSZe6eSHizBcK5LL7dBr2AUZlev1ARRHCljzRALIEog6H3U6bCIyqIZdAT0eBuJYaGiJaHSjmkYIZd+qSGWAQnIaz2OArVnX6vrItQvbhZJtVGB5qX9wKqCMkb9W7aexfCO/rwQRBzsDIsYx4AOz0nhAtWu7bqkEQBO0Pr+Ftjt5fFCUEbm0Sbgdu8WSgJ5NgH2iu46R/o1UcBXJsFusWF/QUaz3RwJMEgngfaGGdSxJkE/Yg4lOBryBiMwvAhZrVMUUvwqU7F05b5WLaUIN4M4hRocQQRnEedgsn7TZB3UCpRrIJwQfqvGwsg18EnI2uSVNC8t+0QmMXogvbPg/xk+Mnw/6kW/rraUlvqgmFreAA09xW5t0AFlHrQZ3CsgvZm0FbHNKyBmheBKIF2cCA8A600aHPmFtRB1XvMsJAiza7LpPog0UJwccKdzw8rdf8MyN2ePYF896LC5hTzdZqxb6VNXInaupARLDNBWgI8spq4T0Qb5H4vWfPmHo8OyB1ito+AysNNz0oglj1U955sjUN9d41LnrX2D/u7eRwxyOaOpfyevCWbTgDEoilsOnu7zsKhjRCsnD/QzhdkYLBLXjiK4f3UWmcx2M7PO21CKVTH84638NTplt6JIQH0ZwCNuiWAfvuLhdrcOYPVO9eW3A67l7hZtgaY9GZo9AFc6cryjoeFBIWeU+npnk/nLE0OxCHL1eQsc1IciehjpJv5mqCsjeopaH6r15/MrxNnVhu7tmcslay2gO2Z1QfcfX0JMACG41/u0RrI9QAAAABJRU5ErkJggg==)](https://t.me/zerobotplugin)
  [![zerobot](https://img.shields.io/badge/zerobot-v1.8.2-black?style=flat-square&logo=go)](https://github.com/wdvxdr1123/ZeroBot)



  [![license](https://img.shields.io/github/license/FloatTech/ZeroBot-Plugin.svg?style=flat-square&logo=gnu)](https://raw.githubusercontent.com/FloatTech/ZeroBot-Plugin/master/LICENSE)
  [![tencent-qq](https://img.shields.io/badge/%E7%BE%A4-1048452984-red?style=flat-square&logo=tencent-qq)](https://jq.qq.com/?_wv=1027&k=QMb7x1mM)
  [![tencent-guild](https://img.shields.io/badge/%E9%A2%91%E9%81%93-Zer0BotPlugin-yellow?style=flat-square&logo=tencent-qq)](https://pd.qq.com/s/fjkx81mnr)
  [![telegram](https://img.shields.io/badge/Telegram-click%20me-informational?style=flat-square&logo=telegram)](https://t.me/zerobotplugin)

  本项目符合 [OneBot](https://github.com/howmanybots/onebot) 标准，可基于以下项目与机器人框架/平台进行交互
  | 项目地址 | 平台 | 核心作者 | 备注 |
  | :---: | :---: | :---: | :---: |
  | [LLOneBot](https://github.com/LLOneBot/LLOneBot) | NTQQ | linyuchen | 目前推荐使用 |
  | [go-cqhttp](https://github.com/Mrs4s/go-cqhttp) | [MiraiGo](https://github.com/Mrs4s/MiraiGo) | Mrs4s | 因签名原因不再维护 |
  | [onebot-kotlin](https://github.com/yyuueexxiinngg/onebot-kotlin) | [Mirai](https://github.com/mamoe/mirai) | yyuueexxiinngg | 不再积极维护 |
  | [oicq/http-api](https://github.com/takayama-lily/oicq/tree/master/http-api) | [OICQ](https://github.com/takayama-lily/oicq) | takayama | 已归档不再维护 |

  [![Star Trend](https://star-history.dera.page/svg?repos=FloatTech/ZeroBot-Plugin&type=Timeline)](https://seladb.github.io/StarTrack-js/#/preload?r=FloatTech,ZeroBot-Plugin)

</div>

> 专为[后 go-cqhttp 时代](https://github.com/Mrs4s/go-cqhttp/issues/2471)开发迁移的`类zbp`新机器人现已出炉，基于官方api，稳定不风控: [NanoBot-Plugin](https://github.com/FloatTech/NanoBot-Plugin)

> 如果您对开发插件感兴趣，欢迎加入[ZeroBot-Plugin-Playground](https://github.com/FloatTech/ZeroBot-Plugin-Playground)

> webui持续开发中, 欢迎加入[ZeroBot-Plugin-Webui](https://github.com/FloatTech/ZeroBot-Plugin-Webui)

## 命令行参数
> `[]`代表是可选参数
```bash
zerobot [-h] [-m] [-n nickname] [-t token] [-u url] [-g url] [-p prefix] [-d|w] [-c|s config.json] [-l latency] [-r ringlen] [-x max process time] [-mirror] [qq1 qq2 qq3 ...] [&]
```
- **-h**: 显示帮助
- **-m**: 不自动标记消息为已读
- **-n nickname**: 设置默认昵称，默认为`椛椛`
- **-t token**: 设置`AccessToken`，默认为空
- **-u url**: 设置`Url`，默认为`ws://127.0.0.1:6700`
- ~~**-g url**~~(默认禁用): 设置`webui url`，默认为`127.0.0.1:3000`
- **-p prefix**: 设置命令前缀，默认为`/`
- **-d|w**: 开启 debug | warning 级别及以上日志输出
- **-c config.json**: 从`config.json`加载`bot`配置
- **-s config.json**: 保存现在`bot`配置到`config.json`
- **-l latency**: 全局处理延时 (ms)
- **-r ringlen**: 接收消息环缓冲区大小，`0`为不设缓冲，并发处理
- **-x max process time**: 最大处理时间 (min)
- **-mirror**: 直接使用镜像懒加载数据站而不尝试访问源站
- **qqs**: superusers 的 qq 号
- **&**: 驻留在后台，必须放在最后，仅`Linux`下有效

默认配置文件格式如下。当选择从配置文件加载时，将忽略相应命令行参数。
```json
{
  "zero": {
    "nickname": ["椛椛", "ATRI", "atri", "亚托莉", "アトリ"],
    "command_prefix": "/",
    "super_users": [],
    "ring_len": 4096,
    "latency": 233000000,
    "max_process_time": 240000000000,
    "mark_message": true
  },
  "ws": [{ "Url": "ws://127.0.0.1:6700", "AccessToken": "" }],
  "wss": null
}
```

## 功能
> 在编译时，以下功能除插件控制外，均可通过注释`main.go`中的相应`import`而物理禁用，减小插件体积。

> 通过插件控制，还可动态管理某个功能在某个群的打开/关闭。

> 插件的优先级为`import`的先后顺序。

> `webui`默认禁用不编译，打开后会增加程序体积。

<details>
  <summary>插件控制</summary>

  - [x] /响应 (在发送的群/用户开始工作)

  - [x] /沉默 (在发送的群/用户停止工作)

  - [x] /全局响应 (在所有位置开始工作，无视单独的沉默)

  - [x] /全局沉默 (在所有本应沉默的位置停止工作，显式指定启用的位置不受影响)

  - [x] /启用 xxx (在发送的群/用户启用xxx)

  - [x] /禁用 xxx (在发送的群/用户禁用xxx)

  - [x] /此处启用所有插件

  - [x] /此处禁用所有插件

  - [x] /全局启用 xxx

  - [x] /全局禁用 xxx

  - [x] /还原 xxx (在发送的群/用户还原xxx的开启状态到初始状态)

  - 注：当全局未配置或与默认相同时，状态取决于单独配置，后备为默认配置；当全局与默认不同时，状态取决于全局配置，单独配置失效。

  - [x] /改变默认启用状态 xxx

  - [x] /禁止 service qq1 qq2... (禁止 qqs 使用服务 service)

  - [x] /允许 service qq1 qq2... (重新允许 qqs 使用服务 service)

  - [x] /封禁 qq1 qq2... (禁止 qqs 使用全部服务)

  - [x] /解封 qq1 qq2... (允许 qqs 使用全部服务)

  - [x] /用法 xxx

  - [x] /服务列表

  - [x] /设置服务列表显示行数 xx (默认值为 9, 该设置仅运行时有效, zbp 重启后重置)

  - [x] (默认禁用) /设置webui用户名 zerobot 密码 123456

  - [x] (默认禁用) /webui启动

  - [x] (默认禁用) /webui停止

  - [x] @Bot 插件冲突检测 (会在本群发送一条消息并在约 1s 后撤回以检测其它同类 bot 中已启用的插件并禁用)

</details>
<details>
  <summary>动态加载插件</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin-Dynamic/dyloader"`

  - 本功能需要`cgo`，故已分离出主线。详见[ZeroBot-Plugin-Dynamic](https://github.com/FloatTech/ZeroBot-Plugin-Dynamic)

</details>

### *高优先级*
<details>
<summary>戳一戳互动</summary>

戳一戳机器人即可触发。插件 chat 独立开关；固定基础回应和群空调已删除。

</details>

<details>
  <summary>睡眠管理</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/sleep_manage"`

  - [x] 早安 | 晚安

</details>
<details>
  <summary>违禁词检测</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/antiabuse"
  `
  - [x] 添加违禁词

  - [x] 删除违禁词

  - [x] 查看违禁词

</details>

<details>
  <summary>群管</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/manager"`

  自动撤回已并入群管，保留独立会话开关：`开启自动撤回`、`关闭自动撤回`、`查看群管功能开关`。见[群管说明](docs/local-admin/group-management.md)。

  - [x] 禁言[@xxx][分钟]

  - [x] 解除禁言[@xxx]

  - [x] 我要自闭 | 禅定 x [分钟 | 小时 | 天]

  - [x] 开启全员禁言

  - [x] 解除全员禁言

  - [x] 升为管理[@xxx]

  - [x] 取消管理[@xxx]

  - [x] 修改名片[@xxx][xxx]

  - [x] 修改头衔[@xxx][xxx]

  - [x] 申请头衔[xxx]

  - [x] 踢出群聊[@xxx]

  - [x] 退出群聊[群号]@Bot

  - [x] \*入群欢迎

  - [x] \*退群通知

  - [x] 设置欢迎语[欢迎~]  可选添加 [{at}] [{nickname}] [{avatar}] [{id}]

  - [x] 在[MM]月[dd]日的[hh]点[mm]分时(用[url])提醒大家[xxx]

  - [x] 在[MM]月[每周 | 周几]的[hh]点[mm]分时(用[url])提醒大家[xxx]

  - [x] 取消在[MM]月[dd]日的[hh]点[mm]分的提醒

  - [x] 取消在[MM]月[每周 | 周几]的[hh]点[mm]分的提醒

  - [x] 在"cron"时(用[url])提醒大家[xxx]

  - [x] 取消在"cron"的提醒

  - [x] 列出所有提醒

  - [x] 翻牌

  - [x] 赞我

  - [x] 群签到

  - [x] [开启 | 关闭]入群验证

  - [x] [开启 | 关闭]gist加群自动审批

  - [x] 对信息回复:[设置 | 取消]精华

  - [x] 取消精华 [信息ID]

  - [x] /精华列表

  - [ ] 同意好友请求

  - [x] 对信息回复: 撤回

  - [ ] 警告[@xxx]

  - 注：使用gist加群自动审批，请在群介绍添加以下说明，同时开启`需要回答问题并由管理员审核`：加群请在github新建一个gist，其文件名为本群群号的字符串的md5(小写)，内容为一行，是当前unix时间戳(10分钟内有效)。然后请将您的用户名和gist哈希(小写)按照username/gisthash的格式填写到回答即可。

  - 设置欢迎语可选添加参数说明：{at}可在发送时艾特被欢迎者 {nickname}是被欢迎者名字 {avatar}是被欢迎者头像 {uid}是被欢迎者QQ号 {gid}是当前群群号 {groupname} 是当前群群名

</details>
<details>
  <summary>群应用：AI声聊</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/airecord"`

  - [x] 设置AI语音群号1048452984(tips：机器人任意所在群聊即可)

  - [x] 设置AI语音模型

  - [x] 查看AI语音配置

  - [x] 发送AI语音xxx

</details>
<details>
  <summary>定时指令触发器</summary>

  `import _ "github.com/FloatTech/zbputils/job"`

  - 注意：触发器具有限速，每 2s 仅允许最多一次触发

  - [x] 记录以"完全匹配关键词"触发的(代表我执行的)指令

  - [x] 取消以"完全匹配关键词"触发的(代表我执行的)指令

  - [x] 记录在"cron"触发的(别名xxx的)指令

  - [x] 取消在"cron"触发的指令

  - [x] 查看所有触发指令

  - [x] 查看在"cron"触发的指令

  - [x] 查看以"完全匹配关键词"触发的(代表我执行的)指令

  - [x] 注入指令结果：任意指令

  - [x] 执行指令：任意指令

  - 注：任意指令可以使用形如`?::参数1提示语::1!`,`?::参数2提示语::2!`,`?::?可选参数3提示语，不回答将填入空值::3!`,`!::从url获取的参数::4!`,`!::?可选的从url获取的参数，出错将填入空值::5!`的未定参数，在注入时一一匹配

  - 一些示例

> 每日12:00以1/2概率执行一言指令

```python
记录在"0 12 * * *"触发的指令
注入指令结果：>runcoderaw py
from random import random
if random() > 0.5: print('一言')
else: print('今天没有一言哦~')
```

> 每日15:00询问设置定时者否想看一言

```python
记录在"0 15 * * *"触发的指令
注入指令结果：>runcoderaw py
if '?::想看一言吗？::1!' == '想': print('一言')
else: print('好吧')
```

> 自行编写简易的选择困难症助手小插件

```python
记录以"简易的选择困难症助手"触发的指令
执行指令：>runcoderaw py
from random import random
if random() > 0.5: print('您最终会选?::请输入您的选择1::1!')
else: print('您最终会选?::请输入您的选择2::2!')
简易的选择困难症助手
```

> 自行编写随机b站404页趣图插件

```python
记录以"随机b站404页趣图"触发的代表我执行的指令
注入指令结果：>runcoderaw py
import json
j = json.loads(r'''!::https://api.iyk0.com/bili_chart::1!''')
print("run[CQ:image,file="+j["img"]+"]")
随机b站404页趣图
```

![随机b站404页趣图](https://user-images.githubusercontent.com/41315874/157371451-c09ad3bb-c61a-4a42-9c47-fab3305bc0f8.png)

  - [x] [我|大家|有人][说|问][正则表达式]你[答|说|做|执行][模版]

  - [x] [查看|看看][我|大家|有人][说|问][正则表达式]

  - [x] 删除[大家|有人|我][说|问|让你做|让你执行][正则表达式]

  - 注：模版是指含有`$1` `$2`这样的未定参数，会在正则匹配时按顺序填入子匹配对应值

</details>

### *中优先级*

<details>
  <summary>AIfalse</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/ai_false"`

  - [x] 查询计算机当前活跃度: [检查身体 | 自检 | 启动自检 | 系统状态]

  - [x] 设置默认限速为每 m [分钟 | 秒] n 次触发

</details>
<details>
  <summary>aiimage</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/aiimage"`

  - [x] 设置AI画图密钥xxx
  - [x] 设置AI画图接口地址https://api.siliconflow.cn/v1/images/generations
  - [x] 设置AI画图模型名Kwai-Kolors/Kolors
  - [x] 查看AI画图配置
  - [x] AI画图 [描述]

</details>






<details>
  <summary>B站资料查询、链接解析与订阅推送</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/bilibiliparse"`

  - [x] 统一B站资料查询、链接解析与动态/直播订阅（P019/P021并入P020）
  - [x] 解析、动态推送、直播推送分别开关，旧查询及订阅命令保留
  - 使用说明：[统一B站功能](docs/local-admin/bilibili.md)

  - [x] t.bilibili.com/642277677329285174 | bilibili.com/read/cv17134450 | bilibili.com/video/BV13B4y1x7pS | live.bilibili.com/22603245

</details>












<details>
  <summary>漂流瓶</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/drift_bottle"`

  - [x] @Bot pick (随机捞一个漂流瓶)

  - [x] @Bot throw xxx (投递内容xxx,支持图片文字,投递内容需要大于10个字符或者带有图片)

</details>


<details>
  <summary>好友申请及群聊邀请事件处理</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/event"`

  - [x] [开启|关闭]自动同意[申请|邀请|主人]

  - [x] [同意|拒绝][申请|邀请][flag]

  - flag跟随事件一起发送, 默认同意主人的事件

</details>















<details>
  <summary>kokomi原神面板</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/kokomi"`

  - [x] kokomi菜单

  - [x] XX面板

  - 注:本插件未并入主仓库,需自行安装(须源码方式运行才能添加插件),安装地址[kokomi-原神面板查询插件](https://github.com/lianhong2758/kokomi-plugin)

</details>



<details>
  <summary>钓鱼模拟器</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/mcfish"`

  - [x] 钓鱼商店
  - [x] 购买xxx [数量]
  - [x] 出售[xxx [数量]|所有垃圾]
  - [x] 钓鱼背包
  - [x] 装备[xx竿|三叉戟|美西螈]
  - [x] 附魔[诱钓|海之眷顾]
  - [x] 修复鱼竿
  - [x] 合成[xx竿|三叉戟]
  - [x] 进行钓鱼
  - [x] 进行n次钓鱼

</details>

<details>
  <summary>Minecraft服务器监控&订阅</summary>

`import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/minecraftobserver"`

- [x] mc服务器状态 [服务器IP/URI]
- [x] mc服务器添加订阅 [服务器IP/URI]
- [x] mc服务器取消订阅 [服务器IP/URI]
- [x] mc服务器订阅拉取 （需要插件定时任务配合使用，全局只需要设置一个）
  - 使用job插件设置定时, 对话例子如下:：
    - 记录在"@every 1m"触发的指令
    - （机器人回答：您的下一条指令将被记录，在@@every 1m时触发）
    - mc服务器订阅拉取
</details>











<details>
  <summary>一群一天一夫一妻制群老婆</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/qqwife"`

  - 引入好感度系统，好感度越高，自由恋爱成功率越高

  - [x] 设置CD为xx小时

  - [x] [允许|禁止]自由恋爱

  - [x] [允许|禁止]牛头人

  - [x] 娶群友

  - [x] [娶|嫁][@对方QQ]

  - [x] 当[对方Q号|@对方QQ]的小三

  - [x] 做媒 @攻方QQ @受方QQ

  - [x] 买礼物给[对方Q号|@对方QQ]

  - [x] 群老婆列表

  - [x] 查好感度[对方Q号|@对方QQ]

  - [x] 好感度列表

  - [x] 重置花名册

</details>







<details>
  <summary>签到与钱包</summary>

`import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/score"`

  - [x] 签到
  - [x] 获得签到背景[@xxx] | 获得签到背景
  - [x] 设置签到预设(0~3)
  - [x] 查看等级排名
  - 注:跨群排行
  - [x] 查看我的钱包 / 查看钱包余额[@xxx]
  - [x] 钱包转账金额[@xxx]（群内）
  - [x] 设置硬币名称XX / 管理钱包余额[+金额|-金额][@xxx]（主人）
  - 使用说明：[签到与钱包](docs/local-admin/signin-wallet.md)
  - [x] 查看钱包排名
  - 注:本群排行，若群人数太多不建议使用该功能!!!

</details>


<details>
  <summary>抽塔罗牌</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/tarot"`

  - [x] 抽[塔罗牌|大阿卡纳|小阿卡纳]
  - [x] 抽n张[塔罗牌|大阿卡纳|小阿卡纳]
  - [x] 解塔罗牌[牌名]
  - [x] [塔罗|大阿卡纳|小阿卡纳|混合]牌阵[圣三角|时间之流|四要素|五牌阵|吉普赛十字|马蹄|六芒星]

</details>



<details>
  <summary>vtb语录</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/vtb_quotation"`

  - [x] vtb语录

  - [x] 随机vtb

  - [x] 更新vtb

</details>



<details>
  <summary>角色库：每日抽取、自定义角色与猜角色</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/wife"`

  - 抽老婆：当前群自定义库优先，无自定义角色时使用内置库
  - 抽内置老婆：明确使用内置库
  - 抽wife、添加wife、删除wife：兼容原群自定义角色命令
  - 猜老婆：保留内置角色猜题
  - 权限、素材与开关见[角色库说明](docs/local-admin/role-library.md)
</details>
<details>
  <summary>聊天热词</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/word_count"`

  - [x] 热词 [消息数目]|热词 1000

</details>
<details>
  <summary>猜单词</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/wordle"`

  - [x] 中文成语与英文单词统一入口、开关和群内对局互斥
  - [x] 个人/团队猜词 中文；个人/团队猜词 英文；结束猜词
  - 使用说明：[猜词游戏](docs/local-admin/word-games.md)

  - [x] 个人猜单词

  - [x] 团队猜单词

  - [x] 团队六阶猜单词

  - [x] 团队七阶猜单词

</details>
<details>
  <summary>鬼东西</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/wtf"`

  - [x] 鬼东西列表

  - [x] 查询鬼东西[序号][@xxx]

  - 注：由于需要科学，默认注释。

</details>



### *低优先级*


<details>
<summary>AI中心：聊天、Agent、GPT与群聊总结</summary>

插件 aichat 统一控制。私聊发送文字、群内@机器人、/gpt 问题或群聊总结 10。原AI配置命令保留，详见 [AI中心与群互动](docs/local-admin/ai-center.md)。

</details>


<details>
  <summary>打断复读</summary>

  `import _ "github.com/FloatTech/ZeroBot-Plugin/plugin/breakrepeat"`

  - [x] (打断三次以上的复读)

</details>

## 三种使用方法，推荐第一种

### 1. 使用稳定版/测试版 (推荐)

可以前往[Release](https://github.com/FloatTech/ZeroBot-Plugin/releases)页面下载对应系统版本可执行文件，编译时开启了全部插件。
### 2. 本地直接运行

1. 下载安装最新 [Go](https://studygolang.com/dl) 环境
2. 下载本项目[压缩包](https://github.com/FloatTech/ZeroBot-Plugin/archive/master.zip)，本地解压
3. 编辑 main.go 文件，内容按需修改
4. 运行 OneBot 框架
5. `Windows`下双击 run.bat 文件，`Linux`下使用 run.sh 运行本插件

### 3. 编译运行

#### a. 利用 Actions 在线编译

1. 点击右上角 Fork 本项目，并转跳到自己 Fork 的仓库
2. 点击仓库上方的 Actions 按钮，确认使用 Actions
3. 编辑 main.go 文件，内容按需修改
4. 前往 Release 页面发布一个 Release，`tag`形如`v1.2.3`，以触发稳定版编译流程
5. 点击 Actions 按钮，等待编译完成，回到 Release 页面下载编译好的文件
6. 运行 OneBot 框架，并同时运行本插件
7. 啾咪~

#### b. 本地编译/交叉编译

1. 下载安装最新 [Go](https://studygolang.com/dl) 环境
2. clone 并进入本项目，下载所需包

```bash
git clone --depth=1 https://github.com/FloatTech/ZeroBot-Plugin.git
cd ZeroBot-Plugin
go version
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GO111MODULE=auto
go mod tidy
go generate main.go
```

3. 编辑 main.go 文件，内容按需修改
4. 按照平台输入命令编译，下面举了一些例子

```bash
# 本机平台
go build -ldflags "-s -w" -o zerobot -trimpath
# x64 Linux 平台 如各种云服务器
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o zerobot -trimpath
# x64 Windows 平台 如大多数家用电脑
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o zerobot.exe -trimpath
# armv6 Linux 平台 如树莓派 zero W
GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=0 go build -ldflags "-s -w" -o zerobot -trimpath
# （由于引入了github.com/fumiama/sqlite3，本项不再可用）mips Linux 平台 如 路由器 wndr4300
GOOS=linux GOARCH=mips GOMIPS=softfloat CGO_ENABLED=0 go build -ldflags "-s -w" -o zerobot -trimpath
```

5. 运行 OneBot 框架，并同时运行本插件

## 特别感谢

- [ZeroBot](https://github.com/wdvxdr1123/ZeroBot)
- [ATRI](https://github.com/Kyomotoi/ATRI)

同时感谢以下开发者对 ZeroBot-Plugin 作出的贡献：

<a href="https://github.com/FloatTech/ZeroBot-Plugin/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=FloatTech/ZeroBot-Plugin&max=1000" />
</a>

请求次数排序已完成开发、72项回归及部署独立检查，[PR #47](https://github.com/qiye-yuri/ZeroBot-Plugin/pull/47)保持依赖草稿；单条QQ集中非空验收累计2/5，未合入。见[验收记录](docs/local-admin/task-count-sort-acceptance.md)。

下一批见[窗口内请求次数筛选计划](docs/local-admin/post-pr47-development-plan.md)：区分单次与多次窗口请求，4个工作包及10项验收标准。该规划现已实现，最新部署及验收见下方清单。

窗口内请求次数筛选已实现，支持全部、仅1次、至少2次与现有条件组合；[使用说明](docs/local-admin/model-request-statistics.md#按窗口内请求次数筛选)及[验收状态](docs/local-admin/task-count-filter-acceptance.md)。

[PR #48](https://github.com/qiye-yuri/ZeroBot-Plugin/pull/48)已完成次数筛选、74项回归和部署独立检查，QQ集中非空验收累计3/5；依赖#47保持草稿未合入。

下一批见[筛选结果请求覆盖统计计划](docs/local-admin/post-pr48-development-plan.md)：复用完整匹配组计数，补充窗口请求覆盖范围。4个工作包、10项验收标准；现已实现，部署及本轮提前集中验收见[本批清单](docs/local-admin/task-request-coverage-acceptance.md)。

四项最终交付（2026-10-28）：PR #46～#49已完成集中非空验收并按依赖顺序合入master，75项回归及完整隔离通过，报告下载副本已清理。队列清空0/5；此前草稿/待验收描述为历史阶段。详见[最终记录](docs/local-admin/task-request-coverage-acceptance.md#四项合入收尾2026-10-28)。
