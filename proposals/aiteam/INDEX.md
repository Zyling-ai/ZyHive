# aiteam 提议目录 INDEX

> 简短目录，详见各 PR-XXX 文件 + 顶部 `README.md`

## PR

| ID | File | 状态 | 优先级 |
|----|------|-----|--------|
| PR-001 | [PR-001-wallet.md](PR-001-wallet.md) | 📝 spec 收集中 | 🔴 P0 |
| PR-002 | [PR-002-payroll.md](PR-002-payroll.md) | 📝 spec 收集中 | 🔴 P0 |
| PR-003 | [PR-003-budget-guard.md](PR-003-budget-guard.md) | 🟡 初稿 v0 | 🔴 P0 |
| PR-004 | [PR-004-judge-agent.md](PR-004-judge-agent.md) | 📝 spec 收集中 | 🔴 P0 |
| PR-007 | [PR-007-sandbox.md](PR-007-sandbox.md) | ✅ landed S2 | 🟠 P1 |

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
| QA artifacts | `bugs/_qa-pass-26.5.10v6-*.{json,txt}` | — | 原始扫描产物 |

## 与 ZyHive 主项目协调

- ZyHive 通用改进：见 `proposals/zyhive-improvements/INDEX.md`
- ZyHive P0 已落地：26.5.10v1 (logging / readyz / CI / self_schedule / budget brake / AdaptiveThrottle)
- aiteam 提议默认 off + experimental flag，零影响 ZyHive 主线
