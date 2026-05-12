// internal/api/approvals_e2e_test.go — end-to-end test for the F-01 approval
// flow. Verifies the full wire: REST + SSE + Broker + AuditHook all working
// together, against a real httptest server.
//
// This test is the high-signal "does it actually work?" gate. If this passes,
// the production binary's approval flow is functionally correct.

package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/tools"
	"github.com/gin-gonic/gin"
)

// buildApprovalTestRouter creates a minimal gin engine wired only with the
// approval endpoints we want to exercise. We deliberately don't bring up the
// full RegisterRoutes (and its many manager dependencies) — keeps this test
// fast and focused.
func buildApprovalTestRouter() (*gin.Engine, *tools.Broker, *[]string, *sync.Mutex) {
	gin.SetMode(gin.ReleaseMode)
	auditEvents := []string{}
	var mu sync.Mutex

	hook := tools.AuditHook(func(req tools.ApprovalRequest, dec tools.ApprovalDecision, ev string) {
		mu.Lock()
		auditEvents = append(auditEvents, ev)
		mu.Unlock()
	})
	broker := tools.NewBroker(hook)
	SetApprovalBroker(broker)

	r := gin.New()
	apH := &approvalHandler{}
	r.GET("/api/approvals/pending", apH.ListPending)
	r.POST("/api/approvals/:id/approve", apH.Approve)
	r.POST("/api/approvals/:id/deny", apH.Deny)
	r.GET("/api/approvals/stream", apH.Stream)

	return r, broker, &auditEvents, &mu
}

// readSSELine reads a single SSE "data: <json>" payload from the reader,
// skipping comments and blank lines. Returns the JSON bytes or err.
func readSSELine(t *testing.T, r *bufio.Reader, deadline time.Duration) ([]byte, error) {
	t.Helper()
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				ch <- result{nil, err}
				return
			}
			line = bytes.TrimRight(line, "\r\n")
			// Skip comments (": ping") and blank lines.
			if len(line) == 0 || bytes.HasPrefix(line, []byte(":")) {
				continue
			}
			if bytes.HasPrefix(line, []byte("data: ")) {
				ch <- result{bytes.TrimPrefix(line, []byte("data: ")), nil}
				return
			}
		}
	}()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-time.After(deadline):
		return nil, fmt.Errorf("timeout waiting for SSE line")
	}
}

// TestApprovalE2E_ApproveFlow runs the full happy path:
//
//   1. SSE client subscribes
//   2. (concurrently) broker.Request fires → SSE delivers "approval_request"
//   3. (parallel) REST POST /api/approvals/:id/approve → 200
//   4. broker.Request returns Approved=true
//   5. SSE delivers "approval_resolved"
//   6. audit hook saw "approval_approved"
//   7. /api/approvals/pending is empty
func TestApprovalE2E_ApproveFlow(t *testing.T) {
	r, broker, audit, mu := buildApprovalTestRouter()
	ts := httptest.NewServer(r)
	defer ts.Close()

	// 1. Open SSE stream.
	req, _ := http.NewRequest("GET", ts.URL+"/api/approvals/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE dial: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("SSE status %d", resp.StatusCode)
	}
	br := bufio.NewReader(resp.Body)

	// Consume "hello" frame.
	hello, err := readSSELine(t, br, 2*time.Second)
	if err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if !bytes.Contains(hello, []byte(`"hello"`)) {
		t.Errorf("expected hello frame, got: %s", hello)
	}

	// 2. Fire broker.Request in a goroutine.
	type reqResult struct {
		dec tools.ApprovalDecision
		err error
	}
	requestDone := make(chan reqResult, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		dec, _, err := broker.Request(ctx, "agent_e2e", "ses_e2e", "exec",
			json.RawMessage(`{"cmd":"ls"}`), 4*time.Second)
		requestDone <- reqResult{dec, err}
	}()

	// 3. Wait for the SSE "approval_request" event.
	requestFrame, err := readSSELine(t, br, 3*time.Second)
	if err != nil {
		t.Fatalf("read approval_request: %v", err)
	}
	var reqEvent struct {
		Type    string `json:"type"`
		Request struct {
			ID       string `json:"id"`
			ToolName string `json:"toolName"`
			AgentID  string `json:"agentId"`
		} `json:"request"`
	}
	if err := json.Unmarshal(requestFrame, &reqEvent); err != nil {
		t.Fatalf("parse approval_request: %v\nframe: %s", err, requestFrame)
	}
	if reqEvent.Type != "approval_request" {
		t.Fatalf("expected approval_request, got %q", reqEvent.Type)
	}
	if reqEvent.Request.ToolName != "exec" {
		t.Errorf("wrong toolName: %s", reqEvent.Request.ToolName)
	}
	if reqEvent.Request.AgentID != "agent_e2e" {
		t.Errorf("wrong agentId: %s", reqEvent.Request.AgentID)
	}
	apID := reqEvent.Request.ID
	if apID == "" {
		t.Fatal("empty approval id")
	}

	// 4. POST /api/approvals/:id/approve
	body := strings.NewReader(`{"reason":"go ahead","by":"e2e-test"}`)
	approveReq, _ := http.NewRequest("POST",
		ts.URL+"/api/approvals/"+apID+"/approve", body)
	approveReq.Header.Set("Content-Type", "application/json")
	approveResp, err := http.DefaultClient.Do(approveReq)
	if err != nil {
		t.Fatalf("approve POST: %v", err)
	}
	approveResp.Body.Close()
	if approveResp.StatusCode != 200 {
		t.Errorf("approve status %d", approveResp.StatusCode)
	}

	// 5. broker.Request should return Approved=true.
	select {
	case rr := <-requestDone:
		if rr.err != nil {
			t.Errorf("Request err: %v", rr.err)
		}
		if !rr.dec.Approved {
			t.Errorf("expected approved, got: %+v", rr.dec)
		}
		if rr.dec.By != "e2e-test" {
			t.Errorf("By: %s", rr.dec.By)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Request did not return within 3s of approve")
	}

	// 6. SSE should deliver "approval_resolved".
	resolvedFrame, err := readSSELine(t, br, 2*time.Second)
	if err != nil {
		t.Fatalf("read approval_resolved: %v", err)
	}
	var resolved struct {
		Type     string `json:"type"`
		ID       string `json:"id"`
		Decision struct {
			Approved bool   `json:"approved"`
			By       string `json:"by"`
		} `json:"decision"`
	}
	if err := json.Unmarshal(resolvedFrame, &resolved); err != nil {
		t.Fatalf("parse resolved: %v", err)
	}
	if resolved.Type != "approval_resolved" {
		t.Errorf("expected approval_resolved, got %s", resolved.Type)
	}
	if resolved.ID != apID {
		t.Errorf("resolved id mismatch: %s vs %s", resolved.ID, apID)
	}
	if !resolved.Decision.Approved {
		t.Errorf("resolved.approved should be true")
	}

	// 7. Audit hook should have fired "approval_approved".
	mu.Lock()
	defer mu.Unlock()
	if len(*audit) != 1 || (*audit)[0] != "approval_approved" {
		t.Errorf("audit events: %v (want [approval_approved])", *audit)
	}

	// 8. /pending should be empty.
	pendingResp, err := http.Get(ts.URL + "/api/approvals/pending")
	if err != nil {
		t.Fatal(err)
	}
	defer pendingResp.Body.Close()
	var listed struct {
		Count int `json:"count"`
	}
	_ = json.NewDecoder(pendingResp.Body).Decode(&listed)
	if listed.Count != 0 {
		t.Errorf("pending should be empty, got %d", listed.Count)
	}
}

// TestApprovalE2E_DenyFlow mirrors the approve flow but rejects, verifies the
// REST 200 + broker decision + audit "approval_denied".
func TestApprovalE2E_DenyFlow(t *testing.T) {
	r, broker, audit, mu := buildApprovalTestRouter()
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Fire request first, then deny via REST.
	type reqResult struct {
		dec tools.ApprovalDecision
	}
	out := make(chan reqResult, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		dec, _, _ := broker.Request(ctx, "agent_e2e", "ses", "exec",
			json.RawMessage(`{}`), 4*time.Second)
		out <- reqResult{dec}
	}()

	// Wait for pending to appear, then deny.
	var apID string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pendingResp, err := http.Get(ts.URL + "/api/approvals/pending")
		if err != nil {
			t.Fatal(err)
		}
		var listed struct {
			Pending []struct {
				ID string `json:"id"`
			} `json:"pending"`
		}
		_ = json.NewDecoder(pendingResp.Body).Decode(&listed)
		pendingResp.Body.Close()
		if len(listed.Pending) == 1 {
			apID = listed.Pending[0].ID
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if apID == "" {
		t.Fatal("approval did not appear in /pending")
	}

	body := strings.NewReader(`{"reason":"not safe","by":"e2e"}`)
	denyReq, _ := http.NewRequest("POST",
		ts.URL+"/api/approvals/"+apID+"/deny", body)
	denyReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(denyReq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("deny status %d", resp.StatusCode)
	}

	rr := <-out
	if rr.dec.Approved {
		t.Errorf("expected denied")
	}
	if rr.dec.Reason != "not safe" {
		t.Errorf("Reason: %s", rr.dec.Reason)
	}

	// Give the audit hook a beat to fire (it runs sync after Decide).
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, e := range *audit {
		if e == "approval_denied" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("audit events missing approval_denied: %v", *audit)
	}
}

// TestApprovalE2E_DecideUnknown — POST /api/approvals/missing/approve → 404
func TestApprovalE2E_DecideUnknownReturns404(t *testing.T) {
	r, _, _, _ := buildApprovalTestRouter()
	ts := httptest.NewServer(r)
	defer ts.Close()

	body := strings.NewReader(`{}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/approvals/missing/approve", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}
