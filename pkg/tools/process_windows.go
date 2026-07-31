//go:build windows

package tools

import (
	"context"
	"os/exec"
	"strconv"
)

func prepareOwnedProcess(_ *exec.Cmd) {}

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
	if cmd != nil && cmd.Process != nil {
		_ = exec.Command(
			"taskkill",
			"/T",
			"/F",
			"/PID",
			strconv.Itoa(cmd.Process.Pid),
		).Run()
		_ = cmd.Process.Kill()
	}
}
