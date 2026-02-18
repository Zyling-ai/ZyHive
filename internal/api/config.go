// Config handler — read/write aipanel.json via API.
// Reference: openclaw/src/gateway/server-runtime-config.ts
package api

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

type configHandler struct{ cfg *config.Config }

func (h *configHandler) Get(c *gin.Context) {
	// Never expose raw token in response
	safe := *h.cfg
	safe.Auth.Token = "***"
	c.JSON(http.StatusOK, safe)
}
func (h *configHandler) Patch(c *gin.Context)  { c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"}) }
func (h *configHandler) TestKey(c *gin.Context) { c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"}) }
