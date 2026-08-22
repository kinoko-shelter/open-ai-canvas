# 故事创作 · AI 文档索引

面向 AI 的短索引。详细文档维护规则见 [AGENTS.md](../AGENTS.md) 第 10 节「文档同步」。

## 合并合同

- 产品运行时品牌固定为“故事创作”。上游同步不得覆盖工作区、后台或 Canvas Agent 的品牌文案；具体扫描要求见 [AGENTS.md](../AGENTS.md) 第 11 节「Git、提交与发布」。
- 上游功能按团队、项目、权限、账务和资源归属合同逐块迁移，不直接用整文件覆盖本地业务实现。
- 系统渠道模型是能力、规格、上游 SKU 与用户价格的唯一配置源。一个 `ChannelModel` 可有多条 `ChannelModelPriceTier`，按 `resolution + videoSeconds` 匹配；`*` 与 `0` 分别表示任意分辨率、任意时长，精确规格优先于通配规格。
- `ChannelModel.modelKey` 是稳定的产品模型标识；每个价格档的 `providerModelKey` 才是该规格实际发送给上游的模型 ID。不得因分辨率或时长新增重复前台模型，也不得把渠道模型的规格拆回多个独立 SKU。
- 前台逻辑模型是系统渠道模型的只读投影：保存渠道模型会创建新的逻辑模型 revision 并切换 active revision；前台不得重复编辑能力、默认参数、供应线路或价格。历史 `Task`、`RouteAttempt`、`BillingOrder` 和旧 revision 必须保持原引用及金额快照。
- 账单必须记录实际命中的 `ChannelModelID`、`PriceTierID`、版本、单价和最终金额；上游同步或价格调整不得回写历史任务与订单。新增模型治理或上游合并前必须检查这些约束，不能用整页或整文件覆盖恢复双配置。

## 设计沉淀

- [工作区外壳设计沉淀](design/workspace-shell-design.mdx)：侧栏（260px 可折叠导航 + 分组折叠）、主区卡片、顶部栏（账户/公告/主题）的设计决策与样式约束。

## 本地协作文档（不随仓库分发）

- [beautifului 创作设计](beautifului-creation-design.md)：本地设计参考，未纳入版本控制。

## 按约定维护的文档（`docs/content/docs/`）

功能、代码地图、待办、待测试分别维护在以下页面（当前缺失，待后续任务重建）：

- 功能：`features.mdx`
- 代码地图：`code-map.mdx`
- 待办：`todo.mdx`
- 待测试：`pending-test.mdx`
