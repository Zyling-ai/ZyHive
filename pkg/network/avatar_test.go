package network

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper: create a store rooted at t.TempDir() with one resolved contact.
func newStoreWithContact(t *testing.T, source, extID string) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s := NewStore(dir)
	c, err := s.Resolve(source, extID, "User "+extID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return s, c.ID
}

func TestSaveAvatarRoundTrip(t *testing.T) {
	s, cid := newStoreWithContact(t, "feishu", "ou_abc")
	payload := bytes.Repeat([]byte{0xff, 0xd8, 0xff}, 100) // tiny "JPEG" stub
	if err := s.SaveAvatar(cid, payload, "image/jpeg"); err != nil {
		t.Fatalf("SaveAvatar: %v", err)
	}
	// Contact must persist avatarPath.
	c, err := s.Get(cid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if c == nil || c.AvatarPath == "" {
		t.Fatalf("AvatarPath not stored")
	}
	if !strings.HasSuffix(c.AvatarPath, ".jpg") {
		t.Fatalf("expected .jpg ext, got %s", c.AvatarPath)
	}
	// File must exist on disk and have the bytes.
	abs := s.AvatarPath(cid)
	if abs == "" {
		t.Fatalf("AvatarPath() returned empty")
	}
	got, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("file content mismatch")
	}
	// Summary must reflect HasAvatar=true.
	list, _ := s.List()
	if len(list) != 1 || !list[0].HasAvatar {
		t.Fatalf("summary HasAvatar not true: %+v", list)
	}
}

func TestSaveAvatarTooLargeRejected(t *testing.T) {
	s, cid := newStoreWithContact(t, "telegram", "12345")
	big := make([]byte, MaxAvatarBytes+1)
	err := s.SaveAvatar(cid, big, "image/png")
	if err == nil {
		t.Fatalf("expected ErrAvatarTooLarge, got nil")
	}
	if err != ErrAvatarTooLarge {
		t.Fatalf("unexpected err: %v", err)
	}
	// No file should exist.
	c, _ := s.Get(cid)
	if c.AvatarPath != "" {
		t.Fatalf("AvatarPath should not be set after rejection: %s", c.AvatarPath)
	}
}

func TestSaveAvatarReplacesOldExtension(t *testing.T) {
	s, cid := newStoreWithContact(t, "web", "sid-xxx")
	if err := s.SaveAvatar(cid, []byte("png-data-1"), "image/png"); err != nil {
		t.Fatal(err)
	}
	c1, _ := s.Get(cid)
	if !strings.HasSuffix(c1.AvatarPath, ".png") {
		t.Fatalf("first save expected .png, got %s", c1.AvatarPath)
	}
	oldAbs := filepath.Join(s.avatarsDir(), c1.AvatarPath)

	if err := s.SaveAvatar(cid, []byte("jpg-data-2"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	c2, _ := s.Get(cid)
	if !strings.HasSuffix(c2.AvatarPath, ".jpg") {
		t.Fatalf("second save expected .jpg, got %s", c2.AvatarPath)
	}
	// Old .png should have been deleted.
	if _, err := os.Stat(oldAbs); !os.IsNotExist(err) {
		t.Fatalf("expected old .png to be deleted, err=%v", err)
	}
}

func TestSaveAvatarRejectsEmpty(t *testing.T) {
	s, cid := newStoreWithContact(t, "feishu", "ou_e")
	if err := s.SaveAvatar(cid, nil, "image/jpeg"); err == nil {
		t.Fatalf("expected error on empty data")
	}
}

func TestSaveAvatarRejectsUnknownContact(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.SaveAvatar("feishu:nope", []byte{0x01, 0x02}, "image/jpeg"); err == nil {
		t.Fatalf("expected error for unknown contact")
	}
}

func TestSaveAvatarBadExtFallsBackJPG(t *testing.T) {
	s, cid := newStoreWithContact(t, "telegram", "777")
	if err := s.SaveAvatar(cid, []byte{0xab, 0xcd}, "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	c, _ := s.Get(cid)
	if !strings.HasSuffix(c.AvatarPath, ".jpg") {
		t.Fatalf("expected .jpg fallback, got %s", c.AvatarPath)
	}
}

func TestSniffAvatarExt(t *testing.T) {
	cases := []struct{ ct, want string }{
		{"image/jpeg", "jpg"},
		{"image/png", "png"},
		{"image/webp", "webp"},
		{"image/gif", "gif"},
		{"image/jpeg; charset=utf-8", "jpg"},
		{"text/plain", "jpg"}, // fallback
		{"", "jpg"},
	}
	for _, c := range cases {
		if got := SniffAvatarExt(c.ct); got != c.want {
			t.Errorf("SniffAvatarExt(%q) = %s, want %s", c.ct, got, c.want)
		}
	}
}

func TestDeleteAvatar(t *testing.T) {
	s, cid := newStoreWithContact(t, "feishu", "ou_del")
	_ = s.SaveAvatar(cid, []byte{0xff, 0xd8}, "image/jpeg")
	abs := s.AvatarPath(cid)
	if abs == "" {
		t.Fatalf("setup failed")
	}
	if err := s.DeleteAvatar(cid); err != nil {
		t.Fatalf("DeleteAvatar: %v", err)
	}
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Fatalf("file should be gone")
	}
	c, _ := s.Get(cid)
	if c.AvatarPath != "" {
		t.Fatalf("AvatarPath should be cleared, got %s", c.AvatarPath)
	}
	// Idempotent
	if err := s.DeleteAvatar(cid); err != nil {
		t.Fatalf("DeleteAvatar second call: %v", err)
	}
}

func TestContactRoundTripsAvatarPathInMD(t *testing.T) {
	s, cid := newStoreWithContact(t, "feishu", "ou_round")
	_ = s.SaveAvatar(cid, []byte{0x01, 0x02, 0x03}, "image/png")
	// Force re-read from disk by creating a fresh store.
	s2 := NewStore(s.workspaceDir)
	c, err := s2.Get(cid)
	if err != nil || c == nil {
		t.Fatalf("re-read failed: %v / nil=%v", err, c == nil)
	}
	if !strings.HasSuffix(c.AvatarPath, ".png") {
		t.Fatalf("AvatarPath did not survive frontmatter round-trip: %q", c.AvatarPath)
	}
}
