package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

type ephemeralTicketStore struct {
	mu      sync.Mutex
	entries map[[sha256.Size]byte]time.Time
	now     func() time.Time
}

func newEphemeralTicketStore() *ephemeralTicketStore {
	return &ephemeralTicketStore{
		entries: make(map[[sha256.Size]byte]time.Time),
		now:     time.Now,
	}
}

func (s *ephemeralTicketStore) issue(ttl time.Duration) (string, bool) {
	if s == nil {
		return "", false
	}
	if ttl <= 0 || ttl > 5*time.Minute {
		ttl = time.Minute
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", false
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.cleanupLocked(now)
	s.entries[hash] = now.Add(ttl)
	return token, true
}

func (s *ephemeralTicketStore) consume(token string) bool {
	if s == nil || token == "" {
		return false
	}
	hash := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.entries[hash]
	if !ok {
		return false
	}
	delete(s.entries, hash)
	return expiresAt.After(s.now())
}

func (s *ephemeralTicketStore) cleanupLocked(now time.Time) {
	for hash, expiresAt := range s.entries {
		if !expiresAt.After(now) {
			delete(s.entries, hash)
		}
	}
}
