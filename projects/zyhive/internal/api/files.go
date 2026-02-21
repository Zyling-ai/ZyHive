// File handler — workspace file management.
// Reference: openclaw/src/agents/workspace.ts
package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
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
func (h *fileHandler) resolveWorkspacePath(c *gin.Context) (string, string, bool) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return "", "", false
	}
	relPath := c.Param("path")
	if relPath == "" || relPath == "/" {
		relPath = "/"
	}
	// Clean and ensure we stay inside workspace
	cleaned := filepath.Clean(relPath)
	absPath := filepath.Join(ag.WorkspaceDir, cleaned)
	// Security: ensure resolved path is within workspace
	if !strings.HasPrefix(absPath, ag.WorkspaceDir) {
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
// Accepts both raw text (Content-Type: text/plain) and JSON {content: string}.
func (h *fileHandler) Write(c *gin.Context) {
	_, absPath, ok := h.resolveWorkspacePath(c)
	if !ok {
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 10*1024*1024)) // 10MB limit
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
			body = []byte(payload.Content)
		}
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	_, absPath, ok := h.resolveWorkspacePath(c)
	if !ok {
		return
	}

	if err := os.RemoveAll(absPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
