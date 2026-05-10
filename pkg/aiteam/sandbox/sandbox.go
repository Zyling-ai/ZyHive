// Package sandbox implements a lightweight, pure-Go execution sandbox for
// the aiteam experimental `exec` tool path (PR-007).
//
// Design philosophy:
//   - Zero external dependencies (no bwrap, firejail, chroot, container).
//     The build remains CGO_ENABLED=0 single-static-binary across
//     Linux/macOS for both amd64 and arm64. This sandbox is intentionally
//     a "weak hardening layer", not a true confinement boundary — its job
//     is to defeat the common accidental-runaway and prompt-injection
//     misuse cases (curl|bash, fork bombs, memory-exhaust loops) while
//     preserving the existing zero-config UX.
//   - Linux & macOS only (other GOOSes fall back to the non-sandboxed
//     path; behaviour identical to a build without aiteam). Encoded via
//     build tags in sandbox_unix.go / sandbox_other.go.
//
// Threat model addressed:
//   * runaway processes (CPU / wall-clock / RSS / fd)
//   * fork-and-detach bypass of context cancellation (kill process group)
//   * working-directory traversal & $HOME bleed-through (per-run temp HOME)
//   * env leak (sanitized + agent-configured env injected on top)
//
// NOT addressed (out of scope; need future PRs):
//   * filesystem confinement (call sites still see real /etc, /tmp, ...)
//   * syscall filtering (no seccomp; would need cgo or platform code)
//   * net policy (no firewall integration)
//
// All hardening is no-op when flags.SandboxEnabled() returns false.
package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Default limits. Conservative enough for normal `exec` workloads but tight
// enough to stop runaway loops / memory bombs. Adjustable per-call via the
// Limits struct.
const (
	DefaultWallClock = 120 * time.Second // matches handleBashWS legacy
	DefaultCPUTime   = 60 * time.Second  // CPU seconds (RLIMIT_CPU)
	DefaultRSSBytes  = 512 << 20         // 512 MiB
	DefaultFDLimit   = 1024
	DefaultMaxOutput = 1 << 20 // 1 MiB combined stdout+stderr (truncation)
)

// Limits captures the resource ceiling for a single sandboxed run.
// Zero values mean "use the default constant".
type Limits struct {
	WallClock time.Duration // wall-clock deadline (context timeout)
	CPUTime   time.Duration // RLIMIT_CPU
	RSSBytes  uint64        // RLIMIT_AS (address-space cap, approximates RSS)
	FDLimit   uint64        // RLIMIT_NOFILE
	MaxOutput int           // truncate combined output past this many bytes
}

// withDefaults returns a copy of l with all zero fields filled in.
func (l Limits) withDefaults() Limits {
	if l.WallClock <= 0 {
		l.WallClock = DefaultWallClock
	}
	if l.CPUTime <= 0 {
		l.CPUTime = DefaultCPUTime
	}
	if l.RSSBytes == 0 {
		l.RSSBytes = DefaultRSSBytes
	}
	if l.FDLimit == 0 {
		l.FDLimit = DefaultFDLimit
	}
	if l.MaxOutput <= 0 {
		l.MaxOutput = DefaultMaxOutput
	}
	return l
}

// RunResult holds the output of a sandboxed run.
//
// CombinedOutput is the merged stdout+stderr stream, truncated to
// Limits.MaxOutput; OutputTruncated is true when truncation happened.
type RunResult struct {
	CombinedOutput  string
	OutputTruncated bool
	ExitCode        int
	TimedOut        bool   // wall-clock fired before completion
	KilledReason    string // "wall_clock" | "cpu_time" | "rss" | "fd" | "" (clean exit)
	Duration        time.Duration
}

// Options configures a single Run.
type Options struct {
	Command string            // exact command line passed to bash -c
	WorkDir string            // working directory (sanitized by caller)
	Env     []string          // sanitized environment; appended after sandbox-managed entries
	Limits  Limits            // resource ceiling
	Extra   map[string]string // reserved for future fields
}

// ErrCommandEmpty is returned by Run when no command was supplied.
var ErrCommandEmpty = errors.New("sandbox: empty command")

// Run executes opts.Command under the sandbox. The parent context controls
// hard cancellation; the wall-clock timeout is derived from
// opts.Limits.WallClock.
//
// Behaviour summary:
//   * always uses bash -c
//   * a fresh temp directory is created and exported as HOME=<tmp>; it is
//     deleted on return regardless of outcome
//   * the child runs in its own process group (Setpgid=true); on
//     wall-clock or ctx cancellation, the entire group is signalled with
//     SIGKILL so detached fork children die too
//   * rlimits are applied via SysProcAttr when supported on the GOOS;
//     on unsupported platforms the limits are best-effort
//   * stdout+stderr are merged and truncated at Limits.MaxOutput
func Run(parent context.Context, opts Options) (*RunResult, error) {
	if strings.TrimSpace(opts.Command) == "" {
		return nil, ErrCommandEmpty
	}
	lim := opts.Limits.withDefaults()

	start := time.Now()

	// Per-run tmp HOME — keeps $HOME clean between runs and prevents one
	// run from reading another's bash history / state.
	tmpHome, err := os.MkdirTemp("", "aiteam-exec-")
	if err != nil {
		return nil, fmt.Errorf("sandbox: mktemp home: %w", err)
	}
	defer os.RemoveAll(tmpHome)

	ctx, cancel := context.WithTimeout(parent, lim.WallClock)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", opts.Command)
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	// Build env: caller-supplied entries first (with HOME / TMPDIR /
	// AITEAM_SANDBOX stripped), then sandbox-managed entries appended so
	// they win in last-write-wins exec semantics.
	stripped := stripReservedEnv(opts.Env)
	env := append([]string{}, stripped...)
	env = append(env,
		"HOME="+tmpHome,
		"TMPDIR="+tmpHome,
		"AITEAM_SANDBOX=1",
	)
	cmd.Env = env

	// SysProcAttr: build via OS-specific helper. On unsupported platforms
	// (windows/...) this is a no-op and limits are best-effort.
	applySysProcAttr(cmd, lim)

	// Capture output with a hard cap that protects against fork-bomb-style
	// output floods that would otherwise OOM us.
	var buf bytes.Buffer
	w := &cappedWriter{w: &buf, max: lim.MaxOutput}
	cmd.Stdout = w
	cmd.Stderr = w

	// We use Start/Wait instead of Run because we need to install the
	// process-group killer AFTER cmd.Process has been assigned (avoids
	// data race with the os/exec internal write to cmd.Process).
	if startErr := cmd.Start(); startErr != nil {
		return &RunResult{Duration: time.Since(start)}, startErr
	}

	// Process-group death signaller fires when the parent ctx fires; this
	// reaches detached forks that exec.CommandContext alone cannot kill.
	// Set up on Unix via configureProcessGroupKill; no-op elsewhere.
	// Safe to read cmd.Process here: Start has already returned.
	stopGroupKiller := configureProcessGroupKill(ctx, cmd)
	defer stopGroupKiller()

	runErr := cmd.Wait()

	res := &RunResult{
		CombinedOutput:  buf.String(),
		OutputTruncated: w.truncated,
		Duration:        time.Since(start),
	}

	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.KilledReason = "wall_clock"
	}

	if runErr != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(runErr, &exitErr):
			res.ExitCode = exitErr.ExitCode()
			// SIGXCPU == 24 on Linux, 24 on Darwin → CPU rlimit fired
			if exitErr.ExitCode() == -1 && exitErr.Sys() != nil {
				if reason := classifyKillReason(exitErr); reason != "" {
					res.KilledReason = reason
				}
			}
		default:
			// Non-exit error (couldn't start bash, etc.). Surface as-is.
			return res, runErr
		}
	}

	return res, nil
}

// FormatToolOutput converts a RunResult into the human-readable text format
// the existing handleBashWS path returns. Centralising it here keeps the
// integration in pkg/tools/registry.go thin.
func FormatToolOutput(res *RunResult, originalLimit time.Duration) string {
	if res == nil {
		return "(no result)"
	}
	out := res.CombinedOutput
	if res.OutputTruncated {
		out += fmt.Sprintf("\n…(output truncated; %d byte cap)", DefaultMaxOutput)
	}
	if res.TimedOut {
		if strings.TrimRight(out, "\n") != "" {
			return fmt.Sprintf("❌ Command timed out after %v.\n\nPartial output:\n%s", originalLimit, out)
		}
		return fmt.Sprintf("❌ Command timed out after %v (no output).", originalLimit)
	}
	if res.ExitCode != 0 {
		if strings.TrimRight(out, "\n") != "" {
			return fmt.Sprintf("❌ Command exited with code %d.\n\n%s", res.ExitCode, out)
		}
		return fmt.Sprintf("❌ Command exited with code %d (no output).", res.ExitCode)
	}
	if strings.TrimRight(out, "\n") == "" {
		return "(command completed successfully, no output)"
	}
	return strings.TrimRight(out, "\n")
}

// reservedEnvKeys lists env vars whose values are owned by the sandbox.
// stripReservedEnv removes any caller-supplied entries with these keys so
// the sandbox's own assignments are not overridden by the appended
// sandbox-managed env tail.
var reservedEnvKeys = map[string]bool{
	"HOME":           true,
	"TMPDIR":         true,
	"AITEAM_SANDBOX": true,
}

func stripReservedEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		if reservedEnvKeys[kv[:eq]] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// cappedWriter forwards bytes to an underlying writer but stops once max
// bytes are written. After that point Write reports success but discards
// data so the producer keeps making progress instead of blocking.
type cappedWriter struct {
	w         io.Writer
	max       int
	written   int
	truncated bool
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.written >= c.max {
		c.truncated = true
		return len(p), nil // pretend we accepted everything
	}
	remaining := c.max - c.written
	if len(p) <= remaining {
		n, err := c.w.Write(p)
		c.written += n
		return n, err
	}
	n, err := c.w.Write(p[:remaining])
	c.written += n
	if err == nil {
		c.truncated = true
		return len(p), nil
	}
	return n, err
}
