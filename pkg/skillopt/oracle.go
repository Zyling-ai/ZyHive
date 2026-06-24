package skillopt

import "time"

// Oracle backfills the real-world outcome for a prediction and marks hit/miss.
func (s *Store) Oracle(entryID, result string, hit bool) error {
	return s.updateEntry(entryID, func(e *LedgerEntry) {
		e.Oracle = result
		h := hit
		e.Hit = &h
		e.OracleTS = time.Now().UnixMilli()
	})
}

// recordAttribution stores the critic's tags + lesson onto the ledger entry.
func (s *Store) recordAttribution(a Attribution) error {
	return s.updateEntry(a.EntryID, func(e *LedgerEntry) {
		e.AttributionTags = a.Tags
		e.Lesson = a.Lesson
	})
}

// backfilledSince returns backfilled entries whose oracle landed after `sinceMs`.
// When onlyMiss is true, only missed predictions are returned.
func (s *Store) backfilledSince(sinceMs int64, onlyMiss bool) ([]LedgerEntry, error) {
	all, err := s.AllEntries()
	if err != nil {
		return nil, err
	}
	out := make([]LedgerEntry, 0)
	for _, e := range all {
		if e.Hit == nil || e.OracleTS <= sinceMs {
			continue
		}
		if onlyMiss && *e.Hit {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}
