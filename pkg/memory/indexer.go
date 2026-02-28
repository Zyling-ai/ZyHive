// pkg/memory/indexer.go — Chunks memory .md files and (optionally) embeds them.
package memory

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

const (
	maxChunkBytes  = 600 // 软上限：每块最大字节数（段落中途超限时强制分割）
	minChunkBytes  = 20  // 低于此长度的片段丢弃（噪声）
	embedBatchSize = 64  // 单次 embedding API 调用的最大文本数
)

// BuildIndex scans all .md files under memory/, chunks them into paragraphs,
// optionally embeds them (embedder may be nil → BM25-only mode),
// and returns the new SearchIndex.
//
// apiKey is the key for the embedding API call; ignored when embedder is nil.
func BuildIndex(ctx context.Context, memTree *MemoryTree, embedder *llm.Embedder, apiKey string) (*SearchIndex, error) {
	chunks, err := chunkAllFiles(memTree)
	if err != nil {
		return nil, err
	}

	if embedder != nil && len(chunks) > 0 {
		texts := make([]string, len(chunks))
		for i, c := range chunks {
			texts[i] = c.Text
		}
		vecs, embedErr := batchEmbed(ctx, embedder, apiKey, texts)
		if embedErr != nil {
			log.Printf("[memory/index] embedding failed (falling back to BM25): %v", embedErr)
			// 继续构建，只是没有向量
		} else {
			for i := range chunks {
				if i < len(vecs) && len(vecs[i]) > 0 {
					chunks[i].Vec = vecs[i]
				}
			}
		}
	}

	return &SearchIndex{
		Version:   1,
		IndexedAt: time.Now().UnixMilli(),
		Chunks:    chunks,
	}, nil
}

// RebuildIndexIfStale checks whether the on-disk index is stale and, if so,
// rebuilds it asynchronously in the background. Non-blocking.
// embedder / apiKey may be zero-value (BM25-only mode).
func RebuildIndexIfStale(memTree *MemoryTree, embedder *llm.Embedder, apiKey string) {
	go func() {
		idx, err := memTree.LoadIndex()
		if err != nil || !memTree.IsStale(idx) {
			return
		}
		newIdx, err := BuildIndex(context.Background(), memTree, embedder, apiKey)
		if err != nil {
			log.Printf("[memory/index] rebuild error: %v", err)
			return
		}
		if err := memTree.SaveIndex(newIdx); err != nil {
			log.Printf("[memory/index] save error: %v", err)
			return
		}
		mode := "BM25"
		if embedder != nil && len(newIdx.Chunks) > 0 && len(newIdx.Chunks[0].Vec) > 0 {
			mode = embedder.Model()
		}
		log.Printf("[memory/index] rebuilt: %d chunks, mode=%s", len(newIdx.Chunks), mode)
	}()
}

// ── Internal helpers ─────────────────────────────────────────────────────────

// chunkAllFiles walks memory/ and splits every .md file into paragraph chunks.
func chunkAllFiles(memTree *MemoryTree) ([]Chunk, error) {
	memDir := memTree.memDir()
	if _, err := os.Stat(memDir); os.IsNotExist(err) {
		return nil, nil
	}

	var chunks []Chunk
	err := filepath.Walk(memDir, func(abs string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		// Skip hidden files (.search_index.gob etc.) and non-markdown
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".md") {
			return nil
		}
		rel, _ := filepath.Rel(memTree.WorkspaceDir, abs)
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil // best-effort
		}
		chunks = append(chunks, splitIntoChunks(string(data), rel)...)
		return nil
	})
	return chunks, err
}

// splitIntoChunks splits file content into paragraph-sized chunks.
// source is the workspace-relative path (e.g. "memory/core/knowledge.md").
func splitIntoChunks(content, source string) []Chunk {
	lines := strings.Split(content, "\n")
	var (
		chunks    []Chunk
		buf       strings.Builder
		startLine int = 1
	)

	flush := func(lineNum int) {
		text := strings.TrimSpace(buf.String())
		if len(text) >= minChunkBytes {
			chunks = append(chunks, Chunk{
				Text:   text,
				Source: source,
				Line:   startLine,
			})
		}
		buf.Reset()
		startLine = lineNum + 1
	}

	for i, line := range lines {
		lineNum := i + 1
		isBlank := strings.TrimSpace(line) == ""

		if isBlank {
			if buf.Len() > 0 {
				flush(lineNum)
			}
		} else {
			if buf.Len() == 0 {
				startLine = lineNum
			}
			buf.WriteString(line)
			buf.WriteByte('\n')
			// Force-split oversized paragraphs
			if buf.Len() >= maxChunkBytes {
				flush(lineNum)
			}
		}
	}
	if buf.Len() > 0 {
		flush(len(lines))
	}
	return chunks
}

// batchEmbed calls the embedding API in batches of embedBatchSize.
func batchEmbed(ctx context.Context, embedder *llm.Embedder, apiKey string, texts []string) ([][]float32, error) {
	all := make([][]float32, len(texts))
	for start := 0; start < len(texts); start += embedBatchSize {
		end := start + embedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := embedder.Embed(ctx, apiKey, texts[start:end])
		if err != nil {
			return nil, err
		}
		for i, v := range vecs {
			all[start+i] = v
		}
	}
	return all, nil
}
