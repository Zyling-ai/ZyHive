> [!WARNING]
> 历史资料：仅用于追溯设计与决策，不代表 `26.8.2v1` 当前实现、支持范围或发布承诺。当前事实请从 [文档中心](../README.md) 开始核对。

# 历史文档索引

> **警告：本页列出的材料不代表当前实现、默认配置、稳定能力、生产部署状态或交付承诺。**

本目录集中保存历史材料，并尽量保留文件名和 Git 迁移记录。判断当前行为时，必须回到当前版本代码与测试、同版本正式 Release、`CHANGELOG.md` 和当前维护文档。历史设计完成、演示成功、旧部署可访问或页面截图存在，都不能证明 `26.8.2v1` 仍以相同方式工作。

## 分类规则

- **路线与计划**：描述未来方向、优先级或当时目标；未完成项为 Planned，不是发布日期承诺。
- **审计与快照**：记录特定时间、版本和测试条件下的观察；结论不能无条件外推到后续版本。
- **设计与实现快照**：解释某一阶段的内部结构；接口、默认值和模块边界可能已变化。
- **实验协议与演示**：用于 Labs 验证；不构成生产、安全、财务或商业承诺。
- **商业概念探索**：用于方向讨论；不代表已实现产品或当前经营计划。

## 路线与计划

- `docs/archive/roadmaps/optimization-roadmap-2026-2027.md`：2026—2027 优化方向和执行基线；方向不等于已交付。
- `docs/archive/roadmaps/roadmap-v0.10.md`：v0.10 历史前瞻稿；不得用于判断当前版本能力。

使用要求：引用具体项目时同时标记 `Planned`、适用日期和当前验证结果；完成情况以代码、测试和正式 Release 为准。

## 审计与版本快照

- `docs/archive/audits/comprehensive-audit-2026-08-02.md`：针对特定版本和测试条件的质量与交互审计。

使用要求：保留审计版本、日期、环境和方法。覆盖率、漏洞、性能、通过项与风险接受结论均是时间点数据，不是永久保证。

## 设计与实现快照

- `docs/archive/designs/system-prompt-and-flow.md`：系统提示词与运行流程快照。
- `docs/archive/designs/session-design.md`：会话设计快照。
- `docs/archive/designs/skillopt-design.md`：SkillOpt 实验设计与演进参考。
- `docs/archive/designs/agent-cli.md`：被当前开发者 API/CLI 参考替代的旧说明。

使用要求：不得把示例结构、接口、事件顺序或默认值直接复制到当前参考文档；先对照 `pkg/`、`internal/api/`、测试和当前配置 schema。

## aiteam 实验协议、部署与演示

- `docs/labs/aiteam/aiteam-architecture.md`：自治经济实验架构。
- `docs/labs/aiteam/aiteam-wallet-protocol.md`：实验钱包协议。
- `docs/labs/aiteam/aiteam-fx-and-currency.md`：实验汇率与币种语义。
- `docs/labs/aiteam/aiteam-revenue-protocol.md`：实验收益接入协议。
- `docs/labs/aiteam/aiteam-deploy-aws.md`：历史 AWS 实验部署记录。
- `docs/labs/aiteam/aiteam-genesis-demo.md`：Genesis 演示步骤和结果。
- `proposals/aiteam/`：分阶段提案、PR 计划和缺陷记录。

这些材料全部按 `Labs` 处理。历史地址、云资源、密钥名称、示例余额、收益、评分和演示成功记录不代表服务仍在线、配置仍安全或数据可用于真实结算。当前实验边界见 [`docs/labs/aiteam.md`](../labs/aiteam.md)。

## ZyStudio 商业概念探索

- `docs/archive/zystudio/README.md`
- `docs/archive/zystudio/concept.md`
- `docs/archive/zystudio/roadmap.md`
- `docs/archive/zystudio/economics.md`

这些文件属于历史概念、路线和经济模型探索，不证明仓库已实现 ZyStudio，也不构成定价、收益、融资、代币、上线日期或客户交付承诺。

## 引用历史材料时

引用必须同时给出：

1. 文件路径、文档日期和适用版本；
2. “历史材料，不代表当前实现”的显著说明；
3. 当前代码/测试/Release 的交叉验证结果；
4. 若用于未来工作，明确标记 `Planned`；若用于实验，明确标记 `Labs`。

禁止仅凭历史材料更新销售文案、安全承诺、部署步骤、API 参考、版本兼容或 Stable 状态。

## 后续归档规则

当当前文档被新规范替代时，优先保留 Git 历史；只有仍具长期背景价值的材料才加入本索引。加入索引不要求移动文件，但必须移除或改写会让读者误认为其仍是当前规范的入口链接。涉及客户数据、凭据或内部基础设施的信息应先清理，不因“归档”而保留敏感内容。
