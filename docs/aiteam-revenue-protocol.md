# aiteam Revenue Webhook Protocol v1

> 与 ZyStudio（或任何兼容任务市场）的入账 webhook 协议。
>
> Endpoint: `POST /api/aiteam/revenue/incoming`
> Activation: `ZYHIVE_EXPERIMENTAL_REVENUE=1`
> Shared secret: `ZYHIVE_AITEAM_REVENUE_SECRET=<random 32+ bytes>`

---

## 1. 认证

每个请求必须携带两层认证：

1. **Bearer token**（仓库主线 auth）：`Authorization: Bearer <ZyHive auth.token>`
2. **HMAC-SHA256 签名**：`X-Revenue-Signature: <hex-digest>`
   - 计算方法：`hmac_sha256(secret, raw_request_body)`
   - secret 是 ZyHive 与市场预先约定的共享密钥（≥32 字节）
   - 必须用 **请求体的原始字节** 计算（避免 JSON 重新序列化漂移）

## 2. 请求体格式

```json
{
  "task_id": "studio-task-001",
  "studio_id": "studio-foo",
  "amount_usdt": "50.000000",
  "fx_at_settlement": {
    "USD": 1.0,
    "CNY": 7.18,
    "EUR": 0.93
  },
  "split": [
    {"agent_id": "alice", "ratio": "0.6"},
    {"agent_id": "bob",   "ratio": "0.4"}
  ],
  "ts": 1736000000,
  "nonce": "f7a3c1d8-9b2e-4f0e-a1b0-3a8c1d8b9b2e"
}
```

| 字段 | 说明 |
|------|------|
| `task_id` | 市场侧的任务唯一 ID（用于审计追溯） |
| `studio_id` | （可选）市场身份标识 |
| `amount_usdt` | 入账金额（USDT decimal 字符串，避免 float 精度损失） |
| `fx_at_settlement` | （可选）结算时刻的 FX 快照，用于事后多币种对账 |
| `split[].agent_id` | 受益 agent ID（必须在 ZyHive 已注册） |
| `split[].ratio` | 分账比例（decimal 字符串），所有 ratio 之和必须 = 1.0（容差 ±0.0001） |
| `ts` | unix 秒时间戳（结合 nonce 防重放） |
| `nonce` | 唯一标识（UUID/随机 32 字符），FIFO 缓存最近 10k 个 |

## 3. 验证流程

服务端按顺序执行：

1. **HMAC 验证**：用 secret 重算签名，`crypto/subtle.ConstantTimeCompare` 比对
   - 失败 → `401 Unauthorized`, `error: revenue: bad signature`
2. **freshness 检查**：`|now - ts| ≤ 5min`
   - 失败 → `410 Gone`, `error: revenue: stale timestamp`
3. **nonce 去重**：内存 FIFO 缓存（10k 容量）
   - 命中 → `409 Conflict`, `error: revenue: nonce already seen`
4. **金额校验**：`amount_usdt` 必须 > 0
   - 失败 → `400 Bad Request`
5. **split 校验**：所有 ratio ≥ 0 且 sum ≈ 1.0
   - 失败 → `400 Bad Request`, `error: revenue: split ratios do not sum to 1.0`
6. **分账入账**：每个 share 调 `wallet.Credit(agent_id, amount × ratio, "revenue task=...")`
   - 单 share 失败 **不影响** 整体 accept；返回中标 `credit_err`
7. **审计 + 持久化**：
   - 旁路 `<dataDir>/aiteam/audit.log`（`revenue.incoming` + 每 share 一条 `revenue.split`）
   - 写日级 ledger `<dataDir>/aiteam/revenue/YYYY-MM-DD.jsonl`

## 4. 响应

成功（200）：

```json
{
  "Accepted": true,
  "TaskID": "studio-task-001",
  "AmountUSDT": "50",
  "Shares": [
    {"agent_id": "alice", "share_usdt": "30"},
    {"agent_id": "bob",   "share_usdt": "20"}
  ]
}
```

失败：HTTP 状态码按错误类型设置（见 § 3），body 含 `error` 字段。

## 5. 重试 / Idempotency

* **Nonce 是 idempotency key**：同 nonce 重传 → `409 Conflict`，但 ledger 不重复扣
* 网络失败时，市场侧应 **保留同 nonce 重试**，幂等保证
* 一旦超过 freshness 窗口（5min），市场必须用新 ts + 新 nonce 重新签名

## 6. 测试样例

```bash
SECRET="$ZYHIVE_AITEAM_REVENUE_SECRET"   # 部署时经环境变量注入（见 aiteam-deploy-aws.md §7）
BODY='{"task_id":"t1","amount_usdt":"50","split":[{"agent_id":"alice","ratio":"1.0"}],"ts":'$(date +%s)',"nonce":"'$(uuidgen)'"}'
SIG=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print $2}')

curl -X POST http://18.162.161.138:8080/api/aiteam/revenue/incoming \
     -H "Authorization: Bearer $ZYHIVE_TOKEN" \
     -H "X-Revenue-Signature: $SIG" \
     -H "Content-Type: application/json" \
     -d "$BODY"
```

## 7. 兼容性

* v1 字段固定；未来扩展通过 `extensions` map 添加
* `fx_at_settlement` 可选；服务端只透传到 audit / ledger，不影响 split 数学
* 客户端可发送额外字段；服务端 JSON 解析忽略未知 key

## 8. 错误码总览

| HTTP | 错误 | 触发 |
|------|------|------|
| 401 | bad signature | HMAC 不匹配 |
| 401 | missing X-Revenue-Signature | 头部缺失 |
| 410 | stale timestamp | ts 超出 ±5min |
| 409 | nonce already seen | nonce 已见过 |
| 400 | invalid amount_usdt | amount 解析失败 / ≤ 0 |
| 400 | invalid ratio | 任一 ratio 解析失败 / 负数 |
| 400 | split ratios do not sum to 1.0 | sum 偏离超过 0.0001 |
| 400 | missing nonce | nonce 字段为空 |
| 503 | revenue not initialised | flag on 但 secret 未配置 |
| 404 | not enabled | flag off |

---

*v1 protocol · 实现见 `pkg/aiteam/revenue/` · 落地 26.5.10v15 (S9)*
