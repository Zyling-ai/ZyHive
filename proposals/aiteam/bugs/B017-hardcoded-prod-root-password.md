# B017 · scripts/deploy-hive.sh hardcoded production root password

> **严重度**: 🔴 CRITICAL（生产服务器 root 密码明文在公共 repo）
> **状态**: ✅ 脚本已修 26.5.10v25 (P3-S6) — 强制 env var，无 fallback
> **报告人**: P3-S6 PAT 清理时连带发现

---

## 影响

`scripts/deploy-hive.sh` 第 17 行（修复前）：

```bash
PASSWORD="${HIVE_ROOT_PASS:-123ABCDabcd}"
```

`release.sh` 末尾的提示文案里也明文写了：

```bash
echo "   sshpass -p '123ABCDabcd' scp ... root@43.164.0.138:/tmp/..."
```

- 密码 `123ABCDabcd` 是 **生产服务器 hive.lilianbot.com (43.164.0.138) 的 root 密码**
- 公共 repo 任何 fork / 搜索都能直接 SSH root 拿下生产
- 危害远大于 B016（PAT 只能动 GitHub release；root 密码能动整台机器）

## 修复

1. **脚本侧**（已做）：
   - `deploy-hive.sh` 改为 `PASSWORD="${HIVE_ROOT_PASS:?...}"` — 没设环境变量直接 exit
   - `release.sh` 提示文案改用 `HIVE_ROOT_PASS=...` env 引用，去掉明文

2. **服务器侧**（仓库所有者必须立刻做）：
   - SSH 到 hive.lilianbot.com 修改 root 密码（`passwd root`）
   - 推荐：禁用 root 密码登录（`PermitRootLogin prohibit-password`），切到 SSH key
   - 检查 auth.log / journalctl 是否有可疑登录（密码泄漏期间）

## PoC

```bash
git clone https://github.com/Zyling-ai/ZyHive
grep 'sshpass -p' scripts/  # → 明文密码出现 2 次
sshpass -p '<password>' ssh root@43.164.0.138 'whoami'
# → root  (假设 sshd 仍允许密码登录)
```

## 兼容性

- 现有 CI/CD 必须迁到 SSH key 或在 secrets 中存 `HIVE_ROOT_PASS`
- 历史 commits 里密码仍可见 — git 重写历史风险大，优先保证密码失效

## 后续 (用户行动项)

- 🔴 立刻改服务器 root 密码
- 🟡 切到 SSH key auth
- 🟢 把新密码（或 key 私钥）存 GitHub Secret `HIVE_ROOT_PASS`
- 🟢 考虑走 GHSA disclosure 流程

仓库所有者动作之后这条 bug 才能从 CRITICAL 真正降到 LOW（git history 不再含活密码）。
