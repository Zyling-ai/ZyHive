// pkg/usage/store.go — append-only JSONL usage records, one file per month.
package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Record captures one LLM API call.
type Record struct {
	ID           string  `json:"id"`
	AgentID      string  `json:"agent_id"`
	SessionID    string  `json:"session_id,omitempty"` // 26.4.23v3+; empty for legacy rows
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Cost         float64 `json:"cost"`   // USD, estimated
	CreatedAt    int64   `json:"created_at"` // Unix seconds
}

// Store writes and reads usage JSONL files under dir/.usage/YYYY-MM.jsonl
type Store struct {
	dir string
	mu  sync.Mutex
}

// NewStore creates a Store rooted at dir (typically the workspace dir).
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) usageDir() string { return filepath.Join(s.dir, ".usage") }

func (s *Store) monthFile(t time.Time) string {
	return filepath.Join(s.usageDir(), t.UTC().Format("2006-01")+".jsonl")
}

// Append writes one record to the current month's JSONL file.
func (s *Store) Append(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.usageDir(), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.monthFile(time.Now()), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(r)
}

// QueryParams filters for Query().
type QueryParams struct {
	From     int64  // Unix seconds, 0 = no lower bound
	To       int64  // Unix seconds, 0 = no upper bound
	AgentID   string // "" = all
	SessionID string // "" = all (26.4.23v3+)
	Provider  string // "" = all
	Model     string // "" = all
	Page     int    // 1-based
	PageSize int    // default 50
}

// QueryResult is returned by Query().
type QueryResult struct {
	Records []Record `json:"records"`
	Total   int      `json:"total"`
}

// Query returns paginated records matching params (newest first).
func (s *Store) Query(p QueryParams) QueryResult {
	if p.PageSize <= 0 {
		p.PageSize = 50
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	all := s.readRange(p.From, p.To)
	// filter
	filtered := all[:0]
	for _, r := range all {
		if p.SessionID != "" && r.SessionID != p.SessionID {
			continue
		}
		if p.AgentID != "" && r.AgentID != p.AgentID {
			continue
		}
		if p.Provider != "" && r.Provider != p.Provider {
			continue
		}
		if p.Model != "" && !strings.Contains(strings.ToLower(r.Model), strings.ToLower(p.Model)) {
			continue
		}
		filtered = append(filtered, r)
	}
	// sort newest first
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].CreatedAt > filtered[j].CreatedAt })
	total := len(filtered)
	start := (p.Page - 1) * p.PageSize
	if start >= total {
		return QueryResult{Records: []Record{}, Total: total}
	}
	end := start + p.PageSize
	if end > total {
		end = total
	}
	return QueryResult{Records: filtered[start:end], Total: total}
}

// Summary aggregates usage over a time range.
type Summary struct {
	TotalCalls    int     `json:"total_calls"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	TotalTokens   int     `json:"total_tokens"`
	TotalCost     float64 `json:"total_cost"`
	ByProvider    map[string]*BucketStat `json:"by_provider"`
	ByAgent       map[string]*BucketStat `json:"by_agent"`
	ByModel       map[string]*BucketStat `json:"by_model"`
}

type BucketStat struct {
	Calls        int     `json:"calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	Cost         float64 `json:"cost"`
}

// Summarize returns aggregate stats for the given filters.
func (s *Store) Summarize(from, to int64, agentID, provider string) Summary {
	records := s.readRange(from, to)
	sum := Summary{
		ByProvider: map[string]*BucketStat{},
		ByAgent:    map[string]*BucketStat{},
		ByModel:    map[string]*BucketStat{},
	}
	for _, r := range records {
		if agentID != "" && r.AgentID != agentID {
			continue
		}
		if provider != "" && r.Provider != provider {
			continue
		}
		sum.TotalCalls++
		sum.InputTokens += r.InputTokens
		sum.OutputTokens += r.OutputTokens
		sum.TotalTokens += r.InputTokens + r.OutputTokens
		sum.TotalCost += r.Cost
		addBucket(sum.ByProvider, r.Provider, r)
		addBucket(sum.ByAgent, r.AgentID, r)
		addBucket(sum.ByModel, r.Model, r)
	}
	return sum
}

// TimelinePoint is one data point in a time-series response.
type TimelinePoint struct {
	Date         string  `json:"date"`
	Calls        int     `json:"calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Cost         float64 `json:"cost"`
}

// Timeline returns daily aggregated stats.
func (s *Store) Timeline(from, to int64, agentID, provider string) []TimelinePoint {
	records := s.readRange(from, to)
	dayMap := map[string]*TimelinePoint{}
	for _, r := range records {
		if agentID != "" && r.AgentID != agentID {
			continue
		}
		if provider != "" && r.Provider != provider {
			continue
		}
		day := time.Unix(r.CreatedAt, 0).UTC().Format("2006-01-02")
		pt := dayMap[day]
		if pt == nil {
			pt = &TimelinePoint{Date: day}
			dayMap[day] = pt
		}
		pt.Calls++
		pt.InputTokens += r.InputTokens
		pt.OutputTokens += r.OutputTokens
		pt.Cost += r.Cost
	}
	// sort chronologically
	points := make([]TimelinePoint, 0, len(dayMap))
	for _, pt := range dayMap {
		points = append(points, *pt)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Date < points[j].Date })
	return points
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func addBucket(m map[string]*BucketStat, key string, r Record) {
	if key == "" {
		key = "unknown"
	}
	b := m[key]
	if b == nil {
		b = &BucketStat{}
		m[key] = b
	}
	b.Calls++
	b.InputTokens += r.InputTokens
	b.OutputTokens += r.OutputTokens
	b.TotalTokens += r.InputTokens + r.OutputTokens
	b.Cost += r.Cost
}

// readRange reads all records from months overlapping [from,to].
func (s *Store) readRange(from, to int64) []Record {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, _ := os.ReadDir(s.usageDir())
	var records []Record
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(s.usageDir(), e.Name())
		rs := readJSONL(path)
		for _, r := range rs {
			if from > 0 && r.CreatedAt < from {
				continue
			}
			if to > 0 && r.CreatedAt > to {
				continue
			}
			records = append(records, r)
		}
	}
	return records
}

func readJSONL(path string) []Record {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r Record
		if json.Unmarshal(line, &r) == nil {
			out = append(out, r)
		}
	}
	return out
}

// NewID generates a simple sortable ID.
func NewID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
