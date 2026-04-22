// pkg/llm/health.go — Live Provider health check (ping) with 30s caching.
//
// Unlike test-key (user-initiated explicit test on the config page), this is
// invoked by the Tool Health card to verify that the LLM provider is actually
// reachable RIGHT NOW — no stale "configured but API's down" confusion.
//
// The ping sends a minimal-cost Chat request (max_tokens=1) rather than calling
// /models: this works for all providers including ones that don't have /models
// (e.g. MiniMax). Cost per ping ≈ 10 input + 1 output tokens.
//
// The 30s cache means a user refreshing the Agent detail page repeatedly
// won't burn tokens — and the ToolHealth handler can safely call Ping() on
// every request.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// PingResult holds the outcome of a single provider probe.
type PingResult struct {
	OK      bool          `json:"ok"`
	Latency time.Duration `json:"latencyMs"`
	Error   string        `json:"error,omitempty"`
	// StatusCode is the HTTP status returned (useful for 401 vs 429 vs 500 distinction).
	StatusCode int `json:"statusCode,omitempty"`
	// CheckedAt is when this result was produced (monotonic time is fine; only
	// used internally for cache expiration).
	CheckedAt time.Time `json:"checkedAt"`
}

// pingCacheTTL is how long a successful OR failed ping result is considered fresh.
// 30s is short enough that real outages are detected quickly, long enough that
// rapid UI refreshes don't spam the provider.
const pingCacheTTL = 30 * time.Second

var (
	pingCache   = make(map[string]*PingResult)
	pingCacheMu sync.RWMutex
)

// pingCacheKey — avoid leaking full API key in cache; use first 12 + last 4 chars.
func pingCacheKey(provider, apiKey, baseURL string) string {
	head := apiKey
	if len(head) > 12 {
		head = head[:12]
	}
	tail := ""
	if len(apiKey) > 4 {
		tail = apiKey[len(apiKey)-4:]
	}
	return provider + "|" + baseURL + "|" + head + "…" + tail
}

// Ping probes whether the given Provider + API key + baseURL can handle a
// minimal Chat request. Results are cached for `pingCacheTTL`.
//
// Returns quickly from cache when fresh. Callers can force a refresh by
// passing forceRefresh=true (e.g. user clicks "retry" button).
func Ping(ctx context.Context, provider, apiKey, baseURL string, forceRefresh bool) *PingResult {
	key := pingCacheKey(provider, apiKey, baseURL)
	if !forceRefresh {
		pingCacheMu.RLock()
		if cached, ok := pingCache[key]; ok && time.Since(cached.CheckedAt) < pingCacheTTL {
			pingCacheMu.RUnlock()
			return cached
		}
		pingCacheMu.RUnlock()
	}

	result := runPing(ctx, provider, apiKey, baseURL)
	result.CheckedAt = time.Now()
	pingCacheMu.Lock()
	pingCache[key] = result
	pingCacheMu.Unlock()
	return result
}

// runPing actually makes the HTTP request. Separated from Ping() so tests can
// bypass the cache.
func runPing(ctx context.Context, provider, apiKey, baseURL string) *PingResult {
	start := time.Now()
	// Use a short timeout — we're just asking "are you alive". Don't hog the
	// user's browser for 30s if the provider is stuck.
	tctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	ok, status, errMsg := pingProvider(tctx, provider, apiKey, baseURL)
	return &PingResult{
		OK:         ok,
		StatusCode: status,
		Latency:    time.Since(start),
		Error:      errMsg,
	}
}

// pingProvider dispatches to the per-provider HTTP probe.
// Returns (ok, httpStatus, errMessage).
func pingProvider(ctx context.Context, provider, apiKey, baseURL string) (bool, int, string) {
	switch strings.ToLower(provider) {
	case "anthropic":
		return pingAnthropic(ctx, apiKey, baseURL)
	case "openai", "deepseek", "moonshot", "kimi", "zhipu", "qwen", "openrouter", "minimax", "custom":
		// OpenAI-compatible: /v1/chat/completions with max_tokens=1
		return pingOpenAICompat(ctx, apiKey, baseURL, defaultModelForProvider(provider))
	}
	return false, 0, "unsupported provider: " + provider
}

func defaultModelForProvider(provider string) string {
	switch strings.ToLower(provider) {
	case "openai":
		return "gpt-4o-mini"
	case "deepseek":
		return "deepseek-chat"
	case "moonshot", "kimi":
		return "moonshot-v1-8k"
	case "zhipu":
		return "glm-4-flash"
	case "qwen":
		return "qwen-turbo"
	case "openrouter":
		return "openai/gpt-4o-mini"
	case "minimax":
		return "abab6.5s-chat"
	}
	return ""
}

func pingAnthropic(ctx context.Context, apiKey, baseURL string) (bool, int, string) {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.Contains(baseURL, "/v1") {
		baseURL += "/v1"
	}
	payload, _ := json.Marshal(map[string]any{
		"model":      "claude-haiku-4-20250514", // cheapest available
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/messages", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, 0, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return true, 200, ""
	}
	// 404 on model name (we used haiku-4 which may not exist for all accounts)
	// still proves the endpoint is alive and credentials valid → OK.
	if resp.StatusCode == 404 {
		return true, 404, ""
	}
	return false, resp.StatusCode, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func pingOpenAICompat(ctx context.Context, apiKey, baseURL, model string) (bool, int, string) {
	if baseURL == "" {
		return false, 0, "baseURL required for OpenAI-compatible providers"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.Contains(baseURL, "/v1") {
		baseURL += "/v1"
	}
	payload, _ := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, 0, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return true, 200, ""
	}
	// 400 bad_request with detail "invalid model" still means creds worked.
	// Distinguish from 401 (auth failure) and 5xx (provider down).
	if resp.StatusCode >= 500 {
		return false, resp.StatusCode, fmt.Sprintf("provider returned %d", resp.StatusCode)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return false, resp.StatusCode, "authentication failed"
	}
	if resp.StatusCode == 429 {
		return false, resp.StatusCode, "rate limited"
	}
	// 400 / 404 on model → credentials OK, model not available → treat as alive
	return true, resp.StatusCode, ""
}

// ClearPingCache is exposed for tests.
func ClearPingCache() {
	pingCacheMu.Lock()
	pingCache = make(map[string]*PingResult)
	pingCacheMu.Unlock()
}
