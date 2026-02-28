// pkg/memory/search.go — Vector + BM25 hybrid search index for agent memory files.
package memory

import (
	"encoding/gob"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const searchIndexFile = ".search_index.gob"

// Chunk is a single indexed memory fragment.
type Chunk struct {
	Text   string    // 段落原文
	Source string    // 相对于 workspace 的路径，如 "memory/core/knowledge.md"
	Line   int       // 在源文件中的起始行号（1-indexed）
	Vec    []float32 // embedding 向量；nil = 仅 BM25 模式
}

// SearchIndex holds all indexed chunks for one agent workspace.
type SearchIndex struct {
	Version   int     // schema 版本，当前 = 1
	IndexedAt int64   // unix ms，用于 stale 判断
	Chunks    []Chunk
}

// indexPath returns the absolute path to the search index gob file.
func (m *MemoryTree) indexPath() string {
	return filepath.Join(m.memDir(), searchIndexFile)
}

// LoadIndex loads the search index from disk.
// Returns an empty (non-nil) index if the file doesn't exist or is corrupt.
func (m *MemoryTree) LoadIndex() (*SearchIndex, error) {
	f, err := os.Open(m.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &SearchIndex{}, nil
		}
		return nil, err
	}
	defer f.Close()
	var idx SearchIndex
	if err := gob.NewDecoder(f).Decode(&idx); err != nil {
		return &SearchIndex{}, nil // 损坏 → 当空处理
	}
	return &idx, nil
}

// SaveIndex writes the index to disk (memory/.search_index.gob).
func (m *MemoryTree) SaveIndex(idx *SearchIndex) error {
	p := m.indexPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return gob.NewEncoder(f).Encode(idx)
}

// IsStale returns true when the index is older than any .md file under memory/.
func (m *MemoryTree) IsStale(idx *SearchIndex) bool {
	if idx == nil || idx.IndexedAt == 0 {
		return true
	}
	return m.anyNewerThan(m.memDir(), time.UnixMilli(idx.IndexedAt))
}

func (m *MemoryTree) anyNewerThan(dir string, t time.Time) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if name == searchIndexFile {
			continue
		}
		abs := filepath.Join(dir, name)
		if e.IsDir() {
			if m.anyNewerThan(abs, t) {
				return true
			}
		} else if strings.HasSuffix(name, ".md") {
			fi, err := e.Info()
			if err == nil && fi.ModTime().After(t) {
				return true
			}
		}
	}
	return false
}

// Search returns the top-K most relevant chunks for the given query.
//
// If chunks have Vec and queryVec is non-nil → cosine similarity.
// Otherwise → BM25 keyword scoring (Chinese + English both supported).
func (idx *SearchIndex) Search(queryVec []float32, query string, topK int) []Chunk {
	if len(idx.Chunks) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = 5
	}

	type scored struct {
		chunk Chunk
		score float64
	}

	// Decide mode: use vectors if available
	hasVec := len(idx.Chunks) > 0 && len(idx.Chunks[0].Vec) > 0

	var scores []scored

	if hasVec && queryVec != nil {
		// ── Cosine similarity ───────────────────────────────────────────
		for _, c := range idx.Chunks {
			if len(c.Vec) == 0 {
				continue
			}
			scores = append(scores, scored{c, cosineSim(queryVec, c.Vec)})
		}
	} else {
		// ── BM25 keyword scoring ─────────────────────────────────────────
		terms := tokenize(query)
		if len(terms) == 0 {
			n := min(topK, len(idx.Chunks))
			return idx.Chunks[:n]
		}

		N := float64(len(idx.Chunks))
		// Precompute IDF per term
		idf := make(map[string]float64, len(terms))
		for _, term := range terms {
			df := 0
			for _, c := range idx.Chunks {
				if strings.Contains(strings.ToLower(c.Text), term) {
					df++
				}
			}
			if df > 0 {
				idf[term] = math.Log(1 + N/float64(df))
			}
		}

		k1, b := 1.5, 0.75 // BM25 params
		// Estimate average doc length
		totalWords := 0
		for _, c := range idx.Chunks {
			totalWords += len(strings.Fields(c.Text))
		}
		avgdl := float64(totalWords) / N

		for _, c := range idx.Chunks {
			lower := strings.ToLower(c.Text)
			dl := float64(len(strings.Fields(c.Text)))
			score := 0.0
			for _, term := range terms {
				tf := float64(strings.Count(lower, term))
				if tf == 0 {
					continue
				}
				idfW := idf[term]
				// BM25 TF normalization
				tfNorm := tf * (k1 + 1) / (tf + k1*(1-b+b*dl/avgdl))
				score += idfW * tfNorm
			}
			if score > 0 {
				scores = append(scores, scored{c, score})
			}
		}
	}

	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })

	result := make([]Chunk, 0, topK)
	for i := 0; i < topK && i < len(scores); i++ {
		result = append(result, scores[i].chunk)
	}
	return result
}

// ── Math helpers ────────────────────────────────────────────────────────────

// cosineSim computes the cosine similarity between two float32 vectors.
func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		fa, fb := float64(a[i]), float64(b[i])
		dot += fa * fb
		na += fa * fa
		nb += fb * fb
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// tokenize splits a query string into lowercase terms (Chinese + English).
func tokenize(s string) []string {
	s = strings.ToLower(s)
	// Replace common CJK / Latin punctuation with space
	replacer := strings.NewReplacer(
		",", " ", ".", " ", "!", " ", "?", " ",
		"；", " ", "，", " ", "。", " ", "、", " ",
		"：", " ", "（", " ", "）", " ", "(", " ", ")", " ",
		"「", " ", "」", " ", "【", " ", "】", " ",
		"\t", " ", "\n", " ",
	)
	s = replacer.Replace(s)

	var terms []string
	for _, p := range strings.Fields(s) {
		if len([]rune(p)) >= 2 { // skip single-char noise
			terms = append(terms, p)
		}
	}
	return terms
}
