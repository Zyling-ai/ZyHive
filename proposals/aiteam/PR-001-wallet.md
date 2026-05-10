# PR-001 · Wallet 钱包抽象

> 状态: 📝 spec 收集中
> 优先级: 🔴 P0（aiteam Genesis 跑真业务前必备）
> 依赖: 无（钱包是 Payroll / Judge / Revenue 的基础）
> 默认 off：experimental flag `ZYHIVE_EXPERIMENTAL_WALLET=1`

---

## 0. 待用户提供（spec 收集清单）

请贴 markdown 描述以下决策，本文件 § 1 起从此处展开。

- [ ] **账户模型**
  - 每个 agent 一个钱包？还是一个 team 共享一个？
  - 嵌套账户（团队下子账户）？
  - 是否有人类账户（owner 入金 / 出金通道）？
- [ ] **计价单位**
  - USD（接现实金钱）？
  - 内部 credits（仿货币，与现实解耦）？
  - 多币种共存？汇率怎么处理？
- [ ] **计费维度**
  - 按 token？复用 `pkg/usage` 现有数据
  - 按 wall-clock？按工具调用次数？
  - 按"任务完成度"（依赖 PR-004 Judge 评分）？
- [ ] **持久化形式**
  - SQLite（事务安全，新依赖）？
  - JSONL append-only（与现有 `pkg/usage`/`session` 风格一致）？
  - 直接复用 `pkg/usage` 累计数据 + 新增余额视图层？
- [ ] **API 形态**
  - REST：`GET/POST /api/aiteam/wallet/:agentId`
  - AI 工具：`wallet_balance(agentId)` / `wallet_transfer(from, to, amount)` / `wallet_deduct(agentId, amount, reason)`
  - 哪些操作 AI 自己能做，哪些要走人类审批？
- [ ] **零余额行为**
  - hard stop 工具调用 → AI 看到 `❌ 余额不足` 自我纠错
  - soft warn 但继续 → 透支记账
  - 与 PR-003 BudgetGuard 是何关系？（重叠？正交？）
- [ ] **审计**
  - `wallet/changes.log`（与 `network/changes.log` 同模式）？
  - 谁能看？AI 自己 read？只有 owner UI？

## 1. 背景
（待用户填）

## 2. 设计
（待用户填）

## 3. 实施步骤
（待用户填）

## 4. 测试计划
（待用户填）

## 5. experimental flag
- 名称：`ZYHIVE_EXPERIMENTAL_WALLET`
- 默认：`""` (off)
- 启用值：`"1"` 或 `"true"`
- 关闭时行为：所有 wallet API 返回 `404 not enabled`，工具不注册

## 6. 兼容性 / 回滚
- 数据存 `workspace/aiteam/wallet/` 子目录，回滚直接删
- 主 schema 不变，零迁移
