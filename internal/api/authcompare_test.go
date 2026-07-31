// 26.5.10v3 — B002 tests.
package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/artifact"
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

func TestAuthMiddleware_NoTokenFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(authMiddleware(""))
	r.GET("/api/ping", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing auth configuration should fail closed, got %d", w.Code)
	}
}

func TestApprovalStreamUsesOneTimeTicket(t *testing.T) {
	previous := approvalStreamTickets
	approvalStreamTickets = newEphemeralTicketStore()
	t.Cleanup(func() { approvalStreamTickets = previous })

	ticket, ok := approvalStreamTickets.issue(time.Minute)
	if !ok {
		t.Fatal("failed to issue approval stream ticket")
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(authMiddleware("admin-secret"))
	r.GET("/api/approvals/stream", func(c *gin.Context) { c.Status(http.StatusOK) })

	first := httptest.NewRequest(http.MethodGet,
		"/api/approvals/stream?ticket="+url.QueryEscape(ticket), nil)
	firstResult := httptest.NewRecorder()
	r.ServeHTTP(firstResult, first)
	if firstResult.Code != http.StatusOK {
		t.Fatalf("valid stream ticket rejected: %d", firstResult.Code)
	}

	replay := httptest.NewRequest(http.MethodGet,
		"/api/approvals/stream?ticket="+url.QueryEscape(ticket), nil)
	replayResult := httptest.NewRecorder()
	r.ServeHTTP(replayResult, replay)
	if replayResult.Code != http.StatusUnauthorized {
		t.Fatalf("replayed stream ticket accepted: %d", replayResult.Code)
	}

	legacy := httptest.NewRequest(http.MethodGet,
		"/api/approvals/stream?token=admin-secret", nil)
	legacyResult := httptest.NewRecorder()
	r.ServeHTTP(legacyResult, legacy)
	if legacyResult.Code != http.StatusUnauthorized {
		t.Fatalf("long-lived query token still accepted: %d", legacyResult.Code)
	}
}

// Download credentials are short-lived and compared in constant time.
func TestDownloadHandler_WrongTokenRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(path, []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	tickets := artifact.NewTicketStore()
	artifactID, _, _, err := tickets.Issue(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	h := &downloadHandler{tickets: tickets}
	r.GET("/api/download", h.ServeFile)

	req := httptest.NewRequest(http.MethodGet,
		"/api/download?id="+url.QueryEscape(artifactID)+"&token=wrong", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong download token should 401, got %d", w.Code)
	}
}

func TestDownloadHandler_MissingCredentialFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &downloadHandler{}
	r.GET("/api/download", h.ServeFile)

	req := httptest.NewRequest(http.MethodGet, "/api/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing download credential should fail closed, got %d", w.Code)
	}
}

func TestMediaHandler_RejectsWrongAndLegacyCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := filepath.Join(t.TempDir(), "preview.png")
	if err := os.WriteFile(path, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	tickets := artifact.NewTicketStore()
	artifactID, _, _, err := tickets.Issue(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	h := &mediaHandler{tickets: tickets}
	r.GET("/api/media", h.ServeMedia)

	cases := []struct{ name, url string }{
		{"wrong ticket", "/api/media?id=" + url.QueryEscape(artifactID) + "&token=wrong"},
		{"legacy admin token", "/api/media?path=" + url.QueryEscape(path) + "&token=admin-secret"},
		{"missing credential", "/api/media"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("got %d want 401", w.Code)
			}
		})
	}
}
