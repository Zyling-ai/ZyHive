package session

import (
	"encoding/json"
	"testing"
)

// TestFixOrphanedToolUse 验证孤立 tool_use 清理逻辑。
func TestFixOrphanedToolUse(t *testing.T) {
	t.Run("无消息时不报错", func(t *testing.T) {
		result := fixOrphanedToolUse(nil)
		if len(result) != 0 {
			t.Errorf("expected empty, got %d messages", len(result))
		}
	})

	t.Run("无 tool_use 块时不修改", func(t *testing.T) {
		msgs := []Message{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"hello"}]`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"world"}]`)},
		}
		result := fixOrphanedToolUse(msgs)
		if len(result) != 2 {
			t.Errorf("expected 2 messages, got %d", len(result))
		}
	})

	t.Run("tool_use 已有对应 tool_result 时不追加", func(t *testing.T) {
		msgs := []Message{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"run tool"}]`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"tool_1","name":"bash","input":{}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"tool_1","content":"ok"}]`)},
		}
		result := fixOrphanedToolUse(msgs)
		if len(result) != 3 {
			t.Errorf("expected 3 messages (no synthetic added), got %d", len(result))
		}
	})

	t.Run("孤立 tool_use 时追加 synthetic tool_result", func(t *testing.T) {
		msgs := []Message{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"run tool"}]`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"tool_abc","name":"bash","input":{}}]`)},
			// 没有 tool_result
		}
		result := fixOrphanedToolUse(msgs)
		if len(result) != 3 {
			t.Fatalf("expected 3 messages (synthetic added), got %d", len(result))
		}
		synthetic := result[2]
		if synthetic.Role != "user" {
			t.Errorf("expected synthetic role=user, got %s", synthetic.Role)
		}
		var blocks []ContentBlock
		if err := json.Unmarshal(synthetic.Content, &blocks); err != nil {
			t.Fatalf("failed to unmarshal synthetic content: %v", err)
		}
		if len(blocks) != 1 {
			t.Fatalf("expected 1 block, got %d", len(blocks))
		}
		b := blocks[0]
		if b.Type != "tool_result" {
			t.Errorf("expected type=tool_result, got %s", b.Type)
		}
		if b.ToolUseID != "tool_abc" {
			t.Errorf("expected tool_use_id=tool_abc, got %s", b.ToolUseID)
		}
		if !b.IsError {
			t.Errorf("expected is_error=true")
		}
	})

	t.Run("多个孤立 tool_use 都被修复", func(t *testing.T) {
		assistantContent := `[
			{"type":"tool_use","id":"t1","name":"bash","input":{}},
			{"type":"tool_use","id":"t2","name":"read","input":{}}
		]`
		msgs := []Message{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"go"}]`)},
			{Role: "assistant", Content: json.RawMessage(assistantContent)},
		}
		result := fixOrphanedToolUse(msgs)
		if len(result) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(result))
		}
		var blocks []ContentBlock
		if err := json.Unmarshal(result[2].Content, &blocks); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if len(blocks) != 2 {
			t.Errorf("expected 2 synthetic blocks, got %d", len(blocks))
		}
		ids := map[string]bool{}
		for _, b := range blocks {
			ids[b.ToolUseID] = true
		}
		if !ids["t1"] || !ids["t2"] {
			t.Errorf("expected t1 and t2 in synthetic blocks, got %v", ids)
		}
	})

	t.Run("部分 tool_use 有 tool_result 时只修复孤立的", func(t *testing.T) {
		assistantContent := `[
			{"type":"tool_use","id":"t1","name":"bash","input":{}},
			{"type":"tool_use","id":"t2","name":"read","input":{}}
		]`
		msgs := []Message{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"go"}]`)},
			{Role: "assistant", Content: json.RawMessage(assistantContent)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"t1","content":"done"}]`)},
			// t2 无 tool_result
		}
		result := fixOrphanedToolUse(msgs)
		if len(result) != 4 {
			t.Fatalf("expected 4 messages, got %d", len(result))
		}
		var blocks []ContentBlock
		if err := json.Unmarshal(result[3].Content, &blocks); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if len(blocks) != 1 {
			t.Fatalf("expected 1 synthetic block (only t2), got %d", len(blocks))
		}
		if blocks[0].ToolUseID != "t2" {
			t.Errorf("expected synthetic for t2, got %s", blocks[0].ToolUseID)
		}
	})

	t.Run("只有 user 消息时不修改", func(t *testing.T) {
		msgs := []Message{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"hi"}]`)},
		}
		result := fixOrphanedToolUse(msgs)
		if len(result) != 1 {
			t.Errorf("expected 1, got %d", len(result))
		}
	})
}
