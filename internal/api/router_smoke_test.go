package api

import (
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/gin-gonic/gin"
)

// Verifies that all new routes register without panicking (no path conflicts).
func TestRegisterRoutesNoConflicts(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	cfg := &config.Config{}
	mgr := agent.NewManager(t.TempDir())
	// Pass nil for optional managers; RegisterRoutes guards against nil.
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("RegisterRoutes panicked: %v", rec)
		}
	}()
	RegisterRoutes(r, cfg, "", mgr, nil, nil, nil, nil, BotControl{}, nil, nil, nil, nil, nil)
}
