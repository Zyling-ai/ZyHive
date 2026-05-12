// internal/api/approvals.go — REST + SSE endpoints for the tool approval broker.
//
// Endpoints (all behind auth middleware):
//
//	GET  /api/approvals/pending           — list all pending (optional ?agentId=)
//	POST /api/approvals/:id/approve       — body: {reason?, by?}
//	POST /api/approvals/:id/deny          — body: {reason?, by?}
//	GET  /api/approvals/stream            — SSE feed of approval events (admin)
//
// Process-global broker is injected by main.go via SetApprovalBroker. When
// nil the endpoints return 503.
//
// Added 26.5.12v1 (F-01).

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/tools"
	"github.com/gin-gonic/gin"
)

// globalApprovalBroker is the process-wide approval broker injected from main.go.
// Nil before SetApprovalBroker is called or when feature is disabled.
var globalApprovalBroker *tools.Broker

// SetApprovalBroker wires the broker so handlers + chat.go can reach it.
// Should be called once at startup before RegisterRoutes.
func SetApprovalBroker(b *tools.Broker) {
	globalApprovalBroker = b
}

// ApprovalBroker returns the global broker (may be nil).
func ApprovalBroker() *tools.Broker {
	return globalApprovalBroker
}

// approvalTimeout returns the configured (or default) per-request timeout.
func approvalTimeout() time.Duration {
	return tools.DefaultApprovalTimeout
}

type approvalHandler struct{}

func (h *approvalHandler) need(c *gin.Context) (*tools.Broker, bool) {
	if globalApprovalBroker == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "approval broker not initialised"})
		return nil, false
	}
	return globalApprovalBroker, true
}

// GET /api/approvals/pending?agentId=...
func (h *approvalHandler) ListPending(c *gin.Context) {
	b, ok := h.need(c)
	if !ok {
		return
	}
	pending := b.ListPending(c.Query("agentId"))
	c.JSON(http.StatusOK, gin.H{"pending": pending, "count": len(pending)})
}

// POST /api/approvals/:id/approve  body: {reason?, by?}
func (h *approvalHandler) Approve(c *gin.Context) {
	b, ok := h.need(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
		return
	}
	var body struct {
		Reason string `json:"reason"`
		By     string `json:"by"`
	}
	raw, _ := io.ReadAll(io.LimitReader(c.Request.Body, 8*1024))
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	dec := tools.ApprovalDecision{Approved: true, Reason: body.Reason, By: defaultBy(body.By, c)}
	if err := b.Decide(id, dec); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/approvals/:id/deny  body: {reason?, by?}
func (h *approvalHandler) Deny(c *gin.Context) {
	b, ok := h.need(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
		return
	}
	var body struct {
		Reason string `json:"reason"`
		By     string `json:"by"`
	}
	raw, _ := io.ReadAll(io.LimitReader(c.Request.Body, 8*1024))
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	dec := tools.ApprovalDecision{Approved: false, Reason: body.Reason, By: defaultBy(body.By, c)}
	if err := b.Decide(id, dec); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

var approvalSSECounter atomic.Uint64

// GET /api/approvals/stream — SSE feed for the admin "🛡 审批中心" view.
// Each line is a JSON Event {type, request|id, decision}.
func (h *approvalHandler) Stream(c *gin.Context) {
	b, ok := h.need(c)
	if !ok {
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	subID := fmt.Sprintf("admin-%d", approvalSSECounter.Add(1))
	ch, unsub := b.Subscribe(subID)
	defer unsub()

	// Initial heartbeat so clients know the stream is alive.
	_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", `{"type":"hello"}`)
	c.Writer.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	c.Stream(func(w io.Writer) bool {
		select {
		case ev, ok := <-ch:
			if !ok {
				return false
			}
			data, err := json.Marshal(ev)
			if err != nil {
				return true
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			return true
		case <-heartbeat.C:
			fmt.Fprintf(w, ": ping\n\n")
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

func defaultBy(by string, c *gin.Context) string {
	if by != "" {
		return by
	}
	// Best-effort: use the auth Bearer token's last 6 chars as an identifier.
	if h := c.GetHeader("Authorization"); len(h) > 16 {
		return "tok:" + h[len(h)-6:]
	}
	return "anon"
}
