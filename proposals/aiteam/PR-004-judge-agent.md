# PR-004 · Judge Agent 评判智能体

> 状态: ✅ landed S7 (26.5.10v13) — heuristic v0
> 优先级: 🔴 P0
> 依赖: 无（结果输入给 PR-002 Payroll）
> Flag: `ZYHIVE_EXPERIMENTAL_JUDGE=1`

## 落地总结

5 维评分（0-10）：completion / quality / communication / creativity / cost。
Average 输入 PR-002 payroll bonus 计算。

* v0：`HeuristicScorer`
  - cost 维按 usage USD 阈值映射（≤$0.10→10, ≤$0.50→8, ≤$1.00→6, ≤$2.50→4, ≤$5.00→2, >$5.00→0）
  - 其他维给中性 baseline 7-6 分（缺 LLM 评分信号时的最低主见）
  - 决定：留 LLM-driven scorer 作 v1（接 promptdef 包裹 transcript）
* 持久化 `<dataDir>/aiteam/judge/<agentID>/<period>.jsonl`
* Override：`POST /api/aiteam/judge/override` 由 owner 手动覆盖 dim 数值
* 测试：`Test_AITeam_Judge_*` 13 case 全 -race 绿

未来 v1 工作（不在 S0-S10 范围）：
* `LLMScorer` 实现 — 调真模型读 transcript 输出 5 维 + rationale
* transcript 输入强制走 `promptdef.Wrap()` 防注入
* daily cron 自动评分（23:00，刚好 payroll 23:30 跑前）

---

## 0. 待用户提供

- [ ] **Judge 是什么形态**
  - 一个特殊 agent？（在 ZyHive 现有 agent 模型里加一个 `Role: "judge"` 字段？）
  - 一个 cron 任务？（每天扫描所有 sessions 评分）
  - 一个独立外部 LLM 调用？
- [ ] **评分维度**
  - 单维 0-100 分？
  - 多维（任务完成度 / 代码质量 / 沟通 / 创造性 / 成本控制）？
  - 用什么 prompt 模板？
- [ ] **评分输入**
  - 整个 session 完整 transcript？
  - 仅最后产物（工件/文件）？
  - 工具调用 trace？
  - 用户反馈（如果有）？
- [ ] **评分输出去向**
  - 写到 `workspace/aiteam/judge/{agentId}/{period}.jsonl` 让 Payroll 读？
  - 直接调 PR-001 wallet API 增减余额？
  - 推到 UI 让人类 override？
- [ ] **否决权**
  - Judge 能否直接停掉一个 agent（panic-stop）？
  - 还是只能给低分 + 走 Payroll 自然流？
- [ ] **元-judging**
  - 谁评 Judge 自己？
  - 多 Judge 投票降低单点偏差？
- [ ] **prompt 注入防御**
  - 被评 agent 的 transcript 含恶意指令试图操控 Judge → 怎么挡？
  - 与 PR-008 B6 注入防御协同设计

## 1. 背景
（待用户填）

## 2. 设计
（待用户填）

## 3-5. 略

## 6. experimental flag
- `ZYHIVE_EXPERIMENTAL_JUDGE=1`
- 关闭时不创建 judge cron / agent role
