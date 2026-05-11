# aiteam 提议目录 INDEX

> 简短目录，详见各 PR-XXX 文件 + 顶部 `README.md`

## PR

| ID | File | 状态 | 优先级 |
|----|------|-----|--------|
| PR-001 | [PR-001-wallet.md](PR-001-wallet.md) | ✅ landed S5 (26.5.10v11) | 🔴 P0 |
| PR-002 | [PR-002-payroll.md](PR-002-payroll.md) | ✅ landed S8 (26.5.10v14) | 🔴 P0 |
| PR-003 | [PR-003-budget-guard.md](PR-003-budget-guard.md) | ✅ landed S4 (26.5.10v10) | 🔴 P0 |
| PR-004 | [PR-004-judge-agent.md](PR-004-judge-agent.md) | ✅ landed S7 (26.5.10v13) heuristic v0 | 🔴 P0 |
| PR-005 | [PR-005-revenue.md](PR-005-revenue.md) | ✅ landed S9 (26.5.10v15) | 🟠 P1 |
| PR-006 | [PR-006-dashboard.md](PR-006-dashboard.md) | 🟡 backend landed S10, UI 后续 | 🟠 P1 |
| PR-007 | [PR-007-sandbox.md](PR-007-sandbox.md) | ✅ landed S2 (26.5.10v8) | 🟠 P1 |
| PR-008 | [PR-008-prompt-defense.md](PR-008-prompt-defense.md) | ✅ landed S3 (26.5.10v9) | 🟠 P1 |

## Bugs

| ID | File | Severity | Fix |
|----|------|----------|-----|
| B001 | [bugs/B001-path-traversal.md](bugs/B001-path-traversal.md) | 🔴 CRITICAL | ✅ 26.5.10v2 |
| B002 | [bugs/B002-timing-attack.md](bugs/B002-timing-attack.md) | 🟠 HIGH | ✅ 26.5.10v3 |
| B003 | [bugs/B003-unbounded-body-oom.md](bugs/B003-unbounded-body-oom.md) | 🟠 HIGH | ✅ 26.5.10v4 |
| B004 | [bugs/B004-slowloris.md](bugs/B004-slowloris.md) | 🟠 HIGH | ✅ 26.5.10v5 |
| B005 | [bugs/B005-stdlib-cve-toolchain.md](bugs/B005-stdlib-cve-toolchain.md) | 🟠 HIGH | 🔄 S1 修复中 (Go toolchain bump) |
| B006 | [bugs/B006-feishu-proto-int-overflow.md](bugs/B006-feishu-proto-int-overflow.md) | 🟡 MEDIUM | 🔄 推后 S3 |
| B007 | [bugs/B007-self-restart-exec.md](bugs/B007-self-restart-exec.md) | 🟢 LOW | 📝 不修 (false positive) |
| B008 | [bugs/B008-cli-editor-exec.md](bugs/B008-cli-editor-exec.md) | 🟢 LOW | 📝 不修 (false positive) |
| B009 | [bugs/B009-llm-retry-weakrand.md](bugs/B009-llm-retry-weakrand.md) | 🟢 LOW | 📝 不修 (非安全敏感) |
| B010 | [bugs/B010-startup-test-ssrf.md](bugs/B010-startup-test-ssrf.md) | 🟢 LOW | 📝 不修 (false positive) |
| B011 | [bugs/B011-cli-token-display-as-credential.md](bugs/B011-cli-token-display-as-credential.md) | 🟢 LOW | 📝 不修 (false positive) |
| B012 | [bugs/B012-tool-readwrite-path-taint.md](bugs/B012-tool-readwrite-path-taint.md) | 🟢 LOW | ✅ B001 已覆盖 |
| B013 | [bugs/B013-memory-indexer-race-walk.md](bugs/B013-memory-indexer-race-walk.md) | 🟢 LOW | 🟡 后续闲时切 WalkDir |
| B014 | [bugs/B014-file-perms-lax.md](bugs/B014-file-perms-lax.md) | 🟡 MEDIUM | 🔄 S2-S4 渐进修 |
| B015 | [bugs/B015-untracked-error-returns.md](bugs/B015-untracked-error-returns.md) | 🟢 LOW | 🔄 S5 (wallet) 核心路径 |
| **B016** | [bugs/B016-hardcoded-github-pat.md](bugs/B016-hardcoded-github-pat.md) | 🟠 HIGH | ✅ 脚本修 P3-S6 / 仓库所有者需 revoke PAT |
| **B017** | [bugs/B017-hardcoded-prod-root-password.md](bugs/B017-hardcoded-prod-root-password.md) | 🔴 CRITICAL | ✅ 脚本修 P3-S6 / 仓库所有者需改服务器密码 |
| QA artifacts | `bugs/_qa-pass-26.5.10v6-*.{json,txt}` | — | 原始扫描产物 |

## 与 ZyHive 主项目协调

- ZyHive 通用改进：见 `proposals/zyhive-improvements/INDEX.md`
- ZyHive P0 已落地：26.5.10v1 (logging / readyz / CI / self_schedule / budget brake / AdaptiveThrottle)
- aiteam 提议默认 off + experimental flag，零影响 ZyHive 主线

## 落地状态总览（2026-05-10 收官）

aiteam S0-S10 全部 11 个阶段单日完成：

| 阶段 | 内容 | 版本 |
|---|------|------|
| S0  | flag 框架 + AWS staging + 路由壳 | 26.5.10v6 |
| S1  | B005-B015 QA pass + B005/B014 fix | 26.5.10v7 |
| S2  | PR-007 工具沙箱 | 26.5.10v8 |
| S3  | PR-008 提示词注入防御 + audit 包 | 26.5.10v9 |
| S4  | PR-003 BudgetGuard (USDT decimal) | 26.5.10v10 |
| S5  | PR-001 Wallet + FX 货币层 | 26.5.10v11 |
| S6  | Guard × Wallet 联动 | 26.5.10v12 |
| S7  | PR-004 Judge Agent (heuristic v0) | 26.5.10v13 |
| S8  | PR-002 Payroll | 26.5.10v14 |
| S9  | PR-005 Revenue webhook | 26.5.10v15 |
| S10 | PR-006 Dashboard overview + Genesis E2E | 26.5.10v16 |

详见 [docs/aiteam-architecture.md](../../docs/aiteam-architecture.md)。

## 配套文档

- [docs/aiteam-architecture.md](../../docs/aiteam-architecture.md) — 总览图、flags、数据流、边界
- [docs/aiteam-wallet-protocol.md](../../docs/aiteam-wallet-protocol.md) — wallet ledger 协议
- [docs/aiteam-fx-and-currency.md](../../docs/aiteam-fx-and-currency.md) — FX 多币种显示层
- [docs/aiteam-revenue-protocol.md](../../docs/aiteam-revenue-protocol.md) — Revenue webhook v1 协议
- [docs/aiteam-deploy-aws.md](../../docs/aiteam-deploy-aws.md) — AWS staging 部署指南
