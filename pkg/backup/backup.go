// Package backup creates, verifies, and restores portable ZyHive backups.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/config"
)

const (
	Format          = "zyhive-backup"
	ManifestVersion = 1
	manifestName    = "manifest.json"
)

// Limits bounds archive parsing and protects inspect/restore from resource exhaustion.
type Limits struct {
	MaxArchiveSize  int64
	MaxManifestSize int64
	MaxFileSize     int64
	MaxTotalSize    int64
	MaxEntries      int
}

func DefaultLimits() Limits {
	return Limits{
		MaxArchiveSize:  16 << 30,
		MaxManifestSize: 16 << 20,
		MaxFileSize:     4 << 30,
		MaxTotalSize:    16 << 30,
		MaxEntries:      1_000_000,
	}
}

type Entry struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256,omitempty"`
	Mode    uint32 `json:"mode"`
	ModTime string `json:"modTime"`
}

type Manifest struct {
	Format     string  `json:"format"`
	Version    int     `json:"version"`
	CreatedAt  string  `json:"createdAt"`
	AppVersion string  `json:"appVersion,omitempty"`
	Entries    []Entry `json:"entries"`
}

type CreateOptions struct {
	Output     string
	ConfigPath string
	WorkDir    string
	AppVersion string
}

type RestoreOptions struct {
	Input      string
	ConfigPath string
	WorkDir    string
	Limits     Limits
}

type sourceItem struct {
	archivePath string
	localPath   string
}

type restoreItem struct {
	name   string
	target string
	stage  string
	old    string
	hadOld bool
}

var renamePath = os.Rename

// ResolveTargets derives all live locations from the current config and explicit work directory.
func ResolveTargets(configPath, workDir string) (map[string]string, error) {
	if configPath == "" {
		return nil, errors.New("config path is required")
	}
	if workDir == "" {
		return nil, errors.New("work directory is required")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load current config: %w", err)
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolve work directory: %w", err)
	}
	agentsDir := cfg.Agents.Dir
	if agentsDir == "" {
		agentsDir = "agents"
	}
	if !filepath.IsAbs(agentsDir) {
		agentsDir = filepath.Join(workDir, agentsDir)
	}
	agentsDir, err = filepath.Abs(agentsDir)
	if err != nil {
		return nil, fmt.Errorf("resolve agents directory: %w", err)
	}
	targets := map[string]string{
		"config":   filepath.Clean(configPath),
		"agents":   filepath.Clean(agentsDir),
		"projects": filepath.Join(filepath.Clean(workDir), "projects"),
		"cron":     filepath.Join(filepath.Clean(workDir), "cron"),
	}
	if err := validateDistinctTargets(targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func validateDistinctTargets(targets map[string]string) error {
	names := []string{"config", "agents", "projects", "cron"}
	for i, a := range names {
		for _, b := range names[i+1:] {
			if pathsOverlap(targets[a], targets[b]) {
				return fmt.Errorf("backup targets overlap: %s and %s", targets[a], targets[b])
			}
		}
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	rel, err := filepath.Rel(a, b)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true
	}
	rel, err = filepath.Rel(b, a)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Create writes a gzip-compressed tar archive through a same-directory temporary file.
func Create(opts CreateOptions) (*Manifest, error) {
	if opts.Output == "" {
		return nil, errors.New("output path is required")
	}
	targets, err := ResolveTargets(opts.ConfigPath, opts.WorkDir)
	if err != nil {
		return nil, err
	}
	output, err := filepath.Abs(opts.Output)
	if err != nil {
		return nil, fmt.Errorf("resolve output path: %w", err)
	}
	for name, root := range targets {
		if pathsOverlap(root, output) {
			return nil, fmt.Errorf("output is inside backup source %s: %s", name, output)
		}
	}
	items := []sourceItem{{"config", targets["config"]}}
	for _, root := range []string{"agents", "projects", "cron"} {
		items = append(items, sourceItem{root, targets[root]})
	}
	entries, err := collectEntries(items)
	if err != nil {
		return nil, err
	}
	manifest := &Manifest{
		Format: Format, Version: ManifestVersion,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		AppVersion: opts.AppVersion, Entries: entries,
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(output), ".zyhive-backup-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create temporary archive: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0600); err != nil {
		cleanup()
		return nil, fmt.Errorf("secure temporary archive: %w", err)
	}
	if err := writeArchive(tmp, manifest, items); err != nil {
		cleanup()
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return nil, fmt.Errorf("sync temporary archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return nil, fmt.Errorf("close temporary archive: %w", err)
	}
	if err := renamePath(tmpName, output); err != nil {
		_ = os.Remove(tmpName)
		return nil, fmt.Errorf("publish archive atomically: %w", err)
	}
	if err := syncDir(filepath.Dir(output)); err != nil {
		return nil, fmt.Errorf("sync output directory: %w", err)
	}
	return manifest, nil
}

func collectEntries(items []sourceItem) ([]Entry, error) {
	var entries []Entry
	for _, item := range items {
		info, err := os.Lstat(item.localPath)
		if err != nil {
			return nil, fmt.Errorf("inspect %s source: %w", item.archivePath, err)
		}
		if item.archivePath == "config" {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("config source must be a regular file")
			}
			e, err := entryFor(item.archivePath, item.localPath, info)
			if err != nil {
				return nil, err
			}
			entries = append(entries, e)
			continue
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s source must be a directory", item.archivePath)
		}
		err = filepath.WalkDir(item.localPath, func(local string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := os.Lstat(local)
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symbolic links are not allowed: %s", local)
			}
			if !info.Mode().IsRegular() && !info.IsDir() {
				return fmt.Errorf("special files are not allowed: %s", local)
			}
			rel, err := filepath.Rel(item.localPath, local)
			if err != nil {
				return err
			}
			name := item.archivePath
			if rel != "." {
				name = path.Join(name, filepath.ToSlash(rel))
			}
			e, err := entryFor(name, local, info)
			if err != nil {
				return err
			}
			entries = append(entries, e)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", item.archivePath, err)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func entryFor(name, local string, info fs.FileInfo) (Entry, error) {
	e := Entry{
		Path: name, Size: info.Size(), Mode: uint32(info.Mode().Perm()),
		ModTime: info.ModTime().UTC().Format(time.RFC3339Nano),
	}
	if info.IsDir() {
		e.Type = "directory"
		e.Size = 0
		return e, nil
	}
	e.Type = "file"
	sum, err := hashFile(local)
	if err != nil {
		return Entry{}, fmt.Errorf("hash %s: %w", local, err)
	}
	e.SHA256 = sum
	return e, nil
}

func hashFile(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeArchive(dst io.Writer, manifest *Manifest, items []sourceItem) (retErr error) {
	gz := gzip.NewWriter(dst)
	tw := tar.NewWriter(gz)
	defer func() {
		retErr = errors.Join(retErr, tw.Close(), gz.Close())
	}()
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: manifestName, Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg, ModTime: time.Now().UTC()}); err != nil {
		return fmt.Errorf("write manifest header: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	entryMap := make(map[string]Entry, len(manifest.Entries))
	for _, e := range manifest.Entries {
		entryMap[e.Path] = e
	}
	for _, item := range items {
		err := filepath.WalkDir(item.localPath, func(local string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := os.Lstat(local)
			if err != nil {
				return err
			}
			rel := "."
			if item.archivePath != "config" {
				rel, err = filepath.Rel(item.localPath, local)
				if err != nil {
					return err
				}
			}
			name := item.archivePath
			if rel != "." {
				name = path.Join(name, filepath.ToSlash(rel))
			}
			expected, ok := entryMap[name]
			if !ok {
				return fmt.Errorf("source changed while creating backup: unexpected %s", local)
			}
			if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
				return fmt.Errorf("source changed to unsupported type: %s", local)
			}
			typeflag := byte(tar.TypeReg)
			size := info.Size()
			if info.IsDir() {
				typeflag, size = tar.TypeDir, 0
			}
			hdr := &tar.Header{Name: name, Mode: int64(expected.Mode), Size: size, Typeflag: typeflag, ModTime: info.ModTime().UTC(), Format: tar.FormatPAX}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if size != expected.Size {
				return fmt.Errorf("source changed size while creating backup: %s", local)
			}
			f, err := os.Open(local)
			if err != nil {
				return err
			}
			h := sha256.New()
			_, copyErr := io.Copy(io.MultiWriter(tw, h), f)
			closeErr := f.Close()
			if copyErr != nil || closeErr != nil {
				return errors.Join(copyErr, closeErr)
			}
			if got := hex.EncodeToString(h.Sum(nil)); got != expected.SHA256 {
				return fmt.Errorf("source changed content while creating backup: %s", local)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("write %s: %w", item.archivePath, err)
		}
	}
	return nil
}

// Inspect completely validates an archive and returns its trusted manifest.
func Inspect(input string, limits Limits) (*Manifest, error) {
	return inspectArchive(input, normalizeLimits(limits), nil)
}

func normalizeLimits(l Limits) Limits {
	d := DefaultLimits()
	if l.MaxArchiveSize <= 0 {
		l.MaxArchiveSize = d.MaxArchiveSize
	}
	if l.MaxManifestSize <= 0 {
		l.MaxManifestSize = d.MaxManifestSize
	}
	if l.MaxFileSize <= 0 {
		l.MaxFileSize = d.MaxFileSize
	}
	if l.MaxTotalSize <= 0 {
		l.MaxTotalSize = d.MaxTotalSize
	}
	if l.MaxEntries <= 0 {
		l.MaxEntries = d.MaxEntries
	}
	return l
}

func inspectArchive(input string, limits Limits, extract map[string]string) (*Manifest, error) {
	info, err := os.Lstat(input)
	if err != nil {
		return nil, fmt.Errorf("inspect archive file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("archive must be a regular file")
	}
	if info.Size() > limits.MaxArchiveSize {
		return nil, fmt.Errorf("archive exceeds size limit")
	}
	f, err := os.Open(input)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("open gzip stream: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil {
		return nil, fmt.Errorf("read manifest header: %w", err)
	}
	if hdr.Name != manifestName || hdr.Typeflag != tar.TypeReg {
		return nil, errors.New("manifest.json must be the first regular archive entry")
	}
	if hdr.Size < 0 || hdr.Size > limits.MaxManifestSize {
		return nil, errors.New("manifest exceeds size limit")
	}
	raw, err := io.ReadAll(io.LimitReader(tr, limits.MaxManifestSize+1))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if int64(len(raw)) != hdr.Size {
		return nil, errors.New("truncated manifest")
	}
	var manifest Manifest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	expected, err := validateManifest(&manifest, limits)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var total int64
	for {
		hdr, err = tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		if !validArchivePath(hdr.Name) || hdr.Name == manifestName {
			return nil, fmt.Errorf("unsafe archive path: %q", hdr.Name)
		}
		if seen[hdr.Name] {
			return nil, fmt.Errorf("duplicate archive entry: %s", hdr.Name)
		}
		seen[hdr.Name] = true
		want, ok := expected[hdr.Name]
		if !ok {
			return nil, fmt.Errorf("entry absent from manifest: %s", hdr.Name)
		}
		if uint32(hdr.Mode)&0777 != want.Mode || hdr.Mode&^int64(0777) != 0 {
			return nil, fmt.Errorf("mode mismatch: %s", hdr.Name)
		}
		modTime, err := time.Parse(time.RFC3339Nano, want.ModTime)
		if err != nil || !hdr.ModTime.UTC().Equal(modTime) {
			return nil, fmt.Errorf("modTime mismatch: %s", hdr.Name)
		}
		if hdr.Size < 0 || hdr.Size > limits.MaxFileSize {
			return nil, fmt.Errorf("entry exceeds size limit: %s", hdr.Name)
		}
		if hdr.Size != want.Size {
			return nil, fmt.Errorf("size mismatch: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if want.Type != "directory" || hdr.Size != 0 {
				return nil, fmt.Errorf("type mismatch: %s", hdr.Name)
			}
			if extract != nil {
				if err := makeExtractDir(extract, hdr.Name, want); err != nil {
					return nil, err
				}
			}
		case tar.TypeReg, tar.TypeRegA:
			if want.Type != "file" {
				return nil, fmt.Errorf("type mismatch: %s", hdr.Name)
			}
			total += hdr.Size
			if total > limits.MaxTotalSize {
				return nil, errors.New("archive extracted size exceeds limit")
			}
			if err := consumeFile(tr, hdr.Name, want, extract); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported archive entry type for %s", hdr.Name)
		}
	}
	for name := range expected {
		if !seen[name] {
			return nil, fmt.Errorf("manifest entry missing from archive: %s", name)
		}
	}
	return &manifest, nil
}

func validateManifest(m *Manifest, limits Limits) (map[string]Entry, error) {
	if m.Format != Format || m.Version != ManifestVersion {
		return nil, fmt.Errorf("unsupported manifest format or version")
	}
	if _, err := time.Parse(time.RFC3339Nano, m.CreatedAt); err != nil {
		return nil, errors.New("invalid manifest creation time")
	}
	if len(m.Entries) == 0 || len(m.Entries) > limits.MaxEntries {
		return nil, errors.New("invalid manifest entry count")
	}
	out := make(map[string]Entry, len(m.Entries))
	for _, e := range m.Entries {
		if !validArchivePath(e.Path) || e.Path == manifestName {
			return nil, fmt.Errorf("unsafe manifest path: %q", e.Path)
		}
		if _, exists := out[e.Path]; exists {
			return nil, fmt.Errorf("duplicate manifest entry: %s", e.Path)
		}
		if e.Size < 0 || e.Size > limits.MaxFileSize {
			return nil, fmt.Errorf("invalid size for %s", e.Path)
		}
		if e.Mode&^uint32(0777) != 0 {
			return nil, fmt.Errorf("invalid mode for %s", e.Path)
		}
		if _, err := time.Parse(time.RFC3339Nano, e.ModTime); err != nil {
			return nil, fmt.Errorf("invalid modTime for %s", e.Path)
		}
		switch e.Type {
		case "file":
			if len(e.SHA256) != 64 {
				return nil, fmt.Errorf("invalid SHA-256 for %s", e.Path)
			}
			if _, err := hex.DecodeString(e.SHA256); err != nil {
				return nil, fmt.Errorf("invalid SHA-256 for %s", e.Path)
			}
		case "directory":
			if e.Size != 0 || e.SHA256 != "" {
				return nil, fmt.Errorf("invalid directory metadata for %s", e.Path)
			}
		default:
			return nil, fmt.Errorf("invalid type for %s", e.Path)
		}
		out[e.Path] = e
	}
	for _, root := range []string{"config", "agents", "projects", "cron"} {
		e, ok := out[root]
		if !ok {
			return nil, fmt.Errorf("required item missing: %s", root)
		}
		wantType := "directory"
		if root == "config" {
			wantType = "file"
		}
		if e.Type != wantType {
			return nil, fmt.Errorf("required item has wrong type: %s", root)
		}
	}
	return out, nil
}

func validArchivePath(name string) bool {
	if name == "" || strings.ContainsRune(name, '\\') || strings.HasPrefix(name, "/") {
		return false
	}
	clean := path.Clean(name)
	if clean != name || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	root := strings.Split(clean, "/")[0]
	if root == "config" {
		return clean == "config"
	}
	return root == "config" || root == "agents" || root == "projects" || root == "cron"
}

func consumeFile(r io.Reader, name string, want Entry, extract map[string]string) error {
	h := sha256.New()
	var w io.Writer = h
	var file *os.File
	if extract != nil {
		dest, err := extractPath(extract, name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return fmt.Errorf("create staging parent: %w", err)
		}
		mode := fs.FileMode(want.Mode) & 0777
		if name == "config" {
			mode = 0600
		}
		file, err = os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return fmt.Errorf("create staged file %s: %w", name, err)
		}
		w = io.MultiWriter(h, file)
	}
	_, copyErr := io.CopyN(w, r, want.Size)
	var closeErr error
	if file != nil {
		closeErr = file.Close()
	}
	if copyErr != nil || closeErr != nil {
		return errors.Join(copyErr, closeErr)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want.SHA256 {
		return fmt.Errorf("SHA-256 mismatch: %s", name)
	}
	return nil
}

func makeExtractDir(extract map[string]string, name string, want Entry) error {
	dest, err := extractPath(extract, name)
	if err != nil {
		return err
	}
	mode := fs.FileMode(want.Mode) & 0777
	if mode == 0 {
		mode = 0700
	}
	if err := os.MkdirAll(dest, mode); err != nil {
		return fmt.Errorf("create staged directory %s: %w", name, err)
	}
	return nil
}

func extractPath(roots map[string]string, name string) (string, error) {
	parts := strings.Split(name, "/")
	root, ok := roots[parts[0]]
	if !ok {
		return "", fmt.Errorf("unknown archive root: %s", parts[0])
	}
	if len(parts) == 1 {
		return root, nil
	}
	dest := filepath.Join(append([]string{root}, parts[1:]...)...)
	rel, err := filepath.Rel(root, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive path escapes staging root: %s", name)
	}
	return dest, nil
}

// Restore verifies the complete archive, stages it, then atomically swaps all live targets.
func Restore(opts RestoreOptions) (*Manifest, error) {
	limits := normalizeLimits(opts.Limits)
	manifest, err := Inspect(opts.Input, limits)
	if err != nil {
		return nil, fmt.Errorf("pre-restore validation failed: %w", err)
	}
	targets, err := ResolveTargets(opts.ConfigPath, opts.WorkDir)
	if err != nil {
		return nil, err
	}
	names := []string{"config", "agents", "projects", "cron"}
	items := make([]restoreItem, 0, len(names))
	extract := make(map[string]string, len(names))
	for _, name := range names {
		target := targets[name]
		parent := filepath.Dir(target)
		if err := os.MkdirAll(parent, 0700); err != nil {
			cleanupStages(items)
			return nil, fmt.Errorf("create target parent for %s: %w", name, err)
		}
		container, err := os.MkdirTemp(parent, ".zyhive-restore-"+name+"-*")
		if err != nil {
			cleanupStages(items)
			return nil, fmt.Errorf("create staging area for %s: %w", name, err)
		}
		stage := filepath.Join(container, "payload")
		items = append(items, restoreItem{name: name, target: target, stage: stage})
		extract[name] = stage
	}
	defer cleanupStages(items)
	if _, err := inspectArchive(opts.Input, limits, extract); err != nil {
		return nil, fmt.Errorf("staging validation failed: %w", err)
	}
	if err := commitRestore(items); err != nil {
		return nil, err
	}
	return manifest, nil
}

func commitRestore(items []restoreItem) error {
	committed := 0
	for i := range items {
		item := &items[i]
		info, err := os.Lstat(item.target)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return rollback(items, committed, fmt.Errorf("restore target is a symbolic link: %s", item.target))
			}
			if item.name == "config" && !info.Mode().IsRegular() {
				return rollback(items, committed, fmt.Errorf("config target is not a regular file"))
			}
			if item.name != "config" && !info.IsDir() {
				return rollback(items, committed, fmt.Errorf("%s target is not a directory", item.name))
			}
			old, err := uniqueSibling(item.target, ".zyhive-rollback-")
			if err != nil {
				return rollback(items, committed, err)
			}
			if err := renamePath(item.target, old); err != nil {
				return rollback(items, committed, fmt.Errorf("move old %s aside: %w", item.name, err))
			}
			item.old, item.hadOld = old, true
		} else if !os.IsNotExist(err) {
			return rollback(items, committed, fmt.Errorf("inspect restore target %s: %w", item.name, err))
		}
		if err := renamePath(item.stage, item.target); err != nil {
			return rollback(items, committed+1, fmt.Errorf("install restored %s: %w", item.name, err))
		}
		committed++
	}
	var errs []error
	for i := range items {
		if items[i].hadOld {
			if err := os.RemoveAll(items[i].old); err != nil {
				errs = append(errs, fmt.Errorf("remove rollback copy for %s: %w", items[i].name, err))
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func rollback(items []restoreItem, count int, cause error) error {
	errs := []error{cause}
	if count > len(items) {
		count = len(items)
	}
	for i := count - 1; i >= 0; i-- {
		item := &items[i]
		_ = os.RemoveAll(item.target)
		if item.hadOld {
			if err := renamePath(item.old, item.target); err != nil {
				errs = append(errs, fmt.Errorf("rollback %s: %w", item.name, err))
			}
		}
	}
	return fmt.Errorf("restore failed and was rolled back: %w", errors.Join(errs...))
}

func uniqueSibling(target, prefix string) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(target), prefix+filepath.Base(target)+"-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func cleanupStages(items []restoreItem) {
	for _, item := range items {
		_ = os.RemoveAll(filepath.Dir(item.stage))
	}
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
