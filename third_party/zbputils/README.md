# zbputils
ZeroBot-Plugin 的工具库

## 本地模型参数补丁

本仓库的 `modelconfig/parameters.go` 定义命名模型的可选最大输出长度、TopP和推理设置，以及无机器人初始化依赖的参数合并；`chat/models.go` 将所选模型解析成一次请求的值快照。字段缺失/null继承全局，显式空推理字符串保留；旧配置无需迁移。配置保存沿用原仓库校验、修订、备份和Agent缓存更新。上游升级时需保留此补丁及对应测试，使用和兼容范围见[命名模型说明](../../docs/local-admin/named-models.md)。

swag初始化

```
swag init -g control/web/gui.go -o control/web/docs/
```
