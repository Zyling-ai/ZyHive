# B016 · scripts/release.sh hardcoded GitHub PAT

> **严重度**: 🟠 HIGH（任何能看公共 repo 的人都拿到了仓库写权限的 PAT）
> **状态**: ✅ 已修 26.5.10v25 (P3-S6) — PAT 已 REVOKE + 脚本改为强制 env var
> **报告人**: 在 26.5.10v24 release 跑 release.sh 时遇到 401，发现 PAT 已失效，回查发现被硬编码

---

## 影响

`scripts/release.sh` 第 14 行长期硬编码：

```bash
GITHUB_TOKEN="${GITHUB_TOKEN:-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}"
```

- 这是个 GitHub Fine-grained PAT
- 公共 repo `Zyling-ai/ZyHive` → 全网可读
- PAT 显然带 release 写权限（不然 release.sh 跑不通）
- 任何 fork 该 repo 的人 / GitHub 搜 `github_pat_` 的人 / 历史 commit 看官都拿到

## 漏洞代码

`scripts/release.sh` line 14（被本次修复替换）。

## PoC

```bash
git clone https://github.com/Zyling-ai/ZyHive
grep -r 'github_pat_' .  # → 命中 release.sh，token 明文
```

## 修复

`scripts/release.sh` 改为 fail-fast：

```bash
if [ -z "${GITHUB_TOKEN}" ]; then
  if command -v gh >/dev/null 2>&1; then
    GITHUB_TOKEN=$(gh auth token 2>/dev/null || true)
  fi
fi
if [ -z "${GITHUB_TOKEN}" ]; then
  echo "❌ GITHUB_TOKEN unset" >&2
  exit 1
fi
```

同时**仓库所有者必须立刻在 GitHub UI 把那个 PAT revoke**（即使无效也建议删干净；
对外形象重要）。

## 测试

下次跑 release.sh 必须先 export GITHUB_TOKEN 或 `gh auth login` —
没有 token 直接 exit 1 不会泄漏后续动作。

## 兼容性

- 跑 release 必须用 env var 或 gh CLI — 但 CI workflow 已经用
  `${{ secrets.GITHUB_TOKEN }}`，本地手动跑加一行 export 即可
- git 历史里仍能查到 PAT（已 revoked），可选用 `git filter-repo` 重写历史，
  但 GitHub 公仓 force push 风险大；优先保证 PAT 失效

## 后续

- ✅ 仓库所有者 revoke 旧 PAT (cursor agent 无法做)
- ✅ 下次发版用新 token（要么 gh CLI 自动给，要么 GitHub Secrets）
