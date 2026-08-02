# 引巢 · ZyHive

> 自托管的 AI 团队操作系统：以团队为核心，把 AI Agent 组织为有身份、记忆、工具、工作区和协作关系的成员。

[![CI](https://github.com/Zyling-ai/ZyHive/actions/workflows/ci.yml/badge.svg)](https://github.com/Zyling-ai/ZyHive/actions/workflows/ci.yml)
[![Version](https://img.shields.io/badge/version-26.8.2v1-brightgreen.svg)](CHANGELOG.md)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)
[![Go 1.25+](https://img.shields.io/badge/Go-1.25+-00ADD8.svg)](go.mod)

当前事实基线是 `26.8.2v1`。ZyHive 面向个人、小团队和可控环境中的单机自托管部署；它不是多租户 SaaS、企业级高可用平台或关键业务连续性方案。完整能力状态与边界见[功能状态](docs/reference/feature-status.md)。

## 快速开始

支持 macOS / Linux 的 `amd64` 与 `arm64`：

```bash
curl -fsSL https://install.zyling.ai/install | bash
```

安装器会下载当前正式 Release、校验 SHA-256、生成权限为 `0600` 的配置，并在可用时注册 systemd 或 launchd 服务。安装结束后按终端输出打开访问地址并保存管理员 Token。

继续阅读：[完整安装说明](docs/getting-started/installation.md) · [创建第一个团队](docs/getting-started/first-team.md)

## 你可以用它做什么

- 创建多个 AI 成员，分别配置身份、行为风格、模型、记忆、技能和工具权限；
- 在 Web 管理界面对话，并持久化会话、用量、工作区文件与工具审计；
- 通过成员关系、共享项目和受控派遣组织协作；
- 配置 Cron、Telegram、飞书和公开 Web 聊天等运行入口；
- 使用全局与成员两层工具策略，以及 `allow` / `deny` / `ask` 审批；
- 备份、恢复和在线更新单机实例。

这些能力并非都在所有配置下自动可用：模型、外部渠道和网络工具需要相应凭据；aiteam 自治经济模块默认关闭且属于 Labs。请以[功能状态](docs/reference/feature-status.md)为准。

## 按角色阅读

- **第一次使用**：[安装](docs/getting-started/installation.md) → [核心概念](docs/getting-started/concepts.md) → [第一个团队](docs/getting-started/first-team.md)
- **日常管理员**：[文档中心](docs/README.md) · [功能状态](docs/reference/feature-status.md) · [备份与变更](CHANGELOG.md)
- **开发者与集成方**：[开发与接口文档](docs/README.md#开发者与集成方) · [Agent CLI](docs/developer/api-and-cli.md)
- **评估者与决策者**：[稳定性与风险边界](docs/reference/feature-status.md) · [开源与商业边界](docs/governance/commercial-boundaries.md) · [优化路线](docs/archive/roadmaps/optimization-roadmap-2026-2027.md)

## 发布与安全边界

正式发布采用 Draft-first 门禁：四个平台的安装与更新验证、备份恢复、健康回滚、SBOM、签名、构建来源和可复现构建全部通过后才公开。安装器默认校验同源 `SHA256SUMS`；如本机已安装 `cosign`，可下载脚本后使用 `--verify-signature` 开启失败即停止的发布者身份验证，详见[安装说明](docs/getting-started/installation.md#严格签名验证可选)。

`26.8.2v1` 仍接受若干明确风险，包括单机架构、非高可用、管理员 Token 模式和部分功能依赖第三方服务。详见[Accepted Risk](docs/reference/feature-status.md)。

## 开发

完整构建必须先构建并同步前端嵌入资源：

```bash
cd ui
npm ci
cd ..
make build
```

运行质量门禁：

```bash
make check
```

Go 版本以 [`go.mod`](go.mod) 为准，当前为 `1.25.10`。不要用裸 `go build` 替代 `make build` 生成面向用户的完整二进制。

## 许可证

ZyHive 使用 [GNU AGPL-3.0](LICENSE)。通过网络提供修改版交互服务时，应向对应用户提供获取相应源代码的明确方式。部署、配置、培训、运维和定制服务可以收费；详见[开源、服务与客户定制边界](docs/governance/commercial-boundaries.md)。
