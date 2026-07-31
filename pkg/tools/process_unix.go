//go:build !windows

package tools

import (
	"context"
	"os/exec"
	"syscall"
)

func prepareOwnedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func watchOwnedProcessGroup(ctx context.Context, cmd *exec.Cmd) func() {
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			killOwnedProcessGroup(cmd)
		case <-stop:
		}
	}()
	return func() {
		select {
		case <-stop:
		default:
			close(stop)
		}
	}
}

func killOwnedProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil && pgid > 0 {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return
	}
	_ = cmd.Process.Kill()
}
