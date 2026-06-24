package skillopt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Store is the on-disk gateway for one skill's evolution data, rooted at
//
//	{workspaceDir}/skills/{skillID}/skillopt/
//
// alongside the skill's own SKILL.md / skill.json.
type Store struct {
	workspaceDir string
	skillID      string
}

// NewStore builds a Store for one agent's skill.
func NewStore(workspaceDir, skillID string) *Store {
	return &Store{workspaceDir: workspaceDir, skillID: skillID}
}

// SkillID returns the skill id this store manages.
func (s *Store) SkillID() string { return s.skillID }

func (s *Store) skillDir() string {
	return filepath.Join(s.workspaceDir, "skills", s.skillID)
}

// Dir is the skillopt data directory for this skill.
func (s *Store) Dir() string { return filepath.Join(s.skillDir(), "skillopt") }

func (s *Store) ledgerPath() string   { return filepath.Join(s.Dir(), "ledger.jsonl") }
func (s *Store) epochPath() string    { return filepath.Join(s.Dir(), "epoch.json") }
func (s *Store) lessonsPath() string  { return filepath.Join(s.Dir(), "lessons.md") }
func (s *Store) versionsDir() string  { return filepath.Join(s.Dir(), "versions") }
func (s *Store) proposalsDir() string { return filepath.Join(s.Dir(), "proposals") }
func (s *Store) skillMDPath() string  { return filepath.Join(s.skillDir(), "SKILL.md") }

// ensure creates the skillopt directory tree (idempotent).
func (s *Store) ensure() error {
	for _, d := range []string{s.Dir(), s.versionsDir(), s.proposalsDir()} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

// Exists reports whether this skill has been initialised for evolution.
func (s *Store) Exists() bool {
	_, err := os.Stat(s.epochPath())
	return err == nil
}

// ── Epoch ───────────────────────────────────────────────────────────────────

// ReadEpoch loads epoch.json, returning DefaultEpoch() when absent.
func (s *Store) ReadEpoch() (EpochState, error) {
	data, err := os.ReadFile(s.epochPath())
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultEpoch(), nil
		}
		return EpochState{}, err
	}
	var e EpochState
	if err := json.Unmarshal(data, &e); err != nil {
		return EpochState{}, fmt.Errorf("parse epoch.json: %w", err)
	}
	// Backfill defaults for older/partial files.
	if e.CurrentEpoch == 0 {
		e.CurrentEpoch = 1
	}
	if e.BaselineVersion == 0 {
		e.BaselineVersion = 1
	}
	if e.SampleThreshold == 0 {
		e.SampleThreshold = DefaultSampleThreshold
	}
	if e.PromoteMargin == 0 {
		e.PromoteMargin = DefaultPromoteMargin
	}
	if e.ShadowMinSample == 0 {
		e.ShadowMinSample = DefaultShadowMinSample
	}
	return e, nil
}

// WriteEpoch persists epoch.json.
func (s *Store) WriteEpoch(e EpochState) error {
	if err := s.ensure(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.epochPath(), data)
}

// ── SKILL.md (live content) ──────────────────────────────────────────────────

// ReadSkillMD returns the live SKILL.md content ("" if missing).
func (s *Store) ReadSkillMD() (string, error) {
	data, err := os.ReadFile(s.skillMDPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// WriteSkillMD overwrites the live SKILL.md content.
func (s *Store) WriteSkillMD(content string) error {
	return writeFileAtomic(s.skillMDPath(), []byte(content))
}

// ── Version snapshots ────────────────────────────────────────────────────────

func (s *Store) versionPath(v int) string {
	return filepath.Join(s.versionsDir(), fmt.Sprintf("v%d-SKILL.md", v))
}

// SnapshotVersion writes versions/v{v}-SKILL.md (no-op if it already exists).
func (s *Store) SnapshotVersion(v int, content string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	p := s.versionPath(v)
	if _, err := os.Stat(p); err == nil {
		return nil // never clobber an existing snapshot
	}
	return writeFileAtomic(p, []byte(content))
}

// ReadVersion returns the snapshot content for a version.
func (s *Store) ReadVersion(v int) (string, error) {
	data, err := os.ReadFile(s.versionPath(v))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ListVersions returns available snapshot version numbers, ascending.
func (s *Store) ListVersions() ([]int, error) {
	entries, err := os.ReadDir(s.versionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []int{}, nil
		}
		return nil, err
	}
	var versions []int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "v") || !strings.HasSuffix(name, "-SKILL.md") {
			continue
		}
		numStr := strings.TrimSuffix(strings.TrimPrefix(name, "v"), "-SKILL.md")
		if n, err := strconv.Atoi(numStr); err == nil {
			versions = append(versions, n)
		}
	}
	sort.Ints(versions)
	if versions == nil {
		versions = []int{}
	}
	return versions, nil
}

// ── Proposals ────────────────────────────────────────────────────────────────

// WriteProposal persists a proposal as proposals/{id}.json.
func (s *Store) WriteProposal(p Proposal) error {
	if err := s.ensure(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(s.proposalsDir(), p.ID+".json"), data)
}

// ReadProposal loads a single proposal by id.
func (s *Store) ReadProposal(id string) (Proposal, error) {
	var p Proposal
	data, err := os.ReadFile(filepath.Join(s.proposalsDir(), id+".json"))
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("parse proposal %s: %w", id, err)
	}
	return p, nil
}

// ListProposals returns all proposals, newest first.
func (s *Store) ListProposals() ([]Proposal, error) {
	entries, err := os.ReadDir(s.proposalsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []Proposal{}, nil
		}
		return nil, err
	}
	var out []Proposal
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		p, err := s.ReadProposal(id)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if out == nil {
		out = []Proposal{}
	}
	return out, nil
}

// PendingProposal returns the first pending proposal, if any.
func (s *Store) PendingProposal() (*Proposal, error) {
	props, err := s.ListProposals()
	if err != nil {
		return nil, err
	}
	for i := range props {
		if props[i].Status == StatusPending {
			return &props[i], nil
		}
	}
	return nil, nil
}

// ── Lessons aggregate ────────────────────────────────────────────────────────

// WriteLessons writes the per-skill lessons.md (the lessons region snapshot).
func (s *Store) WriteLessons(content string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	return writeFileAtomic(s.lessonsPath(), []byte(content))
}

// ReadLessons returns the per-skill lessons.md ("" if absent).
func (s *Store) ReadLessons() (string, error) {
	data, err := os.ReadFile(s.lessonsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// writeFileAtomic writes via a temp file + rename so readers never see a
// half-written file.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
