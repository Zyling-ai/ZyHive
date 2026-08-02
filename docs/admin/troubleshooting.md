# 故障排查

以下命令以默认系统安装为例。先确认操作系统、架构、安装方式、配置路径和服务管理器，不要在用户安装上直接套用 sudo/system 路径。

## 快速采集

```bash
uname -s
uname -m
zyhive --version
curl -sS http://127.0.0.1:8080/healthz
curl -sS http://127.0.0.1:8080/readyz
```

Linux：

```bash
sudo systemctl status zyhive --no-pager
sudo journalctl -u zyhive -n 200 --no-pager
```

macOS：

```bash
launchctl list | grep com.zyhive.zyhive
tail -n 200 /var/log/zyhive/stderr.log 2>/dev/null
tail -n 200 ~/Library/Logs/zyhive/stderr.log 2>/dev/null
```

分享诊断信息前删除 Token、API Key、渠道凭据、消息内容和完整配置。

## 服务无法启动

1. 直接验证配置和错误：

```bash
/usr/local/bin/zyhive --serve --config /etc/zyhive/zyhive.json
```

2. 检查 JSON、路径和权限：

```bash
python3 -m json.tool /etc/zyhive/zyhive.json >/dev/null
ls -l /etc/zyhive/zyhive.json /usr/local/bin/zyhive
```

3. 检查端口：

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
```

常见原因：

- SecretRef 环境变量未设置、值为空或 `$file` 不可读；
- 配置文件不是合法 JSON；
- `gateway.bind` 不是 `localhost|lan|all|IP`；
- 端口已被占用；
- WorkingDirectory、`agents.dir` 或数据目录不可写；
- 二进制架构不匹配。

## systemd 反复重启

安装器 unit 使用 `Restart=always`。先停止重启循环再排查：

```bash
sudo systemctl stop zyhive
sudo journalctl -u zyhive -n 300 --no-pager
sudo systemctl daemon-reload
sudo systemctl start zyhive
```

若手工改过 unit，核对 `ExecStart`、`--config` 和 `WorkingDirectory`。不要同时用 systemd 和手工进程占用同一端口或数据目录。

## launchd 未加载

系统级 plist 位于 `/Library/LaunchDaemons/com.zyhive.zyhive.plist`，用户级位于 `~/Library/LaunchAgents/`。系统级需要 root，用户级只在用户登录后运行。核对 plist 中二进制、配置、WorkingDirectory 和日志路径都存在且可访问。

```bash
plutil -lint /Library/LaunchDaemons/com.zyhive.zyhive.plist
launchctl list | grep com.zyhive.zyhive
```

不要同时加载系统级和用户级同名实例。

## `/healthz` 正常但 `/readyz` 为 503

读取响应中的 `checks`：

- `cron: engine never started`：检查启动日志和 Cron 初始化。
- `no heartbeat in Ns`：进程可能阻塞，检查 CPU、磁盘和 goroutine/会话状态。
- `active workers ... > cap 200`：会话积压，限制入口流量并检查卡住的 Provider 请求。
- `all probed providers failing`：检查 Provider URL、Key、DNS、TLS 和出站网络。

冷启动没有 Provider 探测时会显示 ok/unknown，因此 `/readyz` 200 不证明模型凭据有效。

## 401 或修改 Token 后仍旧异常

管理 API 使用 `Authorization: Bearer TOKEN`。Token 修改后需要重启，旧进程仍使用启动时捕获的值。

```bash
zyhive token
sudo systemctl restart zyhive
```

`zyhive token` 输出明文。若配置使用 SecretRef，确保运行 CLI 的环境或秘密文件与服务一致。

## 修改端口后仍监听旧端口

端口修改同样需要重启。重启后验证：

```bash
sudo systemctl restart zyhive
lsof -nP -iTCP -sTCP:LISTEN | grep zyhive
```

同步更新反向代理、防火墙、健康检查和在线更新所依赖的本机地址。

## 安装或更新校验失败

- `SHA256SUMS 中缺少 ...`：目标版本没有对应四目标资产，停止更新。
- SHA-256 不一致：删除临时下载，检查镜像/Release 是否一致，不要绕过校验。
- 版本不匹配：资产标签与内嵌版本不一致，停止使用该 Release。
- 严格签名模式提示缺少 cosign：安装 cosign 后重试。
- Sigstore identity 失败：核对版本、bundle 和官方工作流身份，不要降级为“忽略签名”继续。

默认签名验证不是强制的；只有 SHA-256 通过不能证明发布者身份。

## 在线更新停在 applying 或发生 rolledback

```bash
curl http://127.0.0.1:8080/api/update/status
ls -la /usr/local/bin/zyhive*
sudo journalctl -u zyhive -n 300 --no-pager
```

检查：

- 服务管理器是否在旧进程退出后启动新进程；
- 新 `/healthz` 是否在约 45 秒内返回目标版本；
- 二进制目录是否可写、磁盘是否充足；
- `.bak`、`.update-pending.json`、`.update-result.json` 是否存在；
- 反向代理状态不影响 watchdog；watchdog访问本机 HTTP 健康端点。

必要时按[升级与回滚](upgrade-and-rollback.md)停止服务并从 `.bak` 手工恢复。

## 备份创建失败

常见原因：

- `projects/` 或 `cron/` 等必需根不存在；
- 输出文件位于备份源内部；
- 源中有符号链接或特殊文件；
- 扫描期间源文件发生变化；
- 配置路径或 `--workdir` 错误；
- 归档目录不可写或空间不足。

要求一致性时停止服务后重试。不要通过删除 manifest 校验或解包重组来“修复”归档。

## 恢复失败

恢复前会完整验证，失败时不会开始替换。替换阶段失败会尝试回滚，但恢复不是掉电事务：

1. 保持服务停止；
2. 保存现场，不要删除 `.zyhive-restore-*` 或 `.zyhive-rollback-*` 痕迹；
3. 再次运行 `backup inspect`；
4. 核对目标父目录权限和同文件系统 rename；
5. 必要时恢复“恢复前备份”，再启动服务。

恢复后因 SecretRef 缺失无法启动时，补回归档范围外的环境变量或秘密文件。

## 数据目录并发或损坏

ZyHive 不支持多个实例共享同一工作目录。发现锁冲突、索引反复重建或数据相互覆盖时：

1. 停止所有实例；
2. 确认只保留一个服务管理入口；
3. 创建现场副本；
4. 从最近一次已检查的备份恢复；
5. 单实例启动并逐项验证。

不要把单机文件锁解释为多节点协调或 HA。
