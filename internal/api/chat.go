// Chat handler — streaming SSE conversation endpoint.
// Reference: openclaw/src/gateway/server-chat.ts
// Full implementation: Phase 2 (pkg/runner must be complete first)
package api

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

type chatHandler struct{ cfg *config.Config }

// Chat POST /api/agents/:id/chat  (SSE streaming)
func (h *chatHandler) Chat(c *gin.Context) {
	// TODO: validate agent exists → call runner.Run(agentID, message) →
	// stream StreamEvents back as SSE: data: {"type":"text_delta","text":"..."}\n\n
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"})
}

func (h *chatHandler) ListSessions(c *gin.Context) {
	c.JSON(http.StatusOK, []any{})
}

func (h *chatHandler) GetSession(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"})
}
