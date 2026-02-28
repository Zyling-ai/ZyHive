// pkg/llm/embed.go — Embedding API support (OpenAI-compatible /v1/embeddings).
// Supports: openai, zhipu, minimax, and any custom OpenAI-compatible baseURL.
// Returns nil from NewEmbedder when the provider doesn't support embeddings.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// embedProviderSpec maps known provider names to their default embedding endpoint + model.
type embedProviderSpec struct {
	DefaultBaseURL string
	Model          string
}

// knownEmbedProviders lists providers with known /v1/embeddings support.
var knownEmbedProviders = map[string]embedProviderSpec{
	"openai":  {"https://api.openai.com/v1", "text-embedding-3-small"},
	"zhipu":   {"https://open.bigmodel.cn/api/paas/v4", "embedding-2"},
	"minimax": {"https://api.minimax.chat/v1", "embo-01"},
}

// Embedder calls an OpenAI-compatible /v1/embeddings endpoint.
type Embedder struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewEmbedder creates an Embedder for the given provider.
// provider: "openai" | "zhipu" | "minimax" | "custom"
// baseURL: override default (empty = use default for known providers)
// apiKey: passed at call time via Embed(); stored here for convenience.
// Returns nil if the provider is not known and no baseURL is provided.
func NewEmbedder(provider, baseURL string) *Embedder {
	spec, known := knownEmbedProviders[provider]
	if !known && baseURL == "" {
		return nil
	}

	effectiveURL := baseURL
	if effectiveURL == "" {
		effectiveURL = spec.DefaultBaseURL
	}
	// Normalize: strip trailing slash, ensure /v1 suffix
	effectiveURL = strings.TrimRight(effectiveURL, "/")
	if !strings.HasSuffix(effectiveURL, "/v1") {
		// If it already ends with /v1/something, don't double-add
		if !strings.Contains(effectiveURL[max(0, len(effectiveURL)-20):], "/v1") {
			effectiveURL += "/v1"
		}
	}

	model := spec.Model
	if !known {
		model = "text-embedding-3-small" // sensible default for custom OpenAI-compat
	}

	return &Embedder{
		baseURL: effectiveURL,
		model:   model,
		client:  &http.Client{},
	}
}

// Model returns the embedding model name used by this embedder.
func (e *Embedder) Model() string { return e.model }

// Embed encodes a batch of texts and returns float32 vectors.
// Output order matches input order.
func (e *Embedder) Embed(ctx context.Context, apiKey string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	payload := map[string]interface{}{
		"model": e.model,
		"input": texts,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("embed API %d: %s", resp.StatusCode, string(errBody))
	}

	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}

	vecs := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index >= 0 && d.Index < len(vecs) {
			vecs[d.Index] = d.Embedding
		}
	}
	return vecs, nil
}

// SupportsEmbedding reports whether the given provider name has a known embedding endpoint.
func SupportsEmbedding(provider string) bool {
	_, ok := knownEmbedProviders[provider]
	return ok
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
