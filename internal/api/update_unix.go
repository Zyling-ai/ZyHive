//go:build !windows

package api

import (
	"log"
	"os"
	"syscall"
)

// selfRestart 在 Unix/Linux/macOS 上发送 SIGTERM 给自身，
// 由 systemd / launchd 负责重启新版本。
func selfRestart() {
	log.Printf("[update] sending SIGTERM to self (pid=%d)", os.Getpid())
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
}
