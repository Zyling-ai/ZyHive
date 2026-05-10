// 26.5.10v4 — B003 unbounded request body OOM mitigation.
//
// Gin's `ShouldBindJSON` reads `c.Request.Body` to EOF without size cap.
// 50+ endpoints rely on this. An attacker (or bug) could POST gigabyte
// payloads → server OOM → DoS.
//
// Fix: middleware that wraps Request.Body in http.MaxBytesReader. When
// limit exceeded, the underlying io.Reader returns http.MaxBytesError and
// Gin surfaces it as a 4xx — Gin's `ShouldBindJSON` returns the wrapped
// error untouched.
//
// Default limit: 4 MiB. Tunable via env `ZYHIVE_MAX_REQUEST_BODY_MB`.
//
// Endpoints with intentionally larger payloads (file uploads via
// `internal/api/files.go::Write`) bypass this limit because they read from
// the raw `c.Request.Body` directly via io.LimitReader with their own cap
// (5 MiB per chunk). The middleware is applied AFTER the file routes are
// registered with their own body handling, OR they are explicitly opted out
// — see registration order in router.go.
package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	defaultMaxBodyBytes = 4 * 1024 * 1024 // 4 MiB
	maxBodyEnvVar       = "ZYHIVE_MAX_REQUEST_BODY_MB"
)

// bodyLimitFromEnv returns the configured cap in bytes, or the default.
// Honors ZYHIVE_MAX_REQUEST_BODY_MB (an integer in MiB).
// Returns 0 to mean "no limit" only when env explicitly says "0" — useful
// for self-hosted users with legitimate giant uploads (and trusted network).
func bodyLimitFromEnv() int64 {
	v := os.Getenv(maxBodyEnvVar)
	if v == "" {
		return defaultMaxBodyBytes
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return defaultMaxBodyBytes
	}
	if n == 0 {
		return 0 // explicit "no limit"
	}
	return n * 1024 * 1024
}

// bodyLimitMiddleware wraps c.Request.Body with http.MaxBytesReader so that
// any subsequent read (Gin's ShouldBindJSON, raw io.ReadAll, ...) gets a
// short read + http.MaxBytesError instead of OOM.
//
// Skip the wrap for the upload endpoints that intentionally accept larger
// bodies — they have their own per-chunk LimitReader.
func bodyLimitMiddleware() gin.HandlerFunc {
	limit := bodyLimitFromEnv()
	if limit <= 0 {
		// "0" or invalid → no-op middleware (escape hatch).
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		// Skip for endpoints that already manage body size:
		// PUT /api/agents/:id/files/*path  → 5 MiB per chunk via io.LimitReader
		// PUT /api/projects/:id/files/*path → 10 MiB per chunk via io.LimitReader
		path := c.FullPath()
		if path == "/api/agents/:id/files/*path" || path == "/api/projects/:id/files/*path" {
			c.Next()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

// IsBodyTooLarge tells whether an error from binding/reading was caused by
// the request body exceeding the cap (handy in handlers that want to surface
// a friendly 413 instead of a generic 400).
func IsBodyTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return true
	}
	// Pre-Go-1.19 / wrapped: fall back to message matching.
	return contains(err.Error(), "http: request body too large")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || indexOf(s, substr) >= 0)))
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// describeBodyLimit returns "ZYHIVE_MAX_REQUEST_BODY_MB=8" or "default 4 MiB"
// for startup logging.
func describeBodyLimit() string {
	v := os.Getenv(maxBodyEnvVar)
	if v == "" {
		return fmt.Sprintf("default %d MiB", defaultMaxBodyBytes/(1024*1024))
	}
	return maxBodyEnvVar + "=" + v + " MiB"
}
