package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseChecksumList(t *testing.T) {
	sum := strings.Repeat("a", 64)
	data := []byte(sum + "  zyhive-linux-amd64\n")

	got, err := parseChecksumList(data, "zyhive-linux-amd64")
	if err != nil {
		t.Fatalf("parse checksum: %v", err)
	}
	if got != sum {
		t.Fatalf("got %q want %q", got, sum)
	}
}

func TestParseChecksumListAcceptsBinaryMarker(t *testing.T) {
	sum := strings.Repeat("b", 64)
	data := []byte(sum + " *zyhive-darwin-arm64\n")

	got, err := parseChecksumList(data, "zyhive-darwin-arm64")
	if err != nil {
		t.Fatalf("parse checksum: %v", err)
	}
	if got != sum {
		t.Fatalf("got %q want %q", got, sum)
	}
}

func TestParseChecksumListRejectsMissingOrInvalidEntry(t *testing.T) {
	if _, err := parseChecksumList([]byte("bad  other-file\n"), "zyhive-linux-amd64"); err == nil {
		t.Fatal("expected missing checksum error")
	}
	if _, err := parseChecksumList([]byte("xyz  zyhive-linux-amd64\n"), "zyhive-linux-amd64"); err == nil {
		t.Fatal("expected invalid checksum error")
	}
}

func TestFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(path, []byte("zyhive"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "6775381e5f124683db92c32e04f2a2ae7ea429f6fa956976a7e2ee349006a602"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSupportedReleaseTarget(t *testing.T) {
	for _, tc := range []struct {
		osName string
		arch   string
		want   bool
	}{
		{"linux", "amd64", true},
		{"linux", "arm64", true},
		{"darwin", "amd64", true},
		{"darwin", "arm64", true},
		{"windows", "amd64", false},
		{"linux", "386", false},
	} {
		if got := isSupportedReleaseTarget(tc.osName, tc.arch); got != tc.want {
			t.Fatalf("%s/%s: got %v want %v", tc.osName, tc.arch, got, tc.want)
		}
	}
}

func TestReleaseVersionPattern(t *testing.T) {
	for _, version := range []string{"26.8.1v1", "26.12.31v12"} {
		if !releaseVersionPattern.MatchString(version) {
			t.Fatalf("expected valid version: %s", version)
		}
	}
	for _, version := range []string{"v1.0.0", "../26.8.1v1", "26.8.1", "latest"} {
		if releaseVersionPattern.MatchString(version) {
			t.Fatalf("expected invalid version: %s", version)
		}
	}
}

func TestDownloadFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "6")
		_, _ = w.Write([]byte("binary"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "download")
	if err := downloadFile(server.URL, dest, nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "binary" {
		t.Fatalf("got %q want binary", data)
	}
}

func TestDownloadFileRejectsOversizedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "268435457")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "download")
	if err := downloadFile(server.URL, dest, nil); err == nil {
		t.Fatal("expected oversized download error")
	}
}
