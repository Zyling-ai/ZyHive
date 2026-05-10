# aiteam 提议目录 INDEX

> 简短目录，详见各 PR-XXX 文件 + 顶部 `README.md`

## PR

| ID | File | 状态 | 优先级 |
|----|------|-----|--------|
| PR-001 | [PR-001-wallet.md](PR-001-wallet.md) | 📝 spec 收集中 | 🔴 P0 |
| PR-002 | [PR-002-payroll.md](PR-002-payroll.md) | 📝 spec 收集中 | 🔴 P0 |
| PR-003 | [PR-003-budget-guard.md](PR-003-budget-guard.md) | 🟡 初稿 v0 | 🔴 P0 |
| PR-004 | [PR-004-judge-agent.md](PR-004-judge-agent.md) | 📝 spec 收集中 | 🔴 P0 |

## Bugs

| ID | File | Severity | Fix |
|----|------|----------|-----|
| B001 | [bugs/B001-path-traversal.md](bugs/B001-path-traversal.md) | 🔴 CRITICAL | ✅ 26.5.10v2 |
| B002 | [bugs/B002-timing-attack.md](bugs/B002-timing-attack.md) | 🟠 HIGH | ✅ 26.5.10v3 |
| B003 | [bugs/B003-unbounded-body-oom.md](bugs/B003-unbounded-body-oom.md) | 🟠 HIGH | ✅ 26.5.10v4 |
| B004 | [bugs/B004-slowloris.md](bugs/B004-slowloris.md) | 🟠 HIGH | ✅ 26.5.10v5 |
| B005-B015 | bugs/B0xx-template.md | ? | 待用户贴 markdown |

## 与 ZyHive 主项目协调

- ZyHive 通用改进：见 `proposals/zyhive-improvements/INDEX.md`
- ZyHive P0 已落地：26.5.10v1 (logging / readyz / CI / self_schedule / budget brake / AdaptiveThrottle)
- aiteam 提议默认 off + experimental flag，零影响 ZyHive 主线
