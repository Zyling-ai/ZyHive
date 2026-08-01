package api

import (
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

type configAccessGuard struct {
	mutex sync.RWMutex
}

func (guard *configAccessGuard) middleware(c *gin.Context) {
	if mutatesSharedConfig(c.Request) {
		guard.mutex.Lock()
		defer guard.mutex.Unlock()
	} else if !bypassesSharedConfigLock(c.Request) {
		guard.mutex.RLock()
		defer guard.mutex.RUnlock()
	}
	c.Next()
}

// Long-lived endpoints that never read the shared config must not retain an
// RLock for the lifetime of their stream, otherwise every config write stalls.
func bypassesSharedConfigLock(request *http.Request) bool {
	return request.Method == http.MethodGet && request.URL.Path == "/api/approvals/stream"
}

func mutatesSharedConfig(request *http.Request) bool {
	if request.Method == http.MethodGet || request.Method == http.MethodHead || request.Method == http.MethodOptions {
		return false
	}
	for _, prefix := range []string{
		"/api/config",
		"/api/providers",
		"/api/models",
		"/api/channels",
		"/api/tools",
		"/api/skills",
		"/api/acp",
	} {
		if request.URL.Path == prefix || strings.HasPrefix(request.URL.Path, prefix+"/") {
			return true
		}
	}
	return false
}
