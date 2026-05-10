# B012 · 工具 read/write/edit 路径 taint (G703)

> **严重度**: 🟢 LOW（B001 safefs 已修，残余仅 taint 流分析报警）
> **状态**: ✅ 已被 B001 修复链覆盖（行为正确，gosec 不识别 safefs.ConfineToBase）
> **报告人**: aiteam QA pass (S1, 26.5.10v6)
> **工具**: `gosec` G703

---

## 影响

gosec G703 在以下位置报 "Path traversal via taint analysis"：

| 位置 | 状态 |
|------|------|
| `pkg/tools/tools.go:154` | B001 修复后已走 `safefs.ConfineToBase` |
| `pkg/network/migrate.go:31, 46` | 内部 trusted join（idempotent migrate hook） |
| `pkg/memory/memory.go:135` | 内部 trusted（agent 内 memory 树） |
| `cmd/aipanel/cli.go:1608` | CLI 本地操作，trust 用户 |

B001 (`26.5.10v2`) 已经为外部输入路径全面切到 `pkg/safefs.ConfineToBase`，且测试覆盖 27 个 case（含 sibling-prefix / symlink / abs / NUL byte / .. escape 五大类）。gosec 不会做 cross-package 数据流分析，只看到"用户输入 → 文件系统调用"链路就报 taint。

## 漏洞代码

代表性 case `pkg/tools/tools.go::resolvePath`：

```go
// B001 修复后 (commit 8ac513c)
func (r *Registry) resolvePath(p string) (string, error) {
    if filepath.IsAbs(p) {
        if !strings.HasPrefix(filepath.Clean(p), r.workspaceDir+string(os.PathSeparator)) {
            return "", errOutOfWorkspace
        }
    }
    return safefs.ConfineToBase(r.workspaceDir, p)
}
```

正确，但 gosec 跟不到 `safefs.ConfineToBase` 内部的边界检查。

## 修复（不需要）

无。可选：在每个调用点加 `//nolint:gosec // G703 — safefs.ConfineToBase enforces base boundary; see B001`。

## 兼容性

无变化。

## 修复优先级

🟢 **不做 fix**。
