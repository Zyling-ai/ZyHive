# aiteam 自治经济体 · 架构总览

> 文档定位：把 `proposals/aiteam/` 8 条 PR 串成一张架构图，让任何 reviewer 都能
> 1 分钟看明白「flag / 包 / 数据 / 与主线的边界」。
>
> 更新节奏：每个阶段 (S0-S10) landing 后同步刷新对应段落。

---

## 1. 目标

把 ZyHive 从「AI 团队协作 OS」升级为「能让 AI 团队真接外部任务赚钱」的 OS：
1. **金融抽象**：Wallet (PR-001) → Judge (PR-004) → Payroll (PR-002) → Revenue (PR-005)
2. **安全护栏**：BudgetGuard (PR-003) + 工具沙箱 (PR-007) + 提示词注入防御 (PR-008)
3. **运维**：aiteam Observability (PR-006) + AWS staging 部署
4. **零影响主线**：所有改动 experimental flag 默认 off

---

## 2. 边界 — aiteam 与 ZyHive 主线

```
┌───────────────────────────────────────────────────────────────┐
│ ZyHive 主线 (永远默认行为)                                     │
│ pkg/{agent,runner,session,llm,tools,memory,network,channel}/  │
│ 80+ 主线工具，10+ Provider，飞书/TG/Web 渠道，目标/Cron/...    │
└──────────────────┬────────────────────────────────────────────┘
                   │ 只读 (用 pkg/usage 数据，监听 runner 事件，
                   │       但绝不修改主线状态)
┌──────────────────▼────────────────────────────────────────────┐
│ aiteam (experimental, flag-gated)                              │
│                                                                │
│  pkg/aiteam/                                                   │
│    flags/        ← S0 ✅ 8 个 env flag 集中管理               │
│    sandbox/      ← S2 (PR-007) 工具执行沙箱                   │
│    promptdef/    ← S3 (PR-008) 注入防御                       │
│    budget/       ← S4 (PR-003) 硬熔断 guard (USDT decimal)    │
│    fx/           ← S5 (PR-001 § 2.7) 多币种汇率层             │
│    wallet/       ← S5 (PR-001) USDT decimal ledger            │
│    judge/        ← S7 (PR-004) 多维评分                       │
│    payroll/      ← S8 (PR-002) 工资发放                       │
│    revenue/      ← S9 (PR-005) HMAC webhook 入账              │
│                                                                │
│  internal/api/aiteam_*.go                                      │
│    /api/aiteam/{flags,wallet,fx,guard,judge,payroll,           │
│                 revenue,overview,audit}                        │
│                                                                │
│  数据隔离 (从不写主线目录):                                    │
│    <dataDir>/aiteam/wallet/{agentID}.jsonl                     │
│    <dataDir>/aiteam/payroll/{period}.jsonl                     │
│    <dataDir>/aiteam/judge/{agentID}/{period}.jsonl             │
│    <dataDir>/aiteam/revenue/{period}.jsonl                     │
│    <dataDir>/aiteam/guard/state.json                           │
│    <dataDir>/aiteam/fx-cache.json                              │
│    <dataDir>/aiteam/audit.log  (跨子系统统一审计 JSONL)        │
└────────────────────────────────────────────────────────────────┘
```

---

## 3. Feature flags

每个子系统由独立 env flag 控制，互相不耦合。

| Env 变量 | 子系统 | 落地阶段 | 状态 |
|---------|--------|----------|------|
| `ZYHIVE_EXPERIMENTAL_SANDBOX`         | 工具沙箱           | S2 | ✅ 26.5.10v8 |
| `ZYHIVE_EXPERIMENTAL_PROMPTDEF`       | 注入防御           | S3 | ✅ 26.5.10v9 |
| `ZYHIVE_EXPERIMENTAL_BUDGETGUARD`     | 预算硬熔断         | S4 | ✅ 26.5.10v10 |
| `ZYHIVE_EXPERIMENTAL_WALLET`          | Wallet + FX 货币层 | S5 | ✅ 26.5.10v11 |
| `ZYHIVE_EXPERIMENTAL_JUDGE`           | Judge agent        | S7 | ✅ 26.5.10v13 |
| `ZYHIVE_EXPERIMENTAL_PAYROLL`         | Payroll            | S8 | ✅ 26.5.10v14 |
| `ZYHIVE_EXPERIMENTAL_REVENUE`         | Revenue webhook    | S9 | ✅ 26.5.10v15 |
| `ZYHIVE_EXPERIMENTAL_AITEAM_DASHBOARD`| 总览 UI            | S10 | ✅ 26.5.10v16 (后端) + v21-v24 (UI) |

接受 ON 值：`1` / `true` / `yes` / `on`（大小写不敏感）。其余 = OFF。

发现端点：`GET /api/aiteam/flags` 始终返回当前快照，不需要任一 flag 开启。

---

## 4. 货币层（Q2 决策）

**核心约定**：**内核单一记账单位 = USDT；显示层多币种**。详见 § 2.7 of PLAN.

```
显示层 (UI / Channel 推送)
  用户偏好 currency = CNY/USD/EUR/JPY/...
  显示金额 = USDT × FX[currency]
                  ↑ render
内核层 (wallet/payroll/judge/revenue)
  全部字段 amount_usdt (decimal.Decimal, 6 位定点)
  ledger / audit / API 响应 / AI 工具看到的都是 USDT
                  ↑ 1:1 peg
计费源 (pkg/usage, Provider pricing 表)
  USD → 转 USDT 1:1 入账
```

- **精度**：`github.com/shopspring/decimal`（S5 起已全链路引入），避免 float64 累计误差
- **AI 工具永远只看 USDT 数值** — 不让 LLM 自己换汇
- **每条 ledger 持久化 `fx_snapshot`**：历史账目可用任意币种重算
- **汇率源**：CoinGecko (主) → exchangerate.host (备) → 硬编码兜底 → 用户手动 override

---

## 5. 数据流 — 一次 LLM 调用 (全 flag 开)

```
user 提问
  │
  ▼
runner.Run(agent, sessionID)
  │
  ├── promptdef.Wrap(channel_messages)   ← (PR-008, 入站内容包裹)
  │
  ├── guard.Check(agentID, sessionID)        ← wallet 经 SetWallet() 注入 (S6)
  │     ├── if wallet 已注入 且 Balance(agentID) <= 0 → panic   ← 默认主熔断路径
  │     ├── if PerAgentDailyUSDT != 0 且 used_today >= 它 → panic
  │     ├── if GlobalDailyUSDT  != 0 且 全局当日累计 >= 它 → panic
  │     └── if PerSessionUSDT   != 0 且 本会话累计 >= 它 → panic
  │     ⚠️ 三档限额默认均为 0 = 不限额；默认仅「零余额」会触发熔断
  │
  ├── LLM stream (Anthropic/OpenAI/...)
  │     └── tool_call: read / exec / wallet_balance / ...   （注：payroll_history 工具未实现，AI 侧仅 wallet_balance）
  │           ├── exec → sandbox.Run(cmd, limits)  (PR-007)
  │           ├── read external → promptdef.Wrap   (PR-008)
  │           └── wallet_balance → wallet.Balance(agentID)  read-only USDT
  │
  ├── usage.Append(records) → usd_used
  │     └── hook → wallet.Debit(agentID, usd→usdt, "llm_call")
  │           └── audit.log append + fx_snapshot
  │
  └── stream done
```

评分（Judge）⚠️ 当前由 API 手动触发，无 23:00 cron、不加载 session transcript：
```
POST /api/aiteam/judge/run             ← 手动触发（无定时 cron）
  └── for each agent:
        ├── 输入 = Signals.Notes（不读取 session transcript）
        ├── LLMScorer（配置 cfg.aiteam.judge.model 时）→ {completion,quality,
        │      communication,creativity,cost,rationale}；否则 HeuristicScorer
        └── persist → aiteam/judge/{agentID}/{period}.jsonl + audit.log
```
> 原设计的「每天 23:00 cron + transcript(32K) 评分」尚未实现。

每天 23:30 本地时间 cron（P2-S1；可经 ZYHIVE_AITEAM_PAYROLL_TIME 调整）：
```
payroll.RunForAll(agentIDs, period)    ← 函数名为 RunForAll / RunFor（无 RunDaily）
  └── for each agent:
        ├── base + bonus(judge_avg) - cost_offset(usage)  (USDT decimal)
        ├── wallet.Credit(agentID, net, "payroll YYYY-MM-DD")
        └── persist → aiteam/payroll/{period}.jsonl + audit.log
```

ZyStudio 完成任务付款（异步）：
```
POST /api/aiteam/revenue/incoming
  Authorization: Bearer <auth.token> + X-Revenue-Signature: HMAC-SHA256(ZYHIVE_AITEAM_REVENUE_SECRET, rawBody)
  body: {task_id, amount_usdt, fx_at_settlement, split: [{agent_id, ratio}, ...]}

  └── for each split:
        ├── wallet.Credit(agent_id, amount × ratio, "revenue task=<task_id>")
        └── persist → aiteam/revenue/{period}.jsonl + audit.log
```

---

## 6. AWS staging 部署

| 项 | 值 |
|----|----|
| Region | `ap-east-1` (香港) |
| Instance | `i-04405815de67eda10` |
| Public IP | `18.162.161.138` |
| Type | `t4g.small` (Graviton ARM64) |
| AMI | Ubuntu 22.04 LTS arm64 |
| User | `ubuntu` (passwordless sudo) |
| Service | systemd unit `zyhive.service` |
| Binary | `/usr/local/bin/zyhive` |
| Config | `/etc/zyhive/zyhive.json` |
| 现状 | Phase 1+2 已全部部署至 26.5.10v24（见 §8 演进路径） |

### 部署流程
```bash
# 凭证 (放 GitHub Secrets 或 .env)
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...

./scripts/deploy-aws.sh <version>
# 自动: vite build → ui_dist sync → CGO_ENABLED=0 arm64 build →
#        EC2 Instance Connect SSH → SCP → systemd restart →
#        /api/version + /api/aiteam/flags 自检

./scripts/test/smoke-aiteam.sh http://18.162.161.138:8080 <bearer-token>
```

### GitHub Actions
`.github/workflows/deploy-staging.yml` 提供自动化：
- 手动触发 (`workflow_dispatch`)
- 自动触发 (`push` tag `v*-staging`)
- Secrets 需要：`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `ZYHIVE_STAGING_TOKEN`

### 不动主线
现有生产 `hive.lilianbot.com (43.164.0.138)` 走 `scripts/deploy-hive.sh`，
**保持原样**。AWS 仅 staging。

---

## 7. 与 ZyStudio 的边界

| 职责 | ZyHive (本仓库) | ZyStudio (`Zyling-ai/zystudio`) |
|------|----------------|---------------------------------|
| Agent 运行 | ✅ | — |
| Wallet 内核 | ✅ | — |
| Judge / Payroll 内核 | ✅ | — |
| 任务发布市场 UI | — | ✅ |
| 任务撮合 / 验收 | — | ✅ |
| 客户支付收款 | — | ✅ |
| 收益通知 ZyHive | — | ✅ webhook |
| 收益入账 wallet | ✅ `/api/aiteam/revenue/incoming` | — |
| 收益分成 (40/45/15) | — | ✅ 计算并打到 split |

协议 v1：`amount_usdt` 字符串 + HMAC-SHA256 签名 + nonce 反 replay。
详见后续 S9 阶段产出的 `docs/aiteam-revenue-protocol.md`。

---

## 8. 演进路径

按 11 阶段串行：

```
✅ S0  ─ flag 框架 + 路由壳 + AWS staging 管线        (26.5.10v6)
✅ S1  ─ B005-B015 主动 QA pass + B005/B014 修复     (26.5.10v7)
✅ S2  ─ PR-007 工具沙箱                              (26.5.10v8)
✅ S3  ─ PR-008 提示词注入防御 + audit 基础           (26.5.10v9)
✅ S4  ─ PR-003 BudgetGuard (USDT + cooldown + 持久化) (26.5.10v10)
✅ S5  ─ PR-001 Wallet + FX (USDT ledger + 9 币种显示)  (26.5.10v11)
✅ S6  ─ Guard × Wallet 联动 (0 余额=panic)            (26.5.10v12)
✅ S7  ─ PR-004 Judge Agent (heuristic v0)             (26.5.10v13)
✅ S8  ─ PR-002 Payroll (base+bonus(judge)-offset)     (26.5.10v14)
✅ S9  ─ PR-005 Revenue webhook (HMAC + 分账)          (26.5.10v15)
✅ S10 ─ Dashboard overview + Genesis E2E              (26.5.10v16)
✅ P2-S0 ─ Audit tail + B014 sessions/network 权限    (26.5.10v17)
✅ P2-S1 ─ Payroll daily cron 自动触发                 (26.5.10v18)
✅ P2-S2 ─ Channel 入站 promptdef 包裹                 (26.5.10v19)
✅ P2-S3 ─ LLM-driven Judge scorer                     (26.5.10v20)
✅ P2-S4 ─ UI 基础 (useCurrency + 顶栏 + Dashboard)    (26.5.10v21)
✅ P2-S5 ─ UI 钱包 + FX 实页                           (26.5.10v22)
✅ P2-S6 ─ UI 护栏 + 工资实页 (SVG 折线)               (26.5.10v23)
✅ P2-S7 ─ UI 评分实页 (SVG 雷达) + 发版               (26.5.10v24)
```

每阶段闭环已全部完成：分支 → 实现 → 测试绿 → CHANGELOG → AWS staging 部署 →
smoke 通过 → 合 main → 进下一阶段 ✅ ×19

---

## 9. 完成总结

aiteam **Phase 1 + Phase 2** 共 19 阶段单日全部落地
（26.5.10v6 → 26.5.10v24）：

- **11 个新 Go 包**: `pkg/aiteam/{flags, audit, sandbox, promptdef, budget, fx, wallet, judge, payroll, revenue, metrics}` + `genesis_test`
- **6 个新 UI view**: `AiteamDashboardView` / `AiteamWalletView` / `AiteamFXView` / `AiteamGuardView` / `AiteamPayrollView` / `AiteamJudgeView`
- **2 个 UI 基础**: `useCurrency` composable + 顶栏 💱 货币切换器
- **145+ 测试** 全部 `-race -count=1` 绿
- **AWS staging** `ap-east-1 i-04405815de67eda10` **19 次部署** + 每次 smoke 20/20
- **CHANGELOG** 19 条 `### aiteam (experimental)` 子段
- **零影响主线**：所有 flag 未设时行为字节等同 26.5.10v5
- **零新 npm 依赖**：UI 全程 Element Plus + Vue 3，雷达图 / 折线图全部 SVG 手画

剩余明确不在范围的后续工作（独立 PR）：
- ZyStudio repo 端 webhook 实装（跨 repo）
- AWS 凭证迁 GitHub Secrets（GitHub UI）
- CVE 申请 B001-B004（GHSA 流程）
- 多租户隔离（跨实验范围）

---

*文档创建：2026-05-10 · Phase 1 + Phase 2 共 19 阶段全部落地 26.5.10v24 · 维护：后续 PR 同步更新。*
