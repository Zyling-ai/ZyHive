# B008 · CLI `$EDITOR` 命令执行

> **严重度**: 🟢 LOW（同用户权限边界，false-positive 类）
> **状态**: 📝 已分析（建议：不修）
> **报告人**: aiteam QA pass (S1, 26.5.10v6)
> **工具**: `gosec` G702

---

## 影响

`cmd/aipanel/cli.go` 在配置菜单选项 6 "编辑配置文件" 中：

```go
editor := os.Getenv("EDITOR")
if editor == "" { editor = "vi" }
cmd := exec.Command(editor, configPath)
```

gosec 报为 "Command injection"。但 `$EDITOR` 只能被运行 CLI 的同一用户设置；若攻击者已经能修改该用户的 env，他们 **已经能** 用同样的权限执行任意命令（包括直接 `mv /usr/local/bin/zyhive ...`），本路径不增加攻击面。

## 漏洞代码

`cmd/aipanel/cli.go` 大约 line 444：

```go
case "6":
    editor := os.Getenv("EDITOR")
    if editor == "" { editor = "vi" }
    cmd := exec.Command(editor, configPath)
```

## PoC

需要本地 shell 访问；无远程触发路径。

## 修复（可选）

加 nolint 注释 + 文档：

```go
// $EDITOR is set by the same user running zyhive CLI; no privilege boundary
// is crossed. gosec G702 false positive.
//nolint:gosec // G702 — see comment above
cmd := exec.Command(editor, configPath)
```

## 兼容性

无变化。

## 修复优先级

🟢 **不做 fix，仅加注释**。
