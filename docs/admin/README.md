# ZyHive 管理员部署手册

本文档以公开稳定版 `26.8.2v1` 的代码、安装脚本和发布流程为事实基线，面向单机、自托管部署。ZyHive 当前不承诺多实例、高可用、无停机升级、多租户或细粒度 RBAC；不要把本文档当作这些能力的说明。

## 支持范围

官方发布并验证四个目标：

- Linux amd64：`zyhive-linux-amd64`
- Linux arm64：`zyhive-linux-arm64`
- macOS amd64：`zyhive-darwin-amd64`
- macOS arm64：`zyhive-darwin-arm64`

Windows 不在安装脚本和在线升级的支持范围内。Linux 系统服务使用 systemd；macOS 使用 launchd。容器、其他 init 系统和外部进程管理器可用 `--no-service`，但需由管理员自行负责启动、停止、日志和重启。

## 文档导航

- [部署](deployment.md)：四目标安装、systemd、launchd 与验收
- [配置](configuration.md)：配置路径、绑定模式、SecretRef 和环境变量
- [安全加固](security-hardening.md)：权限、暴露面、签名验证与 Accepted Risks
- [备份与恢复](backup-and-restore.md)：归档范围、校验、离线恢复与未加密边界
- [升级与回滚](upgrade-and-rollback.md)：发布门禁、安装器更新、在线更新三套保证
- [可观测性](observability.md)：日志、`/healthz`、`/readyz`、`/api/status`
- [故障排查](troubleshooting.md)：常见故障的定位与恢复

## 最小生产基线

1. 固定版本部署，记录 `zyhive --version`。
2. 配置文件保持 `0600`，敏感目录保持 `0700`。
3. 使用 `SecretRef`，避免把新凭据直接写入 JSON。
4. 对外访问放在 TLS 反向代理后，服务本身绑定 `localhost`。
5. 安装时显式启用 `--verify-signature`；在线更新目前没有等价的签名强制校验。
6. 定期创建备份、执行 `inspect`，并在隔离主机演练恢复。
7. 监控 `/healthz` 与 `/readyz`，同时保留服务日志。

## 事实边界

- 产品定位是单机、自托管 Beta。
- 管理 API 使用一个 Bearer Token；这不是用户、角色或资源级 RBAC。
- 备份是带清单与 SHA-256 的 gzip tar，**未加密**。
- 发布资产具有 Sigstore/SBOM/来源证明，但默认安装只强制 SHA-256；发布者签名验证需显式开启。
- 工具权限策略约束 Agent 可调用的工具，不等于操作系统沙箱或管理员 RBAC。
