// pkg/network/avatar.go — local cache of contact avatars fetched from
// upstream channels (Feishu / Telegram). One file per contact, stored at
//
//	workspace/network/avatars/{filenameForID}.{ext}
//
// The avatar's path (relative to workspace/network/avatars/) is persisted on
// the Contact so list views can decide whether to render <img> vs the
// fallback letter-circle without an extra round-trip per row.
//
// Size hard cap: 1 MiB. We refuse to save anything bigger to prevent a
// runaway avatar gobbling per-agent disk; channel handlers should already
// pick a sensibly small size (Feishu avatar_origin is ~30-100 KB, TG photo
// resolution caps far below 1 MiB).
//
// Added 26.5.12v1 (E-01).

package network

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxAvatarBytes is the hard upper limit for SaveAvatar.
const MaxAvatarBytes = 1 << 20 // 1 MiB

var allowedAvatarExts = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "webp": true, "gif": true,
}

// avatarsDir returns the absolute path of workspace/network/avatars/.
func (s *Store) avatarsDir() string {
	return filepath.Join(s.Dir(), "avatars")
}

// ensureAvatarsDir creates the avatars/ directory if missing.
func (s *Store) ensureAvatarsDir() error {
	return os.MkdirAll(s.avatarsDir(), 0o700)
}

// SaveAvatar persists an avatar image for a contact and updates the contact's
// AvatarPath field. The extOrContentType argument accepts either a bare
// extension ("jpg", ".png") or an HTTP Content-Type ("image/jpeg"); both are
// normalised. Anything not recognised falls back to "jpg".
//
// Side effects: writes file then re-saves the Contact (which refreshes INDEX.*).
// Returns ErrAvatarTooLarge when data exceeds MaxAvatarBytes.
func (s *Store) SaveAvatar(contactID string, data []byte, extOrContentType string) error {
	if len(data) == 0 {
		return errors.New("network.SaveAvatar: empty data")
	}
	if len(data) > MaxAvatarBytes {
		return ErrAvatarTooLarge
	}
	ext := strings.ToLower(strings.TrimSpace(extOrContentType))
	if strings.HasPrefix(ext, "image/") {
		ext = SniffAvatarExt(ext)
	}
	ext = strings.TrimPrefix(ext, ".")
	if !allowedAvatarExts[ext] {
		ext = "jpg"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.getUnlocked(contactID)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("network.SaveAvatar: contact not found: %s", contactID)
	}

	if err := s.ensureAvatarsDir(); err != nil {
		return err
	}
	// Use the same filename scheme as contact files (colon → hyphen) to make
	// the relationship between an avatar and its contact obvious on disk.
	rel := strings.TrimSuffix(filenameForID(contactID), ".md") + "." + ext
	abs := filepath.Join(s.avatarsDir(), rel)

	if err := os.WriteFile(abs, data, 0o600); err != nil {
		return err
	}

	// If the contact had a previously-cached avatar with a different extension,
	// clean it up so we don't leave orphan files.
	if c.AvatarPath != "" && c.AvatarPath != rel {
		_ = os.Remove(filepath.Join(s.avatarsDir(), c.AvatarPath))
	}

	c.AvatarPath = rel
	return s.saveUnlocked(c)
}

// AvatarPath returns the absolute filesystem path for a contact's cached
// avatar, or "" if not cached. Convenience for handlers that want to
// http.ServeFile.
func (s *Store) AvatarPath(contactID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.getUnlocked(contactID)
	if err != nil || c == nil || c.AvatarPath == "" {
		return ""
	}
	abs := filepath.Join(s.avatarsDir(), c.AvatarPath)
	if _, err := os.Stat(abs); err != nil {
		return ""
	}
	return abs
}

// DeleteAvatar removes the cached file and clears the AvatarPath field.
// Idempotent; a missing file is not an error.
func (s *Store) DeleteAvatar(contactID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.getUnlocked(contactID)
	if err != nil {
		return err
	}
	if c == nil || c.AvatarPath == "" {
		return nil
	}
	_ = os.Remove(filepath.Join(s.avatarsDir(), c.AvatarPath))
	c.AvatarPath = ""
	return s.saveUnlocked(c)
}

// ErrAvatarTooLarge signals that the caller passed more bytes than MaxAvatarBytes.
var ErrAvatarTooLarge = errors.New("avatar exceeds maximum allowed size (1 MiB)")

// SniffAvatarExt picks a filesystem extension from an HTTP Content-Type header.
// Used by channel handlers that download avatars over HTTP. Falls back to "jpg".
func SniffAvatarExt(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	}
	return "jpg"
}
