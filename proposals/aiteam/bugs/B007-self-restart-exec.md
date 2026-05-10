# B007 · 自更新 `syscall.Exec` 信任 `os.Args` / `os.Environ`

> **严重度**: 🟢 LOW（false-positive 类，但保留为提醒）
> **状态**: 📝 已分析（建议：不修，加注释明确假设）
> **报告人**: aiteam QA pass (S1, 26.5.10v6)
> **工具**: `gosec` G702

---

## 影响

`internal/api/update_unix.go::selfRestart` 用 `syscall.Exec(binary, os.Args, os.Environ())` 原地替换进程。gosec G702 标记为 "Command injection via taint analysis" —— 推断 `os.Args` / `os.Environ` 是用户可控。

实际：
- `binary = os.Executable()` 来自内核 `/proc/self/exe`，trust 链来自启动者（systemd / 用户 shell）
- `os.Args[0]` 是同样的路径
- `os.Args[1:]` 是 systemd unit 的 `ExecStart=` 参数 — root 写入的，trust
- `os.Environ()` 是 systemd 设置的环境 — root 控制，trust

若攻击者已能修改 systemd unit 或 `/usr/local/bin/zyhive` 文件，他们 **已经是 root**，本路径不增加额外攻击面。

## 漏洞代码

`internal/api/update_unix.go::selfRestart`（line 22）：

```go
execErr := syscall.Exec(binary, os.Args, os.Environ())
```

## PoC

无可信 PoC（需要 root 才能让标记的"taint"生效）。

## 修复（可选）

在函数顶部加注释明确威胁模型，安抚 gosec：

```go
// selfRestart trusts os.Args / os.Environ which originate from the systemd
// unit / shell that started this process — both require root to modify.
// gosec G702 false positive: this is not user-controlled input.
//nolint:gosec // G702 — see comment above
execErr := syscall.Exec(binary, os.Args, os.Environ())
```

## 测试用例

无（行为不变）。

## 兼容性

无变化。

## 修复优先级

🟢 **不做 fix，仅加注释**。本 markdown 留作记录，供后续 audit 复查时不必重复调查。
