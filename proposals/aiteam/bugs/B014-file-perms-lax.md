# B014 · 文件 / 目录权限偏宽（0644 / 0755 / 0666）

> **严重度**: 🟡 MEDIUM（敏感文件如 `aipanel.json` / `network/contacts/` 应 0600）
> **状态**: 🟡 调研中
> **报告人**: aiteam QA pass (S1, 26.5.10v6)
> **工具**: `gosec` G301 / G302 / G306（108 处合计）

---

## 影响

仓库内大量 `os.WriteFile / os.MkdirAll` 使用 `0644` / `0755` / 默认 `0666`，包括：

| 类别 | 例 | 风险 |
|------|---|------|
| 配置文件 | `aipanel.json` (含 Provider API key、auth.token) | 同机其他用户可读 → token 泄漏 |
| 网络通讯录 | `workspace/{agent}/network/contacts/*.md` | 同上 |
| 会话 JSONL | `<dataDir>/sessions/*.jsonl`（可能含用户输入隐私） | 同上 |
| Cron jobs.json | `<dataDir>/cron/jobs.json` | 含 cron payload |

在单租户机器（普通 ZyHive 使用场景）这通常不是问题；但：
- 部署在共享服务器 / VPS（多用户 SSH）会泄漏密钥
- 备份脚本若打 tar 也会保留宽权限

## 漏洞代码

108 处 G301/G302/G306 警告（详见 `_qa-pass-26.5.10v6-gosec.json`）。代表性：

```go
// pkg/config/config.go
os.WriteFile(path, data, 0644)   // ⚠️ 应 0600（含 token / API key）

// pkg/network/store.go
os.MkdirAll(dir, 0755)            // ⚠️ 应 0700（含联系人 PII）
```

## PoC

```bash
# 多用户机器:
$ ls -l /etc/zyhive/zyhive.json
-rw-r--r-- 1 root root 290 May  9 23:07 /etc/zyhive/zyhive.json
$ cat /etc/zyhive/zyhive.json   # ← 普通用户读得到 token
```

## 修复

按敏感度分组修：

| 路径模式 | 推荐权限 |
|---------|---------|
| `aipanel.json` / `zyhive.json` | 0600 (file) |
| `workspace/*/SOUL.md`, `IDENTITY.md` | 0640 |
| `workspace/*/network/**` | 0700 (dir) + 0600 (file) |
| `<dataDir>/sessions/**` | 0700 + 0600 |
| `cmd/aipanel/ui_dist/**` | 0644（embedded static OK，无敏感） |
| 临时文件 / 升级下载 | 0600 |

每修改 `os.WriteFile`/`MkdirAll` 调用点要：
1. 用 `safefs` 风格 wrapper `safefs.WriteSecret(path, data)` 集中化（避免每处独立 0600/0700）
2. 加单元测试：写完后 `os.Stat` 验权限位

## 测试用例

`TestSafefs_WriteSecret_HasModeRequired`：写文件后断言 `info.Mode().Perm() == 0o600`。

## 兼容性

- 升级后老用户的现有文件 mode **不会自动调整**（避免修改用户文件元数据）
- 新写文件用 secure mode
- 启动时可选：检查关键文件 mode 偏宽 → log warning（不强改）

## 修复优先级

🟡 **下一程 (S2-S4) 拆分一个独立 commit 修核心 3 处（config / network / sessions），其余渐进**
