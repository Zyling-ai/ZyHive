// Package artifact issues short-lived, one-time download credentials for
// files that have already passed a caller's workspace policy.
package artifact

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultTicketTTL = 10 * time.Minute
	MaxTicketTTL     = time.Hour
)

var (
	ErrInvalidTicket = errors.New("artifact: invalid ticket")
	ErrExpiredTicket = errors.New("artifact: expired ticket")
)

type ticket struct {
	path      string
	tokenHash [sha256.Size]byte
	expiresAt time.Time
}

// TicketStore keeps only token hashes and canonical file paths in memory.
// Tickets are consumed exactly once and disappear after expiration.
type TicketStore struct {
	mu      sync.Mutex
	tickets map[string]ticket
	now     func() time.Time
}

func NewTicketStore() *TicketStore {
	return &TicketStore{
		tickets: make(map[string]ticket),
		now:     time.Now,
	}
}

// DefaultTickets is shared by the API and tool layers in the single-process
// self-hosted runtime.
var DefaultTickets = NewTicketStore()

// Issue registers a regular file and returns an opaque artifact ID, a
// one-time credential, and its expiration time.
func (s *TicketStore) Issue(path string, ttl time.Duration) (string, string, time.Time, error) {
	if s == nil {
		return "", "", time.Time{}, errors.New("artifact: nil ticket store")
	}
	canonical, err := canonicalRegularFile(path)
	if err != nil {
		return "", "", time.Time{}, err
	}
	if ttl <= 0 {
		ttl = DefaultTicketTTL
	}
	if ttl > MaxTicketTTL {
		ttl = MaxTicketTTL
	}

	artifactID, err := randomHex(16)
	if err != nil {
		return "", "", time.Time{}, err
	}
	token, err := randomHex(32)
	if err != nil {
		return "", "", time.Time{}, err
	}
	expiresAt := s.now().Add(ttl)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(s.now())
	s.tickets[artifactID] = ticket{
		path:      canonical,
		tokenHash: sha256.Sum256([]byte(token)),
		expiresAt: expiresAt,
	}
	return artifactID, token, expiresAt, nil
}

// IssueURL creates a same-origin download URL without exposing the file path
// or the long-lived administrator token.
func (s *TicketStore) IssueURL(baseURL, path string, ttl time.Duration) (string, error) {
	return s.IssueURLFor(baseURL, "/api/download", path, ttl)
}

// IssueURLFor creates a ticket URL for a specific same-origin serving route.
func (s *TicketStore) IssueURLFor(baseURL, route, path string, ttl time.Duration) (string, error) {
	if !strings.HasPrefix(route, "/") || strings.ContainsAny(route, "?#") {
		return "", errors.New("artifact: invalid serving route")
	}
	artifactID, token, _, err := s.Issue(path, ttl)
	if err != nil {
		return "", err
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return baseURL + route + "?id=" + url.QueryEscape(artifactID) +
		"&token=" + url.QueryEscape(token), nil
}

// Consume validates and deletes a ticket. A wrong credential does not delete
// the ticket, preventing an attacker who only knows an artifact ID from
// invalidating another user's legitimate download.
func (s *TicketStore) Consume(artifactID, token string) (string, error) {
	if s == nil || artifactID == "" || token == "" {
		return "", ErrInvalidTicket
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.tickets[artifactID]
	if !ok {
		return "", ErrInvalidTicket
	}
	now := s.now()
	if !entry.expiresAt.After(now) {
		delete(s.tickets, artifactID)
		return "", ErrExpiredTicket
	}
	actualHash := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(actualHash[:], entry.tokenHash[:]) != 1 {
		return "", ErrInvalidTicket
	}
	delete(s.tickets, artifactID)
	return entry.path, nil
}

func (s *TicketStore) cleanupLocked(now time.Time) {
	for artifactID, entry := range s.tickets {
		if !entry.expiresAt.After(now) {
			delete(s.tickets, artifactID)
		}
	}
}

func canonicalRegularFile(path string) (string, error) {
	if path == "" {
		return "", errors.New("artifact: file path required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("artifact: resolve file path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("artifact: resolve file symlinks: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("artifact: stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("artifact: only regular files can be downloaded")
	}
	return canonical, nil
}

func randomHex(byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("artifact: generate credential: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
