//go:build !windows

package api

import (
	"log"
	"os"
	"os/exec"
	"syscall"
)

// selfRestart 在 Unix/Linux/macOS 上触发服务重启。
// 策略：先尝试 syscall.Exec（用新二进制原地替换进程，PID 不变，零停机）；
// 若 exec 失败（罕见情况）则发 SIGTERM，由 systemd/launchd 用 Restart=always 重启。
// 注意：调用方（runUpdate）已确保新二进制文件替换完毕。
func selfRestart() {
	// 优先：exec 新二进制替换自身 —— PID 不变，不依赖 systemd Restart 策略
	binary, err := os.Executable()
	if err == nil {
		log.Printf("[update] exec-replacing self with new binary: %s (pid=%d)", binary, os.Getpid())
		os.Stdout.Sync()
		os.Stderr.Sync()
		execErr := syscall.Exec(binary, os.Args, os.Environ())
		if execErr == nil {
			return // 不会到这里：exec 成功则直接变身为新进程
		}
		log.Printf("[update] exec failed (%v), falling back to SIGTERM", execErr)
	}
	// 后备：SIGTERM（要求 systemd 配置 Restart=always）
	log.Printf("[update] sending SIGTERM to self (pid=%d)", os.Getpid())
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
}

func startUpdateWatchdog(backupPath, token string) error {
	cmd := exec.Command(backupPath, updateWatchdogCommand, token)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func terminateUpdatePID(pid int) error {
	err := syscall.Kill(pid, syscall.SIGTERM)
	if err == syscall.ESRCH {
		return nil
	}
	return err
}
