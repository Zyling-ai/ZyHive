// Package session — session reaper.
// Reaper is a background goroutine that periodically removes stale session files
// to prevent unbounded disk growth on long-running deployments.
//
// Deletion criteria:
//   - File mtime older than maxAge (default 30 days)
//   - Session is NOT marked active (Active == true) in the index
//
// On each run the reaper also synchronises sessions.json by removing entries
// whose JSONL files no longer exist.
package session

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	reaperInterval = 24 * time.Hour
	reaperMaxAge   = 30 * 24 * time.Hour
)

// Reaper periodically cleans up stale session files for a Store.
type Reaper struct {
	store    *Store
	interval time.Duration
	maxAge   time.Duration
}

// NewReaper creates a Reaper with default settings (24h interval, 30-day max age).
func NewReaper(store *Store) *Reaper {
	return &Reaper{
		store:    store,
		interval: reaperInterval,
		maxAge:   reaperMaxAge,
	}
}

// Start launches the reaper goroutine.  It runs an initial sweep on startup and
// then repeats every r.interval until ctx is cancelled.
func (r *Reaper) Start(ctx context.Context) {
	go func() {
		// Run once immediately on startup so the first sweep doesn't wait 24h.
		r.runOnce()

		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("[reaper] stopped")
				return
			case <-ticker.C:
				r.runOnce()
			}
		}
	}()
}

// runOnce performs a single reaper sweep on the store directory.
func (r *Reaper) runOnce() {
	store := r.store
	store.mu.Lock()
	defer store.mu.Unlock()

	idx, err := store.loadIndex()
	if err != nil {
		log.Printf("[reaper] failed to load index: %v", err)
		return
	}

	cutoff := time.Now().Add(-r.maxAge)
	deleted := 0

	for sessionID, entry := range idx.Sessions {
		// Never delete active sessions.
		if entry.Active {
			continue
		}

		filePath := filepath.Join(store.dir, entry.FilePath)
		if filePath == "" {
			filePath = filepath.Join(store.dir, sessionID+".jsonl")
		}

		info, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				// File is already gone — clean up the index entry.
				log.Printf("[reaper] removing orphaned index entry: %s", sessionID)
				delete(idx.Sessions, sessionID)
				deleted++
			}
			// Other stat errors: skip silently.
			continue
		}

		if info.ModTime().Before(cutoff) {
			log.Printf("[reaper] deleting stale session %s (mtime=%s, age=%.0fd)",
				sessionID, info.ModTime().Format("2006-01-02"), time.Since(info.ModTime()).Hours()/24)
			if removeErr := os.Remove(filePath); removeErr != nil && !os.IsNotExist(removeErr) {
				log.Printf("[reaper] failed to remove %s: %v", filePath, removeErr)
				continue
			}
			delete(idx.Sessions, sessionID)
			deleted++
		}
	}

	if deleted > 0 {
		if saveErr := store.saveIndex(idx); saveErr != nil {
			log.Printf("[reaper] failed to save updated index: %v", saveErr)
		} else {
			log.Printf("[reaper] sweep complete: removed %d session(s)", deleted)
		}
	} else {
		log.Printf("[reaper] sweep complete: nothing to remove")
	}
}
