package sandbox

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// requiresUnix skips the test on platforms where our sandbox is degraded
// (no Setpgid / no rlimits / no signal forwarding).
func requiresUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("sandbox enforced semantics are only available on Linux / Darwin (have %s)", runtime.GOOS)
	}
}

func Test_AITeam_Sandbox_CleanExit(t *testing.T) {
	res, err := Run(context.Background(), Options{
		Command: "echo hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TimedOut {
		t.Fatalf("clean run should not time out")
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", res.ExitCode)
	}
	if !strings.Contains(res.CombinedOutput, "hello world") {
		t.Fatalf("expected 'hello world' in output, got %q", res.CombinedOutput)
	}
}

func Test_AITeam_Sandbox_WallClockKillsHang(t *testing.T) {
	requiresUnix(t)
	start := time.Now()
	res, err := Run(context.Background(), Options{
		Command: "sleep 30",
		Limits:  Limits{WallClock: 800 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("Run should not return system error on timeout: %v", err)
	}
	if !res.TimedOut {
		t.Fatalf("expected TimedOut=true, got %+v", res)
	}
	if res.KilledReason != "wall_clock" {
		t.Fatalf("expected KilledReason=wall_clock, got %q", res.KilledReason)
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("wall-clock kill took too long: %v", elapsed)
	}
}

func Test_AITeam_Sandbox_TmpHomeIsolated(t *testing.T) {
	requiresUnix(t)
	res, err := Run(context.Background(), Options{
		Command: `echo "$HOME"; ls "$HOME"; pwd`,
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d output=%q", res.ExitCode, res.CombinedOutput)
	}
	if !strings.Contains(res.CombinedOutput, "aiteam-exec-") {
		t.Fatalf("expected HOME to be aiteam-exec-* sandbox tmp dir, got: %q", res.CombinedOutput)
	}

	// And: the tmp dir is gone after Run returns.
	lines := strings.Split(res.CombinedOutput, "\n")
	if len(lines) == 0 {
		t.Fatal("expected output lines")
	}
	tmp := strings.TrimSpace(lines[0])
	if tmp == "" {
		t.Fatalf("could not parse HOME from output: %q", res.CombinedOutput)
	}
	if _, statErr := os.Stat(tmp); !os.IsNotExist(statErr) {
		t.Fatalf("tmp HOME should have been removed after Run; stat err=%v", statErr)
	}
}

func Test_AITeam_Sandbox_KillsForkChild(t *testing.T) {
	requiresUnix(t)
	// Spawn a detached child that writes a flag file then sleeps for a
	// long time. The sandbox must SIGKILL the entire process group on
	// wall-clock expiry; we verify by checking the grandchild PID is
	// gone shortly after Run returns.
	tmp := t.TempDir()
	flag := tmp + "/started"
	pidFile := tmp + "/pid"

	cmd := fmt.Sprintf(
		`(
			echo $$ > %q
			touch %q
			sleep 60
		) &
		# Wait for child to actually write its pid file before main exits.
		# Otherwise the wall-clock could fire before the grandchild is even
		# scheduled, producing a flaky test.
		for _ in 1 2 3 4 5 6 7 8 9 10; do
			[ -f %q ] && break
			sleep 0.1
		done
		# Sleep ourselves so the wall-clock has to kick in.
		sleep 30`,
		pidFile, flag, flag,
	)

	res, err := Run(context.Background(), Options{
		Command: cmd,
		Limits:  Limits{WallClock: 1500 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("run err: %v", err)
	}
	if !res.TimedOut {
		t.Fatalf("expected timeout, got: %+v", res)
	}

	// Give the kernel a moment to deliver SIGKILL to the grandchild.
	time.Sleep(300 * time.Millisecond)

	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Skipf("grandchild never started, can't verify kill: %v", err)
		return
	}
	pid := strings.TrimSpace(string(data))
	if pid == "" {
		t.Skip("no pid recorded")
	}

	// Probe /proc/{pid} on Linux, or `kill -0 pid` style on Darwin.
	stillAlive := false
	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/proc/" + pid); err == nil {
			stillAlive = true
		}
	} else {
		// Darwin: use `kill -0` via shell.
		probe, perr := Run(context.Background(), Options{
			Command: "kill -0 " + pid + " 2>/dev/null && echo alive || echo dead",
		})
		if perr == nil && strings.Contains(probe.CombinedOutput, "alive") {
			stillAlive = true
		}
	}
	if stillAlive {
		t.Fatalf("grandchild pid %s still alive after sandbox timeout — process group kill failed", pid)
	}
}

func Test_AITeam_Sandbox_RejectsEmptyCommand(t *testing.T) {
	_, err := Run(context.Background(), Options{Command: "   "})
	if err == nil || err != ErrCommandEmpty {
		t.Fatalf("expected ErrCommandEmpty, got %v", err)
	}
}

func Test_AITeam_Sandbox_OutputTruncation(t *testing.T) {
	// Generate ~2 MiB of output but cap at 4096 bytes.
	res, err := Run(context.Background(), Options{
		Command: "yes A | head -c 2000000",
		Limits:  Limits{MaxOutput: 4096, WallClock: 5 * time.Second},
	})
	if err != nil {
		t.Fatalf("run err: %v", err)
	}
	if !res.OutputTruncated {
		t.Fatalf("expected OutputTruncated=true, got %d bytes", len(res.CombinedOutput))
	}
	if got := len(res.CombinedOutput); got > 4096+128 /* small slack */ {
		t.Fatalf("output not truncated tight: got %d bytes", got)
	}
}

func Test_AITeam_Sandbox_NonzeroExitPropagates(t *testing.T) {
	res, err := Run(context.Background(), Options{
		Command: "exit 17",
	})
	if err != nil {
		t.Fatalf("run err: %v", err)
	}
	if res.TimedOut {
		t.Fatal("should not time out")
	}
	if res.ExitCode != 17 {
		t.Fatalf("expected exit 17, got %d", res.ExitCode)
	}
}

func Test_AITeam_Sandbox_FormatToolOutput(t *testing.T) {
	cases := []struct {
		name string
		in   *RunResult
		want string
	}{
		{
			name: "clean",
			in:   &RunResult{CombinedOutput: "hi\n", ExitCode: 0},
			want: "hi",
		},
		{
			name: "exit-with-output",
			in:   &RunResult{CombinedOutput: "boom\n", ExitCode: 1},
			want: "❌ Command exited with code 1.\n\nboom\n",
		},
		{
			name: "exit-without-output",
			in:   &RunResult{CombinedOutput: "", ExitCode: 2},
			want: "❌ Command exited with code 2 (no output).",
		},
		{
			name: "timeout",
			in:   &RunResult{CombinedOutput: "", TimedOut: true},
			want: "❌ Command timed out after 5s (no output).",
		},
	}
	for _, c := range cases {
		got := FormatToolOutput(c.in, 5*time.Second)
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}
