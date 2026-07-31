package artifact

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testArtifactFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(path, []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTicketIsConsumedExactlyOnce(t *testing.T) {
	store := NewTicketStore()
	path := testArtifactFile(t)
	artifactID, token, _, err := store.Issue(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := store.Consume(artifactID, token)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != expected {
		t.Fatalf("resolved path=%q want=%q", resolved, expected)
	}
	if _, err := store.Consume(artifactID, token); !errors.Is(err, ErrInvalidTicket) {
		t.Fatalf("replayed ticket should fail, got %v", err)
	}
}

func TestWrongTokenDoesNotConsumeTicket(t *testing.T) {
	store := NewTicketStore()
	artifactID, token, _, err := store.Issue(testArtifactFile(t), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Consume(artifactID, "wrong"); !errors.Is(err, ErrInvalidTicket) {
		t.Fatalf("wrong token should fail, got %v", err)
	}
	if _, err := store.Consume(artifactID, token); err != nil {
		t.Fatalf("valid token should remain usable after wrong attempt: %v", err)
	}
}

func TestExpiredTicketIsRejected(t *testing.T) {
	store := NewTicketStore()
	now := time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	artifactID, token, _, err := store.Issue(testArtifactFile(t), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := store.Consume(artifactID, token); !errors.Is(err, ErrExpiredTicket) {
		t.Fatalf("expired ticket should fail, got %v", err)
	}
}

func TestIssueURLDoesNotExposePath(t *testing.T) {
	store := NewTicketStore()
	path := testArtifactFile(t)
	downloadURL, err := store.IssueURL("https://example.test/", path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(downloadURL, path) {
		t.Fatalf("download URL exposed host path: %s", downloadURL)
	}
	if !strings.HasPrefix(downloadURL, "https://example.test/api/download?id=") ||
		!strings.Contains(downloadURL, "&token=") {
		t.Fatalf("unexpected download URL: %s", downloadURL)
	}
}

func TestIssueURLForUsesRequestedServingRoute(t *testing.T) {
	store := NewTicketStore()
	mediaURL, err := store.IssueURLFor("https://example.test", "/api/media", testArtifactFile(t), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(mediaURL, "https://example.test/api/media?id=") {
		t.Fatalf("unexpected media URL: %s", mediaURL)
	}
	if _, err := store.IssueURLFor("https://example.test", "https://evil.test", testArtifactFile(t), time.Minute); err == nil {
		t.Fatal("absolute serving route should be rejected")
	}
}

func TestIssueRejectsDirectories(t *testing.T) {
	store := NewTicketStore()
	if _, _, _, err := store.Issue(t.TempDir(), time.Minute); err == nil {
		t.Fatal("directory should not be registered as an artifact")
	}
}
