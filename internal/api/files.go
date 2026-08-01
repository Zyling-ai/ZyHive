// File handler — workspace file management.
package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/artifact"
	"github.com/Zyling-ai/zyhive/pkg/safefs"
	"github.com/gin-gonic/gin"
)

type fileHandler struct {
	manager *agent.Manager
}

// FileEntry is one item in a flat directory listing.
type FileEntry struct {
	Name    string    `json:"name"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

// FileNode is one node in a recursive tree listing (?tree=true).
type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"` // relative to workspace root
	IsDir    bool        `json:"isDir"`
	Size     int64       `json:"size"`
	ModTime  time.Time   `json:"modTime"`
	Children []*FileNode `json:"children,omitempty"`
}

// skipDirs are directory names to exclude from tree listing.
var skipDirs = map[string]bool{
	".git": true, "__pycache__": true, "node_modules": true,
}

// buildFileTree recursively builds the file tree for a directory.
func buildFileTree(absDir, relBase string) []*FileNode {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil
	}
	var nodes []*FileNode
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		name := e.Name()
		relPath := name
		if relBase != "" {
			relPath = relBase + "/" + name
		}
		node := &FileNode{
			Name:    name,
			Path:    relPath,
			IsDir:   e.IsDir(),
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
		}
		if e.IsDir() {
			if skipDirs[name] {
				continue
			}
			node.Children = buildFileTree(filepath.Join(absDir, name), relPath)
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// resolveWorkspacePath validates the agent and returns absolute workspace path.
//
// 26.5.10v2 安全修复 (B001): 改用 safefs.ConfineToBase, 抵御 sibling-prefix
// 混淆 / symlink 逃逸 / 绝对路径注入. 旧版 strings.HasPrefix 校验有以下
// 已知绕过, 见 pkg/safefs/safefs.go 顶部注释.
func (h *fileHandler) resolveWorkspacePath(c *gin.Context) (string, string, bool) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return "", "", false
	}
	relPath := c.Param("path")
	// gin's *path captures keep a leading "/", strip it so safefs sees a
	// pure relative path (otherwise IsAbs would reject it).
	relPath = strings.TrimPrefix(relPath, "/")
	if relPath == "" {
		// caller wants the workspace root itself
		relPath = "."
	}
	absPath, err := safefs.ConfineToBase(ag.WorkspaceDir, relPath)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "path escapes workspace"})
		return "", "", false
	}
	return ag.WorkspaceDir, absPath, true
}

// Read GET /api/agents/:id/files/*path
// If path is a directory, returns JSON listing. If a file, returns content.
// Use ?tree=true for recursive directory listing.
func (h *fileHandler) Read(c *gin.Context) {
	wsDir, absPath, ok := h.resolveWorkspacePath(c)
	if !ok {
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Directory listing
	if info.IsDir() {
		// ?tree=true → recursive tree structure
		if c.Query("tree") == "true" {
			// Compute relative base for sub-directory requests
			var relBase string
			if absPath != wsDir {
				relBase, _ = filepath.Rel(wsDir, absPath)
			}
			nodes := buildFileTree(absPath, relBase)
			c.JSON(http.StatusOK, nodes)
			return
		}
		// flat listing (legacy)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		result := make([]FileEntry, 0, len(entries))
		for _, e := range entries {
			fi, err := e.Info()
			if err != nil {
				continue
			}
			result = append(result, FileEntry{
				Name:    e.Name(),
				IsDir:   e.IsDir(),
				Size:    fi.Size(),
				ModTime: fi.ModTime(),
			})
		}
		c.JSON(http.StatusOK, result)
		return
	}

	// File content
	data, err := os.ReadFile(absPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check if binary (contains null bytes in first 512 bytes)
	checkLen := len(data)
	if checkLen > 512 {
		checkLen = 512
	}
	isBinary := false
	for _, b := range data[:checkLen] {
		if b == 0 {
			isBinary = true
			break
		}
	}

	if isBinary {
		c.JSON(http.StatusOK, gin.H{
			"encoding": "base64",
			"content":  base64.StdEncoding.EncodeToString(data),
			"size":     len(data),
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"encoding": "utf-8",
			"content":  string(data),
			"size":     len(data),
		})
	}
}

// Write PUT /api/agents/:id/files/*path
// Accepts raw text (Content-Type: text/plain), JSON {content: string}, or
// JSON {content: "base64:<b64>"} for binary files.
// Optional query params for chunked upload:
//
//	?chunk=N&total=T  — N=0 creates/truncates, N>0 appends; last chunk returns {ok,size}
func (h *fileHandler) Write(c *gin.Context) {
	_, absPath, ok := h.resolveWorkspacePath(c)
	if !ok {
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 5*1024*1024)) // 5MB per chunk
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// If Content-Type is application/json, extract the "content" field
	ct := c.GetHeader("Content-Type")
	if strings.Contains(ct, "application/json") {
		var payload struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(body, &payload); err == nil {
			content := payload.Content
			// Support base64-encoded binary: "base64:<b64data>"
			if strings.HasPrefix(content, "base64:") {
				decoded, decErr := base64.StdEncoding.DecodeString(content[7:])
				if decErr != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64: " + decErr.Error()})
					return
				}
				body = decoded
			} else {
				body = []byte(content)
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Chunked upload support: ?chunk=N&total=T
	chunkStr := c.Query("chunk")
	if chunkStr != "" {
		chunkN := 0
		fmt.Sscanf(chunkStr, "%d", &chunkN)
		var f *os.File
		if chunkN == 0 {
			// First chunk: create or truncate
			f, err = os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		} else {
			// Subsequent chunks: append
			f, err = os.OpenFile(absPath, os.O_APPEND|os.O_WRONLY, 0644)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		_, werr := f.Write(body)
		f.Close()
		if werr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": werr.Error()})
			return
		}
		info, _ := os.Stat(absPath)
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "chunk": chunkN, "size": size})
		return
	}

	if err := os.WriteFile(absPath, body, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "size": len(body)})
}

// Delete DELETE /api/agents/:id/files/*path
func (h *fileHandler) Delete(c *gin.Context) {
	wsDir, absPath, ok := h.resolveWorkspacePath(c)
	if !ok {
		return
	}
	if filepath.Clean(absPath) == filepath.Clean(wsDir) {
		c.JSON(http.StatusForbidden, gin.H{"error": "workspace root is reserved"})
		return
	}

	if err := os.RemoveAll(absPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── Download Handler ──────────────────────────────────────────────────────
// GET /api/download?id=ARTIFACT_ID&token=ONE_TIME_TOKEN
// Serves one registered agent-workspace file through a short-lived,
// single-use credential. Host paths and administrator tokens never appear in
// the URL.

type downloadHandler struct {
	manager *agent.Manager
	tickets *artifact.TicketStore
}

func (h *downloadHandler) ServeFile(c *gin.Context) {
	artifactID := c.Query("id")
	token := c.Query("token")
	if artifactID == "" || token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid download credential"})
		return
	}
	tickets := h.tickets
	if tickets == nil {
		tickets = artifact.DefaultTickets
	}
	filePath, err := tickets.Consume(artifactID, token)
	if err != nil {
		if errors.Is(err, artifact.ErrExpiredTicket) {
			c.JSON(http.StatusGone, gin.H{"error": "download credential expired"})
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid download credential"})
		}
		return
	}

	confinedPath, ok := confineToAgentWorkspace(h.manager, filePath)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "file is outside an agent workspace"})
		return
	}

	info, err := os.Stat(confinedPath)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	baseName := filepath.Base(confinedPath)
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Content-Disposition", `attachment; filename="`+baseName+`"`)
	c.FileAttachment(confinedPath, baseName)
}

// confineToAgentWorkspace resolves an absolute file path against the known
// agent workspaces. It rejects sibling-prefix tricks and symlink escapes.
func confineToAgentWorkspace(manager *agent.Manager, filePath string) (string, bool) {
	if manager == nil || !filepath.IsAbs(filePath) {
		return "", false
	}
	for _, ag := range manager.List() {
		base, err := safefs.ConfineToBase(ag.WorkspaceDir, ".")
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(base, filePath)
		if err != nil {
			continue
		}
		resolved, err := safefs.ConfineToBase(base, rel)
		if err == nil {
			return resolved, true
		}
	}
	return "", false
}
