# 升级与回滚

`26.8.2v1` 存在三套名称相近但保证不同的更新链路。管理员必须区分“Release 能否公开”“安装器能否替换二进制”和“运行中在线更新能否在启动失败后回滚”。

![Draft-first 发布状态机](../assets/diagrams/release-state-machine.svg)

![在线更新与 Watchdog 回滚](../assets/diagrams/update-rollback.svg)

## 三套更新保证

### 1. Release 发布门禁

适用对象：项目发布方，不直接操作本地主机。

- 先创建不可见 Draft Release。
- 构建 Linux amd64/arm64、macOS amd64/arm64 四个目标。
- 验证测试、安装、真实旧版更新、健康回滚、SHA-256、SBOM、Sigstore 签名和构建来源。
- 只有全部门禁通过，唯一 promote 作业才公开 Release；失败时 Draft 不进入 latest。

这保证公开候选经过四目标门禁，不保证用户主机的配置、权限、磁盘和外部依赖一定兼容。

### 2. 安装器更新

适用对象：再次运行 `scripts/install.sh` 或安装端点。

- 在停服务前下载新二进制。
- 强制校验 `SHA256SUMS` 和内嵌版本。
- 将当前二进制保存为 `<binary>.bak`。
- 停止服务，使用同目录 `.new` 原子替换，再启动服务。
- 替换失败时尝试复制 `.bak` 恢复。

限制：

- 默认不验证 Sigstore 发布者签名；需 `--verify-signature` 或 `ZYHIVE_VERIFY_SIGNATURE=1`。
- 启动后不执行版本匹配的 `/healthz` 守护确认。
- 因此“服务启动命令返回”不等于新版本健康。

### 3. Web/API 在线更新

适用对象：面板或 `POST /api/update/apply`。

- 仅支持四个正式目标。
- 校验 SHA-256 和新二进制 `--version`。
- 保存 `.bak` 和权限为 `0600` 的 pending 记录。
- 同目录暂存并原子替换二进制。
- 从旧版 `.bak` 启动独立 watchdog，当前进程退出，由 systemd/launchd 重启。
- watchdog 最长约 45 秒轮询本机 `/healthz`，只有 HTTP 200、`status=ok` 且版本精确匹配才确认。
- 超时、错误版本或不健康时原子恢复 `.bak`，记录 rolledback，并终止失败的新进程。

限制：

- 在线更新只消费 SHA-256，不验证 Sigstore 发布者身份。
- 需要 systemd、launchd 或等价外部管理器在当前进程退出后启动新进程。
- 只替换二进制，不备份或回滚配置和用户数据。

## 推荐升级流程

1. 阅读目标版本变更，确认只跨受支持版本。
2. 记录当前状态：

```bash
zyhive --version
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
```

3. 创建并检查离线备份。
4. 确认 `<binary>.bak` 所在文件系统有空间且服务账户可写。
5. 高信任环境使用严格安装器更新：

```bash
curl -fsSL https://install.zyling.ai/install -o /tmp/zyhive-install.sh
ZYHIVE_VERSION=TARGET_VERSION \
ZYHIVE_VERIFY_SIGNATURE=1 \
bash /tmp/zyhive-install.sh --verify-signature --yes --skip-setup
```

6. 更新后核对版本、`/healthz`、`/readyz`、日志和一条核心业务旅程。

不要复用已经撤回或内容变化过的标签。版本字符串必须匹配 `YY.M.DvN`，例如 `26.8.2v1`。

## 在线更新观察

```bash
curl -H "Authorization: Bearer $ZYHIVE_TOKEN" \
  http://127.0.0.1:8080/api/update/check

curl -X POST \
  -H "Authorization: Bearer $ZYHIVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"version":"26.8.2v1"}' \
  http://127.0.0.1:8080/api/update/apply

curl http://127.0.0.1:8080/api/update/status
```

`/api/update/status` 是公开端点，重启期间可继续轮询。成功必须以目标版本实际通过健康检查为准，不要只依据前端超时提示。

## 手工回滚二进制

若自动回滚失败，先停止服务：

```bash
sudo systemctl stop zyhive
sudo cp -p /usr/local/bin/zyhive.bak /usr/local/bin/zyhive
sudo chmod 755 /usr/local/bin/zyhive
sudo systemctl start zyhive
```

macOS 使用实际二进制路径并通过 launchctl 停启。恢复后验证：

```bash
/usr/local/bin/zyhive --version
curl -fsS http://127.0.0.1:8080/healthz
```

二进制回滚不会撤销已发生的配置 schema 迁移。若旧版无法读取新配置，必须恢复升级前的完整数据备份；不要只替换二进制。

## 回滚失败处理

- 保留 `.bak`、`.update-pending.json`、`.update-result.json` 和服务日志用于取证。
- 检查二进制目录写权限、磁盘空间和 rename 是否跨文件系统。
- 若服务不断重启，先禁用自动重启或停止服务，再执行手工恢复。
- 若配置加载失败，检查 SecretRef、JSON 和文件权限，再决定恢复配置备份。
