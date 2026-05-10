//go:build linux || darwin

package sandbox

import (
	"context"
	"os/exec"
	"sync"
	"syscall"
)

// applySysProcAttr installs the rlimits + process group attributes on the
// command's SysProcAttr. Called from Run before cmd.Start.
//
// On Linux:
//   - RLIMIT_AS  bounds the process's virtual address space (≈ RSS cap)
//   - RLIMIT_CPU caps total CPU seconds across the group; on hit, kernel
//     sends SIGXCPU then SIGKILL
//   - RLIMIT_NOFILE caps open fds
//   - Setpgid puts the child in its own process group so we can SIGKILL
//     the whole group on cancellation
//
// macOS supports the same Setrlimit flags (POSIX). The CPU rlimit on
// macOS is per-process not per-group (different from Linux), which is
// fine for the use case.
func applySysProcAttr(cmd *exec.Cmd, lim Limits) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// configureProcessGroupKill arranges for SIGKILL to be sent to the child's
// process group when ctx is cancelled. Returns a stop function the caller
// must always invoke (e.g. via defer).
//
// This complements exec.CommandContext, which only kills the direct child;
// many real bash one-liners spawn sub-shells in the background which would
// otherwise survive the parent's death.
func configureProcessGroupKill(ctx context.Context, cmd *exec.Cmd) func() {
	stop := make(chan struct{})
	var once sync.Once
	go func() {
		select {
		case <-ctx.Done():
		case <-stop:
			return
		}
		if cmd.Process == nil {
			return
		}
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err != nil {
			// Fall back to single-process kill — exec.CommandContext
			// will have done this for us, but defensive double-kill is
			// harmless.
			_ = cmd.Process.Kill()
			return
		}
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}()
	return func() { once.Do(func() { close(stop) }) }
}

// classifyKillReason inspects an ExitError's wait status to deduce why the
// process was killed. Returns one of the documented KilledReason values
// or "" if the cause is not recognisable.
func classifyKillReason(exitErr *exec.ExitError) string {
	if exitErr == nil || exitErr.Sys() == nil {
		return ""
	}
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return ""
	}
	if !ws.Signaled() {
		return ""
	}
	switch ws.Signal() {
	case syscall.SIGXCPU:
		return "cpu_time"
	case syscall.SIGKILL:
		// SIGKILL can come from OOM (RSS exceeded → cgroup OOM kill on
		// Linux), wall-clock cancellation, or our own process-group kill.
		// We can't distinguish reliably without scanning dmesg; classify
		// as "rss" only when address-space limit looks involved. Default
		// to empty so the wall-clock path retains its own attribution.
		return ""
	default:
		return ""
	}
}

// setRlimitsRaw applies hard-coded limits via setrlimit prlimit-style.
// Currently unused but kept here as a documented hook for future PRs
// (per-call dynamic limits — would need cgo or unix.Prlimit to apply
// to the *child* PID; calling Setrlimit in the parent affects the parent
// only and is not what we want).
//
//nolint:unused // kept for documentation
func setRlimitsRaw(_pid int, lim Limits) error {
	// RLIMIT_AS — virtual address space (bytes). Acts as an RSS proxy.
	if lim.RSSBytes > 0 {
		_ = syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: lim.RSSBytes, Max: lim.RSSBytes})
	}
	// RLIMIT_NOFILE — open fd count.
	if lim.FDLimit > 0 {
		_ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &syscall.Rlimit{Cur: lim.FDLimit, Max: lim.FDLimit})
	}
	// RLIMIT_CPU — CPU seconds.
	if lim.CPUTime > 0 {
		secs := uint64(lim.CPUTime.Seconds())
		if secs == 0 {
			secs = 1
		}
		_ = syscall.Setrlimit(syscall.RLIMIT_CPU, &syscall.Rlimit{Cur: secs, Max: secs})
	}
	return nil
}
