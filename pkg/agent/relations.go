package agent

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// TeamRelation represents a directional relationship edge between two agents.
type TeamRelation struct {
	From string // agent ID that owns this relation entry
	To   string // target agent ID
	Type string // "上下级" | "平级协作" | "支持" | "其他"
}

// EligibleTarget is an agent the caller can interact with in the given mode.
type EligibleTarget struct {
	AgentID  string `json:"agentId"`
	Relation string `json:"relation"` // relation type
	Mode     string `json:"mode"`     // "task" | "report"
}

// GetAllRelations reads RELATIONS.md from every agent's workspace and returns
// all directed relation edges. Edges are stored in the workspace of the FROM agent.
func (m *Manager) GetAllRelations() []TeamRelation {
	m.mu.RLock()
	agents := make([]*Agent, 0, len(m.agents))
	for _, a := range m.agents {
		agents = append(agents, a)
	}
	m.mu.RUnlock()

	var out []TeamRelation
	for _, ag := range agents {
		rows := readRelationsFile(filepath.Join(ag.WorkspaceDir, "RELATIONS.md"))
		for _, r := range rows {
			out = append(out, TeamRelation{
				From: ag.ID,
				To:   r.AgentID,
				Type: r.RelationType,
			})
		}
	}
	return out
}

// EligibleTargets returns agents that fromAgent can assign tasks to or report to.
// mode: "task" | "report"
//
// Permission rules:
//   task delegation:
//     - from has "上下级" pointing TO to → from is superior, may delegate
//     - from has "平级协作" pointing TO to → peers may delegate mutually
//   reporting:
//     - to has "上下级" pointing TO from → to is superior of from, from may report to to
//     - from has "平级协作" pointing TO to → peers may report to each other
func (m *Manager) EligibleTargets(fromID, mode string) []EligibleTarget {
	rels := m.GetAllRelations()

	seen := make(map[string]EligibleTarget)

	for _, r := range rels {
		switch mode {
		case "task":
			// from delegates to subordinates (from→to 上下级) or peers (平级协作)
			if r.From == fromID && (r.Type == "上下级" || r.Type == "平级协作") {
				if _, ok := seen[r.To]; !ok {
					seen[r.To] = EligibleTarget{AgentID: r.To, Relation: r.Type, Mode: mode}
				}
			}
		case "report":
			// from reports to superiors: superior has 上下级 pointing TO from
			if r.To == fromID && r.Type == "上下级" {
				if _, ok := seen[r.From]; !ok {
					seen[r.From] = EligibleTarget{AgentID: r.From, Relation: r.Type, Mode: mode}
				}
			}
			// peer reporting (both directions)
			if r.From == fromID && r.Type == "平级协作" {
				if _, ok := seen[r.To]; !ok {
					seen[r.To] = EligibleTarget{AgentID: r.To, Relation: r.Type, Mode: mode}
				}
			}
		}
	}

	result := make([]EligibleTarget, 0, len(seen))
	for _, v := range seen {
		result = append(result, v)
	}
	return result
}

// CanSpawn checks if fromAgent may interact with toAgent in the given mode.
// Returns (allowed, relationLabel, error).
func (m *Manager) CanSpawn(fromID, toID, mode string) (bool, string) {
	targets := m.EligibleTargets(fromID, mode)
	for _, t := range targets {
		if t.AgentID == toID {
			return true, t.Relation
		}
	}
	return false, ""
}

// ─── RELATIONS.md parser (local, no import cycle) ─────────────────────────

type relRow struct {
	AgentID      string
	RelationType string
}

func readRelationsFile(path string) []relRow {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var rows []relRow
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		// Table row: | agentId | ... | relationType | ... |
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cols := strings.Split(line, "|")
		if len(cols) < 4 {
			continue
		}
		agentID := strings.TrimSpace(cols[1])
		if agentID == "" || agentID == "agentId" || strings.HasPrefix(agentID, "---") {
			continue
		}
		relType := ""
		if len(cols) >= 4 {
			relType = strings.TrimSpace(cols[3])
		}
		if agentID != "" && relType != "" {
			rows = append(rows, relRow{AgentID: agentID, RelationType: relType})
		}
	}
	return rows
}
