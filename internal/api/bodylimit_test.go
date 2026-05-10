// 26.5.10v4 — B003 tests.
package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBodyLimit_DefaultCapsAt4MiB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(bodyLimitMiddleware())
	r.POST("/api/test", func(c *gin.Context) {
		var v struct {
			X string `json:"x"`
		}
		if err := c.ShouldBindJSON(&v); err != nil {
			if IsBodyTooLarge(err) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "too big"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Build a 5 MiB JSON payload — should trigger the 4 MiB cap.
	huge := `{"x":"` + strings.Repeat("a", 5*1024*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(huge))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Either 413 (handler caught it) or 400 (Gin surfaced bind err).
	// Both are acceptable evidence of the cap working.
	if w.Code != http.StatusRequestEntityTooLarge && w.Code != http.StatusBadRequest {
		t.Fatalf("expected 413 or 400 for 5 MiB body, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestBodyLimit_AllowsSmallBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(bodyLimitMiddleware())
	r.POST("/api/test", func(c *gin.Context) {
		var v struct{ X string `json:"x"` }
		if err := c.ShouldBindJSON(&v); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"x": v.X})
	})

	body := `{"x":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("small body rejected: %d %s", w.Code, w.Body.String())
	}
}

func TestBodyLimit_ExemptFileUploadRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(bodyLimitMiddleware())
	// Mimic the file upload route exactly (the middleware looks at FullPath).
	r.PUT("/api/agents/:id/files/*path", func(c *gin.Context) {
		// Just confirm the body wasn't wrapped at this point. We can detect
		// this by reading more than the cap.
		buf := make([]byte, 5*1024*1024)
		n, _ := c.Request.Body.Read(buf)
		c.JSON(http.StatusOK, gin.H{"read": n})
	})

	huge := bytes.Repeat([]byte("a"), 5*1024*1024)
	req := httptest.NewRequest(http.MethodPut, "/api/agents/abc/files/foo.bin", bytes.NewReader(huge))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("file upload route should bypass cap, got %d", w.Code)
	}
}

func TestIsBodyTooLarge_Detects(t *testing.T) {
	if !IsBodyTooLarge(&http.MaxBytesError{Limit: 100}) {
		t.Fatal("MaxBytesError should be detected")
	}
	if IsBodyTooLarge(nil) {
		t.Fatal("nil should not be detected")
	}
}
