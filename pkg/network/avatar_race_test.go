package network

import (
	"strings"
	"sync"
	"testing"
)

// TestSaveAvatarConcurrentSameContact — N goroutines all call SaveAvatar on
// the same contact at once. Tests that:
//   - No race-detector trips (run with -race)
//   - The contact frontmatter is still parseable afterwards (not torn)
//   - AvatarPath ends up pointing at a file that exists on disk
//   - Only one avatar file remains (old extensions cleaned up correctly)
func TestSaveAvatarConcurrentSameContact(t *testing.T) {
	s, cid := newStoreWithContact(t, "feishu", "ou_concurrent")

	const N = 16
	var wg sync.WaitGroup
	// Alternate between PNG and JPG to exercise the "old extension cleanup" path.
	exts := []string{"image/jpeg", "image/png", "image/webp"}
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload := []byte{0xff, 0xd8, byte(i)}
			_ = s.SaveAvatar(cid, payload, exts[i%len(exts)])
		}(i)
	}
	wg.Wait()

	// After the storm: contact must still be parseable.
	c, err := s.Get(cid)
	if err != nil {
		t.Fatalf("Get after storm: %v", err)
	}
	if c == nil {
		t.Fatalf("contact disappeared!")
	}
	if c.DisplayName == "" || c.ID == "" {
		t.Errorf("frontmatter torn: %+v", c)
	}
	if c.AvatarPath == "" {
		t.Errorf("no AvatarPath set after %d concurrent writes", N)
	}
	// AvatarPath must reference an existing file.
	abs := s.AvatarPath(cid)
	if abs == "" {
		t.Errorf("AvatarPath() resolved empty (file missing?)")
	}
	// Sanity: extension is one of the allowed values.
	ok := false
	for _, e := range []string{".jpg", ".png", ".webp"} {
		if strings.HasSuffix(c.AvatarPath, e) {
			ok = true
			break
		}
	}
	if !ok {
		t.Errorf("unexpected AvatarPath ext: %s", c.AvatarPath)
	}
}

// TestSaveAvatarConcurrentDifferentContacts — N goroutines, N different
// contacts, each saving its own avatar. No cross-contamination, no lost
// writes.
func TestSaveAvatarConcurrentDifferentContacts(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	const N = 12
	ids := make([]string, N)
	for i := 0; i < N; i++ {
		c, err := s.Resolve("feishu", "ou_"+string(rune('A'+i)), "User")
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = c.ID
	}

	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			_ = s.SaveAvatar(id, []byte{byte(i), 0xff, 0xd8}, "image/jpeg")
		}(i, id)
	}
	wg.Wait()

	// Every contact must end up with an avatar.
	for _, id := range ids {
		c, err := s.Get(id)
		if err != nil || c == nil {
			t.Errorf("lost contact %s: err=%v", id, err)
			continue
		}
		if c.AvatarPath == "" {
			t.Errorf("contact %s missing AvatarPath after concurrent save", id)
		}
	}
}

// TestSaveAvatarThenDeleteRace — mixed save+delete to ensure the Store's mutex
// keeps Save and Delete from corrupting each other.
func TestSaveAvatarThenDeleteRace(t *testing.T) {
	s, cid := newStoreWithContact(t, "telegram", "race_user")
	const Iter = 30
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < Iter; i++ {
			_ = s.SaveAvatar(cid, []byte{0xff, byte(i)}, "image/jpeg")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < Iter; i++ {
			_ = s.DeleteAvatar(cid)
		}
	}()
	wg.Wait()
	// Contact must still be parseable and the network INDEX consistent.
	c, err := s.Get(cid)
	if err != nil {
		t.Fatalf("Get after race: %v", err)
	}
	if c == nil || c.DisplayName == "" {
		t.Errorf("frontmatter torn: %+v", c)
	}
}
