# B015 · 多处 `err` 静默丢弃（G104 / G703 / G704）

> **严重度**: 🟢 LOW（可观测性问题，非安全漏洞）
> **状态**: 📝 已分析（建议：仅核心路径修）
> **报告人**: aiteam QA pass (S1, 26.5.10v6)
> **工具**: `gosec` G104

---

## 影响

仓库散落约 11 处显式 `_ = someCall()` 或 `cmd.Run()` 直接忽略错误。其中关键路径若静默失败可能掩盖问题：

- `pkg/session/*` 写 JSONL 失败若不报，message 实际丢
- `pkg/usage/*` 写计费失败若不报，钱对不上
- 重试 client 内部 `body.Close()` 失败 OK 忽略

## 漏洞代码

代表性：

```go
// pkg/session/store.go
_ = json.NewEncoder(f).Encode(msg)   // ⚠️ 写消息失败时静默
```

```go
// internal/api/chat.go
_ = c.AbortWithError(500, err)        // 用 _ 接住，但 AbortWithError 本身返回值不重要
```

## 修复策略

按"用户能感知 vs 不能感知"区分：

| 类别 | 处理 |
|------|------|
| 写持久化数据（sessions / usage / wallet ledger） | **必须** 至少 `slog.Error` 记录 |
| Close / Flush / 清理类 | 可以 `_ =`，但建议 `if err != nil { slog.Debug(...) }` |
| Cleanup-on-panic 路径 | 同上 |

## 测试用例

`pkg/session/store_test.go` 加 `TestEncode_ErrorPathLogged`（mock writer 返回 err，断言 log 输出）。

## 兼容性

无（只加日志）。

## 修复优先级

🟢 **S5 wallet 落地时同步把核心持久化路径检查一遍**（钱必须不能静默丢）。其他路径不动。
