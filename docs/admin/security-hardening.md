# 安全加固

ZyHive `26.8.2v1` 是单机、自托管 Beta。加固目标是缩小单机暴露面和凭据泄露面，不应宣称已经具备企业级隔离、HA 或 RBAC。

![ZyHive 信任边界](../assets/diagrams/trust-boundaries.svg)

## 网络边界

1. 服务绑定 `localhost`，仅通过受控 TLS 反向代理暴露。
2. 防火墙只开放必要的 80/443；不要直接向公网开放 8080。
3. 为管理入口增加上游访问控制，例如 VPN、IP allowlist 或身份代理。
4. 限制出站网络到实际使用的模型、消息渠道和更新源。

`/healthz`、`/readyz`、`/api/version` 和 `/api/update/status` 不需要管理员 Token。健康响应会公开版本和部分运行统计，应在防火墙或反向代理层限制来源。实验 AITeam 开关启用后，公开 `/metrics` 可能包含成员 ID、钱包、工资和评分指标；默认不要在公网启用。

## 凭据与 SecretRef

- 新建和经程序保存的主配置使用 `0600`，敏感目录使用 `0700`；加载已有主配置不会自动 `chmod`，升级或迁移后必须人工检查历史文件权限。
- Provider Key、渠道 Token 和管理员 Token 优先使用 SecretRef。
- `$file` 秘密文件只授予服务账户读取权限。
- 不把 Token 放进命令行参数、Shell 历史、工单或日志。
- 修改或疑似泄露管理员 Token 后，重启服务并更新客户端凭据。
- `zyhive token` 会向终端输出完整 Token，只在受控终端使用。

## 运行账户与文件权限

程序已对多类成员、任务和渠道敏感文件使用 `0700/0600` 并在启动时收紧历史权限，但管理员仍应定期检查：

```bash
find /var/lib/zyhive -type d ! -perm 0700 -print
find /var/lib/zyhive -type f -perm /077 -print
stat -f '%Sp %N' /etc/zyhive/zyhive.json 2>/dev/null || \
  stat -c '%A %n' /etc/zyhive/zyhive.json
```

安装器的 systemd unit 没有 `User=`，系统安装默认以 root 运行。改用独立账户可以降低宿主风险，但必须完整验证文件访问、端口、备份和在线更新；在线更新需要写二进制目录。不要在未经测试时直接加只读文件系统或阻止写入工作目录。

## 安装与供应链验证

默认安装和安装器更新会强制：

- 下载对应目标的二进制；
- 核对 Release 的 `SHA256SUMS`；
- 运行新二进制并核对内嵌版本。

但默认 `ZYHIVE_VERIFY_SIGNATURE=0`，即**发布者签名验证并非强制**。高信任环境应安装 `cosign` 并执行：

```bash
ZYHIVE_VERSION=26.8.2v1 \
ZYHIVE_VERIFY_SIGNATURE=1 \
bash /tmp/zyhive-install.sh --verify-signature
```

严格模式验证 Sigstore bundle、GitHub Actions OIDC issuer 和发布工作流身份，失败即停止。Web 在线更新当前只验证 SHA-256 和版本，不消费 Sigstore 签名；要求发布者身份验证时，应使用严格安装器流程进行更新。

## 工具与公开入口

- 全局 `toolPolicy` 是上限，成员策略只能继续收紧。
- 对非必要工具使用 deny 或 ask；审批不可用时 ask 会拒绝。
- 默认 Shell/Process 可访问宿主，不是完整沙箱。
- 公共 Web 聊天和消息渠道都应按外部不可信输入处理。
- 不把 Agent 工具权限当作 Linux 用户隔离、容器隔离或 RBAC。

## 备份保护

备份归档包含配置、管理员 Token、Provider Key、渠道凭据和业务数据。当前归档只有 gzip 压缩、manifest 与 SHA-256，**没有加密**。创建后应：

```bash
chmod 600 /secure/path/zyhive-backup.tar.gz
```

并使用独立加密介质或经过审计的外部加密工具加密后再离机保存。完整性校验不等于保密。

## Accepted Risks

`26.8.2v1` 明确保留以下已接受风险，部署评审必须记录：

1. 默认 Shell / Process 能力保持可用。Agent 可能在宿主上执行命令、访问网络和读取其运行账户可访问的文件；实验沙箱不等于完整 OS 隔离。
2. 目标规划上下文继续允许携带管理 Token。该 Token 可能进入第三方模型请求、会话或日志；使用该功能需接受凭据外发风险并准备轮换。

另有尚未解决但不应隐瞒的边界：

- 默认安装和在线更新不强制验证发布者签名。
- 备份未加密。
- 管理平面只有单一 Token，没有 RBAC。
- 不承诺高可用、多实例或关键业务连续性。
