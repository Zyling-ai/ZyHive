// internal/api/throttle.go — observability endpoint for the P1-03 LLM
// throttle. Read-only; never mutates throttle state.
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/llm"
)

type throttleHandler struct{}

// Get returns the current per-provider AdaptiveThrottle state, or {"kind":
// "fixed"} when no adaptive throttle is installed (to keep the contract
// stable for the UI).
func (throttleHandler) Get(c *gin.Context) {
	t := llm.GlobalThrottle()
	if t == nil {
		c.JSON(http.StatusOK, gin.H{"kind": "fixed", "providers": []any{}})
		return
	}
	if at, ok := t.(*llm.AdaptiveThrottle); ok {
		c.JSON(http.StatusOK, gin.H{
			"kind":      "adaptive",
			"providers": at.Snapshot(),
		})
		return
	}
	// FixedThrottle: no per-provider snapshot; return a minimal stub.
	c.JSON(http.StatusOK, gin.H{"kind": "fixed", "providers": []any{}})
}
