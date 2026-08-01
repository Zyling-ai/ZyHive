package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	root, config, work, agents, archive string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "runtime")
	agents := filepath.Join(root, "agent-data")
	configPath := filepath.Join(root, "config", "zyhive.json")
	for _, dir := range []string{
		filepath.Dir(configPath), agents,
		filepath.Join(work, "projects"), filepath.Join(work, "cron", "goals"),
		filepath.Join(work, "cron", "checks"), filepath.Join(work, "cron", "runs"),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := `{"configVersion":3,"gateway":{"port":8080,"bind":"localhost"},"agents":{"dir":"` +
		filepath.ToSlash(agents) + `"},"models":[],"channels":[],"tools":[],"skills":[],"auth":{"token":"secret"}}`
	writeFile(t, configPath, cfg)
	writeFile(t, filepath.Join(agents, "main", "config.json"), `{"name":"main"}`)
	writeFile(t, filepath.Join(work, "projects", "alpha.md"), "project-v1")
	writeFile(t, filepath.Join(work, "cron", "goals", "goal.json"), "goal-v1")
	writeFile(t, filepath.Join(work, "cron", "checks", "check.json"), "check-v1")
	writeFile(t, filepath.Join(work, "cron", "runs", "run.json"), "run-v1")
	return fixture{root: root, config: configPath, work: work, agents: agents, archive: filepath.Join(root, "out", "backup.tar.gz")}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func createFixtureArchive(t *testing.T, f fixture) *Manifest {
	t.Helper()
	m, err := Create(CreateOptions{Output: f.archive, ConfigPath: f.config, WorkDir: f.work, AppVersion: "v11-test"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCreateInspectRestore(t *testing.T) {
	f := newFixture(t)
	m := createFixtureArchive(t, f)
	if m.Format != Format || m.Version != ManifestVersion {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	info, err := os.Stat(f.archive)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("archive permissions too broad: %o", info.Mode().Perm())
	}
	inspected, err := Inspect(f.archive, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspected.Entries) < 10 {
		t.Fatalf("too few entries: %d", len(inspected.Entries))
	}

	writeFile(t, filepath.Join(f.agents, "main", "config.json"), "destroyed")
	writeFile(t, filepath.Join(f.work, "projects", "alpha.md"), "destroyed")
	if err := os.RemoveAll(filepath.Join(f.work, "cron")); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(RestoreOptions{Input: f.archive, ConfigPath: f.config, WorkDir: f.work}); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(f.agents, "main", "config.json"), `{"name":"main"}`)
	assertFile(t, filepath.Join(f.work, "projects", "alpha.md"), "project-v1")
	assertFile(t, filepath.Join(f.work, "cron", "goals", "goal.json"), "goal-v1")
}

func TestCreateRejectsSymlinkAndRecursiveOutput(t *testing.T) {
	f := newFixture(t)
	if err := os.Symlink(filepath.Join(f.work, "projects", "alpha.md"), filepath.Join(f.agents, "link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := Create(CreateOptions{Output: f.archive, ConfigPath: f.config, WorkDir: f.work}); err == nil || !strings.Contains(err.Error(), "symbolic") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
	if err := os.Remove(filepath.Join(f.agents, "link")); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(f.work, "projects", "backup.tar.gz")
	if _, err := Create(CreateOptions{Output: inside, ConfigPath: f.config, WorkDir: f.work}); err == nil || !strings.Contains(err.Error(), "inside backup source") {
		t.Fatalf("expected recursive output rejection, got %v", err)
	}
}

func TestInspectRejectsCorruptDigest(t *testing.T) {
	f := newFixture(t)
	createFixtureArchive(t, f)
	rewriteArchive(t, f.archive, func(h *tar.Header, data []byte) (*tar.Header, []byte, bool) {
		if h.Name == "projects/alpha.md" {
			data = append([]byte(nil), data...)
			data[0] ^= 0xff
		}
		return h, data, true
	})
	if _, err := Inspect(f.archive, Limits{}); err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("expected digest rejection, got %v", err)
	}
}

func TestInspectRejectsTraversal(t *testing.T) {
	f := newFixture(t)
	createFixtureArchive(t, f)
	rewriteArchive(t, f.archive, func(h *tar.Header, data []byte) (*tar.Header, []byte, bool) {
		if h.Name == manifestName {
			var m Manifest
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatal(err)
			}
			m.Entries[0].Path = "../escape"
			data, _ = json.Marshal(m)
			h.Size = int64(len(data))
		}
		return h, data, true
	})
	if _, err := Inspect(f.archive, Limits{}); err == nil || !strings.Contains(err.Error(), "unsafe manifest path") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestInspectRejectsSymlinkEntry(t *testing.T) {
	f := newFixture(t)
	createFixtureArchive(t, f)
	rewriteArchive(t, f.archive, func(h *tar.Header, data []byte) (*tar.Header, []byte, bool) {
		if h.Name == "projects/alpha.md" {
			h.Typeflag = tar.TypeSymlink
			h.Linkname = "../../escape"
			h.Size = 0
			data = nil
		}
		return h, data, true
	})
	if _, err := Inspect(f.archive, Limits{}); err == nil || !strings.Contains(err.Error(), "size mismatch") && !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected symlink entry rejection, got %v", err)
	}
}

func TestInspectRejectsMissingManifest(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Dir(f.archive), 0700); err != nil {
		t.Fatal(err)
	}
	writeTarGz(t, f.archive, []archiveRecord{{header: tar.Header{Name: "config", Typeflag: tar.TypeReg, Mode: 0600, Size: 2}, data: []byte("{}")}})
	if _, err := Inspect(f.archive, Limits{}); err == nil || !strings.Contains(err.Error(), "manifest.json") {
		t.Fatalf("expected missing manifest rejection, got %v", err)
	}
}

func TestInspectRejectsIncompleteArchive(t *testing.T) {
	f := newFixture(t)
	createFixtureArchive(t, f)
	rewriteArchive(t, f.archive, func(h *tar.Header, data []byte) (*tar.Header, []byte, bool) {
		return h, data, h.Name != "cron"
	})
	if _, err := Inspect(f.archive, Limits{}); err == nil || !strings.Contains(err.Error(), "missing from archive") {
		t.Fatalf("expected incomplete archive rejection, got %v", err)
	}
}

func TestRestoreAtomicFailureRollsBack(t *testing.T) {
	f := newFixture(t)
	createFixtureArchive(t, f)
	writeFile(t, f.config, strings.Replace(string(mustRead(t, f.config)), "secret", "live-secret", 1))
	writeFile(t, filepath.Join(f.agents, "main", "config.json"), "live-agent")
	writeFile(t, filepath.Join(f.work, "projects", "alpha.md"), "live-project")
	writeFile(t, filepath.Join(f.work, "cron", "goals", "goal.json"), "live-goal")

	originalRename := renamePath
	calls := 0
	renamePath = func(old, new string) error {
		calls++
		if calls == 4 {
			return errors.New("injected rename failure")
		}
		return originalRename(old, new)
	}
	defer func() { renamePath = originalRename }()
	if _, err := Restore(RestoreOptions{Input: f.archive, ConfigPath: f.config, WorkDir: f.work}); err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback error, got %v", err)
	}
	assertContains(t, f.config, "live-secret")
	assertFile(t, filepath.Join(f.agents, "main", "config.json"), "live-agent")
	assertFile(t, filepath.Join(f.work, "projects", "alpha.md"), "live-project")
	assertFile(t, filepath.Join(f.work, "cron", "goals", "goal.json"), "live-goal")
}

func TestInspectSizeLimit(t *testing.T) {
	f := newFixture(t)
	createFixtureArchive(t, f)
	if _, err := Inspect(f.archive, Limits{MaxTotalSize: 1}); err == nil || !strings.Contains(err.Error(), "size exceeds") {
		t.Fatalf("expected size limit rejection, got %v", err)
	}
}

func assertFile(t *testing.T, name, want string) {
	t.Helper()
	if got := string(mustRead(t, name)); got != want {
		t.Fatalf("%s = %q, want %q", name, got, want)
	}
}

func assertContains(t *testing.T, name, want string) {
	t.Helper()
	if got := string(mustRead(t, name)); !strings.Contains(got, want) {
		t.Fatalf("%s does not contain %q: %s", name, want, got)
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type archiveRecord struct {
	header tar.Header
	data   []byte
}

func rewriteArchive(t *testing.T, name string, mutate func(*tar.Header, []byte) (*tar.Header, []byte, bool)) {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var records []archiveRecord
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		copyHeader := *h
		h2, data2, keep := mutate(&copyHeader, data)
		if keep {
			h2.Size = int64(len(data2))
			records = append(records, archiveRecord{header: *h2, data: data2})
		}
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	writeTarGz(t, name, records)
}

func writeTarGz(t *testing.T, name string, records []archiveRecord) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, record := range records {
		h := record.header
		if h.ModTime.IsZero() {
			h.ModTime = time.Now().UTC()
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(record.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}
