package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/netguard"
	"github.com/gin-gonic/gin"
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
	if err := downloadFileWithClient(server.Client(), server.URL, dest, nil); err != nil {
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
	if err := downloadFileWithClient(server.Client(), server.URL, dest, nil); err == nil {
		t.Fatal("expected oversized download error")
	}
}

func TestDownloadFileBlocksPrivateRedirectTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("blocked update endpoint received a request")
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "download")
	if err := downloadFile(server.URL, dest, nil); !errors.Is(err, netguard.ErrBlocked) {
		t.Fatalf("expected private endpoint block, got %v", err)
	}
}

func TestUpdateHealthURLUsesAcceptedLocalAddress(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/update/apply", nil)
	req = req.WithContext(context.WithValue(
		req.Context(),
		http.LocalAddrContextKey,
		&net.TCPAddr{IP: net.ParseIP("192.168.1.20"), Port: 9090},
	))
	c, _ := newTestContext(req)

	if got, want := updateHealthURL(c, 8080), "http://192.168.1.20:9090/healthz"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPreparePendingUpdateWritesRestrictedRecord(t *testing.T) {
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "zyhive")
	backupPath := binaryPath + ".bak"
	if err := os.WriteFile(binaryPath, []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	record, err := preparePendingUpdate(
		binaryPath,
		backupPath,
		"http://127.0.0.1:8080/healthz",
		"26.8.1v5",
		"26.8.1v6",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Token) != 64 {
		t.Fatalf("unexpected token length: %d", len(record.Token))
	}
	info, err := os.Stat(pendingUpdatePath(binaryPath))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("pending mode = %o, want 600", got)
	}
	if err := validatePendingUpdate(record, binaryPath, "", false); err != nil {
		t.Fatalf("validate normal process: %v", err)
	}
	if err := validatePendingUpdate(record, backupPath, record.Token, true); err != nil {
		t.Fatalf("validate watchdog: %v", err)
	}
	if err := validatePendingUpdate(record, backupPath, "wrong", true); err == nil {
		t.Fatal("expected invalid token rejection")
	}
}

func TestCheckUpdateHealthRequiresExpectedVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": "26.8.1v6",
		})
	}))
	defer server.Close()

	if err := checkUpdateHealth(server.Client(), server.URL, "26.8.1v6"); err != nil {
		t.Fatalf("expected healthy response: %v", err)
	}
	if err := checkUpdateHealth(server.Client(), server.URL, "26.8.1v7"); err == nil {
		t.Fatal("expected wrong version rejection")
	}
}

func TestWatchdogConfirmsHealthyNewVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": "26.8.1v6",
		})
	}))
	defer server.Close()

	record := newPendingUpdateFixture(t, server.URL, "new", "old")
	if err := runUpdateWatchdogWithConfig(record, 100*time.Millisecond, time.Millisecond, server.Client()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pendingUpdatePath(record.BinaryPath)); !os.IsNotExist(err) {
		t.Fatalf("pending record was not removed: %v", err)
	}
	data, err := os.ReadFile(record.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("healthy binary changed to %q", data)
	}
}

func TestWatchdogRestoresBackupOnWrongVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": "26.8.1v999",
		})
	}))
	defer server.Close()

	record := newPendingUpdateFixture(t, server.URL, "new", "old")
	if err := runUpdateWatchdogWithConfig(record, 20*time.Millisecond, time.Millisecond, server.Client()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(record.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("binary = %q, want restored old version", data)
	}
	if _, err := os.Stat(pendingUpdatePath(record.BinaryPath)); !os.IsNotExist(err) {
		t.Fatalf("pending record was not removed: %v", err)
	}
	var result updateResult
	resultData, err := os.ReadFile(updateResultPath(record.BinaryPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatal(err)
	}
	if result.Stage != StageRolledBack {
		t.Fatalf("result stage = %q", result.Stage)
	}
}

func TestRestoreBackupRejectsUnrelatedPath(t *testing.T) {
	record := &pendingUpdate{
		BinaryPath: "/tmp/zyhive",
		BackupPath: "/tmp/unrelated",
	}
	if err := restoreBackupBinary(record); err == nil {
		t.Fatal("expected unrelated backup path rejection")
	}
}

func newPendingUpdateFixture(t *testing.T, healthURL, binaryContent, backupContent string) *pendingUpdate {
	t.Helper()
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "zyhive")
	backupPath := binaryPath + ".bak"
	if err := os.WriteFile(binaryPath, []byte(binaryContent), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte(backupContent), 0755); err != nil {
		t.Fatal(err)
	}
	record := &pendingUpdate{
		Token:           strings.Repeat("a", 64),
		OldVersion:      "26.8.1v5",
		ExpectedVersion: "26.8.1v6",
		BinaryPath:      binaryPath,
		BackupPath:      backupPath,
		HealthURL:       healthURL,
		PID:             0,
		CreatedAt:       time.Now().UTC(),
	}
	if err := writeJSONAtomic(pendingUpdatePath(binaryPath), record); err != nil {
		t.Fatal(err)
	}
	return record
}

func newTestContext(req *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c, w
}
