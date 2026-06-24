package skillopt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Zyling-ai/zyhive/pkg/skill"
)

// CallLLMForAgent runs one system+user completion using a specific agent's model.
// Production wires pkg/agent.Pool.CallLLMOnce here; tests inject a fake.
type CallLLMForAgent func(ctx context.Context, agentID, system, user string) (string, error)

// Manager is the orchestration entry point for SkillOpt maintenance, shared by
// the cron sentinel and the REST API.
type Manager struct {
	callLLM CallLLMForAgent
}

// NewManager builds a Manager. callLLM may be nil in contexts that never evolve
// (e.g. read-only API queries), but RunMaintenance/Evolve will then fail.
func NewManager(callLLM CallLLMForAgent) *Manager {
	return &Manager{callLLM: callLLM}
}

// AggregateLessonsFile is the workspace-relative file rebuilt after maintenance
// and injected (lightweight, truncated) into the system prompt.
const AggregateLessonsFile = "SKILLOPT_LESSONS.md"

// Overview is the API status payload for one skill's evolution state.
type Overview struct {
	SkillID         string  `json:"skillId"`
	Initialized     bool    `json:"initialized"`
	Epoch           int     `json:"epoch"`
	BaselineVersion int     `json:"baselineVersion"`
	ShadowVersion   int     `json:"shadowVersion"`
	ShadowActive    bool    `json:"shadowActive"`
	AutoAccept      bool    `json:"autoAccept"`
	MaintenanceEnabled bool `json:"maintenanceEnabled"`
	SampleThreshold int     `json:"sampleThreshold"`
	PromoteMargin   float64 `json:"promoteMargin"`
	ShadowMinSample int     `json:"shadowMinSample"`
	HitRateBaseline float64 `json:"hitRateBaseline"`
	HitRateShadow   float64 `json:"hitRateShadow"`
	TotalSamples    int     `json:"totalSamples"`
	BackfilledSamples int   `json:"backfilledSamples"`
	PendingOracle   int     `json:"pendingOracle"`
	SinceEvolveSamples int  `json:"sinceEvolveSamples"`
	PendingProposals int    `json:"pendingProposals"`
}

// GetOverview computes the current evolution status for a skill.
func (m *Manager) GetOverview(workspaceDir, skillID string) (Overview, error) {
	s := NewStore(workspaceDir, skillID)
	ov := Overview{SkillID: skillID, Initialized: s.Exists()}
	ep, err := s.ReadEpoch()
	if err != nil {
		return ov, err
	}
	all, err := s.AllEntries()
	if err != nil {
		return ov, err
	}
	backfilled := 0
	for _, e := range all {
		if e.Hit != nil {
			backfilled++
		}
	}
	since, _ := s.backfilledSince(ep.LastEvolvedAt, false)
	baseRate, _ := s.HitRate(false)
	pending, _ := s.PendingOracle()
	props, _ := s.ListProposals()
	pendingProps := 0
	for _, p := range props {
		if p.Status == StatusPending {
			pendingProps++
		}
	}

	ov.Epoch = ep.CurrentEpoch
	ov.BaselineVersion = ep.BaselineVersion
	ov.ShadowVersion = ep.ShadowVersion
	ov.ShadowActive = ep.ShadowVersion > 0
	ov.AutoAccept = ep.AutoAccept
	ov.MaintenanceEnabled = ep.CronJobID != ""
	ov.SampleThreshold = ep.SampleThreshold
	ov.PromoteMargin = ep.PromoteMargin
	ov.ShadowMinSample = ep.ShadowMinSample
	ov.HitRateBaseline = baseRate
	ov.HitRateShadow = ep.HitRateShadow
	ov.TotalSamples = len(all)
	ov.BackfilledSamples = backfilled
	ov.PendingOracle = len(pending)
	ov.SinceEvolveSamples = len(since)
	ov.PendingProposals = pendingProps
	return ov, nil
}

// RunMaintenance runs one full maintenance pass for a single skill:
// evaluate any active shadow canary, then (if clear) attempt a slow-cadence
// evolve, then refresh display metadata + the aggregate lessons file.
func (m *Manager) RunMaintenance(ctx context.Context, agentID, workspaceDir, skillID string) (string, error) {
	s := NewStore(workspaceDir, skillID)
	if err := s.Init(); err != nil {
		return "", err
	}
	cb := m.boundLLM(agentID)
	var notes []string

	if verdict, err := EvaluateShadow(s); err != nil {
		notes = append(notes, "影子评估失败: "+err.Error())
	} else if verdict != "" {
		notes = append(notes, verdict)
	}

	prop, err := MaybeEvolve(ctx, s, cb, false)
	if err != nil {
		notes = append(notes, "进化跳过: "+err.Error())
	} else if prop != nil {
		notes = append(notes, "生成进化提案 "+prop.ID)
		if ep, e := s.ReadEpoch(); e == nil && ep.AutoAccept {
			if e2 := AcceptProposal(s, prop.ID); e2 == nil {
				notes = append(notes, "已自动接受 "+prop.ID+" 进入影子灰度")
			} else {
				notes = append(notes, "自动接受失败: "+e2.Error())
			}
		}
	}

	m.syncMeta(workspaceDir, skillID)
	_ = m.RebuildAggregateLessons(workspaceDir)

	if len(notes) == 0 {
		return "无变更", nil
	}
	return strings.Join(notes, "；"), nil
}

// RunMaintenanceAll runs maintenance for every initialised skill of an agent.
func (m *Manager) RunMaintenanceAll(ctx context.Context, agentID, workspaceDir string) (string, error) {
	ids, err := initializedSkillIDs(workspaceDir)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "无可维护技能", nil
	}
	var notes []string
	for _, id := range ids {
		out, err := m.RunMaintenance(ctx, agentID, workspaceDir, id)
		if err != nil {
			notes = append(notes, fmt.Sprintf("[%s] 错误: %v", id, err))
			continue
		}
		notes = append(notes, fmt.Sprintf("[%s] %s", id, out))
	}
	return strings.Join(notes, "\n"), nil
}

// Evolve forces an immediate evolve attempt for a skill (manual API trigger).
func (m *Manager) Evolve(ctx context.Context, agentID, workspaceDir, skillID string) (*Proposal, error) {
	s := NewStore(workspaceDir, skillID)
	if err := s.Init(); err != nil {
		return nil, err
	}
	prop, err := MaybeEvolve(ctx, s, m.boundLLM(agentID), true)
	if err != nil {
		return nil, err
	}
	m.syncMeta(workspaceDir, skillID)
	return prop, nil
}

func (m *Manager) boundLLM(agentID string) CallLLM {
	return func(ctx context.Context, system, user string) (string, error) {
		if m.callLLM == nil {
			return "", fmt.Errorf("skillopt: no LLM caller configured")
		}
		return m.callLLM(ctx, agentID, system, user)
	}
}

// SetSkillEvolving flips the skill's display-only Evolving flag (best-effort).
func (m *Manager) SetSkillEvolving(workspaceDir, skillID string, evolving bool) {
	meta, err := skill.ReadSkill(workspaceDir, skillID)
	if err != nil {
		return
	}
	meta.Evolving = evolving
	if evolving {
		s := NewStore(workspaceDir, skillID)
		if ep, err := s.ReadEpoch(); err == nil {
			meta.Epoch = ep.CurrentEpoch
		}
		rate, _ := s.HitRate(false)
		meta.HitRate = rate
	}
	_ = skill.WriteSkill(workspaceDir, *meta)
}

// syncMeta refreshes the skill's display-only meta fields (best-effort).
func (m *Manager) syncMeta(workspaceDir, skillID string) {
	meta, err := skill.ReadSkill(workspaceDir, skillID)
	if err != nil {
		return
	}
	s := NewStore(workspaceDir, skillID)
	ep, err := s.ReadEpoch()
	if err != nil {
		return
	}
	rate, _ := s.HitRate(false)
	meta.Evolving = true
	meta.Epoch = ep.CurrentEpoch
	meta.HitRate = rate
	_ = skill.WriteSkill(workspaceDir, *meta)
}

// RebuildAggregateLessons regenerates skills/SKILLOPT_LESSONS.md from every
// evolving skill's lessons, for lightweight system-prompt injection. The file
// is removed when no skill has lessons yet.
func (m *Manager) RebuildAggregateLessons(workspaceDir string) error {
	ids, err := initializedSkillIDs(workspaceDir)
	if err != nil {
		return err
	}
	var sb strings.Builder
	wrote := false
	for _, id := range ids {
		s := NewStore(workspaceDir, id)
		lessons, _ := s.ReadLessons()
		lessons = strings.TrimSpace(lessons)
		if lessons == "" {
			continue
		}
		rate, n := s.HitRate(false)
		name := id
		if meta, err := skill.ReadSkill(workspaceDir, id); err == nil && meta.Name != "" {
			name = meta.Name
		}
		if !wrote {
			sb.WriteString("# 技能进化教训（SkillOpt 自动维护，请优先遵守）\n\n")
			wrote = true
		}
		sb.WriteString(fmt.Sprintf("## %s（命中率 %.0f%% · %d 样本）\n%s\n\n", name, rate*100, n, lessons))
	}

	path := filepath.Join(workspaceDir, "skills", AggregateLessonsFile)
	if !wrote {
		_ = os.Remove(path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(sb.String()))
}

// initializedSkillIDs returns skill ids that have a skillopt/epoch.json.
func initializedSkillIDs(workspaceDir string) ([]string, error) {
	dir := filepath.Join(workspaceDir, "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if NewStore(workspaceDir, e.Name()).Exists() {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}
