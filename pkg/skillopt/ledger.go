package skillopt

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
)

// Append records a new prediction. ID/TS/Version are filled in when empty.
// The entry is appended as one JSON line (JSONL), mirroring pkg/cron + pkg/goal.
func (s *Store) Append(e LedgerEntry) (LedgerEntry, error) {
	if err := s.ensure(); err != nil {
		return e, err
	}
	if e.ID == "" {
		e.ID = "pred-" + uuid.New().String()[:8]
	}
	if e.TS == 0 {
		e.TS = time.Now().UnixMilli()
	}
	if e.Version == 0 {
		if ep, err := s.ReadEpoch(); err == nil {
			if ep.ShadowVersion > 0 {
				// A shadow/canary window is open → new predictions ride the shadow.
				e.Version = ep.ShadowVersion
				e.Shadow = true
			} else {
				e.Version = ep.BaselineVersion
			}
		}
	}

	f, err := os.OpenFile(s.ledgerPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return e, err
	}
	defer f.Close()
	data, err := json.Marshal(e)
	if err != nil {
		return e, err
	}
	if _, err := fmt.Fprintf(f, "%s\n", data); err != nil {
		return e, err
	}
	return e, nil
}

// AllEntries reads the full ledger in append order.
func (s *Store) AllEntries() ([]LedgerEntry, error) {
	f, err := os.Open(s.ledgerPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []LedgerEntry{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []LedgerEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e LedgerEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	if out == nil {
		out = []LedgerEntry{}
	}
	return out, nil
}

// Query returns the last `limit` entries (newest last). limit<=0 returns all.
func (s *Store) Query(limit int) ([]LedgerEntry, error) {
	all, err := s.AllEntries()
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

// PendingOracle returns entries that have not been backfilled yet.
func (s *Store) PendingOracle() ([]LedgerEntry, error) {
	all, err := s.AllEntries()
	if err != nil {
		return nil, err
	}
	out := make([]LedgerEntry, 0)
	for _, e := range all {
		if e.Hit == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

// HitRate computes the hit-rate over backfilled entries.
// When shadowOnly is true, only entries flagged Shadow are counted; otherwise
// only non-shadow (baseline) entries are counted. Returns (rate, sampleCount).
func (s *Store) HitRate(shadowOnly bool) (float64, int) {
	all, err := s.AllEntries()
	if err != nil {
		return 0, 0
	}
	hits, total := 0, 0
	for _, e := range all {
		if e.Hit == nil || e.Shadow != shadowOnly {
			continue
		}
		total++
		if *e.Hit {
			hits++
		}
	}
	if total == 0 {
		return 0, 0
	}
	return float64(hits) / float64(total), total
}

// shadowHitRateSince computes the hit-rate over shadow predictions made at or
// after sinceMs (scopes to the current canary window, ignoring older shadows).
func (s *Store) shadowHitRateSince(sinceMs int64) (float64, int) {
	all, err := s.AllEntries()
	if err != nil {
		return 0, 0
	}
	hits, total := 0, 0
	for _, e := range all {
		if !e.Shadow || e.Hit == nil || e.TS < sinceMs {
			continue
		}
		total++
		if *e.Hit {
			hits++
		}
	}
	if total == 0 {
		return 0, 0
	}
	return float64(hits) / float64(total), total
}

// rewriteLedger atomically replaces the entire ledger file.
func (s *Store) rewriteLedger(entries []LedgerEntry) error {
	if err := s.ensure(); err != nil {
		return err
	}
	tmp := s.ledgerPath() + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			f.Close()
			return err
		}
		if _, err := fmt.Fprintf(w, "%s\n", data); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.ledgerPath())
}

// updateEntry applies mutate to the entry with the given id and rewrites the ledger.
func (s *Store) updateEntry(entryID string, mutate func(*LedgerEntry)) error {
	all, err := s.AllEntries()
	if err != nil {
		return err
	}
	found := false
	for i := range all {
		if all[i].ID == entryID {
			mutate(&all[i])
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("ledger entry %q not found", entryID)
	}
	return s.rewriteLedger(all)
}
