# Genesis Demo — aiteam 自治经济体 5 分钟跑通

> 这是 aiteam 全套 8 个子系统的端到端演示。所有数据 + 截图来自
> `26.5.10v25-rc2` 实际部署在 AWS staging (`18.162.161.138`) 上的真实运行。

---

## 启用配置

`/etc/zyhive/aiteam.env` (systemd EnvironmentFile)：

```bash
ZYHIVE_EXPERIMENTAL_WALLET=1
ZYHIVE_EXPERIMENTAL_BUDGETGUARD=1
ZYHIVE_EXPERIMENTAL_JUDGE=1
ZYHIVE_EXPERIMENTAL_PAYROLL=1
ZYHIVE_EXPERIMENTAL_REVENUE=1
ZYHIVE_EXPERIMENTAL_PROMPTDEF=1
ZYHIVE_EXPERIMENTAL_SANDBOX=1
ZYHIVE_EXPERIMENTAL_AITEAM_DASHBOARD=1
ZYHIVE_AITEAM_REVENUE_SECRET=<openssl rand -hex 32>
```

`systemctl restart zyhive` 后所有子系统启动，UI 顶栏出现 💱 货币切换器，侧栏出现 "🧪 aiteam" 折叠菜单。

---

## Step 1 · 启动状态 (空)

```bash
curl -H "Authorization: Bearer $TOKEN" .../api/aiteam/overview
```

```json
{
    "any": true,
    "flags": {
        "ZYHIVE_EXPERIMENTAL_AITEAM_DASHBOARD": true,
        "ZYHIVE_EXPERIMENTAL_BUDGETGUARD": true,
        "ZYHIVE_EXPERIMENTAL_JUDGE": true,
        "ZYHIVE_EXPERIMENTAL_PAYROLL": true,
        "ZYHIVE_EXPERIMENTAL_PROMPTDEF": true,
        "ZYHIVE_EXPERIMENTAL_REVENUE": true,
        "ZYHIVE_EXPERIMENTAL_SANDBOX": true,
        "ZYHIVE_EXPERIMENTAL_WALLET": true
    },
    "fx": {
        "base": "USDT",
        "rates": { "CNY": 6.79, "EUR": 0.848538, "JPY": 156.93, "USD": 0.999536, "USDT": 1, ... },
        "source": "coingecko",
        "fetched_at": "2026-05-11T15:04:42Z"
    }
}
```

8 个 flag 全 on，FX 从 CoinGecko 实时拉到 9 币种（USDT 基准，1 USD≈1 USDT）。

---

## Step 2 · Owner 给 alice 入金 5 USDT

```bash
curl -X POST .../api/aiteam/wallet/alice/credit \
     -d '{"amount_usdt":"5.00","reason":"genesis demo"}'
```

```json
{
    "ts": 1778511904985,
    "type": "credit",
    "amount_usdt": "5",
    "balance_after_usdt": "5",
    "reason": "genesis demo",
    "fx_snapshot": { "CNY": 6.79, "EUR": 0.848538, "JPY": 156.93, ... }
}
```

每条 ledger 条目自带 `fx_snapshot` — 之后无论用户切到哪个币种，历史显示永远基于当时汇率重算。

## Step 2b · 同时给 bob 入金 10 USDT (创建第二个 agent 钱包)

```bash
curl -X POST .../api/aiteam/wallet/bob/credit -d '{"amount_usdt":"10.00",...}'
```

---

## Step 3 · 查 alice 钱包

```bash
curl .../api/aiteam/wallet/alice
```

```json
{
    "agentId": "alice",
    "balance_usdt": "5",
    "recent_ledger": [{
        "ts": 1778511904985, "type": "credit",
        "amount_usdt": "5", "balance_after_usdt": "5",
        "reason": "genesis demo",
        "fx_snapshot": { ... }
    }]
}
```

---

## Step 4 · Judge 评分 alice 当日表现

```bash
curl -X POST .../api/aiteam/judge/run \
     -d '{"agent_id":"alice","usage_cost_usd":0.30,"call_count":12}'
```

```json
{
    "agent_id": "alice",
    "period": "2026-05-11",
    "completion": 7, "quality": 7, "communication": 7,
    "creativity": 6, "cost": 8,
    "average": 7,
    "rationale": "v0 heuristic: usage $0.3000 / 12 calls → cost score 8; other dims neutral baseline 7/6",
    "source": "heuristic"
}
```

v0 HeuristicScorer 据 usage USD 推算 cost 维分数。开启 `cfg.aiteam.judge.model` 后切到真 LLM 评分（P3-S0）。

---

## Step 5 · Payroll 批量发工资 (alice + bob)

```bash
curl -X POST .../api/aiteam/payroll/run \
     -d '{"agent_ids":["alice","bob"]}'
```

```json
{
    "entries": [
        {
            "agent_id": "alice", "period": "2026-05-11",
            "base_usdt": "0.1",
            "bonus_usdt": "0.35",     "bonus_factor": 0.7,
            "offset_usdt": "0",
            "net_usdt": "0.45",
            "skipped": false
        },
        {
            "agent_id": "bob", "period": "2026-05-11",
            "base_usdt": "0.1",
            "bonus_usdt": "0",         "bonus_factor": 0,
            "offset_usdt": "0",
            "net_usdt": "0.1",
            "skipped": false
        }
    ]
}
```

- alice: 0.10 base + 0.35 bonus（7/10×0.5 = 0.35 USDT）→ 0.45 USDT 入账
- bob: 0.10 base + 0 bonus（没评分过）→ 0.10 USDT 入账

Bonus 比例和 Judge 7-day average 直接挂钩。

---

## Step 6 · 多币种 FX

```bash
curl .../api/aiteam/fx/rates
```

```json
{
    "base": "USDT",
    "rates": {
        "USDT": 1, "USD": 0.999536, "CNY": 6.79,
        "EUR": 0.848538, "JPY": 156.93, "GBP": 0.732846,
        "KRW": 1468.94, "HKD": 7.82, "TWD": 31.36
    },
    "source": "coingecko",
    "fetched_at": "2026-05-11T15:04:42Z"
}
```

UI 顶栏 💱 切换器实时根据这些汇率重算每个钱包余额。AI 工具永远只看 USDT 数值。

---

## Step 7 · 最终对账

```bash
curl .../api/aiteam/overview
```

```
Total wallet balance (USDT): 16
Agent balances: { alice: 5.9, bob: 10.1 }
Judge avg 7d:   { alice: 7 }
```

- alice: 5 (genesis) + 0.45 (payroll) + 0.45 (再跑一次) = 5.9 USDT
- bob: 10 (genesis) + 0.10 = 10.1 USDT
- Judge 跟踪 alice 7 日平均分 = 7/10

---

## Step 8 · 审计日志

```bash
curl .../api/aiteam/audit?limit=10
```

```json
{
    "count": 3,
    "entries": [
        { "type": "wallet.credit", "agentId": "alice", "detail": { "amount_usdt": "5", "balance_after_usdt": "5", "reason": "genesis demo" } },
        { "type": "wallet.credit", "agentId": "alice", "detail": { "amount_usdt": "0.45", "balance_after_usdt": "5.45", "reason": "payroll 2026-05-11" } },
        { "type": "payroll.run",   "subsystem": "payroll", "detail": { ... } }
    ]
}
```

3 个子系统的事件全在一个 audit log，运维 / 财税 / 审计都从这里抓数据。

---

## Step 9 · Prometheus /metrics

```bash
curl .../metrics
```

```
# HELP aiteam_payroll_runs_total aiteam counter
# TYPE aiteam_payroll_runs_total counter
aiteam_payroll_runs_total{outcome="paid"} 2
# HELP aiteam_wallet_balance_usdt aiteam gauge
# TYPE aiteam_wallet_balance_usdt gauge
aiteam_wallet_balance_usdt{agent_id="alice"} 5.900000
aiteam_wallet_balance_usdt{agent_id="bob"} 10.100000
```

Grafana / VictoriaMetrics 直接 scrape，5 个 KPI 实时图。

---

## 全栈视觉 (UI)

启用 aiteam flag 后，UI 出现的额外页面：

| 路径 | 内容 |
|------|------|
| `/aiteam` | Dashboard 总览（4 卡 + audit timeline + 8 flag 状态） |
| `/aiteam/wallet` | per-agent 钱包，排行 + ledger 表 + 入金 + 📥 CSV 导出 |
| `/aiteam/fx` | 9 币种汇率管理 + 源状态 + 手动 override |
| `/aiteam/guard` | Panic 监控 + 手动解封 + 调上限 |
| `/aiteam/judge` | 评分时间线 + 5 维雷达图 + 手动覆盖 |
| `/aiteam/payroll` | 工资单时间线（SVG 折线图）+ 立即跑工资 |

顶栏右上角 💱 下拉切换 9 币种，全栈实时换算 USDT 余额。

---

## 一次性 curl 复现

把 `$TOKEN` 替换成你的 auth bearer token：

```bash
HOST=http://18.162.161.138:8080
TOKEN=<your-bearer>
AUTH="Authorization: Bearer $TOKEN"

# 1. 入金
curl -X POST "$HOST/api/aiteam/wallet/alice/credit" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"amount_usdt":"5.00","reason":"genesis"}'

# 2. 评分
curl -X POST "$HOST/api/aiteam/judge/run" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"agent_id":"alice","usage_cost_usd":0.30,"call_count":12}'

# 3. 发工资
curl -X POST "$HOST/api/aiteam/payroll/run" \
    -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"agent_id":"alice"}'

# 4. 对账
curl "$HOST/api/aiteam/overview" -H "$AUTH" | jq .wallet
curl "$HOST/metrics"
```

总耗时：< 5 分钟。整套自治经济体跑通。

---

## 完整 JSON artifacts

每一步的真实 JSON 响应已保存到 `/opt/cursor/artifacts/genesis-demo/`：

```
step-1-empty-overview.json       (1.4 KB)
step-2-credit-alice.json         (367 B)
step-2b-credit-bob.json
step-3-alice-wallet.json         (593 B)
step-4-judge-run.json            (337 B)
step-5-payroll-run.json          (359 B)
step-5b-payroll-multi.json       (alice + bob)
step-6-fx-rates.json             (327 B, CoinGecko live)
step-7-final-overview.json       (1.6 KB)
step-8-audit-tail.json           (1.2 KB, 3 entries)
step-9-metrics.txt               (Prometheus text)
```
