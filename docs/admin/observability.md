# 可观测性

`26.8.2v1` 提供进程健康、就绪状态、鉴权状态接口和服务日志。它没有稳定核心的完整 Prometheus 指标、分布式追踪后端或内建告警系统。

## 健康端点

### `GET /healthz`

无需鉴权，进程存活时返回 HTTP 200。主要字段包括：

- `status`、`version`、`uptime_seconds`
- `agents.total/active`
- `cron.total/disabled/running/last_tick_ago_secs`
- `sessions.total/busy`
- `providers.probed_ok/probed_fail`
- `telegram.connected/last_message_at`
- `memory_mb`

`status: ok` 主要表示进程可响应，不表示所有 Provider 和后台任务都正常。`telegram.last_message_at` 在当前实现中可能为 `null`。

```bash
curl -fsS http://127.0.0.1:8080/healthz
```

### `GET /readyz`

无需鉴权，返回 HTTP 200 或 503。检查：

- Cron 心跳：从未启动或超过 60 秒未刷新会失败。
- 会话池：活动 worker 超过硬编码上限 200 会失败。
- Provider 探测快照：未探测的冷启动视为 unknown/ok；已有探测且全部失败时失败。

`/readyz` 不主动请求 Provider，只读取内存快照。响应中的 `checks.cron`、`checks.sessions`、`checks.providers` 是排查依据。

```bash
curl -sS -o /tmp/readyz.json -w '%{http_code}\n' \
  http://127.0.0.1:8080/readyz
```

### `GET /api/status`

需要 `Authorization: Bearer <token>`，返回更详细的成员、Cron、内存和 goroutine 状态：

```bash
curl -fsS \
  -H "Authorization: Bearer $ZYHIVE_TOKEN" \
  http://127.0.0.1:8080/api/status
```

不要把管理员 Token 写入监控系统的公开配置或日志。优先从受限 Secret 文件注入。

## 探针建议

- 存活探针：`/healthz`，连续失败后由 systemd/launchd 或外部管理器重启。
- 就绪/告警探针：`/readyz`，503 时先读取 `checks`，不要直接将其等同于进程崩溃。
- 版本探针：同时核对 `/healthz.version`，升级期间用于确认目标版本。
- 业务探针：另建最小、只读且受鉴权的真实旅程；内建健康端点不会验证完整对话链路。

systemd 可用外部监控定时执行 curl，但不要把探针写成会触发频繁重启的 `ExecStartPost`。launchd 的 `KeepAlive` 只关心进程生命周期，不理解 `/readyz`。

## 日志

环境变量：

```bash
LOG_FORMAT=json
LOG_LEVEL=info
```

- `LOG_FORMAT`：`text` 或 `json`，默认 `text`
- `LOG_LEVEL`：`debug`、`info`、`warn`、`error`，默认 `info`

Linux：

```bash
sudo journalctl -u zyhive --since '1 hour ago'
sudo journalctl -u zyhive -f
```

macOS 系统服务：

```bash
sudo tail -F /var/log/zyhive/stdout.log /var/log/zyhive/stderr.log
```

macOS 用户服务：

```bash
tail -F ~/Library/Logs/zyhive/stdout.log ~/Library/Logs/zyhive/stderr.log
```

API 请求中间件生成或沿用 trace ID，适合在请求日志中关联一次调用；这不等于跨 Provider 的完整分布式追踪。

## 指标边界

稳定核心没有通用、承诺兼容的 `/metrics`。只有实验 AITeam 指标启用时才注册公开 `/metrics`，且可能暴露敏感业务信息。不要为了监控而在生产主实例随意开启实验功能；如确需使用，应在反向代理或防火墙限制采集端。

## 最小告警

建议外部系统至少告警：

- `/healthz` 连续不可达；
- `/readyz` 持续 503；
- 健康版本与期望版本不一致；
- systemd/launchd 重启频率异常；
- 磁盘空间不足；
- 最近一次备份或 `inspect` 失败；
- 日志出现 `rolledback`、`SHA-256 校验失败`、`secretref` 或配置加载失败。

阈值和通知渠道需由部署环境实现，ZyHive 本身不提供 HA 故障转移或完整告警闭环。
