// 26.5.10v2 — B001 path traversal regression tests.
//
// 验证 /api/agents/:id/files/*path 各种攻击向量被 fileHandler.resolveWorkspacePath
// (调用 safefs.ConfineToBase) 阻断.
package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/agent"
)

// setupSecurityTestEnv creates two sibling agent workspaces ("alice" and
// "alice-evil") and registers them in a Manager so we can verify B001
// sibling-prefix bypass.
func setupSecurityTestEnv(t *testing.T) (mgr *agent.Manager, aliceWS string) {
	t.Helper()
	root := t.TempDir()
	aliceDir := filepath.Join(root, "alice")
	evilDir := filepath.Join(root, "alice-evil")
	for _, d := range []string{aliceDir, evilDir} {
		if err := os.MkdirAll(filepath.Join(d, "workspace"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Plant a "secret" file in evil sibling — the attack target.
	if err := os.WriteFile(filepath.Join(evilDir, "workspace", "secret.md"),
		[]byte("EVIL_SECRET"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aliceDir, "workspace", "ok.md"),
		[]byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr = agent.NewManager(root)
	aliceWS = filepath.Join(aliceDir, "workspace")
	// Inject agent records directly via reflection-free helper:
	// NewManager + Load reads from disk; we sidestep by directly using the
	// public Get helper after manually populating. Simplest: write minimal
	// config.json and call Load.
	for _, ag := range []struct{ id, dir string }{
		{"alice", aliceDir},
		{"alice-evil", evilDir},
	} {
		if err := os.WriteFile(filepath.Join(ag.dir, "config.json"),
			[]byte(`{"id":"`+ag.id+`","name":"`+ag.id+`"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	return mgr, aliceWS
}

func newTestRouter(mgr *agent.Manager) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &fileHandler{manager: mgr}
	g := r.Group("/api")
	agents := g.Group("/agents")
	agents.GET("/:id/files/*path", h.Read)
	agents.PUT("/:id/files/*path", h.Write)
	agents.DELETE("/:id/files/*path", h.Delete)
	return r
}

// B001 main attack: sibling-prefix bypass.
func TestB001_RejectsSiblingPrefixBypass(t *testing.T) {
	mgr, _ := setupSecurityTestEnv(t)
	r := newTestRouter(mgr)

	// Old vulnerable code allowed: GET /api/agents/alice/files/../alice-evil/secret.md
	// to read EVIL_SECRET because HasPrefix("/.../alice-evil/secret.md", "/.../alice")
	// returned true.
	req := httptest.NewRequest(http.MethodGet,
		"/api/agents/alice/files/../alice-evil/secret.md", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("B001 sibling-prefix bypass NOT blocked. got %d body=%s",
			w.Code, w.Body.String())
	}
}

func TestB001_RejectsRelativeEscape(t *testing.T) {
	mgr, _ := setupSecurityTestEnv(t)
	r := newTestRouter(mgr)
	req := httptest.NewRequest(http.MethodGet,
		"/api/agents/alice/files/../../etc/passwd", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("../../etc/passwd not blocked: got %d", w.Code)
	}
}

// Symlink within workspace pointing OUT of workspace must be blocked.
func TestB001_RejectsSymlinkEscape(t *testing.T) {
	mgr, aliceWS := setupSecurityTestEnv(t)
	// Create a symlink inside alice's workspace pointing to /etc.
	linkPath := filepath.Join(aliceWS, "evil-link")
	if err := os.Symlink("/etc", linkPath); err != nil {
		t.Skip("symlink unsupported: " + err.Error())
	}
	r := newTestRouter(mgr)
	req := httptest.NewRequest(http.MethodGet,
		"/api/agents/alice/files/evil-link/passwd", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("symlink escape not blocked: got %d body=%s", w.Code, w.Body.String())
	}
}

func TestB001_AllowsLegitimateAccess(t *testing.T) {
	mgr, _ := setupSecurityTestEnv(t)
	r := newTestRouter(mgr)
	req := httptest.NewRequest(http.MethodGet,
		"/api/agents/alice/files/ok.md", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("legit read failed: got %d body=%s", w.Code, w.Body.String())
	}
}

// Write attack with sibling-prefix should also be blocked.
func TestB001_WriteRejectsSiblingPrefixBypass(t *testing.T) {
	mgr, _ := setupSecurityTestEnv(t)
	r := newTestRouter(mgr)
	req := httptest.NewRequest(http.MethodPut,
		"/api/agents/alice/files/../alice-evil/poison.md", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("write to sibling not blocked: got %d", w.Code)
	}
}

// Delete attack — same defense.
func TestB001_DeleteRejectsSiblingPrefixBypass(t *testing.T) {
	mgr, _ := setupSecurityTestEnv(t)
	r := newTestRouter(mgr)
	req := httptest.NewRequest(http.MethodDelete,
		"/api/agents/alice/files/../alice-evil/secret.md", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("delete sibling file not blocked: got %d", w.Code)
	}
	_ = mgr // confirm secret survived would require touching internal layout; the 403 is enough
}
