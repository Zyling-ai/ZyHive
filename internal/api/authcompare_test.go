// 26.5.10v3 — B002 tests.
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecretsEqual_BasicMatch(t *testing.T) {
	if !secretsEqual("abc123", "abc123") {
		t.Fatal("identical secrets must match")
	}
}

func TestSecretsEqual_BasicMismatch(t *testing.T) {
	if secretsEqual("abc123", "abc124") {
		t.Fatal("different secrets must not match")
	}
}

func TestSecretsEqual_LengthMismatchSafe(t *testing.T) {
	// subtle.ConstantTimeCompare returns 0 for length mismatch — that's
	// intended; we just don't want a panic or wrong result.
	if secretsEqual("abc", "abcdef") {
		t.Fatal("different lengths must not match")
	}
	if secretsEqual("abcdef", "abc") {
		t.Fatal("different lengths must not match")
	}
}

func TestSecretsEqual_EmptyAcceptableButNotMatchingNonEmpty(t *testing.T) {
	if !secretsEqual("", "") {
		t.Fatal("two empty strings should match")
	}
	if secretsEqual("", "x") {
		t.Fatal("empty vs non-empty should not match")
	}
	if secretsEqual("x", "") {
		t.Fatal("non-empty vs empty should not match")
	}
}

// authMiddleware integration — confirm the constant-time compare actually
// rejects wrong tokens (defensive regression).
func TestAuthMiddleware_WrongTokenRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(authMiddleware("super-secret-token"))
	r.GET("/api/ping", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	cases := []struct {
		name       string
		header     string
		expectCode int
	}{
		{"empty header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic super-secret-token", http.StatusUnauthorized},
		{"correct prefix wrong tail", "Bearer super-secret-tokenX", http.StatusUnauthorized},
		{"truncated", "Bearer super-secret-toke", http.StatusUnauthorized},
		{"completely wrong", "Bearer aaaaaaaaaaaaaaaa", http.StatusUnauthorized},
		{"correct", "Bearer super-secret-token", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.expectCode {
				t.Fatalf("got %d want %d (body=%s)", w.Code, tc.expectCode, w.Body.String())
			}
		})
	}
}

func TestAuthMiddleware_NoTokenAllowsAll(t *testing.T) {
	// Dev-mode: empty token configured → no auth applied.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(authMiddleware(""))
	r.GET("/api/ping", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("dev-mode (no token) should allow, got %d", w.Code)
	}
}

// download token: query-param auth, also constant-time
func TestDownloadHandler_WrongTokenRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &downloadHandler{authToken: "super-secret-token"}
	r.GET("/api/download", h.ServeFile)

	req := httptest.NewRequest(http.MethodGet, "/api/download?path=/tmp/x&token=wrong", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong download token should 401, got %d", w.Code)
	}
}

// media token: query-param OR header auth, both constant-time
func TestMediaHandler_WrongTokenRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &mediaHandler{token: "super-secret-token"}
	r.GET("/api/media", h.ServeMedia)

	cases := []struct {
		name string
		url  string
		hdr  string
	}{
		{"wrong query", "/api/media?path=/tmp/x.png&token=wrong", ""},
		{"wrong header", "/api/media?path=/tmp/x.png", "Bearer wrong"},
		{"no auth", "/api/media?path=/tmp/x.png", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			if tc.hdr != "" {
				req.Header.Set("Authorization", tc.hdr)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("got %d want 401", w.Code)
			}
		})
	}
}
