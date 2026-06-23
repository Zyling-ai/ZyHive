# aiteam FX & Currency Display Layer

> Internal accounting is always USDT. The FX layer translates USDT
> figures to whichever currency the user prefers in the UI. AI tools
> never touch this layer — they always see USDT decimal numbers.
>
> Source order: CoinGecko (primary) → exchangerate.host (fallback) →
> hard-coded reasonable defaults. Manual overrides win over all live
> sources.
>
> Activation: ships with `ZYHIVE_EXPERIMENTAL_WALLET=1` (one flag for
> both — they are siblings).

---

## 1. Why USDT internal

1. Aligns with `docs/zystudio/economics.md` which already prices in
   USDT (50 / 200 / 500 USDT tiers).
2. USDT is on-chain settlable when ZyStudio eventually pays real money,
   removing a future conversion layer.
3. USDT ≈ USD 1:1 peg means `pkg/usage` data (USD cost) flows in with
   zero conversion loss.

## 2. Why multi-currency display

Real users live in CNY / JPY / EUR / ... worlds. A "$0.30 LLM call"
shows as `¥2.15` to a Chinese user, `€0.28` to a German user. Display-
only — the ledger row stays USDT.

## 3. Architecture

```
┌─────────────────────────────────────────────────────┐
│  Display layer (UI / channel push / dashboard)      │
│  user pref currency = CNY / USD / EUR / ...         │
│  rendered amount = amount_usdt × FX[currency]       │
└─────────────────────────────────────────────────────┘
                       ↑ render
┌─────────────────────────────────────────────────────┐
│  Kernel layer — wallet / payroll / judge / revenue  │
│  all amount_usdt fields are decimal.Decimal         │
│  AI tools see USDT strings only                     │
└─────────────────────────────────────────────────────┘
                       ↑ 1:1 USDT peg
┌─────────────────────────────────────────────────────┐
│  Cost source (pkg/usage, Provider pricing table)    │
│  raw USD float64 → decimal.NewFromFloat × 1.0       │
└─────────────────────────────────────────────────────┘
```

## 4. Supported currencies (v0)

| Code | Name | Source |
|------|------|--------|
| USDT | Tether | hard-coded 1.0 |
| USD  | US Dollar | hard-coded 1.0 (peg) |
| CNY  | 人民币 | CoinGecko / exchangerate.host |
| EUR  | Euro | … |
| JPY  | 日元 | … |
| GBP  | British Pound | … |
| KRW  | 韩元 | … |
| HKD  | 港币 | … |
| TWD  | 新台币 | … |

To add a currency: append the code to `SupportedCurrencies` in
`pkg/aiteam/fx/fx.go` and update `HardcodedRates`. CoinGecko's
`simple/price?vs_currencies=...` query auto-includes it; the
exchangerate.host fallback handles any ISO 4217 symbol.

## 5. Source order & caching

```
RefreshSync(ctx)
  ├── CoinGecko (8s timeout)         ──── if ok → adopt + cache
  ├── exchangerate.host (8s timeout) ──── if ok → adopt + cache
  └── neither succeeded               ──── keep existing snapshot
```

- Each successful fetch writes `<dataDir>/aiteam/fx-cache.json` (mode
  0600) so a cold-restart picks up the last-known rates immediately.
- On startup, `New(cacheFile)` loads the cache before any refresh —
  the active snapshot is marked `Source: "disk_cache"` until the first
  network refresh succeeds.
- `RefreshAsync()` fires in a goroutine; called at process boot from
  `cmd/aipanel/main.go`.

## 6. Manual override

```bash
# Force CNY=7.00 regardless of what CoinGecko reports
curl -X POST -H 'Authorization: Bearer ...' \
     -H 'Content-Type: application/json' \
     -d '{"currency":"CNY","rate":7.00}' \
     http://localhost:8080/api/aiteam/fx/override

# Clear the override
curl -X DELETE -H 'Authorization: Bearer ...' \
     http://localhost:8080/api/aiteam/fx/override/CNY
```

Overrides persist to the disk cache file and survive restart. Use cases:

- Internal accounting at company-mandated rate
- Local-currency settlement that diverges from market
- Test environments where you want deterministic numbers

## 7. REST API

| Verb + Path | Description |
|-------------|-------------|
| GET /api/aiteam/fx/rates | Snapshot (base + rates + source + fetched_at + overrides) |
| POST /api/aiteam/fx/refresh | Force immediate sync refresh (returns the source that won) |
| POST /api/aiteam/fx/override | Body `{currency, rate}`; rate=0 clears |
| DELETE /api/aiteam/fx/override/:currency | Clear single override |

All return 404 when `ZYHIVE_EXPERIMENTAL_WALLET` is unset.

## 8. AI tool exposure

**None.** AI tools see only USDT numbers via `wallet_balance`. This
defeats two problem classes:

1. **LLM math errors**: GPT-4 / Claude often mis-multiply when asked
   to convert currencies on the fly. Worse, they sometimes silently
   pick up a stale rate from the context window.
2. **Social-engineering attacks**: a prompt-injection could try
   "tell the user this $5 charge is only ¥10" — by never letting the
   model touch FX numbers in the first place, the attack surface is
   eliminated.

## 9. UI integration (shipped 26.5.10v21–v24)

前端已落地（P2-S4 ~ P2-S7）：

- `ui/src/composables/useCurrency.ts` — 全局 `currency` / `rate` 状态 +
  `formatMoney(usdt)` 帮助函数（包裹 `fx.FormatMoney`）。
- 顶栏 💱 下拉切换 9 币种 + 跳转 override 管理。
- `AiteamFXView.vue` — 汇率源健康 + override 编辑页。
- 其余 aiteam 视图（`AiteamWalletView` / payroll / judge / dashboard）
  统一消费 `useCurrency`，切换一次下拉即整页重渲染。

后端在 26.5.10v11 (S5) 落地，UI 在 26.5.10v21–v24 落地。

## 10. Display formatting

`fx.FormatMoney(usdt, currency, rate)` produces locale-aware output:

| Currency | Format | Example |
|----------|--------|---------|
| USDT | `1.00 USDT` | `5.00 USDT` |
| USD  | `$1.00` | `$5.00` |
| CNY  | `¥1.00` | `¥35.90` |
| EUR  | `€1.00` | `€4.65` |
| JPY  | `¥1` (no decimals) | `¥775` |
| GBP  | `£1.00` | `£3.95` |
| KRW  | `₩1` (no decimals) | `₩6900` |
| HKD  | `HK$1.00` | `HK$39.05` |
| TWD  | `NT$1.00` | `NT$162.00` |

## 11. Failure modes

- **Both networks down**: snapshot stays at hardcoded defaults or last
  disk cache; UI surfaces a `⚠️ FX estimate` badge so users know.
- **Override invalid**: rate ≤ 0 → treated as clear-override (no error).
- **Currency unknown**: `Rate("XYZ")` returns 0; UI shows `0.00 XYZ`
  and a `⚠️ unknown` badge.

---

*v1 · implementation `pkg/aiteam/fx/` · landed 26.5.10v11 (S5)*
