// internal/api/update.go — 版本检查与在线升级 API
// 升级流程：下载新二进制 → 验证 → 备份旧版 → rm -f → cp → SIGTERM 重启
// 用户数据（agents 目录、配置文件）全程不涉及，仅替换可执行文件本身。
package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	Progress  int         `json:"progress"` // 0-100
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

var releaseVersionPattern = regexp.MustCompile(`^[0-9]{2}\.[0-9]{1,2}\.[0-9]{1,2}v[0-9]+$`)

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
		"hasUpdate":  semverGt(latest, current),
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

	if !releaseVersionPattern.MatchString(targetVersion) {
		s.set(StageFailed, 0, "目标版本格式无效："+targetVersion)
		return
	}
	if targetVersion == AppVersion {
		s.set(StageDone, 100, "当前已是最新版本（"+AppVersion+"），无需升级")
		return
	}

	// 2. 构建下载 URL（支持国内镜像）
	osName := runtime.GOOS // linux / darwin
	arch := runtime.GOARCH // amd64 / arm64
	if !isSupportedReleaseTarget(osName, arch) {
		s.set(StageFailed, 0, fmt.Sprintf("暂不支持在线升级平台：%s/%s", osName, arch))
		return
	}
	filename := fmt.Sprintf("zyhive-%s-%s", osName, arch)
	directURL := fmt.Sprintf(
		"https://github.com/Zyling-ai/zyhive/releases/download/%s/%s",
		targetVersion, filename,
	)
	// 国内镜像代理：直连失败或 404 时自动切换
	// 国内镜像：用自建 CF Worker 代理（install.zyling.ai/dl/VERSION/FILE）
	// 比 ghproxy 更稳定，完全自控
	mirrorURL := fmt.Sprintf(
		"https://install.zyling.ai/dl/%s/%s",
		targetVersion, filename,
	)

	// 优先走镜像（install.zyling.ai 内置国内加速），失败才回退 GitHub 直连
	url := mirrorURL
	if !isURLReachable(mirrorURL) {
		log.Printf("[update] mirror unreachable, falling back to direct GitHub")
		url = directURL
	}

	// 3. 下载到临时文件
	tmpFile, err := os.CreateTemp("", "zyhive-new-*")
	if err != nil {
		s.set(StageFailed, 0, "无法创建临时文件："+err.Error())
		return
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	source := "GitHub"
	if url == mirrorURL {
		source = "国内镜像"
	}
	s.set(StageDownloading, 10, fmt.Sprintf("正在从 %s 下载…", source))
	log.Printf("[update] downloading %s → %s", url, tmpPath)

	downloadErr := downloadFile(url, tmpPath, func(pct int) {
		s.set(StageDownloading, 10+pct*60/100, fmt.Sprintf("下载中… %d%%", pct))
	})
	if downloadErr != nil && url == mirrorURL {
		log.Printf("[update] mirror download failed, retrying direct GitHub: %v", downloadErr)
		source = "GitHub"
		url = directURL
		s.set(StageDownloading, 10, "国内镜像下载失败，切换 GitHub…")
		downloadErr = downloadFile(url, tmpPath, func(pct int) {
			s.set(StageDownloading, 10+pct*60/100, fmt.Sprintf("下载中… %d%%", pct))
		})
	}
	if downloadErr != nil {
		s.set(StageFailed, 0, "下载失败："+downloadErr.Error())
		return
	}

	// 4. 验证 SHA-256 和内嵌版本号
	s.set(StageVerifying, 72, "验证文件完整性和版本…")
	expectedSHA, err := fetchExpectedChecksum(targetVersion, filename)
	if err != nil {
		s.set(StageFailed, 0, "无法验证发布校验和："+err.Error())
		return
	}
	actualSHA, err := fileSHA256(tmpPath)
	if err != nil {
		s.set(StageFailed, 0, "计算文件校验和失败："+err.Error())
		return
	}
	if !strings.EqualFold(actualSHA, expectedSHA) {
		s.set(StageFailed, 0, "SHA-256 校验失败，已拒绝升级")
		return
	}
	if err := os.Chmod(tmpPath, 0755); err != nil {
		s.set(StageFailed, 0, "设置新版本执行权限失败："+err.Error())
		return
	}
	out, err := exec.Command(tmpPath, "--version").Output()
	if err != nil {
		s.set(StageFailed, 0, "新版本验证失败："+err.Error())
		return
	}
	fields := strings.Fields(string(out))
	detectedVer := ""
	if len(fields) > 0 {
		detectedVer = fields[len(fields)-1]
	}
	if detectedVer != targetVersion {
		s.set(StageFailed, 0, fmt.Sprintf("版本验证失败：期望 %s，实际 %s", targetVersion, detectedVer))
		return
	}
	log.Printf("[update] verified new binary: %s", detectedVer)

	// 5. 备份旧二进制
	s.set(StageApplying, 80, "备份旧版本…")
	binaryPath, err := os.Executable()
	if err != nil {
		s.set(StageFailed, 0, "无法获取当前二进制路径："+err.Error())
		return
	}
	if resolved, resolveErr := filepath.EvalSymlinks(binaryPath); resolveErr == nil {
		binaryPath = resolved
	}
	backupPath := binaryPath + ".bak"
	if err := copyFile(binaryPath, backupPath); err != nil {
		s.set(StageFailed, 0, "备份旧版本失败，已取消升级："+err.Error())
		return
	}
	if err := os.Chmod(backupPath, 0755); err != nil {
		s.set(StageFailed, 0, "设置备份文件权限失败："+err.Error())
		return
	}

	// 6. 在目标目录写入临时文件后原子替换，失败时原文件保持不变。
	s.set(StageApplying, 88, "原子替换二进制文件…")
	log.Printf("[update] replacing binary: %s → %s", tmpPath, binaryPath)
	stagedPath := filepath.Join(filepath.Dir(binaryPath), "."+filepath.Base(binaryPath)+".new")
	if err := copyFile(tmpPath, stagedPath); err != nil {
		s.set(StageFailed, 0, "准备新版本失败："+err.Error())
		return
	}
	defer os.Remove(stagedPath)
	if err := os.Chmod(stagedPath, 0755); err != nil {
		s.set(StageFailed, 0, "设置新版本权限失败："+err.Error())
		return
	}
	if err := os.Rename(stagedPath, binaryPath); err != nil {
		s.set(StageFailed, 0, "原子替换失败，旧版本未变更："+err.Error())
		return
	}

	// 7. 标记完成，发 SIGTERM 让 systemd/launchd 重启服务
	// 用户数据（agents dir / config）完全不涉及，进程重启后新版本自动加载
	s.set(StageDone, 100, "升级成功！正在重启服务…（新版本："+targetVersion+"）")
	log.Printf("[update] upgrade complete → %s，sending SIGTERM to self", targetVersion)

	// 短暂等待让 HTTP 响应先返回
	time.Sleep(500 * time.Millisecond)
	selfRestart() // 平台适配：unix=SIGTERM，windows=os.Exit(0)
}

// ── 工具函数 ──────────────────────────────────────────────────────────────────

// fetchLatestRelease 查询最新版本，优先用 CF 镜像（国内可访问），失败才回退 GitHub API
func fetchLatestRelease() (string, string, error) {
	client := &http.Client{Timeout: 8 * time.Second}

	// 优先：install.zyling.ai/latest（CF Worker，国内可访问）
	if resp, err := client.Get("https://install.zyling.ai/latest"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var data struct {
				Version string `json:"version"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && data.Version != "" {
				releaseURL := fmt.Sprintf("https://github.com/Zyling-ai/ZyHive/releases/tag/%s", data.Version)
				return data.Version, releaseURL, nil
			}
		}
	}

	// 回退：GitHub API
	req, _ := http.NewRequest("GET",
		"https://api.github.com/repos/Zyling-ai/zyhive/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("无法获取版本信息（镜像和 GitHub 均不可达）: %w", err)
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

func isSupportedReleaseTarget(osName, arch string) bool {
	if osName != "linux" && osName != "darwin" {
		return false
	}
	return arch == "amd64" || arch == "arm64"
}

func fetchExpectedChecksum(version, filename string) (string, error) {
	urls := []string{
		fmt.Sprintf("https://github.com/Zyling-ai/zyhive/releases/download/%s/SHA256SUMS", version),
		fmt.Sprintf("https://install.zyling.ai/dl/%s/SHA256SUMS", version),
	}
	client := &http.Client{Timeout: 15 * time.Second}
	var failures []string

	for _, url := range urls {
		resp, err := client.Get(url)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if resp.StatusCode != http.StatusOK {
			failures = append(failures, fmt.Sprintf("%s: HTTP %d", url, resp.StatusCode))
			resp.Body.Close()
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		resp.Body.Close()
		if readErr != nil {
			failures = append(failures, readErr.Error())
			continue
		}
		checksum, parseErr := parseChecksumList(data, filename)
		if parseErr != nil {
			failures = append(failures, parseErr.Error())
			continue
		}
		return checksum, nil
	}

	return "", fmt.Errorf("所有校验和来源均失败：%s", strings.Join(failures, "; "))
}

func parseChecksumList(data []byte, filename string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == filename {
			sum := strings.ToLower(fields[0])
			if len(sum) != sha256.Size*2 {
				return "", fmt.Errorf("%s 的 SHA-256 长度无效", filename)
			}
			for _, ch := range sum {
				if !strings.ContainsRune("0123456789abcdef", ch) {
					return "", fmt.Errorf("%s 的 SHA-256 格式无效", filename)
				}
			}
			return sum, nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS 中缺少 %s", filename)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// downloadFile 下载 url 到 dest，progress 回调 0-100
func downloadFile(url, dest string, progress func(int)) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	const maxDownloadSize = int64(256 * 1024 * 1024)
	if resp.ContentLength > maxDownloadSize {
		return fmt.Errorf("文件过大：%d bytes", resp.ContentLength)
	}

	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)

	// Fallback 策略：当 Content-Length 未知（例如经 CF Worker 流式代理）
	// 或 total=-1，按已下载字节数 + 启发式上限估算进度，避免 UI 永远停在 0%。
	// 估算上限：ZyHive 单二进制 ~25MB，给 32MB 冗余。
	const estimatedSize = int64(32 * 1024 * 1024)
	var lastReported int

	buf = make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if downloaded+int64(n) > maxDownloadSize {
				return fmt.Errorf("下载超过大小上限：%d bytes", maxDownloadSize)
			}
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			downloaded += int64(n)
			if progress != nil {
				var pct int
				if total > 0 {
					pct = int(downloaded * 100 / total)
				} else {
					// 未知长度 → 按估算上限算，封顶 95%，最后 progress(100) 收尾
					pct = int(downloaded * 100 / estimatedSize)
					if pct > 95 {
						pct = 95
					}
				}
				// 节流：只在进度百分比变化 ≥1 时回调，避免刷 set 过快锁竞争
				if pct != lastReported {
					progress(pct)
					lastReported = pct
				}
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

// isURLReachable 快速探测 URL 是否可访问（HEAD 请求，3s 超时）
func isURLReachable(url string) bool {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil // 允许重定向
		},
	}
	resp, err := client.Head(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

// semverGt 比较 a > b
// 支持两种格式：
//   - 语义版本：v0.9.26（三段）
//   - 日期版本：26.3.25v1（YY.M.D + vN 修订号）
func semverGt(a, b string) bool {
	parse := func(s string) [4]int {
		s = strings.TrimPrefix(s, "v")
		// 处理日期版本末尾的 vN 修订号，如 26.3.25v1 → 26.3.25 + 1
		var revision int
		if idx := strings.LastIndexAny(s, "vV"); idx > 0 {
			rev, err := strconv.Atoi(s[idx+1:])
			if err == nil {
				revision = rev
				s = s[:idx]
			}
		}
		parts := strings.SplitN(s, ".", 3)
		var r [4]int
		for i := 0; i < 3 && i < len(parts); i++ {
			r[i], _ = strconv.Atoi(parts[i])
		}
		r[3] = revision
		return r
	}
	av, bv := parse(a), parse(b)
	for i := 0; i < 4; i++ {
		if av[i] > bv[i] {
			return true
		}
		if av[i] < bv[i] {
			return false
		}
	}
	return false
}

// copyFile 复制文件（用于备份旧二进制 & 替换）
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0755
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err = out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}
