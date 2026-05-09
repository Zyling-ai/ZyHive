package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/session"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestReadyz_NilDeps — handler tolerates nil cronEngine/workerPool (e.g. in
// partial-init or test environments) and reports them as ok with hint.
func TestReadyz_NilDeps(t *testing.T) {
	llm.ClearPingCache()

	h := &readyzHandler{cronEngine: nil, workerPool: nil}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	h.Handle(c)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 on nil deps cold start, got %d (body=%s)", w.Code, w.Body.String())
	}
	var resp struct {
		Ready  bool                   `json:"ready"`
		Checks map[string]readyzCheck `json:"checks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Ready {
		t.Fatalf("ready should be true with nil deps + cold start, got %+v", resp)
	}
	for name, ck := range resp.Checks {
		if !ck.OK {
			t.Fatalf("check %q unexpectedly failed: %+v", name, ck)
		}
	}
}

// TestReadyz_AllProvidersFailing — when cache shows probed providers and ALL
// of them are failing, /readyz returns 503 with provider check ok=false.
func TestReadyz_AllProvidersFailing(t *testing.T) {
	llm.ClearPingCache()
	t.Cleanup(llm.ClearPingCache)

	// Seed the cache with a failed probe by invoking the package's own probe
	// using an unsupported provider name; that short-circuits at the dispatch
	// layer with no network I/O but still populates the cache.
	r := llm.Ping(context.Background(), "no-such-provider", "fake-key", "http://127.0.0.1:1", false)
	if r.OK {
		t.Fatalf("seed probe should not be OK; got %+v", r)
	}

	pool := session.NewWorkerPool()
	t.Cleanup(pool.StopAll)

	h := &readyzHandler{cronEngine: nil, workerPool: pool}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	h.Handle(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 with all probes failing, got %d (body=%s)", w.Code, w.Body.String())
	}
	var resp struct {
		Ready  bool                   `json:"ready"`
		Checks map[string]readyzCheck `json:"checks"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Ready {
		t.Fatalf("ready should be false")
	}
	if resp.Checks["providers"].OK {
		t.Fatalf("providers check should be ok=false, got %+v", resp.Checks["providers"])
	}
}

// TestPingSnapshot_Empty — snapshot is empty before any probes.
func TestPingSnapshot_Empty(t *testing.T) {
	llm.ClearPingCache()
	if got := llm.PingSnapshot(); len(got) != 0 {
		t.Fatalf("expected empty snapshot, got %d entries", len(got))
	}
}

// TestPingSnapshot_AfterProbe — snapshot reflects cache contents.
func TestPingSnapshot_AfterProbe(t *testing.T) {
	llm.ClearPingCache()
	t.Cleanup(llm.ClearPingCache)

	r := llm.Ping(context.Background(), "no-such-provider", "fake-key", "http://127.0.0.1:1", false)
	if r.OK {
		t.Fatalf("seed probe should not be OK")
	}
	snap := llm.PingSnapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 snapshot entry, got %d", len(snap))
	}
	if snap[0].OK {
		t.Fatalf("snapshot[0].OK should be false")
	}
	if snap[0].AgeSeconds < 0 {
		t.Fatalf("AgeSeconds should be >=0, got %d", snap[0].AgeSeconds)
	}
	// CheckedAt should be very recent.
	if time.Since(snap[0].CheckedAt) > 5*time.Second {
		t.Fatalf("CheckedAt %v looks stale", snap[0].CheckedAt)
	}
}

