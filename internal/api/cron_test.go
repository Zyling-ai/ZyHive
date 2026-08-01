package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/cron"
	"github.com/gin-gonic/gin"
)

func TestCronCreateReturnsBadRequestForInvalidSchedule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &cronHandler{engine: cron.NewEngine(t.TempDir(), nil, nil)}
	router := gin.New()
	router.POST("/cron", handler.Create)

	body := `{
		"name":"invalid",
		"enabled":true,
		"schedule":{"kind":"cron","expr":"invalid","tz":"UTC"},
		"payload":{"kind":"agentTurn","message":"hello"},
		"delivery":{"mode":"none"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/cron", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestCronCreateRejectsUnknownNonEmptyAgentIDAndAllowsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := cron.NewEngine(t.TempDir(), nil, nil)
	handler := &cronHandler{engine: engine, manager: agent.NewManager(t.TempDir())}
	router := gin.New()
	router.POST("/cron", handler.Create)

	base := `{
		"name":"job",
		"enabled":true,
		"schedule":{"kind":"every","everyMs":60000},
		"payload":{"kind":"agentTurn","message":"hello"},
		"delivery":{"mode":"none"}`
	unknown := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cron", strings.NewReader(base+`,"agentId":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(unknown, req)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown agent status = %d, body = %s", unknown.Code, unknown.Body.String())
	}
	if len(engine.ListJobs()) != 0 {
		t.Fatal("unknown agent job must not be persisted")
	}

	global := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/cron", strings.NewReader(base+`,"agentId":""}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(global, req)
	if global.Code != http.StatusCreated {
		t.Fatalf("empty agentId status = %d, body = %s", global.Code, global.Body.String())
	}
}

func TestCronRunReturnsConflictForOverlap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	engine := cron.NewEngine(t.TempDir(), func(ctx context.Context, _, _, _, _, _ string) (string, error) {
		started <- struct{}{}
		select {
		case <-release:
			return "done", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}, nil)
	t.Cleanup(func() { <-engine.Stop().Done() })
	job := &cron.Job{
		Name:     "overlap",
		Enabled:  true,
		Schedule: cron.Schedule{Kind: "every", EveryMs: 60_000},
		Payload:  cron.Payload{Kind: "agentTurn", Message: "hello"},
		Delivery: cron.Delivery{Mode: "none"},
	}
	if err := engine.Add(job); err != nil {
		t.Fatal(err)
	}
	handler := &cronHandler{engine: engine}
	router := gin.New()
	router.POST("/cron/:jobId/run", handler.Run)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/cron/"+job.ID+"/run", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first run status = %d", first.Code)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first run did not start")
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/cron/"+job.ID+"/run", nil))
	if second.Code != http.StatusConflict {
		t.Fatalf("second run status = %d, body = %s", second.Code, second.Body.String())
	}
	close(release)
}
