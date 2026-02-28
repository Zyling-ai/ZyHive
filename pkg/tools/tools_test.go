package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func newTestRegistry(t *testing.T) (*Registry, string) {
	t.Helper()
	ws := t.TempDir()
	agentDir := t.TempDir()
	r := New(ws, agentDir, "test-agent")
	return r, ws
}

func call(r *Registry, tool string, input any) (string, error) {
	return r.Execute(context.Background(), tool, mustJSON(input))
}

func assertOK(t *testing.T, label, result string, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("[%s] unexpected error: %v", label, err)
	}
	if result == "" {
		t.Errorf("[%s] expected non-empty result", label)
	}
}

func assertErr(t *testing.T, label, expected string, err error) {
	t.Helper()
	if err == nil {
		t.Errorf("[%s] expected error containing %q, got nil", label, expected)
		return
	}
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("[%s] error %q does not contain %q", label, err.Error(), expected)
	}
}

// ─── READ ────────────────────────────────────────────────────────────────────

func TestRead(t *testing.T) {
	r, ws := newTestRegistry(t)

	content := "line1\nline2\nline3\nline4\nline5"
	fp := filepath.Join(ws, "test.txt")
	os.WriteFile(fp, []byte(content), 0644)

	t.Run("normal_read", func(t *testing.T) {
		res, err := call(r, "read", map[string]any{"file_path": "test.txt"})
		assertOK(t, "read/normal", res, err)
		if !strings.Contains(res, "line1") {
			t.Errorf("expected content, got %q", res)
		}
	})

	t.Run("offset_and_limit", func(t *testing.T) {
		res, err := call(r, "read", map[string]any{
			"file_path": "test.txt",
			"offset":    2,
			"limit":     2,
		})
		assertOK(t, "read/offset", res, err)
		if !strings.Contains(res, "line2") || strings.Contains(res, "line4") {
			t.Errorf("offset/limit not working, got %q", res)
		}
	})

	t.Run("file_not_found", func(t *testing.T) {
		_, err := call(r, "read", map[string]any{"file_path": "nonexistent.txt"})
		assertErr(t, "read/not-found", "file not found", err)
	})

	t.Run("missing_file_path", func(t *testing.T) {
		_, err := call(r, "read", map[string]any{})
		assertErr(t, "read/no-path", "file_path is required", err)
	})

	t.Run("invalid_json", func(t *testing.T) {
		_, err := r.Execute(context.Background(), "read", json.RawMessage(`{invalid}`))
		assertErr(t, "read/bad-json", "invalid input", err)
	})

	t.Run("offset_beyond_file", func(t *testing.T) {
		_, err := call(r, "read", map[string]any{
			"file_path": "test.txt",
			"offset":    9999,
		})
		assertErr(t, "read/big-offset", "offset", err)
	})

	_ = fp
}

// ─── WRITE ───────────────────────────────────────────────────────────────────

func TestWrite(t *testing.T) {
	r, ws := newTestRegistry(t)

	t.Run("normal_write", func(t *testing.T) {
		res, err := call(r, "write", map[string]any{
			"file_path": "out.txt",
			"content":   "hello world",
		})
		assertOK(t, "write/normal", res, err)
		if !strings.Contains(res, "Written") {
			t.Errorf("expected Written message, got %q", res)
		}
		data, _ := os.ReadFile(filepath.Join(ws, "out.txt"))
		if string(data) != "hello world" {
			t.Errorf("content mismatch: %q", data)
		}
	})

	t.Run("auto_create_parent_dirs", func(t *testing.T) {
		res, err := call(r, "write", map[string]any{
			"file_path": "deep/nested/dir/file.txt",
			"content":   "nested",
		})
		assertOK(t, "write/nested", res, err)
		data, _ := os.ReadFile(filepath.Join(ws, "deep/nested/dir/file.txt"))
		if string(data) != "nested" {
			t.Errorf("nested write mismatch: %q", data)
		}
	})

	t.Run("missing_file_path", func(t *testing.T) {
		_, err := call(r, "write", map[string]any{"content": "hello"})
		assertErr(t, "write/no-path", "file_path is required", err)
	})

	t.Run("invalid_json", func(t *testing.T) {
		_, err := r.Execute(context.Background(), "write", json.RawMessage(`{bad}`))
		assertErr(t, "write/bad-json", "invalid input", err)
	})

	t.Run("empty_content_allowed", func(t *testing.T) {
		res, err := call(r, "write", map[string]any{
			"file_path": "empty.txt",
			"content":   "",
		})
		assertOK(t, "write/empty", res, err)
	})
}

// ─── EDIT ────────────────────────────────────────────────────────────────────

func TestEdit(t *testing.T) {
	r, ws := newTestRegistry(t)

	newFile := func(content string) string {
		fp := filepath.Join(ws, fmt.Sprintf("edit_%d.txt", time.Now().UnixNano()))
		os.WriteFile(fp, []byte(content), 0644)
		return fp
	}

	t.Run("normal_edit", func(t *testing.T) {
		fp := newFile("hello world\nfoo bar\n")
		res, err := call(r, "edit", map[string]any{
			"file_path":  fp,
			"old_string": "hello world",
			"new_string": "goodbye world",
		})
		assertOK(t, "edit/normal", res, err)
		data, _ := os.ReadFile(fp)
		if !strings.Contains(string(data), "goodbye world") {
			t.Errorf("edit not applied: %q", data)
		}
	})

	t.Run("old_string_not_found_with_preview", func(t *testing.T) {
		fp := newFile("actual content here")
		_, err := call(r, "edit", map[string]any{
			"file_path":  fp,
			"old_string": "this does not exist",
			"new_string": "replacement",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		msg := err.Error()
		if !strings.Contains(msg, "not found") {
			t.Errorf("error should say 'not found', got: %q", msg)
		}
		// Must include file preview so LLM can debug whitespace issues
		if !strings.Contains(msg, "actual content") {
			t.Errorf("error should include file preview, got: %q", msg)
		}
		// Must include byte count hint
		if !strings.Contains(msg, "bytes") {
			t.Errorf("error should include file size, got: %q", msg)
		}
	})

	t.Run("file_not_found", func(t *testing.T) {
		_, err := call(r, "edit", map[string]any{
			"file_path":  "/nonexistent/path/file.txt",
			"old_string": "x",
			"new_string": "y",
		})
		assertErr(t, "edit/no-file", "file not found", err)
	})

	t.Run("empty_old_string_rejected", func(t *testing.T) {
		fp := newFile("content")
		_, err := call(r, "edit", map[string]any{
			"file_path":  fp,
			"old_string": "",
			"new_string": "replacement",
		})
		assertErr(t, "edit/empty-old", "old_string is required", err)
	})

	t.Run("missing_file_path", func(t *testing.T) {
		_, err := call(r, "edit", map[string]any{
			"old_string": "x",
			"new_string": "y",
		})
		assertErr(t, "edit/no-path", "file_path is required", err)
	})

	t.Run("invalid_json", func(t *testing.T) {
		_, err := r.Execute(context.Background(), "edit", json.RawMessage(`{bad}`))
		assertErr(t, "edit/bad-json", "invalid input", err)
	})

	t.Run("only_first_occurrence_replaced", func(t *testing.T) {
		fp := newFile("abc abc abc")
		call(r, "edit", map[string]any{ //nolint
			"file_path":  fp,
			"old_string": "abc",
			"new_string": "XYZ",
		})
		data, _ := os.ReadFile(fp)
		if strings.Count(string(data), "abc") != 2 {
			t.Errorf("should replace only first occurrence, got: %q", data)
		}
	})
}

// ─── EXEC (bash) ─────────────────────────────────────────────────────────────

func TestExec(t *testing.T) {
	r, _ := newTestRegistry(t)

	t.Run("simple_success", func(t *testing.T) {
		res, err := call(r, "exec", map[string]any{"command": "echo hello"})
		assertOK(t, "exec/simple", res, err)
		if !strings.Contains(res, "hello") {
			t.Errorf("expected 'hello', got %q", res)
		}
	})

	t.Run("no_output_returns_message", func(t *testing.T) {
		res, err := call(r, "exec", map[string]any{"command": "true"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "completed successfully") {
			t.Errorf("expected 'completed successfully', got %q", res)
		}
	})

	t.Run("failing_command_exit_code_in_result", func(t *testing.T) {
		res, err := call(r, "exec", map[string]any{"command": "exit 42"})
		// Bash failures return as result (not error) so LLM sees full output
		if err != nil {
			t.Errorf("exec should not return go error, got: %v", err)
		}
		if !strings.Contains(res, "42") {
			t.Errorf("expected exit code 42 in result, got %q", res)
		}
		if !strings.Contains(res, "❌") {
			t.Errorf("expected failure indicator ❌, got %q", res)
		}
	})

	t.Run("output_preserved_on_failure", func(t *testing.T) {
		res, err := call(r, "exec", map[string]any{
			"command": "echo 'before error' && exit 1",
		})
		if err != nil {
			t.Errorf("exec should not return go error, got: %v", err)
		}
		if !strings.Contains(res, "before error") {
			t.Errorf("expected command output preserved, got %q", res)
		}
		if !strings.Contains(res, "1") {
			t.Errorf("expected exit code 1, got %q", res)
		}
	})

	t.Run("nonexistent_command", func(t *testing.T) {
		res, err := call(r, "exec", map[string]any{
			"command": "definitely_nonexistent_cmd_xyz123",
		})
		if err != nil {
			t.Errorf("exec should not return go error, got: %v", err)
		}
		// stderr should be captured and shown
		if res == "" {
			t.Errorf("expected non-empty result for failing command")
		}
	})

	t.Run("multiline_output", func(t *testing.T) {
		res, err := call(r, "exec", map[string]any{
			"command": "printf 'line1\nline2\nline3\n'",
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "line1") || !strings.Contains(res, "line3") {
			t.Errorf("expected multiline output, got %q", res)
		}
	})

	t.Run("stderr_merged_with_stdout", func(t *testing.T) {
		res, err := call(r, "exec", map[string]any{
			"command": "echo stdout && echo stderr >&2",
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "stdout") {
			t.Errorf("expected stdout in output, got %q", res)
		}
	})
}

// ─── GREP ────────────────────────────────────────────────────────────────────

func TestGrep(t *testing.T) {
	r, ws := newTestRegistry(t)
	os.WriteFile(filepath.Join(ws, "a.txt"), []byte("hello world\nfoo bar\nbaz"), 0644)
	os.WriteFile(filepath.Join(ws, "b.txt"), []byte("hello again\nnothing"), 0644)

	t.Run("pattern_found", func(t *testing.T) {
		res, err := call(r, "grep", map[string]any{
			"pattern": "hello",
			"path":    "a.txt",
		})
		assertOK(t, "grep/found", res, err)
		if !strings.Contains(res, "hello") {
			t.Errorf("expected match, got %q", res)
		}
	})

	t.Run("no_match_explicit_message", func(t *testing.T) {
		res, err := call(r, "grep", map[string]any{
			"pattern": "zzznomatch_xyz",
			"path":    "a.txt",
		})
		if err != nil {
			t.Errorf("no-match should not error, got: %v", err)
		}
		if !strings.Contains(res, "No matches") {
			t.Errorf("expected 'No matches' message, got %q", res)
		}
	})

	t.Run("invalid_regex", func(t *testing.T) {
		_, err := call(r, "grep", map[string]any{
			"pattern": "[invalid",
			"path":    "a.txt",
		})
		assertErr(t, "grep/bad-regex", "invalid regex", err)
	})

	t.Run("path_not_found", func(t *testing.T) {
		_, err := call(r, "grep", map[string]any{
			"pattern": "foo",
			"path":    "nonexistent_file_xyz.txt",
		})
		assertErr(t, "grep/no-path", "not found", err)
	})

	t.Run("missing_pattern", func(t *testing.T) {
		_, err := call(r, "grep", map[string]any{"path": "a.txt"})
		assertErr(t, "grep/no-pattern", "pattern is required", err)
	})

	t.Run("missing_path_defaults_to_workspace_recursive_works", func(t *testing.T) {
		// handleGrepWS defaults path to workspaceDir. Without -r it fails on a directory
		// (correct grep behavior). With recursive=true it should work.
		res, err := call(r, "grep", map[string]any{"pattern": "hello", "recursive": true})
		if err != nil {
			t.Errorf("grep with recursive=true on workspace dir should not error: %v", err)
		}
		_ = res
	})

	t.Run("recursive", func(t *testing.T) {
		subdir := filepath.Join(ws, "sub")
		os.MkdirAll(subdir, 0755)
		os.WriteFile(filepath.Join(subdir, "c.txt"), []byte("hello sub"), 0644)
		res, err := call(r, "grep", map[string]any{
			"pattern":   "hello",
			"path":      ws,
			"recursive": true,
		})
		if err != nil {
			t.Errorf("recursive grep error: %v", err)
		}
		if !strings.Contains(res, "hello") {
			t.Errorf("expected recursive match, got %q", res)
		}
	})
}

// ─── GLOB ────────────────────────────────────────────────────────────────────

func TestGlob(t *testing.T) {
	r, ws := newTestRegistry(t)
	os.WriteFile(filepath.Join(ws, "file1.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(ws, "file2.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(ws, "other.txt"), []byte("text"), 0644)

	t.Run("by_extension", func(t *testing.T) {
		res, err := call(r, "glob", map[string]any{"pattern": "*.go"})
		assertOK(t, "glob/go-files", res, err)
		if !strings.Contains(res, "file1.go") {
			t.Errorf("expected .go files, got %q", res)
		}
		if strings.Contains(res, "other.txt") {
			t.Errorf("should not include .txt, got %q", res)
		}
	})

	t.Run("no_matches", func(t *testing.T) {
		// Should not error, just empty
		_, err := call(r, "glob", map[string]any{"pattern": "*.xyz"})
		if err != nil {
			t.Errorf("no-match glob should not error: %v", err)
		}
	})

	t.Run("malformed_glob", func(t *testing.T) {
		// Go filepath.Glob returns error on malformed patterns like "["
		_, err := call(r, "glob", map[string]any{"pattern": "["})
		// Error is acceptable (malformed pattern)
		_ = err
	})
}

// ─── WEB FETCH ───────────────────────────────────────────────────────────────

func TestWebFetch(t *testing.T) {
	r, _ := newTestRegistry(t)

	t.Run("missing_url", func(t *testing.T) {
		_, err := call(r, "web_fetch", map[string]any{})
		assertErr(t, "web_fetch/no-url", "url is required", err)
	})

	t.Run("invalid_json", func(t *testing.T) {
		_, err := r.Execute(context.Background(), "web_fetch", json.RawMessage(`{bad}`))
		assertErr(t, "web_fetch/bad-json", "invalid input", err)
	})

	t.Run("connection_refused_explicit_error", func(t *testing.T) {
		_, err := call(r, "web_fetch", map[string]any{
			"url": "http://localhost:19998/test", // nothing listening here
		})
		if err == nil {
			t.Skip("port 19998 unexpectedly open")
		}
		if !strings.Contains(err.Error(), "request failed") {
			t.Errorf("expected 'request failed' in error, got: %q", err.Error())
		}
	})

	// Network-dependent tests — skip if offline
	t.Run("http_404_explicit_status", func(t *testing.T) {
		_, err := call(r, "web_fetch", map[string]any{
			"url": "https://httpbin.org/status/404",
		})
		if err == nil {
			t.Skip("either httpbin unreachable or returned 404 body without error (skip)")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected HTTP 404 in error, got: %q", err.Error())
		}
	})

	t.Run("http_500_explicit_status", func(t *testing.T) {
		_, err := call(r, "web_fetch", map[string]any{
			"url": "https://httpbin.org/status/500",
		})
		if err == nil {
			t.Skip("httpbin unreachable or 500 not triggered")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("expected HTTP 500 in error, got: %q", err.Error())
		}
	})
}

// ─── UNKNOWN TOOL ─────────────────────────────────────────────────────────────

func TestUnknownTool(t *testing.T) {
	r, _ := newTestRegistry(t)

	_, err := r.Execute(context.Background(), "totally_fake_tool_xyz", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	msg := err.Error()
	if !strings.Contains(msg, "totally_fake_tool_xyz") {
		t.Errorf("error must name the bad tool, got: %q", msg)
	}
	if !strings.Contains(msg, "available tools") {
		t.Errorf("error must list available tools, got: %q", msg)
	}
}

// ─── ERROR WRAPPING (tool name prefix) ───────────────────────────────────────

func TestErrorWrapping(t *testing.T) {
	r, _ := newTestRegistry(t)

	_, err := call(r, "read", map[string]any{"file_path": "missing_xyz_abc.txt"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "[read]") {
		t.Errorf("error should be prefixed with [read], got: %q", err.Error())
	}
}

// ─── PARTIAL OUTPUT + ERROR COMBINING ────────────────────────────────────────
// Verifies that when a handler returns (nonEmptyResult, error), Execute returns
// both so the runner can combine them.

func TestPartialOutputAndError(t *testing.T) {
	r, _ := newTestRegistry(t)

	r.register(
		llm.ToolDef{
			Name:        "test_partial",
			Description: "test",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
		func(_ context.Context, _ json.RawMessage) (string, error) {
			return "partial output here", fmt.Errorf("something failed")
		},
	)

	res, err := r.Execute(context.Background(), "test_partial", json.RawMessage(`{}`))
	if res != "partial output here" {
		t.Errorf("partial result should be preserved, got %q", res)
	}
	if err == nil || !strings.Contains(err.Error(), "something failed") {
		t.Errorf("error should propagate, got: %v", err)
	}
	// The runner (executeTools) will combine these into:
	// "ERROR: [test_partial] something failed\n\nOutput:\npartial output here"
}

// ─── SELF-MANAGEMENT ──────────────────────────────────────────────────────────

func TestSelfManagement(t *testing.T) {
	r, _ := newTestRegistry(t)

	t.Run("list_skills_no_error", func(t *testing.T) {
		_, err := call(r, "self_list_skills", map[string]any{})
		if err != nil {
			t.Errorf("self_list_skills unexpected error: %v", err)
		}
	})

	t.Run("install_skill_missing_id", func(t *testing.T) {
		_, err := call(r, "self_install_skill", map[string]any{"name": "MySkill"})
		assertErr(t, "install/no-id", "id is required", err)
	})

	t.Run("install_skill_valid", func(t *testing.T) {
		res, err := call(r, "self_install_skill", map[string]any{
			"id":   "test-skill-xyz",
			"name": "Test",
		})
		assertOK(t, "install/valid", res, err)
	})

	t.Run("uninstall_skill_missing_id", func(t *testing.T) {
		_, err := call(r, "self_uninstall_skill", map[string]any{})
		assertErr(t, "uninstall/no-id", "id is required", err)
	})

	t.Run("rename_missing_name", func(t *testing.T) {
		_, err := call(r, "self_rename", map[string]any{})
		assertErr(t, "rename/no-name", "name is required", err)
	})
}

// ─── ENV VARS ─────────────────────────────────────────────────────────────────

func TestEnvVarTools(t *testing.T) {
	t.Run("set_env_missing_key", func(t *testing.T) {
		r, _ := newTestRegistry(t)
		r.WithEnvUpdater(func(key, value string, remove bool) error { return nil })
		_, err := call(r, "self_set_env", map[string]any{"value": "bar"})
		assertErr(t, "set_env/no-key", "key and value required", err)
	})

	t.Run("set_env_valid", func(t *testing.T) {
		r, _ := newTestRegistry(t)
		r.WithEnvUpdater(func(key, value string, remove bool) error { return nil })
		res, err := call(r, "self_set_env", map[string]any{"key": "FOO", "value": "bar"})
		assertOK(t, "set_env/valid", res, err)
		if !strings.Contains(res, "FOO") {
			t.Errorf("expected key name in result, got %q", res)
		}
	})

	t.Run("set_env_updater_error_propagated", func(t *testing.T) {
		r, _ := newTestRegistry(t)
		r.WithEnvUpdater(func(key, value string, remove bool) error {
			return fmt.Errorf("disk is full")
		})
		_, err := call(r, "self_set_env", map[string]any{"key": "FOO", "value": "bar"})
		assertErr(t, "set_env/updater-err", "disk is full", err)
	})

	t.Run("delete_env_missing_key", func(t *testing.T) {
		r, _ := newTestRegistry(t)
		r.WithEnvUpdater(func(key, value string, remove bool) error { return nil })
		_, err := call(r, "self_delete_env", map[string]any{})
		assertErr(t, "del_env/no-key", "key required", err)
	})
}

// ─── AGENT SPAWN ──────────────────────────────────────────────────────────────

func TestAgentSpawn(t *testing.T) {
	r, _ := newTestRegistry(t)

	t.Run("no_manager_clear_error", func(t *testing.T) {
		// agent_spawn is always registered; without a subagent manager
		// it should return "not configured" (not "unknown tool")
		_, err := call(r, "agent_spawn", map[string]any{
			"agentId": "some-agent",
			"task":    "do something",
		})
		assertErr(t, "spawn/no-mgr", "not configured", err)
	})

	t.Run("agent_tasks_no_manager", func(t *testing.T) {
		_, err := call(r, "agent_tasks", map[string]any{})
		assertErr(t, "tasks/no-mgr", "not configured", err)
	})

	t.Run("agent_kill_no_manager", func(t *testing.T) {
		_, err := call(r, "agent_kill", map[string]any{"taskId": "xyz"})
		assertErr(t, "kill/no-mgr", "not configured", err)
	})

	t.Run("agent_result_no_manager", func(t *testing.T) {
		_, err := call(r, "agent_result", map[string]any{"taskId": "xyz"})
		assertErr(t, "result/no-mgr", "not configured", err)
	})
}

// ─── SHOW IMAGE ───────────────────────────────────────────────────────────────

func TestShowImage(t *testing.T) {
	r, ws := newTestRegistry(t)

	t.Run("valid_png", func(t *testing.T) {
		fp := filepath.Join(ws, "img.png")
		os.WriteFile(fp, []byte("fakepng"), 0644)
		res, err := call(r, "show_image", map[string]any{"path": fp})
		assertOK(t, "show_image/png", res, err)
		if !strings.Contains(res, "img.png") {
			t.Errorf("result should mention filename, got %q", res)
		}
	})

	t.Run("unsupported_type", func(t *testing.T) {
		fp := filepath.Join(ws, "doc.pdf")
		os.WriteFile(fp, []byte("fakepdf"), 0644)
		_, err := call(r, "show_image", map[string]any{"path": fp})
		assertErr(t, "show_image/pdf", "unsupported file type", err)
	})

	t.Run("file_not_found", func(t *testing.T) {
		_, err := call(r, "show_image", map[string]any{"path": "/tmp/notexist_xyz.png"})
		assertErr(t, "show_image/not-found", "not found", err)
	})

	t.Run("empty_path", func(t *testing.T) {
		_, err := call(r, "show_image", map[string]any{"path": ""})
		assertErr(t, "show_image/no-path", "path required", err)
	})
}
