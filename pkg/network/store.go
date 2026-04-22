package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store is the per-agent network manager. One instance per workspace.
type Store struct {
	workspaceDir string
	mu           sync.Mutex
}

// NewStore returns a Store for the given agent workspace. It does NOT create
// any files — files are created lazily on first mutation.
func NewStore(workspaceDir string) *Store {
	return &Store{workspaceDir: workspaceDir}
}

// Dir returns the absolute path of the network/ directory.
func (s *Store) Dir() string {
	return filepath.Join(s.workspaceDir, "network")
}

func (s *Store) contactsDir() string {
	return filepath.Join(s.Dir(), "contacts")
}

func (s *Store) indexMDPath() string {
	return filepath.Join(s.Dir(), "INDEX.md")
}

func (s *Store) indexJSONPath() string {
	return filepath.Join(s.Dir(), "INDEX.json")
}

// ensureDirs creates network/ and network/contacts/ if missing.
func (s *Store) ensureDirs() error {
	return os.MkdirAll(s.contactsDir(), 0755)
}

// ── Contact ID helpers ────────────────────────────────────────────────────

// MakeID builds a canonical contact ID: "{source}:{externalId}".
func MakeID(source, externalID string) string {
	return source + ":" + externalID
}

// FallbackDisplayName picks the first non-empty candidate as a human-readable
// name. Used by channel inbound handlers where some platforms may not give a
// nickname on first contact (e.g. Feishu before chatroom member list is loaded,
// or Telegram users without FirstName/Username).
//
// The last fallback is a short prefix of the externalID so the UI never shows
// an empty display name.
func FallbackDisplayName(externalID string, candidates ...string) string {
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c != "" {
			return c
		}
	}
	// Short ID prefix as last resort. Keep it readable (8 chars is enough to
	// distinguish most external IDs at a glance).
	if len(externalID) > 8 {
		return externalID[:8]
	}
	return externalID
}

// SplitID parses "source:externalId" — reverse of MakeID.
func SplitID(id string) (source, externalID string, ok bool) {
	i := strings.Index(id, ":")
	if i < 0 {
		return "", "", false
	}
	return id[:i], id[i+1:], true
}

// filenameForID returns the filesystem-safe filename for a contact ID.
// Colons are replaced with hyphens to stay cross-platform.
func filenameForID(id string) string {
	return strings.ReplaceAll(id, ":", "-") + ".md"
}

// ── Core operations ───────────────────────────────────────────────────────

// Get reads one contact by ID. Returns (nil, nil) if the contact does not exist.
func (s *Store) Get(id string) (*Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getUnlocked(id)
}

func (s *Store) getUnlocked(id string) (*Contact, error) {
	path := filepath.Join(s.contactsDir(), filenameForID(id))
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return parseContactMD(string(raw), id), nil
}

// saveUnlocked writes a contact to disk (no lock; caller must hold s.mu).
// Also refreshes INDEX.json / INDEX.md.
func (s *Store) saveUnlocked(c *Contact) error {
	if err := s.ensureDirs(); err != nil {
		return err
	}
	path := filepath.Join(s.contactsDir(), filenameForID(c.ID))
	if err := os.WriteFile(path, []byte(renderContactMD(c)), 0644); err != nil {
		return err
	}
	return s.refreshIndexUnlocked()
}

// Save persists the contact, regenerates INDEX.*.
func (s *Store) Save(c *Contact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnlocked(c)
}

// Delete removes a contact and refreshes the index.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.contactsDir(), filenameForID(id))
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return s.refreshIndexUnlocked()
}

// Resolve is the canonical upsert. It is called by every inbound-message handler
// (panel/feishu/telegram/web). Behavior:
//  1. If a contact file at {source}:{externalID} exists → bump LastSeenAt/MsgCount
//  2. Else if another primary contact lists this ID in its `aliases` (i.e. the
//     user has previously merged them) → route to that primary (Bug 3 fix)
//  3. Else → create a new primary contact with the default body
func (s *Store) Resolve(source, externalID, displayName string) (*Contact, error) {
	if source == "" || externalID == "" {
		return nil, fmt.Errorf("network.Resolve: source and externalID required")
	}
	id := MakeID(source, externalID)
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := s.getUnlocked(id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if existing != nil {
		existing.LastSeenAt = now
		existing.MsgCount++
		// Only update displayName if provider gives a better one (non-empty and
		// different; don't overwrite a nicer name with an empty string).
		if displayName != "" && existing.DisplayName == "" {
			existing.DisplayName = displayName
		}
		if err := s.saveUnlocked(existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	// Bug 3 fix: 没有直接命中, 在扫一次 primary 档案的 aliases 字段.
	// 如果该 ID 已被手动合并到某个 primary 下, 把消息计入 primary, 不新建.
	if primary, perr := s.findPrimaryByAliasUnlocked(id); perr == nil && primary != nil {
		primary.LastSeenAt = now
		primary.MsgCount++
		if err := s.saveUnlocked(primary); err != nil {
			return nil, err
		}
		return primary, nil
	}

	c := &Contact{
		ID:          id,
		Source:      source,
		ExternalID:  externalID,
		DisplayName: displayName,
		Primary:     true,
		CreatedAt:   now,
		LastSeenAt:  now,
		MsgCount:    1,
	}
	if err := s.saveUnlocked(c); err != nil {
		return nil, err
	}
	return c, nil
}

// findPrimaryByAliasUnlocked scans every primary contact file looking for one
// whose Aliases contains aliasID. Caller must hold s.mu.
// Returns (nil, nil) when not found. O(N) over file count; acceptable since
// contact counts per agent are expected to be in the hundreds at most.
func (s *Store) findPrimaryByAliasUnlocked(aliasID string) (*Contact, error) {
	entries, err := os.ReadDir(s.contactsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.contactsDir(), e.Name()))
		if err != nil {
			continue
		}
		fallbackID := strings.TrimSuffix(e.Name(), ".md")
		if i := strings.Index(fallbackID, "-"); i >= 0 {
			fallbackID = fallbackID[:i] + ":" + fallbackID[i+1:]
		}
		c := parseContactMD(string(data), fallbackID)
		if !c.Primary {
			continue
		}
		for _, a := range c.Aliases {
			if a == aliasID {
				return c, nil
			}
		}
	}
	return nil, nil
}

// Touch bumps LastSeenAt / MsgCount without changing anything else. Used for
// subsequent messages when the caller already has a resolved contact.
func (s *Store) Touch(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.getUnlocked(id)
	if err != nil || c == nil {
		return err
	}
	c.LastSeenAt = time.Now().UTC()
	c.MsgCount++
	return s.saveUnlocked(c)
}

// List returns all contacts (sorted by LastSeenAt desc).
func (s *Store) List() ([]ContactSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listUnlocked()
}

func (s *Store) listUnlocked() ([]ContactSummary, error) {
	dir := s.contactsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]ContactSummary, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		// Derive ID from filename as a fallback (replace first "-" after source
		// back into colon). But parseContactMD reads ID from frontmatter which
		// is authoritative. If missing, fall back to filename decoded.
		fallbackID := strings.TrimSuffix(e.Name(), ".md")
		if i := strings.Index(fallbackID, "-"); i >= 0 {
			fallbackID = fallbackID[:i] + ":" + fallbackID[i+1:]
		}
		c := parseContactMD(string(data), fallbackID)
		if c.ID == "" {
			c.ID = fallbackID
		}
		out = append(out, c.Summary())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	return out, nil
}

// ── INDEX refresh ────────────────────────────────────────────────────────

// refreshIndexUnlocked regenerates both INDEX.json and INDEX.md from the
// current contacts directory + RELATIONS.md. Caller must hold s.mu.
func (s *Store) refreshIndexUnlocked() error {
	summaries, err := s.listUnlocked()
	if err != nil {
		return err
	}
	// INDEX.json
	idx := Index{
		Contacts:  summaries,
		UpdatedAt: time.Now().UTC(),
	}
	jsonBytes, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.indexJSONPath(), jsonBytes, 0644); err != nil {
		return err
	}
	// INDEX.md
	md := renderIndexMD(summaries, s.readRelationsRowsUnlocked())
	return os.WriteFile(s.indexMDPath(), []byte(md), 0644)
}

// RefreshIndex is the public thread-safe version.
func (s *Store) RefreshIndex() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureDirs(); err != nil {
		return err
	}
	return s.refreshIndexUnlocked()
}

// renderIndexMD builds the lightweight markdown index that is injected into
// every system prompt. Target size: ~500–800 chars.
func renderIndexMD(contacts []ContactSummary, relations []RelationLine) string {
	var sb strings.Builder
	sb.WriteString("# 通讯录 — network/INDEX.md\n\n")
	sb.WriteString("> 每当你发现新联系人或关系，对应文件会被自动维护。\n")
	sb.WriteString("> 完整联系人档案：`read(\"network/contacts/<id>.md\")`\n")
	sb.WriteString("> 关系表（谁和谁）：`read(\"network/RELATIONS.md\")`\n")
	sb.WriteString("> 给联系人添加事实/偏好：`network_note` 工具。\n\n")

	// AI 同事 (from RELATIONS.md)
	aiPeers := filterRelations(relations, "agent")
	if len(aiPeers) > 0 {
		sb.WriteString(fmt.Sprintf("## AI 同事 (%d)\n", len(aiPeers)))
		for _, r := range aiPeers {
			label := strings.TrimSpace(r.Desc)
			if label == "" {
				label = "(无备注)"
			}
			sb.WriteString(fmt.Sprintf("- **%s** — %s · %s · %s\n", r.DisplayName, r.Type, r.Strength, truncate(label, 40)))
		}
		sb.WriteString("\n")
	}

	// 真人联系人
	humanContacts := contacts // all contacts are by definition humans (or external agents)
	if len(humanContacts) > 0 {
		sb.WriteString(fmt.Sprintf("## 真人联系人 (%d)\n", len(humanContacts)))
		limit := 30
		for i, c := range humanContacts {
			if i >= limit {
				sb.WriteString(fmt.Sprintf("- ...另有 %d 位，按需 `read(\"network/INDEX.json\")` 查看完整\n", len(humanContacts)-limit))
				break
			}
			name := c.DisplayName
			if name == "" {
				name = c.ExternalIDPart()
			}
			var tagPart string
			if len(c.Tags) > 0 {
				tagPart = " [" + strings.Join(c.Tags, "/") + "]"
			}
			var ownerMark string
			if c.IsOwner {
				ownerMark = " ⭐主人"
			}
			lastSeen := ""
			if !c.LastSeenAt.IsZero() {
				lastSeen = " · " + c.LastSeenAt.Format("2006-01-02")
			}
			sb.WriteString(fmt.Sprintf("- **%s** (`%s`)%s%s · %d msg%s\n",
				name, c.ID, tagPart, ownerMark, c.MsgCount, lastSeen))
		}
		sb.WriteString("\n")
	}

	if len(aiPeers) == 0 && len(humanContacts) == 0 {
		sb.WriteString("_暂无联系人或关系。有新消息时联系人会自动出现。_\n\n")
	}

	sb.WriteString("## 使用约定\n")
	sb.WriteString("- 识别到新联系人后系统会自动建档。你看到一位陌生人时档案已存在。\n")
	sb.WriteString("- 发现重要事实/偏好/待办请用 `network_note(entityId, section, text)`（section: 事实/偏好/待跟进）。\n")
	sb.WriteString("- 派遣仅限 AI 同事（见 RELATIONS.md），联系人不是可派遣对象。\n")
	return sb.String()
}

// ExternalIDPart returns just the externalID portion of the contact ID.
func (c ContactSummary) ExternalIDPart() string {
	_, ext, _ := SplitID(c.ID)
	if ext != "" {
		return ext
	}
	return c.ID
}

func truncate(s string, max int) string {
	if len([]rune(s)) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}
