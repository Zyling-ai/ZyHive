package skillopt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const criticSystemPrompt = `你是「技能复盘官」。下面是某个 AI 技能做出的若干**预测**以及它们对应的**真实结果**（这些都是预测失败/未命中的样本）。
请逐条分析失败原因，输出归因标签与可执行教训，用于改进该技能。

严格只输出一个 JSON 数组，不要任何解释性文字、不要 markdown 代码块。格式：
[
  {"entryId": "<原样回填的预测ID>", "tags": ["归因标签1","归因标签2"], "lesson": "一句话可执行教训（<=40字）"}
]

要求：
- entryId 必须与输入中的 ID 完全一致；
- tags 用简短中文名词（如「忽略主场优势」「样本过期」「过度自信」）；
- lesson 必须是可落到规则里的具体行动，不要空话；
- 只输出 JSON 数组本身。`

// Critique asks the LLM to attribute each missed prediction to root-cause tags
// and a concrete lesson. The returned slice aligns to input entries by ID; any
// entry the model omits is simply skipped.
func Critique(ctx context.Context, misses []LedgerEntry, callLLM CallLLM) ([]Attribution, error) {
	if len(misses) == 0 {
		return []Attribution{}, nil
	}
	if callLLM == nil {
		return nil, fmt.Errorf("skillopt: critic needs a CallLLM")
	}

	var sb strings.Builder
	for _, e := range misses {
		sb.WriteString(fmt.Sprintf("ID: %s\n", e.ID))
		if e.ContextDigest != "" {
			sb.WriteString(fmt.Sprintf("预测时上下文: %s\n", e.ContextDigest))
		}
		sb.WriteString(fmt.Sprintf("预测: %s\n", e.Prediction))
		sb.WriteString(fmt.Sprintf("真实结果: %s\n\n", e.Oracle))
	}

	raw, err := callLLM(ctx, criticSystemPrompt, sb.String())
	if err != nil {
		return nil, fmt.Errorf("skillopt critic llm: %w", err)
	}

	attrs, err := parseAttributions(raw)
	if err != nil {
		return nil, err
	}

	// Keep only attributions whose entryId exists in the input (defends against
	// hallucinated IDs).
	valid := make(map[string]bool, len(misses))
	for _, e := range misses {
		valid[e.ID] = true
	}
	out := make([]Attribution, 0, len(attrs))
	for _, a := range attrs {
		if valid[a.EntryID] && a.Lesson != "" {
			out = append(out, a)
		}
	}
	return out, nil
}

// parseAttributions tolerantly extracts the JSON array from an LLM reply that
// may be wrapped in prose or ```json fences.
func parseAttributions(raw string) ([]Attribution, error) {
	js := extractJSONArray(raw)
	if js == "" {
		return nil, fmt.Errorf("skillopt critic: no JSON array in reply")
	}
	var attrs []Attribution
	if err := json.Unmarshal([]byte(js), &attrs); err != nil {
		return nil, fmt.Errorf("skillopt critic: parse attributions: %w", err)
	}
	return attrs, nil
}

// extractJSONArray returns the substring from the first '[' to its matching ']'.
func extractJSONArray(s string) string {
	start := strings.IndexByte(s, '[')
	end := strings.LastIndexByte(s, ']')
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}
