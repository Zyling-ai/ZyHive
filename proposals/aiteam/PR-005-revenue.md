# PR-005 · Revenue Webhook Engine

> 状态: ✅ landed S9 (26.5.10v15)
> 优先级: 🟠 P1
> 依赖: PR-001 Wallet (Credit 入账) ✅
> Flag: `ZYHIVE_EXPERIMENTAL_REVENUE=1` + 必需 env `ZYHIVE_AITEAM_REVENUE_SECRET`

完整协议规范见 [docs/aiteam-revenue-protocol.md](../../docs/aiteam-revenue-protocol.md)。

## 落地总结

入口 `POST /api/aiteam/revenue/incoming`：

* HMAC-SHA256 over raw body via `X-Revenue-Signature` 头
* `crypto/subtle.ConstantTimeCompare` 防时延侧信道（与 B002 同模式）
* 5 分钟 freshness window via `ts` field
* 10k-entry FIFO nonce 缓存防 replay
* `split[].ratio` decimal 字符串，sum=1.0 ± 0.0001 容差
* 每个 share 调 `wallet.Credit(agentID, amount × ratio, "revenue task=...")`
* 单 share 失败 → `ShareResult.CreditErr` 标记但不影响整体 200
* 旁路 audit log: `revenue.incoming` + 每 share `revenue.split`
* 持久化 `<dataDir>/aiteam/revenue/<period>.jsonl`
* 测试：`Test_AITeam_Revenue_*` 12 case 全 -race 绿

## 协议商定状态

* ZyHive 端 v1 协议已 freeze（见 docs/aiteam-revenue-protocol.md）
* ZyStudio 端实现 + 上线：留作独立 PR 在 `Zyling-ai/zystudio` repo