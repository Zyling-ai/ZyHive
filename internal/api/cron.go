// Cron job handler.
// Reference: openclaw/src/gateway/server-cron.ts, src/cron/
package api

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

type cronHandler struct{ cfg *config.Config }

func (h *cronHandler) List(c *gin.Context)   { c.JSON(http.StatusOK, []any{}) }
func (h *cronHandler) Create(c *gin.Context) { c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 3"}) }
func (h *cronHandler) Update(c *gin.Context) { c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 3"}) }
func (h *cronHandler) Delete(c *gin.Context) { c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 3"}) }
func (h *cronHandler) Run(c *gin.Context)    { c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 3"}) }
func (h *cronHandler) Runs(c *gin.Context)   { c.JSON(http.StatusOK, []any{}) }
