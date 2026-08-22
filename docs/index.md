# 故事创作 · AI 文档索引

面向 AI 的短索引。详细文档维护规则见 [AGENTS.md](../AGENTS.md) 第 10 节「文档同步」。

## 合并合同

- 产品运行时品牌固定为“故事创作”。上游同步不得覆盖工作区、后台或 Canvas Agent 的品牌文案；具体扫描要求见 [AGENTS.md](../AGENTS.md) 第 11 节「Git、提交与发布」。
- 上游功能按团队、项目、权限、账务和资源归属合同逐块迁移，不直接用整文件覆盖本地业务实现。
- 系统渠道模型是能力、规格、上游 SKU 与用户价格的唯一配置源。一个 `ChannelModel` 代表一个模型家族，`ChannelModelPriceTier` 用规范 `selector` 匹配可计费 SKU：视频使用 `operation + vquality + videoSeconds`，图片使用 `operation + quality + size`；缺失字段或 `*` 表示通配，精确组合优先于通配。
- `ChannelModel.modelKey` 是稳定的系统模型家族标识；每个价格档的 `providerModelKey` 才是该规格实际发送给上游的模型 ID。不得因分辨率、时长、输入类型或图片质量新增重复系统模型或前台模型。Mini、Fast、Pro 等独立产品才是独立模型。
- 前台逻辑模型是产品目录与多渠道故障切换层，只维护展示、创作端能力、默认参数和线路优先级/权重。前台模型必须使用 `pricePolicy=channel`，不得维护第二份价格；保存系统渠道模型不得自动创建、覆盖或删除前台模型、团队权限、项目关联或线路配置。
- 已有单条系统线路但未绑定来源的前台模型，使用 `backend/cmd/reseed-logical-model-sources` 先 dry-run、再 `--apply` 规范为系统渠道投影；命令拒绝改写有排队或运行任务引用的模型，也不会处理多线路或非系统渠道模型。
- 路由由后端根据真实输入推导 `operation`，再选择可匹配 SKU、实际渠道和上游模型键；前端只提交质量、尺寸、分辨率和时长，不得传入或缓存上游 SKU。任一回退线路缺少当前规格的 SKU 时，该线路不参与该规格的切换。
- 账单必须记录实际命中的 `ChannelModelID`、`PriceTierID`、价格档版本、规格选择器、单价和最终金额；上游同步或价格调整不得回写历史 `Task`、`RouteAttempt`、`BillingOrder` 和旧 revision。SKU 合并先 dry-run，拒绝存在排队/运行任务或重复 selector 的计划；被家族模型替代的渠道 SKU 软删除、旧前台 SKU 归档，并写入系统渠道退役清单，避免上游目录拉取再次创建重复记录。

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
