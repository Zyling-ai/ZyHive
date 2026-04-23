// internal/api/usage.go — Usage statistics API handlers.
package api

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/session"
	"github.com/Zyling-ai/zyhive/pkg/usage"
)

type usageHandler struct {
	store   *usage.Store
	manager *agent.Manager
	// sessionTitleCache is short-lived memoization: per-request we read each
	// (agentID, sessionID) at most once from disk instead of per-row.
	titleCacheMu sync.Mutex
}

func newUsageHandler(store *usage.Store, mgr *agent.Manager) *usageHandler {
	return &usageHandler{store: store, manager: mgr}
}

// parsetime reads a Unix-seconds query param; returns 0 on missing/invalid.
func parsetime(c *gin.Context, key string) int64 {
	v := c.Query(key)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// GET /api/usage/summary?from=&to=&agentId=&provider=
func (h *usageHandler) Summary(c *gin.Context) {
	from := parsetime(c, "from")
	to := parsetime(c, "to")
	agentID := c.Query("agentId")
	provider := c.Query("provider")
	sum := h.store.Summarize(from, to, agentID, provider)
	c.JSON(http.StatusOK, sum)
}

// GET /api/usage/timeline?from=&to=&agentId=&provider=
func (h *usageHandler) Timeline(c *gin.Context) {
	from := parsetime(c, "from")
	to := parsetime(c, "to")
	agentID := c.Query("agentId")
	provider := c.Query("provider")
	pts := h.store.Timeline(from, to, agentID, provider)
	if pts == nil {
		pts = []usage.TimelinePoint{}
	}
	c.JSON(http.StatusOK, gin.H{"points": pts})
}

// enrichedRecord is a Record augmented with human-readable fields for the UI:
// session title and agent display name. These are looked up on the fly; empty
// when the underlying session/agent has been deleted.
type enrichedRecord struct {
	usage.Record
	AgentName    string `json:"agentName,omitempty"`
	SessionTitle string `json:"sessionTitle,omitempty"`
}

// GET /api/usage/records?from=&to=&agentId=&sessionId=&provider=&model=&page=&pageSize=
func (h *usageHandler) Records(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	params := usage.QueryParams{
		From:      parsetime(c, "from"),
		To:        parsetime(c, "to"),
		AgentID:   c.Query("agentId"),
		SessionID: c.Query("sessionId"),
		Provider:  c.Query("provider"),
		Model:     c.Query("model"),
		Page:      page,
		PageSize:  pageSize,
	}
	result := h.store.Query(params)

	// Enrich with agentName / sessionTitle for the UI.
	enriched := make([]enrichedRecord, 0, len(result.Records))
	agentNameCache := make(map[string]string)
	titleCache := make(map[string]string) // agentID|sessionID → title
	h.titleCacheMu.Lock()
	defer h.titleCacheMu.Unlock()
	for _, r := range result.Records {
		er := enrichedRecord{Record: r}
		if r.AgentID != "" && h.manager != nil {
			if name, ok := agentNameCache[r.AgentID]; ok {
				er.AgentName = name
			} else if ag, found := h.manager.Get(r.AgentID); found {
				agentNameCache[r.AgentID] = ag.Name
				er.AgentName = ag.Name
			} else {
				agentNameCache[r.AgentID] = ""
			}
		}
		if r.SessionID != "" && r.AgentID != "" && h.manager != nil {
			key := r.AgentID + "|" + r.SessionID
			if t, ok := titleCache[key]; ok {
				er.SessionTitle = t
			} else {
				title := lookupSessionTitle(h.manager, r.AgentID, r.SessionID)
				titleCache[key] = title
				er.SessionTitle = title
			}
		}
		enriched = append(enriched, er)
	}

	// Return both total and enriched list.
	c.JSON(http.StatusOK, gin.H{
		"total":   result.Total,
		"records": enriched,
	})
}

// lookupSessionTitle reads the session index for the given agent and returns
// the title (empty if not found or title not yet auto-generated).
func lookupSessionTitle(mgr *agent.Manager, agentID, sessionID string) string {
	ag, ok := mgr.Get(agentID)
	if !ok {
		return ""
	}
	store := session.NewStore(ag.SessionDir)
	meta, ok := store.GetMeta(sessionID)
	if !ok {
		return ""
	}
	return meta.Title
}
