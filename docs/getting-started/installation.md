# 安装 ZyHive

> 适用于 `26.8.2v1`。支持 Linux / macOS 的 `amd64` 与 `arm64`；不提供 Windows 和 32 位 ARM 正式安装包。

## 安装前

准备：

- 可访问 `install.zyling.ai` 或 GitHub Releases 的网络；
- `curl`（缺失时安装器会尝试通过常见包管理器安装）；
- 建议具备 `sudo`，以便安装系统服务；没有 `sudo` 时会降级到用户目录；
- 至少一个可用模型 Provider 的 API Key，或一个显式配置在本机精确回环地址上的 Ollama 服务。

不要把 API Key 或管理员 Token 粘贴到工单、聊天记录或版本库。

## 标准安装

```bash
curl -fsSL https://install.zyling.ai/install | bash
```

安装向导会：

1. 识别系统与 CPU 架构；
2. 获取最新正式 Release；
3. 下载对应二进制和 `SHA256SUMS` 并强制校验；
4. 首次安装时询问 Provider、模型和可选 Telegram 配置；
5. 生成随机管理员 Token，并以 `0600` 权限写入配置；
6. 在支持的环境注册并启动 systemd 或 launchd。

已有配置不会被向导覆盖。安装完成后记录终端打印的访问地址、配置路径和管理员 Token。

## 首次验证

```bash
zyhive --version
zyhive status
```

期望版本为当前安装的正式 Release。然后打开安装器打印的地址；默认端口是 `8080`。首次进入管理界面时使用安装器输出的管理员 Token。

如果终端无法直接找到 `zyhive`，用户目录安装的二进制位于 `~/.local/bin/zyhive`；将 `~/.local/bin` 加入 `PATH` 后重试。

## 安装位置

| 模式 | 二进制 | 配置 | 成员数据 |
| --- | --- | --- | --- |
| Linux 系统安装 | `/usr/local/bin/zyhive` | `/etc/zyhive/zyhive.json` | `/var/lib/zyhive/agents` |
| macOS 系统安装 | `/usr/local/bin/zyhive` | `/usr/local/etc/zyhive/zyhive.json` | `/var/lib/zyhive/agents` |
| 用户目录安装 | `~/.local/bin/zyhive` | `~/.config/zyhive/zyhive.json` | `~/.local/share/zyhive/agents` |

共享项目默认相对服务工作目录使用 `projects/`。备份时应使用产品提供的备份命令，不要只复制成员目录。

## 常用安装选项

通过 `bash -s --` 把参数传给远程脚本：

```bash
curl -fsSL https://install.zyling.ai/install | bash -s -- --no-root
```

常用参数：

- `--no-root`：强制安装到当前用户目录；
- `--no-service`：不注册系统服务，适合容器、CI 或手动托管；
- `--bind localhost|lan|all|IP`：设置监听模式；
- `--port 8080`：设置端口；
- `--skip-setup`：跳过交互式 Provider/Telegram 向导；
- `--verify-signature`：在 SHA-256 之外严格验证发布者签名。

无服务模式的安装器会打印完整手动启动命令。其形式为：

```bash
~/.local/bin/zyhive --serve --config ~/.config/zyhive/zyhive.json
```

实际路径以安装器输出为准。

## 严格签名验证（可选）

默认安装会强制验证二进制与同源 `SHA256SUMS` 一致，但不会验证签名者身份。高保证环境先安装 `cosign`，再运行：

```bash
curl -fsSL https://install.zyling.ai/install | bash -s -- --verify-signature
```

严格模式会下载对应 Sigstore bundle，并验证 GitHub Actions OIDC 身份；缺少 `cosign`、bundle 或身份不匹配时安装立即失败。

## 服务操作

优先使用统一 CLI：

```bash
zyhive status
zyhive restart
zyhive token
```

`zyhive token` 会以明文输出管理员 Token，避免在共享终端或录屏环境使用。

Linux systemd 也可直接操作：

```bash
sudo systemctl status zyhive
sudo journalctl -u zyhive -f
sudo systemctl restart zyhive
```

macOS 日志位置由安装模式决定：

- 系统安装：`/var/log/zyhive/`
- 用户安装：`~/Library/Logs/zyhive/`

## 源码构建

源码构建需要 Go `1.25.10`（以 [`go.mod`](../../go.mod) 为准）和可运行当前前端工具链的 Node.js/npm。

```bash
git clone https://github.com/Zyling-ai/ZyHive.git
cd ZyHive
cd ui && npm ci && cd ..
make build
./bin/aipanel --serve --config ./aipanel.json
```

`make build` 会构建前端、同步 `ui/dist` 到 Go 嵌入目录，再编译后端。裸 `go build` 不能替代这条完整产品构建链。

## 常见问题

### 页面打不开

1. 运行 `zyhive status`；
2. 检查安装器输出的端口和地址；
3. 检查主机防火墙；
4. 用户目录安装确认对应 launchd 用户服务已加载；
5. 公网部署不要直接暴露明文 HTTP，使用 TLS 反向代理。

### 能登录但不能对话

进入“设置/模型”相关页面，确认：

- Provider 已配置真实凭据；
- Model 引用了正确 Provider；
- 至少一个 Model 可作为默认或已分配给成员；
- 连通性测试通过；
- 所选模型支持所需的工具调用。

### 更新或安装下载失败

安装器会先使用 `install.zyling.ai`，必要时回退 GitHub。确认 DNS、HTTPS 出站和系统时间正常。不要通过关闭 SHA-256 校验绕过失败。

安装完成后继续：[核心概念](concepts.md) → [创建第一个团队](first-team.md)。
