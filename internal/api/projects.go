// Project handlers — shared project workspace CRUD + file management.
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
	"github.com/sunhuihui6688-star/ai-panel/pkg/project"
)

// ─── Project CRUD ────────────────────────────────────────────────────────────

type projectHandler struct {
	mgr *project.Manager
}

type ProjectInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	// Editors: nil/empty = all agents can write; ["__none__"] = read-only for all
	Editors     []string  `json:"editors"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func projectToInfo(p *project.Project) ProjectInfo {
	editors := p.Editors
	if editors == nil {
		editors = []string{}
	}
	return ProjectInfo{
		ID: p.ID, Name: p.Name, Description: p.Description,
		Tags: p.Tags, Editors: editors,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// SetPermissions PUT /api/projects/:id/permissions
func (h *projectHandler) SetPermissions(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Editors []string `json:"editors"` // empty = all can write
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.mgr.SetEditors(id, req.Editors); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	p, _ := h.mgr.Get(id)
	c.JSON(http.StatusOK, projectToInfo(p))
}

// GET /api/projects
func (h *projectHandler) List(c *gin.Context) {
	list := h.mgr.List()
	out := make([]ProjectInfo, 0, len(list))
	for _, p := range list {
		out = append(out, projectToInfo(p))
	}
	c.JSON(http.StatusOK, out)
}

// POST /api/projects
func (h *projectHandler) Create(c *gin.Context) {
	var req struct {
		ID          string   `json:"id" binding:"required"`
		Name        string   `json:"name" binding:"required"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p, err := h.mgr.Create(project.CreateOpts{
		ID: req.ID, Name: req.Name, Description: req.Description, Tags: req.Tags,
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, projectToInfo(p))
}

// GET /api/projects/:id
func (h *projectHandler) Get(c *gin.Context) {
	p, ok := h.mgr.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	c.JSON(http.StatusOK, projectToInfo(p))
}

// PATCH /api/projects/:id
func (h *projectHandler) Update(c *gin.Context) {
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.mgr.Update(c.Param("id"), req.Name, req.Description, req.Tags); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	p, _ := h.mgr.Get(c.Param("id"))
	c.JSON(http.StatusOK, projectToInfo(p))
}

// DELETE /api/projects/:id
func (h *projectHandler) Delete(c *gin.Context) {
	if err := h.mgr.Remove(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── Project File Management ─────────────────────────────────────────────────

type projectFileHandler struct {
	mgr *project.Manager
}

// resolveProjectPath validates project and resolves absolute file path.
func (h *projectFileHandler) resolve(c *gin.Context) (string, string, bool) {
	p, ok := h.mgr.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return "", "", false
	}
	relPath := c.Param("path")
	if relPath == "" || relPath == "/" {
		relPath = "/"
	}
	cleaned := filepath.Clean(relPath)
	absPath := filepath.Join(p.FilesDir, cleaned)
	if !strings.HasPrefix(absPath, p.FilesDir) {
		c.JSON(http.StatusForbidden, gin.H{"error": "path escapes project"})
		return "", "", false
	}
	return p.FilesDir, absPath, true
}

// GET /api/projects/:id/files/*path
func (h *projectFileHandler) Read(c *gin.Context) {
	rootDir, absPath, ok := h.resolve(c)
	if !ok {
		return
	}

	// Skip meta.json from public listing
	if filepath.Base(absPath) == "meta.json" {
		c.JSON(http.StatusForbidden, gin.H{"error": "reserved"})
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

	if info.IsDir() {
		if c.Query("tree") == "true" {
			var relBase string
			if absPath != rootDir {
				relBase, _ = filepath.Rel(rootDir, absPath)
			}
			nodes := buildProjectTree(absPath, relBase)
			c.JSON(http.StatusOK, nodes)
			return
		}
		// flat listing
		entries, _ := os.ReadDir(absPath)
		var result []FileEntry
		for _, e := range entries {
			if e.Name() == "meta.json" {
				continue
			}
			fi, _ := e.Info()
			if fi == nil {
				continue
			}
			result = append(result, FileEntry{
				Name: e.Name(), IsDir: e.IsDir(),
				Size: fi.Size(), ModTime: fi.ModTime(),
			})
		}
		c.JSON(http.StatusOK, result)
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
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
		c.JSON(http.StatusOK, gin.H{"encoding": "base64", "content": base64.StdEncoding.EncodeToString(data), "size": len(data)})
	} else {
		c.JSON(http.StatusOK, gin.H{"encoding": "utf-8", "content": string(data), "size": len(data)})
	}
}

// PUT /api/projects/:id/files/*path
func (h *projectFileHandler) Write(c *gin.Context) {
	_, absPath, ok := h.resolve(c)
	if !ok {
		return
	}
	if filepath.Base(absPath) == "meta.json" {
		c.JSON(http.StatusForbidden, gin.H{"error": "reserved"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 10*1024*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Support JSON {content: string} or raw text
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

// DELETE /api/projects/:id/files/*path
func (h *projectFileHandler) Delete(c *gin.Context) {
	_, absPath, ok := h.resolve(c)
	if !ok {
		return
	}
	if filepath.Base(absPath) == "meta.json" {
		c.JSON(http.StatusForbidden, gin.H{"error": "reserved"})
		return
	}
	if err := os.RemoveAll(absPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// buildProjectTree builds a recursive file tree, skipping meta.json.
func buildProjectTree(absDir, relBase string) []*FileNode {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil
	}
	var nodes []*FileNode
	for _, e := range entries {
		if e.Name() == "meta.json" {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		name := e.Name()
		relPath := name
		if relBase != "" {
			relPath = relBase + "/" + name
		}
		node := &FileNode{Name: name, Path: relPath, IsDir: e.IsDir(), Size: fi.Size(), ModTime: fi.ModTime()}
		if e.IsDir() {
			if skipDirs[name] {
				continue
			}
			node.Children = buildProjectTree(filepath.Join(absDir, name), relPath)
		}
		nodes = append(nodes, node)
	}
	return nodes
}
