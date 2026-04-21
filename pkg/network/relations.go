package network

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// RelationLine is a parsed row from RELATIONS.md — enough to render INDEX.md.
type RelationLine struct {
	To          string // target id (agent id or contact id)
	ToKind      string // "agent" (default) | "contact"
	DisplayName string // resolved at render time (agent name or contact displayName)
	Type        string
	Strength    string
	Desc        string
}

// RelationsPath returns the absolute path of network/RELATIONS.md.
func (s *Store) RelationsPath() string {
	return filepath.Join(s.Dir(), "RELATIONS.md")
}

// readRelationsRowsUnlocked parses RELATIONS.md into RelationLine slices for
// INDEX.md rendering. Unknown/malformed lines are skipped.
//
// Expected markdown table (new schema):
//
//	| to | toKind | type | strength | desc |
//	| --- | --- | --- | --- | --- |
//	| abao | agent | 平级协作 | 常用 | ... |
//	| feishu:ou_boss | contact | 服务 | 强 | ... |
//
// Backward compat: if the row has only 4 pipe-separated cells (legacy
// "to/type/strength/desc" without toKind), we fall back to toKind=agent.
func (s *Store) readRelationsRowsUnlocked() []RelationLine {
	data, err := os.ReadFile(s.RelationsPath())
	if err != nil {
		return nil
	}
	var out []RelationLine
	scan := bufio.NewScanner(strings.NewReader(string(data)))
	scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || !strings.HasPrefix(line, "|") {
			continue
		}
		if strings.HasPrefix(line, "| ---") || strings.HasPrefix(line, "|---") {
			continue
		}
		cells := splitMDRow(line)
		if len(cells) == 0 {
			continue
		}
		// Detect header row by checking first cell.
		if isHeaderRow(cells) {
			continue
		}
		r := parseRelationRow(cells)
		if r.To == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

func splitMDRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

func isHeaderRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	first := strings.ToLower(cells[0])
	return first == "to" || first == "agentid" || first == "id"
}

func parseRelationRow(cells []string) RelationLine {
	var r RelationLine
	switch len(cells) {
	case 4:
		// Legacy: to | type | strength | desc
		r.To = cells[0]
		r.ToKind = "agent"
		r.Type = cells[1]
		r.Strength = cells[2]
		r.Desc = cells[3]
	case 5:
		r.To = cells[0]
		kind := strings.ToLower(cells[1])
		if kind == "" {
			kind = "agent"
		}
		r.ToKind = kind
		r.Type = cells[2]
		r.Strength = cells[3]
		r.Desc = cells[4]
	default:
		if len(cells) >= 5 {
			r.To = cells[0]
			r.ToKind = strings.ToLower(cells[1])
			r.Type = cells[2]
			r.Strength = cells[3]
			r.Desc = cells[4]
		}
	}
	r.DisplayName = r.To // default; caller may override when rendering
	return r
}

// filterRelations returns rows matching the given kind ("agent" or "contact").
func filterRelations(rows []RelationLine, kind string) []RelationLine {
	var out []RelationLine
	for _, r := range rows {
		rk := r.ToKind
		if rk == "" {
			rk = "agent"
		}
		if rk == kind {
			out = append(out, r)
		}
	}
	return out
}
