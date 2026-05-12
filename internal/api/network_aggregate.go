// internal/api/network_aggregate.go — cross-agent aggregation of contacts
// and chats. Endpoints:
//
//	GET /api/network/contacts?source=&q=&tag=&limit=
//	GET /api/network/chats?source=&q=&tag=&limit=
//
// Same canonical ID ("{source}:{externalId}") seen across multiple agents
// collapses into a single row with a perAgent[] breakdown. The endpoint
// names live OUTSIDE the /api/agents/:id namespace because the result spans
// all agents.
//
// Added 26.5.12v1 (B-03).

package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/network"
	"github.com/gin-gonic/gin"
)

type aggregateHandler struct {
	manager *agent.Manager
}

// ── Aggregated DTOs ────────────────────────────────────────────────────────

// ContactPerAgent is the per-agent footprint of a globally-identified contact.
type ContactPerAgent struct {
	AgentID     string    `json:"agentId"`
	AgentName   string    `json:"agentName"`
	AvatarColor string    `json:"avatarColor,omitempty"`
	DisplayName string    `json:"displayName"`
	Tags        []string  `json:"tags,omitempty"`
	MsgCount    int       `json:"msgCount"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
	IsOwner     bool      `json:"isOwner,omitempty"`
	HasAvatar   bool      `json:"hasAvatar,omitempty"`
}

// AggregatedContact is one row in the cross-agent contacts view.
//
// DisplayName / Tags are picked from the per-agent row with the highest
// MsgCount (so the "best-known" rendition wins). All tags from every agent
// are union-merged for filtering convenience.
type AggregatedContact struct {
	ID            string            `json:"id"`
	Source        string            `json:"source"`
	DisplayName   string            `json:"displayName"`
	Tags          []string          `json:"tags,omitempty"`
	TotalMsgCount int               `json:"totalMsgCount"`
	LastSeenAt    time.Time         `json:"lastSeenAt"`
	PerAgent      []ContactPerAgent `json:"perAgent"`
}

// ChatPerAgent is the per-agent footprint of a globally-identified group chat.
type ChatPerAgent struct {
	AgentID     string    `json:"agentId"`
	AgentName   string    `json:"agentName"`
	AvatarColor string    `json:"avatarColor,omitempty"`
	Title       string    `json:"title"`
	Kind        string    `json:"kind"`
	MemberCount int       `json:"memberCount,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	MsgCount    int       `json:"msgCount"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
}

// AggregatedChat is one row in the cross-agent chats view.
type AggregatedChat struct {
	ID            string         `json:"id"`
	Source        string         `json:"source"`
	Title         string         `json:"title"`
	Kind          string         `json:"kind"`
	Tags          []string       `json:"tags,omitempty"`
	TotalMsgCount int            `json:"totalMsgCount"`
	LastSeenAt    time.Time      `json:"lastSeenAt"`
	PerAgent      []ChatPerAgent `json:"perAgent"`
}

// ── HTTP handlers ─────────────────────────────────────────────────────────

const (
	defaultAggregateLimit = 200
	maxAggregateLimit     = 1000
)

func parseLimit(c *gin.Context, def, max int) int {
	v := c.Query("limit")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

// ListAllContacts GET /api/network/contacts?source=&q=&tag=&limit=
func (h *aggregateHandler) ListAllContacts(c *gin.Context) {
	source := c.Query("source")
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	tag := strings.TrimSpace(c.Query("tag"))
	limit := parseLimit(c, defaultAggregateLimit, maxAggregateLimit)

	agents := h.manager.List()
	rows := aggregateContacts(agents, h.manager)

	// Filter
	filtered := rows[:0]
	for _, r := range rows {
		if source != "" && r.Source != source {
			continue
		}
		if tag != "" && !containsStr(r.Tags, tag) {
			continue
		}
		if q != "" && !matchContactQ(r, q) {
			continue
		}
		filtered = append(filtered, r)
	}

	// Order: lastSeenAt desc, secondary totalMsgCount desc.
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].LastSeenAt.Equal(filtered[j].LastSeenAt) {
			return filtered[i].TotalMsgCount > filtered[j].TotalMsgCount
		}
		return filtered[i].LastSeenAt.After(filtered[j].LastSeenAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	c.JSON(http.StatusOK, gin.H{
		"contacts": filtered,
		"total":    len(filtered),
	})
}

// ListAllChats GET /api/network/chats?source=&q=&tag=&limit=
func (h *aggregateHandler) ListAllChats(c *gin.Context) {
	source := c.Query("source")
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	tag := strings.TrimSpace(c.Query("tag"))
	limit := parseLimit(c, defaultAggregateLimit, maxAggregateLimit)

	agents := h.manager.List()
	rows := aggregateChats(agents, h.manager)

	filtered := rows[:0]
	for _, r := range rows {
		if source != "" && r.Source != source {
			continue
		}
		if tag != "" && !containsStr(r.Tags, tag) {
			continue
		}
		if q != "" && !matchChatQ(r, q) {
			continue
		}
		filtered = append(filtered, r)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].LastSeenAt.Equal(filtered[j].LastSeenAt) {
			return filtered[i].TotalMsgCount > filtered[j].TotalMsgCount
		}
		return filtered[i].LastSeenAt.After(filtered[j].LastSeenAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	c.JSON(http.StatusOK, gin.H{
		"chats": filtered,
		"total": len(filtered),
	})
}

// ── Aggregation core (exported for testability) ───────────────────────────

// aggregateContacts walks every agent concurrently and groups by canonical ID.
// Exposed as a free function to make it trivial to test without HTTP plumbing.
func aggregateContacts(agents []*agent.Agent, mgr *agent.Manager) []AggregatedContact {
	type collectedRow struct {
		ag      *agent.Agent
		summary network.ContactSummary
	}

	var (
		mu       sync.Mutex
		rows     []collectedRow
		wg       sync.WaitGroup
	)
	for _, ag := range agents {
		wg.Add(1)
		go func(ag *agent.Agent) {
			defer wg.Done()
			s := network.NewStore(ag.WorkspaceDir)
			list, err := s.List()
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, sm := range list {
				rows = append(rows, collectedRow{ag: ag, summary: sm})
			}
		}(ag)
	}
	wg.Wait()

	// Group by canonical ID.
	byID := make(map[string]*AggregatedContact)
	for _, r := range rows {
		id := r.summary.ID
		ac, ok := byID[id]
		if !ok {
			ac = &AggregatedContact{
				ID:     id,
				Source: r.summary.Source,
			}
			byID[id] = ac
		}
		ac.PerAgent = append(ac.PerAgent, ContactPerAgent{
			AgentID:     r.ag.ID,
			AgentName:   r.ag.Name,
			AvatarColor: r.ag.AvatarColor,
			DisplayName: r.summary.DisplayName,
			Tags:        append([]string{}, r.summary.Tags...),
			MsgCount:    r.summary.MsgCount,
			LastSeenAt:  r.summary.LastSeenAt,
			IsOwner:     r.summary.IsOwner,
			HasAvatar:   r.summary.HasAvatar,
		})
		ac.TotalMsgCount += r.summary.MsgCount
		if r.summary.LastSeenAt.After(ac.LastSeenAt) {
			ac.LastSeenAt = r.summary.LastSeenAt
		}
		// Union-merge tags.
		for _, t := range r.summary.Tags {
			if !containsStr(ac.Tags, t) {
				ac.Tags = append(ac.Tags, t)
			}
		}
	}

	// Pick canonical DisplayName from the perAgent with highest MsgCount; tie-break
	// by most recent LastSeenAt.
	for _, ac := range byID {
		var best ContactPerAgent
		for _, p := range ac.PerAgent {
			if p.DisplayName == "" {
				continue
			}
			if p.MsgCount > best.MsgCount || (p.MsgCount == best.MsgCount && p.LastSeenAt.After(best.LastSeenAt)) {
				best = p
			}
		}
		if best.DisplayName == "" && len(ac.PerAgent) > 0 {
			// Fall back to first perAgent's name even if empty (UI will still show ID).
			best = ac.PerAgent[0]
		}
		ac.DisplayName = best.DisplayName
		// Order perAgent: msgCount desc.
		sort.Slice(ac.PerAgent, func(i, j int) bool {
			return ac.PerAgent[i].MsgCount > ac.PerAgent[j].MsgCount
		})
	}

	out := make([]AggregatedContact, 0, len(byID))
	for _, ac := range byID {
		out = append(out, *ac)
	}
	return out
}

func aggregateChats(agents []*agent.Agent, mgr *agent.Manager) []AggregatedChat {
	type collectedRow struct {
		ag      *agent.Agent
		summary network.ChatSummary
	}
	var (
		mu   sync.Mutex
		rows []collectedRow
		wg   sync.WaitGroup
	)
	for _, ag := range agents {
		wg.Add(1)
		go func(ag *agent.Agent) {
			defer wg.Done()
			s := network.NewStore(ag.WorkspaceDir)
			list, err := s.ListChats()
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, sm := range list {
				rows = append(rows, collectedRow{ag: ag, summary: sm})
			}
		}(ag)
	}
	wg.Wait()

	byID := make(map[string]*AggregatedChat)
	for _, r := range rows {
		id := r.summary.ID
		ac, ok := byID[id]
		if !ok {
			ac = &AggregatedChat{
				ID:     id,
				Source: r.summary.Source,
			}
			byID[id] = ac
		}
		ac.PerAgent = append(ac.PerAgent, ChatPerAgent{
			AgentID:     r.ag.ID,
			AgentName:   r.ag.Name,
			AvatarColor: r.ag.AvatarColor,
			Title:       r.summary.Title,
			Kind:        r.summary.Kind,
			MemberCount: r.summary.MemberCount,
			Tags:        append([]string{}, r.summary.Tags...),
			MsgCount:    r.summary.MsgCount,
			LastSeenAt:  r.summary.LastSeenAt,
		})
		ac.TotalMsgCount += r.summary.MsgCount
		if r.summary.LastSeenAt.After(ac.LastSeenAt) {
			ac.LastSeenAt = r.summary.LastSeenAt
		}
		for _, t := range r.summary.Tags {
			if !containsStr(ac.Tags, t) {
				ac.Tags = append(ac.Tags, t)
			}
		}
	}

	for _, ac := range byID {
		var bestTitle, bestKind string
		var bestMembers, bestMsgs int
		var bestSeen time.Time
		for _, p := range ac.PerAgent {
			if p.MsgCount > bestMsgs || (p.MsgCount == bestMsgs && p.LastSeenAt.After(bestSeen)) {
				bestMsgs = p.MsgCount
				bestSeen = p.LastSeenAt
				if p.Title != "" {
					bestTitle = p.Title
				}
				if p.Kind != "" {
					bestKind = p.Kind
				}
				if p.MemberCount > bestMembers {
					bestMembers = p.MemberCount
				}
			}
		}
		if bestTitle == "" {
			for _, p := range ac.PerAgent {
				if p.Title != "" {
					bestTitle = p.Title
					break
				}
			}
		}
		ac.Title = bestTitle
		ac.Kind = bestKind
		sort.Slice(ac.PerAgent, func(i, j int) bool {
			return ac.PerAgent[i].MsgCount > ac.PerAgent[j].MsgCount
		})
	}

	out := make([]AggregatedChat, 0, len(byID))
	for _, ac := range byID {
		out = append(out, *ac)
	}
	return out
}

// ── Filters ───────────────────────────────────────────────────────────────

func matchContactQ(r AggregatedContact, q string) bool {
	if strings.Contains(strings.ToLower(r.DisplayName), q) {
		return true
	}
	if strings.Contains(strings.ToLower(r.ID), q) {
		return true
	}
	for _, t := range r.Tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	for _, pa := range r.PerAgent {
		if strings.Contains(strings.ToLower(pa.DisplayName), q) {
			return true
		}
	}
	return false
}

func matchChatQ(r AggregatedChat, q string) bool {
	if strings.Contains(strings.ToLower(r.Title), q) {
		return true
	}
	if strings.Contains(strings.ToLower(r.ID), q) {
		return true
	}
	for _, t := range r.Tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	for _, pa := range r.PerAgent {
		if strings.Contains(strings.ToLower(pa.Title), q) {
			return true
		}
	}
	return false
}
