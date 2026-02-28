// pkg/tools/memory_search.go — memory_search built-in tool.
// Registers memory_search on the Registry via WithMemorySearch().
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/memory"
)

var memorySearchToolDef = llm.ToolDef{
	Name: "memory_search",
	Description: "语义搜索 agent 的记忆文件（memory/ 目录下所有 .md 文件）。" +
		"返回与查询最相关的文本片段，包含来源文件路径和行号。" +
		"当需要回忆过去的对话、决策、用户偏好、项目信息时使用此工具。" +
		"支持自然语言查询（中英文均可）。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "搜索查询，用自然语言描述你要找的信息"
			},
			"top_k": {
				"type": "integer",
				"description": "返回的结果数量（默认 5，最大 20）",
				"default": 5
			}
		},
		"required": ["query"]
	}`),
}

// WithMemorySearch registers the memory_search tool on this Registry.
//
// memTree  — the agent's MemoryTree (workspace dir already set).
// embedder — optional embedding client; nil = BM25-only mode.
// apiKey   — API key for the embedding provider; ignored when embedder is nil.
//
// On first use the index is loaded from disk (or built on-the-fly if missing).
// A background rebuild is triggered when the index is stale.
func (r *Registry) WithMemorySearch(memTree *memory.MemoryTree, embedder *llm.Embedder, apiKey string) {
	// Trigger an initial async index build/refresh at registration time
	memory.RebuildIndexIfStale(memTree, embedder, apiKey)

	r.register(memorySearchToolDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Query string `json:"query"`
			TopK  int    `json:"top_k"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if strings.TrimSpace(p.Query) == "" {
			return "", fmt.Errorf("query 不能为空")
		}
		topK := p.TopK
		if topK <= 0 {
			topK = 5
		}
		if topK > 20 {
			topK = 20
		}

		// Load index (may be empty if not yet built)
		idx, err := memTree.LoadIndex()
		if err != nil || len(idx.Chunks) == 0 {
			// 没有索引时同步构建（首次调用）
			idx, err = memory.BuildIndex(ctx, memTree, embedder, apiKey)
			if err != nil {
				return "", fmt.Errorf("构建记忆索引失败: %w", err)
			}
			_ = memTree.SaveIndex(idx)
		}

		// If stale, trigger background rebuild for next call
		if memTree.IsStale(idx) {
			memory.RebuildIndexIfStale(memTree, embedder, apiKey)
		}

		// Optionally embed the query for vector search
		var queryVec []float32
		if embedder != nil && len(idx.Chunks) > 0 && len(idx.Chunks[0].Vec) > 0 {
			vecs, embedErr := embedder.Embed(ctx, apiKey, []string{p.Query})
			if embedErr == nil && len(vecs) > 0 {
				queryVec = vecs[0]
			}
			// Embed failure → silently fall back to BM25
		}

		results := idx.Search(queryVec, p.Query, topK)
		if len(results) == 0 {
			return "（未找到相关记忆）", nil
		}

		var sb strings.Builder
		mode := "BM25"
		if queryVec != nil {
			mode = embedder.Model()
		}
		sb.WriteString(fmt.Sprintf("找到 %d 条相关记忆（搜索模式: %s）：\n\n", len(results), mode))
		for i, c := range results {
			sb.WriteString(fmt.Sprintf("[%d] %s:%d\n%s\n\n", i+1, c.Source, c.Line, strings.TrimSpace(c.Text)))
		}
		return strings.TrimRight(sb.String(), "\n"), nil
	})
}
