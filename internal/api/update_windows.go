//go:build windows

package api

import (
	"log"
	"os"
)

// selfRestart 在 Windows 上直接退出，
// 由 sc.exe 服务管理器（RestartService）或手动重启。
func selfRestart() {
	log.Println("[update] exiting for restart (Windows service manager will restart)")
	os.Exit(0)
}
