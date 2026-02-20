// Relations handler — RELATIONS.md read/write per agent + team graph aggregation.
package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
)

const relationsFilename = "RELATIONS.md"

type relationsHandler struct {
	manager *agent.Manager
}

// RelationRow is one row from RELATIONS.md.
type RelationRow struct {
	AgentID      string `json:"agentId"`
	AgentName    string `json:"agentName"`
	RelationType string `json:"relationType"`
	Strength     string `json:"strength"`
	Desc         string `json:"desc"`
}

// Get reads RELATIONS.md for an agent.
// GET /api/agents/:id/relations
func (h *relationsHandler) Get(c *gin.Context) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	filePath := filepath.Join(ag.WorkspaceDir, relationsFilename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{"content": "", "parsed": []RelationRow{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	content := string(data)
	parsed := parseRelationsMarkdown(content)
	c.JSON(http.StatusOK, gin.H{"content": content, "parsed": parsed})
}

// Put writes RELATIONS.md for an agent and syncs bidirectional relations.
// PUT /api/agents/:id/relations
func (h *relationsHandler) Put(c *gin.Context) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1*1024*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filePath := filepath.Join(ag.WorkspaceDir, relationsFilename)

	// Read old relations before overwriting (for diff)
	var oldRows []RelationRow
	if oldData, err2 := os.ReadFile(filePath); err2 == nil {
		oldRows = parseRelationsMarkdown(string(oldData))
	}

	// Write new content
	if err := os.WriteFile(filePath, body, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Compute diff and sync bidirectional relations
	newRows := parseRelationsMarkdown(string(body))
	h.syncBidirectional(ag, oldRows, newRows)

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// inverseRelationType returns the inverse of a relation type.
// 上级↔下级 are inverses; 平级协作 and 支持 are symmetric.
func inverseRelationType(t string) string {
	switch t {
	case "上级":
		return "下级"
	case "下级":
		return "上级"
	default:
		return t // 平级协作, 支持 → symmetric
	}
}

// syncBidirectional adds/removes inverse relations on peer agents.
func (h *relationsHandler) syncBidirectional(src *agent.Agent, oldRows, newRows []RelationRow) {
	// Build maps for quick lookup
	oldMap := make(map[string]RelationRow, len(oldRows))
	for _, r := range oldRows {
		oldMap[r.AgentID] = r
	}
	newMap := make(map[string]RelationRow, len(newRows))
	for _, r := range newRows {
		newMap[r.AgentID] = r
	}

	// Added relations → add inverse on peer
	for targetID, row := range newMap {
		if _, existed := oldMap[targetID]; !existed {
			h.addInverseRelation(targetID, RelationRow{
				AgentID:      src.ID,
				AgentName:    src.Name,
				RelationType: inverseRelationType(row.RelationType),
				Strength:     row.Strength,
				Desc:         row.Desc,
			})
		}
	}

	// Removed relations → remove inverse on peer
	for targetID := range oldMap {
		if _, stillExists := newMap[targetID]; !stillExists {
				h.removeInverseRelation(targetID, src.ID)
		}
	}
}

// addInverseRelation adds a relation row to targetAgentID's RELATIONS.md (if agent exists).
// Skips if the inverse already exists (dedup).
func (h *relationsHandler) addInverseRelation(targetAgentID string, row RelationRow) {
	target, ok := h.manager.Get(targetAgentID)
	if !ok {
		return // target agent not registered — skip
	}

	filePath := filepath.Join(target.WorkspaceDir, relationsFilename)
	var existing []RelationRow
	if data, err := os.ReadFile(filePath); err == nil {
		existing = parseRelationsMarkdown(string(data))
	}

	// Dedup: don't add if already present
	for _, r := range existing {
		if r.AgentID == row.AgentID {
			return
		}
	}

	existing = append(existing, row)
	_ = writeRelationsFile(filePath, existing)
}

// removeRelationEntry removes the row pointing to removeID from agentID's RELATIONS.md.
func (h *relationsHandler) removeRelationEntry(agentID, removeID string) {
	ag, ok := h.manager.Get(agentID)
	if !ok {
		return
	}
	filePath := filepath.Join(ag.WorkspaceDir, relationsFilename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	rows := parseRelationsMarkdown(string(data))
	filtered := rows[:0]
	for _, r := range rows {
		if r.AgentID != removeID {
			filtered = append(filtered, r)
		}
	}
	_ = writeRelationsFile(filePath, filtered)
}

// removeInverseRelation is kept for compatibility — now truly bidirectional:
// removes references to each other in BOTH agents' RELATIONS.md files.
func (h *relationsHandler) removeInverseRelation(targetAgentID, sourceAgentID string) {
	h.removeRelationEntry(targetAgentID, sourceAgentID) // remove source from target's file
	h.removeRelationEntry(sourceAgentID, targetAgentID) // remove target from source's file (safety)
}

// ClearAllRelations clears RELATIONS.md for every agent.
// DELETE /api/team/relations
func (h *relationsHandler) ClearAllRelations(c *gin.Context) {
	agents := h.manager.List()
	for _, ag := range agents {
		filePath := filepath.Join(ag.WorkspaceDir, relationsFilename)
		_ = os.WriteFile(filePath, []byte(""), 0644)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "cleared": len(agents)})
}

// writeRelationsFile serializes RelationRows as a markdown table and writes to disk.
func writeRelationsFile(filePath string, rows []RelationRow) error {
	if len(rows) == 0 {
		return os.WriteFile(filePath, []byte(""), 0644)
	}
	var sb strings.Builder
	sb.WriteString("| 成员ID | 成员名称 | 关系类型 | 关系程度 | 说明 |\n")
	sb.WriteString("|--------|--------|--------|--------|------|\n")
	for _, r := range rows {
		sb.WriteString("| ")
		sb.WriteString(r.AgentID)
		sb.WriteString(" | ")
		sb.WriteString(r.AgentName)
		sb.WriteString(" | ")
		sb.WriteString(r.RelationType)
		sb.WriteString(" | ")
		sb.WriteString(r.Strength)
		sb.WriteString(" | ")
		sb.WriteString(r.Desc)
		sb.WriteString(" |\n")
	}
	return os.WriteFile(filePath, []byte(sb.String()), 0644)
}

// Graph aggregates RELATIONS.md from all agents and returns graph data.
// GET /api/team/graph
func (h *relationsHandler) Graph(c *gin.Context) {
	agents := h.manager.List()

	type GraphNode struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	type GraphEdge struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Type     string `json:"type"`
		Strength string `json:"strength"`
		Label    string `json:"label"`
	}

	nodeMap := make(map[string]GraphNode)
	edgeMap := make(map[string]GraphEdge) // canonical key → edge (dedup)

	for _, ag := range agents {
		nodeMap[ag.ID] = GraphNode{ID: ag.ID, Name: ag.Name, Status: ag.Status}

		filePath := filepath.Join(ag.WorkspaceDir, relationsFilename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		rows := parseRelationsMarkdown(string(data))
		for _, row := range rows {
			// Ensure target node is present (may not be a registered agent)
			if _, exists := nodeMap[row.AgentID]; !exists {
				nodeMap[row.AgentID] = GraphNode{ID: row.AgentID, Name: row.AgentName, Status: "idle"}
			}

			from := ag.ID
			to := row.AgentID

			// Canonical key to dedup bidirectional declarations
			a, b := from, to
			if a > b {
				a, b = b, a
			}
			edgeKey := a + "|" + b

			if _, exists := edgeMap[edgeKey]; !exists {
				edgeMap[edgeKey] = GraphEdge{
					From:     from,
					To:       to,
					Type:     row.RelationType,
					Strength: row.Strength,
					Label:    row.Desc,
				}
			}
		}
	}

	nodes := make([]GraphNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	// Sort nodes by ID for stable, deterministic layout on every refresh.
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	edges := make([]GraphEdge, 0, len(edgeMap))
	for _, e := range edgeMap {
		edges = append(edges, e)
	}
	// Sort edges for stable render order.
	sort.Slice(edges, func(i, j int) bool {
		ki := edges[i].From + "→" + edges[i].To
		kj := edges[j].From + "→" + edges[j].To
		return ki < kj
	})

	if nodes == nil {
		nodes = []GraphNode{}
	}
	if edges == nil {
		edges = []GraphEdge{}
	}

	c.JSON(http.StatusOK, gin.H{"nodes": nodes, "edges": edges})
}

// parseRelationsMarkdown scans a RELATIONS.md and extracts relation rows.
// Supports two formats:
//   New (markdown table): | agentId | agentName | type | strength | desc |
//   Old (pipe-separated): agentId | agentName | type | strength | desc
func parseRelationsMarkdown(content string) []RelationRow {
	var rows []RelationRow
	seen := make(map[string]bool)

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Must contain at least one pipe
		if !strings.Contains(line, "|") {
			continue
		}

		parts := strings.Split(line, "|")
		var cols []string
		for _, p := range parts {
			cols = append(cols, strings.TrimSpace(p))
		}
		// Remove leading/trailing empty strings from outer pipes (markdown table format)
		if len(cols) > 0 && cols[0] == "" {
			cols = cols[1:]
		}
		if len(cols) > 0 && cols[len(cols)-1] == "" {
			cols = cols[:len(cols)-1]
		}

		if len(cols) < 4 {
			continue
		}

		first := cols[0]
		if first == "" {
			continue
		}

		// Skip header rows
		if strings.Contains(first, "成员") || strings.Contains(first, "Member") ||
			strings.Contains(first, "ID") || strings.Contains(first, "id") {
			continue
		}

		// Skip separator rows (---, :--:, etc.)
		stripped := strings.Trim(first, "-: ")
		if stripped == "" {
			continue
		}

		agentName := ""
		relationType := ""
		strength := ""
		desc := ""
		if len(cols) > 1 {
			agentName = cols[1]
		}
		if len(cols) > 2 {
			relationType = cols[2]
		}
		if len(cols) > 3 {
			strength = cols[3]
		}
		if len(cols) > 4 {
			desc = cols[4]
		}

		// Validate: relationType must be one of the known values
		validTypes := map[string]bool{"上级": true, "下级": true, "平级协作": true, "支持": true}
		if relationType != "" && !validTypes[relationType] {
			continue
		}

		// Dedup by agentId
		if seen[first] {
			continue
		}
		seen[first] = true

		rows = append(rows, RelationRow{
			AgentID:      first,
			AgentName:    agentName,
			RelationType: relationType,
			Strength:     strength,
			Desc:         desc,
		})
	}

	return rows
}
