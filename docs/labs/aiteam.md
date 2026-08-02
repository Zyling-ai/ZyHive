# aiteam 自治经济实验边界

> 标签：**Labs**  
> 默认状态：关闭  
> 适用基线：`26.8.2v1`

aiteam 是自治经济相关的实验模块，不属于 ZyHive Stable 能力。它用于验证内部记账、预算护栏、评分、工资、收益接入和防护机制，不是银行、托管、会计、税务、证券、支付或劳动薪酬系统。

## 实验范围

当前仓库包含以下实验子系统：

- Wallet 与内部 USDT 记账语义；
- FX 汇率与币种换算；
- Budget Guard 预算护栏；
- Judge 评分；
- Payroll 工资计算与定时执行；
- Revenue 收益接入；
- Sandbox 弱加固执行；
- PromptDef 提示注入防护；
- aiteam Dashboard 与审计展示。

实现事实源为 `pkg/aiteam/`、`internal/api/aiteam_routes.go`、`cmd/aipanel/main.go` 及对应测试。`docs/aiteam-*.md` 和 `proposals/aiteam/` 保留了设计、协议、部署和演示语境，不能单独证明当前行为。

## 启用开关

各子系统独立控制，未设置时默认关闭：

- `ZYHIVE_EXPERIMENTAL_WALLET`
- `ZYHIVE_EXPERIMENTAL_PAYROLL`
- `ZYHIVE_EXPERIMENTAL_BUDGETGUARD`
- `ZYHIVE_EXPERIMENTAL_JUDGE`
- `ZYHIVE_EXPERIMENTAL_REVENUE`
- `ZYHIVE_EXPERIMENTAL_SANDBOX`
- `ZYHIVE_EXPERIMENTAL_PROMPTDEF`
- `ZYHIVE_EXPERIMENTAL_AITEAM_DASHBOARD`

不得一次性在生产环境打开全部开关。启用前应从 `docs/reference/environment-variables.md` 和当前代码核对精确值、依赖和组合关系；不同子系统可能依赖 Wallet、Agent Pool、模型配置或其他实验模块。

## API 与页面边界

- 后端实验 API 位于 `/api/aiteam/*`；除收益 webhook 的专用认证路径外，路由位于管理鉴权边界内。
- 关闭的子系统通常以 404 隐藏；开关打开但依赖未初始化时可能返回 503，尚未实现的能力可能返回 501。
- `/api/aiteam/flags` 只报告布尔开关，不证明依赖健康、数据完整或功能可用。
- UI 路由 `/aiteam`、`/aiteam/wallet`、`/aiteam/fx`、`/aiteam/guard`、`/aiteam/judge`、`/aiteam/payroll` 的存在不表示子系统已启用或受稳定支持。

## 明确不承诺

- 不托管真实资金，不保证链上资产、法币或第三方支付余额；
- 不把内部 USDT 数字等同于可兑付资产；
- 不用于真实工资发放、劳动关系认定、税务申报或财务记账；
- Judge 评分不用于雇佣、解雇、信用、合规或其他影响个人权益的自动决策；
- Revenue webhook 不代表收入已结算、可撤销性已处理或来源合规；
- FX 不保证实时性、完整性或交易可执行价格；
- Sandbox 不是容器、虚拟机、chroot、命名空间、文件系统隔离或网络防火墙，不能作为恶意代码的强安全边界；
- PromptDef 只能降低部分提示注入风险，不能保证模型输出或工具调用安全；
- 实验数据格式、接口、页面和迁移方式可能随版本变化。

## 安全与数据要求

1. 只在独立实例、测试账号、测试凭据和非关键数据上启用。
2. 启用前创建可验证备份，记录版本、环境变量和数据目录；先演练停用与恢复。
3. 不写入真实私钥、助记词、支付密钥、客户财务数据、员工薪资或受监管数据。
4. 对所有 credit、transfer、payroll、revenue 和配置变更保留人工审批与外部审计记录。
5. 对 webhook 使用独立强密钥、重放防护、来源限制和最小网络暴露；不能仅依赖路径保密。
6. `SANDBOX` 开启后仍按宿主机命令执行风险管理，限制工具策略和运行账户权限。
7. 实验异常时先停止相关开关和入口，再核对账本、审计和外部系统；不得自动重放不确定副作用。

## 停用与回滚

- 停用相应 `ZYHIVE_EXPERIMENTAL_*` 开关并重启服务；
- 确认 `/api/aiteam/flags` 与相关入口已关闭；
- 保留实验数据和审计副本，不在未核对外部副作用前直接删除；
- 如需恢复，使用启用前备份，并按同一版本验证；
- 版本升级前先阅读 `CHANGELOG.md`，在隔离副本验证数据兼容后再迁移。

## 历史材料

下列材料可用于理解设计背景，但不代表当前生产能力：

- `docs/labs/aiteam/aiteam-architecture.md`
- `docs/labs/aiteam/aiteam-wallet-protocol.md`
- `docs/labs/aiteam/aiteam-fx-and-currency.md`
- `docs/labs/aiteam/aiteam-revenue-protocol.md`
- `docs/labs/aiteam/aiteam-deploy-aws.md`
- `docs/labs/aiteam/aiteam-genesis-demo.md`
- `proposals/aiteam/`

分类说明见[历史文档索引](../archive/README.md)。当前承诺程度以 `docs/reference/feature-status.md`、代码、测试和正式 Release 为准。
