# PR-002 · Payroll 工资发放

> 状态: ✅ landed S8 (26.5.10v14)
> 优先级: 🔴 P0
> 依赖: PR-001 (Wallet) ✅ + PR-004 (Judge) ✅
> Flag: `ZYHIVE_EXPERIMENTAL_PAYROLL=1`

## 落地总结

公式：

```
net = base + (bonusMax * judgeAvg/10) - (usageUSD * offsetRatio)
```

默认配置：base = 0.10 USDT / bonusMax = 0.50 USDT / lookback = 7d / offset = 50%.

* net ≤ 0 → 行记 skipped:true 但不扣钱（防 debt-spiral）
* net > 0 → 自动调 `wallet.Credit`
* 持久化 `<dataDir>/aiteam/payroll/<period>.jsonl`
* 旁路 audit log: `payroll.run`
* AI 工具：暂未暴露（v0 通过 REST 触发 + cron 自动跑留作 v1）
* 测试：`Test_AITeam_Payroll_*` 14 case 全 -race 绿
* 触发：REST `POST /api/aiteam/payroll/run`；daily cron 自动 23:30 留作后续 PR

---

## 0. 待用户提供

- [ ] **触发器**
  - 每天定时？每完成一个任务？每月结算？
  - 谁发起：cron / Judge 完成评分后 / 人类审批？
- [ ] **金额计算**
  - 固定底薪 + 绩效？
  - 纯按 Judge 评分？
  - 考虑 token 消耗（成本中心）？
- [ ] **支付目的地**
  - agent 自己钱包（PR-001）？
  - agent 控制的外部账户（USDT 地址 / 银行账户 / Stripe）？
- [ ] **失败处理**
  - 钱包余额不足扣不动公司付钱怎么办？
  - 网络出错重试策略？
  - 人工介入接口？
- [ ] **审计**
  - 工资单存哪？（`workspace/aiteam/payroll/{period}.jsonl`？）
  - AI 能否查自己历史工资？
  - 公开度（团队内可见 / 仅 agent 自己）？
- [ ] **税务/合规**（如果对接现实）
  - W-2 / 1099 / 中国增值税发票？
  - 暂不做？

## 1. 背景
（待用户填）

## 2. 设计
（待用户填）

## 3-5. 略
（待用户填）

## 6. experimental flag
- `ZYHIVE_EXPERIMENTAL_PAYROLL=1`
- 关闭时所有 payroll cron / API 不注册
