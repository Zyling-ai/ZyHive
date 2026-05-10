# B010 · Provider 启动自检 SSRF taint

> **严重度**: 🟢 LOW（baseURL 来自管理员配置，非用户输入）
> **状态**: 📝 已分析（建议：不修）
> **报告人**: aiteam QA pass (S1, 26.5.10v6)
> **工具**: `gosec` G704

---

## 影响

`cmd/aipanel/main.go::startupTestAnthropic` / `startupTestOpenAICompat` 用配置文件 `aipanel.json` 中的 `baseURL` 字段构造 HTTP 请求：

```go
req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/messages", ...)
```

gosec G704 标记 "SSRF via taint analysis"。

实际：
- `baseURL` 来自 `zyhive.json` Provider 配置，**只能由 root / admin 修改**（文件权限 0600）
- 启动时一次性自检，不是网关式代理
- 即使有人写了恶意 `baseURL`，他们已经能直接修改任何代码

非 SSRF：SSRF 的本质是"低权限用户能控制后端服务发起的 HTTP 请求目标"，而这里目标完全由 admin 直接指定。

## 漏洞代码

`cmd/aipanel/main.go` lines 639, 643, 661, 664：四处 `http.NewRequestWithContext(ctx, ..., baseURL+"/x", ...)`。

## PoC

无（需要 root 权限改配置）。

## 修复（不需要）

无。可选：加 nolint 注释。

## 兼容性

无变化。

## 修复优先级

🟢 **不做 fix**。
