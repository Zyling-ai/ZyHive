package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

var webSearchToolDef = llm.ToolDef{
	Name:        "web_search",
	Description: "Search the web using Brave Search API. Returns titles, URLs, and snippets.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"query":{"type":"string","description":"Search query"},
			"count":{"type":"number","description":"Number of results (1-10, default 5)"},
			"freshness":{"type":"string","description":"Filter by time: pd=past day, pw=past week, pm=past month, py=past year"}
		},
		"required":["query"]
	}`),
}

// WithWebSearch registers the web_search tool backed by the given Brave API key.
// If apiKey is empty, the tool is not registered.
func (r *Registry) WithWebSearch(apiKey string) {
	if apiKey == "" {
		return
	}
	r.register(webSearchToolDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return handleWebSearch(ctx, input, apiKey)
	})
}

func handleWebSearch(_ context.Context, input json.RawMessage, apiKey string) (string, error) {
	var p struct {
		Query     string `json:"query"`
		Count     int    `json:"count"`
		Freshness string `json:"freshness"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("web_search: invalid input: %v", err)
	}
	if p.Query == "" {
		return "", fmt.Errorf("web_search: query is required")
	}
	if p.Count <= 0 || p.Count > 10 {
		p.Count = 5
	}

	params := url.Values{}
	params.Set("q", p.Query)
	params.Set("count", fmt.Sprintf("%d", p.Count))
	if p.Freshness != "" {
		params.Set("freshness", p.Freshness)
	}

	reqURL := "https://api.search.brave.com/res/v1/web/search?" + params.Encode()
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("web_search: build request: %v", err)
	}
	req.Header.Set("X-Subscription-Token", apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("web_search: request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("web_search: API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("web_search: decode response: %v", err)
	}

	if len(result.Web.Results) == 0 {
		return "No results found.", nil
	}

	var sb strings.Builder
	for i, r := range result.Web.Results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description))
	}
	return sb.String(), nil
}
