# ZyHive 文档入口

> 文档版本：V1.0  
> 基准日期：2026-08-01  
> 状态：当前文档分类入口  
> 适用范围：`docs/` 下全部产品、设计、实验与历史文档

## 先读什么

1. 当前产品、安装与功能：`../README.md`
2. 当前发布变化：`../CHANGELOG.md`
3. 当前优化路线：`optimization-roadmap-2026-2027.md`
4. 开源与商业边界：`commercial-boundaries.md`
5. Agent CLI：`agent-cli.md`

## 文档分类

### 当前执行文档

- `optimization-roadmap-2026-2027.md`：当前路线与完成状态；
- `commercial-boundaries.md`：当前 AGPL、服务与定制边界；
- `agent-cli.md`：当前 CLI 使用说明，具体命令仍以 `zyhive --help` 为准。

### 实现快照

- `system-prompt-and-flow.md`：截至 `26.5.16v1` 的提示词与运行流程快照；
- `session-design.md`：截至 `26.5.16v1` 的会话设计快照；
- `skillopt-design.md`：SkillOpt 原始设计与实施参考。

实现快照不是 `26.8.1v6` 的完整规范。发生冲突时，以当前代码、测试和正式 Release 为准。

### 历史前瞻与概念

- `roadmap-v0.10.md`：2026-02-21 前瞻稿，部分采用不同方案落地；
- `zystudio/`：历史商业概念和实验方向，不是当前立项或已上线产品。

### aiteam 实验文档

以下文档描述默认关闭的实验经济模块：

- `aiteam-architecture.md`
- `aiteam-wallet-protocol.md`
- `aiteam-revenue-protocol.md`
- `aiteam-fx-and-currency.md`
- `aiteam-genesis-demo.md`
- `aiteam-deploy-aws.md`

这些文件只用于实验追溯和受控测试，不代表稳定版默认能力、公司金融服务、真实资产托管、现行生产环境或对外商业承诺。文中的公网地址、部署记录和版本均为历史快照。

## 判断规则

- “已实现”以当前代码和测试为准；
- “已发布”以 GitHub 正式 Release 为准；
- “默认可用”必须在未设置实验环境变量时可见；
- 提案编号、历史版本和演示成功不能证明当前仍支持；
- 安全、权限、网络和进程边界以最新代码与安全测试为准。

