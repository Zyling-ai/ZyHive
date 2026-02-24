// Package goal provides the Goals & Planning system for ZyHive.
package goal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	cronpkg "github.com/Zyling-ai/zyhive/pkg/cron"
)

// CronAdder is the subset of cron.Engine used by the goal manager.
type CronAdder interface {
	Add(job *cronpkg.Job) error
	Remove(id string) error
	RunNow(id string) error
}

// Manager manages goals, persisting to <dataDir>/goals.json.
type Manager struct {
	dataDir    string
	goals      map[string]*Goal
	mu         sync.RWMutex
	cronEngine CronAdder
}

// NewManager creates a new goal manager.
func NewManager(dataDir string, cronEngine CronAdder) *Manager {
	return &Manager{
		dataDir:    dataDir,
		goals:      make(map[string]*Goal),
		cronEngine: cronEngine,
	}
}

// Load reads goals.json from disk.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(m.dataDir, "goals-checks"), 0755); err != nil {
		return err
	}

	data, err := os.ReadFile(filepath.Join(m.dataDir, "goals.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var goals []*Goal
	if err := json.Unmarshal(data, &goals); err != nil {
		return fmt.Errorf("parse goals.json: %w", err)
	}
	for _, g := range goals {
		if g.Milestones == nil {
			g.Milestones = []Milestone{}
		}
		if g.AgentIDs == nil {
			g.AgentIDs = []string{}
		}
		if g.Checks == nil {
			g.Checks = []GoalCheck{}
		}
		m.goals[g.ID] = g
	}
	return nil
}

// List returns all goals.
func (m *Manager) List() []*Goal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Goal, 0, len(m.goals))
	for _, g := range m.goals {
		result = append(result, g)
	}
	return result
}

// ListByAgent returns goals that include the given agentID.
func (m *Manager) ListByAgent(agentID string) []*Goal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Goal, 0)
	for _, g := range m.goals {
		for _, id := range g.AgentIDs {
			if id == agentID {
				result = append(result, g)
				break
			}
		}
	}
	return result
}

// Get returns a single goal by ID.
func (m *Manager) Get(id string) (*Goal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.goals[id]
	if !ok {
		return nil, fmt.Errorf("goal %q not found", id)
	}
	return g, nil
}

// Create saves a new goal. If StartAt/EndAt are set, creates at-type cron stubs.
func (m *Manager) Create(g *Goal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if g.ID == "" {
		g.ID = "goal-" + uuid.New().String()[:8]
	}
	now := time.Now()
	g.CreatedAt = now
	g.UpdatedAt = now
	if g.Status == "" {
		g.Status = StatusDraft
	}
	if g.Milestones == nil {
		g.Milestones = []Milestone{}
	}
	if g.AgentIDs == nil {
		g.AgentIDs = []string{}
	}
	if g.Checks == nil {
		g.Checks = []GoalCheck{}
	}

	// Cron integration: at-type is not yet supported by engine;
	// create disabled jobs, store IDs in goal for future activation.
	if !g.StartAt.IsZero() && m.cronEngine != nil {
		job := &cronpkg.Job{
			ID:      "gs-" + g.ID,
			Name:    "目标开始：" + g.Title,
			Enabled: false, // at-type not supported yet
			Schedule: cronpkg.Schedule{
				Kind: "at",
				Expr: g.StartAt.Format(time.RFC3339),
				TZ:   "Asia/Shanghai",
			},
			Payload: cronpkg.Payload{
				Kind:    "systemEvent",
				Message: fmt.Sprintf("目标「%s」已开始，请通知相关成员并开始推进。", g.Title),
			},
			Delivery: cronpkg.Delivery{Mode: "none"},
		}
		if err := m.cronEngine.Add(job); err == nil {
			g.StartCronID = job.ID
		}
	}
	if !g.EndAt.IsZero() && m.cronEngine != nil {
		job := &cronpkg.Job{
			ID:      "ge-" + g.ID,
			Name:    "目标截止：" + g.Title,
			Enabled: false,
			Schedule: cronpkg.Schedule{
				Kind: "at",
				Expr: g.EndAt.Format(time.RFC3339),
				TZ:   "Asia/Shanghai",
			},
			Payload: cronpkg.Payload{
				Kind:    "systemEvent",
				Message: fmt.Sprintf("目标「%s」已到截止日期，请检查完成情况。", g.Title),
			},
			Delivery: cronpkg.Delivery{Mode: "none"},
		}
		if err := m.cronEngine.Add(job); err == nil {
			g.EndCronID = job.ID
		}
	}

	m.goals[g.ID] = g
	return m.save()
}

// Update applies patch fields to an existing goal.
func (m *Manager) Update(id string, patch *Goal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.goals[id]
	if !ok {
		return fmt.Errorf("goal %q not found", id)
	}

	if patch.Title != "" {
		existing.Title = patch.Title
	}
	if patch.Description != "" {
		existing.Description = patch.Description
	}
	if patch.Type != "" {
		existing.Type = patch.Type
	}
	if patch.AgentIDs != nil {
		existing.AgentIDs = patch.AgentIDs
	}
	if patch.Status != "" {
		existing.Status = patch.Status
	}
	if patch.Progress > 0 {
		existing.Progress = patch.Progress
	}
	if !patch.StartAt.IsZero() {
		existing.StartAt = patch.StartAt
	}
	if !patch.EndAt.IsZero() {
		existing.EndAt = patch.EndAt
	}
	if patch.Milestones != nil {
		existing.Milestones = patch.Milestones
	}
	existing.UpdatedAt = time.Now()
	return m.save()
}

// UpdateProgress sets the goal's progress (0-100).
func (m *Manager) UpdateProgress(id string, progress int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	g, ok := m.goals[id]
	if !ok {
		return fmt.Errorf("goal %q not found", id)
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	g.Progress = progress
	g.UpdatedAt = time.Now()
	return m.save()
}

// SetMilestoneDone marks a milestone as done/undone.
func (m *Manager) SetMilestoneDone(goalID, milestoneID string, done bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	g, ok := m.goals[goalID]
	if !ok {
		return fmt.Errorf("goal %q not found", goalID)
	}
	for i, ms := range g.Milestones {
		if ms.ID == milestoneID {
			g.Milestones[i].Done = done
			g.UpdatedAt = time.Now()
			return m.save()
		}
	}
	return fmt.Errorf("milestone %q not found in goal %q", milestoneID, goalID)
}

// Delete removes a goal and its associated cron jobs.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	g, ok := m.goals[id]
	if !ok {
		return fmt.Errorf("goal %q not found", id)
	}
	if g.StartCronID != "" && m.cronEngine != nil {
		_ = m.cronEngine.Remove(g.StartCronID)
	}
	if g.EndCronID != "" && m.cronEngine != nil {
		_ = m.cronEngine.Remove(g.EndCronID)
	}
	for _, ch := range g.Checks {
		if ch.CronJobID != "" && m.cronEngine != nil {
			_ = m.cronEngine.Remove(ch.CronJobID)
		}
	}
	delete(m.goals, id)
	return m.save()
}

// AddCheck adds a periodic check plan to a goal and creates an associated cron job.
func (m *Manager) AddCheck(goalID string, check *GoalCheck) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	g, ok := m.goals[goalID]
	if !ok {
		return fmt.Errorf("goal %q not found", goalID)
	}

	if check.ID == "" {
		check.ID = "chk-" + uuid.New().String()[:8]
	}
	check.GoalID = goalID
	check.CreatedAt = time.Now()
	if check.TZ == "" {
		check.TZ = "Asia/Shanghai"
	}

	// Create cron job for this check
	if m.cronEngine != nil && check.Schedule != "" {
		prompt := buildCheckPrompt(check.Prompt, g)
		job := &cronpkg.Job{
			ID:      "goalchk-" + check.ID,
			Name:    fmt.Sprintf("目标检查[%s]：%s", g.Title, check.Name),
			Enabled: check.Enabled,
			Schedule: cronpkg.Schedule{
				Kind: "cron",
				Expr: check.Schedule,
				TZ:   check.TZ,
			},
			Payload: cronpkg.Payload{
				Kind:    "agentTurn",
				Message: prompt,
			},
			Delivery: cronpkg.Delivery{Mode: "announce"},
			AgentID:  check.AgentID,
		}
		if err := m.cronEngine.Add(job); err == nil {
			check.CronJobID = job.ID
		}
	}

	g.Checks = append(g.Checks, *check)
	g.UpdatedAt = time.Now()
	return m.save()
}

// UpdateCheck modifies an existing check plan (removes old cron, creates new).
func (m *Manager) UpdateCheck(goalID, checkID string, patch *GoalCheck) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	g, ok := m.goals[goalID]
	if !ok {
		return fmt.Errorf("goal %q not found", goalID)
	}
	for i, ch := range g.Checks {
		if ch.ID != checkID {
			continue
		}
		// Remove old cron job
		if ch.CronJobID != "" && m.cronEngine != nil {
			_ = m.cronEngine.Remove(ch.CronJobID)
			ch.CronJobID = ""
		}
		// Apply patch
		if patch.Name != "" {
			ch.Name = patch.Name
		}
		if patch.Schedule != "" {
			ch.Schedule = patch.Schedule
		}
		if patch.TZ != "" {
			ch.TZ = patch.TZ
		}
		if patch.AgentID != "" {
			ch.AgentID = patch.AgentID
		}
		if patch.Prompt != "" {
			ch.Prompt = patch.Prompt
		}
		ch.Enabled = patch.Enabled

		// Recreate cron job
		if m.cronEngine != nil && ch.Schedule != "" {
			prompt := buildCheckPrompt(ch.Prompt, g)
			job := &cronpkg.Job{
				ID:      "goalchk-" + ch.ID,
				Name:    fmt.Sprintf("目标检查[%s]：%s", g.Title, ch.Name),
				Enabled: ch.Enabled,
				Schedule: cronpkg.Schedule{
					Kind: "cron",
					Expr: ch.Schedule,
					TZ:   ch.TZ,
				},
				Payload: cronpkg.Payload{
					Kind:    "agentTurn",
					Message: prompt,
				},
				Delivery: cronpkg.Delivery{Mode: "announce"},
				AgentID:  ch.AgentID,
			}
			if err := m.cronEngine.Add(job); err == nil {
				ch.CronJobID = job.ID
			}
		}
		g.Checks[i] = ch
		g.UpdatedAt = time.Now()
		return m.save()
	}
	return fmt.Errorf("check %q not found in goal %q", checkID, goalID)
}

// RemoveCheck deletes a check plan and its cron job.
func (m *Manager) RemoveCheck(goalID, checkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	g, ok := m.goals[goalID]
	if !ok {
		return fmt.Errorf("goal %q not found", goalID)
	}
	for i, ch := range g.Checks {
		if ch.ID == checkID {
			if ch.CronJobID != "" && m.cronEngine != nil {
				_ = m.cronEngine.Remove(ch.CronJobID)
			}
			g.Checks = append(g.Checks[:i], g.Checks[i+1:]...)
			g.UpdatedAt = time.Now()
			return m.save()
		}
	}
	return fmt.Errorf("check %q not found in goal %q", checkID, goalID)
}

// RunCheckNow triggers a check immediately via its cron job.
func (m *Manager) RunCheckNow(goalID, checkID string) error {
	m.mu.RLock()
	g, ok := m.goals[goalID]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("goal %q not found", goalID)
	}
	var cronJobID string
	for _, ch := range g.Checks {
		if ch.ID == checkID {
			cronJobID = ch.CronJobID
			break
		}
	}
	m.mu.RUnlock()

	if cronJobID == "" {
		return fmt.Errorf("check %q has no associated cron job", checkID)
	}
	if m.cronEngine == nil {
		return fmt.Errorf("cron engine not available")
	}
	return m.cronEngine.RunNow(cronJobID)
}

// AppendCheckRecord appends a check execution record (JSONL format).
func (m *Manager) AppendCheckRecord(record CheckRecord) error {
	dir := filepath.Join(m.dataDir, "goals-checks")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, record.GoalID+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, _ := json.Marshal(record)
	fmt.Fprintf(f, "%s\n", data)
	return nil
}

// ListCheckRecords returns the last 50 check records for a goal.
func (m *Manager) ListCheckRecords(goalID string) ([]CheckRecord, error) {
	path := filepath.Join(m.dataDir, "goals-checks", goalID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []CheckRecord{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var records []CheckRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r CheckRecord
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		records = append(records, r)
	}
	if len(records) > 50 {
		records = records[len(records)-50:]
	}
	if records == nil {
		records = []CheckRecord{}
	}
	return records, nil
}

// save writes all goals to disk (must be called with mu held).
func (m *Manager) save() error {
	goals := make([]*Goal, 0, len(m.goals))
	for _, g := range m.goals {
		goals = append(goals, g)
	}
	data, err := json.MarshalIndent(goals, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.dataDir, "goals.json"), data, 0644)
}

// buildCheckPrompt replaces template variables in a prompt string.
func buildCheckPrompt(tmpl string, g *Goal) string {
	r := strings.NewReplacer(
		"{goal.title}",    g.Title,
		"{goal.progress}", fmt.Sprintf("%d%%", g.Progress),
		"{goal.endAt}",    g.EndAt.Format("2006-01-02"),
		"{goal.startAt}",  g.StartAt.Format("2006-01-02"),
		"{goal.status}",   string(g.Status),
	)
	return r.Replace(tmpl)
}
