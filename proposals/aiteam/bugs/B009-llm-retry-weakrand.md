# B009 · LLM 重试抖动用 `math/rand` 而非 `crypto/rand`

> **严重度**: 🟢 LOW（性能/统计随机够用，不是安全敏感场景）
> **状态**: 📝 已分析（建议：不修）
> **报告人**: aiteam QA pass (S1, 26.5.10v6)
> **工具**: `gosec` G404

---

## 影响

`pkg/llm/httpclient.go::jitterDelay` 用 `math/rand.Int63n` 对 HTTP 重试 backoff 加 ±25% 抖动：

```go
return time.Duration(base - jitterRange + rand.Int63n(2*jitterRange))
```

gosec G404 标记 "weak random number generator"。

实际：HTTP retry jitter 是为了避免 thundering herd（多客户端同步重试打垮 server），统计意义上的伪随机已足够。攻击者预测重试时间也不会带来任何利益（最多能精确 timing 一次连接重试时刻，这不构成漏洞）。

## 漏洞代码

`pkg/llm/httpclient.go` line 166：

```go
import "math/rand"  // 不是 crypto/rand
return time.Duration(base - jitterRange + rand.Int63n(2*jitterRange))
```

## 修复（不需要）

如果一定要消 gosec 警告，可加 nolint 注释。功能性不需要切 `crypto/rand`（额外 4-5× 慢，每次重试都跑显然开销不值）。

## 兼容性

无变化。

## 修复优先级

🟢 **不做 fix，仅加注释**。
