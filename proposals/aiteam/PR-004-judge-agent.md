# PR-004 · Judge Agent 评判智能体

> 状态: 📝 spec 收集中
> 优先级: 🔴 P0
> 依赖: 无（可独立实现，结果输入给 PR-002 Payroll）
> 默认 off：experimental flag `ZYHIVE_EXPERIMENTAL_JUDGE=1`

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
