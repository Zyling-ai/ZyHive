package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	updateWatchdogCommand = "__update-watchdog"
	updatePendingSuffix   = ".update-pending.json"
	updateResultSuffix    = ".update-result.json"
	updateHealthTimeout   = 45 * time.Second
	updateHealthInterval  = 500 * time.Millisecond
)

type pendingUpdate struct {
	Token           string    `json:"token"`
	OldVersion      string    `json:"oldVersion"`
	ExpectedVersion string    `json:"expectedVersion"`
	BinaryPath      string    `json:"binaryPath"`
	BackupPath      string    `json:"backupPath"`
	HealthURL       string    `json:"healthUrl"`
	PID             int       `json:"pid,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

type updateResult struct {
	Stage      UpdateStage `json:"stage"`
	OldVersion string      `json:"oldVersion"`
	NewVersion string      `json:"newVersion"`
	Message    string      `json:"message"`
	CreatedAt  time.Time   `json:"createdAt"`
}

func updateHealthURL(c *gin.Context, fallbackPort int) string {
	if fallbackPort < 1 || fallbackPort > 65535 {
		fallbackPort = 8080
	}
	hostPort := net.JoinHostPort("127.0.0.1", strconv.Itoa(fallbackPort))
	if c != nil && c.Request != nil {
		if addr, ok := c.Request.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && addr != nil {
			host, port, err := net.SplitHostPort(addr.String())
			if err == nil && port != "" {
				ip := net.ParseIP(strings.Trim(host, "[]"))
				if host == "" || (ip != nil && ip.IsUnspecified()) {
					host = "127.0.0.1"
				}
				hostPort = net.JoinHostPort(host, port)
			}
		}
	}
	return (&url.URL{Scheme: "http", Host: hostPort, Path: "/healthz"}).String()
}

func pendingUpdatePath(binaryPath string) string {
	return binaryPath + updatePendingSuffix
}

func updateResultPath(binaryPath string) string {
	return binaryPath + updateResultSuffix
}

func preparePendingUpdate(binaryPath, backupPath, healthURL, oldVersion, targetVersion string) (*pendingUpdate, error) {
	binaryPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("resolve binary path: %w", err)
	}
	backupPath, err = filepath.Abs(backupPath)
	if err != nil {
		return nil, fmt.Errorf("resolve backup path: %w", err)
	}
	binaryPath = filepath.Clean(binaryPath)
	backupPath = filepath.Clean(backupPath)
	if backupPath != binaryPath+".bak" {
		return nil, fmt.Errorf("backup path must be binary path plus .bak")
	}
	if !releaseVersionPattern.MatchString(targetVersion) {
		return nil, fmt.Errorf("invalid expected version %q", targetVersion)
	}
	parsedHealth, err := url.Parse(healthURL)
	if err != nil || parsedHealth.Scheme != "http" || parsedHealth.Host == "" || parsedHealth.Path != "/healthz" {
		return nil, fmt.Errorf("invalid local health URL")
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate watchdog token: %w", err)
	}
	record := &pendingUpdate{
		Token:           hex.EncodeToString(tokenBytes),
		OldVersion:      oldVersion,
		ExpectedVersion: targetVersion,
		BinaryPath:      binaryPath,
		BackupPath:      backupPath,
		HealthURL:       parsedHealth.String(),
		PID:             os.Getpid(),
		CreatedAt:       time.Now().UTC(),
	}
	if err := writeJSONAtomic(pendingUpdatePath(binaryPath), record); err != nil {
		return nil, err
	}
	return record, nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func readPendingUpdate(path string) (*pendingUpdate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record pendingUpdate
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func validatePendingUpdate(record *pendingUpdate, executablePath, token string, watchdog bool) error {
	if record == nil {
		return fmt.Errorf("missing pending update")
	}
	binaryPath, err := filepath.Abs(record.BinaryPath)
	if err != nil {
		return err
	}
	backupPath, err := filepath.Abs(record.BackupPath)
	if err != nil {
		return err
	}
	executablePath, err = filepath.Abs(executablePath)
	if err != nil {
		return err
	}
	binaryPath = filepath.Clean(binaryPath)
	backupPath = filepath.Clean(backupPath)
	executablePath = filepath.Clean(executablePath)
	if backupPath != binaryPath+".bak" {
		return fmt.Errorf("invalid backup relationship")
	}
	expectedExecutable := binaryPath
	if watchdog {
		expectedExecutable = backupPath
	}
	if executablePath != expectedExecutable {
		return fmt.Errorf("watchdog executable path mismatch")
	}
	if !releaseVersionPattern.MatchString(record.ExpectedVersion) {
		return fmt.Errorf("invalid expected version")
	}
	if token != "" && (len(token) != len(record.Token) ||
		subtle.ConstantTimeCompare([]byte(token), []byte(record.Token)) != 1) {
		return fmt.Errorf("invalid watchdog token")
	}
	health, err := url.Parse(record.HealthURL)
	if err != nil || health.Scheme != "http" || health.Host == "" || health.Path != "/healthz" {
		return fmt.Errorf("invalid health URL")
	}
	return nil
}

// RunUpdateWatchdogCommand 处理内部独立守护进程命令。
// 命令只接受一次性令牌；文件路径来自权限为 0600 的待确认记录。
func RunUpdateWatchdogCommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != updateWatchdogCommand {
		return false, nil
	}
	if len(args) != 2 {
		return true, fmt.Errorf("invalid watchdog arguments")
	}
	executablePath, err := os.Executable()
	if err != nil {
		return true, err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executablePath); resolveErr == nil {
		executablePath = resolved
	}
	if !strings.HasSuffix(executablePath, ".bak") {
		return true, fmt.Errorf("watchdog must run from the backup binary")
	}
	binaryPath := strings.TrimSuffix(executablePath, ".bak")
	record, err := readPendingUpdate(pendingUpdatePath(binaryPath))
	if err != nil {
		return true, err
	}
	if err := validatePendingUpdate(record, executablePath, args[1], true); err != nil {
		return true, err
	}
	return true, runUpdateWatchdog(record)
}

func runUpdateWatchdog(record *pendingUpdate) error {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
	timeout := updateHealthTimeout
	interval := updateHealthInterval
	if os.Getenv("ZYHIVE_RELEASE_E2E") == "1" {
		if parsed, err := time.ParseDuration(os.Getenv("ZYHIVE_UPDATE_HEALTH_TIMEOUT")); err == nil &&
			parsed >= 100*time.Millisecond && parsed <= 5*time.Second {
			timeout = parsed
			interval = 50 * time.Millisecond
		}
	}
	return runUpdateWatchdogWithConfig(record, timeout, interval, client)
}

func runUpdateWatchdogWithConfig(record *pendingUpdate, timeout, interval time.Duration, client *http.Client) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := checkUpdateHealth(client, record.HealthURL, record.ExpectedVersion); err == nil {
			log.Printf("[update-watchdog] version %s is healthy", record.ExpectedVersion)
			return os.Remove(pendingUpdatePath(record.BinaryPath))
		} else {
			lastErr = err
		}
		time.Sleep(interval)
	}

	latest, err := readPendingUpdate(pendingUpdatePath(record.BinaryPath))
	if err == nil && validatePendingUpdate(latest, record.BackupPath, record.Token, true) == nil {
		record = latest
	}
	reason := fmt.Sprintf("新版本 %s 健康检查超时，已自动恢复 %s", record.ExpectedVersion, record.OldVersion)
	if lastErr != nil {
		reason += "：" + lastErr.Error()
	}
	if err := restoreBackupBinary(record); err != nil {
		return fmt.Errorf("health check failed (%v), rollback failed: %w", lastErr, err)
	}
	result := updateResult{
		Stage:      StageRolledBack,
		OldVersion: record.OldVersion,
		NewVersion: record.ExpectedVersion,
		Message:    reason,
		CreatedAt:  time.Now().UTC(),
	}
	if err := writeJSONAtomic(updateResultPath(record.BinaryPath), &result); err != nil {
		log.Printf("[update-watchdog] write rollback result: %v", err)
	}
	_ = os.Remove(pendingUpdatePath(record.BinaryPath))
	if record.PID > 1 {
		if err := terminateUpdatePID(record.PID); err != nil {
			log.Printf("[update-watchdog] terminate failed process %d: %v", record.PID, err)
		}
	}
	log.Printf("[update-watchdog] %s", reason)
	return nil
}

func checkUpdateHealth(client *http.Client, healthURL, expectedVersion string) error {
	resp, err := client.Get(healthURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health HTTP %d", resp.StatusCode)
	}
	var data struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&data); err != nil {
		return err
	}
	if data.Status != "ok" {
		return fmt.Errorf("health status %q", data.Status)
	}
	if data.Version != expectedVersion {
		return fmt.Errorf("health version %q, expected %q", data.Version, expectedVersion)
	}
	return nil
}

func restoreBackupBinary(record *pendingUpdate) error {
	if record == nil || filepath.Clean(record.BackupPath) != filepath.Clean(record.BinaryPath)+".bak" {
		return fmt.Errorf("invalid rollback paths")
	}
	stagedPath := filepath.Join(filepath.Dir(record.BinaryPath), "."+filepath.Base(record.BinaryPath)+".rollback")
	defer os.Remove(stagedPath)
	if err := copyFile(record.BackupPath, stagedPath); err != nil {
		return err
	}
	if err := os.Chmod(stagedPath, 0755); err != nil {
		return err
	}
	return os.Rename(stagedPath, record.BinaryPath)
}

// ResumePendingUpdate 登记重启后的进程 PID，并恢复跨进程更新状态。
func ResumePendingUpdate(version string) {
	binaryPath, err := os.Executable()
	if err != nil {
		return
	}
	if resolved, resolveErr := filepath.EvalSymlinks(binaryPath); resolveErr == nil {
		binaryPath = resolved
	}

	var result updateResult
	if data, readErr := os.ReadFile(updateResultPath(binaryPath)); readErr == nil &&
		json.Unmarshal(data, &result) == nil && result.Stage == StageRolledBack {
		globalUpdateStatus.mu.Lock()
		globalUpdateStatus.Stage = StageRolledBack
		globalUpdateStatus.Progress = 100
		globalUpdateStatus.Message = result.Message
		globalUpdateStatus.OldVer = result.OldVersion
		globalUpdateStatus.NewVer = result.NewVersion
		globalUpdateStatus.UpdatedAt = result.CreatedAt
		globalUpdateStatus.mu.Unlock()
		_ = os.Remove(updateResultPath(binaryPath))
	}

	path := pendingUpdatePath(binaryPath)
	record, err := readPendingUpdate(path)
	if err != nil || record.ExpectedVersion != version ||
		validatePendingUpdate(record, binaryPath, "", false) != nil {
		return
	}
	record.PID = os.Getpid()
	if err := writeJSONAtomic(path, record); err != nil {
		log.Printf("[update] record restarted process PID: %v", err)
		return
	}
	globalUpdateStatus.mu.Lock()
	globalUpdateStatus.Stage = StageApplying
	globalUpdateStatus.Progress = 98
	globalUpdateStatus.Message = "新版本已启动，等待健康检查确认…"
	globalUpdateStatus.OldVer = record.OldVersion
	globalUpdateStatus.NewVer = record.ExpectedVersion
	globalUpdateStatus.UpdatedAt = time.Now()
	globalUpdateStatus.mu.Unlock()

	go func() {
		deadline := time.Now().Add(updateHealthTimeout + 5*time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				globalUpdateStatus.set(StageDone, 100, "升级成功，新版本已通过健康检查（"+version+"）")
				return
			}
			time.Sleep(updateHealthInterval)
		}
	}()
}
