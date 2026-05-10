# B011 · CLI 状态显示文案被误标 "Hardcoded credentials"

> **严重度**: 🟢 LOW（false positive）
> **状态**: 📝 已分析（建议：不修）
> **报告人**: aiteam QA pass (S1, 26.5.10v6)
> **工具**: `gosec` G101

---

## 影响

gosec G101 报告 `cmd/aipanel/cli.go:104, 171` 两处 "Potential hardcoded credentials"。
代码实际是：

```go
token := "(未配置)"   // line 104 — 占位文案，UI 显示用
...
fmt.Printf("Token: %s\n", token)  // line 171
```

不是 credential 字面量，只是中文 placeholder 字符串。

## 漏洞代码

```go
token := "(未配置)"
```

## PoC

无。

## 修复（不需要）

无。

## 兼容性

无。

## 修复优先级

🟢 **不做 fix**。本 markdown 留作 audit 复查记录。
