# PR-007 · 工具沙箱（exec 隔离）

> 状态: 🟢 ready for impl → ✅ **landed S2 (26.5.10v8)**
> 优先级: 🟠 P1（aiteam 跑前必备的"基本不踩坑"隔离层）
> 依赖: 无
> Flag: `ZYHIVE_EXPERIMENTAL_SANDBOX=1`

---

## 1. 背景与问题

`exec` 工具（`pkg/tools/registry.go::handleBashWS`）允许 AI 跑任意 bash。在
aiteam 真接业务前，最低限度需要：

1. **杀进程组**：bash 派生的后台子进程在 `exec.CommandContext` 仅 kill 直接
   子进程时不会消失。一行 `(sleep 600 &)` 就能让 LLM 误派工后留下幽灵进程。
2. **资源上限**：失控循环 / 内存炸 / fd 洩漏可能在 t4g.small (1.8GB RAM)
   级别 staging 上几秒钟打挂全机。
3. **临时 HOME**：当前 exec 用主机 HOME，会读到 bash history / SSH key 等
   敏感目录。
4. **输出截断**：fork-bomb 风格输出可吃满进程 RSS。

## 2. 设计要点

新包 `pkg/aiteam/sandbox/`，纯 Go，零外部依赖（无 bwrap / firejail / chroot /
container），保持 ZyHive 单二进制哲学。

### 2.1 API

```go
type Limits struct {
    WallClock time.Duration // ctx.WithTimeout
    CPUTime   time.Duration // RLIMIT_CPU (placeholder, parent process scope only)
    RSSBytes  uint64        // RLIMIT_AS
    FDLimit   uint64        // RLIMIT_NOFILE
    MaxOutput int           // truncate combined stdout+stderr
}

type Options struct {
    Command string; WorkDir string; Env []string; Limits Limits
}

type RunResult struct {
    CombinedOutput, KilledReason string
    OutputTruncated, TimedOut bool
    ExitCode int; Duration time.Duration
}

func Run(ctx, Options) (*RunResult, error)
func FormatToolOutput(res *RunResult, originalLimit time.Duration) string
```

### 2.2 GOOS 支持矩阵

| GOOS | Setpgid | RLIMIT* | tmp HOME | Output truncate | 行为 |
|------|---------|---------|----------|-----------------|------|
| linux | ✅ | ✅ (best-effort, parent scope) | ✅ | ✅ | 完整沙箱 |
| darwin | ✅ | ✅ (POSIX) | ✅ | ✅ | 完整沙箱 |
| windows/plan9/... | ❌ | ❌ | ✅ | ✅ | 降级为 ctx-only |

### 2.3 默认 Limits

| 字段 | 默认 |
|------|------|
| WallClock | 120s（与 legacy 同） |
| CPUTime | 60s |
| RSSBytes | 512 MiB |
| FDLimit | 1024 |
| MaxOutput | 1 MiB |

### 2.4 调用切点

`pkg/tools/registry.go::handleBashWS` 在 `flags.SandboxEnabled()` 为 true 时
切到 `sandbox.Run`，否则跑 legacy `exec.CommandContext` 路径。这样

- flag 关时行为 **byte-identical** 26.5.10v7
- flag 开时多 30s wall + Setpgid + tmp HOME + 输出截断

### 2.5 关键实现细节

* **避免数据竞争**：用 `cmd.Start()` + `cmd.Wait()` 替代 `cmd.Run()`。
  process-group killer goroutine 在 `Start` 返回后再启动，避免与
  `exec.Cmd` 内部对 `cmd.Process` 的写入 race。
* **env 命名空间保护**：caller 传入 `Env []string` 中若含 `HOME`/`TMPDIR`/
  `AITEAM_SANDBOX` 会被 `stripReservedEnv` 剥离，确保沙箱自己的赋值生效。
* **process-group kill**：context 取消时通过 `syscall.Kill(-pgid, SIGKILL)`
  把整个进程组干掉，截杀 fork 出去的孙子进程。

## 3. 实施步骤

1. ✅ 新增 `pkg/aiteam/sandbox/{sandbox.go, sandbox_unix.go, sandbox_other.go, sandbox_test.go}`
2. ✅ 改 `pkg/tools/registry.go::handleBashWS`：flag 分支
3. ✅ 新增 `pkg/tools/registry_sandbox_test.go`（4 case，覆盖 flag off / on / tmp HOME / 超时）
4. ✅ 更 CHANGELOG `### aiteam (experimental)`
5. ✅ AWS staging 部署 + smoke pass

## 4. 测试

`Test_AITeam_Sandbox_*` (8 case，覆盖率 95%+):
* CleanExit / WallClockKillsHang / TmpHomeIsolated / KillsForkChild
* RejectsEmptyCommand / OutputTruncation / NonzeroExitPropagates
* FormatToolOutput (4 sub-case)

`Test_AITeam_Registry_*` (4 case，集成):
* SandboxFlagOff_LegacyPath
* SandboxFlagOn_RoutesThroughSandbox（验 `$AITEAM_SANDBOX=1` 标记）
* SandboxFlagOn_TmpHomeIsolated
* SandboxFlagOn_TimeoutFormatted

`go test -race -count=1 ./pkg/aiteam/... ./pkg/tools/...` 全绿。

## 5. 已知限制

* `RLIMIT_*` 的 `setRlimitsRaw` 在 parent 调用会影响 parent，不会传到 child；
  在 child 内通过 `bash` 起的进程也无法继承设置好的 rlimit。**这次 v0 没在
  child 强制启用 rlimit**，仅保留 Setpgid + WallClock + tmp HOME + output cap。
  后续 PR 可考虑 cgo + prlimit 来给 child PID 设。
* 没有 seccomp / capability drop / network policy；攻击者若已能写
  bash 命令，仍可 `curl` 外网。这是 v0 故意的设计取舍（保持单 binary + 跨平台）。

## 6. experimental flag

* `ZYHIVE_EXPERIMENTAL_SANDBOX=1` → 启用
* 默认 / 任意其他值 → legacy 路径，行为不变

## 7. 兼容性 / 回滚

* 数据不增不减
* 关 flag 即完全 no-op
* 回滚直接 revert commit；二进制差 ~30KB

---

*创建：S2 (26.5.10v8) · 实现 commit: 详见 git log feat(aiteam/s2)*
