// File handler — workspace file management.
// Reference: openclaw/src/agents/workspace.ts
package api

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

type fileHandler struct{ cfg *config.Config }

func (h *fileHandler) Read(c *gin.Context)   { c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"}) }
func (h *fileHandler) Write(c *gin.Context)  { c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"}) }
func (h *fileHandler) Delete(c *gin.Context) { c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"}) }
