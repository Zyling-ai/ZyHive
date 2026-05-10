# B003 · 无界请求体 OOM DoS

> **严重度**: 🟠 HIGH（无认证场景下任何匿名 POST 都能触发，含 `/api/public_chat`、`/api/update/status` 等公共端点的某些方法变种）
> **状态**: ✅ 已修复 26.5.10v4
> **报告人**: ZyHive 团队主动审计

---

## 影响

Gin 的 `c.ShouldBindJSON(&v)` 内部对 `c.Request.Body` 调 `io.ReadAll` 直到 EOF，**没有任何 size cap**。攻击者只需 POST 几 GB 的 body：

- 服务端 `io.ReadAll` 在堆上为该 buffer 持续 `append + grow` → 进程内存爆掉 → OOM Killer
- 单个请求即可让整台服务下线
- **无需认证**：`/api/public_chat`、`/api/update/status` 等公共端点也走同一 gin engine

## 漏洞代码

`internal/api/router.go` 全局中间件链：
```go
r.Use(corsMiddleware())
r.Use(logging.TraceMiddleware())
r.Use(requestLogger())
// ⚠️ 此处缺一个 body size 限制中间件
v1 := r.Group("/api")
v1.Use(authMiddleware(...))
```

且 `c.ShouldBindJSON` 调用点：

```bash
$ rg "c.ShouldBindJSON" internal/api/ | wc -l
50+
```

50+ 端点全部受影响。已经显式用 `io.LimitReader` 的只有 2 处（files.go::Write 5 MiB、projects.go::Write 10 MiB），其他全部裸 `ShouldBindJSON`。

## PoC

```bash
# 任何已 deploy 的 ZyHive 实例 (假设 8080 端口公开)
yes "a" | head -c 5000000000 | curl -X POST -H "Content-Type: application/json" \
  --data-binary @- http://target:8080/api/public_chat
# 服务进程内存暴涨 -> OOM Kill -> 重启循环
```

## 修复

新增 `internal/api/bodylimit.go::bodyLimitMiddleware`：

```go
func bodyLimitMiddleware() gin.HandlerFunc {
    limit := bodyLimitFromEnv() // 默认 4 MiB, 可改 ZYHIVE_MAX_REQUEST_BODY_MB
    return func(c *gin.Context) {
        // 文件上传路由自带 io.LimitReader, 跳过包装
        if c.FullPath() == "/api/agents/:id/files/*path" ||
           c.FullPath() == "/api/projects/:id/files/*path" {
            c.Next()
            return
        }
        c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
        c.Next()
    }
}
```

加到 `router.go` 全局链尾部。

辅助 helper `IsBodyTooLarge(err)` 给 handler 区分"body 太大"vs"json 解析失败"，可选地返回 413 而非通用 400。

## 配置

| 环境变量 | 含义 | 默认 |
|---------|------|------|
| `ZYHIVE_MAX_REQUEST_BODY_MB` | 全局 body 上限 (MiB)，整数 | `4` |
| `ZYHIVE_MAX_REQUEST_BODY_MB=0` | 显式无限制（自托管 + 受信网络） | — |
| `ZYHIVE_MAX_REQUEST_BODY_MB=8` | 改成 8 MiB | — |

文件上传路由不受此限（per-chunk 5/10 MiB 已自带）。

## 测试

`internal/api/bodylimit_test.go` 4 用例：
- `TestBodyLimit_DefaultCapsAt4MiB`：5 MiB body → 413/400
- `TestBodyLimit_AllowsSmallBody`：13B body → 200
- `TestBodyLimit_ExemptFileUploadRoute`：upload 路由可读 5 MiB
- `TestIsBodyTooLarge_Detects`：helper 正确识别

`go test -race -count=1 ./internal/api/...` 全绿。

## 兼容性

- 50+ JSON 端点：之前能跑都低于 4 MiB（实际负载远小于此），不破坏
- 文件上传路由保留原有 5/10 MiB chunk 限
- 用户可通过环境变量调高或关闭

## 行为变更

- 攻击者发 > 4 MiB JSON body 现在会被 `http.MaxBytesReader` 在读取时截断 + 返回 `MaxBytesError`，handler 看到 `ShouldBindJSON` 报错并返回 400/413
- 正常用户无感知（4 MiB JSON 是已经很大的 payload）

## 升级建议

- ✅ 立即升级
- 自托管巨型上传：用 `ZYHIVE_MAX_REQUEST_BODY_MB=N` 调整或归零
