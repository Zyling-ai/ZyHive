// internal/api/update.go — 版本检查与在线升级 API
// 升级流程：下载新二进制 → 验证 → 备份旧版 → rm -f → cp → SIGTERM 重启
// 用户数据（agents 目录、配置文件）全程不涉及，仅替换可执行文件本身。
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

// ── 更新状态 ─────────────────────────────────────────────────────────────────

type UpdateStage string

const (
	StageIdle        UpdateStage = "idle"
	StageDownloading UpdateStage = "downloading"
	StageVerifying   UpdateStage = "verifying"
	StageApplying    UpdateStage = "applying"
	StageDone        UpdateStage = "done"
	StageFailed      UpdateStage = "failed"
	StageRolledBack  UpdateStage = "rolledback"
)

type updateStatus struct {
	mu        sync.RWMutex
	Stage     UpdateStage `json:"stage"`
	Progress  int         `json:"progress"`  // 0-100
	Message   string      `json:"message"`
	OldVer    string      `json:"oldVersion"`
	NewVer    string      `json:"newVersion"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

func (s *updateStatus) set(stage UpdateStage, progress int, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Stage = stage
	s.Progress = progress
	s.Message = msg
	s.UpdatedAt = time.Now()
}

func (s *updateStatus) snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{
		"stage":      s.Stage,
		"progress":   s.Progress,
		"message":    s.Message,
		"oldVersion": s.OldVer,
		"newVersion": s.NewVer,
		"updatedAt":  s.UpdatedAt,
	}
}

// 全局单例——同一时刻只允许一个升级任务
var globalUpdateStatus = &updateStatus{Stage: StageIdle}

// ── handler ───────────────────────────────────────────────────────────────────

type updateHandler struct{}

// GET /api/update/check
// 返回 {current, latest, hasUpdate, releaseUrl}
func (h *updateHandler) Check(c *gin.Context) {
	latest, releaseURL, err := fetchLatestRelease()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "无法连接 GitHub：" + err.Error()})
		return
	}
	current := AppVersion
	c.JSON(http.StatusOK, gin.H{
		"current":    current,
		"latest":     latest,
		"hasUpdate":  latest != current && latest != "",
		"releaseUrl": releaseURL,
	})
}

// GET /api/update/status
// 返回当前升级任务状态（前端轮询）
func (h *updateHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, globalUpdateStatus.snapshot())
}

// POST /api/update/apply
// 触发异步升级；已有任务进行中返回 409
func (h *updateHandler) Apply(c *gin.Context) {
	globalUpdateStatus.mu.Lock()
	if globalUpdateStatus.Stage != StageIdle &&
		globalUpdateStatus.Stage != StageDone &&
		globalUpdateStatus.Stage != StageFailed &&
		globalUpdateStatus.Stage != StageRolledBack {
		globalUpdateStatus.mu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": "升级任务正在进行中，请稍候"})
		return
	}
	globalUpdateStatus.mu.Unlock()

	// 获取目标版本（可选，默认用最新）
	var body struct {
		Version string `json:"version"`
	}
	c.ShouldBindJSON(&body)

	go runUpdate(body.Version)
	c.JSON(http.StatusAccepted, gin.H{"message": "升级任务已启动，请轮询 /api/update/status 查看进度"})
}

// ── 核心升级逻辑 ──────────────────────────────────────────────────────────────

func runUpdate(targetVersion string) {
	s := globalUpdateStatus
	s.OldVer = AppVersion

	// 1. 确定目标版本
	s.set(StageDownloading, 5, "正在查询最新版本…")
	if targetVersion == "" {
		latest, _, err := fetchLatestRelease()
		if err != nil {
			s.set(StageFailed, 0, "查询版本失败："+err.Error())
			return
		}
		targetVersion = latest
	}
	s.NewVer = targetVersion

	if targetVersion == AppVersion {
		s.set(StageDone, 100, "当前已是最新版本（"+AppVersion+"），无需升级")
		return
	}

	// 2. 构建下载 URL
	osName := runtime.GOOS   // linux / darwin / windows
	arch := runtime.GOARCH   // amd64 / arm64
	suffix := ""
	if osName == "windows" {
		suffix = ".exe"
	}
	url := fmt.Sprintf(
		"https://github.com/Zyling-ai/zyhive/releases/download/%s/aipanel-%s-%s%s",
		targetVersion, osName, arch, suffix,
	)

	// 3. 下载到临时文件
	tmpPath := fmt.Sprintf("/tmp/zyhive-new-%s%s", targetVersion, suffix)
	if osName == "windows" {
		tmpPath = os.TempDir() + "\\zyhive-new-" + targetVersion + suffix
	}
	s.set(StageDownloading, 10, "正在下载 "+url)
	log.Printf("[update] downloading %s → %s", url, tmpPath)

	if err := downloadFile(url, tmpPath, func(pct int) {
		s.set(StageDownloading, 10+pct*60/100, fmt.Sprintf("下载中… %d%%", pct))
	}); err != nil {
		s.set(StageFailed, 0, "下载失败："+err.Error())
		os.Remove(tmpPath)
		return
	}

	// 4. 验证：运行 --version 检查可执行性
	s.set(StageVerifying, 72, "验证新版本…")
	if osName != "windows" {
		os.Chmod(tmpPath, 0755)
	}
	out, err := exec.Command(tmpPath, "--version").Output()
	if err != nil {
		s.set(StageFailed, 0, "新版本验证失败："+err.Error())
		os.Remove(tmpPath)
		return
	}
	detectedVer := strings.TrimSpace(string(out))
	log.Printf("[update] verified new binary: %s", detectedVer)

	// 5. 备份旧二进制
	s.set(StageApplying, 80, "备份旧版本…")
	binaryPath, err := os.Executable()
	if err != nil {
		s.set(StageFailed, 0, "无法获取当前二进制路径："+err.Error())
		os.Remove(tmpPath)
		return
	}
	backupPath := binaryPath + ".bak"
	if err := copyFile(binaryPath, backupPath); err != nil {
		log.Printf("[update] backup warning: %v", err)
		// 备份失败不阻断升级，只警告
	}

	// 6. rm -f 旧二进制，cp 新二进制（避免 Text file busy）
	s.set(StageApplying, 88, "替换二进制文件…")
	log.Printf("[update] replacing binary: %s → %s", tmpPath, binaryPath)
	if err := os.Remove(binaryPath); err != nil {
		s.set(StageFailed, 0, "删除旧二进制失败："+err.Error())
		os.Remove(tmpPath)
		return
	}
	if err := copyFile(tmpPath, binaryPath); err != nil {
		// 替换失败 → 回滚
		log.Printf("[update] copy failed, rolling back: %v", err)
		if rb := copyFile(backupPath, binaryPath); rb == nil {
			s.set(StageRolledBack, 0, "替换失败，已回滚到旧版本："+err.Error())
		} else {
			s.set(StageFailed, 0, "替换失败且回滚也失败，请手动恢复："+backupPath)
		}
		os.Remove(tmpPath)
		return
	}
	os.Chmod(binaryPath, 0755)
	os.Remove(tmpPath)

	// 7. 标记完成，发 SIGTERM 让 systemd/launchd 重启服务
	// 用户数据（agents dir / config）完全不涉及，进程重启后新版本自动加载
	s.set(StageDone, 100, "升级成功！正在重启服务…（新版本："+targetVersion+"）")
	log.Printf("[update] upgrade complete → %s，sending SIGTERM to self", targetVersion)

	// 短暂等待让 HTTP 响应先返回
	time.Sleep(500 * time.Millisecond)
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
}

// ── 工具函数 ──────────────────────────────────────────────────────────────────

// fetchLatestRelease 查询 GitHub releases/latest，返回 (tag, htmlURL, error)
func fetchLatestRelease() (string, string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET",
		"https://api.github.com/repos/Zyling-ai/zyhive/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
	}
	var data struct {
		TagName string `json:"tag_name"`
		HtmlURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", err
	}
	return data.TagName, data.HtmlURL, nil
}

// downloadFile 下载 url 到 dest，progress 回调 0-100
func downloadFile(url, dest string, progress func(int)) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			downloaded += int64(n)
			if total > 0 && progress != nil {
				progress(int(downloaded * 100 / total))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if progress != nil {
		progress(100)
	}
	return nil
}

// copyFile 复制文件（用于备份旧二进制 & 替换）
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
