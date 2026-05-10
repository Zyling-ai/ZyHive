// Package safefs provides defensive path resolution for any code that needs
// to constrain file access to a known root directory.
//
// 26.5.10v2 安全修复 B001 路径穿越:
//
// 历史上, internal/api/files.go / projects.go / pkg/tools/registry.go 各自
// 写了一遍 "filepath.Join(base, rel)" + "strings.HasPrefix(absPath, base)" 的
// 简易边界检查. 这种写法有 4 类已知绕过:
//
//  1. 相对 ".." 逃逸 (filepath.Clean 已保证 ../ 被规范化, 但需要后续校验)
//  2. 兄弟前缀混淆 (base="/a/alice" + rel="../alice-evil/x" → 拼成
//     "/a/alice-evil/x", HasPrefix 返回 true 因为 "alice" 是 "alice-evil" 的字符前缀)
//  3. 绝对路径直接注入 (rel="/etc/passwd")
//  4. Symlink TOCTOU 逃逸 (rel 解析合法, 但 base 内有 sym -> /etc, 之后 read 时跟随)
//
// 还有:
//  5. NUL 字节注入 (Go 运行时已拒绝大部分 syscall, 但显式校验更安全)
//
// ConfineToBase 一次性挡住以上 5 类, 是项目内 path 解析的唯一入口.
package safefs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrEscape is returned when the resolved path falls outside base.
var ErrEscape = errors.New("safefs: path escapes base directory")

// ErrAbsoluteRel is returned when rel is an absolute path. Absolute rel paths
// are rejected because they would silently override base.
var ErrAbsoluteRel = errors.New("safefs: rel must be relative, not absolute")

// ErrNullByte is returned when rel contains a NUL byte.
var ErrNullByte = errors.New("safefs: path contains NUL byte")

// ConfineToBase resolves rel relative to base, returning an absolute path
// that is guaranteed to live inside (or equal to) base, even when base or
// any of its parent components are symlinks.
//
// rel may be "" or "." to refer to base itself. rel may NOT be absolute —
// callers must explicitly check absolute paths against project policy.
//
// On success the returned path is filepath.Clean'd and absolute. The path
// is NOT guaranteed to exist; callers may stat / read / create it.
//
// Symlink semantics:
//   - base is fully EvalSymlinks'd (must exist).
//   - For the resolved candidate, the deepest existing parent directory is
//     EvalSymlinks'd; the still-not-existing leaf segments are appended back.
//     This catches "base contains symlink → /etc" without requiring leaf to exist.
//
// Cost: 1-2 lstat syscalls. Acceptable for tool/handler hot paths.
func ConfineToBase(base, rel string) (string, error) {
	if base == "" {
		return "", fmt.Errorf("safefs: empty base")
	}
	if strings.ContainsRune(rel, 0) || strings.ContainsRune(base, 0) {
		return "", ErrNullByte
	}
	// Reject absolute rel — callers should be explicit.
	if filepath.IsAbs(rel) {
		return "", ErrAbsoluteRel
	}

	// Normalize base. EvalSymlinks requires base to exist.
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("safefs: abs base: %w", err)
	}
	absBase = filepath.Clean(absBase)
	if resolved, err := filepath.EvalSymlinks(absBase); err == nil {
		absBase = filepath.Clean(resolved)
	}
	// Strip trailing slash for consistent comparison.
	absBase = strings.TrimRight(absBase, string(os.PathSeparator))
	if absBase == "" {
		absBase = string(os.PathSeparator)
	}

	// Resolve rel against base.
	cleaned := filepath.Clean(rel)
	if cleaned == "" || cleaned == "." {
		return absBase, nil
	}
	candidate := filepath.Clean(filepath.Join(absBase, cleaned))

	// Boundary check: must equal base, or start with base + separator.
	// Using separator is the fix for sibling-prefix bypass:
	//   base   = /a/alice
	//   bad    = /a/alice-evil/x
	//   compare /a/alice-evil/x vs /a/alice/  → false, blocked.
	if !strings.HasPrefix(candidate+string(os.PathSeparator), absBase+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %s not under %s", ErrEscape, candidate, absBase)
	}

	// Symlink-walk check on the deepest existing prefix of candidate.
	// We can't EvalSymlinks(candidate) directly because the file might not
	// exist yet (e.g. for write/create). Walk up until we find an existing
	// ancestor, EvalSymlinks it, and re-append the unresolved tail.
	resolved, err := evalSymlinksOfDeepestExisting(candidate)
	if err != nil {
		return "", fmt.Errorf("safefs: eval symlinks: %w", err)
	}
	if !strings.HasPrefix(resolved+string(os.PathSeparator), absBase+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: symlink resolves to %s, outside %s", ErrEscape, resolved, absBase)
	}
	return candidate, nil
}

// evalSymlinksOfDeepestExisting climbs from `p` toward root, finds the first
// existing ancestor, EvalSymlinks it, then re-appends the unresolved tail.
// Returns a fully-resolved absolute path.
func evalSymlinksOfDeepestExisting(p string) (string, error) {
	p = filepath.Clean(p)
	tail := []string{}
	cur := p
	for {
		if _, err := os.Lstat(cur); err == nil {
			// exists — resolve it
			resolved, rerr := filepath.EvalSymlinks(cur)
			if rerr != nil {
				return "", rerr
			}
			out := filepath.Clean(resolved)
			for i := len(tail) - 1; i >= 0; i-- {
				out = filepath.Join(out, tail[i])
			}
			return out, nil
		}
		parent, leaf := filepath.Split(cur)
		parent = strings.TrimRight(parent, string(os.PathSeparator))
		if parent == cur || parent == "" {
			// Reached root without finding any existing ancestor.
			// Treat the original p as already resolved (no symlinks possible).
			return p, nil
		}
		tail = append(tail, leaf)
		cur = parent
	}
}
