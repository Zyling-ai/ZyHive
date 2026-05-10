# PR-006 · aiteam Dashboard / Observability

> 状态: 🟡 后端 landed S10 (26.5.10v16) · 🔜 前端 UI 留作后续 PR
> 优先级: 🟠 P1
> 依赖: 所有 aiteam 子系统 (S2-S9)
> Flag: `ZYHIVE_EXPERIMENTAL_AITEAM_DASHBOARD=1`

## 落地总结 (backend)

`GET /api/aiteam/overview` — 单次拉取整体状态：

```json
{
  "flags": { "ZYHIVE_EXPERIMENTAL_WALLET": true, ... },
  "any": true,
  "wallet": {
    "total_balance_usdt": "55.00",
    "agents": {"alice": "35.00", "bob": "20.00"},
    "count": 2
  },
  "fx": { "base": "USDT", "rates": {...}, "source": "coingecko", "fetched_at": "..." },
  "guard": { "enabled": true, "day_key": "2026-05-10", "agents": {...} },
  "judge": { "agents": ["alice", "bob"], "avg_7d_by_agent": {"alice": 7.2, "bob": 6.8} },
  "payroll": { "enabled": true },
  "revenue": { "enabled": true }
}
```

每个子系统按自身 flag 出现 / 缺席，gracefully degrade。

`GET /api/aiteam/audit` — v0 返回 503 + hint「直接读
`<dataDir>/aiteam/audit.log`」；UI 接入留后续 PR。

## 待后续 PR

后端 API 全部就位，前端工作量约 2-3 个独立大 PR：

1. **核心 UI**：
   - `ui/src/views/AiteamDashboardView.vue` — 总览页（4 卡：钱包/Guard/Judge/Revenue）
   - `ui/src/views/AiteamWalletView.vue` — 每 agent 余额排行 + 时间序列
   - `ui/src/views/AiteamJudgeView.vue` — 评分趋势 + 维度雷达图
   - `ui/src/views/AiteamPayrollView.vue` — 工资单时间线
   - `ui/src/views/AiteamGuardView.vue` — Panic 状态 + 手动解封
   - `ui/src/views/AiteamFXView.vue` — 汇率源 + override 编辑

2. **共享基础**：
   - `ui/src/composables/useCurrency.ts` — 全局货币切换 store
   - `ui/src/composables/useAuditTail.ts` — audit.log JSONL tail（依赖后端 tail endpoint）
   - 顶栏 💱 货币切换器

3. **后端 audit tail endpoint**：`/api/aiteam/audit?limit=200` 读
   `<dataDir>/aiteam/audit.log` 最后 N 行 JSON 解析返回