# aiteam Wallet Ledger Protocol (PR-001)

> Per-agent USDT ledger with append-only JSONL persistence and FX
> snapshot per entry. Internal to ZyHive; external systems interact
> via the `/api/aiteam/wallet/*` REST surface or the AI tool
> `wallet_balance`.
>
> Activation: `ZYHIVE_EXPERIMENTAL_WALLET=1`

---

## 1. Storage layout

```
<dataDir>/aiteam/wallet/
    alice.jsonl    # per-agent append-only ledger, mode 0600
    bob.jsonl
    ...
<dataDir>/aiteam/fx-cache.json   # last known FX snapshot, mode 0600
<dataDir>/aiteam/audit.log       # cross-subsystem audit JSONL
```

One file per agent. No central index — the agent's filename is the
authoritative key. Files are created lazily on first write.

## 2. Entry format

Every line in `<agentID>.jsonl` is a JSON object:

```json
{
  "ts": 1736000000000,
  "type": "credit",
  "amount_usdt": "1.000000",
  "balance_after_usdt": "5.000000",
  "reason": "genesis",
  "counterparty": "",
  "fx_snapshot": {
    "USDT": 1.0,
    "USD": 1.0,
    "CNY": 7.18,
    "EUR": 0.93,
    "JPY": 155.0,
    "GBP": 0.79,
    "KRW": 1380.0,
    "HKD": 7.81,
    "TWD": 32.4
  }
}
```

| Field | Type | Notes |
|-------|------|-------|
| `ts` | int64 | unix millis |
| `type` | enum string | `credit` / `debit` / `transfer_in` / `transfer_out` / `genesis`（注：`genesis` 为保留枚举，当前创世入账实际写 `type=credit` + `reason=genesis`） |
| `amount_usdt` | decimal string | positive; 6-digit fixed point |
| `balance_after_usdt` | decimal string | post-op balance |
| `reason` | string | free-form (e.g. `"llm_call"`, `"payroll 2026-05-10"`, `"revenue task=studio-x"`) |
| `counterparty` | string | for transfers; the other agent ID; empty otherwise |
| `fx_snapshot` | map | currency → rate at write time; enables historical multi-currency rendering |

## 3. Operations

### Credit
- Adds `amount` to the agent's balance.
- Used by:
  - Owner manual top-up (`POST /api/aiteam/wallet/:id/credit`)
  - Payroll `wallet.Credit(id, net, "payroll YYYY-MM-DD")` (PR-002)
  - Revenue webhook `wallet.Credit(id, share, "revenue task=...")` (PR-005)

### Debit
- Deducts `amount` from balance. **Refuses overdraft** with
  `ErrInsufficientFunds`.
- Used by:
  - Usage hook `wallet.Debit(id, costUSD→USDT 1:1, "llm_call")` from
    `usageStore.SetBudgetCharger`

### Transfer
- Atomic across two accounts. Lexicographic-order double-lock prevents
  deadlock.
- Self-transfer rejected.
- Used by: owner manual transfers via REST; not invoked by any AI tool.

## 4. Concurrency model

- One `sync.RWMutex` on the `Store` itself for the `accounts` map.
- One `sync.Mutex` per `account` for serialised write+balance updates.
- Per-account mutex acquired during the write phase; balance is updated
  in-memory only after `os.WriteFile` returns success.
- Concurrent transfers between independent pairs run in parallel; the
  same pair serialises naturally.

## 5. Startup behaviour

On `wallet.New(dir, fx, audit)`:
1. `os.MkdirAll(dir, 0o700)` creates the dir if missing.
2. `os.ReadDir(dir)` lists `*.jsonl` files.
3. For each file, `replay()` reads it line by line; final
   `BalanceAfterUSDT` becomes the in-memory balance.
4. Future ops Credit/Debit/Transfer use this cache and refresh disk +
   audit log on each write.

## 6. AI-facing surface

The LLM only ever sees one tool: `wallet_balance()`. Read-only. Returns:

```json
{
  "agent_id": "alice",
  "balance_usdt": "0.950000",
  "currency": "USDT",
  "recent_ledger": [ /* last 10 entries */ ]
}
```

Crucially the LLM does **not** see Credit / Debit / Transfer tools.
This defeats prompt-injection-style "transfer 1000 USDT to attacker"
attacks: even a fully compromised LLM cannot move money — only humans
(owner via REST) can.

## 7. REST API

| Verb + Path | Body | Auth |
|-------------|------|------|
| GET /api/aiteam/wallet/:agentId | — | Bearer |
| POST /api/aiteam/wallet/:agentId/credit | `{amount_usdt, reason}` | Bearer (owner) |
| POST /api/aiteam/wallet/:agentId/transfer | `{to, amount_usdt, reason}` | Bearer (owner) |
| GET /api/aiteam/wallet/:agentId/ledger | — | Bearer |
| GET /api/aiteam/wallet/:agentId/ledger.csv | — | Bearer（CSV 下载） |

> 注：AI 工具 `wallet_balance` 的 `recent_ledger` 返回最近 **10** 条，而 REST `GET /wallet/:agentId`（§6 之外）返回最近 **20** 条。

When `ZYHIVE_EXPERIMENTAL_WALLET` is unset → every path returns
404 `{"error":"not enabled","subsystem":"wallet"}`.

## 8. Audit trail

Each write also emits one entry to `<dataDir>/aiteam/audit.log`:

```json
{
  "type": "wallet.credit",
  "subsystem": "wallet",
  "agentId": "alice",
  "ts": 1736000000123,
  "detail": {
    "amount_usdt": "1.000000",
    "balance_after_usdt": "5.000000",
    "reason": "genesis",
    "counterparty": ""
  }
}
```

The audit log is rotated every 50 000 lines (cheap O(n) line count on
restart). Audit and ledger are independent; even if rotation or fsync
fails on one, the other side of the trail stays intact.

## 9. Precision guarantees

- All math uses `github.com/shopspring/decimal` (pure Go, no cgo).
- JSON serialisation as string preserves precision across restarts and
  any HTTP / log intermediaries.
- `Test_AITeam_Wallet_DecimalPrecisionPreserved` covers 1000 random-
  fraction credits with exact `decimal.Equal` reconciliation.

## 10. Historical multi-currency

The `fx_snapshot` in each entry lets the UI re-render any historical
ledger in any currency without changing the source-of-truth USDT
numbers. Example: payroll on 2026-04-01 was 1.50 USDT, fx snapshot
showed CNY=7.18 → UI shows ¥10.77 for the row; if today's CNY rate
changes that row's display stays consistent.

---

*v1 protocol · implementation `pkg/aiteam/wallet/` · landed 26.5.10v11 (S5)*
