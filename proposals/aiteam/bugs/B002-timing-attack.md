# B002 · Bearer Token / Download Token Timing 侧信道

> **严重度**: 🟠 HIGH（远程可触发，但需要网络抖动控制）
> **状态**: ✅ 已修复 26.5.10v3
> **报告人**: ZyHive 团队主动审计（B001 修复后例行审视）

---

## 影响

`Authorization: Bearer <token>` 的字符串比较使用 Go 的 `==`/`!=`，这是**短路比较**：第一个字节不同就返回，第二个字节才不同则多跑一轮 CPU 指令。

攻击者能在网络层（甚至本地局域网）对响应时延做统计回归（每个 token 候选发 N 次取均值），从首字节开始猜，逐字节恢复完整 token。

经典例子：[Coda Hale 2009 timing attack on string comparison](https://codahale.com/a-lesson-in-timing-attacks/)。

## 漏洞代码

3 处：

### 1. `internal/api/router.go::authMiddleware`
```go
if auth != "Bearer "+token {
    c.AbortWithStatusJSON(http.StatusUnauthorized, ...)
}
```
影响所有 `/api/*` 端点。

### 2. `internal/api/files.go::downloadHandler.ServeFile`
```go
if h.authToken != "" && token != h.authToken {
    c.JSON(http.StatusUnauthorized, ...)
}
```
影响 `/api/download?token=`。

### 3. `internal/api/media.go::mediaHandler.ServeMedia`
```go
if auth != "Bearer "+h.token && qToken != h.token {
    c.JSON(http.StatusUnauthorized, ...)
}
```
影响 `/api/media?token=`。

## PoC（理论）

```python
import time, requests

URL = "http://target:8080/api/version"

def measure(token, n=50):
    t0 = time.perf_counter()
    for _ in range(n):
        requests.get(URL, headers={"Authorization": f"Bearer {token}"})
    return (time.perf_counter() - t0) / n

# 已知前缀 "abc"，猜下一字节
for c in "0123456789abcdef":
    avg = measure("abc" + c + "x" * 28)
    print(c, avg)  # 真正的下一字节响应时延略长
```

实际可行性受网络抖动制约。LAN 环境与暴露给互联网的 admin 面板均 > 0 风险。

## 修复

新建 `internal/api/authcompare.go::secretsEqual(a, b string) bool`，包装 `crypto/subtle.ConstantTimeCompare`：

```go
func secretsEqual(a, b string) bool {
    return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
```

3 个调用点都改用 `secretsEqual`，代码无功能变更，仅消除时延差。

## 测试

`internal/api/authcompare_test.go` 覆盖：

| Test | 验证 |
|------|------|
| `TestSecretsEqual_BasicMatch` | 相同串返 true |
| `TestSecretsEqual_BasicMismatch` | 不同串返 false |
| `TestSecretsEqual_LengthMismatchSafe` | 不同长度无 panic 返 false |
| `TestSecretsEqual_EmptyAcceptableButNotMatchingNonEmpty` | 边界 case |
| `TestAuthMiddleware_WrongTokenRejected` (6 子 case) | 端到端 401 |
| `TestAuthMiddleware_NoTokenAllowsAll` | 空 token 配置→ dev mode 通行 |
| `TestDownloadHandler_WrongTokenRejected` | download 401 |
| `TestMediaHandler_WrongTokenRejected` (3 子 case) | media 401 (header / query / 无) |

合计 18 个用例（含子测），全绿。`go test -race` 全包绿。

## 兼容性

- 零 API 变更（HTTP status / body 一致）
- 零行为变更（只是消除时延差）
- 客户端无感知

## 升级建议

- ✅ 立即升级
- ⚠️ 已暴露 admin 面板的实例：升级后**轮换 token**（`zyhive token --rotate`），假定旧 token 可能已被探测
