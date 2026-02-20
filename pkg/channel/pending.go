// Package channel — PendingStore tracks Telegram users who messaged the bot
// but are not yet in the allowlist. Admins can approve them from the UI.
package channel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// PendingUser holds info about a user who knocked on the bot's door.
type PendingUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"firstName,omitempty"`
	LastSeen  int64  `json:"lastSeen"` // Unix ms
}

// PendingStore persists pending users to a JSON file per channel.
type PendingStore struct {
	mu    sync.RWMutex
	path  string
	users map[int64]PendingUser
}

// NewPendingStore creates a store backed by {dir}/{channelID}-pending.json.
func NewPendingStore(dir, channelID string) *PendingStore {
	_ = os.MkdirAll(dir, 0755)
	ps := &PendingStore{
		path:  filepath.Join(dir, channelID+"-pending.json"),
		users: make(map[int64]PendingUser),
	}
	ps.load()
	return ps
}

// Add inserts or updates a pending user (idempotent).
func (ps *PendingStore) Add(id int64, username, firstName string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.users[id] = PendingUser{
		ID:        id,
		Username:  username,
		FirstName: firstName,
		LastSeen:  time.Now().UnixMilli(),
	}
	ps.save()
}

// Remove deletes a user from the pending list (after approval or rejection).
func (ps *PendingStore) Remove(id int64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.users, id)
	ps.save()
}

// List returns all pending users sorted by LastSeen desc.
func (ps *PendingStore) List() []PendingUser {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	result := make([]PendingUser, 0, len(ps.users))
	for _, u := range ps.users {
		result = append(result, u)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastSeen > result[j].LastSeen
	})
	return result
}

func (ps *PendingStore) load() {
	data, err := os.ReadFile(ps.path)
	if err != nil {
		return
	}
	var list []PendingUser
	if json.Unmarshal(data, &list) == nil {
		for _, u := range list {
			ps.users[u.ID] = u
		}
	}
}

func (ps *PendingStore) save() {
	list := make([]PendingUser, 0, len(ps.users))
	for _, u := range ps.users {
		list = append(list, u)
	}
	data, _ := json.MarshalIndent(list, "", "  ")
	_ = os.WriteFile(ps.path, data, 0644)
}
