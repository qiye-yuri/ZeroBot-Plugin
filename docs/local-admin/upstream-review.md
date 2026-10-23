# 上游仓库缺失与去留评估

核查日期：2026-10-18。按使用者要求，单列缺少或失效的上游源码仓库，不把接口失败混作源码丢失。本文是阶段清单，不是全部第三方模块的完整仓库审计。

## 已确认缺少上游源码的功能

目前没有足够证据确认某个保留插件的实现源码已不可获得。99个保留插件在本项目中均有源码；[ZeroBot-Plugin主仓库](https://github.com/FloatTech/ZeroBot-Plugin)、[AnimeAPI](https://github.com/FloatTech/AnimeAPI)、[缩写释义上游](https://github.com/itorr/nbnhhsh)、[ATRI参考来源](https://github.com/Kyomotoi/ATRI)及[投胎参考来源](https://github.com/YukariChiba/tgbot)本次可访问。仓库可访问不等于所有功能正常或仍积极维护。

## 已按用户决定删除的来源异常项

| 编号/功能 | 异常来源及证据 | 实际缺失范围 | 最终处理决定 |
| --- | --- | --- | --- |
| P043 gif制图 | [ImageMaterials](https://gitea.seku.su/fumiama/ImageMaterials)本次HTTP 404 | 模板素材仓库；专属制图源码已按需求删除 | 已按用户要求删除专属源码、注册与用户目录入口，不再等待替换资源或自行实现；历史数据保留。 |
| P059 midicreate | [midi安装辅助仓库](https://gitcode.net/anto_july/midi)及原timidity.bat跳转CSDN登录页，200响应实际为HTML | 外部安装脚本公开访问受阻，不能证明仓库已删除；MIDI插件源码已按需求删除 | 已按用户要求删除专属源码、注册与用户目录入口，不再等待替换资源或自行实现；历史数据保留。 |
| P076 qzone | [avatar仓库](https://gitcode.net/anto_july/avatar)跳转登录页 | 匿名头像素材来源，非QQ空间插件源码 | 已按用户要求删除专属源码、注册与用户目录入口，不再等待替换资源或自行实现；历史数据保留。 |

404只说明本次公开地址不可用，登录跳转只说明匿名访问受阻；使用者自行登录仍无法获取后选择删除上述三项，本轮未登录他人仓库，也没有将搜索无结果解释为仓库永久不存在。

## 不属于缺少源码的故障

P091翻译调用的`http://api.cloolc.club/fanyi?data=good`实际返回404，HTTPS另出现SSL连接失败。客户端实现可在AnimeAPI中找到，但本轮没有定位到此服务端的公开实现仓库。当前结论是“旧接口不可用、服务端源码来源未确认”，不是“已证明无法修复”。可选择接入可靠翻译服务，或复用已配置模型自行实现翻译入口；涉及服务选择及费用的方案应另行确定。目前仅修正错误提示，不声称翻译已恢复。

P066缩写释义的正常接口查询成功，有公开上游仓库；已修正HTTP错误/无效数据被显示为空结果的问题。P044 GitHub搜索正常查询成功，修正失败分支缺少return造成的重复误导回复；“没有找到仓库”是搜索结果提示，不代表该插件没有源码。

百人一首与浅草寺求签的素材仓库本次均返回200并显示相应仓库页面，未列入异常候选。后续按[逐项清单](feature-plugin-inventory.md)继续补充业务验证。

原始HTTP状态、最终跳转和响应类型证据保存在本机`.local/feature-execution/upstream-probe.json`；其中不含用户凭据。
